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
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/scenario"
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
				_, err := h.runConnectivityProbe(context.Background(), "127.0.0.1", 0, "success")
				return err
			},
		},
		{
			name: "connectivity too high",
			run: func() error {
				_, err := h.runConnectivityProbe(context.Background(), "127.0.0.1", 65536, "success")
				return err
			},
		},
		{
			name: "http zero",
			run: func() error {
				_, err := h.runHTTPProbe(context.Background(), "127.0.0.1", 0, "reachable")
				return err
			},
		},
		{
			name: "http too high",
			run: func() error {
				_, err := h.runHTTPProbe(context.Background(), "127.0.0.1", 65536, "reachable")
				return err
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

	if _, err := h.runDNSProbe(context.Background(), "empty.example.com", "resolves"); err == nil {
		t.Fatal("expected resolves probe to fail on empty DNS response")
	}
	if _, err := h.runDNSProbe(context.Background(), "empty.example.com", "not_resolves"); err != nil {
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

// `blocked` is asked once. Retrying waits for a condition to arrive,
// which is right for `success` and meaningless here: an open port does
// not close itself while you wait. All the retry buys is the full
// window's delay before reporting what the first dial already knew --
// and the one place this runs is a holdout, where the value is a fast
// clear answer.
func TestConnectivityProbeAsksBlockedOnce(t *testing.T) {
	dials := 0
	h := NewRealProbeHarness(ProbeConfig{Timeout: time.Second, Retries: 60})
	h.dialFunc = func(context.Context, string, string) (net.Conn, error) {
		dials++
		// The port IS open, so `blocked` is not satisfied. The old
		// code retried this 60 times.
		return &net.TCPConn{}, nil
	}

	_, err := h.runConnectivityProbe(context.Background(), "203.0.113.1", 22, "blocked")

	require.Error(t, err, "an open port fails a blocked check")
	assert.Equal(t, 1, dials, "a blocked check that fails must report immediately, not after the full retry window")
}

// ...and `success` keeps retrying, because that one really is waiting
// for infrastructure to come up.
func TestConnectivityProbeStillRetriesSuccess(t *testing.T) {
	dials := 0
	h := NewRealProbeHarness(ProbeConfig{Timeout: time.Millisecond, Retries: 3})
	h.dialFunc = func(context.Context, string, string) (net.Conn, error) {
		dials++
		return nil, errors.New("connection refused")
	}

	_, err := h.runConnectivityProbe(context.Background(), "203.0.113.1", 80, "success")

	require.Error(t, err)
	assert.Equal(t, 3, dials, "waiting for a stack to come up is what retries are for")
}

// awsWebLiveState is a terraform state captured by applying
// internal/e2e/testdata/aws-web-step-one/web-step-one.tf on fakeaws.
const awsWebLiveState = "testdata/realprobe/aws/" + LiveStateFilename

func loadAWSWebLiveState(t *testing.T) (terraformState, map[string]any) {
	t.Helper()
	state, err := loadLiveTerraformState(awsWebLiveState)
	require.NoError(t, err)
	for _, resource := range state.Resources {
		if resource.Type == "aws_instance" {
			require.Len(t, resource.Instances, 1)
			return state, resource.Instances[0].Attributes
		}
	}
	t.Fatal("captured state has no aws_instance")
	return state, nil
}

// The holdout dials `compute`; on AWS that must be the instance's
// public_ip. private_ip is unroutable from the probe, and public_dns
// would resolve through DNS the holdout does not control.
func TestResolveProbeHostAWSInstancePublicIP(t *testing.T) {
	state, attrs := loadAWSWebLiveState(t)
	publicIP, _ := attrs["public_ip"].(string)
	require.NotEmpty(t, publicIP)

	host, err := resolveProbeHost(state, "compute")
	require.NoError(t, err)
	assert.Equal(t, publicIP, host)
	assert.NotEqual(t, attrs["private_ip"], host)

	// fakeaws leaves the DNS names empty; real AWS fills them, and they
	// must not outrank public_ip.
	attrs["public_dns"] = "ec2-203-0-113-195.compute-1.amazonaws.com"
	attrs["private_dns"] = "ip-10-80-1-4.ec2.internal"
	host, err = resolveProbeHost(state, "compute")
	require.NoError(t, err)
	assert.Equal(t, publicIP, host)
}

// Runs the aws-web-live holdout's own checks over the captured state:
// every dial goes to the instance's public_ip, on 22, 443 and 80.
func TestRealProbeHarnessDialsAWSInstancePublicIPForHoldout(t *testing.T) {
	_, attrs := loadAWSWebLiveState(t)
	publicIP, _ := attrs["public_ip"].(string)
	require.NotEmpty(t, publicIP)

	workDir := t.TempDir()
	payload, err := os.ReadFile(awsWebLiveState)
	require.NoError(t, err)
	writeLiveState(t, workDir, string(payload))

	holdout, err := scenario.LoadWithSchema("../../scenarios/holdout/aws-web-live-unseen.yaml", "../../scenario.schema.json")
	require.NoError(t, err)
	specs, err := holdout.ExecutableChecks()
	require.NoError(t, err)
	checks := make([]ProbeCheck, 0, len(specs))
	for _, spec := range specs {
		require.NotNil(t, spec.Connectivity)
		checks = append(checks, ProbeCheck{Type: spec.Type, Expect: spec.Expect, From: spec.Connectivity.From, To: spec.Connectivity.To, Port: spec.Connectivity.Port})
	}

	var dialed []string
	h := NewRealProbeHarness(ProbeConfig{Timeout: time.Second, Retries: 1})
	h.dialFunc = func(_ context.Context, _, address string) (net.Conn, error) {
		dialed = append(dialed, address)
		if strings.HasSuffix(address, ":80") {
			client, server := net.Pipe()
			_ = server.Close()
			return client, nil
		}
		return nil, errors.New("i/o timeout")
	}

	result, err := h.Run(context.Background(), workDir, "aws-web-live", checks)
	require.NoError(t, err)
	assert.Empty(t, result.Failures)
	assert.Equal(t, []string{
		net.JoinHostPort(publicIP, "22"),
		net.JoinHostPort(publicIP, "443"),
		net.JoinHostPort(publicIP, "80"),
	}, dialed)
}

// A passing check keeps its evidence: the status that came back and how
// many attempts it took to get there.
func TestRealProbeHarnessRecordsHTTPStatusAndAttempts(t *testing.T) {
	workDir := t.TempDir()
	writeLiveState(t, workDir, `{"resources":[{"type":"aws_instance","instances":[{"attributes":{"public_ip":"203.0.113.7"}}]}]}`)
	calls := 0
	h := NewRealProbeHarness(ProbeConfig{Timeout: time.Second, Retries: 3})
	h.getHTTP = func(*http.Request) (*http.Response, error) {
		if calls++; calls == 1 {
			return nil, errors.New("connection refused")
		}
		return &http.Response{StatusCode: http.StatusFound, Body: http.NoBody}, nil
	}

	result, err := h.Run(context.Background(), workDir, "demo", []ProbeCheck{
		{Type: "http_probe", Target: "compute", Port: 80, Expect: "reachable"},
	})

	require.NoError(t, err)
	assert.Empty(t, result.Failures)
	require.Len(t, result.Records, 1)
	record := result.Records[0]
	assert.Equal(t, "http://203.0.113.7:80", record.Address)
	assert.Equal(t, strconv.Itoa(http.StatusFound), record.Status)
	assert.Equal(t, 2, record.Attempts)
}

// A blocked check that passes names the address it dialled.
func TestRealProbeHarnessRecordsBlockedDialAddress(t *testing.T) {
	workDir := t.TempDir()
	writeLiveState(t, workDir, `{"resources":[{"type":"aws_instance","instances":[{"attributes":{"public_ip":"203.0.113.7"}}]}]}`)
	h := NewRealProbeHarness(ProbeConfig{Timeout: time.Second, Retries: 3})
	h.dialFunc = func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("connection refused")
	}

	result, err := h.Run(context.Background(), workDir, "demo", []ProbeCheck{
		{Type: "connectivity", From: "public_internet", To: "compute", Port: 22, Expect: "blocked"},
	})

	require.NoError(t, err)
	assert.Empty(t, result.Failures)
	require.Len(t, result.Records, 1)
	assert.Equal(t, "203.0.113.7:22", result.Records[0].Address)
	assert.Equal(t, 1, result.Records[0].Attempts)
	assert.Contains(t, result.Records[0].Status, "connection refused")
}

// A record says whether the check succeeded: seconds only for a success,
// "no success" after the attempts a failure made, "not attempted" when it
// never got as far as a dial.
func TestProbeRecordSaysHowTheCheckEnded(t *testing.T) {
	workDir := t.TempDir()
	writeLiveState(t, workDir, `{"resources":[{"type":"aws_instance","instances":[{"attributes":{"public_ip":"203.0.113.7"}}]}]}`)
	h := NewRealProbeHarness(ProbeConfig{Timeout: time.Second, Retries: 2})
	h.getHTTP = func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusBadGateway, Body: http.NoBody}, nil
	}
	h.dialFunc = func(context.Context, string, string) (net.Conn, error) {
		client, server := net.Pipe()
		_ = server.Close()
		return client, nil
	}

	result, err := h.Run(context.Background(), workDir, "demo", []ProbeCheck{
		{Type: "http_probe", Target: "compute", Port: 80, Expect: "reachable"},
		{Type: "connectivity", To: "database", Port: 5432, Expect: "success"},
		{Type: "connectivity", To: "compute", Port: 80, Expect: "success"},
	})

	require.NoError(t, err)
	require.Len(t, result.Records, 3)
	exhausted, unresolved, passed := result.Records[0].String(), result.Records[1].String(), result.Records[2].String()
	assert.Contains(t, exhausted, "no success after 2 attempt(s)")
	assert.NotContains(t, exhausted, "0.0s")
	assert.Contains(t, unresolved, "not attempted")
	assert.NotContains(t, unresolved, "0.0s")
	assert.Contains(t, passed, "succeeded after 1 attempt(s) in ")
	assert.Equal(t, "no per-check records", ProbeRecordsDetail(nil))
}
