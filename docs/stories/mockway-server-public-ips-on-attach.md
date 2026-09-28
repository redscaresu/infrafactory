---
kind: code
status: later
repo: mockway
touches: ["repository/repository.go"]
---

# mockway updates a server's `public_ips` when an IP is attached or detached later

Follow-up from mockway #31: a server's `public_ips` is set only at server create. Attaching an IP
afterwards via `PATCH /ips/{id} {server}` (what the provider does when `ip_id` changes on an
existing server) leaves the server's `public_ips` stale, so a re-plan drifts on `ip_id`.

**Done when:** a handler test attaches and detaches an IP after server create and GET server shows
`public_ips` matching; a provider example that changes `ip_id` on an existing server re-plans clean.
