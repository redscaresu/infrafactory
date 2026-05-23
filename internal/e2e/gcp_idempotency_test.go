package e2e

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestE2E_GCPDoubleApplyIdempotency proves the cross-repo idempotency
// contract: running `infrafactory run --no-destroy` twice with the same
// generator output must converge on the same fakegcp state — no extra
// resources, no replacement churn. The first run creates topic +
// subscription; the second run should be a no-op at the mock layer.
//
// This is the Go counterpart to fakegcp/scripts/e2e.sh's apply→plan
// no-op gate (S41-T6). The shell harness covers the tofu CLI path;
// this test covers the infrafactory-run path which threads the same
// HCL through the run loop's state-policy + destroy bookkeeping.
//
// Gated behind INFRAFACTORY_ENABLE_E2E=1; requires `tofu` on PATH and
// the sibling ../fakegcp source repo.
func TestE2E_GCPDoubleApplyIdempotency(t *testing.T) {
	SkipUnlessEnabled(t)
	if _, err := exec.LookPath("tofu"); err != nil {
		t.Fatalf("tofu binary required for e2e: %v", err)
	}
	mock := StartFakegcp(t)

	workspace := t.TempDir()
	outputRoot := filepath.Join(workspace, "output")
	configPath := filepath.Join(workspace, "infrafactory.yaml")
	scenarioPath := repoScenariosPath(t, "gcp-pubsub.yaml")

	WriteConfigMultiCloud(t, configPath, "http://127.0.0.1:1", mock.URL, outputRoot)

	files := gcpPubSubFiles(mock.URL)

	// First apply.
	first := RunInfrafactory(t, InfrafactoryRunOptions{
		Args:           []string{"run", scenarioPath, "--config", configPath, "--no-destroy"},
		GeneratorFiles: files,
	})
	if first.Err != nil {
		t.Fatalf("first --no-destroy run failed: %v\nstdout:\n%s\nstderr:\n%s\nfakegcp log: %s",
			first.Err, first.Stdout, first.Stderr, mock.LogPath())
	}
	if !strings.Contains(first.Stdout, "run/terminal_reason: pass (target_reached)") {
		t.Fatalf("expected first run to reach target_reached, got:\n%s", first.Stdout)
	}

	stateAfterFirst := mock.FetchState(t)
	topicsBefore := gcpStateItemCount(stateAfterFirst, "pubsub", "topics")
	subsBefore := gcpStateItemCount(stateAfterFirst, "pubsub", "subscriptions")
	if topicsBefore == 0 || subsBefore == 0 {
		t.Fatalf("first run produced no topics/subs: topics=%d subs=%d", topicsBefore, subsBefore)
	}

	// Snapshot the per-resource names so a silent delete-recreate is
	// detected, not just a count match.
	expects := []gcpStateExpect{
		{root: "pubsub", collection: "topics", minCount: topicsBefore},
		{root: "pubsub", collection: "subscriptions", minCount: subsBefore},
	}
	identitiesBefore := collectIdentities(stateAfterFirst, expects)

	// Second apply with the same files. Must not grow the mock state.
	second := RunInfrafactory(t, InfrafactoryRunOptions{
		Args:           []string{"run", scenarioPath, "--config", configPath, "--no-destroy"},
		GeneratorFiles: files,
	})
	if second.Err != nil {
		t.Fatalf("second --no-destroy run failed: %v\nstdout:\n%s\nstderr:\n%s\nfakegcp log: %s",
			second.Err, second.Stdout, second.Stderr, mock.LogPath())
	}
	if !strings.Contains(second.Stdout, "run/terminal_reason: pass (target_reached)") {
		t.Fatalf("expected second run to reach target_reached, got:\n%s", second.Stdout)
	}

	stateAfterSecond := mock.FetchState(t)
	topicsAfter := gcpStateItemCount(stateAfterSecond, "pubsub", "topics")
	subsAfter := gcpStateItemCount(stateAfterSecond, "pubsub", "subscriptions")
	if topicsAfter != topicsBefore {
		t.Errorf("double-apply grew topics: before=%d after=%d (expected no change)", topicsBefore, topicsAfter)
	}
	if subsAfter != subsBefore {
		t.Errorf("double-apply grew subscriptions: before=%d after=%d (expected no change)", subsBefore, subsAfter)
	}

	identitiesAfter := collectIdentities(stateAfterSecond, expects)
	for key, before := range identitiesBefore {
		after := identitiesAfter[key]
		if !sameIdentities(before, after) {
			t.Errorf("double-apply silently replaced %s: ids before=%v after=%v", key, before, after)
		}
	}
}
