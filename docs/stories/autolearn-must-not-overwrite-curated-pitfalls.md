---
kind: code
risk: high
status: ready
touches: [internal/generator]
---

# Auto-learning must not overwrite a curated pitfall

After generated run `20260927T171147Z` (`web-live-paris`) self-corrected, auto-learning rewrote
`pitfalls/scaleway.yaml`. It did not add an entry. It **replaced** two `scaleway_lb_backend` entries —
a `descriptive` one from `incremental-project-paris` and a reviewed `source: avoid` one, the
destroy-safety guardrail about indexing `private_ips` — with the **same** new `source: fix` entry
written twice:

```
- resource: scaleway_lb_backend
  rule: |-
    exit status 1 | stderr: ╷ Fix observed in scenario "web-live-paris": add the following HCL.
    resource "scaleway_lb_backend" "web" {
      server_ips       = [scaleway_instance_ip.web.address]
      ...
  source: fix
  discovered_from: web-live-paris
```

The rewrite was caught in review before it merged, and the file was restored to `main`. Find the
code path that replaced entries rather than appending (likely keyed by resource), and why the same
entry was written twice.

**Done when:** a learned entry never replaces or removes an existing entry, least of all a
`source: avoid` one; an identical entry is not written twice; and a test that starts from the
pre-run file and applies this learning event fails without the fix.
