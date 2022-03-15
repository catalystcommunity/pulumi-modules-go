package kubernetes

import (
	"github.com/catalystsquad/app-utils-go/errorutils"
	"github.com/catalystsquad/pulumi-modules-go/pkg/eks"
	"github.com/catalystsquad/pulumi-modules-go/pkg/templates"
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

	// deploy kube-prometheus-stack, this should happen first because the argo-cd helm chart installs service monitors
	prometheus, err := deployKubePrometheusStack(ctx, k8sConfig)
	errorutils.LogOnErr(nil, "error deploying kube-prometheus-stack", err)
	if err != nil {
		return err
	}

	// deploy argocd
	err = deployArgocd(ctx, cfg, k8sConfig, []pulumi.Resource{prometheus})
	errorutils.LogOnErr(nil, "error deploying argocd", err)
	if err != nil {
		return err
	}
	// create cert-manager dns secret
	err = deployCertManagerDnsSolverSecret(ctx)
	errorutils.LogOnErr(nil, "error deploying cert manager dns solver secret", err)
	if err != nil {
		return err
	}
	// deploy cluster argocd application
	err = deployPlatformApplicationManifest(ctx)
	errorutils.LogOnErr(nil, "error deploying cluster application manifest", err)
	return err
}

func deployArgocd(ctx *pulumi.Context, cfg *config.Config, k8sConfig K8sPlatformConfigInput, dependsOn []pulumi.Resource) error {
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
	_, err := helm.NewRelease(ctx, "argo-cd", &helm.ReleaseArgs{
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
	}, pulumi.DependsOn(dependsOn)) // this helm chart installs service monitors, so it depends on kube-prometheus-stack
	return err
}

func deployKubePrometheusStack(ctx *pulumi.Context, cfg K8sPlatformConfigInput) (pulumi.Resource, error) {
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
	})
}

func deployCertManagerDnsSolverSecret(ctx *pulumi.Context) error {
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
	})
	return err
}

func deployPlatformApplicationManifest(ctx *pulumi.Context) error {
	var platformApplicationConfig PlatformApplicationConfig
	cfg := config.New(ctx, "")
	cfg.RequireObject("platform-application", &platformApplicationConfig)
	if platformApplicationConfig.Enabled {
		// get application from template
		application, err := NewApplicationFromBytes(templates.PlatformApplicationBytes)
		if err != nil {
			return err
		}
		// set variables from stack config
		application.Spec.SyncPolicy = platformApplicationConfig.SyncPolicy
		application.Spec.Source.TargetRevision = platformApplicationConfig.TargetRevision
		application.Spec.Source.Helm.Values = platformApplicationConfig.Values
		// sync
		err = SyncArgocdApplication(ctx, "cluster-services", application, "")
		errorutils.LogOnErr(nil, "error syncing cluster application", err)
	}
	return nil
}

func stringArrayToAssetOrArchiveArrayOutput(in []string) pulumi.AssetOrArchiveArrayOutput {
	var o pulumi.AssetOrArchiveArray
	for _, i := range in {
		o = append(o, pulumi.NewFileAsset(i))
	}
	return o.ToAssetOrArchiveArrayOutput()
}
