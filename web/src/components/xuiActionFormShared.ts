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
