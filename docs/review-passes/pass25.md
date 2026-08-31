# S165 review pass 25 — one finding, acted on

### [P2] The run project leaked when destruction was skipped

Third cleanup-placement bug in this slice, and the one that made the pattern
obvious. The cleanup lived inside the `Destruction.Enabled && !opts.SkipDestroy`
branch, so `--no-destroy` or disabled destruction walked straight past it: a
preflight/init/plan failure created the project and exited through the
`else if deployErr == nil` path without releasing it.

The three placements, in order: happy-path only (pass 22), destroy-branch only
(this pass), and now **outside every branch**, immediately before the result is
assembled. The project is created in one place, so it is released in one place —
placing it inside any branch is what kept producing this bug, and moving it again
would produce it again.

The condition is unchanged and remains the important part: delete only when
nothing of the run can still exist (account proven clean, or no state ever
written), otherwise keep it and say so, because the id is the handle to whatever
survived.
