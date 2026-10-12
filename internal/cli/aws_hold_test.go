package cli

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/redscaresu/infrafactory/internal/livestore"
	"github.com/redscaresu/infrafactory/internal/runstore"
)

// holdCopied is logged before the first AWS request when the hold's copy
// is already on disk.
const holdCopied = "hold copy on disk"

// releaseHoldFlags is the release of a hold by lifecycleOtherHolder.
var releaseHoldFlags = []string{"--take-over", lifecycleOtherHolder, "--release-hold"}

// heldBy makes lc's claim the hold's.
func heldBy(lc *awsLifecycle) *awsLifecycle {
	lc.params[harness.AWSClaimParameter] = lifecycleOtherHolder
	return lc
}

// copyWitness logs holdCopied ahead of the first AWS request it sees, when
// the copy exists by then.
type copyWitness struct {
	lc    *awsLifecycle
	first *sync.Once
}

func (w copyWitness) Do(req *http.Request) (*http.Response, error) {
	w.first.Do(func() {
		if _, err := os.Lstat(holdCopyPath(awsHoldScenario)); err == nil {
			w.lc.record(holdCopied)
		}
	})
	return w.lc.Do(req)
}

func witnessTheCopy(lc *awsLifecycle) func(*RuntimeDependencies) {
	return func(deps *RuntimeDependencies) {
		w := copyWitness{lc: lc, first: &sync.Once{}}
		deps.AWSSTS, deps.AWSSSM, deps.AWSEC2 = w, w, w
	}
}

// plantCopy writes data as the hold's copy of scenarioName, dirs 0700 and
// file 0600, in the cwd.
func plantCopy(t *testing.T, scenarioName string, data []byte) string {
	t.Helper()
	path := holdCopyPath(scenarioName)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

// describeCopy is everything a refusal must leave as it was at path.
func describeCopy(t *testing.T, path string) string {
	t.Helper()
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "absent"
	}
	require.NoError(t, err)
	if info.Mode()&fs.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		require.NoError(t, err)
		return "symlink to " + target
	}
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return fmt.Sprintf("%v %d links: %s", info.Mode(), info.Sys().(*syscall.Stat_t).Nlink, data)
}

func assertReleasedAfter(t *testing.T, lc *awsLifecycle, first string) {
	t.Helper()
	calls := lc.log()
	order := []int{
		slices.Index(calls, first),
		slices.Index(calls, destroyRun),
		slices.Index(calls, sweepKeyPairs),
		lastIndex(calls, deleteClaim),
	}
	assert.NotContains(t, order, -1, "every step ran: %v", calls)
	assert.True(t, slices.IsSorted(order), "%s, destroy, sweep, release: %v", first, calls)
	_, held := lc.claim()
	assert.False(t, held, "released")
}

func TestReleaseHoldCopiesTheStateBeforeTheDestroy(t *testing.T) {
	lc := heldBy(newAWSLifecycle(t))
	var outDir string
	lc.destroy.during = func(context.Context) {
		copied, err := os.ReadFile(holdCopyPath(awsHoldScenario))
		require.NoError(t, err, "the copy exists when the destroy runs")
		state, err := os.ReadFile(filepath.Join(outDir, harness.LiveStateFilename))
		require.NoError(t, err)
		assert.Equal(t, state, copied, "the copy is the output dir's state")
		for _, dir := range []string{holdsRoot, filepath.Dir(holdCopyPath(awsHoldScenario))} {
			info, err := os.Lstat(dir)
			require.NoError(t, err)
			assert.Equal(t, fs.ModeDir|0o700, info.Mode(), dir)
		}
		info, err := os.Lstat(holdCopyPath(awsHoldScenario))
		require.NoError(t, err)
		assert.Equal(t, fs.FileMode(0o600), info.Mode())
	}

	r := reapAWSWith(t, lc, awsReapSetup{
		name: awsHoldScenario, withState: true, flags: releaseHoldFlags,
		prepare: func(dir string) { outDir = dir },
		deps:    witnessTheCopy(lc),
	})

	require.NoError(t, r.err, r.output)
	assert.Equal(t, holdCopied, lc.log()[0], "the copy is written before any AWS request")
	assertReleasedAfter(t, lc, holdCopied)
	assert.Contains(t, r.output, "Hold copy "+holdCopyPath(awsHoldScenario)+": written from")
	assert.Contains(t, r.output, "kept for the read")
	assert.FileExists(t, holdCopyPath(awsHoldScenario), "the release leaves the copy")
}

func TestReleaseHoldRefusesBeforeAnyAWSRequest(t *testing.T) {
	// The usage refusals refuse with a good copy in place.
	identical := func(name string) func(*testing.T, string) {
		return func(t *testing.T, _ string) { plantCopy(t, name, []byte(awsLiveState)) }
	}
	for name, tc := range map[string]struct {
		setup awsReapSetup
		// prepare plants what the refusal must leave as it was.
		prepare  func(t *testing.T, outDir string)
		takeOver string
		want     string
	}{
		"non-aws scenario": {
			setup:    awsReapSetup{name: awsHoldScenario, cloud: "scaleway", withState: true, flags: releaseHoldFlags},
			prepare:  identical(awsHoldScenario),
			takeOver: lifecycleOtherHolder, want: "not aws-web-live on scaleway",
		},
		"another aws scenario": {
			setup:    awsReapSetup{withState: true, flags: releaseHoldFlags},
			prepare:  identical("example-scenario"),
			takeOver: lifecycleOtherHolder, want: "not example-scenario on aws",
		},
		"no --take-over": {
			setup:   awsReapSetup{name: awsHoldScenario, withState: true, flags: []string{"--release-hold"}},
			prepare: identical(awsHoldScenario),
			want:    "needs --take-over",
		},
		"--dry-run": {
			setup:    awsReapSetup{name: awsHoldScenario, withState: true, flags: append([]string{"--dry-run"}, releaseHoldFlags...)},
			prepare:  identical(awsHoldScenario),
			takeOver: lifecycleOtherHolder, want: "cannot run with --dry-run",
		},
		"state missing, no copy": {
			setup:    awsReapSetup{name: awsHoldScenario, flags: releaseHoldFlags},
			takeOver: lifecycleOtherHolder, want: "no copy can be taken",
		},
		"state mode 0000": {
			setup: awsReapSetup{name: awsHoldScenario, withState: true, flags: releaseHoldFlags},
			prepare: func(t *testing.T, dir string) {
				require.NoError(t, os.Chmod(filepath.Join(dir, harness.LiveStateFilename), 0))
			},
			takeOver: lifecycleOtherHolder, want: "permission denied",
		},
		"holds dir pre-made 0755": {
			setup: awsReapSetup{name: awsHoldScenario, withState: true, flags: releaseHoldFlags},
			prepare: func(t *testing.T, _ string) {
				require.NoError(t, os.MkdirAll(holdsRoot, 0o700))
				require.NoError(t, os.Chmod(holdsRoot, 0o755))
			},
			takeOver: lifecycleOtherHolder, want: holdsRoot + " is drwxr-xr-x",
		},
		"holds dir a symlink": {
			setup: awsReapSetup{name: awsHoldScenario, withState: true, flags: releaseHoldFlags},
			prepare: func(t *testing.T, _ string) {
				require.NoError(t, os.MkdirAll(filepath.Dir(holdsRoot), 0o700))
				require.NoError(t, os.Symlink(t.TempDir(), holdsRoot))
			},
			takeOver: lifecycleOtherHolder, want: holdsRoot + " is L",
		},
		"copy differs from the output state": {
			setup:    awsReapSetup{name: awsHoldScenario, withState: true, flags: releaseHoldFlags},
			prepare:  func(t *testing.T, _ string) { plantCopy(t, awsHoldScenario, []byte(`{"resources":[]}`)) },
			takeOver: lifecycleOtherHolder, want: "differs from the copy",
		},
		"copy a symlink to the output state": {
			setup: awsReapSetup{name: awsHoldScenario, withState: true, flags: releaseHoldFlags},
			prepare: func(t *testing.T, dir string) {
				path := plantCopy(t, awsHoldScenario, nil)
				require.NoError(t, os.Remove(path))
				require.NoError(t, os.Symlink(filepath.Join(dir, harness.LiveStateFilename), path))
			},
			takeOver: lifecycleOtherHolder, want: "not a regular file of mode 0600 with one",
		},
		"copy mode 0644": {
			setup: awsReapSetup{name: awsHoldScenario, withState: true, flags: releaseHoldFlags},
			prepare: func(t *testing.T, _ string) {
				require.NoError(t, os.Chmod(plantCopy(t, awsHoldScenario, []byte(awsLiveState)), 0o644))
			},
			takeOver: lifecycleOtherHolder, want: "is -rw-r--r-- with 1 links",
		},
		"copy hard-linked": {
			setup: awsReapSetup{name: awsHoldScenario, withState: true, flags: releaseHoldFlags},
			prepare: func(t *testing.T, dir string) {
				path := plantCopy(t, awsHoldScenario, nil)
				require.NoError(t, os.Remove(path))
				require.NoError(t, os.Link(filepath.Join(dir, harness.LiveStateFilename), path))
			},
			takeOver: lifecycleOtherHolder, want: "is -rw------- with 2 links",
		},
	} {
		t.Run(name, func(t *testing.T) {
			lc := heldBy(newAWSLifecycle(t))
			copyPath := holdCopyPath(cmp.Or(tc.setup.name, "example-scenario"))
			var before string
			tc.setup.prepare = func(dir string) {
				if tc.prepare != nil {
					tc.prepare(t, dir)
				}
				before = describeCopy(t, copyPath)
			}

			r := reapAWSWith(t, lc, tc.setup)

			require.Error(t, r.err)
			assert.Contains(t, r.err.Error(), tc.want)
			holder := "<holder>"
			if tc.takeOver != "" {
				holder = shellQuote(tc.takeOver)
			}
			assert.Contains(t, r.err.Error(),
				"`"+reapCommand(r.h.ConfigPath, r.h.ScenarioPath)+" --take-over "+holder+"` destroys without the copy")
			assert.Empty(t, lc.log(), "no SSM, EC2 or STS request and no destroy")
			assert.Zero(t, lc.destroy.calls, "SandboxDestroy")
			lc.scw.assertUntouched(t)
			assert.Equal(t, before, describeCopy(t, copyPath), "the copy is left as it was")
		})
	}
}

func TestReleaseHoldKeepsAByteIdenticalCopy(t *testing.T) {
	lc := heldBy(newAWSLifecycle(t))
	var planted os.FileInfo

	r := reapAWSWith(t, lc, awsReapSetup{
		name: awsHoldScenario, withState: true, flags: releaseHoldFlags,
		prepare: func(string) {
			var err error
			planted, err = os.Lstat(plantCopy(t, awsHoldScenario, []byte(awsLiveState)))
			require.NoError(t, err)
		},
	})

	require.NoError(t, r.err, r.output)
	assert.Contains(t, r.output, "kept, identical to")
	after, err := os.Lstat(holdCopyPath(awsHoldScenario))
	require.NoError(t, err)
	assert.True(t, os.SameFile(planted, after), "the same file, not a rewrite")
	assertReleasedAfter(t, lc, putClaim)
}

func TestReleaseHoldRestoresAMissingStateFromTheCopy(t *testing.T) {
	lc := heldBy(newAWSLifecycle(t))
	var outDir string
	held := []byte(awsLiveState + "\n")
	lc.destroy.during = func(context.Context) {
		state, err := os.ReadFile(filepath.Join(outDir, harness.LiveStateFilename))
		require.NoError(t, err, "the destroy finds the state")
		assert.Equal(t, held, state, "the copy's bytes")
	}

	r := reapAWSWith(t, lc, awsReapSetup{
		name: awsHoldScenario, withState: true, flags: releaseHoldFlags,
		prepare: func(dir string) {
			outDir = dir
			plantCopy(t, awsHoldScenario, held)
			require.NoError(t, os.RemoveAll(dir))
		},
	})

	require.NoError(t, r.err, r.output)
	assert.Equal(t, 1, lc.destroy.calls)
	assertReleasedAfter(t, lc, putClaim)
	assert.Contains(t, r.output, "restored to "+filepath.Join(outDir, harness.LiveStateFilename))
}

// A release whose destroy ran and whose sweep then failed leaves the
// output state changed and the claim kept by the reap: its retry is
// refused before any AWS request, naming the take-over from that reap.
func TestReleaseHoldRetryAfterAFailedSweepIsRefused(t *testing.T) {
	lc := heldBy(newAWSLifecycle(t))
	// Every delete answers success, and the key pairs are still listed.
	lc.ec2["DescribeKeyPairs"] = strayKeyPairs
	var outDir string
	lc.destroy.during = func(context.Context) {
		require.NoError(t, os.WriteFile(filepath.Join(outDir, harness.LiveStateFilename), []byte(`{"resources":[]}`), 0o600))
	}
	first := reapAWSWith(t, lc, awsReapSetup{
		name: awsHoldScenario, withState: true, flags: releaseHoldFlags,
		prepare: func(dir string) { outDir = dir },
	})
	require.Error(t, first.err, "the sweep found strays")
	reapHolder, held := lc.claim()
	require.True(t, held)
	require.True(t, strings.HasPrefix(reapHolder, awsReapHolderPrefix), reapHolder)
	copied := describeCopy(t, holdCopyPath(awsHoldScenario))
	requests := len(lc.log())

	retry := first.again("--take-over", reapHolder, "--release-hold")

	require.Error(t, retry.err)
	assert.Contains(t, retry.err.Error(), "differs from the copy")
	assert.Contains(t, retry.err.Error(),
		"`"+reapCommand(retry.h.ConfigPath, retry.h.ScenarioPath)+" --take-over "+shellQuote(reapHolder)+"` destroys without the copy")
	assert.Len(t, lc.log(), requests, "no AWS request and no destroy")
	assert.Equal(t, copied, describeCopy(t, holdCopyPath(awsHoldScenario)), "the copy is left as it was")
}

// The copy's path is fixed: INFRAFACTORY_RUNSTORE_ROOT does not move it,
// it is under neither store the UI serves, and git ignores it.
func TestHoldCopyIsAtAFixedIgnoredPath(t *testing.T) {
	want := filepath.Join(".infrafactory", "holds", awsHoldScenario, harness.LiveStateFilename)
	for _, served := range []string{runstore.DefaultRoot, livestore.DefaultRoot} {
		rel, err := filepath.Rel(served, want)
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(rel, ".."+string(filepath.Separator)), "%s is under %s", want, served)
	}
	assertGitIgnores(t, filepath.Join("..", ".."), want)

	t.Setenv("INFRAFACTORY_RUNSTORE_ROOT", filepath.Join(t.TempDir(), "runs"))
	lc := heldBy(newAWSLifecycle(t))
	r := reapAWSWith(t, lc, awsReapSetup{name: awsHoldScenario, withState: true, flags: releaseHoldFlags})

	require.NoError(t, r.err, r.output)
	assert.FileExists(t, want)
}

// assertGitIgnores runs git check-ignore under a deadline that kills its
// whole process group.
func assertGitIgnores(t *testing.T, repo, path string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	git := exec.CommandContext(ctx, "git", "-C", repo, "check-ignore", "--quiet", path)
	git.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	git.Cancel = func() error { return syscall.Kill(-git.Process.Pid, syscall.SIGKILL) }
	assert.NoError(t, git.Run(), "git check-ignore %s", path)
}

func TestReapWithoutReleaseHoldMakesNoHoldsDir(t *testing.T) {
	lc := heldBy(newAWSLifecycle(t))

	r := reapAWSWith(t, lc, awsReapSetup{name: awsHoldScenario, withState: true,
		flags: []string{"--take-over", lifecycleOtherHolder}})

	require.NoError(t, r.err, r.output)
	assert.Equal(t, 1, lc.destroy.calls)
	_, err := os.Lstat(holdsRoot)
	assert.ErrorIs(t, err, fs.ErrNotExist)
}
