import type { Dispatch, SetStateAction } from 'react'
import { useCallback, useDeferredValue, useEffect, useRef, useState } from 'react'
import type { AdminPageKey } from '../../lib/adminRoute'
import { type AgentHealthFilter } from '../../lib/agentHealth'
import type { AgentViewMode } from '../../lib/appHelpers'
import { readStoredAgentViewMode, storeAgentViewMode } from '../../lib/appHelpers'
import type { AdminUser, CustomerAssignmentDraft } from '../../types'

interface Options {
  adminUser: AdminUser | null
}

export function useAdminNavigationState({ adminUser }: Options) {
  const [selectedTag, setSelectedTag] = useState('')

  const [agentHealthFilter, setAgentHealthFilter] = useState<AgentHealthFilter>('all')

  const [topologySearch, setTopologySearch] = useState('')

  const [activeAdminPage, setActiveAdminPage] = useState<AdminPageKey>('dashboard')

  const [agentViewMode, setAgentViewMode] = useState<AgentViewMode>('card')

  const [selectedAgentId, setSelectedAgentId] = useState('')

  const [customerModalOpen, setCustomerModalOpen] = useState(false)

  const [customerAssignmentDraft, setCustomerAssignmentDraft] = useState<CustomerAssignmentDraft | null>(null)

  const [customerAuthorizationDraft, setCustomerAuthorizationDraft] = useState<CustomerAssignmentDraft | null>(null)

  const [reloadToken, setReloadToken] = useState(0)

  const [activeTabKey, setActiveTabKey] = useState('overview')

  const [topologyVisible, setTopologyVisibleState] = useState(false)

  const topologyVisibleRef = useRef(false)

  const setTopologyVisible = useCallback<Dispatch<SetStateAction<boolean>>>((value) => {
    setTopologyVisibleState((current) => {
      const next = typeof value === 'function' ? value(current) : value
      topologyVisibleRef.current = next
      return next
    })
  }, [])

  const [selectedOutboundTag, setSelectedOutboundTag] = useState('')

  const [selectedRuleIndex, setSelectedRuleIndex] = useState<number | null>(null)

  const [selectedNodeAnchor, setSelectedNodeAnchor] = useState('')

  const [clientSearch, setClientSearch] = useState('')

  const [agentListSearch, setAgentListSearch] = useState('')

  const deferredClientSearch = useDeferredValue(clientSearch.trim().toLowerCase())

  const deferredAgentListSearch = useDeferredValue(agentListSearch)

  const pendingClientSearchRef = useRef<{ agentID: string; query: string } | null>(null)

  useEffect(() => {
    if (!adminUser) {
      setAgentViewMode('card')
      return
    }
    setAgentViewMode(readStoredAgentViewMode(adminUser.username))
  }, [adminUser?.username])

  function toggleAgentViewMode() {
    setAgentViewMode((current) => {
      const next: AgentViewMode = current === 'list' ? 'card' : 'list'
      if (adminUser) {
        storeAgentViewMode(adminUser.username, next)
      }
      return next
    })
  }

  return {
    selectedTag,
    setSelectedTag,
    agentHealthFilter,
    setAgentHealthFilter,
    topologySearch,
    setTopologySearch,
    activeAdminPage,
    setActiveAdminPage,
    agentViewMode,
    setAgentViewMode,
    selectedAgentId,
    setSelectedAgentId,
    customerModalOpen,
    setCustomerModalOpen,
    customerAssignmentDraft,
    setCustomerAssignmentDraft,
    customerAuthorizationDraft,
    setCustomerAuthorizationDraft,
    reloadToken,
    setReloadToken,
    activeTabKey,
    setActiveTabKey,
    topologyVisible,
    setTopologyVisibleState,
    topologyVisibleRef,
    setTopologyVisible,
    selectedOutboundTag,
    setSelectedOutboundTag,
    selectedRuleIndex,
    setSelectedRuleIndex,
    selectedNodeAnchor,
    setSelectedNodeAnchor,
    clientSearch,
    setClientSearch,
    agentListSearch,
    setAgentListSearch,
    deferredClientSearch,
    deferredAgentListSearch,
    pendingClientSearchRef,
    toggleAgentViewMode,
  }
}
