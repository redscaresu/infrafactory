# Slice 51 — Collapse `constraints` into `acceptance_criteria`

Status: in_progress (2026-05-23)
Owner: claude + codex

## Motivation

The scenario YAML today carries two sections that describe the same intent
in slightly different shapes:

```yaml
constraints:
  region: fr-par
  encryption_at_rest: true

acceptance_criteria:
  - type: policy
    check: encryption_at_rest
    expect: pass
  - type: destruction
    expect: no_orphans
```

`constraints` is a free-form map fed to OPA as `input.constraints`;
`acceptance_criteria` is the list of executable checks the run-loop
iterates over. Empirically, audit of `policies/**/*.rego` shows:

- **3 of N** rego files actually read `input.constraints` (Scaleway +
  GCP + AWS `region_restriction.rego` — they need `region` / `zone`
  as a parameter).
- Every other `constraints` entry is **decorative** — `encryption_at_rest:
  true` doesn't feed any policy logic; the policy hardcodes the check.

So two redundancy patterns are hiding in the same field:
1. **Real parameters** policies consume (`region`, `zone`).
2. **Decorative declarations** that just restate what the policy
   already encodes (`encryption_at_rest: true`).

Pattern (2) is dead weight; pattern (1) belongs *inside* the criterion
that uses it. The collapse:

```yaml
acceptance_criteria:
  - type: policy
    check: region_restriction
    params: { region: fr-par }
  - type: policy
    check: encryption_at_rest
  - type: destruction
    expect: no_orphans
```

Wins:
- No top-level field that can quietly hold values nobody enforces (no
  more "wish list" footgun).
- Every parameter has a visible consumer.
- One concept (criteria) instead of two (constraints + criteria).

## Design decision: option A (auto-discovery preserved)

Validate-time policy evaluation today auto-discovers every `.rego` in
`policy_paths` and runs them all against the plan, with `sc.Constraints`
fed as `input.constraints`. State-time evaluation (`test_command.go`) is
already criterion-driven via `cloudConstraintPolicies`.

Two implementation paths:
- **(A) Keep auto-discovery; merge all criterion `params` maps into
  one `input.params` blob.** Closest to today; only the field name
  flips from `constraints` → `params`. Minimal blast radius.
- **(B) Flip validate to fully criteria-driven** — only evaluate
  policies named by criteria. Cleaner mental model but breaks any
  scenario relying on a rego file's deny rule auto-firing.

This slice ships **option A**. Auto-discovery semantics are preserved;
the only contract change is the field name (`input.constraints` →
`input.params`) and where the values come from (scenario-level
`constraints` map → merged criterion `params` maps).

If a follow-up wants option B, it's a separate slice that touches only
`validate_command.go` + adds a new audit asserting every rego file is
named by ≥1 criterion.

## Tickets

| id | title | priority | deps |
|---|---|---|---|
| S51-T1 | Schema + loader struct: drop top-level `constraints`, add `params` to policy criterion. Update `Scenario` and `AcceptanceCriterion` Go structs; thread `Params` through `PolicyCheckSpec`. | P1 | — |
| S51-T2 | OPA helper rename + validate_command rewire: `EvaluatePlanPoliciesWithConstraints` → `EvaluatePlanPolicies` (params arg); merge criterion params at validate site; rename `input.constraints` → `input.params` in the 3 region_restriction rego files. | P1 | S51-T1 |
| S51-T3 | Surface cleanup: drop `Constraints` field from `scenarioDetailResponse` (API), `ScenarioDetail` (TS types), and `internal/generator/*adapter.go` + `prompt.tmpl` prompt-injection blocks. Replace with serialized criterion-list view if anything downstream needs it. | P1 | S51-T1 |
| S51-T4 | Scenario YAML migration: rewrite all 35 scenarios — fold parametric `constraints` values into the matching criterion's `params`; drop decorative entries with no consuming policy. Dispatched to codex agent with explicit per-pattern mapping table. | P1 | S51-T1, S51-T2 |
| S51-T5 | Full verification: `make test` green (Go unit + UI unit + Playwright e2e + pre-commit gitleaks). Each fakeaws / fakegcp / mockway working example still applies cleanly (smoke). Every scenario in `scenarios/training/` loads without validation error. | P1 | S51-T1..T4 |

## Per-pattern migration table (used by S51-T4 agent)

| Old `constraints` key | Migration |
|---|---|
| `region: <value>` | Add `params: {region: <value>}` to the criterion `check: region_restriction` (cloud-specific). If no such criterion exists today, ADD one with `expect: pass`. |
| `zone: <value>` | Same as region — fold into the `region_restriction` criterion's params. |
| `encryption_at_rest: true` | Drop. The matching `check: encryption_at_rest` criterion (if present) doesn't read input.constraints; the policy already encodes the assertion. |
| `no_public_endpoints: true` | Drop. Same reason as encryption_at_rest. |
| `vpc_required: true` | Drop. |
| `<other boolean flag>` | Drop unless `grep -r input.constraints.<key>` finds a policy that reads it. If found, fold into the matching criterion's params. |

The migration's success metric: every modified scenario still loads
(`go test ./internal/scenario/...`), and every existing acceptance
criterion still has the same intent — no criterion is dropped, no new
criterion is added except the region_restriction one promoted from a
former constraint.

## Exit criteria

- `grep -rn "sc\.Constraints" internal/` returns zero hits.
- `grep -rn "input\.constraints" policies/` returns zero hits.
- `grep -rn "^constraints:" scenarios/` returns zero hits.
- `make test` is green (Go unit + UI unit + Playwright e2e).
- Every working terraform example in `../fakegcp/examples/working/`,
  `../fakeaws/examples/working/`, `../mockway/examples/working/` still
  `tofu validate` cleanly (mocks don't read constraints, but smoke-test
  anyway since the schema change touches the shared loader).

## Backward compatibility

Hard cut. The scenario corpus is 35 files all in this repo; we own
every one. A grace-period parser that accepts both old `constraints:`
and new `params:` shapes would double the work for no real benefit —
nobody outside this tree consumes scenarios.

The CLI does NOT need to be a stable contract for external users at
this point; this is in active development with one team owning every
scenario + every policy file. Re-opening that contract decision is
out of scope for S51.
