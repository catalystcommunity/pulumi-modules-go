package kubernetes

import (
	"github.com/catalystsquad/app-utils-go/errorutils"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"gopkg.in/yaml.v3"
)

func SyncArgocdApplication(ctx *pulumi.Context, pulumiResourceName string, application ArgocdApplication, id string) error {
	// marshall application to yaml
	bytes, err := yaml.Marshal(application)
	errorutils.LogOnErr(nil, "error marshalling application to yaml", err)
	if err != nil {
		return err
	}
	return SyncKubernetesManifest(ctx, pulumiResourceName, bytes, id)
}

// ArgocdApplication is a struct that marshalls into valid argocd application yaml. We could use the argo types but we have had
// problems with the yaml marshalling, and that also requires depending on argo, and nearly the entire k8s api.  This
// is let DRY and less direct but more simple and straightforward. We'll need to keep this in sync with their spec though.
// see spec at https://github.com/argoproj/argo-cd/blob/master/pkg/apis/application/v1alpha1/types.go
type ArgocdApplication struct {
	ApiVersion string
	Kind       string
	Metadata   map[string]interface{}
	Spec       struct {
		Source struct {
			RepoUrl        string `yaml:"repoURL"`
			Path           string `yaml:"path,omitempty"`
			TargetRevision string `yaml:"targetRevision,omitempty"`
			Helm           struct {
				ValueFiles []string `yaml:"valueFiles,omitempty"`
				Parameters []struct {
					Name        string `yaml:"name,omitempty"`
					Value       string `yaml:"value,omitempty"`
					ForceString bool   `yaml:"forceString,omitempty"`
				} `yaml:"parameters,omitempty"`
				ReleaseName    string `yaml:"releaseName,omitempty"`
				Values         string `yaml:"values,omitempty"`
				FileParameters []struct {
					Name string
					Path string
				} `yaml:"fileParameters,omitempty"`
				Version                 string `yaml:"version,omitempty"`
				PassCredentials         bool   `yaml:"passCredentials,omitempty"`
				IgnoreMissingValueFiles bool   `yaml:"ignoreMissingValueFiles,omitempty"`
				SkipCrds                bool   `yaml:"skipCrds,omitempty"`
			} `yaml:"helm,omitempty"`
			Kustomize struct {
				NamePrefix             string            `yaml:"namePrefix,omitempty"`
				NameSuffix             string            `yaml:"nameSuffix,omitempty"`
				Images                 []string          `yaml:"images,omitempty"`
				CommonLabels           map[string]string `yaml:"commonLabels,omitempty"`
				Version                string            `yaml:"version,omitempty"`
				CommonAnnotations      map[string]string `yaml:"commonAnnotations,omitempty"`
				ForceCommonLabels      bool              `yaml:"forceCommonLabels,omitempty"`
				ForceCommonAnnotations bool              `yaml:"forceCommonAnnotations,omitempty"`
			} `yaml:"kustomize,omitempty"`
			Directory struct {
				Recurse bool `json:"recurse,omitempty"`
				Jsonnet struct {
					ExtVars []struct {
						Name  string
						Value string
						Code  bool `yaml:"code,omitempty"`
					} `yaml:"extVars,omitempty"`
					TLAs []struct {
						Name  string
						Value string
						Code  bool
					} `yaml:"TLAs,omitempty"`
					Libs []string `yaml:"libs,omitempty"`
				}
				Exclude string `yaml:"exclude,omitempty"`
				Include string `yaml:"include,omitempty"`
			} `yaml:"directory,omitempty"`
			Plugin struct {
				Name string `yaml:"name,omitempty"`
				Env  []struct {
					Name  string
					Value string
				} `yaml:"env,omitempty"`
			} `yaml:"plugin,omitempty"`
			Chart string `yaml:"chart,omitempty"`
		}
		Destination struct {
			Server    string `yaml:"server,omitempty"`
			Namespace string `yaml:"namespace,omitempty"`
			Name      string `yaml:"name,omitempty"`
		}
		Project    string
		SyncPolicy struct {
			Automated struct {
				Prune      bool `yaml:"prune,omitempty"`
				SelfHeal   bool `yaml:"selfHeal,omitempty"`
				AllowEmpty bool `yaml:"allowEmpty,omitempty"`
			} `yaml:"automated,omitempty"`
			Retry struct {
				Limit   int `yaml:"limit,omitempty"`
				Backoff struct {
					Duration    string `yaml:"duration,omitempty"`
					Factor      int    `yaml:"factor,omitempty"`
					MaxDuration string `yaml:"maxDuration,omitempty"`
				} `yaml:"backoff,omitempty"`
			} `yaml:"retry,omitempty"`
			SyncOptions []string `yaml:"syncOptions,omitempty"`
		} `yaml:"syncPolicy,omitempty"`
		IgnoreDifferences []struct {
			Group                 string   `yaml:"group,omitempty"`
			Kind                  string   `yaml:"kind,omitempty"`
			Name                  string   `yaml:"name,omitempty"`
			Namespace             string   `yaml:"namespace,omitempty"`
			JsonPointers          []string `yaml:"jsonPointers,omitempty"`
			JQPathExpressions     []string `yaml:"jqPathExpressions,omitempty"`
			ManagedFieldsManagers []string `yaml:"managedFieldsManagers,omitempty"`
		} `yaml:"ignoreDifferences,omitempty"`
	}
}
