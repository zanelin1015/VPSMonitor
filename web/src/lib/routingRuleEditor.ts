import type { XUIRoutingRuleView } from '../types'

export interface RoutingRuleUpdatePayload {
  rules_fingerprint: string
  rule_index: number
  rule_fingerprint: string
  rule: Record<string, unknown>
}

const fingerprintPattern = /^[a-f0-9]{64}$/
export const ROUTING_MATCH_FIELDS = ['domain', 'ip', 'port', 'sourcePort', 'localPort', 'network', 'source', 'sourceIP', 'localIP', 'user', 'inboundTag', 'protocol', 'attrs', 'process', 'vlessRoute']

export function routingRuleEditDisabledReason(rule: XUIRoutingRuleView): string {
  if (rule.protected) return '系统 API 路由不能修改'
  if (!Number.isSafeInteger(rule.index) || rule.index <= 0 || !rule.fingerprint || !fingerprintPattern.test(rule.fingerprint) || !rule.raw_rule || typeof rule.raw_rule !== 'object' || Array.isArray(rule.raw_rule)) return '请更新 Server 并刷新 Client 信息，以读取完整路由规则'
  return ''
}

export function createRoutingRuleDraft(rule: XUIRoutingRuleView): Record<string, unknown> {
  const reason = routingRuleEditDisabledReason(rule)
  if (reason) throw new Error(reason)
  return JSON.parse(JSON.stringify(rule.raw_rule))
}

export function parseRoutingRuleJSON(text: string): Record<string, unknown> {
  let rule: unknown
  try { rule = JSON.parse(text) } catch { throw new Error('JSON 格式不正确，请检查引号、逗号和括号') }
  if (!rule || typeof rule !== 'object' || Array.isArray(rule)) throw new Error('路由规则必须是一个 JSON 对象')
  return rule as Record<string, unknown>
}

export function routingRuleList(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String)
  return typeof value === 'string' && value ? value.split(',').map(item => item.trim()).filter(Boolean) : []
}

export function routingRuleChanged(original: Record<string, unknown>, next: Record<string, unknown>): boolean {
  const canonical = (value: unknown): unknown => Array.isArray(value) ? value.map(canonical) : value && typeof value === 'object' ? Object.fromEntries(Object.entries(value).sort(([a], [b]) => a.localeCompare(b)).map(([key, item]) => [key, canonical(item)])) : value
  return JSON.stringify(canonical(original)) !== JSON.stringify(canonical(next))
}

export function buildRoutingRuleUpdatePayload(record: XUIRoutingRuleView, rulesFingerprint: string, rule: Record<string, unknown>): RoutingRuleUpdatePayload {
  const reason = routingRuleEditDisabledReason(record)
  if (reason) throw new Error(reason)
  if (!fingerprintPattern.test(rulesFingerprint)) throw new Error('路由规则校验信息缺失，请刷新后重新编辑')
  if (rule.type !== undefined && rule.type !== 'field') throw new Error('仅支持 field 路由规则')
  const outbound = typeof rule.outboundTag === 'string' ? rule.outboundTag.trim() : ''
  const balancer = typeof rule.balancerTag === 'string' ? rule.balancerTag.trim() : ''
  if (Boolean(outbound) === Boolean(balancer)) throw new Error('请选择一个出站或均衡器，不能同时指定两者')
  if (rule.enabled !== undefined && typeof rule.enabled !== 'boolean') throw new Error('enabled 必须是布尔值')
  for (const key of ['domain', 'ip', 'source', 'sourceIP', 'localIP', 'user', 'inboundTag', 'protocol', 'process']) {
    if (rule[key] !== undefined && (!Array.isArray(rule[key]) || (rule[key] as unknown[]).some(item => typeof item !== 'string' || !item.trim()))) throw new Error(`${key} 必须是非空字符串组成的数组`)
  }
  for (const key of ['port', 'sourcePort', 'localPort']) {
    if (rule[key] === undefined) continue
    const value = rule[key]
    if ((typeof value !== 'string' && typeof value !== 'number') || !String(value).trim()) throw new Error(`${key} 端口格式无效`)
    for (const part of String(value).split(',')) {
      const bounds = part.trim().split('-')
      if (bounds.length > 2 || bounds.some(item => !/^\d+$/.test(item.trim()) || Number(item) < 1 || Number(item) > 65535) || (bounds.length === 2 && Number(bounds[0]) > Number(bounds[1]))) throw new Error(`${key} 请输入 1–65535 的端口或范围，例如 443,1000-2000`)
    }
  }
  if (rule.network !== undefined && (!routingRuleList(rule.network).length || !routingRuleList(rule.network).every(value => ['tcp', 'udp'].includes(value)))) throw new Error('network 必须是 tcp、udp 或 tcp,udp')
  if (!ROUTING_MATCH_FIELDS.some(key => { const value = rule[key]; return typeof value === 'string' ? Boolean(value.trim()) : Array.isArray(value) ? value.length > 0 : value && typeof value === 'object' ? Object.keys(value).length > 0 : typeof value === 'number' })) throw new Error('请至少保留一项匹配条件，空条件规则不能保存')
  if (!routingRuleChanged(record.raw_rule!, rule)) throw new Error('规则内容未变化，无需保存')
  return { rules_fingerprint: rulesFingerprint, rule_index: record.index, rule_fingerprint: record.fingerprint!, rule: JSON.parse(JSON.stringify(rule)) }
}

export function isPendingRoutingUpdate(action: { kind: string; status: string }): boolean {
  return action.kind === 'update_routing_rule' && ['pending', 'running'].includes(action.status)
}
