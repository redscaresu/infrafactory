package generator

import (
	"context"
	"errors"
	"testing"
)

func TestSeedGeneratorFuncImplementsContract(t *testing.T) {
	t.Parallel()

	called := false
	gen := SeedGeneratorFunc(func(_ context.Context, req Request) (*GeneratedCode, error) {
		called = true
		if req.Iteration != 2 {
			t.Fatalf("expected iteration 2, got %d", req.Iteration)
		}
		return &GeneratedCode{
			Files: map[string][]byte{
				"main.tf": []byte("resource {}"),
			},
		}, nil
	})

	var contract SeedGenerator = gen
	out, err := contract.Generate(context.Background(), Request{Iteration: 2})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !called {
		t.Fatal("expected generator to be called")
	}
	if out == nil || len(out.Files) != 1 {
		t.Fatalf("expected one generated file, got %#v", out)
	}
}

func TestGeneratedCodeValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		code        *GeneratedCode
		expectedErr error
	}{
		{
			name:        "nil output",
			code:        nil,
			expectedErr: ErrParseFailed,
		},
		{
			name: "no files",
			code: &GeneratedCode{
				Files: map[string][]byte{},
			},
			expectedErr: ErrParseFailed,
		},
		{
			name: "empty filename",
			code: &GeneratedCode{
				Files: map[string][]byte{
					"": []byte("ok"),
				},
			},
			expectedErr: ErrParseFailed,
		},
		{
			name: "empty content",
			code: &GeneratedCode{
				Files: map[string][]byte{
					"main.tf": nil,
				},
			},
			expectedErr: ErrParseFailed,
		},
		{
			name: "valid",
			code: &GeneratedCode{
				Files: map[string][]byte{
					"main.tf": []byte("terraform {}"),
				},
			},
			expectedErr: nil,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.code.Validate()
			if !errors.Is(err, tc.expectedErr) {
				t.Fatalf("expected error %v, got %v", tc.expectedErr, err)
			}
		})
	}
}

func TestGenerateErrorWrapping(t *testing.T) {
	t.Parallel()

	rootErr := errors.New("exec failed")
	err := NewGenerateError(ErrTransportFailed, "generate_hcl", rootErr)

	if !errors.Is(err, ErrTransportFailed) {
		t.Fatalf("expected errors.Is(..., ErrTransportFailed) true, got %v", err)
	}
	if !errors.Is(err, ErrGenerateFailed) {
		t.Fatalf("expected errors.Is(..., ErrGenerateFailed) true, got %v", err)
	}
	if !errors.Is(err, rootErr) {
		t.Fatalf("expected errors.Is(..., rootErr) true, got %v", err)
	}

	var typed *GenerateError
	if !errors.As(err, &typed) {
		t.Fatalf("expected errors.As(..., *GenerateError) true, got %T", err)
	}
	if typed.Phase != "generate_hcl" {
		t.Fatalf("expected phase generate_hcl, got %q", typed.Phase)
	}
}

func TestDefaultSeedGeneratorReturnsTypedTransportError(t *testing.T) {
	t.Parallel()

	gen := NewDefaultSeedGenerator("claude-code")

	_, err := gen.Generate(context.Background(), Request{Iteration: 1})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrTransportFailed) {
		t.Fatalf("expected transport error, got: %v", err)
	}
	if !errors.Is(err, ErrGenerateFailed) {
		t.Fatalf("expected generate failed error, got: %v", err)
	}
}

func TestContractForAgentType(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name                string
		agentType           string
		expectedEnvCount    int
		expectedConfigCount int
		expectErr           error
	}{
		{
			name:                "claude code",
			agentType:           AgentTypeClaudeCode,
			expectedEnvCount:    0,
			expectedConfigCount: 4,
		},
		{
			name:                "openrouter",
			agentType:           AgentTypeOpenRouter,
			expectedEnvCount:    1,
			expectedConfigCount: 7,
		},
		{
			name:      "unknown",
			agentType: "unknown",
			expectErr: ErrUnknownTransport,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			contract, err := ContractForAgentType(tc.agentType)
			if !errors.Is(err, tc.expectErr) {
				t.Fatalf("expected error %v, got %v", tc.expectErr, err)
			}
			if tc.expectErr != nil {
				return
			}

			if contract.AgentType != tc.agentType {
				t.Fatalf("expected agent type %q, got %q", tc.agentType, contract.AgentType)
			}
			if len(contract.RequiredEnv) != tc.expectedEnvCount {
				t.Fatalf("expected %d required env vars, got %d", tc.expectedEnvCount, len(contract.RequiredEnv))
			}
			if len(contract.RequiredConfigPaths) != tc.expectedConfigCount {
				t.Fatalf(
					"expected %d required config paths, got %d",
					tc.expectedConfigCount,
					len(contract.RequiredConfigPaths),
				)
			}
		})
	}
}
