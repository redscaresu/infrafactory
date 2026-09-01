package generator

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// relativeMarkdownLink matches `](target.md)` and `](dir/target.md)`,
// skipping anchors and absolute URLs.
var relativeMarkdownLink = regexp.MustCompile(`\]\(([^)#][^)]*?\.md)\)`)

// TestDocLinksResolve is a ratchet on relative links between markdown
// documents.
//
// STATUS.md lives at the repository ROOT while almost everything it
// points at lives under docs/, so `](plans/x.md)` looks right and
// resolves to nothing. Four such links were written in one session before
// a reviewer noticed the first (S158, pass 69) — the mistake is easy,
// invisible in review, and only shows up when somebody follows the link
// and gives up.
//
// Cheap to enforce and the repository was already clean when this landed,
// so it starts green and stays that way. Same idiom as the cloud-prefix
// lockstep and pitfall-source ratchets: when a convention needs
// enforcing, a small audit beats a written rule.
func TestDocLinksResolve(t *testing.T) {
	root := repoRoot(t)

	var files []string
	for _, name := range []string{"STATUS.md", "README.md", "AGENTS.md", "CHANGELOG.md"} {
		if path := filepath.Join(root, name); fileExists(path) {
			files = append(files, path)
		}
	}
	err := filepath.Walk(filepath.Join(root, "docs"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".md") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk docs: %v", err)
	}
	if len(files) < 2 {
		t.Fatalf("audited %d file(s); the walk found nothing, so this ratchet is vacuous", len(files))
	}

	for _, file := range files {
		payload, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for _, match := range relativeMarkdownLink.FindAllStringSubmatch(stripCode(string(payload)), -1) {
			target := match[1]
			if strings.HasPrefix(target, "http") {
				continue
			}
			resolved := filepath.Join(filepath.Dir(file), target)
			if !fileExists(resolved) {
				rel, _ := filepath.Rel(root, file)
				t.Errorf("%s links to %q, which does not exist (resolved to %s)", rel, target, resolved)
			}
		}
	}
}

// stripCode blanks fenced blocks and inline code spans before links are
// matched.
//
// A document explaining this very ratchet writes `](plans/x.md)` in
// backticks as an example of the mistake, and a checker that cannot tell
// an illustration from a link fails on the prose describing it -- which
// is exactly what happened when this test first ran.
//
// Replaced with spaces rather than deleted so nothing outside the span
// shifts position and the surrounding text still matches as it should.
func stripCode(doc string) string {
	out := []byte(doc)
	blank := func(from, to int) {
		for i := from; i < to && i < len(out); i++ {
			if out[i] != '\n' {
				out[i] = ' '
			}
		}
	}

	// Fenced blocks first: a ``` region may contain unbalanced single
	// backticks that would otherwise throw the inline pass off.
	for _, m := range regexp.MustCompile("(?s)```.*?```").FindAllStringIndex(doc, -1) {
		blank(m[0], m[1])
	}
	// Then inline spans, over what is left.
	for _, m := range regexp.MustCompile("`[^`\n]*`").FindAllStringIndex(string(out), -1) {
		blank(m[0], m[1])
	}
	return string(out)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
