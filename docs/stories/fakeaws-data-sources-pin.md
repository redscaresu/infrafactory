---
kind: chore
repo: infrafactory
status: blocked
blocked_by: [fakeaws-subnet-overlap-refused, fakeaws-data-source-example, infrafactory-filter-refusal-routes-to-mock-gaps, infrafactory-fakeaws-identity-pin, fakeaws-rehearsal-instance-pin-and-docs]
epic: fakeaws-data-sources
depends_on: [fakeaws-subnet-overlap-refused, fakeaws-data-source-example, infrafactory-filter-refusal-routes-to-mock-gaps, infrafactory-fakeaws-identity-pin, fakeaws-rehearsal-instance-pin-and-docs]
touches: [.github/workflows/ci.yml, docs/layer3/real-vs-mock-deltas.md, docs/hld/2026-09-30-import-production-state-into-the-mocks.md, docs/epics/fakeaws-data-sources.md, docs/stories/prod-read-seam-fakeaws-filtered-e2e.md, internal/cli/rehearsal_assumptions_test.go, docs/operations.md, docs/stories/fakeaws-data-sources-pin.md]
---

# Pin FAKEAWS_SHA to the fakeaws main holding the data sources, close D8, record the unverified codes, re-point the filtered e2e, close the epic

One infrafactory PR, landing after both sibling pins, so the shared FAKEAWS_SHA line
(.github/workflows/ci.yml:226), the HLD's Epics list and docs/operations.md are each edited in turn.

- Bump FAKEAWS_SHA to a fakeaws main commit holding the nine fakeaws merges of this epic.
- D8 (docs/layer3/real-vs-mock-deltas.md:230-241) records that sibling overlap is refused with
  InvalidSubnet.Conflict since the fakeaws PR.
- A new deltas.md section lists the EC2 error codes fakeaws now answers from the API reference, not
  a recording, each marked `UNVERIFIED(aws-code)` at its fakeaws site: VPCIdNotSpecified (GroupName.N),
  InvalidSubnet.Conflict and its message, InvalidAddress.NotFound (PublicIp.N),
  InvalidKeyPair.NotFound (KeyPairId.N), InvalidParameterValue (unknown ZoneName.N/ZoneId.N); and the
  fixed a-c AZ set. Owner: the next real-AWS recording (prod-read-seam-real-recording or a Layer 3
  run), which replaces each with what AWS answered.
- In the HLD, replace the Epics line (:480) with `- fakeaws-data-sources — done (#…)`, without a
  link, and make :327-329 stop saying overlap is not checked. Delete the epic file and this story
  file in the same PR (doc-hygiene fails a story naming a missing epic).
- In prod-read-seam-fakeaws-filtered-e2e, blocked_by and depends_on name fakeaws-data-sources-pin
  (the board PR that landed this plan re-pointed them; if not, do it here). This PR then drops
  fakeaws-data-sources-pin from its blocked_by, keeping depends_on, and sets status ready if
  prod-read-seam-ec2-readers has merged. Check that story records
  DescribeSecurityGroupRules as not modelled at this pin, owned by fakeaws-loader.
- Correct the comment 'fakeaws ignores DescribeVpcs filters' at
  internal/cli/rehearsal_assumptions_test.go:290-291. internal/cli/ is a DECISION_PREFIX
  (scripts/check_doc_hygiene.sh:94), so the tip commit carries
  `ADR: none — pin, comment and docs only; no infrafactory behaviour changes` (the learner change is
  its own story).
- If docs/operations.md names a status for filter-refused, make it 501.

**Done when:**
- `git -C ../fakeaws merge-base --is-ancestor <sha> <FAKEAWS_SHA>` exits 0 for each of the nine
  fakeaws story merges, and for the FAKEAWS_SHA that infrafactory-fakeaws-identity-pin and
  fakeaws-rehearsal-instance-pin-and-docs each set.
- CI 'AWS against fakeaws' is green at the new pin with .github/fakeaws-e2e-tests unchanged, and each
  test name prints its PASS line.
- `grep -n 'still not checked' docs/layer3/real-vs-mock-deltas.md` prints nothing, D8 names
  InvalidSubnet.Conflict and the fakeaws PR, and the new section names each unverified code with its
  owner.
- The HLD Epics line reads `fakeaws-data-sources — done (#…)` with no link.
  docs/epics/fakeaws-data-sources.md no longer exists, and
  `grep -rn 'epics/fakeaws-data-sources' docs STATUS.md` prints nothing.
- `grep -n -E 'fakeaws-data-sources[],]' docs/stories/*.md` prints nothing, and
  prod-read-seam-fakeaws-filtered-e2e names fakeaws-loader as DescribeSecurityGroupRules' owner.
- `grep -n -i 'filter-refused' docs/operations.md` shows no 409, and `make doc-hygiene` passes.
