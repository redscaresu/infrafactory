---
kind: lead
status: blocked
blocked_by: [check-avoid-command]
epic: avoid-pitfall-retirement
depends_on: [check-avoid-command]
touches: ["pitfalls/aws.yaml", "pitfalls/avoid-checks/aws.yaml", "pitfalls/avoid-checks/shapes/<check-id>/main.tf", "docs/hld/2026-09-27-aws-web-stack.md", "docs/epics/avoid-pitfall-retirement.md"]
---

# The pipeline retires the map_public_ip_on_launch avoid rule on its own recorded failing shape, reviewed as ADR-0034 §2 asks

This is a mock run, so the lead does it. Work in a throwaway worktree on main (operations.md:53-54), with fakeaws at the CI pin 6933409 (ci.yml:98), which contains the fix 2118400 (#28). Run `infrafactory pitfalls check-avoid aws --resource aws_subnet --attribute map_public_ip_on_launch --from <main checkout>/.infrafactory/runs/aws-eks/20260603T214517Z/iterations/1/generated`. That dir is gitignored, so it is read only; the command copies the cut. No LLM, no credentials, no real cloud. Commit only the diff the command wrote, with the command, check id, exits and layer_evidence in the commit message. The PR body answers ADR-0034 §2 (0034:52-62). What the rule made impossible to write: a subnet that assigns public IPv4 on launch, set on both aws-eks subnets (network.tf:15,26 of that iteration). Why that is safe to allow again: the only cause was fakeaws's no-op ModifySubnetAttribute (HLD :406-410; auto-learning-loop.md:110-116), fixed in 2118400. Plus the evidence. On close: mark the epic done in the HLD Epics list (:477) and remove the epic file, as done epics are.

**Done when:**
- On the PR, CI's TestE2E_RetiredAvoidPitfallsStayContradicted replays the new aws record against fakeaws at FAKEAWS_SHA and PASSes.
- TestRetiredAvoidPitfallsStayRetired and TestPitfallsNoHumanSeeding PASS with the record present.
- The pitfalls/aws.yaml diff is exactly the removal of aws.yaml:7-10. The ledger gains one retired record with learned_layer mock_deploy, layer_evidence naming run 20260603T214517Z iteration 1, and a shape (aws_vpc.main, aws_subnet.a, aws_subnet.b) whose sha matches.
- If the layer is not established or the outcome is kept, the kept record is the PR instead and the epic escalates.
