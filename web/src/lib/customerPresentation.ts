import type { CustomerLinkStep, CustomerLinkView, CustomerOverviewResponse, CustomerUsageResponse } from '../types'
import { nodeImportURL, type NodeImportTarget } from './nodeImport'

export function customerUsageFallback(overview: CustomerOverviewResponse | null): CustomerUsageResponse {
  let upload = 0
  let download = 0
  let counted = 0
  let quota = 0
  let unlimited = false
  for (const link of overview?.links || []) {
    const multiplierValue = Number(link.traffic_multiplier || 1)
    const multiplier = Number.isFinite(multiplierValue) && multiplierValue > 0 ? multiplierValue : 1
    upload += Math.max(0, Math.round(Number(link.traffic_upload_bytes || 0) * multiplier))
    download += Math.max(0, Math.round(Number(link.traffic_download_bytes || 0) * multiplier))
    counted += Math.max(0, Number(link.traffic_used_bytes || 0))
    if (Number(link.traffic_limit_bytes || 0) <= 0) unlimited = true
    else quota += Number(link.traffic_limit_bytes || 0)
  }
  return {
    generated_at: overview?.generated_at || new Date().toISOString(),
    range_start: '',
    range_end: '',
    upload_bytes: upload,
    download_bytes: download,
    raw_total_bytes: upload + download,
    counted_total_bytes: counted,
    quota_bytes: quota,
    remaining_bytes: unlimited ? 0 : Math.max(0, quota - counted),
    unlimited,
    daily: [],
  }
}

export function customerAnnouncementReadKey(username: string): string {
  return `bridge-core.customer-announcements-read:${username}`
}

export function safeCustomerAnnouncementURL(value?: string): string {
  const raw = (value || '').trim()
  if (!raw) return ''
  try {
    const parsed = new URL(raw)
    return ['http:', 'https:', 'tg:', 'mailto:', 'tel:'].includes(parsed.protocol) ? parsed.toString() : ''
  } catch {
    return ''
  }
}

export function customerTopologyStepLabel(step: CustomerLinkStep): string {
  if (step.role !== 'exit' || !step.exit_ip) {
    return step.label
  }
  const escapedIP = step.exit_ip.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  return step.label.replace(new RegExp(`\\s*${escapedIP}\\s*$`), '').trim() || step.label
}

export function customerLinkDisplayName(link: CustomerLinkView, draft?: string): string {
  const draftName = (draft || '').trim()
  const savedRemark = (link.remark || '').trim()
  const routeName = customerLinkRouteName(link)
  return draftName || savedRemark || routeName || link.client_email || '授权链路'
}

export function customerLinkSummaryText(link: CustomerLinkView): string {
  const summary = (link.summary || '').trim()
  const withoutExit = summary.replace(/\s+出口\b.*$/u, '').trim()
  return withoutExit || customerLinkRouteName(link) || link.entry_client_name || `链路 #${link.assignment_id}`
}

function customerLinkRouteName(link: CustomerLinkView): string {
  const parts = [
    link.entry_client_name,
    ...(link.steps || []).filter((step) => step.role === 'relay').map((step) => step.label),
  ]
    .map((value) => (value || '').trim())
    .filter(Boolean)
  return Array.from(new Set(parts)).join('-')
}

type CustomerLinkMetaItem = {
  key: string
  text: string
  expiry?: {
    endTime: number
    label: string
    percent?: number
    strokeColor: string
    source: 'xray' | 'billing'
  }
}

export function customerLinkMetaItems(link: CustomerLinkView): CustomerLinkMetaItem[] {
  const items: CustomerLinkMetaItem[] = []
  if (link.traffic_reset_supported) {
    items.push({
      key: 'traffic-reset',
      text: `流量重置：${customerTrafficResetLabel(link.traffic_reset, link.traffic_reset_day)}`,
    })
  }
  if (Number(link.revenue_amount || 0) > 0) {
    items.push({
      key: 'revenue',
      text: `费用：${formatCustomerRecurringPrice(Number(link.revenue_amount || 0), link.revenue_currency || 'CNY', link.revenue_cycle)}`,
    })
  }
  if (Number(link.xray_expire_time || 0) > 0 || Number(link.expire_time || 0) > 0 || (Number(link.start_time || 0) > 0 && customerBillingCycleMonths(link.revenue_cycle || link.expire_cycle) > 0)) {
    const expiry = customerLinkExpiryProgress(link)
    const xrayExpiryTime = Number(link.xray_expire_time || 0)
    const expiryLabel = expiry?.endTime || Number(link.expire_time || 0)
    items.push({
      key: 'expiry',
      text: `${xrayExpiryTime > 0 ? 'Xray 到期' : '客户端到期'}：${formatCustomerExpiryTime(expiryLabel)}`,
      expiry,
    })
  }
  return items
}

function customerTrafficResetLabel(period?: string, day?: number): string {
  switch ((period || '').toLowerCase()) {
    case 'hourly':
      return '每小时'
    case 'daily':
      return '每日'
    case 'weekly':
      return '每周日'
    case 'monthly': {
      const value = Number(day)
      const normalizedDay = Number.isInteger(value) && value >= 1 && value <= 31 ? value : 1
      return `每月 ${normalizedDay} 号`
    }
    case 'never':
      return '从不'
    default:
      return '未上报'
  }
}

function customerLinkExpiryProgress(link: CustomerLinkView): CustomerLinkMetaItem['expiry'] {
  const xrayExpireTime = Number(link.xray_expire_time || 0)
  const expireTime = Number(link.expire_time || 0)
  const now = Date.now()
  const period = customerBillingPeriod(link, now)
  const hasXrayExpiry = Number.isFinite(xrayExpireTime) && xrayExpireTime > 0
  const hasBillingExpiry = Number.isFinite(expireTime) && expireTime > 0
  if (!hasXrayExpiry && !hasBillingExpiry && !period) return undefined
  // Xray owns the actual client validity. Billing periods still provide the
  // reset/progress baseline, but can never extend access past Xray expiry.
  const effectiveExpireTime = hasXrayExpiry ? xrayExpireTime : (period?.endTime ?? expireTime)
  const remainingDays = Math.ceil((effectiveExpireTime - now) / 86400000)
  const label = remainingDays <= 0 ? '已到期' : `剩余 ${remainingDays} 天`
  const progressEndTime = period && hasXrayExpiry
    ? Math.min(period.endTime, xrayExpireTime)
    : period?.endTime
  const percent = period && progressEndTime && progressEndTime > period.startTime
    ? Math.min(100, Math.max(0, (now - period.startTime) / (progressEndTime - period.startTime) * 100))
    : period
      ? 100
      : undefined

  return {
    endTime: effectiveExpireTime,
    label,
    percent,
    strokeColor: remainingDays <= 0 ? '#ef4444' : remainingDays <= 7 ? '#f59e0b' : '#2563eb',
    source: hasXrayExpiry ? 'xray' : 'billing',
  }
}

function customerBillingPeriod(link: CustomerLinkView, now: number): { startTime: number; endTime: number } | undefined {
  const startTime = Number(link.start_time || 0)
  const cycleMonths = customerBillingCycleMonths(link.revenue_cycle || link.expire_cycle)
  if (!Number.isFinite(startTime) || startTime <= 0 || cycleMonths <= 0) return undefined

  const anchor = new Date(startTime)
  const current = new Date(now)
  let cycleIndex = Math.max(0, Math.floor(monthDifference(anchor, current) / cycleMonths))
  let periodStart = addCalendarMonths(anchor, cycleIndex * cycleMonths)
  let nextPeriodStart = addCalendarMonths(anchor, (cycleIndex + 1) * cycleMonths)
  while (nextPeriodStart.getTime() <= now) {
    cycleIndex += 1
    periodStart = nextPeriodStart
    nextPeriodStart = addCalendarMonths(anchor, (cycleIndex + 1) * cycleMonths)
  }
  while (periodStart.getTime() > now && cycleIndex > 0) {
    cycleIndex -= 1
    nextPeriodStart = periodStart
    periodStart = addCalendarMonths(anchor, cycleIndex * cycleMonths)
  }

  return {
    startTime: periodStart.getTime(),
    // The next period starts at 00:00, so the current billing period ends just before it.
    endTime: nextPeriodStart.getTime() - 1,
  }
}

function customerBillingCycleMonths(cycle?: string): number {
  switch ((cycle || '').toLowerCase()) {
    case 'quarter':
    case 'quarterly':
      return 3
    case 'semiannual':
    case 'halfyear':
    case 'half-year':
      return 6
    case 'year':
    case 'yearly':
      return 12
    case 'month':
    case 'monthly':
      return 1
    default:
      return 0
  }
}

function monthDifference(start: Date, end: Date): number {
  return (end.getFullYear() - start.getFullYear()) * 12 + end.getMonth() - start.getMonth()
}

function addCalendarMonths(value: Date, months: number): Date {
  const result = new Date(value)
  const day = result.getDate()
  result.setDate(1)
  result.setMonth(result.getMonth() + months)
  const lastDay = new Date(result.getFullYear(), result.getMonth() + 1, 0).getDate()
  result.setDate(Math.min(day, lastDay))
  return result
}

function formatCustomerRecurringPrice(amount: number, currency: string, cycle?: string): string {
  return `${formatCustomerMoney(amount, currency)}/${cycleUnitLabel(cycle)}`
}

function formatCustomerMoney(amount: number, currency: string): string {
  const normalizedCurrency = (currency || 'CNY').toUpperCase()
  const amountText = formatCompactAmount(amount)
  if (normalizedCurrency === 'CNY') {
    return `${amountText}元`
  }
  return `${amountText}${normalizedCurrency}`
}

function formatCompactAmount(amount: number): string {
  if (Number.isInteger(amount)) {
    return String(amount)
  }
  try {
    return new Intl.NumberFormat('zh-CN', {
      minimumFractionDigits: 0,
      maximumFractionDigits: 2,
    }).format(amount)
  } catch {
    return amount.toFixed(2).replace(/\.?0+$/, '')
  }
}

function formatCustomerExpiryTime(value: number): string {
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  }).format(new Date(value))
}

export function formatCustomerTrafficRecordTime(value: string): string {
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) return value
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(parsed)
}

function cycleUnitLabel(cycle?: string): string {
  switch (cycle) {
    case 'quarter':
      return '季'
    case 'semiannual':
      return '半年'
    case 'year':
      return '年'
    case 'month':
    default:
      return '月'
  }
}

export function customerLinkImportURL(link: CustomerLinkView, draft?: string, target: NodeImportTarget = 'shadowrocket'): string {
  const source = nodeImportURL(link, target)
  if (!source) {
    return ''
  }
  const displayName = customerLinkDisplayName(link, draft)
  if (source.toLowerCase().startsWith('vmess://')) {
    return rewriteVMessImportName(source, displayName)
  }
  return rewriteURLFragment(source, displayName)
}

function rewriteURLFragment(source: string, displayName: string): string {
  try {
    const url = new URL(source)
    url.hash = displayName
    return url.toString()
  } catch {
    const encodedName = encodeURIComponent(displayName)
    return source.includes('#') ? source.replace(/#.*$/, `#${encodedName}`) : `${source}#${encodedName}`
  }
}

function rewriteVMessImportName(source: string, displayName: string): string {
  try {
    const payload = source.slice('vmess://'.length).trim()
    const normalized = payload.replace(/-/g, '+').replace(/_/g, '/')
    const padded = normalized.padEnd(Math.ceil(normalized.length / 4) * 4, '=')
    const raw = decodeURIComponent(escape(window.atob(padded)))
    const data = JSON.parse(raw) as Record<string, unknown>
    data.ps = displayName
    const nextRaw = unescape(encodeURIComponent(JSON.stringify(data)))
    return `vmess://${window.btoa(nextRaw)}`
  } catch {
    return source
  }
}
