---
kind: lead
status: blocked
blocked_by: [prod-read-seam-reader-describe-volumes, prod-read-seam-real-recording]
epic: fakeaws-loader
depends_on: [fakeaws-load-read-back, prod-read-seam-reader-describe-volumes, prod-read-seam-real-recording]
touches: []
risk: high
costs: money
---

# Lead: load the raw step-0 recording into a private in-process fakeaws and read it back, inside the recording's window (no cloud)

Blocked on prod-read-seam-real-recording **reaching its step 5b**, not on its merge: this runs as
that story's step 5b, after the scrub (step 5) and before step 6 deletes the raw recording. So no
raw read is kept longer, and none is re-recorded for this. prod-read-seam-real-recording's PR
records the result and deletes this file.

No cloud and no credentials: the read is already on disk. The user's steps for that read (the
go-ahead and the key file) are prod-read-seam-real-recording's **You:** lines.

**You:** only if prod-read-seam-real-recording has already run without step 5b: say go for a
fresh read under the €5 cap; the lead then files it as a new lead story.

**Lead:**
1. In a fakeaws checkout at the main holding fakeaws-load-read-back, run
   `FAKEAWS_LOAD_READ=<raw recording> go test ./handlers/ -run TestLoadReadBack -count=1`: in
   process, `:memory:`, loopback only.
2. Record in prod-read-seam-real-recording's PR only PASS, or the failing actions and paths, and
   the per-type loaded, absent and unresolved counts: no account id, ARN or value.
3. File a fakeaws story for each refusal or difference.
4. The raw recording is still deleted at step 6, on schedule.

**Done when:**
- prod-read-seam-real-recording's PR records TestLoadReadBack PASS on the raw recording, or names
  each refused action and path with the fakeaws story filed for it.
- After step 6 the raw recording is absent (ls), and no fakeaws db file was written: the run was
  `:memory:`.
- Grepping the PR body for 12-digit runs finds only 111122223333 and public image owner ids.
