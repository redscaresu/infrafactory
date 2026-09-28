---
kind: code
status: ready
touches: ["internal/cli/mockway_client.go", "internal/cli/mockway_client_test.go"]
---

# The mock reset honours `s3.auto_reset: false`

`cloudMockStateRouter.Reset` and `ResetAll` call `resetS3Backend` whenever `s3.url` is set
(`internal/cli/mockway_client.go:129-139`, `:241-244`), ignoring `s3.auto_reset`. With no S3
backend running, every AWS iteration fails at `test/reset` before the generated code is tested,
even for a scenario that uses no S3 (found by the aws-web-live lead run, 2026-09-28).

**Done when:** a test with `s3.url` set and `s3.auto_reset: false` shows `Reset` and `ResetAll`
make no S3 request; with `auto_reset: true` they still reset it (the M59 BucketAlreadyExists fix
stays).
