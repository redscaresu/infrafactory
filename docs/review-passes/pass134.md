# Review pass 134 — S163c, mutation matrix run directly

Three fresh-context reviewers were launched for this slice and **all three died on
API connection errors**, one of them leaving a mutation applied in the working
tree. Rather than relaunch a fourth, the mutation matrix was run directly. That
loses independence of judgement and keeps the part that matters here, which is
mechanical: does a test fail when the code is broken?

| mutation | result |
|---|---|
| the lock never claims | **caught** |
| the lock is never released | **caught** |
| `InFlight` always returns empty | **caught** |
| the listing reports `null` instead of `[]` | **caught** |
| **claim moved BEFORE name resolution** | **MISSED** |
| removing the 409 branch in `deployHandler` | **caught** (found by the reviewer before it died) |

## The miss is a wrong claim, not a wrong behaviour

I wrote: *"Claimed AFTER resolution, so a typo cannot lock a name nothing will ever
deploy."* Moving the claim before resolution passes every test — because the
deferred release fires on the resolution failure too. **The lock is
self-correcting either way, and the ordering buys nothing.**

The comment asserted a safety property that does not exist. Both it and the test's
docstring now say what is actually true: the ordering is tidiness, the *release*
is what is load-bearing, and that is what the test catches.

This is mutation testing earning its keep in an unusual way — not by finding
broken code, but by disproving a claim *about* correct code. The code was fine;
the explanation next to it was false, and a false explanation is what the next
person will reason from.

## And a test that hung instead of failing

The first attempt at running this matrix appeared to hang. Two causes, both mine:

- `timeout` does not exist on macOS, so several "runs" never executed and reported
  nothing — I read empty output as a hang.
- `TestDeployerRefusesASecondDeployOfTheSameScenario` genuinely did deadlock under
  the no-claim mutation: its fake runtime factory blocked unconditionally, so the
  second `Deploy` waited on a release the test had not sent. Go's default timeout
  is ten minutes.

**A test that hangs on regression is barely better than one that passes** — CI
reports a timeout rather than a name, and nobody reads a timeout as "the lock is
gone". Only the first factory call blocks now, and the mutation fails in 0.5s.

## Process, recorded because it nearly cost something

While a reviewer was mutation-testing, I ran `git add -A` on the same tree and
staged its half-applied mutation. Caught before committing. **A reviewer that
mutates owns the working tree**: no `add -A`, no branch switches, no commits until
it hands back. The previous round's reviewer warned about exactly this from the
other side, and I noted it and then did it anyway.
