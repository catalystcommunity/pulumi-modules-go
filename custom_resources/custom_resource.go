package custom_resources

import (
	"github.com/catalystsquad/app-utils-go/errorutils"
	"github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/apiextensions"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"gopkg.in/yaml.v3"
)

func GetPulumiCustomResourceFromManifestYaml(ctx *pulumi.Context, name string, application []byte) (*apiextensions.CustomResource, error) {
	var args *apiextensions.CustomResourceArgs
	err := yaml.Unmarshal(application, &args)
	errorutils.LogOnErr(nil, "error marshalling manifest yaml to pulumi custom resource args", err)
	if err != nil {
		return nil, err
	}
	// return custom resource
	return apiextensions.NewCustomResource(ctx, name, args)
}
