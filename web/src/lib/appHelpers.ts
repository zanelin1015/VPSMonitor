import type {
  AgentEntryConfig,
  AgentEntryMapping,
  AgentListItem,
  AgentRealtimeMetrics,
  ClientChainView,
  ClientInstallInfo,
  ConfigAuditLog,
  DashboardAgentView,
  DashboardTagView,
  FrontendSettings,
  GlobalDashboardView,
  IPGeoView,
  ManagedAgentConfig,
  TopologyLinkView,
  VPSRenewalConfig,
  VPSSummary,
  XUIClientBillingConfig,
  XUIClientView,
  XUIOverview,
  XUIRoutingRuleView,
} from '../types'
import { DEFAULT_COST_CURRENCY, normalizeCurrencyCode } from './currency'
import { clampMetricPercent, formatMem, formatPercent } from './traffic'

export const DASHBOARD_AUTO_REFRESH_MS = 20_000
export const XUI_ACTION_KINDS = [
  { value: 'add_client', label: '节点下新增客户端' },
  { value: 'upsert_routing_rule', label: '新增 / 修改转发规则' },
]

const AGENT_VIEW_MODE_STORAGE_PREFIX = 'bridge-core.agent-view-mode.'
const TAG_COLOR_PALETTE = [
  { bg: '#dbeafe', border: '#93c5fd', text: '#1d4ed8' },
  { bg: '#dcfce7', border: '#86efac', text: '#15803d' },
  { bg: '#fef3c7', border: '#fcd34d', text: '#b45309' },
  { bg: '#fce7f3', border: '#f9a8d4', text: '#be185d' },
  { bg: '#ede9fe', border: '#c4b5fd', text: '#6d28d9' },
  { bg: '#ccfbf1', border: '#5eead4', text: '#0f766e' },
  { bg: '#fee2e2', border: '#fca5a5', text: '#b91c1c' },
  { bg: '#e0f2fe', border: '#7dd3fc', text: '#0369a1' },
]

export type AgentViewMode = 'card' | 'list'
export type ConfigSectionKey = 'client' | 'renewal' | 'xui' | 'entry'
export type ClientInstallCommandKind = 'linux' | 'windows-powershell' | 'windows-cmd'

export interface TLSCertificateSelectionForm {
  mode: 'none' | 'domain_auto' | 'inventory' | 'manual'
  inventory_id: string
  domain: string
  certificate_file: string
  key_file: string
}

export interface XUIInboundClientForm {
  email: string
  uuid: string
  password: string
  flow: string
  limit_ip: number
  total_gb: number
  expiry_days: number
  comment: string
  sub_id: string
  enabled: boolean
}

export interface XUIInboundActionForm {
  remark: string
  tag: string
  enabled: boolean
  listen: string
  port: number
  protocol: string
  transport: string
  security: string
  server_name: string
  ws_path: string
  ws_host: string
  sniffing: boolean
  tls: TLSCertificateSelectionForm
  clients: XUIInboundClientForm[]
  restart: boolean
}

export interface XUIOutboundActionForm {
  tag: string
  protocol: string
  send_through: string
  address: string
  port: number
  uuid: string
  flow: string
  password: string
  method: string
  security: string
  server_name: string
  alpn: string
  grpc_service: string
  reality_public_key: string
  reality_short_id: string
  reality_fingerprint: string
  reality_spider_x: string
  network: string
  ws_path: string
  ws_host: string
  source_agent_id: string
  source_client_key: string
  restart: boolean
}

export interface XUIRoutingActionForm {
  rule_index: number | null
  target_mode: 'existing_outbound' | 'registered_client'
  outbound_tag: string
  balancer_tag: string
  inbound_tags: string[]
  users: string[]
  domains: string
  ips: string
  ports: string
  source_ips: string
  source_ports: string
  networks: string[]
  protocols: string[]
  restart: boolean
}

export interface XUIAddClientActionForm {
  inbound_id: number
  inbound_tag: string
  inbound_name: string
  protocol: string
  client: XUIInboundClientForm
  restart: boolean
}

export interface TelegramBotForm {
  name: string
  bot_token: string
  chat_id: string
  enabled: boolean
}

export interface ClientInstallCommandForm {
  server_url: string
  registration_token: string
  install_script_url: string
  poll_interval: string
  request_timeout_seconds: number
  server_skip_tls_verify: boolean
}

export interface FrontendSettingsForm {
  custom_code: string
}

class APIError extends Error {
  status: number

  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

async function fetchJSON<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, { ...init, credentials: 'same-origin' })
  if (!response.ok) {
    let detail = response.statusText
    try {
      const payload = (await response.json()) as { error?: string }
      if (payload.error) {
        detail = payload.error
      }
    } catch {
      // ignore invalid json
    }
    throw new APIError(response.status, detail || `request failed: ${response.status}`)
  }
  return (await response.json()) as T
}

function buildDashboardRealtimeURL(): string {
  const url = new URL('/api/v1/dashboard/realtime', window.location.href)
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  return url.toString()
}

function buildAgentTerminalURL(agentID: string, shell: string, cols = 120, rows = 36): string {
  const url = new URL(`/api/v1/agents/${encodeURIComponent(agentID)}/terminal/ws`, window.location.href)
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  if (shell) {
    url.searchParams.set('shell', shell)
  }
  url.searchParams.set('cols', String(cols))
  url.searchParams.set('rows', String(rows))
  return url.toString()
}

function mergeRealtimeMetricsIntoAgents<T extends AgentListItem>(agents: T[], metrics: AgentRealtimeMetrics[]): T[] {
  if (!metrics.length) {
    return agents
  }
  const byAgent = new Map(metrics.map((metric) => [metric.agent_id, metric]))
  let changed = false
  const next = agents.map((agent) => {
    const metric = byAgent.get(agent.agent_id)
    if (!metric) {
      return agent
    }
    changed = true
    return {
      ...agent,
      agent_name: agent.agent_name || metric.agent_name,
      client_version: metric.client_version || agent.client_version,
      client_os: metric.client_os || agent.client_os,
      client_arch: metric.client_arch || agent.client_arch,
      system_version: metric.system_version || agent.system_version,
      realtime_at: metric.reported_at || agent.realtime_at,
      summary: mergeRealtimeSummary(agent.summary, metric.summary),
    }
  })
  return changed ? next : agents
}

function sortAgentsByOrder<T extends AgentListItem>(agents: T[]): T[] {
  return [...agents].sort((left, right) => {
    const leftOrder = Number(left.sort_order || 0)
    const rightOrder = Number(right.sort_order || 0)
    if (leftOrder > 0 || rightOrder > 0) {
      if (leftOrder <= 0) return 1
      if (rightOrder <= 0) return -1
      if (leftOrder !== rightOrder) return leftOrder - rightOrder
    }
    const leftRegistered = Date.parse(left.registered_at || '')
    const rightRegistered = Date.parse(right.registered_at || '')
    if (!Number.isNaN(leftRegistered) || !Number.isNaN(rightRegistered)) {
      if (Number.isNaN(leftRegistered)) return 1
      if (Number.isNaN(rightRegistered)) return -1
      if (leftRegistered !== rightRegistered) return leftRegistered - rightRegistered
    }
    return left.agent_id.localeCompare(right.agent_id)
  })
}

function mergeRealtimeSummary(current: VPSSummary, realtime: VPSSummary): VPSSummary {
  return {
    ...current,
    hostname: realtime.hostname || current.hostname,
    observed_ip: realtime.observed_ip || current.observed_ip,
    public_ipv4: realtime.public_ipv4 || current.public_ipv4,
    public_ipv6: realtime.public_ipv6 || current.public_ipv6,
    cpu: realtime.cpu ?? current.cpu,
    mem_used: realtime.mem_total ? realtime.mem_used : current.mem_used,
    mem_total: realtime.mem_total || current.mem_total,
    net_traffic_sent: realtime.net_traffic_sent ?? current.net_traffic_sent,
    net_traffic_recv: realtime.net_traffic_recv ?? current.net_traffic_recv,
    net_traffic_total: realtime.net_traffic_total ?? current.net_traffic_total,
    net_io_up: realtime.net_io_up ?? current.net_io_up,
    net_io_down: realtime.net_io_down ?? current.net_io_down,
    xray_state: realtime.xray_state || current.xray_state,
  }
}

function isUnauthorized(error: unknown): boolean {
  return error instanceof APIError && error.status === 401
}

function defaultTLSCertificateSelection(): TLSCertificateSelectionForm {
  return {
    mode: 'none',
    inventory_id: '',
    domain: '',
    certificate_file: '',
    key_file: '',
  }
}

function defaultInboundClientForm(): XUIInboundClientForm {
  return {
    email: 'user@example.com',
    uuid: '',
    password: '',
    flow: '',
    limit_ip: 0,
    total_gb: 0,
    expiry_days: 0,
    comment: '',
    sub_id: '',
    enabled: true,
  }
}

function defaultInboundActionForm(): XUIInboundActionForm {
  return {
    remark: 'vless-auto-443',
    tag: '',
    enabled: true,
    listen: '',
    port: 443,
    protocol: 'vless',
    transport: 'tcp',
    security: 'none',
    server_name: '',
    ws_path: '/',
    ws_host: '',
    sniffing: true,
    tls: defaultTLSCertificateSelection(),
    clients: [defaultInboundClientForm()],
    restart: true,
  }
}

function defaultOutboundActionForm(): XUIOutboundActionForm {
  return {
    tag: 'relay-hk',
    protocol: 'freedom',
    send_through: '',
    address: '',
    port: 443,
    uuid: '',
    flow: '',
    password: '',
    method: 'aes-256-gcm',
    security: 'none',
    server_name: '',
    alpn: '',
    grpc_service: '',
    reality_public_key: '',
    reality_short_id: '',
    reality_fingerprint: '',
    reality_spider_x: '',
    network: 'tcp',
    ws_path: '/',
    ws_host: '',
    source_agent_id: '',
    source_client_key: '',
    restart: true,
  }
}

function defaultRoutingActionForm(): XUIRoutingActionForm {
  return {
    rule_index: null,
    target_mode: 'existing_outbound',
    outbound_tag: '',
    balancer_tag: '',
    inbound_tags: [],
    users: [],
    domains: '',
    ips: '',
    ports: '',
    source_ips: '',
    source_ports: '',
    networks: [],
    protocols: [],
    restart: true,
  }
}

function defaultAddClientActionForm(): XUIAddClientActionForm {
  return {
    inbound_id: 0,
    inbound_tag: '',
    inbound_name: '',
    protocol: 'vless',
    client: defaultInboundClientForm(),
    restart: true,
  }
}

function defaultTelegramBotForm(): TelegramBotForm {
  return {
    name: '',
    bot_token: '',
    chat_id: '',
    enabled: true,
  }
}

function defaultClientInstallCommandForm(): ClientInstallCommandForm {
  return {
    server_url: typeof window !== 'undefined' ? window.location.origin : 'http://SERVER_IP:8090',
    registration_token: '',
    install_script_url: 'https://raw.githubusercontent.com/zanelin1015/VPSMonitor/main/install.sh',
    poll_interval: '30s',
    request_timeout_seconds: 15,
    server_skip_tls_verify: false,
  }
}

function normalizeClientInstallCommandForm(info: ClientInstallInfo): ClientInstallCommandForm {
  return {
    server_url: info.server_url || defaultClientInstallCommandForm().server_url,
    registration_token: info.registration_token || '',
    install_script_url: info.install_script_url || defaultClientInstallCommandForm().install_script_url,
    poll_interval: info.poll_interval || '30s',
    request_timeout_seconds: Number(info.request_timeout_seconds || 15),
    server_skip_tls_verify: Boolean(info.server_skip_tls_verify),
  }
}

function defaultFrontendSettingsForm(): FrontendSettingsForm {
  return { custom_code: '' }
}

function normalizeFrontendSettingsForm(settings: FrontendSettings): FrontendSettingsForm {
  return { custom_code: settings.custom_code || '' }
}

function clientInstallCommandByKind(
  kind: ClientInstallCommandKind,
  commands: { linux: string; windowsPowerShell: string; windowsCMD: string },
): string {
  switch (kind) {
    case 'windows-powershell':
      return commands.windowsPowerShell
    case 'windows-cmd':
      return commands.windowsCMD
    case 'linux':
    default:
      return commands.linux
  }
}

function buildClientInstallCommand(form: ClientInstallCommandForm): string {
  const scriptURL = form.install_script_url.trim() || defaultClientInstallCommandForm().install_script_url
  const envValues: Array<[string, string]> = [
    ['VPSMONITOR_SERVER_URL', form.server_url.trim()],
    ['VPSMONITOR_REGISTRATION_TOKEN', form.registration_token.trim()],
    ['VPSMONITOR_SERVER_SKIP_TLS_VERIFY', String(Boolean(form.server_skip_tls_verify))],
    ['VPSMONITOR_POLL_INTERVAL', form.poll_interval.trim() || '30s'],
    ['VPSMONITOR_REQUEST_TIMEOUT_SECONDS', String(Math.max(1, Number(form.request_timeout_seconds || 15)))],
    ['VPSMONITOR_ASSUME_YES', 'true'],
  ]
  const envText = envValues.map(([key, value]) => `${key}=${shellQuote(value)}`).join(' ')
  return `curl -L ${shellQuote(scriptURL)} -o vpsmonitor-install.sh && chmod +x vpsmonitor-install.sh && env ${envText} ./vpsmonitor-install.sh client`
}

function buildWindowsPowerShellInstallCommand(form: ClientInstallCommandForm): string {
  const scriptURL = windowsInstallScriptURL(form.install_script_url)
  const envValues: Array<[string, string]> = [
    ['VPSMONITOR_SERVER_URL', form.server_url.trim()],
    ['VPSMONITOR_REGISTRATION_TOKEN', form.registration_token.trim()],
    ['VPSMONITOR_SERVER_SKIP_TLS_VERIFY', String(Boolean(form.server_skip_tls_verify))],
    ['VPSMONITOR_POLL_INTERVAL', form.poll_interval.trim() || '30s'],
    ['VPSMONITOR_REQUEST_TIMEOUT_SECONDS', String(Math.max(1, Number(form.request_timeout_seconds || 15)))],
    ['VPSMONITOR_ASSUME_YES', 'true'],
  ]
  const envText = envValues.map(([key, value]) => `$env:${key}=${powerShellQuote(value)}`).join('; ')
  return `${envText}; $script=Join-Path $env:TEMP 'vpsmonitor-install.ps1'; Remove-Item -Force $script -ErrorAction SilentlyContinue; iwr -UseBasicParsing -Headers @{'Cache-Control'='no-cache'} ${powerShellQuote(scriptURL)} -OutFile $script; Select-String -Path $script -Pattern 'InstallerVersion' | Write-Host; powershell -NoProfile -ExecutionPolicy Bypass -File $script client`
}

function buildWindowsCMDInstallCommand(form: ClientInstallCommandForm): string {
  return `powershell -NoProfile -ExecutionPolicy Bypass -Command ${powerShellQuote(buildWindowsPowerShellInstallCommand(form))}`
}

function windowsInstallScriptURL(scriptURL: string): string {
  const value = (scriptURL || defaultClientInstallCommandForm().install_script_url).trim()
  const psURL = value.endsWith('.sh') ? `${value.slice(0, -3)}.ps1` : value
  if (psURL.includes('raw.githubusercontent.com') && !psURL.includes('?')) {
    return `${psURL}?v=2026050902`
  }
  return psURL
}

function shellQuote(value: string): string {
  return `'${value.replace(/'/g, `'\\''`)}'`
}

function powerShellQuote(value: string): string {
  return `'${value.replace(/'/g, `''`)}'`
}

function buildXUIActionPayload(
  kind: string,
  forms: {
    addClient: XUIAddClientActionForm
    outbound: XUIOutboundActionForm
    routing: XUIRoutingActionForm
  },
): Record<string, unknown> {
  switch (kind) {
    case 'add_client':
      return buildAddClientActionPayload(forms.addClient)
    case 'upsert_routing_rule':
      return buildUpsertRoutingActionPayload(forms.routing, forms.outbound)
    case 'add_routing_rule':
      return buildRoutingActionPayload(forms.routing)
    case 'add_outbound':
    default:
      return buildOutboundActionPayload(forms.outbound)
  }
}

function buildAddClientActionPayload(form: XUIAddClientActionForm): Record<string, unknown> {
  if (!form.inbound_id && !form.inbound_tag.trim()) {
    throw new Error('请选择要新增客户端的节点')
  }
  const protocol = normalizeOutboundProtocol(form.protocol || 'vless')
  const client = buildInboundClientPayload(form.client, protocol, 0)
  const email = String(client.email || '').trim()
  if (!email) {
    throw new Error('客户端邮箱 / 标识不能为空')
  }
  return {
    inbound_id: form.inbound_id,
    inbound_tag: form.inbound_tag.trim(),
    protocol,
    client,
    restart: form.restart,
  }
}

function buildUpsertRoutingActionPayload(form: XUIRoutingActionForm, outboundForm: XUIOutboundActionForm): Record<string, unknown> {
  const payload = buildRoutingActionPayload(form)
  if (form.rule_index && form.rule_index > 0) {
    payload.rule_index = form.rule_index
  }
  if (form.target_mode === 'registered_client') {
    const outboundPayload = buildOutboundActionPayload(outboundForm)
    payload.outbound = outboundPayload.outbound
    const outboundTag = String((outboundPayload.outbound as Record<string, unknown>)?.tag || '')
    if (!outboundTag) {
      throw new Error('未能生成内部 Client 出站标签')
    }
    const rule = payload.rule as Record<string, unknown>
    rule.outboundTag = outboundTag
    delete rule.balancerTag
  }
  return payload
}

function buildInboundActionPayload(form: XUIInboundActionForm): Record<string, unknown> {
  if (!form.port) {
    throw new Error('入站端口不能为空')
  }
  if (!form.protocol) {
    throw new Error('入站协议不能为空')
  }

  const protocol = normalizeOutboundProtocol(form.protocol)
  const tag = form.tag.trim() || `in-${protocol}-${form.port}`
  const clients = form.clients.map((client, index) => buildInboundClientPayload(client, protocol, index))
  const settings: Record<string, unknown> = {
    clients,
    fallbacks: [],
  }
  if (protocol === 'vless') {
    settings.decryption = 'none'
  }

  const streamSettings: Record<string, unknown> = {
    network: form.transport,
    security: form.security,
    tcpSettings: { acceptProxyProtocol: false, header: { type: 'none' } },
  }
  if (form.transport === 'ws') {
    streamSettings.wsSettings = {
      path: form.ws_path || '/',
      headers: form.ws_host ? { Host: form.ws_host } : {},
    }
  }
  if (form.security === 'tls') {
    streamSettings.tlsSettings = {
      serverName: form.server_name.trim(),
      alpn: ['h2', 'http/1.1'],
      certificates: [],
    }
  }

  const payload: Record<string, unknown> = {
    inbound: {
      remark: form.remark.trim(),
      tag,
      enable: form.enabled,
      listen: form.listen.trim(),
      port: form.port,
      protocol,
      total: 0,
      expiryTime: 0,
      settings: JSON.stringify(settings),
      streamSettings: JSON.stringify(streamSettings),
      sniffing: JSON.stringify({
        enabled: form.sniffing,
        destOverride: ['http', 'tls', 'quic', 'fakedns'],
        metadataOnly: false,
        routeOnly: false,
      }),
    },
    restart: true,
  }

  if (form.security === 'tls' && form.tls.mode !== 'none') {
    payload.tls_certificate = {
      mode: form.tls.mode,
      inventory_id: form.tls.inventory_id.trim(),
      domain: (form.tls.domain || form.server_name).trim(),
      certificate_file: form.tls.certificate_file.trim(),
      key_file: form.tls.key_file.trim(),
    }
  }

  return payload
}

function buildInboundClientPayload(client: XUIInboundClientForm, protocol: string, index: number): Record<string, unknown> {
  const email = client.email.trim() || `user-${index + 1}@local`
  const payload: Record<string, unknown> = {
    email,
    enable: client.enabled,
    flow: client.flow.trim(),
    limitIp: Math.max(0, client.limit_ip || 0),
    totalGB: Math.max(0, client.total_gb || 0) * 1024 * 1024 * 1024,
    expiryTime: client.expiry_days > 0 ? Date.now() + client.expiry_days * 24 * 60 * 60 * 1000 : 0,
    subId: client.sub_id.trim(),
    comment: client.comment.trim(),
  }

  if (protocol === 'trojan') {
    payload.password = client.password.trim() || `trojan-${index + 1}`
  } else {
    payload.id = client.uuid.trim() || `00000000-0000-0000-0000-${String(index + 1).padStart(12, '0')}`
  }
  return payload
}

function buildOutboundActionPayload(form: XUIOutboundActionForm): Record<string, unknown> {
  if (!form.source_agent_id || !form.source_client_key) {
    throw new Error('请选择源 Client 和源节点客户端')
  }
  const protocol = form.protocol.toLowerCase()
  const tag = form.tag.trim()
  if (!tag) {
    throw new Error('未能从源节点生成出站标签')
  }

  const outbound: Record<string, unknown> = {
    tag,
    protocol,
  }
  if (form.send_through.trim()) {
    outbound.sendThrough = form.send_through.trim()
  }
  const address = normalizeOutboundEndpointValue(form.address)
  const port = normalizeOutboundPort(form.port)

  switch (protocol) {
    case 'freedom':
    case 'blackhole':
      outbound.settings = {}
      break
    case 'vless':
      if (!address || !port) {
        throw new Error(`${protocol.toUpperCase()} 出站需要有效的远端地址和端口，请重新选择源节点客户端`)
      }
      outbound.settings = {
        address,
        port,
        id: form.uuid.trim() || '00000000-0000-0000-0000-000000000000',
        flow: form.flow.trim(),
        encryption: 'none',
      }
      outbound.streamSettings = buildOutboundStreamSettings(form)
      break
    case 'vmess':
      if (!address || !port) {
        throw new Error(`${protocol.toUpperCase()} 出站需要有效的远端地址和端口，请重新选择源节点客户端`)
      }
      outbound.settings = {
        vnext: [
          {
            address,
            port,
            users: [
              buildVNextUser(form, protocol),
            ],
          },
        ],
      }
      outbound.streamSettings = buildOutboundStreamSettings(form)
      break
    case 'trojan':
      if (!address || !port) {
        throw new Error('Trojan 出站需要有效的远端地址和端口，请重新选择源节点客户端')
      }
      outbound.settings = {
        servers: [
          {
            address,
            port,
            password: form.password.trim() || 'change-me',
          },
        ],
      }
      outbound.streamSettings = buildOutboundStreamSettings(form)
      break
    case 'shadowsocks':
      if (!address || !port) {
        throw new Error('Shadowsocks 出站需要有效的远端地址和端口，请重新选择源节点客户端')
      }
      outbound.settings = {
        servers: [
          {
            address,
            port,
            method: form.method.trim() || 'aes-256-gcm',
            password: form.password.trim() || 'change-me',
          },
        ],
      }
      break
    case 'http':
    case 'socks':
      if (!address || !port) {
        throw new Error(`${protocol.toUpperCase()} 出站需要有效的远端地址和端口，请重新选择源节点客户端`)
      }
      outbound.settings = {
        servers: [
          {
            address,
            port,
            users: form.password.trim()
              ? [{ user: form.uuid.trim() || 'user', pass: form.password.trim() }]
              : [],
          },
        ],
      }
      break
    default:
      throw new Error(`暂不支持该出站协议: ${form.protocol}`)
  }

  return {
    outbound,
    restart: true,
  }
}

function normalizeOutboundProtocol(value: unknown): string {
  const protocol = normalizeOutboundEndpointValue(value).toLowerCase()
  if (protocol === 'socks5') {
    return 'socks'
  }
  if (protocol === 'ss') {
    return 'shadowsocks'
  }
  return protocol
}

function normalizeOutboundEndpointValue(value: unknown): string {
  const text = String(value ?? '').trim()
  if (!text || /^(undefined|null|nan)$/i.test(text)) {
    return ''
  }
  return text
}

function normalizeOutboundPort(value: unknown): number {
  const port = Number(value || 0)
  return Number.isInteger(port) && port > 0 && port <= 65535 ? port : 0
}

function buildVNextUser(form: XUIOutboundActionForm, protocol: string): Record<string, unknown> {
  const user: Record<string, unknown> = {
    id: form.uuid.trim() || '00000000-0000-0000-0000-000000000000',
  }
  if (protocol === 'vless') {
    user.encryption = 'none'
    if (form.flow.trim()) {
      user.flow = form.flow.trim()
    }
    return user
  }
  user.security = 'auto'
  return user
}

function buildOutboundStreamSettings(form: XUIOutboundActionForm): Record<string, unknown> {
  const network = normalizeOutboundEndpointValue(form.network) || 'tcp'
  const security = normalizeOutboundEndpointValue(form.security) || 'none'
  const streamSettings: Record<string, unknown> = {
    network,
    security,
  }
  const path = normalizeOutboundEndpointValue(form.ws_path)
  const host = normalizeOutboundEndpointValue(form.ws_host)
  const alpn = splitOutboundList(form.alpn)
  const fingerprint = normalizeOutboundEndpointValue(form.reality_fingerprint)

  if (network === 'ws') {
    streamSettings.wsSettings = {
      path: path || '/',
      headers: host ? { Host: host } : {},
    }
  }
  if (network === 'grpc') {
    const grpcSettings: Record<string, unknown> = {
      serviceName: normalizeOutboundEndpointValue(form.grpc_service),
    }
    if (host) {
      grpcSettings.authority = host
    }
    streamSettings.grpcSettings = grpcSettings
  }
  if (network === 'http' || network === 'h2') {
    streamSettings.httpSettings = {
      path: path || '/',
      host: host ? [host] : [],
    }
  }
  if (security === 'tls') {
    const tlsSettings: Record<string, unknown> = {
      allowInsecure: false,
    }
    const serverName = normalizeOutboundEndpointValue(form.server_name)
    if (serverName) {
      tlsSettings.serverName = serverName
    }
    if (alpn.length) {
      tlsSettings.alpn = alpn
    }
    if (fingerprint) {
      tlsSettings.fingerprint = fingerprint
    }
    streamSettings.tlsSettings = tlsSettings
  }
  if (security === 'reality') {
    const serverName = normalizeOutboundEndpointValue(form.server_name)
    const publicKey = normalizeOutboundEndpointValue(form.reality_public_key)
    if (!serverName || !publicKey) {
      throw new Error('Reality 出站需要 SNI/serverName 和 publicKey，请重新选择源节点客户端')
    }
    const realitySettings: Record<string, unknown> = {
      serverName,
      fingerprint: fingerprint || 'chrome',
      publicKey,
    }
    const shortId = normalizeOutboundEndpointValue(form.reality_short_id)
    const spiderX = normalizeOutboundEndpointValue(form.reality_spider_x)
    if (shortId) {
      realitySettings.shortId = shortId
    }
    if (spiderX) {
      realitySettings.spiderX = spiderX
    }
    streamSettings.realitySettings = realitySettings
  }
  return streamSettings
}

function splitOutboundList(value: unknown): string[] {
  return normalizeOutboundEndpointValue(value)
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean)
}

function buildRoutingActionPayload(form: XUIRoutingActionForm): Record<string, unknown> {
  if (form.target_mode === 'existing_outbound' && !form.outbound_tag && !form.balancer_tag) {
    throw new Error('路由规则需要选择已存在的出站')
  }
  const rule: Record<string, unknown> = {
    type: 'field',
  }
  if (form.inbound_tags.length) {
    rule.inboundTag = form.inbound_tags
  }
  if (form.users.length) {
    rule.user = form.users
  }
  if (form.outbound_tag) {
    rule.outboundTag = form.outbound_tag
  }
  if (form.balancer_tag) {
    rule.balancerTag = form.balancer_tag
  }
  setRoutingList(rule, 'domain', form.domains)
  setRoutingList(rule, 'ip', form.ips)
  setRoutingList(rule, 'port', form.ports)
  setRoutingList(rule, 'sourceIP', form.source_ips)
  setRoutingList(rule, 'sourcePort', form.source_ports)
  if (form.networks.length) {
    rule.network = form.networks
  }
  if (form.protocols.length) {
    rule.protocol = form.protocols
  }

  return {
    rule,
    restart: true,
  }
}

function setRoutingList(target: Record<string, unknown>, key: string, rawText: string) {
  const values = splitTextList(rawText)
  if (values.length) {
    target[key] = values
  }
}

function splitTextList(rawText: string): string[] {
  return rawText
    .split(/[\n,]/)
    .map((item) => item.trim())
    .filter(Boolean)
}

function actionKindLabel(kind: string): string {
  switch (kind) {
    case 'upsert_routing_rule':
      return '新增 / 修改转发规则'
    case 'add_client':
      return '节点下新增客户端'
    case 'add_outbound':
      return '从内部导入出站'
    case 'add_routing_rule':
      return '新增转发 / 路由规则'
    case 'restart_xui':
      return '重启 x-ui / Xray'
    case 'execute_command':
      return '远程命令'
    case 'update_3xui':
      return '升级 3x-ui'
    case 'delete_client':
      return '删除 Client'
    default:
      return kind
  }
}

function actionStatusLabel(status: string): string {
  switch (status) {
    case 'pending':
      return '等待 client 执行'
    case 'running':
      return '执行中'
    case 'succeeded':
      return '成功'
    case 'failed':
      return '失败'
    default:
      return status || '-'
  }
}

function actionStatusColor(status: string): string {
  switch (status) {
    case 'pending':
      return 'gold'
    case 'running':
      return 'processing'
    case 'succeeded':
      return 'success'
    case 'failed':
      return 'error'
    default:
      return 'default'
  }
}

function shortJSON(value: unknown): string {
  if (!value || (typeof value === 'object' && Object.keys(value as Record<string, unknown>).length === 0)) {
    return ''
  }
  const text = JSON.stringify(value)
  return text.length > 140 ? `${text.slice(0, 140)}...` : text
}

function summarizeConfigAudit(item: ConfigAuditLog): string {
  const before = (item.before || {}) as ManagedAgentConfig
  const after = (item.after || {}) as ManagedAgentConfig
  const changes: string[] = []
  if (before.agent_name !== after.agent_name) {
    changes.push('名称')
  }
  if (before.customer_display_name !== after.customer_display_name) {
    changes.push('用户展示名称')
  }
  if (JSON.stringify(before.tags || []) !== JSON.stringify(after.tags || [])) {
    changes.push('标签')
  }
  if (JSON.stringify(before.renewal || {}) !== JSON.stringify(after.renewal || {})) {
    changes.push('VPS 周期')
  }
  if (JSON.stringify(before.entry || {}) !== JSON.stringify(after.entry || {})) {
    changes.push('入口/NAT')
  }
  if (JSON.stringify(before.xui || {}) !== JSON.stringify(after.xui || {})) {
    changes.push('X-UI')
  }
  return changes.length ? `修改了 ${changes.join('、')}` : '保存了配置'
}

function confidenceLabel(value?: string): string {
  switch (value) {
    case 'high':
      return '高置信'
    case 'medium':
      return '中置信'
    case 'low':
      return '低置信'
    default:
      return '未标记'
  }
}

function parseTagInput(rawText: string): string[] {
	const seen = new Set<string>()
	const result: string[] = []
	rawText
		.split(/[,\n，、]/)
		.map((item) => item.trim())
		.filter(Boolean)
		.forEach((item) => {
      const key = item.toLowerCase()
      if (!seen.has(key)) {
        seen.add(key)
        result.push(item)
      }
    })
	return result.sort((left, right) => left.localeCompare(right))
}

function formatTagInput(tags: string[] | undefined): string {
	return (tags || []).join(', ')
}

function mergeTagOptions(current: string[], incoming: string[]): string[] {
  return parseTagInput([...current, ...incoming].join(','))
}

function mergeDashboardTagOptions(dashboardTags: DashboardTagView[], tagOptions: string[]): DashboardTagView[] {
  const byTag = new Map<string, DashboardTagView>()
  for (const tag of dashboardTags) {
    byTag.set(tag.tag, tag)
  }
  for (const tag of tagOptions) {
    if (!byTag.has(tag)) {
      byTag.set(tag, { tag, agent_count: 0, node_count: 0, client_count: 0, online_client_count: 0 })
    }
  }
  return Array.from(byTag.values()).sort((left, right) => left.tag.localeCompare(right.tag))
}

function parseAddressInput(rawText: string): string[] {
  const seen = new Set<string>()
  const result: string[] = []
  rawText
    .split(/[,\n，、]/)
    .map((item) => item.trim())
    .filter(Boolean)
    .forEach((item) => {
      const key = item.toLowerCase()
      if (!seen.has(key)) {
        seen.add(key)
        result.push(item)
      }
    })
  return result.sort((left, right) => left.localeCompare(right))
}

function formatAddressInput(addresses: string[] | undefined): string {
  return (addresses || []).join('\n')
}

function normalizeEntryConfig(config?: AgentEntryConfig): AgentEntryConfig {
  return {
    addresses: parseAddressInput((config?.addresses || []).join('\n')),
    import_domain: normalizeImportDomain(config?.import_domain),
    mappings: (config?.mappings || []).map((mapping) => ({
      address: mapping.address || '',
      external_port: Math.max(0, Number(mapping.external_port || 0)),
      internal_port: Math.max(0, Number(mapping.internal_port || 0)),
      protocol: normalizeEntryProtocol(mapping.protocol),
      note: mapping.note || '',
    })),
  }
}

function normalizeImportDomain(value?: string): string {
  let domain = (value || '').trim().toLowerCase()
  domain = domain.replace(/^https?:\/\//, '').split('/')[0].replace(/\.$/, '')
  const portMatch = domain.match(/^([^:[\]]+):\d+$/)
  if (portMatch) {
    domain = portMatch[1]
  }
  if (!domain || domain.includes(' ') || domain.includes('*') || domain.includes(':') || isLikelyIP(domain)) {
    return ''
  }
  return domain
}

function isLikelyIP(value: string): boolean {
  if (/^\d{1,3}(\.\d{1,3}){3}$/.test(value)) {
    return true
  }
  return /^[0-9a-f:]+$/i.test(value) && value.includes(':')
}

function normalizeEntryProtocol(protocol?: string): AgentEntryMapping['protocol'] {
  switch ((protocol || '').toLowerCase()) {
    case 'vless':
      return 'vless'
    case 'vmess':
      return 'vmess'
    case 'http':
      return 'http'
    case 'socks':
    case 'socks5':
      return 'socks'
    default:
      return 'vless'
  }
}

function buildSectionSavePayload(base: ManagedAgentConfig, draft: ManagedAgentConfig, section: ConfigSectionKey, agentID: string): ManagedAgentConfig {
  const payload: ManagedAgentConfig = {
    ...base,
    agent_id: agentID,
    agent_name: base.agent_name || draft.agent_name || agentID,
    customer_display_name: base.customer_display_name || '',
    sort_order: base.sort_order || draft.sort_order || 0,
    tags: [...(base.tags || [])],
    renewal: { ...(base.renewal || {}) },
    entry: {
      addresses: [...(base.entry?.addresses || [])],
      import_domain: base.entry?.import_domain || '',
      mappings: (base.entry?.mappings || []).map((mapping) => ({ ...mapping })),
    },
    xui: { ...base.xui },
  }
  switch (section) {
    case 'client':
      payload.agent_name = draft.agent_name || agentID
      payload.customer_display_name = draft.customer_display_name || ''
      payload.sort_order = Number(draft.sort_order || base.sort_order || 0)
      payload.tags = [...(draft.tags || [])]
      break
    case 'renewal':
      payload.renewal = { ...(draft.renewal || {}) }
      break
    case 'xui':
      payload.xui = { ...draft.xui }
      break
    case 'entry':
      payload.entry = {
        addresses: [...(draft.entry?.addresses || [])],
        import_domain: draft.entry?.import_domain || '',
        mappings: (draft.entry?.mappings || []).map((mapping) => ({ ...mapping })),
      }
      break
  }
  return payload
}

function mergeSavedSectionIntoDraft(draft: ManagedAgentConfig, saved: ManagedAgentConfig, section: ConfigSectionKey): ManagedAgentConfig {
  const next: ManagedAgentConfig = {
    ...draft,
    agent_id: saved.agent_id || draft.agent_id,
  }
  switch (section) {
    case 'client':
      next.agent_name = saved.agent_name
      next.customer_display_name = saved.customer_display_name || ''
      next.sort_order = saved.sort_order
      next.tags = [...(saved.tags || [])]
      break
    case 'renewal':
      next.renewal = { ...(saved.renewal || {}) }
      break
    case 'xui':
      next.xui = { ...saved.xui }
      break
    case 'entry':
      next.entry = {
        addresses: [...(saved.entry?.addresses || [])],
        import_domain: saved.entry?.import_domain || '',
        mappings: (saved.entry?.mappings || []).map((mapping) => ({ ...mapping })),
      }
      break
  }
  return next
}

function configSectionLabel(section: ConfigSectionKey): string {
  switch (section) {
    case 'client':
      return 'Client 信息'
    case 'renewal':
      return 'VPS 信息'
    case 'xui':
      return 'X-UI 配置'
    case 'entry':
      return '入口/NAT 配置'
  }
}

function configSignature(config: ManagedAgentConfig): string {
  return JSON.stringify(config)
}

function normalizeRenewalConfig(config?: VPSRenewalConfig): VPSRenewalConfig {
  const cycle = config?.cycle === 'week' || config?.cycle === 'quarter' || config?.cycle === 'year' ? config.cycle : 'month'
  const trafficLimitBytes = Math.max(0, Number(config?.traffic_limit_bytes || 0))
  const costAmount = Math.max(0, Number(config?.cost_amount || 0))
  const costCurrency = normalizeCurrencyCode(config?.cost_currency)
  const costCycle = config?.cost_cycle === 'quarter' || config?.cost_cycle === 'year' ? config.cost_cycle : 'month'
  return {
    enabled: Boolean(config?.enabled || config?.start_date || config?.expire_date),
    start_date: config?.start_date || '',
    expire_date: config?.expire_date || '',
    cycle,
    auto_renew: Boolean(config?.auto_renew),
    cost_amount: costAmount,
    cost_currency: costCurrency,
    cost_cycle: costCycle,
    client_billings: normalizeClientBillings(config?.client_billings || []),
    traffic_limit_bytes: trafficLimitBytes,
    bandwidth_mbps: Math.max(0, Number(config?.bandwidth_mbps || 0)),
    traffic_baseline_bytes: trafficLimitBytes > 0 ? Math.max(0, Number(config?.traffic_baseline_bytes || 0)) : 0,
    traffic_sent_baseline_bytes: trafficLimitBytes > 0 ? Math.max(0, Number(config?.traffic_sent_baseline_bytes || 0)) : 0,
    traffic_recv_baseline_bytes: trafficLimitBytes > 0 ? Math.max(0, Number(config?.traffic_recv_baseline_bytes || 0)) : 0,
    traffic_baseline_period_start: trafficLimitBytes > 0 ? config?.traffic_baseline_period_start || '' : '',
  }
}

function normalizeClientBillings(items: XUIClientBillingConfig[]): XUIClientBillingConfig[] {
  const seen = new Set<string>()
  const normalized: XUIClientBillingConfig[] = []
  for (const item of items) {
    const billing: XUIClientBillingConfig = {
      inbound_id: Number(item.inbound_id || 0),
      inbound_tag: item.inbound_tag || '',
      email: item.email || '',
      revenue_amount: Math.max(0, Number(item.revenue_amount || 0)),
      revenue_currency: item.revenue_currency === 'USDT' ? 'USDT' : 'CNY',
      revenue_cycle: item.revenue_cycle === 'quarter' || item.revenue_cycle === 'year' ? item.revenue_cycle : 'month',
      expire_time: Math.max(0, Number(item.expire_time || 0)),
      expire_cycle: item.expire_cycle === 'quarter' || item.expire_cycle === 'year' ? item.expire_cycle : 'month',
      expire_auto_renew: Boolean(item.expire_auto_renew),
    }
    const key = clientBillingKey(billing)
    if (seen.has(key)) {
      continue
    }
    seen.add(key)
    normalized.push(billing)
  }
  return normalized
}

function clientBillingKey(value: Pick<XUIClientBillingConfig, 'inbound_id' | 'inbound_tag' | 'email'>): string {
  return `${Number(value.inbound_id || 0)}\u0000${value.inbound_tag || ''}\u0000${value.email || ''}`
}

function billingKeyForClient(client: XUIClientView): string {
  return clientBillingKey({ inbound_id: client.inbound_id, inbound_tag: client.inbound_tag, email: client.email })
}

function defaultClientBilling(client: XUIClientView): XUIClientBillingConfig {
  return {
    inbound_id: client.inbound_id,
    inbound_tag: client.inbound_tag || '',
    email: client.email || '',
    revenue_amount: 0,
    revenue_currency: 'CNY',
    revenue_cycle: 'month',
    expire_time: Math.max(0, Number(client.expiry_time || 0)),
    expire_cycle: 'month',
    expire_auto_renew: false,
  }
}

function findClientBilling(items: XUIClientBillingConfig[] | undefined, client: XUIClientView): XUIClientBillingConfig | undefined {
  const key = billingKeyForClient(client)
  return (items || []).find((item) => clientBillingKey(item) === key)
}

function upsertClientBilling(items: XUIClientBillingConfig[], client: XUIClientView, billing: XUIClientBillingConfig): XUIClientBillingConfig[] {
  const next = normalizeClientBillings(items)
  const normalized = normalizeClientBillings([{ ...defaultClientBilling(client), ...billing }])[0]
  const key = billingKeyForClient(client)
  const index = next.findIndex((item) => clientBillingKey(item) === key)
  if (index >= 0) {
    next[index] = normalized
    return next
  }
  return [...next, normalized]
}

function calculateRenewalStatus(config?: VPSRenewalConfig): {
  remainingLabel: string
  endLabel: string
  level: 'ok' | 'warn' | 'bad'
  autoRenew: boolean
  percent: number
} | null {
  const normalized = normalizeRenewalConfig(config)
  if (!normalized.enabled || (!normalized.start_date && !normalized.expire_date)) {
    return null
  }
  const period = calculateRenewalPeriod(normalized)
  if (!period) {
    return null
  }
  const now = startOfLocalDay(new Date())
  if (now < period.start) {
    const daysUntilStart = daysBetween(now, period.start)
    return {
      remainingLabel: `${daysUntilStart} 天后开始`,
      endLabel: `到期 ${formatLocalDate(period.end)}`,
      level: 'ok',
      autoRenew: Boolean(normalized.auto_renew),
      percent: 0,
    }
  }
  const remainingDays = daysBetween(now, period.end)
  if (remainingDays < 0) {
    const overdueDays = Math.abs(remainingDays)
    return {
      remainingLabel: `已过期 ${overdueDays} 天`,
      endLabel: `到期 ${formatLocalDate(period.end)}`,
      level: 'bad',
      autoRenew: Boolean(normalized.auto_renew),
      percent: 100,
    }
  }
  const totalDays = Math.max(1, daysBetween(period.start, period.end))
  const remainingPercent = (Math.max(0, remainingDays) / totalDays) * 100
  return {
    remainingLabel: remainingDays === 0 ? '今天到期' : `剩余 ${remainingDays} 天`,
    endLabel: `到期 ${formatLocalDate(period.end)}`,
    level: remainingDays <= 3 ? 'bad' : remainingDays <= 7 ? 'warn' : 'ok',
    autoRenew: Boolean(normalized.auto_renew),
    percent: clampMetricPercent(remainingPercent),
  }
}

function formatRenewalHint(config?: VPSRenewalConfig): string {
  const status = calculateRenewalStatus(config)
  if (!status) {
    return '设置后会在 Client 卡片上展示到期、周期刷新、总/上传/下载流量配额和带宽信息。'
  }
  return `当前周期${status.remainingLabel}，${status.endLabel}，${status.autoRenew ? '到期后自动刷新下一周期，并重新计算总/上传/下载流量' : '到期后不自动刷新'}。`
}

function calculateRenewalPeriod(config: VPSRenewalConfig): { start: Date; end: Date } | null {
  const now = startOfLocalDay(new Date())
  if (config.expire_date) {
    let end = parseLocalDate(config.expire_date)
    if (!end) {
      return null
    }
    end = startOfLocalDay(end)
    if (config.auto_renew && config.cycle) {
      while (end <= now) {
        end = addRenewalCycle(end, config.cycle)
      }
    }
    return {
      start: config.cycle ? subtractRenewalCycle(end, config.cycle) : end,
      end,
    }
  }
  const start = parseLocalDate(config.start_date || '')
  if (!start || !config.cycle) {
    return null
  }
  let periodStart = startOfLocalDay(start)
  let periodEnd = addRenewalCycle(periodStart, config.cycle)
  if (config.auto_renew !== false) {
    while (periodEnd <= now) {
      periodStart = periodEnd
      periodEnd = addRenewalCycle(periodStart, config.cycle)
    }
  }
  return { start: periodStart, end: periodEnd }
}

function parseLocalDate(value: string): Date | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value)
  if (!match) {
    return null
  }
  const year = Number(match[1])
  const month = Number(match[2]) - 1
  const day = Number(match[3])
  const date = new Date(year, month, day)
  if (date.getFullYear() !== year || date.getMonth() !== month || date.getDate() !== day) {
    return null
  }
  return date
}

function startOfLocalDay(date: Date): Date {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate())
}

function addRenewalCycle(date: Date, cycle: VPSRenewalConfig['cycle']): Date {
  if (cycle === 'week') {
    return new Date(date.getFullYear(), date.getMonth(), date.getDate() + 7)
  }
  if (cycle === 'quarter') {
    return addClampedMonths(date, 3)
  }
  if (cycle === 'year') {
    return addClampedMonths(date, 12)
  }
  return addClampedMonths(date, 1)
}

function subtractRenewalCycle(date: Date, cycle: VPSRenewalConfig['cycle']): Date {
  if (cycle === 'week') {
    return new Date(date.getFullYear(), date.getMonth(), date.getDate() - 7)
  }
  if (cycle === 'quarter') {
    return addClampedMonths(date, -3)
  }
  if (cycle === 'year') {
    return addClampedMonths(date, -12)
  }
  return addClampedMonths(date, -1)
}

function addClampedMonths(date: Date, months: number): Date {
  const targetMonth = date.getMonth() + months
  const firstOfTarget = new Date(date.getFullYear(), targetMonth, 1)
  const lastDay = new Date(firstOfTarget.getFullYear(), firstOfTarget.getMonth() + 1, 0).getDate()
  return new Date(firstOfTarget.getFullYear(), firstOfTarget.getMonth(), Math.min(date.getDate(), lastDay))
}

function daysBetween(start: Date, end: Date): number {
  const msPerDay = 24 * 60 * 60 * 1000
  return Math.ceil((startOfLocalDay(end).getTime() - startOfLocalDay(start).getTime()) / msPerDay)
}

function formatLocalDate(date: Date): string {
  const year = date.getFullYear()
  const month = `${date.getMonth() + 1}`.padStart(2, '0')
  const day = `${date.getDate()}`.padStart(2, '0')
  return `${year}/${month}/${day}`
}

function hasSelectedTag(tags: string[] | undefined, selectedTag: string): boolean {
  if (!selectedTag) {
    return true
  }
  return (tags || []).some((tag) => tag === selectedTag)
}

function isAgentRunning(agent: AgentListItem): boolean {
  const seenAt = Date.parse(agent.realtime_at || agent.last_seen_at || agent.reported_at || '')
  if (Number.isNaN(seenAt)) {
    return false
  }
  return Date.now() - seenAt <= 5 * 60 * 1000
}

function topologyMatchesSelectedTag(link: TopologyLinkView, selectedTag: string): boolean {
  if (!selectedTag) {
    return true
  }
  return [...(link.source.agent_tags || []), ...(link.target.agent_tags || [])].includes(selectedTag)
}

function findOutboundLinkedClient(view: GlobalDashboardView | null, agentID: string, outboundTag?: string): TopologyLinkView | undefined {
  if (!view || !agentID || !outboundTag) {
    return undefined
  }
  return (view.links || []).find((link) => link.source.agent_id === agentID && link.source.outbound_tag === outboundTag)
}

function chainMatchesSelectedTag(chain: ClientChainView, selectedTag: string): boolean {
  if (!selectedTag) {
    return true
  }
  return (chain.root_agent_tags || []).includes(selectedTag)
}

function tagChipStyle(tag: string, active = false) {
  const normalized = tag.trim().toLowerCase()
  let hash = 0
  for (let index = 0; index < normalized.length; index += 1) {
    hash = (hash * 31 + normalized.charCodeAt(index)) >>> 0
  }
  const color = TAG_COLOR_PALETTE[hash % TAG_COLOR_PALETTE.length]
  return {
    backgroundColor: active ? color.text : color.bg,
    borderColor: color.border,
    color: active ? '#ffffff' : color.text,
    boxShadow: active ? `0 0 0 3px ${color.bg}` : undefined,
  }
}

function createEmptyManagedConfig(agentID: string, agentName?: string): ManagedAgentConfig {
  return {
    agent_id: agentID,
    agent_name: agentName || agentID,
    customer_display_name: '',
    sort_order: 0,
    tags: [],
    renewal: {
      enabled: false,
      start_date: '',
      expire_date: '',
      cycle: 'month',
      auto_renew: false,
      cost_amount: 0,
      cost_currency: DEFAULT_COST_CURRENCY,
      cost_cycle: 'month',
      client_billings: [],
      traffic_limit_bytes: 0,
      bandwidth_mbps: 0,
      traffic_baseline_bytes: 0,
      traffic_sent_baseline_bytes: 0,
      traffic_recv_baseline_bytes: 0,
      traffic_baseline_period_start: '',
    },
    entry: {
      addresses: [],
      import_domain: '',
      mappings: [],
    },
    xui: {
      enabled: false,
      base_url: '',
      db_path: '',
      username: '',
      password: '',
      api_token: '',
      two_factor_code: '',
      skip_tls_verify: false,
    },
  }
}

function normalizeManagedConfig(config: ManagedAgentConfig, agentID: string, agentName?: string): ManagedAgentConfig {
  const base = createEmptyManagedConfig(agentID, agentName)
  return {
    agent_id: config.agent_id || base.agent_id,
    agent_name: config.agent_name || agentName || base.agent_name,
    customer_display_name: config.customer_display_name || '',
    sort_order: Number(config.sort_order || base.sort_order || 0),
    tags: parseTagInput((config.tags || []).join(',')),
    renewal: normalizeRenewalConfig(config.renewal || base.renewal),
    entry: normalizeEntryConfig(config.entry || base.entry),
    xui: {
      ...base.xui,
      ...config.xui,
      enabled: Boolean(config.xui?.enabled),
      skip_tls_verify: Boolean(config.xui?.skip_tls_verify),
    },
  }
}

function normalizeXUIOverview(overview: XUIOverview): XUIOverview {
  return {
    ...overview,
    nodes: Array.isArray(overview.nodes) ? overview.nodes : [],
    clients: Array.isArray(overview.clients) ? overview.clients : [],
    outbounds: Array.isArray(overview.outbounds) ? overview.outbounds : [],
    routing_rules: Array.isArray(overview.routing_rules) ? overview.routing_rules : [],
    certificates: Array.isArray(overview.certificates) ? overview.certificates : [],
  }
}

function readStoredAgentViewMode(username: string): AgentViewMode {
  try {
    const value = window.localStorage.getItem(agentViewModeStorageKey(username))
    return value === 'list' ? 'list' : 'card'
  } catch {
    return 'card'
  }
}

function storeAgentViewMode(username: string, mode: AgentViewMode) {
  try {
    window.localStorage.setItem(agentViewModeStorageKey(username), mode)
  } catch {
    // Preference storage is optional; the UI still switches for the current session.
  }
}


function formatDateInputFromMillis(value?: number): string {
  if (!value) {
    return ''
  }
  const date = new Date(value)
  if (!Number.isFinite(date.getTime())) {
    return ''
  }
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

function dateInputToExpiryMillis(value: string): number {
  if (!value) {
    return 0
  }
  const date = new Date(`${value}T23:59:59`)
  if (!Number.isFinite(date.getTime())) {
    return 0
  }
  return date.getTime()
}

function formatExpiryTime(value?: number): string {
  if (!value) {
    return '无限期'
  }
  const date = new Date(value)
  if (!Number.isFinite(date.getTime())) {
    return '无限期'
  }
  return formatDateTime(date.toISOString())
}

function agentViewModeStorageKey(username: string): string {
  return `${AGENT_VIEW_MODE_STORAGE_PREFIX}${username || 'default'}`
}

function formatDateTime(value?: string): string {
  if (!value) {
    return '-'
  }
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).format(new Date(value))
}

function formatRelativeTime(value?: number): string {
  if (!value) {
    return '未活跃'
  }
  const target = new Date(value)
  const diffMinutes = Math.round((Date.now() - target.getTime()) / 60000)
  if (diffMinutes < 1) {
    return '刚刚'
  }
  if (diffMinutes < 60) {
    return `${diffMinutes} 分钟前`
  }
  const diffHours = Math.round(diffMinutes / 60)
  if (diffHours < 24) {
    return `${diffHours} 小时前`
  }
  const diffDays = Math.round(diffHours / 24)
  if (diffDays < 30) {
    return `${diffDays} 天前`
  }
  return formatDateTime(target.toISOString())
}

function isClientOnline(lastOnline?: number, reportedAt?: string): boolean {
  if (!lastOnline) {
    return false
  }
  const compareAt = reportedAt ? new Date(reportedAt).getTime() : Date.now()
  return compareAt - lastOnline <= 5 * 60 * 1000
}

function scopeLabel(scope?: string): string {
  switch (scope) {
    case 'user':
      return '用户规则'
    case 'inbound':
      return '节点规则'
    case 'default':
      return '默认出口'
    case 'global':
      return '全局规则'
    default:
      return '未推断'
  }
}

function scopeColor(scope?: string): string {
  switch (scope) {
    case 'user':
      return 'gold'
    case 'inbound':
      return 'blue'
    case 'default':
      return 'green'
    case 'global':
      return 'magenta'
    default:
      return 'default'
  }
}

function summarizeRule(rule: XUIRoutingRuleView): string {
  const segments: string[] = []
  if (rule.domain?.length) {
    segments.push(`domain(${rule.domain.length})`)
  }
  if (rule.ip?.length) {
    segments.push(`ip(${rule.ip.length})`)
  }
  if (rule.protocol?.length) {
    segments.push(`protocol(${rule.protocol.length})`)
  }
  if (rule.port?.length) {
    segments.push(`port(${rule.port.join(',')})`)
  }
  if (rule.network?.length) {
    segments.push(`network(${rule.network.join(',')})`)
  }
  return segments.join(' · ') || '无额外条件'
}

function statusColor(state?: string): string {
  switch ((state || '').toLowerCase()) {
    case 'running':
      return '#0f766e'
    case 'stop':
      return '#c05621'
    case 'error':
      return '#c53030'
    default:
      return '#94a3b8'
  }
}

function agentStatusLevel(state?: string): 'ok' | 'warn' | 'bad' | 'neutral' {
  switch ((state || '').toLowerCase()) {
    case 'running':
      return 'ok'
    case 'stop':
    case 'stopped':
      return 'warn'
    case 'error':
      return 'bad'
    default:
      return 'neutral'
  }
}

function agentDisplayStatus(agent: AgentListItem): { label: string; level: 'ok' | 'warn' | 'bad' | 'neutral' } {
  if (!isAgentRunning(agent)) {
    return { label: 'client 离线', level: 'bad' }
  }
  if (agent.summary.last_collection_err || xrayIssueLabel(agent)) {
    return { label: 'client 警告', level: 'warn' }
  }
  return { label: 'running', level: 'ok' }
}

function xrayIssueLabel(agent: AgentListItem): string {
  const xrayState = (agent.summary.xray_state || '').trim()
  if (!xrayState || xrayState.toLowerCase() === 'running') {
    return ''
  }
  return `Xray ${xrayState}`
}

function countryFlag(code?: string): string {
  const normalized = (code || '').trim().toUpperCase()
  if (!/^[A-Z]{2}$/.test(normalized)) {
    return '🌐'
  }
  return Array.from(normalized)
    .map((char) => String.fromCodePoint(127397 + char.charCodeAt(0)))
    .join('')
}

function agentCountryCode(agent: AgentListItem): string {
  const explicitCode = explicitAgentCountryCode(agent)
  if (explicitCode) {
    return explicitCode
  }
  const geoCode = normalizeCountryCode(agent.geo?.country_code)
  return geoCode || ''
}

function explicitAgentCountryCode(agent: AgentListItem): string {
  const candidates = [agent.agent_name || '', ...(agent.tags || []), agent.summary.hostname || '', agent.agent_id || '']
  for (const value of candidates) {
    const code = explicitCountryCodeFromText(value)
    if (code) {
      return code
    }
  }
  return ''
}

function explicitCountryCodeFromText(value?: string): string {
  const text = (value || '').trim().toUpperCase()
  if (!text) {
    return ''
  }
  const direct = normalizeCountryCode(text)
  if (direct) {
    return direct
  }
  const match = /(?:^|[^A-Z0-9])(TH|MY|VN|IN|SG|HK|MO|TW|JP|KR|CA|US|CN|PH|DE|FR|GB|AU)(?=$|[^A-Z0-9])/.exec(text)
  return match ? match[1] : ''
}

function normalizeCountryCode(value?: string): string {
  const code = (value || '').trim().toUpperCase()
  if (['TH', 'MY', 'VN', 'IN', 'SG', 'HK', 'MO', 'TW', 'JP', 'KR', 'CA', 'US', 'CN', 'PH', 'DE', 'FR', 'GB', 'AU'].includes(code)) {
    return code
  }
  return ''
}

function formatAgentLocation(agent: AgentListItem, displayCountryCode: string): string {
  const geoCode = normalizeCountryCode(agent.geo?.country_code)
  const geoLabel = formatGeoLabel(agent.geo)
  if (!displayCountryCode) {
    return geoLabel
  }
  const displayCountry = countryName(displayCountryCode)
  if (!geoLabel) {
    return displayCountry
  }
  if (geoCode && geoCode !== displayCountryCode) {
    return `${displayCountry} · IP库: ${geoLabel}`
  }
  return geoLabel || displayCountry
}

function formatGeoLabel(geo?: IPGeoView): string {
  if (!geo) {
    return ''
  }
  return [geo.country_name || geo.country_code, geo.region_name, geo.city].filter(Boolean).join(' · ')
}

function countryName(code: string): string {
  switch (code) {
    case 'TH':
      return 'Thailand'
    case 'MY':
      return 'Malaysia'
    case 'VN':
      return 'Vietnam'
    case 'IN':
      return 'India'
    case 'SG':
      return 'Singapore'
    case 'HK':
      return 'Hong Kong'
    case 'MO':
      return 'Macao'
    case 'TW':
      return 'Taiwan'
    case 'JP':
      return 'Japan'
    case 'KR':
      return 'South Korea'
    case 'CA':
      return 'Canada'
    case 'US':
      return 'United States'
    case 'CN':
      return 'China'
    case 'PH':
      return 'Philippines'
    case 'DE':
      return 'Germany'
    case 'FR':
      return 'France'
    case 'GB':
      return 'United Kingdom'
    case 'AU':
      return 'Australia'
    default:
      return code
  }
}

function outboundElementId(tag: string): string {
  return `outbound-${sanitizeFragment(tag)}`
}

function ruleElementId(index: number): string {
  return `rule-${index}`
}

function nodeElementId(agentID: string, nodeLabel: string): string {
  return `node-${sanitizeFragment(agentID)}-${sanitizeFragment(normalizeNodeAnchorLabel(nodeLabel))}`
}

function normalizeNodeAnchorLabel(value: string): string {
  return value.replace(/:\d+$/, '').trim()
}

function sanitizeFragment(value: string): string {
  return value.replace(/[^a-zA-Z0-9_-]/g, '-')
}

function summarizeAgent(summary?: VPSSummary): string {
  if (!summary) {
    return '-'
  }
  return `${formatPercent(summary.cpu)} CPU · ${formatMem(summary.mem_used, summary.mem_total)}`
}
export {
  APIError,
  fetchJSON,
  buildDashboardRealtimeURL,
  buildAgentTerminalURL,
  mergeRealtimeMetricsIntoAgents,
  sortAgentsByOrder,
  mergeRealtimeSummary,
  isUnauthorized,
  defaultTLSCertificateSelection,
  defaultInboundClientForm,
  defaultInboundActionForm,
  defaultOutboundActionForm,
  defaultRoutingActionForm,
  defaultAddClientActionForm,
  defaultTelegramBotForm,
  defaultClientInstallCommandForm,
  normalizeClientInstallCommandForm,
  defaultFrontendSettingsForm,
  normalizeFrontendSettingsForm,
  clientInstallCommandByKind,
  buildClientInstallCommand,
  buildWindowsPowerShellInstallCommand,
  buildWindowsCMDInstallCommand,
  windowsInstallScriptURL,
  shellQuote,
  powerShellQuote,
  buildXUIActionPayload,
  buildAddClientActionPayload,
  buildInboundActionPayload,
  buildInboundClientPayload,
  buildOutboundActionPayload,
  buildUpsertRoutingActionPayload,
  buildVNextUser,
  buildOutboundStreamSettings,
  buildRoutingActionPayload,
  setRoutingList,
  splitTextList,
  actionKindLabel,
  actionStatusLabel,
  actionStatusColor,
  shortJSON,
  summarizeConfigAudit,
  confidenceLabel,
  parseTagInput,
  formatTagInput,
  mergeTagOptions,
  mergeDashboardTagOptions,
  parseAddressInput,
  formatAddressInput,
  normalizeEntryConfig,
  normalizeEntryProtocol,
  buildSectionSavePayload,
  mergeSavedSectionIntoDraft,
  configSectionLabel,
  configSignature,
  normalizeRenewalConfig,
  normalizeClientBillings,
  clientBillingKey,
  billingKeyForClient,
  defaultClientBilling,
  findClientBilling,
  upsertClientBilling,
  calculateRenewalStatus,
  formatRenewalHint,
  calculateRenewalPeriod,
  parseLocalDate,
  startOfLocalDay,
  addRenewalCycle,
  subtractRenewalCycle,
  addClampedMonths,
  daysBetween,
  formatLocalDate,
  hasSelectedTag,
  isAgentRunning,
  topologyMatchesSelectedTag,
  findOutboundLinkedClient,
  chainMatchesSelectedTag,
  tagChipStyle,
  createEmptyManagedConfig,
  normalizeManagedConfig,
  normalizeXUIOverview,
  readStoredAgentViewMode,
  storeAgentViewMode,
  agentViewModeStorageKey,
  formatDateTime,
  formatDateInputFromMillis,
  dateInputToExpiryMillis,
  formatExpiryTime,
  formatRelativeTime,
  isClientOnline,
  scopeLabel,
  scopeColor,
  summarizeRule,
  statusColor,
  agentStatusLevel,
  agentDisplayStatus,
  xrayIssueLabel,
  countryFlag,
  agentCountryCode,
  explicitAgentCountryCode,
  explicitCountryCodeFromText,
  normalizeCountryCode,
  formatAgentLocation,
  formatGeoLabel,
  countryName,
  outboundElementId,
  ruleElementId,
  nodeElementId,
  normalizeNodeAnchorLabel,
  sanitizeFragment,
  summarizeAgent
}
