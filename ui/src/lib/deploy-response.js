/**
 * How a deploy response is read.
 *
 * Plain JS, and in its own module, for the reason `pitfalls-api.js`
 * exists: `ui/tests/*.test.js` runs under `node --test`, which resolves
 * JS only, so anything living in `api.ts` is reachable by Playwright
 * route interception and nothing else. These four decide whether a
 * reader is told "nothing was created" or "resources may still be
 * running" — the most consequential judgement in this slice — and
 * inverting any of them passed the whole suite.
 */

/**
 * DeployError says whether the apply STARTED, which is the only thing a
 * caller can safely conclude from a failed deploy request.
 *
 * The deploy is deliberately detached from the request that starts it,
 * so a rejected promise has two completely different meanings: the
 * server refused before anything ran, or the apply may be running right
 * now, creating a project and a bill. Collapsing the second into the
 * first is how a page came to delete the progress log of a live apply
 * and tell the reader nothing had happened.
 */
export class DeployError extends Error {
  /**
   * `conclusion` is what the caller may conclude about infrastructure:
   *
   *   - "refused"   -- the server said it rejected the request before
   *                    anything ran. Nothing exists, and any progress
   *                    lines collected while waiting were somebody
   *                    else's apply.
   *   - "clean"     -- the server answered a success status whose body
   *                    could not be read. `writeActionResult` answers
   *                    2xx only for a PROVABLY clean deploy, so nothing
   *                    was left behind; the log is still ours.
   *   - "unknown"   -- anything else. The apply may be running right
   *                    now, and a report has to be filed.
   *
   * One value, three states, because two booleans is what this kept
   * turning into: "did the server refuse?" and "may this have created
   * something?" are different questions, and a caller that has to
   * combine them gets one of them wrong.
   */
  constructor(message, conclusion) {
    super(message);
    this.name = "DeployError";
    this.conclusion = conclusion;
  }
}

/**
 * startedNothing reads the SERVER's word for it.
 *
 * `started_nothing: true` is written by `writeRefusal`, on the paths
 * that reject a request before it can touch the cloud. Absence means
 * unknown, which is the safe default: erring this way sends a reader to
 * check the Deployments page for infrastructure that was never created,
 * and erring the other way tells them nothing happened while a project
 * is being created and billed.
 *
 * This replaced a client-side allowlist of status codes, which was a
 * copy of server semantics — and the copy was already wrong, because
 * `deployHandler` answers 404 both for "no such scenario", before the
 * apply, and for an `os.ErrNotExist` returned by Deploy, after it.
 */
export function startedNothing(body) {
  return typeof body === "object" && body !== null && body.started_nothing === true;
}

/**
 * isActionResult decides whether a body is a result or an error.
 *
 * The `clean` field is the discriminator, and checking it closes a
 * class rather than an instance: the deploy path special-cased a bare
 * 409 because `writeActionResult` produces one, but so can a reverse
 * proxy or the next refusal somebody adds. A `{"error": ...}` body
 * parsed as a result has no `clean` field, so it rendered "resources
 * may still be running" for a request that never reached the deployer.
 */
export function isActionResult(body) {
  return typeof body === "object" && body !== null && "clean" in body;
}

/**
 * readJSON returns `{ok, value}` rather than throwing.
 *
 * A truncated body — a proxy giving up, a server killed mid-write —
 * makes `res.json()` throw a SyntaxError, which escapes as the error a
 * page renders: "Unexpected end of JSON input" where it means to say
 * what went wrong. And `{ok, value}` rather than a bare null, because
 * `null` is a perfectly good JSON document: run artifacts are served
 * verbatim with a JSON content type and one of them can be exactly
 * that.
 */
export async function readJSON(res) {
  try {
    return { ok: true, value: await res.json() };
  } catch {
    return { ok: false, value: null };
  }
}
