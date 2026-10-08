import type { AgentListItem, FinanceClientView } from '../types'

type SearchableClient = Pick<FinanceClientView, 'client_id' | 'inbound_id' | 'inbound_tag' | 'inbound_remark' | 'email' | 'comment'>

function fieldsMatchQuery(fields: (string | number | undefined)[], query: string): boolean {
  return fields.some((value) => value !== undefined && String(value).toLowerCase().includes(query))
}

export function clientMatchesSearch(client: SearchableClient, query: string): boolean {
  const normalizedQuery = query.trim().toLowerCase()
  return !normalizedQuery || fieldsMatchQuery([
    client.client_id,
    client.inbound_id,
    client.inbound_tag,
    client.inbound_remark,
    client.email,
    client.comment,
  ], normalizedQuery)
}

export function matchingAgentClients(agent: AgentListItem, query: string): FinanceClientView[] {
  if (!query.trim()) {
    return []
  }
  return (agent.finance_clients || []).filter((client) => clientMatchesSearch(client, query))
}

export function agentMatchesSearch(agent: AgentListItem, query: string): boolean {
  const normalizedQuery = query.trim().toLowerCase()
  if (!normalizedQuery) {
    return true
  }
  return fieldsMatchQuery([
    agent.agent_id,
    agent.agent_name,
    agent.customer_display_name,
    agent.summary.hostname,
    agent.summary.observed_ip,
    agent.summary.public_ipv4,
    agent.summary.public_ipv6,
    ...(agent.tags || []),
  ], normalizedQuery) || matchingAgentClients(agent, normalizedQuery).length > 0
}
