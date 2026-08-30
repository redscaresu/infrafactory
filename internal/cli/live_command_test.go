package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/livestore"
)

func liveTestDeployment() livestore.Deployment {
	created := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	return livestore.Deployment{
		ID:        "web-live-paris-001",
		Scenario:  "web-live-paris",
		ProjectID: "11111111-2222-3333-4444-555555555555",
		Address:   "51.15.0.1",
		Image:     "nginx",
		Tag:       "1.27",
		State:     livestore.StateLive,
		CreatedAt: created,
		ExpiresAt: created.Add(4 * time.Hour),
	}
}

// newLiveListTestCmd builds a command carrying the same --output flag the
// real root command installs, so the rendering paths are exercised through
// the contract callers actually use.
func newLiveListTestCmd(t *testing.T, root string, out *bytes.Buffer) *cobra.Command {
	t.Helper()

	cmd := &cobra.Command{Use: "ls"}
	cmd.Flags().String("output", string(OutputModeHuman), "")
	cmd.SetOut(out)
	cmd.RunE = func(c *cobra.Command, args []string) error {
		return runLiveListCommand(c, args, &CommandRuntime{livestoreRoot: root})
	}

	return cmd
}

func TestLiveListReportsAnEmptyStore(t *testing.T) {
	var out bytes.Buffer
	cmd := newLiveListTestCmd(t, t.TempDir(), &out)

	require.NoError(t, cmd.Execute())
	assert.Contains(t, out.String(), "No live deployments.")
}

func TestLiveListRendersADeployment(t *testing.T) {
	root := t.TempDir()
	store := livestore.NewFilesystemStore(root)
	require.NoError(t, store.Put(liveTestDeployment()))

	var out bytes.Buffer
	cmd := newLiveListTestCmd(t, root, &out)
	require.NoError(t, cmd.Execute())

	rendered := out.String()
	assert.Contains(t, rendered, "web-live-paris-001")
	assert.Contains(t, rendered, "nginx:1.27", "image and tag render together")
	assert.Contains(t, rendered, "11111111-2222-3333-4444-555555555555")
	assert.Contains(t, rendered, "51.15.0.1")
}

func TestLiveListJSONShape(t *testing.T) {
	root := t.TempDir()
	store := livestore.NewFilesystemStore(root)
	require.NoError(t, store.Put(liveTestDeployment()))

	var out bytes.Buffer
	cmd := newLiveListTestCmd(t, root, &out)
	require.NoError(t, cmd.Flags().Set("output", string(OutputModeJSON)))
	require.NoError(t, cmd.Execute())

	var payload liveListJSON
	require.NoError(t, json.Unmarshal(out.Bytes(), &payload))

	assert.Equal(t, "infrafactory.live.list.v1", payload.Schema)
	require.Len(t, payload.Deployments, 1)
	assert.Equal(t, "web-live-paris-001", payload.Deployments[0].ID)
	assert.Equal(t, "nginx:1.27", payload.Deployments[0].ImageWithTag)
	assert.Empty(t, payload.Unreadable)
}

// The listing must not report success when it could not account for
// every record: "we could not check" and "nothing is running" have to
// look different, and cost different amounts.
func TestLiveListFailsWhenARecordCannotBeRead(t *testing.T) {
	root := t.TempDir()
	store := livestore.NewFilesystemStore(root)
	require.NoError(t, store.Put(liveTestDeployment()))
	require.NoError(t, os.WriteFile(filepath.Join(root, "mystery.json"), []byte("{truncated"), 0o644))

	var out bytes.Buffer
	cmd := newLiveListTestCmd(t, root, &out)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	err := cmd.Execute()

	require.Error(t, err)
	var cliErr *CLIError
	require.True(t, errors.As(err, &cliErr))
	assert.Contains(t, err.Error(), "may describe running infrastructure")
	assert.Contains(t, out.String(), "mystery", "and the operator is told which record")
	assert.Contains(t, out.String(), "web-live-paris-001", "readable records still render")
}

func TestTTLLabelDistinguishesLiveExpiredAndReleased(t *testing.T) {
	d := liveTestDeployment()

	assert.Equal(t, "1h0m0s", ttlLabel(d, d.ExpiresAt.Add(-time.Hour)))
	assert.Equal(t, "EXPIRED", ttlLabel(d, d.ExpiresAt.Add(time.Hour)),
		"expired must not render as 0s: the operator's next action differs")

	released := d
	released.State = livestore.StateReleased
	assert.Equal(t, "-", ttlLabel(released, d.ExpiresAt.Add(time.Hour)))
}

func TestImageRefHandlesMissingFields(t *testing.T) {
	assert.Equal(t, "nginx:1.27", imageRef(livestore.Deployment{Image: "nginx", Tag: "1.27"}))
	assert.Equal(t, "nginx", imageRef(livestore.Deployment{Image: "nginx"}))
	assert.Equal(t, "-", imageRef(livestore.Deployment{}))
}

func TestResolveLiveStoreRootPrefersTheEnvOverride(t *testing.T) {
	t.Setenv("INFRAFACTORY_LIVESTORE_ROOT", "/tmp/somewhere-else")
	assert.Equal(t, "/tmp/somewhere-else", resolveLiveStoreRoot())

	t.Setenv("INFRAFACTORY_LIVESTORE_ROOT", "")
	assert.Equal(t, livestore.DefaultRoot, resolveLiveStoreRoot())
}

// The live store must be reachable from the command runtime the same way
// the run store is, and must honour a test override so no test can read
// or write the operator's real record of what is running.
func TestLiveStoreRootHonoursTheRuntimeOverride(t *testing.T) {
	runtime := &CommandRuntime{livestoreRoot: "/tmp/isolated-live"}
	assert.Equal(t, "/tmp/isolated-live", runtime.LiveStoreRoot())

	t.Setenv("INFRAFACTORY_LIVESTORE_ROOT", "/tmp/from-env")
	assert.Equal(t, "/tmp/from-env", (&CommandRuntime{}).LiveStoreRoot())
}

func TestLiveCommandIsRegisteredOnRoot(t *testing.T) {
	cfg := &rootConfig{}
	live := newLiveCmd(cfg)

	assert.Equal(t, "live", live.Name())
	names := make([]string, 0, len(live.Commands()))
	for _, sub := range live.Commands() {
		names = append(names, sub.Name())
	}
	assert.Contains(t, names, "ls")
}
