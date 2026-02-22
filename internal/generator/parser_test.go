package generator

import (
	"errors"
	"testing"
)

func TestParseFileBlocks(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		input       string
		expected    map[string]string
		expectedErr error
	}{
		{
			name: "single file",
			input: `# File: main.tf
terraform {
  required_version = ">= 1.6"
}`,
			expected: map[string]string{
				"main.tf": "terraform {\n  required_version = \">= 1.6\"\n}",
			},
		},
		{
			name: "multiple files",
			input: `# File: main.tf
resource "x" "y" {}
# File: variables.tf
variable "region" {}`,
			expected: map[string]string{
				"main.tf":      `resource "x" "y" {}`,
				"variables.tf": `variable "region" {}`,
			},
		},
		{
			name:  "code fences are stripped",
			input: "# File: outputs.tf\n```hcl\noutput \"id\" {\n  value = \"abc\"\n}\n```",
			expected: map[string]string{
				"outputs.tf": "output \"id\" {\n  value = \"abc\"\n}",
			},
		},
		{
			name: "duplicate files last block wins",
			input: `# File: main.tf
resource "x" "old" {}
# File: main.tf
resource "x" "new" {}`,
			expected: map[string]string{
				"main.tf": `resource "x" "new" {}`,
			},
		},
		{
			name:  "fenced content drops trailing markdown artifacts",
			input: "# File: outputs.tf\n```hcl\noutput \"x\" {\n  value = \"ok\"\n}\n```\n|---|---|\n| a | b |",
			expected: map[string]string{
				"outputs.tf": "output \"x\" {\n  value = \"ok\"\n}",
			},
		},
		{
			name:  "unfenced content strips fence markers and trailing markdown prose",
			input: "# File: compute.tf\nresource \"x\" \"y\" {}\n```\n\n```hcl\n## Summary\n| a | b |",
			expected: map[string]string{
				"compute.tf": "resource \"x\" \"y\" {}",
			},
		},
		{
			name:        "missing headers",
			input:       `resource "x" "y" {}`,
			expectedErr: ErrParseFailed,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			files, err := ParseFileBlocks(tc.input)
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
				actual, ok := files[name]
				if !ok {
					t.Fatalf("expected file %q to exist", name)
				}
				if string(actual) != expected {
					t.Fatalf("unexpected content for %q:\nexpected:\n%s\nactual:\n%s", name, expected, string(actual))
				}
			}
		})
	}
}
