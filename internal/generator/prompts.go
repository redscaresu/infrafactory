package generator

import (
	"os"
	"path/filepath"
	"strings"
)

// resolvePromptTemplatePath returns the full path to a phase prompt
// template, preferring a cloud-specific subdirectory when one exists.
// Search order:
//
//  1. <promptsDir>/<cloud>/<fileName>  — preferred when cloud is set and
//     the cloud-specific template exists.
//  2. <promptsDir>/<fileName>          — legacy fallback for callers
//     (notably tests) that still write phase templates directly into
//     promptsDir without a cloud subdirectory.
//
// Returning the legacy path when neither candidate exists keeps the
// existing "missing template" error behaviour intact at the caller's
// next ReadFile.
func resolvePromptTemplatePath(promptsDir, cloud, fileName string) string {
	if strings.TrimSpace(cloud) != "" {
		cloudPath := filepath.Join(promptsDir, cloud, fileName)
		if _, err := os.Stat(cloudPath); err == nil {
			return cloudPath
		}
	}
	return filepath.Join(promptsDir, fileName)
}
