import type { XUIBalancerView, XUIOutboundView, XUIRoutingRuleView } from '../types'

export interface OutboundRuleReference {
  rule: XUIRoutingRuleView
  direct: boolean
  balancers: string[]
}

// Use the same rule collection and indices as the routing page. Saved rules
// include disabled entries; runtime-only snapshots remain a display fallback.
export function buildOutboundRuleReferences(outbounds: XUIOutboundView[], rules: XUIRoutingRuleView[], balancers: XUIBalancerView[] = []): Map<string, OutboundRuleReference[]> {
  const result = new Map(outbounds.filter(item => item.tag).map(item => [item.tag!, [] as OutboundRuleReference[]]))
  const byBalancer = new Map(balancers.filter(item => item.tag).map(item => [item.tag!, item]))
  function targets(tag: string, seen = new Set<string>()): Set<string> {
    if (seen.has(tag)) return new Set()
    seen.add(tag)
    const balancer = byBalancer.get(tag)
    if (!balancer) return new Set()
    // outbound_tags are resolved by the Server using the panel's selectors.
    const tags = new Set((balancer.outbound_tags || []).filter(item => result.has(item)))
    if (balancer.fallback_tag) {
      if (result.has(balancer.fallback_tag)) tags.add(balancer.fallback_tag)
      else for (const item of targets(balancer.fallback_tag, seen)) tags.add(item)
    }
    return tags
  }
  const resolved = new Map([...byBalancer.keys()].map(tag => [tag, targets(tag)]))
  for (const rule of rules) {
    const tags = new Set<string>()
    if (rule.outbound_tag && result.has(rule.outbound_tag)) tags.add(rule.outbound_tag)
    if (rule.balancer_tag) for (const tag of resolved.get(rule.balancer_tag) || []) tags.add(tag)
    for (const tag of tags) result.get(tag)!.push({ rule, direct: rule.outbound_tag === tag, balancers: rule.balancer_tag && resolved.get(rule.balancer_tag)?.has(tag) ? [rule.balancer_tag] : [] })
  }
  return result
}
