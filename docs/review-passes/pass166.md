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
