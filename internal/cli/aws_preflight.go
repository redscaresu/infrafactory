package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/harness"
)

// awsHTTPTimeout bounds each attempt of the default AWS HTTP clients:
// the preflight has no command context to inherit a deadline from, and
// the scope teardown's requests ignore the command's cancellation.
const awsHTTPTimeout = 20 * time.Second

// newAWSHTTPClient is the real AWS transport a runtime gives each AWS
// dependency left nil. internal/cli's TestMain replaces it, so a test
// that forgets its fake fails instead of calling AWS.
var newAWSHTTPClient = func() *http.Client { return &http.Client{Timeout: awsHTTPTimeout} }

// assertAWSCredentials is assertSandboxCredentials for AWS.
func assertAWSCredentials(runtime *CommandRuntime) error {
	_, err := awsVerifiedEnv(context.Background(), runtime)
	return err
}

// awsVerifiedEnv returns the sealed env once its key is proven. Every
// local check -- the aws block, the credential file and its mode -- runs
// before the one call, sts:GetCallerIdentity, which must answer exactly
// the configured account and principal (ADR-0025 ordering, ADR-0023
// rule 2).
func awsVerifiedEnv(ctx context.Context, runtime *CommandRuntime) (map[string]string, error) {
	env, err := awsLayer3Env(runtime.Config.AWS)
	if err != nil {
		return nil, err
	}
	// Nil would make NewAWSSTSClient use the SDK's own client; refusing
	// keeps a runtime built without one from reaching real AWS unasked.
	if runtime.Deps.AWSSTS == nil {
		return nil, errors.New("aws Layer 3 preflight: no STS HTTP client is configured")
	}
	cfg := runtime.Config.AWS
	if err := harness.VerifyAWSIdentity(ctx, env, runtime.Deps.AWSSTS, "", cfg.AccountID, cfg.PrincipalARN); err != nil {
		return nil, err
	}
	return env, nil
}

// awsCommandEnvForAccount is sandboxCommandEnvForProject for AWS: the
// scope is the account, and the env is returned only for the configured
// one. It makes no call; assertAWSCredentials already verified the key.
func awsCommandEnvForAccount(runtime *CommandRuntime, scope string) (map[string]string, error) {
	account := runtime.Config.AWS.AccountID
	if strings.TrimSpace(scope) == "" {
		return nil, errors.New("refusing to build an aws Layer 3 environment with no account scope")
	}
	if scope != account {
		return nil, fmt.Errorf("refusing to build an aws Layer 3 environment for account %q: aws.account_id is %q", scope, account)
	}
	return awsLayer3Env(runtime.Config.AWS)
}

// awsCredentialFile is the Layer 3 key file under $HOME, written at scope
// setup (docs/operations.md § Layer 3 (AWS)).
const awsCredentialFile = ".config/infrafactory/layer3-aws.env"

// awsLayer3Env checks the aws block and builds the sealed env from the
// credential file beside layer3.env. It makes no call.
func awsLayer3Env(cfg config.AWSConfig) (map[string]string, error) {
	for _, field := range []struct{ name, value string }{
		{"aws.region", cfg.Region},
		{"aws.account_id", cfg.AccountID},
		{"aws.principal_arn", cfg.PrincipalARN},
	} {
		if strings.TrimSpace(field.value) == "" {
			return nil, fmt.Errorf("aws Layer 3 preflight: %s is empty in the config", field.name)
		}
	}
	if !harness.ValidAWSRegion(cfg.Region) {
		return nil, fmt.Errorf("aws Layer 3 preflight: aws.region %q is not an AWS region name", cfg.Region)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("aws Layer 3 preflight: locating the credential file: %w", err)
	}
	env, err := harness.AWSSealedEnv(filepath.Join(home, awsCredentialFile), cfg.Region)
	if err != nil {
		return nil, fmt.Errorf("aws Layer 3 preflight: %w", err)
	}
	return env, nil
}
