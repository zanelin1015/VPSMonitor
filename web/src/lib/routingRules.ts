import type { XUIRoutingRuleView } from '../types'

export interface RoutingRulesDeletePayload {
  rules_fingerprint: string
  rules: { index: number; fingerprint: string }[]
}

const fingerprintPattern = /^[a-f0-9]{64}$/

export function routingRuleDeleteDisabledReason(rule: XUIRoutingRuleView): string {
  if (rule.protected) {
    return 'x-ui 系统 API 路由不能删除'
  }
  if (!rule.fingerprint || !fingerprintPattern.test(rule.fingerprint)) {
    return '请更新 Server 并刷新 Client 信息后操作'
  }
  return ''
}

export function buildRoutingRulesDeletePayload(rules: XUIRoutingRuleView[], rulesFingerprint?: string): RoutingRulesDeletePayload {
  if (!rulesFingerprint || !fingerprintPattern.test(rulesFingerprint)) {
    throw new Error('路由规则校验信息缺失，请刷新 Client 信息')
  }
  if (!rules.length || rules.length > 1000) {
    throw new Error('请选择 1 至 1000 条路由规则')
  }
  const seen = new Set<number>()
  const targets = rules.map((rule) => {
    const reason = routingRuleDeleteDisabledReason(rule)
    if (reason) throw new Error(reason)
    if (!Number.isSafeInteger(rule.index) || rule.index <= 0 || seen.has(rule.index)) {
      throw new Error('路由规则索引无效，请刷新后重新选择')
    }
    seen.add(rule.index)
    return { index: rule.index, fingerprint: rule.fingerprint! }
  })
  // Descending order avoids shifted indices even if the consumer processes
  // the selected rules one at a time.
  return { rules_fingerprint: rulesFingerprint, rules: targets.sort((a, b) => b.index - a.index) }
}

export function isPendingRoutingDeletion(action: { kind: string; status: string }): boolean {
  return action.kind === 'delete_routing_rules' && ['pending', 'running'].includes(action.status)
}
