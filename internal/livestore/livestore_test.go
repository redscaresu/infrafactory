package livestore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validDeployment() Deployment {
	created := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	return Deployment{
		ID:        "web-live-paris-001",
		Scenario:  "web-live-paris",
		ProjectID: "11111111-2222-3333-4444-555555555555",
		Address:   "51.15.0.1",
		Image:     "nginx",
		Tag:       "1.27",
		CreatedAt: created,
		ExpiresAt: created.Add(4 * time.Hour),
	}
}

func TestValidateAcceptsAWellFormedDeployment(t *testing.T) {
	require.NoError(t, validDeployment().Validate())
}

func TestValidateRejectsRecordsThatCouldNotBeReaped(t *testing.T) {
	created := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)

	cases := map[string]struct {
		mutate  func(*Deployment)
		wantMsg string
	}{
		"no id":         {func(d *Deployment) { d.ID = "" }, "id is required"},
		"blank id":      {func(d *Deployment) { d.ID = "   " }, "id is required"},
		"no scenario":   {func(d *Deployment) { d.Scenario = "" }, "scenario is required"},
		"no project":    {func(d *Deployment) { d.ProjectID = "" }, "project_id is required"},
		"no created_at": {func(d *Deployment) { d.CreatedAt = time.Time{} }, "created_at is required"},
		"no expiry":     {func(d *Deployment) { d.ExpiresAt = time.Time{} }, "expires_at is required"},
		"expiry before created": {
			func(d *Deployment) { d.ExpiresAt = created.Add(-time.Hour) },
			"must be after created_at",
		},
		"expiry equals created": {
			func(d *Deployment) { d.ExpiresAt = d.CreatedAt },
			"must be after created_at",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			d := validDeployment()
			tc.mutate(&d)
			err := d.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantMsg)
		})
	}
}

// The fail-closed rule the whole package rests on: a record with no expiry
// is expired, not immortal. If this ever inverts, a hand-edited or
// truncated record bills forever.
func TestExpiredTreatsAMissingExpiryAsExpired(t *testing.T) {
	d := validDeployment()
	d.ExpiresAt = time.Time{}

	assert.True(t, d.Expired(d.CreatedAt))
	assert.True(t, d.Reapable(d.CreatedAt))
	assert.Equal(t, time.Duration(0), d.TimeToLive(d.CreatedAt))
}

func TestExpiredBoundaries(t *testing.T) {
	d := validDeployment()

	assert.False(t, d.Expired(d.ExpiresAt.Add(-time.Second)), "a second before expiry is still live")
	assert.True(t, d.Expired(d.ExpiresAt), "expiry is inclusive: at the instant it expires, it is expired")
	assert.True(t, d.Expired(d.ExpiresAt.Add(time.Second)))
}

func TestTimeToLiveNeverGoesNegative(t *testing.T) {
	d := validDeployment()

	assert.Equal(t, time.Hour, d.TimeToLive(d.ExpiresAt.Add(-time.Hour)))
	assert.Equal(t, time.Duration(0), d.TimeToLive(d.ExpiresAt.Add(48*time.Hour)))
}

func TestReleasedDeploymentsAreNeverReaped(t *testing.T) {
	d := validDeployment()
	d.State = StateReleased

	assert.True(t, d.Expired(d.ExpiresAt.Add(time.Hour)), "still expired")
	assert.False(t, d.Reapable(d.ExpiresAt.Add(time.Hour)), "but nothing is left to reap")
}

func TestPutThenGetRoundTrips(t *testing.T) {
	store := NewFilesystemStore(t.TempDir())
	want := validDeployment()

	require.NoError(t, store.Put(want))

	got, err := store.Get(want.ID)
	require.NoError(t, err)

	assert.Equal(t, DeploymentSchemaVersion, got.Schema, "schema is stamped on write")
	assert.Equal(t, StateLive, got.State, "state defaults to live")
	assert.Equal(t, want.ProjectID, got.ProjectID)
	assert.Equal(t, want.Tag, got.Tag)
	assert.True(t, want.ExpiresAt.Equal(got.ExpiresAt))
}

func TestPutRefusesAnUnreapableRecord(t *testing.T) {
	store := NewFilesystemStore(t.TempDir())
	d := validDeployment()
	d.ExpiresAt = time.Time{}

	err := store.Put(d)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no unbounded form")
	_, statErr := os.Stat(filepath.Join(store.Root, d.ID+".json"))
	assert.True(t, os.IsNotExist(statErr), "a refused record must not be written")
}

func TestListReturnsNewestFirst(t *testing.T) {
	store := NewFilesystemStore(t.TempDir())
	base := validDeployment()

	older := base
	older.ID = "older"
	older.CreatedAt = base.CreatedAt.Add(-2 * time.Hour)
	older.ExpiresAt = older.CreatedAt.Add(time.Hour)

	require.NoError(t, store.Put(older))
	require.NoError(t, store.Put(base))

	got, unreadable, err := store.List()
	require.NoError(t, err)
	assert.Empty(t, unreadable)
	require.Len(t, got, 2)
	assert.Equal(t, base.ID, got[0].ID)
	assert.Equal(t, "older", got[1].ID)
}

func TestListOnAnAbsentStoreIsEmptyNotAnError(t *testing.T) {
	store := NewFilesystemStore(filepath.Join(t.TempDir(), "never-created"))

	got, unreadable, err := store.List()

	require.NoError(t, err)
	assert.Empty(t, got)
	assert.Empty(t, unreadable)
}

// A file the store cannot decode may still describe real infrastructure.
// Dropping it would turn a leak into an empty list, so it must surface.
func TestListSurfacesUnreadableRecordsRatherThanDroppingThem(t *testing.T) {
	store := NewFilesystemStore(t.TempDir())
	require.NoError(t, store.Put(validDeployment()))
	require.NoError(t, os.WriteFile(filepath.Join(store.Root, "corrupt.json"), []byte("{not json"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(store.Root, "notes.txt"), []byte("ignored"), 0o644))

	got, unreadable, err := store.List()

	require.NoError(t, err)
	require.Len(t, unreadable, 1, "the corrupt one is reported, not silently skipped")
	assert.Contains(t, unreadable[0].Error(), "corrupt")

	// And it comes back AS a deployment, so the reaper sees it. ADR-0024
	// rule 3 says unreadable means expired; a record that never entered
	// the set was not honouring that.
	require.Len(t, got, 2)
	byID := map[string]Deployment{}
	for _, d := range got {
		byID[d.ID] = d
	}
	assert.False(t, byID["web-live-paris-001"].Undecodable)
	corrupt := byID["corrupt"]
	assert.True(t, corrupt.Undecodable, "the unparseable record is surfaced as a deployment")
	assert.True(t, corrupt.Reapable(time.Now()), "and it is reapable, per ADR-0024 rule 3")
}

// The gap the previous test missed: an undecodable record could never be
// reaped (it never reached the set) and never released (MarkReleased
// decodes first), so it failed every pass forever with no way out.
func TestUndecodableRecordsAreReapableAndReleasable(t *testing.T) {
	store := NewFilesystemStore(t.TempDir())
	require.NoError(t, os.WriteFile(filepath.Join(store.Root, "truncated.json"), []byte(`{"id":"trunc`), 0o644))

	expired, unreadable, err := store.Reapable(time.Now())
	require.NoError(t, err)
	require.Len(t, unreadable, 1)
	require.Len(t, expired, 1, "an unreadable record reaches the reaper")
	assert.Equal(t, "truncated", expired[0].ID)

	require.NoError(t, store.MarkReleased("truncated"), "and a human who cleaned up by hand can clear it")

	after, _, err := store.Reapable(time.Now())
	require.NoError(t, err)
	assert.Empty(t, after, "released, so it stops failing every pass")
}

func TestNewFilesystemStoreResolvesRootToAbsolute(t *testing.T) {
	store := NewFilesystemStore(".infrafactory/live")
	assert.True(t, filepath.IsAbs(store.Root),
		"a relative root makes a reaper in another directory see an empty store and report nothing expired")
}

// Ids reach the filesystem via filepath.Join, and `live teardown <id>`
// takes one straight from the command line.
func TestValidateRejectsIDsThatEscapeTheStore(t *testing.T) {
	for _, id := range []string{"../evil", "a/b", "..", ".", "  padded", "x/../../y"} {
		t.Run(id, func(t *testing.T) {
			d := validDeployment()
			d.ID = id
			require.Error(t, d.Validate())
		})
	}
}

func TestPutWritesAtomically(t *testing.T) {
	store := NewFilesystemStore(t.TempDir())
	require.NoError(t, store.Put(validDeployment()))

	entries, err := os.ReadDir(store.Root)
	require.NoError(t, err)
	for _, e := range entries {
		assert.False(t, strings.HasSuffix(e.Name(), ".tmp"),
			"the staging file is renamed or removed, never left behind: %s", e.Name())
	}
	assert.Len(t, entries, 1)
}

func TestReapableSelectsExpiredAndUnreleased(t *testing.T) {
	store := NewFilesystemStore(t.TempDir())
	base := validDeployment()
	now := base.ExpiresAt.Add(time.Hour)

	stillLive := base
	stillLive.ID = "still-live"
	stillLive.ExpiresAt = now.Add(time.Hour)

	released := base
	released.ID = "released"
	released.State = StateReleased

	require.NoError(t, store.Put(base))
	require.NoError(t, store.Put(stillLive))
	require.NoError(t, store.Put(released))

	got, _, err := store.Reapable(now)

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, base.ID, got[0].ID)
}

// Regression: MarkReleased once routed through Put, so a record that
// failed Validate could not be marked released. The reaper would destroy
// its resources, fail to record that, and destroy them again next pass.
func TestMarkReleasedSucceedsOnARecordPutWouldRefuse(t *testing.T) {
	store := NewFilesystemStore(t.TempDir())
	d := validDeployment()
	require.NoError(t, store.Put(d))

	// Simulate a hand-edited or truncated record: no expiry, so Validate
	// rejects it, but it is reapable and its resources are real.
	damaged := d
	damaged.ExpiresAt = time.Time{}
	require.Error(t, damaged.Validate(), "precondition: Put would refuse this")
	require.NoError(t, store.write(damaged))

	require.NoError(t, store.MarkReleased(d.ID))

	got, err := store.Get(d.ID)
	require.NoError(t, err)
	assert.Equal(t, StateReleased, got.State)
	assert.False(t, got.Reapable(time.Now()), "and it is no longer reaped on every pass")
}

func TestMarkReleasedOnAnUnknownIDErrors(t *testing.T) {
	store := NewFilesystemStore(t.TempDir())

	err := store.MarkReleased("nope")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "read deployment nope")
}
