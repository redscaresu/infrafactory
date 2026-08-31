package harness

import (
	"context"
	"encoding/json"
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
