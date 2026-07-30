import type { Dispatch, SetStateAction } from 'react'

import type { CustomerAssignment, CustomerAssignmentDraft } from '../types'
import { type AdminRouteState, type AdminPageKey } from './adminRoute'
import type { AgentHealthFilter } from './agentHealth'
import { nodeElementId, outboundElementId, ruleElementId } from './appHelpersAgent'

interface LoadOptions {
  silent?: boolean
}

type TopologyLoader = (options?: LoadOptions) => void | Promise<void>
type TransitionRunner = (callback: () => void) => void
type TabSetter = Dispatch<SetStateAction<string>>
type NumberSetter = Dispatch<SetStateAction<number | null>>

interface AppNavigationHandlersOptions {
  topologyVisible: boolean
  topologyLoaded: boolean
  setActiveAdminPage: Dispatch<SetStateAction<AdminPageKey>>
  setTopologyVisible: Dispatch<SetStateAction<boolean>>
  setSelectedTag: Dispatch<SetStateAction<string>>
  setAgentHealthFilter: Dispatch<SetStateAction<AgentHealthFilter>>
  setTopologySearch: Dispatch<SetStateAction<string>>
  setSelectedOutboundTag: Dispatch<SetStateAction<string>>
  setSelectedRuleIndex: NumberSetter
  setSelectedNodeAnchor: Dispatch<SetStateAction<string>>
  setClientSearch: Dispatch<SetStateAction<string>>
  setSelectedAgentId: Dispatch<SetStateAction<string>>
  setActiveTabKey: TabSetter
  loadTopology: TopologyLoader
  runTransition: TransitionRunner
  setCustomerModalOpen: Dispatch<SetStateAction<boolean>>
  setCustomerAssignmentDraft: Dispatch<SetStateAction<CustomerAssignmentDraft | null>>
}

function scrollIntoViewById(targetID: string, options: ScrollIntoViewOptions, delay = 80) {
  window.setTimeout(() => {
    document.getElementById(targetID)?.scrollIntoView(options)
  }, delay)
}

function retryScrollToElement(
  resolveElement: () => HTMLElement | null,
  options: ScrollIntoViewOptions,
  maxAttempts = 20,
  delay = 120,
) {
  let attempts = 0
  const scrollToTarget = () => {
    attempts += 1
    const element = resolveElement()
    if (element) {
      element.scrollIntoView(options)
      return
    }
    if (attempts < maxAttempts) {
      window.setTimeout(scrollToTarget, delay)
    }
  }
  window.setTimeout(scrollToTarget, 80)
}

export function createAppNavigationHandlers(options: AppNavigationHandlersOptions) {
  const {
    topologyVisible,
    topologyLoaded,
    setActiveAdminPage,
    setTopologyVisible,
    setSelectedTag,
    setAgentHealthFilter,
    setTopologySearch,
    setSelectedOutboundTag,
    setSelectedRuleIndex,
    setSelectedNodeAnchor,
    setClientSearch,
    setSelectedAgentId,
    setActiveTabKey,
    loadTopology,
    runTransition,
    setCustomerModalOpen,
    setCustomerAssignmentDraft,
  } = options

  return {
    applyAdminRoute(route: AdminRouteState) {
      setActiveAdminPage(route.page)
      setTopologyVisible(route.topology)
      setSelectedTag(route.tag)
      setAgentHealthFilter(route.healthFilter)
      setTopologySearch(route.topologySearch)
      setSelectedOutboundTag(route.outboundTag)
      setSelectedRuleIndex(route.ruleIndex)
      setSelectedNodeAnchor(route.nodeAnchor)
      setClientSearch('')
      if (route.topology) {
        void loadTopology({ silent: topologyLoaded })
      }

      if (route.page === 'assets' || route.topology) {
        setSelectedAgentId(route.agentId)
        setActiveTabKey(route.topology ? 'overview' : route.tabKey || 'overview')
        return
      }

      setSelectedAgentId('')
      setActiveTabKey('overview')
    },

    openTopologyPanel() {
      setActiveAdminPage('dashboard')
      setAgentHealthFilter('all')
      setTopologyVisible(true)
      void loadTopology({ silent: topologyLoaded })
      scrollIntoViewById('topology-panel', { behavior: 'smooth', block: 'start' })
    },

    selectDashboardTag(tag: string) {
      setSelectedTag(tag)
      if (topologyVisible) {
        setSelectedAgentId('')
        setSelectedNodeAnchor('')
        setSelectedOutboundTag('')
        setSelectedRuleIndex(null)
      }
    },

    selectTopologyAgent(agentID: string) {
      setTopologyVisible(true)
      setSelectedNodeAnchor('')
      setSelectedOutboundTag('')
      setSelectedRuleIndex(null)
      runTransition(() => {
        setSelectedAgentId(agentID)
      })
      scrollIntoViewById('topology-panel', { behavior: 'smooth', block: 'start' })
    },

    returnHome() {
      setActiveAdminPage('dashboard')
      setAgentHealthFilter('all')
      setTopologyVisible(false)
      setSelectedAgentId('')
      setActiveTabKey('overview')
      setSelectedOutboundTag('')
      setSelectedRuleIndex(null)
      setSelectedNodeAnchor('')
    },

    openAgentDetailPanel(agentID: string, tabKey = 'overview') {
      setActiveAdminPage('assets')
      setAgentHealthFilter('all')
      setTopologyVisible(false)
      setActiveTabKey(tabKey)
      runTransition(() => {
        setSelectedAgentId(agentID)
      })
      scrollIntoViewById('agent-detail-panel', { behavior: 'smooth', block: 'start' })
    },

    openAgentHealthFilter(filter: Exclude<AgentHealthFilter, 'all'>) {
      setActiveAdminPage('assets')
      setTopologyVisible(false)
      setSelectedTag('')
      setAgentHealthFilter(filter)
      setSelectedAgentId('')
      setActiveTabKey('overview')
      setSelectedOutboundTag('')
      setSelectedRuleIndex(null)
      setSelectedNodeAnchor('')
    },

    openCustomerAssignment(assignment: CustomerAssignment) {
      if (!assignment.agent_id) {
        return
      }
      setCustomerModalOpen(false)
      setActiveAdminPage('assets')
      setAgentHealthFilter('all')
      setTopologyVisible(false)
      setSelectedOutboundTag('')
      setSelectedRuleIndex(null)

      if (assignment.client_email) {
        setSelectedNodeAnchor('')
        setClientSearch(assignment.client_email)
        setActiveTabKey('clients')
      } else {
        const nodeLabel = assignment.inbound_tag || String(assignment.inbound_id)
        setClientSearch('')
        setSelectedNodeAnchor(nodeElementId(assignment.agent_id, nodeLabel))
        setActiveTabKey('nodes')
      }

      runTransition(() => {
        setSelectedAgentId(assignment.agent_id)
      })

      retryScrollToElement(
        () => {
          const anchor = assignment.client_email
            ? 'agent-detail-panel'
            : nodeElementId(assignment.agent_id, assignment.inbound_tag || String(assignment.inbound_id))
          return document.getElementById(anchor) || document.getElementById('agent-detail-panel')
        },
        { behavior: 'smooth', block: assignment.client_email ? 'start' : 'center' },
      )
    },

    openCustomerAuthorization(draft: CustomerAssignmentDraft) {
      setCustomerModalOpen(false)
      setTopologyVisible(false)
      setSelectedOutboundTag('')
      setSelectedRuleIndex(null)
      setCustomerAssignmentDraft(draft)
      setActiveAdminPage('customers')
      scrollIntoViewById('customer-management-panel', { behavior: 'smooth', block: 'start' })
    },

    jumpToOutbound(tag?: string) {
      if (!tag) {
        return
      }
      setSelectedOutboundTag(tag)
      setActiveTabKey('outbounds')
      scrollIntoViewById(outboundElementId(tag), { behavior: 'smooth', block: 'center' }, 60)
    },

    jumpToRule(index?: number) {
      if (!index) {
        return
      }
      setSelectedRuleIndex(index)
      setActiveTabKey('routes')
      scrollIntoViewById(ruleElementId(index), { behavior: 'smooth', block: 'center' }, 60)
    },

    jumpToNode(agentID?: string, nodeLabel?: string) {
      if (!agentID || !nodeLabel) {
        return
      }
      const anchor = nodeElementId(agentID, nodeLabel)
      setSelectedNodeAnchor(anchor)
      if (topologyVisible) {
        setActiveAdminPage('dashboard')
        setTopologyVisible(true)
        setSelectedOutboundTag('')
        setSelectedRuleIndex(null)
        runTransition(() => {
          setSelectedAgentId(agentID)
        })
        scrollIntoViewById('topology-panel', { behavior: 'smooth', block: 'start' })
        return
      }
      setActiveAdminPage('assets')
      setAgentHealthFilter('all')
      setTopologyVisible(false)
      setSelectedAgentId(agentID)
      setActiveTabKey('nodes')

      retryScrollToElement(
        () => document.getElementById(anchor),
        { behavior: 'smooth', block: 'center' },
      )
    },
  }
}
