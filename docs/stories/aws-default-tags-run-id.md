---
kind: code
status: blocked
blocked_by: [fakeaws-tags-every-service]
epic: aws-layer-neutral-hcl
depends_on: [fakeaws-tags-every-service]
touches: ["internal/cli/generate_command.go", "internal/cli/cloud_parity_test.go", "internal/cli/aws_default_tags_test.go (new)", "prompts/aws/phase2_generate_hcl.md", "docs/decisions/0039-layer-neutral-aws-generated-hcl.md", "docs/stories/aws-default-tags-run-id.md (delete)"]
---

# The generated AWS provider block carries default_tags with the run id, now that every service a full-stack scenario can touch round-trips tags on fakeaws

ADR-0039 decision 9 deferred `default_tags` out of `aws-layer-neutral-hcl` because fakeaws did not
round-trip tags on sqs, iam, rds, route53, dynamodb and eks — a `default_tags` block added before
that gap closed would silently no-op against those services. `fakeaws-tags-every-service` closes
the gap. This story adds the block `buildAwsProviderBlock` (internal/cli/generate_command.go)
already returns region and s3_use_path_style from: `default_tags { tags = { infrafactory-run-id =
"<run id>" } }`, sourced the same way region is, from the run's id, never from the model. The tag
key name matches whatever `aws-layer3-claim-sweep-reap`'s sweep/reap ends up reading by, so that
epic's collection-list-only sweep can add a tag-based cross-check once this lands.

**Done when:**
- `buildAwsProviderBlock`'s output gains `default_tags { tags = { infrafactory-run-id = "<id>" } }`
  for a non-empty run id; the id is never taken from model output or scenario YAML
- The byte-identity test from `aws-provider-layer-neutral` is extended: the provider block is
  identical across layers except for the tag value, which equals the run id at both
- A recordingRunner or fakeaws-backed test proves at least one resource in each of sqs, iam, rds,
  route53, dynamodb and eks carries the run-id tag after apply, round-tripped through fakeaws's
  now-tagging handlers
- prompts/aws/phase2_generate_hcl.md tells the model default_tags is infrafactory's block and the
  model must not declare its own `default_tags` or per-resource `tags.infrafactory-run-id`
- ADR-0039 gets a short amendment recording that decision 9's deferral is resolved and pointing at
  this story's PR
