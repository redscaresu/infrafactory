import test from "node:test";
import assert from "node:assert/strict";

import {
  addressHref,
  deployConfirmation,
  deployWarnings,
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
