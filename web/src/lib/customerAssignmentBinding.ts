import type { CustomerAssignment, DashboardAgentView, FinanceClientView, XUIOverview } from '../types'
import { agentHealthCategory } from './agentHealth'

export type AssignmentBindingOverview =
  | { status: 'loading' }
  | { status: 'error' }
  | { status: 'ready'; overview: XUIOverview }

export type AssignmentBindingOverviews = Record<string, AssignmentBindingOverview>

export interface AssignmentBindingDiagnostic {
  status: 'matched' | 'legacy' | 'mismatch' | 'waiting_sync' | 'node_deleted' | 'select_client' | 'unresolved' | 'unavailable'
  label: string
  current?: FinanceClientView
}

export function isForwardingAssignment(record: CustomerAssignment, agents: DashboardAgentView[]): boolean {
  if (/^(realm|haproxy):/i.test(record.inbound_tag || '')) return true
  const entry = agents.find(agent => agent.agent_id === record.agent_id)?.entry
  // Also recognize older assignments whose tag is the user-defined rule name.
  return Boolean(
    entry?.port_forwarding?.rules?.some(rule => rule.listen_port === record.inbound_id) ||
    entry?.haproxy?.rules?.some(rule => rule.listen_port === record.inbound_id),
  )
}

export function assignmentBindingDiagnostic(
  record: CustomerAssignment,
  agents: DashboardAgentView[],
  overviews: AssignmentBindingOverviews = {},
): AssignmentBindingDiagnostic {
  const agent = agents.find(item => item.agent_id === record.agent_id)
  const forwarded = isForwardingAssignment(record, agents)
  const state = overviews[record.agent_id]
  if (forwarded && (!state || state.status === 'loading')) {
    return { status: 'waiting_sync', label: '等待同步' }
  }
  if (forwarded && state?.status === 'error') {
    return { status: 'unavailable', label: '无法确认绑定' }
  }
  // The overview endpoint already resolves multi-hop Realm/HAProxy clients
  // into the source listener's namespace. Never merge these into finance data.
  const overview = forwarded && state?.status === 'ready' ? state.overview : undefined
  if (overview && overview.agent_id !== record.agent_id) {
    return { status: 'unavailable', label: '无法确认绑定' }
  }
  const clients = (overview?.clients || (forwarded ? [] : agent?.finance_clients) || [])
    .filter(client => client.inbound_id === record.inbound_id)
  const current = record.client_id ? clients.find(client => client.client_id === record.client_id) : undefined
  if (current) return { status: 'matched', label: '已匹配', current }
  if (!agent || agentHealthCategory(agent) === 'offline' || agent.summary.last_collection_err) {
    return { status: 'waiting_sync', label: '等待同步' }
  }
  if (!forwarded && !agent.finance_clients_ready) {
    return { status: 'waiting_sync', label: '等待同步' }
  }
  if (overview && !overview.nodes.some(node => node.id === record.inbound_id) && clients.length === 0) {
    // Empty forwarded results can mean a missing target snapshot or a broken
    // intermediate rule; they do not prove that the final client was deleted.
    return { status: 'unresolved', label: '转发链路未解析' }
  }
  const legacy = clients.filter(client =>
    !record.client_email || (client.email || '').trim().toLowerCase() === record.client_email.trim().toLowerCase(),
  )
  if (legacy.length > 1) return { status: 'select_client', label: '请选择客户端' }
  if (legacy.length === 1) {
    return { status: record.client_id ? 'mismatch' : 'legacy', label: record.client_id ? 'ID 不匹配' : '旧数据匹配', current: legacy[0] }
  }
  const suspicious = clients.filter(client =>
    [client.comment, client.email].some(value => value && [record.client_email, record.public_client_name].includes(value)),
  )
  if (suspicious.length === 1) {
    return { status: 'mismatch', label: '疑似备注误绑', current: suspicious[0] }
  }
  if (clients.length || overview?.nodes.some(node => node.id === record.inbound_id)) {
    return { status: 'select_client', label: '请选择客户端' }
  }
  return { status: 'node_deleted', label: '节点已删除' }
}
