import { Button, Space, Tag } from 'antd'

import type { XUIRouteTrace } from '../types'
import { scopeColor, scopeLabel } from '../lib/appHelpers'

export interface RouteBadgeProps {
  route: XUIRouteTrace
  onJumpOutbound: (tag?: string) => void
  onJumpRule: (index?: number) => void
}

export function RouteBadge({ route, onJumpOutbound, onJumpRule }: RouteBadgeProps) {
  const targetLabel = route.outbound_tag || (route.balancer_tag ? `balancer:${route.balancer_tag}` : '未识别')

  return (
    <Space wrap size={[6, 6]}>
      {route.outbound_tag ? (
        <Button type="link" className="route-link" onClick={() => onJumpOutbound(route.outbound_tag)}>
          {targetLabel}
        </Button>
      ) : route.rule_index ? (
        <Button type="link" className="route-link" onClick={() => onJumpRule(route.rule_index)}>
          {targetLabel}
        </Button>
      ) : (
        <Tag>{targetLabel}</Tag>
      )}
      <Tag color={scopeColor(route.match_scope)}>{scopeLabel(route.match_scope)}</Tag>
      {route.rule_index ? (
        <Button type="link" className="route-rule-link" onClick={() => onJumpRule(route.rule_index)}>
          R{route.rule_index}
        </Button>
      ) : null}
      {route.has_global_rules && route.global_rule_indexes?.length ? (
        <Tag>+{route.global_rule_indexes.length} 全局规则</Tag>
      ) : null}
    </Space>
  )
}
