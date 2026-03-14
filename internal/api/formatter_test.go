package api

import (
	"net/http/httptest"
	"testing"
)

func TestShouldFormatFile(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"main.tf", "terraform.tfvars", "policy.hcl"} {
		if !shouldFormatFile(name) {
			t.Fatalf("expected %s to be formattable", name)
		}
	}
	for _, name := range []string{"README.md", "app.log", "iteration.json"} {
		if shouldFormatFile(name) {
			t.Fatalf("expected %s to not be formattable", name)
		}
	}
}

func TestShouldFormatRequest(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest("GET", "/api/runs/example/run-1/files/main.tf?format=1", nil)
	if !shouldFormatRequest(req, "main.tf") {
		t.Fatal("expected format query to enable formatting")
	}
	req = httptest.NewRequest("GET", "/api/runs/example/run-1/files/main.tf", nil)
	if shouldFormatRequest(req, "main.tf") {
		t.Fatal("expected formatting to be disabled without query")
	}
	req = httptest.NewRequest("GET", "/api/runs/example/run-1/files/run.json?format=1", nil)
	if shouldFormatRequest(req, "run.json") {
		t.Fatal("expected non-HCL files to ignore format query")
	}
}
