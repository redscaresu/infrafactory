---
kind: verify
status: ready
epic: aws-layer-neutral-hcl
depends_on: [aws-compute-http-probe, aws-provider-layer-neutral, aws-user-data-rendered]
touches: ["internal/e2e/aws_web_step_one_test.go (new)", "internal/e2e/testdata/aws-web-step-one/ (new)", ".github/workflows/ci.yml", "docs/decisions/0013-cross-repo-e2e-and-multi-cloud.md", "docs/stories/aws-web-step-one-e2e.md (delete)"]
---

# Deterministic ADR-0013 e2e: fixed step-one HCL, the rendered script and cloudEnv apply, converge and destroy on fakeaws in CI, and the bytes that reached the instance are the script

New internal/e2e/aws_web_step_one_test.go, sealed with SealNetwork. It uses a testdata scenario (cloud aws, service nginx:1.27 on port 80, http_probe target compute port 80) and a fixed fixture: aws_vpc, aws_subnet, aws_internet_gateway, aws_route_table, aws_route, aws_route_table_association, one aws_security_group with ingress 80 from 0.0.0.0/0 and allow-all egress, and one aws_instance with ami ami-0al2023x8664, associate_public_ip_address = true and the exported user_data line. There is no terraform or provider block. RunInfrafactory `run` drives the real wiring, renderer and cloudEnv. A negative run moves the ingress rule to an unattached group. The CI fakeaws step bumps FAKEAWS_SHA to 5cf7693 (#33); it re-greps every earlier PASS line at that SHA and adds these two. The fixture does not wait on fakeaws's absent examples/working/web_step_one. ADR-0013 gains an AWS note: this test, and that it runs in CI against a pinned fakeaws.

**Done when:**
- Ungated: the fixture has no terraform or provider block, its user_data line equals the exported constant, and the testdata scenario passes schema validation
- CI, sealed: TestE2E_AWSWebStepOne PASSes. The run passes with a converge-plan stage exiting 0 and http_probe compute:80 true. Generated providers.tf pins exactly 5.100.0 with no endpoints
- Same test: fakeaws DescribeInstanceAttribute(userData) for the applied instance, base64-decoded, equals the renderer's golden bytes byte for byte. This is what reached the instance, not the file on disk. After destroy, /mock/state lists no instance
- CI, sealed: TestE2E_AWSWebStepOneUnattachedGroup PASSes by asserting the run FAILED on http_probe compute:80 with the unattached-group diagnostic
- At the bumped SHA the step still greps PASS for TestAWSPreflightAgainstFakeaws, TestE2E_AWSLayer2IgnoresShellAWS, TestE2E_AWSLayer2CatchAllFailsClosed, TestE2E_AWSEnvOnlyEveryService and TestE2E_AWSValidateNeedsFakeawsSTS
