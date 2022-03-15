package secrets

import (
	"github.com/catalystsquad/app-utils-go/templating"
	"github.com/joomcode/errorx"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
	"strings"
	"sync"
)

// SecretProvider enum
type SecretProvider int64

// SecretProvider num values
const (
	Unknown SecretProvider = iota
	Pulumi
	AWS
	GCP
)

// SecretProvider string values
const (
	SecretProviderTypePulumi  = "pulumi"
	SecretProviderTypeAWS     = "aws"
	SecretProviderTypeGCP     = "gcp"
	SecretProviderTypeUnknown = "unknown"
)

// String transforms a secret provider enum into its string reprsentation
func (s SecretProvider) String() string {
	switch s {
	case Pulumi:
		return SecretProviderTypePulumi
	case AWS:
		return SecretProviderTypeAWS
	case GCP:
		return SecretProviderTypeGCP
	}
	return SecretProviderTypeUnknown
}

// SecretProviderFromString transforms a string into a SecretProvider enum
func SecretProviderFromString(secretProvider string) SecretProvider {
	switch secretProvider {
	case SecretProviderTypePulumi:
		return Pulumi
	case SecretProviderTypeAWS:
		return AWS
	case SecretProviderTypeGCP:
		return GCP
	}
	return Unknown
}

// ReplaceSecrets uses the configured secret provider to retrieve secret values and replace them in the given string
// using catalyst squad templatying syntax, i.e. given <<mySecretValue>> in the string, the secret named `mySecretValue`
// will be pulled from the secret provider, and <<mySecretValue>> in the source string will be replaced with the value
// from the secret. Authentication/authorization should happen before running `pulumi up`. This makes no attempt to
// auth to providers and depends on that configuration already being present via env vars.
func ReplaceSecrets(ctx *pulumi.Context, source string) (string, error) {
	conf := config.New(ctx, "")
	secretProvider := conf.Require("secretProvider")
	switch SecretProviderFromString(secretProvider) {
	case Pulumi:
		return ReplaceSecretsFromPulumi(conf, source)
	case AWS:
		return ReplaceSecretsFromAWS(conf, source)
	case GCP:
		return ReplaceSecretsFromGCP(conf, source)
	default:
		return "", errorx.IllegalArgument.New("unknown secretProvider: %s . Please use one of ['%s','%s','%s']", secretProvider, SecretProviderTypePulumi, SecretProviderTypeAWS, SecretProviderTypeGCP)
	}
}

// ReplaceSecretsFromPulumi uses pulumi as the secrets provider to retrieve secrets
func ReplaceSecretsFromPulumi(conf *config.Config, source string) (string, error) {
	return templating.TemplateWithFunction(source, func(key string) (string, error) {
		// require secret and apply are async, so we need to wait until we get the value back
		wg := sync.WaitGroup{}
		wg.Add(1)
		var secretValue string
		key = strings.ReplaceAll(key, "<<", "")
		key = strings.ReplaceAll(key, ">>", "")
		conf.RequireSecret(key).ApplyT(func(value string) string {
			defer wg.Done()
			secretValue = value
			return value
		})
		// wait for apply to set the secret value
		wg.Wait()
		// return the secret value
		return secretValue, nil
	})
}

// ReplaceSecretsFromAWS uses AWS Secrets Manager as the secrets provider to retrieve secrets
func ReplaceSecretsFromAWS(conf *config.Config, source string) (string, error) {
	return "", errorx.IllegalArgument.New("AWS secret provider is not yet implemented")
}

// ReplaceSecretsFromGCP uses GCP Secrets Manager as the secrets provider to retrieve secrets
func ReplaceSecretsFromGCP(conf *config.Config, source string) (string, error) {
	return "", errorx.IllegalArgument.New("AWS secret provider is not yet implemented")
}
