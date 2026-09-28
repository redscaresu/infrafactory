---
kind: code
status: ready
epic: aws-layer3-seal-and-dispatch
depends_on: []
touches: ["internal/harness/aws_placement.go (new)", "internal/harness/aws_placement_test.go (new)", "docs/stories/aws-account-helper.md (delete)"]
risk: high
---

# One helper answers 'does this state resource belong to account X', with the AWS child list, for aws-layer3-gate's post-apply check

New internal/harness/aws_placement.go exports AWSChildScopedTypes: aws_route [route_table_id] and aws_route_table_association [route_table_id, subnet_id]. It also exports one helper that reads terraform-live.tfstate the way RunWithoutConfig does (loadLiveTerraformState) and names every managed AWS resource not in account X. A resource is placed when its ARN account field and owner_id, whichever are present and non-empty, all equal X, and at least one is present. A child is placed through parents that are managed resources in the same state. Anything else is refused (HLD:338-341). Data sources and builtins are skipped (managedCloudResource). sandbox_destroy.go is unchanged: unplacedResources already refuses AWS resources because they carry no project_id (:211-218), and no AWS caller of RunWithoutConfig exists in this epic.

**Done when:**
- Table test, each case named: ARN account X is placed. Another account is refused, naming type, id and account. An empty ARN account with no owner_id (fakeaws's shape, HLD:99) is refused. owner_id X alone is placed. ARN X with owner_id Y is refused. aws_route is placed with its route table in the state and refused without it. aws_route_table_association without subnet_id is refused. A type with neither field and off the child list is refused. A data source is ignored. An unreadable state is an error, not an empty list
- RunWithoutConfig on an AWS state still returns ErrProtectedProject and runs no tofu (recording runner), so admitting AWS there stays a deliberate later change
- sandbox_destroy_test.go passes unmodified
