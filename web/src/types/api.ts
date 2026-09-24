export type TimeWindow = '1h' | '24h' | '7d' | '30d';

export interface OverviewProviderItem {
  name: string;
  dialect: string;
  status: 'healthy' | 'cooldown';
  cooldown_until?: string;
  total_requests: number;
  success_rate: number;
}

export interface ModelUsageStats {
  model: string;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
}

export interface TrafficBucket {
  timestamp: number;
  count: number;
  time_label?: string;
}

export interface OverviewData {
  active_window: string;
  total_requests: number;
  success_requests: number;
  success_rate: number;
  total_prompt_tokens: number;
  total_completion_tokens: number;
  avg_latency_ms: number;
  provider_traffic_list: OverviewProviderItem[];
  top_models: ModelUsageStats[];
  buckets: TrafficBucket[];
  recent_sessions: HistoryRecord[];
}

export interface ProviderAccount {
  name: string;
  type: string;
  status: string;
  expires_at?: string;
  masked_token?: string;
  affected_combo_count?: number;
}

export type ProviderStatus = 'connected' | 'cooldown' | 'awaiting_credentials' | 'not_connected';

export interface ProviderItem {
  name: string;
  display_name?: string;
  logo?: string;
  dialect: string;
  base_url: string;
  api_key?: string;
  configured: boolean;
  in_topology?: boolean;
  models: string[];
  accounts: ProviderAccount[];
  connection_count?: number;
  status?: ProviderStatus;
  in_cooldown: boolean;
  cooldown_remaining_sec?: number;
  oauth_capable?: boolean;
  tier?: string;
  free_note?: string;
  section?: string;
}

export interface CatalogModel {
  id: string;
  name: string;
  whitelisted: boolean;
}

export interface ProviderDetail {
  name: string;
  display_name: string;
  logo: string;
  dialect: string;
  base_url: string;
  configured: boolean;
  in_topology: boolean;
  oauth_capable: boolean;
  health_status: string;
  status: ProviderStatus;
  connection_count: number;
  connections: ProviderAccount[];
  models: CatalogModel[];
  whitelisted_models: CatalogModel[];
  available_models: CatalogModel[];
}

export interface PresetItem {
  name: string;
  display_name: string;
  dialect: string;
  default_base_url: string;
  models: string[];
  logo?: string;
  tier?: string;
  free_note?: string;
  oauth_capable?: boolean;
  description: string;
  section: string;
}

export interface ComboItem {
  name: string;
  mode: 'ordered' | 'pool' | 'fused';
  members: string[];
  capabilities: string[];
  enabled: boolean;
}

export interface HistoryAttempt {
  provider: string;
  model: string;
  status: number;
  elapsed_ms: number;
  error?: string;
}

export interface HistoryRecord {
  id: string;
  timestamp: number;
  time_formatted?: string;
  session_id: string;
  key_id: string;
  provider: string;
  endpoint: string;
  model_requested: string;
  model_served: string;
  outcome: string;
  status_code: number;
  status_variant?: 'success' | 'warning' | 'error' | 'neutral';
  latency_ms: number;
  input_tokens: number;
  output_tokens: number;
  cache_read_tokens?: number;
  cache_creation_tokens?: number;
  total_tokens?: number;
  has_request_body: boolean;
  has_response_body: boolean;
  request_body?: string;
  translated_request_body?: string;
  raw_response_body?: string;
  response_body?: string;
  request_body_truncated?: boolean;
  translated_req_truncated?: boolean;
  raw_resp_truncated?: boolean;
  response_body_truncated?: boolean;
  attempts?: HistoryAttempt[];
}

export interface KeyItem {
  id: string;
  name: string;
  prefix: string;
  masked: string;
  secret?: string;
  rate_spec: string;
  rate_limit_rpm?: number;
  rate_limit_rpd?: number;
  created_at: number;
  expires?: number;
  expires_formatted?: string;
  last_used_at?: number;
  last_used_formatted?: string;
  is_active: boolean;
  is_expired?: boolean;
  is_revoked: boolean;
}

export interface ClientModelSlot {
  id: string;
  name: string;
  kind: number; // 0 = single, 1 = multi
  required: boolean;
}

export interface ClientItem {
  id: string;
  name: string;
  dialect: string;
  category: string;
  logo: string;
  status_state: 'connected' | 'not_configured' | 'not_installed';
  status_label: string;
  installed: boolean;
  configured: boolean;
  config_path: string;
  current_base_url?: string;
  default_base_url?: string;
  endpoints?: { url: string; is_default: boolean; is_current?: boolean }[];
  masked_key?: string;
  raw_key?: string;
  routable_models?: string[];
  model_slots?: ClientModelSlot[];
  slot_values?: Record<string, string>;
  existing_keys?: { id: string; name: string; prefix: string }[];
  selected_key_id?: string;
  manual_snippet?: string;
}

export interface ServerSettings {
  listen: string;
  config_path: string;
  keys_path: string;
  history_db_path: string;
  log_level: string;
  capture_mode: string;
  is_default_password: boolean;
  tls_enabled: boolean;
  version: string;
}
