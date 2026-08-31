package harness

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	markerOrgID     = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	markerProjectID = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
)

func stampedProvenance() ProjectProvenance {
	return ProjectProvenance{Exists: true, Name: "if-run-block-paris-x", Description: RunProjectDescription}
}

func TestMarkerRoundTrips(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, WriteRunProjectMarker(dir, RunProject{ID: markerProjectID, Name: "if-run-x"}))

	marker, err := ReadRunProjectMarker(dir)

	require.NoError(t, err)
	assert.Equal(t, markerProjectID, marker.ProjectID)
	assert.Equal(t, "if-run-x", marker.Name)
}

func TestMarkerIsWrittenAtomically(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, WriteRunProjectMarker(dir, RunProject{ID: markerProjectID, Name: "if-run-x"}))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "the staging file is renamed or removed, never left behind")
	assert.Equal(t, RunProjectMarkerFilename, entries[0].Name())
}

// "No witness" must never read as "no project" — the caller has to be
// unable to mistake one for the other.
func TestReadMarkerRefusesMissingEmptyAndCorrupt(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		_, err := ReadRunProjectMarker(t.TempDir())
		require.Error(t, err)
	})

	t.Run("corrupt", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, RunProjectMarkerFilename), []byte("{trunc"), 0o600))
		_, err := ReadRunProjectMarker(dir)
		require.Error(t, err)
	})

	t.Run("no project id", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, RunProjectMarkerFilename), []byte(`{"name":"if-run-x"}`), 0o600))
		_, err := ReadRunProjectMarker(dir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "records no project id")
	})
}

func TestAssertRunProjectDeletableAcceptsTheRunsOwnStampedProject(t *testing.T) {
	marker := RunProjectMarker{ProjectID: markerProjectID, Name: "if-run-block-paris-x"}

	assert.NoError(t, AssertRunProjectDeletable(marker, markerProjectID, markerOrgID, stampedProvenance()))
}

// The guard that matters most: the organization default holds real
// infrastructure, and its id equals the organization id.
func TestAssertRunProjectDeletableRefusesTheOrganizationDefault(t *testing.T) {
	marker := RunProjectMarker{ProjectID: markerOrgID, Name: "if-run-x"}

	err := AssertRunProjectDeletable(marker, markerOrgID, markerOrgID, stampedProvenance())

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrProtectedProject)
	assert.Contains(t, err.Error(), "organization's default project")
}

// Identity: without a marker there is nothing saying this run created it.
func TestAssertRunProjectDeletableRefusesWithoutAMarker(t *testing.T) {
	err := AssertRunProjectDeletable(RunProjectMarker{}, markerProjectID, markerOrgID, stampedProvenance())

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrProtectedProject)
	assert.Contains(t, err.Error(), "this run did not create it")
}

// The argument must not win over the record — the same property that made
// reap pass the state-derived id as both arguments.
func TestAssertRunProjectDeletableRefusesATargetTheMarkerDoesNotName(t *testing.T) {
	marker := RunProjectMarker{ProjectID: markerProjectID, Name: "if-run-x"}

	err := AssertRunProjectDeletable(marker, "cccccccc-cccc-cccc-cccc-cccccccccccc", markerOrgID, stampedProvenance())

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrProtectedProject)
	assert.Contains(t, err.Error(), "does not match")
}

// The half a forged marker cannot defeat: an unstamped project is
// somebody's real infrastructure, whatever the local file claims.
func TestAssertRunProjectDeletableRefusesAnUnstampedProject(t *testing.T) {
	marker := RunProjectMarker{ProjectID: markerProjectID, Name: "if-run-x"}

	cases := map[string]ProjectProvenance{
		"wrong name":        {Exists: true, Name: "openclaw", Description: RunProjectDescription},
		"wrong description": {Exists: true, Name: "if-run-x", Description: "something else"},
		"neither":           {Exists: true, Name: "production", Description: "our real project"},
	}

	for name, prov := range cases {
		t.Run(name, func(t *testing.T) {
			err := AssertRunProjectDeletable(marker, markerProjectID, markerOrgID, prov)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrProtectedProject)
			assert.Contains(t, err.Error(), "does not carry infrafactory's stamp")
		})
	}
}

// Already gone is the outcome asked for.
func TestAssertRunProjectDeletableAllowsAnAlreadyDeletedProject(t *testing.T) {
	marker := RunProjectMarker{ProjectID: markerProjectID, Name: "if-run-x"}

	assert.NoError(t, AssertRunProjectDeletable(marker, markerProjectID, markerOrgID, ProjectProvenance{Exists: false}))
}

func TestIsInfrafactoryRunProject(t *testing.T) {
	assert.True(t, ProjectProvenance{Name: "if-run-a", Description: RunProjectDescription}.IsInfrafactoryRunProject())
	assert.False(t, ProjectProvenance{Name: "openclaw", Description: RunProjectDescription}.IsInfrafactoryRunProject())
	assert.False(t, ProjectProvenance{Name: "if-run-a", Description: ""}.IsInfrafactoryRunProject())
}

func TestDescribeReportsTheStamp(t *testing.T) {
	client := NewScalewayRunProjectWithDoer("https://api.test", func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK,
			`{"name":"if-run-block-paris-x","description":"`+RunProjectDescription+`"}`), nil
	})

	prov, err := client.Describe(context.Background(), "secret", markerProjectID)

	require.NoError(t, err)
	assert.True(t, prov.Exists)
	assert.True(t, prov.IsInfrafactoryRunProject())
}

// A 404 is a positive answer; anything else is an error, because an
// unreachable API must never look like "already deleted".
func TestDescribeDistinguishesGoneFromUnreachable(t *testing.T) {
	gone := NewScalewayRunProjectWithDoer("https://api.test", func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusNotFound, `{"message":"not found"}`), nil
	})
	prov, err := gone.Describe(context.Background(), "secret", markerProjectID)
	require.NoError(t, err)
	assert.False(t, prov.Exists, "gone is a fact the guard can act on")

	unreachable := NewScalewayRunProjectWithDoer("https://api.test", func(*http.Request) (*http.Response, error) {
		return nil, errors.New("dial tcp: connection refused")
	})
	_, err = unreachable.Describe(context.Background(), "secret", markerProjectID)
	require.Error(t, err, "an unreachable API is an error, never an absence")

	serverErr := NewScalewayRunProjectWithDoer("https://api.test", func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusInternalServerError, `{"message":"boom"}`), nil
	})
	_, err = serverErr.Describe(context.Background(), "secret", markerProjectID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "http 500")
}
