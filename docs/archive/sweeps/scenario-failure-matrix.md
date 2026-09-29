# Scenario failure matrix

Snapshot of `infrafactory run --clean <scenario>` outcomes across all
39 training scenarios, built from M88 (baseline) + M94 (AWS re-run
with SeaweedFS up) data on 2026-05-28.

**Headline**: 25/39 pass (64%) with the full M86+M90+M91+M92 chain
live and SeaweedFS started. The remaining 14 are LLM-side issues
(not infrastructure gaps) — the auto-learning loop will improve
these over time as `pitfalls/*.yaml` grow from real runs.

## Pass / fail split

|Outcome|Count|Notes|
|---|---|---|
|Pass (M88 baseline)|20|Scaleway 15/16, GCP 5/12, AWS 0/11 in M88 — but M94 fixed 5 AWS|
|Fixed by SeaweedFS (M94)|5|AWS scenarios that converged once port 9090 was up|
|Other (LLM stuck)|14|See per-cloud breakdown below|

## Per-scenario detail

|scenario|outcome|notes|
|---|---|---|
|aws-dynamodb|fail|M94 still stuck at 2 iter — LLM repeats same DynamoDB ContinuousBackups error|
|aws-eks|✅ pass|Fixed by SeaweedFS — converged in 2 iter on M94|
|aws-full-stack|fail|M94 stuck at 2 iter — composite scenario, multiple resource problems|
|aws-iam|✅ pass|Fixed by SeaweedFS — converged in 1 iter on M94|
|aws-instance|fail|M94 stuck at 2 iter — LLM-side|
|aws-rds|✅ pass|Fixed by SeaweedFS — 4 iter on M94|
|aws-route53|fail|M94 hit repair_budget_exhausted at 5 iter — apex CNAME / record-set rules|
|aws-s3|fail|M94 stuck at 4 iter — bucket sub-resource shape|
|aws-secrets-manager|✅ pass|Fixed by SeaweedFS — 2 iter on M94|
|aws-sqs|✅ pass|Fixed by SeaweedFS — 1 iter on M94|
|aws-vpc-network|fail|M94 repair_budget_exhausted at 5 iter — VPC peering / subnet AZ count|
|block-paris|✅ pass|M88 1 iter|
|compute-lb-multi-paris|✅ pass|M88 1 iter|
|domain-paris|✅ pass|M88 2 iter|
|full-stack-paris|✅ pass|M88 1 iter|
|gcp-cloud-run|✅ pass|M88 2 iter|
|gcp-cloud-sql|fail|M88 stuck at 3 iter — Cloud SQL deletion_protection / IAM SA bind|
|gcp-dns|✅ pass|M88 2 iter|
|gcp-full-stack|fail|M88 stuck at 4 iter — composite scenario, multiple issues|
|gcp-gke-cluster|fail|M88 stuck at 2 iter — GKE node-pool config mismatch|
|gcp-iam|fail|M88 stuck at 4 iter — service-account binding shape|
|gcp-load-balancer|fail|M88 repair_budget_exhausted at 5 iter — backend health-check + frontend wiring|
|gcp-memorystore|✅ pass|M88 2 iter (M70/M86 work)|
|gcp-pubsub|✅ pass|M88 2 iter|
|gcp-secret-manager|✅ pass|M88 2 iter|
|gcp-storage|fail|M88 stuck at 2 iter — bucket lifecycle rules|
|gcp-vm-network|fail|M88 stuck at 2 iter — network/subnetwork referencing|
|iam-policies-paris|✅ pass|M88 1 iter|
|incremental-project-paris|✅ pass|M88 1 iter|
|k8s-cluster-paris|✅ pass|M88 1 iter|
|k8s-medium-override-paris|✅ pass|M88 1 iter|
|lb-paris|✅ pass|M88 1 iter|
|mysql-ha-paris|✅ pass|M88 2 iter|
|private-lb-db-paris|fail|M88 stuck at 2 iter — private LB + DB binding|
|public-registry-iam-paris|✅ pass|M88 1 iter|
|redis-paris|✅ pass|M88 1 iter|
|redis-xlarge-session-paris|✅ pass|M88 1 iter|
|registry-paris|✅ pass|M88 1 iter|
|web-app-paris|✅ pass|M88 1 iter|

## Failure categories

**Infrastructure-side (fixed)**: 5 AWS scenarios (eks/iam/rds/secrets-manager/sqs) — caused by missing SeaweedFS on `make mocks-up`. **Closed by M94.**

**LLM-side (open)**: 14 scenarios that hit stuck-detection or repair_budget_exhausted while the harness was working correctly. These are the natural targets for the auto-learning loop (M86+M90+M91+M92) — each run that hits stuck deposits a learned pitfall, so subsequent runs against the same scenario should converge faster.

|Cloud|Open|Notes|
|---|---|---|
|AWS|6|aws-dynamodb, aws-full-stack, aws-instance, aws-route53, aws-s3, aws-vpc-network|
|GCP|7|gcp-cloud-sql, gcp-full-stack, gcp-gke-cluster, gcp-iam, gcp-load-balancer, gcp-storage, gcp-vm-network|
|Scaleway|1|private-lb-db-paris|

## Reproducibility

```
make mocks-up                                # all 4 services (incl. SeaweedFS)
make build                                   # ./bin/infrafactory
bash scripts/m88_sweep.sh                    # full 39-scenario sweep
bash scripts/m94_aws_proof.sh                # AWS-only re-run
```

Per-scenario logs land in `/tmp/m88_logs/`, `/tmp/m94_logs/`. Results
TSVs in `docs/archive/sweeps/m88-sweep-results.tsv`, `docs/archive/sweeps/m94-aws-resweep-results.tsv`.
