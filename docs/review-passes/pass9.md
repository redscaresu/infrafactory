# S147 review pass 9 — one finding declined

## Declined: "Recount key-only Layer 3 blockers" (P2)

The claim was that `compute-lb-multi-paris` is no longer blocked by the
allowlist, because "its current artifacts use only account/project,
VPC/private network, LB, instance server, and private NIC resources, all of
which are now allowlisted" — and therefore the totals should be 6 key-only
and 8 both, not 5 and 9.

The artifacts say otherwise:

    $ find .infrafactory/runs/compute-lb-multi-paris -name '*.tf' \
        | xargs grep -ho '^resource "[a-z_]*"' | sort -u
    resource "scaleway_instance_ip"
    resource "scaleway_instance_private_nic"
    resource "scaleway_instance_security_group"
    resource "scaleway_ipam_ip"
    ...

`scaleway_ipam_ip` and `scaleway_instance_security_group` are both absent
from the allowlist — the Instances entries admitted were `instance_ip`,
`instance_server` and `instance_private_nic`, named individually rather
than as a glob, precisely so that other Instances types stay denied.

So the scenario remains blocked by **both** gates and the table is right at
5 and 9. Acting on this finding would have moved a scenario into a bucket
it does not belong in, in the document used to decide what to spend real
money on.

## What was done instead

This was the fourth consecutive pass to find an arithmetic or consistency
error in `docs/layer3-coverage.md`, and the third to be correct. Rather
than fix a fifth by hand, the counts are now checked by
`TestLayer3CoverageDocTotalsMatchItsTable` and
`TestLayer3CoverageDocAllowlistMatchesConfig`: the totals line and the
gated remainder are derived from the table's own rows, and the enumerated
allowlist is diffed against `infrafactory.yaml`.

Both verified against synthetic drift. Following the project's
drift-becomes-a-failed-test pattern — the same reasoning as ADR-0021's
cloud-prefix lockstep and the sibling contract audits.
