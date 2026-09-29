# Codex review pass 4 — lb-paris probe canary + Layer 3 cleanup gating

Base: `main`. Scope: the `connectivity` criterion on `lb-paris`, the
failed-apply cleanup fix in `test_command.go`, and the LB connectivity
derivation.

Seven passes, five findings, **all accepted**. The most productive loop so
far, and the only one where the reviewer repeatedly caught the *fix* rather
than the original code.

| Pass | Findings | Outcome |
|---|---|---|
| 1 | 3 (P2 ×3) | all accepted |
| 2 | none | — |
| 3 | 1 (P2) | accepted |
| 4 | 1 (P1) | accepted |
| 5 | 1 (P2) | accepted |
| 6 | none | — |
| 7 | none | **converged** |

## Pass 1

**a. The new criterion would have broken every mock-only run.** The one
that mattered. With the committed default (`sandbox_deploy.enabled: false`)
a `connectivity` criterion is evaluated by the topology deriver, not the
real probe. The deriver emitted no `public_internet->load_balancer` key,
and `EvaluateTopology` reads a missing key as `false`, so `lb-paris` would
have failed for every ordinary user. Verified by reproducing it exactly:
`connectivity "public_internet->load_balancer:80" expected true got false`.

Fixed in the deriver rather than by weakening the scenario, gated on the LB
having an IP but **not** a backend — which is what makes the mock agree
with real Scaleway, where a frontend completes the TCP handshake and then
answers 503.

**b. Cleanup fired for pre-apply failures.** The gate was "the sandbox was
attempted", which includes init and plan failures. Those create nothing, so
cleanup would destroy nothing and the sweep would then report itself
unverifiable — sending the operator after a leak that cannot exist.

**c. The new tests passed for the wrong reason.** They never remapped
`paths.output`, so they ran against the package-relative `./output` and
only saw live state because another test had left some there. Now they
plant their own fixture in a temp workspace.

## Passes 3, 4, 5 — three rounds on the same predicate

Worth recording as a sequence, because each fix introduced the next
problem and the loop caught all three.

- **Pass 3**: gating on the state *file* was wrong — a successful destroy
  leaves it in place but emptied, so "exists and non-empty" read the same
  for "resources may exist" and "already cleaned up".
- **Pass 4 (P1)**: keying off the run project id over-corrected into
  **fail-open** — unparseable state silently skipped cleanup, and it broke
  `TestRunCommandAutoDestroysRealResourcesOnFailure` outright. ADR-0023
  requires the safe branch when the answer cannot be determined.
- **Pass 5**: the rewrite still returned `false` for *every* `os.ReadFile`
  error, so a permissions failure on an existing state file skipped
  cleanup — fail-open again, contradicting the contract documented three
  lines above it.

Final shape: absence is the only read error meaning "nothing applied";
unreadable, unparseable, or resources-present all get cleanup.

## Declined

None. Every finding was real, and two were regressions I had introduced.

## Note

The reviewer was most valuable on the *second-order* problems — not the
original leak, which the live canary found, but the three successive
mis-fixes for it. A fail-closed contract is easy to state and easy to
violate one refactor later.
