# Codex review pass 100 — S159a

One finding, accepted, and **the third instance of the same defect in this
slice.**

## [P2] The embedded record leaked year-1 upgrade timestamps

`deploymentJSON` embeds `livestore.Deployment`, whose `UpgradedAt` and
`UpgradeStartedAt` are `time.Time` with `omitempty` — a tag that does nothing on
a struct. A deployment that was never upgraded arrived carrying an upgrade date
in the year 1.

## Three findings, one defect, and I wrote the lesson down between two of them

- the **version label** would have rendered as `""` — the reason this slice
  exists;
- the **observation timestamp** as `0001-01-01` — pass 99;
- the **upgrade timestamps** as `0001-01-01` — this one.

Pass 99's note says, in as many words, that *"checking the field I was thinking
about and not its neighbours"* caused it. I then fixed the field I was thinking
about and not its neighbours.

## So it is closed as a class, not a field

`TestDeploymentPayloadNeverCarriesAYearOneTimestamp` marshals a deployment with
**every optional field unset** and asserts the string `0001-01-01` appears
nowhere in the payload. The next optional time added to the record would inherit
the defect silently; now it fails a test instead.

That is the narrow-audit lesson applied to a serialisation
bug: three fields fixed one at a time is a snapshot, and the class is what the
next person needs closed.
