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
  const protocol = (sourceNode.protocol || sourceClient.protocol || currentForm.protocol || 'freedom').toLowerCase()
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
    uuid: protocol === 'socks' ? sourceClient.email || '' : sourceClient.auth_uuid || currentForm.uuid,
    password: sourceClient.auth_password || currentForm.password,
    flow: sourceClient.flow || '',
    security: sourceNode.security || importEndpoint.security || 'none',
    server_name: sourceNode.tls_server_name || sourceNode.ws_host || importEndpoint.serverName || currentForm.server_name,
    network: sourceNode.network || importEndpoint.network || 'tcp',
    ws_path: sourceNode.ws_path || importEndpoint.wsPath || '/',
    ws_host: sourceNode.ws_host || importEndpoint.wsHost || '',
  }
}

function parseImportEndpoint(importURL?: string): { address: string; port: number; network: string; security: string; serverName: string; wsPath: string; wsHost: string } {
  const empty = { address: '', port: 0, network: '', security: '', serverName: '', wsPath: '', wsHost: '' }
  if (!importURL) {
    return empty
  }
  try {
    if (importURL.startsWith('vmess://')) {
      const payload = JSON.parse(decodeBase64URL(importURL.slice('vmess://'.length))) as Record<string, unknown>
      return {
        address: usableEndpointValue(payload.add),
        port: validPort(payload.port),
        network: usableEndpointValue(payload.net),
        security: usableEndpointValue(payload.tls),
        serverName: usableEndpointValue(payload.sni) || usableEndpointValue(payload.host),
        wsPath: usableEndpointValue(payload.path),
        wsHost: usableEndpointValue(payload.host),
      }
    }
    const parsed = new URL(importURL)
    const params = parsed.searchParams
    return {
      address: usableEndpointValue(parsed.hostname),
      port: validPort(parsed.port),
      network: usableEndpointValue(params.get('type')) || usableEndpointValue(params.get('network')),
      security: usableEndpointValue(params.get('security')) || usableEndpointValue(params.get('tls')),
      serverName: usableEndpointValue(params.get('sni')) || usableEndpointValue(params.get('peer')) || usableEndpointValue(params.get('host')),
      wsPath: usableEndpointValue(params.get('path')),
      wsHost: usableEndpointValue(params.get('host')),
    }
  } catch {
    return empty
  }
}

function usableEndpointValue(value: unknown): string {
  const text = String(value ?? '').trim()
  if (!text || /^(undefined|null|nan)$/i.test(text)) {
    return ''
  }
  return text
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
