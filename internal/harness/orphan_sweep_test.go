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
)

const testProjectID = "11111111-1111-1111-1111-111111111111"
const testOrgID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"

func writeSweepLiveState(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, LiveStateFilename), []byte(body), 0o600); err != nil {
		t.Fatalf("write live state: %v", err)
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
	result, err := sweep.Run(context.Background(), dir, "secret")
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
	result, err := sweep.Run(context.Background(), dir, "secret")
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
	result, err := sweep.Run(context.Background(), dir, "secret")
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
	result, err := sweep.Run(context.Background(), dir, "secret")
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
	result, err := sweep.Run(context.Background(), dir, "secret")
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

func TestOrphanSweepRequiresProjectInState(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeSweepLiveState(t, dir, `{"resources":[{"type":"scaleway_block_volume","instances":[{"attributes":{"id":"vol-1"}}]}]}`)

	sweep := NewScalewayOrphanSweepWithDoer("https://api.example", respondWith(http.StatusNotFound, `{}`))
	if _, err := sweep.Run(context.Background(), dir, "secret"); !errors.Is(err, ErrOrphanSweepFailed) {
		t.Fatalf("state without a project resource means the blast radius is unknown; expected ErrOrphanSweepFailed, got %v", err)
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
