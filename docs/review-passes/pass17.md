# S153a/b review pass 17 (#172) — one finding, acted on

### [P2] STATUS pointed at `pass10.md` for content that had moved

A broken reference created by pass 15's own fix: splitting the bundled passes
left `STATUS.md` claiming the convergence record lives in
`docs/review-passes/pass10.md`, which now ends at "Re-running for a clean pass".
A reader following that pointer would miss passes 11–13 and the clean
convergence.

Factually wrong rather than a matter of taste, and caused by this PR — so acted
on. STATUS now points at the directory and the range.

Worth noting as a small instance of the pattern this session kept producing: a
fix that corrects one thing and silently invalidates something that referred to
it. The review loop caught it; nothing else would have.
