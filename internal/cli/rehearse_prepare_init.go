package cli

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/redscaresu/infrafactory/internal/harness"
)

// rehearsalTofuTimeout bounds each tofu command prepare runs. At it, the
// runner interrupts tofu's process group, then kills it.
const rehearsalTofuTimeout = 10 * time.Minute

// gateAndInit reads the copy's lock, runs gate 1 over the copy, then the
// first init (ADR-0042 § The lock, § The two inits). It returns the lock.
// The copy, the lock and gate 1 do not read ctx, so a cancel during them
// is refused before the first tofu, and one during the last steps after
// them: a cancel never ends in a success report.
func (p rehearsePrepare) gateAndInit(ctx context.Context, rehearsal string, in rehearsalGateInput) (rehearsalLock, error) {
	lock, problems := in.lock()
	if err := rehearsalRefusal(problems); err != nil {
		return rehearsalLock{}, err
	}
	in.awsVersion = lock.aws()
	if err := rehearsalGate(in); err != nil {
		return rehearsalLock{}, err
	}
	if err := rehearsalCancelled(ctx); err != nil {
		return rehearsalLock{}, err
	}
	if err := p.firstInit(ctx, rehearsal, in, lock); err != nil {
		return rehearsalLock{}, err
	}
	return lock, rehearsalCancelled(ctx)
}

// rehearsalCancelled refuses a cancelled prepare.
func rehearsalCancelled(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("prepare is cancelled: %w", err)
	}
	return nil
}

// firstInit is the network-only first init in the copy's root: get, which
// installs the module calls providers lock needs installed
// (TestAssumption_ProvidersLockKeepsLockedVersion), providers lock for
// this platform, then init with no backend. Each runs under replace mode
// with an empty HOME, TF_DATA_DIR at <rehearsal>/data, and git told to
// read no system config, never prompt and fetch only from files. Gate 1
// has refused every module download, so git has nothing to fetch; the git
// variables are a second wall in case the gate is wrong, and allow no git
// transport at all, file included. Then the copy's lock may pin no
// provider at a version lock does not, and the rehearsal's modes are
// tightened again.
func (p rehearsePrepare) firstInit(ctx context.Context, rehearsal string, in rehearsalGateInput, lock rehearsalLock) error {
	home, data := filepath.Join(rehearsal, "home"), filepath.Join(rehearsal, "data")
	for _, dir := range []string{home, data} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			return err
		}
	}
	env := map[string]string{
		"HOME":                home,
		"TF_DATA_DIR":         data,
		"GIT_CONFIG_NOSYSTEM": "1", // TestAssumption_GitNoSystemDropsCredentialHelper
		"GIT_TERMINAL_PROMPT": "0",
		"GIT_ALLOW_PROTOCOL":  "none", // names no transport: TestAssumption_GitAllowProtocolNoneRefusesEveryTransport
	}
	for _, args := range [][]string{
		{"get", "-no-color"},
		{"providers", "lock", "-platform=" + runtime.GOOS + "_" + runtime.GOARCH},
		{"init", "-backend=false", "-input=false", "-no-color"},
	} {
		if err := p.tofu(ctx, harness.Command{Name: "tofu", Args: args, Dir: in.root, Env: env, ReplaceEnv: true}); err != nil {
			return err
		}
	}
	// providers lock keeps the locked version (TestAssumption_ProvidersLockKeepsLockedVersion);
	// this holds every provider's whatever tofu does.
	after, problems := in.lock()
	if err := rehearsalRefusal(problems); err != nil {
		return fmt.Errorf("after the first init: %w", err)
	}
	if changes := rehearsalLockChanges(lock, after); len(changes) > 0 {
		return fmt.Errorf("the first init is refused: it changed the locked provider versions: %s", strings.Join(changes, "; "))
	}
	return rehearsalTightenModes(rehearsal)
}

// tofu runs one tofu command under rehearsalTofuTimeout. A failure, a
// cancel included, is an error carrying tofu's stderr.
func (p rehearsePrepare) tofu(ctx context.Context, cmd harness.Command) error {
	ctx, cancel := context.WithTimeout(ctx, rehearsalTofuTimeout)
	defer cancel()
	result, err := p.runner.Run(ctx, cmd)
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		err = fmt.Errorf("%w (%v)", ctx.Err(), err)
	}
	return fmt.Errorf("tofu %s failed: %w\n%s", strings.Join(cmd.Args, " "), err, strings.TrimSpace(string(result.Stderr)))
}

// rehearsalTightenModes sets every dir under dir to 0700, every regular
// file with an execute bit to 0700 (a provider binary must stay runnable)
// and every other regular file to 0600. A symlink is left as it is, since
// chmod would follow it.
func rehearsalTightenModes(dir string) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir():
			return os.Chmod(path, 0o700)
		case !d.Type().IsRegular():
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		mode := fs.FileMode(0o600)
		if info.Mode()&0o111 != 0 {
			mode = 0o700
		}
		return os.Chmod(path, mode)
	})
}
