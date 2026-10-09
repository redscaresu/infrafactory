package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/harness"
)

const (
	// orderAMI is the AL2023 id the doer serves: not the fixture's, so a
	// stack that does not carry the resolved id fails the gate.
	orderAMI = "ami-0123456789abcdef0"

	probeHTTP    = "RealProbe http_probe"
	probeHoldout = "RealProbe holdout"
	describeVPCs = "ec2:DescribeVpcs"
	userDataRead = "ec2:DescribeInstanceAttribute"

	orderHoldout = `scenario: example-scenario-unseen
type: holdout
references: example-scenario
version: "1.0"
cloud: aws
description: a criterion the generator never saw
acceptance_criteria:
  - type: connectivity
    from: public_internet
    to: compute
    port: 22
    expect: blocked
`
)

// orderStage is one forward stage of an aws Layer 3 run, from the gate
// on, and the call that marks it in the log. account_check reads only
// the state, so it has none: its place shows by what its failure stops.
type orderStage struct {
	name, call string
}

var orderForward = []orderStage{
	{"gate (generation)", gateCall},
	{"gate (test)", gateCall},
	{"sts", "sts:GetCallerIdentity"},
	{"stamp", getStamp},
	{"default vpc", describeVPCs},
	{"claim", putClaim},
	{"apply", deployRun},
	{"account_check", ""},
	{"user_data_check", userDataRead},
	{"http_probe", probeHTTP},
	{"holdout", probeHoldout},
}

// orderProbe is the real probe: it logs the scenario's http_probe and the
// holdout's connectivity check apart, and fails the one named by fail. A
// call is the holdout's when it carries the holdout's own port 22 check,
// so how many checks the scenario's probe gets cannot move it.
type orderProbe struct {
	lc   *awsLifecycle
	fail string
}

func (p orderProbe) Run(_ context.Context, _, _ string, checks []harness.ProbeCheck) (*harness.RealProbeResult, error) {
	call := probeHTTP
	if slices.ContainsFunc(checks, func(c harness.ProbeCheck) bool { return c.Type == "connectivity" && c.Port == 22 }) {
		call = probeHoldout
	}
	p.lc.record(call)
	if call == p.fail {
		return nil, errors.New(call + " failed")
	}
	return &harness.RealProbeResult{}, nil
}

// orderSTS is the doer with its nth sts:GetCallerIdentity, counted from
// 1, answered AccessDenied; 0 denies none.
type orderSTS struct {
	*awsLifecycle
	failOn int
	calls  *int
}

func (s orderSTS) Do(req *http.Request) (*http.Response, error) {
	if !strings.HasPrefix(req.URL.Host, "sts.") {
		return s.awsLifecycle.Do(req)
	}
	if *s.calls++; *s.calls != s.failOn {
		return s.awsLifecycle.Do(req)
	}
	s.record("sts:GetCallerIdentity")
	return lifecycleAnswer(req, http.StatusForbidden, "text/xml",
		`<ErrorResponse><Error><Type>Sender</Type><Code>AccessDenied</Code><Message>denied</Message></Error>`+
			`<RequestId>00000000-0000-0000-0000-000000000000</RequestId></ErrorResponse>`), nil
}

// orderSetup is what one ordered run varies: the nth gate call or STS
// call to refuse, and the probe to fail.
type orderSetup struct {
	gateFailsOn, stsFailsOn int
	probeFails              string
}

// runAWSOrder is one `run --holdout` of the aws-web-step-one fixture
// against lc: the real gate behind a logging wrapper, the generator
// returning the raw fixture with the id the doer served, a probe, and a
// holdout, with one iteration.
func runAWSOrder(t *testing.T, lc *awsLifecycle, setup orderSetup) awsRun {
	t.Helper()
	lc.params[harness.AWSAL2023AMIParameter] = orderAMI
	lc.amiImage = strings.Replace(lc.amiImage, harness.AWSLayer2AMI, orderAMI, 1)
	fixture, err := os.ReadFile(filepath.Join(awsStepOneFixtures, "web-step-one.tf"))
	require.NoError(t, err)
	require.Contains(t, string(fixture), harness.AWSLayer2AMI)

	// The script the apply sees is infrafactory's, rendered from the
	// scenario's service, and a file of its own.
	apply := lc.deploy.onRunDir
	lc.deploy.onRunDir = func(dir string) {
		path := filepath.Join(dir, generator.AWSUserDataFile)
		if info, err := os.Lstat(path); assert.NoError(t, err) {
			assert.True(t, info.Mode().IsRegular(), "%s is %s", path, info.Mode())
		}
		got, err := os.ReadFile(path)
		assert.NoError(t, err)
		want, err := renderAWSUserData(awsLifecycleService)
		require.NoError(t, err)
		assert.Equal(t, string(want), string(got))
		apply(dir)
	}

	gateCalls, stsCalls := 0, 0
	var scenarios string
	return runAWSRun(t, lc, awsRunOptions{
		repairs: 1,
		flags:   []string{"--holdout"},
		scenario: func(h *CommandTestHarness) {
			raw, err := os.ReadFile(h.ScenarioPath)
			require.NoError(t, err)
			var doc map[string]any
			require.NoError(t, yaml.Unmarshal(raw, &doc))
			require.Contains(t, doc, "service")
			doc["acceptance_criteria"] = []map[string]any{
				{"type": "http_probe", "target": "compute", "port": 80, "expect": "reachable"},
			}
			raw, err = yaml.Marshal(doc)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(h.ScenarioPath, raw, 0o600))
			writeHoldout(t, h, orderHoldout)
			scenarios = filepath.Join(h.WorkspaceDir, "scenarios")
		},
		customize: func(cfg *config.Config) {
			cfg.AWS.Region = awsAdmittedRegion
			cfg.Paths.Scenarios = scenarios
		},
		deps: func(d *RuntimeDependencies) {
			d.Generator = generator.SeedGeneratorFunc(func(_ context.Context, req generator.Request) (*generator.GeneratedCode, error) {
				lc.record(generated)
				main := bytes.ReplaceAll(fixture, []byte(harness.AWSLayer2AMI), []byte(req.AMIID))
				return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": main}}, nil
			})
			d.Layer3HCLGate = func(cloud layer3Cloud, dir string, in awsGateInputs) error {
				lc.record(gateCall)
				if gateCalls++; gateCalls == setup.gateFailsOn {
					return errors.New("refused by the gate's fake")
				}
				return layer3PreflightHCLForCloud(cloud, dir, in)
			}
			d.AWSSTS = orderSTS{awsLifecycle: lc, failOn: setup.stsFailsOn, calls: &stsCalls}
			// A result, as the real mock deploy returns: with none,
			// executeTest evaluates no criterion at all.
			d.MockDeploy = &fakeMockDeployHarness{result: &harness.MockDeployResult{}}
			d.RealProbe = orderProbe{lc: lc, fail: setup.probeFails}
		},
	})
}

// orderResolve is the run's one AMI resolve and generation, before any
// forward stage.
var orderResolve = []string{"sts:GetCallerIdentity", getAMI, "ec2:DescribeImages " + orderAMI, generated}

func TestAWSRunWalksTheLayer3StagesInOrder(t *testing.T) {
	lc := newAWSLifecycle(t)

	run := runAWSOrder(t, lc, orderSetup{})

	require.NoError(t, run.err, run.output)
	assert.Equal(t, "target_reached", run.terminalReason())
	calls := lc.log()
	want := slices.Clone(orderResolve)
	for _, s := range orderForward {
		if s.call != "" {
			want = append(want, s.call)
		}
	}
	want = append(want, destroyRun)
	require.Greater(t, len(calls), len(want)+2, "%q", calls)
	assert.Equal(t, want, calls[:len(want)], "every forward stage, in order, then the destroy")

	// Then the sweep, then the release: read the claim, then delete it.
	sweep, release := calls[len(want):len(calls)-2], calls[len(calls)-2:]
	for _, c := range sweep {
		assert.Contains(t, c, ":Describe", "only the sweep runs between the destroy and the release: %q", sweep)
	}
	assert.Contains(t, sweep, "ec2:DescribeInstances")
	assert.Contains(t, sweep, "ssm:DescribeParameters")
	assert.Equal(t, []string{getClaim, deleteClaim}, release)
}

// otherAccountState is awsLiveState with its instance in another account.
var otherAccountState = strings.Replace(awsLiveState, preflightAWSAccount, "999999999999", 1)

// Each forward stage, failed by its fake, ends the iteration there. From
// the apply on, the teardown still destroys and sweeps, and releases the
// claim only after a clean sweep; a dirty one keeps it and names reap.
// Each row names the failure its fake causes, so a run that ends at the
// right stage for another reason fails the row.
func TestAWSRunEndsAtTheLayer3StageThatFails(t *testing.T) {
	const testStage = "iteration_1_test"
	rows := map[string]struct {
		lc    func(*testing.T, *awsLifecycle)
		setup orderSetup
		// fails is the failure the run reports: its Stage and Check
		// exactly, and its Detail as a substring.
		fails FailureSummary
	}{
		"gate (generation)": {
			setup: orderSetup{gateFailsOn: 1},
			fails: FailureSummary{Stage: "iteration_1_generate", Check: "generate", Detail: "refused by the gate's fake"},
		},
		"gate (test)": {
			setup: orderSetup{gateFailsOn: 2},
			fails: FailureSummary{Stage: testStage, Check: "allow_resource_types", Detail: "refused by the gate's fake"},
		},
		"sts": {
			setup: orderSetup{stsFailsOn: 2},
			fails: FailureSummary{Stage: testStage, Check: "credentials", Detail: "AccessDenied"},
		},
		"stamp": {
			lc:    func(_ *testing.T, lc *awsLifecycle) { delete(lc.params, harness.AWSStampParameter) },
			fails: FailureSummary{Stage: testStage, Check: "stamp", Detail: harness.AWSStampParameter + " does not exist"},
		},
		"default vpc": {
			lc: func(_ *testing.T, lc *awsLifecycle) {
				lc.ec2["DescribeVpcs"] = `<vpcSet><item><vpcId>vpc-0default</vpcId><isDefault>true</isDefault></item></vpcSet>`
			},
			fails: FailureSummary{Stage: testStage, Check: "default_vpc", Detail: "vpc-0default is the region's default VPC"},
		},
		"claim": {
			lc:    func(_ *testing.T, lc *awsLifecycle) { lc.params[harness.AWSClaimParameter] = lifecycleOtherHolder },
			fails: FailureSummary{Stage: testStage, Check: "claim", Detail: "claimed by \"" + lifecycleOtherHolder + "\""},
		},
		"apply": {
			lc:    func(_ *testing.T, lc *awsLifecycle) { lc.deploy.err = errors.New("tofu apply failed") },
			fails: FailureSummary{Stage: testStage, Detail: "tofu apply failed"},
		},
		"account_check": {
			lc: func(t *testing.T, lc *awsLifecycle) {
				lc.deploy.onRunDir = func(dir string) {
					lc.record(deployRun)
					require.NoError(t, os.WriteFile(filepath.Join(dir, harness.LiveStateFilename), []byte(otherAccountState), 0o600))
				}
			},
			fails: FailureSummary{Stage: testStage, Check: "account_check", Detail: `in account "999999999999"`},
		},
		"user_data_check": {
			lc: func(_ *testing.T, lc *awsLifecycle) {
				lc.ec2["DescribeInstanceAttribute"] = `<instanceId>i-0abc</instanceId><userData><value>` +
					base64.StdEncoding.EncodeToString([]byte("#!/bin/bash\n")) + `</value></userData>`
			},
			fails: FailureSummary{Stage: testStage, Check: "user_data_check", Detail: "does not run the rendered user data"},
		},
		"http_probe": {
			setup: orderSetup{probeFails: probeHTTP},
			fails: FailureSummary{Stage: testStage, Check: "real_probe", Detail: probeHTTP + " failed"},
		},
		"holdout": {
			setup: orderSetup{probeFails: probeHoldout},
			fails: FailureSummary{Stage: testStage, Check: "holdout", Detail: probeHoldout + " failed"},
		},
	}
	applyAt := slices.IndexFunc(orderForward, func(s orderStage) bool { return s.name == "apply" })
	for k, stage := range orderForward {
		row, ok := rows[stage.name]
		require.True(t, ok, "a row for %s", stage.name)
		scopes := []string{""}
		if k >= applyAt {
			scopes = []string{"clean scope", "dirty scope"}
		}
		for _, scope := range scopes {
			t.Run(strings.TrimSuffix(stage.name+"/"+scope, "/"), func(t *testing.T) {
				lc := newAWSLifecycle(t)
				if row.lc != nil {
					row.lc(t, lc)
				}
				if scope == "dirty scope" {
					dirtySweep(lc)
				}

				run := runAWSOrder(t, lc, row.setup)

				require.Error(t, run.err)
				assert.Equal(t, 1, lc.count(generated), "one iteration")
				reached := slices.Clone(orderResolve)
				for _, s := range orderForward[:k+1] {
					reached = append(reached, s.call)
				}
				// Every forward call up to this stage ran as often as the
				// happy path makes it, so the row got here; none after it
				// ran at all, so it went no further. The sweep repeats
				// DescribeVpcs, so the reached stages count before the
				// destroy.
				calls := lc.log()
				forward := calls
				if destroy := slices.Index(calls, destroyRun); destroy != -1 {
					forward = calls[:destroy]
				}
				for j, other := range orderForward {
					if other.call == "" {
						continue
					}
					in := forward
					if j > k {
						in = calls
					}
					assert.Equal(t, countOf(reached, other.call), countOf(in, other.call),
						"%s when %s fails: %q", other.name, stage.name, calls)
				}
				want := row.fails
				assert.True(t, slices.ContainsFunc(run.result.Failures, func(f FailureSummary) bool {
					return f.Stage == want.Stage && f.Check == want.Check && strings.Contains(f.Detail, want.Detail)
				}), "the run fails on %s with %+v: %+v", stage.name, want, run.result.Failures)
				if k < applyAt {
					return
				}

				destroy := slices.Index(calls, destroyRun)
				require.NotEqual(t, -1, destroy, "the teardown destroys: %q", calls)
				assert.Less(t, slices.Index(calls, deployRun), destroy, "%q", calls)
				assert.Contains(t, calls[destroy:], "ec2:DescribeInstances", "then sweeps")
				assert.Contains(t, calls[destroy:], "ssm:DescribeParameters", "then sweeps")
				reap := reapCommand(run.h.ConfigPath, run.h.ScenarioPath)
				_, held := lc.claim()
				if scope == "clean scope" {
					assert.False(t, held, "released after the clean sweep")
					assert.Positive(t, lc.count(deleteClaim))
					assert.NotContains(t, run.output, reap, "nothing is left to reap")
					return
				}
				assert.True(t, held, "kept after the dirty sweep")
				assert.Zero(t, lc.count(deleteClaim))
				assert.Contains(t, run.output, reap)
			})
		}
	}
}

func countOf(calls []string, call string) int {
	n := 0
	for _, c := range calls {
		if c == call {
			n++
		}
	}
	return n
}
