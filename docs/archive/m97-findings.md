# M97 findings — template-matched prescriptive rules

Result of taking the cheap "hardcode prescriptive rules for 5 common
error shapes" path against the M96 problem (descriptive learned rules
don't help the LLM converge).

## What shipped

5 templates in `internal/generator/pitfalls_learn.go::ExtractLearnedPitfall`:

1. `matchMissingSubnetwork` — "no subnetwork" → declare network + subnetwork + reference via network_interface
2. `matchMissingEncryption` — **disabled at ship** (see below)
3. `matchNotImplemented` — 501 → don't use this resource
4. `matchOAuthEscape` — 401 OAuth → check `*_custom_endpoint`
5. `matchDestroyBlockers` — deletion_protection / force_destroy / skip_final_snapshot

Each has a regression test. The descriptive fallback still runs when none of these match.

## What the validation run showed

`gcp-full-stack × 5 passes` with M97 active, starting from 0 learned pitfalls:

| Pass | Iters | Time | Terminal | Learned before | Learned after |
|---|---|---|---|---|---|
| 1 | 2 | 261s | stuck | 2 | 4 |
| 2 | 4 | 736s | stuck | 4 | 4 |
| 3 | 2 | 392s | stuck | 4 | 4 |
| 4 | 2 | 307s | stuck | 4 | 4 |
| 5 | 2 | 289s | stuck | 4 | 4 |

(2 entries seeded by an earlier run before stripping for this validation.)

**iter count did not drop.** The templates fire, learning happens, but the LLM still hits the same wall every pass.

## Why — two upstream bugs surfaced

### (1) OPA `vpc_required.rego` false-positives on known-after-apply references

Inspecting the LLM's generated HCL on pass 5 iter 2:

```hcl
resource "google_compute_instance" "api_server" {
  network_interface {
    network    = google_compute_network.main.id
    subnetwork = google_compute_subnetwork.private.id    # ← present
  }
}
```

The HCL IS correct. But `policies/gcp/vpc_required.rego` fires:

```
google_compute_instance.api_server has no network_interface.subnetwork
```

Root cause: the policy checks `resource.values.network_interface[_].subnetwork != null/""`. At plan time, `google_compute_subnetwork.private.id` resolves to `null` in `planned_values` because the subnetwork hasn't been created yet (`known after apply`). The policy doesn't account for references. Same shape applies to `policies/gcp/encryption.rego` and probably others.

This is filed as **M98**. The auto-learning loop can't possibly close the gap when the gate itself is firing on correct HCL.

### (2) The CMEK template was actively misleading

My initial `matchMissingEncryption` template told the LLM:

> "Test scenarios MUST omit customer-managed encryption (CMEK) entirely."

But `policies/gcp/encryption.rego` enforces the opposite:

> "google_storage_bucket — must declare encryption.default_kms_key_name (CMEK)"

The template was DIRECTLY CONTRADICTING the OPA gate. The LLM seeing both would be told to do opposite things. **Disabled the template at ship**; descriptive fallback still fires so the LLM at least sees the failure in context. A correct prescriptive form needs cross-policy awareness (M98 scope).

## Honest summary

**M97 templates work mechanically** — they extract prescriptive rules from common error shapes. But:

- **Sufficient conditions for convergence include the gates themselves working correctly.** M98 surfaced that `vpc_required.rego` flags valid HCL because it doesn't grok terraform references.
- **A template that contradicts a gate is worse than no template.** The CMEK case is filed as a warning.
- The mechanical learning loop (M86+M90+M91+M92) is one layer; the gate-correctness is another. Both have to be right.

## What to ship next

**M98** — make `policies/gcp/vpc_required.rego` and `policies/gcp/encryption.rego` recognize references (or move the checks to post-apply). Until that lands, even a perfect prescriptive-rule generator can't close the gcp-full-stack failures.

## Reproducibility

```
# Strip + re-run multi-pass
python3 -c "import yaml; d = yaml.safe_load(open('pitfalls/gcp.yaml')); d['pitfalls'] = []; yaml.dump(d, open('pitfalls/gcp.yaml','w'))"
go build -o bin/infrafactory ./cmd/infrafactory
bash scripts/m95_multipass.sh
```

Look at any `iteration_N` failure detail — if it mentions `network_interface.subnetwork` but the generated `compute.tf` has it attached, that's M98.
