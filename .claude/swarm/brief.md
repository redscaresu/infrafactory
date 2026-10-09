infrafactory rules: do not source ~/.config/infrafactory/*.env, and do not run deploy or anything
with sandbox_deploy enabled. Title your commit and PR with a plain description of the change and no
slice number (S###): those belong to the lead. If you touch internal/cli without a real decision,
put `ADR: none — <reason>` on your final commit; doc hygiene reads it from the tip only, so repeat
it in any merge commit's message. End commit messages with
"Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>" and PR bodies with
"🤖 Generated with [Claude Code](https://claude.com/claude-code)".
Never press GitHub's "Update branch": its merge commit has no `ADR:` trailer and fails doc
hygiene. To catch up with the base, `git merge origin/main` locally and repeat the trailer (a
reason of at least 10 characters) in the merge commit's message.
