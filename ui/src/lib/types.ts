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
  last_run?: {
    run_id: string;
    status: string;
    terminal_reason?: string;
  };
}

export interface ScenarioGroup {
  name: string;
  scenarios: ScenarioItem[];
}

export interface ScenarioDetail {
  name: string;
  path: string;
  description: string;
  raw_yaml: string;
  resources: Record<string, unknown>;
  constraints?: Record<string, unknown>;
  criteria: Array<Record<string, unknown>>;
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
