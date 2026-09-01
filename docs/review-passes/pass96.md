# Codex review pass 96 — S156d

One finding, accepted after checking it rather than taking it on trust — six
passes in, a plausible-sounding finding deserves verification before another
round of changes.

## [P2] An upgrade having happened is not an upgrade having worked

`applyRan()` returns true for a **failed** apply, provided it got past init and
plan. That is deliberate and correct: a failed apply may still have created
resources, and a record that ignored it would describe infrastructure that no
longer exists. So `live upgrade` sets `UpgradedAt` on a partial upgrade too.

`Repairs` read `UpgradedAt` as "the new configuration is what is running". After a
partial apply it is not — the running infrastructure is some mixture of the two,
so diffing previous against current describes a change **that was never fully
made**. Pair that with two healthy probes and the corpus gains a remedy crediting
the recovery to HCL that was never applied.

Verified before accepting: `applyRan` at `live_upgrade.go:505` returns true for
any `SandboxDeployError` whose stage is neither `init` nor `plan`.

Records now carry `upgrade_succeeded`, set only when the apply returned no error.
False by default, so records written before it existed are declined — the
intended direction.

## Six passes is the finding

Nine accepted findings. Not one was a coding slip; every one was a rule the
surrounding system already enforced that this path did not inherit. That is what
a slice too large for one question looks like, and S156d had at least four
questions in it: when is an upgrade a repair, what does the diff attribute to,
which observations count, and what counts as an upgrade at all.

The plan for S156 already learned this once — S155b's seven passes were what
split S156 into five slices. The split was not fine enough, and the signal was
available before the code was written: this slice's plan entry names *"is the
before/after pair a usable diff?"* as its one question, and the pair turned out
not to be the hard part at all.
