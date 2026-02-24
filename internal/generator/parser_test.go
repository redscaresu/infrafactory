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
			name:  "trailing bold prose is stripped",
			input: "# File: outputs.tf\noutput \"id\" {\n  value = \"abc\"\n}\n\n**Key fixes from the previous iteration:** The validation errors...",
			expected: map[string]string{
				"outputs.tf": "output \"id\" {\n  value = \"abc\"\n}",
			},
		},
		{
			name:  "trailing horizontal rule and prose is stripped",
			input: "# File: outputs.tf\noutput \"id\" {\n  value = \"abc\"\n}\n---\nSome explanation here.",
			expected: map[string]string{
				"outputs.tf": "output \"id\" {\n  value = \"abc\"\n}",
			},
		},
		{
			name:  "trailing bullet list is stripped",
			input: "# File: main.tf\nresource \"x\" \"y\" {}\n\n- **All resources defined** in every file\n- Second bullet point",
			expected: map[string]string{
				"main.tf": "resource \"x\" \"y\" {}",
			},
		},
		{
			name:  "trailing blockquote is stripped",
			input: "# File: main.tf\nresource \"x\" \"y\" {}\n\n> Note: this implementation...",
			expected: map[string]string{
				"main.tf": "resource \"x\" \"y\" {}",
			},
		},
		{
			name:  "fenced content with trailing bold prose after fence",
			input: "# File: outputs.tf\n```hcl\noutput \"x\" {\n  value = \"ok\"\n}\n```\n\n**Key points about this implementation:**\n- All resources defined",
			expected: map[string]string{
				"outputs.tf": "output \"x\" {\n  value = \"ok\"\n}",
			},
		},
		{
			name: "heredoc with cloud-init YAML preserved",
			input: `# File: compute.tf
resource "scaleway_instance_server" "web" {
  type  = "DEV1-S"
  image = "ubuntu_focal"

  user_data = <<-EOF
---
packages:
  - nginx
  - curl
runcmd:
  - systemctl start nginx
EOF
}`,
			expected: map[string]string{
				"compute.tf": "resource \"scaleway_instance_server\" \"web\" {\n  type  = \"DEV1-S\"\n  image = \"ubuntu_focal\"\n\n  user_data = <<-EOF\n---\npackages:\n  - nginx\n  - curl\nruncmd:\n  - systemctl start nginx\nEOF\n}",
			},
		},
		{
			name: "heredoc with bullet items preserved",
			input: `# File: main.tf
resource "null_resource" "readme" {
  provisioner "local-exec" {
    command = <<EOT
cat > README.md <<'INNER'
- first item
- second item
> blockquote
**bold line**
INNER
EOT
  }
}`,
			expected: map[string]string{
				"main.tf": "resource \"null_resource\" \"readme\" {\n  provisioner \"local-exec\" {\n    command = <<EOT\ncat > README.md <<'INNER'\n- first item\n- second item\n> blockquote\n**bold line**\nINNER\nEOT\n  }\n}",
			},
		},
		{
			name: "heredoc preserved but trailing prose after HCL still stripped",
			input: `# File: compute.tf
resource "scaleway_instance_server" "web" {
  user_data = <<-EOF
---
- name: install nginx
EOF
}

**Key fixes from the previous iteration:** fixed resources`,
			expected: map[string]string{
				"compute.tf": "resource \"scaleway_instance_server\" \"web\" {\n  user_data = <<-EOF\n---\n- name: install nginx\nEOF\n}",
			},
		},
		{
			name:  "fenced heredoc content preserved",
			input: "# File: compute.tf\n```hcl\nresource \"x\" \"y\" {\n  data = <<EOF\n---\n- item\nEOF\n}\n```\n\n**Explanation of changes:**",
			expected: map[string]string{
				"compute.tf": "resource \"x\" \"y\" {\n  data = <<EOF\n---\n- item\nEOF\n}",
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

func TestSelfReviewIndicatesNoChanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want bool
	}{
		{name: "exact canonical phrase", text: "NO ISSUES FOUND", want: true},
		{name: "canonical phrase with whitespace", text: "  NO ISSUES FOUND  \n", want: true},
		{name: "canonical phrase case insensitive", text: "no issues found", want: true},
		{name: "prose with no issues phrase", text: "After reviewing, no issues found in the code.", want: false},
		{name: "everything looks correct", text: "Everything looks correct. No changes needed.", want: false},
		{name: "looks good prose", text: "The generated code looks good and follows best practices.", want: false},
		{name: "no changes needed", text: "I've reviewed all files. No changes needed.", want: false},
		{name: "contains file blocks", text: "Looks good but\n# File: main.tf\nterraform {}", want: false},
		{name: "ambiguous prose no pattern", text: "Here are some thoughts about the infrastructure.", want: false},
		{name: "empty string", text: "", want: false},
		{name: "code is correct", text: "The code is correct and implements the plan.", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := SelfReviewIndicatesNoChanges(tc.text)
			if got != tc.want {
				t.Fatalf("SelfReviewIndicatesNoChanges(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}
