package cli

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/harness"
)

const getClaim = "ssm:GetParameter " + harness.AWSClaimParameter

type awsRunOptions struct {
	repairs int
	// loopEnded runs once the loop has ended, before the failure path.
	loopEnded func()
	notify    func(context.Context, ...os.Signal) (context.Context, context.CancelFunc)
	flags     []string
	// scenario, customize and deps edit the scenario, the config and the
	// dependencies after runAWSRun's defaults are set.
	scenario  func(*CommandTestHarness)
	customize func(*config.Config)
	deps      func(*RuntimeDependencies)
	// iterateFails: the loop ends in an error, so there is no result and
	// no terminal reason.
	iterateFails bool
}

// awsRun is one `infrafactory run` of an aws scenario, with the gate
// stubbed, through runRunWithNotify.
type awsRun struct {
	awsTestRun
	generates int
	// armCalls is every AWS and Layer 3 call made after the loop ended.
	armCalls []string
}

// logEvent calls fn on each log line carrying event.
type logEvent struct {
	event string
	fn    func()
}

func (w logEvent) Write(p []byte) (int, error) {
	if bytes.Contains(p, []byte(`"event":"`+w.event+`"`)) {
		w.fn()
	}
	return len(p), nil
}

func runAWSRun(t *testing.T, lc *awsLifecycle, o awsRunOptions) awsRun {
	t.Helper()
	h := newCommandTestHarness(t)
	setAWSLifecycleScenario(t, h.ScenarioPath)
	if o.scenario != nil {
		o.scenario(h)
	}
	if o.notify == nil {
		o.notify = lc.notify
	}

	opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
		cfg = layer3On(cfg)
		cfg.Agent.RepairIterationsMax = o.repairs
		cfg.AWS = config.AWSConfig{Region: "eu-west-2", AccountID: preflightAWSAccount, PrincipalARN: preflightAWSPrincipal}
		if o.customize != nil {
			o.customize(&cfg)
		}
		return cfg
	})
	run := awsRun{}
	opts.deps = RuntimeDependencies{
		Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
			lc.record(generated)
			run.generates++
			return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": []byte("terraform {}\n")}}, nil
		}),
		Static:         &fakeStaticHarness{result: &harness.StaticResult{PlanJSON: []byte(`{}`)}},
		MockState:      &fakeRunMockStateClient{statePayload: []byte(`{"instance":{"servers":[]}}`)},
		MockDeploy:     &fakeMockDeployHarness{},
		Destroy:        mockDestroyHook(nil),
		SandboxDeploy:  lc.deploy,
		SandboxDestroy: lc.destroy,
		RunProject:     lc.scw.runProject,
		OrphanSweep:    lc.scw.sweep,
		AutoCreated:    lc.scw.purge,
		AWSSTS:         lc,
		AWSSSM:         lc,
		AWSEC2:         lc,
		AWSSweepSleep:  func(context.Context, time.Duration) error { return nil },
		Layer3HCLGate:  func(layer3Cloud, string, awsGateInputs) error { return nil },
	}
	if o.deps != nil {
		o.deps(&opts.deps)
	}

	loopEnd := -1
	logs := logEvent{event: "terminal_reason", fn: func() {
		loopEnd = len(lc.log())
		if o.loopEnded != nil {
			o.loopEnded()
		}
	}}
	cmd := newRunCommandForTest(opts)
	cmd.RunE = withRuntimeWithOptions("run", opts, sealedHandler(logs,
		func(cmd *cobra.Command, args []string, rt *CommandRuntime) error {
			return runRunWithNotify(cmd, args, rt, o.notify)
		}))
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(append([]string{h.ScenarioPath, "--config", h.ConfigPath, "--output", string(OutputModeJSON),
		"--reset-mocks=false"}, o.flags...))
	run.h, run.err, run.output = h, cmd.Execute(), stdout.String()+stderr.String()
	if o.iterateFails {
		return run
	}
	require.Truef(t, strings.HasPrefix(stdout.String(), "{"), "run wrote no JSON result: %v\n%s", run.err, run.output)
	run.result = decodeMachineOutput(t, bytes.NewBufferString(stdout.String()))
	require.NotEqual(t, -1, loopEnd, "the loop ended: %s", run.output)
	run.armCalls = lc.log()[loopEnd:]
	return run
}

func (r awsRun) terminalReason() string {
	for _, s := range r.result.Stages {
		if s.Stage == "terminal_reason" {
			return s.Detail
		}
	}
	return ""
}

func writesIn(calls []string) []string {
	var out []string
	for _, c := range calls {
		if strings.HasPrefix(c, "ssm:PutParameter") || strings.HasPrefix(c, "ssm:DeleteParameter") {
			out = append(out, c)
		}
	}
	return out
}

// dirtySweep keeps the claim: the iteration's sweep finds an instance.
func dirtySweep(lc *awsLifecycle) { lc.ec2["DescribeInstances"] = runningInstance }

// applyFails fails the iteration without writing state, so a clean scope
// is released by the iteration's own teardown.
func applyFails(lc *awsLifecycle) {
	lc.deploy.err = errors.New("tofu apply failed")
	lc.deploy.onRunDir = func(string) { lc.record(deployRun) }
}

func TestAWSRunFailureArmLeavesAReleasedClaimAlone(t *testing.T) {
	lc := newAWSLifecycle(t)
	applyFails(lc)

	run := runAWSRun(t, lc, awsRunOptions{repairs: 1})

	require.Error(t, run.err)
	assert.Equal(t, 1, lc.count(deleteClaim), "only the iteration's release")
	assert.Equal(t, []string{getClaim}, run.armCalls, "the arm only reads the claim")
	assert.NotContains(t, run.output, "ParameterNotFound")
	i := slices.IndexFunc(run.result.Stages, isStage("auto_destroy"))
	require.NotEqual(t, -1, i, "the arm reports its skip")
	assert.Equal(t, StageStatusSkip, run.result.Stages[i].Status)
	assert.Contains(t, run.result.Stages[i].Detail, "not held by this run")
}

// Every release in one process uses its one holder: a release does not
// clear it, so the next iteration's claim is released too.
func TestAWSRunReleasesEachIterationsClaimWithTheProcessHolder(t *testing.T) {
	lc := newAWSLifecycle(t)
	applyFails(lc)

	run := runAWSRun(t, lc, awsRunOptions{repairs: 2})

	require.Error(t, run.err)
	assert.Equal(t, 2, run.generates, "generate")
	assert.Equal(t, 2, lc.count(deleteClaim), "each iteration released its claim")
	assert.NotContains(t, run.output, "this run may hold the aws scope's claim", "no release was unknown")
	_, held := lc.claim()
	assert.False(t, held)
}

func TestAWSRunFailureArmLeavesAnotherHoldersClaimAlone(t *testing.T) {
	lc := newAWSLifecycle(t)
	dirtySweep(lc)

	run := runAWSRun(t, lc, awsRunOptions{repairs: 1, loopEnded: func() {
		lc.mu.Lock()
		defer lc.mu.Unlock()
		lc.params[harness.AWSClaimParameter] = lifecycleOtherHolder
	}})

	require.Error(t, run.err)
	assert.Equal(t, []string{getClaim}, run.armCalls, "zero writes and zero EC2 calls")
	holder, _ := lc.claim()
	assert.Equal(t, lifecycleOtherHolder, holder)
}

// Interrupted with a dirty scope that the failure arm then finds clean:
// the notice waits for the arm, so it says the claim was released.
func TestInterruptedAWSRunNamesTheClaimTheFailureArmLeaves(t *testing.T) {
	lc := newAWSLifecycle(t)
	dirtySweep(lc)
	lc.deploy.err = context.Canceled
	lc.deploy.onRunDir = func(dir string) {
		lc.record(deployRun)
		writeAWSStateAndStaleMarker(t, dir, false)
		lc.signal()
	}

	notifies := 0
	notify := func(parent context.Context, sigs ...os.Signal) (context.Context, context.CancelFunc) {
		notifies++
		ctx, cancel := lc.notify(parent, sigs...)
		if notifies == 2 {
			cancel() // a second Ctrl-C, in the arm
		}
		return ctx, cancel
	}

	run := runAWSRun(t, lc, awsRunOptions{repairs: 2, notify: notify, loopEnded: func() {
		lc.mu.Lock()
		defer lc.mu.Unlock()
		delete(lc.ec2, "DescribeInstances")
	}})

	require.Error(t, run.err)
	assert.Equal(t, 2, notifies, "the arm runs under caught signals after the loop's interrupt")
	assert.Contains(t, run.armCalls, deleteClaim, "the arm released the claim")
	assertInterruptNamesTheReap(t, lc, run.output, run.h.ConfigPath, run.h.ScenarioPath, awsClaimNotHeld)
	assert.Equal(t, 1, strings.Count(run.output, "Interrupted:"), "one settled notice")
	assert.Equal(t, 1, strings.Count(run.output, "Interrupted —"), "one first-signal notice per process")
}

// A second Ctrl-C before the failure arm reads the claim does not make a
// claim this run holds unknown: the read ignores cancellation, as the
// sweep does, and the arm still destroys.
func TestAWSRunFailureArmReadsTheClaimThroughASecondInterrupt(t *testing.T) {
	lc := newAWSLifecycle(t)
	dirtySweep(lc)
	notifies := 0
	notify := func(parent context.Context, sigs ...os.Signal) (context.Context, context.CancelFunc) {
		ctx, cancel := lc.notify(parent, sigs...)
		notifies++
		if notifies == 2 {
			cancel() // the arm's teardown: interrupted before it reads the claim
		}
		return ctx, cancel
	}

	// The first Ctrl-C lands after the loop, so the arm is the teardown it
	// leaves; the second, on that teardown's context.
	run := runAWSRun(t, lc, awsRunOptions{repairs: 1, notify: notify, loopEnded: func() { lc.signal() }})

	require.Error(t, run.err)
	assert.Equal(t, getClaim, run.armCalls[0], "the arm reads the claim first")
	assert.Contains(t, run.armCalls, destroyRun, "the arm destroys")
	assert.NotContains(t, run.output, "this run may hold the aws scope's claim for "+lc.runHolder+":", "no hedged kept-claim detail")
	assertInterruptNamesTheReap(t, lc, run.output, run.h.ConfigPath, run.h.ScenarioPath, awsClaimHeld)
}

// A clean release clears runtime.awsClaim's holder; the arm still knows
// the run's own, so a claim stored under it is the run's to tear down.
func TestAWSRunFailureArmKnowsTheRunsHolderAfterARelease(t *testing.T) {
	lc := newAWSLifecycle(t)
	applyFails(lc)

	run := runAWSRun(t, lc, awsRunOptions{repairs: 1, loopEnded: func() {
		lc.mu.Lock()
		defer lc.mu.Unlock()
		lc.params[harness.AWSClaimParameter] = lc.runHolder
	}})

	require.Error(t, run.err)
	assert.Equal(t, []string{deleteClaim}, writesIn(run.armCalls), "the arm released the run's claim")
	_, held := lc.claim()
	assert.False(t, held)
}

// A stalled claim read in the arm, which no signal can end, is ended by
// its bound: the arm returns and reports the claim unknown.
func TestAWSRunFailureArmBoundsAStalledClaimRead(t *testing.T) {
	lc := newAWSLifecycle(t)
	dirtySweep(lc)
	awsClaimReadTimeout = 100 * time.Millisecond
	t.Cleanup(func() { awsClaimReadTimeout = harness.AWSClaimTimeout })

	run := runAWSRun(t, lc, awsRunOptions{repairs: 1, loopEnded: func() {
		lc.mu.Lock()
		defer lc.mu.Unlock()
		lc.stallClaimRead = true
	}})

	require.Error(t, run.err)
	lc.mu.Lock()
	assert.True(t, lc.stalledToContext, "the read ended by its bound, not by the stall's own limit")
	lc.mu.Unlock()
	assert.Contains(t, run.output, "this run may hold the aws scope's claim for "+lc.runHolder+":")
}

// A release that settles the claim as not held, or held by another, is
// not a kept claim: the claim stage's own failure, and the run ends for
// its own reason.
func TestAWSRunReleaseThatSettlesTheClaimElsewhereIsNotAKeptClaim(t *testing.T) {
	for name, tc := range map[string]struct {
		settle func(lc *awsLifecycle)
		want   string
	}{
		"not held": {settle: func(lc *awsLifecycle) { delete(lc.params, harness.AWSClaimParameter) },
			want: "this run does not hold the aws scope's claim:"},
		"another holder": {settle: func(lc *awsLifecycle) { lc.params[harness.AWSClaimParameter] = lifecycleOtherHolder },
			want: "this run does not hold the aws scope's claim: " + lifecycleOtherHolder + " does:"},
	} {
		t.Run(name, func(t *testing.T) {
			lc := newAWSLifecycle(t)
			applyFails(lc)
			lc.onEC2 = func(action string) {
				if action == "DescribeInstances" {
					tc.settle(lc)
				}
			}

			run := runAWSRun(t, lc, awsRunOptions{repairs: 1})

			require.Error(t, run.err)
			assert.False(t, slices.ContainsFunc(run.result.Stages, isStage(StageAWSScopeClaimKept)), "no %s stage", StageAWSScopeClaimKept)
			assert.NotEqual(t, terminalReasonAWSScopeClaimKept, run.terminalReason())
			assert.Contains(t, run.output, tc.want)
		})
	}
}

// The arm cannot read the claim, so it cannot tell whether this run still
// holds it: it names both reaps, each with the case it fits.
func TestAWSRunFailureArmNamesBothReapsWhenTheClaimIsUnreadable(t *testing.T) {
	lc := newAWSLifecycle(t)
	dirtySweep(lc)

	run := runAWSRun(t, lc, awsRunOptions{repairs: 1, loopEnded: func() {
		lc.mu.Lock()
		defer lc.mu.Unlock()
		lc.denied[getClaim] = true
	}})

	require.Error(t, run.err)
	assert.Equal(t, []string{getClaim}, run.armCalls, "the arm only tries to read the claim")
	details := run.output
	assert.Contains(t, details, "this run may hold the aws scope's claim for "+lc.runHolder)
	assert.Contains(t, details, "If this run holds the claim, `"+lc.takeOver(run.h.ConfigPath, run.h.ScenarioPath)+"` sweeps the scope")
	assert.Contains(t, details, "if no one holds it, `"+reapCommand(run.h.ConfigPath, run.h.ScenarioPath)+"` does")
}

func TestAWSRunEndsWhenAnIterationKeepsTheClaimForADirtySweep(t *testing.T) {
	for name, tc := range map[string]struct {
		loopEnded func(lc *awsLifecycle)
		released  bool
	}{
		"still dirty": {},
		"clean by the arm's sweep": {
			loopEnded: func(lc *awsLifecycle) { delete(lc.ec2, "DescribeInstances") },
			released:  true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			lc := newAWSLifecycle(t)
			dirtySweep(lc)
			o := awsRunOptions{repairs: 2}
			if tc.loopEnded != nil {
				o.loopEnded = func() {
					lc.mu.Lock()
					defer lc.mu.Unlock()
					tc.loopEnded(lc)
				}
			}

			run := runAWSRun(t, lc, o)

			require.Error(t, run.err)
			assert.Equal(t, 1, run.generates, "generate")
			assert.Equal(t, terminalReasonAWSScopeClaimKept, run.terminalReason())
			require.NotEmpty(t, run.armCalls)
			assert.Equal(t, getClaim, run.armCalls[0], "the arm reads the claim first")
			destroy := slices.Index(run.armCalls, destroyRun)
			sweep := slices.Index(run.armCalls, "ec2:DescribeInstances")
			require.NotEqual(t, -1, destroy, "the arm destroys: %v", run.armCalls)
			assert.Less(t, destroy, sweep, "then sweeps: %v", run.armCalls)

			_, held := lc.claim()
			if tc.released {
				assert.Equal(t, []string{deleteClaim}, writesIn(run.armCalls), "released after the clean sweep")
				assert.False(t, held)
				return
			}
			assert.Empty(t, writesIn(run.armCalls), "nothing released")
			assert.True(t, held, "the claim is kept")
			assert.Contains(t, run.output, lc.takeOver(run.h.ConfigPath, run.h.ScenarioPath))
		})
	}
}

// A deliberate keep is not a failure, as for test: a passing iteration
// reaches its target and still names reap for the claim it kept.
func TestAWSRunWithNoDestroyEndsAfterOneIteration(t *testing.T) {
	for name, tc := range map[string]struct {
		setup func(*awsLifecycle)
		want  string
	}{
		"failing iteration": {setup: applyFails, want: terminalReasonAWSScopeClaimKept},
		"passing iteration": {setup: func(*awsLifecycle) {}, want: "target_reached"},
	} {
		t.Run(name, func(t *testing.T) {
			lc := newAWSLifecycle(t)
			tc.setup(lc)

			run := runAWSRun(t, lc, awsRunOptions{repairs: 2, flags: []string{"--no-destroy"}})

			assert.Equal(t, tc.want == "target_reached", run.err == nil, "error: %v", run.err)
			assert.Equal(t, 1, run.generates, "generate")
			assert.Equal(t, tc.want, run.terminalReason())
			assert.Empty(t, run.armCalls, "--no-destroy: the failure path tears nothing down")
			_, held := lc.claim()
			assert.True(t, held, "the claim is kept")
			i := slices.IndexFunc(run.result.Stages, isStage(StageAWSScopeClaimKept))
			require.NotEqual(t, -1, i, "the run's stages carry %s", StageAWSScopeClaimKept)
			assert.Contains(t, run.result.Stages[i].Detail, lc.takeOver(run.h.ConfigPath, run.h.ScenarioPath))
			if tc.want == "target_reached" {
				assert.NotContains(t, run.output, "Run ended:", "a run that reached its target names no reap")
				return
			}
			assert.Contains(t, run.output, "\nRun ended: this run keeps the aws scope's claim for "+lc.runHolder+". `"+
				lc.takeOver(run.h.ConfigPath, run.h.ScenarioPath)+"` sweeps the scope")
		})
	}
}

// An iterate error ends the run before the failure arm, with the claim
// an iteration kept: the run still names the take-over.
func TestAWSRunIterateErrorNamesTheTakeOver(t *testing.T) {
	lc := newAWSLifecycle(t)
	dirtySweep(lc)
	var runstore string
	lc.deploy.onRunDir = func(string) {
		lc.record(deployRun)
		// persistRunIteration, after this iteration, cannot write.
		require.NoError(t, filepath.WalkDir(runstore, func(path string, d fs.DirEntry, err error) error {
			if err == nil && d.IsDir() {
				err = os.Chmod(path, 0o555)
			}
			return err
		}))
	}

	run := runAWSRun(t, lc, awsRunOptions{repairs: 2, iterateFails: true, scenario: func(h *CommandTestHarness) {
		runstore = h.RunstoreRoot()
		t.Cleanup(func() {
			_ = filepath.WalkDir(runstore, func(path string, d fs.DirEntry, err error) error {
				if err == nil && d.IsDir() {
					_ = os.Chmod(path, 0o755)
				}
				return nil
			})
		})
	}})

	require.Error(t, run.err)
	assert.Contains(t, run.err.Error(), "persist run iteration 1")
	_, held := lc.claim()
	require.True(t, held, "the iteration kept the claim")
	assert.Contains(t, run.output, "\nRun ended: this run keeps the aws scope's claim for "+lc.runHolder+". `"+
		lc.takeOver(run.h.ConfigPath, run.h.ScenarioPath)+"` sweeps the scope")
}

// Interrupted during the failure path's own sweep, after the loop: the
// claim is kept, reap is named, and the result is still written.
func TestAWSRunInterruptedDuringTheFailureArmPrintsTheReapCommand(t *testing.T) {
	lc := newAWSLifecycle(t)
	dirtySweep(lc)

	run := runAWSRun(t, lc, awsRunOptions{repairs: 1, loopEnded: func() {
		lc.mu.Lock()
		defer lc.mu.Unlock()
		lc.onEC2 = func(string) { lc.signal() }
	}})

	require.Error(t, run.err)
	assert.Contains(t, run.armCalls, destroyRun, "the arm reached its destroy")
	assert.Empty(t, writesIn(run.armCalls), "nothing released")
	_, held := lc.claim()
	assert.True(t, held, "the claim is kept")
	assertInterruptNamesTheReap(t, lc, run.output, run.h.ConfigPath, run.h.ScenarioPath, awsClaimHeld)
	assert.True(t, slices.ContainsFunc(run.result.Stages, isStage(StageAWSScopeClaimKept)))
}

// A first Ctrl-C during the failure arm's destroy stops that destroy, as
// it stops tofu; the arm runs again on a fresh context, its destroy
// completes and the clean sweep releases the claim.
func TestAWSRunFailureArmInterruptedDuringItsDestroyFinishes(t *testing.T) {
	lc := newAWSLifecycle(t)
	dirtySweep(lc)

	run := runAWSRun(t, lc, awsRunOptions{repairs: 1, loopEnded: func() {
		lc.destroy.clears = true
		lc.destroy.during = func(ctx context.Context) {
			lc.destroy.during = nil
			lc.signal()
			tofuReturns(ctx)
		}
	}})

	require.Error(t, run.err)
	destroys := slices.DeleteFunc(slices.Clone(run.armCalls), func(c string) bool { return c != destroyRun })
	assert.Len(t, destroys, 2, "the interrupted destroy, then the one that finished it")
	assert.Contains(t, run.armCalls, deleteClaim)
	_, held := lc.claim()
	assert.False(t, held, "the clean sweep released the claim")
	assertInterruptNamesTheReap(t, lc, run.output, run.h.ConfigPath, run.h.ScenarioPath, awsClaimNotHeld)
}

// A Ctrl-C after the loop and before the failure arm, while auto-learn
// would run, is caught: its notice prints, the arm still tears down, and
// the reap advice names the claim the arm leaves.
func TestAWSRunInterruptedBetweenTheLoopAndTheArmStillTearsDown(t *testing.T) {
	lc := newAWSLifecycle(t)
	dirtySweep(lc)

	run := runAWSRun(t, lc, awsRunOptions{repairs: 1, loopEnded: func() {
		lc.destroy.clears = true
		lc.signal()
	}})

	require.Error(t, run.err)
	assert.Equal(t, 1, strings.Count(run.output, "Interrupted — finishing teardown before exit"), "the signal was caught")
	assert.Contains(t, run.armCalls, destroyRun, "the arm tore down")
	assert.Contains(t, run.armCalls, deleteClaim)
	assertInterruptNamesTheReap(t, lc, run.output, run.h.ConfigPath, run.h.ScenarioPath, awsClaimNotHeld)
}

// Interrupted inside the apply. A dirty scope keeps the claim; a clean
// one is released, and the loop still stops.
func TestInterruptedAWSRunPrintsTheReapCommand(t *testing.T) {
	for name, tc := range map[string]struct {
		dirty bool
		want  string
		state awsClaimState
	}{
		"dirty scope": {dirty: true, want: terminalReasonAWSScopeClaimKept, state: awsClaimHeld},
		"clean scope": {want: "interrupted", state: awsClaimNotHeld},
	} {
		t.Run(name, func(t *testing.T) {
			lc := newAWSLifecycle(t)
			if tc.dirty {
				dirtySweep(lc)
			}
			lc.deploy.err = context.Canceled
			lc.deploy.onRunDir = func(dir string) {
				lc.record(deployRun)
				writeAWSStateAndStaleMarker(t, dir, false)
				lc.signal()
			}

			run := runAWSRun(t, lc, awsRunOptions{repairs: 2})

			require.Error(t, run.err)
			assert.Equal(t, 1, run.generates, "generate")
			assert.Equal(t, tc.want, run.terminalReason())
			assertInterruptNamesTheReap(t, lc, run.output, run.h.ConfigPath, run.h.ScenarioPath, tc.state)
			assert.NotContains(t, run.output, "nothing to clean up")
			lc.scw.assertUntouched(t)
		})
	}
}

// The same interrupt in `run` ends it "interrupted", never target_reached.
func TestAWSRunInterruptedDuringItsTeardownEndsInterrupted(t *testing.T) {
	lc := newAWSLifecycle(t)
	lc.onDelete = lc.signal

	run := runAWSRun(t, lc, awsRunOptions{repairs: 2})

	require.Error(t, run.err)
	assert.Equal(t, "interrupted", run.terminalReason())
	assert.Equal(t, 1, run.generates, "generate")
}
