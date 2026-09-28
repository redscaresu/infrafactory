# Review — AWS default_tags carry the run id

Named for the story rather than numbered: pass numbers are a shared counter, and
parallel agents picking the next one collide.

One finding partly declined across the loop; the rest are in the PR body.

## Partly declined: [P2] "Inspect actual tag assignments before refusing the run-id key"

The claim: `refuseAwsRunIDTag` refuses a `.tf` file that mentions `infrafactory-run-id`
anywhere, so a comment, output or explanatory string that echoes the key fails a valid
generation. Codex asked for an HCL parse that checks only `tags` keys.

Accepted for comments: the check lexes the file and skips comment tokens, since a model
echoing the prompt in a comment is the likely case and a comment cannot tag anything.

Declined for everything else. A resource's `tags` takes its value from an expression, so
the key reaches it through a `locals` map, a variable default, `merge()` or a module input
as easily as through a literal in the attribute. A check that reads only `tags` attributes
passes all of those, and each one lets the model choose which run a resource belongs to,
which is the property the refusal exists for. A false refusal costs one repair iteration,
and the message names the file and the key, so the model removes the mention; a false pass
mis-attributes a resource that sweep and reap later identify by this tag.
