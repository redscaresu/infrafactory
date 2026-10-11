---
kind: code
status: blocked
blocked_by: [rehearsal-backend-init-assumptions]
epic: rehearsal-assumption-tests
depends_on: [rehearsal-backend-init-assumptions]
touches: [internal/cli/rehearsal_assumptions_test.go, .github/fakeaws-e2e-tests]
---

# Claims (e) and (f) against fakeaws: an unreadable data source exits refresh-only with 1, never 2; an empty mock still receives every independent data source's request

These tests are gated and go in `rehearsal_assumptions_test.go`, with the file's env, `max_retries` and deadline rules. Each test starts its own fresh `e2e.StartFakeaws`, before SealNetwork.

**Recording requests.** A recording reverse proxy in the test (httputil.ReverseProxy in front of fakeaws) records the service and the Action, taken from the form body (EC2/STS) or from X-Amz-Target. fakeaws's audit table is empty at the pin, and fakeaws changes are out of scope.

**Routing.** The endpoints override routes through the recorder with fakeaws's path prefixes (`<recorder>/ec2/region/<region>`, `<recorder>/sts`, as in awsFakeawsPaths, internal/cli/test_command.go:199-212). The recorder preserves the path; a bare host gives STS a 501.

**Note for the builder:** at this pin fakeaws ignores DescribeVpcs filters and lists every VPC (../fakeaws/handlers/ec2.go:278-288). So a filter that matches nothing does not make a data source unreadable here.

**(e) TestAssumption_RefreshOnlyUnreadableDataSourceExitsOne**, relied on by rehearsal-against-the-copy:
1. Apply an aws_vpc to fakeaws.
2. Settle the state with `apply -refresh-only -auto-approve`, then assert that `plan -refresh-only -detailed-exitcode` exits 0.
3. Make drift through fakeaws's EC2 API (CreateTags). `plan -refresh-only -detailed-exitcode -out p` now exits 2, and `show -json p` carries the drift tag.
4. Add `data "aws_vpc" "missing" { id = "vpc-0000000000000dead" }`. The plan then exits 1, and its output names `data.aws_vpc.missing`.
5. `apply -refresh-only -auto-approve` exits 0 with drift alone, and 1 with the data source, again naming it.

**(f) TestAssumption_DataSourcesAllSendBeforePlanFails**, relied on by rehearsal-against-the-copy. Against a fresh, empty fakeaws, it declares more independent data sources than tofu's default -parallelism (10): a counted `aws_vpc` data source, each looking up a distinct nonexistent id. `plan` exits 1, and the recorder holds one DescribeVpcs per index, told apart by the id.

Append both names to `.github/fakeaws-e2e-tests`. Doc comments cite ADR-0028, ADR-0032, and the HLD's "Discover" and "Load and check" sections. Tip commit carries `ADR: none — …`.

**Done when:**
- With INFRAFACTORY_ENABLE_E2E=1 and ../fakeaws at the CI FAKEAWS_SHA, `go test ./internal/cli -run '^TestAssumption_(RefreshOnlyUnreadableDataSourceExitsOne|DataSourcesAllSendBeforePlanFails)$' -count=1 -v` PASSes locally, and the PR's aws-e2e log has both `--- PASS: ` lines.
- The PR body records the cp-backup breaks with their FAIL output:
  - (e): the missing data source's id set to the applied VPC's id, so it exits 2, not 1
  - (e): the CreateTags call dropped, so it exits 0, not 2
  - (f): a depends_on from every index to index 0, so the recorder misses indexes
- `LC_ALL=C sort -c <(grep -v '^#' .github/fakeaws-e2e-tests)` exits 0; there is no os.Environ() in the file; no file under ../fakeaws changes.
- `make doc-hygiene` passes.
