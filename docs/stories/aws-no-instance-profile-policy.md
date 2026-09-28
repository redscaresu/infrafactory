---
kind: code
status: ready
epic: aws-ingress-policy-and-holdout
depends_on: []
touches: ["policies/aws/no_instance_profile.rego (new)", "policies/aws/no_instance_profile_test.rego (new)", "internal/harness/testdata/instance-profile/aws/ (new)", "docs/stories/aws-no-instance-profile-policy.md (deleted)", "docs/review-passes/aws-no-instance-profile-policy.md (new)"]
---

# policies/aws/no_instance_profile.rego denies an instance profile on any AWS plan, at any module depth, known or unknown

- This is the epic's 'iam_instance_profile on aws_instance denied', kept out of default_deny_ingress because it is not an ingress rule and its denial text teaches a different fix. It is plan-only: every AWS plan runs it at Layer 1, and it needs no criterion. aws-layer3-gate refuses the same attribute only at Layer 3 (aws-layer3-gate.md:9).
- Walk resource_changes (module depth included). Deny an aws_instance whose change.after.iam_instance_profile is a non-empty string or whose after_unknown.iam_instance_profile is true. Deny an aws_launch_template carrying an iam_instance_profile block: it is the other way an instance launches with a profile, and walking only one fails open on the other.
- Fixtures are captured 5.100.0 plans from a sealed fakeaws, placed under internal/harness/testdata/instance-profile/aws/.
- Outcomes that can change in LLM runs: aws-instance and aws-full-stack (the aws_instance scenarios). The e2e fixture has no profile.

**Done when:**
- TestRegoPolicyTests passes with no_instance_profile.rego and its _test.rego, and reports no unkilled deny body.
- Captured plans deny: an aws_instance with a literal profile name; one whose profile references an aws_iam_instance_profile with name omitted (unknown at plan); the same inside a local child module; and an aws_launch_template with an iam_instance_profile block. The captured web-step-one plan, which has no profile, passes.
