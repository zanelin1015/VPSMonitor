import type { XUIClientView, XUINodeView, XUIOverview } from '../types'
import type { XUIOutboundActionForm } from '../lib/appHelpers'

export function sourceClientKey(client: XUIClientView): string {
  return [client.inbound_id || 0, client.inbound_tag || '', client.email || ''].join('::')
}

export function buildOutboundImportPatch(
  sourceOverview: XUIOverview,
  sourceNode: XUINodeView,
  sourceClient: XUIClientView,
  currentForm: XUIOutboundActionForm,
): Partial<XUIOutboundActionForm> {
  const importEndpoint = parseImportEndpoint(sourceClient.import_url)
  const address =
    usableEndpointValue(importEndpoint.address) ||
    usableEndpointValue(sourceOverview.summary.observed_ip) ||
    usableEndpointValue(sourceOverview.summary.public_ipv4) ||
    usableEndpointValue(sourceOverview.summary.public_ipv6) ||
    hostnameFromURL(sourceOverview.base_url) ||
    ''
  const protocol = normalizeOutboundProtocol(sourceNode.protocol || sourceClient.protocol || currentForm.protocol || 'freedom')
  const port = validPort(sourceNode.port) || validPort(importEndpoint.port) || parsePortFromText(sourceNode.tag, sourceNode.remark, sourceClient.inbound_tag, sourceClient.inbound_remark) || validPort(currentForm.port)
  const tagParts = [
    sourceOverview.agent_name || sourceOverview.agent_id,
    sourceNode.tag || sourceNode.remark || String(sourceNode.id),
    sourceClient.email || 'link',
  ]

  return {
    source_type: 'registered_client',
    tag: normalizeOutboundTag(tagParts.join('-')),
    protocol,
    address,
    port,
    uuid: protocol === 'socks' || protocol === 'http' ? importEndpoint.username || sourceClient.email || currentForm.uuid : sourceClient.auth_uuid || importEndpoint.username || currentForm.uuid,
    password: importEndpoint.password || sourceClient.auth_password || currentForm.password,
    method: importEndpoint.method || currentForm.method,
    flow: sourceClient.flow || '',
    security: sourceNode.security || importEndpoint.security || 'none',
    server_name: sourceNode.tls_server_name || sourceNode.ws_host || importEndpoint.serverName || currentForm.server_name,
    alpn: sourceNode.alpn || importEndpoint.alpn || currentForm.alpn,
    grpc_service: sourceNode.grpc_service || importEndpoint.grpcService || currentForm.grpc_service,
    reality_public_key: sourceNode.reality_public_key || importEndpoint.realityPublicKey || currentForm.reality_public_key,
    reality_short_id: sourceNode.reality_short_id || importEndpoint.realityShortID || currentForm.reality_short_id,
    reality_fingerprint: sourceNode.reality_fingerprint || importEndpoint.realityFingerprint || currentForm.reality_fingerprint || 'chrome',
    reality_spider_x: sourceNode.reality_spider_x || importEndpoint.realitySpiderX || currentForm.reality_spider_x,
    network: sourceNode.network || importEndpoint.network || 'tcp',
    ws_path: sourceNode.ws_path || importEndpoint.wsPath || '/',
    ws_host: sourceNode.ws_host || importEndpoint.wsHost || '',
  }
}

export function parseOutboundImportText(text: string, currentForm: XUIOutboundActionForm): Partial<XUIOutboundActionForm> {
  const trimmed = text.trim()
  if (!trimmed) {
    throw new Error('请粘贴出站 JSON 或节点链接')
  }
  if (trimmed.startsWith('{')) {
    return buildOutboundPatchFromXrayOutbound(JSON.parse(trimmed) as Record<string, unknown>, currentForm)
  }
  const endpoint = parseImportEndpoint(trimmed)
  const protocol = normalizeOutboundProtocol(trimmed.split(':', 1)[0] || currentForm.protocol)
  return {
    source_type: 'manual',
    tag: currentForm.tag || normalizeOutboundTag(decodeURIComponent(trimmed.split('#')[1] || protocol || 'outbound')),
    protocol,
    address: endpoint.address,
    port: endpoint.port || currentForm.port,
    uuid: protocol === 'socks' || protocol === 'http' ? endpoint.username || currentForm.uuid : endpoint.username || currentForm.uuid,
    password: endpoint.password || currentForm.password,
    method: endpoint.method || currentForm.method,
    security: endpoint.security || currentForm.security || 'none',
    server_name: endpoint.serverName || currentForm.server_name,
    alpn: endpoint.alpn || currentForm.alpn,
    grpc_service: endpoint.grpcService || currentForm.grpc_service,
    reality_public_key: endpoint.realityPublicKey || currentForm.reality_public_key,
    reality_short_id: endpoint.realityShortID || currentForm.reality_short_id,
    reality_fingerprint: endpoint.realityFingerprint || currentForm.reality_fingerprint || 'chrome',
    reality_spider_x: endpoint.realitySpiderX || currentForm.reality_spider_x,
    network: endpoint.network || currentForm.network || 'tcp',
    ws_path: endpoint.wsPath || currentForm.ws_path || '/',
    ws_host: endpoint.wsHost || currentForm.ws_host,
  }
}

export function buildOutboundPatchFromXrayOutbound(outbound: Record<string, unknown>, currentForm: XUIOutboundActionForm): Partial<XUIOutboundActionForm> {
  const protocol = normalizeOutboundProtocol(outbound.protocol || currentForm.protocol || 'vless')
  const settings = objectMap(outbound.settings)
  const stream = objectMap(outbound.streamSettings)
  const endpoint = endpointFromOutboundSettings(protocol, settings)
  const streamPatch = patchFromStreamSettings(stream, currentForm)
  return {
    source_type: 'manual',
    tag: usableEndpointValue(outbound.tag) || currentForm.tag,
    protocol,
    send_through: usableEndpointValue(outbound.sendThrough) || currentForm.send_through,
    address: endpoint.address || currentForm.address,
    port: endpoint.port || currentForm.port,
    uuid: endpoint.uuid || currentForm.uuid,
    flow: endpoint.flow || currentForm.flow,
    password: endpoint.password || currentForm.password,
    method: endpoint.method || currentForm.method,
    ...streamPatch,
  }
}

export function outboundFormToXrayOutbound(form: XUIOutboundActionForm): Record<string, unknown> {
  const protocol = normalizeOutboundProtocol(form.protocol)
  const outbound: Record<string, unknown> = {
    tag: form.tag.trim(),
    protocol,
  }
  if (form.send_through.trim()) {
    outbound.sendThrough = form.send_through.trim()
  }
  if (protocol === 'vless') {
    outbound.settings = {
      address: form.address.trim(),
      port: Number(form.port || 0),
      id: form.uuid.trim(),
      flow: form.flow.trim(),
      encryption: 'none',
    }
    outbound.streamSettings = outboundStreamSettingsFromForm(form)
  } else if (protocol === 'vmess') {
    outbound.settings = {
      vnext: [{
        address: form.address.trim(),
        port: Number(form.port || 0),
        users: [{ id: form.uuid.trim(), security: form.method.trim() || 'auto' }],
      }],
    }
    outbound.streamSettings = outboundStreamSettingsFromForm(form)
  } else if (protocol === 'shadowsocks') {
    outbound.settings = { servers: [{ address: form.address.trim(), port: Number(form.port || 0), method: form.method.trim(), password: form.password.trim() }] }
  } else if (protocol === 'socks' || protocol === 'http') {
    outbound.settings = { servers: [{ address: form.address.trim(), port: Number(form.port || 0), users: form.password.trim() ? [{ user: form.uuid.trim(), pass: form.password.trim() }] : [] }] }
  }
  return outbound
}

function endpointFromOutboundSettings(protocol: string, settings: Record<string, unknown>) {
  if (protocol === 'vless') {
    return {
      address: usableEndpointValue(settings.address),
      port: validPort(settings.port),
      uuid: usableEndpointValue(settings.id),
      flow: usableEndpointValue(settings.flow),
      password: '',
      method: '',
    }
  }
  if (protocol === 'vmess') {
    const vnext = objectMapArray(settings.vnext)[0] || {}
    const user = objectMapArray(vnext.users)[0] || {}
    return { address: usableEndpointValue(vnext.address), port: validPort(vnext.port), uuid: usableEndpointValue(user.id), flow: '', password: '', method: usableEndpointValue(user.security) }
  }
  const server = objectMapArray(settings.servers)[0] || {}
  const user = objectMapArray(server.users)[0] || {}
  return {
    address: usableEndpointValue(server.address),
    port: validPort(server.port),
    uuid: usableEndpointValue(user.user),
    flow: '',
    password: usableEndpointValue(server.password) || usableEndpointValue(user.pass),
    method: usableEndpointValue(server.method),
  }
}

function patchFromStreamSettings(stream: Record<string, unknown>, currentForm: XUIOutboundActionForm): Partial<XUIOutboundActionForm> {
  const security = usableEndpointValue(stream.security) || 'none'
  const network = usableEndpointValue(stream.network) || 'tcp'
  const tls = objectMap(stream.tlsSettings)
  const reality = objectMap(stream.realitySettings)
  const ws = objectMap(stream.wsSettings)
  const wsHeaders = objectMap(ws.headers)
  const grpc = objectMap(stream.grpcSettings)
  const tcp = objectMap(stream.tcpSettings)
  const http = objectMap(stream.httpSettings)
  return {
    security,
    network,
    server_name: usableEndpointValue(tls.serverName) || usableEndpointValue(reality.serverName) || currentForm.server_name,
    alpn: stringList(tls.alpn).join(','),
    reality_public_key: usableEndpointValue(reality.publicKey) || currentForm.reality_public_key,
    reality_short_id: usableEndpointValue(reality.shortId) || currentForm.reality_short_id,
    reality_fingerprint: usableEndpointValue(reality.fingerprint) || usableEndpointValue(tls.fingerprint) || currentForm.reality_fingerprint,
    reality_spider_x: usableEndpointValue(reality.spiderX) || currentForm.reality_spider_x,
    ws_path: usableEndpointValue(ws.path) || usableEndpointValue(http.path) || currentForm.ws_path,
    ws_host: usableEndpointValue(wsHeaders.Host) || stringList(http.host)[0] || currentForm.ws_host,
    grpc_service: usableEndpointValue(grpc.serviceName) || currentForm.grpc_service,
  }
}

function outboundStreamSettingsFromForm(form: XUIOutboundActionForm): Record<string, unknown> {
  const stream: Record<string, unknown> = {
    network: form.network || 'tcp',
    security: form.security || 'none',
  }
  if (form.network === 'tcp') {
    stream.tcpSettings = { header: { type: 'none' } }
  }
  if (form.network === 'ws') {
    stream.wsSettings = { path: form.ws_path || '/', headers: form.ws_host ? { Host: form.ws_host } : {} }
  }
  if (form.security === 'tls') {
    stream.tlsSettings = { serverName: form.server_name || undefined, alpn: form.alpn ? form.alpn.split(',').map((item) => item.trim()).filter(Boolean) : undefined }
  }
  if (form.security === 'reality') {
    stream.realitySettings = {
      serverName: form.server_name,
      publicKey: form.reality_public_key,
      shortId: form.reality_short_id,
      fingerprint: form.reality_fingerprint || 'chrome',
      spiderX: form.reality_spider_x,
    }
  }
  return stream
}

function parseImportEndpoint(importURL?: string): {
  address: string
  port: number
  network: string
  security: string
  serverName: string
  alpn: string
  wsPath: string
  wsHost: string
  grpcService: string
  username: string
  password: string
  method: string
  realityPublicKey: string
  realityShortID: string
  realityFingerprint: string
  realitySpiderX: string
} {
  const empty = {
    address: '',
    port: 0,
    network: '',
    security: '',
    serverName: '',
    alpn: '',
    wsPath: '',
    wsHost: '',
    grpcService: '',
    username: '',
    password: '',
    method: '',
    realityPublicKey: '',
    realityShortID: '',
    realityFingerprint: '',
    realitySpiderX: '',
  }
  if (!importURL) {
    return empty
  }
  try {
    if (importURL.startsWith('vmess://')) {
      const payload = JSON.parse(decodeBase64URL(importURL.slice('vmess://'.length))) as Record<string, unknown>
      const network = usableEndpointValue(payload.net)
      return {
        address: usableEndpointValue(payload.add),
        port: validPort(payload.port),
        network,
        security: usableEndpointValue(payload.tls),
        serverName: usableEndpointValue(payload.sni) || usableEndpointValue(payload.host),
        alpn: usableEndpointValue(payload.alpn),
        wsPath: network === 'grpc' ? '' : usableEndpointValue(payload.path),
        wsHost: usableEndpointValue(payload.host),
        grpcService: network === 'grpc' ? usableEndpointValue(payload.path) || usableEndpointValue(payload.serviceName) : usableEndpointValue(payload.serviceName),
        username: usableEndpointValue(payload.id),
        password: '',
        method: usableEndpointValue(payload.scy),
        realityPublicKey: usableEndpointValue(payload.pbk),
        realityShortID: usableEndpointValue(payload.sid),
        realityFingerprint: usableEndpointValue(payload.fp),
        realitySpiderX: usableEndpointValue(payload.spx),
      }
    }
    const parsed = new URL(importURL)
    const params = parsed.searchParams
    const network = usableEndpointValue(params.get('type')) || usableEndpointValue(params.get('network'))
    const userInfo = parseShareUserInfo(parsed)
    return {
      address: usableEndpointValue(parsed.hostname),
      port: validPort(parsed.port),
      network,
      security: usableEndpointValue(params.get('security')) || usableEndpointValue(params.get('tls')),
      serverName: usableEndpointValue(params.get('sni')) || usableEndpointValue(params.get('peer')) || usableEndpointValue(params.get('host')),
      alpn: usableEndpointValue(params.get('alpn')),
      wsPath: network === 'grpc' ? '' : usableEndpointValue(params.get('path')),
      wsHost: usableEndpointValue(params.get('host')),
      grpcService: usableEndpointValue(params.get('serviceName')) || usableEndpointValue(params.get('service')) || (network === 'grpc' ? usableEndpointValue(params.get('path')) : ''),
      username: userInfo.username,
      password: userInfo.password,
      method: userInfo.method,
      realityPublicKey: usableEndpointValue(params.get('pbk')) || usableEndpointValue(params.get('publicKey')),
      realityShortID: usableEndpointValue(params.get('sid')) || usableEndpointValue(params.get('shortId')),
      realityFingerprint: usableEndpointValue(params.get('fp')) || usableEndpointValue(params.get('fingerprint')),
      realitySpiderX: usableEndpointValue(params.get('spx')) || usableEndpointValue(params.get('spiderX')),
    }
  } catch {
    return empty
  }
}

function parseShareUserInfo(parsed: URL): { username: string; password: string; method: string } {
  const username = usableEndpointValue(decodeURIComponent(parsed.username || ''))
  const password = usableEndpointValue(decodeURIComponent(parsed.password || ''))
  if (parsed.protocol === 'ss:') {
    const decoded = username.includes(':') ? username : tryDecodeBase64(username)
    const separator = decoded.indexOf(':')
    if (separator > 0) {
      return {
        username: '',
        method: decoded.slice(0, separator),
        password: decoded.slice(separator + 1),
      }
    }
  }
  return {
    username,
    password,
    method: '',
  }
}

function tryDecodeBase64(value: string): string {
  try {
    return decodeBase64URL(value)
  } catch {
    return ''
  }
}

function usableEndpointValue(value: unknown): string {
  const text = String(value ?? '').trim()
  if (!text || /^(undefined|null|nan)$/i.test(text)) {
    return ''
  }
  return text
}

function objectMap(value: unknown): Record<string, unknown> {
  if (typeof value === 'string') {
    try {
      const parsed = JSON.parse(value) as unknown
      return parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed as Record<string, unknown> : {}
    } catch {
      return {}
    }
  }
  return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {}
}

function objectMapArray(value: unknown): Array<Record<string, unknown>> {
  if (typeof value === 'string') {
    try {
      const parsed = JSON.parse(value) as unknown
      return objectMapArray(parsed)
    } catch {
      return []
    }
  }
  return Array.isArray(value) ? value.filter((item) => item && typeof item === 'object' && !Array.isArray(item)) as Array<Record<string, unknown>> : []
}

function stringList(value: unknown): string[] {
  if (Array.isArray(value)) {
    return value.map((item) => usableEndpointValue(item)).filter(Boolean)
  }
  const text = usableEndpointValue(value)
  return text ? [text] : []
}

function normalizeOutboundProtocol(value: unknown): string {
  const protocol = usableEndpointValue(value).toLowerCase()
  if (protocol === 'socks5') {
    return 'socks'
  }
  if (protocol === 'ss') {
    return 'shadowsocks'
  }
  return protocol
}

function validPort(value: unknown): number {
  const port = Number(value || 0)
  return Number.isInteger(port) && port > 0 && port <= 65535 ? port : 0
}

function parsePortFromText(...values: Array<string | undefined>): number {
  for (const value of values) {
    const text = value || ''
    const match = /(?:^|[^0-9])([1-9][0-9]{1,4})(?=$|[^0-9])/.exec(text)
    if (!match) {
      continue
    }
    const port = validPort(match[1])
    if (port) {
      return port
    }
  }
  return 0
}

function decodeBase64URL(value: string): string {
  const normalized = value.trim().replace(/-/g, '+').replace(/_/g, '/')
  const padded = normalized.padEnd(Math.ceil(normalized.length / 4) * 4, '=')
  return decodeURIComponent(escape(window.atob(padded)))
}

function hostnameFromURL(raw?: string): string {
  if (!raw) {
    return ''
  }
  try {
    return new URL(raw).hostname
  } catch {
    return ''
  }
}

function normalizeOutboundTag(value: string): string {
  const normalized = value
    .toLowerCase()
    .replace(/[^a-z0-9._-]+/g, '-')
    .replace(/-+/g, '-')
    .replace(/^-|-$/g, '')
  return normalized || 'relay-link'
}
