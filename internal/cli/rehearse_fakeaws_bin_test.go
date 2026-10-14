package cli

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/harness"
)

// TestRehearsalFakeawsPinMatchesCI holds rehearsalFakeawsPin equal to
// ci.yml's FAKEAWS_SHA, so a rehearsal runs the fakeaws CI tested. A bump
// of either must move both.
func TestRehearsalFakeawsPinMatchesCI(t *testing.T) {
	ci, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "ci.yml"))
	require.NoError(t, err)
	shas := regexp.MustCompile(`FAKEAWS_SHA:\s*(\S+)`).FindAllSubmatch(ci, -1)
	require.NotEmpty(t, shas, "ci.yml sets no FAKEAWS_SHA")
	for _, m := range shas {
		assert.Equal(t, string(m[1]), rehearsalFakeawsPin, "ci.yml's FAKEAWS_SHA and rehearsalFakeawsPin differ: move both")
	}
}

// rehearsalEmptyUserCache points os.UserCacheDir at an empty temp dir, on
// macOS (HOME) and Linux (XDG_CACHE_HOME) alike, and returns it.
func rehearsalEmptyUserCache(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tmp, "cache"))
	cache, err := os.UserCacheDir()
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(cache, tmp), "the user cache dir %s is not under %s", cache, tmp)
	return cache
}

// With an empty cache, the binary is installed at the pin by one go
// install under replace mode with exactly the install's env: the module
// proxy with no direct, so no git runs, and the local toolchain.
func TestRehearsalFakeawsBinaryInstallsAtPin(t *testing.T) {
	dir := filepath.Join(rehearsalEmptyUserCache(t), "infrafactory", "fakeaws", rehearsalFakeawsPin)
	var calls []harness.Command
	runner := harness.CommandRunnerFunc(func(_ context.Context, c harness.Command) (harness.CommandResult, error) {
		calls = append(calls, c)
		entries, err := os.ReadDir(c.Env["HOME"])
		assert.NoError(t, err)
		assert.Empty(t, entries, "HOME is empty")
		if info, err := os.Stat(c.Env["HOME"]); assert.NoError(t, err) {
			assert.Equal(t, fs.FileMode(0o700), info.Mode().Perm())
		}
		return harness.CommandResult{}, nil
	})

	_, err := rehearsalFakeawsBinary(context.Background(), runner, "")
	assert.ErrorIs(t, err, fs.ErrNotExist, "the fake install writes no binary")
	require.Len(t, calls, 1)
	c := calls[0]
	assert.Equal(t, "go", c.Name)
	assert.Equal(t, []string{"install", "github.com/redscaresu/fakeaws/cmd/fakeaws@" + rehearsalFakeawsPin}, c.Args)
	assert.True(t, c.ReplaceEnv)
	home := c.Env["HOME"]
	assert.Equal(t, dir, filepath.Dir(home), "HOME is under the cache dir")
	assert.Equal(t, map[string]string{
		"HOME":        home,
		"GOBIN":       dir,
		"GOPATH":      filepath.Join(dir, "gopath"),
		"GOCACHE":     filepath.Join(dir, "gocache"),
		"GOTOOLCHAIN": "local",
		"GOPROXY":     "https://proxy.golang.org",
		"GOSUMDB":     "sum.golang.org",
		"CGO_ENABLED": "0",
	}, c.Env)
	assert.NoDirExists(t, home, "HOME is removed after the install")
}

// A binary already in the cache is not installed again, and is checked
// like any other.
func TestRehearsalFakeawsBinaryChecksTheCachedBinary(t *testing.T) {
	dir := filepath.Join(rehearsalEmptyUserCache(t), "infrafactory", "fakeaws", rehearsalFakeawsPin)
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "fakeaws"), []byte("#!/bin/sh\n"), 0o700))
	runner := &recordingRunner{}

	_, err := rehearsalFakeawsBinary(context.Background(), runner, "")
	assert.ErrorContains(t, err, "it has no Go build info")
	assert.Empty(t, runner.commands)
}

// --fakeaws-bin must be an absolute path to a regular, executable file;
// anything else is refused before any command runs.
func TestRehearsalFakeawsBinaryRefusesFlagPath(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "plain")
	require.NoError(t, os.WriteFile(plain, []byte("x"), 0o600))
	for _, tc := range []struct{ name, path, want string }{
		{"a relative path", "bin/fakeaws", "not an absolute path"},
		{"a directory", dir, "not a regular file"},
		{"a non-executable file", plain, "not executable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := &recordingRunner{}
			_, err := rehearsalFakeawsBinary(context.Background(), runner, tc.path)
			assert.ErrorContains(t, err, tc.want)
			assert.Empty(t, runner.commands)
		})
	}
}

// testFakeawsSHA is a commit no test module is built at.
const testFakeawsSHA = "0123456789abcdef0123456789abcdef01234567"

// buildTestModule git-inits a module at module with package main at
// cmd/fakeaws, commits it, applies dirty (if set) to the tree, and builds
// it with -buildvcs=true. It returns the binary and the commit's SHA.
func buildTestModule(t *testing.T, module string, dirty func(dir string)) (string, string) {
	t.Helper()
	for _, tool := range []string{"git", "go"} {
		_, err := exec.LookPath(tool)
		require.NoError(t, err, "%s required", tool)
	}
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "cmd", "fakeaws"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module "+module+"\n\ngo 1.26\n"), 0o600))
	main := filepath.Join(dir, "cmd", "fakeaws", "main.go")
	require.NoError(t, os.WriteFile(main, []byte("package main\n\nfunc main() {}\n"), 0o600))
	run := func(name string, args ...string) string {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		// Inherited env for go's caches; no git config or Go flags of
		// the user's, and no network.
		result, err := execCommandRunner{}.Run(ctx, harness.Command{Name: name, Args: args, Dir: dir, Env: map[string]string{
			"GIT_CONFIG_GLOBAL": os.DevNull, "GIT_CONFIG_NOSYSTEM": "1",
			"GOFLAGS": "", "GOTOOLCHAIN": "local", "GOWORK": "off", "GOPROXY": "off",
		}})
		require.NoError(t, err, "%s %v: %s", name, args, result.Stderr)
		return strings.TrimSpace(string(result.Stdout))
	}
	run("git", "init", "-q")
	run("git", "add", ".")
	run("git", "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "fakeaws")
	sha := run("git", "rev-parse", "HEAD")
	if dirty != nil {
		dirty(dir)
	}
	bin := filepath.Join(t.TempDir(), "fakeaws")
	run("go", "build", "-buildvcs=true", "-o", bin, "./cmd/fakeaws")
	return bin, sha
}

// A build at the pin is admitted; one from another commit, a dirty tree,
// another module, or a file with no build info is refused, naming what was
// found.
func TestRehearsalVerifyFakeawsBuildinfo(t *testing.T) {
	clean, sha := buildTestModule(t, "github.com/redscaresu/fakeaws", nil)
	assert.NoError(t, rehearsalVerifyFakeawsBuildinfo(clean, sha), "a clean build at the pin")

	err := rehearsalVerifyFakeawsBuildinfo(clean, testFakeawsSHA)
	assert.ErrorContains(t, err, `vcs.revision "`+sha+`"`, "another pin names the found revision")

	dirty, dirtySHA := buildTestModule(t, "github.com/redscaresu/fakeaws", func(dir string) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "cmd", "fakeaws", "main.go"), []byte("package main\n\nfunc main() { println() }\n"), 0o600))
	})
	err = rehearsalVerifyFakeawsBuildinfo(dirty, dirtySHA)
	assert.ErrorContains(t, err, `vcs.modified="true"`, "a dirty tree at the pin")

	other, otherSHA := buildTestModule(t, "example.com/notfakeaws", nil)
	err = rehearsalVerifyFakeawsBuildinfo(other, otherSHA)
	assert.ErrorContains(t, err, "example.com/notfakeaws", "another module is named")

	script := filepath.Join(t.TempDir(), "fakeaws")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\n"), 0o700))
	err = rehearsalVerifyFakeawsBuildinfo(script, sha)
	assert.ErrorContains(t, err, "it has no Go build info", "a shell script")
}

// TestRehearsalFakeawsInstallAtPin runs the real install, through the
// module proxy, into an empty cache, and the installed binary is fakeaws
// at the pin.
func TestRehearsalFakeawsInstallAtPin(t *testing.T) {
	if os.Getenv("INFRAFACTORY_ENABLE_E2E") != "1" {
		t.Skip("set INFRAFACTORY_ENABLE_E2E=1 to enable e2e tests")
	}
	_, err := exec.LookPath("go")
	require.NoError(t, err, "go required")
	cache := rehearsalEmptyUserCache(t)
	// The module cache is read-only; make it removable for TempDir.
	t.Cleanup(func() {
		_ = filepath.WalkDir(cache, func(path string, d fs.DirEntry, err error) error {
			if err == nil && d.IsDir() {
				_ = os.Chmod(path, 0o700)
			}
			return nil
		})
	})

	bin, err := rehearsalFakeawsBinary(context.Background(), execCommandRunner{}, "")
	require.NoError(t, err)
	assert.NoError(t, rehearsalVerifyFakeawsBuildinfo(bin, rehearsalFakeawsPin))
}
