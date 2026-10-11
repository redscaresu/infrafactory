---
kind: code
status: ready
epic: rehearsal-assumption-tests
depends_on: []
touches: [internal/cli/assumptions_test.go]
---

# Claim (d): an external key provider runs a command at the backend-initialising init (not at `init -backend=false`), and state pull emits decrypted state (E2)

Ungated and offline: no provider is fetched, only the built-in `terraform_data`. Append to `internal/cli/assumptions_test.go` (package cli, which ADR-0028's file rule allows).
- **Skip conditions:** skip when tofu is not on PATH (`if _, err := exec.LookPath("tofu"); err != nil { t.Skip(...) }`), or when `tofu version` is below 1.10, saying so.
- **Environment:** the child env is explicit (PATH, HOME=t.TempDir()), never os.Environ().
- **The key provider script:** the test writes it into t.TempDir(). It touches a marker file, prints the header `{"magic":"OpenTofu-External-Key-Provider","version":1}`, reads stdin, then prints `{"keys":{"encryption_key":<b64 32 bytes>,"decryption_key":<same>}}` with no extra fields. Verified on 1.12.6.
- **Config:** `terraform { encryption { key_provider "external" …; method "aes_gcm" …; state { method = … } } }`.

**TestAssumption_ExternalKeyProviderRunsAtBackendInit**, relied on by rehearsal-prepare-gate. It runs the rehearsal's own sequence in one copy:
1. `tofu init -backend=false` exits 0, and no marker exists.
2. With an override `backend "local" {}`, `tofu init -reconfigure -get=false -lockfile=readonly -backend-config=path=<outside>` exits 0, and the marker exists before any plan.

**TestAssumption_StatePullEmitsDecryptedState**, relied on by rehearsal-state-pull. After `apply -auto-approve`, the resource's generated id is absent from the on-disk state file and present in `tofu state pull` stdout.

Doc comments cite ADR-0028 and the HLD's "Prepare" section. Failure messages carry `tofu version`. Tip commit carries `ADR: none — …`.

**Done when:**
- `go test ./internal/cli -run '^TestAssumption_(ExternalKeyProviderRunsAtBackendInit|StatePullEmitsDecryptedState)$' -count=1 -v` prints two `--- PASS` lines (not SKIP) with tofu ≥1.10 on PATH, and the CI `test` job is green.
- The PR body records the cp-backup breaks with their FAIL output:
  - the encryption block removed: no marker after step 2
  - the script's JSON given an unknown field: init exits 1 and the test fails, even though the marker exists
  - `state { method }` removed: the on-disk state is plaintext and the pull test fails
- `grep -n 'os.Environ' internal/cli/assumptions_test.go` shows no new use.
- `make doc-hygiene` passes.
