export interface RunSummary {
  scenario: string;
  run_id: string;
  status: string;
  terminal_reason?: string;
  started_at: string;
}

export interface ConfigResponse {
  version: string;
  agent: {
    type: string;
  };
}

export interface ScenarioItem {
  name: string;
  path: string;
  description: string;
  cloud: string;
  last_run?: {
    run_id: string;
    status: string;
    terminal_reason?: string;
  };
}

// Backwards-compat alias used by the sidebar's cloud-regrouping helper.
export type Scenario = ScenarioItem;

export interface ScenarioGroup {
  name: string;
  scenarios: ScenarioItem[];
}

export interface ScenarioDetail {
  name: string;
  path: string;
  description: string;
  cloud: string;
  raw_yaml: string;
  resources: Record<string, unknown>;
  // constraints removed in S51 — per-criterion params live on each
  // criterion in the criteria array.
  criteria: Array<Record<string, unknown>>;
}

export interface StartRunOptions {
  clean?: boolean;
  no_destroy?: boolean;
  // No layer3_enabled. Real-cloud apply is settled when the server starts
  // (`infrafactory ui --allow-layer3`), so a request cannot ask for it --
  // see ADR-0026. The server ignores the field; the type omits it so
  // nobody writes code that believes otherwise.
}

export interface ScenarioRunModeResponse {
  name: string;
  path: string;
  cloud?: string;
  mock_provider?: string;
  mode: "clean" | "incremental";
  reason: string;
  previous_run_id?: string;
  has_mock_resources: boolean;
  has_tfstate: boolean;
  has_previous_successful_run: boolean;
}

export interface ScenarioLayer3StatusResponse {
  name: string;
  path: string;
  cloud?: string;
  // What the server WILL do, not a default this page may override.
  server_allows_layer3: boolean;
  credentials_ready: boolean;
  missing_credentials: string[];
  ready: boolean;
  detail: string;
}

export interface DiagnosticsCheck {
  name: string;
  status: string;
  detail: string;
}

export interface DiagnosticsResponse {
  agent_type: string;
  ready: boolean;
  summary: string;
  checks: DiagnosticsCheck[];
  session_id?: string;
  started_at?: string;
  limitations?: string[];
}

export interface Pitfall {
  resource: string;
  rule: string;
  source: string;
  discovered_from?: string;
}

export interface PitfallProviderGroup {
  provider: string;
  pitfalls: Pitfall[];
  // parse_error is set when the on-disk YAML for this provider
  // couldn't be parsed; the UI should render an inline error banner
  // alongside the (typically empty) pitfalls list.
  parse_error?: string;
}

export interface PitfallsResponse {
  providers: PitfallProviderGroup[];
}

export interface SavePitfallsResponse {
  provider: string;
  count: number;
}

export interface DeploymentHealth {
  status: string;
  // Always spelled out -- "confirmed" | "unconfirmed" | "unchecked".
  // Never an empty string: the API sends the word so this UI cannot
  // render nobody-looked as a blank cell (ADR-0024, S159a).
  version: string;
  // null when never observed, rather than a zero time that would render
  // as a date in the year 1.
  at: string | null;
  detail?: string;
  observations: number;
}

export interface Deployment {
  id: string;
  scenario?: string;
  cloud?: string;
  project_id?: string;
  state?: string;
  image?: string;
  tag?: string;
  address?: string;
  // The service port snapshotted at deploy time. `live observe` probes
  // address:port, so a link that drops it points somewhere the system
  // never checked.
  port?: number;
  unreadable: boolean;
  health: DeploymentHealth;
  expired: boolean;
  time_to_live_seconds: number;
  upgraded: boolean;
  upgraded_at: string | null;
  upgrade_started_at: string | null;
}

export interface DeploymentsResponse {
  schema: string;
  deployments: Deployment[];
  // Records the store could not decode. They may describe running,
  // billing infrastructure, so the page must show them rather than
  // letting "we could not check" look like "nothing is running".
  unreadable: string[];
  // Whether the server was started with --allow-teardown. A page should
  // not offer a button it knows will 404; the SAFETY is that the
  // endpoint does not exist, and this field cannot make it exist.
  teardown_allowed: boolean;
  // Scenarios currently applying. They have no record yet, so they
  // cannot appear in `deployments` -- the estate page renders this to
  // avoid being silent about the most active thing in the estate.
  // Optional for the same reason `already_live` is, and the `?` is
  // load-bearing: the estate page guards it with `Array.isArray` and
  // `knownEmpty` takes a `deployingKnown` term, because reading an
  // absent field as "nothing is applying" licenses the page's only
  // permitted emptiness claim. Declaring it required makes that guard
  // provably dead and invites its deletion.
  deploying?: string[];
}

export interface ActionStep {
  stage: string;
  status: string;
  detail?: string;
}

export interface ActionResult {
  // Its own field, not the absence of failures: ADR-0024's rule is that
  // a teardown which cannot PROVE the account clean must not report
  // success.
  clean: boolean;
  steps: ActionStep[];
  failures: ActionStep[];
}

export interface CostComponent {
  name: string;
  count: number;
  eur_per_hour: number;
  // Free and unknown are different facts. `false` means this project has
  // no list price for the shape, not that it is free.
  priced: boolean;
  source?: string;
}

export interface CostEstimate {
  components: CostComponent[];
  eur_per_hour: number;
  unpriced: string[];
  // Every component priced. When false, eur_per_hour is a LOWER BOUND.
  complete: boolean;
  // False when this scenario's resource shape is not modelled at all —
  // an empty component list then means "unknown", not "nothing".
  modelled: boolean;
}

export interface DeployPreview {
  scenario: string;
  cloud?: string;
  deployable: boolean;
  reason?: string;
  image?: string;
  ttl?: string;
  ttl_seconds?: number;
  // null when the scenario cannot be deployed, rather than a zero time
  // that would render as a date in the year 1.
  expires_at: string | null;
  expires_at_wall_clock?: string;
  cost: CostEstimate;
  cost_summary?: string;
  internet_facing: boolean;
  deploy_allowed: boolean;
  // Deployments of this scenario that are already running. The lock
  // stops the accidental duplicate; this is what warns about the
  // deliberate one.
  // OPTIONAL, and the `?` is load-bearing. `alreadyLiveWarnings`
  // treats an absent list as "we could not look" rather than as "there
  // is nothing" -- an older server, or a body trimmed by an
  // intermediary, must not be able to produce a silent all-clear on a
  // guard about billable infrastructure. Declaring it required would
  // make that branch provably dead and invite its deletion.
  already_live?: string[];
  // Applying right now, which the estate cannot see: registration runs
  // after the apply returns.
  // Optional for the same wire reason as its siblings: a server that
  // predates this field simply omits it. Unlike `already_live` it has
  // no unknown state to fall back to -- the server cannot see an apply
  // started by the CLI or by another process either, so absence and
  // "nothing is applying here" are already indistinguishable, and that
  // limit is stated in `InFlight`'s docstring and in ADR-0027.
  already_deploying?: boolean;
  // True when the estate could not be fully read. An empty already_live
  // is a claim; this says when it cannot be made.
  already_live_unknown?: boolean;
}
