# Codex review pass 112 — S162a

Three findings, all accepted, and two of them are the same one.

## [P2] `exposure: private` was reported as internet-facing, and billed a public IP

The schema says `exposure` is *"whether the load balancer has a public IP"*. The
preview treated **any** load balancer as internet-facing, and the estimator billed
a public IPv4 for it regardless.

Two consequences, and the second is worse than the arithmetic:

- The cost estimate lists a component the scenario says will not exist. A
  component list with things in it that are not real is one a reader learns to
  skim, which costs more than the €0.005/hour.
- **A warning that fires on everything is one people learn to skip.** The
  internet-facing line is the part of the confirmation people forget to ask about
  unprompted; making it always-true removes the only signal it carries.

Wrong in the *cautious* direction, which is why it would have survived a long
time.

Fixed at the type: `LoadBalancer.Public()` is a method rather than a comparison at
each call site, because the two callers must agree. A cost estimate billing a
public address while the confirmation calls the shape private would be two answers
to one question.

## [P2] `.yml` scenarios 500'd

`findScenarioPathByName` strips the extension and accepts **both** `.yaml` and
`.yml`; the preview appended `.yaml`. Every `.yml`-backed scenario would have
returned 500 — for a file the discovery walk deliberately supports.

The fix tries both suffixes, and deliberately does **not** swallow a parse error
while doing so: a file that exists and will not parse is a real error, and masking
it by trying the other spelling would report "not found" for a file that is
right there.
