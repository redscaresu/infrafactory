# Plan: Dynamic Pitfalls (Slice 32)

## Context

Provider pitfalls are currently hardcoded in `prompts/phase2_generate_hcl.md`. Every new pitfall requires a code change. The LLM feedback loop can self-correct within a single run, but forgets the fix between runs — the same mistake gets rediscovered every time.

This slice externalizes pitfalls into a standalone YAML file that:
1. Gets injected into prompts via a template variable (replacing the hardcoded section)
2. Can grow over time without code changes
3. Eventually supports auto-discovery from run feedback (future slice)

## Quick Reference

| Key | Value |
|---|---|
| Slice | 32 |
| Ticket IDs | S32-T1 through S32-T5 |
| Depends on | Slice 31 (done) |
| Core directory | `pitfalls/` (one YAML per provider) |
| Files | `pitfalls/scaleway.yaml`, `pitfalls/gcp.yaml`, etc. |
| Template variable | `{{.Pitfalls}}` |

## Architecture

```
  pitfalls/
  ├── scaleway.yaml              <-- Scaleway provider pitfalls
  ├── gcp.yaml                   <-- GCP provider pitfalls (future)
  ├── aws.yaml                   <-- AWS/EKS pitfalls (future)
  └── common.yaml                <-- Cross-provider pitfalls (future)
         |
         v
  Load by scenario cloud field   <-- scenario.cloud = "scaleway" → load scaleway.yaml
         |
         v
  PromptContext.Pitfalls           <-- rendered as markdown
         |
         v
  {{.Pitfalls}} in prompts         <-- injected into phase 2 + phase 3
```

The scenario YAML already has a `cloud: scaleway` field. At runtime, load `pitfalls/{cloud}.yaml`. If a `common.yaml` exists, merge it in too.

## Pitfalls File Format

```yaml
# pitfalls/scaleway.yaml
provider: scaleway
pitfalls:
  - resource: scaleway_k8s_cluster
    rule: >
      Version and auto_upgrade MUST be consistent: without auto_upgrade
      use a full patch version like "1.31.2"; with auto_upgrade use only
      a minor version like "1.31".
    source: static  # "static" = manually written, "learned" = auto-discovered

  - resource: scaleway_instance_server
    rule: >
      Use ONLY the exact instance type from the architecture plan.
      Do NOT invent types like GP1-L or GP1-XL.
    source: static

  - resource: scaleway_domain_record
    rule: >
      Do NOT create DNS records unless the scenario explicitly lists
      DNS/domain resources. The dns_resolution criterion is auto-evaluated.
    source: learned
    discovered_from: web-app-paris
```

```yaml
# pitfalls/gcp.yaml (future)
provider: gcp
pitfalls:
  - resource: google_compute_instance
    rule: >
      Use machine_type short names (e.g. "e2-micro") not full URLs.
    source: static
```

## Tickets

### S32-T1: Create `pitfalls/scaleway.yaml` with all existing pitfalls

Port all 16 pitfalls from `prompts/phase2_generate_hcl.md` into the YAML file. Remove the hardcoded `## Scaleway Provider Pitfalls` section from the prompt template.

**Files**:
- `pitfalls/scaleway.yaml` (new)
- `prompts/phase2_generate_hcl.md` (remove pitfalls section, add `{{if .Pitfalls}}` block)

### S32-T2: Add `Pitfalls` field to `PromptContext` and load by cloud provider

- Add `Pitfalls string` to `PromptContext` in `internal/generator/prompt.go`
- Add `Pitfalls string` to `PromptContext` (empty string for phase 1, populated for phases 2+3)
- Add `Cloud string` to `generator.Request` — the CLI layer (which already parses scenario YAML) populates this
- Load `pitfalls/{cloud}.yaml` based on `req.Cloud`. If `pitfalls/common.yaml` exists, append those pitfalls after provider-specific ones (no deduplication needed — each rule is unique text)
- Render pitfalls as markdown bullet list grouped by resource
- Add `Pitfalls` path to `PathsConfig` in config.go — **must be in same commit as the `infrafactory.yaml` change** because `KnownFields(true)` rejects unknown YAML keys
- Inject into phase 2 template via `{{if .Pitfalls}}` block. Phase 3 gets pitfalls too so the self-review checklist can verify compliance.

**YAML note**: Use `>` (folded scalar) for single-paragraph rules. Use `|` (literal scalar) for rules containing code examples.

**Files**:
- `internal/generator/prompt.go` — add `Pitfalls` field
- `internal/generator/generator.go` — add `Cloud string` to `Request`
- `internal/generator/pitfalls.go` (new) — `LoadPitfalls(dir, cloud string) (string, error)`
- `internal/generator/claude_adapter.go` — populate `req.Cloud`, load and inject pitfalls
- `internal/generator/openrouter_adapter.go` — same
- `internal/cli/generate_command.go` — pass `Cloud` from scenario to `Request`
- `internal/config/config.go` — add `Pitfalls` to `PathsConfig`
- `infrafactory.yaml` — add `paths.pitfalls: ./pitfalls`
- `prompts/phase2_generate_hcl.md` — replace hardcoded section with `{{if .Pitfalls}}` block
- `prompts/phase3_self_review.md` — add `{{if .Pitfalls}}` block in review checklist

### S32-T3: Tests

- Unit test: pitfalls YAML loads and renders correctly
- Unit test: PromptContext includes pitfalls in rendered output
- Unit test: empty/missing pitfalls file produces empty section (no crash)
- Verify all 12 scenarios still pass `infrafactory test`

### S32-T4: Update docs

- CONCEPT.md: document dynamic pitfalls architecture
- AGENTS.md: document pitfalls file location and format
- README.md: mention pitfalls in the How It Works section
- BACKLOG.md/ROADMAP.md: Slice 32 tickets

### S32-T5: Future — auto-discovery (design only, no implementation)

Document the design for auto-learning pitfalls from run feedback:
- After a successful retry (iteration N fails, iteration N+1 passes), extract the error→fix pattern
- Append to `pitfalls/scaleway.yaml` with `source: learned`
- Future slice to implement

## Execution Order

```
S32-T1 (create pitfalls file) ─────── first
S32-T2 (wire into prompts) ────────── depends on T1
S32-T3 (tests) ────────────────────┐
S32-T4 (docs) ─────────────────────┼── parallel, depend on T2
S32-T5 (future design doc) ──────── last, depends on T3
```

## Verification

```bash
make test
# Then verify scenarios still work:
infrafactory test scenarios/training/web-app-paris.yaml
```

## Out of Scope

- Auto-discovery implementation (future slice, design only in T5)
- Provider-agnostic pitfalls format (Scaleway-only for now)
- UI for managing pitfalls (edit the YAML file directly)
