package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/redscaresu/infrafactory/internal/scenario"
	"github.com/spf13/cobra"
)

// holdsRoot holds each hold's copy of its pre-destroy tfstate (ADR-0040's
// hold amendment). It is fixed, like runstore.DefaultRoot and
// livestore.DefaultRoot, and read from no environment variable: the UI
// serves those two stores, and the copy must never move under either.
const holdsRoot = ".infrafactory/holds"

// awsHoldScenario is the one scenario a hold keeps.
const awsHoldScenario = "aws-web-live"

// holdCopyPath is where scenario's hold copy lives.
func holdCopyPath(scenarioName string) string {
	return filepath.Join(holdsRoot, scenarioName, harness.LiveStateFilename)
}

// refuseWhileHeld refuses op (generate, run or test) of sc while anything
// is at sc's hold copy path, a broken symlink included, or that path
// cannot be read: generation removes the output dir, which holds a held
// stack's only live state, and an unreadable holds dir is not an empty
// one. The copy outlives a release, kept for the read, so the advice
// serves both a held stack and a released one. Only an absent copy
// passes. A local check, before any AWS request.
func refuseWhileHeld(op string, runtime *CommandRuntime, sc scenario.Scenario) error {
	copyPath := holdCopyPath(sc.Name)
	_, err := os.Lstat(copyPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	found := "a hold's copy is at " + copyPath
	if err != nil {
		found = fmt.Sprintf("a hold's copy may be at %s, which cannot be read: %v", copyPath, err)
	}
	return &CLIError{Op: op, Code: errorCodeUsage, Err: fmt.Errorf(
		"refusing %s of %s, with nothing asked of AWS: %s. If the stack is still held, release it with `%s --take-over <holder> --release-hold`, "+
			"which keeps the copy for the read; once the read is done, delete the copy by hand, which lifts this refusal",
		op, sc.Name, found, reapCommand(runtime.ConfigPath, runtime.scenarioPath))}
}

// holdRefusal refuses test's --hold, as a usage error before the AMI
// resolve or any AWS request, when it cannot run as asked: it holds
// aws-web-live on Layer 3 only, and a failed hold must destroy.
func holdRefusal(runtime *CommandRuntime, sc scenario.Scenario, noDestroy bool) error {
	layers := runtime.Config.Validation.Layers
	var refusal string
	switch {
	case layer3TeardownCloud(sc.Cloud) != layer3AWS || sc.Name != awsHoldScenario:
		refusal = fmt.Sprintf("--hold holds the aws scenario %s only, not %s on %s", awsHoldScenario, sc.Name, sc.Cloud)
	case !layers.MockDeploy.Enabled || !layers.SandboxDeploy.Enabled:
		// test runs Layer 3 only after Layer 2: without it, nothing is held.
		refusal = "--hold keeps a Layer 3 stack, which test applies only with validation.layers.mock_deploy and validation.layers.sandbox_deploy on"
	case !layers.Destruction.Enabled:
		refusal = "--hold destroys a stack it does not hold, and destruction (validation.layers.destruction) is disabled"
	case noDestroy:
		refusal = "hold and no-destroy are mutually exclusive: --hold destroys a stack it does not hold, --no-destroy destroys nothing"
	default:
		return nil
	}
	return &CLIError{Op: "test", Code: errorCodeUsage, Err: errors.New(refusal)}
}

// holdCopy writes a held test's copy. A var only so a test can interrupt
// the run from inside the write.
var holdCopy = writeNewHoldCopy

// writeNewHoldCopy writes the copy from the state with release's helpers:
// private dirs, a 0600 file read back. Anything already at the copy path
// refuses and is left as it is: the guard refuses a test while one is
// there, so one that appeared since is not this run's.
func writeNewHoldCopy(copyPath, statePath string) error {
	if err := privateHoldDirs(filepath.Dir(copyPath)); err != nil {
		return err
	}
	if _, err := os.Lstat(copyPath); err == nil {
		return fmt.Errorf("something is already at %s, not this run's; it is left as it is", copyPath)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return writeHoldCopy(copyPath, statePath)
}

// prepareHoldRelease is reap's --release-hold, all of it local work before
// any AWS request: the usage refusals, then the copy settled with the
// output dir's state. Every refusal of an aws scenario names the take-over
// that destroys without the copy; another cloud's reap takes no take-over.
func prepareHoldRelease(cmd *cobra.Command, runtime *CommandRuntime, sc scenario.Scenario, dryRun bool) error {
	releaseHold, err := cmd.Flags().GetBool("release-hold")
	if err != nil {
		return &CLIError{Op: "reap", Code: errorCodeUsage, Err: fmt.Errorf("read --release-hold flag: %w", err)}
	}
	if !releaseHold {
		return nil
	}
	takeOver, err := cmd.Flags().GetString("take-over")
	if err != nil {
		return &CLIError{Op: "reap", Code: errorCodeUsage, Err: fmt.Errorf("read --take-over flag: %w", err)}
	}
	withoutCopy := destroyWithoutCopy(runtime, takeOver)
	if refusal := releaseHoldRefusal(sc, takeOver, dryRun); refusal != "" {
		if layer3TeardownCloud(sc.Cloud) == layer3AWS {
			refusal += ". " + withoutCopy
		}
		return &CLIError{Op: "reap", Code: errorCodeUsage, Err: errors.New(refusal)}
	}
	copyPath := holdCopyPath(sc.Name)
	settled, err := settleHoldCopy(copyPath, filepath.Join(runtime.OutputDir(), harness.LiveStateFilename))
	if err != nil {
		return &CLIError{Op: "reap", Code: errorCodeCommandFailed, Err: fmt.Errorf(
			"refusing --release-hold, with nothing asked of AWS: %w. %s", err, withoutCopy)}
	}
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Hold copy %s: %s. It is kept for the read; delete it by hand once the read is done.\n",
		copyPath, settled)
	return nil
}

// releaseHoldRefusal is why --release-hold cannot run as asked, or "".
func releaseHoldRefusal(sc scenario.Scenario, takeOver string, dryRun bool) string {
	switch {
	case layer3TeardownCloud(sc.Cloud) != layer3AWS || sc.Name != awsHoldScenario:
		return fmt.Sprintf("--release-hold releases a hold of the aws scenario %s only, not %s on %s", awsHoldScenario, sc.Name, sc.Cloud)
	case takeOver == "":
		return "--release-hold needs --take-over, naming the holder of the held test's claim"
	case dryRun:
		return "--release-hold copies the state and destroys, so it cannot run with --dry-run"
	}
	return ""
}

// destroyWithoutCopy names the plain take-over: the reap that destroys a
// hold without its copy.
func destroyWithoutCopy(runtime *CommandRuntime, takeOver string) string {
	reap := reapCommand(runtime.ConfigPath, runtime.scenarioPath) + " --take-over <holder>"
	if takeOver != "" {
		reap = awsTakeOverCommand(runtime, takeOver)
	}
	return "`" + reap + "` destroys without the copy"
}

// settleHoldCopy makes the copy and the output dir's state agree, the
// copy being the authority: with no copy yet, it is written from the
// state and verified; with the state byte-identical, the copy is kept;
// with the state missing, the state is restored from the copy. Anything
// else refuses and changes neither file: holds dirs that are not private
// directories, a copy that is not a private regular file with one link,
// a state that cannot be read, or a state that differs from the copy.
// It returns what it did.
func settleHoldCopy(copyPath, statePath string) (string, error) {
	if err := privateHoldDirs(filepath.Dir(copyPath)); err != nil {
		return "", err
	}
	if _, err := os.Lstat(copyPath); errors.Is(err, fs.ErrNotExist) {
		return "written from " + statePath, writeHoldCopy(copyPath, statePath)
	} else if err != nil {
		return "", err
	}
	held, err := readHoldCopy(copyPath)
	if err != nil {
		return "", err
	}
	state, err := os.ReadFile(statePath)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "restored to " + statePath, restoreHeldState(statePath, held)
	case err != nil:
		return "", fmt.Errorf("the copy %s exists, and the state beside it cannot be read: %w", copyPath, err)
	case !bytes.Equal(state, held):
		return "", fmt.Errorf("%s differs from the copy %s; both are left as they are", statePath, copyPath)
	}
	return "kept, identical to " + statePath, nil
}

// privateHoldDirs makes .infrafactory, holdsRoot and dir, each 0700 where
// it is missing, and refuses any of them that is a symlink or not a
// directory, and a holds dir open to group or other. It never chmods: a
// dir made by someone else is refused as found. .infrafactory may be 0755,
// as the run and live stores make it.
func privateHoldDirs(dir string) error {
	infrafactory := filepath.Dir(holdsRoot)
	for _, d := range []string{infrafactory, holdsRoot, dir} {
		// Mkdir, not MkdirAll: it never creates through a symlink.
		if err := os.Mkdir(d, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
			return err
		}
		info, err := os.Lstat(d)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("%s is %v, not a directory", d, info.Mode())
		}
		if d != infrafactory && info.Mode().Perm()&0o077 != 0 {
			return fmt.Errorf("%s is %v, not a directory of mode 0700, and is never chmodded", d, info.Mode())
		}
	}
	return nil
}

// writeHoldCopy writes the state to copyPath, 0600 through a synced temp
// file in its dir, then reads the copy back. An absent or unreadable state
// refuses: it is not an empty one.
func writeHoldCopy(copyPath, statePath string) error {
	state, err := os.ReadFile(statePath)
	if err != nil {
		return fmt.Errorf("no copy can be taken: %w", err)
	}
	if err := writeSynced(copyPath, state); err != nil {
		return err
	}
	written, err := readHoldCopy(copyPath)
	if err != nil {
		return err
	}
	if !bytes.Equal(written, state) {
		return fmt.Errorf("the copy %s does not read back as %s", copyPath, statePath)
	}
	return nil
}

// readHoldCopy reads the copy only when it is a regular file of mode 0600
// with one link: through a symlink or a second link, it could change by a
// path the hold does not guard.
func readHoldCopy(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	var links uint64
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		links = uint64(st.Nlink)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || links != 1 {
		return nil, fmt.Errorf("the copy %s is %v with %d links, not a regular file of mode 0600 with one; it is left as it is",
			path, info.Mode(), links)
	}
	return os.ReadFile(path)
}

// restoreHeldState writes the copy's bytes back as the output dir's state
// and reads them back.
func restoreHeldState(statePath string, held []byte) error {
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		return err
	}
	if err := writeSynced(statePath, held); err != nil {
		return err
	}
	restored, err := os.ReadFile(statePath)
	if err != nil {
		return err
	}
	if !bytes.Equal(restored, held) {
		return fmt.Errorf("%s does not read back as the copy", statePath)
	}
	return nil
}

// writeSynced writes data to path, 0600, through a synced temp file in
// path's dir renamed into place, so no reader sees a half-written file.
func writeSynced(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	_, err = tmp.Write(data)
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
