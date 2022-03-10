package utils

import "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

func GetImportOpt(id string) pulumi.ResourceOption {
	if id == "" {
		return nil
	}
	return pulumi.Import(pulumi.ID(id))
}
