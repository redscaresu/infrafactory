package harness

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testProjectID = "11111111-1111-1111-1111-111111111111"
const testOrgID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"

func writeSweepLiveState(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, LiveStateFilename), []byte(body), 0o600); err != nil {
		t.Fatalf("write live state: %v", err)
	}
	// ADR-0025: CaptureSweepTarget takes the project from the marker, not
	// from a scaleway_account_project in state.
	if err := WriteRunProjectMarker(dir, RunProject{ID: testProjectID, Name: RunProjectNamePrefix + "sweep"}); err != nil {
		t.Fatalf("write marker: %v", err)
	}
}

func stateWithProjectAndVolume(projectOfVolume string) string {
	return `{"resources":[
	  {"type":"scaleway_account_project","instances":[{"attributes":{"id":"` + testProjectID + `","name":"run"}}]},
	  {"type":"scaleway_block_volume","instances":[{"attributes":{"id":"vol-1","project_id":"` + projectOfVolume + `"}}]}
	]}`
}

func respondWith(status int, body string) func(*http.Request) (*http.Response, error) {
	return func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	}
}

func TestOrphanSweepCleanWhenProjectGone(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeSweepLiveState(t, dir, stateWithProjectAndVolume(testProjectID))

	sweep := NewScalewayOrphanSweepWithDoer("https://api.example", respondWith(http.StatusNotFound, `{}`))
	result, err := sweepDir(t, sweep, dir)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if !result.Clean() {
		t.Fatalf("expected clean sweep, got failures: %+v", result.Failures)
	}
}

func TestOrphanSweepFailsWhenProjectSurvives(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeSweepLiveState(t, dir, stateWithProjectAndVolume(testProjectID))

	sweep := NewScalewayOrphanSweepWithDoer("https://api.example", respondWith(http.StatusOK, `{"id":"`+testProjectID+`"}`))
	result, err := sweepDir(t, sweep, dir)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if result.Clean() {
		t.Fatal("a surviving project is billable and must be reported as a leak")
	}
	if !strings.Contains(result.Failures[0].Detail, "infrafactory reap") {
		t.Errorf("failure should tell the operator how to clean up, got: %s", result.Failures[0].Detail)
	}
}

// An unverifiable sweep must never look like a clean one.
func TestOrphanSweepFailsWhenAPIUnreachable(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeSweepLiveState(t, dir, stateWithProjectAndVolume(testProjectID))

	sweep := NewScalewayOrphanSweepWithDoer("https://api.example", func(*http.Request) (*http.Response, error) {
		return nil, errors.New("dial tcp: no route to host")
	})
	result, err := sweepDir(t, sweep, dir)
	if err != nil {
		t.Fatalf("transport failure should surface as a reported leak, not a hard error: %v", err)
	}
	if result.Clean() {
		t.Fatal("could-not-check must not be reported as clean")
	}
}

func TestOrphanSweepFailsOnUnexpectedStatus(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeSweepLiveState(t, dir, stateWithProjectAndVolume(testProjectID))

	sweep := NewScalewayOrphanSweepWithDoer("https://api.example", respondWith(http.StatusForbidden, `{"message":"denied"}`))
	result, err := sweepDir(t, sweep, dir)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if result.Clean() {
		t.Fatal("a 403 means we do not know whether the project survived — must not be clean")
	}
}

// The failure the project-scoped check alone cannot see: a resource that
// omitted project_id landed in the org default project, so destroying
// the run project succeeds and the sweep 404s while the stray bills on.
func TestOrphanSweepDetectsResourceOutsideRunProject(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeSweepLiveState(t, dir, stateWithProjectAndVolume(testOrgID))

	sweep := NewScalewayOrphanSweepWithDoer("https://api.example", respondWith(http.StatusNotFound, `{}`))
	result, err := sweepDir(t, sweep, dir)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if result.Clean() {
		t.Fatal("a resource created outside the run project is invisible to the project check and must be caught from state")
	}
	detail := result.Failures[0].Detail
	for _, want := range []string{"vol-1", testOrgID, "project_id"} {
		if !strings.Contains(detail, want) {
			t.Errorf("stray failure should name %q so it can be tracked down, got: %s", want, detail)
		}
	}
}

// ADR-0025 moved the blast radius from the state file to the marker, so
// this is the shape that now means "we cannot tell what to verify".
func TestOrphanSweepRequiresTheRunProjectMarker(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// State but no marker: nothing says which project this run owns.
	if err := os.WriteFile(filepath.Join(dir, LiveStateFilename),
		[]byte(`{"resources":[{"type":"scaleway_block_volume","instances":[{"attributes":{"id":"vol-1"}}]}]}`), 0o600); err != nil {
		t.Fatalf("write live state: %v", err)
	}

	if _, err := CaptureSweepTarget(dir); !errors.Is(err, ErrOrphanSweepFailed) {
		t.Fatalf("without a marker the blast radius is unknown; expected ErrOrphanSweepFailed, got %v", err)
	}
}

// "We could not read the state" is not "there were no strays" -- the
// same rule the sweep applies to an unreachable API.
func TestCaptureSweepTargetRefusesAnUnreadableState(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, WriteRunProjectMarker(dir, RunProject{ID: testProjectID, Name: RunProjectNamePrefix + "x"}))
	require.NoError(t, os.WriteFile(filepath.Join(dir, LiveStateFilename), []byte("{truncated"), 0o600))

	_, err := CaptureSweepTarget(dir)

	require.ErrorIs(t, err, ErrOrphanSweepFailed)
	assert.Contains(t, err.Error(), "strays outside project")
}

// An apply that wrote no state still created a project, and the sweep
// must still verify it.
func TestCaptureSweepTargetWorksWithoutState(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := WriteRunProjectMarker(dir, RunProject{ID: testProjectID, Name: RunProjectNamePrefix + "x"}); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	target, err := CaptureSweepTarget(dir)
	if err != nil {
		t.Fatalf("a marker with no state is still verifiable: %v", err)
	}
	if target.ProjectID != testProjectID {
		t.Fatalf("got project %q, want %q", target.ProjectID, testProjectID)
	}
}

// AssertProjectDeletable is the guard standing between an automated
// teardown and someone's real infrastructure.
func TestAssertProjectDeletable(t *testing.T) {
	t.Parallel()

	if err := AssertProjectDeletable(testProjectID, testProjectID, testOrgID); err != nil {
		t.Fatalf("the run's own project must be deletable: %v", err)
	}

	cases := []struct {
		name          string
		stateProject  string
		targetProject string
		why           string
	}{
		{"default project", testOrgID, testOrgID, "the org default project must never be deletable — its id is the org id"},
		{"foreign project", testProjectID, "99999999-9999-9999-9999-999999999999", "a project this run did not create must be refused"},
		{"no state project", "", testProjectID, "without a recorded project there is no evidence the run created it"},
		{"empty target", testProjectID, "", "an empty id must not fall through to a delete"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := AssertProjectDeletable(tc.stateProject, tc.targetProject, testOrgID)
			if !errors.Is(err, ErrProtectedProject) {
				t.Fatalf("%s; expected ErrProtectedProject, got %v", tc.why, err)
			}
		})
	}
}

// sweepDir mirrors production ordering: capture the target from live
// state, then sweep. Production captures before destroy; tests capture
// from a state file that was never destroyed.
func sweepDir(t *testing.T, sweep *ScalewayOrphanSweep, dir string) (*OrphanSweepResult, error) {
	t.Helper()
	target, err := CaptureSweepTarget(dir)
	if err != nil {
		return nil, err
	}
	return sweep.Run(context.Background(), target, "secret")
}
