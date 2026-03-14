package api

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type ExternalIaCFormatter struct{}

func NewExternalIaCFormatter() *ExternalIaCFormatter {
	return &ExternalIaCFormatter{}
}

func (f *ExternalIaCFormatter) Format(ctx context.Context, filename string, content []byte) ([]byte, error) {
	tool, ok := firstAvailableTool("tofu", "terraform")
	if !ok {
		return content, nil
	}
	dir, err := os.MkdirTemp("", "infrafactory-fmt-*")
	if err != nil {
		return content, err
	}
	defer os.RemoveAll(dir)

	base := filepath.Base(filename)
	if base == "." || base == string(filepath.Separator) || base == "" {
		base = "main.tf"
	}
	path := filepath.Join(dir, base)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return content, err
	}

	cmd := exec.CommandContext(ctx, tool, "fmt", "-no-color", path)
	if err := cmd.Run(); err != nil {
		return content, nil
	}

	formatted, err := os.ReadFile(path)
	if err != nil {
		return content, err
	}
	return formatted, nil
}

func shouldFormatFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".tf") || strings.HasSuffix(lower, ".tfvars") || strings.HasSuffix(lower, ".hcl")
}

func shouldFormatRequest(r *http.Request, file string) bool {
	if r == nil || !shouldFormatFile(file) {
		return false
	}
	value := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("format")))
	return value == "1" || value == "true" || value == "yes"
}

func firstAvailableTool(candidates ...string) (string, bool) {
	for _, candidate := range candidates {
		if _, err := exec.LookPath(candidate); err == nil {
			return candidate, true
		}
	}
	return "", false
}
