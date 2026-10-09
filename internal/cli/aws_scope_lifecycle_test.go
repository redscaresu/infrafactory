package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/redscaresu/infrafactory/internal/scenario"
)

const (
	lifecycleOtherHolder = "run-0@other-host.example:99"

	putClaim    = "ssm:PutParameter " + harness.AWSClaimParameter
	deleteClaim = "ssm:DeleteParameter " + harness.AWSClaimParameter
	deployRun   = "SandboxDeploy.Run"
	destroyRun  = "SandboxDestroy.Run"
	getAMI      = "ssm:GetParameter " + harness.AWSAL2023AMIParameter
	describeAMI = "ec2:DescribeImages " + harness.AWSLayer2AMI
	gateCall    = "Layer3HCLGate"
	generated   = "Generator.Generate"

	runningInstance = `<reservationSet><item><instancesSet><item><instanceId>i-0stray</instanceId>` +
		`<instanceState><name>running</name></instanceState></item></instancesSet></item></reservationSet>`
	settlingInstance = `<reservationSet><item><instancesSet><item><instanceId>i-0settling</instanceId>` +
		`<instanceState><name>shutting-down</name></instanceState></item></instancesSet></item></reservationSet>`

	// awsAMIImage is the AL2023 image the SSM parameter names, with
	// awsAdmittedInputs' root: 8 GiB gp3, deleted on termination.
	awsAMIImage = `<imagesSet><item><imageId>` + harness.AWSLayer2AMI + `</imageId>` +
		`<rootDeviceType>ebs</rootDeviceType><rootDeviceName>/dev/xvda</rootDeviceName><blockDeviceMapping>` +
		`<item><deviceName>/dev/xvda</deviceName><ebs><volumeSize>8</volumeSize><volumeType>gp3</volumeType>` +
		`<deleteOnTermination>true</deleteOnTermination></ebs></item></blockDeviceMapping></item></imagesSet>`
)

// awsLifecycle is STS, SSM and EC2 behind one doer, with the Layer 3
// harness fakes, all logging to one ordered call log. A request whose
// context is done fails as a real client's would.
type awsLifecycle struct {
	mu     sync.Mutex
	calls  []string
	params map[string]string
	ec2    map[string]string // a Describe's result set, empty when absent
	// denied answers 403 to an EC2 Describe by its action, and to an
	// SSM call by its logged name.
	denied map[string]bool
	// amiImage is DescribeImages' answer for an image id, the AMI
	// resolve's call, which the sweep's Owners=self listing never gets.
	amiImage string
	putFail  bool // PutParameter, and every claim read after it, is denied
	putSent  bool
	// runHolder is the holder the run's claim PutParameter sent, taken or
	// not; reap's own take is not it.
	runHolder string
	onPut     func()
	// onEC2 sees each EC2 action with mu held, so it may change ec2.
	onEC2  func(action string)
	sleeps int
	cancel context.CancelFunc
	// credFile is the credential file under the test's HOME.
	credFile string

	deploy  *fakeSandboxDeployHarness
	destroy *loggingSandboxDestroy
	// scw is every Scaleway Layer 3 dependency, none of which aws may call.
	scw layer3Fakes
}

func newAWSLifecycle(t *testing.T) *awsLifecycle {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	credDir := filepath.Join(home, ".config", "infrafactory")
	require.NoError(t, os.MkdirAll(credDir, 0o700))
	lc := &awsLifecycle{
		params: map[string]string{
			harness.AWSStampParameter:     preflightAWSAccount,
			harness.AWSAL2023AMIParameter: harness.AWSLayer2AMI,
		},
		ec2:      map[string]string{},
		denied:   map[string]bool{},
		amiImage: awsAMIImage,
		credFile: filepath.Join(credDir, "layer3-aws.env"),
		scw:      newLayer3Fakes(),
	}
	require.NoError(t, os.WriteFile(lc.credFile,
		[]byte("AWS_ACCESS_KEY_ID="+preflightAWSKeyID+"\nAWS_SECRET_ACCESS_KEY="+preflightAWSSecret+"\n"), 0o600))
	lc.deploy = &fakeSandboxDeployHarness{onRunDir: func(dir string) {
		lc.record(deployRun)
		require.NoError(t, os.WriteFile(filepath.Join(dir, harness.LiveStateFilename), []byte(awsLiveState), 0o600))
	}}
	lc.destroy = &loggingSandboxDestroy{lc: lc}
	// The instance awsLiveState names runs the script rendered from
	// awsLifecycleService, so the post-apply user_data_check passes.
	script, err := renderAWSUserData(awsLifecycleService)
	require.NoError(t, err)
	lc.ec2["DescribeInstanceAttribute"] = `<instanceId>i-0abc</instanceId><userData><value>` +
		base64.StdEncoding.EncodeToString(script) + `</value></userData>`
	return lc
}

// awsLifecycleService is the service: block setAWSLifecycleScenario adds.
var awsLifecycleService = scenario.ServiceSpec{Image: "nginx", Tag: "1.27", Port: 80}

// setAWSLifecycleScenario makes the harness scenario an aws one with a
// service, so the gate has a script to check and EC2 one to report.
func setAWSLifecycleScenario(t *testing.T, path string) {
	t.Helper()
	setScenarioCloud(t, path, "aws")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	raw = append(raw, "service:\n  image: nginx\n  tag: \"1.27\"\n  port: 80\n  ttl: 4h\n"...)
	require.NoError(t, os.WriteFile(path, raw, 0o600))
}

func (lc *awsLifecycle) record(call string) {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	lc.calls = append(lc.calls, call)
}

func (lc *awsLifecycle) log() []string {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	return slices.Clone(lc.calls)
}

func (lc *awsLifecycle) count(call string) int {
	n := 0
	for _, c := range lc.log() {
		if c == call {
			n++
		}
	}
	return n
}

// takeOver is the command a kept claim names: reap, taking the claim
// over from the run's holder.
func (lc *awsLifecycle) takeOver(configPath, scenarioPath string) string {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	return reapCommand(configPath, scenarioPath) + " --take-over " + shellQuote(lc.runHolder)
}

func (lc *awsLifecycle) claim() (string, bool) {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	holder, held := lc.params[harness.AWSClaimParameter]
	return holder, held
}

func (lc *awsLifecycle) Do(req *http.Request) (*http.Response, error) {
	if err := req.Context().Err(); err != nil {
		return nil, err
	}
	payload, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	if target := req.Header.Get("X-Amz-Target"); target != "" {
		status, body := lc.ssm(strings.TrimPrefix(target, "AmazonSSM."), payload)
		return lifecycleAnswer(req, status, "application/x-amz-json-1.1", body), nil
	}
	form, err := url.ParseQuery(string(payload))
	if err != nil {
		return nil, err
	}
	action := form.Get("Action")
	if action == "GetCallerIdentity" {
		lc.record("sts:" + action)
		return lifecycleAnswer(req, http.StatusOK, "text/xml", callerIdentityXML(preflightAWSAccount, preflightAWSPrincipal)), nil
	}
	if image := form.Get("ImageId.1"); action == "DescribeImages" && image != "" {
		lc.record("ec2:" + action + " " + image)
		lc.mu.Lock()
		defer lc.mu.Unlock()
		return lifecycleAnswer(req, http.StatusOK, "text/xml",
			`<DescribeImagesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">`+lc.amiImage+`</DescribeImagesResponse>`), nil
	}
	lc.record("ec2:" + action)
	lc.mu.Lock()
	if lc.onEC2 != nil {
		lc.onEC2(action)
	}
	denied, items := lc.denied[action], lc.ec2[action]
	lc.mu.Unlock()
	if denied {
		return lifecycleAnswer(req, http.StatusForbidden, "text/xml",
			`<Response><Errors><Error><Code>UnauthorizedOperation</Code><Message>denied</Message></Error></Errors>`+
				`<RequestID>00000000-0000-0000-0000-000000000000</RequestID></Response>`), nil
	}
	return lifecycleAnswer(req, http.StatusOK, "text/xml",
		`<`+action+`Response xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">`+items+`</`+action+`Response>`), nil
}

func (lc *awsLifecycle) ssm(op string, payload []byte) (int, string) {
	var in struct {
		Name, Value string
		Overwrite   bool
	}
	if err := json.Unmarshal(payload, &in); err != nil {
		return http.StatusBadRequest, `{"__type":"ValidationException","message":"bad json"}`
	}
	call := "ssm:" + op
	if in.Name != "" {
		call += " " + in.Name
	}
	lc.record(call)
	if op == "PutParameter" && lc.onPut != nil {
		lc.onPut()
	}

	lc.mu.Lock()
	defer lc.mu.Unlock()
	value, exists := lc.params[in.Name]
	claimUnreadable := lc.putFail && lc.putSent && in.Name == harness.AWSClaimParameter
	if op == "PutParameter" && in.Name == harness.AWSClaimParameter && !strings.HasPrefix(in.Value, awsReapHolderPrefix) {
		lc.runHolder = in.Value
	}
	switch {
	case lc.denied[call]:
		return lifecycleSSMError("AccessDeniedException")
	case op == "DescribeParameters":
		return http.StatusOK, `{"Parameters":[]}`
	case op == "PutParameter" && lc.putFail:
		lc.putSent = true
		return lifecycleSSMError("AccessDeniedException")
	case claimUnreadable:
		return lifecycleSSMError("AccessDeniedException")
	case op == "PutParameter" && exists && !in.Overwrite:
		return lifecycleSSMError("ParameterAlreadyExists")
	case op == "PutParameter":
		lc.params[in.Name] = in.Value
		return http.StatusOK, `{"Version":1}`
	case !exists:
		return lifecycleSSMError("ParameterNotFound")
	case op == "GetParameter":
		body, _ := json.Marshal(map[string]any{"Parameter": map[string]any{"Name": in.Name, "Type": "String", "Value": value, "Version": 1}})
		return http.StatusOK, string(body)
	case op == "DeleteParameter":
		delete(lc.params, in.Name)
		return http.StatusOK, `{}`
	}
	return http.StatusNotImplemented, `{"__type":"NotImplemented","message":"` + op + `"}`
}

func lifecycleSSMError(code string) (int, string) {
	return http.StatusBadRequest, `{"__type":"` + code + `","message":"` + code + `"}`
}

func lifecycleAnswer(req *http.Request, status int, contentType, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": {contentType}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}

func (lc *awsLifecycle) notify(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	lc.cancel = cancel
	return ctx, cancel
}

type loggingSandboxDestroy struct {
	lc    *awsLifecycle
	err   error
	calls int
}

func (d *loggingSandboxDestroy) Run(context.Context, string, map[string]string) (*harness.SandboxDestroyResult, error) {
	d.calls++
	d.lc.record(destroyRun)
	if d.err != nil {
		return nil, d.err
	}
	return &harness.SandboxDestroyResult{Destroy: harness.StageResult{Stage: "destroy"}}, nil
}

func (d *loggingSandboxDestroy) RunWithoutConfig(context.Context, string, string, map[string]string) (*harness.SandboxDestroyResult, error) {
	d.lc.record("SandboxDestroy.RunWithoutConfig")
	return nil, errors.New("aws has no without-config destroy")
}

// mockDestroyHook is the Layer 2 destroy, which runs between the Layer 3
// apply and its teardown.
type mockDestroyHook func()

func (h mockDestroyHook) Run(context.Context, string, map[string]string) (*harness.DestroyResult, error) {
	if h != nil {
		h()
	}
	return nil, nil
}

// awsTestRun is one `infrafactory test` of an aws scenario through
// runTestWithNotify.
type awsTestRun struct {
	h      *CommandTestHarness
	result OutputResult
	output string
	err    error
}

// awsTestSetup is what an aws test run varies. With gated, the real gate
// runs: the output directory holds the admitted web_step_one stack after
// stackEdits, and lc serves its AMI and root to the resolve. Otherwise
// the gate is stubbed, logging gateCall.
type awsTestSetup struct {
	customize   func(*config.Config)
	mockDestroy mockDestroyHook
	gated       bool
	stackEdits  []awsStackEdit
	// deps edits the dependencies after the defaults are set.
	deps  func(*RuntimeDependencies)
	flags []string
}

func runAWSTest(t *testing.T, lc *awsLifecycle, customize func(*config.Config), mockDestroy mockDestroyHook, flags ...string) awsTestRun {
	t.Helper()
	return runAWSTestWith(t, lc, awsTestSetup{customize: customize, mockDestroy: mockDestroy, flags: flags})
}

func runAWSTestWith(t *testing.T, lc *awsLifecycle, setup awsTestSetup) awsTestRun {
	t.Helper()
	h := newCommandTestHarness(t)
	setAWSLifecycleScenario(t, h.ScenarioPath)

	opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
		cfg = layer3On(cfg)
		cfg.AWS = config.AWSConfig{Region: "eu-west-2", AccountID: preflightAWSAccount, PrincipalARN: preflightAWSPrincipal}
		if setup.gated {
			cfg.AWS.Region = awsAdmittedRegion
		}
		if setup.customize != nil {
			setup.customize(&cfg)
		}
		return cfg
	})
	opts.deps = RuntimeDependencies{
		MockDeploy:     &fakeMockDeployHarness{},
		Destroy:        setup.mockDestroy,
		SandboxDeploy:  lc.deploy,
		SandboxDestroy: lc.destroy,
		RunProject:     lc.scw.runProject,
		OrphanSweep:    lc.scw.sweep,
		AutoCreated:    lc.scw.purge,
		AWSSTS:         lc,
		AWSSSM:         lc,
		AWSEC2:         lc,
		AWSSweepSleep: func(context.Context, time.Duration) error {
			lc.mu.Lock()
			defer lc.mu.Unlock()
			lc.sleeps++
			return nil
		},
		Layer3HCLGate: func(layer3Cloud, string, awsGateInputs) error {
			lc.record(gateCall)
			return nil
		},
	}
	if setup.gated {
		opts.deps.Layer3HCLGate = nil
		writeAWSAdmittedStack(t, filepath.Join(h.OutputDir(), "example-scenario"), setup.stackEdits...)
	}
	if setup.deps != nil {
		setup.deps(&opts.deps)
	}

	cmd := newTestCommandForTest(opts)
	cmd.RunE = withRuntimeWithOptions("test", opts, sealedHandler(io.Discard,
		func(cmd *cobra.Command, args []string, rt *CommandRuntime) error {
			return runTestWithNotify(cmd, args, rt, lc.notify)
		}))
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(append([]string{h.ScenarioPath, "--config", h.ConfigPath, "--output", string(OutputModeJSON)}, setup.flags...))
	run := awsTestRun{h: h, err: cmd.Execute(), output: stdout.String() + stderr.String()}
	run.result = decodeMachineOutput(t, bytes.NewBufferString(stdout.String()))
	return run
}

func (r awsTestRun) failureDetails() string {
	var b strings.Builder
	for _, f := range r.result.Failures {
		b.WriteString(f.Detail + "\n")
	}
	return b.String()
}

func (r awsTestRun) hasStage(stage string) bool {
	return slices.ContainsFunc(r.result.Stages, func(s StageSummary) bool { return s.Stage == stage })
}

func assertNoProjectAdvice(t *testing.T, output string) {
	t.Helper()
	assert.NotContains(t, output, "delete it by hand")
	assert.NotContains(t, output, "holds nothing but could not be deleted")
}

// assertClaimKept: the claim is still this run's, nothing deleted it,
// and the run says so and how to release it.
func assertClaimKept(t *testing.T, lc *awsLifecycle, run awsTestRun) {
	t.Helper()
	holder, held := lc.claim()
	assert.True(t, held, "the claim is kept")
	assert.Contains(t, holder, "@", "the claim is this run's holder")
	assert.Zero(t, lc.count(deleteClaim), "DeleteParameter")
	assert.True(t, run.hasStage(StageAWSScopeClaimKept), "stages carry %s", StageAWSScopeClaimKept)
	assert.Equal(t, holder, lc.runHolder)
	assert.Contains(t, run.output, lc.takeOver(run.h.ConfigPath, run.h.ScenarioPath))
	assertNoProjectAdvice(t, run.output)
}

func TestAWSTestRefusesBeforeTheClaim(t *testing.T) {
	for name, tc := range map[string]struct {
		setup func(*awsLifecycle)
		want  string
	}{
		"no stamp": {
			setup: func(lc *awsLifecycle) { delete(lc.params, harness.AWSStampParameter) },
			want:  harness.AWSStampParameter + " does not exist",
		},
		"a default vpc": {
			setup: func(lc *awsLifecycle) {
				lc.ec2["DescribeVpcs"] = `<vpcSet><item><vpcId>vpc-0default</vpcId><isDefault>true</isDefault></item></vpcSet>`
			},
			want: "vpc-0default is the region's default VPC",
		},
	} {
		t.Run(name, func(t *testing.T) {
			lc := newAWSLifecycle(t)
			tc.setup(lc)

			run := runAWSTest(t, lc, nil, nil)

			require.Error(t, run.err)
			assert.Contains(t, run.failureDetails(), tc.want)
			assert.Zero(t, lc.count(putClaim), "PutParameter")
			assert.Zero(t, lc.deploy.calls, "SandboxDeploy")
			assert.Zero(t, lc.destroy.calls, "SandboxDestroy")
			assert.False(t, run.hasStage(StageAWSScopeClaimKept), "no claim was taken, so none is kept")
		})
	}
}

func TestAWSTestRefusesAHeldClaimNamingItsHolder(t *testing.T) {
	lc := newAWSLifecycle(t)
	lc.params[harness.AWSClaimParameter] = lifecycleOtherHolder

	run := runAWSTest(t, lc, nil, nil)

	require.Error(t, run.err)
	assert.Contains(t, run.failureDetails(), lifecycleOtherHolder)
	assert.Zero(t, lc.deploy.calls, "SandboxDeploy")
	assert.Zero(t, lc.count(deleteClaim), "DeleteParameter")
	holder, _ := lc.claim()
	assert.Equal(t, lifecycleOtherHolder, holder)
}

func TestAWSTestTakesTheClaimBeforeTheApplyAndReleasesItAfterTheSweep(t *testing.T) {
	lc := newAWSLifecycle(t)

	run := runAWSTest(t, lc, nil, nil)

	require.NoError(t, run.err, run.output)
	calls := lc.log()
	take := slices.Index(calls, putClaim)
	deploy := slices.Index(calls, deployRun)
	destroy := slices.Index(calls, destroyRun)
	lastDescribe := -1
	for i, c := range calls {
		if strings.Contains(c, ":Describe") {
			lastDescribe = i
		}
	}
	release := slices.Index(calls, deleteClaim)
	require.NotEqual(t, -1, take, "the claim was taken: %v", calls)
	assert.Less(t, take, deploy, "TakeAWSClaim before SandboxDeploy.Run: %v", calls)
	assert.Less(t, deploy, destroy, "SandboxDestroy.Run after the apply: %v", calls)
	assert.Less(t, destroy, lastDescribe, "the sweep after the destroy: %v", calls)
	assert.Less(t, lastDescribe, release, "the release after the sweep's last Describe: %v", calls)
	assert.Equal(t, 1, lc.count(deleteClaim))

	_, held := lc.claim()
	assert.False(t, held, "released")
	assert.Contains(t, lc.params, harness.AWSStampParameter, "the stamp stays")
	assert.False(t, run.hasStage(StageAWSScopeClaimKept))
	assert.Contains(t, run.result.Stages, StageSummary{
		Layer: "sandbox_deploy", Stage: "preflight", Status: StageStatusPass,
		Detail: "credentials verified by sts:GetCallerIdentity as account " + preflightAWSAccount +
			" (" + preflightAWSPrincipal + "); the SCP that bounds the scope is not asserted",
	})
	assertNoProjectAdvice(t, run.output)
}

// Each exit after the take runs the sweep; a clean one releases the
// claim, anything else keeps it.
func TestAWSTestEveryExitAfterTheTakeReleasesOnlyAfterACleanSweep(t *testing.T) {
	exits := map[string]struct {
		setup       func(t *testing.T, lc *awsLifecycle) mockDestroyHook
		deploys     int
		alwaysDirty bool
		want        string
	}{
		"env build failure": {
			// The credential file turns unreadable once the claim is taken,
			// failing the apply's env, and is restored before the teardown.
			setup: func(t *testing.T, lc *awsLifecycle) mockDestroyHook {
				lc.onPut = func() { require.NoError(t, os.Chmod(lc.credFile, 0o644)) }
				return func() { require.NoError(t, os.Chmod(lc.credFile, 0o600)) }
			},
			want: "mode 0644",
		},
		"apply fails with no state written": {
			setup: func(_ *testing.T, lc *awsLifecycle) mockDestroyHook {
				lc.deploy.err = errors.New("tofu apply failed")
				lc.deploy.onRunDir = func(string) { lc.record(deployRun) }
				return nil
			},
			deploys: 1,
			want:    "tofu apply failed",
		},
		"destroy fails": {
			setup: func(_ *testing.T, lc *awsLifecycle) mockDestroyHook {
				lc.destroy.err = errors.New("tofu destroy failed")
				return nil
			},
			deploys: 1,
			want:    "tofu destroy failed",
		},
		"cancelled": {
			setup: func(t *testing.T, lc *awsLifecycle) mockDestroyHook {
				lc.deploy.err = context.Canceled
				lc.deploy.onRunDir = func(dir string) {
					lc.record(deployRun)
					require.NoError(t, os.WriteFile(filepath.Join(dir, harness.LiveStateFilename), []byte(awsLiveState), 0o600))
					lc.cancel()
				}
				return nil
			},
			deploys: 1,
			want:    context.Canceled.Error(),
		},
		"sweep 403": {
			setup: func(_ *testing.T, lc *awsLifecycle) mockDestroyHook {
				lc.denied["DescribeNetworkInterfaces"] = true
				return nil
			},
			deploys:     1,
			alwaysDirty: true,
			want:        "UnauthorizedOperation",
		},
		"settle timeout": {
			setup: func(_ *testing.T, lc *awsLifecycle) mockDestroyHook {
				lc.ec2["DescribeInstances"] = settlingInstance
				return nil
			},
			deploys:     1,
			alwaysDirty: true,
			want:        "still settling after",
		},
	}
	for name, exit := range exits {
		for _, dirty := range []bool{false, true} {
			if exit.alwaysDirty && !dirty {
				continue
			}
			scope := "clean scope"
			if dirty {
				scope = "dirty scope"
			}
			t.Run(name+"/"+scope, func(t *testing.T) {
				lc := newAWSLifecycle(t)
				if dirty && !exit.alwaysDirty {
					lc.ec2["DescribeInstances"] = runningInstance
				}
				run := runAWSTest(t, lc, nil, exit.setup(t, lc))

				require.Error(t, run.err)
				assert.Contains(t, run.output, exit.want)
				assert.Equal(t, exit.deploys, lc.deploy.calls, "SandboxDeploy")
				calls := lc.log()
				take := slices.Index(calls, putClaim)
				require.NotEqual(t, -1, take, "the claim was taken: %v", calls)
				assert.Contains(t, calls[take:], "ec2:DescribeInstances", "the sweep ran after the take")
				if exit.want == "still settling after" {
					assert.Equal(t, harness.AWSSweepMaxPolls-1, lc.sleeps, "settle waits")
				}

				if !dirty {
					assert.Equal(t, 1, lc.count(deleteClaim), "released")
					_, held := lc.claim()
					assert.False(t, held)
					assert.False(t, run.hasStage(StageAWSScopeClaimKept))
					assertNoProjectAdvice(t, run.output)
					return
				}
				assertClaimKept(t, lc, run)
			})
		}
	}
}

func TestAWSTestKeepsTheClaimWhenDestructionIsNotWanted(t *testing.T) {
	for name, tc := range map[string]struct {
		customize func(*config.Config)
		flags     []string
	}{
		"--no-destroy":         {flags: []string{"--no-destroy"}},
		"destruction disabled": {customize: func(cfg *config.Config) { cfg.Validation.Layers.Destruction.Enabled = false }},
	} {
		t.Run(name, func(t *testing.T) {
			lc := newAWSLifecycle(t)

			run := runAWSTest(t, lc, tc.customize, nil, tc.flags...)

			require.NoError(t, run.err, "a deliberate keep is not a failure")
			assert.Equal(t, 1, lc.deploy.calls, "SandboxDeploy")
			assert.Zero(t, lc.destroy.calls, "SandboxDestroy")
			assertClaimKept(t, lc, run)
		})
	}
}

func TestAWSTestTreatsAnUnknownClaimOutcomeAsHeld(t *testing.T) {
	lc := newAWSLifecycle(t)
	lc.putFail = true

	run := runAWSTest(t, lc, nil, nil)

	require.Error(t, run.err)
	assert.Contains(t, run.failureDetails(), harness.ErrAWSClaimOutcomeUnknown.Error())
	assert.Zero(t, lc.deploy.calls, "SandboxDeploy")
	assert.Zero(t, lc.count(deleteClaim), "DeleteParameter")
	assert.True(t, run.hasStage(StageAWSScopeClaimKept))
	details := run.failureDetails()
	assert.Contains(t, details, "this run may hold the aws scope's claim for "+lc.runHolder)
	assert.Contains(t, details, "If this run holds the claim, `"+lc.takeOver(run.h.ConfigPath, run.h.ScenarioPath)+"` sweeps the scope")
	assert.Contains(t, details, "if no one holds it, `"+reapCommand(run.h.ConfigPath, run.h.ScenarioPath)+"` does")
	assertNoProjectAdvice(t, run.output)
}

// An env the teardown cannot build either leaves nothing to sweep with:
// the claim is kept, and nothing is deleted.
func TestAWSTestKeepsTheClaimWhenTheTeardownHasNoEnv(t *testing.T) {
	lc := newAWSLifecycle(t)
	lc.onPut = func() { require.NoError(t, os.Chmod(lc.credFile, 0o644)) }

	run := runAWSTest(t, lc, nil, nil)

	require.Error(t, run.err)
	assert.Contains(t, run.failureDetails(), "mode 0644")
	assert.Zero(t, lc.deploy.calls, "SandboxDeploy")
	assertClaimKept(t, lc, run)
}

// nonTestGoFiles parses every non-test Go file under cmd/ and internal/.
func nonTestGoFiles(t *testing.T) map[string]*ast.File {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	files := map[string]*ast.File{}
	for _, dir := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, path)
			files[rel] = f
			return nil
		})
		require.NoError(t, err)
	}
	require.NotEmpty(t, files)
	return files
}

// callsIn lists "<file>:<func>" for each call to name, qualified or not.
func callsIn(files map[string]*ast.File, name string) []string {
	var found []string
	for path, f := range files {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok && calleeName(call) == name {
					found = append(found, path+":"+fn.Name.Name)
				}
				return true
			})
		}
	}
	return found
}

func calleeName(call *ast.CallExpr) string {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name
	case *ast.SelectorExpr:
		return fun.Sel.Name
	}
	return ""
}

func TestTheAWSClaimIsReleasedOnlyAfterACleanSweep(t *testing.T) {
	files := nonTestGoFiles(t)

	assert.Equal(t, []string{"internal/cli/aws_scope_lifecycle.go:awsReleaseAfterCleanSweep"}, callsIn(files, "ReleaseAWSClaim"))
	assert.Equal(t, []string{"internal/cli/test_command.go:executeTestWithScenario"}, callsIn(files, "awsScopeTeardown"))

	var sweep, release token.Pos
	for _, decl := range files["internal/cli/aws_scope_lifecycle.go"].Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "awsReleaseAfterCleanSweep" {
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok {
					switch calleeName(call) {
					case "SweepAWSScope":
						sweep = call.Pos()
					case "ReleaseAWSClaim":
						release = call.Pos()
					}
				}
				return true
			})
		}
	}
	require.NotZero(t, sweep, "awsReleaseAfterCleanSweep calls SweepAWSScope")
	assert.Less(t, sweep, release, "SweepAWSScope before ReleaseAWSClaim")
}

func TestNoProductionCodeSetsTheLayer3HCLGate(t *testing.T) {
	for path, f := range nonTestGoFiles(t) {
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.AssignStmt:
				for _, lhs := range n.Lhs {
					if sel, ok := lhs.(*ast.SelectorExpr); ok && sel.Sel.Name == "Layer3HCLGate" {
						t.Errorf("%s assigns Deps.Layer3HCLGate; only tests stub the gate", path)
					}
				}
			case *ast.KeyValueExpr:
				if key, ok := n.Key.(*ast.Ident); ok && key.Name == "Layer3HCLGate" {
					t.Errorf("%s sets Layer3HCLGate in a literal; only tests stub the gate", path)
				}
			}
			return true
		})
	}
}

// Only the AMI resolve sets the id the model writes and the root the gate
// checks, so neither can come from anywhere but STS, SSM and EC2.
func TestOnlyTheAMIResolveSetsTheLayer3AMI(t *testing.T) {
	fields := map[string]bool{"AWSLayer3AMI": true, "AWSLayer3AMIRoot": true}
	assigned := func(n ast.Node) []string {
		var names []string
		ast.Inspect(n, func(n ast.Node) bool {
			if assign, ok := n.(*ast.AssignStmt); ok {
				for _, lhs := range assign.Lhs {
					if sel, ok := lhs.(*ast.SelectorExpr); ok && fields[sel.Sel.Name] {
						names = append(names, sel.Sel.Name)
					}
				}
			}
			return true
		})
		return names
	}
	var resolveSets []string
	for path, f := range nonTestGoFiles(t) {
		for _, decl := range f.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "resolveAWSLayer3AMI" {
				resolveSets = append(resolveSets, assigned(fn)...)
				continue
			}
			for _, name := range assigned(decl) {
				t.Errorf("%s assigns %s; only resolveAWSLayer3AMI sets it", path, name)
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if kv, ok := n.(*ast.KeyValueExpr); ok {
				if key, ok := kv.Key.(*ast.Ident); ok && fields[key.Name] {
					t.Errorf("%s sets %s in a literal; only resolveAWSLayer3AMI sets it", path, key.Name)
				}
			}
			return true
		})
	}
	assert.ElementsMatch(t, []string{"AWSLayer3AMI", "AWSLayer3AMIRoot"}, resolveSets, "the resolve sets both")
}
