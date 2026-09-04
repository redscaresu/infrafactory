import test from "node:test";
import assert from "node:assert/strict";

import {
  alreadyLiveWarnings,
  addressHref,
  deployConfirmation,
  deployWarnings,
  deployOutcome,
  isProgressEvent,
  nothingRecorded,
  deployingLabel,
  teardownOutcome,
  teardownPrompt,
  knownEmpty,
  addressLabel,
  estateSummary,
  healthBadge,
  needsAttention,
  observedLabel,
  ttlLabel,
  versionBadge
} from "../src/lib/deployments-view.js";

// The whole reason this page is specified the way it is: a blank cell
// reads as "fine", and nobody having looked is not fine.
test("healthBadge never renders an empty label", () => {
  for (const health of [undefined, {}, { status: "" }, { status: "unobserved" }]) {
    const badge = healthBadge(health);
    assert.ok(badge.label.length > 0, `empty label for ${JSON.stringify(health)}`);
  }
  assert.equal(healthBadge({ status: "unobserved" }).label, "never observed");
});

// A status this UI has not heard of is a fact about the system. Hiding
// it would turn a new backend state into an invisible one.
test("healthBadge shows an unrecognised status rather than hiding it", () => {
  assert.equal(healthBadge({ status: "quarantined" }).label, "quarantined");
});

// `unchecked` and `unconfirmed` are opposite meanings -- nobody looked
// versus somebody looked and it was wrong -- and an empty cell merges
// them.
test("versionBadge distinguishes unchecked from unconfirmed", () => {
  assert.equal(versionBadge({ version: "unchecked" }).label, "version unchecked");
  assert.equal(versionBadge({ version: "unconfirmed" }).label, "version NOT confirmed");
  assert.equal(versionBadge({ version: "confirmed" }).label, "version confirmed");
  assert.equal(versionBadge(undefined).label, "version unchecked");
});

// The most dangerous state the system can be in, and the one every other
// signal calls healthy. If this page renders it as a quiet green row,
// nothing anywhere flags it.
test("needsAttention flags a healthy service on an unconfirmed version", () => {
  assert.equal(needsAttention({ health: { status: "healthy", version: "unconfirmed" } }), true);
});

test("needsAttention leaves a genuinely healthy deployment alone", () => {
  assert.equal(needsAttention({ health: { status: "healthy", version: "confirmed" } }), false);
});

test("needsAttention flags failures, expiry and unreadable records", () => {
  assert.equal(needsAttention({ health: { status: "unhealthy" } }), true);
  assert.equal(needsAttention({ health: { status: "unreachable" } }), true);
  assert.equal(needsAttention({ expired: true, health: { status: "healthy" } }), true);
  assert.equal(needsAttention({ unreadable: true }), true);
});

// A deployment past its TTL that still exists means the reaper has not
// run. That is a thing to act on, not a cosmetic detail.
test("ttlLabel says how long something is overdue", () => {
  assert.equal(ttlLabel(-3600, true), "expired 1h 0m ago");
  assert.equal(ttlLabel(0, true), "expired");
  assert.equal(ttlLabel(45, false), "45s");
  assert.equal(ttlLabel(3600, false), "1h 0m");
  assert.equal(ttlLabel(90000, false), "1d 1h");
});

// The API sends null rather than a zero time precisely so this cannot
// render a date in the year 1.
test("observedLabel says never rather than inventing a date", () => {
  assert.equal(observedLabel({ at: null }), "never");
  assert.equal(observedLabel({}), "never");
  assert.equal(observedLabel(undefined), "never");
});

// "0 needing attention" out of zero deployments and out of forty read
// identically and mean opposite things.
test("estateSummary states what was examined, not only what is wrong", () => {
  assert.equal(estateSummary([], []), "Nothing is deployed.");
  assert.equal(
    estateSummary([{ health: { status: "healthy", version: "confirmed" } }], []),
    "1 deployment"
  );
  assert.equal(
    estateSummary(
      [
        { health: { status: "healthy", version: "confirmed" } },
        { health: { status: "unhealthy" } }
      ],
      ["dep-broken.json: unexpected end of JSON input"]
    ),
    "2 deployments, 1 needing attention, 1 record that could not be read"
  );
});

// `live observe` probes address:port using the port snapshotted on the
// record. A link that drops it sends the reader somewhere the system
// never checked, and a deployment on 8080 looks broken when nothing is
// wrong with it.
test("addressHref keeps the recorded port", () => {
  assert.equal(addressHref({ address: "51.15.0.1", port: 8080 }), "http://51.15.0.1:8080");
  assert.equal(addressLabel({ address: "51.15.0.1", port: 8080 }), "51.15.0.1:8080");
});

// A URL carrying :80 is the same URL, and a bare host reads better.
test("addressHref omits port 80", () => {
  assert.equal(addressHref({ address: "51.15.0.1", port: 80 }), "http://51.15.0.1");
});

// Zero means the record predates the field or no port was declared.
// Fall back rather than inventing one.
test("addressHref falls back to the bare address when no port is recorded", () => {
  assert.equal(addressHref({ address: "51.15.0.1" }), "http://51.15.0.1");
  assert.equal(addressHref({ address: "51.15.0.1", port: 0 }), "http://51.15.0.1");
});

test("addressHref is empty when there is no address at all", () => {
  assert.equal(addressHref({}), "");
  assert.equal(addressLabel({}), "");
});

// The page's whole thesis, applied to the page's own summary line. An
// empty list means "nothing is deployed" ONLY when the read succeeded;
// under a failure it means we do not know.
test("estateSummary never calls a failed read an empty estate", () => {
  assert.equal(
    estateSummary([], [], "failed"),
    "The live estate could not be read. Whether anything is running is unknown."
  );
});

test("estateSummary marks stale rows as stale rather than current", () => {
  const summary = estateSummary(
    [{ health: { status: "healthy", version: "confirmed" } }],
    [],
    "failed"
  );
  assert.match(summary, /read before the error/);
});

test("estateSummary does not answer before it has asked", () => {
  assert.equal(estateSummary([], [], "loading"), "Reading the live estate\u2026");
});

// An undecodable record is not an absence of infrastructure; it is an
// absence of knowledge. The page may claim nothing is running only when
// all three hold.
test("knownEmpty requires a successful read, no deployments, and nothing unreadable", () => {
  assert.equal(knownEmpty([], [], "loaded"), true);
  assert.equal(knownEmpty([], ["dep-broken.json: bad"], "loaded"), false);
  assert.equal(knownEmpty([], [], "failed"), false);
  assert.equal(knownEmpty([], [], "loading"), false);
  assert.equal(knownEmpty([{ id: "dep-1" }], [], "loaded"), false);
});

// `http://2001:db8::1:8080` is not a URL. The probe path uses Go's
// net.JoinHostPort, which brackets, and pickHost accepts any address
// net.ParseIP understands -- so IPv6 can reach this page.
test("addressHref brackets an IPv6 literal", () => {
  assert.equal(addressHref({ address: "2001:db8::1", port: 8080 }), "http://[2001:db8::1]:8080");
  assert.equal(addressHref({ address: "2001:db8::1" }), "http://[2001:db8::1]");
});

test("addressHref leaves an already-bracketed address alone", () => {
  assert.equal(addressHref({ address: "[2001:db8::1]", port: 8080 }), "http://[2001:db8::1]:8080");
});

test("addressHref does not bracket IPv4 or a hostname", () => {
  assert.equal(addressHref({ address: "51.15.0.1", port: 8080 }), "http://51.15.0.1:8080");
  assert.equal(addressHref({ address: "example.test", port: 8080 }), "http://example.test:8080");
});

// "Are you sure?" is a speed bump people learn to click through. What
// makes a confirmation real is that it names WHICH thing is about to be
// destroyed, so a misclick on the wrong row is visible beforehand.
test("teardownPrompt names the scenario, project and address", () => {
  const prompt = teardownPrompt({
    id: "dep-1",
    scenario: "web-live-paris",
    project_id: "proj-abc",
    address: "51.15.0.1"
  });
  assert.match(prompt, /web-live-paris/);
  assert.match(prompt, /proj-abc/);
  assert.match(prompt, /51\.15\.0\.1/);
  assert.match(prompt, /cannot be undone/);
});

// ADR-0024: a teardown that cannot PROVE the account clean must not
// report success. A green tick over "resources may still be running" is
// the false green this project exists to avoid.
test("teardownOutcome refuses to call an unproven teardown a success", () => {
  const outcome = teardownOutcome({
    clean: false,
    steps: [],
    failures: [{ stage: "teardown", status: "fail", detail: "state file has vanished" }]
  });
  assert.equal(outcome.ok, false);
  assert.match(outcome.message, /may still be running/);
  assert.match(outcome.message, /state file has vanished/);
});

test("teardownOutcome reads clean rather than counting failures", () => {
  // No failures listed, but not clean. These are different claims.
  const outcome = teardownOutcome({ clean: false, steps: [], failures: [] });
  assert.equal(outcome.ok, false);
});

test("teardownOutcome reports a proven teardown as done", () => {
  assert.equal(teardownOutcome({ clean: true, steps: [], failures: [] }).ok, true);
});

test("teardownOutcome treats a missing result as a failure", () => {
  assert.equal(teardownOutcome(undefined).ok, false);
});

const previewFixture = (over = {}) => ({
  scenario: "lb-serving-paris",
  deployable: true,
  // The server always sends this, and an ABSENT list is deliberately
  // not the same as an empty one -- so a fixture that omitted it was
  // testing the "we could not look" path by accident.
  already_live: [],
  already_live_unknown: false,
  image: "nginx:1.27",
  ttl: "4h0m0s",
  expires_at: "2026-09-03T03:47:00Z",
  expires_at_wall_clock: "Wed 3 Sep 03:47 UTC",
  cost_summary: "about €0.04/hour at list price, €0.17 for 4h0m0s",
  internet_facing: true,
  deploy_allowed: true,
  cost: {
    components: [
      { name: "DEV1-S instance", count: 1, eur_per_hour: 0.00898, priced: true },
      { name: "public IPv4 address", count: 2, eur_per_hour: 0.005, priced: true }
    ],
    eur_per_hour: 0.042,
    unpriced: [],
    complete: true,
    modelled: true,
    ...over.cost
  },
  ...over
});

// Each line is a separate thing somebody might object to. A paragraph is
// a thing people skim.
test("deployConfirmation states shape, cost, expiry and exposure", () => {
  const lines = deployConfirmation(previewFixture());

  assert.match(lines[0], /DEV1-S instance/);
  assert.match(lines[0], /2 × public IPv4 address/);
  assert.match(lines.join(" "), /list price/);
  assert.match(lines.join(" "), /Wed 3 Sep 03:47/);
  assert.match(lines.join(" "), /public internet/);
  assert.match(lines.join(" "), /nginx:1\.27/);
});

// An unmodelled scenario's empty component list and €0.00 mean
// "unknown", not "nothing" — and that invalidates everything above it.
test("deployWarnings leads with an unmodelled scenario", () => {
  const warnings = deployWarnings(
    previewFixture({ cost: { components: [], eur_per_hour: 0, unpriced: [], complete: false, modelled: false } })
  );

  assert.match(warnings[0], /not modelled/);
  assert.match(warnings[0], /Do not read the figures above as complete/);
});

test("deployWarnings says when the cost is a floor rather than a total", () => {
  const warnings = deployWarnings(
    previewFixture({
      internet_facing: false,
      cost: {
        components: [],
        eur_per_hour: 0.042,
        unpriced: ["Kubernetes cluster"],
        complete: false,
        modelled: true
      }
    })
  );

  assert.equal(warnings.length, 1);
  assert.match(warnings[0], /floor, not a total/);
  assert.match(warnings[0], /Kubernetes cluster/);
});

test("deployWarnings warns about internet exposure", () => {
  const warnings = deployWarnings(previewFixture());
  assert.ok(warnings.some((w) => /public internet/.test(w)));
});

// A complete, private, modelled estimate has nothing to warn about, and
// a warning that always fires is one people stop reading.
test("deployWarnings stays silent when there is nothing to warn about", () => {
  assert.deepEqual(deployWarnings(previewFixture({ internet_facing: false })), []);
});

test("deployConfirmation admits when it does not know what will be created", () => {
  const lines = deployConfirmation(
    previewFixture({ cost: { components: [], eur_per_hour: 0, unpriced: [], complete: false, modelled: false } })
  );
  assert.match(lines[0], /unknown/);
});

// This filter had NO test while it lived inline in the component: the
// e2e tests intercept the POST in the browser, so the server never
// broadcasts and the filter was never invoked. Typo-ing the event type,
// which kills the entire stream, passed the whole suite.
// Its only production caller is the store's socket handler, which is
// what makes these worth having: the e2e tests intercept the POST in the
// browser, so the server never broadcasts and this filter is never
// exercised there. Typo-ing the event type kills the entire stream and
// passes the whole Playwright suite.
test("isProgressEvent takes deploy progress and nothing else", () => {
  const event = (over = {}) => ({
    type: "deploy_progress",
    data: { subject: "web-app-paris", line: "apply: running", ...over }
  });

  assert.equal(isProgressEvent(event()), true);
  assert.equal(isProgressEvent({ ...event(), type: "log" }), false);
  assert.equal(isProgressEvent({ ...event(), type: "deploy_progres" }), false, "a typo kills the stream");
});

test("isProgressEvent ignores malformed events rather than rendering blanks", () => {
  assert.equal(isProgressEvent({ type: "deploy_progress" }), false);
  assert.equal(isProgressEvent({ type: "deploy_progress", data: { subject: "a" } }), false);
  assert.equal(isProgressEvent({ type: "deploy_progress", data: { subject: "a", line: "" } }), false);
  assert.equal(isProgressEvent(undefined), false);
  assert.equal(isProgressEvent(null), false);
});

test("deployWarnings leads with an existing deployment of the same scenario", () => {
  const warnings = deployWarnings(
    previewFixture({ internet_facing: false, already_live: ["dep-existing"] })
  );

  assert.match(warnings[0], /dep-existing/);
  assert.match(warnings[0], /SECOND project/);
  assert.match(warnings[0], /does not replace it/);
});

test("deployWarnings counts several existing deployments", () => {
  const warnings = deployWarnings(
    previewFixture({ internet_facing: false, already_live: ["dep-a", "dep-b"] })
  );

  assert.match(warnings[0], /2 deployments/);
  assert.match(warnings[0], /dep-a, dep-b/);
  // The plural case is the MORE expensive one, and it used to be the
  // only one that dropped the cost consequence -- the language
  // disappeared exactly where it mattered most, invisibly, because this
  // test asserted only the count and the ids.
  assert.match(warnings[0], /another bill/);
});

test("alreadyLiveWarnings is silent when nothing is live", () => {
  assert.deepEqual(alreadyLiveWarnings({ already_live: [] }), []);
});

// The server makes an empty list a CHECKED claim. Reading a missing
// field as an empty one throws that away at the client boundary: an
// older server, or a body trimmed by an intermediary, would render no
// warning at all -- indistinguishable from "we looked and there is
// nothing", on a guard about billable infrastructure.
test("alreadyLiveWarnings does not read an absent list as an empty one", () => {
  const [missing] = alreadyLiveWarnings({});
  assert.match(missing, /could not be fully read/);
  const [nothing] = alreadyLiveWarnings(undefined);
  assert.match(nothing, /could not be fully read/);
});

test("alreadyLiveWarnings says so when the estate could not be read", () => {
  const [warning] = alreadyLiveWarnings({ already_live: [], already_live_unknown: true });
  assert.match(warning, /could not be fully read/);
  assert.match(warning, /unknown/);
});

// The unreadable flag is estate-global -- one corrupt record anywhere
// sets it -- so returning early replaced the strongest, most actionable
// warning with the vaguest one, for every scenario, until somebody found
// the bad file.
test("alreadyLiveWarnings keeps the concrete list even when the estate is partly unreadable", () => {
  const [warning] = alreadyLiveWarnings({
    already_live: ["dep-existing"],
    already_live_unknown: true
  });

  assert.match(warning, /dep-existing/, "what was found must not be discarded");
  assert.match(warning, /SECOND project/);
  assert.match(warning, /may be more than this/, "and the gap is still stated");
});

// A deploy that is APPLYING has no record yet, so it is absent from
// `deployments` while being the most active thing in the estate. The
// page said "Nothing is deployed." directly under a banner naming a
// billable apply in flight.
test("knownEmpty is false while something is deploying", () => {
  assert.equal(knownEmpty([], [], "loaded", ["web-app-paris"]), false);
  assert.equal(knownEmpty([], [], "loaded", []), true);
});

test("estateSummary counts deploys in progress", () => {
  // A successful read that found nothing still has to say so: the
  // empty-state panel is suppressed here and the table does not render,
  // so this line is the only thing that speaks about the estate.
  assert.equal(
    estateSummary([], [], "loaded", ["web-app-paris"]),
    "1 deploy in progress. Nothing else is deployed."
  );
  assert.equal(
    estateSummary(
      [{ health: { status: "healthy", version: "confirmed" } }],
      [],
      "loaded",
      ["web-app-paris"]
    ),
    "1 deployment, 1 deploy in progress"
  );
});

// An applying deploy has no record, so the estate cannot see it — and it
// is exactly the case where a reader is most likely duplicating.
test("alreadyLiveWarnings reports a deploy that is applying right now", () => {
  const [warning] = alreadyLiveWarnings({ already_deploying: true, already_live: [] });
  assert.match(warning, /being deployed right now/);
  assert.match(warning, /will be refused/);
});

// Three separate questions, all of which can be true at once. Returning
// on the first dropped the strongest and most actionable of them --
// "dep-x is already deployed; deploying again creates a SECOND project
// and a second bill" -- exactly when the reader was most likely to be
// duplicating something.
// One warning per fact, and the STRONGEST first. Concatenating them
// into a paragraph demoted the second-bill warning to the second
// sentence of the first string -- the same demotion the early return
// caused, one layer down, and invisible to a test reading `.first()`.
test("alreadyLiveWarnings keeps every warning rather than choosing between them", () => {
  const warnings = alreadyLiveWarnings({
    already_deploying: true,
    already_live: ["dep-existing"],
    already_live_unknown: true
  });
  assert.equal(warnings.length, 2, "separate warnings, so a page can render them separately");
  assert.match(warnings[0], /dep-existing/, "the second-bill warning leads");
  assert.match(warnings[0], /SECOND project/);
  assert.match(warnings[0], /could not be read/, "and carries its own caveat");
  assert.match(warnings[1], /being deployed right now/);
});

test("alreadyLiveWarnings still reports an unreadable estate while something is applying", () => {
  const warnings = alreadyLiveWarnings({
    already_deploying: true,
    already_live: [],
    already_live_unknown: true
  });
  assert.equal(warnings.length, 2);
  assert.match(warnings[0], /could not be fully read/);
  assert.match(warnings[1], /being deployed right now/);
});

// `deploying` is kept from the last successful poll precisely so it
// survives a failed one, and the banner beneath the summary renders it.
// A summary that said "whether anything is running is unknown" directly
// above "1 deploy in progress" was a third claim about emptiness that
// neither knownEmpty nor the banner knew about.
test("estateSummary does not call a running deploy unknown when the read fails", () => {
  const summary = estateSummary([], [], "failed", ["web-app-paris"]);
  assert.match(summary, /1 deploy in progress/);
  assert.match(summary, /what is running now is unknown/);
});

// Surviving the error does not make it current. It was read at the same
// moment as the rows, and they say "read before the error" about
// themselves; an unqualified "1 deploy in progress" asserts as
// present-tense a deploy that may have finished a minute ago, and keeps
// asserting it for as long as polling fails.
test("estateSummary does not present a stale in-flight list as current", () => {
  const summary = estateSummary([], [], "failed", ["web-app-paris"]);
  assert.match(summary, /when the estate was last read/);
});

test("deployingLabel is silent when nothing is applying", () => {
  assert.equal(deployingLabel([]), "");
  assert.equal(deployingLabel(undefined, true), "");
});

test("deployingLabel says whether the count is current", () => {
  assert.equal(deployingLabel(["a"]), "1 deploy in progress");
  assert.equal(deployingLabel(["a", "b"]), "2 deploys in progress");
  assert.equal(deployingLabel(["a"], true), "1 deploy in progress when the estate was last read");
});

// A deploy must not speak in teardown's vocabulary. Reusing
// teardownOutcome put "Teardown returned nothing." next to a Deploy
// button, reachable only on the failure branch -- where a reader is
// least equipped to discount it.
test("deployOutcome never describes a deploy as a teardown", () => {
  for (const outcome of [
    deployOutcome(null),
    deployOutcome({ clean: false, failures: [] }),
    deployOutcome({ clean: false, failures: [{ detail: "project delete failed" }] }),
    deployOutcome({ clean: true, failures: [] })
  ]) {
    assert.doesNotMatch(outcome.message, /[Tt]eardown|[Dd]estroyed/);
  }
});

test("deployOutcome refuses to call an unproven deploy a success", () => {
  assert.equal(deployOutcome({ clean: false, failures: [] }).ok, false);
  assert.equal(deployOutcome({ clean: true, failures: [] }).ok, true);
});

// `api.deployScenario` returns only when `isActionResult` holds, so a
// null cannot reach here from the app today. This asserts the TYPE's
// behaviour, not a path a screen renders -- an exported function whose
// caller's guarantee is not local to it, and whose worst possible
// answer would be a green tick over nothing at all.
test("deployOutcome does not treat an absent result as a success", () => {
  assert.equal(deployOutcome(null).ok, false);
  assert.equal(deployOutcome(undefined).ok, false);
  assert.equal(teardownOutcome(null).ok, false);
});

test("deployOutcome carries the per-stage failures, which name what leaked", () => {
  const outcome = deployOutcome({
    clean: false,
    failures: [{ detail: "project if-run-abc could not be deleted" }]
  });
  assert.match(outcome.message, /if-run-abc/);
  assert.match(outcome.message, /may have created resources/);
});

test("estateSummary still admits total ignorance when nothing is applying", () => {
  assert.equal(
    estateSummary([], [], "failed", []),
    "The live estate could not be read. Whether anything is running is unknown."
  );
});

test("estateSummary carries deploys in progress alongside a partial read", () => {
  const summary = estateSummary(
    [{ id: "dep-1", state: "live", health: { status: "healthy", version: "confirmed" } }],
    [],
    "failed",
    ["web-app-paris"]
  );
  assert.match(summary, /1 deploy in progress/);
  assert.match(summary, /read before the error/);
});

// Deploy and teardown differ in their WORDS, never in the rule. Two
// structural copies of "clean, not failures.length" are two places a
// future change to ADR-0024's judgement has to find, and applied to one
// and not the other, the deploy screen would report a success the
// teardown screen refuses.
test("deploy and teardown judge an unproven action identically", () => {
  const cases = [
    null,
    { clean: false, failures: [] },
    { clean: false, failures: [{ detail: "project 7c98d82e is live" }] },
    { clean: true, failures: [] }
  ];
  for (const result of cases) {
    assert.equal(
      deployOutcome(result).ok,
      teardownOutcome(result).ok,
      "one rule, whatever the vocabulary"
    );
  }
});

test("nothingRecorded answers only what was recorded", () => {
  assert.equal(nothingRecorded([], []), true);
  assert.equal(nothingRecorded([{ id: "dep-1" }], []), false);
  assert.equal(nothingRecorded([], ["dep-broken.json"]), false);
  // Deliberately says nothing about whether the read succeeded, or
  // whether anything is applying -- that is knownEmpty's job, and
  // keeping them apart is what stops this becoming a copy of it.
  assert.equal(nothingRecorded(undefined, undefined), true);
});

// The store looks the entry up by the event's own subject, so passing
// that subject back in as "the scenario on screen" compared it with
// itself -- a guard that reads as scoping while scoping nothing.
test("isProgressEvent checks the shape and not the scenario", () => {
  assert.equal(isProgressEvent({ type: "deploy_progress", data: { subject: "a", line: "x" } }), true);
  assert.equal(isProgressEvent({ type: "log", data: { subject: "a", line: "x" } }), false);
  assert.equal(isProgressEvent({ type: "deploy_progress", data: { subject: "a" } }), false);
  assert.equal(isProgressEvent(undefined), false);
});

// The server always sends `deploying`. One that predates the field, or
// a body trimmed by an intermediary, does not — and reading that as
// "nothing is applying" licenses the page's only permitted emptiness
// claim on an estate that may be busy creating something. The same
// absent-vs-empty distinction `already_live` is given.
test("knownEmpty will not call an estate empty without being told what is applying", () => {
  assert.equal(knownEmpty([], [], "loaded", [], true), true);
  assert.equal(knownEmpty([], [], "loaded", [], false), false, "asked, and not told");
});

// The summary and the empty-state panel are two derived claims about
// the same thing, and knownEmpty exists so they cannot disagree.
// Dropping the fifth argument re-entered it with the parameter
// defaulting to true, so the summary said "Nothing is deployed." while
// the panel beside it was correctly suppressed.
test("estateSummary and knownEmpty agree about an unanswered deploying field", () => {
  assert.equal(knownEmpty([], [], "loaded", [], false), false);
  assert.notEqual(
    estateSummary([], [], "loaded", [], false),
    "Nothing is deployed.",
    "one screen, one answer"
  );
});
