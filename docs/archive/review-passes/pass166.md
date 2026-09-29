# Review pass 166 — web-live-paris: destroyable, and given time to serve

`codex exec review --base main` on `fix/web-live-paris-destroyable-and-servable`.
Three passes; the last clean. All three findings accepted — none was a nit.

## Pass 1

**[P2] the denial text steered the repair loop back to the shape being removed.**
The policy learned to accept the inline block, but its message still said the
server was "not attached to a private network **via
`scaleway_instance_private_nic`**". That string is repair-loop *input*: it is fed
back to the generator as the reason to fix. The next iteration would have been
told, by the policy itself, to write the undestroyable shape — the fix undone by
its own error message. Accepted; the message now names the inline block first and
the standalone form as the alternative.

**[P3] the probe arithmetic was wrong.** The comment claimed 24 × 5s = 120s. That
holds for a 503, which answers immediately, but a black-holed endpoint pays the
timeout on every attempt as well: 24 × (5s + 5s) ≈ 235s. Accepted; both bounds
are now stated, since the second is the one that matters when a Layer 3 probe
hangs instead of answering.

## Pass 2

**[P2] any reference satisfied `pn_id`.** The new rule checked that
`private_network[].pn_id` had *some* reference, so
`pn_id = scaleway_instance_server.web.id` would have passed Layer 1 and failed at
apply. Accepted, and it deserved to be: a mistyped reference between two resources
whose ids are both UUIDs is the first example on the talk's own slide about what
`plan` cannot see. The rule now requires the reference to name a
`scaleway_vpc_private_network`.

## Pass 3

> The policy change is covered by focused OPA fixtures, the new defaults are
> consistent with the updated configuration, and the full Go test suite passes.
> I did not find a discrete correctness issue introduced by this diff.

## Mutation checks

Every policy fixture was checked against the implementation it claims to test —
not assumed from a green run:

| mutation | result |
|---|---|
| inline-block rule removed | `vpc required passes for an inline private_network block` fails |
| `startswith(... "scaleway_vpc_private_network.")` → any reference | `vpc required fails when pn_id references something other than a private network` fails |

The first attempt at this used `go test -run 'OPA'`, which matches no test in the
package: it printed `ok` having run nothing, and the mutant "survived". The real
test is `TestScalewayPoliciesPlanEvaluation`. Mutate at whole-package scope —
[[feedback-narrow-sample-wrong-conclusion]], for the second time in one day.

## Two audits caught things the review did not

- `TestLayer3CoverageDocTotalsMatchItsTable` — a status token that belonged to no
  bucket, and a "Fifteen scenarios remain gated" sentence that no longer matched
  its own table.
- `TestPitfallsLearnedFromDiffSnippetCap` — both new pitfalls were over the
  1000-byte cap. Trimming them was an improvement: a shorter instruction is more
  likely to be followed.

---

# Passes 167–171 — the gate (same branch, second round)

Adding the Layer 3 refusal drew five more findings across five passes. None was
a nit; three were bugs the change itself introduced.

**[P1] the replacement was uncontained.** Refusing the standalone NIC moved the
attachment into a *nested* block, and `layer3ContainmentProblems` reads
**top-level** attributes. So `private_network { pn_id = "<someone-else's-id>" }`
would have passed — a server in the run's project wired to a network the run
does not own and no teardown touches. Moving the shape without moving the guard
would have traded an undestroyable stack for an uncontained one. Accepted.

**[P2] the denial offered the shape the gate now refuses.** `vpc_required`'s
message still named `scaleway_instance_private_nic` as an alternative, and that
string is repair-loop input — the next iteration would have been sent straight
into the new refusal. Accepted.

**[P2] the check ran on resources it did not understand.** `private_network` is
a block on several types and they disagree on the key: `scaleway_lb` uses
`private_network_id`, `scaleway_redis_cluster` uses `id`, only the server and
`rdb_instance` use `pn_id` (checked against the 2.81.0 schema, not assumed). The
unconditional call would have demanded `pn_id` from a valid load balancer.
Accepted, scoped to `scaleway_instance_server`, and the *other* nested
attachments are recorded as a pre-existing containment gap rather than left to
look covered.

**[P2] count-indexed references.** `x.main[count.index].id` parses as a
`RelativeTraversalExpr`, not a `ScopeTraversalExpr` — confirmed by parsing all
three forms rather than reasoning about them. Accepted, with a correction to the
finding's premise: counted stacks cannot reach a real apply *anyway*, because
S168's index rule fires on any `resource[index].attr` on any resource type.
Verified by pointing an unrelated resource at a counted reference and getting the
same refusal. The test now says which rule refuses what, instead of implying this
check covers it.

**[P1] `dynamic` blocks.** `dynamic "private_network" { content { pn_id = ... } }`
parses as type `dynamic`, so the containment check never saw it. This was not a
hole in the new rule but in **every** nested-block rule in the file — the same
trick hides a `provisioner` or a `root_volume`. Refused wholesale rather than
taught to each rule one at a time. Free: zero stacks in the entire run corpus use
one.

## One finding declined, with the reason recorded

**[P2] counted `pn_id` cannot be distinguished from `.name` in rego.** True, and
irreducible at that layer: tofu records identical references for
`main[count.index].id` and `main[count.index].name` — the bare resource plus
`count.index` — so the attribute is not in the plan JSON to test. Demanding a
`.id` suffix would deny every counted scenario at Layer 1 *including mock runs*,
a real cost against a hypothetical mistake. And it is not the last line of
defence: `layer3IsPrivateNetworkIDRef` reads the HCL, where the attribute is
present, and refuses before any real apply. The imprecision lives only where it
is cheap; the gate that guards money is exact. Documented in the rego rather than
left to be rediscovered.

## Mutation checks

| mutation | result |
|---|---|
| gate not wired into the resource loop | `TestLayer3ShapeRefusesStandalonePrivateNIC` fails |
| containment call removed | `…RefusesInlinePrivateNetworkWithLiteralID` fails |
| `RelativeTraversalExpr` case removed | reference-forms test fails |

## The correction worth keeping

The gate was first justified by "the next generation emitted the standalone NIC
anyway, so the pitfall was ignored". That was **wrong**. The repository had been
switched to a branch without the new pitfalls **33 seconds** before the run
started, and `paths.pitfalls` is read from the working tree, so the generator was
still being told to write one. The pitfall is untested, not ineffective.

The gate stands on grounds that survive the correction — a gate that never fires
costs nothing, advice cannot be relied on, and `deploy` never reads a pitfall at
all — and ADR-0029 records the false start rather than quietly taking credit for
it. A run does not record the commit it was generated under; that gap is now
named and open.
