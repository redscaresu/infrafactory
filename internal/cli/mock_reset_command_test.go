package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/spf13/cobra"
)

// TestMockResetCommandFansOutAcrossBackends pins the S67 wiring: a
// single `infrafactory mock reset` invocation hits the configured
// mockway + fakegcp + fakeaws endpoints AND walks SeaweedFS's bucket
// listing for the s3 cascade. The hit-count assertion is the
// regression for the S54 SeaweedFS state-leak.
func TestMockResetCommandFansOutAcrossBackends(t *testing.T) {
	t.Parallel()

	var mockwayCalls, fakegcpCalls, fakeawsCalls, s3ListCalls int32
	mockwaySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/mock/reset" {
			atomic.AddInt32(&mockwayCalls, 1)
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer mockwaySrv.Close()

	fakegcpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/mock/reset" {
			atomic.AddInt32(&fakegcpCalls, 1)
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer fakegcpSrv.Close()

	fakeawsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/mock/reset" {
			atomic.AddInt32(&fakeawsCalls, 1)
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer fakeawsSrv.Close()

	// SeaweedFS-shaped S3 server: GET / lists buckets (empty list is
	// fine — resetS3Backend's iteration loop runs zero times but the
	// list request still counts as the s3 cascade firing).
	s3Srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/" {
			atomic.AddInt32(&s3ListCalls, 1)
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<ListAllMyBucketsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Buckets></Buckets></ListAllMyBucketsResult>`))
			return
		}
		http.NotFound(w, r)
	}))
	defer s3Srv.Close()

	workspace := t.TempDir()
	configPath := filepath.Join(workspace, "infrafactory.yaml")
	mustWriteFile(t, configPath, `version: "1.0"
agent:
  type: claude-code
mockway:
  url: `+mockwaySrv.URL+`
fakegcp:
  url: `+fakegcpSrv.URL+`
fakeaws:
  url: `+fakeawsSrv.URL+`
s3:
  url: `+s3Srv.URL+`
`)

	opts := runtimeOptions{
		configLoader:   config.Load,
		scenarioLoader: defaultScenarioLoader,
	}
	cmd := &cobra.Command{
		Use:  "reset",
		RunE: withRuntimeWithOptions("mock reset", opts, runMockResetCommand),
	}
	cmd.Flags().String("config", config.DefaultPath, "")
	cmd.Flags().String("output", string(OutputModeHuman), "")
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--config", configPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute mock reset: %v", err)
	}

	if got := atomic.LoadInt32(&mockwayCalls); got != 1 {
		t.Errorf("mockway reset calls = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&fakegcpCalls); got != 1 {
		t.Errorf("fakegcp reset calls = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&fakeawsCalls); got != 1 {
		t.Errorf("fakeaws reset calls = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&s3ListCalls); got != 1 {
		t.Errorf("s3 list-buckets calls = %d, want 1 (s3 cascade should fire)", got)
	}
	if !strings.Contains(stdout.String(), "- mock/reset: pass (reset mockway+fakegcp+fakeaws+s3)") {
		t.Fatalf("expected fan-out summary in output, got:\n%s", stdout.String())
	}
}

// TestMockResetCommandWithoutS3SkipsCascade pins that the s3 cascade
// is skipped (no GET / call) when no s3 URL is configured — relevant
// for Scaleway- and GCP-only deployments.
func TestMockResetCommandWithoutS3SkipsCascade(t *testing.T) {
	t.Parallel()

	var mockwayCalls int32
	mockwaySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/mock/reset" {
			atomic.AddInt32(&mockwayCalls, 1)
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer mockwaySrv.Close()

	workspace := t.TempDir()
	configPath := filepath.Join(workspace, "infrafactory.yaml")
	mustWriteFile(t, configPath, `version: "1.0"
agent:
  type: claude-code
mockway:
  url: `+mockwaySrv.URL+`
`)

	opts := runtimeOptions{
		configLoader:   config.Load,
		scenarioLoader: defaultScenarioLoader,
	}
	cmd := &cobra.Command{
		Use:  "reset",
		RunE: withRuntimeWithOptions("mock reset", opts, runMockResetCommand),
	}
	cmd.Flags().String("config", config.DefaultPath, "")
	cmd.Flags().String("output", string(OutputModeHuman), "")
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--config", configPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute mock reset: %v", err)
	}
	if got := atomic.LoadInt32(&mockwayCalls); got != 1 {
		t.Errorf("mockway reset calls = %d, want 1", got)
	}
	if !strings.Contains(stdout.String(), "- mock/reset: pass (reset mockway)") {
		t.Fatalf("expected mockway-only summary in output, got:\n%s", stdout.String())
	}
}
