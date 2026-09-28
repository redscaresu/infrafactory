---
kind: code
status: ready
epic: aws-layer-neutral-hcl
depends_on: [aws-web-step-one-e2e]
touches: ["internal/harness/aws_ami.go (new)", "internal/harness/aws_ami_test.go (new)", "internal/e2e/aws_ami_fakeaws_test.go (new)", "internal/e2e/aws_web_step_one_test.go", "internal/generator/generator.go", "internal/generator/prompt.go", "internal/generator/claude_adapter.go", "internal/generator/openrouter_adapter.go", "internal/generator/aws_phase1_literals_test.go (new)", "internal/cli/generate_command.go", "internal/cli/runtime.go", "internal/cli/layer3_cloud_test.go", "prompts/aws/phase1_plan_architecture.md", "prompts/aws/phase2_generate_hcl.md", "go.mod", "go.sum", ".github/workflows/ci.yml", "docs/stories/aws-phase1-literals.md (delete)"]
risk: high
---

# AWS phases 1 and 2 get the literal AMI id and size table they are told to use

New internal/harness/aws_ami.go. AWSLayer2AMI is "ami-0al2023x8664", fakeaws's AL2023 answer (../fakeaws handlers/ec2.go:2354, ssm.go:27,153). ResolveAWSAMIFromSSM(ctx, env, doer, endpoint) builds an ssm client exactly as NewAWSSTSClient does (aws_client.go:28-52): static creds from the sealed env map and an explicit BaseEndpoint, never config.LoadDefaultConfig. It reads /aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-x86_64. It refuses a nil doer. An error, an empty value or a non-ami- value fails naming the parameter. endpoint "" means real SSM, so every test passes harness.NewLoopbackOnlyHTTPClient and an explicit endpoint; a missing endpoint then dies at the dialer. go.mod gains aws-sdk-go-v2/service/ssm. CommandRuntime gains the resolved Layer 3 AMI, set by aws-layer3-wiring-proof's once-per-run call. generate puts AWSLayer2AMI on Request for aws at Layer 2. For aws at Layer 3 with no resolved id, it refuses before calling the generator. Request and PromptContext gain the id, and both renderPhasePrompt builders copy it. prompts/aws/phase1 drops the empty {{.ResolvedMappings}} block (:9-13) and puts a literal AWS size table in its place; :41 tells the model to write the rendered AMI id verbatim. phase2 repeats the AMI instruction. mappings.yaml is untouched. aws_web_step_one_test.go's fixture switches to harness.AWSLayer2AMI. CI: the fakeaws step (already at 5cf7693, which contains SSM a72573e) greps the parity test's PASS line. ADR: none — implements aws-layer-neutral-decisions.

**Done when:**
- An httptest SSM stand-in answers ami-0deadbeef1234567 through a loopback-only doer with an explicit endpoint. The resolver returns that id and records exactly one GetParameter for the AL2023 name, so a resolver returning AWSLayer2AMI fails
- It refuses, naming the parameter, on ParameterNotFound, an empty value, a non-ami- value, a 500 and a nil doer. With endpoint "" and the loopback-only doer it fails with ErrNonLoopbackDial: nothing reaches real SSM
- CI fakeaws step: TestAWSAMIParityAgainstFakeaws (loopback-only doer, explicit fakeaws SSM endpoint) returns exactly AWSLayer2AMI
- A fake-generator generate for aws at Layer 2 puts ami-0al2023x8664 on Request. renderPhasePrompt on BOTH ClaudeSeedGenerator and OpenRouterSeedGenerator, with the real prompts/aws phase1 and phase2 templates, contains the id. An adapter that forgets the field fails
- TestGenerationGateIsKeyedOnTheScenarioCloud (internal/cli/layer3_cloud_test.go:160-190) changes meaning, as named here. Its 'aws is refused' case now sets a resolved AMI on the runtime and asserts "has no Layer 3 HCL gate yet" (layer3_cloud.go:40), so it still reaches the gate. A new 'aws without a resolved AMI' case asserts the AMI refusal and that the generator was never called
- Rendering prompts/aws/phase1 with a zero PromptContext yields no empty yaml block where ResolvedMappings was. The table maps every (resource, size) pair declared in scenarios/training/aws-*.yaml, with compute small = t3.micro. Emptying the table or restoring the placeholder fails
- TestNoProductionCodeUsesTheAWSDefaultChain (aws_sdk_audit_test.go:39), TestPromptTemplateFieldsExistOnPromptContext and TestPromptTemplatesRenderAgainstZeroValueContext pass
