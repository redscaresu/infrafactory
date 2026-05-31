# Contributing to InfraFactory

Thanks for considering a contribution! This document is the human contributor's entry point. AI agents working in this repo should also read `AGENTS.md` — that file is the *complete* working agreement for AI-assisted work and is intentionally denser.

## TL;DR

1. **Open an issue first** for anything non-trivial. We'd rather discuss the approach than discard a finished PR.
2. **Pick a focused change.** Mixing a feature + refactor + dependency bump in one PR is the fastest way to get blocked on review.
3. **Add tests** with behavior changes. Coverage is enforced (`make test` runs Go unit + UI unit + Playwright e2e).
4. **Run `make test` locally** before pushing. The pre-commit hook also runs `gitleaks` + `go test`.
5. **Update `BACKLOG.md` and `STATUS.md`** when your change closes or opens a ticket. Keep the BACKLOG honest.

## First-time contributors

If this is your first PR:

- Read `README.md` (architecture + quickstart).
- Skim `BACKLOG.md` — see the "How to read this BACKLOG" note below.
- Look for issues labelled `good first issue` on GitHub. They're scoped to land in one focused PR each.
- Run `make up` to bring up the four-mock stack (mockway, fakegcp, fakeaws, SeaweedFS) plus the UI in one shot, then `infrafactory run scenarios/training/web-app-paris.yaml` to see the run loop end-to-end. `make down` tears it all down.

## How to read this BACKLOG

`BACKLOG.md` is intentionally dense — each row is the *complete* spec for a slice or ticket, including acceptance criteria, dependencies, and historical commit hashes. It's the working document the team and AI agents iterate on. For a first-time contributor:

- **Don't try to read the whole table.** It's ~250 rows of layered history.
- Filter by `status: todo` (column 5) to see what's open.
- The `deps` column tells you what has to land before a given ticket can start — most open rows have one dependency edge inward.
- A row like `S51-T4` is "Slice 51, Ticket 4". Slices group related work; tickets are the unit of one PR.

## Setup

Required:

- Go 1.24+ (matches `go.mod`)
- Node.js 20+ (UI build)
- OpenTofu OR Terraform on `PATH` (Layer 1 + Layer 2 validation)
- `make`

Optional:

- `gh` (GitHub CLI — used by some maintenance scripts)
- `gitleaks` (runs in the pre-commit hook; `make install-hooks` configures it)

```bash
git clone https://github.com/redscaresu/infrafactory.git
cd infrafactory
make install-hooks   # gitleaks + go test pre-commit
make test            # Go unit + UI unit + Playwright e2e
make run             # builds + starts the UI at http://127.0.0.1:4173
```

## Workflow

1. **Pick a focused change.** One slice / one feature / one fix per PR.
2. **Branch off `main`** with a descriptive name: `feat/`, `fix/`, `docs/`, `chore/`.
3. **Add or update tests.** Behavior changes without tests don't merge.
4. **Run the full test suite locally**: `make test`.
5. **Keep errors explicit and actionable.** Wrapped with `fmt.Errorf("%w: %v", sentinel, cause)` where appropriate.
6. **If the change is decision-impacting** (CLI surface, schema, cross-package boundary, dependency-strategy), add or update an ADR under `docs/decisions/`.
7. **If a major architecture/contract shifted**, update `CONCEPT.md`.
8. **Update `STATUS.md`** at the end of a meaningful session.
9. **Update `BACKLOG.md`** ticket status when your PR closes one.
10. **Open a PR** with a clear summary + test plan (see PR template).

## Commit messages

- Imperative subject line, ≤72 chars: `S52-T1: ...`, `fix: ...`, `docs: ...`.
- Reference the ticket id (`S52-T3`, `M41`) when the work maps to one.
- Body explains *why* before *what*. The diff already shows what.
- Sign-off / Co-Authored-By trailers are fine; we don't enforce DCO.

Example:

```
S52-T1: add tagging support to fakeaws S3 buckets

Real S3 tagging is a separate sub-resource (PutBucketTagging) that
terraform-provider-aws wires via aws_s3_bucket_tagging. Adding it to
fakeaws unlocks the new aws-s3-tagged training scenario in S53.

Co-Authored-By: ...
```

## ADR policy

Add or update an ADR (`docs/decisions/NNNN-title.md`) when your change affects:

- The public CLI contract (commands, flags, output shape).
- Cross-package architecture boundaries.
- Schema semantics (`scenario.schema.json`, `infrafactory.yaml`).
- External dependency strategy (tofu/mockway/opa integration model).
- Durable workflow governance.

Prompt-only changes and internal refactors usually don't need an ADR.

## Source-of-truth precedence

When two sources of documentation appear to conflict:

1. `scenario.schema.json` — schema is canonical.
2. `infrafactory.yaml` — runtime config defaults.
3. `CONCEPT.md` — architecture prose.

Working `go test` on `main` is the ultimate tie-breaker; prose loses to a passing test.

## Reporting issues

Use the issue templates under `.github/ISSUE_TEMPLATE/` — bug reports and feature requests both have one. For security issues, see `SECURITY.md` (do NOT open a public issue).

## Code of Conduct

This project follows the [Contributor Covenant v2.1](CODE_OF_CONDUCT.md). Be kind, assume good faith, attack ideas not people.

## AI-assisted work

InfraFactory is itself an AI-collaboration project, so AI-authored contributions are welcome. Two requirements:

1. **AI contributors must follow `AGENTS.md`** — that's the complete working agreement (slice discipline, file ecosystem, ADR triggers, hygiene checks).
2. **Co-Authored-By: Claude / `<other tool>`** trailer in commits authored with AI help. Transparency matters.

## License

By contributing, you agree your work will be released under the Apache-2.0 license (see `LICENSE`).
