# Epics

An epic is a goal bigger than one PR. It is scoped into stories (`docs/stories/`), each one PR,
and the epic is done when its last story merges; that PR deletes the epic file too.

```
---
status: active | later
---

# <Goal, as a sentence>

**Done when:** how we will know the goal is met — observable, not "stories merged".
**Out of scope:** what this epic will not do.
**Constraints:** ADRs, safety rules and budgets that bound it.
```

A story joins an epic with `epic: <epic-file-name-without-.md>` in its front matter. Scope a new
epic with `/plan-epic <slug>` (`docs/operations.md` § Scoping an epic). In Obsidian,
`docs/Board.base` shows the board by status and by epic.
