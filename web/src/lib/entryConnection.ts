import type { AgentEntryConfig, AgentEntryMapping, EntryEndpoint, XUINodeView } from '../types'

export interface EntryTarget {
  type: 'xui' | 'realm' | 'haproxy'
  id: string
  name: string
  port: number
  transport: 'tcp' | 'udp' | 'both'
  protocol?: string
}

export function xuiEntryTransport(node: XUINodeView): EntryTarget['transport'] {
  if (['hysteria', 'hysteria2', 'tuic', 'wireguard'].includes((node.protocol || '').toLowerCase()) || ['kcp', 'mkcp', 'quic', 'udp'].includes((node.network || '').toLowerCase())) return 'udp'
  return node.protocol?.toLowerCase() === 'shadowsocks' ? 'both' : 'tcp'
}

export function entryTargets(entry: AgentEntryConfig, nodes: XUINodeView[]): EntryTarget[] {
  return [
    ...nodes.filter((node) => !node.realm_target_agent_id && !['realm', 'haproxy'].includes(node.route?.match_scope)).map((node) => ({
      type: 'xui' as const, id: String(node.id), name: node.remark || node.tag || `入站 #${node.id}`, port: node.port || 0, transport: xuiEntryTransport(node), protocol: node.protocol,
    })),
    ...(entry.port_forwarding?.rules || []).map((rule) => ({
      type: 'realm' as const, id: rule.id || `realm-${rule.listen_port}`, name: rule.name || `Realm :${rule.listen_port}`, port: rule.listen_port || 0, transport: (rule.network === 'udp' ? 'udp' : ['both', 'tcp+udp'].includes(rule.network || '') ? 'both' : 'tcp') as EntryTarget['transport'],
    })),
    ...(entry.haproxy?.rules || []).map((rule) => ({
      type: 'haproxy' as const, id: rule.id || `haproxy-${rule.listen_port}`, name: rule.name || `HAProxy :${rule.listen_port}`, port: rule.listen_port || 0, transport: 'tcp' as const,
    })),
  ]
}

export function entryUsesDockerMapping(entry: AgentEntryConfig, type?: string): boolean {
  return type === 'xui' && entry.connection?.deployment === 'docker' && entry.connection?.docker_network === 'bridge'
}

export function validEntryHost(host: string): boolean {
  const value = host.trim()
  if (value.includes(':')) {
    try { const parsed = new URL(`http://[${value.replace(/^\[|\]$/g, '')}]/`); return parsed.hostname !== '[::]' && parsed.hostname !== '[::1]' } catch { return false }
  }
  if (!value || value.length > 253 || /[:/@?#*\[\]\s\\]/.test(value) || value.toLowerCase() === 'localhost' || /^127\./.test(value) || value === '0.0.0.0') return false
  if (/^[\d.]+$/.test(value)) return value.split('.').length === 4 && value.split('.').every((part) => Number(part) <= 255 && part !== '')
  return value.replace(/\.$/, '').split('.').every((part) => part.length <= 63 && /^[a-z\d](?:[a-z\d-]*[a-z\d])?$/i.test(part))
}

export function entryMappingPort(entry: AgentEntryConfig, mapping: AgentEntryMapping): number {
  return Number(entry.connection?.public_access === 'nat' ? mapping.external_port : entryUsesDockerMapping(entry, mapping.target_type) ? mapping.host_port : mapping.internal_port) || 0
}

const validPort = (port?: number) => Number.isInteger(port) && Number(port) > 0 && Number(port) <= 65535
const mappingHost = (entry: AgentEntryConfig, mapping: AgentEntryMapping) => (mapping.address || entry.connection?.public_address || entry.import_domain || '').trim()

export function validateEntryConnection(entry: AgentEntryConfig): string {
  if (entry.connection?.public_address && !validEntryHost(entry.connection.public_address)) return '公网入口请填写有效域名或 IP，不包含端口或 URL 路径'
  const defaults = new Set<string>()
  const active: AgentEntryMapping[] = []
  for (const [index, mapping] of (entry.mappings || []).entries()) {
    if (!mapping.use_for_export) continue
    const prefix = `第 ${index + 1} 条连接映射：`
    if (!mapping.target_id || !['xui', 'realm', 'haproxy'].includes(mapping.target_type || '')) return prefix + '请选择节点'
    if (!validPort(mapping.internal_port)) return prefix + '内部端口必须为 1–65535'
    if (!['tcp', 'udp', 'both'].includes(mapping.transport || '')) return prefix + '请选择 TCP、UDP 或 TCP+UDP'
    if (entryUsesDockerMapping(entry, mapping.target_type) && !validPort(mapping.host_port)) return prefix + '请填写服务器端口（1–65535）'
    if (entry.connection?.public_access === 'nat' && !validPort(mapping.external_port)) return prefix + '请填写公网端口（1–65535）'
    if (!validEntryHost(mappingHost(entry, mapping))) return prefix + '请填写公网入口域名或 IP'
    const key = `${mapping.target_type}:${mapping.target_id}`
    if (mapping.default_export && defaults.has(key)) return prefix + '同一节点只能有一个默认导出入口'
    if (mapping.default_export) defaults.add(key)
    if (active.some((other) => `${other.target_type}:${other.target_id}` !== key && entryUsesDockerMapping(entry, other.target_type) && entryUsesDockerMapping(entry, mapping.target_type) && other.host_port === mapping.host_port && (other.transport === mapping.transport || other.transport === 'both' || mapping.transport === 'both'))) return prefix + '服务器端口与其他 Docker 节点冲突'
    if (active.some((other) => `${other.target_type}:${other.target_id}` !== key && mappingHost(entry, other).toLowerCase() === mappingHost(entry, mapping).toLowerCase() && entryMappingPort(entry, other) === entryMappingPort(entry, mapping) && (other.transport === mapping.transport || other.transport === 'both' || mapping.transport === 'both'))) return prefix + '公网地址、端口与其他节点冲突'
    active.push(mapping)
  }
  return ''
}

// Preview mirrors the server resolver; exported links always use the server result.
export function resolveEntryEndpoint(entry: AgentEntryConfig, target: EntryTarget, fallbackHost = ''): EntryEndpoint {
  const bridge = entryUsesDockerMapping(entry, target.type)
  const mapped = bridge || entry.connection?.public_access === 'nat'
  const base: EntryEndpoint = { internal_port: target.port, host_port: target.port, transport: target.transport, mapped }
  const fail = (error: string): EntryEndpoint => ({ ...base, error })
  if (!validPort(target.port)) return fail('节点监听端口无效')
  const error = validateEntryConnection(entry)
  if (error) return fail(error)
  const rows = (entry.mappings || []).filter((m) => m.use_for_export && m.target_type === target.type && m.target_id === target.id)
  if (!rows.length) {
    if (mapped) return fail('缺少节点端口映射，请在连接与端口中配置')
    const address = (entry.connection?.public_address || fallbackHost).trim()
    return address ? { ...base, address, port: target.port } : fail('缺少对外连接地址')
  }
  const selected = rows.length === 1 ? rows[0] : rows.find((m) => m.default_export)
  if (!selected) return fail('存在多个映射，请指定默认导出入口')
  if (selected.internal_port !== target.port) return fail('节点监听端口已变化，请重新确认映射')
  if (selected.transport !== 'both' && selected.transport !== target.transport) return fail(`映射未覆盖节点所需的 ${target.transport.toUpperCase()}`)
  return { ...base, address: mappingHost(entry, selected).replace(/^\[|\]$/g, ''), host_port: bridge ? selected.host_port : target.port, port: entryMappingPort(entry, selected) }
}

export function formatEntryEndpoint(endpoint?: EntryEndpoint): string {
  if (!endpoint?.address || !endpoint.port || endpoint.error) return ''
  return `${endpoint.address.includes(':') ? `[${endpoint.address.replace(/^\[|\]$/g, '')}]` : endpoint.address}:${endpoint.port}`
}
