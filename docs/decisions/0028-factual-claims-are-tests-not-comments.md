# ADR-0028: a factual claim about another component is a test, not a comment

Status: accepted (2026-09-07, S169)

## Context

This codebase writes long "why" comments deliberately, and they are load-bearing:
they are how a decision survives being read six months later. That style is a
strength and it is also the vector for the most expensive defects found so far.

In the week of 2026-09-01, every serious review finding was the same shape — **not
broken code, but a confident claim about how another component behaves, written
into a comment and never checked**:

| claim | reality |
|---|---|
| "the YAML error names the file it was read from" | `yaml.Unmarshal` receives bytes; it has no filename |
| "the cause is on the command's stderr" | `runDeployCommand` is called directly, and cobra only prints from `Execute()` |
| "there is no spelling that does not call `.Error()`" | `fmt.Sprintf("%v", err)` is one |
| "the console groups by stage" | nothing reads that field |
| "a dispose that never calls back, exactly like `connectWS`" | it calls back, and late |

Each was checkable in under two minutes. Each propagated — into an ADR, a PR body,
a commit message, a report to the operator — because a well-argued paragraph reads
as something somebody already verified. **A rule that is wrong fails once; a rule
that is wrong and carries a confident rationale keeps failing until somebody runs
it.**

Three of them reached already-merged slices, which is the other half of the
problem: a comment cannot notice when the dependency underneath it changes.

## Decision

A factual claim about another component may not be written as a comment. It is
written as a **test**, and the comment cites it by name.

```go
// yaml.Unmarshal has no filename to report
// (TestAssumption_YAMLErrorsCannotNameTheFile).
ParseError: err.Error(),
```

Facts go in tests. **Decisions stay in prose** — a trade-off ("a tee rather than an
adapter", "the exit code stays 0 because forget is deliberate") cannot be tested and
belongs in a comment or an ADR, which is what those are for.

Assumption tests live in `assumptions_test.go` per package, named
`TestAssumption_<claim>`, and each names the site that relies on it.

**The citation runs both ways**, and both directions earn their keep. The test
names its relying site, so deleting the site is visibly the moment the test stops
mattering. The site names its test, so a reader who doubts the claim verifies it
with one grep instead of reconstructing an argument — which is precisely what
nobody did for the five claims that failed.

## Why not a convention

Because S167 is the counter-example. "Do not put an error's text in a response body"
was a known rule, and it was violated at sixty-five sites; only a failing `go test`
stopped it. Adding "verify your claims" to the same pile would be the same mistake
one level up.

This works instead because it **inverts the cost**. Asserting used to be cheaper
than checking; now the check is the artifact. You cannot cite a test you did not
write, and writing it runs it — the discipline and the deliverable become one
action, with no gap for good intentions.

## Consequences

- Claims fail loudly when a dependency changes, which prose never did.
- A reviewer verifies a claim with one grep instead of reconstructing an argument.
- Test volume grows, bounded by "claims load-bearing enough to write a paragraph
  about".
- **It does not reach everything.** Facts about real Scaleway are expensive to test
  and stay prose — the S168 teardown finding is exactly that kind, and remains a
  risk this ADR does not cover.


## Amendment, 2026-09-21 (S193): an undefined rego rule is not a passing one

Rego rules are **undefined** rather than false when they do not exist, and an undefined
rule evaluates to zero results — which is exactly what a defined rule that found nothing
wrong returns. The evaluator cannot tell the two apart, so both render green.

That turned a scenario's `acceptance_criteria` into a claim nobody was checking.
`web-live-paris` names `check: region_restriction`; the run reported
`state_policy: pass`; and `region_restriction.rego` had no `deny_state` rule, so nothing
was evaluated against deployed state at all.

### Decision

**A policy named by a criterion that has no `deny_state` rule is reported, not passed.**
Rule existence is answered from the **AST** (`ast.ParseModule`), not by grepping the file
— a rule named in a comment or inside a string is not a definition.

It is a **skip**, not a failure: some policies are legitimately plan-only, and failing
them would refuse scenarios that are not wrong. The requirement is that the gap is
visible.

**Except for `expect: fail`,** which is a failure. A negative criterion asks the policy to
deny; a policy with no state rule can never deny, so skipping would report success for an
assertion nothing could satisfy.

**And the check emits exactly one stage.** A skip beside a pass is two contradictory
claims about the same thing, and the green one is the one people read. Green here means
*every policy this scenario named was evaluated against deployed state and none denied* —
nothing weaker earns it.

### The enabling fix

The state evaluator forwarded only `target`, never the criterion's `params`. So
`input.params.region` was undefined and **any** parameterised state rule would have
silently never fired. The rule was not merely unwritten; it was not writable. Params now
go through exactly as the plan evaluator passes them.

### Why this belongs to ADR-0028

The rule here is *factual claims are tests, not comments*. This is the same failure one
level down: a claim encoded as a **criterion** rather than a comment, which the system
accepted, displayed, and never evaluated. An assertion that cannot fail is documentation
wearing a test's clothes.

Two counting bugs while implementing it made the same mistake in miniature — "how many
were evaluated" was derived first by parsing the skip message's prose, then by subtracting
the skip count from the spec count. Both derive a structured fact from something shaped
for humans, inside the function that exists to stop a check being reported as run when it
was not. It is counted where the evaluation happens now.

## Amendment, 2026-09-28: a policy's claim is a mutation-checked Rego test

A policy is a claim too: its name, its comments and the criterion that cites it all say what it
denies. That claim is now a Rego unit test beside the policy (`<name>_test.rego`, `test_` rules),
which `go test` runs through `opa/v1/tester` (`TestRegoPolicyTests`,
`internal/harness/rego_policy_test.go`).

A policy test counts only when it is **mutation-checked**: each `deny`/`deny_state` body in turn
is made unsatisfiable, and a body after which no test fails is a failure. A test that passes
whether or not the rule exists is a comment with a green tick. Inputs are trimmed from a real
capture, a plan from the pinned provider or the mock's `/mock/state`, never hand-shaped: a
hand-shaped input tests the author's belief about the shape, which is the claim under test.

**conftest was not adopted.** It would replace an evaluator that was not the problem, and cost
`deny_state`, params, per-cloud routing and repair-loop feedback to rebuild. What was missing was
tests, and the OPA library infrafactory already uses runs them.

## Amendment, 2026-10-11: the tofu and provider claims the import-production-state rehearsal rests on

The rehearsal design (`docs/hld/2026-09-30-import-production-state-into-the-mocks.md`) rests on
claims about tofu and the AWS provider. Each is a `TestAssumption_*` test, so a tofu or
provider release that breaks one fails CI rather than a rehearsal.

| Claim | Test(s) | HLD section | Relying epic | Verdict |
|---|---|---|---|---|
| (a) A provider override naming every `endpoints` key, STS included, makes `tofu apply` with real-looking keys die at a dead loopback address | `TestAssumption_EndpointsOverrideKeepsApplyOffProd` | Goals (goal 7); Risks and safety | rehearsal-walls | true |
| (b) A `backend "local" {}` override with `-backend-config=path=<outside>` keeps state out of the copy | `TestAssumption_CLIBackendPathKeepsStateOutOfTheCopy` | Prepare | rehearsal-walls | true |
| (c) `tofu init -backend=false` under a credential-free env with an empty HOME and no `TF_CLI_CONFIG_FILE`/`TF_PLUGIN_CACHE_DIR` fetches providers and modules; then the second `init -reconfigure -get=false -lockfile=readonly -plugin-dir=...` makes no proxied connection | `TestAssumption_SecondInitMakesNoProxiedConnection` | Prepare (E3) | rehearsal-prepare-gate | true |
| (d) An `external` key provider runs its command at a backend-initialising init, and `tofu state pull` emits decrypted state | `TestAssumption_ExternalKeyProviderRunsAtBackendInit`, `TestAssumption_StatePullEmitsDecryptedState` | Prepare (E2) | rehearsal-prepare-gate, rehearsal-state-pull | true |
| (e) A data source the mock cannot read makes `plan -refresh-only -detailed-exitcode` exit 1, distinct from 2; `apply -refresh-only` has no exit 2 | `TestAssumption_RefreshOnlyUnreadableDataSourceExitsOne` | Load and check (E1) | rehearsal-against-the-copy | true |
| (f) The provider sends a data source's request before the plan fails, and independent data sources all send in one pass | `TestAssumption_DataSourcesAllSendBeforePlanFails` | Discover | rehearsal-against-the-copy | true |
| (g) A user `zz_override.tf` merges after infrafactory's override, and a root override does not reach a `provider "aws"` inside a child module | `TestAssumption_UserOverrideMergesAfterOurs`, `TestAssumption_RootOverrideMissesChildModuleProvider` | Prepare | rehearsal-prepare-gate | true |
| (h) Tofu's configuration loader reads only its configuration suffixes from a module directory, and no subdirectory but the test directory: broken HCL in `a.hcl`, `a.tf.bak`, `notes.txt`, `x.tfvars.example` and `sub/b.tf` leaves `init -backend=false` and `validate` passing, and broken HCL in `tests/a.tftest.hcl` fails both. Tofu reads `.terraform.lock.hcl` and `.terraform/` by other routes; ADR-0042 § The extension and file policy names the rule that owns each | `TestAssumption_LoaderIgnoresUnknownSuffixes` | Prepare | rehearsal-prepare-gate | true |
| (i) `tofu init -backend=false`, offline with `TF_DATA_DIR` outside the root, writes the module manifest at `<TF_DATA_DIR>/modules/modules.json`: the root entry is Key `""`, Source `""`, Dir `.`; a local entry's Source is as written and its Dir is relative to the root, cleaned. A root with no module blocks gets no manifest. A module fetched from an absolute path gets an absolute Dir under `<TF_DATA_DIR>/modules`, a symlink to the path it was fetched from, and a local module it calls keeps an absolute Dir at that path | `TestAssumption_ModulesManifestShape` | Prepare | rehearsal-prepare-gate | true |
| (j) A root variable takes the value of the last of its sources in this order: its default, `terraform.tfvars`, each `*.auto.tfvars` in lexical order (not the order written), then `-var-file` | `TestAssumption_VariablePrecedenceMatchesGate` | Prepare | rehearsal-prepare-gate | true |
| (k) With an empty HOME and `GIT_CONFIG_NOSYSTEM=1`, `git config --show-scope --list` lists no system entry and no `credential.helper`. A control without the variable shows the system entries where a system config exists (darwin's git sets osxkeychain's helper there); CI's ubuntu has none, so CI exercises only the linux case and the control logs that it was vacuous | `TestAssumption_GitNoSystemDropsCredentialHelper` | Prepare | rehearsal-prepare-gate | true |
| (l) Under `GIT_ALLOW_PROTOCOL=none`, a value naming no transport, `git ls-remote` of an https, an ssh and a file:// URL each fail with git's `transport '<scheme>' not allowed` before any connection: the https and ssh URLs point at a closed loopback port, and the file:// URL at a real repository that a control under `GIT_ALLOW_PROTOCOL=file` lists | `TestAssumption_GitAllowProtocolNoneRefusesEveryTransport` | Prepare | rehearsal-prepare-gate | true |
| (m) `tofu providers lock -platform=<this platform>` fails on a root whose local module call is not installed, until `tofu get` installs it; then, on a lock pinning hashicorp/aws at an earlier version than its constraint admits, it keeps the locked version | `TestAssumption_ProvidersLockKeepsLockedVersion` | Prepare | rehearsal-prepare-gate | true |
| (n) A lock whose only hashicorp/aws entry is registry.terraform.io's initialises under tofu: `providers lock` adds a registry.opentofu.org entry at the same version, and `init -backend=false` installs that version and drops the registry.terraform.io entry | `TestAssumption_RegistryTerraformIoLockInitialises` | Prepare | rehearsal-prepare-gate | true |

Refinements the tests recorded:
- (d) runs at an init that initialises the backend, not at `init -backend=false`.
- (b) holds because the command-line path wins over any path in the block.
- (c) means no *proxied* connection: a direct dial is beyond what the test can observe. It also
  depends on a pinned third-party registry module.

Where they live: the gated claims are in `internal/cli/rehearsal_assumptions_test.go` as package
`cli_test`, because `internal/e2e` imports `internal/cli`. (b) and both halves of (d), which fetch
no provider, are in `internal/cli/assumptions_test.go`. (k) and (l), which need only git, sit with
the gated claims but run ungated, so a machine without git fails them.

Bounds: the provider is `harness.AWSProviderVersion`, not a lock file, except in (m) and (n), whose
lock pins 5.99.0. CI's tofu floats, so a tofu
release can turn main red with no repo change; that is the test doing its job.
