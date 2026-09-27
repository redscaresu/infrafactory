package main

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The index is generated, so a stale one is a failing test rather than a
// review comment. Fix by running `make adr-index`.
func TestADRIndexIsCurrent(t *testing.T) {
	dir := filepath.Join("..", "..", "docs", "decisions")
	readme, err := os.ReadFile(filepath.Join(dir, "README.md"))
	require.NoError(t, err)

	index, err := render(os.DirFS(dir))
	require.NoError(t, err)
	want, err := splice(string(readme), index)
	require.NoError(t, err)

	assert.Equal(t, want, string(readme), "docs/decisions/README.md index is stale: run `make adr-index`")
}

func TestNormalizeStatus(t *testing.T) {
	for raw, want := range map[string]string{
		"Accepted — 2026-09-20 (S187)":                          "Accepted",
		"accepted (2026-09-01, S160a)":                          "Accepted",
		"Proposed":                                              "Proposed",
		"Superseded by ADR-0010":                                "**Superseded by ADR-0010**",
		"**SUPERSEDED by ADR-0031 on the same day.** Its":       "**Superseded by ADR-0031**",
		"Accepted (supersedes ADR-0003)":                        "Accepted; supersedes ADR-0003",
		"Accepted (amends ADR-0010)":                            "Accepted; amends ADR-0010",
		"Accepted. **Supersedes ADR-0030 entirely** and":        "Accepted; supersedes ADR-0030",
		"Accepted — **with its central mechanism claim REFUTED": "Accepted, mechanism refuted",
	} {
		assert.Equal(t, want, normalizeStatus(raw), raw)
	}
}

func TestRenderReadsBothStatusForms(t *testing.T) {
	fsys := fstest.MapFS{
		"0001-a.md":          {Data: []byte("# ADR-0001: First\n\n## Status\n\nAccepted\n")},
		"0002-b.md":          {Data: []byte("# ADR-0002: Second\n\nStatus: superseded by ADR-0001\n")},
		"ADR_TEMPLATE.md":    {Data: []byte("# not an ADR\n")},
		"DECISION_RUBRIC.md": {Data: []byte("# not an ADR\n")},
	}

	got, err := render(fsys)
	require.NoError(t, err)
	assert.Equal(t,
		"- [0001](0001-a.md) First — Accepted\n"+
			"- [0002](0002-b.md) Second — **Superseded by ADR-0001**\n",
		got)
}

// A new ADR that does not say whether it is in force cannot enter the
// index silently.
func TestRenderRefusesAnADRWithNoStatus(t *testing.T) {
	_, err := render(fstest.MapFS{"0001-a.md": {Data: []byte("# ADR-0001: First\n\nbody\n")}})
	assert.ErrorContains(t, err, "0001-a.md: no status")
}

func TestSpliceRequiresMarkers(t *testing.T) {
	_, err := splice("# Decision Records\n", "- x\n")
	assert.ErrorContains(t, err, "missing")
}
