package generator

import (
	"bufio"
	"fmt"
	"regexp"
	"strings"
)

var fileHeaderPattern = regexp.MustCompile(`^#\s*File:\s*(.+)\s*$`)

// SelfReviewIndicatesNoChanges returns true only when a self-review response
// exactly matches the canonical "NO ISSUES FOUND" phrase (case-insensitive,
// whitespace-trimmed). This strict check avoids false positives from fuzzy
// substring matching — e.g. "looks good but the IP config is wrong" would
// incorrectly suppress corrections if matched broadly.
func SelfReviewIndicatesNoChanges(text string) bool {
	return strings.EqualFold(strings.TrimSpace(text), "NO ISSUES FOUND")
}
var heredocOpenPattern = regexp.MustCompile(`<<-?\s*(\w+)\s*$`)

// updateHeredocState tracks whether the current line is inside an HCL
// heredoc block. Returns true if the line is heredoc content and markdown
// artifact detection should be skipped. The heredocEnd pointer holds the
// expected closing identifier, or empty string when outside a heredoc.
func updateHeredocState(line string, heredocEnd *string) bool {
	if *heredocEnd != "" {
		if strings.TrimSpace(line) == *heredocEnd {
			*heredocEnd = ""
		}
		return true
	}
	if m := heredocOpenPattern.FindStringSubmatch(line); len(m) == 2 {
		*heredocEnd = m[1]
	}
	return false
}

// ParseFileBlocks parses LLM output in `# File: path` blocks.
// Duplicate files are resolved deterministically using "last block wins".
func ParseFileBlocks(output string) (map[string][]byte, error) {
	files := make(map[string][]byte)

	scanner := bufio.NewScanner(strings.NewReader(output))
	var currentFile string
	var contentLines []string

	flush := func() {
		if currentFile == "" {
			return
		}

		content := strings.Join(contentLines, "\n")
		content = stripCodeFence(content)
		files[currentFile] = []byte(content)
		contentLines = contentLines[:0]
	}

	for scanner.Scan() {
		line := scanner.Text()
		matches := fileHeaderPattern.FindStringSubmatch(line)
		if len(matches) == 2 {
			flush()

			currentFile = strings.TrimSpace(matches[1])
			if currentFile == "" {
				return nil, NewGenerateError(ErrParseFailed, "parse_output", fmt.Errorf("empty filename in header %q", line))
			}
			continue
		}

		if currentFile != "" {
			contentLines = append(contentLines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, NewGenerateError(ErrParseFailed, "parse_output", fmt.Errorf("scan output: %w", err))
	}
	flush()

	if len(files) == 0 {
		return nil, NewGenerateError(ErrParseFailed, "parse_output", fmt.Errorf("no '# File:' blocks found"))
	}

	return files, nil
}

func stripCodeFence(content string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return ""
	}

	lines := strings.Split(trimmed, "\n")
	if len(lines) < 2 {
		return sanitizeBodyLines(lines)
	}
	if strings.HasPrefix(strings.TrimSpace(lines[0]), "```") {
		// Prefer the first fenced payload block and ignore any trailing prose.
		body := make([]string, 0, len(lines)-1)
		var heredocEnd string
		for _, line := range lines[1:] {
			trimmedLine := strings.TrimSpace(line)
			if trimmedLine == "```" {
				break
			}
			inHeredoc := updateHeredocState(line, &heredocEnd)
			if !inHeredoc && isLikelyMarkdownArtifact(trimmedLine) {
				break
			}
			body = append(body, line)
		}
		return sanitizeBodyLines(body)
	}
	return sanitizeBodyLines(lines)
}

func isLikelyMarkdownArtifact(line string) bool {
	if line == "" {
		return false
	}
	// Markdown tables and headers are common contamination from review prose.
	if strings.HasPrefix(line, "|") {
		return true
	}
	if strings.HasPrefix(line, "##") || strings.HasPrefix(line, "###") {
		return true
	}
	// Horizontal rules / YAML separators (never valid HCL).
	if line == "---" || line == "***" || line == "___" {
		return true
	}
	// Bold text, bullet points, blockquotes, numbered lists — all
	// indicate the model started emitting explanatory prose.
	if strings.HasPrefix(line, "**") {
		return true
	}
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
		return true
	}
	if strings.HasPrefix(line, "> ") {
		return true
	}
	return false
}

func sanitizeBodyLines(lines []string) string {
	body := make([]string, 0, len(lines))
	var heredocEnd string
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		inHeredoc := updateHeredocState(line, &heredocEnd)
		if !inHeredoc {
			if strings.HasPrefix(trimmedLine, "```") {
				// Drop markdown fence markers leaking from model output.
				continue
			}
			if isLikelyMarkdownArtifact(trimmedLine) {
				// Stop once the model starts emitting markdown review prose.
				break
			}
		}
		body = append(body, line)
	}
	return strings.TrimSpace(strings.Join(body, "\n"))
}
