# Codex review pass 5 — dedicated Layer 3 credential

Base: `main`. Scope: ADR-0023 credential amendment, AGENTS.md and
NEXT_SESSION.md credential guidance. Documentation-only.

| Pass | Findings | Outcome |
|---|---|---|
| 1 | 1 (P2) | accepted |
| 2 | none | — |
| 3 | none | **converged** |

## Pass 1 — `scaleway_domain*` left unaccounted for — accepted

The amendment named `scaleway_iam*` and `scaleway_registry_namespace` as
types the new credential deliberately cannot create, but omitted
`scaleway_domain*`, which sits in exactly the same position: still in the
default `allow_resource_types`, not covered by the policy. A run
generating one would pass the local allowlist and then fail at the real
API with a 403 that reads like a bug.

A documentation gap with an operational cost, so worth fixing rather than
waving through.

**Documented rather than granted.** `DomainsDNSFullAccess` would let a
canary modify real DNS, and nothing in the suite manages domains today —
`dns_resolution` probes only resolve names, which needs no permission.
Granting a capability nothing uses, on a key whose entire purpose is
minimal privilege, would have been the wrong direction.

## Note

Documentation-only changes still go through the loop, and this one earned
it: the finding was about a mismatch between two documents and a live IAM
policy, which is exactly the kind of thing that is invisible until someone
hits the 403 months later.
