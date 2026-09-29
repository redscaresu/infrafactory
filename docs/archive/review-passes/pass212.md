# Review pass 212: an empty try() fallback in an IP field

`codex exec review --base main`, 2026-09-27. This file records the one finding that was declined.

## [P1] Evaluate pure-function try() fallbacks before accepting them (declined)

The Layer 3 gate evaluates a `try()` fallback with a nil context, so a fallback built from an
allowlisted function, such as `format("")` or `tostring("")`, fails to evaluate and is skipped,
even though it yields `""`.

The finding is accurate, and closing it would not change what the gate can promise. An
expression that becomes a non-IP only after apply needs no `try()` at all:

    server_ips = [replace(scaleway_instance_ip.web.address, "/.*/", "")]

is unknown at plan time, `""` at apply, and strands the stack exactly as run `20260927T171147Z`
did. Parsing the HCL cannot rule that out, and no set of rules about `try()` fallbacks can. Giving
the check an evaluation context would mean mapping every allowlisted function to an implementation,
and all that buys is a longer list of shapes that no generator writes.

So the check covers the mistake the generator actually made: a literal `""`, or a constant
collection holding one (pass 1's finding, accepted). Surviving a config that destroy cannot
evaluate is the teardown's job, which is the open story `teardown-survives-config-error`. The code
comment on `layer3UndestroyableProblems` now states this limit, so the check is not read as a
guarantee.
