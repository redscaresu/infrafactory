# Codex review pass 97 — S156d

One finding, accepted. **It is pass 85's finding again, in a new place.**

## [P2] The remedy was not part of the remedy's identity

`repairKey` was `repair + cloud + symptom`. Two upgrades can clear the same
normalized `HTTP 503` on the same resource by different changes — one adds a
health check, another corrects a port — and those are two lessons. Keyed on the
symptom alone, the second silently refreshed over the first and the corpus kept
whichever happened to be learned last.

Pass 85 said the same thing about descriptive entries: *the persisted identity
was narrower than the thing it identifies.* S156c ended by making
`Candidate.Key()` derive identity from the gate that defines it, precisely so
this could not recur — and it recurred one slice later, in a key written by hand
in a different file.

The remedy now enters the key as a digest of the **extractor's** rule, taken
before the evidence wrapper goes on: the extracted text is derived
deterministically from the two configurations, while the wrapped text states
counts that grow. The changed address stays in the clear so a human reading the
YAML can see what the digest refers to.

## The test was wrong, not the code

`TestLearnKeepsTwoDifferentRemediesForTheSameSymptom` first asserted two corpus
entries and got three. The third was correct: both deployments reported the same
symptom, so the **descriptive** path promoted it on breadth as it should. The
assertion now counts remedies specifically.

Worth noting because the reflex on a red test is to change the code.
