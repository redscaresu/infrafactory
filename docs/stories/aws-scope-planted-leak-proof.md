---
kind: lead
status: blocked
blocked_by: [aws-scope-hand-setup, aws-scope-claim, aws-scope-sweep, aws-scope-reap-body, aws-scope-test-lifecycle, aws-scope-reap-command]
epic: aws-layer3-claim-sweep-reap
depends_on: [aws-scope-hand-setup, aws-scope-claim, aws-scope-sweep, aws-scope-reap-body, aws-scope-test-lifecycle, aws-scope-reap-command]
touches: ["docs/operations.md", "docs/layer3-real-vs-mock-deltas.md"]
risk: high
---

# On the real account, one planted leak per swept collection is named by the sweep. A held claim refuses reap by name, and reap removes every leak through each delete it performs. The release leaves only the stamp, and everything still unproven is recorded for the lift

Run by the lead with the user after every code story and the hand setup, from a throwaway worktree built from main (docs/operations.md:52-56). Set S=scenarios/training/aws-web-live.yaml. Planting uses AWS_PROFILE=infrafactory-admin AWS_REGION=$REGION, never the infrafactory key.
1. Baseline: `infrafactory reap $S --dry-run` exits 0 on an empty scope.
2. Plant. Create a VPC 10.99.0.0/16 and a subnet 10.99.1.0/24. Create an IGW and attach it. Create a route table with 0.0.0.0/0 to the IGW, associated with the subnet. Create security groups A and B, where A admits tcp/80 from B. Create a key pair with --query KeyPairId, so the private key is discarded. Run a t3.micro from the AL2023 SSM parameter in the subnet with group A, and wait until it is running. Allocate EIP1 and associate it with the instance; allocate EIP2 and leave it unassociated. Create a detached ENI in the subnet with group B. Create a 1 GiB gp3 volume and a snapshot of it, register an image from that snapshot, then `aws ec2 disable-image`. Create a launch template. Allocate EIP3 and create a NAT gateway with it, waiting until available. Put the SSM parameter /leak/outside.
3. `infrafactory reap $S --dry-run` exits non-zero naming every planted id, including the disabled image, the ENI and the unassociated EIP. A describe afterwards shows everything still present.
4. `aws ssm put-parameter --name /infrafactory/layer3/claim --type String --value planted@elsewhere:1`. Then `infrafactory reap $S` is refused naming planted@elsewhere:1 and printing the --take-over command, and deletes nothing.
5. `infrafactory reap $S --take-over planted@elsewhere:1` performs every delete in the reap table. It fails only on /leak/outside, which is report-only, and keeps the claim.
6. `aws ssm delete-parameter --name /leak/outside` (admin). Then `infrafactory reap $S --take-over <the holder step 5 printed>` sweeps clean, releases, and exits 0.
7. Describes of every collection are empty, and describe-parameters shows only the stamp.
8. `INFRAFACTORY_AWS_LAYER3_REAL=1 INFRAFACTORY_AWS_LAYER3_REGION=$REGION INFRAFACTORY_AWS_LAYER3_ACCOUNT=$ACCOUNT_ID go test ./internal/harness -run '^TestAWSScopeClaimAgainstRealSSM$' -count=1 -v` PASSes. This proves TakeAWSClaim, the test and run claim primitive, on real SSM.

**Done when:**
- Step 3 names each planted id, the disabled image included, and a describe afterwards shows them all still present.
- Step 4 refuses naming the holder and makes no change.
- Step 5's output shows each delete action in the reap table sent at least once, and no AccessDenied or UnauthorizedOperation anywhere.
- Steps 6 and 7: exit 0, every collection empty, only the stamp under the prefix. Step 8 PASSes.
- docs/operations.md § Layer 3 (AWS) gets a dated line: the policy in force was sufficient for claim, sweep and every reap delete. Any real-vs-fake difference goes into docs/layer3-real-vs-mock-deltas.md.
- The PR body lists what stays unproven for aws-layer3-gate-lift and the first real run to require, since the gate refuses aws before the claim (test_command.go:690 vs :836): test and run taking and releasing the claim on real AWS; a second test refused naming the holder; the failed-apply sweep; the run and test interrupt prints; tofu destroy's IAM actions. The PR body also gives the spend (NAT hour, three EIP hours, a t3.micro, under €0.20) and the date.
