# Stories

Open work, one file per item. A file's front matter says whether it can be picked up:

    status: ready | blocked | later

`ready` stories are the queue, and each one is written to be handed to an agent as its brief:
what to change, and **Done when** — the acceptance. The PR that finishes a story deletes its
file; the PR is the record. List them with `grep -H '^status:' docs/stories/*.md`.

`kind` (code | docs | chore | verify | lead | operator) and `risk: high` choose who builds it
and with which model (swarm-dev `docs/method.md` § Model and effort); `lead` and `operator` stories are
never given to a swarm agent.

A story may belong to an epic (`epic: <slug>`, see `docs/epics/`) and list the files it
`touches`, which is how a wave avoids two agents editing the same file. A story whose work lands
in a sibling repo says so with `repo: <name>` (`fakeaws`, `mockway`, ...): its agent builds in a
worktree of `../<name>`, and the lead deletes the story file once that PR merges.

Running several at once: `docs/operations.md` § The swarm. In Obsidian, `docs/` is
the vault and `docs/Board.base` is the board.
