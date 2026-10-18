---
kind: chore
status: ready
depends_on: []
touches: [.github/workflows/release.yml]
---

# Re-running release.yml never edits an already-published release

Found in review of #611, which publishes in one job after every binary builds. A re-run (or a
second push of the same tag) against a release that is already published would replace its
assets. Before uploading, the publish job reads the release for the tag. If one exists and is not
a draft, the job fails with a message naming the tag, and changes nothing.

**Done when:**
- The publish step checks `isDraft` (or the REST `draft` field) and exits non-zero on a published release. The PR body shows the step's output from a `workflow_dispatch` run against a throwaway published pre-release, which is deleted afterwards along with its tag.
- A run against a new tag still publishes as before (the same dry-run path #611 used, as a draft, deleted afterwards).
