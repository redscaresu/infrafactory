package harness

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ServiceProbeResult is what one probe of a live service's health path
// concluded.
//
// Reachable and Healthy are separate because the difference matters
// downstream: "we could not reach it" and "it answered and said it is
// broken" are different facts about the world, and a learning loop that
// collapses them learns the wrong lesson (S154).
type ServiceProbeResult struct {
	Reachable bool
	Healthy   bool
	// Status is the HTTP status when the service answered, zero when it
	// did not.
	Status int
	// Detail is the reason, phrased for the pitfall extractors that
	// eventually consume it. Empty when healthy.
	Detail string
	// Body is the start of what the service said, bounded. Kept so a
	// caller can check the running version against what the record
	// claims (S155); a service answering with a firehose must not be
	// able to grow this.
	Body string
	// BodyComplete reports whether Body is the whole response.
	//
	// It is false when the read failed or hit the byte limit, and the
	// distinction is load-bearing: FINDING a version string in a partial
	// body proves it is there, but NOT finding one proves nothing. A
	// caller that ignores this reports a mismatch on evidence it does
	// not have.
	BodyComplete bool
}

// maxProbeBodyBytes bounds what a probe keeps from the response.
//
// Enough to hold a version string or a small health document, and small
// enough that a service answering with a firehose cannot grow a
// deployment record -- which is stored, and stored per observation.
const maxProbeBodyBytes = 4096

// ServiceProbe checks a live deployment's health path.
//
// Separate from RealProbeHarness on purpose. That one runs inside a run,
// against the plan it just applied, with retries tuned to an apply that
// has only just finished. This one runs out-of-band against an address
// recorded minutes or hours ago, and a retry here would smear over
// exactly the flapping it exists to notice: one probe, one observation.
type ServiceProbe struct {
	doer    func(*http.Request) (*http.Response, error)
	timeout time.Duration
}

// NewServiceProbe builds a probe with its own client, so a hung service
// cannot outlive the probe that found it.
func NewServiceProbe(timeout time.Duration) *ServiceProbe {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	client := &http.Client{
		Timeout: timeout,
		// A health path that redirects has not answered the question
		// asked, and following the redirect could leave the account's
		// own network entirely. Report the 3xx as-is.
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return &ServiceProbe{doer: client.Do, timeout: timeout}
}

// NewServiceProbeWithDoer is the test seam.
func NewServiceProbeWithDoer(doer func(*http.Request) (*http.Response, error)) *ServiceProbe {
	return &ServiceProbe{doer: doer, timeout: 10 * time.Second}
}

// Probe requests healthPath on address:port once.
//
// Any 2xx is healthy. Everything else the service says is unhealthy, and
// anything that stops the request reaching it is unreachable.
func (p *ServiceProbe) Probe(ctx context.Context, address string, port int, healthPath string) (ServiceProbeResult, error) {
	target, err := serviceProbeURL(address, port, healthPath)
	if err != nil {
		return ServiceProbeResult{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return ServiceProbeResult{}, fmt.Errorf("build health request for %s: %w", target, err)
	}

	resp, err := p.doer(req)
	if err != nil {
		// Not an error return: "unreachable" is a successful
		// observation, not a failure to observe. Returning an error
		// here would make the caller unable to tell a service that is
		// down from a probe that never ran.
		return ServiceProbeResult{
			Detail: fmt.Sprintf("health path %s is unreachable: %v", target, err),
		}, nil
	}
	// Read the head of the body rather than discarding it: the version
	// check needs what the service said. Still bounded, and the rest is
	// still drained so the connection can be reused.
	//
	// One byte over the limit is read on purpose, so hitting it is
	// distinguishable from a body that merely ends there.
	head, readErr := io.ReadAll(io.LimitReader(resp.Body, maxProbeBodyBytes+1))
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	complete := readErr == nil && len(head) <= maxProbeBodyBytes
	if len(head) > maxProbeBodyBytes {
		head = head[:maxProbeBodyBytes]
	}

	result := ServiceProbeResult{
		Reachable: true, Status: resp.StatusCode,
		Body: string(head), BodyComplete: complete,
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		result.Healthy = true
		return result, nil
	}

	result.Detail = fmt.Sprintf("health path %s returned HTTP %d", target, resp.StatusCode)
	return result, nil
}

// serviceProbeURL refuses anything it cannot turn into a plain http URL
// rather than guessing, because the address comes off a stored record and
// a malformed one must not be probed as if it were something else.
func serviceProbeURL(address string, port int, healthPath string) (string, error) {
	host := strings.TrimSpace(address)
	if host == "" {
		return "", fmt.Errorf("no address recorded, so there is nothing to probe")
	}
	if port < 1 || port > 65535 {
		return "", fmt.Errorf("port %d is not a valid service port", port)
	}

	raw := strings.TrimSpace(healthPath)
	if raw == "" {
		raw = "/"
	}

	// Parsed BEFORE the leading slash is added, not after. Normalising
	// first turns "http://elsewhere.example/health" into the harmless-
	// looking path "/http:/elsewhere.example/health", which passes a
	// scheme-and-host check that never sees a scheme or a host -- so the
	// probe would quietly request something nobody asked for.
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("health path %q is not a valid path: %w", healthPath, err)
	}
	if parsed.Scheme != "" || parsed.Host != "" || parsed.Opaque != "" {
		return "", fmt.Errorf("health path %q must be a path, not a URL", healthPath)
	}

	path := parsed.Path
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	return (&url.URL{
		Scheme:   "http",
		Host:     net.JoinHostPort(host, strconv.Itoa(port)),
		Path:     path,
		RawQuery: parsed.RawQuery,
	}).String(), nil
}
