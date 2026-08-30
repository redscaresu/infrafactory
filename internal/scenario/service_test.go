package scenario

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeScenarioFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func validServiceSpec() ServiceSpec {
	return ServiceSpec{Image: "nginx", Tag: "1.27", Port: 80, HealthPath: "/", TTL: "4h"}
}

func TestServiceSpecAccessors(t *testing.T) {
	s := validServiceSpec()

	assert.Equal(t, "nginx:1.27", s.Ref())
	assert.Equal(t, "/", s.HealthPathOrDefault())

	ttl, err := s.TimeToLive()
	require.NoError(t, err)
	assert.Equal(t, 4*time.Hour, ttl)
}

func TestHealthPathFallsBackToTheSchemaDefault(t *testing.T) {
	s := validServiceSpec()
	s.HealthPath = ""
	assert.Equal(t, DefaultHealthPath, s.HealthPathOrDefault())

	s.HealthPath = "   "
	assert.Equal(t, DefaultHealthPath, s.HealthPathOrDefault(), "whitespace is not a path")

	s.HealthPath = "/healthz"
	assert.Equal(t, "/healthz", s.HealthPathOrDefault())
}

func TestValidateAcceptsAPinnedVersion(t *testing.T) {
	require.NoError(t, validServiceSpec().Validate())
}

// A tag that moves defeats the only property a live service needs from
// its image: being able to name the version that is running. Without it
// an upgrade cannot be told from a no-op.
func TestValidateRefusesMovingTags(t *testing.T) {
	for _, tag := range []string{"latest", "LATEST", " latest ", "stable", "edge", "main", "master", "nightly"} {
		t.Run(tag, func(t *testing.T) {
			s := validServiceSpec()
			s.Tag = tag

			err := s.Validate()

			require.Error(t, err)
			assert.Contains(t, err.Error(), "moving tag")
			assert.Contains(t, err.Error(), "cannot be told from a no-op")
		})
	}
}

func TestValidateRejectsUnusableTTLs(t *testing.T) {
	cases := map[string]struct{ ttl, want string }{
		"unparseable": {"4 hours", "parse ttl"},
		"zero":        {"0s", "must be positive"},
		"negative":    {"-1h", "must be positive"},
		"typo":        {"400h", "exceeds the 168h0m0s maximum"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			s := validServiceSpec()
			s.TTL = tc.ttl

			err := s.Validate()

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestValidateAcceptsTheTTLBoundary(t *testing.T) {
	s := validServiceSpec()
	s.TTL = "168h"
	assert.NoError(t, s.Validate(), "the maximum itself is allowed")
}

func TestValidateServiceIgnoresInfrastructureOnlyScenarios(t *testing.T) {
	assert.NoError(t, validateService(&Scenario{}), "a scenario with no service block is unaffected")
}

func TestValidateServiceWrapsInvalidScenario(t *testing.T) {
	spec := validServiceSpec()
	spec.Tag = "latest"

	err := validateService(&Scenario{Service: &spec})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidScenario)
}

// End-to-end through the loader: the schema accepts the block and the Go
// rules reject what the schema cannot express.
func TestLoadAcceptsALiveServiceScenario(t *testing.T) {
	sc, err := LoadWithSchema(
		filepath.Join("..", "..", "scenarios", "training", "web-live-paris.yaml"),
		filepath.Join("..", "..", "scenario.schema.json"),
	)
	require.NoError(t, err)

	require.NotNil(t, sc.Service, "the service block survives decoding")
	assert.Equal(t, "nginx", sc.Service.Image)
	assert.Equal(t, "1.27", sc.Service.Tag)
	assert.Equal(t, 80, sc.Service.Port)
	assert.Equal(t, "4h", sc.Service.TTL)
	assert.Equal(t, "nginx:1.27", sc.Service.Ref())
}

func TestLoadRejectsAServiceWithAMovingTag(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "moving.yaml")
	writeScenarioFile(t, path, `
scenario: moving-tag-paris
version: "1.0"
cloud: scaleway
description: A service pinned to a tag that moves.
service:
  image: nginx
  tag: latest
  port: 80
  ttl: 4h
resources:
  compute:
    purpose: web-server
    size: small
acceptance_criteria:
  - type: destruction
    expect: no_orphans
`)

	_, err := LoadWithSchema(path, filepath.Join("..", "..", "scenario.schema.json"))

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidScenario)
	assert.Contains(t, err.Error(), "moving tag")
}

// The schema, not the Go rules, must reject a service missing its TTL --
// so the mandatory-expiry rule holds even for callers that validate
// without decoding (the UI's real-time validation path).
func TestSchemaRejectsAServiceWithoutATTL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "no-ttl.yaml")
	writeScenarioFile(t, path, `
scenario: no-ttl-paris
version: "1.0"
cloud: scaleway
description: A service that never expires.
service:
  image: nginx
  tag: "1.27"
  port: 80
resources:
  compute:
    purpose: web-server
    size: small
acceptance_criteria:
  - type: destruction
    expect: no_orphans
`)

	_, err := LoadWithSchema(path, filepath.Join("..", "..", "scenario.schema.json"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "ttl")
}
