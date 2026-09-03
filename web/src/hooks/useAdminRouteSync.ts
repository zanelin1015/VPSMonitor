import { useEffect, useRef } from 'react'

import type { AdminPageKey, AdminRouteState } from '../lib/adminRoute'
import { buildAdminRouteURL, parseAdminRouteState } from '../lib/adminRoute'
import type { AgentHealthFilter } from '../lib/agentHealth'

export function useAdminRouteSync(input: {
  enabled: boolean
  sessionIdentity: unknown
  canManageSystem: boolean
  canViewFrontProxies: boolean
  activeAdminPage: AdminPageKey
  activeTabKey: string
  selectedAgentId: string
  selectedNodeAnchor: string
  selectedOutboundTag: string
  selectedRuleIndex: number | null
  selectedTag: string
  agentHealthFilter: AgentHealthFilter
  topologySearch: string
  topologyVisible: boolean
  applyAdminRoute: (route: AdminRouteState) => void
}) {
  const applyingRouteRef = useRef(false)
  const lastAdminURLRef = useRef('')
  const {
    enabled,
    sessionIdentity,
    canManageSystem,
    canViewFrontProxies,
    activeAdminPage,
    activeTabKey,
    selectedAgentId,
    selectedNodeAnchor,
    selectedOutboundTag,
    selectedRuleIndex,
    selectedTag,
    agentHealthFilter,
    topologySearch,
    topologyVisible,
    applyAdminRoute,
  } = input

  useEffect(() => {
    if (!enabled) {
      return
    }

    const applyCurrentURL = () => {
      const route = parseAdminRouteState(canManageSystem, canViewFrontProxies)
      const normalizedURL = buildAdminRouteURL(route)
      const currentURL = `${window.location.pathname}${window.location.search}`
      if (currentURL !== normalizedURL) {
        window.history.replaceState(null, '', normalizedURL)
      }
      lastAdminURLRef.current = normalizedURL
      applyingRouteRef.current = true
      applyAdminRoute(route)
      window.setTimeout(() => {
        applyingRouteRef.current = false
      }, 0)
    }

    applyCurrentURL()
    window.addEventListener('popstate', applyCurrentURL)
    return () => {
      window.removeEventListener('popstate', applyCurrentURL)
    }
  }, [canManageSystem, canViewFrontProxies, enabled, sessionIdentity])

  useEffect(() => {
    if (!enabled || applyingRouteRef.current) {
      return
    }
    const nextURL = buildAdminRouteURL({
      page: activeAdminPage,
      topology: topologyVisible,
      agentId: selectedAgentId,
      tabKey: activeTabKey,
      tag: selectedTag,
      healthFilter: agentHealthFilter,
      outboundTag: selectedOutboundTag,
      ruleIndex: selectedRuleIndex,
      nodeAnchor: selectedNodeAnchor,
      topologySearch,
    })
    const currentURL = `${window.location.pathname}${window.location.search}`
    if (currentURL === nextURL || lastAdminURLRef.current === nextURL) {
      lastAdminURLRef.current = nextURL
      return
    }
    window.history.pushState(null, '', nextURL)
    lastAdminURLRef.current = nextURL
  }, [
    activeAdminPage,
    activeTabKey,
    enabled,
    sessionIdentity,
    selectedAgentId,
    agentHealthFilter,
    selectedNodeAnchor,
    selectedOutboundTag,
    selectedRuleIndex,
    selectedTag,
    topologySearch,
    topologyVisible,
  ])
}
