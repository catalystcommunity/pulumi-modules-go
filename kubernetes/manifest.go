package kubernetes

import (
	"fmt"
	"github.com/catalystsquad/app-utils-go/errorutils"
	"github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/yaml"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"os"
)

func SyncKubernetesManifest(ctx *pulumi.Context, pulumiResourceName string, manifest []byte) error {
	// write bytes to file
	tempFileName := fmt.Sprintf("/tmp/%s.yaml", pulumiResourceName)
	err := os.WriteFile(tempFileName, manifest, 0644)
	errorutils.LogOnErr(nil, "error writing manifest to file", err)
	if err != nil {
		return err
	}
	// defer file deletion
	defer func() {
		err = os.Remove(tempFileName)
		errorutils.LogOnErr(nil, "error deleting manifest file", err)
	}()
	// get pulumi configfile from written manifest
	_, err = yaml.NewConfigFile(ctx, pulumiResourceName, &yaml.ConfigFileArgs{
		File: tempFileName,
	})
	errorutils.LogOnErr(nil, "error getting pulumi configfile from manifest file", err)
	return err
}
