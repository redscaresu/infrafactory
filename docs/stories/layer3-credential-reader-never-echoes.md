---
kind: code
status: ready
depends_on: []
touches: [internal/harness/aws_sealed_env.go, internal/harness/aws_sealed_env_test.go]
risk: high
---

# The Layer 3 credential reader never echoes a line it cannot parse, and reads the file it checked

Found by /code-review on #469 (prod-read-seam's reader): the Layer 3 reader `readAWSCredentialFile`
(internal/harness/aws_sealed_env.go:~85) puts an unknown key in its error with `%q`. A secret that
wraps onto a second line, or a line written VALUE=KEY, has its left side printed: a fragment of the
key reaches the terminal and any log. It also stats the path and then reads it by name (an Lstat →
ReadFile race), where the new reader opens once with O_NOFOLLOW and stats and reads that handle.

Bring it to the new reader's rules (internal/harness/aws_reader_credentials.go), keeping its
"exactly two keys" contract and every existing test unchanged in meaning:
- echo an unknown key only when it matches `^[A-Z][A-Z0-9_]*$`; otherwise "line N has an
  unrecognised key", with nothing from the line;
- open once (O_NOFOLLOW, O_NONBLOCK), refuse anything that is not a regular file of mode 0600,
  and read the same handle. Share the core with the new reader only if that stays smaller than two
  copies; otherwise mirror it.

**Done when:**
- Refusal rows for a wrapped-secret continuation line (`<distinctive>==`) and a reversed line
  (`<distinctive>=AWS_SECRET_ACCESS_KEY`) each assert the planted fragment is absent from the error.
- A symlink, a FIFO and a 0644 file are refused; a test swaps the file after the check cannot make
  it read another file.
- Every existing aws_sealed_env test passes with no assertion line changed.
- Each new test is shown failing by a cp-backup break of the code it guards, recorded in the PR.
