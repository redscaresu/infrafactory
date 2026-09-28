# aws-web-live-scenario: the lead's model run

Adding `scenarios/training/aws-web-live.yaml` makes the scenario gate run the model on it, so the
lead ran it first (2026-09-28), mock-only: `sandbox_deploy.enabled: false`, no `AWS_*` or `SCW_*`
in the environment, the `claude-code` transport, a throwaway worktree, and a fresh fakeaws built
from origin/main `0a11723` on :18482.

**Result:** run `20260928T160458Z`, `target_reached` in one iteration. The model wrote exactly the
HLD's step-one stack: `aws_vpc`, `aws_subnet`, `aws_internet_gateway`, `aws_route_table`,
`aws_route`, `aws_route_table_association`, one `aws_security_group` (inline `ingress` 80/tcp from
0.0.0.0/0, allow-all egress) and one `aws_instance` (`ami = "ami-0al2023x8664"`,
`associate_public_ip_address = true`, `user_data = file("${path.module}/infrafactory-user-data.sh")`).
infrafactory wrote the provider block (region, `s3_use_path_style`, `default_tags` with the run id)
and the `5.100.0` pin. No pitfall was learned.

**Two earlier attempts** (`20260928T155904Z`, `20260928T160137Z`) never reached the generated code:
the run's mock reset calls `resetS3Backend` whenever `s3.url` is set, ignoring
`s3.auto_reset: false` (`internal/cli/mockway_client.go:129-139`), and no S3 backend was up on
:9091. The third run set `s3.url: ""`. Filed as `s3-reset-honours-auto-reset`.

**For aws-layer3-gate:** the model also wrote `user_data_replace_on_change = true`, which the HLD
has the AWS gate refuse. Recorded on `aws-gate-user-data-and-ami` so the prompt says so first.
