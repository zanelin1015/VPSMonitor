export interface VPSSummary {
  hostname?: string
  observed_ip?: string
  public_ipv4?: string
  public_ipv6?: string
  cpu?: number
  mem_used?: number
  mem_total?: number
  net_traffic_sent?: number
  net_traffic_recv?: number
  net_traffic_total?: number
  net_io_up?: number
  net_io_down?: number
  xray_state?: string
  inbound_count?: number
  outbound_count?: number
  routing_rule_count?: number
  last_collection_err?: string
}

export interface AgentListItem {
  agent_id: string
  agent_name?: string
  customer_display_name?: string
  client_version?: string
  client_os?: string
  client_arch?: string
  system_version?: string
  sort_order?: number
  tags?: string[]
  renewal?: VPSRenewalConfig
  entry?: AgentEntryConfig
  reported_at?: string
  realtime_at?: string
  registered_at?: string
  updated_at?: string
  last_seen_at?: string
  has_config: boolean
  summary: VPSSummary
  geo?: IPGeoView
}

export interface AgentLogEntry {
  time: string
  level?: string
  source?: string
  message: string
}

export interface AgentLogsResponse {
  agent_id: string
  reported_at?: string
  last_collection_err?: string
  logs: AgentLogEntry[]
}

export interface VPSRenewalConfig {
  enabled?: boolean
  start_date?: string
  expire_date?: string
  cycle?: 'week' | 'month' | 'quarter' | 'year' | ''
  auto_renew?: boolean
  cost_amount?: number
  cost_currency?: string
  cost_cycle?: 'month' | 'quarter' | 'year' | ''
  client_billings?: XUIClientBillingConfig[]
  traffic_limit_bytes?: number
  bandwidth_mbps?: number
  traffic_baseline_bytes?: number
  traffic_sent_baseline_bytes?: number
  traffic_recv_baseline_bytes?: number
  traffic_baseline_period_start?: string
}

export interface XUIClientBillingConfig {
  inbound_id?: number
  inbound_tag?: string
  email?: string
  revenue_amount?: number
  revenue_currency?: 'CNY' | 'USDT' | ''
  revenue_cycle?: 'month' | 'quarter' | 'year' | ''
  expire_time?: number
  expire_cycle?: 'month' | 'quarter' | 'year' | ''
  expire_auto_renew?: boolean
}

export interface XUIConfig {
  enabled: boolean
  base_url?: string
  db_path?: string
  username?: string
  password?: string
  api_token?: string
  two_factor_code?: string
  skip_tls_verify: boolean
}

export interface ManagedAgentConfig {
  agent_id?: string
  agent_name?: string
  customer_display_name?: string
  sort_order?: number
  tags?: string[]
  renewal?: VPSRenewalConfig
  entry?: AgentEntryConfig
  xui: XUIConfig
}

export interface AgentEntryConfig {
  addresses?: string[]
  import_domain?: string
  mappings?: AgentEntryMapping[]
}

export interface AgentEntryMapping {
  address?: string
  external_port?: number
  internal_port?: number
  protocol?: 'vless' | 'vmess' | 'http' | 'socks' | ''
  note?: string
}

export interface AdminUser {
  id?: number
  username: string
  display_name?: string
  avatar_url?: string
  role?: 'admin' | 'area_manager'
  agent_ids?: string[]
  updated_at: string
}

export interface AdminAuthResponse {
  user: AdminUser
  system?: SystemInfo
}

export interface CustomerUser {
  id: number
  username: string
  display_name?: string
  style_code?: string
  owner_type?: string
  owner_id?: number
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface CustomerAuthResponse {
  user: CustomerUser
}

export interface CustomerAssignment {
  id: number
  customer_id: number
  agent_id: string
  inbound_id: number
  inbound_tag?: string
  client_email?: string
  public_client_name?: string
  remark?: string
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface AreaManagerAssignment {
  id: number
  manager_id: number
  agent_id: string
  inbound_id: number
  inbound_tag?: string
  client_email?: string
  public_client_name?: string
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface CustomerAssignmentDraft {
  agent_id: string
  inbound_id: number
  inbound_tag?: string
  client_email?: string
  public_client_name?: string
  revenue_amount?: number
  revenue_currency?: 'CNY' | 'USDT'
  revenue_cycle?: 'month' | 'quarter' | 'year'
}

export interface CustomerAdminView extends CustomerUser {
  assignments: CustomerAssignment[]
}

export interface AreaManagerAdminView {
  id: number
  username: string
  display_name?: string
  enabled: boolean
  agent_ids?: string[]
  assignments?: AreaManagerAssignment[]
  customers?: CustomerAdminView[]
  created_at: string
  updated_at: string
}

export interface CustomerLinkStep {
  role: string
  label: string
  country_code?: string
  country_name?: string
  exit_ip?: string
}

export interface CustomerLinkView {
  assignment_id: number
  entry_client_name: string
  inbound_tag?: string
  client_email?: string
  client_remark?: string
  import_url?: string
  remark?: string
  summary: string
  exit_country_code?: string
  exit_country_name?: string
  exit_ip?: string
  resolved: boolean
  unresolved_reason?: string
  steps: CustomerLinkStep[]
  updated_at: string
}

export interface CustomerOverviewResponse {
  user: CustomerUser
  generated_at: string
  links: CustomerLinkView[]
}

export interface SystemInfo {
  role: string
  version: string
  build_time?: string
  git_commit?: string
  go_version?: string
  platform?: string
}

export interface UpdateAgentStatus {
  agent_id: string
  agent_name?: string
  version?: string
  os?: string
  arch?: string
  package_name?: string
  supported: boolean
  update_available: boolean
  reason?: string
}

export interface UpdateLatestInfo {
  repo: string
  package_prefix: string
  current_server_version: string
  latest_version: string
  latest_tag: string
  latest_server_version?: string
  latest_server_tag?: string
  latest_client_version?: string
  latest_client_tag?: string
  latest_3xui_version?: string
  latest_3xui_tag?: string
  latest_3xui_error?: string
  server_update_available: boolean
  client_update_available_count: number
  xui_update_available_count: number
  supported_client_count: number
  unknown_client_count: number
  unsupported_client_count: number
  supported_xui_count: number
  unknown_xui_count: number
  unsupported_xui_count: number
  assets?: string[]
  server_assets?: string[]
  client_assets?: string[]
  agent_status?: UpdateAgentStatus[]
  xui_agent_status?: UpdateAgentStatus[]
  fetched_at: string
}

export interface UpdateResponse {
  status: string
  count?: number
  skipped?: number
  latest?: UpdateLatestInfo
  agent_status?: UpdateAgentStatus[]
}


export interface ExchangeRatesResponse {
  base: string
  date: string
  rates: Record<string, number>
  source?: string
  fetched_at?: string
  stale?: boolean
  error?: string
}

export interface ClientInstallInfo {
  server_url: string
  registration_token: string
  install_script_url: string
  poll_interval: string
  request_timeout_seconds: number
  server_skip_tls_verify: boolean
}

export interface TagSettingsResponse {
  tags: string[]
}

export interface AreaAgentTagsResponse {
  agent_id: string
  tags: string[]
}

export interface FrontendSettings {
  custom_code: string
}

export interface TelegramBot {
  id: number
  name: string
  chat_id: string
  enabled: boolean
  has_bot_token: boolean
  created_at: string
  updated_at: string
}

export interface ConfigAuditLog {
  id: number
  agent_id: string
  actor: string
  before?: unknown
  after?: unknown
  created_at: string
}

export interface XUIRouteTrace {
  match_scope: string
  rule_index?: number
  outbound_tag?: string
  balancer_tag?: string
  has_global_rules?: boolean
  global_rule_indexes?: number[]
  note?: string
}

export interface XUINodeView {
  id: number
  tag?: string
  remark?: string
  protocol?: string
  listen?: string
  port?: number
  network?: string
  security?: string
  tls_server_name?: string
  alpn?: string
  ws_path?: string
  ws_host?: string
  grpc_service?: string
  reality_public_key?: string
  reality_short_id?: string
  reality_fingerprint?: string
  reality_spider_x?: string
  enabled: boolean
  expiry_time?: number
  up?: number
  down?: number
  total?: number
  all_time?: number
  client_count?: number
  online_count?: number
  route: XUIRouteTrace
}

export interface XUIClientView {
  inbound_id: number
  inbound_tag?: string
  inbound_remark?: string
  protocol?: string
  email?: string
  comment?: string
  enabled: boolean
  auth_uuid?: string
  auth_password?: string
  flow?: string
  import_url?: string
  limit_ip?: number
  total_gb?: number
  expiry_time?: number
  sub_id?: string
  created_at?: number
  updated_at?: number
  up?: number
  down?: number
  all_time?: number
  traffic_total?: number
  last_online?: number
  route: XUIRouteTrace
}

export interface XUIOutboundView {
  tag?: string
  protocol?: string
  target?: string
  address?: string
  port?: number
  send_through?: string
  network?: string
  security?: string
  tls_server_name?: string
  ws_path?: string
  ws_host?: string
  up?: number
  down?: number
  total?: number
  is_default?: boolean
}

export interface XUIBalancerView {
  tag?: string
  selectors?: string[]
  strategy?: string
  fallback_tag?: string
  outbound_tags?: string[]
}

export interface XUIRoutingRuleView {
  index: number
  type?: string
  inbound_tags?: string[]
  users?: string[]
  outbound_tag?: string
  balancer_tag?: string
  domain?: string[]
  ip?: string[]
  port?: string[]
  source_port?: string[]
  source_ip?: string[]
  network?: string[]
  protocol?: string[]
  vless_route?: string[]
  summary?: string
}

export interface XUILocalCertificate {
  id: string
  name?: string
  subject?: string
  issuer?: string
  dns_names?: string[]
  cert_path?: string
  key_path?: string
  source_dir?: string
  not_after?: string
}

export interface XUIOverview {
  agent_id: string
  agent_name?: string
  base_url?: string
  reported_at: string
  collected_at: string
  summary: VPSSummary
  node_count: number
  client_count: number
  online_client_count: number
  nodes: XUINodeView[]
  clients: XUIClientView[]
  outbounds: XUIOutboundView[]
  balancers?: XUIBalancerView[]
  routing_rules: XUIRoutingRuleView[]
  certificates: XUILocalCertificate[]
}

export interface XUIAction {
  id: number
  agent_id: string
  kind: string
  status: string
  payload?: Record<string, unknown>
  result?: Record<string, unknown>
  error?: string
  created_at: string
  updated_at: string
  claimed_at?: string
  completed_at?: string
}

export interface DashboardTotals {
  agent_count: number
  tagged_agent_count: number
  node_count: number
  client_count: number
  online_client_count: number
  outbound_count: number
  routing_rule_count: number
  link_count: number
  chain_count: number
}

export interface DashboardTagView {
  tag: string
  agent_count: number
  node_count: number
  client_count: number
  online_client_count: number
}

export interface DashboardAgentView extends AgentListItem {
  node_count: number
  client_count: number
  online_client_count: number
  outbound_count: number
  routing_rule_count: number
}

export interface AgentRealtimeMetrics {
  agent_id: string
  agent_name?: string
  client_version?: string
  client_os?: string
  client_arch?: string
  system_version?: string
  reported_at?: string
  summary: VPSSummary
}

export interface DashboardRealtimeMessage {
  type: 'snapshot' | 'metrics'
  metrics?: AgentRealtimeMetrics[]
  metric?: AgentRealtimeMetrics
}

export interface AgentRefreshResponse {
  status: string
  mode: 'websocket'
  message?: string
}

export interface TopologyInboundRef {
  agent_id: string
  agent_name?: string
  agent_tags?: string[]
  inbound_id: number
  inbound_tag?: string
  inbound_name?: string
  protocol?: string
  port?: number
  network?: string
  security?: string
  ws_path?: string
  ws_host?: string
  domains?: string[]
  ips?: string[]
  resolved_ips?: string[]
  entry_addresses?: string[]
  entry_ips?: string[]
  entry_mappings?: TopologyEntryMapping[]
}

export interface TopologyEntryMapping {
  address?: string
  external_port?: number
  internal_port?: number
  protocol?: string
  note?: string
  resolved_ips?: string[]
}

export interface TopologyOutboundRef {
  agent_id: string
  agent_name?: string
  agent_tags?: string[]
  outbound_tag?: string
  protocol?: string
  target?: string
  address?: string
  port?: number
  network?: string
  security?: string
  tls_server_name?: string
  ws_path?: string
  ws_host?: string
  resolved_ips?: string[]
  target_ip?: string
  target_geo?: IPGeoView
}

export interface TopologyLinkView {
  key: string
  source: TopologyOutboundRef
  target: TopologyInboundRef
  match_score: number
  match_confidence?: string
  match_reason?: string
  match_explanation?: string
  match_fields?: string[]
}

export interface ClientChainStep {
  step_type: string
  agent_id: string
  agent_name?: string
  agent_tags?: string[]
  label: string
  detail?: string
  protocol?: string
  port?: number
  route_scope?: string
  rule_index?: number
  outbound_tag?: string
  target?: string
  target_ip?: string
  target_geo?: IPGeoView
  match_reason?: string
}

export interface IPGeoView {
  ip?: string
  country_code?: string
  country_name?: string
  region_name?: string
  city?: string
}

export interface ClientChainView {
  key: string
  root_agent_id: string
  root_agent_name?: string
  root_agent_tags?: string[]
  root_client_email?: string
  root_client_remark?: string
  root_inbound_tag?: string
  matched_link_count: number
  loop_detected?: boolean
  unresolved_reason?: string
  steps: ClientChainStep[]
}

export interface GlobalDashboardView {
  generated_at: string
  totals: DashboardTotals
  tags: DashboardTagView[]
  agents: DashboardAgentView[]
  links: TopologyLinkView[]
  client_chains: ClientChainView[]
}
