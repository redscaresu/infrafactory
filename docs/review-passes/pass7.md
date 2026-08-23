# Codex review pass 7 — the "Presentable" arc plan (S144–S150)

11 passes, 6 findings, all accepted. The most productive loop of the
project, on a document containing no executable code — because a plan that
misleads is executed later, by someone with less context.

| Pass | Findings | Outcome |
|---|---|---|
| 1 | 2 (P2) | accepted |
| 2, 3 | 1 (P1, same issue twice) | accepted |
| 4 | none | — |
| 5 | 1 (P3) | accepted |
| 6, 7 | 1 (P2, same issue twice) | accepted |
| 8 | none | — |
| 9 | 1 (P2) | accepted |
| 10, 11 | none | **converged** |

## The two that mattered

**Pass 2/3 — "a label is not a security boundary" (P1).** The fork-PR
ticket offered `pull_request_target` semantics *or* a maintainer-applied
label as alternatives. Neither is a boundary. `pull_request_target` grants
base-repo secrets to a job that then executes PR content — that is the
exfiltration path, not a defence. And a label is a moment in time: the PR
can be updated afterwards and the workflow runs the new code under the old
approval.

Now three requirements, each necessary and none sufficient: same-repo PRs
only for the real-cloud path, secrets behind an Environment with required
reviewers so approval does not carry to new commits, and run-time
re-verification of the approved head SHA. *Label for intent, gate on
identity and SHA.*

This mattered because the repo is public and the workflow will hold real
cloud credentials.

**Pass 9 — a contradiction between two of my own tickets (P2).** S144-T3
required the gate to fail closed on absent credentials. S150-T2 told
implementers to reuse goldfinger's pattern, which skips *green* when its
secret is missing. Copied into the Layer 3 gate, a misconfigured same-repo
run would report success without applying anything.

A false green on the one check whose purpose is proving a change survived
contact with the real cloud — which is the precise failure this project was
built to eliminate. S139 exists because an inherited `SCW_API_URL` let a
"real" apply pass against a mock. Fork skips green; same-repo fails closed;
never one guard.

## The rest

- **Pass 1**: `AGENTS.md` still told fresh agents the API refuses Instances
  access to `openclaw-prod` — false as of an hour earlier. A stale *safety*
  claim is worse than a stale feature claim, because someone relies on it.
  Also: `STATUS.md` / `NEXT_SESSION.md` not repointed, so a fresh session
  would not have found the new arc.
- **Pass 5**: plan title said S144–S149 after S150 was added.
- **Pass 6/7**: `docs/layer3-coverage.md` still described the pre-Instances
  credential. It is the source of truth S146 implementers are pointed at,
  so a stale copy would have sent them to decide from the old model.

## Note

Four of the six findings were **staleness introduced by the same change** —
grant a permission, and three documents that describe it become wrong at
once. The loop caught every one. Worth remembering that a change to a
*fact* about the system has a wider blast radius in the docs than a change
to code.
