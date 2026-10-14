import type { XUIClientView } from '../types'

export const CLIENT_TRAFFIC_RESET_OPTIONS = [
  { value: 'never', label: '从不' },
  { value: 'hourly', label: '每小时' },
  { value: 'daily', label: '每日' },
  { value: 'weekly', label: '每周' },
  { value: 'monthly', label: '每月' },
] as const

export type ClientTrafficResetPeriod = typeof CLIENT_TRAFFIC_RESET_OPTIONS[number]['value']
export type ClientTrafficResetChoice = ClientTrafficResetPeriod | 'monthly_from_start'

export function clientTrafficResetStartDay(startTime?: number): number | null {
  if (!Number.isFinite(startTime) || !startTime || startTime <= 0) return null
  // Billing dates use Beijing time, independently of the browser's timezone.
  const date = new Date(startTime + 8 * 60 * 60 * 1000)
  return Number.isFinite(date.getTime()) ? date.getUTCDate() : null
}

export function clientTrafficResetOptions(startTime?: number): Array<{ value: ClientTrafficResetChoice; label: string; disabled?: boolean }> {
  const day = clientTrafficResetStartDay(startTime)
  return [
    ...CLIENT_TRAFFIC_RESET_OPTIONS,
    {
      value: 'monthly_from_start',
      label: day === null ? '按开始日每月重置（未设置开始日期）' : `每月 ${day} 号（按开始日）`,
      disabled: day === null,
    },
  ]
}

export function resolveClientTrafficResetChoice(choice: ClientTrafficResetChoice, day: number | null, startTime?: number): { period: ClientTrafficResetPeriod; day: number } | null {
  if (choice === 'monthly_from_start') {
    const startDay = clientTrafficResetStartDay(startTime)
    return startDay === null ? null : { period: 'monthly', day: startDay }
  }
  if (!CLIENT_TRAFFIC_RESET_OPTIONS.some((option) => option.value === choice)) return null
  const validDay = day !== null && Number.isInteger(day) && day >= 1 && day <= 31
  if (choice === 'monthly' && !validDay) return null
  return { period: choice, day: validDay ? day! : 1 }
}

export function clientTrafficResetDay(value: unknown): number {
  const day = Number(value)
  return Number.isInteger(day) && day >= 1 && day <= 31 ? day : 1
}

export function clientTrafficResetSummary(record: XUIClientView): string {
  if (!record.traffic_reset_supported) return '未上报'
  switch (record.traffic_reset) {
    case 'monthly': return `每月 ${clientTrafficResetDay(record.traffic_reset_day)} 号`
    case 'weekly': return '每周日'
    default: return CLIENT_TRAFFIC_RESET_OPTIONS.find((option) => option.value === record.traffic_reset)?.label || '未上报'
  }
}

export function buildClientTrafficResetAction(record: XUIClientView, period: ClientTrafficResetPeriod, day: number, agentID: string) {
  if (!CLIENT_TRAFFIC_RESET_OPTIONS.some((option) => option.value === period)) throw new Error('请选择有效的流量重置周期')
  if (!Number.isInteger(day) || day < 1 || day > 31) throw new Error('每月重置日必须为 1–31 的整数')
  const targetAgentID = record.realm_target_agent_id || agentID
  const inboundID = record.realm_target_inbound_id || record.inbound_id
  const inboundTag = record.realm_target_inbound_tag || record.inbound_tag || ''
  if (!targetAgentID || !record.email || (!inboundID && !inboundTag)) throw new Error('客户端或入站信息不完整，请刷新采集')
  return {
    agentID: targetAgentID,
    request: {
      kind: 'update_client_traffic_reset',
      payload: {
        inbound_id: inboundID,
        inbound_tag: inboundTag,
        email: record.email,
        client_id: record.client_id || record.auth_uuid || record.auth_password || '',
        traffic_reset: period,
        traffic_reset_day: day,
      },
    },
  }
}
