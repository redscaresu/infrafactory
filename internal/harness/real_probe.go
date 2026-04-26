package harness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/redscaresu/infrafactory/internal/feedback"
)

var ErrRealProbeFailed = errors.New("real probe failed")

type ProbeConfig struct {
	Timeout    time.Duration
	Retries    int
	RetryDelay time.Duration
}

type ProbeCheck struct {
	Type   string
	Expect string
	From   string
	To     string
	Target string
	Port   int
	Domain string
}

type RealProbeHarness struct {
	cfg      ProbeConfig
	dialFunc func(context.Context, string, string) (net.Conn, error)
	lookup   func(context.Context, string) ([]string, error)
	getHTTP  func(*http.Request) (*http.Response, error)
}

type RealProbeResult struct {
	Failures []feedback.Failure
}

type RealProbeError struct {
	Check ProbeCheck
	Err   error
}

func NewRealProbeHarness(cfg ProbeConfig) *RealProbeHarness {
	transport := &http.Transport{}
	client := &http.Client{
		Timeout:   cfg.Timeout,
		Transport: transport,
	}
	dialer := &net.Dialer{Timeout: cfg.Timeout}
	return &RealProbeHarness{
		cfg: cfg,
		dialFunc: func(ctx context.Context, network, address string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, address)
		},
		lookup: func(ctx context.Context, host string) ([]string, error) {
			return net.DefaultResolver.LookupHost(ctx, host)
		},
		getHTTP: client.Do,
	}
}

func (e *RealProbeError) Error() string {
	if e == nil {
		return ErrRealProbeFailed.Error()
	}
	return fmt.Sprintf("%s: %s: %v", ErrRealProbeFailed, e.Check.Type, e.Err)
}

func (e *RealProbeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *RealProbeError) Is(target error) bool {
	return target == ErrRealProbeFailed
}

func (h *RealProbeHarness) Run(ctx context.Context, workDir string, scenarioName string, checks []ProbeCheck) (*RealProbeResult, error) {
	state, err := loadLiveTerraformState(filepath.Join(workDir, LiveStateFilename))
	if err != nil {
		return nil, err
	}

	failures := make([]feedback.Failure, 0)
	for _, check := range checks {
		var probeErr error
		switch check.Type {
		case "connectivity":
			host, resolveErr := resolveProbeHost(state, check.To)
			if resolveErr != nil {
				probeErr = resolveErr
			} else {
				probeErr = h.runConnectivityProbe(ctx, host, check.Port, check.Expect)
			}
		case "http_probe":
			host, resolveErr := resolveProbeHost(state, check.Target)
			if resolveErr != nil {
				probeErr = resolveErr
			} else {
				probeErr = h.runHTTPProbe(ctx, host, check.Port, check.Expect)
			}
		case "dns_resolution":
			domain := strings.ReplaceAll(check.Domain, "{{scenario_name}}", scenarioName)
			probeErr = h.runDNSProbe(ctx, domain, check.Expect)
		}
		if probeErr == nil {
			continue
		}
		failures = append(failures, feedback.Failure{
			Layer:   "sandbox_deploy",
			Stage:   "real_probe",
			Status:  "fail",
			Check:   check.Type,
			Command: "real probe harness",
			Detail:  probeErr.Error(),
		})
	}

	return &RealProbeResult{Failures: failures}, nil
}

func (h *RealProbeHarness) runConnectivityProbe(ctx context.Context, host string, port int, expect string) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("connectivity probe requires port between 1 and 65535")
	}
	address := net.JoinHostPort(host, strconv.Itoa(port))
	err := h.retry(ctx, func(ctx context.Context) error {
		conn, err := h.dialFunc(ctx, "tcp", address)
		if err == nil {
			_ = conn.Close()
		}
		expectedSuccess := expect == "success"
		if expectedSuccess && err != nil {
			return fmt.Errorf("tcp connect %s: %w", address, err)
		}
		if !expectedSuccess && err == nil {
			return fmt.Errorf("tcp connect %s unexpectedly succeeded", address)
		}
		if !expectedSuccess {
			return nil
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("connectivity probe %s: %w", address, err)
	}
	return nil
}

func (h *RealProbeHarness) runHTTPProbe(ctx context.Context, host string, port int, expect string) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("http_probe requires port between 1 and 65535")
	}
	url := fmt.Sprintf("http://%s", net.JoinHostPort(host, strconv.Itoa(port)))
	err := h.retry(ctx, func(ctx context.Context) error {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if reqErr != nil {
			return reqErr
		}
		resp, callErr := h.getHTTP(req)
		if resp != nil && resp.Body != nil {
			defer resp.Body.Close()
		}
		expectedReachable := expect == "reachable"
		if expectedReachable {
			if callErr != nil {
				return callErr
			}
			if resp.StatusCode >= 400 {
				return fmt.Errorf("unexpected status %d", resp.StatusCode)
			}
			return nil
		}
		if callErr == nil && resp != nil && resp.StatusCode < 400 {
			return fmt.Errorf("unexpectedly reachable with status %d", resp.StatusCode)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("http probe %s: %w", url, err)
	}
	return nil
}

func (h *RealProbeHarness) runDNSProbe(ctx context.Context, domain, expect string) error {
	err := h.retry(ctx, func(ctx context.Context) error {
		hosts, lookupErr := h.lookup(ctx, domain)
		expectedResolve := expect == "resolves"
		if expectedResolve {
			if lookupErr != nil {
				return lookupErr
			}
			if len(hosts) == 0 {
				return fmt.Errorf("no A/AAAA records returned")
			}
			return nil
		}
		if lookupErr == nil && len(hosts) == 0 {
			return nil
		}
		if lookupErr == nil && len(hosts) > 0 {
			return fmt.Errorf("unexpectedly resolved to %s", strings.Join(hosts, ", "))
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("dns probe %s: %w", domain, err)
	}
	return nil
}

func (h *RealProbeHarness) retry(ctx context.Context, fn func(context.Context) error) error {
	retries := h.cfg.Retries
	if retries < 1 {
		retries = 1
	}

	var lastErr error
	for attempt := 1; attempt <= retries; attempt++ {
		attemptCtx := ctx
		cancel := func() {}
		if h.cfg.Timeout > 0 {
			attemptCtx, cancel = context.WithTimeout(ctx, h.cfg.Timeout)
		}
		lastErr = fn(attemptCtx)
		cancel()
		if lastErr == nil {
			return nil
		}
		if attempt == retries {
			break
		}
		if h.cfg.RetryDelay > 0 {
			timer := time.NewTimer(h.cfg.RetryDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	return lastErr
}

type terraformState struct {
	Resources []terraformResource `json:"resources"`
}

type terraformResource struct {
	Type      string                      `json:"type"`
	Instances []terraformResourceInstance `json:"instances"`
}

type terraformResourceInstance struct {
	Attributes map[string]any `json:"attributes"`
}

func loadLiveTerraformState(path string) (terraformState, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return terraformState{}, fmt.Errorf("read live terraform state: %w", err)
	}
	var state terraformState
	if err := json.Unmarshal(payload, &state); err != nil {
		return terraformState{}, fmt.Errorf("decode live terraform state: %w", err)
	}
	return state, nil
}

func resolveProbeHost(state terraformState, target string) (string, error) {
	resourceTypes := probeTargetResourceTypes(target)
	if len(resourceTypes) == 0 {
		return "", fmt.Errorf("no live endpoint mapping for target %q", target)
	}
	for _, resourceType := range resourceTypes {
		if host := findHostForResourceType(state, resourceType); host != "" {
			return host, nil
		}
	}
	return "", fmt.Errorf("could not resolve live endpoint for target %q", target)
}

// probeTargetResourceTypes returns the Terraform resource types whose live
// state may carry the host/IP for a probe target, ordered from most likely
// to least. The list deliberately mixes Scaleway and GCP types: live state
// only carries the resources that the scenario actually generated, so
// `findHostForResourceType` will skip past types absent from state without
// caring which cloud the scenario targeted.
func probeTargetResourceTypes(target string) []string {
	switch target {
	case "load_balancer":
		return []string{
			// Scaleway
			"scaleway_lb_ip", "scaleway_lb",
			// GCP — global addresses are the canonical anchor for an
			// L7 LB; forwarding rules carry the IP for L4 LBs.
			"google_compute_global_address", "google_compute_forwarding_rule",
		}
	case "database":
		return []string{"scaleway_rdb_instance", "google_sql_database_instance"}
	case "redis":
		return []string{"scaleway_redis_cluster", "google_redis_instance"}
	case "compute":
		return []string{"scaleway_instance_server", "google_compute_instance"}
	case "kubernetes":
		return []string{"scaleway_k8s_cluster", "google_container_cluster"}
	default:
		return nil
	}
}

func findHostForResourceType(state terraformState, resourceType string) string {
	for _, resource := range state.Resources {
		if resource.Type != resourceType {
			continue
		}
		for _, instance := range resource.Instances {
			if host := pickHost(instance.Attributes, resourceType); host != "" {
				return host
			}
		}
	}
	return ""
}

func pickHost(attrs map[string]any, resourceType string) string {
	if len(attrs) == 0 {
		return ""
	}
	patterns := []string{"ip_address", "public_ip", "private_ip", "endpoint", "host", "hostname", "address", "ip"}
	switch resourceType {
	case "scaleway_rdb_instance":
		patterns = []string{"endpoint_ip", "private_network.0", "load_balancer_ip", "host", "address", "ip"}
	case "scaleway_lb_ip":
		patterns = []string{"ip_address", "address", "ip"}
	case "scaleway_instance_server":
		patterns = []string{"public_ip.0.address", "public_ip", "private_ip", "address", "ip"}
	case "google_compute_global_address":
		patterns = []string{"address", "ip_address", "ip"}
	case "google_compute_forwarding_rule":
		patterns = []string{"ip_address", "load_balancing_scheme", "address", "ip"}
	case "google_sql_database_instance":
		patterns = []string{"public_ip_address", "private_ip_address", "first_ip_address", "ip_address", "host", "address", "ip"}
	case "google_redis_instance":
		patterns = []string{"host", "read_endpoint", "current_location_id", "address", "ip"}
	case "google_compute_instance":
		patterns = []string{"network_interface.0.access_config.0.nat_ip", "network_interface.0.network_ip", "public_ip", "private_ip", "address", "ip"}
	case "google_container_cluster":
		patterns = []string{"endpoint", "private_cluster_config.0.private_endpoint", "address", "ip"}
	}
	type candidate struct {
		key   string
		value string
	}
	candidates := make([]candidate, 0)
	for key, value := range flattenStringValues(attrs, "") {
		if parsed := net.ParseIP(value); parsed != nil || isHostname(value) {
			candidates = append(candidates, candidate{key: key, value: value})
		}
	}
	slices.SortStableFunc(candidates, func(left, right candidate) int {
		leftScore := scoreProbeKey(left.key, patterns)
		rightScore := scoreProbeKey(right.key, patterns)
		switch {
		case leftScore < rightScore:
			return -1
		case leftScore > rightScore:
			return 1
		default:
			if left.key < right.key {
				return -1
			}
			if left.key > right.key {
				return 1
			}
			return 0
		}
	})
	for _, candidate := range candidates {
		if candidate.value != "" {
			return candidate.value
		}
	}
	return ""
}

func flattenStringValues(value any, prefix string) map[string]string {
	out := map[string]string{}
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			next := key
			if prefix != "" {
				next = prefix + "." + key
			}
			for nestedKey, nestedValue := range flattenStringValues(nested, next) {
				out[nestedKey] = nestedValue
			}
		}
	case []any:
		for idx, nested := range typed {
			next := strconv.Itoa(idx)
			if prefix != "" {
				next = prefix + "." + next
			}
			for nestedKey, nestedValue := range flattenStringValues(nested, next) {
				out[nestedKey] = nestedValue
			}
		}
	case string:
		out[prefix] = typed
	}
	return out
}

func scoreProbeKey(key string, patterns []string) int {
	lower := strings.ToLower(key)
	for idx, pattern := range patterns {
		if strings.Contains(lower, strings.ToLower(pattern)) {
			return idx
		}
	}
	return len(patterns) + 1
}

func isHostname(value string) bool {
	if strings.Contains(value, "://") || strings.Contains(value, "/") || strings.Contains(value, " ") {
		return false
	}
	return strings.Contains(value, ".")
}
