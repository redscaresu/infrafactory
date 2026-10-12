---
kind: code
status: blocked
blocked_by: [rehearsal-prepare-gate-core]
epic: rehearsal-prepare-gate
depends_on: [rehearsal-prepare-gate-core]
touches: [internal/cli/rehearse_gate_paths.go, internal/cli/rehearse_gate_paths_test.go]
risk: high
---

# File-family functions and local_file/archive_file paths admitted only within the copy

Widen rehearse_gate_paths.go, which gate-core opened closed. The boundary is the copy root (`<id>/work`); every admitted module is local and copied, so there is no other boundary.

These path arguments must resolve inside the boundary: local_file/local_sensitive_file `filename`, data local_file/local_sensitive_file `filename`, archive_file `output_path`/`source_file`/`source_dir`, and the first argument of file, fileexists, fileset, filebase64, filemd5, filesha1, filesha256, filesha512, filebase64sha256, filebase64sha512 and templatefile. These names are admitted by exact match only, so a `::`-qualified spelling stays refused by gate-core's rule. A path is admitted only if it is a literal relative path, or path.module/path.root joined with a literal, and it resolves (lexically, from the module dir) inside the boundary. Absolute, escaping, variable or otherwise computed paths are refused naming the address. abspath and pathexpand stay refused by name.

Tip: `ADR: none — ADR-0042 records the path rule`.

**Done when:**
- Refused, each naming its address: `file("/etc/passwd")`, `file("../../x")` escaping the boundary, `templatefile(var.p, {})`, `pathexpand("~/.aws/credentials")`, `core::templatefile("${path.module}/t.tpl", {})`, local_file `filename = "/tmp/x"` and archive_file `source_dir = "../.."`.
- Admitted: `file("${path.module}/files/a.txt")`, `templatefile("${path.root}/t.tpl", {})` and local_file `filename = "${path.module}/out.txt"`.
- Each new test is shown failing by a cp-backup break recorded in the PR.
