---
kind: code
status: blocked
blocked_by: [rehearsal-prepare-gate-core]
epic: rehearsal-prepare-gate
depends_on: [rehearsal-prepare-gate-core]
touches: [internal/cli/rehearse_gate_modules.go, internal/cli/rehearse_gate_modules_test.go, internal/cli/rehearsal_assumptions_test.go, docs/decisions/0028-factual-claims-are-tests-not-comments.md, .github/fakeaws-e2e-tests]
risk: high
---

# Module sources judged by what the download does: only local modules, and the modules.json manifest gated

Decision 2, the stricter option (survey option b): a registry module is downloaded by git from github.com, so every non-local source is refused, registry modules included. There is no `--allow-module`.

**Assumption test first (ADR-0028).** Before the parser, add `TestAssumption_ModulesManifestShape` to rehearsal_assumptions_test.go, gated by e2e.SkipUnlessEnabled. It runs real `tofu init -backend=false` offline (no providers) with TF_DATA_DIR outside the root on (a) a root calling `./m`, where m calls `../shared`, and (b) a root with no module blocks. It asserts the observed manifest: where it lives, the root entry's Key and Dir, each local entry's Key, Source and Dir form (relative to what), and whether (b) writes a manifest at all. The parser and its fixtures follow what this test pins, not a guess. Add its ADR-0028 row and its name to `.github/fakeaws-e2e-tests` (sorted). Hot-file chain: after rehearsal-prepare-gate-core, before rehearsal-prepare-gate-aws-provider.

**Gate 1.** `source` must be a literal string starting `./` or `../` that resolves (Abs plus EvalSymlinks) inside the boundary. Everything else is refused naming the source: a registry address (with or without host, with or without version), git::, git@, ssh://, github.com/, bitbucket.org/, http(s)://, s3::, gcs::, hg::, any other host, a non-literal source, and any `version` argument (local modules take none). Export one helper returning a dir's local module sources (`rehearsalLocalModuleSources`), which rehearsal-prepare-copy calls to find what to copy.

**Gate 2.** A manifest function reads `<data>/modules/modules.json` and returns the dirs to gate. Admitted: the root entry, and local entries whose Dir, resolved by the pinned rule and then EvalSymlinked, lies inside the copy. Refused: any entry whose Dir resolves under `<data>/modules` or anywhere outside the copy, any entry whose Source is not local, an unparseable manifest, and a missing manifest. If the assumption test shows tofu writes no manifest for a module-free root, a missing manifest is admitted only when gate 1 counted zero module blocks; otherwise it is refused. The full gate then runs over every returned dir. rehearsal-prepare-second-init wires this in; this story changes no prepare flow.

Tip: `ADR: none — ADR-0042 records the module-source decision` (the ADR-0028 edit also satisfies doc-hygiene).

**Done when:**
- Gate 1: `cloudposse/label/null` with `version = "0.25.0"`, `registry.opentofu.org/cloudposse/label/null`, `git::https://github.com/cloudposse/terraform-null-label.git?ref=0.25.0`, `git@github.com:o/r.git`, `app.terraform.io/o/m/aws`, `../../outside-the-repo` and a source set from a variable are each refused naming the source. `./m` inside the boundary is admitted; `./m` with a `version` is refused.
- Gate 2 tests use manifests in the shape the assumption test pinned: an entry with Dir under `<data>/modules` is refused naming its key; a local entry whose Dir escapes the copy is refused; a local entry whose Dir root is a symlink pointing out of the copy is refused; a local entry's dir holding `provisioner "local-exec"` is refused naming the key and file; an unparseable modules.json is refused; a missing one is refused when module blocks exist.
- `grep -x TestAssumption_ModulesManifestShape .github/fakeaws-e2e-tests` hits; ADR-0028's table names it.
- Each new test, the assumption test included, is shown failing by a cp-backup break recorded in the PR.
