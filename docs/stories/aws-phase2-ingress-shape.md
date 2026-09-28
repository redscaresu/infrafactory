---
kind: code
status: ready
epic: aws-ingress-policy-and-holdout
depends_on: []
touches: ["prompts/aws/phase2_generate_hcl.md", "internal/generator/aws_ingress_shape_prompt_test.go (new)", "docs/stories/aws-phase2-ingress-shape.md (deleted)", "docs/review-passes/aws-phase2-ingress-shape.md (new)"]
---

# prompts/aws/phase2_generate_hcl.md tells a service scenario to open ports with inline `ingress` blocks, and why

- This story is the single owner of the shape choice, and aws-web-stack-load-balancer inherits it. It is scoped to scenarios with a service:. It renders under {{if .UserDataLine}}, which is set only for an AWS scenario with a service: block (generator.go:35), so no existing AWS scenario's prompt changes (none has one). aws-full-stack and aws-eks can still write groups that reference each other with aws_security_group_rule.
- Proposed sentence, one contiguous string: "Open each port the service needs with an inline `ingress` block inside its `aws_security_group`, and write no `aws_security_group_rule` or `aws_vpc_security_group_ingress_rule` resources: inline blocks are the form this validation environment applies end to end."
- The stated reason claims only what CI proves (TestE2E_AWSWebStepOne applies web-step-one.tf:31-48). The prompt names no policy file (TestPromptsNoOPAPolicyCitations) and no port the holdout probes.

**Done when:**
- A new test renders phase 2 through both the claude and the openrouter adapter, as TestPhase2PromptCarriesUserDataLineOnlyWhenSet does. With UserDataLine set, the prompt contains the exact sentence as one string. Without it, the sentence and both standalone type names are absent. Deleting, inverting or ungating the sentence fails the test.
- The same test parses internal/e2e/testdata/aws-web-step-one/web-step-one.tf with hclsyntax and asserts that every aws_security_group opens its ports through inline ingress blocks and that no aws_security_group_rule or aws_vpc_security_group_ingress_rule resource exists. That ties the stated reason to what CI applies.
- TestPromptsNoOPAPolicyCitations, TestPromptTemplatesRenderAgainstZeroValueContext and TestPhase2PromptCarriesUserDataLineOnlyWhenSet pass.
