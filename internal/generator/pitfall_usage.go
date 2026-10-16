package generator

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// PitfallID is an entry's stable id across runs: a hash of its scope,
// resource and rule text as rendered. The scope is the cloud for an entry
// from <cloud>.yaml and "common" for one from common.yaml, so a common
// rule has one id whichever cloud's run carried it. Any edit to the rule
// is a new id, so a run record names the exact rule its prompt carried.
func PitfallID(e PitfallEntry) string {
	scope := strings.TrimSuffix(e.File, ".yaml")
	sum := sha256.Sum256([]byte(scope + "\x00" + e.Resource + "\x00" + strings.TrimSpace(e.Rule)))
	return hex.EncodeToString(sum[:8])
}

// RenderedPitfalls returns every entry a phase prompt of this generation
// carried, once each, in the order first rendered.
func (m GenerationMetadata) RenderedPitfalls() []PitfallEntry {
	var out []PitfallEntry
	seen := map[string]bool{}
	for _, p := range m.Phases {
		for _, e := range p.Pitfalls {
			if id := PitfallID(e); !seen[id] {
				seen[id] = true
				out = append(out, e)
			}
		}
	}
	return out
}

// renderPhaseWithPitfalls renders a phase prompt with the entries and
// reports the entries it carried: all of them when the rendered pitfalls
// change the prompt, none when the template leaves them out.
func renderPhaseWithPitfalls(phase, templatePath string, ctx PromptContext, entries []PitfallEntry) (string, []PitfallEntry, error) {
	if len(entries) == 0 {
		prompt, err := RenderPromptFile(phase, templatePath, ctx)
		return prompt, nil, err
	}
	ctx.Pitfalls = renderPitfalls(entries)
	prompt, err := RenderPromptFile(phase, templatePath, ctx)
	if err != nil {
		return "", nil, err
	}
	ctx.Pitfalls = ""
	without, err := RenderPromptFile(phase, templatePath, ctx)
	if err != nil {
		return "", nil, err
	}
	if without == prompt {
		return prompt, nil, nil
	}
	return prompt, entries, nil
}

// MatchBrokenPitfall returns the id of the first rendered entry whose
// rule the failing resource's own block in files breaks, or "". The
// failing resource is the failure's address, else the first address in
// its detail; a module path is stripped to the resource's type and name.
//
// Only an avoid rule (ParseAvoidRule) can be judged against HCL: it is
// broken when the block sets every attribute it names (setsAll). Any
// other rule never matches.
func MatchBrokenPitfall(files map[string][]byte, failureResource, failureDetail string, rendered []PitfallEntry) (string, error) {
	address := failureResource
	if address == "" {
		address = firstResourceAddress(failureDetail)
	}
	resourceType, name, ok := resourceTypeAndName(address)
	if !ok {
		return "", nil
	}
	blocks, err := resourceBlocks(files, resourceType)
	if err != nil {
		return "", fmt.Errorf("read generated HCL: %w", err)
	}
	for _, b := range blocks {
		if b.Labels[1] != name {
			continue
		}
		for _, e := range rendered {
			resource, attrs, ok := ParseAvoidRule(e.Rule)
			if ok && e.Resource == resourceType && resource == resourceType && setsAll(b.Body, attrs) {
				return PitfallID(e), nil
			}
		}
	}
	return "", nil
}

// resourceTypeAndName resolves a resource address, module-qualified or
// indexed (module.net.aws_subnet.a[0]), to its type and name.
func resourceTypeAndName(address string) (string, string, bool) {
	for strings.HasPrefix(address, "module.") {
		rest := strings.TrimPrefix(address, "module.")
		i := strings.Index(rest, ".")
		if i < 0 {
			return "", "", false
		}
		address = rest[i+1:]
	}
	resourceType, name, ok := splitAddress(address)
	if i := strings.Index(name, "["); i >= 0 {
		name = name[:i]
	}
	return resourceType, name, ok && name != ""
}
