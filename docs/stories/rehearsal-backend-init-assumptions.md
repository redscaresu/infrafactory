---
kind: code
status: blocked
blocked_by: [rehearsal-override-assumptions, rehearsal-encryption-assumptions]
epic: rehearsal-assumption-tests
depends_on: [rehearsal-override-assumptions, rehearsal-encryption-assumptions]
touches: [internal/cli/assumptions_test.go, internal/cli/rehearsal_assumptions_test.go, .github/fakeaws-e2e-tests]
risk: high
---

# Claims (b) and (c): the command-line backend path keeps state out of the copy; the first init fetches under a bare env, and the second makes no proxied connection (E3)

**(b) TestAssumption_CLIBackendPathKeepsStateOutOfTheCopy** is ungated and goes in `internal/cli/assumptions_test.go` (package cli; skips without tofu). It is relied on by rehearsal-walls.
- **Config:** main.tf declares `backend "s3" {…}` and a `terraform_data`; an override sets `backend "local" {}`.
- **Steps:** `tofu init -backend-config=path=<outside>/x.tfstate`, then `apply -auto-approve`.
- **Assertions:** the generated id is in the outside file and in no file under the copy, `.terraform` included.
- **Doc comment:** the command-line path wins over any path in the block (verified on 1.12.6). That is the fact the walls rely on; "no path in the block" is incidental.

**(c) TestAssumption_SecondInitMakesNoProxiedConnection** is gated (SkipUnlessEnabled) and goes in `rehearsal_assumptions_test.go`, reusing its recorder and the `max_retries`/deadline rules. It is relied on by rehearsal-prepare-gate.
- **Network:** it does NOT call SealNetwork, because it needs the real registry. The env is explicit: PATH and an empty HOME, with no TF_CLI_CONFIG_FILE, TF_PLUGIN_CACHE_DIR or AWS_*. TF_DATA_DIR is outside the copy.
- **Config:** aws at `harness.AWSProviderVersion`, one local module and one public-registry module pinned to an exact version (named in the test as a third-party dependency).
- **First init:** `tofu init -backend=false` runs with `TF_LOG=debug`. Afterwards the provider is under `<TF_DATA_DIR>/providers`, modules.json names both modules, and a lock file exists. The test logs the debug line giving the registry module's download location (git:: or https), and the PR quotes it.
- **Second init:** with the backend override added, `tofu init -reconfigure -get=false -lockfile=readonly -plugin-dir=<TF_DATA_DIR>/providers -backend-config=path=<outside>` runs with HTTPS_PROXY/HTTP_PROXY at the recorder and NO_PROXY unset. It exits 0 and the recorder counts 0.
- **Control:** the same init without `-plugin-dir`, in a sibling copy, counts 1 or more (verified: it requests registry.opentofu.org/.well-known/terraform.json).
- **Wording:** the claim is "no proxied connection". Direct dials are beyond what the test can observe, and the ADR says so.

Append the (c) name to `.github/fakeaws-e2e-tests`. Doc comments cite ADR-0028 and the HLD's "Prepare" section. Tip commit carries `ADR: none — …`.

**Done when:**
- `go test ./internal/cli -run '^TestAssumption_CLIBackendPathKeepsStateOutOfTheCopy$' -count=1 -v` PASSes with tofu on PATH.
- With INFRAFACTORY_ENABLE_E2E=1 and network, `-run '^TestAssumption_SecondInitMakesNoProxiedConnection$'` PASSes locally, and the PR's aws-e2e log has its `--- PASS: ` line. The PR quotes the module download-location line.
- The PR body records the cp-backup breaks with their FAIL output:
  - (b): `-backend-config=path=<outside>` dropped, so the id lands in `<copy>/terraform.tfstate`
  - (c): `-plugin-dir` dropped from the second init (the recorder counts ≥1, or init fails)
  - (c): the first init given a TF_CLI_CONFIG_FILE whose filesystem_mirror points at an empty dir (the fetch fails)
- `LC_ALL=C sort -c <(grep -v '^#' .github/fakeaws-e2e-tests)` exits 0; neither test file gains an os.Environ() use.
- `make doc-hygiene` passes.
