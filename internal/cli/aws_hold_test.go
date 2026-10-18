package cli

import (
	"cmp"
	"context"
	"crypto/sha256"
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

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/api"
	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/feedback"
	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/redscaresu/infrafactory/internal/livestore"
	"github.com/redscaresu/infrafactory/internal/runstore"
	"github.com/redscaresu/infrafactory/internal/scenario"
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
	elsewhere := t.TempDir()
	for name, tc := range map[string]struct {
		setup awsReapSetup
		// prepare plants what the refusal must leave as it was.
		prepare  func(t *testing.T, outDir string)
		takeOver string
		want     string
		// noTakeOver: the scenario's reap takes no --take-over, so the
		// refusal names none.
		noTakeOver bool
		// after checks what else the refusal must leave as it was.
		after func(t *testing.T)
	}{
		"non-aws scenario": {
			setup:    awsReapSetup{name: awsHoldScenario, cloud: "scaleway", withState: true, flags: releaseHoldFlags},
			prepare:  identical(awsHoldScenario),
			takeOver: lifecycleOtherHolder, want: "not aws-web-live on scaleway", noTakeOver: true,
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
		".infrafactory a symlink": {
			setup: awsReapSetup{name: awsHoldScenario, withState: true, flags: releaseHoldFlags},
			prepare: func(t *testing.T, _ string) {
				require.NoError(t, os.Symlink(elsewhere, filepath.Dir(holdsRoot)))
			},
			takeOver: lifecycleOtherHolder, want: filepath.Dir(holdsRoot) + " is L",
			after: func(t *testing.T) {
				written, err := os.ReadDir(elsewhere)
				require.NoError(t, err)
				assert.Empty(t, written, "nothing written under the symlink's target")
			},
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
			if tc.noTakeOver {
				assert.NotContains(t, r.err.Error(), "--take-over")
				assert.NotContains(t, r.err.Error(), "destroys without the copy")
			} else {
				assert.Contains(t, r.err.Error(),
					"`"+reapCommand(r.h.ConfigPath, r.h.ScenarioPath)+" --take-over "+holder+"` destroys without the copy")
			}
			assert.Empty(t, lc.log(), "no SSM, EC2 or STS request and no destroy")
			assert.Zero(t, lc.destroy.calls, "SandboxDestroy")
			lc.scw.assertUntouched(t)
			assert.Equal(t, before, describeCopy(t, copyPath), "the copy is left as it was")
			if tc.after != nil {
				tc.after(t)
			}
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

// chdirBesideSchema makes a fresh temp dir the cwd, so holdsRoot resolves
// in it, with the schema the default scenario loader looks for there.
func chdirBesideSchema(t *testing.T) {
	t.Helper()
	schema, err := os.ReadFile(filepath.Join("..", "..", scenario.DefaultSchemaPath))
	require.NoError(t, err)
	t.Chdir(t.TempDir())
	require.NoError(t, os.WriteFile(scenario.DefaultSchemaPath, schema, 0o600))
}

// renameScenario names the harness scenario at path.
func renameScenario(t *testing.T, path, name string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, []byte(strings.Replace(string(raw), "scenario: example-scenario", "scenario: "+name, 1)), 0o600))
}

// writeHeldOutput is a held stack's output dir: its state and its HCL.
func writeHeldOutput(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, harness.LiveStateFilename), []byte(`{"serial":7}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), []byte("terraform {}\n"), 0o600))
}

// hashTree is every entry under root: each file by its sha256, each dir
// as a dir.
func hashTree(t *testing.T, root string) map[string]string {
	t.Helper()
	sums := map[string]string{}
	require.NoError(t, filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			sums[path] = "dir"
			return err
		}
		data, err := os.ReadFile(path)
		sums[path] = fmt.Sprintf("%x", sha256.Sum256(data))
		return err
	}))
	return sums
}

// heldCopies plants, in the cwd, each thing at aws-web-live's copy path
// that refuses generate, run and test.
var heldCopies = map[string]func(t *testing.T){
	"a copy": func(t *testing.T) { plantCopy(t, awsHoldScenario, []byte("held state")) },
	"a broken symlink": func(t *testing.T) {
		path := holdCopyPath(awsHoldScenario)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.Symlink("nowhere", path))
	},
	"a holds dir of mode 0000": func(t *testing.T) {
		holds, err := filepath.Abs(holdsRoot)
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(holds, 0o700))
		require.NoError(t, os.Chmod(holds, 0))
		t.Cleanup(func() { _ = os.Chmod(holds, 0o700) })
	},
}

func assertRefusedWhileHeld(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	assert.Contains(t, err.Error(), holdCopyPath(awsHoldScenario), "names the copy")
	assert.Contains(t, err.Error(), "If the stack is still held, release it with", "names the release for a held stack")
	assert.Contains(t, err.Error(), "--take-over <holder> --release-hold", "names the release")
	assert.Contains(t, err.Error(), "once the read is done, delete the copy by hand, which lifts this refusal",
		"names what lifts it, which is all a released stack needs")
}

func TestGenerateRunAndTestRefuseWhileHeld(t *testing.T) {
	for held, plant := range heldCopies {
		for _, name := range []string{"generate", "run", "test"} {
			t.Run(held+"/"+name, func(t *testing.T) {
				chdirBesideSchema(t)
				plant(t)
				lc := newAWSLifecycle(t)
				var outDir string
				var before map[string]string
				run := awsCommand{name: name, layer3: true, scenario: awsHoldScenario, prepare: func(h *CommandTestHarness) {
					outDir = h.OutputDir()
					writeHeldOutput(t, filepath.Join(outDir, awsHoldScenario))
					before = hashTree(t, outDir)
				}}.run(t, lc)

				assert.Empty(t, lc.log(), "no STS, SSM or EC2 request, and no generation")
				assert.Zero(t, run.mock.calls, "no Layer 2 deploy")
				assert.Equal(t, before, hashTree(t, outDir), "the output dir is as it was")
				assertRefusedWhileHeld(t, run.err)
			})
		}
	}
}

// awsRequests counts, and refuses, every request a runtime built with
// its default AWS clients makes.
type awsRequests struct {
	mu    sync.Mutex
	hosts []string
}

func (r *awsRequests) RoundTrip(req *http.Request) (*http.Response, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hosts = append(r.hosts, req.URL.Host)
	return nil, errors.New("refused by the test")
}

// The UI starts a run through the run command's own code, so the hold
// refuses it there too. Without the refusal, this run reaches STS.
func TestUIRunRefusesWhileHeld(t *testing.T) {
	for held, plant := range heldCopies {
		t.Run(held, func(t *testing.T) {
			chdirBesideSchema(t)
			plant(t)
			newAWSLifecycle(t) // for its HOME and credential file
			requests := &awsRequests{}
			defaultClient := newAWSHTTPClient
			newAWSHTTPClient = func() *http.Client { return &http.Client{Transport: requests} }
			t.Cleanup(func() { newAWSHTTPClient = defaultClient })
			h := newCommandTestHarness(t)
			t.Setenv("INFRAFACTORY_RUNSTORE_ROOT", h.RunstoreRoot())
			t.Setenv("INFRAFACTORY_LIVESTORE_ROOT", h.LivestoreRoot())
			setAWSLifecycleScenario(t, h.ScenarioPath)
			renameScenario(t, h.ScenarioPath, awsHoldScenario)
			cfgFile, err := os.OpenFile(h.ConfigPath, os.O_APPEND|os.O_WRONLY, 0)
			require.NoError(t, err)
			_, err = fmt.Fprintf(cfgFile, "paths:\n  output: %s\naws:\n  region: eu-west-2\n  account_id: %q\n  principal_arn: %s\n",
				h.OutputDir(), preflightAWSAccount, preflightAWSPrincipal)
			require.NoError(t, err)
			require.NoError(t, cfgFile.Close())
			outDir := filepath.Join(h.OutputDir(), awsHoldScenario)
			writeHeldOutput(t, outDir)
			before := hashTree(t, h.OutputDir())
			llm := filepath.Join(t.TempDir(), "claude")
			writeScript(t, llm, "#!/bin/sh\ntouch \"$0.called\"\nexit 1\n")

			serverCfg := config.Default()
			serverCfg.Agent.Type = generator.AgentTypeClaudeCode
			serverCfg.Paths.Scenarios = filepath.Join(h.WorkspaceDir, "scenarios")
			serverCfg.Validation.Layers.SandboxDeploy.Enabled = true
			starter := &uiRunStarter{configPath: h.ConfigPath, cfg: serverCfg, hub: api.NewHub(), resolvedClaude: llm, schemaRunner: recordedSchemaRunner{}}
			err = starter.executeRun(context.Background(),
				api.StartRunRequest{ScenarioName: awsHoldScenario, ScenarioPath: "training/example"}, "20261012T000000Z")

			assert.Empty(t, requests.hosts, "no STS, SSM or EC2 request")
			assert.NoFileExists(t, llm+".called", "no LLM call")
			assert.Equal(t, before, hashTree(t, h.OutputDir()), "the output dir is as it was")
			assertRefusedWhileHeld(t, err)
		})
	}
}

func TestAnotherScenariosCopyRefusesNothing(t *testing.T) {
	for _, name := range []string{"generate", "run", "test"} {
		t.Run(name, func(t *testing.T) {
			chdirBesideSchema(t)
			plantCopy(t, "another-scenario", []byte("held state"))
			lc := newAWSLifecycle(t)

			run := awsCommand{name: name, layer3: true, scenario: awsHoldScenario}.run(t, lc)

			require.NoError(t, run.err, run.output)
			assert.Contains(t, lc.log(), "sts:GetCallerIdentity")
		})
	}
}

func TestDeletingTheCopyLiftsTheRefusal(t *testing.T) {
	chdirBesideSchema(t)
	path := plantCopy(t, awsHoldScenario, []byte("held state"))
	test := awsCommand{name: "test", layer3: true, scenario: awsHoldScenario}
	assertRefusedWhileHeld(t, test.run(t, newAWSLifecycle(t)).err)

	require.NoError(t, os.Remove(path))
	lc := newAWSLifecycle(t)
	run := test.run(t, lc)

	require.NoError(t, run.err, run.output)
	assert.Contains(t, lc.log(), "sts:GetCallerIdentity")
}

// runHoldTest is one `test aws-web-live` in the cwd, which the caller
// made a fresh dir, with probe as the real probe, so the scenario's
// criteria run.
func runHoldTest(t *testing.T, lc *awsLifecycle, probe *fakeRealProbeHarness, flags ...string) awsTestRun {
	t.Helper()
	return runAWSTestWith(t, lc, awsTestSetup{name: awsHoldScenario, flags: flags, deps: func(deps *RuntimeDependencies) {
		deps.RealProbe = probe
		deps.MockDeploy = &fakeMockDeployHarness{result: &harness.MockDeployResult{}}
	}})
}

func passingProbe() *fakeRealProbeHarness {
	return &fakeRealProbeHarness{result: &harness.RealProbeResult{}}
}

// holdAWSWebLive runs a passing `test aws-web-live --hold` in a fresh
// cwd, and returns it with the hold's holder.
func holdAWSWebLive(t *testing.T, lc *awsLifecycle) (awsTestRun, string) {
	t.Helper()
	chdirBesideSchema(t)
	run := runHoldTest(t, lc, passingProbe(), "--hold")
	require.NoError(t, run.err, run.output)
	holder, held := lc.claim()
	require.True(t, held, "held")
	return run, holder
}

func TestHoldKeepsAPassingStackUnderTheClaim(t *testing.T) {
	lc := newAWSLifecycle(t)

	run, holder := holdAWSWebLive(t, lc)

	assert.Equal(t, lc.runHolder, holder, "the claim is the run's")
	assert.Zero(t, lc.count(destroyRun), "no destroy")
	assert.Zero(t, lc.count(deleteClaim), "no release")
	assert.False(t, run.hasStage("aws_scope_sweep"))
	assert.False(t, run.hasStage("aws_scope_release"))
	i := slices.IndexFunc(run.result.Stages, isStage(StageAWSScopeClaimKept))
	require.NotEqual(t, -1, i, "%+v", run.result.Stages)
	kept := run.result.Stages[i]
	assert.Equal(t, StageStatusSkip, kept.Status)
	copyPath := holdCopyPath(awsHoldScenario)
	assert.Contains(t, kept.Detail, "kept the aws scope's claim for "+holder+" on purpose (--hold)")
	assert.Contains(t, kept.Detail, "what this run applied is still there. Its tfstate is copied to "+copyPath)
	assert.Contains(t, kept.Detail, "nothing expires the hold")
	assert.Contains(t, kept.Detail, "`"+reapCommand(run.h.ConfigPath, run.h.ScenarioPath)+" --take-over "+shellQuote(holder)+" --release-hold`")
	assert.Contains(t, kept.Detail, "--config "+shellQuote(run.h.ConfigPath), "names the --config passed")
	assert.NotContains(t, kept.Detail, preflightAWSAccount)

	state, err := os.ReadFile(filepath.Join(run.h.OutputDir(), awsHoldScenario, harness.LiveStateFilename))
	require.NoError(t, err)
	assert.Equal(t, "-rw------- 1 links: "+string(state), describeCopy(t, copyPath), "the copy is the output dir's state")
	for _, dir := range []string{holdsRoot, filepath.Dir(copyPath)} {
		info, err := os.Lstat(dir)
		require.NoError(t, err)
		assert.Equal(t, fs.ModeDir|0o700, info.Mode(), dir)
	}
}

// The hold's process has exited, as it has once a hold is kept: a run in
// this process in the same second would otherwise mint the same holder.
func TestWhileHeldTheClaimRefusesEveryOtherCommand(t *testing.T) {
	mintHolderOfAReapedChild(t)
	lc := newAWSLifecycle(t)
	held, holder := holdAWSWebLive(t, lc)
	newAWSClaimHolder = harness.NewAWSClaimHolder
	deploys := lc.deploy.calls

	requests := len(lc.log())
	again := runHoldTest(t, lc, passingProbe())
	assertRefusedWhileHeld(t, again.err)
	assert.Len(t, lc.log(), requests, "the guard refuses test aws-web-live with no AWS request")

	other := runAWSTest(t, lc, nil, nil)
	require.Error(t, other.err)
	assert.Contains(t, failureAt(t, other.result, "aws_scope_claim").Detail, holder)
	assert.Equal(t, deploys, lc.deploy.calls, "no apply")

	written := writes(lc)
	plain := reapAWSWith(t, lc, awsReapSetup{h: held.h})
	require.Error(t, plain.err)
	assert.Contains(t, plain.text(), holder)
	assert.Equal(t, written, writes(lc), "plain reap writes nothing")
	assert.Zero(t, lc.count(destroyRun))

	still, _ := lc.claim()
	assert.Equal(t, holder, still)
}

// Each hold that is not kept ends as any other run does. The instance
// runs until a destroy completes, so only that destroy lets the sweep
// find the scope empty and release.
func TestHoldNotKeptDestroysSweepsAndReleases(t *testing.T) {
	for name, tc := range map[string]struct {
		setup func(t *testing.T, lc *awsLifecycle, probe *fakeRealProbeHarness)
		want  string
	}{
		"a failing real_probe": {
			setup: func(_ *testing.T, _ *awsLifecycle, probe *fakeRealProbeHarness) {
				probe.result = &harness.RealProbeResult{Failures: []feedback.Failure{{Layer: "sandbox_deploy", Stage: "real_probe", Check: "connectivity", Detail: "the probe was refused"}}}
			},
			want: "the probe was refused",
		},
		"a copy that cannot be written": {
			setup: func(t *testing.T, _ *awsLifecycle, _ *fakeRealProbeHarness) {
				require.NoError(t, os.MkdirAll(holdsRoot, 0o700))
				require.NoError(t, os.Chmod(holdsRoot, 0o755))
			},
			want: "its tfstate copy " + holdCopyPath(awsHoldScenario) + " could not be written",
		},
		"an interrupt from the last check": {
			setup: func(_ *testing.T, lc *awsLifecycle, probe *fakeRealProbeHarness) {
				probe.failWhen = func([]harness.ProbeCheck) *harness.RealProbeResult {
					lc.signal()
					return nil
				}
			},
			want: "not held, so the teardown runs as it does for any other run: the run was interrupted",
		},
		"an interrupt from inside the copy write": {
			setup: func(t *testing.T, lc *awsLifecycle, _ *fakeRealProbeHarness) {
				holdCopy = func(copyPath, statePath string) error {
					err := writeNewHoldCopy(copyPath, statePath)
					lc.signal()
					return err
				}
				t.Cleanup(func() { holdCopy = writeNewHoldCopy })
			},
			want: "the run was interrupted. The copy " + holdCopyPath(awsHoldScenario) + " is kept",
		},
	} {
		t.Run(name, func(t *testing.T) {
			chdirBesideSchema(t)
			lc := newAWSLifecycle(t)
			probe := passingProbe()
			tc.setup(t, lc, probe)
			lc.mu.Lock()
			lc.ec2["DescribeInstances"] = runningInstance
			lc.mu.Unlock()
			lc.destroy.clears = true

			run := runHoldTest(t, lc, probe, "--hold")

			require.Error(t, run.err)
			assert.Contains(t, run.output, tc.want)
			assert.Positive(t, lc.count(destroyRun), "destroyed")
			assert.Equal(t, StageStatusPass, stageStatus(run.result.Stages, "sandbox_deploy", "aws_scope_sweep"))
			assert.Equal(t, StageStatusPass, stageStatus(run.result.Stages, "sandbox_deploy", "aws_scope_release"))
			_, held := lc.claim()
			assert.False(t, held, "released")
			assert.False(t, run.hasStage(StageAWSScopeClaimKept), "no held line")
			assert.NotContains(t, run.output, "on purpose (--hold)")
		})
	}
}

func TestHoldRefusalsAreUsageErrorsBeforeAnyAWSRequest(t *testing.T) {
	for name, tc := range map[string]struct {
		scenario  string
		customize func(*config.Config)
		flags     []string
		want      string
	}{
		"another scenario": {scenario: "example-scenario", want: "--hold holds the aws scenario aws-web-live only, not example-scenario on aws"},
		"layer 3 off": {
			customize: func(cfg *config.Config) { cfg.Validation.Layers.SandboxDeploy.Enabled = false },
			want:      "--hold keeps a Layer 3 stack, which test applies only with",
		},
		// Layer 3 runs only after Layer 2, so without it nothing is held.
		"layer 2 off": {
			customize: func(cfg *config.Config) { cfg.Validation.Layers.MockDeploy.Enabled = false },
			want:      "--hold keeps a Layer 3 stack, which test applies only with",
		},
		"destruction disabled": {
			customize: func(cfg *config.Config) { cfg.Validation.Layers.Destruction.Enabled = false },
			want:      "destruction (validation.layers.destruction) is disabled",
		},
		"with --no-destroy": {flags: []string{"--no-destroy"}, want: "hold and no-destroy are mutually exclusive"},
	} {
		t.Run(name, func(t *testing.T) {
			chdirBesideSchema(t)
			lc := newAWSLifecycle(t)

			run := runAWSTestWith(t, lc, awsTestSetup{
				name: cmp.Or(tc.scenario, awsHoldScenario), customize: tc.customize, flags: append([]string{"--hold"}, tc.flags...),
			})

			var cliErr *CLIError
			require.ErrorAs(t, run.err, &cliErr)
			assert.Equal(t, errorCodeUsage, cliErr.Code)
			assert.Contains(t, cliErr.Error(), tc.want)
			assert.Empty(t, lc.log(), "no STS, SSM or EC2 request")
			assert.NoDirExists(t, holdsRoot)
		})
	}
}

func TestWithoutHoldNothingIsCopied(t *testing.T) {
	for name, flags := range map[string][]string{"no flag": nil, "--no-destroy": {"--no-destroy"}} {
		t.Run(name, func(t *testing.T) {
			chdirBesideSchema(t)
			lc := newAWSLifecycle(t)

			run := runHoldTest(t, lc, passingProbe(), flags...)

			require.NoError(t, run.err, run.output)
			assert.NoDirExists(t, holdsRoot)
		})
	}
}

// Run's flags are the UI's, so the UI cannot hold.
func TestHoldIsATestFlagOnly(t *testing.T) {
	run := &cobra.Command{}
	registerRunFlags(run, true)
	assert.Nil(t, run.Flags().Lookup("hold"))

	test, _, err := NewRootCmd().Find([]string{"test"})
	require.NoError(t, err)
	assert.NotNil(t, test.Flags().Lookup("hold"))
}

// mintHolderOfAReapedChild makes this process mint the holder a held
// test leaves once its process has exited: the pid of a child this test
// started and waited for.
func mintHolderOfAReapedChild(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, "true")
	require.NoError(t, child.Run())
	host, err := os.Hostname()
	require.NoError(t, err)
	minted := newAWSClaimHolder
	newAWSClaimHolder = func(runID string) (string, error) {
		return fmt.Sprintf("%s@%s:%d", runID, host, child.ProcessState.Pid()), nil
	}
	t.Cleanup(func() { newAWSClaimHolder = minted })
}

func TestHoldThenReleaseHoldDestroysTheRestoredState(t *testing.T) {
	mintHolderOfAReapedChild(t)
	lc := newAWSLifecycle(t)
	held, holder := holdAWSWebLive(t, lc)
	copied, err := os.ReadFile(holdCopyPath(awsHoldScenario))
	require.NoError(t, err)
	outDir := filepath.Join(held.h.OutputDir(), awsHoldScenario)
	require.NoError(t, os.RemoveAll(outDir))
	lc.mu.Lock()
	lc.ec2["DescribeInstances"] = runningInstance
	lc.mu.Unlock()
	lc.destroy.clears = true
	lc.destroy.during = func(context.Context) {
		state, err := os.ReadFile(filepath.Join(outDir, harness.LiveStateFilename))
		require.NoError(t, err, "the destroy runs on the restored state")
		assert.Equal(t, copied, state)
	}

	r := reapAWSWith(t, lc, awsReapSetup{h: held.h, flags: []string{"--take-over", holder, "--release-hold"}})

	require.NoError(t, r.err, r.output)
	assert.Contains(t, r.output, "Hold copy "+holdCopyPath(awsHoldScenario)+": restored to")
	assert.Equal(t, 1, lc.count(destroyRun))
	assert.Contains(t, r.output, "every swept collection in the scope is empty")
	_, stillHeld := lc.claim()
	assert.False(t, stillHeld, "released")
}

func TestReleaseHoldRefusesWhileTheHoldersProcessRuns(t *testing.T) {
	lc := newAWSLifecycle(t)
	held, holder := holdAWSWebLive(t, lc)

	r := reapAWSWith(t, lc, awsReapSetup{h: held.h, flags: []string{"--take-over", holder, "--release-hold"}})

	require.Error(t, r.err)
	assert.Contains(t, r.text(), fmt.Sprintf("process %d on this host is still running", os.Getpid()))
	assert.Zero(t, lc.count(destroyRun))
	still, _ := lc.claim()
	assert.Equal(t, holder, still)
}

// A passing run's teardown, with the hold wanted, keeps only a claim this
// process knows it holds: any other claim state destroys, sweeps and
// writes no copy. A passing run through test always holds its claim, so
// the claim state is set on the runtime, as a teardown would find it.
func TestHoldKeepsOnlyAClaimThisRunHolds(t *testing.T) {
	const holder = "run-9@this-host.example:1"
	for name, claim := range map[string]awsClaim{
		"not held":        {state: awsClaimNotHeld},
		"held by another": {state: awsClaimHeldByOther, other: lifecycleOtherHolder},
		"unknown":         {state: awsClaimUnknown},
	} {
		t.Run(name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			lc := newAWSLifecycle(t)
			if claim.state == awsClaimHeldByOther {
				lc.params[harness.AWSClaimParameter] = lifecycleOtherHolder
			}
			cfg := layer3On(config.Default())
			cfg.AWS = config.AWSConfig{Region: "eu-west-2", AccountID: preflightAWSAccount, PrincipalARN: preflightAWSPrincipal}
			rt := &CommandRuntime{ConfigPath: config.DefaultPath, Config: cfg, scenarioPath: "scenarios/training/aws-web-live.yaml",
				awsHolder: holder, awsClaim: claim, Logger: NewAppLogger(&strings.Builder{})}
			rt.Deps.SandboxDestroy = lc.destroy
			rt.Deps.AWSSTS, rt.Deps.AWSSSM, rt.Deps.AWSEC2 = lc, lc, lc
			rt.Deps.AWSSweepSleep = func(context.Context, time.Duration) error { return nil }
			outDir := t.TempDir()
			writeAWSStateAndStaleMarker(t, outDir, true)

			stages, failures := awsScopeTeardown(context.Background(), rt, outDir, testExecutionOptions{},
				awsHold{wanted: true, run: context.Background()})

			assert.Equal(t, 1, lc.count(destroyRun), "destroyed")
			assert.Equal(t, StageStatusPass, stageStatus(stages, "sandbox_deploy", "aws_scope_sweep"))
			assert.NoDirExists(t, holdsRoot, "no copy")
			assert.Equal(t, StageStatusFail, stageStatus(stages, "sandbox_deploy", StageAWSHold))
			assert.False(t, slices.ContainsFunc(stages, func(s StageSummary) bool {
				return s.Stage == StageAWSScopeClaimKept && s.Status == StageStatusSkip
			}), "no held line: %+v", stages)
			hold := slices.IndexFunc(failures, func(f FailureSummary) bool { return f.Stage == StageAWSHold })
			require.NotEqual(t, -1, hold, "%+v", failures)
			assert.Equal(t, "claim", failures[hold].Check)
			assert.Contains(t, failures[hold].Detail, "not held, so the teardown runs as it does for any other run: "+awsClaimHead(rt, claim))
		})
	}
}
