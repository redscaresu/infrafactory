# Review — AWS phases 1 and 2 get the literal AMI id and size table

Named for the story rather than numbered: pass numbers are a shared counter, and
parallel agents picking the next one collide.

## Declined: [P2] "Set the Layer 3 AWS AMI before requiring it"

The claim: `awsAMIForGeneration` refuses an AWS generate at Layer 3 while
`CommandRuntime.AWSLayer3AMI` is empty, and no production path sets it yet, so a real
AWS Layer 3 `run` or `generate` fails before the generator runs. Wire
`ResolveAWSAMIFromSSM` into the AWS preflight first.

The call site belongs to a different story, by design. The epic
(`docs/epics/aws-layer-neutral-hcl.md`) puts it in aws-layer3-wiring-proof's "AMI
resolved" preflight stage, once per run, after GetCallerIdentity, and says "not this
epic". Wiring it here would add a real-SSM call to the Layer 3 preflight without that
story's evidence or its ordering tests.

Nothing that works today breaks. Before this change an AWS Layer 3 generate already
failed: the model was called, then `layer3PreflightHCLForCloud` refused with "has no
Layer 3 HCL gate yet". Now it fails one step earlier, before the model call, and the
error says what is missing. That is the fail-closed direction: a Layer 3 run that
reached generation without a resolved id would otherwise have handed the model no AMI,
or the fakeaws fixture, for a real account.
