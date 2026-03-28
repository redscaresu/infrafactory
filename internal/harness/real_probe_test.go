package harness

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRealProbeHarnessConnectivityAndHTTP(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	lbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer lbServer.Close()
	lbHost, lbPort := splitHostPort(t, strings.TrimPrefix(lbServer.URL, "http://"))

	tcpListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp: %v", err)
	}
	defer tcpListener.Close()
	go func() {
		conn, acceptErr := tcpListener.Accept()
		if acceptErr == nil {
			_ = conn.Close()
		}
	}()
	dbHost, dbPort := splitHostPort(t, tcpListener.Addr().String())

	writeLiveState(t, workDir, `{
  "resources": [
    {
      "type": "scaleway_lb_ip",
      "instances": [{"attributes": {"ip_address": "`+lbHost+`"}}]
    },
    {
      "type": "scaleway_rdb_instance",
      "instances": [{"attributes": {"endpoint_ip": "`+dbHost+`"}}]
    }
  ]
}`)

	h := NewRealProbeHarness(ProbeConfig{Timeout: time.Second, Retries: 1})
	result, err := h.Run(context.Background(), workDir, "demo", []ProbeCheck{
		{Type: "http_probe", Target: "load_balancer", Port: lbPort, Expect: "reachable"},
		{Type: "connectivity", To: "database", Port: dbPort, Expect: "success"},
	})
	if err != nil {
		t.Fatalf("run real probes: %v", err)
	}
	if len(result.Failures) != 0 {
		t.Fatalf("expected no failures, got %+v", result.Failures)
	}
}

func TestRealProbeHarnessDNSAndFailures(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	writeLiveState(t, workDir, `{"resources":[]}`)

	h := NewRealProbeHarness(ProbeConfig{Timeout: time.Second, Retries: 1})
	h.lookup = func(_ context.Context, host string) ([]string, error) {
		switch host {
		case "demo.example.com":
			return []string{"203.0.113.10"}, nil
		default:
			return nil, errors.New("not found")
		}
	}

	result, err := h.Run(context.Background(), workDir, "demo", []ProbeCheck{
		{Type: "dns_resolution", Domain: "{{scenario_name}}.example.com", Expect: "resolves"},
		{Type: "dns_resolution", Domain: "missing.example.com", Expect: "not_resolves"},
		{Type: "connectivity", To: "database", Port: 5432, Expect: "success"},
	})
	if err != nil {
		t.Fatalf("run real probes: %v", err)
	}
	if len(result.Failures) != 1 {
		t.Fatalf("expected one failure, got %+v", result.Failures)
	}
	if result.Failures[0].Check != "connectivity" {
		t.Fatalf("expected connectivity failure, got %+v", result.Failures[0])
	}
}

func TestRealProbeHarnessRejectsInvalidPorts(t *testing.T) {
	t.Parallel()

	h := NewRealProbeHarness(ProbeConfig{Timeout: time.Second, Retries: 1})

	for _, tc := range []struct {
		name string
		run  func() error
	}{
		{
			name: "connectivity zero",
			run: func() error {
				return h.runConnectivityProbe(context.Background(), "127.0.0.1", 0, "success")
			},
		},
		{
			name: "connectivity too high",
			run: func() error {
				return h.runConnectivityProbe(context.Background(), "127.0.0.1", 65536, "success")
			},
		},
		{
			name: "http zero",
			run: func() error {
				return h.runHTTPProbe(context.Background(), "127.0.0.1", 0, "reachable")
			},
		},
		{
			name: "http too high",
			run: func() error {
				return h.runHTTPProbe(context.Background(), "127.0.0.1", 65536, "reachable")
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); err == nil {
				t.Fatal("expected invalid port error")
			}
		})
	}
}

func TestRealProbeHarnessTreatsEmptyDNSResponsesDefensively(t *testing.T) {
	t.Parallel()

	h := NewRealProbeHarness(ProbeConfig{Timeout: time.Second, Retries: 1})
	h.lookup = func(_ context.Context, host string) ([]string, error) {
		switch host {
		case "empty.example.com":
			return []string{}, nil
		default:
			return nil, fmt.Errorf("unexpected lookup %q", host)
		}
	}

	if err := h.runDNSProbe(context.Background(), "empty.example.com", "resolves"); err == nil {
		t.Fatal("expected resolves probe to fail on empty DNS response")
	}
	if err := h.runDNSProbe(context.Background(), "empty.example.com", "not_resolves"); err != nil {
		t.Fatalf("expected not_resolves probe to accept empty DNS response, got %v", err)
	}
}

func writeLiveState(t *testing.T, workDir, payload string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(workDir, LiveStateFilename), []byte(payload), 0o644); err != nil {
		t.Fatalf("write live state: %v", err)
	}
}

func splitHostPort(t *testing.T, address string) (string, int) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatalf("split host port %q: %v", address, err)
	}
	port, err := net.LookupPort("tcp", portStr)
	if err != nil {
		t.Fatalf("lookup port %q: %v", portStr, err)
	}
	return host, port
}
