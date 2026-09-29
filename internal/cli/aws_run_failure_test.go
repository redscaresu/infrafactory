package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
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
	setScenarioCloud(t, h.ScenarioPath, "aws")
	if o.notify == nil {
		o.notify = lc.notify
	}

	opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
		cfg = layer3On(cfg)
		cfg.Agent.RepairIterationsMax = o.repairs
		cfg.AWS = config.AWSConfig{Region: "eu-west-2", AccountID: preflightAWSAccount, PrincipalARN: preflightAWSPrincipal}
		return cfg
	})
	run := awsRun{}
	opts.deps = RuntimeDependencies{
		Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
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
		Layer3HCLGate:  func(layer3Cloud, string, []string) error { return nil },
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
			rt.AWSLayer3AMI = "ami-0deadbeef1234567"
			return runRunWithNotify(cmd, args, rt, o.notify)
		}))
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(append([]string{h.ScenarioPath, "--config", h.ConfigPath, "--output", string(OutputModeJSON),
		"--reset-mocks=false"}, o.flags...))
	run.h, run.err, run.output = h, cmd.Execute(), stdout.String()+stderr.String()
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
			assert.Contains(t, run.output, reapCommand(run.h.ConfigPath, run.h.ScenarioPath))
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
			assert.Contains(t, run.result.Stages[i].Detail, reapCommand(run.h.ConfigPath, run.h.ScenarioPath))
		})
	}
}

// Interrupted during the failure path's own sweep, after the loop: the
// claim is kept, reap is named, and the result is still written.
func TestAWSRunInterruptedDuringTheFailureArmPrintsTheReapCommand(t *testing.T) {
	lc := newAWSLifecycle(t)
	dirtySweep(lc)

	run := runAWSRun(t, lc, awsRunOptions{repairs: 1, loopEnded: func() {
		lc.mu.Lock()
		defer lc.mu.Unlock()
		lc.onEC2 = func(string) { lc.cancel() }
	}})

	require.Error(t, run.err)
	assert.Contains(t, run.armCalls, destroyRun, "the arm reached its destroy")
	assert.Empty(t, writesIn(run.armCalls), "nothing released")
	_, held := lc.claim()
	assert.True(t, held, "the claim is kept")
	assert.Contains(t, run.output, "Interrupted: this run keeps the aws scope's claim")
	assert.Contains(t, run.output, reapCommand(run.h.ConfigPath, run.h.ScenarioPath))
	assert.True(t, slices.ContainsFunc(run.result.Stages, isStage(StageAWSScopeClaimKept)))
}

// Interrupted inside the apply. A dirty scope keeps the claim; a clean
// one is released, and the loop still stops.
func TestInterruptedAWSRunPrintsTheReapCommand(t *testing.T) {
	for name, tc := range map[string]struct {
		dirty bool
		want  string
	}{
		"dirty scope": {dirty: true, want: terminalReasonAWSScopeClaimKept},
		"clean scope": {want: "interrupted"},
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
				lc.cancel()
			}

			run := runAWSRun(t, lc, awsRunOptions{repairs: 2})

			require.Error(t, run.err)
			assert.Equal(t, 1, run.generates, "generate")
			assert.Equal(t, tc.want, run.terminalReason())
			assert.Contains(t, run.output, "Interrupted: this run keeps the aws scope's claim")
			assert.Contains(t, run.output, reapCommand(run.h.ConfigPath, run.h.ScenarioPath))
			assert.NotContains(t, run.output, "nothing to clean up")
			lc.scw.assertUntouched(t)
		})
	}
}
