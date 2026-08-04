import type { DashboardAgentView, FinanceClientView, XUIClientBillingConfig } from '../types'

export const CLIENT_EXPIRY_WARNING_DAYS = 5

export type ClientExpiryCycle = 'month' | 'quarter' | 'semiannual' | 'year' | ''

export type ClientExpiryRow = {
  key: string
  agentID: string
  agentName: string
  inboundName: string
  clientName: string
  cycle: ClientExpiryCycle
  expiryTime: number
  expiryDate: string
  remainingDays: number
  level: 'warn' | 'bad'
}

export function buildClientExpiryRows(
  agents: DashboardAgentView[],
  now = Date.now(),
  warningDays = CLIENT_EXPIRY_WARNING_DAYS,
): ClientExpiryRow[] {
  const rows: ClientExpiryRow[] = []
  const threshold = Math.max(0, Math.floor(warningDays))

  for (const agent of agents) {
    const billings = agent.renewal?.client_billings || []
    if (agent.finance_clients_ready) {
      for (const client of agent.finance_clients || []) {
        if (client.enabled === false || client.node_enabled === false) {
          continue
        }
        const billing = findClientBilling(billings, client)
        appendExpiryRow(rows, agent, client, billing, now, threshold)
      }
      continue
    }

    for (const billing of billings) {
      appendExpiryRow(rows, agent, undefined, billing, now, threshold)
    }
  }

  return [...new Map(rows.map((row) => [row.key, row])).values()].sort((left, right) => (
    left.remainingDays - right.remainingDays
    || left.expiryTime - right.expiryTime
    || left.agentName.localeCompare(right.agentName)
    || left.clientName.localeCompare(right.clientName)
  ))
}

export function clientExpiryCycleLabel(cycle: ClientExpiryCycle): string {
  switch (cycle) {
    case 'month':
      return '月付'
    case 'quarter':
      return '季付'
    case 'semiannual':
      return '半年付'
    case 'year':
      return '年付'
    default:
      return '未设周期'
  }
}

export function clientExpiryRemainingLabel(remainingDays: number): string {
  if (remainingDays < 0) {
    return `已过期 ${Math.abs(remainingDays)} 天`
  }
  if (remainingDays === 0) {
    return '今天到期'
  }
  return `剩余 ${remainingDays} 天`
}

function appendExpiryRow(
  rows: ClientExpiryRow[],
  agent: DashboardAgentView,
  client: FinanceClientView | undefined,
  billing: XUIClientBillingConfig | undefined,
  now: number,
  warningDays: number,
) {
  const expiryTime = Math.max(0, Number(client?.expiry_time || billing?.expire_time || 0))
  if (!expiryTime || !Number.isFinite(expiryTime)) {
    return
  }
  const remainingDays = localCalendarDaysBetween(now, expiryTime)
  if (remainingDays > warningDays) {
    return
  }

  const inboundID = Number(client?.inbound_id || billing?.inbound_id || 0)
  const inboundTag = client?.inbound_tag || billing?.inbound_tag || ''
  const email = client?.email || billing?.email || ''
  rows.push({
    key: `${agent.agent_id}\u0000${inboundID}\u0000${inboundTag}\u0000${email}`,
    agentID: agent.agent_id,
    agentName: agent.agent_name || agent.summary?.hostname || agent.agent_id,
    inboundName: client?.inbound_remark || inboundTag || (inboundID > 0 ? `Inbound ${inboundID}` : '未命名节点'),
    clientName: client?.comment || email || '未命名客户端',
    cycle: normalizeCycle(billing?.revenue_cycle || billing?.expire_cycle),
    expiryTime,
    expiryDate: formatLocalDate(expiryTime),
    remainingDays,
    level: remainingDays <= 1 ? 'bad' : 'warn',
  })
}

function findClientBilling(billings: XUIClientBillingConfig[], client: FinanceClientView): XUIClientBillingConfig | undefined {
  const exactKey = clientIdentityKey(client.inbound_id, client.inbound_tag, client.email)
  const exact = billings.find((billing) => clientIdentityKey(billing.inbound_id, billing.inbound_tag, billing.email) === exactKey)
  if (exact) {
    return exact
  }

  const email = normalizeIdentity(client.email)
  return billings.find((billing) => {
    if (!email || normalizeIdentity(billing.email) !== email) {
      return false
    }
    const billingInboundID = Number(billing.inbound_id || 0)
    if (client.inbound_id > 0 && billingInboundID > 0) {
      return client.inbound_id === billingInboundID
    }
    return normalizeIdentity(client.inbound_tag) === normalizeIdentity(billing.inbound_tag)
  })
}

function clientIdentityKey(inboundID?: number, inboundTag?: string, email?: string): string {
  return `${Number(inboundID || 0)}\u0000${normalizeIdentity(inboundTag)}\u0000${normalizeIdentity(email)}`
}

function normalizeIdentity(value?: string): string {
  return (value || '').trim().toLowerCase()
}

function normalizeCycle(cycle?: string): ClientExpiryCycle {
  if (cycle === 'month' || cycle === 'quarter' || cycle === 'semiannual' || cycle === 'year') {
    return cycle
  }
  return ''
}

function localCalendarDaysBetween(startTime: number, endTime: number): number {
  const start = new Date(startTime)
  const end = new Date(endTime)
  const startDay = Date.UTC(start.getFullYear(), start.getMonth(), start.getDate())
  const endDay = Date.UTC(end.getFullYear(), end.getMonth(), end.getDate())
  return Math.ceil((endDay - startDay) / (24 * 60 * 60 * 1000))
}

function formatLocalDate(value: number): string {
  const date = new Date(value)
  const year = date.getFullYear()
  const month = `${date.getMonth() + 1}`.padStart(2, '0')
  const day = `${date.getDate()}`.padStart(2, '0')
  return `${year}-${month}-${day}`
}
