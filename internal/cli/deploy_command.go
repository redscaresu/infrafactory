package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/redscaresu/infrafactory/internal/livestore"
	"github.com/redscaresu/infrafactory/internal/scenario"
)

// deployStateFiles are copied from the validated output directory into
// the deployment's own workdir. The lock file comes with them so the
// apply uses the provider build the validation ran against, rather than
// whatever the registry resolves at init time.
var deployCopiedExtensions = []string{".tf", ".hcl"}

// newDeployCmd puts a validated change up and leaves it running.
//
// Deliberately a separate verb from `run`, and deliberately does not
// generate. `run` proves a change is safe -- generate, validate, mock
// apply, real apply, destroy, sweep. `deploy` takes what `run` already
// validated and keeps it. Splitting them means deploy cannot become a
// way to skip the layers: it applies HCL that has already been through
// them, in its own workdir, and records the result.
func newDeployCmd(cfg *rootConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy <scenario>",
		Short: "Apply a validated scenario to real infrastructure and leave it running under a TTL",
		Args:  requireScenarioArg,
		RunE:  cfg.withRuntime("deploy", runDeployCommand),
	}

	cmd.Flags().String("ttl", "", "Override the scenario's service.ttl (e.g. 2h). Still bounded by the schema maximum")

	return cmd
}

func runDeployCommand(cmd *cobra.Command, args []string, runtime *CommandRuntime) error {
	ctx := cmd.Context()
	// Progress on stderr; stdout carries the output contract, and mixing
	// them makes --output json unparseable from the first byte.
	progress := cmd.ErrOrStderr()

	sc, err := runtime.LoadScenario(args[0])
	if err != nil {
		return &CLIError{Op: "deploy", Code: errorCodeUsage, Err: err}
	}

	// Only a scenario that names a versioned application can be deployed
	// as a live service. Without one there is nothing whose version
	// could be rolled forward, and "deploy" would just mean "apply and
	// forget to destroy".
	if sc.Service == nil {
		return &CLIError{Op: "deploy", Code: errorCodeUsage, Err: fmt.Errorf(
			"scenario %q declares no service: block, so there is no versioned application to deploy. "+
				"Use `infrafactory run` for infrastructure-only scenarios", sc.Name)}
	}

	ttl, err := deployTTL(cmd, *sc.Service)
	if err != nil {
		return &CLIError{Op: "deploy", Code: errorCodeUsage, Err: err}
	}

	if !runtime.Config.Validation.Layers.SandboxDeploy.Enabled {
		return &CLIError{Op: "deploy", Code: errorCodeCommandFailed, Err: errors.New(
			"deploy applies to real infrastructure and requires validation.layers.sandbox_deploy.enabled")}
	}

	source := runtime.OutputDir()
	if err := assertDeployableSource(source); err != nil {
		return &CLIError{Op: "deploy", Code: errorCodeUsage, Err: err}
	}

	// Deny-by-default on anything that may touch the cloud, exactly as
	// the PR gate does. deploy takes already-validated HCL, but "already
	// validated" is a claim about a previous command; this is the check.
	if err := layer3PreflightHCL(source, runtime.Config.Validation.Layers.SandboxDeploy.AllowResourceTypes); err != nil {
		return &CLIError{Op: "deploy", Code: errorCodeCommandFailed, Err: fmt.Errorf("layer 3 hcl validation: %w", err)}
	}

	sandboxEnv, err := sandboxCommandEnv(runtime)
	if err != nil {
		return &CLIError{Op: "deploy", Code: errorCodeCommandFailed, Err: err}
	}

	// Its own workdir, never the shared output directory. Two deployments
	// of the same scenario would otherwise share one
	// terraform-live.tfstate, and the second apply would overwrite the
	// first one's -- orphaning real resources with nothing left that
	// knows how to destroy them.
	// Store first: NewFilesystemStore resolves Root to an absolute path,
	// and the workdir is derived from it so the recorded WorkDir is
	// absolute too. A relative one would be unusable to a reaper running
	// from any other directory.
	store := livestore.NewFilesystemStore(runtime.LiveStoreRoot())
	deploymentID := newDeploymentID(sc.Name, time.Now())
	workDir := deploymentWorkDir(store.Root, deploymentID)
	if err := copyDeploySource(source, workDir); err != nil {
		return &CLIError{Op: "deploy", Code: errorCodeCommandFailed, Err: err}
	}

	_, _ = fmt.Fprintf(progress, "Deploying %s (%s) for %s\n", sc.Name, sc.Service.Ref(), ttl)
	_, _ = fmt.Fprintf(progress, "  workdir: %s\n", workDir)

	// The deployment gets its own project, exactly as a run does. It
	// simply outlives the command: `live teardown` deletes it once the
	// destroy and sweep prove the account clean.
	runProjectID, runProjectStages, runProjectFailures := ensureRunProject(ctx, runtime, sc.Name, workDir)
	stages := append([]StageSummary{}, runProjectStages...)
	failures := append([]FailureSummary{}, runProjectFailures...)
	if len(runProjectFailures) > 0 {
		return &CLIError{Op: "deploy", Code: errorCodeCommandFailed, Err: fmt.Errorf(
			"could not create the deployment's project, so nothing was applied")}
	}

	sandboxEnv, err = sandboxCommandEnvForProject(runtime, runProjectID)
	if err != nil {
		return &CLIError{Op: "deploy", Code: errorCodeCommandFailed, Err: err}
	}

	deployResult, deployErr := runDeployApply(cmd, ctx, signal.NotifyContext, func(applyCtx context.Context) (*harness.SandboxDeployResult, error) {
		return runtime.Deps.SandboxDeploy.Run(applyCtx, workDir, sandboxEnv)
	})
	stages, failures = appendSandboxDeployResult(stages, failures, deployResult, deployErr)

	// Registered from whatever the state shows, whether or not the apply
	// succeeded. A half-finished apply leaves real resources behind, and
	// the record is the only thing that will bring the reaper back to
	// them -- so it is written on the failure path too, not just the
	// happy one.
	registerStages, registerFailures := registerDeployment(store, sc, deploymentID, workDir, runProjectID, ttl)
	stages = append(stages, registerStages...)
	failures = append(failures, registerFailures...)
	recorded := len(registerFailures) == 0 && len(registerStages) > 0 && registerStages[0].Status == StageStatusPass

	status := CommandStatusSuccess
	if len(failures) > 0 {
		status = CommandStatusFailed
	}

	if err := writeCommandOutput(cmd, OutputResult{
		Command:  "deploy",
		Scenario: sc.Name,
		Status:   status,
		Stages:   stages,
		Failures: failures,
	}); err != nil {
		return err
	}

	if status == CommandStatusFailed {
		// Only point at teardown when a record actually exists. Telling
		// an operator to reap something that was never recorded yields
		// "no such file or directory" as a usage error, which reads like
		// they mistyped rather than like nothing was created.
		// Keyed off whether registration SUCCEEDED, not off whether a file
		// happens to exist. registerDeployment fails precisely when a
		// project is live and could not be recorded -- saying "nothing to
		// tear down" there contradicts its own failure detail and
		// reassures the operator that nothing leaked.
		recovery := "resources may be live and could NOT be recorded — see the failure detail above and destroy by hand"
		switch {
		case recorded:
			recovery = fmt.Sprintf("tear it down with `infrafactory live teardown %s`", deploymentID)
		case len(registerFailures) == 0:
			recovery = "no resources were recorded, so there is nothing to tear down"
		}
		return &CLIError{Op: "deploy", Code: errorCodeCommandFailed, Err: fmt.Errorf(
			"deploy failed; %s", recovery)}
	}

	_, _ = fmt.Fprintf(progress, "\nDeployed as %s. It expires in %s; `infrafactory live reap` destroys it then.\n", deploymentID, ttl)

	return nil
}

// runDeployApply runs the real apply under a SIGINT/SIGTERM handler.
//
// Without one, Ctrl-C during a ~140s apply kills the process outright:
// scaleway_account_project and everything after it already exist, but
// the record is written after the apply returns, so nothing is written
// at all. `live ls` then says "No live deployments", `live reap` says
// "Nothing has expired", and the project bills indefinitely.
//
// Catching the signal cancels the apply's context instead, the harness
// unwinds, and the caller reaches registerDeployment -- which reads
// whatever state exists and records it, so the reaper can find it. That
// is deliberately different from run/test's guard, which DESTROYS on
// interrupt: deploy's whole purpose is that things stay up, so an
// interrupted deploy is recorded and left to its TTL rather than torn
// down out from under the operator.
//
// stop() runs before returning, so a second Ctrl-C kills the process
// outright rather than being swallowed.
func runDeployApply(
	cmd *cobra.Command,
	ctx context.Context,
	notify func(context.Context, ...os.Signal) (context.Context, context.CancelFunc),
	apply func(context.Context) (*harness.SandboxDeployResult, error),
) (*harness.SandboxDeployResult, error) {
	sigCtx, stop := notify(ctx, os.Interrupt, syscall.SIGTERM)
	result, err := apply(sigCtx)
	// A signal, not merely a cancelled parent. sigCtx derives from ctx, so
	// sigCtx.Err() is non-nil for a command timeout or an SDK cancel too --
	// reporting those as "interrupted" names the wrong cause.
	interrupted := ctx.Err() == nil && sigCtx.Err() != nil

	// Restored before the message: from here on there is only the
	// registration write, so a second signal should terminate normally
	// rather than be swallowed. The message therefore does not invite one
	// -- by the time it prints there is nothing left to abandon.
	stop()

	if interrupted {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
			"\nInterrupted during apply. Recording whatever was created so `infrafactory live reap` can destroy it.\n")
	}

	return result, err
}

// registerDeployment records what the apply created. A deployment whose
// state names no project cannot be reaped, so that is a failure rather
// than an empty record: something may be running that nothing tracks.
func registerDeployment(
	store *livestore.FilesystemStore,
	sc scenario.Scenario,
	deploymentID, workDir, runProjectID string,
	ttl time.Duration,
) ([]StageSummary, []FailureSummary) {
	// Deliberately NOT gated on live state existing. Since ADR-0025 the
	// project is created before the apply, so an apply that fails at
	// preflight, init or plan leaves a real project behind with no state
	// to show for it. Skipping registration there would hide it from
	// `live teardown` entirely -- the leak this record exists to prevent.

	// The run's own project, not one derived from state: under ADR-0025
	// the project is not a Terraform resource, so the state never names
	// it.
	projectID := runProjectID
	if strings.TrimSpace(projectID) == "" {
		detail := fmt.Sprintf(
			"no run project was recorded for %s, so a deployment that may be running cannot be reaped. Destroy it by hand",
			workDir)
		return []StageSummary{{Layer: "live", Stage: "register", Status: StageStatusFail}},
			[]FailureSummary{{
				Layer: "live", Stage: "register", Check: "project_id",
				Command: "deploy", Detail: detail,
			}}
	}

	// Best effort: an unreachable address is worth recording as empty
	// rather than failing a deploy that otherwise worked.
	address, _ := harness.LiveEndpoint(workDir, "load_balancer")

	now := time.Now()
	d := livestore.Deployment{
		ID:        deploymentID,
		Scenario:  sc.Name,
		ProjectID: projectID,
		Address:   address,
		Image:     sc.Service.Image,
		Tag:       sc.Service.Tag,
		State:     livestore.StateLive,
		WorkDir:   workDir,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}

	if err := store.Put(d); err != nil {
		return []StageSummary{{Layer: "live", Stage: "register", Status: StageStatusFail}},
			[]FailureSummary{{
				Layer: "live", Stage: "register", Check: "record",
				Command: "deploy",
				Detail: fmt.Sprintf(
					"project %s is live but could not be recorded: %v. Destroy it by hand — nothing will reap it",
					projectID, err),
			}}
	}

	return []StageSummary{{
		Layer: "live", Stage: "register", Status: StageStatusPass,
		Detail: fmt.Sprintf("%s recorded (project %s, expires %s)",
			deploymentID, projectID, d.ExpiresAt.Format(time.RFC3339)),
	}}, nil
}

// deployTTL resolves the effective TTL, letting --ttl override the
// scenario while keeping the schema's bounds. An override that is
// unbounded or absurd is refused for the same reason the schema refuses
// one (ADR-0024).
func deployTTL(cmd *cobra.Command, spec scenario.ServiceSpec) (time.Duration, error) {
	override, err := cmd.Flags().GetString("ttl")
	if err != nil {
		return 0, fmt.Errorf("read --ttl flag: %w", err)
	}

	if strings.TrimSpace(override) == "" {
		return spec.TimeToLive()
	}

	overridden := spec
	overridden.TTL = override
	if err := overridden.Validate(); err != nil {
		return 0, fmt.Errorf("--ttl: %w", err)
	}

	return overridden.TimeToLive()
}

// assertDeployableSource refuses to deploy a directory that holds no
// HCL. `deploy` does not generate, so an empty output dir means the
// operator has not run `infrafactory run` yet and an apply would do
// nothing while still creating a project.
func assertDeployableSource(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("no generated HCL in %s: run `infrafactory run <scenario>` first (%w)", dir, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".tf") {
			return nil
		}
	}
	return fmt.Errorf("no .tf files in %s: run `infrafactory run <scenario>` first", dir)
}

func copyDeploySource(source, workDir string) error {
	// Refuse to reuse a workdir that already holds live state. Adopting
	// another deployment's state is how a project ends up running with
	// nothing tracking it, so this fails loudly rather than proceeding.
	if _, err := os.Stat(filepath.Join(workDir, harness.LiveStateFilename)); err == nil {
		return fmt.Errorf(
			"deployment workdir %s already holds %s: refusing to reuse it, because the apply would adopt another deployment's state",
			workDir, harness.LiveStateFilename)
	}

	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return fmt.Errorf("create deployment workdir: %w", err)
	}

	entries, err := os.ReadDir(source)
	if err != nil {
		return fmt.Errorf("read generated HCL: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !hasDeployableExtension(entry.Name()) {
			continue
		}
		payload, err := os.ReadFile(filepath.Join(source, entry.Name()))
		if err != nil {
			return fmt.Errorf("read %s: %w", entry.Name(), err)
		}
		if err := os.WriteFile(filepath.Join(workDir, entry.Name()), payload, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", entry.Name(), err)
		}
	}

	return nil
}

func hasDeployableExtension(name string) bool {
	for _, ext := range deployCopiedExtensions {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}

// deploymentWorkDir keeps applied state in a subdirectory rather than
// beside the record files, so nothing in the store root is ambiguous
// between "a deployment record" and "a deployment's Terraform state".
func deploymentWorkDir(root, deploymentID string) string {
	return filepath.Join(root, "workdirs", deploymentID)
}

// newDeploymentID is scenario-scoped, timestamped, and carries random
// entropy.
//
// The timestamp alone was second-resolution, so two deploys of one
// scenario inside the same second produced the same id -- and therefore
// the same record path AND the same workdir. The second apply then
// adopted the first's terraform-live.tfstate and the second Put
// overwrote the first's record, leaving a project running with nothing
// that knew how to destroy it. That is the leak the per-deployment
// workdir exists to prevent, reintroduced by the id.
//
// The suffix is defence in depth, not the only guard: copyDeploySource
// also refuses a workdir that already holds state.
func newDeploymentID(scenarioName string, now time.Time) string {
	return fmt.Sprintf("%s-%s-%s", scenarioName, now.UTC().Format("20060102T150405Z"), randomIDSuffix())
}

// randomIDSuffix returns six hex characters. crypto/rand cannot fail in
// practice, but a fallback keeps a deploy from dying on entropy: the
// timestamp still separates ids in every realistic case, and
// copyDeploySource is the guard that actually prevents state adoption.
func randomIDSuffix() string {
	buf := make([]byte, 3)
	if _, err := rand.Read(buf); err != nil {
		return "000000"
	}
	return hex.EncodeToString(buf)
}
