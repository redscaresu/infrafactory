package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
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

// TestMain gives every AWS dependency a test leaves nil a transport that
// panics, so the test fails, naming itself, before a request leaves.
func TestMain(m *testing.M) {
	newAWSHTTPClient = func() *http.Client { return &http.Client{Transport: realAWSRefused{}} }
	os.Exit(m.Run())
}

const realAWSRefusedPanic = "a test reached real AWS through a nil Deps.AWSSTS, AWSSSM or AWSEC2: inject the awsLifecycle doer"

type realAWSRefused struct{}

func (realAWSRefused) RoundTrip(*http.Request) (*http.Response, error) { panic(realAWSRefusedPanic) }

// resolvedStage is the resolve's pass stage for the AMI and root
// newAWSLifecycle serves.
var resolvedStage = StageSummary{
	Layer: "sandbox_deploy", Stage: StageAWSAMIResolve, Status: StageStatusPass,
	Detail: harness.AWSAL2023AMIParameter + " names " + harness.AWSLayer2AMI + ", whose root is 8 GiB gp3, deleted on termination: true",
}

// awsCommand is one generate, run or test of a scenario against lc, the
// generator and a stub gate logging to lc's call log.
type awsCommand struct {
	name   string
	layer3 bool
	// cloud "" is aws, with awsLifecycleService.
	cloud string
	// gateRefusals is how many of the gate's first calls refuse, so a
	// run iterates again.
	gateRefusals int
	// cancelDuring cancels the command's context as the first request to
	// this service ("sts", "ssm" or "ec2") arrives.
	cancelDuring string
	customize    func(*config.Config)
}

type awsCommandRun struct {
	h      *CommandTestHarness
	err    error
	stdout string
	// output is stdout, stderr and the logs.
	output string
	amis   []string
	mock   *fakeMockDeployHarness
}

func (r awsCommandRun) result(t *testing.T) OutputResult {
	t.Helper()
	return decodeMachineOutput(t, bytes.NewBufferString(r.stdout))
}

// cancelDuring is lc, with ctx cancelled as a request to service
// arrives, which then fails as a cancelled request does.
type cancelDuring struct {
	*awsLifecycle
	service string
	cancel  context.CancelFunc
}

func (c cancelDuring) Do(req *http.Request) (*http.Response, error) {
	if strings.HasPrefix(req.URL.Host, c.service+".") {
		c.cancel()
		return nil, req.Context().Err()
	}
	return c.awsLifecycle.Do(req)
}

func (c awsCommand) run(t *testing.T, lc *awsLifecycle) awsCommandRun {
	t.Helper()
	h := newCommandTestHarness(t)
	if c.cloud == "" {
		setAWSLifecycleScenario(t, h.ScenarioPath)
	} else {
		setScenarioCloud(t, h.ScenarioPath, c.cloud)
	}
	opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
		cfg.Validation.Layers.SandboxDeploy.Enabled = c.layer3
		cfg.Agent.RepairIterationsMax = 2
		cfg.AWS = config.AWSConfig{Region: "eu-west-2", AccountID: preflightAWSAccount, PrincipalARN: preflightAWSPrincipal}
		if c.customize != nil {
			c.customize(&cfg)
		}
		return cfg
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var doer interface {
		Do(*http.Request) (*http.Response, error)
	} = lc
	if c.cancelDuring != "" {
		doer = cancelDuring{awsLifecycle: lc, service: c.cancelDuring, cancel: cancel}
	}

	run := awsCommandRun{h: h, mock: &fakeMockDeployHarness{}}
	gateCalls := 0
	opts.deps = RuntimeDependencies{
		Generator: generator.SeedGeneratorFunc(func(_ context.Context, req generator.Request) (*generator.GeneratedCode, error) {
			lc.record(generated)
			run.amis = append(run.amis, req.AMIID)
			return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": []byte("terraform {}\n")}}, nil
		}),
		Static:         &fakeStaticHarness{result: &harness.StaticResult{PlanJSON: []byte(`{}`)}},
		MockState:      &fakeRunMockStateClient{statePayload: []byte(`{"instance":{"servers":[]}}`)},
		MockDeploy:     run.mock,
		Destroy:        mockDestroyHook(nil),
		SandboxDeploy:  lc.deploy,
		SandboxDestroy: lc.destroy,
		RunProject:     lc.scw.runProject,
		OrphanSweep:    lc.scw.sweep,
		AutoCreated:    lc.scw.purge,
		AWSSTS:         doer,
		AWSSSM:         doer,
		AWSEC2:         doer,
		AWSSweepSleep:  func(context.Context, time.Duration) error { return nil },
		Layer3HCLGate: func(layer3Cloud, string, awsGateInputs) error {
			lc.record(gateCall)
			if gateCalls++; gateCalls <= c.gateRefusals {
				return errors.New("refused by the stub gate")
			}
			return nil
		},
	}

	logs := &bytes.Buffer{}
	args := []string{h.ScenarioPath, "--config", h.ConfigPath, "--output", string(OutputModeJSON)}
	var cmd *cobra.Command
	switch c.name {
	case "generate":
		cmd = newGenerateCommandForTest(opts)
		cmd.RunE = withRuntimeWithOptions(c.name, opts, sealedHandler(logs, runGenerateCommand))
	case "run":
		cmd = newRunCommandForTest(opts)
		cmd.RunE = withRuntimeWithOptions(c.name, opts, sealedHandler(logs, func(cmd *cobra.Command, args []string, rt *CommandRuntime) error {
			return runRunWithNotify(cmd, args, rt, lc.notify)
		}))
		args = append(args, "--reset-mocks=false")
	case "test":
		cmd = newTestCommandForTest(opts)
		cmd.RunE = withRuntimeWithOptions(c.name, opts, sealedHandler(logs, func(cmd *cobra.Command, args []string, rt *CommandRuntime) error {
			return runTestWithNotify(cmd, args, rt, lc.notify)
		}))
	default:
		t.Fatalf("no command %q", c.name)
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(args)
	run.err = cmd.ExecuteContext(ctx)
	run.stdout = stdout.String()
	run.output = stdout.String() + stderr.String() + logs.String()
	return run
}

func countStage(stages []StageSummary, stage string) int {
	n := 0
	for _, s := range stages {
		if s.Stage == stage {
			n++
		}
	}
	return n
}

func TestAWSAMIResolveRunsOnceAfterSTSAndBeforeGenerationOrTheGate(t *testing.T) {
	for name, tc := range map[string]struct {
		cmd awsCommand
		// before is the first call the resolve must precede.
		before    string
		generates int
	}{
		"generate": {cmd: awsCommand{name: "generate", layer3: true}, before: generated, generates: 1},
		// The gate refuses iteration 1, so the run generates twice.
		"run":  {cmd: awsCommand{name: "run", layer3: true, gateRefusals: 1}, before: generated, generates: 2},
		"test": {cmd: awsCommand{name: "test", layer3: true}, before: gateCall},
	} {
		t.Run(name, func(t *testing.T) {
			lc := newAWSLifecycle(t)

			run := tc.cmd.run(t, lc)

			require.NoError(t, run.err, run.output)
			calls := lc.log()
			sts := slices.Index(calls, "sts:GetCallerIdentity")
			require.NotEqual(t, -1, sts, "%v", calls)
			assert.Less(t, sts, slices.Index(calls, getAMI), "STS before SSM: %v", calls)
			assert.Less(t, slices.Index(calls, getAMI), slices.Index(calls, describeAMI), "SSM before EC2: %v", calls)
			assert.Less(t, slices.Index(calls, describeAMI), slices.Index(calls, tc.before), "EC2 before %s: %v", tc.before, calls)
			assert.Equal(t, 1, lc.count(getAMI), "%v", calls)
			assert.Equal(t, 1, lc.count(describeAMI), "%v", calls)
			assert.Equal(t, tc.generates, lc.count(generated), "%v", calls)
			for _, ami := range run.amis {
				assert.Equal(t, harness.AWSLayer2AMI, ami, "the model is handed the resolved id")
			}
			stages := run.result(t).Stages
			assert.Contains(t, stages, resolvedStage)
			assert.Equal(t, 1, countStage(stages, StageAWSAMIResolve), "the output records one resolve")
		})
	}
}

// Every iteration generates against the run's one resolve, so each
// iteration.json leads with its stage.
func TestAWSRunRecordsItsResolveFirstInEveryIteration(t *testing.T) {
	lc := newAWSLifecycle(t)

	run := awsCommand{name: "run", layer3: true, gateRefusals: 1}.run(t, lc)

	require.NoError(t, run.err, run.output)
	paths, err := filepath.Glob(filepath.Join(run.h.RunstoreRoot(), "*", "*", "iterations", "*", "iteration.json"))
	require.NoError(t, err)
	require.Len(t, paths, 2)
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		var iteration struct {
			Stages []StageSummary `json:"stages"`
		}
		require.NoError(t, json.Unmarshal(raw, &iteration))
		require.NotEmpty(t, iteration.Stages, path)
		assert.Equal(t, resolvedStage, iteration.Stages[0], path)
		assert.Equal(t, 1, countStage(iteration.Stages, StageAWSAMIResolve), path)
	}
}

func amiImageEdit(old, replacement string) func(*awsLifecycle) {
	return func(lc *awsLifecycle) { lc.amiImage = strings.Replace(lc.amiImage, old, replacement, 1) }
}

func TestAWSAMIResolveFailureEndsTheCommandBeforeAnyModelCallOrClaim(t *testing.T) {
	steps := map[string]struct {
		cmd  awsCommand
		lc   func(*awsLifecycle)
		want string
	}{
		"empty aws.region":          {cmd: awsCommand{customize: func(cfg *config.Config) { cfg.AWS.Region = "" }}, want: "aws.region is empty"},
		"sts names another account": {cmd: awsCommand{customize: func(cfg *config.Config) { cfg.AWS.AccountID = "999999999999" }}, want: "not the configured account"},
		"ssm denies the parameter":  {lc: func(lc *awsLifecycle) { lc.denied[getAMI] = true }, want: "AccessDeniedException"},
		"ssm holds no AMI id":       {lc: func(lc *awsLifecycle) { lc.params[harness.AWSAL2023AMIParameter] = "resolve:ssm:al2023" }, want: "which is not an AMI id"},
		"a root that is not ebs":    {lc: amiImageEdit("<rootDeviceType>ebs<", "<rootDeviceType>instance-store<"), want: "not ebs"},
		"no mapping for the root":   {lc: amiImageEdit("<deviceName>/dev/xvda<", "<deviceName>/dev/sdb<"), want: "has no EBS mapping for its root device"},
		"cancelled during sts":      {cmd: awsCommand{cancelDuring: "sts"}, want: context.Canceled.Error()},
		"cancelled during ssm":      {cmd: awsCommand{cancelDuring: "ssm"}, want: context.Canceled.Error()},
		"cancelled during ec2":      {cmd: awsCommand{cancelDuring: "ec2"}, want: context.Canceled.Error()},
		"ec2 returns no image":      {lc: func(lc *awsLifecycle) { lc.amiImage = "" }, want: "returned 0 images"},
	}
	for _, command := range []string{"generate", "run", "test"} {
		for name, step := range steps {
			t.Run(command+"/"+name, func(t *testing.T) {
				lc := newAWSLifecycle(t)
				if step.lc != nil {
					step.lc(lc)
				}
				c := step.cmd
				c.name, c.layer3 = command, true

				run := c.run(t, lc)

				require.Error(t, run.err)
				assert.Contains(t, run.err.Error(), StageAWSAMIResolve+": ")
				assert.Contains(t, run.err.Error(), step.want)
				assert.Zero(t, lc.count(generated), "generator")
				assert.Zero(t, lc.count(gateCall), "gate")
				assert.Zero(t, run.mock.calls, "MockDeploy")
				assert.Zero(t, lc.count(putClaim), "claim written")
				assert.NotContains(t, run.output, reapCommand(run.h.ConfigPath, run.h.ScenarioPath))
				assert.NotContains(t, run.output, "Interrupted")
				if command != "test" {
					return
				}
				result := run.result(t)
				assert.Equal(t, []StageSummary{{Layer: "sandbox_deploy", Stage: StageAWSAMIResolve, Status: StageStatusFail}}, result.Stages)
				require.Len(t, result.Failures, 1)
				assert.Equal(t, StageAWSAMIResolve, result.Failures[0].Stage)
				assert.Contains(t, result.Failures[0].Detail, step.want)
			})
		}
	}
}

func TestAWSAMIResolveCallsNothingAtLayer2OrForAnotherCloud(t *testing.T) {
	for name, tc := range map[string]struct {
		layer3 bool
		cloud  string
	}{
		"layer 2 aws": {},
		"layer 3 gcp": {layer3: true, cloud: "gcp"},
	} {
		for _, command := range []string{"generate", "run", "test"} {
			t.Run(name+"/"+command, func(t *testing.T) {
				lc := newAWSLifecycle(t)

				run := awsCommand{name: command, layer3: tc.layer3, cloud: tc.cloud}.run(t, lc)

				for _, call := range []string{"sts:GetCallerIdentity", getAMI, describeAMI} {
					assert.NotContains(t, lc.log(), call, run.output)
				}
				if tc.cloud != "" || command == "test" {
					return
				}
				require.NotEmpty(t, run.amis, run.output)
				assert.Equal(t, harness.AWSLayer2AMI, run.amis[0])
			})
		}
	}
}

// buildRuntime gives a nil AWS dependency newAWSHTTPClient's client,
// which TestMain makes panic: a test that forgets its fake fails rather
// than calling AWS.
func TestAnAWSDependencyLeftNilFailsTheTestInsteadOfCallingAWS(t *testing.T) {
	for name, unset := range map[string]func(*RuntimeDependencies){
		"AWSSTS": func(d *RuntimeDependencies) { d.AWSSTS = nil },
		"AWSSSM": func(d *RuntimeDependencies) { d.AWSSSM = nil },
		"AWSEC2": func(d *RuntimeDependencies) { d.AWSEC2 = nil },
	} {
		t.Run(name, func(t *testing.T) {
			h := newCommandTestHarness(t)
			lc := newAWSLifecycle(t)
			opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
				cfg = layer3On(cfg)
				cfg.AWS = config.AWSConfig{Region: "eu-west-2", AccountID: preflightAWSAccount, PrincipalARN: preflightAWSPrincipal}
				return cfg
			})
			opts.deps = RuntimeDependencies{
				Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
					return nil, errors.New("not called")
				}),
				AWSSTS: lc, AWSSSM: lc, AWSEC2: lc,
			}
			unset(&opts.deps)
			cmd := newGenerateCommandForTest(opts)
			require.NoError(t, cmd.Flags().Set("config", h.ConfigPath))
			rt, err := buildRuntime(cmd, opts)
			require.NoError(t, err)

			assert.PanicsWithValue(t, realAWSRefusedPanic, func() {
				_, _ = resolveAWSLayer3AMI(context.Background(), rt, layer3AWS)
			})
		})
	}
}
