import type { MessageInstance } from 'antd/es/message/interface'
import type { Dispatch, MutableRefObject, SetStateAction } from 'react'
import { useRef, useState } from 'react'
import { useDashboardRealtime } from './useDashboardRealtime'
import { sanitizeAreaRealtimeMetric } from '../../lib/adminUsers'
import {
  fetchJSON,
  isUnauthorized,
  mergeRealtimeMetricsIntoAgents,
  mergeRealtimeSummary,
  mergeTagOptions,
  sortAgentsByOrder,
} from '../../lib/appHelpers'
import type { AdminUser, AgentRealtimeMetrics, DashboardAgentView, GlobalDashboardView, XUIOverview } from '../../types'

interface LoadOptions {
  silent?: boolean
}

function hasCustomerDisplayNameField(value: unknown): boolean {
  return Boolean(
    value && typeof value === 'object' && Object.prototype.hasOwnProperty.call(value, 'customer_display_name'),
  )
}

interface Options {
  adminUser: AdminUser | null
  topologyVisibleRef: MutableRefObject<boolean>
  selectedAgentId: string
  setSelectedAgentId: Dispatch<SetStateAction<string>>
  setAdminUser: Dispatch<SetStateAction<AdminUser | null>>
  message: MessageInstance
  isAreaManagerAccount: boolean
  setOverview: Dispatch<SetStateAction<XUIOverview | null>>
}

export function useDashboardData({
  adminUser,
  topologyVisibleRef,
  selectedAgentId,
  setSelectedAgentId,
  setAdminUser,
  message,
  isAreaManagerAccount,
  setOverview,
}: Options) {
  const [dashboardLoaded, setDashboardLoaded] = useState(false)

  const [agents, setAgents] = useState<DashboardAgentView[]>([])

  const [agentsLoading, setAgentsLoading] = useState(false)

  const [agentsError, setAgentsError] = useState('')

  const [dashboardView, setDashboardView] = useState<GlobalDashboardView | null>(null)

  const customerDisplayNameCacheRef = useRef<Record<string, string>>({})

  const [tagOptions, setTagOptions] = useState<string[]>([])

  const [topologyLoading, setTopologyLoading] = useState(false)

  const [topologyError, setTopologyError] = useState('')

  const [topologyLoaded, setTopologyLoaded] = useState(false)

  const topologyLoadedRef = useRef(false)

  const inFlightRequestsRef = useRef<Set<string>>(new Set())

  useDashboardRealtime(adminUser, applyRealtimeMetrics)

  async function loadAgents(options: LoadOptions = {}) {
    const requestKey = 'dashboard'
    if (inFlightRequestsRef.current.has(requestKey)) {
      return
    }
    inFlightRequestsRef.current.add(requestKey)
    const silent = Boolean(options.silent)
    if (!silent) {
      setAgentsLoading(true)
      setAgentsError('')
    }
    try {
      const data = await fetchJSON<GlobalDashboardView>('/api/v1/dashboard')
      const sortedAgents = sortAgentsByOrder((data.agents || []).map(applyCachedCustomerDisplayName))
      const nextView = { ...data, agents: sortedAgents }
      setDashboardView((current) => {
        if (!topologyVisibleRef.current || !current || !topologyLoadedRef.current) {
          return nextView
        }
        return {
          ...nextView,
          links: current.links || [],
          client_chains: current.client_chains || [],
          totals: {
            ...nextView.totals,
            link_count: current.totals?.link_count ?? nextView.totals.link_count,
            chain_count: current.totals?.chain_count ?? nextView.totals.chain_count,
          },
        }
      })
      setAgents(sortedAgents)
      setDashboardLoaded(true)
      setTagOptions((current) =>
        mergeTagOptions(
          current,
          sortedAgents.flatMap((agent) => agent.tags || []),
        ),
      )
      setAgentsError('')
      if (
        !sortedAgents.length ||
        (selectedAgentId && !sortedAgents.some((item) => item.agent_id === selectedAgentId))
      ) {
        setSelectedAgentId('')
      }
      if (topologyVisibleRef.current) {
        void loadTopology({ silent: true })
      }
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      if (!silent) {
        setAgentsError(error instanceof Error ? error.message : '加载 VPS 列表失败')
        message.error('无法加载 VPS 列表')
      }
    } finally {
      if (!silent) {
        setAgentsLoading(false)
      }
      inFlightRequestsRef.current.delete(requestKey)
    }
  }

  async function loadTopology(options: LoadOptions = {}) {
    const requestKey = 'dashboard-topology'
    if (inFlightRequestsRef.current.has(requestKey)) {
      return
    }
    inFlightRequestsRef.current.add(requestKey)
    const silent = Boolean(options.silent)
    if (!silent) {
      setTopologyLoading(true)
      setTopologyError('')
    }
    try {
      const data = await fetchJSON<GlobalDashboardView>('/api/v1/dashboard/topology')
      const sortedAgents = sortAgentsByOrder((data.agents || []).map(applyCachedCustomerDisplayName))
      setDashboardView({ ...data, agents: sortedAgents })
      setAgents(sortedAgents)
      setTagOptions((current) =>
        mergeTagOptions(
          current,
          sortedAgents.flatMap((agent) => agent.tags || []),
        ),
      )
      setDashboardLoaded(true)
      topologyLoadedRef.current = true
      setTopologyLoaded(true)
      setTopologyError('')
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      const messageText = error instanceof Error ? error.message : '加载链路拓扑失败'
      setTopologyError(messageText)
      if (!silent) {
        message.error('无法加载链路拓扑')
      }
    } finally {
      if (!silent) {
        setTopologyLoading(false)
      }
      inFlightRequestsRef.current.delete(requestKey)
    }
  }

  function rememberCustomerDisplayName(agentID: string, name: string) {
    if (!agentID) {
      return
    }
    customerDisplayNameCacheRef.current = {
      ...customerDisplayNameCacheRef.current,
      [agentID]: name,
    }
  }

  function cachedCustomerDisplayName(agentID: string): string | undefined {
    if (!Object.prototype.hasOwnProperty.call(customerDisplayNameCacheRef.current, agentID)) {
      return undefined
    }
    return customerDisplayNameCacheRef.current[agentID]
  }

  function applyCachedCustomerDisplayName(agent: DashboardAgentView): DashboardAgentView {
    if (hasCustomerDisplayNameField(agent)) {
      rememberCustomerDisplayName(agent.agent_id, agent.customer_display_name || '')
      return agent
    }
    const cachedName = cachedCustomerDisplayName(agent.agent_id)
    if (cachedName === undefined) {
      return agent
    }
    return { ...agent, customer_display_name: cachedName }
  }

  function applyRealtimeMetrics(metrics: AgentRealtimeMetrics[]) {
    const visibleMetrics = isAreaManagerAccount ? metrics.map(sanitizeAreaRealtimeMetric) : metrics
    setAgents((current) => mergeRealtimeMetricsIntoAgents(current, visibleMetrics))
    setDashboardView((current) =>
      current
        ? {
            ...current,
            agents: mergeRealtimeMetricsIntoAgents(current.agents, visibleMetrics),
          }
        : current,
    )
    setOverview((current) => {
      if (!current) {
        return current
      }
      if (isAreaManagerAccount) {
        return current
      }
      const metric = visibleMetrics.find((item) => item.agent_id === current.agent_id)
      if (!metric) {
        return current
      }
      return {
        ...current,
        summary: mergeRealtimeSummary(current.summary, metric.summary),
      }
    })
  }

  return {
    dashboardLoaded,
    setDashboardLoaded,
    agents,
    setAgents,
    agentsLoading,
    setAgentsLoading,
    agentsError,
    setAgentsError,
    dashboardView,
    setDashboardView,
    customerDisplayNameCacheRef,
    tagOptions,
    setTagOptions,
    topologyLoading,
    setTopologyLoading,
    topologyError,
    setTopologyError,
    topologyLoaded,
    setTopologyLoaded,
    topologyLoadedRef,
    inFlightRequestsRef,
    loadAgents,
    loadTopology,
    rememberCustomerDisplayName,
    cachedCustomerDisplayName,
    applyCachedCustomerDisplayName,
    applyRealtimeMetrics,
  }
}
