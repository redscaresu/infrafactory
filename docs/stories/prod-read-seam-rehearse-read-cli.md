---
kind: code
status: blocked
blocked_by: [prod-read-seam-ec2-readers, rehearsal-prepare-command-group]
epic: prod-read-seam
depends_on: [prod-read-seam-ec2-readers, rehearsal-prepare-command-group]
touches: [internal/cli/rehearse_read.go, internal/cli/rehearse_read_test.go, internal/cli/rehearse.go, internal/cli/contract.go, internal/cli/testdata/golden/, docs/operations.md]
risk: high
---

# `infrafactory rehearse read`: one read step, the key held in this process only, output only under .infrafactory/

Merge main in before building: the `rehearse` group (rehearsal-prepare-command-group, ADR-0042) is there. This story's rehearse.go edit is one AddCommand entry registering `read` (the group's first AddCommand call if `prepare` has not landed), and nothing else.

A `read` subcommand under the `rehearse` group. --account, --region, --credentials, --read-set and --out are required; --principal is optional. Missing required flags come back as usage through the `rehearse` group's persistent pre-run, so read sets no PersistentPreRunE of its own (cobra runs only the nearest, so one would shadow the group's). The account and region never come from infrafactory.yaml's `aws:` block or from the environment.

The command is off by default: it runs only when invoked, and no run, validate, UI or CI path calls it. It reads the key with prod-read-seam-credentials' reader, holds it in this process only, calls ReadAWSAccount with the real identity check and preflight, then writes the recorded read.

**--out** must resolve (Abs, plus EvalSymlinks on the deepest existing ancestor) under `<repo>/.infrafactory/`, and not under `.infrafactory/runs` or `.infrafactory/live`, which the UI serves (livestore.go:31-40). The directory is 0700 and the file 0600.

Tip commit: `ADR: none — ADR-0041 records the rehearse read contract`. docs/operations.md gets the command's runbook lines, after prod-read-seam-reader-policy's edit. If docs/operations.md has no `## Rehearsal` section yet, this story creates one and puts its lines there.

**Done when:**
- Refusal tests for --out: a repo path, a path outside the repo, `.infrafactory/../x`, a symlink inside .infrafactory that points out, `.infrafactory/runs/x` and `.infrafactory/live/x`. Each exits non-zero before any request.
- A written read has directory mode 0700 and file mode 0600 (stat).
- With aws.account_id set in infrafactory.yaml and no --account, the command refuses, naming --account.
- These run through the cli's fake-doer seam (newAWSHTTPClient), with TestMain's refuse-real-AWS transport kept (aws_ami_resolve_test.go:28). A wrong account exits non-zero with one request on the wire. A DryRunOperation answer exits non-zero naming the action, with no read-set request sent. An out-of-table entry exits non-zero naming it.
- Stdout, stderr and the error never contain the key id, secret or token values planted in the test file.
- Golden help and contract files are updated; `make doc-hygiene` passes; the PR records a cp-backup break that turns each new test red.
