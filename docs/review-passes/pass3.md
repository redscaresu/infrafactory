# Codex review pass 3 — dependency backlog (PRs #129, #132, #133, #134, #135)

Five dependabot PRs, each reviewed against `main`. Reviewed in parallel via
`git worktree`, since `codex exec review` operates on the current repository.

## Results

| PR | Bump | Pass 1 | Pass 2 | Outcome |
|---|---|---|---|---|
| #134 | go-deps (3 modules) | clean | clean | merged |
| #132 | @sveltejs/kit 2.70.1 → 2.70.3 | clean | clean | merged |
| #133 | vite 8.1.5 → 8.2.1 | clean | clean | merged |
| #135 | svelte 5.56.8 → 5.56.9 | clean | clean | merged |
| #129 | actions group (4 majors) | clean | 1 finding — **declined** | merged |

## #129 — declined, with evidence

**Finding**: bumping to `checkout@v7`, `setup-go@v7`, `setup-node@v7`,
`cache@v6` risks CI never starting, "if this runs before the corresponding
v7/v6 tags exist for the official GitHub actions."

**Declined — refuted by the PR's own CI.** The conditional is the whole
claim, and it is false here. The green run on this PR resolved and
downloaded every one of those tags:

```
actions/cache@v6      SHA 55cc8345863c7cc4c66a329aec7e433d2d1c52a9
actions/checkout@v7   SHA 3d3c42e5aac5ba805825da76410c181273ba90b1
actions/setup-go@v7   SHA b7ad1dad31e06c5925ef5d2fc7ad053ef454303e
actions/setup-node@v7 SHA 820762786026740c76f36085b0efc47a31fe5020
```

For `pull_request` events the workflow definition comes from the PR branch,
so that run *is* the test of whether these refs resolve. They do, on real
SHAs, and the job reached and passed the test suite. A speculative
"if the tag doesn't exist" cannot outrank a run that already used the tag.

Recorded rather than implemented, per the rule that pushing back is a real
option.

## #130 — not merged

Genuinely broken, and the only red PR in the backlog. Dependabot grouped
**typescript 5.9.3 → 7.0.2** with four harmless patch bumps;
`@sveltejs/kit@2.70.1` peers on `typescript: ^5.3.3 || ^6.0.0`, so `npm ci`
failed with `ERESOLVE` before any test ran:

```
npm error While resolving: @sveltejs/kit@2.70.1
npm error Found: typescript@7.0.2
npm error Conflicting peer dependency: typescript@6.0.3
```

Not a code defect — a grouping defect. `typescript` is now in the
`ui-deps` `exclude-patterns` and the `ui-majors` group, so its majors get
their own review PR instead of poisoning four safe bumps. #130 is closed;
dependabot will regenerate the group without typescript.

## Note on the loop's value here

Five of six PRs produced nothing, which is the expected shape for lockfile
bumps and is worth recording as evidence the loop ran. The one finding it
did produce was wrong, and disproving it took a CI log lookup — cheap. The
actual defect in this backlog (#130) was found by reading CI, not by codex.
