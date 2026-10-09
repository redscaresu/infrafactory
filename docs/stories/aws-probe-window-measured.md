---
kind: lead
status: blocked
blocked_by: [aws-learned-pitfall-from-real-aws]
epic: aws-web-live-on-real-aws
depends_on: [aws-learned-pitfall-from-real-aws]
touches: [infrafactory.yaml, internal/config/config.go]
---

# The shared probe window is checked against AWS's measured first-success times, and the measurement is recorded

The epic's sixth bullet. No real run here. There is no per-cloud field: RealProbeConfig is one
shared value (internal/config/config.go:185-189, 366-370), pinned to infrafactory.yaml by
TestRealProbeWindowPinnedToDefault (internal/config/real_probe_window_test.go:19).
aws-layer-neutral-hcl deferred any split to this epic. Take seconds to first success from every
real run's `real_probe` record (aws-layer3-run-evidence). If the slowest fits inside 60 x 5s with
margin, leave the value and add a dated AWS line beside the Scaleway measurement in
infrafactory.yaml (:203-249) and the config.go comment. If not, widen the one shared value in both
files; the probe returns on first success, so a healthy stack pays nothing. No new field (YAGNI).
Shared root config, so the lead edits it alone. Trailer on the tip commit:
`ADR: none — measured probe window, no contract change`.

**You:** nothing; the lead tells you the measured numbers in the PR.

**Done when:**
- infrafactory.yaml and config.go each carry a dated line giving the AWS runs' seconds to first
  success and the run ids, and TestRealProbeWindowPinnedToDefault passes.
- The slowest measured AWS first success is under retries x retry_delay_seconds in the committed
  value. The PR shows the arithmetic.
- `make doc-hygiene` passes on the PR's range, with the ADR trailer on the tip commit.
