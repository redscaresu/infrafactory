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
  constraints?: Record<string, unknown>;
  criteria: Array<Record<string, unknown>>;
}

export interface StartRunOptions {
  clean?: boolean;
  no_destroy?: boolean;
  layer3_enabled?: boolean;
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
  config_default_enabled: boolean;
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
