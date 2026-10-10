package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/harness"
)

// Test names here avoid "Scaleway": t.TempDir() embeds the test name in
// the state path every refusal quotes, and these tests assert the word
// is absent.

// awsLiveState is an AWS stack that really applied. The marker beside it
// in these fixtures is a stale Scaleway one from an earlier run of the
// same scenario: acting on it would reap a project the AWS run never
// touched.
const awsLiveState = `{"resources":[{"type":"aws_instance","instances":[{"attributes":{"id":"i-0abc","owner_id":"123456789012","public_ip":"203.0.113.10"}}]}]}`

const (
	staleMarkerProjectID = "11111111-1111-1111-1111-111111111111"
	staleMarker          = `{"project_id":"` + staleMarkerProjectID + `","name":"if-run-stale"}`
)

var nonTeardownClouds = []string{"aws", "gcp"}

func writeAWSStateAndStaleMarker(t *testing.T, dir string, withState bool) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, harness.RunProjectMarkerFilename), []byte(staleMarker), 0o600))
	if withState {
		require.NoError(t, os.WriteFile(filepath.Join(dir, harness.LiveStateFilename), []byte(awsLiveState), 0o600))
	}
}

func assertNoScalewayAdvice(t *testing.T, text, where string) {
	t.Helper()
	assert.NotContains(t, text, "Scaleway", where)
	assert.NotContains(t, text, "infrafactory reap", where)
}

// sealedHandler runs next with its logs captured and no provider-schema
// fetch, which would run `tofu init` for the scenario's cloud.
func sealedHandler(logs io.Writer, next runtimeHandler) runtimeHandler {
	return func(cmd *cobra.Command, args []string, rt *CommandRuntime) error {
		rt.schemaRunner = nil
		rt.Logger = NewAppLogger(logs)
		return next(cmd, args, rt)
	}
}

func setScenarioCloud(t *testing.T, path, cloud string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, []byte(withCloud(t, string(raw), cloud)), 0o600))
}

// layer3On turns Layer 3 on, with one repair so a failing run ends quickly.
func layer3On(cfg config.Config) config.Config {
	cfg.Validation.Layers.SandboxDeploy.Enabled = true
	cfg.Agent.RepairIterationsMax = 1
	return cfg
}

func TestFailedRunOnAnotherCloudTearsNothingDown(t *testing.T) {
	for _, cloud := range nonTeardownClouds {
		t.Run(cloud, func(t *testing.T) {
			h := newCommandTestHarness(t)
			sandboxCredsForTest(t)
			setScenarioCloud(t, h.ScenarioPath, cloud)

			fakes := newLayer3Fakes()
			customize := layer3On
			var lc *awsLifecycle
			if cloud == "aws" {
				// A complete aws block and doers that answer, so zero
				// calls is the arm's choice rather than a missing config.
				lc = newAWSLifecycle(t)
				customize = func(cfg config.Config) config.Config {
					cfg = layer3On(cfg)
					cfg.AWS = config.AWSConfig{Region: "eu-west-2", AccountID: preflightAWSAccount, PrincipalARN: preflightAWSPrincipal}
					return cfg
				}
			}
			opts := isolatedRunOpts(h, customize)
			opts.deps = RuntimeDependencies{
				// generateAndWriteFiles persists these before the gate
				// refuses the cloud, so the failure path finds them.
				Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
					return &generator.GeneratedCode{Files: map[string][]byte{
						"main.tf":                        []byte("terraform {}\n"),
						harness.LiveStateFilename:        []byte(awsLiveState),
						harness.RunProjectMarkerFilename: []byte(staleMarker),
					}}, nil
				}),
				Static:     &fakeStaticHarness{err: errors.New("static must not be reached")},
				MockDeploy: &fakeMockDeployHarness{},
				Destroy:    &fakeDestroyHarness{},
				MockState:  &fakeRunMockStateClient{statePayload: []byte(`{"instance":{"servers":[]}}`)},
			}
			fakes.install(&opts.deps)
			if lc != nil {
				opts.deps.AWSSTS, opts.deps.AWSSSM, opts.deps.AWSEC2 = lc, lc, lc
			}

			logs := &bytes.Buffer{}
			cmd := newRunCommandForTest(opts)
			cmd.RunE = withRuntimeWithOptions("run", opts, sealedHandler(logs, runRunCommand))
			stdout := &bytes.Buffer{}
			cmd.SetOut(stdout)
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath, "--output", string(OutputModeJSON)})
			require.Error(t, cmd.Execute())

			result := decodeMachineOutput(t, stdout)
			// aws acts only on a claim an iteration took, and the gate
			// refused before any take: the only AWS calls are the AMI
			// resolve's, which got aws past generation to the gate.
			if cloud == "aws" {
				assert.Equal(t, []string{"sts:GetCallerIdentity", getAMI, describeAMI}, lc.log(), "AWS calls")
				assert.Contains(t, result.Stages, StageSummary{
					Layer: "sandbox_deploy", Stage: "auto_destroy", Status: StageStatusSkip,
					Detail: "no iteration took the aws scope's claim, so none applied to it",
				})
			} else {
				statePath := filepath.Join(h.OutputDir(), "example-scenario", harness.LiveStateFilename)
				want := layer3TeardownNotBuilt(layer3Cloud(cloud), statePath)
				assert.Contains(t, result.Failures, FailureSummary{
					Layer: "sandbox_deploy", Stage: "auto_destroy_preflight", Check: "cloud",
					Command: "auto-destroy preflight", Detail: want,
				})
				assert.Contains(t, want, cloud)
				assert.Contains(t, logs.String(), `"event":"layer3_auto_destroy"`)
			}
			assert.Contains(t, result.Stages, StageSummary{
				Layer: "live", Stage: "stray_run_projects", Status: StageStatusSkip,
				Detail: "cloud " + cloud + " has no run projects to check",
			})
			assertNoScalewayAdvice(t, stdout.String(), "output")
			assertNoScalewayAdvice(t, logs.String(), "logs")
			fakes.assertUntouched(t)
		})
	}
}

// aws has a reap arm, which never reads the marker either:
// aws_reap_command_test.go.
func TestReapOnAnotherCloudRefusesBeforeReadingTheMarker(t *testing.T) {
	for _, cloud := range nonTeardownClouds {
		if cloud == "aws" {
			continue
		}
		t.Run(cloud, func(t *testing.T) {
			h := newCommandTestHarness(t)
			sandboxCredsForTest(t)
			setScenarioCloud(t, h.ScenarioPath, cloud)
			outDir := filepath.Join(h.OutputDir(), "example-scenario")
			writeAWSStateAndStaleMarker(t, outDir, true)

			cfg, err := config.Load(h.ConfigPath)
			require.NoError(t, err)
			cfg.Paths.Output = h.OutputDir()
			logs := &bytes.Buffer{}
			fakes := newLayer3Fakes()
			rt := &CommandRuntime{Config: layer3On(cfg), scenarioLoader: defaultScenarioLoader, Logger: NewAppLogger(logs)}
			fakes.install(&rt.Deps)

			out := &strings.Builder{}
			err = runReap(t, rt, h.ScenarioPath, out)
			require.Error(t, err)
			assert.Contains(t, err.Error(),
				layer3TeardownNotBuilt(layer3Cloud(cloud), filepath.Join(outDir, harness.LiveStateFilename)))
			assertNoScalewayAdvice(t, err.Error(), "error")
			assertNoScalewayAdvice(t, out.String(), "output")
			assertNoScalewayAdvice(t, logs.String(), "logs")
			fakes.assertUntouched(t)
		})
	}
}

// midRunListenWait bounds midRunSignal's wait for the guard, so a guard
// that never listens again fails its test rather than hanging it.
const midRunListenWait = 5 * time.Second

// midRunSignal is a stdout that sends one signal as the run writes its
// result, and holds that write until the guard listens for the next
// signal, which it does only once it has ended the run's span. So the
// run is still in flight when it sees its interrupt, however the
// watcher goroutine is scheduled; a signal already sent before the run
// starts can lose that race to a run that never waits on its context.
type midRunSignal struct {
	bytes.Buffer
	mu        sync.Mutex
	signal    context.CancelFunc // the first registration's
	listening chan struct{}      // closed at the second registration
}

func newMidRunSignal() *midRunSignal { return &midRunSignal{listening: make(chan struct{})} }

func (s *midRunSignal) notify(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case s.signal == nil:
		s.signal = cancel
	case s.listening != nil:
		close(s.listening)
		s.listening = nil
	}
	return ctx, cancel
}

func (s *midRunSignal) Write(p []byte) (int, error) {
	s.mu.Lock()
	signal, listening := s.signal, s.listening
	s.mu.Unlock()
	if signal != nil && listening != nil {
		signal()
		select {
		case <-listening:
		case <-time.After(midRunListenWait):
		}
	}
	return s.Buffer.Write(p)
}

// aws has an interrupt arm, which keeps the claim and names reap:
// aws_reap_command_test.go.
func TestInterruptedTestOnAnotherCloudTearsNothingDown(t *testing.T) {
	for _, cloud := range nonTeardownClouds {
		if cloud == "aws" {
			continue
		}
		for _, withState := range []bool{true, false} {
			name := cloud + "/marker only"
			if withState {
				name = cloud + "/state and marker"
			}
			t.Run(name, func(t *testing.T) {
				h := newCommandTestHarness(t)
				sandboxCredsForTest(t)
				setScenarioCloud(t, h.ScenarioPath, cloud)
				outDir := filepath.Join(h.OutputDir(), "example-scenario")
				writeAWSStateAndStaleMarker(t, outDir, withState)

				fakes := newLayer3Fakes()
				mock := &fakeMockDeployHarness{result: &harness.MockDeployResult{StateSnapshot: []byte(`{}`)}}
				opts := isolatedRunOpts(h, layer3On)
				opts.deps = RuntimeDependencies{MockDeploy: mock, Destroy: &fakeDestroyHarness{}}
				fakes.install(&opts.deps)

				logs := &bytes.Buffer{}
				stdout, stderr := newMidRunSignal(), &bytes.Buffer{}
				cmd := newTestCommandForTest(opts)
				cmd.RunE = withRuntimeWithOptions("test", opts, sealedHandler(logs,
					func(cmd *cobra.Command, args []string, rt *CommandRuntime) error {
						return runTestWithNotify(cmd, args, rt, stdout.notify)
					}))
				cmd.SetOut(stdout)
				cmd.SetErr(stderr)
				cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath, "--output", string(OutputModeJSON)})
				require.Error(t, cmd.Execute(), "the gate refuses the cloud")

				assert.Contains(t, stderr.String(),
					layer3TeardownNotBuilt(layer3Cloud(cloud), filepath.Join(outDir, harness.LiveStateFilename)))
				assertNoScalewayAdvice(t, stderr.String(), "stderr")
				assertNoScalewayAdvice(t, stdout.String(), "stdout")
				assertNoScalewayAdvice(t, logs.String(), "logs")
				assert.Zero(t, mock.calls)
				fakes.assertUntouched(t)
			})
		}
	}
}

// teardownSeams calls every Layer 3 seam with cloud against workDir and
// returns what each said.
func teardownSeams(rt *CommandRuntime, cloud layer3Cloud, workDir string) map[string]string {
	ctx := context.Background()
	env := map[string]string{"SCW_SECRET_KEY": "real-secret", "SCW_DEFAULT_ORGANIZATION_ID": reapOrgID}
	errText := func(err error) string {
		if err == nil {
			return ""
		}
		return err.Error()
	}
	details := func(_ []StageSummary, failures []FailureSummary) string {
		var b strings.Builder
		for _, f := range failures {
			b.WriteString(f.Detail + "\n")
		}
		return b.String()
	}
	_, envErr := sandboxCommandEnvForProject(rt, cloud, staleMarkerProjectID)
	_, _, destroyErr := destroySandbox(ctx, rt, cloud, workDir, env, staleMarkerProjectID)
	_, ensureStages, ensureFailures := ensureRunProject(ctx, rt, cloud, "stale", workDir)
	guardOut := &strings.Builder{}
	_ = withSandboxInterruptGuard(guardCmd(guardOut), rt, cloud, cancelledNotify(), untilInterrupted)

	return map[string]string{
		"assertSandboxCredentials":    errText(assertSandboxCredentials(rt, cloud)),
		"sandboxCommandEnvForProject": errText(envErr),
		"ensureRunProject":            details(ensureStages, ensureFailures),
		"releaseRunProject":           details(releaseRunProject(ctx, rt, cloud, workDir, staleMarkerProjectID, env)),
		"assertRunProjectDeletable":   errText(assertRunProjectDeletable(ctx, rt, cloud, workDir, staleMarkerProjectID, env)),
		"destroySandbox":              errText(destroyErr),
		"appendOrphanSweepResult": details(appendOrphanSweepResult(ctx, nil, nil, rt, cloud,
			&harness.SweepTarget{ProjectID: staleMarkerProjectID}, nil, env)),
		"reportStrayRunProjects":    details(reportStrayRunProjects(ctx, rt, cloud)),
		"withSandboxInterruptGuard": guardOut.String(),
	}
}

func seamFixture(t *testing.T) (*CommandRuntime, layer3Fakes, string) {
	t.Helper()
	sandboxCredsForTest(t)
	workDir := t.TempDir()
	writeAWSStateAndStaleMarker(t, workDir, true)
	fakes := newLayer3Fakes()
	rt := &CommandRuntime{Config: layer3On(config.Default()), outputDir: workDir}
	fakes.install(&rt.Deps)
	return rt, fakes, workDir
}

// The gate refuses aws before `test` reaches any of these, so each seam
// is called directly as well.
func TestEveryTeardownSeamRefusesAnotherCloud(t *testing.T) {
	for _, cloud := range nonTeardownClouds {
		t.Run(cloud, func(t *testing.T) {
			rt, fakes, workDir := seamFixture(t)
			for seam, said := range teardownSeams(rt, layer3Cloud(cloud), workDir) {
				// aws has a destroy arm: aws_destroy_arm_test.go.
				if cloud == "aws" && seam == "destroySandbox" {
					continue
				}
				// aws has a claim arm: aws_scope_lifecycle_test.go. With
				// no aws block it refuses before building an env.
				if cloud == "aws" && seam == "ensureRunProject" {
					assert.Contains(t, said, "aws.region is empty in the config", seam)
					assertNoScalewayAdvice(t, said, seam)
					continue
				}
				// aws has an interrupt arm: aws_reap_command_test.go. It
				// names reap, never Scaleway's advice.
				if cloud == "aws" && seam == "withSandboxInterruptGuard" {
					assert.Contains(t, said, "infrafactory reap", seam)
					assert.NotContains(t, said, "Scaleway", seam)
					assert.NotContains(t, said, "nothing to clean up", seam)
					continue
				}
				assert.Contains(t, said, cloud, seam)
				assertNoScalewayAdvice(t, said, seam)
			}
			fakes.assertUntouched(t)
		})
	}

	// The premise: Scaleway's arms act on the same fixture, so the zero
	// calls above are the cloud's doing.
	t.Run("scaleway reaches its dependencies", func(t *testing.T) {
		rt, fakes, workDir := seamFixture(t)
		teardownSeams(rt, layer3Scaleway, workDir)
		assert.NotZero(t, fakes.runProject.calls, "RunProject")
		assert.NotZero(t, fakes.destroy.calls, "SandboxDestroy")
		assert.NotZero(t, fakes.sweep.calls, "OrphanSweep")
	})
}
