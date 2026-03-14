package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/redscaresu/infrafactory/internal/config"
)

type fakeFormatter struct {
	formatted []byte
	err       error
	calls     int
}

func (f *fakeFormatter) Format(context.Context, string, []byte) ([]byte, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.formatted, nil
}

func TestOutputHandlersListAndRead(t *testing.T) {
	t.Parallel()

	outputRoot := filepath.Join(t.TempDir(), "output")
	scenarioDir := filepath.Join(outputRoot, "web-app-paris")
	if err := os.MkdirAll(filepath.Join(scenarioDir, ".terraform"), 0o755); err != nil {
		t.Fatalf("mkdir output: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scenarioDir, "main.tf"), []byte("terraform {}"), 0o644); err != nil {
		t.Fatalf("write main.tf: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scenarioDir, "terraform.tfstate"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write tfstate: %v", err)
	}

	cfg := config.Default()
	cfg.Paths.Output = outputRoot
	srv := NewServer(ServerConfig{Config: cfg})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	listResp, err := http.Get(ts.URL + "/api/output/web-app-paris")
	if err != nil {
		t.Fatalf("get output list: %v", err)
	}
	body, _ := io.ReadAll(listResp.Body)
	listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", listResp.StatusCode)
	}
	if !strings.Contains(string(body), "main.tf") {
		t.Fatalf("expected main.tf in response: %s", string(body))
	}
	if strings.Contains(string(body), "tfstate") {
		t.Fatalf("expected state files filtered out: %s", string(body))
	}

	fileResp, err := http.Get(ts.URL + "/api/output/web-app-paris/main.tf")
	if err != nil {
		t.Fatalf("get output file: %v", err)
	}
	fileBody, _ := io.ReadAll(fileResp.Body)
	fileResp.Body.Close()
	if fileResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", fileResp.StatusCode)
	}
	if !strings.Contains(string(fileBody), "terraform") {
		t.Fatalf("unexpected output file body: %s", string(fileBody))
	}
}

func TestOutputHandlersRejectTraversal(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Paths.Output = filepath.Join(t.TempDir(), "output")
	srv := NewServer(ServerConfig{Config: cfg})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/output/web-app-paris/%2e%2e/%2e%2e/etc/passwd")
	if err != nil {
		t.Fatalf("get traversal path: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestOutputHandlersFormatOnReadAndFallbackRawWhenFormatterUnavailable(t *testing.T) {
	t.Parallel()

	outputRoot := filepath.Join(t.TempDir(), "output")
	scenarioDir := filepath.Join(outputRoot, "web-app-paris")
	if err := os.MkdirAll(scenarioDir, 0o755); err != nil {
		t.Fatalf("mkdir output: %v", err)
	}
	raw := []byte("resource \"x\" \"y\"{}")
	if err := os.WriteFile(filepath.Join(scenarioDir, "main.tf"), raw, 0o644); err != nil {
		t.Fatalf("write main.tf: %v", err)
	}

	cfg := config.Default()
	cfg.Paths.Output = outputRoot
	formatter := &fakeFormatter{formatted: []byte("resource \"x\" \"y\" {}\n")}
	srv := NewServer(ServerConfig{Config: cfg, Formatter: formatter})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/output/web-app-paris/main.tf?format=1")
	if err != nil {
		t.Fatalf("get formatted output file: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if string(body) != string(formatter.formatted) {
		t.Fatalf("expected formatted output, got %q", string(body))
	}
	if formatter.calls != 1 {
		t.Fatalf("expected formatter call, got %d", formatter.calls)
	}

	rawResp, err := http.Get(ts.URL + "/api/output/web-app-paris/main.tf")
	if err != nil {
		t.Fatalf("get raw output file: %v", err)
	}
	rawBody, _ := io.ReadAll(rawResp.Body)
	rawResp.Body.Close()
	if string(rawBody) != string(raw) {
		t.Fatalf("expected raw output without query, got %q", string(rawBody))
	}
}
