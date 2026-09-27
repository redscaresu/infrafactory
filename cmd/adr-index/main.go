// Command adr-index regenerates the ADR index in docs/decisions/README.md
// from the ADR files themselves.
//
// The index is read by every fresh session, and it used to be written by
// hand: one paragraph per ADR, restating the ADR. A hand-written copy of a
// document drifts from it, and the paragraphs had grown into a second
// version of every decision. The title and status are now the index, and
// TestADRIndexIsCurrent fails CI when the README disagrees with the files.
package main

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	startMarker = "<!-- adr-index:start -->"
	endMarker   = "<!-- adr-index:end -->"
)

var (
	adrFile      = regexp.MustCompile(`^\d{4}-.*\.md$`)
	titlePrefix  = regexp.MustCompile(`^# ADR-\d{4}:\s*`)
	supersededBy = regexp.MustCompile(`(?i)superseded by (ADR-\d{4})`)
	relation     = regexp.MustCompile(`(?i)\b(supersedes|amends) (ADR-\d{4})`)
)

func main() {
	dir := filepath.Join("docs", "decisions")
	readme := filepath.Join(dir, "README.md")
	if err := run(dir, readme); err != nil {
		fmt.Fprintln(os.Stderr, "adr-index:", err)
		os.Exit(1)
	}
}

func run(dir, readme string) error {
	current, err := os.ReadFile(readme)
	if err != nil {
		return err
	}
	index, err := render(os.DirFS(dir))
	if err != nil {
		return err
	}
	updated, err := splice(string(current), index)
	if err != nil {
		return err
	}
	return os.WriteFile(readme, []byte(updated), 0o644)
}

// render builds one line per ADR, in file-name order.
func render(fsys fs.FS) (string, error) {
	names, err := fs.Glob(fsys, "*.md")
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, name := range names {
		if !adrFile.MatchString(name) {
			continue
		}
		raw, err := fs.ReadFile(fsys, name)
		if err != nil {
			return "", err
		}
		title, rawStatus, err := parseADR(string(raw))
		if err != nil {
			return "", fmt.Errorf("%s: %w", name, err)
		}
		fmt.Fprintf(&b, "- [%s](%s) %s — %s\n", name[:4], path.Base(name), title, normalizeStatus(rawStatus))
	}
	return b.String(), nil
}

// parseADR returns the title and the first line of the status. Two status
// forms exist in the corpus: a `## Status` heading followed by a line, and
// a `Status:` line. A file with neither fails, so a new ADR cannot enter
// the index without saying whether it is in force.
func parseADR(doc string) (title, status string, err error) {
	lines := strings.Split(doc, "\n")
	for i, line := range lines {
		switch {
		case title == "" && strings.HasPrefix(line, "# "):
			title = strings.TrimSpace(titlePrefix.ReplaceAllString(line, ""))
		case status == "" && strings.HasPrefix(strings.ToLower(line), "status:"):
			status = strings.TrimSpace(line[len("status:"):])
		case status == "" && strings.TrimSpace(line) == "## Status":
			status = nextNonEmpty(lines[i+1:])
		}
	}
	if title == "" {
		return "", "", fmt.Errorf("no `# ADR-NNNN: Title` heading")
	}
	if status == "" {
		return "", "", fmt.Errorf("no status (`## Status` section or `Status:` line)")
	}
	return title, status, nil
}

func nextNonEmpty(lines []string) string {
	for _, l := range lines {
		if s := strings.TrimSpace(l); s != "" {
			return s
		}
	}
	return ""
}

// normalizeStatus reduces a free-text status to what someone scanning the
// index needs: is this in force, and if not, what replaced it.
func normalizeStatus(raw string) string {
	if m := supersededBy.FindStringSubmatch(raw); m != nil {
		return "**Superseded by " + strings.ToUpper(m[1]) + "**"
	}
	lower := strings.ToLower(raw)
	status := "Accepted"
	if strings.Contains(lower, "proposed") {
		status = "Proposed"
	}
	if strings.Contains(lower, "refuted") {
		status += ", mechanism refuted"
	}
	if m := relation.FindStringSubmatch(raw); m != nil {
		status += "; " + strings.ToLower(m[1]) + " " + strings.ToUpper(m[2])
	}
	return status
}

// splice replaces the text between the markers, leaving the rest of the
// README (preamble, rubric pointer, template) as written.
func splice(readme, index string) (string, error) {
	start := strings.Index(readme, startMarker)
	end := strings.Index(readme, endMarker)
	if start < 0 || end < start {
		return "", fmt.Errorf("README is missing %s ... %s", startMarker, endMarker)
	}
	return readme[:start+len(startMarker)] + "\n" + index + readme[end:], nil
}
