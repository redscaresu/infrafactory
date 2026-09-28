---
kind: code
status: ready
epic: avoid-pitfall-retirement
depends_on: [avoid-check-ledger]
touches: ["internal/cli/pitfalls_command.go", "internal/cli/pitfalls_avoid_check.go", "internal/cli/pitfalls_avoid_check_test.go", "internal/generator/avoid_shape.go", "internal/generator/avoid_shape_test.go", "internal/e2e/avoid_check_test.go", ".github/workflows/ci.yml", "docs/decisions/0034-a-prohibition-is-a-specification.md", "docs/operations.md", "docs/plans/pitfall-pruning-automation-plan.md"]
---

# `infrafactory pitfalls check-avoid` replays a mock-learned rule's shape against its cloud's mock, with no LLM, and CI replays every retired record

Register check-avoid beside retire (pitfalls_command.go:20-29) per evidence_contract.how_it_runs_without_an_llm. New internal/generator/avoid_shape.go cuts the shape from a --from directory with loadResourceBlocks and referencesIn (prescriptive_extractor.go:206,611), and establishes the legacy layer from run artifacts per evidence_contract.legacy_layer. It refuses before any tofu call when: a required attribute is missing; a value is a literal false/null/""; the layer is not established; the ledger is malformed; the cloud is invalid; or the cloud has no mock URL. Only aws is wired; other clouds are refused. It classifies, copies the cut under pitfalls/avoid-checks/shapes/<id>/, calls RetireAvoidPitfall or records kept, and reports every record written. The replay goes through one constructor that both the command and the e2e replay use. Extends the ADR-0034 amendment with this story's controls; updates the pitfall-pruning plan; adds a docs/operations.md line.

**Done when:**
- Unit tests with a fake CommandRunner and a fake mock client: apply 0 and plan 0 retire the rule. Plan exit 2 gives kept (drift) with the corpus byte-identical. An apply failure naming MapPublicIpOnLaunch or aws_subnet gives kept/recurred; any other failure gives kept/inconclusive.
- Every command the fake runner sees carries Layer2StripEnv, AWS_ACCESS_KEY_ID=test and the fakeaws endpoint env. Each refusal records zero runner calls.
- The written providers.tf comes from ensureAwsProviderWiring with runID "": no default_tags, no endpoints. The recorded shape_sha256 equals ShapeSHA256 of the stored cut.
- TestEstablishLegacyLayer on temp dirs in the run layout: established when all three conditions hold. Refused for each of: scenario mismatch, a non-loopback or missing endpoints block, a missing iteration.json, and a detail that does not name every attribute.
- The handler is built with withRuntimeNoGenerator, and a test proves a generate call from it errors.
- TestE2E_CheckAvoidAgainstFakeaws runs the binary's `pitfalls check-avoid` with mockway configured at a closed port and fakeaws started. The temp corpus holds an avoid entry for (aws_subnet, map_public_ip_on_launch) with learned_layer mock_deploy, and --from is a temp dir holding web-step-one.tf. The test asserts fakeaws's request log has POST /mock/reset and the record is retired with apply_exit 0 and plan_exit 0.
- TestE2E_RetiredAvoidPitfallsStayContradicted replays every retired record's stored shape in the repo's pitfalls/avoid-checks/aws.yaml through the same constructor against fakeaws. It fails, naming the record, unless the outcome is contradicted; it passes on an empty ledger and fails closed on a retired record for a cloud with no CI mock.
- Both e2e tests are added to the ci.yml:105 list, and the step requires their PASS lines.
- doc-hygiene CI is green.
