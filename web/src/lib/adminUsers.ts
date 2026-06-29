import type { AdminUser, AgentRealtimeMetrics } from '../types'

export function sanitizeAreaRealtimeMetric(metric: AgentRealtimeMetrics): AgentRealtimeMetrics {
  return {
    agent_id: metric.agent_id,
    reported_at: metric.reported_at,
    summary: {
      net_traffic_sent: metric.summary?.net_traffic_sent,
      net_traffic_recv: metric.summary?.net_traffic_recv,
      net_traffic_total: metric.summary?.net_traffic_total,
      net_io_up: metric.summary?.net_io_up,
      net_io_down: metric.summary?.net_io_down,
    },
  }
}

export function isAreaManagerAdminUser(user: AdminUser | null): boolean {
  if (!user) {
    return false
  }
  if (user.role === 'area_manager') {
    return true
  }
  if (user.role === 'admin') {
    return false
  }
  return Boolean((user.agent_ids || []).length || (user.id && user.id !== 1))
}
