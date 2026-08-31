# S165 review pass 22 — three findings, all acted on

### [P1] The lifecycle helpers were untracked

`run_project_lifecycle.go` and its test existed only in the working tree, so the
diff referenced `ensureRunProject`/`releaseRunProject` without defining them — a
clean checkout would not compile. An artifact of reviewing a working tree rather
than a commit, and exactly the kind of thing a human reader would miss. Files
added.

### [P1] A run project leaked when the apply never wrote state

The real defect. The delete was gated behind the destroy branch, which is itself
gated on `liveStateMayHoldResources`. An apply that fails at preflight, init or
plan writes no state, so the project was created and never deleted — on exactly
the runs most likely to be repeated. Empty projects are free and still
accumulate.

There is now one cleanup covering every exit from the sandbox block. It deletes
when nothing of the run can still exist — the account was proven clean, or no
state was ever written — and otherwise **keeps** the project and says so, because
the project id is the handle to whatever survived. Deleting it after a failed
destroy would discard the only pointer to the leak.

### [P2] The comment claimed `run` honoured the flag

Checked rather than accepted: `run` delegates its apply to `executeTest`, so the
flag *is* honoured there — the `sandboxCommandEnv` call in `run_command.go` is
the auto-destroy-on-failure path, not the apply. The finding's conclusion was
wrong but its instinct was right, because that same auto-destroy path is the one
case where a project is knowingly kept.

Both comments now say precisely what is and is not covered, including the gap,
rather than claiming blanket support.
