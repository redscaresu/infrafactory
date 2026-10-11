---
kind: code
status: blocked
blocked_by: [prod-read-seam-read-core]
epic: prod-read-seam
depends_on: [prod-read-seam-read-core]
touches: [internal/harness/aws_reader_scrub.go, internal/harness/aws_reader_scrub_test.go, internal/harness/testdata/aws_reader_scrub/, cmd/aws-read-scrub/main.go]
risk: high
---

# The scrub: one synthetic account id, user data, values and ids replaced, and a leak check that fails closed

New `internal/harness/aws_reader_scrub.go` and `cmd/aws-read-scrub`, a dev tool, not the public CLI. It reads a recorded read and the kept pre-destroy tfstate and writes both out.

**Mapping.** The read's account id maps to one synthetic 12-digit id, a named constant, not 000000000000, wherever it appears: the top-level `account` field, params, bodies (plain and URL-encoded, ARNs and owner fields) and the state. The same map applies to both files.

**Placeholders.** Named placeholders replace: EC2 DescribeInstanceAttribute userData (real EC2 XML `<userData><value>…</value></userData>`), the state's user_data, GetCallerIdentity's UserId, every RequestId/requestId, the state's lineage, and every SSM `Parameter.Value` except those under `/aws/service/`, which are AWS-published and carry no account (aws_scope_policy_test.go:29-30). Value is scoped to SSM Parameter.Value and userData to DescribeInstanceAttribute, so tag `<value>`s stay.

**State format.** The tfstate is decoded into `map[string]any` at every level and only the named values change, so version, serial, outputs, schema_version, private and dependencies survive.

**Leak check.** Exported as `harness.CheckScrubbedRead` for prod-read-seam-real-recording's fixture test. It runs over the exact bytes of each output file, whole, before anything is written, and refuses when it finds:
- a 12-digit hit other than the synthetic id, where a hit is a maximal digit run of exactly 12;
- an AWS unique or key id (`A[A-Z]{3}[A-Z0-9]{12,}`) other than a placeholder;
- a userData or Parameter.Value that is neither a placeholder nor an `/aws/service/` value.

Its error names the file and the JSON path, never the matched value.

**Done when:**
- Scrub tests plant the account id in params, the `account` field, a body, an ARN, a URL-encoded ARN, an ownerId and the state, plus a UserId. The output holds only the synthetic id and placeholders, and the same map applies to both files.
- Fixtures use real wire shapes: EC2 XML DescribeInstanceAttribute userData and SSM JSON Parameter.Value, and a real-shaped tfstate (v4, with lineage, outputs and resources with schema_version, private and dependencies).
- Every key of the input state survives in the scrubbed state, and only the named values differ.
- The leak check refuses each of these, and the tool writes nothing: a stray 12-digit number in a body, the account in params, an unreplaced userData, an unreplaced private Parameter.Value, an AKIA/AIDA-style id, a leak in the state. Each error contains no planted value.
- A body whose RequestId ends in 12 decimal digits scrubs and passes; a 13-digit run is not a hit.
- The tool writes its files at mode 0600.
- The PR records a cp-backup break that turns each new test red, including one that drops the mapping for the state.
