package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	out := cmd.OutOrStdout()

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
	deploymentID := newDeploymentID(sc.Name, time.Now())
	workDir := deploymentWorkDir(runtime.LiveStoreRoot(), deploymentID)
	if err := copyDeploySource(source, workDir); err != nil {
		return &CLIError{Op: "deploy", Code: errorCodeCommandFailed, Err: err}
	}

	_, _ = fmt.Fprintf(out, "Deploying %s (%s) for %s\n", sc.Name, sc.Service.Ref(), ttl)
	_, _ = fmt.Fprintf(out, "  workdir: %s\n", workDir)

	store := livestore.NewFilesystemStore(runtime.LiveStoreRoot())
	deployResult, deployErr := runtime.Deps.SandboxDeploy.Run(ctx, workDir, sandboxEnv)
	stages, failures := appendSandboxDeployResult(nil, nil, deployResult, deployErr)

	// Registered from whatever the state shows, whether or not the apply
	// succeeded. A half-finished apply leaves real resources behind, and
	// the record is the only thing that will bring the reaper back to
	// them -- so it is written on the failure path too, not just the
	// happy one.
	registerStages, registerFailures := registerDeployment(store, sc, deploymentID, workDir, ttl)
	stages = append(stages, registerStages...)
	failures = append(failures, registerFailures...)

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
		return &CLIError{Op: "deploy", Code: errorCodeCommandFailed, Err: fmt.Errorf(
			"deploy failed; if resources were created they are recorded as %s — tear it down with `infrafactory live teardown %s`",
			deploymentID, deploymentID)}
	}

	_, _ = fmt.Fprintf(out, "\nDeployed as %s. It expires in %s; `infrafactory live reap` destroys it then.\n", deploymentID, ttl)

	return nil
}

// registerDeployment records what the apply created. A deployment whose
// state names no project cannot be reaped, so that is a failure rather
// than an empty record: something may be running that nothing tracks.
func registerDeployment(
	store *livestore.FilesystemStore,
	sc scenario.Scenario,
	deploymentID, workDir string,
	ttl time.Duration,
) ([]StageSummary, []FailureSummary) {
	statePath := filepath.Join(workDir, harness.LiveStateFilename)
	if _, err := os.Stat(statePath); errors.Is(err, os.ErrNotExist) {
		// No state at all means the apply never created anything. There
		// is nothing to record and nothing to reap.
		return []StageSummary{{
			Layer: "live", Stage: "register", Status: StageStatusSkip,
			Detail: "no live state written, so nothing was created",
		}}, nil
	}

	projectID, err := harness.RunProjectIDFromState(workDir)
	if err != nil || projectID == "" {
		detail := fmt.Sprintf(
			"live state in %s names no %s, so a deployment that may be running cannot be recorded or reaped. Destroy it by hand",
			workDir, harness.ProjectResourceType)
		if err != nil {
			detail = fmt.Sprintf("read live state in %s: %v", workDir, err)
		}
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

// newDeploymentID is scenario-scoped and timestamped so two deployments
// of the same scenario never collide -- which is the whole reason each
// gets its own workdir.
func newDeploymentID(scenarioName string, now time.Time) string {
	return fmt.Sprintf("%s-%s", scenarioName, now.UTC().Format("20060102T150405Z"))
}
