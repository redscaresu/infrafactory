# Review — policies/aws/default_deny_ingress.rego

Named for the story rather than numbered: pass numbers are a shared counter, and
parallel agents picking the next one collide.

## Cases no capture produces

Every fixture under `internal/harness/testdata/ingress/aws/` comes from
`capture.sh`. These cases are not among them, and none is hand-shaped into one:

- **A wholly unknown inline rule rendered as `null`.** jsonplan writes an unknown
  element that way, and `as_object` turns one into a rule with every field unknown,
  which denies. No plan yields it: `ingress = [unknown ? a : b]` plans as
  `after_unknown.ingress = true` for the whole set (probed against 5.100.0), which the
  unknown-list rule handles.
- **A whole inline set collapsing because one rule holds an unknown reference.** The
  story allowed for it; the captures refute it. 5.100.0 plans an unknown reference,
  a dynamic block over an unknown value and a dynamic block over an unknown `for_each`
  as that rule's unknown fields (`open_dynamic`, `open_dynamic_collection`,
  `open_attribute`). Only an `ingress` list unknown as a whole collapses
  (`open_attribute_collection`), and then the configuration holds `references`, never
  constants, since a constant list is known. So the fallback reads whether an
  `ingress` expression exists, not its constants.
- **A collapsed `ingress` whose configuration is not found.** A real plan always
  carries it. `test_undeclared_ingress_not_found_denies` trims a captured plan's
  configuration to show the lookup failing closed; an instance key containing `]` is
  the only way a real plan could miss it.
- **A delete-only change with a live `after`.** A delete plans `after` as null, so
  nothing is walked with or without the `actions != ["delete"]` filter; the filter
  stays because the story names it, with no test.
- **A tcp `aws_vpc_security_group_ingress_rule` with null ports.** Null ports are
  known, not unknown, so it passes: no port it names covers 22. What EC2 does with
  such a rule is not checked here.

## Known over-denial

- A dynamic `ingress` over a `for_each` unknown at plan is denied even when every
  rule it would write is private: the plan holds one rule with every field unknown.
- `ports 0-0` is denied for any protocol, as the story specifies, so public ICMP
  type 0 code 0 is denied too. ICMP with no ports (`ping` in the fixtures) passes.
