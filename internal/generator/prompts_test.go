package generator

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePromptTemplatePath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	scwDir := filepath.Join(dir, "scaleway")
	if err := os.MkdirAll(scwDir, 0o755); err != nil {
		t.Fatalf("mkdir scaleway: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scwDir, "phase1.md"), []byte("scaleway"), 0o644); err != nil {
		t.Fatalf("write scaleway phase1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "phase2.md"), []byte("legacy"), 0o644); err != nil {
		t.Fatalf("write legacy phase2: %v", err)
	}

	for _, tc := range []struct {
		name      string
		cloud     string
		fileName  string
		wantPath  string
		wantBytes string
	}{
		{
			name:      "cloud-specific file exists",
			cloud:     "scaleway",
			fileName:  "phase1.md",
			wantPath:  filepath.Join(scwDir, "phase1.md"),
			wantBytes: "scaleway",
		},
		{
			name:      "cloud-specific file missing falls back to legacy",
			cloud:     "scaleway",
			fileName:  "phase2.md",
			wantPath:  filepath.Join(dir, "phase2.md"),
			wantBytes: "legacy",
		},
		{
			name:      "empty cloud uses legacy directly",
			cloud:     "",
			fileName:  "phase2.md",
			wantPath:  filepath.Join(dir, "phase2.md"),
			wantBytes: "legacy",
		},
		{
			name:     "cloud-specific path returned even when neither file exists",
			cloud:    "gcp",
			fileName: "phase1.md",
			// gcp directory doesn't exist; we still return the legacy path
			// so the existing "missing template" error fires at the
			// caller's next ReadFile.
			wantPath: filepath.Join(dir, "phase1.md"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := resolvePromptTemplatePath(dir, tc.cloud, tc.fileName)
			if got != tc.wantPath {
				t.Fatalf("expected %s, got %s", tc.wantPath, got)
			}
			if tc.wantBytes != "" {
				data, err := os.ReadFile(got)
				if err != nil {
					t.Fatalf("read resolved path: %v", err)
				}
				if string(data) != tc.wantBytes {
					t.Fatalf("expected %q, got %q", tc.wantBytes, string(data))
				}
			}
		})
	}
}
