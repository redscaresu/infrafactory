# Review pass 163 — S167

**15 findings, 13 accepted, 2 declined.** The load-bearing one: **the audit I wrote
to stop a convention drifting repeated the exact defect it forbade.**

## The rule was about the wrong thing

The first audit asked *"was `writeJSONError` called with an error?"*, and its own
comment claimed it covered "EVERY body-writer, not just the obvious one". That
sentence was false, and the falseness was the tell — the same false-explanation
class this arc has been closing all week, committed in the file written to prevent
it.

`writeJSON` is the bigger door. Error text reaches a response through it as a
**struct field**, which a function-shaped rule cannot see:

- `handlers_deployments.go:196` — `payload.Unreadable = append(..., e.Error())`,
  reproduced: `{"unreadable":["read deployment dep-locked: open
  /var/.../secret-live-dir/dep-locked.json: permission denied"]}`, rendered
  verbatim by the estate page **thirty lines below** yesterday's fix for the
  identical `*fs.PathError`.
- `handlers_pitfalls.go:165` — `ParseError: err.Error()`, on the pitfalls page.
- `internal/cli/live_service.go:301` — `Detail: deployErr.Error()` into an
  `ActionResult` the scenario page renders, **outside the audited package**: the
  boundary version of the same mistake.

And a local variable defeated it entirely: `msg := "read: " + err.Error()` then
`writeJSONError(w, status, msg)`.

The rule now asks whether the package touches an error's text **at all**, outside a
named allowlist carrying a reason per entry. There is no spelling of "put this
error in a response" that does not first call `.Error()` somewhere. Both escapes
verified by reintroducing them.

## My own regressions, accepted

- **The scenario editor stopped showing YAML syntax errors.** A reader got "see the
  server log" for their own typo, on the page that exists to fix it, with no access
  to the log. `extractYAMLSyntaxDetail` already solved this twelve lines away.
- **The 403 traversal answer lost its specific reason.** `resolveScenarioFile`
  refuses four ways and three are literals it composed; only the one wrapping an
  `*fs.PathError` needs withholding, now via a sentinel rather than all four.
- **A 500 in `handleRunBundle` reached neither client nor log** — worse than the
  leak, and invisible to the audit because there is no error text to find.
- **`ErrNoSuchScenario` stopped logging** when I composed its message, alone among
  its neighbours.
- **The doc comment was glued to its neighbour**, so `writeInternalError` had none. **Recorded as fixed here and was not** — a dashed separator inside one comment block is still one comment block. Actually fixed in round 164, which also caught that the sentinel I added had displaced `extractYAMLSyntaxDetail`'s doc the same way.
- **The count appeared as 43, 61 and 65** across my own files. It is 65.

Also taken rather than argued: `strconv`/`token.Position` instead of a hand-rolled
`itoa`; `state.writeInternalError(w, status, msg, err)` as a method — four
parameters instead of five, matching the existing `logDetail` idiom, at 60 call
sites; and the redundant `Sprintf` branch removed, which additionally panicked on a
zero-argument call.

## Declined (2)

**Pin `writeRequestError` to errors originating from `r.Body`.** The finding is
right that it is an unenforced escape hatch. But the proposed rule — trace the
error's origin to a call whose receiver chain reaches `r.Body` — is a dataflow
analysis in a test whose value is being cheap. The surface is five call sites and a
doc comment; the allowlist already makes each exception review-visible. Revisit if
it grows.

**`unreadable` should name each record.** It reads better and it drops any error the
store reported *without* a matching record. The filesystem store always pairs them;
`DeploymentLister` is an interface and need not. A record nobody can read going
unmentioned is what the field exists to prevent, so the count is preserved and the
ids stay visible as rows with `unreadable: true`.

## The correction worth keeping

I called this a leak without qualification — in STATUS, the PR body, the ADR and to
the user. The server binds `127.0.0.1` and answers loopback origins only
(ADR-0026): the reader is the operator looking at their own paths. It is a
CLAUDE.md violation and a usability defect, plus defence in depth since `--addr`
overrides the binding. **It is not a remote-disclosure vulnerability**, and the
archive should not imply it was.
