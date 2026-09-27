---
kind: code
epic: firewall-end-to-end
status: ready
touches: [internal/generator, internal/cli/run_command.go, prompts/scaleway]
---

# The repair loop brings back an error it had already fixed

Run `20260927T163652Z` (`web-live-paris`): iteration 1 omitted the server's private network and
`vpc_required` denied it; iteration 2 failed on something else (`strcontains()` refused by the
Layer 3 gate); iteration 3 fixed that and omitted the private network again, and stuck detection
ended the run. The fix from iteration 1's feedback did not survive a failure elsewhere. The
iterations' HCL and feedback are under `.infrafactory/runs/web-live-paris/20260927T163652Z/`.

Find why: whether iteration 3's prompt still carried iteration 1's failure, whether the pitfall
for `vpc_required` fired, and whether the security-group pitfall crowded it out.

**Done when:** the cause is named with evidence, the fix keeps an earlier iteration's correction in
force when a later iteration fails elsewhere, and a test reproduces the three-iteration sequence and
fails without the fix. No real-cloud runs; generator runs against the mocks only.
