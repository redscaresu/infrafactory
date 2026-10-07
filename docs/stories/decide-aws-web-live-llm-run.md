---
kind: operator
status: blocked
blocked_by: [aws-layer3-ami-resolve-wiring, aws-layer3-stage-order-test, aws-layer3-run-checklist]
epic: aws-layer3-wiring-proof
depends_on: [aws-layer3-ami-resolve-wiring, aws-layer3-stage-order-test, aws-layer3-run-checklist]
---

# Decide: go for the one real-LLM Layer 2 run of aws-web-live

The user's call, once the wiring, order test and checklist have landed. The run costs one LLM
run against fakeaws (no real cloud) and is the only LLM run before aws-web-live-on-real-aws. If
yes, the lead runs aws-web-live-layer2-llm-run and reports the run id.

**Done when:** the user has said go or no. This file is deleted either way.
