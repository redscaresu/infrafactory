# Review pass 159 — S163e, twenty-fifth round

**11 findings, 9 accepted, 2 declined.**

My first reading of this pass was that it showed the same one-for-one
regeneration rate as pass 155, with four defects traced to pass 158's own
fixes. `git log -S` on each finding says otherwise, and the correction is
the most useful thing in this file.

| # | Where it came from |
|---|---|
| 2 — path leak in the `os.ErrNotExist` branch | **S162b (#201)** — the original slice |
| 4 — absent field blamed on a failed read | round 10 |
| 7 — refresh failure in the "Saved" slot | round 13 |
| 1 — `finish()` overwrote the outcome | round 14 |
| 3 — pointer at a dismissible report | round 14 |
| 5 — heading contradicting its body | round 23 |
| 6 — stale `already_deploying` disable | **round 24** |
| 11 — `__connected: false` on disposal | **round 24** |
| 9 — ad-hoc `log.Printf` | **round 24** (design, not a defect) |

So pass 158 produced **two** behaviour defects, not four. Six of the nine
had survived between ten and nineteen prior rounds, and one predates the
review loop entirely. The loop is not chasing its own tail at the rate I
claimed; this pass went deeper than its predecessors and found things they
all walked past.

That is the finding worth keeping, and it cuts against "one clean pass"
as a correctness signal. A clean pass means one sampling turned up
nothing — the same sampling that missed a raw `err.Error()` path leak
nineteen times, twenty lines below a branch with a test asserting exactly
that leak must not happen.

## Accepted (9)

| # | Finding | Fix |
|---|---------|-----|
| 1 | `finish()` overwrote every `mayHaveCreated` outcome with the flat "this deploy did not finish cleanly", asserting a negative the page had not observed — the dropped-connection case concludes *unknown*, and its own message says the apply may still be running | the ending uses `outcome.message` verbatim |
| 3 | the ending pointed at a layout report the reader can dismiss, and nothing cleared the pointer — prose directing them to a banner that was gone | pointer deleted along with the overwrite; the ending now stands on its own |
| 2 | the `os.ErrNotExist` branch wrote `err.Error()` into the body — the exact `*fs.PathError` path leak the branch 20 lines above suppresses, with a test | stable sentence + logged detail; `TestAMissingFileIsReportedWithoutItsPath` |
| 11 | `releaseSocket` writing `__connected: false` made the reconnect window render as a lost connection — the mirror image of the stale-`true` bug pass 158 fixed | three states: absent = no claim, `true` = open, `false` = had one and lost it. New `isDisconnected` |
| 6 | `already_deploying` is a dialog-open snapshot; disabling the confirm button on it left the button dead for the dialog's life once the other apply finished | disable removed. The comment justifying it was **wrong on its own facts** — `claim` is taken before anything touches the cloud, so the refusal is milliseconds, not "minutes of output" |
| 4 | `alreadyLiveWarnings` blamed a failed estate read when the field was merely ABSENT | `ESTATE_NOT_REPORTED` — same conclusion, honest cause |
| 5 | the estate banner's bold "Applying now" heading contradicted its own body ("whether it is still applying is unknown") | heading follows the body's tense |
| 7 | a failed post-save refresh was routed into `status`, rendering grey in the "Saved" slot and replacing it | its own rose `refreshError` slot |
| 9 | `log.Printf` was the first ad-hoc stdlib logging in `internal/api`, and the only surviving copy of detail the response withholds | `ServerConfig.Logf` seam + `TestWithheldDetailIsLogged` |

## Declined (2)

**8 — the `os.ErrNotExist` branch is unreachable for `LiveDeployer`.** Half
accepted: the comment *was* false and is rewritten. The branch stays.
`DeploymentDeployer` is an interface this package does not own, a bare
`os.ErrNotExist` says nothing about *when* a file went missing, and the
alternative is a handler that cannot answer an implementation's error at
all. It is now documented as an interface contract rather than an observed
path.

**10 — `estateSummary` is ~45 lines with a sentence duplicated across two
arms.** Duplication and length only; no wrong output is produced, and the
two copies differ in lead-in because they sit in different sentences.
Extracting a helper would couple two arms that are allowed to diverge.
Declined as a nit under the standing "one clean pass, nits do not count"
rule.

## The thing worth recording

Both of pass 158's two regressions were **mirror images of the fix that
produced them**: curing "the flag reads a stale `true`" produced "the flag
reads false while connecting", and curing "the reader watches a foreign
apply's output" produced "the button never re-enables". Both times the fix
added a guard instead of removing the thing that needed guarding — the
same move the pass-155 simplification was supposed to have retired.

Finding 6's comment is the sharpest instance. It justified a guard with a
cost — "minutes of output attributed to a deploy that never started" — that
the server's own ordering makes impossible, because `claim` is taken before
anything touches the cloud. The guard was reasoned about, not measured, and
it cost a button that never re-enabled.
