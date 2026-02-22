package runstore

import (
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func BenchmarkRunstoreWriteReadMetadata(b *testing.B) {
	store := NewFilesystemStore(filepath.Join(b.TempDir(), ".infrafactory", "runs"))
	base := time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		runID := "run-bench-" + strconv.Itoa(i)
		meta := RunMetadata{
			Schema:    RunMetadataSchemaVersion,
			Scenario:  "bench-scenario",
			RunID:     runID,
			Status:    "success",
			StartedAt: base,
		}
		if err := store.WriteRunMetadata(meta); err != nil {
			b.Fatalf("write run metadata: %v", err)
		}
		if _, err := store.ReadRunMetadata("bench-scenario", runID); err != nil {
			b.Fatalf("read run metadata: %v", err)
		}
	}
}
