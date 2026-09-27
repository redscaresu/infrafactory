---
kind: chore
status: later
---

# TypeScript 7 (dependabot #237)

When 5→7 was tried on 2026-08-31, `@sveltejs/kit` declared `peerOptional typescript` as
`^5.3.3 || ^6.0.0` and `npm install` failed with ERESOLVE. Check the current range first.
