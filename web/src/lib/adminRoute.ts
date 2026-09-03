import { normalizeAgentHealthFilter, type AgentHealthFilter } from './agentHealth'

export type AdminPageKey = 'dashboard' | 'assets' | 'customers' | 'front-proxies' | 'support' | 'access-logs' | 'settings' | 'schedules'

export interface AdminRouteState {
  page: AdminPageKey
  topology: boolean
  agentId: string
  tabKey: string
  tag: string
  healthFilter: AgentHealthFilter
  outboundTag: string
  ruleIndex: number | null
  nodeAnchor: string
  topologySearch: string
}

export function parseAdminRouteState(canManageSystem: boolean, canViewFrontProxies = canManageSystem): AdminRouteState {
  const params = new URLSearchParams(window.location.search)
  const path = window.location.pathname.replace(/\/+$/, '')
  const rawPage = (params.get('page') || pageFromAdminPath(path)).toLowerCase()
  const topology = rawPage === 'topology' || params.get('topology') === '1'
  let page: AdminPageKey = topology ? 'dashboard' : normalizeAdminPage(rawPage)
  if ((page === 'settings' || page === 'schedules' || page === 'access-logs') && !canManageSystem) {
    page = 'dashboard'
  }
  if (page === 'front-proxies' && !canViewFrontProxies) {
    page = 'dashboard'
  }

  const ruleParam = Number(params.get('rule') || '')
  return {
    page,
    topology,
    agentId: params.get('agent') || agentFromAdminPath(path),
    tabKey: params.get('tab') || 'overview',
    tag: params.get('tag') || '',
    healthFilter: normalizeAgentHealthFilter(params.get('health')),
    outboundTag: params.get('outbound') || '',
    ruleIndex: Number.isInteger(ruleParam) && ruleParam > 0 ? ruleParam : null,
    nodeAnchor: params.get('node') || '',
    topologySearch: params.get('q') || '',
  }
}

export function buildAdminRouteURL(route: AdminRouteState): string {
  const params = new URLSearchParams()
  if (route.topology) {
    params.set('page', 'topology')
  } else if (route.page !== 'dashboard') {
    params.set('page', route.page)
  }
  if (route.tag) {
    params.set('tag', route.tag)
  }
  if (route.topology) {
    if (route.agentId) {
      params.set('agent', route.agentId)
    }
    if (route.topologySearch.trim()) {
      params.set('q', route.topologySearch.trim())
    }
  } else if (route.page === 'assets') {
    if (route.healthFilter !== 'all') {
      params.set('health', route.healthFilter)
    }
    if (route.agentId) {
      params.set('agent', route.agentId)
    }
    if (route.agentId && route.tabKey && route.tabKey !== 'overview') {
      params.set('tab', route.tabKey)
    }
    if (route.outboundTag) {
      params.set('outbound', route.outboundTag)
    }
    if (route.ruleIndex) {
      params.set('rule', String(route.ruleIndex))
    }
    if (route.nodeAnchor) {
      params.set('node', route.nodeAnchor)
    }
  } else if (route.page === 'support') {
    const conversation = Number(new URLSearchParams(window.location.search).get('conversation') || '')
    if (Number.isInteger(conversation) && conversation > 0) {
      params.set('conversation', String(conversation))
    }
  }
  const query = params.toString()
  return query ? `/?${query}` : '/'
}

function normalizeAdminPage(value: string): AdminPageKey {
  switch (value) {
    case 'assets':
    case 'customers':
    case 'front-proxies':
    case 'support':
    case 'access-logs':
    case 'settings':
    case 'schedules':
      return value
    default:
      return 'dashboard'
  }
}

function pageFromAdminPath(path: string): string {
  switch (path) {
    case '/admin/assets':
      return 'assets'
    case '/admin/customers':
      return 'customers'
    case '/admin/front-proxies':
      return 'front-proxies'
    case '/admin/support':
      return 'support'
    case '/admin/access-logs':
      return 'access-logs'
    case '/admin/settings':
      return 'settings'
    case '/admin/schedules':
      return 'schedules'
    case '/admin/topology':
      return 'topology'
    default:
      return 'dashboard'
  }
}

function agentFromAdminPath(path: string): string {
  const match = path.match(/^\/admin\/assets\/([^/]+)$/)
  return match ? decodeURIComponent(match[1]) : ''
}
