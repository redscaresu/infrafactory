---
kind: code
status: ready
epic: policy-correctness
depends_on: [policy-loader-skips-test-files, rego-test-harness]
touches: ["policies/aws/vpc_required.rego", "policies/aws/vpc_required_test.rego", "internal/harness/opa_test.go"]
risk: high
---

# AWS vpc_required denies an instance or RDS instance with no subnet in its configuration, joined by index-stripped address

On 5.100.0, subnet_id and db_subnet_group_name are Optional+Computed. Omitted, they are absent from planned_values and after_unknown is true, so `== null` (:19, :37) never fires, and after_unknown (:24-28, :42-46) cannot tell omitted from a reference. Decide from configuration.root_module.resources[].expressions, joined by the address with `[N]` stripped (scaleway vpc_required.rego:97-98). EKS: vpc_config.subnet_ids is a set, so any reference makes it unknown. Keep the rule, which fires only on known IDs, and say so in its comment. It keeps its after_unknown helper, so opa_m98_test.go:52-59 stays green. opa_test.go:756-790 (TestM98_AWSVpcRequiredAcceptsKnownAfterApplyRefs) feeds the omitted shape with no configuration block and would now deny. Rewrite it to add configuration expressions referencing aws_subnet and aws_db_subnet_group, because that is the reference shape it means to pin. Outcome at Layer 1, on every AWS plan: aws-instance, aws-rds, aws-full-stack, aws-eks and aws-vpc-network deny when HCL omits the attribute. The aws_full_stack e2e stub sets db_subnet_group_name (:316) and still passes.

**Done when:**
- Rego test: aws_instance with subnet_id omitted (absent in values, after_unknown true, no expression) denies (fails today)
- Rego test: aws_instance.web[0] whose configuration aws_instance.web has a subnet_id expression does not deny. aws_instance.web[0] without one denies (fails on an unstripped join)
- Rego test: the same omitted and indexed pair for aws_db_instance.db_subnet_group_name
- Rego test: EKS with one literal subnet id denies; EKS with one reference does not (the pinned, stated limit)
- opa_test.go TestM98_AWSVpcRequiredAcceptsKnownAfterApplyRefs passes with configuration expressions added, and still expects zero failures
- The harness mutation check kills all three deny bodies
