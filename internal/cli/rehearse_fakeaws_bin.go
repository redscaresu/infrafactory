package cli

import (
	"context"
	"debug/buildinfo"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/redscaresu/infrafactory/internal/harness"
)

// rehearsalFakeawsPin is the fakeaws commit a rehearsal runs. It equals
// ci.yml's FAKEAWS_SHA (TestRehearsalFakeawsPinMatchesCI), so a rehearsal
// runs the fakeaws CI tested (ADR-0042 § The fakeaws binary).
const rehearsalFakeawsPin = "ece63928a1ecd9129aa7731544131ff8313009e3"

const (
	rehearsalFakeawsModule = "github.com/redscaresu/fakeaws"
	rehearsalFakeawsCmd    = rehearsalFakeawsModule + "/cmd/fakeaws"
)

// rehearsalFakeawsInstallTimeout bounds the go install of the pin. At it,
// the runner interrupts go's process group, then kills it.
const rehearsalFakeawsInstallTimeout = 10 * time.Minute

// rehearsalPseudoVersion matches a Go pseudo-version and captures its
// 12-character revision. A +dirty or any other suffix does not match.
var rehearsalPseudoVersion = regexp.MustCompile(`^v\d+\.\d+\.\d+-(?:[0-9A-Za-z.-]*\.)?\d{14}-([0-9a-f]{12})$`)

// rehearsalFakeawsBinary is the fakeaws binary a rehearsal starts:
// flagPath (--fakeaws-bin) when it is set, otherwise
// <UserCacheDir>/infrafactory/fakeaws/<pin>/fakeaws, installed at the pin
// when it is missing. Either way it must be an absolute path to a regular,
// executable file whose build info is fakeaws at rehearsalFakeawsPin.
func rehearsalFakeawsBinary(ctx context.Context, runner harness.CommandRunner, flagPath string) (string, error) {
	bin := flagPath
	if bin == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return "", fmt.Errorf("the fakeaws binary has no cache dir: %w", err)
		}
		dir := filepath.Join(cache, "infrafactory", "fakeaws", rehearsalFakeawsPin)
		bin = filepath.Join(dir, "fakeaws")
		if err := rehearsalInstallFakeaws(ctx, runner, dir, bin); err != nil {
			return "", err
		}
	}
	if err := rehearsalFakeawsFile(bin); err != nil {
		return "", err
	}
	if err := rehearsalVerifyFakeawsBuildinfo(bin, rehearsalFakeawsPin); err != nil {
		return "", err
	}
	return bin, nil
}

// rehearsalInstallFakeaws installs fakeaws at the pin into dir when bin is
// missing. It runs under replace mode with only HOME (an empty 0700 dir,
// removed after), GOBIN, GOPATH and GOCACHE under dir, the local
// toolchain, and the module proxy and checksum database with no `direct`,
// so no git runs and no Go config of the user's applies.
func rehearsalInstallFakeaws(ctx context.Context, runner harness.CommandRunner, dir, bin string) error {
	// nil when bin exists: it is checked like any other binary.
	if _, err := os.Lstat(bin); !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	home, err := os.MkdirTemp(dir, "home-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(home)
	ctx, cancel := context.WithTimeout(ctx, rehearsalFakeawsInstallTimeout)
	defer cancel()
	args := []string{"install", rehearsalFakeawsCmd + "@" + rehearsalFakeawsPin}
	result, err := runner.Run(ctx, harness.Command{Name: "go", Args: args, Dir: home, ReplaceEnv: true, Env: map[string]string{
		"HOME":        home,
		"GOBIN":       dir,
		"GOPATH":      filepath.Join(dir, "gopath"),
		"GOCACHE":     filepath.Join(dir, "gocache"),
		"GOTOOLCHAIN": "local",
		"GOPROXY":     "https://proxy.golang.org",
		"GOSUMDB":     "sum.golang.org",
		"CGO_ENABLED": "0",
	}})
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		err = fmt.Errorf("%w (%v)", ctx.Err(), err)
	}
	return fmt.Errorf("go %s failed: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(result.Stderr)))
}

// rehearsalFakeawsFile admits only an absolute path to a regular,
// executable file.
func rehearsalFakeawsFile(path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("the fakeaws binary %s is refused: it is not an absolute path", path)
	}
	info, err := os.Stat(path)
	switch {
	case err != nil:
		return fmt.Errorf("the fakeaws binary %s is refused: %w", path, err)
	case !info.Mode().IsRegular():
		return fmt.Errorf("the fakeaws binary %s is refused: it is not a regular file", path)
	case info.Mode()&0o111 == 0:
		return fmt.Errorf("the fakeaws binary %s is refused: it is not executable", path)
	}
	return nil
}

// rehearsalVerifyFakeawsBuildinfo admits path only when its Go build info
// says it is fakeaws's cmd/fakeaws at pin: its version is a pseudo-version
// ending in pin's first 12 characters (a `go install …@pin`), or its
// vcs.revision is pin (a build from a clone). A binary carrying vcs
// settings must also carry vcs.modified=false, so a build from a dirty
// tree at the pin is refused.
func rehearsalVerifyFakeawsBuildinfo(path, pin string) error {
	refused := func(format string, args ...any) error {
		return fmt.Errorf("the fakeaws binary %s is refused: %s", path, fmt.Sprintf(format, args...))
	}
	if len(pin) != 40 {
		return refused("the pin %q is not a full commit SHA", pin)
	}
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		return refused("it has no Go build info (%v)", err)
	}
	if info.Path != rehearsalFakeawsCmd || info.Main.Path != rehearsalFakeawsModule {
		return refused("it is %s from module %s, not %s", info.Path, info.Main.Path, rehearsalFakeawsCmd)
	}
	settings, hasVCS := map[string]string{}, false
	for _, s := range info.Settings {
		settings[s.Key] = s.Value
		hasVCS = hasVCS || s.Key == "vcs" || strings.HasPrefix(s.Key, "vcs.")
	}
	if hasVCS && settings["vcs.modified"] != "false" {
		return refused("it was built from a modified tree (vcs.modified=%q)", settings["vcs.modified"])
	}
	if m := rehearsalPseudoVersion.FindStringSubmatch(info.Main.Version); m != nil && m[1] == pin[:12] {
		return nil
	}
	if settings["vcs.revision"] == pin {
		return nil
	}
	return refused("it is version %s with vcs.revision %q, not fakeaws at %s", info.Main.Version, settings["vcs.revision"], pin)
}
