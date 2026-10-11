---
status: later
hld: 2026-09-30-import-production-state-into-the-mocks
depends_on: [rehearsal-without-state]
---

# A rehearsal against one estate infrafactory never built either rehearses or refuses by name, and the refusal list orders the work queue (Rollout 5).

**Done when:**
- The user names the estate and shares its `tofu state list | cut -d. -f1 | sort | uniq -c` and `tofu providers` counts (no ids); the lead sizes the queue and writes it as stories in order.
- Each type in the queue is a full bundle, not fakeaws-only: prod reader and reader-policy rows, recorded-read shape, loader widening and read-back, fakeaws model, state-to-read-set mapping, and refusal-shortening tests.
- The user runs the rehearsal in that estate with their own read-only identity; the result is a plan against the copy or a refusal naming every unmodelled type, and that list matches the queue (a mismatch is a bug in goal 6's derivation).
- ELBv2 has one owner: aws-web-stack-load-balancer's fakeaws ELBv2 Done-when, scoped once. Volumes are fakeaws-loader's, referenced, not repeated.
- Each closed queue item re-runs the user's rehearsal and shortens the refusal list; done when the estate rehearses, or the user stops at a named remainder.

**Out of scope:** multi-region (waits on SigV4 scope, goal 11); other clouds; SSE-KMS state.

**Constraints:** status later until the user names the estate; agents never see the estate's credentials, ids or recording; counts shared by the user are dated and scoped; fix forward at the source.

**Your part:** pick the estate and share the counts; create the read-only identity there with `reader-policy.json`, and the backend read identity with `backend-reader-policy.json`, each key in a mode-0600 file; run `rehearse` there and report the refusal list. Lead: turn the list into stories and keep the order.

**Areas:** `docs/stories/`; `docs/epics/`; `docs/hld/2026-09-30-import-production-state-into-the-mocks.md`; `docs/layer3/real-vs-mock-deltas.md`
