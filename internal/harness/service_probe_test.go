package harness

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceProbeURLBuildsFromTheRecord(t *testing.T) {
	cases := map[string]struct{ address, path, want string }{
		"plain":         {"1.2.3.4", "/healthz", "http://1.2.3.4:80/healthz"},
		"empty path":    {"1.2.3.4", "", "http://1.2.3.4:80/"},
		"missing slash": {"1.2.3.4", "healthz", "http://1.2.3.4:80/healthz"},
		"query kept":    {"1.2.3.4", "/healthz?deep=1", "http://1.2.3.4:80/healthz?deep=1"},
		"ipv6":          {"2001:db8::1", "/", "http://[2001:db8::1]:80/"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := serviceProbeURL(tc.address, 80, tc.path)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// The address and path come off a stored record, so a malformed one must
// be refused rather than probed as if it were something else.
func TestServiceProbeURLRefusesWhatItCannotBuild(t *testing.T) {
	_, err := serviceProbeURL("", 80, "/")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nothing to probe")

	for _, port := range []int{0, -1, 70000} {
		_, err := serviceProbeURL("1.2.3.4", port, "/")
		require.Error(t, err, "port %d", port)
	}

	// A health path carrying its own host would send the probe somewhere
	// else entirely and record the answer against this deployment.
	_, err = serviceProbeURL("1.2.3.4", 80, "http://elsewhere.example/health")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a path")
}

func TestProbeTreatsAny2xxAsHealthy(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusNoContent, 299} {
		probe := NewServiceProbeWithDoer(func(*http.Request) (*http.Response, error) {
			return jsonResponse(status, `{}`), nil
		})

		result, err := probe.Probe(context.Background(), "1.2.3.4", 80, "/")

		require.NoError(t, err)
		assert.True(t, result.Healthy, "status %d", status)
		assert.True(t, result.Reachable)
		assert.Empty(t, result.Detail, "a healthy probe has nothing to say")
	}
}

// Answering wrongly and not answering at all are different facts about
// the world, and a learning loop that collapses them learns the wrong
// lesson.
func TestProbeSeparatesUnhealthyFromUnreachable(t *testing.T) {
	unhealthy := NewServiceProbeWithDoer(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusServiceUnavailable, `{}`), nil
	})
	result, err := unhealthy.Probe(context.Background(), "1.2.3.4", 80, "/")
	require.NoError(t, err)
	assert.True(t, result.Reachable, "the service answered")
	assert.False(t, result.Healthy)
	assert.Equal(t, http.StatusServiceUnavailable, result.Status)
	assert.Contains(t, result.Detail, "HTTP 503")

	unreachable := NewServiceProbeWithDoer(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("dial tcp: connection refused")
	})
	result, err = unreachable.Probe(context.Background(), "1.2.3.4", 80, "/")
	// Not an error: "unreachable" is a successful observation, not a
	// failure to observe. An error return would make the caller unable to
	// tell a service that is down from a probe that never ran.
	require.NoError(t, err)
	assert.False(t, result.Reachable)
	assert.False(t, result.Healthy)
	assert.Contains(t, result.Detail, "unreachable")
	assert.Contains(t, result.Detail, "connection refused")
}

// A URL it cannot build is a failure to observe, and must be
// distinguishable from a service that is down.
func TestProbeErrorsRatherThanReportingAnUnbuildableTargetAsDown(t *testing.T) {
	probe := NewServiceProbeWithDoer(func(*http.Request) (*http.Response, error) {
		t.Fatal("must not reach the network with an unbuildable target")
		return nil, nil
	})

	_, err := probe.Probe(context.Background(), "", 80, "/")

	require.Error(t, err)
}

// A health path that redirects has not answered the question asked, and
// following it could leave the account's network entirely.
func TestProbeDoesNotFollowRedirects(t *testing.T) {
	probe := NewServiceProbeWithDoer(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusFound, `{}`), nil
	})

	result, err := probe.Probe(context.Background(), "1.2.3.4", 80, "/")

	require.NoError(t, err)
	assert.False(t, result.Healthy, "a 302 is not a service that is serving")
	assert.Equal(t, http.StatusFound, result.Status)
}
