package generator

import (
	"bufio"
	"fmt"
	"regexp"
	"strings"
)

var fileHeaderPattern = regexp.MustCompile(`^#\s*File:\s*(.+)\s*$`)

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
		return trimmed
	}
	if !strings.HasPrefix(strings.TrimSpace(lines[0]), "```") {
		return trimmed
	}
	if strings.TrimSpace(lines[len(lines)-1]) != "```" {
		return trimmed
	}

	return strings.Join(lines[1:len(lines)-1], "\n")
}
