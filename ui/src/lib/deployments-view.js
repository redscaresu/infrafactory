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
 * nothingRecorded is "the read returned no records, and none were
 * undecodable" -- and nothing more.
 *
 * Extracted because three places asked it: `knownEmpty`, and both the
 * failed and loaded branches of `estateSummary`. The last two were
 * copies inside the function that CALLS knownEmpty, reachable only when
 * it said no -- so a fourth term added to knownEmpty (as `deploying`
 * was) would leave them still asserting emptiness without it.
 *
 * Deliberately NOT the whole emptiness question: it says nothing about
 * whether the read succeeded or whether anything is applying. Those are
 * knownEmpty's job, and keeping them apart is what stops this becoming
 * a fourth copy of it.
 */
export function nothingRecorded(deployments, unreadable) {
  return (deployments?.length || 0) === 0 && (unreadable?.length || 0) === 0;
}

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
 * FOUR things must all hold: the read succeeded, it returned no
 * deployments, there is nothing the store could not decode, and nothing
 * is applying. An undecodable record is not an absence of
 * infrastructure; it is an absence of knowledge. An applying deploy is
 * the most active thing in the estate and has no record at all.
 *
 * (This block was stranded above `nothingRecorded` when that was
 * extracted between it and its function -- the same insertion defect it
 * had just been moved to fix for `estateSummary`. Moved, and the count
 * corrected from three, 2026-09-04.)
 */
export function knownEmpty(deployments, unreadable, state = "loaded", deploying = [], deployingKnown = true) {
  return (
    state === "loaded" &&
    // An absent `deploying` field is not an empty one. The server
    // always sends it; one that predates the field, or a body trimmed
    // by an intermediary, does not -- and reading that as "nothing is
    // applying" licenses the page's only permitted emptiness claim on
    // an estate that may be busy creating something. The same
    // absent-vs-empty distinction `already_live` is given two files
    // away, on the same wire contract.
    deployingKnown &&
    nothingRecorded(deployments, unreadable) &&
    // A deploy that is APPLYING has no record yet, so it is absent from
    // `deployments` while being the most active thing in the estate.
    // Without this the page said "Nothing is deployed." directly under a
    // banner naming a billable apply in flight -- a third copy of the
    // emptiness question that neither of the other two knew about.
    (deploying?.length || 0) === 0
  );
}

/**
 * deployingLabel is the ONE phrase that says how many deploys are
 * applying, and whether that count is current.
 *
 * Exported and shared because the estate page states it twice -- in the
 * summary line and in the banner two lines below it -- and two copies of
 * one claim is the shape `knownEmpty` was extracted to remove. They can
 * silently diverge on wording, on pluralisation, and on the staleness
 * qualifier, while sitting next to each other on screen.
 *
 * `stale` matters more than it looks. The in-flight list is kept across
 * a failed refresh on purpose, but that does NOT make it current: it is
 * exactly as old as the rows beside it, which do say they were "read
 * before the error". An unqualified "1 deploy in progress" asserts as
 * present-tense fact something that may have finished a minute ago, and
 * keeps asserting it for as long as polling fails.
 */
export function deployingLabel(deploying, stale = false) {
  const applying = deploying?.length || 0;
  if (applying === 0) return "";
  const count = `${applying} deploy${applying === 1 ? "" : "s"} in progress`;
  return stale ? `${count} when the estate was last read` : count;
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
 *
 * (This block sat above `knownEmpty` for two slices, because a second
 * JSDoc comment was inserted between it and the function it describes.
 * Moved 2026-09-03.)
 */
export function estateSummary(
  deployments,
  unreadable,
  state = "loaded",
  deploying = [],
  deployingKnown = true
) {
  const applying = deploying?.length || 0;

  if (state === "loading") return "Reading the live estate…";

  if (state === "failed") {
    // A failed read says nothing about what is APPLYING, and the list
    // is kept across the error rather than cleared -- so the failed
    // branch has to carry it too. Without this the summary read
    // "Whether anything is running is unknown." directly above the
    // banner naming a billable apply in flight. `knownEmpty` was
    // extracted so two derived claims about emptiness could not
    // contradict each other; this branch was a third claim that neither
    // of them knew about.
    //
    // Carried as STALE, though. Surviving the error does not make it
    // current: it was read at the same moment as the rows, and they say
    // so about themselves.
    const staleApplying = deployingLabel(deploying, true);
    if (nothingRecorded(deployments, unreadable)) {
      if (!staleApplying) {
        return "The live estate could not be read. Whether anything is running is unknown.";
      }
      return `${staleApplying}. The live estate could not be read since, so what is running now is unknown.`;
    }
    const read = `${describe(deployments, unreadable)} — read before the error, and possibly out of date.`;
    return staleApplying ? `${staleApplying}. ${read}` : read;
  }

  // `deployingKnown` passed THROUGH. Dropping it here re-entered
  // knownEmpty with the parameter defaulting to true, so the summary
  // line said "Nothing is deployed." while the empty-state panel beside
  // it was correctly suppressed -- two derived emptiness claims
  // contradicting each other on one screen, which is the defect the
  // extraction exists to prevent.
  if (knownEmpty(deployments, unreadable, state, deploying, deployingKnown)) {
    return "Nothing is deployed.";
  }

  const described = describe(deployments, unreadable);
  const applyingText = deployingLabel(deploying);
  if (applying === 0) return described;
  // A successful read that found nothing still has to SAY it found
  // nothing. Returning the in-flight count alone left the reader with
  // no statement at all about the estate: the empty-state panel is
  // suppressed here (`knownEmpty` is false) and the table does not
  // render, so this line is the only thing that speaks.
  if (nothingRecorded(deployments, unreadable)) {
    return `${applyingText}. Nothing else is deployed.`;
  }
  return `${described}, ${applyingText}`;
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
  return actionOutcome(result, {
    nothing: "Teardown returned nothing.",
    proven: "Destroyed. The account is provably clean.",
    unproven: "Not provably clean — resources may still be running."
  });
}

/**
 * deployOutcome is `teardownOutcome`'s sibling, and exists because the
 * two verbs are not interchangeable.
 *
 * Reusing `teardownOutcome` for a deploy put teardown's words on the
 * deploy screen: a 409 rendered "Not provably clean — resources may
 * still be running", and a malformed body rendered "Teardown returned
 * nothing." next to a Deploy button. The success case was masked
 * because the template overrides it with deploy-specific text, so the
 * wrong-verb strings were reachable only on the failure branch -- where
 * a reader is least equipped to discount them.
 *
 * `ok` still means the same thing it means everywhere here: PROVEN. A
 * deploy that cannot prove its account clean is not a success (ADR-0024).
 */
export function deployOutcome(result) {
  return actionOutcome(result, {
    nothing: "The deploy returned nothing, so what it created is unknown.",
    // The whole sentence, because the template renders `message` for
    // both branches now. It used to hardcode its own success text and
    // ignore this field, so an edit here changed nothing on screen
    // while every unit test asserting on it kept passing.
    proven: "Deployed. It is listed on the Deployments page until its TTL expires.",
    unproven:
      "The deploy did not finish cleanly — it may have created resources that are still running."
  });
}

/**
 * actionOutcome is ADR-0024's rule, once.
 *
 * The rule is `clean`, not `failures.length`: an action that cannot
 * PROVE its account clean is not a success, whatever else it reports.
 * Deploy and teardown must differ in their WORDS -- "Teardown returned
 * nothing." beside a Deploy button is its own defect -- but they must
 * not differ in the rule, and two structural copies of it are two
 * places a future change has to find. Applied to one and not the other,
 * the deploy screen would report a success the teardown screen refuses.
 *
 * The per-stage failure details are appended because they are the
 * useful part: they name the project that could not be deleted, which
 * is the handle for removing it by hand.
 *
 * The absent-result branch is not reachable from either caller today:
 * `api.deployScenario` and `api.tearDownDeployment` return only when
 * `isActionResult` holds and throw otherwise. It stays because this
 * judgement is the one place a green tick could appear over nothing at
 * all, and the guarantee that prevents it lives in another module.
 */
function actionOutcome(result, words) {
  // `mayHaveCreated` is what decides whether a banner is a REPORT that
  // has to survive until somebody acts on it, or a claim that goes
  // stale. It is not the same question as `ok`:
  //
  //   - a success is recorded on the estate page, so the banner is a
  //     claim about a TTL that may already have expired -- droppable;
  //   - a refusal started nothing, so there is nothing to report --
  //     droppable, and it used to reappear on every later visit for the
  //     rest of the session;
  //   - an unproven action, or one whose request failed after the apply
  //     began, may have left resources with no record anywhere. That is
  //     the only kind that must not be forgotten.
  //
  // An absent result is the same case: nobody knows what it created.
  if (!result) return { ok: false, mayHaveCreated: true, message: words.nothing };
  if (result.clean) return { ok: true, mayHaveCreated: false, message: words.proven };
  const reasons = (result.failures || []).map((f) => f.detail).filter(Boolean);
  return {
    ok: false,
    mayHaveCreated: true,
    message: reasons.length > 0 ? `${words.unproven} ${reasons.join(" ")}` : words.unproven
  };
}

/**
 * deployConfirmation is what a person must read before creating
 * infrastructure (ADR-0027 §2).
 *
 * Returns an ordered list of lines rather than a sentence, because each
 * one is a separate thing they might object to and a paragraph is a
 * thing people skim.
 */
export function deployConfirmation(preview) {
  if (!preview) return [];

  const lines = [];

  const shapes = (preview.cost?.components || [])
    .map((c) => (c.count > 1 ? `${c.count} × ${c.name}` : c.name))
    .join(", ");
  lines.push(shapes ? `Creates: ${shapes}` : "Creates: unknown — see below");

  // The cost line always says list price, and always says when it is a
  // floor rather than a total.
  lines.push(preview.cost_summary || "Cost unknown.");

  if (preview.expires_at_wall_clock) {
    lines.push(`Expires ${preview.expires_at_wall_clock} — after that, reap destroys it.`);
  }

  if (preview.internet_facing) {
    lines.push("Reachable from the public internet.");
  }

  if (preview.image) lines.push(`Running ${preview.image}.`);

  return lines;
}

/**
 * deployWarnings are the things that should stop somebody, separated
 * from the descriptive lines so a page can render them differently.
 *
 * Order matters and is asserted by tests that read the first warning.
 * An EXISTING deployment of this scenario comes first: it is the one a
 * reader is most likely to have simply forgotten, and the only one whose
 * cost is a second bill. `modelled === false` follows, because it
 * invalidates the figures above it -- an unmodelled scenario's empty
 * component list and €0.00 mean "unknown", not "nothing".
 */
export function deployWarnings(preview) {
  const warnings = [];
  if (!preview) return warnings;

  // First, because they are the ones a reader is most likely to have
  // simply forgotten -- and the only ones whose cost is a whole second
  // bill. Pushed individually so each renders as its own warning.
  warnings.push(...alreadyLiveWarnings(preview));

  if (preview.cost && preview.cost.modelled === false) {
    warnings.push(
      "This scenario's resources are not modelled here, so what it creates and what it costs are both unknown. Do not read the figures above as complete."
    );
  } else if (preview.cost && preview.cost.complete === false) {
    const unpriced = (preview.cost.unpriced || []).join(", ");
    warnings.push(
      `The cost is a floor, not a total: ${unpriced || "some components"} have no list price here.`
    );
  }

  if (preview.internet_facing) {
    warnings.push("This will be reachable from the public internet for its whole lifetime.");
  }

  return warnings;
}


/**
 * isProgressEvent decides whether a websocket message is a deploy
 * progress line at all.
 *
 * Extracted from the component so it can be tested. While it was inline
 * in `+page.svelte` it had NO test at all: the e2e tests intercept the
 * POST in the browser, so the server never broadcasts and the filter was
 * never invoked. Typo-ing the event type — which kills the entire
 * stream — passed the whole suite.
 *
 * SHAPE only, deliberately. The subject is the SCENARIO, which is what
 * the request carries -- it cannot be the deployment id, because that id
 * is minted inside the command after the request is accepted -- and the
 * store looks its entry up BY that subject. So the predicate that used
 * to take a "scenario on screen" was being handed the event's own
 * subject and comparing it with itself: a guard that read as scoping
 * while scoping nothing, which a later edit would have trusted. The real
 * scoping is the keyed lookup and the running check, in the store.
 */
export function isProgressEvent(event) {
  if (event?.type !== "deploy_progress") return false;
  return Boolean(event?.data?.line);
}


/**
 * alreadyLiveWarnings names what already exists, or might.
 *
 * The in-flight lock stops the accidental duplicate. It does nothing
 * about deploy → wait → deploy again, and a lock is the wrong tool for
 * that: it cannot tell "I forgot" from "I meant it", and refusing
 * outright would break redeploying after a teardown.
 *
 * So the confirmation says what exists and the reader decides. The
 * server computed this list before this function existed and NOTHING
 * read it -- the guard the ADR described was documented, tested on the
 * server, and absent from the screen it was for.
 */
export function alreadyLiveWarnings(preview) {
  // A LIST, one entry per fact, strongest first.
  //
  // They answer different questions -- "some are already live", "one is
  // applying right now", "we could not finish looking" -- and all three
  // can be true at once, so none may be dropped. Two earlier versions
  // dropped one: the first returned early on `already_deploying`, and
  // the fix then concatenated all three into a single paragraph, which
  // demoted the strongest of them to the second sentence of the first
  // warning. Separate strings let the page render them separately, and
  // let the order mean something.
  const warnings = [];

  // An ABSENT list is not an empty one. The server goes to some trouble
  // to make an empty `already_live` a CHECKED claim -- `out :=
  // []string{}`, and `(out, true)` on every path that did not look -- and
  // reading a missing field as `[]` throws that away at the client
  // boundary: an older server, or a body trimmed by an intermediary,
  // would render no warning at all, indistinguishable from "we looked
  // and there is nothing".
  const looked = Array.isArray(preview?.already_live);
  const live = looked ? preview.already_live : [];
  const unknown = preview?.already_live_unknown === true || !looked;

  // "Could not look" is not "nothing is there", and this guard is about
  // billable infrastructure. It must not DISCARD what was found either:
  // the unreadable flag is estate-global -- one corrupt record anywhere
  // sets it -- so it qualifies the concrete list rather than replacing
  // it.
  const caveat = unknown
    ? " Some live records could not be read, so there may be more than this."
    : "";

  // First: the only one whose cost is a whole second bill.
  if (live.length > 0) {
    const ids = live.join(", ");
    warnings.push(
      live.length === 1
        ? `${ids} is already deployed from this scenario. Deploying again creates a SECOND project and a second bill; it does not replace it.${caveat}`
        : `${live.length} deployments from this scenario are already live (${ids}). Deploying again creates ANOTHER project and another bill; it does not replace them.${caveat}`
    );
  } else if (unknown) {
    warnings.push(
      "The live estate could not be fully read, so whether this scenario is already deployed is unknown. Check the Deployments page before continuing."
    );
  }

  // An applying deploy has no record, so the estate cannot see it. The
  // second attempt would simply be refused, which is why this ranks
  // below an existing deployment rather than above it.
  if (preview?.already_deploying) {
    warnings.push(
      "This scenario is being deployed right now. A second deploy will be refused until it finishes."
    );
  }

  return warnings;
}
