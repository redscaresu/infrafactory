package harness

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeSchemaRunner struct {
	calls   []Command
	results []CommandResult
	errs    []error
}

func (f *fakeSchemaRunner) Run(_ context.Context, cmd Command) (CommandResult, error) {
	idx := len(f.calls)
	f.calls = append(f.calls, cmd)
	if idx >= len(f.results) {
		return CommandResult{}, errors.New("unexpected call")
	}
	return f.results[idx], f.errs[idx]
}

func TestExtractProviderSchemaHappyPath(t *testing.T) {
	t.Parallel()

	schemaJSON := `{"provider_schemas":{"scaleway/scaleway":{"resource_schemas":{}}}}`
	runner := &fakeSchemaRunner{
		results: []CommandResult{
			{Stdout: []byte("init ok"), Stderr: nil},
			{Stdout: []byte(schemaJSON), Stderr: nil},
		},
		errs: []error{nil, nil},
	}

	got, err := ExtractProviderSchema(context.Background(), runner, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != schemaJSON {
		t.Fatalf("expected schema JSON %q, got %q", schemaJSON, string(got))
	}
	if len(runner.calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(runner.calls))
	}
	if runner.calls[0].Name != "tofu" || runner.calls[0].Args[0] != "init" {
		t.Fatalf("expected first call to be tofu init, got %+v", runner.calls[0])
	}
	if runner.calls[1].Name != "tofu" || runner.calls[1].Args[0] != "providers" {
		t.Fatalf("expected second call to be tofu providers, got %+v", runner.calls[1])
	}
}

func TestExtractProviderSchemaInitFailure(t *testing.T) {
	t.Parallel()

	runner := &fakeSchemaRunner{
		results: []CommandResult{
			{Stderr: []byte("init failed")},
		},
		errs: []error{errors.New("exit status 1")},
	}

	_, err := ExtractProviderSchema(context.Background(), runner, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "tofu init") {
		t.Fatalf("expected init error, got %v", err)
	}
}

func TestExtractProviderSchemaSchemaCommandFailure(t *testing.T) {
	t.Parallel()

	runner := &fakeSchemaRunner{
		results: []CommandResult{
			{Stdout: []byte("init ok")},
			{Stderr: []byte("schema failed")},
		},
		errs: []error{nil, errors.New("exit status 1")},
	}

	_, err := ExtractProviderSchema(context.Background(), runner, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "tofu providers schema") {
		t.Fatalf("expected schema error, got %v", err)
	}
}

func TestExtractProviderSchemaContextCancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	runner := &fakeSchemaRunner{
		results: []CommandResult{
			{Stderr: []byte("cancelled")},
		},
		errs: []error{context.Canceled},
	}

	_, err := ExtractProviderSchema(ctx, runner, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestExtractProviderSchemaPassesEnv(t *testing.T) {
	t.Parallel()

	runner := &fakeSchemaRunner{
		results: []CommandResult{
			{Stdout: []byte("ok")},
			{Stdout: []byte("{}")},
		},
		errs: []error{nil, nil},
	}

	env := map[string]string{"TF_LOG": "DEBUG"}
	_, err := ExtractProviderSchema(context.Background(), runner, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, call := range runner.calls {
		if call.Env["TF_LOG"] != "DEBUG" {
			t.Fatalf("expected env to be passed through, got %+v", call.Env)
		}
	}
}
