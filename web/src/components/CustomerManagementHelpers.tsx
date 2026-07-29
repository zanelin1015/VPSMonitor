import { Space, Tag, Typography } from 'antd'

import type { AdminUser, AreaManagerAdminView, AreaManagerAssignment, AreaManagerOutboundGrant, CustomerAdminView, CustomerAssignment, CustomerAssignmentDraft, DashboardAgentView, RealmForwardRule, XUIClientBillingConfig, XUIClientView, XUINodeView, XUIOverview } from '../types'

const { Text } = Typography

export const DEFAULT_ACCOUNT_PASSWORD = '12345678'

export type ManagementTabKey = 'area' | 'customers'

export interface CustomerFormState {
  username: string
  password: string
  display_name: string
  enabled: boolean
}

export interface AssignmentFormState {
  agent_id: string
  client_key: string
  inbound_id: number
  inbound_tag: string
  client_email: string
  public_client_name: string
  traffic_multiplier: number
  revenue_amount: number
  revenue_currency: 'CNY' | 'USDT'
  revenue_cycle: 'month' | 'quarter' | 'year'
  enabled: boolean
}

export interface AreaManagerAssignmentDraft {
  agent_id: string
  inbound_id: number
  inbound_tag: string
  client_email: string
  public_client_name: string
  enabled: boolean
}

export interface AreaManagerOutboundGrantDraft {
  agent_id: string
  outbound_tag: string
}

export interface AreaManagerFormState {
  username: string
  password: string
  display_name: string
  enabled: boolean
  agent_ids: string[]
  billing_enabled: boolean
  revenue_amount: number
  revenue_currency: 'CNY' | 'USDT'
  revenue_cycle: 'month' | 'quarter' | 'year'
  grant_agent_id: string
  xui_grant_agent_id: string
  outbound_create_enabled: boolean
  outbound_grant_agent_id: string
  outbound_grants: AreaManagerOutboundGrantDraft[]
  assignments: AreaManagerAssignmentDraft[]
}

export interface AreaBatchAssignmentFormState {
  manager_id: number | null
  agent_id: string
  xui_agent_id: string
  selected_realm_keys: string[]
  selected_xui_keys: string[]
}

export const emptyCustomerForm: CustomerFormState = {
  username: '',
  password: '',
  display_name: '',
  enabled: true,
}

export const emptyAssignmentForm: AssignmentFormState = {
  agent_id: '',
  client_key: '',
  inbound_id: 0,
  inbound_tag: '',
  client_email: '',
  public_client_name: '',
  traffic_multiplier: 1,
  revenue_amount: 0,
  revenue_currency: 'CNY',
  revenue_cycle: 'month',
  enabled: true,
}

export const emptyAreaManagerForm: AreaManagerFormState = {
  username: '',
  password: '',
  display_name: '',
  enabled: true,
  agent_ids: [],
  billing_enabled: false,
  revenue_amount: 0,
  revenue_currency: 'CNY',
  revenue_cycle: 'month',
  grant_agent_id: '',
  xui_grant_agent_id: '',
  outbound_create_enabled: false,
  outbound_grant_agent_id: '',
  outbound_grants: [],
  assignments: [],
}

export const emptyAreaBatchAssignmentForm: AreaBatchAssignmentFormState = {
  manager_id: null,
  agent_id: '',
  xui_agent_id: '',
  selected_realm_keys: [],
  selected_xui_keys: [],
}

export function isAreaManagerAdminUser(user: AdminUser | null): boolean {
  if (!user) {
    return false
  }
  if (user.role === 'area_manager') {
    return true
  }
  if (user.role === 'admin') {
    return false
  }
  return Boolean((user.agent_ids || []).length || (user.id && user.id !== 1))
}

export function clientKey(client: XUIClientView): string {
  return `client:${client.inbound_id}::${client.email || ''}`
}

export function buildAssignmentTargetOptions(overview: XUIOverview | null) {
  const clients = (overview?.clients || [])
    .filter((client) => !isRealmForwardedClientOption(client))
    .map((client) => ({
      value: clientKey(client),
      label: clientLabel(client),
      client,
      node: undefined as XUINodeView | undefined,
    }))
  const nodes = (overview?.nodes || []).map((node) => ({
    value: nodeKey(node),
    label: nodeLabel(node),
    client: undefined as XUIClientView | undefined,
    node,
  }))
  return [...clients, ...nodes]
}

export function buildClientAssignmentTreeData(agentID: string, overview: XUIOverview | null, agents: DashboardAgentView[]) {
  if (!agentID || !overview) {
    return []
  }
  return [{
    title: agentName(agentID, agents),
    value: `agent:${agentID}`,
    selectable: false,
    children: overviewNodeGroups(overview, false).map(({ node, clients }) => ({
      title: nodeLabel(node),
      value: nodeKey(node),
      selectable: node.can_assign_all_clients !== false,
      children: clients.map((client) => ({
        title: clientTreeTitle(client),
        value: clientKey(client),
      })),
    })),
  }]
}

export function buildAreaAssignmentTreeData(agentID: string, overview: XUIOverview | null, agents: DashboardAgentView[]) {
  if (!agentID || !overview) {
    return []
  }
  return [{
    title: agentName(agentID, agents),
    value: `agent:${agentID}`,
    selectable: false,
    children: overviewNodeGroups(overview, true).map(({ node, clients }) => ({
      title: nodeLabel(node),
      value: areaAssignmentKey(areaAssignmentDraftFromTargetOption(agentID, { node }, agents)),
      children: clients.map((client) => ({
        title: clientTreeTitle(client),
        value: areaAssignmentKey(areaAssignmentDraftFromTargetOption(agentID, { client }, agents)),
      })),
    })),
  }]
}

function overviewNodeGroups(overview: XUIOverview | null, excludeRealmForwarded: boolean) {
  const nodes = [...(overview?.nodes || [])]
  const clients = (overview?.clients || []).filter((client) => !excludeRealmForwarded || !isRealmForwardedClientOption(client))
  const nodeIDs = new Set(nodes.map((node) => Number(node.id || 0)))
  for (const client of clients) {
    if (!nodeIDs.has(Number(client.inbound_id || 0))) {
      nodes.push({
        id: client.inbound_id,
        tag: client.inbound_tag || '',
        remark: client.inbound_remark || client.inbound_tag || `Inbound #${client.inbound_id}`,
        protocol: client.protocol || '',
        enabled: client.enabled !== false,
        route: client.route,
      })
      nodeIDs.add(Number(client.inbound_id || 0))
    }
  }
  return nodes.map((node) => ({
    node,
    clients: clients.filter((client) => Number(client.inbound_id || 0) === Number(node.id || 0)),
  }))
}

export function assignmentNodeKeys(agentID: string, overview: XUIOverview | null, agents: DashboardAgentView[]): string[] {
  if (!agentID) {
    return []
  }
  return (overview?.nodes || []).map((node) => areaAssignmentKey(areaAssignmentDraftFromTargetOption(agentID, { node }, agents)))
}

export function buildRealmGrantOptions(agentID: string, agents: DashboardAgentView[]) {
  const agent = agents.find((item) => item.agent_id === agentID)
  const realmRules = agent?.entry?.port_forwarding?.rules || []
  const haProxyRules = agent?.entry?.haproxy?.enabled === false ? [] : agent?.entry?.haproxy?.rules || []
  const seenPorts = new Set<number>()
  const realmOptions = realmRules
    .filter((rule) => rule.enabled !== false && Number(rule.listen_port || 0) > 0)
    .filter((rule) => {
      const port = Number(rule.listen_port || 0)
      if (seenPorts.has(port)) {
        return false
      }
      seenPorts.add(port)
      return true
    })
    .map((rule) => {
      const assignment = areaAssignmentDraftFromRealmRule(agentID, rule, agents)
      return {
        value: areaAssignmentKey(assignment),
        label: realmGrantLabel(rule),
        assignment,
        rule,
      }
    })
  const haProxyOptions = haProxyRules
    .filter((rule) => rule.enabled !== false && Number(rule.listen_port || 0) > 0 && Number(rule.primary?.port || 0) > 0)
    .filter((rule) => {
      const port = Number(rule.listen_port || 0)
      if (seenPorts.has(port)) {
        return false
      }
      seenPorts.add(port)
      return true
    })
    .map((rule) => {
      const listenPort = Number(rule.listen_port || 0)
      const label = haProxyGrantLabel(rule)
      const assignment: AreaManagerAssignmentDraft = {
        agent_id: agentID,
        inbound_id: listenPort,
        inbound_tag: `haproxy:${listenPort}`,
        client_email: '',
        public_client_name: `${agentName(agentID, agents)} / ${label}`,
        enabled: true,
      }
      const targetProbe: RealmForwardRule = {
        enabled: true,
        target_agent_id: rule.primary?.agent_id || '',
        target_address: rule.primary?.address || '',
        target_port: Number(rule.primary?.port || 0),
      }
      return {
        value: areaAssignmentKey(assignment),
        label,
        assignment,
        rule: targetProbe,
      }
    })
  return [...realmOptions, ...haProxyOptions]
}

export function realmRuleTargetAgentID(rule: RealmForwardRule, agents: DashboardAgentView[]): string {
  const explicit = (rule.target_agent_id || '').trim()
  if (explicit) {
    return explicit
  }
  const target = normalizeEndpointHost(rule.target_address || '')
  if (!target) {
    return ''
  }
  return agents.find((agent) => agentAddressCandidates(agent).some((candidate) => normalizeEndpointHost(candidate) === target))?.agent_id || ''
}

function agentAddressCandidates(agent: DashboardAgentView): string[] {
  return [
    agent.agent_id,
    agent.agent_name || '',
    agent.customer_display_name || '',
    agent.entry?.import_domain || '',
    ...(agent.entry?.addresses || []),
    agent.summary?.public_ipv4 || '',
    agent.summary?.public_ipv6 || '',
    agent.summary?.observed_ip || '',
    agent.summary?.server_seen_ip || '',
    agent.summary?.hostname || '',
  ].filter(Boolean)
}

function normalizeEndpointHost(value: string): string {
  let text = String(value || '').trim().toLowerCase()
  if (!text) {
    return ''
  }
  if (text.includes('://')) {
    try {
      text = new URL(text).hostname
    } catch {
      text = text.replace(/^[a-z][a-z0-9+.-]*:\/\//, '')
    }
  }
  text = text.replace(/^\[/, '').replace(/\]$/, '')
  if (text.includes('/') || text.includes('?') || text.includes('#')) {
    text = text.split(/[/?#]/)[0]
  }
  if (text.includes(':') && !text.includes('::')) {
    text = text.split(':')[0]
  }
  return text
}

export function findRealmTargetNode(overview: XUIOverview | null | undefined, rule: RealmForwardRule): XUINodeView | null {
  const targetPort = Number(rule.target_port || 0)
  if (!overview || targetPort <= 0) {
    return null
  }
  const node = (overview.nodes || []).find((item) => Number(item.port || 0) === targetPort) ||
    (overview.nodes || []).find((item) => Number(item.id || 0) === targetPort)
  if (node) {
    return node
  }
  const client = (overview.clients || []).find((item) => Number(item.inbound_id || 0) === targetPort)
  if (!client) {
    return null
  }
  return {
    id: client.inbound_id,
    tag: client.inbound_tag || '',
    remark: client.inbound_remark || client.inbound_tag || `Inbound #${client.inbound_id}`,
    protocol: client.protocol || '',
    enabled: client.enabled !== false,
    route: client.route,
  }
}

export function areaAssignmentDraftFromRealmRule(agentID: string, rule: RealmForwardRule, agents: DashboardAgentView[]): AreaManagerAssignmentDraft {
  const listenPort = Number(rule.listen_port || 0)
  const label = realmGrantLabel(rule)
  return {
    agent_id: agentID,
    inbound_id: listenPort,
    inbound_tag: `realm:${listenPort}`,
    client_email: '',
    public_client_name: `${agentName(agentID, agents)} / ${label}`,
    enabled: true,
  }
}

function realmGrantLabel(rule: RealmForwardRule) {
  const listen = rule.listen_port || '-'
  const name = (rule.name || '').trim()
  return name ? `${name} (${listen})` : `Realm 端口 ${listen}`
}

function haProxyGrantLabel(rule: { name?: string; listen_port?: number }) {
  const listen = rule.listen_port || '-'
  const name = (rule.name || '').trim()
  return name ? `${name} (${listen})` : `HAProxy 端口 ${listen}`
}

function realmAssignmentDisplayName(item: { agent_id: string; inbound_id: number; inbound_tag?: string; public_client_name?: string }, agents: DashboardAgentView[]): string {
  const agentPrefix = `${agentName(item.agent_id, agents)} / `
  const publicName = (item.public_client_name || '').trim()
  if (publicName) {
    return publicName.startsWith(agentPrefix) ? publicName : `${agentPrefix}${publicName}`
  }
  const typeLabel = (item.inbound_tag || '').trim().toLowerCase().startsWith('haproxy:') ? 'HAProxy' : 'Realm'
  return `${agentPrefix}${typeLabel} 端口 ${item.inbound_id}`
}

export function areaAssignmentDraftFromTargetOption(
  agentID: string,
  option: { client?: XUIClientView; node?: XUINodeView },
  agents: DashboardAgentView[],
): AreaManagerAssignmentDraft {
  if (option.client) {
    const client = option.client
    return {
      agent_id: agentID,
      inbound_id: client.inbound_id,
      inbound_tag: client.inbound_tag || '',
      client_email: client.email || '',
      public_client_name: defaultPublicClientName(client, agentID, agents),
      enabled: true,
    }
  }
  const node = option.node
  return {
    agent_id: agentID,
    inbound_id: node?.id || 0,
    inbound_tag: node?.tag || '',
    client_email: '',
    public_client_name: node ? defaultPublicNodeName(node, agentID, agents) : agentName(agentID, agents),
    enabled: true,
  }
}

export function normalizeAreaManagerAssignmentDrafts(items: Array<AreaManagerAssignment | AreaManagerAssignmentDraft>): AreaManagerAssignmentDraft[] {
  const result: AreaManagerAssignmentDraft[] = []
  const seen = new Set<string>()
  for (const item of items || []) {
    if (isLegacyRealmForwardedClientAssignment(item)) {
      continue
    }
    const draft: AreaManagerAssignmentDraft = {
      agent_id: item.agent_id,
      inbound_id: Number(item.inbound_id || 0),
      inbound_tag: item.inbound_tag || '',
      client_email: item.client_email || '',
      public_client_name: item.public_client_name || item.client_email || item.inbound_tag || `Inbound #${item.inbound_id}`,
      enabled: item.enabled !== false,
    }
    if (!draft.agent_id || !draft.inbound_id) {
      continue
    }
    if (!draft.client_email && isRealmAssignmentTagValue(draft.inbound_tag)) {
      const prefix = draft.inbound_tag.trim().toLowerCase().startsWith('haproxy:') ? 'haproxy' : 'realm'
      draft.inbound_tag = `${prefix}:${draft.inbound_id}`
      draft.public_client_name = draft.public_client_name || `${prefix === 'haproxy' ? 'HAProxy' : 'Realm'} ${draft.inbound_id}`
    }
    const key = areaAssignmentKey(draft)
    if (seen.has(key)) {
      continue
    }
    seen.add(key)
    result.push(draft)
  }
  return dedupeAreaManagerAssignmentDrafts(result)
}

export function dedupeAreaManagerAssignmentDrafts(items: AreaManagerAssignmentDraft[]): AreaManagerAssignmentDraft[] {
  const result: AreaManagerAssignmentDraft[] = []
  const seen = new Set<string>()
  const exactClientAssignments = items.filter((item) => item.client_email && !isRealmAssignmentTagValue(item.inbound_tag || ''))
  for (const item of items) {
    if (!item.client_email && !isRealmAssignmentTagValue(item.inbound_tag || '') && exactClientAssignments.some((exact) => assignmentMatchesInbound(exact, item.agent_id, item.inbound_id, item.inbound_tag))) {
      continue
    }
    const key = areaAssignmentKey(item)
    if (seen.has(key)) {
      continue
    }
    seen.add(key)
    result.push(item)
  }
  return result
}

export function areaOutboundGrantKey(item: { agent_id: string; outbound_tag: string }): string {
  return `${encodeURIComponent(item.agent_id || '')}::${encodeURIComponent(item.outbound_tag || '')}`
}

export function normalizeAreaManagerOutboundGrants(items: Array<AreaManagerOutboundGrant | AreaManagerOutboundGrantDraft>): AreaManagerOutboundGrantDraft[] {
  const result: AreaManagerOutboundGrantDraft[] = []
  const seen = new Set<string>()
  for (const item of items || []) {
    const grant = {
      agent_id: (item.agent_id || '').trim(),
      outbound_tag: (item.outbound_tag || '').trim(),
    }
    if (!grant.agent_id || !grant.outbound_tag) {
      continue
    }
    const key = areaOutboundGrantKey(grant)
    if (seen.has(key)) {
      continue
    }
    seen.add(key)
    result.push(grant)
  }
  return result
}

export function assignmentMatchesInbound(item: { agent_id: string; inbound_id: number; inbound_tag?: string }, agentID: string, inboundID: number, inboundTag?: string): boolean {
  if ((item.agent_id || '') !== agentID || Number(item.inbound_id || 0) !== Number(inboundID || 0)) {
    return false
  }
  const leftTag = (item.inbound_tag || '').trim().toLowerCase()
  const rightTag = (inboundTag || '').trim().toLowerCase()
  return !leftTag || !rightTag || leftTag === rightTag
}

export function firstRealmAssignmentAgentID(items: AreaManagerAssignment[], agents: DashboardAgentView[]): string {
  return normalizeAreaManagerAssignmentDrafts(items).find((item) => isRealmAssignmentDraft(item, agents))?.agent_id || ''
}

export function firstXUIAssignmentAgentID(items: AreaManagerAssignment[], agents: DashboardAgentView[]): string {
  return normalizeAreaManagerAssignmentDrafts(items).find((item) => !isRealmAssignmentDraft(item, agents))?.agent_id || ''
}

function isRealmForwardedClientOption(client: XUIClientView): boolean {
  return Boolean(
    client.forward_type === 'realm' ||
    client.forward_type === 'haproxy' ||
    client.is_realm_forwarded ||
    client.realm_source_agent_id ||
    client.realm_target_agent_id ||
    looksLikeRealmForwardedInboundTag(client.inbound_tag || '') ||
    looksLikeRealmForwardedInboundTag(client.inbound_remark || ''),
  )
}

function isLegacyRealmForwardedClientAssignment(item: { inbound_tag?: string; client_email?: string; public_client_name?: string }): boolean {
  if (!item.client_email) {
    return false
  }
  return looksLikeRealmForwardedInboundTag(item.inbound_tag || '') || looksLikeRealmForwardedInboundTag(item.public_client_name || '')
}

function looksLikeRealmForwardedInboundTag(value: string): boolean {
  return /^(realm|haproxy)\s+\d+\s*->/i.test(value.trim())
}

export function isRealmAssignmentDraft(item: { agent_id: string; inbound_id: number; inbound_tag?: string; client_email?: string }, agents: DashboardAgentView[]) {
  if (item.client_email) {
    return false
  }
  if (isRealmAssignmentTagValue(item.inbound_tag || '')) {
    return true
  }
  const rules = agents.find((agent) => agent.agent_id === item.agent_id)?.entry?.port_forwarding?.rules || []
  const haProxyRules = agents.find((agent) => agent.agent_id === item.agent_id)?.entry?.haproxy?.rules || []
  return rules.some((rule) => Number(rule.listen_port || 0) === Number(item.inbound_id || 0)) ||
    haProxyRules.some((rule) => Number(rule.listen_port || 0) === Number(item.inbound_id || 0))
}

export function areaAssignmentKey(item: { agent_id: string; inbound_id: number; inbound_tag?: string; client_email?: string }): string {
  return [
    item.agent_id || '',
    String(item.inbound_id || 0),
    item.inbound_tag || '',
    item.client_email || '',
  ].map((part) => encodeURIComponent(part)).join('::')
}

export function areaAssignmentLabel(item: { agent_id: string; inbound_id: number; inbound_tag?: string; client_email?: string; public_client_name?: string }, agents: DashboardAgentView[]): string {
  if (isRealmAssignmentDraft(item, agents)) {
    return realmAssignmentDisplayName(item, agents)
  }
  const scope = item.client_email ? item.client_email : '整个节点'
  const name = item.public_client_name || item.inbound_tag || `Inbound #${item.inbound_id}`
  return `${agentName(item.agent_id, agents)} / ${name} / ${scope}`
}

export function renderAssignmentHierarchy(
  items: Array<{ agent_id: string; inbound_id: number; inbound_tag?: string; client_email?: string; public_client_name?: string }>,
  agents: DashboardAgentView[],
  onRemove?: (key: string) => void,
) {
  if (!items.length) {
    return <Tag>未分配</Tag>
  }
  const grouped = new Map<string, Map<string, Array<{ key: string; label: string; nodeLabel: string }>>>()
  for (const item of items) {
    const agentID = item.agent_id || ''
    const realm = isRealmAssignmentDraft(item, agents)
    const nodeLabelText = realm ? realmAssignmentDisplayName(item, agents) : item.inbound_tag || `Inbound #${item.inbound_id}`
    const nodeKeyText = `${item.inbound_id}\x00${nodeLabelText}`
    const clientLabelText = realm ? '端口授权' : item.client_email || '整个节点'
    if (!grouped.has(agentID)) {
      grouped.set(agentID, new Map())
    }
    const nodeMap = grouped.get(agentID)!
    if (!nodeMap.has(nodeKeyText)) {
      nodeMap.set(nodeKeyText, [])
    }
    nodeMap.get(nodeKeyText)!.push({
      key: areaAssignmentKey(item),
      label: clientLabelText,
      nodeLabel: nodeLabelText,
    })
  }
  return (
    <Space direction="vertical" size={4} style={{ width: '100%' }}>
      {Array.from(grouped.entries()).map(([agentID, nodeMap]) => (
        <div key={agentID}>
          <Text strong>{agentName(agentID, agents)}</Text>
          <Space direction="vertical" size={3} style={{ width: '100%', marginTop: 4, paddingLeft: 10 }}>
            {Array.from(nodeMap.entries()).map(([nodeKeyText, clients]) => (
              <div key={nodeKeyText}>
                <Tag color="blue">{clients[0]?.nodeLabel || '-'}</Tag>
                <Space size={[4, 4]} wrap>
                  {clients.map((client) => (
                    <Tag
                      key={client.key}
                      closable={Boolean(onRemove)}
                      onClose={(event) => {
                        event.preventDefault()
                        onRemove?.(client.key)
                      }}
                    >
                      {client.label}
                    </Tag>
                  ))}
                </Space>
              </div>
            ))}
          </Space>
        </div>
      ))}
    </Space>
  )
}

export function isRealmAssignmentTagValue(value: string): boolean {
  const normalized = value.trim().toLowerCase()
  return normalized.startsWith('realm:') || normalized.startsWith('haproxy:')
}

export function uniqueStrings(values: string[]): string[] {
  return Array.from(new Set(values.map((value) => value.trim()).filter(Boolean)))
}

export function clientLabel(client: XUIClientView): string {
  const inbound = client.inbound_remark || client.inbound_tag || `Inbound #${client.inbound_id}`
  const email = client.email || '未指定客户端'
  return `客户端：${inbound} / ${email}`
}

export function clientTreeTitle(client: XUIClientView): string {
  return ['客户端', client.email || '未指定客户端', client.comment || client.sub_id || ''].filter(Boolean).join(' / ')
}

export function nodeKey(node: XUINodeView): string {
  return `node:${node.id}::`
}

export function nodeLabel(node: XUINodeView): string {
  return `节点：${node.remark || node.tag || `Inbound #${node.id}`} / ${node.protocol || '-'}`
}

export function defaultPublicClientName(client: XUIClientView, agentID: string, agents: DashboardAgentView[]): string {
  const agent = customerAgentName(agentID, agents)
  const node = client.inbound_remark || client.inbound_tag || `Inbound #${client.inbound_id}`
  return [agent, node, client.email || client.comment || client.sub_id].filter(Boolean).join(' - ')
}

export function defaultPublicNodeName(node: XUINodeView, agentID: string, agents: DashboardAgentView[]): string {
  const agent = customerAgentName(agentID, agents)
  return [agent, node.remark || node.tag || `Inbound #${node.id}`].filter(Boolean).join(' - ')
}

export function agentName(agentID: string, agents: DashboardAgentView[]): string {
  const agent = agents.find((item) => item.agent_id === agentID)
  return agent?.agent_name || agentID
}

export function customerAgentName(agentID: string, agents: DashboardAgentView[]): string {
  const agent = agents.find((item) => item.agent_id === agentID)
  return agent?.customer_display_name || agent?.agent_name || agentID
}

export function assignmentBilling(record: CustomerAssignment, agents: DashboardAgentView[]): XUIClientBillingConfig | undefined {
  return clientBilling(record.agent_id, record.inbound_id, record.inbound_tag || '', record.client_email || '', agents)
}

export function assignmentFormFromAssignment(record: CustomerAssignment, agents: DashboardAgentView[]): AssignmentFormState {
  const billing = assignmentBilling(record, agents)
  return {
    agent_id: record.agent_id,
    client_key: record.client_email ? `client:${record.inbound_id}::${record.client_email}` : `node:${record.inbound_id}::`,
    inbound_id: record.inbound_id,
    inbound_tag: record.inbound_tag || '',
    client_email: record.client_email || '',
    public_client_name: record.public_client_name || '',
    traffic_multiplier: Number(billing?.traffic_multiplier || 1),
    revenue_amount: Number(billing?.revenue_amount || 0),
    revenue_currency: billing?.revenue_currency === 'USDT' ? 'USDT' : 'CNY',
    revenue_cycle: billing?.revenue_cycle === 'quarter' || billing?.revenue_cycle === 'year' ? billing.revenue_cycle : 'month',
    enabled: record.enabled,
  }
}

export function assignmentFormFromDraft(draft: CustomerAssignmentDraft, agents: DashboardAgentView[]): AssignmentFormState {
  const inboundTag = draft.inbound_tag || ''
  const clientEmail = draft.client_email || ''
  const billing = clientBilling(draft.agent_id, draft.inbound_id, inboundTag, clientEmail, agents)
  const publicClientName = draft.public_client_name || defaultPublicNameFromDraft(draft, agents)
  return {
    agent_id: draft.agent_id,
    client_key: clientEmail ? `client:${draft.inbound_id}::${clientEmail}` : `node:${draft.inbound_id}::`,
    inbound_id: draft.inbound_id,
    inbound_tag: inboundTag,
    client_email: clientEmail,
    public_client_name: publicClientName,
    traffic_multiplier: Number(draft.traffic_multiplier ?? billing?.traffic_multiplier ?? 1),
    revenue_amount: Number(draft.revenue_amount ?? billing?.revenue_amount ?? 0),
    revenue_currency: draft.revenue_currency === 'USDT' ? 'USDT' : billing?.revenue_currency === 'USDT' ? 'USDT' : 'CNY',
    revenue_cycle: draft.revenue_cycle === 'quarter' || draft.revenue_cycle === 'year'
      ? draft.revenue_cycle
      : billing?.revenue_cycle === 'quarter' || billing?.revenue_cycle === 'year'
        ? billing.revenue_cycle
        : 'month',
    enabled: true,
  }
}

export function defaultPublicNameFromDraft(draft: CustomerAssignmentDraft, agents: DashboardAgentView[]): string {
  const agent = customerAgentName(draft.agent_id, agents)
  const tail = draft.client_email || draft.inbound_tag || `Inbound #${draft.inbound_id}`
  return [agent, tail].filter(Boolean).join(' - ')
}

export function findMatchingAssignment(customers: CustomerAdminView[], draft: CustomerAssignmentDraft): { customer: CustomerAdminView, assignment: CustomerAssignment } | null {
  const exactKey = billingKey(draft.inbound_id, draft.inbound_tag || '', draft.client_email || '')
  const emailKey = draft.client_email ? billingEmailKey(draft.inbound_id, draft.client_email) : ''
  for (const customer of customers) {
    for (const assignment of customer.assignments || []) {
      if (assignment.agent_id !== draft.agent_id) {
        continue
      }
      if (billingKey(assignment.inbound_id, assignment.inbound_tag || '', assignment.client_email || '') === exactKey) {
        return { customer, assignment }
      }
      if (emailKey && billingEmailKey(assignment.inbound_id, assignment.client_email || '') === emailKey) {
        return { customer, assignment }
      }
    }
  }
  return null
}

export function clientBilling(agentID: string, inboundID: number, inboundTag: string, email: string, agents: DashboardAgentView[]): XUIClientBillingConfig | undefined {
  const agent = agents.find((item) => item.agent_id === agentID)
  const exactKey = billingKey(inboundID, inboundTag, email)
  const emailKey = email ? billingEmailKey(inboundID, email) : ''
  return (agent?.renewal?.client_billings || []).find((billing) => {
    if (billingKey(Number(billing.inbound_id || 0), billing.inbound_tag || '', billing.email || '') === exactKey) {
      return true
    }
    return Boolean(emailKey && billingEmailKey(Number(billing.inbound_id || 0), billing.email || '') === emailKey)
  })
}

export function billingFormPatch(billing?: XUIClientBillingConfig): Pick<AssignmentFormState, 'traffic_multiplier' | 'revenue_amount' | 'revenue_currency' | 'revenue_cycle'> {
  return {
    traffic_multiplier: Number(billing?.traffic_multiplier || 1),
    revenue_amount: Number(billing?.revenue_amount || 0),
    revenue_currency: billing?.revenue_currency === 'USDT' ? 'USDT' : 'CNY',
    revenue_cycle: billing?.revenue_cycle === 'quarter' || billing?.revenue_cycle === 'year' ? billing.revenue_cycle : 'month',
  }
}

export function billingKey(inboundID: number, inboundTag: string, email: string): string {
  return `${Number(inboundID || 0)}\u0000${String(inboundTag || '').trim().toLowerCase()}\u0000${String(email || '').trim().toLowerCase()}`
}

export function billingEmailKey(inboundID: number, email: string): string {
  return `${Number(inboundID || 0)}\u0000${String(email || '').trim().toLowerCase()}`
}

export function revenueCycleLabel(cycle?: string): string {
  switch (cycle) {
    case 'quarter':
      return '季'
    case 'year':
      return '年'
    case 'month':
    default:
      return '月'
  }
}
