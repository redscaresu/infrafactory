# Security Policy

## Reporting a vulnerability

Please **do not** open a public GitHub issue for security vulnerabilities.
Instead, report privately via one of:

- GitHub's [private vulnerability reporting](https://github.com/redscaresu/infrafactory/security/advisories/new) (preferred — keeps the entire thread off public timelines).
- Email: `ukashouri@gmail.com` with subject prefix `[security] infrafactory:`.

Please include:

- A description of the issue and its impact.
- Steps to reproduce (or a proof-of-concept).
- Affected commit / version / branch.
- Any mitigations you've already identified.

## What to expect

- **Acknowledgement**: within 5 working days of report.
- **Assessment**: within 14 days of acknowledgement we'll either confirm the issue or explain why we believe it isn't one.
- **Fix**: timeline depends on severity. Critical issues are prioritized; lower-severity issues batch into the next minor release.
- **Disclosure**: coordinated. We'll credit you in the release notes / advisory unless you ask otherwise.

## Scope

In scope:

- This repository (`redscaresu/infrafactory`).
- The sibling mock servers shipped alongside it: `redscaresu/mockway`, `redscaresu/fakegcp`, `redscaresu/fakeaws`.
- Any infrastructure-as-code emitted by the generator that exposes a known CVE pattern (e.g., generated HCL that creates a public S3 bucket without owner-block hardening).

Out of scope:

- Vulnerabilities in upstream dependencies — please report those to the upstream project. We monitor advisories via Dependabot.
- Issues in OpenTofu, Claude Code, OpenRouter, or any cloud provider's API surface — report upstream.
- Reports based solely on outdated cryptographic algorithm preferences without a concrete attack scenario.

## Hardening notes for users

- The CLI accepts cloud credentials via env vars (`SCW_ACCESS_KEY`, `GOOGLE_CREDENTIALS`, `AWS_ACCESS_KEY_ID`, `OPENROUTER_API_KEY`, etc.). Never commit `.env` files or credential JSON — `.gitignore` blocks the common patterns and a pre-commit `gitleaks protect` runs on every commit when `make install-hooks` has been run.
- Layer 3 ("real cloud") deploys are opt-in. The default `infrafactory.yaml` has `validation.layers.sandbox_deploy.enabled: false` so a fresh checkout cannot reach any cloud control plane.
- The generated HCL is sandboxed under `output/<scenario>/` and is intended for `tofu apply` against a mock or a throw-away project; it is not a hardening reference for production.
