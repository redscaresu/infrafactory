---
status: active
---

# A generated stack is firewalled, not only a declared one

**Done when:** a generated `run --holdout` of `web-live-paris` passes the holdout's port-22 check
against real Scaleway, with no hand-written HCL.

**Out of scope:** security groups for scenarios other than `web-live-paris`; changing the holdout
itself (ADR-0033 §5 forbids feeding it back).

**Constraints:** ADR-0033 (the holdout stays unseen), ADR-0034 (a declared group must deny by
default), ADR-0023 (Layer 3 safety). Real-cloud runs are free-tier or same-run-destroyed.

**Progress, 2026-09-27.** The generator now writes a drop-default security group unprompted, and the
seeded shape passes against real Scaleway. What is left is a generated run that converges.
