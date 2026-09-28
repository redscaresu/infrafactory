---
kind: code
status: blocked
blocked_by: [layer3-gate-shared-rules, aws-ec2-reads]
epic: aws-layer3-gate
depends_on: [layer3-gate-shared-rules, aws-ec2-reads]
touches: ["internal/cli/layer3_aws_user_data_gate.go (new)", "internal/cli/layer3_aws_user_data_gate_test.go (new)", "docs/stories/aws-gate-user-data-and-ami.md (delete)"]
risk: high
---

# AWS gate, boot leg: user_data is exactly file("${path.module}/infrafactory-user-data.sh"), the file equals this run's render byte for byte, ami is this run's resolved id, and that AMI's root volume is at most 20 GiB of gp3 and deleted on termination

New file internal/cli/layer3_aws_user_data_gate.go. It defines the gate input type awsResolvedAMI{ID string; Root harness.AWSAMIRoot} (aws-ec2-reads).
- awsUserDataExempt is the predicate given to layer3FunctionCallProblems. It is true only for the attribute user_data directly on a resource "aws_instance" whose expression is exactly this: a FunctionCallExpr named "file", with 1 argument and no ExpandFinal, whose argument is a TemplateExpr of exactly [ScopeTraversalExpr path.module, LiteralValueExpr "/infrafactory-user-data.sh"]. That is the parse of generator.AWSUserDataLine (generator.go:50) under hcl v2.24.0. core::file parses as Name "core::file", and a heredoc argument's literal ends in "\n", so neither matches.
- awsInstanceUserDataProblems(block, file) requires user_data to be present and in that shape.
- awsAMIProblems(block, file, resolved) requires ami to be a literal equal to resolved.ID. An empty ID refuses.
- awsAMIRootProblems(resolved) always requires Root.SizeGiB <= 20, Root.VolumeType gp3 and Root.DeleteOnTermination true. This bounds an omitted root_block_device against the real image each run, rather than requiring the block: fakeaws models no block devices (its state carries root_block_device [], internal/harness/testdata/realprobe/aws/terraform-live.tfstate), so a required block would drift at Layer 2. A zero-value Root refuses.
- awsUserDataFileProblem(outputDir, rendered) requires <outputDir>/infrafactory-user-data.sh to be a regular file by Lstat, not a symlink or a directory, with bytes equal to rendered. A nil or empty rendered refuses.

This story makes no ADR edit. The amendment describes these controls once they take effect, in aws-layer3-gate-lift.

**Done when:**
- The expression of generator.AWSUserDataLine, parsed from the e2e web-step-one fixture, gets true from awsUserDataExempt, nothing from awsInstanceUserDataProblems, and nothing from layer3FunctionCallProblems(block, file, awsUserDataExempt)
- One table, two assertions per row. For each shape, awsUserDataExempt returns false, and separately awsInstanceUserDataProblems refuses it. The shapes: file("infrafactory-user-data.sh"), file("/proc/self/environ"), path.root and path.cwd prefixes, file("${path.module}/../infrafactory-user-data.sh"), a trailing space, file(local.p), file(args...), core::file(...), filebase64(...), templatefile(...), trimspace(file(...)), "${file(...)}", "${file(exact)}${file("/proc/self/environ")}", a heredoc, a literal string, var.ud. awsInstanceUserDataProblems also refuses user_data absent
- awsUserDataExempt returns false for the exact expression in an aws_instance tags value, in an output, in a local, and on aws_eip. layer3FunctionCallProblems with it therefore refuses each of those, and awsInstanceUserDataProblems refuses user_data = local.ud
- awsAMIProblems admits the resolved id. It refuses another literal id, var.ami, an empty resolved id, and a missing ami
- awsAMIRootProblems admits {8, gp3, true} and {20, gp3, true}. It refuses {21, gp3, true}, {8, gp2, true}, {8, gp3, false} and the zero value
- awsUserDataFileProblem admits the rendered bytes. It refuses, naming the path: a missing file, a directory, a symlink to a byte-equal file, one differing byte, a missing trailing newline, and nil rendered
- validateLayer3HCLShape still refuses a Scaleway stack whose scaleway_instance_server sets user_data to the exact AWS expression (layer3FunctionCallProblems with nil)
- doc-hygiene passes, and the tip commit carries 'ADR: none — the file() exception is recorded in ADR-0023 by aws-layer3-gate-lift, when it takes effect'
