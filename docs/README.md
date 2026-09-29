# docs/

What each live doc is for. `archive/` is frozen history: never edited, and excluded from
searches unless you are looking for history (`grep -r --exclude-dir=archive`).

## Planning (the board)
- [`hld/`](hld/README.md) — agreed designs; each lists its epics.
- `epics/` — goals bigger than one PR.
- `stories/` — one open item per file; `Board.base` is the Obsidian board over them.
- [`decisions/`](decisions/README.md) — ADRs.
- `review-passes/<slug>.md` — review reasoning too long for a PR body.

## How it works
- [`architecture.md`](architecture.md) — components and data flow.
- [`auto-learning-loop.md`](auto-learning-loop.md) — classifier, extractors, ratchets, sweep protocol.
- [`mockway-contract.md`](mockway-contract.md) — what infrafactory expects from mockway.
- [`ci-security-posture.md`](ci-security-posture.md) — CI and supply-chain posture.

## Running it
- [`operations.md`](operations.md) — runbooks: the swarm (infrafactory's rules for swarm-dev), sibling mocks, Layer 3.
- [`demo-runbook.md`](demo-runbook.md) — driving a live demo from the UI; assets in `demo/`.
- `process/` — ticket template and reusable execution prompt.

## Layer 3 (real cloud)
- [`layer3/coverage.md`](layer3/coverage.md) — which scenarios can run against the real API.
- [`layer3/evidence.md`](layer3/evidence.md) — what real-cloud runs caught, and their cost.
- [`layer3/real-vs-mock-deltas.md`](layer3/real-vs-mock-deltas.md) — real Scaleway vs mockway.
- `layer3/aws/` — the scope's IAM policy and SCP.

## Generated
- `mock-gaps.md` — gitignored; written by runs (`internal/generator/pitfalls_learn.go`).
