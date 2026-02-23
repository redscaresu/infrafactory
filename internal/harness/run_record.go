package harness

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/redscaresu/infrafactory/internal/runstore"
)

func PersistDestroyRun(
	store *runstore.FilesystemStore,
	scenario string,
	runID string,
	startedAt time.Time,
	destroyResult *DestroyResult,
	runErr error,
) error {
	status := "success"
	if runErr != nil {
		status = "failed"
	}

	if err := store.WriteRunMetadata(runstore.RunMetadata{
		Schema:    runstore.RunMetadataSchemaVersion,
		Scenario:  scenario,
		RunID:     runID,
		Status:    status,
		StartedAt: startedAt,
	}); err != nil {
		return fmt.Errorf("write run metadata: %w", err)
	}

	if destroyResult != nil && len(destroyResult.StateSnapshot) > 0 {
		if err := store.WriteIterationArtifact(scenario, runID, 1, "destroy_state.json", destroyResult.StateSnapshot); err != nil {
			return fmt.Errorf("write destroy state artifact: %w", err)
		}
	}

	if runErr != nil {
		payload, err := json.Marshal(map[string]string{
			"error": runErr.Error(),
		})
		if err != nil {
			return fmt.Errorf("encode run failure artifact: %w", err)
		}
		if err := store.WriteIterationArtifact(scenario, runID, 1, "failures.json", payload); err != nil {
			return fmt.Errorf("write run failure artifact: %w", err)
		}
	}

	return nil
}
