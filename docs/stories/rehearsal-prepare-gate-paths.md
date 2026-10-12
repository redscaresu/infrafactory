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

These path arguments must resolve inside the boundary: local_file/local_sensitive_file `filename` and `source`, data local_file/local_sensitive_file `filename`, archive_file (resource and data) `output_path`/`source_file`/`source_dir`, and the first argument of file, fileexists, fileset, filebase64, filemd5, filesha1, filesha256, filesha512, filebase64sha256, filebase64sha512 and templatefile. These names are admitted by exact match only, so a `::`-qualified spelling stays refused by gate-core's rule. A path is admitted only if it is path.module or path.root joined with a literal, and it resolves (lexically, from that directory) inside the boundary. A bare relative literal is refused naming the address: which directory tofu resolves it from is not pinned, and refusing it needs no claim (ADR-0042). Absolute, escaping, variable or otherwise computed paths are refused naming the address. abspath and pathexpand stay refused by name. A written path (local_file/local_sensitive_file `filename`, archive_file `output_path`) is also refused, naming the address, when its base name has a `.tf` or `.tfvars` suffix or any suffix gate-core refuses by name (reuse gate-core's list, not a copy), when it is `.terraform.lock.hcl`, or when any element is `terraform.d` or `.terraform`, compared case-insensitively: an apply could otherwise write config every later step loads ungated (ADR-0042). fileset's second argument (the glob, joined onto the path before globbing) must be a literal relative pattern with no `..` element: a non-literal, absolute or `..`-holding pattern is refused naming the address, any `..` and not only an escaping one, because `**` can match zero directories (ADR-0042).

**Templates.** The template a templatefile names is gated too, since other files are admitted unparsed and a template can call any function: read it from its checked path, parse it with hclsyntax.ParseTemplate, and hold every function call and path in it to the same function and path rules. A missing or unparseable template is refused naming the templatefile's address.

Tip: `ADR: none — ADR-0042 records the path rule`.

**Done when:**
- Refused, each naming its address: `file("/etc/passwd")`, `file("../../x")` escaping the boundary, `templatefile(var.p, {})`, `pathexpand("~/.aws/credentials")`, `core::templatefile("${path.module}/t.tpl", {})`, local_file `filename = "/tmp/x"` and archive_file `source_dir = "../.."`.
- Refused: a bare relative literal, `file("files/a.txt")`, naming its address; local_file `source = "/etc/passwd"`.
- `fileset(path.module, "../../../../home/*/.aws/*")`, `fileset(path.module, "**/../x")` and `fileset(path.module, var.p)` are each refused naming the address; `fileset(path.module, "files/*.txt")` is admitted.
- `local_file { filename = "${path.root}/zz_override.tf" }`, `filename = "${path.module}/x.TOFU"`, `filename = "${path.root}/.terraform.lock.hcl"` and archive_file `output_path = "${path.root}/terraform.d/x.zip"` are each refused naming the address; `filename = "${path.module}/out.txt"` is admitted.
- A template calling file on an absolute path is refused; a template that does not parse is refused.
- Admitted: `file("${path.module}/files/a.txt")`, `templatefile("${path.root}/t.tpl", {})` and local_file `filename = "${path.module}/out.txt"`.
- Each new test is shown failing by a cp-backup break recorded in the PR.
