---
kind: code
status: ready
epic: prod-read-seam
depends_on: [prod-read-seam-adr-0041]
touches: [internal/harness/aws_reader_credentials.go, internal/harness/aws_reader_credentials_test.go, internal/harness/aws_sealed_env.go, docs/operations.md]
risk: high
---

# A second credential-file reader: the two keys plus an optional AWS_SESSION_TOKEN, mode 0600, never echoed

New `internal/harness/aws_reader_credentials.go` follows the Layer 3 reader's rules (aws_sealed_env.go:58-99): Lstat, regular file only, no group or other bit, KEY=VALUE lines, duplicate keys refused by key name. It admits exactly AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY and an optional AWS_SESSION_TOKEN. An empty AWS_SESSION_TOKEN is refused, never read as absent.

**Never echo.** An unknown key is named only when it matches `^[A-Z][A-Z0-9_]*$`. Anything else reports `line N has an unrecognised key` and prints nothing from the line. That covers a wrapped token (base64 ending in `=`) and a reversed `<secret>=AWS_SECRET_ACCESS_KEY` line.

**Shared core.** It may share a parsing core with readAWSCredentialFile only if that core takes the allowed-key set as a parameter, the Layer 3 call keeps its two keys, and the Layer 3 error text stays as is. aws_sealed_env.go is edited only if the core is shared.

**Config builder.** A reader aws.Config builder lives in aws_reader_credentials.go: region from the caller, validated with ValidAWSRegion, and a static provider carrying the session token. sealedAWSConfig (aws_client.go:50-62) drops the token and stays as is; aws_client.go is not in the diff. If a builder edit there proves unavoidable, add it to touches.

docs/operations.md gets a section for the reader key file. On that file this story comes first: credentials, then reader-policy, then the CLI.

**Done when:**
- A refusal table covers a symlink, a directory, modes 0640 and 0604, an unknown key, a duplicate key, a missing secret, an empty value, an empty `AWS_SESSION_TOKEN=`, a wrapped-token line `<distinctive>==` and a reversed line `<distinctive>=AWS_SECRET_ACCESS_KEY`. Each error names the key or the rule, and contains none of the distinctive values or fragments planted in the file.
- The builder refuses the regions `''`, `x@evil.example/` and `us-east-1.evil.com` (as aws_sealed_env_test.go:103-106).
- With AWS_SESSION_TOKEN set, a request signed through the built config carries X-Amz-Security-Token equal to it; without one, that header is absent.
- TestAWSSealedEnvRefuses, including its AWS_SESSION_TOKEN row, and TestAWSSealedEnvHasExactlyTheSixKeys pass; aws_sealed_env_test.go and aws_client.go are not in the diff.
- docs/operations.md names the file's keys and mode and says it is a separate file from layer3-aws.env, whose lines 72-73 are unchanged; `make doc-hygiene` passes.
- The PR records a cp-backup break that turns each new test red.
