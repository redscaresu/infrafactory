package harness

import (
	"context"
	"encoding/json"
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
