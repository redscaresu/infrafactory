---
kind: code
status: ready
epic: rehearsal-prepare-gate
depends_on: [rehearsal-prepare-first-init, rehearsal-prepare-gate-modules]
touches: [internal/cli/rehearse_prepare.go, internal/cli/rehearse_prepare_init.go, internal/cli/rehearse_prepare_init_test.go, internal/cli/rehearse_prepare_e2e_test.go, internal/cli/assumptions_test.go, internal/cli/export_test.go, internal/cli/testdata/golden/commands/, .github/fakeaws-e2e-tests]
risk: high
---

# Gate 2 over modules.json, then the offline second init under the dead proxy; override helpers move to production

Last in the hot-file chain (`.github/fakeaws-e2e-tests`), after rehearsal-prepare-first-init.

**Helpers.** Move ourOverride and localBackendOverride from assumptions_test.go:255-308 into rehearse_prepare_init.go as they are; export_test.go's OurOverride and LocalBackendOverride point at the production symbols. The PR says that localBackendOverride's blockPath branch serves only the assumption tests; production always passes "".

**Gate 2.** After init 1, prepare runs rehearsal-prepare-gate-modules' manifest function, then the full gate over every dir it returns; a refusal stops before init 2.

**Init 2.** `rehearsalDeadProxy` defaults to `http://127.0.0.1:9` and is a var so a test can move it. Before init 2, prepare dials it and refuses if the dial connects. It writes `<work>/infrafactory_override.tf` = localBackendOverride("") at 0600, then runs `tofu init -input=false -no-color -reconfigure -get=false -lockfile=readonly -backend-config=path=<id>/state/terraform.tfstate -plugin-dir=<id>/data/providers` with ReplaceEnv and Env exactly {HOME, TF_DATA_DIR, HTTPS_PROXY, HTTP_PROXY}, both proxies at rehearsalDeadProxy, and no NO_PROXY. Then it tightens modes and reports prepared (id, work dir, lock version, warnings) in rehearse_prepare.go.

**What proves what.** That init 2 makes no proxied connection stays proven by `TestAssumption_SecondInitMakesNoProxiedConnection`; this story's e2e does not re-prove it through a listener, because prepare's own liveness dial would connect to one. The liveness refusal is a fake-runner test.

Tip: `ADR: none — ADR-0042 records the second init and gate 2`.

**Done when:**
- A wiring test uses a fake runner whose init-1 call writes a modules.json naming an entry with Dir under `<data>/modules`: prepare exits 1 naming the module key, and the runner log holds no second init. A second case has init 1 plant `provisioner "local-exec"` in a local module dir: prepare exits 1 naming the key and file, with no second init.
- A fake-runner test checks the init-2 args exactly, ReplaceEnv with that exact Env key set, and no NO_PROXY. The state path lies outside `<id>/work`. infrafactory_override.tf equals LocalBackendOverride("") at mode 0600.
- With a listener on rehearsalDeadProxy, prepare refuses and the fake runner records zero init-2 calls.
- `grep -n 'ourOverride =\|func localBackendOverride' internal/cli/*_test.go` is empty, and the rehearsal assumption tests pass with e2e enabled.
- `TestRehearsePrepare` (e2e.SkipUnlessEnabled, real tofu) runs the whole prepare at the default dead address on (a) a root with a local module and (b) a root with no module blocks. Each exits 0 and the mode walk passes; (b) passes gate 2 on what tofu actually wrote.
- `grep -x TestRehearsePrepare .github/fakeaws-e2e-tests` hits.
- Each new test is shown failing by a cp-backup break recorded in the PR.
