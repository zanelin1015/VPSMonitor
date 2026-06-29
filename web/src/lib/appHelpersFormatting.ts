import type { AgentListItem, ConfigAuditLog, ManagedAgentConfig, XUIRoutingRuleView, VPSSummary } from '../types'
import { formatMem, formatPercent } from './traffic'

export function shortJSON(value: unknown): string {
  if (!value || (typeof value === 'object' && Object.keys(value as Record<string, unknown>).length === 0)) {
    return ''
  }
  const text = JSON.stringify(value)
  return text.length > 140 ? `${text.slice(0, 140)}...` : text
}

export function summarizeConfigAudit(item: ConfigAuditLog): string {
  const before = (item.before || {}) as ManagedAgentConfig
  const after = (item.after || {}) as ManagedAgentConfig
  const changes: string[] = []
  if (before.agent_name !== after.agent_name) {
    changes.push('名称')
  }
  if (before.customer_display_name !== after.customer_display_name) {
    changes.push('用户展示名称')
  }
  if (JSON.stringify(before.tags || []) !== JSON.stringify(after.tags || [])) {
    changes.push('标签')
  }
  if (JSON.stringify(before.renewal || {}) !== JSON.stringify(after.renewal || {})) {
    changes.push('VPS 周期')
  }
  if (JSON.stringify(before.entry || {}) !== JSON.stringify(after.entry || {})) {
    changes.push('入口/NAT')
  }
  if (JSON.stringify(before.xui || {}) !== JSON.stringify(after.xui || {})) {
    changes.push('X-UI')
  }
  return changes.length ? `修改了 ${changes.join('、')}` : '保存了配置'
}

export function confidenceLabel(value?: string): string {
  switch (value) {
    case 'high':
      return '高置信'
    case 'medium':
      return '中置信'
    case 'low':
      return '低置信'
    default:
      return '未标记'
  }
}

export function formatDateInputFromMillis(value?: number): string {
  if (!value) {
    return ''
  }
  const date = new Date(value)
  if (!Number.isFinite(date.getTime())) {
    return ''
  }
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

export function dateInputToExpiryMillis(value: string): number {
  if (!value) {
    return 0
  }
  const date = new Date(`${value}T23:59:59`)
  if (!Number.isFinite(date.getTime())) {
    return 0
  }
  return date.getTime()
}

export function formatExpiryTime(value?: number): string {
  if (!value) {
    return '无限期'
  }
  const date = new Date(value)
  if (!Number.isFinite(date.getTime())) {
    return '无限期'
  }
  return formatDateTime(date.toISOString())
}

export function formatDateTime(value?: string): string {
  if (!value) {
    return '-'
  }
  const parsed = parseTimestampMillis(value)
  if (!Number.isFinite(parsed)) {
    return '-'
  }
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).format(new Date(parsed))
}

export function formatRelativeTime(value?: number): string {
  if (!value) {
    return '未活跃'
  }
  const target = new Date(value)
  const diffMinutes = Math.round((Date.now() - target.getTime()) / 60000)
  if (diffMinutes < 1) {
    return '刚刚'
  }
  if (diffMinutes < 60) {
    return `${diffMinutes} 分钟前`
  }
  const diffHours = Math.round(diffMinutes / 60)
  if (diffHours < 24) {
    return `${diffHours} 小时前`
  }
  const diffDays = Math.round(diffHours / 24)
  if (diffDays < 30) {
    return `${diffDays} 天前`
  }
  return formatDateTime(target.toISOString())
}

export function scopeLabel(scope?: string): string {
  switch (scope) {
    case 'user':
      return '用户规则'
    case 'inbound':
      return '节点规则'
    case 'default':
      return '默认出口'
    case 'global':
      return '全局规则'
    default:
      return '未推断'
  }
}

export function scopeColor(scope?: string): string {
  switch (scope) {
    case 'user':
      return 'gold'
    case 'inbound':
      return 'blue'
    case 'default':
      return 'green'
    case 'global':
      return 'magenta'
    default:
      return 'default'
  }
}

export function summarizeRule(rule: XUIRoutingRuleView): string {
  const segments: string[] = []
  if (rule.domain?.length) {
    segments.push(`domain(${rule.domain.length})`)
  }
  if (rule.ip?.length) {
    segments.push(`ip(${rule.ip.length})`)
  }
  if (rule.protocol?.length) {
    segments.push(`protocol(${rule.protocol.length})`)
  }
  if (rule.port?.length) {
    segments.push(`port(${rule.port.join(',')})`)
  }
  if (rule.network?.length) {
    segments.push(`network(${rule.network.join(',')})`)
  }
  return segments.join(' · ') || '无额外条件'
}

export function statusColor(state?: string): string {
  switch ((state || '').toLowerCase()) {
    case 'running':
      return '#0f766e'
    case 'stop':
      return '#c05621'
    case 'error':
      return '#c53030'
    default:
      return '#94a3b8'
  }
}

export function agentStatusLevel(state?: string): 'ok' | 'warn' | 'bad' | 'neutral' {
  switch ((state || '').toLowerCase()) {
    case 'running':
      return 'ok'
    case 'stop':
    case 'stopped':
      return 'warn'
    case 'error':
      return 'bad'
    default:
      return 'neutral'
  }
}

export function summarizeAgent(summary?: VPSSummary): string {
  if (!summary) {
    return '-'
  }
  return `${formatPercent(summary.cpu)} CPU · ${formatMem(summary.mem_used, summary.mem_total)} · 硬盘 ${formatMem(summary.disk_used, summary.disk_total)}`
}

function parseTimestampMillis(value?: string): number {
  const candidates = parseTimestampMillisCandidates(value)
  return candidates.length ? candidates[0] : Number.NaN
}

function parseTimestampMillisCandidates(value?: string): number[] {
  const text = String(value || '').trim()
  if (!text) {
    return []
  }
  const candidates: number[] = []
  const add = (candidate: string) => {
    const parsed = Date.parse(candidate)
    if (Number.isFinite(parsed) && !candidates.includes(parsed)) {
      candidates.push(parsed)
    }
  }

  add(text)

  const normalized = text.replace(/^(\d{4})\/(\d{2})\/(\d{2})/, '$1-$2-$3').replace(' ', 'T')
  const hasTime = /T\d{2}:\d{2}/.test(normalized)
  const hasTimezone = /(?:Z|[+-]\d{2}:?\d{2})$/i.test(normalized)
  if (hasTime && !hasTimezone) {
    add(`${normalized}Z`)
  }
  return candidates
}
