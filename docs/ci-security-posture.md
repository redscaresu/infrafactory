# CI and supply-chain posture

Written 2026-08-23 after comparing against `redscaresu/goldfinger`, which
prompted S150. Recorded so "did we copy goldfinger's security setup?" has an
answer, including the parts infrafactory already had.

The honest summary: **infrafactory was already ahead on most measures.** The
comparison produced one genuine gap, not a wholesale import.

## Where the two repos stood

| Measure | goldfinger | infrafactory (before S150) |
|---|---|---|
| gitleaks in CI | yes | yes |
| `.gitleaks.toml` allowlist | yes | yes |
| `go test -race -count=2` | yes | yes |
| `-trimpath` builds | yes | yes |
| `SECURITY.md` | yes | yes |
| dependabot | yes | yes |
| pre-commit secret hook | no | yes |
| **`govulncheck`** | **yes** | **no ← the gap** |
| `permissions:` on every workflow | no (1 of 2) | no (2 of 4) |
| SHA-pinned actions | no | no |

## What S150 changed

**`govulncheck` in CI.** The one thing goldfinger had that we did not, so a
vulnerable Go dependency could ship silently.

Run with **`-tags noui`**, which is a correctness requirement rather than a
convenience. `cmd/infrafactory/embed.go` carries `//go:embed all:ui/build`,
and that directory is generated and gitignored — so on a clean runner
`go build ./...` cannot load that package. govulncheck does not fail on that:
it exits 0 and says nothing, having simply skipped what it could not load. A
green scan covering fewer packages than it appears to is precisely the false
green this repo keeps encountering. The tag swaps the embed for a stub so
every package loads.

Symbol-level (the default) rather than `-scan module`, deliberately. The tree
requires `golang.org/x/crypto@v0.53.0`, which carries **GO-2026-5932** with
`Fixed in: N/A` — no fix exists. A module-level gate would be permanently red
with no remedy available, and a check nobody can act on is a check everybody
learns to ignore. Symbol-level fails only when our code actually reaches the
vulnerable path, which is the question worth blocking a merge on.

### What it found on its first run

The gate paid for itself immediately, and not in the way expected. `go.mod`
pinned `go 1.25.0`, CI installs exactly that (`GOTOOLCHAIN: local`), and the
**Go 1.25.0 standard library** carries roughly twenty known advisories with
fixes spread from 1.25.2 to 1.25.13. Several were genuinely reachable, not
theoretical — for example `GO-2026-6218` in `net/url`, reached from
`internal/api/server.go` via `http.Client.Do`.

So every binary this repo built was linked against a stdlib with known,
reachable vulnerabilities. `go.mod` now pins `go 1.25.13`, the highest patch
any of the findings required.

**This was invisible locally**, which is the part worth remembering. A local
`govulncheck` reported clean because the toolchain auto-switched to go1.26.7
(`golang.org/x/vuln` requires ≥1.25.0) and scanned *that* stdlib. CI pins the
toolchain and scanned go1.25.0. Local and CI were answering different
questions, and only the CI answer was about what ships. When verifying a
toolchain-sensitive check, pin the toolchain: `GOTOOLCHAIN=go1.25.13
govulncheck ...`.

**Explicit `permissions:` on every workflow.** `ci.yml` and `doc-hygiene.yml`
had none, so `GITHUB_TOKEN` took the default scope — wider than either needs.
Both are now `contents: read`. This matters more than it did last week: S144
adds a workflow holding real cloud credentials, and it must declare its own
narrow set rather than inherit a permissive default.

**SHA-pinned actions, scoped to workflows with elevated capability.** A tag is
mutable and can be repointed at new code by a compromised upstream.
`scenario-gate.yml` (sees `OPENROUTER_API_KEY`) and `release.yml` (holds a
`contents: write` token) are pinned; the trailing `# vN` comment is what lets
dependabot keep them current.

`ci.yml` and `doc-hygiene.yml` stay on tags on purpose. They are read-only and
hold no secrets, so pinning there would be churn without a threat to answer.
Pin what an attacker would want to reach.

## What was taken from goldfinger and what was not

**Taken**: `govulncheck`, and the *shape* of its secret-guarded job — a job
performing real external mutations, guarded on the secret's presence.

**Explicitly not taken**: goldfinger's "absent secret ⇒ skip green" semantics,
for the Layer 3 gate. goldfinger's e2e opens a PR on a sandbox repo; a green
skip there claims nothing. infrafactory's Layer 3 gate asserts *"this change
was applied to a real cloud and cleaned up"*, so a same-repo run that skips
green would be a false green — the exact failure S139 exists to prevent. Fork
PRs skip green because they legitimately are not running the gate; same-repo
runs fail closed. See S150-T2a in the Presentable arc plan.

## Known gaps

- **No SBOM, no CodeQL, no OpenSSF Scorecard.** Not obviously worth it for a
  repo of this size; revisit if the project gains external contributors.
- **`ci.yml` and `doc-hygiene.yml` use mutable action tags**, by the reasoning
  above. If either ever gains access to a secret, pin it in the same change.
- **`npm audit` reports 3 low-severity advisories** in the UI dev tree. Dev
  dependencies only, nothing shipped, no fix without a breaking upgrade.
