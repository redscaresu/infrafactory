package harness

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestLayer2IntegrationSmoke(t *testing.T) {
	t.Parallel()

	if os.Getenv("INFRAFACTORY_ENABLE_INTEGRATION") != "1" {
		t.Skip("set INFRAFACTORY_ENABLE_INTEGRATION=1 to run integration smoke test")
	}

	mockwayURL := os.Getenv("INFRAFACTORY_MOCKWAY_URL")
	if mockwayURL == "" {
		t.Fatal("INFRAFACTORY_MOCKWAY_URL is required when integration test is enabled")
	}

	if _, err := exec.LookPath("tofu"); err != nil {
		t.Fatalf("tofu binary is required for integration smoke test: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mockwayURL+"/mock/state", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("query mockway state endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 from mockway state endpoint, got %d", resp.StatusCode)
	}
}
