// Presentation rules for the live estate (S161).
//
// Kept out of the component so they can be tested directly, because the
// thing this page must get right is not layout. It is that SILENCE MUST
// NOT LOOK LIKE HEALTH -- and a blank table cell is the most natural way
// in the world to render "we do not know".

/** The states a deployment's health can be in, including "nobody looked". */
export const HEALTH_TONES = {
  healthy: "bg-emerald-100 text-emerald-900",
  unhealthy: "bg-rose-100 text-rose-900",
  unreachable: "bg-amber-100 text-amber-900",
  unobserved: "bg-slate-200 text-slate-700"
};

/**
 * healthBadge always returns a label. Never "", never undefined.
 *
 * An unknown status renders as itself rather than as a blank: a status
 * this UI has not heard of is a fact about the system, and hiding it
 * would turn a new backend state into an invisible one.
 */
export function healthBadge(health) {
  const status = health?.status || "unobserved";
  return {
    label: status === "unobserved" ? "never observed" : status,
    tone: HEALTH_TONES[status] || "bg-slate-200 text-slate-700"
  };
}

/**
 * versionBadge distinguishes the three version states, and says the word
 * for all of them.
 *
 * `unchecked` means no version_path was declared, so nobody looked.
 * `unconfirmed` means somebody looked and the running service did NOT
 * report the version this record claims. Those are opposite meanings and
 * a UI that rendered either as an empty cell would merge them.
 */
export function versionBadge(health) {
  const version = health?.version || "unchecked";
  if (version === "confirmed") {
    return { label: "version confirmed", tone: "bg-emerald-100 text-emerald-900" };
  }
  if (version === "unconfirmed") {
    return { label: "version NOT confirmed", tone: "bg-rose-100 text-rose-900" };
  }
  return { label: "version unchecked", tone: "bg-slate-200 text-slate-700" };
}

/**
 * needsAttention marks the rows a person should look at first.
 *
 * The interesting case is the last one: a service answering perfectly
 * while running something other than what the record claims. Every other
 * signal in the system calls that healthy, which is exactly why the
 * estate page has to shout about it -- if this page renders it as a
 * quiet green row, nothing anywhere ever flags it.
 */
export function needsAttention(deployment) {
  if (deployment?.unreadable) return true;
  if (deployment?.expired) return true;

  const health = deployment?.health || {};
  if (health.status === "unhealthy" || health.status === "unreachable") return true;
  return health.status === "healthy" && health.version === "unconfirmed";
}

/**
 * ttlLabel renders remaining life. Expired is said out loud, and so is
 * overdue time, because a deployment past its TTL that still exists is
 * the reaper not having run -- which is a thing to act on, not a
 * cosmetic detail.
 */
export function ttlLabel(seconds, expired) {
  if (expired || seconds <= 0) {
    const overdue = Math.abs(Math.floor(seconds || 0));
    return overdue > 0 ? `expired ${humanDuration(overdue)} ago` : "expired";
  }
  return humanDuration(Math.floor(seconds));
}

function humanDuration(seconds) {
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ${minutes % 60}m`;
  return `${Math.floor(hours / 24)}d ${hours % 24}h`;
}

/**
 * observedLabel says when, or says that nobody has.
 *
 * `at` is null for a never-observed deployment -- the API sends null
 * rather than a zero time precisely so this cannot render a date in the
 * year 1 (S159a).
 */
export function observedLabel(health) {
  if (!health?.at) return "never";
  return new Date(health.at).toLocaleString();
}

/**
 * estateSummary is the one line a person reads before the table.
 *
 * It states what was examined as well as what is wrong, for the same
 * reason `live reconcile` does: "0 needing attention" out of zero
 * deployments and out of forty read identically and mean opposite
 * things.
 *
 * `state` distinguishes the three situations that all produce an empty
 * list, and getting this wrong is the page's whole thesis violated in
 * the page itself:
 *
 *   - "loading"  -- we have not asked yet
 *   - "failed"   -- we asked and could not find out
 *   - "loaded"   -- we asked, and the answer is what you see
 *
 * Only the last may say "Nothing is deployed". An empty list under the
 * other two means WE DO NOT KNOW, and saying otherwise is exactly the
 * falsehood every other part of this page is built to avoid.
 */
/**
 * knownEmpty is the ONLY condition under which anything on this page may
 * claim that nothing is running.
 *
 * Derived once and shared, because the page makes that claim in more
 * than one place -- a summary line and an empty-state panel -- and two
 * copies of the condition is how one of them keeps saying "no live
 * deployments" underneath a banner warning that unreadable records may
 * describe running, billable infrastructure.
 *
 * Three things must all hold: the read succeeded, it returned no
 * deployments, and there is nothing the store could not decode. An
 * undecodable record is not an absence of infrastructure; it is an
 * absence of knowledge.
 */
export function knownEmpty(deployments, unreadable, state = "loaded") {
  return state === "loaded" && (deployments?.length || 0) === 0 && (unreadable?.length || 0) === 0;
}

export function estateSummary(deployments, unreadable, state = "loaded") {
  const total = deployments?.length || 0;
  const unread = unreadable?.length || 0;

  if (state === "loading") return "Reading the live estate…";

  if (state === "failed") {
    if (total === 0 && unread === 0) {
      return "The live estate could not be read. Whether anything is running is unknown.";
    }
    return `${describe(deployments, unreadable)} — read before the error, and possibly out of date.`;
  }

  if (knownEmpty(deployments, unreadable, state)) return "Nothing is deployed.";
  return describe(deployments, unreadable);
}

function describe(deployments, unreadable) {
  const total = deployments?.length || 0;
  const unread = unreadable?.length || 0;
  const attention = (deployments || []).filter(needsAttention).length;

  const parts = [`${total} deployment${total === 1 ? "" : "s"}`];
  if (attention > 0) parts.push(`${attention} needing attention`);
  if (unread > 0) parts.push(`${unread} record${unread === 1 ? "" : "s"} that could not be read`);
  return parts.join(", ");
}

/**
 * addressHref builds the URL a person clicking the address should reach.
 *
 * It must include the PORT. `live observe` probes `address:port` using
 * the port snapshotted on the record, so a link that drops it sends the
 * reader somewhere the system never checked -- and a deployment on 8080
 * would look reachable-but-broken when nothing is wrong with it.
 *
 * Port 80 is omitted because a URL carrying it is the same URL, and a
 * bare host reads better. Zero means the record predates the field or
 * the scenario declared no port; fall back to the bare address rather
 * than inventing one.
 */
export function addressHref(deployment) {
  const address = deployment?.address;
  if (!address) return "";
  const host = hostForURL(address);
  const port = deployment?.port || 0;
  if (port === 0 || port === 80) return `http://${host}`;
  return `http://${host}:${port}`;
}

/**
 * hostForURL brackets an IPv6 literal, because `http://2001:db8::1:8080`
 * is not a URL -- the colons are ambiguous and a browser will not open
 * it.
 *
 * The probe path uses Go's `net.JoinHostPort`, which does exactly this,
 * and `pickHost` accepts any address `net.ParseIP` understands. So an
 * IPv6 deployment can reach this page, and a link that does not match
 * what `live observe` actually probed is the same falsehood as a link
 * that drops the port.
 */
function hostForURL(address) {
  if (address.startsWith("[")) return address;
  // An IPv4 address or a hostname has no colons; an IPv6 literal has at
  // least two. One colon means the address already carries a port, which
  // this function leaves alone rather than guessing about.
  return address.split(":").length > 2 ? `[${address}]` : address;
}

/** addressLabel shows what the link will actually open. */
export function addressLabel(deployment) {
  const href = addressHref(deployment);
  return href ? href.replace(/^http:\/\//, "") : "";
}

/**
 * teardownPrompt is what a person must read before destroying something.
 *
 * It names the scenario, the project and the address, because "are you
 * sure?" is not a safeguard -- it is a speed bump that people learn to
 * click through. What makes a confirmation real is that it states WHICH
 * thing is about to be destroyed, so a misclick on the wrong row is
 * visible before it is irreversible.
 */
export function teardownPrompt(deployment) {
  const parts = [`Destroy ${deployment?.scenario || deployment?.id || "this deployment"}?`];
  if (deployment?.project_id) parts.push(`Project ${deployment.project_id}`);
  if (deployment?.address) parts.push(`serving ${deployment.address}`);
  parts.push("This deletes real infrastructure and cannot be undone.");
  return parts.join(" · ");
}

/**
 * teardownOutcome turns an ActionResult into the one thing to say.
 *
 * `clean` is read rather than `failures.length`, because they are
 * different claims and ADR-0024 turns on the difference: a teardown that
 * cannot PROVE the account clean must not report success, and a green
 * tick over "the state file has vanished and the resources may still be
 * running" is exactly the false green this project exists to avoid.
 */
export function teardownOutcome(result) {
  if (!result) return { ok: false, message: "Teardown returned nothing." };
  if (result.clean) {
    return { ok: true, message: "Destroyed. The account is provably clean." };
  }
  const reasons = (result.failures || []).map((f) => f.detail).filter(Boolean);
  return {
    ok: false,
    message:
      reasons.length > 0
        ? `Not provably clean — resources may still be running. ${reasons.join(" ")}`
        : "Not provably clean — resources may still be running."
  };
}
