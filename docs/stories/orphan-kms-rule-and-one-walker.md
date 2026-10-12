---
kind: code
status: ready
touches: [internal/generator/orphan_classify.go, internal/generator/orphan_classify_test.go, internal/harness/destroy.go, internal/harness/destroy_test.go]
risk: high
---

# The aws_kms_key orphan rule teaches a real argument, and one walker decides what is an orphan

From the fakeaws-data-sources-pin conductor (#528).

1. `orphanSubshapeTable`'s `aws_kms_key` entry (orphan_classify.go ~110) teaches `force_destroy = true`,
   which is not an `aws_kms_key` argument. Since #528 it fires only for an `Enabled` key that leaked
   after destroy. Replace the rule with what actually explains that leak, or drop the entry if no
   sub-shape is real. A pitfall must never teach an argument the provider rejects.
2. The post-destroy orphan gate (`internal/harness/destroy.go`) and `ClassifyOrphans` walk mock state
   separately; the mockway default project and the default-VPC exemptions live only in the gate.
   Share one walker (with `OrphanEndState`) so both agree on what is an orphan.

**Done when**
- No orphan rule names an argument absent from the provider schema for its resource (a test checks the
  `aws_kms_key` rule against the schema, or the entry is gone).
- A test feeds the same mock state (mockway default project, default VPC, a PendingDeletion key, a real
  leaked resource) to the gate and to `ClassifyOrphans`, and both report exactly the real leak.
- Deleting either exemption from the shared walker fails that test.
