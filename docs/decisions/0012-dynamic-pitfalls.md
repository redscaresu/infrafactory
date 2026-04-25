# ADR-0012: Dynamic Pitfalls by Cloud Provider

## Status
Accepted

## Context
Provider pitfalls (e.g., "don't use `ip_id`, use `ip_ids`") were hardcoded in `prompts/phase2_generate_hcl.md`. Every new pitfall required a code change. The LLM feedback loop could self-correct within a single run, but forgot the fix between runs — rediscovering the same mistake every time.

## Decision
Externalize pitfalls into `pitfalls/{cloud}.yaml` files loaded at runtime based on the scenario's `cloud` field. Implement auto-learning: when a run self-corrects (iteration N fails, N+1 succeeds), extract the error pattern and append it to the pitfalls file.

1. **File-per-provider**: `pitfalls/scaleway.yaml`, `pitfalls/gcp.yaml`, etc. Optional `pitfalls/common.yaml` merged into all providers.
2. **Runtime injection**: `LoadPitfalls(dir, cloud)` renders pitfalls as markdown, injected via `{{.Pitfalls}}` in phases 2 and 3.
3. **Auto-learning**: `ExtractLearnedPitfall` parses failure details for actionable patterns (password constraints, unsupported arguments, missing config). `AppendPitfall` writes to YAML with deduplication and `source: learned`.
4. **Conservative extraction**: Only specific, actionable errors produce pitfalls. Vague errors ("test checks failed") are ignored.
5. **Best-effort**: Learning errors are logged but never break the run.

## Consequences
**Benefits**:
- New pitfalls added by editing a YAML file — no code changes.
- System gets smarter over time — each self-correcting run teaches future runs.
- Multi-provider ready — add `pitfalls/gcp.yaml` when GCP support lands.
- Deduplication prevents pitfall bloat.

**Tradeoffs**:
- Learned pitfalls may be noisy if extraction patterns are too broad.
- YAML file grows over time — may need periodic human review to promote `learned` → `static` or prune low-value entries.
- Conservative extraction means some learnable patterns are missed.
