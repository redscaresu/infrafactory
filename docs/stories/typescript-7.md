---
kind: chore
status: ready
touches: [ui/package.json, ui/package-lock.json]
---

# TypeScript 7 (dependabot #237)

When 5→7 was tried on 2026-08-31, `@sveltejs/kit` declared `peerOptional typescript` as
`^5.3.3 || ^6.0.0` and `npm install` failed with ERESOLVE. Dependabot #237 (6.0.3 → 7.0.2) was
closed. Check the current peer range of `@sveltejs/kit` and the other ui dependencies first
(`npm view @sveltejs/kit peerDependencies`). If a released kit admits TypeScript 7, bump kit as
needed and `typescript` to ^7 in ui/package.json. Never use `--force` or `--legacy-peer-deps`.

**Done when:**
- `npm install` in ui/ succeeds with no ERESOLVE and no override flags, and ui/package.json holds typescript ^7.
- `npm run check`, `npm run build` and `npm test` pass in ui/ (and the Playwright suite in CI).
- Or, if no released `@sveltejs/kit` admits TypeScript 7: reply `IMPOSSIBLE:` quoting the current peer range, and open no PR.
