package harness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
}

func TestRunProjectNameIsStampedAndSafe(t *testing.T) {
	cases := map[string]struct{ scenario, stamp, want string }{
		"plain":            {"web-live-paris", "20260830T170405Z", "if-run-web-live-paris-20260830t170405z"},
		"no stamp":         {"block-paris", "", "if-run-block-paris"},
		"uppercase":        {"Web-Live", "A1", "if-run-web-live-a1"},
		"unsafe chars":     {"../evil name", "x", "if-run-evil-name-x"},
		"underscores":      {"web_live", "y", "if-run-web-live-y"},
		"collapses hyphen": {"a---b", "z", "if-run-a-b-z"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := RunProjectName(tc.scenario, tc.stamp)
			assert.Equal(t, tc.want, got)
			assert.True(t, strings.HasPrefix(got, RunProjectNamePrefix), "the provenance stamp is the point")
			assert.NotContains(t, got, "/", "a project name must never carry a path separator")
			assert.NotContains(t, got, "..")
		})
	}
}

func TestRunProjectNameStaysWithinTheLengthLimit(t *testing.T) {
	got := RunProjectName(strings.Repeat("scenario-", 20), "20260830T170405Z")

	assert.LessOrEqual(t, len(got), maxRunProjectNameLength)
	assert.True(t, strings.HasPrefix(got, RunProjectNamePrefix))
	assert.False(t, strings.HasSuffix(got, "-"), "a truncated name must not end mid-separator")
}

func TestCreateStampsNameAndDescription(t *testing.T) {
	var gotBody map[string]string
	var gotToken, gotURL string

	client := NewScalewayRunProjectWithDoer("https://api.test", func(r *http.Request) (*http.Response, error) {
		gotToken = r.Header.Get("X-Auth-Token")
		gotURL = r.URL.String()
		payload, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(payload, &gotBody)
		return jsonResponse(http.StatusOK, `{"id":"proj-1","name":"if-run-web-live-paris-x"}`), nil
	})

	project, err := client.Create(context.Background(), "secret", "org-1", "web-live-paris", "x")

	require.NoError(t, err)
	assert.Equal(t, "proj-1", project.ID)
	assert.Equal(t, "secret", gotToken)
	assert.Equal(t, "https://api.test/account/v3/projects", gotURL)
	assert.Equal(t, "if-run-web-live-paris-x", gotBody["name"])
	assert.Equal(t, "org-1", gotBody["organization_id"])
	assert.Equal(t, RunProjectDescription, gotBody["description"],
		"the description is half the provenance stamp S166 will check")
}

// A create that reports success without an id leaves a project nothing
// can find, destroy, or reap — worse than a failed create.
func TestCreateFailsWhenTheResponseCarriesNoID(t *testing.T) {
	client := NewScalewayRunProjectWithDoer("https://api.test", func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"name":"if-run-x"}`), nil
	})

	_, err := client.Create(context.Background(), "secret", "org-1", "x", "y")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be tracked or destroyed")
}

func TestCreateSurfacesTheAPIError(t *testing.T) {
	client := NewScalewayRunProjectWithDoer("https://api.test", func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusForbidden, `{"message":"insufficient permissions"}`), nil
	})

	_, err := client.Create(context.Background(), "secret", "org-1", "x", "y")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "http 403")
	assert.Contains(t, err.Error(), "insufficient permissions",
		"the provider message is the whole diagnostic value (ADR-0023)")
}

func TestCreateRefusesWithoutCredentialsOrOrganization(t *testing.T) {
	client := NewScalewayRunProjectWithDoer("https://api.test", func(*http.Request) (*http.Response, error) {
		t.Fatal("must not reach the API without credentials")
		return nil, nil
	})

	_, err := client.Create(context.Background(), "", "org-1", "x", "y")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no secret key")

	_, err = client.Create(context.Background(), "secret", "", "x", "y")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no organization id")
}

func TestDeleteTreatsAMissingProjectAsSuccess(t *testing.T) {
	client := NewScalewayRunProjectWithDoer("https://api.test", func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusNotFound, `{"message":"not found"}`), nil
	})

	assert.NoError(t, client.Delete(context.Background(), "secret", "proj-1"),
		"gone is the outcome asked for")
}

func TestDeleteSurfacesAFailure(t *testing.T) {
	client := NewScalewayRunProjectWithDoer("https://api.test", func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusPreconditionFailed, `{"message":"resource_still_in_use"}`), nil
	})

	err := client.Delete(context.Background(), "secret", "proj-1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "http 412")
	assert.Contains(t, err.Error(), "resource_still_in_use",
		"this is the D6 signature and must reach the operator")
}

func TestDeleteRefusesWithoutCredentialsOrProject(t *testing.T) {
	client := NewScalewayRunProjectWithDoer("https://api.test", func(*http.Request) (*http.Response, error) {
		t.Fatal("must not reach the API")
		return nil, nil
	})

	require.Error(t, client.Delete(context.Background(), "", "proj-1"))
	require.Error(t, client.Delete(context.Background(), "secret", ""))
}

func listResponse(projects string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"projects":[` + projects + `]}`)),
	}
}

func project(id, name, description string) string {
	return `{"id":"` + id + `","name":"` + name + `","description":"` + description + `"}`
}

// List returns the whole organization, unfiltered. Filtering here would
// only return projects matching something we already knew to ask for,
// which is the assumption reconciliation exists to check.
func TestListReturnsEveryProjectIncludingOnesNotOurs(t *testing.T) {
	var gotURL string
	client := NewScalewayRunProjectWithDoer("https://api.example", func(r *http.Request) (*http.Response, error) {
		gotURL = r.URL.String()
		return listResponse(
			project("p1", "if-run-a", RunProjectDescription) + "," +
				project("p2", "openclaw", "our real project")), nil
	})

	got, err := client.List(context.Background(), "secret", "org-1")
	require.NoError(t, err)

	require.Len(t, got, 2)
	assert.True(t, got[0].Provenance().IsInfrafactoryRunProject())
	assert.False(t, got[1].Provenance().IsInfrafactoryRunProject(),
		"the caller decides what is ours, using the stamp that guards teardown")
	assert.Contains(t, gotURL, "organization_id=org-1")
}

// A short page is the last page. Stopping on an empty one instead would
// spend an extra request against a rate-limited API on every call.
func TestListStopsOnAShortPage(t *testing.T) {
	pages := 0
	client := NewScalewayRunProjectWithDoer("https://api.example", func(*http.Request) (*http.Response, error) {
		pages++
		return listResponse(project("p1", "if-run-a", RunProjectDescription)), nil
	})

	got, err := client.List(context.Background(), "secret", "org-1")
	require.NoError(t, err)

	assert.Len(t, got, 1)
	assert.Equal(t, 1, pages)
}

// Refusing beats truncating. A caller comparing a partial list against
// the live store would read the missing projects as "nothing
// unaccounted for" -- the precise falsehood reconciliation prevents.
func TestListRefusesToReportAPartialEstate(t *testing.T) {
	full := make([]string, 0, projectListPageSize)
	for i := 0; i < projectListPageSize; i++ {
		full = append(full, project(fmt.Sprintf("p%d", i), "if-run-a", RunProjectDescription))
	}
	body := strings.Join(full, ",")

	client := NewScalewayRunProjectWithDoer("https://api.example", func(*http.Request) (*http.Response, error) {
		return listResponse(body), nil
	})

	_, err := client.List(context.Background(), "secret", "org-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to report a partial estate")
}

// An unreachable API is an error, never an empty organization.
func TestListFailsRatherThanReportingAnEmptyOrganization(t *testing.T) {
	client := NewScalewayRunProjectWithDoer("https://api.example", func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusForbidden,
			Body:       io.NopCloser(strings.NewReader(`{"message":"denied"}`)),
		}, nil
	})

	_, err := client.List(context.Background(), "secret", "org-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "403")
}

func TestListRefusesWithoutCredentials(t *testing.T) {
	client := NewScalewayRunProjectWithDoer("https://api.example", func(*http.Request) (*http.Response, error) {
		t.Fatal("must not reach the API without credentials")
		return nil, nil
	})

	_, err := client.List(context.Background(), "", "org-1")
	require.Error(t, err)

	_, err = client.List(context.Background(), "secret", "")
	require.Error(t, err)
}

// A 412 is a TIMING answer and is marked as one.
//
// Scaleway checks whether a project still holds resources before
// deleting it, and that check lags its own deletions: immediately after
// a successful destroy it answers `precondition is not respected` and
// tells you to retry. Observed 2026-09-07 on a project verified empty by
// hand — it refused for about twenty minutes, then deleted with nothing
// else changed.
//
// The sentinel is keyed on the STATUS, deliberately. Matching the
// message text would break the first time Scaleway rewords it.
func TestDeleteMarksAPreconditionFailureAsNotYet(t *testing.T) {
	p := NewScalewayRunProjectWithDoer("https://api.example",
		func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusPreconditionFailed,
				Body: io.NopCloser(strings.NewReader(
					`{"message":"precondition is not respected","precondition":"resource_not_usable"}`)),
			}, nil
		})

	err := p.Delete(context.Background(), "sk", "proj-1")

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrRunProjectNotYetDeletable),
		"a 412 means wait, not that something is broken")
	assert.Contains(t, err.Error(), "proj-1", "and it still names the project")
}

// Every other failure stays an ordinary failure.
//
// Without this the sentinel could widen to swallow real errors, which is
// the opposite of what it is for: a genuine block must not be reported
// as "try again shortly".
func TestDeleteDoesNotMarkOtherFailuresAsNotYet(t *testing.T) {
	for name, status := range map[string]int{
		"forbidden": http.StatusForbidden,
		"conflict":  http.StatusConflict,
		"server":    http.StatusInternalServerError,
	} {
		t.Run(name, func(t *testing.T) {
			p := NewScalewayRunProjectWithDoer("https://api.example",
				func(*http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: status,
						Body:       io.NopCloser(strings.NewReader(`{"message":"nope"}`)),
					}, nil
				})

			err := p.Delete(context.Background(), "sk", "proj-1")

			require.Error(t, err)
			assert.False(t, errors.Is(err, ErrRunProjectNotYetDeletable),
				"only 412 is a timing answer")
		})
	}
}
