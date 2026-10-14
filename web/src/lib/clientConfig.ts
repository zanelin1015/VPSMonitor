import type { XUIClientBillingConfig, XUIClientConfigChanges, XUIClientConfigRequest, XUIClientConfigSave, XUIClientView } from '../types'
import { clientBillingPatchFromStart, defaultClientBilling, effectiveClientBillingStartTime, normalizeClientBillings } from './appHelpersBilling'
import { CLIENT_TRAFFIC_RESET_OPTIONS, clientTrafficResetDay, resolveClientTrafficResetChoice } from './clientTrafficReset'
import type { ClientTrafficResetChoice } from './clientTrafficReset'

export interface ClientConfigDraft {
  enabled: boolean
  totalGB: number | null
  billing: XUIClientBillingConfig
  resetChoice: ClientTrafficResetChoice
  resetDay: number | null
  manualExpiry: boolean
  unlimitedExpiry: boolean
}

export function accountBasedClient(record: XUIClientView) {
  return ['http', 'socks', 'socks5'].includes((record.protocol || '').toLowerCase())
}

export function createClientConfigDraft(record: XUIClientView, billing?: XUIClientBillingConfig): ClientConfigDraft {
  const normalized = normalizeClientBillings([{ ...defaultClientBilling(record), ...billing }])[0]
  const start = effectiveClientBillingStartTime(normalized, record.expiry_time || 0)
  const calculated = clientBillingPatchFromStart(start, normalized.revenue_cycle).expire_time
  return {
    enabled: record.config_enabled ?? record.enabled,
    totalGB: Math.max(0, Number(record.total_gb || 0)) / (1024 ** 3),
    billing: { ...normalized, start_time: start },
    resetChoice: CLIENT_TRAFFIC_RESET_OPTIONS.find(option => option.value === record.traffic_reset)?.value || 'never',
    resetDay: clientTrafficResetDay(record.traffic_reset_day),
    manualExpiry: Boolean(normalized.start_time && normalized.expire_time && normalized.expire_time !== calculated),
    unlimitedExpiry: !normalized.expire_time,
  }
}

export function editClientBilling(draft: ClientConfigDraft, patch: Partial<XUIClientBillingConfig>): ClientConfigDraft {
  const billing = { ...draft.billing, ...patch }
  if ((patch.start_time !== undefined || patch.revenue_cycle !== undefined) && !draft.manualExpiry && !draft.unlimitedExpiry) {
    billing.expire_time = clientBillingPatchFromStart(billing.start_time || 0, billing.revenue_cycle).expire_time
  }
  return { ...draft, billing }
}

export function buildClientConfigRequest(record: XUIClientView, previous: XUIClientBillingConfig | undefined, initial: ClientConfigDraft, draft: ClientConfigDraft, requestID: string): XUIClientConfigRequest {
  if (draft.totalGB === null || !Number.isFinite(draft.totalGB) || draft.totalGB < 0 || !Number.isSafeInteger(Math.round(draft.totalGB * 1024 ** 3))) throw new Error('请输入有效的总流量上限')
  const billing = { ...draft.billing, expire_auto_renew: false, expire_cycle: draft.billing.revenue_cycle }
  // An inferred start date is display-only until explicitly edited.
  if (billing.start_time === initial.billing.start_time) billing.start_time = previous?.start_time || 0
  if (draft.unlimitedExpiry) billing.expire_time = 0
  if (!draft.unlimitedExpiry && (!billing.expire_time || !Number.isSafeInteger(billing.expire_time))) throw new Error('请选择到期时间或开启不限期')
  if (billing.start_time && billing.expire_time && billing.expire_time <= billing.start_time) throw new Error('到期时间必须晚于开始日期')
  if (!Number.isFinite(billing.revenue_amount) || Number(billing.revenue_amount) < 0) throw new Error('请输入有效的收费金额')
  if (!Number.isFinite(billing.traffic_multiplier) || Number(billing.traffic_multiplier) < 0.1 || Number(billing.traffic_multiplier) > 100) throw new Error('流量倍数应为 0.1–100')
  const changes: XUIClientConfigChanges = {}
  const expected: XUIClientConfigChanges = {}
  if (!accountBasedClient(record)) {
    if (draft.enabled !== initial.enabled) { changes.enabled = draft.enabled; expected.enabled = initial.enabled }
    if (draft.totalGB !== initial.totalGB) { changes.total_bytes = Math.round(draft.totalGB * 1024 ** 3); expected.total_bytes = Math.max(0, Number(record.total_gb || 0)) }
    if (billing.expire_time !== initial.billing.expire_time) { changes.expiry_time = billing.expire_time; expected.expiry_time = Math.max(0, Number(record.expiry_time || 0)) }
    const reset = resolveClientTrafficResetChoice(draft.resetChoice, draft.resetDay, draft.billing.start_time)
    if (!reset) throw new Error('每月重置日必须为 1–31 的整数，按开始日重置需先设置开始日期')
    if (record.traffic_reset_supported && (reset.period !== (record.traffic_reset || 'never') || reset.day !== clientTrafficResetDay(record.traffic_reset_day))) {
      changes.traffic_reset = reset.period; changes.traffic_reset_day = reset.day
      expected.traffic_reset = record.traffic_reset || 'never'; expected.traffic_reset_day = clientTrafficResetDay(record.traffic_reset_day)
    }
  }
  return { request_id: requestID, client_id: record.client_id || '', inbound_id: record.inbound_id, inbound_tag: record.inbound_tag || '', email: record.email || '', billing, previous_billing: previous || null, changes, expected }
}

const remoteFieldNames: Record<keyof XUIClientConfigChanges, string> = { enabled: 'enable', total_bytes: 'totalGB', expiry_time: 'expiryTime', traffic_reset: 'trafficReset', traffic_reset_day: 'trafficResetDay' }
export const CLIENT_CONFIG_FIELD_LABELS: Record<string, string> = { enable: '启用状态', totalGB: '总流量上限', expiryTime: '到期时间', trafficReset: '重置周期', trafficResetDay: '每月重置日' }

export function retryClientConfigRequest(request: XUIClientConfigRequest, save: XUIClientConfigSave, requestID: string): XUIClientConfigRequest {
  const results = save.action?.result?.field_results as Record<string, { verified?: boolean }> | undefined
  const changes: XUIClientConfigChanges = { ...request.changes }
  for (const key of Object.keys(changes) as (keyof XUIClientConfigChanges)[]) {
    if (results?.[remoteFieldNames[key]]?.verified === true) delete changes[key]
  }
  // x-ui requires the reset period/day pair even when just one failed.
  if ('traffic_reset' in changes || 'traffic_reset_day' in changes) {
    changes.traffic_reset = request.changes.traffic_reset
    changes.traffic_reset_day = request.changes.traffic_reset_day
  }
  return { ...request, request_id: requestID, billing: save.billing, previous_billing: save.billing, changes }
}

export function clientConfigSaveStatus(save?: XUIClientConfigSave): { text: string; color?: string } | null {
  if (!save) return null
  switch (save.action?.status) {
    case 'pending': return { text: '待下发', color: 'orange' }
    case 'running': return { text: '执行中', color: 'processing' }
    case 'failed': return { text: '部分失败', color: 'error' }
    case 'succeeded': return { text: '已生效', color: 'success' }
    default: return { text: '已保存', color: 'success' }
  }
}

export function beijingDateInput(millis?: number, withTime = false): string {
  return millis && Number.isFinite(millis) ? new Date(millis + 8 * 3600000).toISOString().slice(0, withTime ? 19 : 10) : ''
}
