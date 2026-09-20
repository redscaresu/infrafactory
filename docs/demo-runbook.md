# Demo runbook — driving infrafactory live from the UI

The commands, in order. One line each. Everything below the fold is reference
material for when something goes wrong.

Measured, not estimated: `make build` is 8.5s, a deploy ~70s, a full run 2–5
minutes because the LLM phases vary and the repair loop may take a second pass.

---

## From a fresh clone

Only needed on a machine that has never built this. `make build` pulls
`npm install` in behind it, which is the slow part — minutes, not the 8.5
seconds a warm build takes. Do it well before the day.

```bash
# go, node/npm, tofu, and the scw CLI must be on PATH
git clone https://github.com/redscaresu/infrafactory && cd infrafactory

# npm install + vite build + three Go binaries into bin/
make build

# mockway lives NEXT TO this repo, not inside it
cd .. && git clone https://github.com/redscaresu/mockway && cd infrafactory
```

**mockway is the only mock this demo needs.** The mock-state client is a router
keyed by the scenario's `cloud`, so a Scaleway scenario never contacts fakegcp,
fakeaws or fakegenesys. `make mocks-up` starts the whole family and wants all
four clones plus Docker for SeaweedFS; `make mockway-up` starts the one.

Credentials are not in the repo: `~/.config/infrafactory/layer3.env` holds the
Scaleway key, and is not committed.

There is also a `~/.config/infrafactory/layer3-on.yaml`, which is the repo's
`infrafactory.yaml` with one line changed — `sandbox_deploy.enabled: true`. The
demo does **not** need it. `--allow-layer3` overwrites that exact field at
startup, deliberately: a config file is not allowed to decide that a UI may
spend money (ADR-0026). Only `reap` needs the file, because it has no such
flag.

---

## Green room — before you go on

```bash
cd ~/go/src/github.com/redscaresu/infrafactory

# never switch branches after this point
git checkout main && git pull

# the only mock this demo needs, started in YOUR terminal
# (anything started from an agent session dies with it)
make mockway-up

# real Scaleway credentials
set -a; source ~/.config/infrafactory/layer3.env; set +a

# must list ONLY: default, openclaw, infrafactory
scw account project list
```

Then one full rehearsal run, and kill the UI afterwards so the first command on
stage starts cold. The rehearsal leaves HCL in `output/web-live-paris/`, which
Deploy needs — `deploy` does not generate.

---

## On stage

```bash
# 1 — build the UI and both binaries. 8.5s.
make build

# 2 — credentials into this shell
set -a; source ~/.config/infrafactory/layer3.env; set +a

# 3 — start it. The three flags are the consent; no --config needed, because
#     --allow-layer3 overwrites sandbox_deploy.enabled at startup anyway.
./bin/infrafactory ui --allow-deploy --allow-teardown --allow-layer3
```

**4 — in the browser:** http://127.0.0.1:4173 → `web-live-paris` → **Run**,
all boxes unticked. *Real Scaleway: apply, probe, destroy, sweep. 2–5 min.*

**5 — same page:** **Deploy**. *Applies the same HCL and leaves it up under the
scenario's 4h TTL.* Open the load balancer URL — that is the nginx page.

*Or, instead of 4 and 5:* tick **Keep it running (`--keep`)** before Run. One pass
— generate, mock, apply, probe — and it stops before the destroy, registering what
it left as a live deployment on the same 4h TTL. Same nginx page, one button, and
`live ls` finds it. Slower than Deploy and it can fail on the way; Deploy is the
safe path if the clock is tight.

The UI holds that terminal. **Open a second one** for what follows, and run the
`set -a; source …` line in it first.

```bash
# 6 — the live deployment and its remaining TTL
./bin/infrafactory live ls

# 7 — destroy it, sweep the account, release the record
./bin/infrafactory live teardown <id>

# 8 — prove it. Only default, openclaw, infrafactory.
scw account project list
```

---

## The three things that will bite

- **Do not tick "Keep state (`--no-destroy`)".** It is the wrong one, and it sits
  next to the right one. It leaves real infrastructure with **nothing tracking
  it**: no record, no TTL, nothing `live reap` will find. The box you want is
  **"Keep it running (`--keep`)"**, which registers what it leaves.
- **Do not switch git branches while anything runs.** Policies and pitfalls are
  read from the working tree; a switch silently changes what the generator is
  told.
- **Do not start the mocks from an agent session.** They die with it.

---

## What to say at each stage

Optional. The commands above are the runbook; this is the narration.

| stage | the line |
|---|---|
| `make build` | "Eight seconds. That's the UI and the CLI." |
| the three flags | "Real-cloud apply is decided here, by the person typing the command. The config file isn't allowed to turn it on." |
| `allowlist` | "Refused before anything is created. A refusal costs nothing." |
| `real_probe` | "HTTP 200 through a real load balancer. Not a plan assertion." |
| `orphan_sweep` | "That's the run checking the account, not trusting its own destroy." |
| Deploy vs Run | "Run proves a change is safe and destroys it. Deploy keeps it. Two buttons, deliberately." |
| Keep it running | "Same run, minus the destroy — and it registers what it left, so it still has a deadline and a teardown command. Keeping something untracked is the failure mode, not the feature." |
| the nginx page | "Nobody wrote that Terraform." |
| teardown | "And it's gone. The account is empty, and the run checked." |

---

## Timing

| | |
|---|---|
| build + start | 30s |
| **Run** | **2–5 min** ⚠️ the variable one |
| Deploy + nginx | 90s |
| `live ls` + teardown | 2 min |

**~4 minutes without the Run, up to 8 with it.** Decide before you are up
there. If it has to go, show a recording and start at Deploy — you still end
with nginx on screen and the account provably empty.

---

## Recovery — if a run leaves infrastructure behind

The supported path first:

```bash
# reap is the ONE command here that needs the config: unlike the UI it has
# no --allow-layer3 flag, so it reads sandbox_deploy.enabled from the file.
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
