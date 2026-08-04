import type { VPSRenewalConfig, XUIClientBillingConfig, XUIClientView } from '../types'
import { DEFAULT_COST_CURRENCY, normalizeCurrencyCode } from './currency'
import { clampMetricPercent } from './traffic'

export function normalizeRenewalConfig(config?: VPSRenewalConfig): VPSRenewalConfig {
  const cycle = config?.cycle === 'week' || config?.cycle === 'quarter' || config?.cycle === 'semiannual' || config?.cycle === 'year' ? config.cycle : 'month'
  const trafficLimitBytes = Math.max(0, Number(config?.traffic_limit_bytes || 0))
  const costAmount = Math.max(0, Number(config?.cost_amount || 0))
  const costCurrency = normalizeCurrencyCode(config?.cost_currency || DEFAULT_COST_CURRENCY)
  const costCycle = config?.cost_cycle === 'quarter' || config?.cost_cycle === 'semiannual' || config?.cost_cycle === 'year' ? config.cost_cycle : 'month'
  return {
    enabled: Boolean(config?.enabled || config?.start_date || config?.expire_date),
    start_date: config?.start_date || '',
    expire_date: config?.expire_date || '',
    cycle,
    auto_renew: Boolean(config?.auto_renew),
    cost_amount: costAmount,
    cost_currency: costCurrency,
    cost_cycle: costCycle,
    client_billings: normalizeClientBillings(config?.client_billings || []),
    traffic_limit_bytes: trafficLimitBytes,
    traffic_accounting_mode: config?.traffic_accounting_mode === 'single_direction' ? 'single_direction' : 'bidirectional',
    bandwidth_mbps: Math.max(0, Number(config?.bandwidth_mbps || 0)),
    traffic_baseline_bytes: trafficLimitBytes > 0 ? Math.max(0, Number(config?.traffic_baseline_bytes || 0)) : 0,
    traffic_sent_baseline_bytes: trafficLimitBytes > 0 ? Math.max(0, Number(config?.traffic_sent_baseline_bytes || 0)) : 0,
    traffic_recv_baseline_bytes: trafficLimitBytes > 0 ? Math.max(0, Number(config?.traffic_recv_baseline_bytes || 0)) : 0,
    traffic_baseline_period_start: trafficLimitBytes > 0 ? config?.traffic_baseline_period_start || '' : '',
  }
}

export function normalizeClientBillings(items: XUIClientBillingConfig[]): XUIClientBillingConfig[] {
  const seen = new Set<string>()
  const normalized: XUIClientBillingConfig[] = []
  for (const item of items) {
    const billingCycle = normalizeClientExpireCycle(item.revenue_cycle || item.expire_cycle)
    const startTime = Math.max(0, Number(item.start_time || 0))
    const billing: XUIClientBillingConfig = {
      inbound_id: Number(item.inbound_id || 0),
      inbound_tag: item.inbound_tag || '',
      email: item.email || '',
      traffic_multiplier: normalizeClientTrafficMultiplier(item.traffic_multiplier),
      revenue_amount: Math.max(0, Number(item.revenue_amount || 0)),
      revenue_currency: item.revenue_currency === 'USDT' ? 'USDT' : 'CNY',
      revenue_cycle: billingCycle,
      start_time: startTime,
      expire_time: startTime > 0
        ? calculateClientBillingExpiryTime(startTime, billingCycle)
        : Math.max(0, Number(item.expire_time || 0)),
      expire_cycle: billingCycle,
      expire_auto_renew: startTime > 0,
    }
    const key = clientBillingKey(billing)
    if (seen.has(key)) {
      continue
    }
    seen.add(key)
    normalized.push(billing)
  }
  return normalized
}

export function clientBillingKey(value: Pick<XUIClientBillingConfig, 'inbound_id' | 'inbound_tag' | 'email'>): string {
  return `${Number(value.inbound_id || 0)}\u0000${value.inbound_tag || ''}\u0000${value.email || ''}`
}

export function billingKeyForClient(client: XUIClientView): string {
  return clientBillingKey({ inbound_id: client.inbound_id, inbound_tag: client.inbound_tag, email: client.email })
}

export function defaultClientBilling(client: XUIClientView): XUIClientBillingConfig {
  return {
    inbound_id: client.inbound_id,
    inbound_tag: client.inbound_tag || '',
    email: client.email || '',
    traffic_multiplier: 1,
    revenue_amount: 0,
    revenue_currency: 'CNY',
    revenue_cycle: 'month',
    start_time: 0,
    expire_time: 0,
    expire_cycle: 'month',
    expire_auto_renew: false,
  }
}

export function normalizeClientTrafficMultiplier(value?: number): number {
  const multiplier = Number(value || 0)
  if (!Number.isFinite(multiplier) || multiplier <= 0) {
    return 1
  }
  return Math.min(100, Math.max(0.1, multiplier))
}

export function scaleClientTraffic(value: number, multiplier?: number): number {
  return Math.max(0, Number(value || 0)) * normalizeClientTrafficMultiplier(multiplier)
}

export function findClientBilling(items: XUIClientBillingConfig[] | undefined, client: XUIClientView): XUIClientBillingConfig | undefined {
  const key = billingKeyForClient(client)
  return (items || []).find((item) => clientBillingKey(item) === key)
}

export function upsertClientBilling(items: XUIClientBillingConfig[], client: XUIClientView, billing: XUIClientBillingConfig): XUIClientBillingConfig[] {
  const next = normalizeClientBillings(items)
  const normalized = normalizeClientBillings([{ ...defaultClientBilling(client), ...billing }])[0]
  const key = billingKeyForClient(client)
  const index = next.findIndex((item) => clientBillingKey(item) === key)
  if (index >= 0) {
    next[index] = normalized
    return next
  }
  return [...next, normalized]
}

export function dateInputToStartMillis(value: string): number {
  if (!value) {
    return 0
  }
  const date = new Date(`${value}T00:00:00`)
  if (!Number.isFinite(date.getTime())) {
    return 0
  }
  return date.getTime()
}

export function clientBillingPatchFromStart(startTime: number, cycle?: string): Pick<XUIClientBillingConfig, 'start_time' | 'expire_time' | 'expire_cycle' | 'expire_auto_renew'> {
  const normalizedCycle = normalizeClientExpireCycle(cycle)
  const normalizedStart = Math.max(0, Number(startTime || 0))
  return {
    start_time: normalizedStart,
    expire_time: calculateClientBillingExpiryTime(normalizedStart, normalizedCycle),
    expire_cycle: normalizedCycle,
    expire_auto_renew: normalizedStart > 0,
  }
}

export function effectiveClientBillingStartTime(billing: XUIClientBillingConfig, fallbackExpiry = 0): number {
  const startTime = Math.max(0, Number(billing.start_time || 0))
  if (startTime > 0) {
    return startTime
  }
  return deriveClientBillingStartTime(Math.max(0, Number(billing.expire_time || fallbackExpiry || 0)), clientBillingCycle(billing))
}

export function effectiveClientBillingExpiryTime(billing: XUIClientBillingConfig, fallbackExpiry = 0): number {
  const startTime = Math.max(0, Number(billing.start_time || 0))
  if (startTime > 0) {
    return calculateClientBillingExpiryTime(startTime, clientBillingCycle(billing))
  }
  return Math.max(0, Number(billing.expire_time || fallbackExpiry || 0))
}

export function calculateRenewalStatus(config?: VPSRenewalConfig): {
  remainingLabel: string
  endLabel: string
  level: 'ok' | 'warn' | 'bad'
  autoRenew: boolean
  percent: number
} | null {
  const normalized = normalizeRenewalConfig(config)
  if (!normalized.enabled || (!normalized.start_date && !normalized.expire_date)) {
    return null
  }
  const period = calculateRenewalPeriod(normalized)
  if (!period) {
    return null
  }
  const now = startOfLocalDay(new Date())
  if (now < period.start) {
    const daysUntilStart = daysBetween(now, period.start)
    return {
      remainingLabel: `${daysUntilStart} 天后开始`,
      endLabel: `到期 ${formatLocalDate(period.end)}`,
      level: 'ok',
      autoRenew: Boolean(normalized.auto_renew),
      percent: 0,
    }
  }
  const remainingDays = daysBetween(now, period.end)
  if (remainingDays < 0) {
    const overdueDays = Math.abs(remainingDays)
    return {
      remainingLabel: `已过期 ${overdueDays} 天`,
      endLabel: `到期 ${formatLocalDate(period.end)}`,
      level: 'bad',
      autoRenew: Boolean(normalized.auto_renew),
      percent: 100,
    }
  }
  const totalDays = Math.max(1, daysBetween(period.start, period.end))
  const remainingPercent = (Math.max(0, remainingDays) / totalDays) * 100
  return {
    remainingLabel: remainingDays === 0 ? '今天到期' : `剩余 ${remainingDays} 天`,
    endLabel: `到期 ${formatLocalDate(period.end)}`,
    level: remainingDays <= 3 ? 'bad' : remainingDays <= 7 ? 'warn' : 'ok',
    autoRenew: Boolean(normalized.auto_renew),
    percent: clampMetricPercent(remainingPercent),
  }
}

export function formatRenewalHint(config?: VPSRenewalConfig): string {
  const status = calculateRenewalStatus(config)
  const trafficMode = config?.traffic_accounting_mode === 'single_direction' ? '单向取上传/下载较大值' : '双向按上传+下载'
  if (!status) {
    return `设置后会在 Client 卡片上展示到期、周期刷新、流量配额和带宽信息；当前流量计算：${trafficMode}。`
  }
  return `当前周期${status.remainingLabel}，${status.endLabel}，流量计算：${trafficMode}；${status.autoRenew ? '到期后自动刷新下一周期，并重置流量基线' : '到期后不自动刷新'}。`
}

export function calculateRenewalPeriod(config: VPSRenewalConfig): { start: Date; end: Date } | null {
  const now = startOfLocalDay(new Date())
  if (config.expire_date) {
    let end = parseLocalDate(config.expire_date)
    if (!end) {
      return null
    }
    end = startOfLocalDay(end)
    if (config.auto_renew && config.cycle) {
      while (end <= now) {
        end = addRenewalCycle(end, config.cycle)
      }
    }
    return {
      start: config.cycle ? subtractRenewalCycle(end, config.cycle) : end,
      end,
    }
  }
  const start = parseLocalDate(config.start_date || '')
  if (!start || !config.cycle) {
    return null
  }
  let periodStart = startOfLocalDay(start)
  let periodEnd = addRenewalCycle(periodStart, config.cycle)
  if (config.auto_renew !== false) {
    while (periodEnd <= now) {
      periodStart = periodEnd
      periodEnd = addRenewalCycle(periodStart, config.cycle)
    }
  }
  return { start: periodStart, end: periodEnd }
}

export function parseLocalDate(value: string): Date | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value)
  if (!match) {
    return null
  }
  const year = Number(match[1])
  const month = Number(match[2]) - 1
  const day = Number(match[3])
  const date = new Date(year, month, day)
  if (date.getFullYear() !== year || date.getMonth() !== month || date.getDate() !== day) {
    return null
  }
  return date
}

export function startOfLocalDay(date: Date): Date {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate())
}

export function addRenewalCycle(date: Date, cycle: VPSRenewalConfig['cycle']): Date {
  if (cycle === 'week') {
    return new Date(date.getFullYear(), date.getMonth(), date.getDate() + 7)
  }
  if (cycle === 'quarter') {
    return addClampedMonths(date, 3)
  }
  if (cycle === 'semiannual') {
    return addClampedMonths(date, 6)
  }
  if (cycle === 'year') {
    return addClampedMonths(date, 12)
  }
  return addClampedMonths(date, 1)
}

export function subtractRenewalCycle(date: Date, cycle: VPSRenewalConfig['cycle']): Date {
  if (cycle === 'week') {
    return new Date(date.getFullYear(), date.getMonth(), date.getDate() - 7)
  }
  if (cycle === 'quarter') {
    return addClampedMonths(date, -3)
  }
  if (cycle === 'semiannual') {
    return addClampedMonths(date, -6)
  }
  if (cycle === 'year') {
    return addClampedMonths(date, -12)
  }
  return addClampedMonths(date, -1)
}

export function addClampedMonths(date: Date, months: number): Date {
  const targetMonth = date.getMonth() + months
  const firstOfTarget = new Date(date.getFullYear(), targetMonth, 1)
  const lastDay = new Date(firstOfTarget.getFullYear(), firstOfTarget.getMonth() + 1, 0).getDate()
  return new Date(firstOfTarget.getFullYear(), firstOfTarget.getMonth(), Math.min(date.getDate(), lastDay))
}

export function daysBetween(start: Date, end: Date): number {
  const msPerDay = 24 * 60 * 60 * 1000
  return Math.ceil((startOfLocalDay(end).getTime() - startOfLocalDay(start).getTime()) / msPerDay)
}

export function formatLocalDate(date: Date): string {
  const year = date.getFullYear()
  const month = `${date.getMonth() + 1}`.padStart(2, '0')
  const day = `${date.getDate()}`.padStart(2, '0')
  return `${year}/${month}/${day}`
}

function normalizeClientExpireCycle(cycle?: string): 'month' | 'quarter' | 'semiannual' | 'year' {
  return cycle === 'quarter' || cycle === 'semiannual' || cycle === 'year' ? cycle : 'month'
}

function clientBillingCycle(billing: Pick<XUIClientBillingConfig, 'revenue_cycle' | 'expire_cycle'>): 'month' | 'quarter' | 'semiannual' | 'year' {
  return normalizeClientExpireCycle(billing.revenue_cycle || billing.expire_cycle)
}

function calculateClientBillingExpiryTime(startTime: number, cycle?: string, now = Date.now()): number {
  if (!startTime) {
    return 0
  }
  let periodStart = startOfLocalDay(new Date(startTime))
  if (!Number.isFinite(periodStart.getTime())) {
    return 0
  }
  let nextStart = addRenewalCycle(periodStart, normalizeClientExpireCycle(cycle))
  let periodEnd = new Date(nextStart.getTime() - 1000)
  while (periodEnd.getTime() <= now) {
    periodStart = nextStart
    nextStart = addRenewalCycle(periodStart, normalizeClientExpireCycle(cycle))
    periodEnd = new Date(nextStart.getTime() - 1000)
  }
  return periodEnd.getTime()
}

function deriveClientBillingStartTime(expireTime: number, cycle?: string): number {
  if (!expireTime) {
    return 0
  }
  const expireDate = new Date(expireTime)
  if (!Number.isFinite(expireDate.getTime())) {
    return 0
  }
  const nextStart = new Date(expireDate.getFullYear(), expireDate.getMonth(), expireDate.getDate() + 1)
  return subtractRenewalCycle(nextStart, normalizeClientExpireCycle(cycle)).getTime()
}
