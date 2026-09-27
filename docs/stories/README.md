# Stories

Open work, one file per item. A file's front matter says whether it can be picked up:

    status: ready | blocked | later

`ready` stories are the queue, and each one is written to be handed to an agent as its brief:
what to change, and **Done when** — the acceptance. The PR that finishes a story deletes its
file; the PR is the record. List them with `grep -H '^status:' docs/stories/*.md`.

Running several at once: `docs/operations.md` § Parallel agents (herdr). In Obsidian, open the
repository as a vault and add a Bases view over this folder grouped by `status`.
