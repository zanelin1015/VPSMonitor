import { useEffect, useRef } from 'react'

import type { AdminPageKey, AdminRouteState } from '../lib/adminRoute'
import { buildAdminRouteURL, parseAdminRouteState } from '../lib/adminRoute'

export function useAdminRouteSync(input: {
  enabled: boolean
  sessionIdentity: unknown
  canManageSystem: boolean
  activeAdminPage: AdminPageKey
  activeTabKey: string
  selectedAgentId: string
  selectedNodeAnchor: string
  selectedOutboundTag: string
  selectedRuleIndex: number | null
  selectedTag: string
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
    activeAdminPage,
    activeTabKey,
    selectedAgentId,
    selectedNodeAnchor,
    selectedOutboundTag,
    selectedRuleIndex,
    selectedTag,
    topologySearch,
    topologyVisible,
    applyAdminRoute,
  } = input

  useEffect(() => {
    if (!enabled) {
      return
    }

    const applyCurrentURL = () => {
      const route = parseAdminRouteState(canManageSystem)
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
  }, [canManageSystem, enabled, sessionIdentity])

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
    selectedNodeAnchor,
    selectedOutboundTag,
    selectedRuleIndex,
    selectedTag,
    topologySearch,
    topologyVisible,
  ])
}
