package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/redscaresu/infrafactory/internal/harness"
)

// rehearsalTofuTimeout bounds each tofu command prepare runs. At it, the
// runner interrupts tofu's process group, then kills it.
const rehearsalTofuTimeout = 10 * time.Minute

// rehearsalDeadProxy is the proxy the second init runs under. Nothing
// listens there, which prepare checks before the second init. A var so a
// test can move it.
var rehearsalDeadProxy = "http://127.0.0.1:9"

// rehearsalDialTimeout bounds prepare's dial of the dead proxy.
const rehearsalDialTimeout = 5 * time.Second

// ourOverride is the name infrafactory's override takes: it sorts before a
// user's zz_override.tf.
const ourOverride = "infrafactory_override.tf"

// localBackendOverride is the rehearsal's backend override: `backend
// "local"`, with blockPath as its path when it is not empty.
func localBackendOverride(blockPath string) string {
	block := "{}"
	if blockPath != "" {
		block = fmt.Sprintf("{\n    path = %q\n  }", blockPath)
	}
	return "terraform {\n  backend \"local\" " + block + "\n}\n"
}

// gateAndInit reads the copy's lock, runs gate 1 over the copy, the first
// init, gate 2 over what it wrote, then the second init (ADR-0042 § The
// lock, § The two inits). It returns the lock. The copy, the lock and the
// gates do not read ctx, so a cancel during them is refused before the
// next tofu, and one during the last steps after them: a cancel never
// ends in a success report.
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
	if err := rehearsalModulesGate(in, filepath.Join(rehearsal, "data")); err != nil {
		return rehearsalLock{}, fmt.Errorf("gate 2, after the first init: %w", err)
	}
	if err := rehearsalCancelled(ctx); err != nil {
		return rehearsalLock{}, err
	}
	if err := p.secondInit(ctx, rehearsal, in.root); err != nil {
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
// read no system config, never prompt and fetch through no transport,
// file included. Gate 1 has refused every module download, so git has
// nothing to fetch; the git variables are a second wall in case the gate
// is wrong. Then the copy's lock may pin no
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

// secondInit is the offline second init in the copy's root. The dead
// proxy must refuse a dial; then the backend override goes into the root,
// and init reads only the providers the first init installed, with the
// state under <rehearsal>/state, outside the copy. It runs under replace
// mode with an empty HOME, TF_DATA_DIR at <rehearsal>/data, both proxies at
// the dead proxy and no NO_PROXY, so it makes no proxied connection
// (TestAssumption_SecondInitMakesNoProxiedConnection). Then the
// rehearsal's modes are tightened again.
func (p rehearsePrepare) secondInit(ctx context.Context, rehearsal, root string) error {
	if err := rehearsalProxyRefuses(ctx, rehearsalDeadProxy); err != nil {
		return err
	}
	state, data := filepath.Join(rehearsal, "state"), filepath.Join(rehearsal, "data")
	if err := os.Mkdir(state, 0o700); err != nil {
		return err
	}
	if err := writeSynced(filepath.Join(root, ourOverride), []byte(localBackendOverride(""))); err != nil {
		return err
	}
	env := map[string]string{
		"HOME":        filepath.Join(rehearsal, "home"),
		"TF_DATA_DIR": data,
		"HTTPS_PROXY": rehearsalDeadProxy,
		"HTTP_PROXY":  rehearsalDeadProxy,
	}
	// -plugin-dir is relative to the root, tofu's working dir, so it holds
	// only .. and data/providers: tofu 1.13 splits the flag at a comma,
	// which the rehearsal's absolute path may hold (TestRehearsePrepare's
	// repo path does).
	plugins, err := filepath.Rel(root, filepath.Join(data, "providers"))
	if err != nil {
		return err
	}
	args := []string{"init", "-input=false", "-no-color", "-reconfigure", "-get=false", "-lockfile=readonly",
		"-backend-config=path=" + filepath.Join(state, "terraform.tfstate"), "-plugin-dir=" + plugins}
	if err := p.tofu(ctx, harness.Command{Name: "tofu", Args: args, Dir: root, Env: env, ReplaceEnv: true}); err != nil {
		return err
	}
	return rehearsalTightenModes(rehearsal)
}

// rehearsalProxyRefuses dials proxy and admits only a refused connection:
// a dial that connects, or fails any other way, refuses the second init. A
// dial that fails because ctx is cancelled is refused as the cancel.
func rehearsalProxyRefuses(ctx context.Context, proxy string) error {
	refused := func(why string) error {
		return fmt.Errorf("the second init is refused: the dead proxy %s %s, so it cannot be relied on to refuse the init's requests", proxy, why)
	}
	u, err := url.Parse(proxy)
	if err != nil {
		return refused(fmt.Sprintf("does not parse (%v)", err))
	}
	dialCtx, cancel := context.WithTimeout(ctx, rehearsalDialTimeout)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", u.Host)
	switch {
	case err == nil:
		conn.Close()
		return refused("accepted a connection")
	case ctx.Err() != nil:
		return rehearsalCancelled(ctx)
	case errors.Is(err, syscall.ECONNREFUSED):
		return nil
	}
	return refused(fmt.Sprintf("did not refuse a dial (%v)", err))
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
