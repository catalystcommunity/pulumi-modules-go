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
	ApiVersion string                 `yaml:"apiVersion"`
	Kind       string                 `yaml:"kind"`
	Metadata   map[string]interface{} `yaml:"metadata"`
	Spec       ArgocdApplicationSpec  `yaml:"spec"`
}

type ArgocdApplicationSpec struct {
	Source            ArgocdApplicationSpecSource          `yaml:"source"`
	Destination       ArgocdApplicationSpecDestination     `yaml:"destination"`
	Project           string                               `yaml:"project"`
	SyncPolicy        ArgocdApplicationSyncPolicy          `yaml:"syncPolicy,omitempty"`
	IgnoreDifferences []ArgocdApplicationIgnoreDifferences `yaml:"ignoreDifferences,omitempty"`
}

type ArgocdApplicationSpecSource struct {
	RepoUrl        string          `yaml:"repoURL"`
	Path           string          `yaml:"path,omitempty"`
	TargetRevision string          `yaml:"targetRevision,omitempty"`
	Helm           HelmSource      `yaml:"helm,omitempty"`
	Kustomize      KustomizeSource `yaml:"kustomize,omitempty"`
	Directory      DirectorySource `yaml:"directory,omitempty"`
	Plugin         PluginSource    `yaml:"plugin,omitempty"`
	Chart          string          `yaml:"chart,omitempty"`
}

type HelmSource struct {
	ValueFiles              []string                  `yaml:"valueFiles,omitempty"`
	Parameters              []HelmSourceParameter     `yaml:"parameters,omitempty"`
	ReleaseName             string                    `yaml:"releaseName,omitempty"`
	Values                  string                    `yaml:"values,omitempty"`
	FileParameters          []HelmSourceFileParameter `yaml:"fileParameters,omitempty"`
	Version                 string                    `yaml:"version,omitempty"`
	PassCredentials         bool                      `yaml:"passCredentials,omitempty"`
	IgnoreMissingValueFiles bool                      `yaml:"ignoreMissingValueFiles,omitempty"`
	SkipCrds                bool                      `yaml:"skipCrds,omitempty"`
}

type HelmSourceParameter struct {
	Name        string `yaml:"name,omitempty"`
	Value       string `yaml:"value,omitempty"`
	ForceString bool   `yaml:"forceString,omitempty"`
}

type HelmSourceFileParameter struct {
	Name string
	Path string
}

type KustomizeSource struct {
	NamePrefix             string            `yaml:"namePrefix,omitempty"`
	NameSuffix             string            `yaml:"nameSuffix,omitempty"`
	Images                 []string          `yaml:"images,omitempty"`
	CommonLabels           map[string]string `yaml:"commonLabels,omitempty"`
	Version                string            `yaml:"version,omitempty"`
	CommonAnnotations      map[string]string `yaml:"commonAnnotations,omitempty"`
	ForceCommonLabels      bool              `yaml:"forceCommonLabels,omitempty"`
	ForceCommonAnnotations bool              `yaml:"forceCommonAnnotations,omitempty"`
}

type DirectorySource struct {
	Recurse bool                   `yaml:"recurse,omitempty"`
	Jsonnet DirectorySourceJsonnet `yaml:"jsonnet,omitempty"`
	Exclude string                 `yaml:"exclude,omitempty"`
	Include string                 `yaml:"include,omitempty"`
}

type DirectorySourceJsonnet struct {
	ExtVars []JsonnetVar `yaml:"extVars,omitempty"`
	TLAs    []JsonnetVar `yaml:"TLAs,omitempty"`
	Libs    []string     `yaml:"libs,omitempty"`
}

type JsonnetVar struct {
	Name  string
	Value string
	Code  bool `yaml:"code,omitempty"`
}

type PluginSource struct {
	Name string            `yaml:"name,omitempty"`
	Env  []PluginSourceEnv `yaml:"env,omitempty"`
}

type PluginSourceEnv struct {
	Name  string
	Value string
}

type ArgocdApplicationSpecDestination struct {
	Server    string `yaml:"server,omitempty"`
	Namespace string `yaml:"namespace,omitempty"`
	Name      string `yaml:"name,omitempty"`
}

type ArgocdApplicationSyncPolicy struct {
	Automated   SyncPolicyAutomated `yaml:"automated,omitempty"`
	Retry       SyncPolicyRetry     `yaml:"retry,omitempty"`
	SyncOptions []string            `yaml:"syncOptions,omitempty"`
}

type SyncPolicyAutomated struct {
	Prune      bool `yaml:"prune,omitempty"`
	SelfHeal   bool `yaml:"selfHeal,omitempty"`
	AllowEmpty bool `yaml:"allowEmpty,omitempty"`
}

type SyncPolicyRetry struct {
	Limit   int          `yaml:"limit,omitempty"`
	Backoff RetryBackoff `yaml:"backoff,omitempty"`
}

type RetryBackoff struct {
	Duration    string `yaml:"duration,omitempty"`
	Factor      int    `yaml:"factor,omitempty"`
	MaxDuration string `yaml:"maxDuration,omitempty"`
}

type ArgocdApplicationIgnoreDifferences struct {
	Group                 string   `yaml:"group,omitempty"`
	Kind                  string   `yaml:"kind,omitempty"`
	Name                  string   `yaml:"name,omitempty"`
	Namespace             string   `yaml:"namespace,omitempty"`
	JsonPointers          []string `yaml:"jsonPointers,omitempty"`
	JQPathExpressions     []string `yaml:"jqPathExpressions,omitempty"`
	ManagedFieldsManagers []string `yaml:"managedFieldsManagers,omitempty"`
}
