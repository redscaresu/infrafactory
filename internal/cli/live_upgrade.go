package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/redscaresu/infrafactory/internal/livestore"
)

// PreviousHCLDirname holds the configuration a deployment was running
// before its most recent upgrade.
//
// Kept because it is the input S156 needs and cannot reconstruct.
// `ExtractFixPitfall` is the extractor that produces prescriptive rules,
// and it works by diffing a failing configuration against a passing one.
// A live failure has no "next iteration that fixed it" -- but an upgrade
// has a before and an after, which is the same shape. Discarding the
// previous HCL would leave live signals stuck at the weakest class of
// lesson.
const PreviousHCLDirname = ".infrafactory-previous"

// runLiveUpgradeCommand rolls a live deployment forward onto new
// configuration, in place (S155).
//
// infrafactory does not invent the new HCL. It owns the part that is
// hard to get right: applying it into the SAME project the deployment
// already owns, under the same guards a first deploy runs, and proving
// afterwards that the version actually changed.
//
// Deliberately not parameterised through TF_VAR. `SandboxStripEnv`
// removes TF_VAR_* because the cost bounds read a variable's DEFAULT to
// decide blast radius, so an injected variable would make those checks
// vouch for a number that never reaches the API. Handing over whole HCL
// keeps every existing check applying to the configuration that is
// actually applied.
func runLiveUpgradeCommand(cmd *cobra.Command, args []string, runtime *CommandRuntime) error {
	ctx := cmd.Context()
	progress := cmd.ErrOrStderr()

	source, err := cmd.Flags().GetString("from")
	if err != nil {
		return &CLIError{Op: "live upgrade", Code: errorCodeUsage, Err: err}
	}
	if strings.TrimSpace(source) == "" {
		return &CLIError{Op: "live upgrade", Code: errorCodeUsage, Err: fmt.Errorf(
			"--from is required: an upgrade applies new configuration, and there is nowhere else for it to come from")}
	}
	newTag, err := cmd.Flags().GetString("tag")
	if err != nil {
		return &CLIError{Op: "live upgrade", Code: errorCodeUsage, Err: err}
	}

	store := livestore.NewFilesystemStore(runtime.LiveStoreRoot())
	d, err := store.Get(args[0])
	if err != nil {
		return &CLIError{Op: "live upgrade", Code: errorCodeCommandFailed, Err: err}
	}
	if d.State == livestore.StateReleased {
		return &CLIError{Op: "live upgrade", Code: errorCodeCommandFailed, Err: fmt.Errorf(
			"%s was already torn down, so there is nothing running to upgrade", d.ID)}
	}
	if d.WorkDir == "" {
		return &CLIError{Op: "live upgrade", Code: errorCodeCommandFailed, Err: fmt.Errorf(
			"%s records no work_dir, so its existing state cannot be updated in place", d.ID)}
	}

	// The same opt-in `deploy` requires. An upgrade applies to real
	// infrastructure exactly as a first deploy does, and a gate that
	// guards one entry point and not the other guards nothing.
	if !runtime.Config.Validation.Layers.SandboxDeploy.Enabled {
		return &CLIError{Op: "live upgrade", Code: errorCodeCommandFailed, Err: errors.New(
			"live upgrade applies to real infrastructure and requires validation.layers.sandbox_deploy.enabled")}
	}

	if err := assertDeployableSource(source); err != nil {
		return &CLIError{Op: "live upgrade", Code: errorCodeUsage, Err: err}
	}
	// The same deny-by-default the first deploy ran. New configuration is
	// new configuration whether it arrives through `deploy` or here.
	if err := layer3PreflightHCL(source, runtime.Config.Validation.Layers.SandboxDeploy.AllowResourceTypes); err != nil {
		return &CLIError{Op: "live upgrade", Code: errorCodeCommandFailed, Err: fmt.Errorf("layer 3 hcl validation: %w", err)}
	}

	stages, failures := upgradePreflight(ctx, runtime, d)
	if len(failures) > 0 {
		return reportUpgrade(cmd, d, stages, failures)
	}

	// Refuse a source that IS the workdir, or lives inside it.
	//
	// replaceDeployedHCL removes the superseded .tf files before copying
	// the new ones in, so pointing --from at the deployment's own workdir
	// deletes the very files it is about to read: the workdir ends up
	// holding no configuration at all, for infrastructure that is still
	// running.
	if err := assertUpgradeSourceIsSeparate(source, d.WorkDir); err != nil {
		return &CLIError{Op: "live upgrade", Code: errorCodeUsage, Err: err}
	}

	// Every fallible step that does NOT touch the workdir happens first.
	//
	// The swap below is destructive, so anything that can fail after it
	// leaves the workdir holding configuration that was never applied.
	// Ordering removes that failure mode instead of compensating for it,
	// which is the difference between one rollback path and one per
	// early return.
	//
	// The deployment's OWN project, so the apply updates what is already
	// there rather than building a second copy somewhere else.
	sandboxEnv, err := sandboxCommandEnvForProject(runtime, d.ProjectID)
	if err != nil {
		return &CLIError{Op: "live upgrade", Code: errorCodeCommandFailed, Err: err}
	}

	_, _ = fmt.Fprintf(progress, "Upgrading %s in project %s\n", d.ID, d.ProjectID)

	if err := stashPreviousHCL(d.WorkDir); err != nil {
		return &CLIError{Op: "live upgrade", Code: errorCodeCommandFailed, Err: err}
	}
	if err := replaceDeployedHCL(source, d.WorkDir); err != nil {
		// The swap failed partway, so the workdir may hold a mixture.
		// Put back what was running rather than leaving it ambiguous.
		if restoreErr := restorePreviousHCL(d.WorkDir); restoreErr != nil {
			return &CLIError{Op: "live upgrade", Code: errorCodeCommandFailed, Err: fmt.Errorf(
				"%w; and the previous configuration could not be restored: %v (it is in %s)",
				err, restoreErr, PreviousHCLDirname)}
		}
		return &CLIError{Op: "live upgrade", Code: errorCodeCommandFailed, Err: err}
	}

	applyResult, applyErr := runDeployApply(cmd, ctx, signal.NotifyContext, func(applyCtx context.Context) (*harness.SandboxDeployResult, error) {
		return runtime.Deps.SandboxDeploy.Run(applyCtx, d.WorkDir, sandboxEnv)
	})
	stages, failures = appendSandboxDeployResult(stages, failures, applyResult, applyErr)

	// An init or plan failure never reached the cloud, so the deployment
	// is still running the OLD configuration -- and the workdir would be
	// left holding the new, rejected one. Every later operation would
	// then plan against configuration that was never applied. Put it
	// back, and say so.
	if !applyRan(applyErr) {
		if restoreErr := restorePreviousHCL(d.WorkDir); restoreErr != nil {
			failures = append(failures, FailureSummary{
				Layer: "live", Stage: "upgrade_rollback", Check: "restore",
				Command: "live upgrade " + d.ID,
				Detail: fmt.Sprintf(
					"%s was not applied, but its workdir still holds the rejected configuration and could not be reverted: %v. "+
						"The previous configuration is in %s",
					d.ID, restoreErr, PreviousHCLDirname),
			})
		} else {
			stages = append(stages, StageSummary{
				Layer: "live", Stage: "upgrade_rollback", Status: StageStatusPass,
				Detail: fmt.Sprintf(
					"nothing was applied, so %s keeps the configuration it was already running", d.ID),
			})
		}
	}

	// The record moves to the new tag only if the apply actually RAN.
	//
	// A half-finished apply is running something, and a record still
	// claiming the old version would send the next observation looking
	// for the wrong thing -- so a failure during apply still advances it.
	// But a failure at init or plan changed nothing at all, and advancing
	// the tag there would make the record claim a version that was never
	// deployed, which is the exact falsehood S155a exists to prevent.
	if applyRan(applyErr) {
		if strings.TrimSpace(newTag) != "" {
			d.Tag = strings.TrimSpace(newTag)
		}
		d.UpgradedAt = time.Now()

		// The address can move: replacement HCL may recreate the load
		// balancer. Verifying against the address captured at first
		// deploy would probe infrastructure this deployment no longer
		// owns, and leave every later observation pointed there too.
		if address, addrErr := harness.LiveEndpoint(d.WorkDir, "load_balancer"); addrErr == nil && address != "" {
			if address != d.Address {
				stages = append(stages, StageSummary{
					Layer: "live", Stage: "upgrade_address", Status: StageStatusPass,
					Detail: fmt.Sprintf("%s moved from %s to %s", d.ID, d.Address, address),
				})
			}
			d.Address = address
		} else if d.Address != "" {
			// Said out loud rather than assumed unchanged: everything
			// after this probes an address nothing just confirmed.
			stages = append(stages, StageSummary{
				Layer: "live", Stage: "upgrade_address", Status: StageStatusSkip,
				Detail: fmt.Sprintf(
					"could not re-read the endpoint after the apply, so %s is still assumed to serve at %s",
					d.ID, d.Address),
			})
		}
	}

	if err := store.Put(d); err != nil {
		failures = append(failures, FailureSummary{
			Layer: "live", Stage: "upgrade", Check: "record",
			Command: "live upgrade " + d.ID,
			Detail:  fmt.Sprintf("%s was applied but the record still names the old version: %v", d.ID, err),
		})
	}

	if applyErr == nil {
		stages, failures = appendUpgradeVerification(ctx, runtime, d, stages, failures)
	}

	return reportUpgrade(cmd, d, stages, failures)
}

// upgradePreflight refuses an upgrade whose starting point is not known.
//
// An upgrade from a version nobody confirmed proves nothing, and one
// from a version the service actively contradicts is worse: it would
// record a v1→v2 transition that never happened. Unchecked is allowed
// and said out loud; contradicted is not.
func upgradePreflight(ctx context.Context, runtime *CommandRuntime, d livestore.Deployment) ([]StageSummary, []FailureSummary) {
	if d.VersionPath == "" {
		return []StageSummary{{
			Layer: "live", Stage: "upgrade_preflight", Status: StageStatusSkip,
			Detail: fmt.Sprintf(
				"%s declares no version_path, so what it is running now is unverified and the upgrade cannot be proven to have changed anything",
				d.ID),
		}}, nil
	}

	before, detail := checkRunningVersion(ctx, runtime, d)
	switch before {
	case livestore.VersionUnconfirmed:
		return []StageSummary{{Layer: "live", Stage: "upgrade_preflight", Status: StageStatusFail}},
			[]FailureSummary{{
				Layer: "live", Stage: "upgrade_preflight", Check: "version_before",
				Command: "live upgrade " + d.ID,
				Detail: fmt.Sprintf(
					"refusing to upgrade %s: %s. Upgrading from a version the service contradicts would record a transition that never happened",
					d.ID, detail),
			}}
	case livestore.VersionUnchecked:
		return []StageSummary{{
			Layer: "live", Stage: "upgrade_preflight", Status: StageStatusSkip,
			Detail: fmt.Sprintf("%s: starting version unchecked (%s)", d.ID, detail),
		}}, nil
	}

	return []StageSummary{{
		Layer: "live", Stage: "upgrade_preflight", Status: StageStatusPass,
		Detail: fmt.Sprintf("%s confirms %s before the upgrade", d.ID, imageRef(d)),
	}}, nil
}

// appendUpgradeVerification is the point of the whole command.
//
// A successful apply says Terraform reached its desired state. It does
// NOT say the service is running the new version -- the instance may not
// have restarted, the image may not have been pulled, the user data may
// never have run. Reporting an upgrade on the strength of the apply is
// the same error as trusting the record over the service, one step
// later.
func appendUpgradeVerification(
	ctx context.Context,
	runtime *CommandRuntime,
	d livestore.Deployment,
	stages []StageSummary,
	failures []FailureSummary,
) ([]StageSummary, []FailureSummary) {
	if d.VersionPath == "" {
		return append(stages, StageSummary{
			Layer: "live", Stage: "upgrade_verify", Status: StageStatusSkip,
			Detail: "no version_path, so the apply is all there is to go on",
		}), failures
	}

	after, detail := checkRunningVersion(ctx, runtime, d)
	switch after {
	case livestore.VersionConfirmed:
		return append(stages, StageSummary{
			Layer: "live", Stage: "upgrade_verify", Status: StageStatusPass,
			Detail: fmt.Sprintf("%s now confirms %s", d.ID, imageRef(d)),
		}), failures
	case livestore.VersionUnconfirmed:
		return append(stages, StageSummary{Layer: "live", Stage: "upgrade_verify", Status: StageStatusFail}),
			append(failures, FailureSummary{
				Layer: "live", Stage: "upgrade_verify", Check: "version_after",
				Command: "live upgrade " + d.ID,
				Detail: fmt.Sprintf(
					"the apply succeeded but %s. An apply reaching its desired state is not the service running the new version",
					detail),
			})
	}

	return append(stages, StageSummary{
		Layer: "live", Stage: "upgrade_verify", Status: StageStatusSkip,
		Detail: fmt.Sprintf("%s: could not verify the new version (%s)", d.ID, detail),
	}), failures
}

// stashPreviousHCL copies the configuration currently deployed into
// PreviousHCLDirname before it is overwritten.
//
// One generation deep, deliberately: S156 needs the pair either side of
// one change, and keeping every generation would turn a workdir into an
// archive nobody prunes.
func stashPreviousHCL(workDir string) error {
	previous := filepath.Join(workDir, PreviousHCLDirname)
	if err := os.RemoveAll(previous); err != nil {
		return fmt.Errorf("clear previous hcl: %w", err)
	}
	if err := os.MkdirAll(previous, 0o755); err != nil {
		return fmt.Errorf("create previous hcl dir: %w", err)
	}

	entries, err := os.ReadDir(workDir)
	if err != nil {
		return fmt.Errorf("read deployment workdir: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !hasDeployableExtension(entry.Name()) {
			continue
		}
		payload, err := os.ReadFile(filepath.Join(workDir, entry.Name()))
		if err != nil {
			return fmt.Errorf("read %s: %w", entry.Name(), err)
		}
		if err := os.WriteFile(filepath.Join(previous, entry.Name()), payload, 0o644); err != nil {
			return fmt.Errorf("stash %s: %w", entry.Name(), err)
		}
	}
	return nil
}

// replaceDeployedHCL swaps the workdir's configuration for the new one.
//
// Old .tf files are removed first. Copying over the top would leave a
// resource behind that the new configuration no longer declares, and
// tofu would keep managing it -- an upgrade that silently kept something
// the operator had deleted.
func replaceDeployedHCL(source, workDir string) error {
	entries, err := os.ReadDir(workDir)
	if err != nil {
		return fmt.Errorf("read deployment workdir: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !hasDeployableExtension(entry.Name()) {
			continue
		}
		if err := os.Remove(filepath.Join(workDir, entry.Name())); err != nil {
			return fmt.Errorf("remove superseded %s: %w", entry.Name(), err)
		}
	}

	newEntries, err := os.ReadDir(source)
	if err != nil {
		return fmt.Errorf("read new hcl: %w", err)
	}
	for _, entry := range newEntries {
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

func reportUpgrade(cmd *cobra.Command, d livestore.Deployment, stages []StageSummary, failures []FailureSummary) error {
	status := CommandStatusSuccess
	if len(failures) > 0 {
		status = CommandStatusFailed
	}
	if err := writeCommandOutput(cmd, OutputResult{
		Command:  "live upgrade",
		Scenario: d.Scenario,
		Status:   status,
		Stages:   stages,
		Failures: failures,
	}); err != nil {
		return err
	}
	if status == CommandStatusFailed {
		return &CLIError{Op: "live upgrade", Code: errorCodeCommandFailed, Err: fmt.Errorf(
			"%s was not upgraded cleanly", d.ID)}
	}
	return nil
}

// applyRan reports whether the apply stage actually executed.
//
// A failure at init or plan changed nothing on the cloud, so the record
// must not move; a failure during apply may have changed a great deal,
// so it must. Anything unrecognised is treated as "it ran", because
// assuming nothing happened is the answer that loses infrastructure.
func applyRan(err error) bool {
	if err == nil {
		return true
	}
	var deployErr *harness.SandboxDeployError
	if errors.As(err, &deployErr) {
		return deployErr.Stage != "init" && deployErr.Stage != "plan"
	}
	return true
}

// assertUpgradeSourceIsSeparate refuses a source that is, or is inside,
// the deployment's own workdir.
//
// Symlinks are resolved before comparing, because the check protects
// against deleting the files about to be read and a symlinked path
// deletes them just as effectively as a literal one.
func assertUpgradeSourceIsSeparate(source, workDir string) error {
	resolve := func(path string) (string, error) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return "", err
		}
		if resolved, err := filepath.EvalSymlinks(abs); err == nil {
			return resolved, nil
		}
		return abs, nil
	}

	src, err := resolve(source)
	if err != nil {
		return fmt.Errorf("resolve --from %s: %w", source, err)
	}
	wd, err := resolve(workDir)
	if err != nil {
		return fmt.Errorf("resolve deployment workdir %s: %w", workDir, err)
	}

	if src == wd || strings.HasPrefix(src, wd+string(os.PathSeparator)) {
		return fmt.Errorf(
			"--from %s is the deployment's own workdir (%s): the superseded configuration is removed before "+
				"the new one is copied in, so this would leave the workdir with no configuration at all while "+
				"the infrastructure is still running",
			source, workDir)
	}
	return nil
}

// restorePreviousHCL puts the stashed configuration back, for an upgrade
// that never reached the cloud.
func restorePreviousHCL(workDir string) error {
	previous := filepath.Join(workDir, PreviousHCLDirname)
	entries, err := os.ReadDir(previous)
	if err != nil {
		return fmt.Errorf("read stashed configuration: %w", err)
	}

	current, err := os.ReadDir(workDir)
	if err != nil {
		return fmt.Errorf("read deployment workdir: %w", err)
	}
	for _, entry := range current {
		if entry.IsDir() || !hasDeployableExtension(entry.Name()) {
			continue
		}
		if err := os.Remove(filepath.Join(workDir, entry.Name())); err != nil {
			return fmt.Errorf("remove rejected %s: %w", entry.Name(), err)
		}
	}

	for _, entry := range entries {
		if entry.IsDir() || !hasDeployableExtension(entry.Name()) {
			continue
		}
		payload, err := os.ReadFile(filepath.Join(previous, entry.Name()))
		if err != nil {
			return fmt.Errorf("read stashed %s: %w", entry.Name(), err)
		}
		if err := os.WriteFile(filepath.Join(workDir, entry.Name()), payload, 0o644); err != nil {
			return fmt.Errorf("restore %s: %w", entry.Name(), err)
		}
	}
	return nil
}
