package kubernetes

import (
	"fmt"
	"github.com/catalystcommunity/app-utils-go/errorutils"
	"github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/yaml"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"os"
)

// SyncKubernetesManifest takes in a pulumi resource name, and a yaml kubernetes manifest as byte array.
// It writes the manifest to a file, defers deletion of said file, and creates a pulumi config file from it.
// Pulumi creates the k8s resources from the config file. Recommended use is to store your manifests in yaml file,
// embed them, template them with pulumi secrets, or variables, and then pass them to this method to sync
// the kubernetes resource, whatever it may be.
func SyncKubernetesManifest(ctx *pulumi.Context, pulumiResourceName string, manifest []byte, opts ...pulumi.ResourceOption) (pulumi.Resource, error) {
	// write bytes to file
	tempFileName := fmt.Sprintf("/tmp/%s.yaml", pulumiResourceName)
	err := os.WriteFile(tempFileName, manifest, 0644)
	errorutils.LogOnErr(nil, "error writing manifest to file", err)
	if err != nil {
		return nil, err
	}
	// defer file deletion
	defer func() {
		err = os.Remove(tempFileName)
		errorutils.LogOnErr(nil, "error deleting manifest file", err)
	}()
	// get pulumi configfile from written manifest
	resource, err := yaml.NewConfigFile(ctx, pulumiResourceName, &yaml.ConfigFileArgs{
		File: tempFileName,
	}, opts...)
	errorutils.LogOnErr(nil, "error getting pulumi configfile from manifest file", err)
	return resource, err
}
