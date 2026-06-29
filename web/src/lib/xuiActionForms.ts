export const XUI_ACTION_KINDS = [
  { value: 'add_client', label: '节点下新增客户端' },
  { value: 'upsert_routing_rule', label: '新增 / 修改转发规则' },
]

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
  source_type: 'registered_client' | 'library' | 'manual'
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
  previous_outbound_tag: string
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

export function defaultTLSCertificateSelection(): TLSCertificateSelectionForm {
  return {
    mode: 'none',
    inventory_id: '',
    domain: '',
    certificate_file: '',
    key_file: '',
  }
}

export function defaultInboundClientForm(): XUIInboundClientForm {
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

export function defaultInboundActionForm(): XUIInboundActionForm {
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

export function defaultOutboundActionForm(): XUIOutboundActionForm {
  return {
    source_type: 'registered_client',
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

export function defaultRoutingActionForm(): XUIRoutingActionForm {
  return {
    rule_index: null,
    previous_outbound_tag: '',
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

export function defaultAddClientActionForm(): XUIAddClientActionForm {
  return {
    inbound_id: 0,
    inbound_tag: '',
    inbound_name: '',
    protocol: 'vless',
    client: defaultInboundClientForm(),
    restart: false,
  }
}

export function buildXUIActionPayload(
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

export function buildAddClientActionPayload(form: XUIAddClientActionForm): Record<string, unknown> {
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
    restart: false,
  }
}

export function buildUpsertRoutingActionPayload(form: XUIRoutingActionForm, outboundForm: XUIOutboundActionForm): Record<string, unknown> {
  const payload = buildRoutingActionPayload(form)
  if (form.rule_index && form.rule_index > 0) {
    payload.rule_index = form.rule_index
  }
  if (form.target_mode === 'registered_client') {
    const outboundPayload = buildOutboundActionPayload(outboundForm)
    payload.outbound = outboundPayload.outbound
    if (form.previous_outbound_tag.trim()) {
      payload.previous_outbound_tag = form.previous_outbound_tag.trim()
    }
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

export function buildInboundActionPayload(form: XUIInboundActionForm): Record<string, unknown> {
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

export function buildInboundClientPayload(client: XUIInboundClientForm, protocol: string, index: number): Record<string, unknown> {
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
  }
  return payload
}

export function buildOutboundActionPayload(form: XUIOutboundActionForm): Record<string, unknown> {
  const address = normalizeOutboundEndpointValue(form.address)
  const port = normalizeOutboundPort(form.port)
  if ((!address || !port) && !form.source_agent_id && !form.source_client_key && form.protocol !== 'freedom' && form.protocol !== 'blackhole') {
    throw new Error('请选择源 Client / 出口链接库，或填写有效的远端地址和端口')
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

export function buildVNextUser(form: XUIOutboundActionForm, protocol: string): Record<string, unknown> {
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
  user.security = form.method.trim() || 'auto'
  return user
}

export function buildOutboundStreamSettings(form: XUIOutboundActionForm): Record<string, unknown> {
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

export function buildRoutingActionPayload(form: XUIRoutingActionForm): Record<string, unknown> {
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

function splitOutboundList(value: unknown): string[] {
  return normalizeOutboundEndpointValue(value)
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean)
}

export function setRoutingList(target: Record<string, unknown>, key: string, rawText: string) {
  const values = splitTextList(rawText)
  if (values.length) {
    target[key] = values
  }
}

export function splitTextList(rawText: string): string[] {
  return rawText
    .split(/[\n,]/)
    .map((item) => item.trim())
    .filter(Boolean)
}

export function actionKindLabel(kind: string): string {
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
    case 'set_client_enabled':
      return '启用 / 停用 Client'
    default:
      return kind
  }
}

export function actionStatusLabel(status: string): string {
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

export function actionStatusColor(status: string): string {
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
