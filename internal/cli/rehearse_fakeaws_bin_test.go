package cli

import (
	"context"
	"debug/buildinfo"
	"errors"
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

// fakeInstall is a go install that writes a shell script as GOBIN's
// fakeaws, then answers err.
func fakeInstall(t *testing.T, calls *[]harness.Command, err error) harness.CommandRunner {
	return harness.CommandRunnerFunc(func(_ context.Context, c harness.Command) (harness.CommandResult, error) {
		*calls = append(*calls, c)
		require.NoError(t, os.MkdirAll(c.Env["GOBIN"], 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(c.Env["GOBIN"], "fakeaws"), []byte("#!/bin/sh\n"), 0o700))
		return harness.CommandResult{}, err
	})
}

// cacheEntries lists dir's entries by name.
func cacheEntries(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := []string{}
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// With an empty cache, the binary is installed at the pin by one go
// install under replace mode with exactly the install's env: the module
// proxy with no direct, so no git runs, the local toolchain, and every Go
// dir in a fresh dir that is removed after. Only the binary stays, and it
// is checked like any other.
func TestRehearsalFakeawsBinaryInstallsAtPin(t *testing.T) {
	dir := filepath.Join(rehearsalEmptyUserCache(t), "infrafactory", "fakeaws", rehearsalFakeawsPin)
	var calls []harness.Command
	install := fakeInstall(t, &calls, nil)
	runner := harness.CommandRunnerFunc(func(ctx context.Context, c harness.Command) (harness.CommandResult, error) {
		entries, err := os.ReadDir(c.Env["HOME"])
		assert.NoError(t, err)
		assert.Empty(t, entries, "HOME is empty")
		if info, err := os.Stat(c.Env["HOME"]); assert.NoError(t, err) {
			assert.Equal(t, fs.FileMode(0o700), info.Mode().Perm())
		}
		return install.Run(ctx, c)
	})

	_, err := rehearsalFakeawsBinary(context.Background(), runner, "")
	assert.ErrorContains(t, err, "it has no Go build info", "the installed binary is checked")
	require.Len(t, calls, 1)
	c := calls[0]
	assert.Equal(t, "go", c.Name)
	assert.Equal(t, []string{"install", "github.com/redscaresu/fakeaws/cmd/fakeaws@" + rehearsalFakeawsPin}, c.Args)
	assert.True(t, c.ReplaceEnv)
	tmp := filepath.Dir(c.Env["HOME"])
	assert.Equal(t, dir, filepath.Dir(tmp), "the install runs in a dir under the cache dir")
	assert.Equal(t, map[string]string{
		"HOME":        filepath.Join(tmp, "home"),
		"GOBIN":       filepath.Join(tmp, "bin"),
		"GOPATH":      filepath.Join(tmp, "gopath"),
		"GOCACHE":     filepath.Join(tmp, "gocache"),
		"GOFLAGS":     "-modcacherw",
		"GOTOOLCHAIN": "local",
		"GOPROXY":     "https://proxy.golang.org",
		"GOSUMDB":     "sum.golang.org",
		"CGO_ENABLED": "0",
	}, c.Env)
	assert.Equal(t, []string{"fakeaws"}, cacheEntries(t, dir), "only the binary stays")
}

// An install that fails after writing a binary leaves nothing in the
// cache, so a later rehearsal installs again instead of meeting a partial
// binary.
func TestRehearsalFakeawsBinaryFailedInstallLeavesNoBinary(t *testing.T) {
	dir := filepath.Join(rehearsalEmptyUserCache(t), "infrafactory", "fakeaws", rehearsalFakeawsPin)
	var calls []harness.Command

	_, err := rehearsalFakeawsBinary(context.Background(), fakeInstall(t, &calls, errors.New("signal: interrupt")), "")
	assert.ErrorContains(t, err, "signal: interrupt")
	assert.Len(t, calls, 1)
	assert.Empty(t, cacheEntries(t, dir))
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
// cmd/fakeaws, commits it, tags it with tag (if set), applies dirty (if
// set) to the tree, and builds it with -buildvcs=true. It returns the
// binary and the commit's SHA.
func buildTestModule(t *testing.T, module, tag string, dirty func(dir string)) (string, string) {
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
	// go records the version it stamps for a revision in GOMODCACHE, by
	// module path: the user's would collect fake fakeaws versions, and a
	// SHA two builds share (same tree, same second) would answer the
	// first build's version to the second.
	modcache := t.TempDir()
	run := func(name string, args ...string) string {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		// Inherited env for go's build cache; no git config or Go flags
		// of the user's, and no network.
		result, err := execCommandRunner{}.Run(ctx, harness.Command{Name: name, Args: args, Dir: dir, Env: map[string]string{
			"GIT_CONFIG_GLOBAL": os.DevNull, "GIT_CONFIG_NOSYSTEM": "1", "GOMODCACHE": modcache,
			"GOFLAGS": "-modcacherw", "GOTOOLCHAIN": "local", "GOWORK": "off", "GOPROXY": "off",
		}})
		require.NoError(t, err, "%s %v: %s", name, args, result.Stderr)
		return strings.TrimSpace(string(result.Stdout))
	}
	run("git", "init", "-q")
	run("git", "add", ".")
	run("git", "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "fakeaws")
	sha := run("git", "rev-parse", "HEAD")
	if tag != "" {
		run("git", "tag", tag)
	}
	if dirty != nil {
		dirty(dir)
	}
	bin := filepath.Join(t.TempDir(), "fakeaws")
	run("go", "build", "-buildvcs=true", "-o", bin, "./cmd/fakeaws")
	return bin, sha
}

// A build at the pin, tagged or not, is admitted; one from another commit, a dirty tree,
// another module, or a file with no build info is refused, naming what was
// found.
func TestRehearsalVerifyFakeawsBuildinfo(t *testing.T) {
	clean, sha := buildTestModule(t, "github.com/redscaresu/fakeaws", "", nil)
	assert.NoError(t, rehearsalVerifyFakeawsBuildinfo(clean, sha), "a clean build at the pin")

	tagged, taggedSHA := buildTestModule(t, "github.com/redscaresu/fakeaws", "v1.2.3", nil)
	info, err := buildinfo.ReadFile(tagged)
	require.NoError(t, err)
	require.Equal(t, "v1.2.3", info.Main.Version, "only vcs.revision can admit a build whose version is a tag")
	assert.NoError(t, rehearsalVerifyFakeawsBuildinfo(tagged, taggedSHA), "a clean build at a tagged pin")

	err = rehearsalVerifyFakeawsBuildinfo(clean, testFakeawsSHA)
	assert.ErrorContains(t, err, `vcs.revision "`+sha+`"`, "another pin names the found revision")

	dirty, dirtySHA := buildTestModule(t, "github.com/redscaresu/fakeaws", "", func(dir string) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "cmd", "fakeaws", "main.go"), []byte("package main\n\nfunc main() { println() }\n"), 0o600))
	})
	err = rehearsalVerifyFakeawsBuildinfo(dirty, dirtySHA)
	assert.ErrorContains(t, err, `vcs.modified="true"`, "a dirty tree at the pin")

	other, otherSHA := buildTestModule(t, "example.com/notfakeaws", "", nil)
	err = rehearsalVerifyFakeawsBuildinfo(other, otherSHA)
	assert.ErrorContains(t, err, "example.com/notfakeaws", "another module is named")

	script := filepath.Join(t.TempDir(), "fakeaws")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\n"), 0o700))
	err = rehearsalVerifyFakeawsBuildinfo(script, sha)
	assert.ErrorContains(t, err, "it has no Go build info", "a shell script")
}

// A symlink is resolved once and the target is checked and returned, so
// the caller starts the file that was checked.
func TestRehearsalFakeawsResolveFollowsSymlinks(t *testing.T) {
	clean, sha := buildTestModule(t, "github.com/redscaresu/fakeaws", "", nil)
	links := t.TempDir()
	link := filepath.Join(links, "fakeaws")
	require.NoError(t, os.Symlink(clean, link))
	want, err := filepath.EvalSymlinks(clean)
	require.NoError(t, err)

	got, err := rehearsalFakeawsResolve(link, sha)
	require.NoError(t, err)
	assert.Equal(t, want, got, "the target is returned, not the link")

	script := filepath.Join(t.TempDir(), "script")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\n"), 0o700))
	bad := filepath.Join(links, "bad")
	require.NoError(t, os.Symlink(script, bad))
	_, err = rehearsalFakeawsResolve(bad, sha)
	assert.ErrorContains(t, err, "it has no Go build info", "a link to a file that is not fakeaws")
}

// TestRehearsalFakeawsInstallAtPin runs the real install, through the
// module proxy, into an empty cache. The installed binary is fakeaws at
// the pin, and it is all the install leaves: the module cache went with
// the install's dir, or TempDir's cleanup could not remove it.
func TestRehearsalFakeawsInstallAtPin(t *testing.T) {
	if os.Getenv("INFRAFACTORY_ENABLE_E2E") != "1" {
		t.Skip("set INFRAFACTORY_ENABLE_E2E=1 to enable e2e tests")
	}
	_, err := exec.LookPath("go")
	require.NoError(t, err, "go required")
	dir := filepath.Join(rehearsalEmptyUserCache(t), "infrafactory", "fakeaws", rehearsalFakeawsPin)

	bin, err := rehearsalFakeawsBinary(context.Background(), execCommandRunner{}, "")
	require.NoError(t, err)
	assert.NoError(t, rehearsalVerifyFakeawsBuildinfo(bin, rehearsalFakeawsPin))
	assert.Equal(t, []string{"fakeaws"}, cacheEntries(t, dir), "only the binary stays")
}
