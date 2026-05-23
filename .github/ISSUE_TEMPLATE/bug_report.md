---
name: Bug report
about: Something broke. Help us reproduce it.
title: '[bug] '
labels: bug
assignees: ''
---

## Summary

<!-- One sentence describing what's wrong. -->

## Steps to reproduce

<!--
The smallest sequence of commands that reproduces the issue. Include the
scenario YAML if it's the trigger.
-->

```bash
# example
infrafactory run scenarios/training/web-app-paris.yaml
```

## Expected behavior

<!-- What did you expect to see? -->

## Actual behavior

<!-- What did you see instead? Paste the relevant log lines, not the full transcript. -->

## Environment

- InfraFactory commit: <!-- output of `git rev-parse --short HEAD` -->
- OS / arch: <!-- macOS arm64 / Linux amd64 / ... -->
- Go version: <!-- `go version` -->
- Node version (if UI bug): <!-- `node -v` -->
- OpenTofu version (if Layer 1/2 bug): <!-- `tofu version` -->
- Which mock is running: <!-- mockway / fakegcp / fakeaws / none -->

## Additional context

<!--
- Pre-existing on `main` or introduced by a recent commit? Bisect range if you have one.
- Layer where the failure surfaced (static / mock_deploy / sandbox_deploy / destruction).
- Run ID under `.infrafactory/runs/<scenario>/<run-id>/` if applicable.
-->
