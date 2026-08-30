# S153a/b review pass 15 (#172) — one finding, acted on

### [P2] Record each pass in its own file

Passes 11–14 had been appended to `pass10.md` rather than written as
`pass11.md`…`pass14.md`. Checked before acting rather than waved through as a
filing nitpick: `pass5.md`–`pass9.md` are each separate files, so one-file-per-
pass is an actual convention, and it has an audit purpose — anyone asking "did
pass 14 happen, and did this PR converge?" looks for `pass14.md` and finds
nothing.

Split into one file per pass. Cheaper to comply than to argue, and the reviewer
was right about discoverability.

Recorded here rather than argued: the standing instruction is to push back on
low-value findings, and this one sits close to that line. It survives because the
convention is verifiable in the directory listing, not a matter of taste.
