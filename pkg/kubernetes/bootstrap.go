package kubernetes

import (
	"github.com/catalystcommunity/app-utils-go/errorutils"
	"github.com/catalystcommunity/pulumi-modules-go/pkg/eks"
	"github.com/catalystcommunity/pulumi-modules-go/pkg/templates"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

type PlatformApplicationConfig struct {
	Enabled        bool
	TargetRevision string
	SyncPolicy     ArgocdApplicationSyncPolicy
	Values         string
}

type K8sPlatformConfigInput struct {
	// user input
	ArgocdHelm              HelmReleaseConfigInput `json:"argocd-helm-release"`
	KubePrometheusStackHelm HelmReleaseConfigInput `json:"kube-prometheus-stack-helm-release"`

	// optional, enable management of eks auth config
	ManageEksAuthConfigMap bool `json:"manage-eks-auth-configmap"`

	// optional, management of prometheus remote write basic auth secret
	ManagePrometheusRemoteWriteBasicAuthSecret bool `json:"manage-prometheus-remote-write-basic-auth-secret"`
	// defaults to stack name
	PrometheusRemoteWriteBasicAuthUsername string `json:"prometheus-remote-write-basic-auth-username"`
	// defaults to "prometheus-remote-write-basic-auth"
	PrometheusRemoteWriteSecretName string `json:"prometheus-remote-write-basic-auth-secret-name"`

	// input from eks module
	KubeConfig pulumi.StringOutput
}

type HelmReleaseConfigInput struct {
	Version     string   `json:"version"`
	ValuesFiles []string `json:"values-files"`
}

// BootstrapCluster installs argo-cd and kube-prometheus-stack as helm charts, bootstraps the aws-auth configmap, and
// installs the catalyst squad platform-services chart as an argocd application. Configurations set on stacks are respected.
func BootstrapCluster(ctx *pulumi.Context) error {
	var k8sConfig K8sPlatformConfigInput
	// get config
	cfg := config.New(ctx, "")
	err := cfg.GetObject("k8s", &k8sConfig)
	errorutils.LogOnErr(nil, "error marshalling config to struct", err)
	if err != nil {
		return err
	}

	// manage aws auth configmap, require additional configuration object if enabled
	if k8sConfig.ManageEksAuthConfigMap {
		var eksAuthConfig eks.AuthConfigMapInput
		err = cfg.GetObject("eks-auth", &eksAuthConfig)
		if err != nil {
			return err
		}

		err = eks.SyncAuthConfigMap(ctx, eksAuthConfig)
		if err != nil {
			return err
		}
	}

	// deploy kube-prometheus-stack remote-write basic auth secret
	prometheusRemoteWriteSecret, err := deployPrometheusRemoteWriteBasicAuthSecret(ctx, cfg, k8sConfig)
	errorutils.LogOnErr(nil, "error deploying kube-prometheus-stack remote-write basic auth secret", err)
	if err != nil {
		return err
	}

	// dynamic depends on for an optional resource
	var prometheusDependsOn pulumi.ResourceOption
	if prometheusRemoteWriteSecret != nil {
		prometheusDependsOn = pulumi.DependsOn([]pulumi.Resource{prometheusRemoteWriteSecret})
	}

	// deploy kube-prometheus-stack, this should happen first because the argo-cd helm chart installs service monitors
	prometheus, err := deployKubePrometheusStack(ctx, k8sConfig, prometheusDependsOn)
	errorutils.LogOnErr(nil, "error deploying kube-prometheus-stack", err)
	if err != nil {
		return err
	}

	// deploy argocd
	argocd, err := deployArgocd(ctx, cfg, k8sConfig, pulumi.DependsOn([]pulumi.Resource{prometheus})) // this helm chart installs service monitors, so it depends on kube-prometheus-stack
	errorutils.LogOnErr(nil, "error deploying argocd", err)
	if err != nil {
		return err
	}

	// deploy cluster argocd application
	platformApplication, err := deployPlatformApplicationManifest(ctx, pulumi.DependsOn([]pulumi.Resource{argocd})) // depend on argocd for application CRDs
	errorutils.LogOnErr(nil, "error deploying cluster application manifest", err)
	if err != nil {
		return err
	}

	// create cert-manager dns secret
	err = deployCertManagerDnsSolverSecret(ctx, pulumi.DependsOn([]pulumi.Resource{platformApplication}))
	errorutils.LogOnErr(nil, "error deploying cert manager dns solver secret", err)
	return err
}

func deployPrometheusRemoteWriteBasicAuthSecret(ctx *pulumi.Context, cfg *config.Config, k8sConfig K8sPlatformConfigInput) (pulumi.Resource, error) {
	if k8sConfig.ManagePrometheusRemoteWriteBasicAuthSecret {
		username := ctx.Stack()
		if k8sConfig.PrometheusRemoteWriteBasicAuthUsername != "" {
			username = k8sConfig.PrometheusRemoteWriteBasicAuthUsername
		}

		secretName := "prometheus-remote-write-basic-auth"
		if k8sConfig.PrometheusRemoteWriteSecretName != "" {
			secretName = k8sConfig.PrometheusRemoteWriteSecretName
		}

		secret, err := corev1.NewSecret(ctx, "prometheus-remote-write-basic-auth-secret", &corev1.SecretArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String(secretName),
				Namespace: pulumi.String("kube-prometheus-stack"),
			},
			StringData: pulumi.StringMap{
				"username": pulumi.String(username),
				"password": cfg.RequireSecret("prometheusRemoteWriteBasicAuthPassword"),
			},
		})
		return secret, err
	}

	return nil, nil
}

func deployArgocd(ctx *pulumi.Context, cfg *config.Config, k8sConfig K8sPlatformConfigInput, opts ...pulumi.ResourceOption) (pulumi.Resource, error) {
	// set default helm chart versions if not defined
	argocdVersion := "3.33.8"
	if k8sConfig.ArgocdHelm.Version != "" {
		argocdVersion = k8sConfig.ArgocdHelm.Version
	}

	// set default helm values files if not defined
	argocdValues := []string{
		"./helm-values/argo-cd-values.yaml",
	}
	if len(k8sConfig.ArgocdHelm.ValuesFiles) != 0 {
		argocdValues = k8sConfig.ArgocdHelm.ValuesFiles
	}

	// deploy argo using helm
	argocd, err := helm.NewRelease(ctx, "argo-cd", &helm.ReleaseArgs{
		Chart:           pulumi.String("argo-cd"),
		Name:            pulumi.String("argo-cd"),
		Namespace:       pulumi.String("argo-cd"),
		CreateNamespace: pulumi.Bool(true),
		Version:         pulumi.String(argocdVersion),
		RepositoryOpts: helm.RepositoryOptsArgs{
			Repo: pulumi.String("https://argoproj.github.io/argo-helm"),
		},
		ValueYamlFiles: stringArrayToAssetOrArchiveArrayOutput(argocdValues),
		Values: pulumi.Map{
			"configs": pulumi.Map{
				"repositories": pulumi.Map{
					"matthews-helm": pulumi.Map{
						"name":     pulumi.String("MatthewsREIS Github Helm Repository"),
						"type":     pulumi.String("helm"),
						"url":      pulumi.String("https://raw.githubusercontent.com/MatthewsREIS/charts/main"),
						"username": cfg.RequireSecret("helmRepoPat"),
						"password": cfg.RequireSecret("helmRepoPat"),
					},
				},
			}},
	}, opts...)
	return argocd, err
}

func deployKubePrometheusStack(ctx *pulumi.Context, cfg K8sPlatformConfigInput, opts ...pulumi.ResourceOption) (pulumi.Resource, error) {
	kubePrometheusStackVersion := "33.1.0"
	if cfg.KubePrometheusStackHelm.Version != "" {
		kubePrometheusStackVersion = cfg.KubePrometheusStackHelm.Version
	}

	prometheusValues := []string{
		"./helm-values/prometheus-values.yaml",
	}
	if len(cfg.KubePrometheusStackHelm.ValuesFiles) != 0 {
		prometheusValues = cfg.KubePrometheusStackHelm.ValuesFiles
	}

	// deploy prometheus using helm
	return helm.NewRelease(ctx, "kube-prometheus-stack", &helm.ReleaseArgs{
		Chart:           pulumi.String("kube-prometheus-stack"),
		Name:            pulumi.String("kube-prometheus-stack"),
		Namespace:       pulumi.String("kube-prometheus-stack"),
		CreateNamespace: pulumi.Bool(true),
		Version:         pulumi.String(kubePrometheusStackVersion),
		RepositoryOpts: helm.RepositoryOptsArgs{
			Repo: pulumi.String("https://prometheus-community.github.io/helm-charts"),
		},
		ValueYamlFiles: stringArrayToAssetOrArchiveArrayOutput(prometheusValues),
	}, opts...)
}

func deployCertManagerDnsSolverSecret(ctx *pulumi.Context, opts ...pulumi.ResourceOption) error {
	cfg := config.New(ctx, "")
	_, err := corev1.NewSecret(ctx, "cert-manager-cloudflare-api-token-secret", &corev1.SecretArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("cloudflare-api-token-secret"),
			Namespace: pulumi.String("cert-manager"),
		},
		StringData: pulumi.StringMap{
			"api-token": cfg.RequireSecret("cloudflareApiToken"),
		},
		Type: pulumi.String("Opaque"),
	}, opts...)
	return err
}

func deployPlatformApplicationManifest(ctx *pulumi.Context, opts ...pulumi.ResourceOption) (pulumi.Resource, error) {
	var platformApplicationConfig PlatformApplicationConfig
	cfg := config.New(ctx, "")
	cfg.RequireObject("platform-application", &platformApplicationConfig)
	if platformApplicationConfig.Enabled {
		// get application from template
		application, err := NewApplicationFromBytes(templates.PlatformApplicationBytes)
		if err != nil {
			return nil, err
		}
		// set variables from stack config
		application.Spec.SyncPolicy = platformApplicationConfig.SyncPolicy
		application.Spec.Source.TargetRevision = platformApplicationConfig.TargetRevision
		application.Spec.Source.Helm.Values = platformApplicationConfig.Values
		// sync
		resource, err := SyncArgocdApplication(ctx, "cluster-services", application, opts...)
		errorutils.LogOnErr(nil, "error syncing cluster application", err)
		return resource, err
	}
	return nil, nil
}

func stringArrayToAssetOrArchiveArrayOutput(in []string) pulumi.AssetOrArchiveArrayOutput {
	var o pulumi.AssetOrArchiveArray
	for _, i := range in {
		o = append(o, pulumi.NewFileAsset(i))
	}
	return o.ToAssetOrArchiveArrayOutput()
}
