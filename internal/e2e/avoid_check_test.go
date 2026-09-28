package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/redscaresu/infrafactory/internal/cli"
	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/generator"
)

// avoidCheckSubnetRule is the rule pitfalls/aws.yaml carries for
// map_public_ip_on_launch, in buildAvoidRule's form.
const avoidCheckSubnetRule = "exit status 1 | stderr: ╷ Do NOT use attribute `map_public_ip_on_launch` on `aws_subnet` — observed in scenario \"aws-eks\" to cause the failure above."

// TestE2E_CheckAvoidAgainstFakeaws runs `pitfalls check-avoid` through
// the command with mockway at a closed port: the replay must reset and
// apply on fakeaws, which the cloud-state router would not do with no
// scenario loaded. The required CI job runs it by name.
func TestE2E_CheckAvoidAgainstFakeaws(t *testing.T) {
	SkipUnlessEnabled(t)
	_, err := exec.LookPath("tofu")
	require.NoError(t, err, "tofu binary required for e2e")
	mock := StartFakeaws(t, "--echo")
	SealNetwork(t)

	workspace := t.TempDir()
	configPath := filepath.Join(workspace, "infrafactory.yaml")
	WriteConfigMultiCloud(t, configPath, "http://127.0.0.1:1", "", mock.URL, "", filepath.Join(workspace, "output"))
	cfg, err := config.Load(configPath)
	require.NoError(t, err)
	corpus, err := yaml.Marshal(generator.PitfallsFile{Provider: "aws", Pitfalls: []generator.PitfallEntry{{
		Resource: "aws_subnet", Rule: avoidCheckSubnetRule, Source: generator.AvoidSource,
		DiscoveredFrom: "aws-eks", LearnedLayer: generator.MockDeployLayer,
	}}})
	require.NoError(t, err)
	WriteFile(t, filepath.Join(cfg.Paths.Pitfalls, "aws.yaml"), corpus)

	from := t.TempDir()
	hcl, err := os.ReadFile(filepath.Join(RepoRoot(t), "internal", "e2e", "testdata", awsWebStepOneDir, awsWebStepOneName+".tf"))
	require.NoError(t, err)
	WriteFile(t, filepath.Join(from, awsWebStepOneName+".tf"), hcl)

	result := RunInfrafactory(t, InfrafactoryRunOptions{Args: []string{
		"pitfalls", "check-avoid", "aws", "--config", configPath,
		"--resource", "aws_subnet", "--attribute", "map_public_ip_on_launch", "--from", from,
	}})
	require.NoError(t, result.Err, "stdout:\n%s\nfakeaws log: %s", result.Stdout, mock.LogPath())

	log, err := os.ReadFile(mock.LogPath())
	require.NoError(t, err)
	assert.Contains(t, string(log), "echo: POST /mock/reset", "the replay reset fakeaws")

	ledger, err := generator.ReadAvoidLedger(cfg.Paths.Pitfalls, "aws")
	require.NoError(t, err)
	require.Len(t, ledger.Records, 1)
	rec := ledger.Records[0]
	assert.Equal(t, generator.AvoidRecordRetired, rec.Status, "detail: %s", rec.Check.Detail)
	assert.Equal(t, 0, *rec.Check.ApplyExit)
	assert.Equal(t, 0, *rec.Check.PlanExit)

	// The record it wrote replays as CI replays the repo's.
	assert.Empty(t, replayRetiredAvoidRecords(t, cfg.Paths.Pitfalls, func() string { return mock.URL }))
}

// TestE2E_RetiredAvoidPitfallsStayContradicted replays every retired
// record's stored shape in the repo's ledgers against fakeaws, through
// the same constructor check-avoid uses. A mock regression that brings a
// retired failure back fails CI here, not the corpus. The required CI job
// runs it by name.
func TestE2E_RetiredAvoidPitfallsStayContradicted(t *testing.T) {
	SkipUnlessEnabled(t)
	var fakeawsURL string
	startMock := func() string {
		if fakeawsURL == "" {
			_, err := exec.LookPath("tofu")
			require.NoError(t, err, "tofu binary required for e2e")
			fakeawsURL = StartFakeaws(t).URL
			SealNetwork(t)
		}
		return fakeawsURL
	}
	for _, failure := range replayRetiredAvoidRecords(t, filepath.Join(RepoRoot(t), "pitfalls"), startMock) {
		t.Error(failure)
	}
}

func TestRetiredAvoidReplayPassesOnAnEmptyLedger(t *testing.T) {
	dir := t.TempDir()
	WriteFile(t, filepath.Join(dir, "avoid-checks", "aws.yaml"), []byte("provider: aws\nrecords: []\n"))

	failures := replayRetiredAvoidRecords(t, dir, func() string { t.Fatal("no record needs a mock"); return "" })

	assert.Empty(t, failures)
}

// A retired record for a cloud with no mock in CI can never be replayed,
// so it fails rather than passing unchecked.
func TestRetiredAvoidReplayFailsClosedOnAnUnmockedCloud(t *testing.T) {
	dir := t.TempDir()
	zero := 0
	ledger := generator.AvoidLedger{Provider: "gcp", Records: []generator.AvoidLedgerRecord{{
		Status: generator.AvoidRecordRetired, Resource: "google_compute_subnetwork", Attributes: []string{"private_ip_google_access"},
		LearnedLayer: generator.MockDeployLayer,
		Entry:        &generator.PitfallEntry{Resource: "google_compute_subnetwork", Source: generator.AvoidSource, LearnedLayer: generator.MockDeployLayer},
		Check: &generator.AvoidCheck{ID: "20260928T120000Z-google_compute_subnetwork", At: "2026-09-28T12:00:00Z", From: "x",
			ShapeSHA256: "abc", ApplyExit: &zero, PlanExit: &zero, Outcome: generator.AvoidOutcomeContradicted},
	}}}
	payload, err := yaml.Marshal(ledger)
	require.NoError(t, err)
	WriteFile(t, filepath.Join(dir, "avoid-checks", "gcp.yaml"), payload)

	failures := replayRetiredAvoidRecords(t, dir, func() string { t.Fatal("gcp has no CI mock"); return "" })

	require.Len(t, failures, 1)
	assert.Contains(t, failures[0], "20260928T120000Z-google_compute_subnetwork")
	assert.Contains(t, failures[0], "no mock in CI")
}

// replayRetiredAvoidRecords returns one failure, naming the record, for
// each retired record that does not replay as contradicted. fakeawsURL is
// called only when a record needs the mock.
func replayRetiredAvoidRecords(t *testing.T, pitfallsDir string, fakeawsURL func() string) []string {
	t.Helper()
	ledgers, err := filepath.Glob(filepath.Join(pitfallsDir, "avoid-checks", "*.yaml"))
	require.NoError(t, err)
	var failures []string
	for _, path := range ledgers {
		cloud := strings.TrimSuffix(filepath.Base(path), ".yaml")
		ledger, err := generator.ReadAvoidLedger(pitfallsDir, cloud)
		require.NoError(t, err)
		for _, rec := range ledger.Records {
			if rec.Status != generator.AvoidRecordRetired {
				continue
			}
			name := fmt.Sprintf("%s record %s (%s %v)", cloud, rec.Check.ID, rec.Resource, rec.Attributes)
			if cloud != "aws" {
				failures = append(failures, name+": the cloud has no mock in CI, so the retirement cannot be replayed")
				continue
			}
			cfg := config.Config{Fakeaws: config.FakeawsConfig{URL: fakeawsURL()}}
			shape := filepath.Join(pitfallsDir, "avoid-checks", "shapes", rec.Check.ID)
			got, err := cli.ReplayAvoidShape(context.Background(), cfg, cloud, shape, rec.Resource, rec.Attributes)
			switch {
			case err != nil:
				failures = append(failures, fmt.Sprintf("%s: replay did not run: %v", name, err))
			case got.Outcome != generator.AvoidOutcomeContradicted:
				failures = append(failures, fmt.Sprintf("%s: %s (apply_exit %d, plan_exit %d): %s", name, got.Outcome, got.ApplyExit, got.PlanExit, got.Detail))
			}
		}
	}
	return failures
}
