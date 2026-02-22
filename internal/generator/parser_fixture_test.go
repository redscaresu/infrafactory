package generator

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestParseFileBlocksFixtures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		fixture     string
		expected    map[string]string
		expectedErr error
	}{
		{
			name:    "valid multi file fixture",
			fixture: filepath.Join("testdata", "parser", "valid_multi_with_fences.txt"),
			expected: map[string]string{
				"main.tf":      "terraform {}",
				"variables.tf": `variable "region" {}`,
			},
		},
		{
			name:        "malformed fixture no file headers",
			fixture:     filepath.Join("testdata", "parser", "malformed_no_headers.txt"),
			expectedErr: ErrParseFailed,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			payload, err := os.ReadFile(tc.fixture)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}

			files, err := ParseFileBlocks(string(payload))
			if !errors.Is(err, tc.expectedErr) {
				t.Fatalf("expected error %v, got %v", tc.expectedErr, err)
			}
			if tc.expectedErr != nil {
				return
			}

			if len(files) != len(tc.expected) {
				t.Fatalf("expected %d files, got %d", len(tc.expected), len(files))
			}
			for name, expected := range tc.expected {
				if string(files[name]) != expected {
					t.Fatalf("unexpected content for %q: expected %q, got %q", name, expected, string(files[name]))
				}
			}
		})
	}
}
