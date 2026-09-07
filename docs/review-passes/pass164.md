# Review pass 164 — S167, third round

**15 findings, 13 accepted, 2 declined.** Three of them contradict claims I had
already written into STATUS, the ADR, a review archive and a message to the user.

## The rule still did not do what it said

**`fmt.Sprintf("...%v", err)` was not caught.** Verified by reintroducing it: the
audit stays green. The rule's own comment asserted "there is no spelling of 'put
this error in a response' that does not first call `.Error()` somewhere", and `%v`
on an error is exactly that spelling — one of the shapes actually present before
the sweep, and still live at `handlers_deploy_preview.go:283,294`.

The cause is precise and worth recording. Round 163 said the `Sprintf` branch was
**redundant**, and it was — of the implementation it reviewed, which also walked
for bare error identifiers. I then rewrote the rule to walk only for `.Error()`
calls *and* deleted the branch, applying a finding to code it was no longer true
of. Accepting a review point without re-checking it against the redesign put the
hole back.

## Two changes of mine built on false premises

**The pitfalls parse error.** I withheld it, writing that "the YAML error names the
file it was read from". It does not: `yaml.Unmarshal` receives bytes and has no
filename, so the text is `yaml: line 2: mapping values are not allowed in this
context`. Verified by running it. I stripped the only useful fact from the page
that exists to edit that file — the identical regression I had caught and reverted
for the scenario editor twelve lines earlier in the same diff. Reverted.

**The deploy failure detail.** I withheld it, writing that the cause "is on the
command's stderr". It is not: `runDeployCommand` is called directly rather than
through `cmd.Execute()`, and cobra only prints a returned error on the latter. So
the most useful errors on that path — `scenario %q declares no service: block ...
Use infrafactory run for infrastructure-only scenarios` — reached nobody. I hit
that exact refusal during the S164 canary and was told only "see the server log".
`*CLIError` text is composed by this codebase and is now shown.

## The rest, accepted

- `scenario.ErrInvalidScenario` (moving tags, over-long TTLs) was withheld from the
  editor — caller-fixable rules, and `deployPreviewHandler` already shows the same
  text as `preview.Reason`. Now shown.
- `resolveScenarioFile` has **seven** error returns, not the four I wrote. Two wrap
  `filepath.Abs`/`filepath.Rel` and were still echoed verbatim at the 403 — on the
  one path this file's allowlist blesses. Marked with the sentinel.
- The sentinel's message equalled the client-facing sentence, so the log read
  `that scenario path is not allowed: that scenario path is not allowed: <cause>`.
  Renamed. And it answered **403** for a server-side failure, blaming the caller
  for a fault they cannot fix — now 500.
- The `server.go` doc comment was **still** glued; pass163 recorded it as fixed and
  it was not. A dashed separator inside one comment block is still one block. My
  sentinel had displaced `extractYAMLSyntaxDetail`'s doc the same way, taking with
  it the warning against `strings.LastIndex`.
- The `unreadable` list was N copies of one sentence under a heading that already
  said N. It carries the ids now, taken from the paired `Undecodable` records, with
  the count still driven by the errors so nothing can go unmentioned.
- Four sites hand-rolled `writeInternalError`'s body next to the helper introduced
  for it.
- The runtime test `chmod 0000`s a directory and asserts something fails; as root
  or on Windows nothing does, and it failed pointing at a logging bug that does not
  exist. It probes and skips now.
- `STATUS.md`'s "Last updated" predated its own newest entry.

## Declined (2)

**`render` should be `types.ExprString`.** True, and the package-scope name is a
real collision risk. Both are in a test helper whose output is a diagnostic string;
the change is churn against a file that has now been rewritten twice in two rounds.
Worth doing when it is next touched for a reason.

**Pin `writeRequestError` to `r.Body`-derived errors.** Re-raised, and stronger now
that the `%v` hole is known — the argument that the `.Error()` rule was airtight was
wrong. Still declined: the enforcement needs dataflow analysis in a test whose value
is being cheap, the surface is five call sites, and the allowlist makes each
exception review-visible. Recorded as the known boundary rather than pretended away.

## What this round is about

Three rounds, and each one found the previous round's *justification* false rather
than its code broken. A rule that is wrong fails once; a rule that is wrong and
carries a confident explanation of why it is right fails until somebody checks the
explanation. All three were checkable in under two minutes — run `yaml.Unmarshal`,
read `cmd.Execute`, reintroduce a `%v`.
