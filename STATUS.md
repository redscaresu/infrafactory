# STATUS

Current state only, and the one place to start. Kept under 150 lines, and CI enforces it:
each PR adds one line to **Recent** and drops the oldest, and an item leaves **Open** when its
PR merges. History: `docs/status/ARCHIVE.md` (per-arc close-outs),
`docs/status/STATUS_HISTORY.md` (this file before S197), and `git log`.

## Now

No arc in flight. The process-efficiency arc (ADR-0035, S195–S198) is complete. Pick from
§ Open. Multi-slice arcs get a plan in `docs/plans/<arc>-plan.md`.

## Open

- **Does the generator now write a locked-down security group?** ADR-0034 lifted the pitfall
  that forbade one, but no visible criterion asks for it. A seeded-HCL `test` run can prove the
  allowlist, `default_deny_ingress` and the holdout today, with no LLM. A generated
  `run --holdout` answers the real question and needs the next item fixed.
- **Blocked on a decision: nested `claude` hangs in `self_review` to the 5-minute timeout.**
  The two earlier phases complete. Its stderr warns about three allow rules in the local
  `.claude/settings.local.json` that match nothing. That is suspected, not proven, to be the
  cause. Correcting them would grant access they do not grant today.
- **Tie `vpc_required.rego`'s claim about the Layer 3 gate to the gate.** Its denial text says
  the gate refuses a standalone `scaleway_instance_private_nic`; nothing checks that against
  `layer3_hcl_shape.go`, and the same claim has gone stale in three places. Use a lockstep test
  in the shape of `TestCloudPrefixLockstep`.
- **M100: mockway's gated examples 501 on the v2alpha1 NIC route.** Probably fixed by the
  v2alpha1 work in S188–S189. Re-run with `MOCKWAY_ENABLE_E2E=1` to confirm, or close it.
- Later:
  - a rule requiring a firewall on the server, only if the verification above shows none is
    declared (it cannot come from the holdout, ADR-0033 §5);
  - TypeScript 7 (#237), after checking `@sveltejs/kit`'s peer range, which excluded 7 on
    2026-08-31;
  - move `docs/demo/` recordings to release assets;
  - fakegenesys public visibility and branch protection (operator click-ops);
  - pitfall-pruning automation, shelved (`docs/plans/pitfall-pruning-automation-plan.md`).

## Recent

- 2026-09-27 S202 — `destruction: no_orphans` is refused as a criterion; teardown is the destruction layer (ADR-0036)
- 2026-09-27 S200 — the pre-rotation `scw-layer3.env` is deleted; docs no longer mention it
- 2026-09-27 S199 — `AGENTS.md` holds only always-on rules; task detail moved to `docs/operations.md`, three stale Layer 3 claims corrected (#261)
- 2026-09-27 S198 — each change is written down once; review-pass files only for declined findings (#260)
- 2026-09-27 S197 — one entry point: this file, current state only; `NEXT_SESSION.md` and `BACKLOG.md` removed (#259)
- 2026-09-27 S196 — pre-commit is fast, and a decision path no longer forces an ADR edit (#258)
- 2026-09-27 S195 — the ADR index is generated (#257)
- 2026-09-27 S194 — the pitfall that forbade a firewall (#256)
- 2026-09-21 S193 — the state-side policy check actually runs (#253)
- 2026-09-20 S192 — the holdout, rebuilt as a negative check against the running stack (#252)
