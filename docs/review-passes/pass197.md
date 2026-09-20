# Review pass 197 — S190 probe window

`codex exec review --base main`, 2026-09-20.

> The change consistently updates the default and sample configuration for the real probe
> retry count, with accompanying documentation. I did not identify any actionable
> correctness issue introduced by this diff.

Clean. Two numbers and a comment; the work was the six real boots behind them, which a
diff review cannot see.

Worth recording what the measurement corrected, since it went both ways. I first told the
user the 120s window was "too tight" on the strength of ONE failure — an inference, stated
as a finding. The first measurement came back at 20s and contradicted it. Only the full
set (0, 0, 20, 45, 66, and one >120s failure) supported widening, and for a different
reason than the one I originally gave: not "boots are slow" but "the tail is long and the
probe exits early on success, so widening is nearly free".
