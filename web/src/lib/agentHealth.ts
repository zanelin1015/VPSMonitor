import type { AgentListItem } from '../types'
import { agentDisplayStatus } from './appHelpersAgent'
import { calculateMemoryPercent } from './traffic'

export type AgentHealthCategory = 'healthy' | 'warning' | 'offline'
export type AgentHealthFilter = 'all' | AgentHealthCategory

export function agentHealthCategory(agent: AgentListItem): AgentHealthCategory {
  const status = agentDisplayStatus(agent)
  if (status.level === 'bad') {
    return 'offline'
  }
  if (
    status.level === 'warn' ||
    calculateMemoryPercent(agent.summary) >= 75 ||
    Number(agent.summary.cpu || 0) >= 75 ||
    Boolean(agent.summary.last_collection_err)
  ) {
    return 'warning'
  }
  return 'healthy'
}

export function agentMatchesHealthFilter(agent: AgentListItem, filter: AgentHealthFilter): boolean {
  return filter === 'all' || agentHealthCategory(agent) === filter
}

export function normalizeAgentHealthFilter(value?: string | null): AgentHealthFilter {
  const normalized = (value || '').trim().toLowerCase()
  switch (normalized) {
    case 'healthy':
    case 'warning':
    case 'offline':
      return normalized
    default:
      return 'all'
  }
}

export function agentHealthFilterLabel(filter: AgentHealthFilter): string {
  switch (filter) {
    case 'healthy':
      return '健康'
    case 'warning':
      return '负载告警'
    case 'offline':
      return '离线'
    default:
      return '全部状态'
  }
}
