import type { AgentListItem, AgentRealtimeMetrics, VPSSummary } from '../types'
import { appPathname, withAppBasePath } from './appBasePath'

export const CUSTOMER_SESSION_EXPIRED_EVENT = 'customer-session-expired'

let customerRefreshPromise: Promise<void> | null = null
let customerAuthGeneration = 0

export class APIError extends Error {
  status: number

  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

export async function fetchJSON<T>(url: string, init?: RequestInit): Promise<T> {
  const requestURL = withAppBasePath(url)
  const target = new URL(requestURL, window.location.href)
  const path = appPathname(target.pathname)
  const customerRequest = target.origin === window.location.origin && path.startsWith('/api/v1/customer/')
  const authAction = customerRequest && /^\/api\/v1\/customer\/(login|logout|refresh)$/.test(path)
  const canRefresh = customerRequest && !authAction && !path.startsWith('/api/v1/customer/subscription/')

  // A login/logout must finish after any pending renewal so it cannot be
  // overwritten by that renewal's Set-Cookie response.
  if (authAction && path !== '/api/v1/customer/refresh' && customerRefreshPromise) {
    await customerRefreshPromise.catch(() => {})
  }
  const generation = customerAuthGeneration
  const request = () => fetch(requestURL, { ...init, credentials: 'same-origin' })
  let response = await request()
  if (response.status === 401 && canRefresh && !init?.signal?.aborted) {
    try {
      // A late 401 may belong to a request sent before another request renewed
      // the cookie. Retry it with that cookie instead of refreshing again.
      if (generation === customerAuthGeneration) await refreshCustomerSession()
      response = await request()
    } catch (error) {
      if (isUnauthorized(error)) customerSessionExpired()
      throw error
    }
  }
  if (response.status === 401 && canRefresh) customerSessionExpired()
  const data = await readJSONResponse<T>(response)
  if (authAction && path !== '/api/v1/customer/refresh') customerAuthGeneration++
  return data
}

async function refreshCustomerSession(): Promise<void> {
  if (!customerRefreshPromise) {
    customerRefreshPromise = (async () => {
      const response = await fetch(withAppBasePath('/api/v1/customer/refresh'), {
        method: 'POST',
        credentials: 'same-origin',
      })
      await readJSONResponse(response)
      customerAuthGeneration++
    })().finally(() => { customerRefreshPromise = null })
  }
  await customerRefreshPromise
}

function customerSessionExpired() {
  window.dispatchEvent(new Event(CUSTOMER_SESSION_EXPIRED_EVENT))
}

async function readJSONResponse<T>(response: Response): Promise<T> {
  if (!response.ok) {
    let detail = response.statusText
    try {
      const payload = (await response.json()) as { error?: string }
      if (payload.error) {
        detail = payload.error
      }
    } catch {
      // ignore invalid json
    }
    throw new APIError(response.status, detail || `request failed: ${response.status}`)
  }
  return (await response.json()) as T
}

export function buildDashboardRealtimeURL(): string {
  const url = new URL(withAppBasePath('/api/v1/dashboard/realtime'), window.location.href)
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  return url.toString()
}

export function buildAgentTerminalURL(agentID: string, shell: string, cols = 120, rows = 36): string {
  const url = new URL(withAppBasePath(`/api/v1/agents/${encodeURIComponent(agentID)}/terminal/ws`), window.location.href)
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  if (shell) {
    url.searchParams.set('shell', shell)
  }
  url.searchParams.set('cols', String(cols))
  url.searchParams.set('rows', String(rows))
  return url.toString()
}

export function mergeRealtimeMetricsIntoAgents<T extends AgentListItem>(agents: T[], metrics: AgentRealtimeMetrics[]): T[] {
  if (!metrics.length) {
    return agents
  }
  const byAgent = new Map(metrics.map((metric) => [metric.agent_id, metric]))
  let changed = false
  const next = agents.map((agent) => {
    const metric = byAgent.get(agent.agent_id)
    if (!metric) {
      return agent
    }
    changed = true
    return {
      ...agent,
      agent_name: agent.agent_name || metric.agent_name,
      client_version: metric.client_version || agent.client_version,
      client_os: metric.client_os || agent.client_os,
      client_arch: metric.client_arch || agent.client_arch,
      system_version: metric.system_version || agent.system_version,
      realtime_at: metric.reported_at || agent.realtime_at,
      summary: mergeRealtimeSummary(agent.summary, metric.summary),
      ...(metric.haproxy ? { haproxy: metric.haproxy } : {}),
    }
  })
  return changed ? next : agents
}

export function sortAgentsByOrder<T extends AgentListItem>(agents: T[]): T[] {
  return [...agents].sort((left, right) => {
    const leftOrder = Number(left.sort_order || 0)
    const rightOrder = Number(right.sort_order || 0)
    if (leftOrder > 0 || rightOrder > 0) {
      if (leftOrder <= 0) return 1
      if (rightOrder <= 0) return -1
      if (leftOrder !== rightOrder) return leftOrder - rightOrder
    }
    const leftRegistered = Date.parse(left.registered_at || '')
    const rightRegistered = Date.parse(right.registered_at || '')
    if (!Number.isNaN(leftRegistered) || !Number.isNaN(rightRegistered)) {
      if (Number.isNaN(leftRegistered)) return 1
      if (Number.isNaN(rightRegistered)) return -1
      if (leftRegistered !== rightRegistered) return leftRegistered - rightRegistered
    }
    return left.agent_id.localeCompare(right.agent_id)
  })
}

export function mergeRealtimeSummary(current: VPSSummary, realtime: VPSSummary): VPSSummary {
  return {
    ...current,
    hostname: realtime.hostname || current.hostname,
    observed_ip: realtime.observed_ip || current.observed_ip,
    server_seen_ip: realtime.server_seen_ip || current.server_seen_ip,
    public_ipv4: realtime.public_ipv4 || current.public_ipv4,
    public_ipv6: realtime.public_ipv6 || current.public_ipv6,
    cpu: realtime.cpu ?? current.cpu,
    mem_used: realtime.mem_total ? realtime.mem_used : current.mem_used,
    mem_total: realtime.mem_total || current.mem_total,
    disk_used: realtime.disk_total ? realtime.disk_used : current.disk_used,
    disk_total: realtime.disk_total || current.disk_total,
    net_traffic_sent: realtime.net_traffic_sent ?? current.net_traffic_sent,
    net_traffic_recv: realtime.net_traffic_recv ?? current.net_traffic_recv,
    net_traffic_total: realtime.net_traffic_total ?? current.net_traffic_total,
    net_io_up: realtime.net_io_up ?? current.net_io_up,
    net_io_down: realtime.net_io_down ?? current.net_io_down,
    xray_state: realtime.xray_state || current.xray_state,
  }
}

export function isUnauthorized(error: unknown): boolean {
  return error instanceof APIError && error.status === 401
}
