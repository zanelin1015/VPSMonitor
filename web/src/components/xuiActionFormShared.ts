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
    importEndpoint.address ||
    sourceOverview.summary.observed_ip ||
    sourceOverview.summary.public_ipv4 ||
    sourceOverview.summary.public_ipv6 ||
    hostnameFromURL(sourceOverview.base_url) ||
    ''
  const protocol = (sourceNode.protocol || sourceClient.protocol || currentForm.protocol || 'freedom').toLowerCase()
  const tagParts = [
    sourceOverview.agent_name || sourceOverview.agent_id,
    sourceNode.tag || sourceNode.remark || String(sourceNode.id),
    sourceClient.email || 'link',
  ]

  return {
    tag: normalizeOutboundTag(tagParts.join('-')),
    protocol,
    address,
    port: sourceNode.port || importEndpoint.port || currentForm.port,
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
        address: String(payload.add || ''),
        port: Number(payload.port || 0),
        network: String(payload.net || ''),
        security: String(payload.tls || ''),
        serverName: String(payload.sni || payload.host || ''),
        wsPath: String(payload.path || ''),
        wsHost: String(payload.host || ''),
      }
    }
    const parsed = new URL(importURL)
    const params = parsed.searchParams
    return {
      address: parsed.hostname,
      port: Number(parsed.port || 0),
      network: params.get('type') || params.get('network') || '',
      security: params.get('security') || params.get('tls') || '',
      serverName: params.get('sni') || params.get('peer') || params.get('host') || '',
      wsPath: params.get('path') || '',
      wsHost: params.get('host') || '',
    }
  } catch {
    return empty
  }
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
