# S155b canary: an apply is not an upgrade, demonstrated

Run 2026-09-01 against real Scaleway, `fr-par-1`, ~€0.01.

The slice's whole claim is that **Terraform reaching its desired state does not
mean the service is running the new version**. That claim was unit-tested and
unproven. This run proved it — and not with a contrived fixture, but with an
ordinary property of the cloud.

## The run

| step | result |
|---|---|
| `deploy` v1 (`--tag 1.27`) | pass — LB serving at `212.47.241.224` |
| `live observe` | **pass** — *"is serving and confirms nginx:1.27"* |
| `live upgrade --from <v1.28> --tag 1.28` | **upgrade_verify: FAIL** |
| `live teardown` | pass — project deleted, sweep clean |
| account afterwards | 3 projects (all pre-existing), 1 server, 0 LBs |

## What actually happened

```
- live/upgrade_preflight: pass (confirms nginx:1.27 before the upgrade)
- sandbox_deploy/apply:   pass
- live/upgrade_verify:    fail
```

Confirmed by hand rather than taken from the verdict:

```
$ curl http://212.47.241.224/
nginx/1.27.0                       <- the service, still on the OLD version

$ grep nginx <workdir>/main.tf
echo "nginx/1.28.0" > ...          <- what tofu applied

$ grep nginx <workdir>/.infrafactory-previous/main.tf
echo "nginx/1.27.0" > ...          <- what it replaced
```

**The mechanism is mundane and that is the point.** Changing `user_data` on a
running Scaleway instance does not re-run cloud-init. Terraform updated the
resource, reported success, and the machine kept serving what it was already
serving. Nothing was wrong with the apply.

## What this says about every layer below it

That upgrade was **green everywhere else**:

- `sandbox_deploy/apply: pass`
- the orphan sweep would have reported the account clean, because it was
- a cost check would have reported nothing unusual, because nothing was
- the deployment record would have said `nginx:1.28`, because that is what was asked for

Only `upgrade_verify` disagreed, and only because it asked the service instead of
the record. This is the same shape as **D6** — a failure that every green signal
in the system agreed was fine — and the same lesson: *a check that asks the thing
itself is worth more than any number of checks that ask about it.*

## And the diff S156 wanted, produced by accident

The workdir now holds a **failed upgrade with both configurations preserved**:
the one that was running and the one that did not take effect. That is precisely
the before/after pair `ExtractFixPitfall` needs, and it arrived from a real
failure rather than a constructed one.

The lesson available here is genuinely prescriptive — *a user_data change alone
does not restart a Scaleway instance, so an upgrade expressed that way applies
cleanly and changes nothing* — which is the class of rule S156e has to produce to
call the loop closed.

## Cost of the slice, stated

Eight codex passes, fourteen findings, one real-cloud run. The findings are
catalogued in `docs/review-passes/pass50.md` through `pass58.md`; eleven of the
fourteen were one incomplete answer spread across six interacting invariants,
which is why S156 was cut into five slices before being built.
