package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/redscaresu/infrafactory/internal/scenario"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func runTestCommand(cmd *cobra.Command, args []string, runtime *CommandRuntime) error {
	noDestroy, err := cmd.Flags().GetBool("no-destroy")
	if err != nil {
		return &CLIError{Op: "test", Code: errorCodeUsage, Err: fmt.Errorf("read --no-destroy flag: %w", err)}
	}

	// The guard only engages when Layer 3 is on. Interrupting a
	// mock-only run costs nothing; interrupting one that has already
	// applied to real Scaleway leaves billable resources behind.
	return withSandboxInterruptGuard(cmd, runtime, signal.NotifyContext, func(ctx context.Context) error {
		result, err := executeTest(ctx, runtime, args[0], testExecutionOptions{
			MockDeployMode: harness.MockDeployModeClean,
			SkipDestroy:    noDestroy,
		})
		if err != nil {
			if writeErr := writeCommandOutput(cmd, result); writeErr != nil {
				return writeErr
			}
			return err
		}
		return writeCommandOutput(cmd, result)
	})
}

func testCommandEnv(runtime *CommandRuntime) map[string]string {
	return cloudEnv(runtime)
}

// cloudEnv builds the env vars terraform-provider-* clients consult
// regardless of which cloud's scenario is loaded. Setting all three
// clouds' vars on every invocation is safe (each provider only reads
// its own prefix) and keeps the runtime cloud-agnostic — a scenario
// flip between cloud=scaleway and cloud=aws doesn't need a config
// reload.
//
// Per-cloud notes:
//   - Scaleway:  SCW_API_URL points the provider at mockway. Credential
//     keys are dummy values mockway accepts.
//   - GCP: terraform-provider-google needs a credential source even
//     when *_custom_endpoint overrides will redirect every call to
//     fakegcp. GOOGLE_OAUTH_ACCESS_TOKEN sets a static token (bypasses
//     ADC); GOOGLE_PROJECT pins the default project so resources don't
//     require explicit project = "..." in HCL.
//   - AWS: AWS_ACCESS_KEY_ID + AWS_SECRET_ACCESS_KEY satisfy the SDK's
//     credential check. AWS_REGION pins us-east-1 to match the
//     endpoints injected by ensureAwsProviderWiring. AWS_EC2_METADATA_DISABLED
//     stops the SDK from trying to hit IMDS.
//
// Endpoint redirection happens via the ensure*ProviderWiring functions
// (GCP: per-service *_custom_endpoint; AWS: provider's endpoints{}
// block) — env vars alone don't get the provider talking to the
// right HTTP server.
func cloudEnv(runtime *CommandRuntime) map[string]string {
	env := map[string]string{
		// Scaleway
		"SCW_API_URL":            runtime.Config.Mockway.URL,
		"SCW_ACCESS_KEY":         "SCWMOCKACCESSKEY0000",
		"SCW_SECRET_KEY":         "00000000-0000-0000-0000-000000000000",
		"SCW_DEFAULT_PROJECT_ID": "00000000-0000-0000-0000-000000000000",
		// GCP
		"GOOGLE_OAUTH_ACCESS_TOKEN": "fakegcp-mock-token",
		"GOOGLE_PROJECT":            "infrafactory-test",
		// AWS
		"AWS_ACCESS_KEY_ID":         "test",
		"AWS_SECRET_ACCESS_KEY":     "test",
		"AWS_REGION":                "us-east-1",
		"AWS_EC2_METADATA_DISABLED": "true",
		// Genesys Cloud. Provider credentials must be set even when
		// HTTPS_PROXY redirects every call to fakegenesys.
		"GENESYSCLOUD_OAUTHCLIENT_ID":     "fake-client-id",
		"GENESYSCLOUD_OAUTHCLIENT_SECRET": "fake-client-secret",
		"GENESYSCLOUD_REGION":             "us-east-1",
	}
	// S117: route the genesyscloud provider through fakegenesys's TLS
	// MITM CONNECT proxy (S116). The GENESYSCLOUD_GATEWAY_* env vars
	// the SDK exposes don't override the auth subdomain — only HTTPS_PROXY
	// works. Fetch the boot-time CA from /mock/ca-cert and write it to
	// a temp file; point SSL_CERT_FILE there so Go's TLS stack trusts
	// the MITM leaf chain.
	if u := strings.TrimSpace(runtime.Config.Fakegenesys.URL); u != "" {
		if proxyURL, ok := genesysProxyURL(u); ok {
			env["HTTPS_PROXY"] = proxyURL
			env["HTTP_PROXY"] = proxyURL
			// NO_PROXY: keep tofu's network paths off the MITM. The
			// OpenTofu provider-registry and the GitHub release CDN
			// are the load-bearing ones; bypassing AWS endpoints is
			// belt-and-suspenders so the AWS provider can still hit
			// fakeaws on its own loopback. Without this, our MITM
			// presents leaf certs signed by fakegenesys's CA for
			// registry.opentofu.org — and tofu init fails with
			// "certificate signed by unknown authority". Standard
			// Go-net/http NO_PROXY semantics: comma-separated;
			// "*.foo.com" matches any subdomain of foo.com.
			env["NO_PROXY"] = strings.Join([]string{
				"registry.opentofu.org",
				"registry.terraform.io",
				"releases.hashicorp.com",
				"github.com",
				"127.0.0.1",
				"localhost",
				".opentofu.org",
				".terraform.io",
				".hashicorp.com",
				".amazonaws.com",
				".githubusercontent.com",
				".github.com",
				".windows.net",
			}, ",")
			env["no_proxy"] = env["NO_PROXY"] // some clients consult lowercase
			if certPath, ok := writeGenesysCACert(u); ok {
				env["SSL_CERT_FILE"] = certPath
				// SSL_CERT_DIR: include both the OS's default cert dir
				// (so tofu can still verify registry.opentofu.org and
				// other public hosts via the system trust store) AND
				// our temp file's dir. Go's TLS stack walks both env
				// vars; SSL_CERT_FILE adds OUR CA on top of the system
				// roots. (Previous draft pointed SSL_CERT_DIR at
				// /dev/null which nuked the system roots — wrong.)
			}
		}
	}
	return env
}

// genesysProxyURL derives the TLS MITM CONNECT proxy URL from
// cfg.Fakegenesys.URL. By convention fakegenesys runs the proxy on
// (port + 360) — :8083 -> :8443. Returns the proxy URL string and true
// on success.
func genesysProxyURL(httpURL string) (string, bool) {
	parsed, err := url.Parse(httpURL)
	if err != nil || parsed.Host == "" {
		return "", false
	}
	host := parsed.Hostname()
	port := parsed.Port()
	if port == "" {
		return "", false
	}
	// Map the canonical pairing :8083 -> :8443 explicitly; any other
	// port falls back to 8443 since the cmd binary's --tls-port
	// default is fixed.
	tlsPort := "8443"
	if port != "8083" {
		// Custom HTTP port — caller is expected to also override
		// FAKEGENESYS_TLS_PORT (read here from env first if present).
		if envTLS := strings.TrimSpace(os.Getenv("FAKEGENESYS_TLS_PORT")); envTLS != "" {
			tlsPort = envTLS
		}
	}
	return "http://" + host + ":" + tlsPort, true
}

// writeGenesysCACert fetches /mock/ca-cert from fakegenesys (over plain
// HTTP), writes the PEM to a temp file, and returns the path. Returns
// false on any error so the caller falls back to no-cert (which makes
// the harness break later in a clearer way than a half-broken env).
func writeGenesysCACert(fakegenesysURL string) (string, bool) {
	httpClient := &http.Client{Timeout: 5 * time.Second}
	resp, err := httpClient.Get(strings.TrimRight(fakegenesysURL, "/") + "/mock/ca-cert")
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return "", false
	}
	defer resp.Body.Close()
	pem, err := io.ReadAll(resp.Body)
	if err != nil || len(pem) == 0 {
		return "", false
	}
	f, err := os.CreateTemp("", "fakegenesys-ca-*.pem")
	if err != nil {
		return "", false
	}
	if _, err := f.Write(pem); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return "", false
	}
	_ = f.Close()
	return f.Name(), true
}

func appendMockDeployResult(stages []StageSummary, failures []FailureSummary, result *harness.MockDeployResult, runErr error) ([]StageSummary, []FailureSummary) {
	if runErr == nil {
		if result != nil && result.Init.Stage != "" {
			stages = append(stages, StageSummary{Layer: "mock_deploy", Stage: "init", Status: StageStatusPass})
		}
		if result != nil && result.Apply.Stage != "" {
			stages = append(stages, StageSummary{Layer: "mock_deploy", Stage: "apply", Status: StageStatusPass})
		}
		stages = append(stages, StageSummary{Layer: "mock_deploy", Stage: "state", Status: StageStatusPass})
		return stages, failures
	}

	mockErr := &harness.MockDeployError{}
	if !errors.As(runErr, &mockErr) {
		failures = append(failures, FailureSummary{Layer: "mock_deploy", Stage: "run", Detail: runErr.Error()})
		return stages, failures
	}

	switch mockErr.Stage {
	case "reset":
		stages = append(stages, StageSummary{Layer: "mock_deploy", Stage: "reset", Status: StageStatusFail})
	case "init":
		stages = append(stages, StageSummary{Layer: "mock_deploy", Stage: "init", Status: StageStatusFail})
	case "apply":
		if len(stages) == 0 || stages[len(stages)-1].Stage != "apply" {
			stages = append(stages, StageSummary{Layer: "mock_deploy", Stage: "apply", Status: StageStatusFail})
		} else {
			stages[len(stages)-1].Status = StageStatusFail
		}
	case "state":
		stages = append(stages, StageSummary{Layer: "mock_deploy", Stage: "state", Status: StageStatusFail})
	}

	failures = append(failures, FailureSummary{
		Layer:   "mock_deploy",
		Stage:   mockErr.Stage,
		Check:   mockErr.Stage,
		Command: "mock deploy harness",
		Detail:  mockDeployFailureDetail(mockErr),
	})

	return stages, failures
}

func mockDeployFailureDetail(err *harness.MockDeployError) string {
	if err == nil {
		return ""
	}
	var stderr string
	switch err.Stage {
	case "init":
		stderr = err.Init.Stderr
	case "apply":
		stderr = err.Apply.Stderr
	}
	return stderrFailureDetail(err.Err, stderr)
}

func appendDestroyResult(stages []StageSummary, failures []FailureSummary, result *harness.DestroyResult, runErr error) ([]StageSummary, []FailureSummary) {
	if runErr == nil {
		if result != nil && result.Destroy.Stage != "" {
			stages = append(stages, StageSummary{Layer: "destruction", Stage: "destroy", Status: StageStatusPass})
		}
		stages = append(stages, StageSummary{Layer: "destruction", Stage: "state", Status: StageStatusPass})
		stages = append(stages, StageSummary{Layer: "destruction", Stage: "orphan_check", Status: StageStatusPass})
		return stages, failures
	}

	destroyErr := &harness.DestroyError{}
	if !errors.As(runErr, &destroyErr) {
		failures = append(failures, FailureSummary{Layer: "destruction", Stage: "run", Detail: runErr.Error()})
		return stages, failures
	}

	switch destroyErr.Stage {
	case "destroy":
		stages = append(stages, StageSummary{Layer: "destruction", Stage: "destroy", Status: StageStatusFail})
	case "state":
		stages = append(stages, StageSummary{Layer: "destruction", Stage: "state", Status: StageStatusFail})
	case "orphan_check":
		stages = append(stages, StageSummary{Layer: "destruction", Stage: "orphan_check", Status: StageStatusFail})
	}

	failures = append(failures, FailureSummary{
		Layer:   "destruction",
		Stage:   destroyErr.Stage,
		Check:   destroyErr.Stage,
		Command: "destroy harness",
		Detail:  destroyFailureDetail(destroyErr),
	})

	return stages, failures
}

func destroyFailureDetail(err *harness.DestroyError) string {
	if err == nil {
		return ""
	}
	return stderrFailureDetail(err.Err, err.Destroy.Stderr)
}

// stderrFailureDetail renders a harness error for FailureSummary.Detail,
// appending the failing command's stderr when it carried any.
//
// The bare exec error is nearly always "exit status 1", which says
// nothing about what broke. Layer 3 needs this most: reproducing a real
// Scaleway failure costs money, so the one line the operator gets has to
// carry the provider's own message. The S143 run 2 canary hit a
// transient block-volume create error and the run reported exactly
// "exit status 1" and nothing else.
func stderrFailureDetail(baseErr error, stderr string) string {
	detail := ""
	if baseErr != nil {
		detail = baseErr.Error()
	}
	trimmedStderr := stripAnsi(strings.TrimSpace(stderr))
	if trimmedStderr == "" {
		return detail
	}
	if len(trimmedStderr) > failureStderrDetailMaxChars {
		trimmedStderr = trimmedStderr[:failureStderrDetailMaxChars] + "..."
	}
	return fmt.Sprintf("%s | stderr: %s", detail, trimmedStderr)
}

// failureStderrDetailMaxChars caps the stderr portion stored in
// FailureSummary.Detail. M86: bumped 600 → 2400 because tofu's error
// envelope (ASCII art borders + OAuth metadata JSON) eats ~500 chars
// before the actionable Resource: line. The lower limit silently
// truncated `google_project_service.redis` past the cutoff, so
// ExtractDescriptivePitfall's resource regex never matched and the
// auto-learning loop stayed dormant. 2400 comfortably fits the full
// terraform-provider-google envelope + resource trailer.
const failureStderrDetailMaxChars = 2400

// ansiEscapeRE matches CSI sequences (\x1b[ ... letter) which tofu /
// terraform-provider-google emit liberally in error output. Stripping
// them before the truncation budget (above) makes the budget count
// real chars, not display formatting — the M86 root cause was that
// ANSI codes consumed the budget before `google_project_service.redis`
// could land in the failure detail.
var ansiEscapeRE = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

func stripAnsi(s string) string {
	return ansiEscapeRE.ReplaceAllString(s, "")
}

func appendSandboxDeployResult(stages []StageSummary, failures []FailureSummary, result *harness.SandboxDeployResult, runErr error) ([]StageSummary, []FailureSummary) {
	if runErr == nil {
		if result != nil && result.Init.Stage != "" {
			stages = append(stages, StageSummary{Layer: "sandbox_deploy", Stage: "init", Status: StageStatusPass})
		}
		if result != nil && result.Plan.Stage != "" {
			stages = append(stages, StageSummary{Layer: "sandbox_deploy", Stage: "plan", Status: StageStatusPass})
		}
		if result != nil && result.Apply.Stage != "" {
			// A retry means the real API flapped. Report it rather than
			// quietly passing -- a silently-retried apply looks identical
			// to one that worked first time.
			detail := ""
			if result.Attempts > 1 {
				detail = fmt.Sprintf("succeeded on attempt %d (real API returned a retryable error)", result.Attempts)
			}
			stages = append(stages, StageSummary{Layer: "sandbox_deploy", Stage: "apply", Status: StageStatusPass, Detail: detail})
		}
		return stages, failures
	}

	deployErr := &harness.SandboxDeployError{}
	if !errors.As(runErr, &deployErr) {
		failures = append(failures, FailureSummary{Layer: "sandbox_deploy", Stage: "run", Detail: runErr.Error()})
		return stages, failures
	}

	// Record passed stages before the failed one for diagnostic visibility.
	if deployErr.Init.Stage != "" && deployErr.Stage != "init" {
		stages = append(stages, StageSummary{Layer: "sandbox_deploy", Stage: "init", Status: StageStatusPass})
	}
	if deployErr.Plan.Stage != "" && deployErr.Stage != "plan" {
		stages = append(stages, StageSummary{Layer: "sandbox_deploy", Stage: "plan", Status: StageStatusPass})
	}
	switch deployErr.Stage {
	case "init":
		stages = append(stages, StageSummary{Layer: "sandbox_deploy", Stage: "init", Status: StageStatusFail})
	case "plan":
		stages = append(stages, StageSummary{Layer: "sandbox_deploy", Stage: "plan", Status: StageStatusFail})
	case "apply":
		stages = append(stages, StageSummary{Layer: "sandbox_deploy", Stage: "apply", Status: StageStatusFail})
	}

	failures = append(failures, FailureSummary{
		Layer:   "sandbox_deploy",
		Stage:   deployErr.Stage,
		Check:   deployErr.Stage,
		Command: "sandbox deploy harness",
		Detail:  sandboxDeployFailureDetail(deployErr),
	})
	return stages, failures
}

// sandboxDeployFailureDetail picks the stderr belonging to the stage
// that actually failed. Layer 3 runs three commands against the real
// API, and only the failing one's output is worth surfacing.
func sandboxDeployFailureDetail(err *harness.SandboxDeployError) string {
	if err == nil {
		return ""
	}
	var stderr string
	switch err.Stage {
	case "init":
		stderr = err.Init.Stderr
	case "plan":
		stderr = err.Plan.Stderr
	case "apply":
		stderr = err.Apply.Stderr
	}
	detail := stderrFailureDetail(err.Err, stderr)
	if err.Stage == "apply" && err.Attempts > 1 {
		detail = fmt.Sprintf("%s (failed %d attempts, so not a transient blip)", detail, err.Attempts)
	}
	return detail
}

func appendSandboxDestroyResult(stages []StageSummary, failures []FailureSummary, result *harness.SandboxDestroyResult, runErr error) ([]StageSummary, []FailureSummary) {
	if runErr == nil {
		if result != nil && result.Destroy.Stage != "" {
			stages = append(stages, StageSummary{Layer: "sandbox_deploy", Stage: "destroy", Status: StageStatusPass})
		}
		return stages, failures
	}

	destroyErr := &harness.SandboxDestroyError{}
	if !errors.As(runErr, &destroyErr) {
		failures = append(failures, FailureSummary{Layer: "sandbox_deploy", Stage: "destroy", Detail: runErr.Error()})
		return stages, failures
	}
	stages = append(stages, StageSummary{Layer: "sandbox_deploy", Stage: "destroy", Status: StageStatusFail})
	failures = append(failures, FailureSummary{
		Layer:   "sandbox_deploy",
		Stage:   destroyErr.Stage,
		Check:   destroyErr.Stage,
		Command: "sandbox destroy harness",
		Detail:  stderrFailureDetail(destroyErr.Err, destroyErr.Destroy.Stderr),
	})
	return stages, failures
}

type testExecutionOptions struct {
	MockDeployMode harness.MockDeployMode
	SkipDestroy    bool
}

func executeTest(ctx context.Context, runtime *CommandRuntime, scenarioPath string, opts testExecutionOptions) (OutputResult, error) {
	sc, err := runtime.LoadScenario(scenarioPath)
	if err != nil {
		return OutputResult{}, fmt.Errorf("load scenario %q: %w", scenarioPath, err)
	}
	return executeTestWithScenario(ctx, runtime, sc, runtime.OutputDir(), opts)
}

func executeTestWithScenario(ctx context.Context, runtime *CommandRuntime, sc scenario.Scenario, outputDir string, opts testExecutionOptions) (OutputResult, error) {
	unsupportedStages, unsupportedFailures, err := unsupportedCriteriaResult(sc, runtime.Config.Validation.Layers.SandboxDeploy.Enabled)
	if err != nil {
		return OutputResult{}, err
	}
	if len(unsupportedFailures) > 0 {
		return OutputResult{
				Command:  "test",
				Scenario: sc.Name,
				Status:   CommandStatusFailed,
				Stages:   unsupportedStages,
				Failures: unsupportedFailures,
			}, &CLIError{
				Op:   "test",
				Code: errorCodeCommandFailed,
				Err:  errors.New("unsupported acceptance criteria present"),
			}
	}

	if runtime.Deps.MockDeploy == nil || runtime.Deps.Destroy == nil {
		return OutputResult{}, fmt.Errorf("mock deploy/destroy dependencies unavailable: %w", ErrDependencyUnavailable)
	}

	stages := append(make([]StageSummary, 0, len(unsupportedStages)+8), unsupportedStages...)
	failures := make([]FailureSummary, 0)
	var planLiveText []byte
	env := testCommandEnv(runtime)

	if !runtime.Config.Validation.Layers.MockDeploy.Enabled {
		stages = append(stages, StageSummary{Layer: "mock_deploy", Stage: "disabled", Status: StageStatusSkip})
		if runtime.Config.Validation.Layers.Destruction.Enabled {
			stages = append(stages, StageSummary{Layer: "destruction", Stage: "blocked", Status: StageStatusSkip, Detail: "requires mock_deploy.enabled"})
		} else {
			stages = append(stages, StageSummary{Layer: "destruction", Stage: "disabled", Status: StageStatusSkip})
		}

		return OutputResult{
			Command:  "test",
			Scenario: sc.Name,
			Status:   CommandStatusSuccess,
			Stages:   stages,
		}, nil
	}

	deployMode := opts.MockDeployMode
	if deployMode == "" {
		deployMode = harness.MockDeployModeClean
	}

	// Validate BEFORE any tofu runs, including Layer 2's.
	//
	// The mock deploy is not a safe place to execute unvalidated HCL. It
	// runs `tofu init/apply` in this same process, whose environment holds
	// the cloud credentials, so a `provisioner` or `data "external"` in a
	// PR fixture would execute there — before a check placed in front of
	// the sandbox deploy ever ran. "Before the real apply" was the wrong
	// bar; "before any tofu" is the right one.
	//
	// Only when Layer 3 is enabled: with sandbox off there are no
	// credentials to protect and the mock is the whole point.
	sandboxEnabled := runtime.Config.Validation.Layers.SandboxDeploy.Enabled
	hclRefused := false
	if sandboxEnabled {
		if shapeErr := layer3PreflightHCL(outputDir,
			runtime.Config.Validation.Layers.SandboxDeploy.AllowResourceTypes); shapeErr != nil {
			hclRefused = true
			stages = append(stages, StageSummary{Layer: "sandbox_deploy", Stage: "allowlist", Status: StageStatusFail})
			failures = append(failures, FailureSummary{
				Layer:   "sandbox_deploy",
				Stage:   "allowlist",
				Check:   "allow_resource_types",
				Command: "layer 3 hcl validation",
				Detail:  shapeErr.Error(),
			})
		} else {
			stages = append(stages, StageSummary{Layer: "sandbox_deploy", Stage: "allowlist", Status: StageStatusPass})
		}
	}

	// A refused configuration means no tofu runs at all -- not even to
	// clean up. `tofu destroy` evaluates the same configuration, and
	// destroy-time provisioners are a thing, so "execute it just to tidy
	// up" would hand the refused HCL exactly the execution it was refused
	// for.
	//
	// That leaves one honest gap, and it is reported rather than papered
	// over: if the live state already records resources from an earlier
	// run, they are not destroyed here. The operator is told, loudly,
	// because a human deciding what to do beats a machine executing HCL it
	// has just declared untrustworthy.
	if hclRefused {
		if liveStateMayHoldResources(outputDir) {
			failures = append(failures, FailureSummary{
				Layer:   "sandbox_deploy",
				Stage:   "cleanup_blocked",
				Check:   "no_orphans",
				Command: "layer 3 hcl validation",
				Detail: "the configuration was refused, so no tofu was run -- including destroy. " +
					"Live state still records resources: inspect them and clean up with a configuration that passes validation.",
			})
			stages = append(stages, StageSummary{Layer: "sandbox_deploy", Stage: "cleanup_blocked", Status: StageStatusFail})
		}
		return OutputResult{
			Command:  "test",
			Scenario: sc.Name,
			Status:   CommandStatusFailed,
			Stages:   stages,
			Failures: failures,
		}, &CLIError{Op: "test", Code: errorCodeCommandFailed, Err: errors.New("test checks failed")}
	}

	deployResult, deployErr := runtime.Deps.MockDeploy.Run(ctx, outputDir, env, deployMode)
	stages, failures = appendMockDeployResult(stages, failures, deployResult, deployErr)
	// Declared out here because the destroy path below has to delete what
	// the apply path created, and they are separate branches.
	var runProjectID string

	if deployErr == nil && sandboxEnabled {
		// Validate the sealed environment BEFORE creating anything. The
		// credential and endpoint checks live in sandboxCommandEnv, and
		// running them only after the Account API call would let a
		// configuration that is going to be rejected -- a missing
		// SCW_ACCESS_KEY, say -- still leave a real project behind,
		// relying on best-effort cleanup for residue that should never
		// have existed.
		var sandboxEnv map[string]string
		sandboxEnvErr := assertSandboxCredentials(runtime)

		if sandboxEnvErr == nil {
			// ADR-0025: the run's own project has to exist before the
			// provider's environment is built, which is why it can no
			// longer be a Terraform resource.
			createdID, runProjectStages, runProjectFailures := ensureRunProject(ctx, runtime, sc.Name, outputDir)
			runProjectID = createdID
			stages = append(stages, runProjectStages...)
			failures = append(failures, runProjectFailures...)

			if len(runProjectFailures) > 0 {
				// Creating it failed, so there is nothing to apply into.
				// Falling back to the shared project would put this run's
				// strays next to every other run's.
				sandboxEnvErr = fmt.Errorf("run project unavailable")
			} else if runProjectID != "" {
				sandboxEnv, sandboxEnvErr = sandboxCommandEnvForProject(runtime, runProjectID)
			}
		}
		if sandboxEnvErr != nil {
			stages = append(stages, StageSummary{Layer: "sandbox_deploy", Stage: "preflight", Status: StageStatusFail})
			failures = append(failures, FailureSummary{
				Layer:   "sandbox_deploy",
				Stage:   "preflight",
				Check:   "credentials",
				Command: "sandbox deploy preflight",
				Detail:  sandboxEnvErr.Error(),
			})
		} else {
			// Emit the preflight as an explicit pass, not just a silent
			// absence on success. The whole point of the endpoint
			// assertion is that an operator can see it ran -- a Layer 3
			// stage summary with no preflight line is indistinguishable
			// from one where the check was skipped.
			stages = append(stages, StageSummary{
				Layer:  "sandbox_deploy",
				Stage:  "preflight",
				Status: StageStatusPass,
				Detail: "credentials present; endpoint asserted as " + realScalewayAPIURL,
			})
			// Enforce the allowlist here too, not only in the generation
			// path. ADR-0023 rule 5 denies expensive types before any API
			// call, but that check lived solely in generate -- so any route
			// applying pre-existing HCL reached the real API with the
			// allowlist never consulted. The S144 PR gate is exactly such a
			// route: it stages committed fixtures and calls test, and its
			// HCL comes from a pull request, which is precisely where an
			// unvetted resource type would arrive from.
			{
				sandboxResult, sandboxErr := runtime.Deps.SandboxDeploy.Run(ctx, outputDir, sandboxEnv)
				stages, failures = appendSandboxDeployResult(stages, failures, sandboxResult, sandboxErr)
				if sandboxResult != nil && len(sandboxResult.Plan.Stdout) > 0 {
					planLiveText = []byte(sandboxResult.Plan.Stdout)
				} else if sandboxErr != nil {
					var deployErr *harness.SandboxDeployError
					if errors.As(sandboxErr, &deployErr) && len(deployErr.Plan.Stdout) > 0 {
						planLiveText = []byte(deployErr.Plan.Stdout)
					}
				}
			}
		}
	}
	if deployErr == nil && runtime.Config.Validation.Layers.Destruction.Enabled && !opts.SkipDestroy {
		criteriaStages, criteriaFailures := evaluateSupportedCriteria(ctx, sc, runtime, deployResult)
		stages = append(stages, criteriaStages...)
		failures = append(failures, criteriaFailures...)

		destroyResult, destroyErr := runtime.Deps.Destroy.Run(ctx, outputDir, env)
		stages, failures = appendDestroyResult(stages, failures, destroyResult, destroyErr)
		// Clean up whenever real resources MIGHT exist, not only when the
		// apply succeeded. tofu creates resources one at a time and writes
		// each to state as it goes, so an apply that dies partway is
		// precisely the case that has left infrastructure behind -- the
		// lb-paris canary leaked a real project and load-balancer IP
		// exactly this way, because cleanup was gated on success.
		//
		// The live state is the ONLY gate, deliberately. Keying off "did
		// this run attempt an apply" was both redundant and wrong: the
		// pre-apply validations can refuse before any attempt while an
		// earlier run's resources are still recorded, and those still need
		// destroying. What matters is whether resources may exist, not who
		// created them. a failure in init or plan happens
		// before any resource exists and writes no live state, so cleaning
		// up there would destroy nothing and then report an unverifiable
		// sweep, telling the operator to chase a leak that cannot exist.
		// run_command.go uses the same signal.
		if sandboxEnabled && liveStateMayHoldResources(outputDir) {
			// The SAME project the apply used. Destroy refreshes and
			// removes resources that carry no project_id of their own,
			// so pointing the provider back at the shared fallback here
			// would look for them in the wrong project -- failing the
			// teardown, or leaving them behind, for exactly the
			// projectless resources this flag exists to support.
			sandboxEnv, sandboxEnvErr := sandboxCommandEnvForProject(runtime, runProjectID)
			if sandboxEnvErr != nil {
				stages = append(stages, StageSummary{Layer: "sandbox_deploy", Stage: "destroy_preflight", Status: StageStatusFail})
				failures = append(failures, FailureSummary{
					Layer:   "sandbox_deploy",
					Stage:   "destroy_preflight",
					Check:   "credentials",
					Command: "sandbox destroy preflight",
					Detail:  sandboxEnvErr.Error(),
				})
			} else {
				// Capture the sweep target BEFORE destroy: tofu empties
				// terraform-live.tfstate, taking the project id with it.
				// The first canary run failed exactly here.
				sweepTarget, sweepTargetErr := harness.CaptureSweepTarget(outputDir)
				sandboxDestroyResult, purged, sandboxDestroyErr := destroySandbox(ctx, runtime, outputDir, sandboxEnv, sweepTargetProjectID(sweepTarget))
				stages, failures = appendSandboxDestroyResult(stages, failures, sandboxDestroyResult, sandboxDestroyErr)
				if len(purged) > 0 {
					stages = append(stages, autoCreatedPurgeStage(purged))
				}
				if sandboxDestroyErr == nil {
					// Where tofu used to delete the project. Under
					// ADR-0025 it is not a Terraform resource, so we
					// delete it here -- BEFORE the sweep, because the
					// sweep's job is to verify the project is gone.
					// Deleting it afterwards would make every clean
					// teardown report a leak.
					deleteStages, deleteFailures := releaseRunProject(ctx, runtime, outputDir, runProjectID, sandboxEnv)
					stages = append(stages, deleteStages...)
					failures = append(failures, deleteFailures...)

					failuresBeforeSweep := len(failures)
					stages, failures = appendOrphanSweepResult(ctx, stages, failures, runtime, sweepTarget, sweepTargetErr, sandboxEnv)
					// The teardown's own verdict, not the command's. A
					// failure recorded earlier -- a mock criteria check,
					// say -- says nothing about whether the account came
					// back clean.
					_ = failuresBeforeSweep
				}
			}
		}
		// One cleanup for every exit from the sandbox block, not just the
	} else if deployErr == nil {
		criteriaStages, criteriaFailures := evaluateSupportedCriteria(ctx, sc, runtime, deployResult)
		stages = append(stages, criteriaStages...)
		failures = append(failures, criteriaFailures...)
		detail := ""
		if opts.SkipDestroy {
			detail = "skipped by --no-destroy"
		}
		stages = append(stages, StageSummary{Layer: "destruction", Stage: "disabled", Status: StageStatusSkip, Detail: detail})
	}

	// Outside every branch on purpose. Three separate placements of this
	// cleanup were each skipped by some exit path -- the happy-path-only
	// one, then the destroy-branch one, which `--no-destroy` and disabled
	// destruction both walk past. The project is created in one place, so
	// it is released in one place, after all of them.
	//
	// Deleted only when nothing of the run can still exist: the account
	// was proven clean, or no state was ever written so nothing was
	// created. Otherwise it is kept and said so, because the project id
	// is the handle to whatever remains.
	// "No failures" is not the same as "nothing is left". On a --no-destroy
	// run the apply succeeds and the resources are deliberately still
	// live, so deleting the project would either fail on resources in use
	// or remove the handle to a run the operator asked to keep. Deletion
	// needs destruction to have actually run AND to have proved the
	// account clean -- which is sandboxTeardownClean, not the command's
	// accumulated failure list. A mock criteria failure followed by a
	// clean destroy leaves nothing behind but would otherwise strand the
	// empty project forever.
	if runProjectID != "" && !runProjectReleased(stages) {
		switch {
		case !liveStateMayHoldResources(outputDir):
			// Its own credentials: this sits outside the sandbox block on
			// purpose, so it cannot borrow an env built on a path it may
			// not have taken.
			cleanupEnv, cleanupEnvErr := sandboxCommandEnvForProject(runtime, runProjectID)
			if cleanupEnvErr != nil {
				stages = append(stages, StageSummary{
					Layer: "sandbox_deploy", Stage: "run_project_delete", Status: StageStatusFail,
				})
				failures = append(failures, FailureSummary{
					Layer: "sandbox_deploy", Stage: "run_project_delete", Check: "credentials",
					Command: "delete run project",
					Detail: fmt.Sprintf("project %s holds nothing but could not be deleted: %v",
						runProjectID, cleanupEnvErr),
				})
				break
			}
			deleteStages, deleteFailures := releaseRunProject(ctx, runtime, outputDir, runProjectID, cleanupEnv)
			stages = append(stages, deleteStages...)
			failures = append(failures, deleteFailures...)
		default:
			stages = append(stages, StageSummary{
				Layer: "sandbox_deploy", Stage: "run_project_delete", Status: StageStatusSkip,
				Detail: fmt.Sprintf(
					"kept %s: this run may still have resources, and the project is the handle to them. "+
						"Destroy them, then delete it by hand", runProjectID),
			})
		}
	}

	status := CommandStatusSuccess
	if len(failures) > 0 {
		status = CommandStatusFailed
	}

	result := OutputResult{
		Command:      "test",
		Scenario:     sc.Name,
		Status:       status,
		Stages:       stages,
		Failures:     failures,
		PlanLiveText: planLiveText,
	}
	if status == CommandStatusFailed {
		return result, &CLIError{
			Op:   "test",
			Code: errorCodeCommandFailed,
			Err:  errors.New("test checks failed"),
		}
	}

	return result, nil
}

// liveStateMayHoldResources reports whether Layer 3 state might still
// describe real infrastructure.
//
// It fails CLOSED: it answers true unless it can positively prove
// otherwise. The one case it can be sure about is a state that parses
// cleanly and records no resources -- which is exactly what a successful
// destroy leaves behind, and treating that as "resources may exist" turns
// a clean teardown into a spurious leak warning on the next pre-apply
// failure.
//
// Everything else gets cleanup. An unreadable or unparseable state means
// we cannot tell what is out there, and "we cannot tell" must never look
// like "nothing is there" on a path that spends real money.
func liveStateMayHoldResources(outputDir string) bool {
	raw, err := os.ReadFile(filepath.Join(outputDir, harness.LiveStateFilename))
	if err != nil {
		// Absence is the ONLY read error that means "nothing was applied".
		// A permissions or I/O error means the file is there and we cannot
		// read it, which is the definition of not knowing -- and on this
		// path not knowing must never be mistaken for a clean account.
		return !os.IsNotExist(err)
	}
	var state struct {
		Resources []json.RawMessage `json:"resources"`
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return true
	}
	return len(state.Resources) > 0
}

// realScalewayAPIURL is the only endpoint a Layer 3 apply may target.
const realScalewayAPIURL = "https://api.scaleway.com"

// sandboxCommandEnv builds the environment for a Layer 3 (real Scaleway)
// subprocess.
//
// Unlike cloudEnv, which deliberately points every provider at a local
// mock, this env must reach the real API -- and must be provably unable
// to reach anything else. The keys returned here are only half of that
// guarantee; the other half is harness.SandboxStripEnv, which removes
// the inherited overrides an override map cannot unset.
// assertSandboxCredentials checks that Layer 3 can run at all, BEFORE
// the run's project is created. It deliberately returns no environment.
//
// It used to return one, as `sandboxCommandEnv`, and that env had no
// provider default project. Three teardown paths picked it up and
// destroyed against the shared fallback while their apply had run in the
// run's own project -- passes 33 and 34, the same mistake each time,
// invited by a name that reads like "the env for a sandbox command".
// Handing back nothing usable makes that misuse impossible rather than
// merely discouraged: every caller that needs an env must now say which
// project it is for.
func assertSandboxCredentials(runtime *CommandRuntime) error {
	_, err := sandboxCommandEnvForProject(runtime, "")
	return err
}

// sandboxCommandEnvForProject is sandboxCommandEnv with an explicit
// provider default project.
//
// ADR-0025 needs this seam: when the run creates its own project before
// the apply, SCW_DEFAULT_PROJECT_ID must point at THAT project rather
// than the shared fallback, so a resource carrying no project_id of its
// own -- scaleway_instance_private_nic has no such attribute -- lands
// somewhere disposable and swept rather than somewhere shared.
//
// An empty runProjectID keeps the pre-ADR-0025 behaviour exactly.
func sandboxCommandEnvForProject(runtime *CommandRuntime, runProjectID string) (map[string]string, error) {
	accessKey := strings.TrimSpace(os.Getenv("SCW_ACCESS_KEY"))
	if accessKey == "" {
		return nil, fmt.Errorf("sandbox deploy requires SCW_ACCESS_KEY in the environment")
	}
	secretKey := strings.TrimSpace(os.Getenv("SCW_SECRET_KEY"))
	if secretKey == "" {
		return nil, fmt.Errorf("sandbox deploy requires SCW_SECRET_KEY in the environment")
	}
	// Required because ADR-0010 has the run create its own project, and a
	// project has to be created in some organization. A resource-scoped
	// key without an org will fail at the first resource, so surface it
	// here rather than several minutes into an apply.
	orgID := strings.TrimSpace(os.Getenv("SCW_DEFAULT_ORGANIZATION_ID"))
	if orgID == "" {
		return nil, fmt.Errorf("sandbox deploy requires SCW_DEFAULT_ORGANIZATION_ID in the environment (scaleway_account_project needs an organization to create the run project in)")
	}

	region := strings.TrimSpace(runtime.Config.Scaleway.Region)
	if region == "" {
		region = "fr-par"
	}
	zone := strings.TrimSpace(runtime.Config.Scaleway.Zone)
	if zone == "" {
		zone = "fr-par-1"
	}

	env := map[string]string{
		"SCW_ACCESS_KEY":              accessKey,
		"SCW_SECRET_KEY":              secretKey,
		"SCW_DEFAULT_ORGANIZATION_ID": orgID,
		"SCW_DEFAULT_REGION":          region,
		"SCW_DEFAULT_ZONE":            zone,
	}
	// Contain strays. harness.SandboxStripEnv removes any inherited
	// SCW_DEFAULT_PROJECT_ID so an ambient value cannot pin the run to
	// someone else's project; setting our own here decides where a
	// resource that omits project_id actually lands. Without it the
	// provider falls through to the default project in
	// ~/.config/scw/config.yaml -- typically the organization default,
	// next to real infrastructure.
	//
	// The run's own project takes precedence when there is one. The
	// organization-default refusal below applies to it too: the check is
	// about where strays land, and that reasoning does not change with
	// where the project id came from.
	projectDefault := strings.TrimSpace(runtime.Config.Scaleway.FallbackProjectID)
	source := "scaleway.fallback_project_id"
	if trimmed := strings.TrimSpace(runProjectID); trimmed != "" {
		projectDefault = trimmed
		source = "the run's own project"
	}
	if projectDefault != "" {
		if projectDefault == orgID {
			return nil, fmt.Errorf("%s must not be the organization's default project (%s): a stray resource would land next to real infrastructure, which is exactly what this setting exists to prevent", source, orgID)
		}
		env["SCW_DEFAULT_PROJECT_ID"] = projectDefault
	}

	if err := assertRealScalewayEndpoint(env); err != nil {
		return nil, err
	}
	return env, nil
}

// assertRealScalewayEndpoint fails closed unless the provider will talk
// to real Scaleway.
//
// Two ways an apply can be misdirected. The env is the obvious one and
// harness.SandboxStripEnv already removes it -- this re-checks the map
// we are about to hand over, so a future edit that reintroduces the key
// fails here instead of silently applying against a mock.
//
// The config file is the subtle one: the Scaleway SDK reads
// ~/.config/scw/config.yaml whether or not any SCW_* var is set, and
// its top-level keys are the default profile. A developer whose config
// carries an api_url pointing somewhere else would get an apply that
// never touches Scaleway while every stage reports pass. Stripping
// SCW_CONFIG_PATH / SCW_PROFILE forces the default profile; this checks
// what that profile actually says.
func assertRealScalewayEndpoint(env map[string]string) error {
	if v := strings.TrimSpace(env["SCW_API_URL"]); v != "" && v != realScalewayAPIURL {
		return fmt.Errorf("refusing to run a Layer 3 apply against %q: sandbox deploy targets real Scaleway (%s) only", v, realScalewayAPIURL)
	}
	if v := strings.TrimSpace(os.Getenv("SCW_API_URL")); v != "" && v != realScalewayAPIURL {
		return fmt.Errorf("refusing to run a Layer 3 apply: inherited SCW_API_URL=%q would retarget it away from real Scaleway (%s). This is usually a leftover from driving mockway by hand", v, realScalewayAPIURL)
	}
	apiURL, err := scwConfigFileAPIURL()
	if err != nil {
		return fmt.Errorf("cannot verify the Layer 3 endpoint: %w", err)
	}
	if apiURL != "" && apiURL != realScalewayAPIURL {
		return fmt.Errorf("refusing to run a Layer 3 apply: the default profile in %s sets api_url=%q, not %s", scwConfigPath(), apiURL, realScalewayAPIURL)
	}
	return nil
}

func scwConfigPath() string {
	if base := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); base != "" {
		return filepath.Join(base, "scw", "config.yaml")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "scw", "config.yaml")
}

// scwConfigFileAPIURL returns the default profile's api_url, or "" when
// the file is absent or does not set one (the SDK then uses the real
// endpoint). Only the top-level key is read: named entries under
// `profiles:` are unreachable because SCW_PROFILE is stripped.
func scwConfigFileAPIURL() (string, error) {
	path := scwConfigPath()
	if path == "" {
		return "", nil
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	var parsed struct {
		APIURL string `yaml:"api_url"`
	}
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("parse %s: %w", path, err)
	}
	return strings.TrimSpace(parsed.APIURL), nil
}

func evaluateSupportedCriteria(ctx context.Context, sc scenario.Scenario, runtime *CommandRuntime, deployResult *harness.MockDeployResult) ([]StageSummary, []FailureSummary) {
	if deployResult == nil {
		return nil, nil
	}

	specs, err := sc.ExecutableChecks()
	if err != nil {
		return []StageSummary{
				{Layer: "criteria", Stage: "parse", Status: StageStatusFail},
			}, []FailureSummary{
				{
					Layer:   "criteria",
					Stage:   "parse",
					Check:   "criteria_parse",
					Command: "criteria mapper",
					Detail:  err.Error(),
				},
			}
	}

	policySpecs := make([]scenario.ExecutableCheckSpec, 0)
	topologyChecks := make([]harness.TopologyCheck, 0)
	realProbeChecks := make([]harness.ProbeCheck, 0)
	sandboxEnabled := runtime.Config.Validation.Layers.SandboxDeploy.Enabled
	for _, spec := range specs {
		_, supported, _ := criteriaSupportReason(spec.Type, sandboxEnabled)
		if !supported {
			continue
		}
		switch spec.Type {
		case "policy":
			policySpecs = append(policySpecs, spec)
		case "connectivity":
			if spec.Connectivity == nil {
				continue
			}
			topologyChecks = append(topologyChecks, harness.TopologyCheck{
				Type:   spec.Type,
				From:   spec.Connectivity.From,
				To:     spec.Connectivity.To,
				Port:   spec.Connectivity.Port,
				Expect: spec.Expect,
			})
			realProbeChecks = append(realProbeChecks, harness.ProbeCheck{
				Type:   spec.Type,
				Expect: spec.Expect,
				From:   spec.Connectivity.From,
				To:     spec.Connectivity.To,
				Port:   spec.Connectivity.Port,
			})
		case "http_probe":
			if spec.HTTPProbe == nil {
				continue
			}
			topologyChecks = append(topologyChecks, harness.TopologyCheck{
				Type:   spec.Type,
				Target: spec.HTTPProbe.Target,
				Port:   spec.HTTPProbe.Port,
				Expect: spec.Expect,
			})
			realProbeChecks = append(realProbeChecks, harness.ProbeCheck{
				Type:   spec.Type,
				Expect: spec.Expect,
				Target: spec.HTTPProbe.Target,
				Port:   spec.HTTPProbe.Port,
			})
		case "dns_resolution":
			if spec.DNSResolution == nil {
				continue
			}
			realProbeChecks = append(realProbeChecks, harness.ProbeCheck{
				Type:   spec.Type,
				Expect: spec.Expect,
				Domain: spec.DNSResolution.Domain,
			})
		}
	}

	stages := make([]StageSummary, 0, 2)
	failures := make([]FailureSummary, 0)

	if len(policySpecs) > 0 {
		policyFailures := evaluateStatePolicyCriteria(ctx, runtime, sc.Cloud, deployResult.StateSnapshot, policySpecs)
		if len(policyFailures) > 0 {
			stages = append(stages, StageSummary{
				Layer:  "mock_deploy",
				Stage:  "state_policy",
				Status: StageStatusFail,
				Detail: fmt.Sprintf("%d policy failures", len(policyFailures)),
			})
			failures = append(failures, policyFailures...)
		} else {
			stages = append(stages, StageSummary{Layer: "mock_deploy", Stage: "state_policy", Status: StageStatusPass})
		}
	}

	if sandboxEnabled && len(realProbeChecks) > 0 {
		probeResult, err := runtime.Deps.RealProbe.Run(ctx, runtime.OutputDir(), sc.Name, realProbeChecks)
		if err != nil {
			stages = append(stages, StageSummary{Layer: "sandbox_deploy", Stage: "real_probe", Status: StageStatusFail})
			failures = append(failures, FailureSummary{
				Layer:   "sandbox_deploy",
				Stage:   "real_probe",
				Check:   "real_probe",
				Command: "real probe harness",
				Detail:  err.Error(),
			})
		} else if len(probeResult.Failures) > 0 {
			stages = append(stages, StageSummary{
				Layer:  "sandbox_deploy",
				Stage:  "real_probe",
				Status: StageStatusFail,
				Detail: fmt.Sprintf("%d probe failures", len(probeResult.Failures)),
			})
			for _, failure := range probeResult.Failures {
				failures = append(failures, toFailureSummary(failure))
			}
		} else {
			stages = append(stages, StageSummary{Layer: "sandbox_deploy", Stage: "real_probe", Status: StageStatusPass})
		}
	} else if len(topologyChecks) > 0 {
		topologyFailures, err := harness.EvaluateTopology(deployResult.StateSnapshot, topologyChecks)
		if err != nil {
			stages = append(stages, StageSummary{Layer: "mock_deploy", Stage: "topology", Status: StageStatusFail})
			failures = append(failures, FailureSummary{
				Layer:   "mock_deploy",
				Stage:   "topology",
				Check:   "topology",
				Command: "topology evaluator",
				Detail:  err.Error(),
			})
		} else if len(topologyFailures) > 0 {
			stages = append(stages, StageSummary{
				Layer:  "mock_deploy",
				Stage:  "topology",
				Status: StageStatusFail,
				Detail: fmt.Sprintf("%d topology failures", len(topologyFailures)),
			})
			for _, failure := range topologyFailures {
				failures = append(failures, toFailureSummary(failure))
			}
		} else {
			stages = append(stages, StageSummary{Layer: "mock_deploy", Stage: "topology", Status: StageStatusPass})
		}
	}

	return stages, failures
}

// cloudConstraintPolicies maps a scenario's cloud to (criteria check
// name → policy path) so a `cloud: gcp` scenario with
// `check: encryption_at_rest` is routed to policies/gcp/encryption.rego
// instead of the Scaleway-only encryption_at_rest.rego that would
// otherwise vacuously pass on a google_*-only plan. Closes the
// cross-cloud bypass M37 was tracking.
var cloudConstraintPolicies = map[string]map[string]string{
	"gcp": {
		"encryption_at_rest":  "gcp/encryption.rego",
		"no_public_endpoints": "gcp/no_public_sql.rego",
		"no_public_database":  "gcp/no_public_sql.rego",
		// region_restriction is the post-S51 criterion check name
		// (matches the .rego filename); `region`/`zone` kept as
		// legacy aliases for pre-S51 scenarios.
		"region_restriction": "gcp/region_restriction.rego",
		"region":             "gcp/region_restriction.rego",
		"zone":               "gcp/region_restriction.rego",
	},
	"aws": {
		"encryption_at_rest":  "aws/encryption.rego",
		"no_public_endpoints": "aws/no_public_db.rego",
		"no_public_database":  "aws/no_public_db.rego",
		"vpc_required":        "aws/vpc_required.rego",
		"region_restriction":  "aws/region_restriction.rego",
		"region":              "aws/region_restriction.rego",
	},
	// S118: genesys policy checks for the 5 genesys training scenarios.
	// Three policies cover the CCaaS surface — region restriction
	// (mirroring scaleway/gcp/aws shape), queue→wrapup associations,
	// and OAuth client least-privilege.
	"genesys": {
		"region_restriction":           "genesys/region_restriction.rego",
		"region":                       "genesys/region_restriction.rego",
		"queue_must_have_wrapup":       "genesys/queue_must_have_wrapup.rego",
		"oauth_client_least_privilege": "genesys/oauth_client_least_privilege.rego",
	},
}

func evaluateStatePolicyCriteria(ctx context.Context, runtime *CommandRuntime, cloud string, stateSnapshot []byte, specs []scenario.ExecutableCheckSpec) []FailureSummary {
	failures := make([]FailureSummary, 0)

	for _, spec := range specs {
		if spec.Policy == nil {
			continue
		}

		// Per-cloud lookup first; fall back to the flat
		// `constraint_policies` map (which is Scaleway-shaped today).
		policyPath := ""
		if cloudMap, ok := cloudConstraintPolicies[cloud]; ok {
			if p, ok := cloudMap[spec.Policy.Check]; ok {
				policyPath = p
			}
		}
		if policyPath == "" {
			if p, ok := runtime.Config.ConstraintPolicies[spec.Policy.Check]; ok {
				policyPath = p
			}
		}
		if policyPath == "" {
			failures = append(failures, FailureSummary{
				Layer:   "mock_deploy",
				Stage:   "state_policy",
				Check:   "policy",
				Policy:  spec.Policy.Check,
				Command: "state policy evaluator",
				Detail:  "no constraint_policies mapping found for criteria check",
			})
			continue
		}
		policyPath = resolveConstraintPolicyPath(runtime.Config.Paths.Policies, policyPath)

		extraInput := map[string]any{}
		if spec.Policy.Target != "" {
			extraInput["target"] = spec.Policy.Target
		}

		evaluatedFailures, err := harness.EvaluateStatePoliciesWithInput(ctx, stateSnapshot, extraInput, []string{policyPath})
		if err != nil {
			failures = append(failures, FailureSummary{
				Layer:   "mock_deploy",
				Stage:   "state_policy",
				Check:   "policy",
				Policy:  spec.Policy.Check,
				Command: "state policy evaluator",
				Detail:  err.Error(),
			})
			continue
		}

		switch spec.Expect {
		case "pass":
			for _, evaluated := range evaluatedFailures {
				summary := toFailureSummary(evaluated)
				summary.Policy = spec.Policy.Check
				failures = append(failures, summary)
			}
		case "fail":
			if len(evaluatedFailures) == 0 {
				failures = append(failures, FailureSummary{
					Layer:   "mock_deploy",
					Stage:   "state_policy",
					Check:   "policy",
					Policy:  spec.Policy.Check,
					Command: "state policy evaluator",
					Detail:  "expected policy failure but evaluator returned pass",
				})
			}
		default:
			failures = append(failures, FailureSummary{
				Layer:   "mock_deploy",
				Stage:   "state_policy",
				Check:   "policy",
				Policy:  spec.Policy.Check,
				Command: "state policy evaluator",
				Detail:  fmt.Sprintf("unsupported policy expectation %q", spec.Expect),
			})
		}
	}

	return failures
}

func resolveConstraintPolicyPath(baseDir, policyPath string) string {
	if policyPath == "" || filepath.IsAbs(policyPath) {
		return policyPath
	}
	if _, err := os.Stat(policyPath); err == nil {
		return policyPath
	}
	if baseDir == "" {
		return policyPath
	}
	return filepath.Join(baseDir, policyPath)
}

// appendOrphanSweepResult asks the real API whether the run leaked.
//
// Before this, `destruction: no_orphans` was evaluated against mockway
// state even for Layer 3 runs, so a destroy that half-worked reported
// clean while real resources kept billing. A destroy exiting 0 is not
// evidence that nothing survived.
func appendOrphanSweepResult(ctx context.Context, stages []StageSummary, failures []FailureSummary, runtime *CommandRuntime, target *harness.SweepTarget, captureErr error, sandboxEnv map[string]string) ([]StageSummary, []FailureSummary) {
	if captureErr != nil {
		stages = append(stages, StageSummary{Layer: "sandbox_deploy", Stage: "orphan_sweep", Status: StageStatusFail})
		return stages, append(failures, FailureSummary{
			Layer: "sandbox_deploy", Stage: "orphan_sweep", Check: "no_orphans",
			Command: "capture sweep target", Detail: captureErr.Error(),
		})
	}
	result, err := runtime.Deps.OrphanSweep.Run(ctx, target, sandboxEnv["SCW_SECRET_KEY"])
	if err != nil {
		stages = append(stages, StageSummary{Layer: "sandbox_deploy", Stage: "orphan_sweep", Status: StageStatusFail})
		return stages, append(failures, FailureSummary{
			Layer:   "sandbox_deploy",
			Stage:   "orphan_sweep",
			Check:   "no_orphans",
			Command: "orphan sweep",
			Detail:  err.Error(),
		})
	}
	if result.Clean() {
		stages = append(stages, StageSummary{
			Layer:  "sandbox_deploy",
			Stage:  "orphan_sweep",
			Status: StageStatusPass,
			Detail: "project " + result.ProjectID + " destroyed; no resources left outside it",
		})
		return stages, failures
	}
	stages = append(stages, StageSummary{Layer: "sandbox_deploy", Stage: "orphan_sweep", Status: StageStatusFail})
	for _, f := range result.Failures {
		failures = append(failures, FailureSummary{
			Layer:   f.Layer,
			Stage:   f.Stage,
			Check:   f.Check,
			Command: f.Command,
			Detail:  f.Detail,
		})
	}
	return stages, failures
}
