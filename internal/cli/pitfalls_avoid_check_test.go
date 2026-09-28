package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/harness"
)

// checkAvoidSubnetRule is pitfalls/aws.yaml's map_public_ip_on_launch rule.
const checkAvoidSubnetRule = "exit status 1 | stderr: ╷ Do NOT use attribute `map_public_ip_on_launch` on `aws_subnet` — observed in scenario \"aws-eks\" to cause the failure above."

const checkAvoidFakeawsURL = "http://127.0.0.1:8082"

const checkAvoidSource = `resource "aws_vpc" "main" {
  cidr_block = "10.0.0.0/16"
}

resource "aws_subnet" "a" {
  vpc_id                  = aws_vpc.main.id
  cidr_block              = "10.0.1.0/24"
  map_public_ip_on_launch = true
}
`

// fakeTofu answers each tofu subcommand from a script and records what
// it saw, including the providers.tf in the work dir at init.
type fakeTofu struct {
	results   map[string]harness.CommandResult
	errs      map[string]error
	calls     []harness.Command
	providers string
}

func (f *fakeTofu) Run(_ context.Context, cmd harness.Command) (harness.CommandResult, error) {
	f.calls = append(f.calls, cmd)
	if cmd.Args[0] == "init" {
		payload, _ := os.ReadFile(filepath.Join(cmd.Dir, "providers.tf"))
		f.providers = string(payload)
	}
	return f.results[cmd.Args[0]], f.errs[cmd.Args[0]]
}

type fakeMockState struct{ resets int }

func (m *fakeMockState) Reset(context.Context) error           { m.resets++; return nil }
func (m *fakeMockState) Snapshot(context.Context) error        { return nil }
func (m *fakeMockState) Restore(context.Context) error         { return nil }
func (m *fakeMockState) State(context.Context) ([]byte, error) { return []byte("{}"), nil }

type checkAvoidFixture struct {
	pitfalls string
	from     string
	cfg      config.Config
	tofu     *fakeTofu
	mock     *fakeMockState
}

func newCheckAvoidFixture(t *testing.T, entry generator.PitfallEntry) *checkAvoidFixture {
	t.Helper()
	f := &checkAvoidFixture{pitfalls: t.TempDir(), from: t.TempDir(), tofu: &fakeTofu{}, mock: &fakeMockState{}}
	f.cfg = config.Config{Fakeaws: config.FakeawsConfig{URL: checkAvoidFakeawsURL}, Paths: config.PathsConfig{Pitfalls: f.pitfalls}}
	mustWriteFile(t, filepath.Join(f.from, "network.tf"), checkAvoidSource)
	corpus := "provider: aws\npitfalls: []\n"
	if entry.Rule != "" {
		payload, err := yaml.Marshal(generator.PitfallsFile{Provider: "aws", Pitfalls: []generator.PitfallEntry{entry}})
		require.NoError(t, err)
		corpus = string(payload)
	}
	mustWriteFile(t, filepath.Join(f.pitfalls, "aws.yaml"), corpus)
	return f
}

func subnetAvoid() generator.PitfallEntry {
	return generator.PitfallEntry{Resource: "aws_subnet", Rule: checkAvoidSubnetRule, Source: generator.AvoidSource,
		DiscoveredFrom: "aws-eks", LearnedLayer: generator.MockDeployLayer}
}

// check runs the command's path: the replay constructor, then checkAvoid.
func (f *checkAvoidFixture) check(t *testing.T, cloud string) (generator.AvoidLedgerRecord, error) {
	t.Helper()
	replay, err := newAvoidReplay(f.cfg, cloud, f.tofu, f.mock)
	if err != nil {
		return generator.AvoidLedgerRecord{}, err
	}
	req := avoidCheckRequest{Resource: "aws_subnet", Attributes: []string{"map_public_ip_on_launch"}, From: f.from}
	return checkAvoid(context.Background(), f.pitfalls, cloud, req, replay, time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC))
}

func (f *checkAvoidFixture) corpus(t *testing.T) string {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join(f.pitfalls, "aws.yaml"))
	require.NoError(t, err)
	return string(payload)
}

func TestCheckAvoidRetiresOnACleanApplyAndPlan(t *testing.T) {
	f := newCheckAvoidFixture(t, subnetAvoid())

	rec, err := f.check(t, "aws")
	require.NoError(t, err)

	assert.Equal(t, generator.AvoidRecordRetired, rec.Status)
	assert.Equal(t, generator.AvoidOutcomeContradicted, rec.Check.Outcome)
	assert.Equal(t, 0, *rec.Check.ApplyExit)
	assert.Equal(t, 0, *rec.Check.PlanExit)
	assert.NotContains(t, f.corpus(t), "map_public_ip_on_launch", "the rule left the corpus")
	assert.Equal(t, 1, f.mock.resets, "the replay starts from a reset mock")

	shapeDir := filepath.Join(f.pitfalls, "avoid-checks", "shapes", rec.Check.ID)
	sum, err := generator.ShapeSHA256(shapeDir)
	require.NoError(t, err)
	assert.Equal(t, sum, rec.Check.ShapeSHA256)

	ledger, err := generator.ReadAvoidLedger(f.pitfalls, "aws")
	require.NoError(t, err)
	require.Len(t, ledger.Records, 1)
	assert.Equal(t, rec.Check.ID, ledger.Records[0].Check.ID)
}

// The replay writes infrafactory's own provider block with runID "", and
// reaches fakeaws through the Layer 2 env, never through the HCL.
func TestCheckAvoidReplaysAtLayer2WithInfrafactorysProviders(t *testing.T) {
	f := newCheckAvoidFixture(t, subnetAvoid())

	rec, err := f.check(t, "aws")
	require.NoError(t, err)

	shape, err := readShapeFiles(filepath.Join(f.pitfalls, "avoid-checks", "shapes", rec.Check.ID))
	require.NoError(t, err)
	require.NoError(t, ensureAwsProviderWiring(shape, f.cfg, ""))
	assert.Equal(t, string(shape["providers.tf"]), f.tofu.providers)
	assert.NotContains(t, f.tofu.providers, "default_tags")
	assert.NotContains(t, f.tofu.providers, "endpoints")

	require.Len(t, f.tofu.calls, 3, "init, apply, converge plan")
	for _, cmd := range f.tofu.calls {
		assert.Equal(t, harness.Layer2StripEnv, cmd.StripEnv, "%v", cmd.Args)
		assert.Equal(t, "test", cmd.Env["AWS_ACCESS_KEY_ID"], "%v", cmd.Args)
		assert.Equal(t, checkAvoidFakeawsURL+"/ec2/region/us-east-1", cmd.Env["AWS_ENDPOINT_URL_EC2"], "%v", cmd.Args)
	}
}

func TestCheckAvoidKeepsTheRuleOnDrift(t *testing.T) {
	f := newCheckAvoidFixture(t, subnetAvoid())
	f.tofu.results = map[string]harness.CommandResult{"plan": {ExitCode: 2, Stdout: []byte("# aws_subnet.a will be updated in-place")}}
	f.tofu.errs = map[string]error{"plan": errors.New("exit status 2")}
	before := f.corpus(t)

	rec, err := f.check(t, "aws")
	require.NoError(t, err)

	assert.Equal(t, generator.AvoidRecordKept, rec.Status)
	assert.Equal(t, 0, *rec.Check.ApplyExit)
	assert.Equal(t, 2, *rec.Check.PlanExit)
	assert.Contains(t, rec.Check.Detail, "converge plan not empty")
	assert.Equal(t, before, f.corpus(t), "a kept rule leaves the corpus byte-identical")
}

func TestCheckAvoidClassifiesAFailedApply(t *testing.T) {
	for name, tc := range map[string]struct {
		stderr string
		want   string
	}{
		"names the attribute": {"Error: waiting for EC2 Subnet MapPublicIpOnLaunch update: timeout", generator.AvoidOutcomeRecurred},
		"names the resource":  {"Error: creating EC2 Subnet\n  with aws_subnet.a,", generator.AvoidOutcomeRecurred},
		"anything else":       {"Error: creating EC2 VPC: connection refused", generator.AvoidOutcomeInconclusive},
	} {
		t.Run(name, func(t *testing.T) {
			f := newCheckAvoidFixture(t, subnetAvoid())
			f.tofu.results = map[string]harness.CommandResult{"apply": {ExitCode: 1, Stderr: []byte(tc.stderr)}}
			f.tofu.errs = map[string]error{"apply": errors.New("exit status 1")}
			before := f.corpus(t)

			rec, err := f.check(t, "aws")
			require.NoError(t, err)

			assert.Equal(t, generator.AvoidRecordKept, rec.Status)
			assert.Equal(t, tc.want, rec.Check.Outcome)
			assert.Equal(t, 1, *rec.Check.ApplyExit)
			assert.Equal(t, generator.AvoidExitNotRun, *rec.Check.PlanExit)
			assert.Equal(t, before, f.corpus(t))
		})
	}
}

// Every refusal happens before tofu runs, and writes nothing.
func TestCheckAvoidRefusesBeforeAnyTofuCall(t *testing.T) {
	legacy := subnetAvoid()
	legacy.LearnedLayer = ""
	for name, tc := range map[string]struct {
		entry generator.PitfallEntry
		cloud string
		edit  func(t *testing.T, f *checkAvoidFixture)
		want  string
	}{
		"attribute missing": {subnetAvoid(), "aws", func(t *testing.T, f *checkAvoidFixture) {
			mustWriteFile(t, filepath.Join(f.from, "network.tf"), strings.ReplaceAll(checkAvoidSource, "map_public_ip_on_launch = true", ""))
		}, "sets every attribute"},
		"literal false": {subnetAvoid(), "aws", func(t *testing.T, f *checkAvoidFixture) {
			mustWriteFile(t, filepath.Join(f.from, "network.tf"), strings.ReplaceAll(checkAvoidSource, "= true", "= false"))
		}, "literal off"},
		"layer not established": {legacy, "aws", nil, "layer is not established"},
		"ledger malformed": {subnetAvoid(), "aws", func(t *testing.T, f *checkAvoidFixture) {
			mustWriteFile(t, filepath.Join(f.pitfalls, "avoid-checks", "aws.yaml"), "provider: aws\nrecords: nope\n")
		}, "ledger unreadable"},
		"invalid cloud":   {subnetAvoid(), "../aws", nil, "no wired mock"},
		"unwired cloud":   {subnetAvoid(), "gcp", nil, "no wired mock"},
		"no mock URL":     {subnetAvoid(), "aws", func(_ *testing.T, f *checkAvoidFixture) { f.cfg.Fakeaws.URL = "" }, "fakeaws.url is not set"},
		"no corpus entry": {generator.PitfallEntry{}, "aws", nil, "no corpus entry"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newCheckAvoidFixture(t, tc.entry)
			if tc.edit != nil {
				tc.edit(t, f)
			}
			before := f.corpus(t)

			_, err := f.check(t, tc.cloud)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			assert.Empty(t, f.tofu.calls)
			assert.Zero(t, f.mock.resets)
			assert.Equal(t, before, f.corpus(t))
			assert.NoDirExists(t, filepath.Join(f.pitfalls, "avoid-checks", "shapes"))
		})
	}
}

// check-avoid is built with withRuntimeNoGenerator: it runs with the
// agent pointing at a binary that does not exist, and the runtime it gets
// refuses any generate call.
func TestCheckAvoidNeverGenerates(t *testing.T) {
	h := newCommandTestHarness(t)
	raw, err := os.ReadFile(h.ConfigPath)
	require.NoError(t, err)
	patched := strings.Replace(string(raw), "type: claude-code",
		"type: claude-code\n  claude:\n    command: /nonexistent/claude", 1)
	require.NotEqual(t, string(raw), patched)
	require.NoError(t, os.WriteFile(h.ConfigPath, []byte(patched), 0o644))

	root := NewRootCmd()
	var out strings.Builder
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"pitfalls", "check-avoid", "aws", "--config", h.ConfigPath,
		"--resource", "aws_subnet", "--attribute", "map_public_ip_on_launch", "--from", t.TempDir()})
	err = root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fakeaws.url is not set", "the runtime was built without a generator")

	var generateErr error
	handler := (&rootConfig{}).withRuntimeNoGenerator("pitfalls check-avoid", func(cmd *cobra.Command, _ []string, rt *CommandRuntime) error {
		_, generateErr = rt.Deps.Generator.Generate(cmd.Context(), generator.Request{})
		return nil
	})
	cmd := &cobra.Command{Use: "check-avoid"}
	cmd.Flags().String("output", string(OutputModeHuman), "")
	cmd.Flags().String("config", h.ConfigPath, "")
	cmd.SetContext(context.Background())
	require.NoError(t, handler(cmd, []string{"aws"}))
	require.Error(t, generateErr)
	assert.Contains(t, generateErr.Error(), "does not generate")
}
