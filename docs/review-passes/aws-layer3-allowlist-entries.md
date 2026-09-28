# aws-layer3-allowlist-entries: the one partly declined finding

**Codex pass 1, P2:** `aws-web-live` should not sit in the `runnable, unrun` bucket, because
`layer3PreflightHCLForCloud` still refuses every AWS stack ("no Layer 3 HCL gate yet"); move it
to a blocked bucket until the AWS gate lands.

**Accepted:** the substance. The row said the AWS path was "still being built", which a reader
deciding what can reach the real API could miss. It now names the AWS HCL gate as the blocker
and cites the test that proves the refusal, and a line under the totals says the row is ungated
by the allowlist and the key only.

**Declined:** the bucket move. The doc's blocked buckets are the two gates it counts, `key only`
and `allowlist + key`, and `aws-web-live` is held by neither: the allowlist admits every type it
declares, and the blocker is gate code that epic `aws-layer3-gate` is building. Filing it under
a gate that does not hold it would be the stale-reason defect this doc has retracted twice. The
story's **Done when**, and the epic's, prescribe `runnable, unrun` for this row.
