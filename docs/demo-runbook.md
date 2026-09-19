# Demo runbook — driving infrafactory live from the UI

For the conference talk, slide **"A Mock Is Not Production"** onwards. Six acts:
build it, start it, run `web-live-paris` through both layers, deploy it, look at
an nginx page served by infrastructure nobody wrote, read the logs, tear it down.

Everything here has been executed end to end against real Scaleway. Numbers are
measured, not estimated; where something is variable it says so.

---

## Before you go on — green room, ~10 minutes

The slow and variable work happens here. Nothing in this section is worth
watching.

```bash
cd ~/go/src/github.com/redscaresu/infrafactory
git checkout main && git pull
make mocks-up
set -a; source ~/.config/infrafactory/layer3.env; set +a
scw account project list
```

`scw account project list` must show **exactly** `default`, `openclaw`,
`infrafactory`. Anything named `if-run-*` is litter from an earlier run — clean
it before you go anywhere near a stage (see "Recovery" at the bottom).

Then do **one full rehearsal run**. It warms the Go module cache, proves the
credentials work, and — the part that matters — leaves valid HCL in
`output/web-live-paris/`, which Act 4's deploy needs. `deploy` does not
generate.

Kill the UI afterwards, so Act 1 starts genuinely cold.

> **Run everything in your own terminal.** Processes started from an agent
> session belong to its process group and die with it.

---

## Act 1 — build it · ~20s

```bash
make build
```

**8.5 seconds warm.** That is the SvelteKit UI and both Go binaries.

> "Eight seconds. That's the whole thing — the UI and the CLI."

---

## Act 2 — start it · ~10s

```bash
set -a; source ~/.config/infrafactory/layer3.env; set +a
./bin/infrafactory --config ~/.config/infrafactory/layer3-on.yaml \
  ui --allow-deploy --allow-teardown --allow-layer3
```

Open **http://127.0.0.1:4173**.

Point at the banner: **Layer 3 (Real Scaleway): enabled**.

> "Three flags. Real-cloud apply is decided here, by the person typing the
> command in the shell that already holds the credentials — and nowhere else.
> The config file is not allowed to turn it on, which is the stricter half of
> the rule: nobody re-reads a config at the moment they start a server."

(ADR-0026. `--allow-deploy` is deliberately independent of `--allow-layer3`:
an ephemeral apply the run destroys, and one that persists, are different kinds
of harm — ADR-0027.)

---

## Act 3 — Run · 2–5 minutes ⚠️ the variable one

`web-live-paris` → **Run**. Leave **both checkboxes unticked**.

Talk over the stage list as it fills. This is the slide's `plan → apply →
destroy → sweep` actually happening:

```
generate → validate → mock_deploy → destruction
  → sandbox_deploy: allowlist → run_project → init → plan → apply
       → real_probe → private_nic_detach → destroy
       → auto_created_purge → orphan_sweep → run_project_delete
  → live/stray_run_projects
```

Things worth saying while it runs:

- **`allowlist` comes before `run_project`.** Refusal costs nothing because
  nothing has been created yet.
- **`real_probe`** is a real HTTP request through a real load balancer. Not a
  plan assertion.
- **`orphan_sweep`** is the run checking the account rather than trusting its
  own destroy.
- **`live/stray_run_projects`** is the run checking for litter left by *earlier*
  runs, and failing if it finds any.

> "It destroyed everything it made, and then went and checked."

### The risk, and how to play it

The LLM phases take 40–60s each with genuine variance, and the repair loop may
take a second iteration. On 2026-09-10 run `20260910T214848Z` did exactly that:
iteration 1 was refused by `vpc_required`, the refusal named the fix, iteration 2
complied, `target_reached`. Nothing was created during iteration 1 — `validate`
runs on the plan.

**A second iteration is the best thing that can happen to this demo.** It is the
loop closing in front of the room. Have the line ready:

> "That's the policy refusing it — and the refusal says what to write instead.
> Watch what the next iteration does."

**If you do not have five minutes, skip this act**, show a recording, and go
straight to Act 4.

---

## Act 4 — Deploy, and open the page · ~70s

Same scenario page → **Deploy** → confirm.

> "Run proves a change is safe and then destroys it. Deploy keeps it. Two
> buttons, deliberately not next to each other."

It applies the HCL the run just generated and leaves it up under the scenario's
mandatory **`ttl: 4h`**. The deployment record carries the load balancer URL.

**Open it.** That is nginx, in a container the model chose, behind a real
Scaleway load balancer, on infrastructure no human wrote.

If someone asks whether the VM is reachable directly: **yes, it is.** The
generated stack gives the instance a public IP and the load balancer forwards to
that public address, so there are two routes in. The private network exists
because `vpc_required` demands it, not because it carries traffic. Say so; it is
a better answer than implying isolation the stack does not have.

---

## Act 5 — the logs · ~60s

Two places. The second is the better one.

**The Live Run panel** — the event stream is already on screen:

```json
{"event":"layer3_private_nic_detach","status":"success","detail":"removed 1 private NIC ..."}
{"event":"sandbox_deploy_progress","stage":"apply","detail":"apply: done in 1m7s"}
```

**The Deployments page**, or a terminal:

```bash
./bin/infrafactory --config ~/.config/infrafactory/layer3-on.yaml live ls
```

> "It has a TTL because ADR-0024 says every live deployment must. There is no
> value meaning 'forever'."

---

## Act 6 — tear it down, on stage · ~60s

```bash
./bin/infrafactory --config ~/.config/infrafactory/layer3-on.yaml live ls
./bin/infrafactory --config ~/.config/infrafactory/layer3-on.yaml live teardown <id>
```

Do not skip this. Destroying it in front of the room, and watching the sweep
report the account clean, is the strongest version of the closing claim — and
it costs a minute.

---

## Timing

| act | time |
|---|---|
| 1 · build | 20s |
| 2 · start | 10s |
| **3 · Run** | **2–5 min** ⚠️ |
| 4 · Deploy + nginx | 90s |
| 5 · logs | 60s |
| 6 · teardown | 60s |

**Without Act 3: ~4 minutes. With it: up to 8.** Decide before you are up there.

The one number nobody can give you is how long *you* take talking over it. Run
all six acts back to back against a stopwatch at least once.

---

## Things that will bite

**Do not tick "Keep state" (`--no-destroy`).** It looks like the way to keep
nginx alive for the audience. It is not: the run can no longer satisfy
`destruction: no_orphans` so it reports failure, and **nothing reaps it** —
`service.ttl` binds `deploy`, not `run`. Deploy is the supported path and it has
the TTL.

**Do not switch git branches while anything is running.** `paths.policies` and
`paths.pitfalls` are read from the working tree at run time, so a branch switch
silently changes what the generator is told and what the plan is judged against.
On 2026-09-10 a switch 33 seconds before a run produced a confident and entirely
wrong diagnosis. Runs now record their commit in `run.json`, which is how that
was caught.

**Do not start the mocks from an agent session.** They die with it.

**Check the account afterwards, every time:**

```bash
scw account project list
```

Since #234 a Layer 3 run performs this reconciliation itself and fails if it
leaves anything behind. Check anyway — it costs three seconds.

---

## Recovery — if a run leaves infrastructure behind

The supported path first:

```bash
./bin/infrafactory --config ~/.config/infrafactory/layer3-on.yaml \
  reap scenarios/training/web-live-paris.yaml
```

`reap` needs the live state file. If the next iteration has already overwritten
it, go by project instead:

```bash
scw account project list                      # find the if-run-* project
scw instance server list        zone=fr-par-1 project-id=<P>
scw lb lb              list     zone=fr-par-1 project-id=<P>
```

Delete servers first (`with-volumes=all with-ip=true`), then the private
network, the security group and the VPC, then the project. Two of those are
**auto-created by Scaleway** and Terraform never owns them — the
`Default security group` and the default VPC — so they block the project delete
until removed by hand.

The project delete can also need several attempts over ~2 minutes: the API
answers `an unexpected error happened when checking if project has resources,
please retry later` while its own consistency check catches up. Retry; it
clears.

---

## There is no PR gate any more

A `pull_request_target` workflow used to do this on a labelled pull request,
applying `block-paris` (no compute, ~9s) and posting the stage list back as a
comment. It was **removed on 2026-09-19**.

Two reasons, and both matter on stage. A screenshot of a green check is the
weakest evidence in the room — it is the easiest thing in the world to fake,
and an audience of platform engineers knows that. And a `pull_request_target`
workflow grants secrets to a pull request's context, which is a standing
security surface to carry for a capability that is better demonstrated live.

So if a slide still shows `Layer 3 gate: pass … 9s`, replace it. The numbers
do not describe this demo either: that was a scenario with no compute.
`git log .github/workflows/layer3-gate.yml` has the workflow if it is ever
wanted back.

---

## Why the demo works at all — one paragraph of background

For two days every compute scenario failed teardown with
`Can't delete a private network interface attached to a server`. It is not
about power state, HCL shape or timing: provider 2.81.0 deletes private NICs
through Instance **v2alpha1**, and that endpoint refuses every NIC there is
(being "attached to a server" is what a NIC *is*). The **v1** route has no such
precondition. infrafactory removes NICs through v1 before `tofu destroy`
(ADR-0031), which is the `private_nic_detach` stage in the list above.

Three fixes were shipped and retracted before that — one of them "verified"
against mockway, which reported `7 added, 7 destroyed` because it did not model
the refusal. **A mock more permissive than reality does not merely miss a bug;
it certifies a wrong fix.** mockway models it now (mockway#28), which is why
Layer 2 needs the same detach.

If you want one sentence from the whole episode, that is the one.
