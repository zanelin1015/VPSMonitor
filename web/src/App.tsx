import { lazy, startTransition, Suspense, useCallback, useDeferredValue, useEffect, useRef, useState } from 'react'
import type { Dispatch, SetStateAction } from 'react'
import {
  Alert,
  App as AntdApp,
  Button,
  Card,
  Spin,
} from 'antd'

import type {
  AreaManagerAdminView,
  AreaAgentTagsResponse,
  AgentLogsResponse,
  AgentRefreshResponse,
  AgentReplacementResult,
  AgentRealtimeMetrics,
  ConfigAuditLog,
  CustomerAdminView,
  CustomerAssignmentDraft,
  DashboardAgentView,
  DashboardRealtimeMessage,
  ExchangeRatesResponse,
  GlobalDashboardView,
  ManagedAgentConfig,
  SupportUnreadResponse,
  TagSettingsResponse,
  VPSSummary,
  XUIAction,
  XUIClientBillingConfig,
  XUIClientView,
  XUINodeView,
  XUIOverview,
} from './types'
import {
  COMMON_COST_CURRENCIES,
  COST_CURRENCY_STORAGE_KEY,
  DEFAULT_COST_CURRENCY,
  type CurrencyCode,
  type ExchangeRatesState,
  defaultExchangeRatesState,
  mergeCurrencyOptions,
  normalizeCurrencyCode,
  readStoredCostCurrency,
} from './lib/currency'
import { useCurrentAgentRequest } from './hooks/useCurrentAgentRequest'
import { useAdminSession } from './hooks/useAdminSession'
import { useAdminSystemTools } from './hooks/useAdminSystemTools'
import { useAdminRouteSync } from './hooks/useAdminRouteSync'
import { useXUIRemoteActions } from './hooks/useXUIRemoteActions'
import type { AdminPageKey } from './lib/adminRoute'
import { isAreaManagerAdminUser, sanitizeAreaRealtimeMetric } from './lib/adminUsers'
import { createAppNavigationHandlers } from './lib/appNavigation'
import { normalizeScheduledTaskSettings } from './lib/scheduledTasks'
import { buildDashboardScope } from './lib/appDashboardScope'
import { agentMatchesHealthFilter, type AgentHealthFilter } from './lib/agentHealth'

import type {
  AgentViewMode,
  ConfigSectionKey,
  XUIAddClientActionForm,
  XUIOutboundActionForm,
  XUIRoutingActionForm,
} from './lib/appHelpers'
import {
  FrontendSettingsPanel,
  ScheduledTasksPanel,
} from './components/AdminModals'
import { AdminShellNavigation, AdminShellTopbar } from './components/AdminShellNavigation'
import { AdminAccessLogsPage } from './components/AdminAccessLogsPage'
import { renderCNFlowPanel } from './components/DashboardTopologyPanels'
import { AgentRail, AdminWorkbenchDashboard, OverviewSummaryCard } from './components/DashboardSidebar'
import { LoginScreen } from './components/LoginScreen'
import { VisualEffects } from './components/VisualEffects'
import { useAppTheme } from './theme'
import {
  DASHBOARD_AUTO_REFRESH_MS,
  billingKeyForClient,
  buildDashboardRealtimeURL,
  buildSectionSavePayload,
  buildXUIActionPayload,
  configSectionLabel,
  configSignature,
  createEmptyManagedConfig,
  defaultAddClientActionForm,
  defaultClientBilling,
  defaultOutboundActionForm,
  defaultRoutingActionForm,
  effectiveClientBillingExpiryTime,
  findOutboundLinkedClient,
  findClientBilling,
  fetchJSON,
  formatAddressInput,
  formatRenewalHint,
  formatTagInput,
  isUnauthorized,
  mergeRealtimeMetricsIntoAgents,
  mergeRealtimeSummary,
  mergeSavedSectionIntoDraft,
  mergeTagOptions,
  normalizeEntryConfig,
  normalizeManagedConfig,
  normalizeNodeAnchorLabel,
  normalizeXUIOverview,
  parseAddressInput,
  parseTagInput,
  readStoredAgentViewMode,
  scopeColor,
  scopeLabel,
  sortAgentsByOrder,
  statusColor,
  storeAgentViewMode,
  summarizeAgent,
  summarizeConfigAudit,
  summarizeRule,
  upsertClientBilling,
} from './lib/appHelpers'

interface LoadOptions {
  silent?: boolean
}

function hasCustomerDisplayNameField(value: unknown): boolean {
  return Boolean(value && typeof value === 'object' && Object.prototype.hasOwnProperty.call(value, 'customer_display_name'))
}

function exchangeRateNotice(data: ExchangeRatesResponse): string {
  if (!data.stale) {
    return ''
  }
  const source = String(data.source || '').toLowerCase()
  if (source.includes('fallback')) {
    return '汇率接口暂不可用，已使用兜底汇率'
  }
  return '汇率接口暂不可用，已使用缓存汇率'
}

const AgentDetailPanel = lazy(() => import('./components/AgentDetailPanel').then((module) => ({ default: module.AgentDetailPanel })))
const ConsoleModals = lazy(() => import('./components/ConsoleModals').then((module) => ({ default: module.ConsoleModals })))
const CustomerManagementModal = lazy(() => import('./components/CustomerManagementModal').then((module) => ({ default: module.CustomerManagementModal })))
const FrontProxyManagementPage = lazy(() => import('./components/FrontProxyManagementPage').then((module) => ({ default: module.FrontProxyManagementPage })))
const CustomerPortal = lazy(() => import('./components/CustomerPortal').then((module) => ({ default: module.CustomerPortal })))
const AdminSupportPage = lazy(() => import('./components/AdminSupportPage').then((module) => ({ default: module.AdminSupportPage })))
const PublicSite = lazy(() => import('./components/PublicSite').then((module) => ({ default: module.PublicSite })))

export default function App() {
  const { message } = AntdApp.useApp()
  const { mode: themeMode, effectiveMode, setMode: setThemeMode } = useAppTheme()
  const {
    accountForm,
    accountModalOpen,
    accountSaving,
    adminUser,
    loginForm,
    loginLoading,
    sessionLoading,
    systemInfo,
    loadSession,
    login,
    logoutSession,
    saveAccount,
    setAccountForm,
    setAccountModalOpen,
    setAdminUser,
    setLoginForm,
    setSessionLoading,
  } = useAdminSession()
  const [agents, setAgents] = useState<DashboardAgentView[]>([])
  const [agentsLoading, setAgentsLoading] = useState(false)
  const [agentsError, setAgentsError] = useState('')
  const [dashboardView, setDashboardView] = useState<GlobalDashboardView | null>(null)
  const [financeCustomers, setFinanceCustomers] = useState<CustomerAdminView[]>([])
  const [financeAreaManagers, setFinanceAreaManagers] = useState<AreaManagerAdminView[]>([])
  const [financeAccountsLoaded, setFinanceAccountsLoaded] = useState(false)
  const [financeAccountsError, setFinanceAccountsError] = useState('')
  const [costCurrency, setCostCurrency] = useState<CurrencyCode>(() => readStoredCostCurrency())
  const [exchangeRates, setExchangeRates] = useState<ExchangeRatesState>(() => defaultExchangeRatesState())
  const [currencyOptions, setCurrencyOptions] = useState<CurrencyCode[]>(() => [...COMMON_COST_CURRENCIES])
  const [selectedTag, setSelectedTag] = useState('')
  const [agentHealthFilter, setAgentHealthFilter] = useState<AgentHealthFilter>('all')
  const [topologySearch, setTopologySearch] = useState('')
  const [activeAdminPage, setActiveAdminPage] = useState<AdminPageKey>('dashboard')
  const [supportUnreadCount, setSupportUnreadCount] = useState(0)
  const [agentViewMode, setAgentViewMode] = useState<AgentViewMode>('card')
  const [selectedAgentId, setSelectedAgentId] = useState('')
  const [overview, setOverview] = useState<XUIOverview | null>(null)
  const [overviewLoading, setOverviewLoading] = useState(false)
  const [overviewError, setOverviewError] = useState('')
  const [managedConfig, setManagedConfig] = useState<ManagedAgentConfig | null>(null)
  const [savedManagedConfig, setSavedManagedConfig] = useState<ManagedAgentConfig | null>(null)
  const managedConfigDirtyRef = useRef(false)
  const customerDisplayNameCacheRef = useRef<Record<string, string>>({})
  const [tagOptions, setTagOptions] = useState<string[]>([])
  const [newTagName, setNewTagName] = useState('')
  const [tagSaving, setTagSaving] = useState(false)
  const [entryAddressInputText, setEntryAddressInputText] = useState('')
  const [configLoading, setConfigLoading] = useState(false)
  const [configSavingSection, setConfigSavingSection] = useState<ConfigSectionKey | null>(null)
  const [clientBillingSavingKey, setClientBillingSavingKey] = useState('')
  const [configError, setConfigError] = useState('')
  const [configAudits, setConfigAudits] = useState<ConfigAuditLog[]>([])
  const [configAuditsLoading, setConfigAuditsLoading] = useState(false)
  const [customerModalOpen, setCustomerModalOpen] = useState(false)
  const [customerAssignmentDraft, setCustomerAssignmentDraft] = useState<CustomerAssignmentDraft | null>(null)
  const [reloadToken, setReloadToken] = useState(0)
  const [activeTabKey, setActiveTabKey] = useState('overview')
  const [topologyVisible, setTopologyVisibleState] = useState(false)
  const [topologyLoading, setTopologyLoading] = useState(false)
  const [topologyError, setTopologyError] = useState('')
  const [topologyLoaded, setTopologyLoaded] = useState(false)
  const topologyVisibleRef = useRef(false)
  const topologyLoadedRef = useRef(false)
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
  const [xuiActions, setXUIActions] = useState<XUIAction[]>([])
  const [xuiActionsLoading, setXUIActionsLoading] = useState(false)
  const [agentLogs, setAgentLogs] = useState<AgentLogsResponse | null>(null)
  const [agentLogsLoading, setAgentLogsLoading] = useState(false)
  const [agentLogsError, setAgentLogsError] = useState('')
  const [agentRefreshLoading, setAgentRefreshLoading] = useState(false)
  const [agentDeleteLoading, setAgentDeleteLoading] = useState(false)
  const [agentReplaceLoading, setAgentReplaceLoading] = useState(false)
  const [realmCopyLoading, setRealmCopyLoading] = useState(false)
  const [xuiActionModalOpen, setXUIActionModalOpen] = useState(false)
  const [xuiActionSaving, setXUIActionSaving] = useState(false)
  const [xuiActionKind, setXUIActionKind] = useState('upsert_routing_rule')
  const [xuiActionAgentId, setXUIActionAgentId] = useState('')
  const [addClientActionInbounds, setAddClientActionInbounds] = useState<XUINodeView[]>([])
  const [addClientActionForm, setAddClientActionForm] = useState<XUIAddClientActionForm>(() => defaultAddClientActionForm())
  const [outboundActionForm, setOutboundActionForm] = useState<XUIOutboundActionForm>(() => defaultOutboundActionForm())
  const [routingActionForm, setRoutingActionForm] = useState<XUIRoutingActionForm>(() => defaultRoutingActionForm())
  const [outboundSourceOverview, setOutboundSourceOverview] = useState<XUIOverview | null>(null)
  const [outboundSourceLoading, setOutboundSourceLoading] = useState(false)
  const [clientSearch, setClientSearch] = useState('')
  const [importURLClient, setImportURLClient] = useState<XUIClientView | null>(null)
  const deferredClientSearch = useDeferredValue(clientSearch.trim().toLowerCase())
  const inFlightRequestsRef = useRef<Set<string>>(new Set())
  const lastDetailAgentIdRef = useRef('')
  const isCurrentAgentRequest = useCurrentAgentRequest(selectedAgentId)
  const {
    accessLogFilters,
    accessLogs,
    accessLogsLoading,
    accessLogsTotal,
    clientInstallCommand,
    clientInstallCommandKind,
    clientInstallForm,
    clientInstallLoading,
    clientInstallModalOpen,
    clientInstallSaving,
    clientOpenWrtCommand,
    clientWindowsCMDCommand,
    clientWindowsPowerShellCommand,
    editingTelegramBotId,
    frontendSettingsForm,
    frontendSettingsLoading,
    frontendSettingsModalOpen,
    frontendSettingsSaving,
    scheduledTasks,
    scheduledTasksLoading,
    scheduledTasksSaving,
    telegramBotForm,
    telegramBotModalOpen,
    telegramBotSaving,
    telegramBots,
    telegramBotsLoading,
    updateLatestError,
    updateLatestInfo,
    updateLatestLoading,
    updateLoading,
    updateModalOpen,
    copyClientInstallCommand,
    deleteTelegramBot,
    loadAccessLogs,
    loadScheduledTasks,
    loadTelegramBots,
    loadUpdateLatestInfo,
    openClientInstallModal,
    openFrontendSettingsModal,
    saveClientInstallSettings,
    saveFrontendSettings,
    saveScheduledTasks,
    saveTelegramBot,
    setAccessLogFilters,
    setClientInstallCommandKind,
    setClientInstallForm,
    setClientInstallModalOpen,
    setEditingTelegramBotId,
    setFrontendSettingsForm,
    setFrontendSettingsModalOpen,
    setScheduledTasks,
    setTelegramBotForm,
    setTelegramBotModalOpen,
    setTelegramBots,
    setUpdateModalOpen,
    testTelegramBot,
    updateAllClientsOnline,
    updateServerOnline,
  } = useAdminSystemTools(setAdminUser)
  const {
    remoteCommandLoading,
    xuiClientDeleteLoadingKey,
    xuiClientToggleLoadingKey,
    xuiClientTrafficSavingKey,
    xuiRestartLoading,
    xuiUpdateLoading,
    deleteXUIClient,
    executeRemoteCommand,
    restartXUIService,
    saveXUIClientTrafficLimit,
    setXUIClientEnabled,
    update3XUI,
  } = useXUIRemoteActions({
    selectedAgentId,
    setAdminUser,
    loadAgents,
    loadAgentLogs,
    loadOverview,
    loadXUIActions,
    scheduleXUIActionResultRefresh,
  })

  const selectedAgent = agents.find((item) => item.agent_id === selectedAgentId)
  const currentOverview = overview?.agent_id === selectedAgentId ? overview : null
  const selectedSummary = currentOverview?.summary || selectedAgent?.summary || {}
  const centerPanelOpen = topologyVisible || Boolean(selectedAgent)
  const {
    applyAdminRoute,
    jumpToNode,
    jumpToOutbound,
    jumpToRule,
    openAgentDetailPanel,
    openAgentHealthFilter,
    openCustomerAssignment,
    openCustomerAuthorization,
    openTopologyPanel,
    returnHome,
    selectDashboardTag,
    selectTopologyAgent,
  } = createAppNavigationHandlers({
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
    runTransition: startTransition,
    setCustomerModalOpen,
    setCustomerAssignmentDraft,
  })
  const topologyScopeLabel = selectedAgentId ? selectedAgent?.agent_name || selectedAgentId : selectedTag ? `${selectedTag} 标签` : '全部 Client'
  const heroTitle = 'ZaneLin'
  const normalizedPath = window.location.pathname.replace(/\/+$/, '') || '/'
  const customerMode = normalizedPath === '/customer'
  const publicSiteMode = normalizedPath === '/site' || normalizedPath === '/official' || new URLSearchParams(window.location.search).get('page') === 'site'
  const isAreaManagerAccount = isAreaManagerAdminUser(adminUser)
  const canManageSystem = Boolean(adminUser && !isAreaManagerAccount)
  useAdminRouteSync({
    enabled: Boolean(!customerMode && !publicSiteMode && !sessionLoading && adminUser),
    sessionIdentity: adminUser,
    canManageSystem,
    canViewFrontProxies: canManageSystem || isAreaManagerAccount,
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
  })
  useEffect(() => {
    if (customerMode || publicSiteMode) {
      setSessionLoading(false)
      return
    }
    void loadSession()
  }, [customerMode, publicSiteMode])

  useEffect(() => {
    if (!adminUser) {
      setAgentViewMode('card')
      return
    }
    setAgentViewMode(readStoredAgentViewMode(adminUser.username))
  }, [adminUser?.username])

  useEffect(() => {
    setFinanceAccountsLoaded(false)
    setFinanceAccountsError('')
    if (!adminUser) {
      setFinanceCustomers([])
      setFinanceAreaManagers([])
    }
  }, [adminUser?.id, adminUser?.username])

  useEffect(() => {
    if (adminUser) {
      void loadAgents()
      if (canManageSystem) {
        void loadTelegramBots()
        void loadTagSettings()
        void loadExchangeRates()
        void loadFinanceAccounts()
      }
    }
  }, [adminUser, canManageSystem])

  useEffect(() => {
    if (!adminUser) {
      setSupportUnreadCount(0)
      return
    }
    let cancelled = false
    const loadUnread = async () => {
      try {
        const data = await fetchJSON<SupportUnreadResponse>('/api/v1/admin/support/unread')
        if (!cancelled) setSupportUnreadCount(data.unread_count || 0)
      } catch {
        // The session loader handles authentication failures; the badge can stay stale briefly.
      }
    }
    let timer: number | undefined
    const poll = async () => {
      await loadUnread()
      if (!cancelled) timer = window.setTimeout(() => void poll(), 3000)
    }
    void poll()
    return () => {
      cancelled = true
      if (timer !== undefined) window.clearTimeout(timer)
    }
  }, [adminUser?.id, adminUser?.role, adminUser?.username])

  useEffect(() => {
    if (adminUser && updateModalOpen) {
      void loadUpdateLatestInfo()
    }
  }, [adminUser, updateModalOpen])

  useEffect(() => {
    if (adminUser && canManageSystem) {
      void loadUpdateLatestInfo({ silent: true })
    }
  }, [adminUser, canManageSystem])

  useEffect(() => {
    if (!adminUser || canManageSystem) {
      return
    }
    if (activeAdminPage === 'settings' || activeAdminPage === 'schedules' || activeAdminPage === 'access-logs') {
      setActiveAdminPage('dashboard')
    }
    if (['config', 'logs', 'certificates'].includes(activeTabKey)) {
      setActiveTabKey('overview')
    }
  }, [activeAdminPage, activeTabKey, adminUser, canManageSystem])

  useEffect(() => {
    if (adminUser && canManageSystem && activeAdminPage === 'schedules') {
      void loadScheduledTasks()
    }
  }, [activeAdminPage, adminUser, canManageSystem])

  useEffect(() => {
    if (adminUser && canManageSystem && activeAdminPage === 'access-logs') {
      void loadAccessLogs()
    }
  }, [activeAdminPage, adminUser, canManageSystem])

  useEffect(() => {
    try {
      window.localStorage.setItem(COST_CURRENCY_STORAGE_KEY, costCurrency)
    } catch {
      // Ignore storage errors; the selector still works for the current session.
    }
  }, [costCurrency])

  useEffect(() => {
    if (!adminUser) {
      return
    }
    let closed = false
    let socket: WebSocket | null = null
    let reconnectTimer: number | undefined

    const connect = () => {
      if (closed) {
        return
      }
      socket = new WebSocket(buildDashboardRealtimeURL())
      socket.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data) as DashboardRealtimeMessage
          const metrics = data.type === 'snapshot' ? data.metrics || [] : data.metric ? [data.metric] : []
          if (metrics.length) {
            applyRealtimeMetrics(metrics)
          }
        } catch {
          // Ignore malformed realtime frames; the polling path remains the fallback.
        }
      }
      socket.onerror = () => {
        socket?.close()
      }
      socket.onclose = () => {
        if (!closed) {
          reconnectTimer = window.setTimeout(connect, 3000)
        }
      }
    }

    connect()
    return () => {
      closed = true
      if (reconnectTimer !== undefined) {
        window.clearTimeout(reconnectTimer)
      }
      socket?.close()
    }
  }, [adminUser])

  useEffect(() => {
    if (!adminUser) {
      return
    }
    if (!selectedAgentId) {
      lastDetailAgentIdRef.current = ''
      setActiveTabKey((current) => (current === 'config' ? 'overview' : current))
      setOverview(null)
      setManagedConfig(null)
      setSavedManagedConfig(null)
      managedConfigDirtyRef.current = false
      setEntryAddressInputText('')
      setOverviewError('')
      setConfigError('')
      setXUIActions([])
      setAgentLogs(null)
      setAgentLogsError('')
      setConfigAudits([])
      return
    }

    if (lastDetailAgentIdRef.current !== selectedAgentId) {
      lastDetailAgentIdRef.current = selectedAgentId
      setOverview(null)
      setManagedConfig(null)
      setSavedManagedConfig(null)
      managedConfigDirtyRef.current = false
      setEntryAddressInputText('')
      setOverviewError('')
      setConfigError('')
      setXUIActions([])
      setAgentLogs(null)
      setAgentLogsError('')
      setConfigAudits([])
      setClientSearch('')
      setSelectedOutboundTag('')
      setSelectedRuleIndex(null)
      setSelectedNodeAnchor('')
    }

    void loadOverview(selectedAgentId)
    void loadManagedConfig(selectedAgentId)
    void loadXUIActions(selectedAgentId)
    void loadAgentLogs(selectedAgentId)
    void loadConfigAudits(selectedAgentId)
  }, [selectedAgentId, reloadToken, adminUser])

  useEffect(() => {
    if (!adminUser || !outboundActionForm.source_agent_id) {
      setOutboundSourceOverview(null)
      setOutboundSourceLoading(false)
      return
    }
    if (currentOverview && outboundActionForm.source_agent_id === currentOverview.agent_id) {
      setOutboundSourceOverview(currentOverview)
      setOutboundSourceLoading(false)
      return
    }

    let cancelled = false
    setOutboundSourceLoading(true)
    void fetchJSON<XUIOverview>(`/api/v1/agents/${outboundActionForm.source_agent_id}/xui/overview`)
      .then((data) => {
        if (!cancelled) {
          setOutboundSourceOverview(normalizeXUIOverview(data))
        }
      })
      .catch(() => {
        if (!cancelled) {
          setOutboundSourceOverview(null)
        }
      })
      .finally(() => {
        if (!cancelled) {
          setOutboundSourceLoading(false)
        }
      })
    return () => {
      cancelled = true
    }
  }, [adminUser, outboundActionForm.source_agent_id, currentOverview])

  useEffect(() => {
    if (!agents.length || (selectedAgentId && !agents.some((item) => item.agent_id === selectedAgentId))) {
      setSelectedAgentId('')
    }
  }, [agents, selectedAgentId])

  function toggleAgentViewMode() {
    setAgentViewMode((current) => {
      const next: AgentViewMode = current === 'list' ? 'card' : 'list'
      if (adminUser) {
        storeAgentViewMode(adminUser.username, next)
      }
      return next
    })
  }

  async function logout() {
    await logoutSession()
    setDashboardView(null)
    setSelectedTag('')
    setAgentHealthFilter('all')
    setAgents([])
    setFinanceCustomers([])
    setFinanceAreaManagers([])
    setFinanceAccountsLoaded(false)
    setFinanceAccountsError('')
    setSelectedAgentId('')
    setOverview(null)
    setManagedConfig(null)
    setTelegramBots([])
    setAgentLogs(null)
    setConfigAudits([])
  }

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
      setTagOptions((current) => mergeTagOptions(current, sortedAgents.flatMap((agent) => agent.tags || [])))
      setAgentsError('')
      if (!sortedAgents.length || (selectedAgentId && !sortedAgents.some((item) => item.agent_id === selectedAgentId))) {
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
      setTagOptions((current) => mergeTagOptions(current, sortedAgents.flatMap((agent) => agent.tags || [])))
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

  async function loadExchangeRates() {
    setExchangeRates((current) => ({ ...current, loading: true, error: '' }))
    try {
      const data = await fetchJSON<ExchangeRatesResponse>('/api/v1/exchange-rates')
      const rates: Record<CurrencyCode, number> = { EUR: 1 }
      for (const [rawCurrency, rawRate] of Object.entries(data.rates || {})) {
        const currency = normalizeCurrencyCode(rawCurrency)
        const value = Number(rawRate)
        if (Number.isFinite(value) && value > 0) {
          rates[currency] = value
        }
      }
      setCurrencyOptions(mergeCurrencyOptions(Object.keys(rates)))
      setExchangeRates({
        base: normalizeCurrencyCode(data.base || 'EUR'),
        date: data.date || '',
        rates,
        loading: false,
        error: exchangeRateNotice(data),
      })
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      setExchangeRates((current) => ({
        ...current,
        loading: false,
        error: '汇率接口暂不可用，已保留当前汇率',
      }))
    }
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

  async function loadFinanceAccounts() {
    try {
      const [customers, areaManagers] = await Promise.all([
        fetchJSON<CustomerAdminView[]>('/api/v1/admin/customers'),
        fetchJSON<AreaManagerAdminView[]>('/api/v1/admin/area-managers'),
      ])
      setFinanceCustomers(Array.isArray(customers) ? customers : [])
      setFinanceAreaManagers(Array.isArray(areaManagers) ? areaManagers : [])
      setFinanceAccountsLoaded(true)
      setFinanceAccountsError('')
    } catch (error) {
      setFinanceAccountsError(error instanceof Error ? error.message : '财务账号数据加载失败')
    }
  }

  async function loadOverview(agentID: string, options: LoadOptions = {}) {
    const requestKey = `overview:${agentID}`
    if (inFlightRequestsRef.current.has(requestKey)) {
      return
    }
    inFlightRequestsRef.current.add(requestKey)
    const silent = Boolean(options.silent)
    if (!silent) {
      setOverviewLoading(true)
      setOverviewError('')
    }
    try {
      const data = await fetchJSON<XUIOverview>(`/api/v1/agents/${agentID}/xui/overview`)
      if (!isCurrentAgentRequest(agentID)) {
        return
      }
      setOverview(normalizeXUIOverview(data))
      setOverviewError('')
      setSelectedOutboundTag('')
      setSelectedRuleIndex(null)
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      } else if (!silent && isCurrentAgentRequest(agentID)) {
        setOverview(null)
        setOverviewError(error instanceof Error ? error.message : '加载 x-ui 概览失败')
      }
    } finally {
      if (!silent && isCurrentAgentRequest(agentID)) {
        setOverviewLoading(false)
      }
      inFlightRequestsRef.current.delete(requestKey)
    }
  }

  async function loadManagedConfig(agentID: string, options: LoadOptions = {}) {
    const requestKey = `config:${agentID}`
    if (inFlightRequestsRef.current.has(requestKey)) {
      return
    }
    inFlightRequestsRef.current.add(requestKey)
    const silent = Boolean(options.silent)
    if (!silent) {
      setConfigLoading(true)
      setConfigError('')
    }
    try {
      const data = await fetchJSON<ManagedAgentConfig>(`/api/v1/agents/${agentID}/config`)
      if (!isCurrentAgentRequest(agentID)) {
        return
      }
      const cachedName = cachedCustomerDisplayName(agentID)
      const dataWithCustomerName = !hasCustomerDisplayNameField(data) && cachedName !== undefined
        ? { ...data, customer_display_name: cachedName }
        : data
      const agentName = agents.find((item) => item.agent_id === agentID)?.agent_name
      const normalized = normalizeManagedConfig(dataWithCustomerName, agentID, agentName)
      rememberCustomerDisplayName(agentID, normalized.customer_display_name || '')
      setSavedManagedConfig(normalized)
      if (silent && managedConfigDirtyRef.current) {
        setConfigError('')
        return
      }
      managedConfigDirtyRef.current = false
      setManagedConfig(normalized)
      setEntryAddressInputText(formatAddressInput(normalized.entry?.addresses))
      setConfigError('')
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      } else if (!silent && isCurrentAgentRequest(agentID)) {
        const agentName = agents.find((item) => item.agent_id === agentID)?.agent_name
        const emptyConfig = createEmptyManagedConfig(agentID, agentName)
        managedConfigDirtyRef.current = false
        setSavedManagedConfig(emptyConfig)
        setManagedConfig(emptyConfig)
        setEntryAddressInputText(formatAddressInput(emptyConfig.entry?.addresses))
        setConfigError(error instanceof Error ? error.message : '加载托管配置失败')
      }
    } finally {
      if (!silent && isCurrentAgentRequest(agentID)) {
        setConfigLoading(false)
      }
      inFlightRequestsRef.current.delete(requestKey)
    }
  }

  async function loadXUIActions(agentID = selectedAgentId, options: LoadOptions = {}) {
    if (!agentID) {
      setXUIActions([])
      return
    }
    const requestKey = `actions:${agentID}`
    if (inFlightRequestsRef.current.has(requestKey)) {
      return
    }
    inFlightRequestsRef.current.add(requestKey)
    const silent = Boolean(options.silent)
    if (!silent) {
      setXUIActionsLoading(true)
    }
    try {
      const data = await fetchJSON<XUIAction[]>(`/api/v1/agents/${agentID}/xui/actions?limit=30`)
      if (!isCurrentAgentRequest(agentID)) {
        return
      }
      setXUIActions(Array.isArray(data) ? data : [])
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      } else if (!silent && isCurrentAgentRequest(agentID)) {
        message.error(error instanceof Error ? error.message : '加载 x-ui 操作记录失败')
      }
    } finally {
      if (!silent && isCurrentAgentRequest(agentID)) {
        setXUIActionsLoading(false)
      }
      inFlightRequestsRef.current.delete(requestKey)
    }
  }

  async function loadAgentLogs(agentID = selectedAgentId, options: LoadOptions = {}) {
    if (!agentID) {
      setAgentLogs(null)
      setAgentLogsError('')
      return
    }
    const requestKey = `logs:${agentID}`
    if (inFlightRequestsRef.current.has(requestKey)) {
      return
    }
    inFlightRequestsRef.current.add(requestKey)
    const silent = Boolean(options.silent)
    if (!silent) {
      setAgentLogsLoading(true)
      setAgentLogsError('')
    }
    try {
      const data = await fetchJSON<AgentLogsResponse>(`/api/v1/agents/${agentID}/logs`)
      if (!isCurrentAgentRequest(agentID)) {
        return
      }
      setAgentLogs({ ...data, logs: Array.isArray(data.logs) ? data.logs : [] })
      setAgentLogsError('')
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      } else if (!silent && isCurrentAgentRequest(agentID)) {
        setAgentLogs(null)
        setAgentLogsError(error instanceof Error ? error.message : '加载日志失败')
      }
    } finally {
      if (!silent && isCurrentAgentRequest(agentID)) {
        setAgentLogsLoading(false)
      }
      inFlightRequestsRef.current.delete(requestKey)
    }
  }

  async function requestAgentSnapshot(agentID = selectedAgentId) {
    if (!agentID) {
      return
    }
    setAgentRefreshLoading(true)
    try {
      const data = await fetchJSON<AgentRefreshResponse>(`/api/v1/agents/${agentID}/refresh`, {
        method: 'POST',
      })
      message.success(data.message || '已通知 Client 立即采集')
      window.setTimeout(() => {
        void loadAgents({ silent: true })
        void loadOverview(agentID, { silent: true })
        void loadManagedConfig(agentID, { silent: true })
        void loadXUIActions(agentID, { silent: true })
        void loadAgentLogs(agentID, { silent: true })
      }, 2500)
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      message.error(error instanceof Error ? error.message : '通知 Client 采集失败')
    } finally {
      setAgentRefreshLoading(false)
    }
  }

  async function deleteAgent(agentID = selectedAgentId) {
    if (!agentID) {
      return
    }
    const targetName = agents.find((item) => item.agent_id === agentID)?.agent_name || agentID
    setAgentDeleteLoading(true)
    try {
      const result = await fetchJSON<{ status: string; agent_id: string; client_stop_sent?: boolean }>(`/api/v1/agents/${encodeURIComponent(agentID)}`, {
        method: 'DELETE',
      })
      customerDisplayNameCacheRef.current = Object.fromEntries(
        Object.entries(customerDisplayNameCacheRef.current).filter(([key]) => key !== agentID),
      )
      if (selectedAgentId === agentID) {
        returnHome()
      }
      message.success(result.client_stop_sent
        ? `已删除 Client / VPS：${targetName}，并已通知远端停止 Client 服务`
        : `已删除 Client / VPS：${targetName}；远端不在线，未能即时停止 Client 服务`)
      await loadAgents({ silent: true })
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      message.error(error instanceof Error ? error.message : '删除 Client / VPS 失败')
    } finally {
      setAgentDeleteLoading(false)
    }
  }

  async function replaceAgent(replacementAgentID: string, sourceAgentID = selectedAgentId): Promise<boolean> {
    if (!sourceAgentID || !replacementAgentID || sourceAgentID === replacementAgentID) {
      return false
    }
    const sourceName = agents.find((item) => item.agent_id === sourceAgentID)?.agent_name || sourceAgentID
    const replacementName = agents.find((item) => item.agent_id === replacementAgentID)?.agent_name || replacementAgentID
    setAgentReplaceLoading(true)
    try {
      const result = await fetchJSON<AgentReplacementResult>(`/api/v1/agents/${encodeURIComponent(sourceAgentID)}/replace`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ replacement_agent_id: replacementAgentID }),
      })
      setSelectedAgentId(replacementAgentID)
      setReloadToken((current) => current + 1)
      await loadAgents({ silent: true })
      if (topologyLoaded || topologyVisible) {
        await loadTopology({ silent: true })
      }
      const permissionCount = result.area_manager_agents_migrated
        + result.area_assignments_migrated
        + result.customer_assignments_migrated
        + result.outbound_grants_migrated
      const forwardingCount = result.realm_references_updated + result.haproxy_references_updated
      message.success(`已将 ${sourceName} 替换为 ${replacementName}：迁移 ${permissionCount} 条授权，更新 ${forwardingCount} 条转发引用；旧 Client 已保留`)
      return true
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      message.error(error instanceof Error ? error.message : '替换 Client 失败')
      return false
    } finally {
      setAgentReplaceLoading(false)
    }
  }

  async function copyRealmConfigToAgent(targetAgentID: string, sourceAgentID = selectedAgentId) {
    if (!sourceAgentID || !targetAgentID) {
      return
    }
    setRealmCopyLoading(true)
    setConfigError('')
    try {
      const result = await fetchJSON<{ apply_sent?: boolean }>(`/api/v1/agents/${sourceAgentID}/realm/copy`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ target_agent_id: targetAgentID }),
      })
      message.success(result.apply_sent ? 'Realm 配置已复制到目标 Client，并已通过 WS 通知立即生效' : 'Realm 配置已复制到目标 Client；目标 Client 不在线时会在下次轮询后生效')
      await loadAgents()
      if (targetAgentID === selectedAgentId) {
        await loadManagedConfig(targetAgentID, { silent: true })
        await loadConfigAudits(targetAgentID, { silent: true })
      }
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      const detail = error instanceof Error ? error.message : '复制 Realm 配置失败'
      setConfigError(detail)
      message.error(detail)
    } finally {
      setRealmCopyLoading(false)
    }
  }

  async function loadConfigAudits(agentID = selectedAgentId, options: LoadOptions = {}) {
    if (!agentID) {
      setConfigAudits([])
      return
    }
    const silent = Boolean(options.silent)
    if (!silent) {
      setConfigAuditsLoading(true)
    }
    try {
      const data = await fetchJSON<ConfigAuditLog[]>(`/api/v1/admin/audit?agent_id=${encodeURIComponent(agentID)}&limit=8`)
      if (!isCurrentAgentRequest(agentID)) {
        return
      }
      setConfigAudits(Array.isArray(data) ? data : [])
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      } else if (!silent && isCurrentAgentRequest(agentID)) {
        message.error(error instanceof Error ? error.message : '加载配置修改记录失败')
      }
    } finally {
      if (!silent && isCurrentAgentRequest(agentID)) {
        setConfigAuditsLoading(false)
      }
    }
  }

  async function loadTagSettings() {
    try {
      const data = await fetchJSON<TagSettingsResponse>('/api/v1/admin/tags')
      setTagOptions((current) => mergeTagOptions(current, data.tags || []))
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
    }
  }

  async function saveTagOptions(nextTags: string[]) {
    setTagSaving(true)
    try {
      const normalized = mergeTagOptions([], nextTags)
      const data = await fetchJSON<TagSettingsResponse>('/api/v1/admin/tags', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ tags: normalized }),
      })
      setTagOptions(mergeTagOptions(normalized, data.tags || []))
      return true
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      message.error(error instanceof Error ? error.message : '保存标签失败')
      return false
    } finally {
      setTagSaving(false)
    }
  }

  async function saveAreaAgentTags(nextTags = managedConfig?.tags || []) {
    if (!selectedAgentId) {
      return false
    }
    setTagSaving(true)
    try {
      const normalized = mergeTagOptions([], nextTags)
      const data = await fetchJSON<AreaAgentTagsResponse>(`/api/v1/admin/area-agent-tags/${encodeURIComponent(selectedAgentId)}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ tags: normalized }),
      })
      const savedTags = mergeTagOptions([], data.tags || [])
      setTagOptions((current) => mergeTagOptions(current, savedTags))
      setManagedConfig((current) => current ? { ...current, tags: savedTags } : current)
      setSavedManagedConfig((current) => current ? { ...current, tags: savedTags } : current)
      managedConfigDirtyRef.current = false
      message.success('区域账号标签已保存')
      await loadAgents({ silent: true })
      return true
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      message.error(error instanceof Error ? error.message : '保存区域标签失败')
      return false
    } finally {
      setTagSaving(false)
    }
  }

  async function createTagOption() {
    const parsed = parseTagInput(newTagName)
    if (!parsed.length) {
      message.warning('请输入标签名称')
      return
    }
    if (!canManageSystem) {
      const nextTags = mergeTagOptions(tagOptions, parsed)
      setTagOptions(nextTags)
      updateManagedConfig((current) => ({ ...current, tags: mergeTagOptions(current.tags || [], parsed) }))
      setNewTagName('')
      message.success('标签已加入当前区域草稿，请保存后生效')
      return
    }
    const nextTags = mergeTagOptions(tagOptions, parsed)
    const ok = await saveTagOptions(nextTags)
    if (ok) {
      setNewTagName('')
      message.success('标签已创建')
    }
  }

  async function createXUIAction() {
    const actionAgentID = xuiActionAgentId || selectedAgentId
    if (!actionAgentID) {
      return
    }
    let payload: Record<string, unknown>
    try {
      payload = buildXUIActionPayload(xuiActionKind, {
        addClient: addClientActionForm,
        outbound: outboundActionForm,
        routing: routingActionForm,
      })
    } catch (error) {
      message.error(error instanceof Error ? error.message : '操作表单不完整')
      return
    }
    setXUIActionSaving(true)
    try {
      const action = await fetchJSON<XUIAction>(`/api/v1/agents/${actionAgentID}/xui/actions`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ kind: xuiActionKind, payload }),
      })
      setXUIActionModalOpen(false)
      message.success(action.status === 'running' ? 'x-ui 操作已通过 WS 下发，正在等待实时回传结果' : 'x-ui 操作已创建，Client 不在线时会等待轮询执行')
      await loadXUIActions(actionAgentID)
      scheduleXUIActionResultRefresh(actionAgentID)
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      message.error(error instanceof Error ? error.message : '创建 x-ui 操作失败')
    } finally {
      setXUIActionSaving(false)
    }
  }

  function scheduleXUIActionResultRefresh(agentID: string) {
    ;[500, 1000, 2000, 4000, 8000, 15000].forEach((delay) => {
      window.setTimeout(() => {
        void loadXUIActions(agentID, { silent: true })
      }, delay)
    })
  }

  async function copyImportURL(client: XUIClientView) {
    if (!client.import_url) {
      message.warning('当前客户端暂不支持生成单节点导入 URL')
      return
    }
    try {
      await navigator.clipboard.writeText(client.import_url)
      message.success('导入 URL 已复制')
    } catch {
      setImportURLClient(client)
      message.warning('浏览器不允许直接复制，已打开 URL 弹窗')
    }
  }

  async function refreshAfterExternalConfigChange(agentID?: string) {
    await loadAgents()
    if (canManageSystem) {
      await loadFinanceAccounts()
    }
    if (agentID && agentID === selectedAgentId) {
      await loadManagedConfig(agentID, { silent: true })
    }
  }

  async function saveManagedConfigSection(section: ConfigSectionKey, draftOverride?: ManagedAgentConfig) {
    const draftConfig = draftOverride || managedConfig
    if (!selectedAgentId || !draftConfig) {
      return
    }
    const baseConfig = savedManagedConfig || createEmptyManagedConfig(selectedAgentId, selectedAgent?.agent_name)
    const payload = buildSectionSavePayload(baseConfig, draftConfig, section, selectedAgentId)
    setConfigSavingSection(section)
    setConfigError('')
    try {
      const saved = await fetchJSON<ManagedAgentConfig>(`/api/v1/agents/${selectedAgentId}/config`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      })
      const savedMissingCustomerDisplayName = section === 'client' && !hasCustomerDisplayNameField(saved)
      const savedWithSubmittedCustomerName = savedMissingCustomerDisplayName
        ? { ...saved, customer_display_name: payload.customer_display_name || '' }
        : saved
      const normalized = normalizeManagedConfig(savedWithSubmittedCustomerName, selectedAgentId, saved.agent_name || selectedAgent?.agent_name)
      rememberCustomerDisplayName(selectedAgentId, normalized.customer_display_name || '')
      setSavedManagedConfig(normalized)
      const nextDraft = mergeSavedSectionIntoDraft(draftConfig, normalized, section)
      managedConfigDirtyRef.current = configSignature(nextDraft) !== configSignature(normalized)
      setManagedConfig(nextDraft)
      if (section === 'client') {
        setTagOptions((current) => mergeTagOptions(current, normalized.tags || []))
      }
      if (section === 'entry') {
        setEntryAddressInputText(formatAddressInput(normalized.entry?.addresses))
      }
      if (savedMissingCustomerDisplayName && payload.customer_display_name) {
        message.warning('用户展示名称已保留在当前页面；用户侧显示需要后端服务更新到新版后生效')
      } else {
        message.success(`${configSectionLabel(section)}已保存，已通过 WS 通知 Client 立即生效；离线时下次轮询生效`)
      }
      await loadAgents()
      await loadConfigAudits(selectedAgentId, { silent: true })
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      const detail = error instanceof Error ? error.message : '保存配置失败'
      setConfigError(detail)
      message.error(detail)
    } finally {
      setConfigSavingSection(null)
    }
  }

  function savePrimaryDomain(value: string) {
    if (!selectedAgentId) {
      return
    }
    const draftConfig = managedConfig || savedManagedConfig || createEmptyManagedConfig(selectedAgentId, selectedAgent?.agent_name)
    const nextDraft = normalizeManagedConfig({
      ...draftConfig,
      entry: {
        ...draftConfig.entry,
        import_domain: value,
      },
    }, selectedAgentId, selectedAgent?.agent_name)
    managedConfigDirtyRef.current = true
    setManagedConfig(nextDraft)
    void saveManagedConfigSection('entry', nextDraft)
  }

  async function saveClientBilling(record: XUIClientView) {
    if (!selectedAgentId || !managedConfig) {
      return
    }
    const savingKey = billingKeyForClient(record)
    const baseConfig = savedManagedConfig || createEmptyManagedConfig(selectedAgentId, selectedAgent?.agent_name)
    const billing = findClientBilling(managedConfig.renewal?.client_billings, record) || defaultClientBilling(record)
    const expiryTime = effectiveClientBillingExpiryTime(billing, record.expiry_time || 0)
    const accountBasedProxy = ['http', 'socks', 'socks5'].includes((record.protocol || '').toLowerCase())
    const shouldSyncExpiry = !accountBasedProxy && expiryTime > 0 && expiryTime !== Math.max(0, Number(record.expiry_time || 0))
    const targetAgentID = record.realm_target_agent_id || selectedAgentId
    const targetInboundID = record.realm_target_inbound_id || record.inbound_id
    const targetInboundTag = record.realm_target_inbound_tag || record.inbound_tag || ''
    const nextConfig: ManagedAgentConfig = {
      ...managedConfig,
      renewal: {
        ...managedConfig.renewal,
        client_billings: upsertClientBilling(managedConfig.renewal?.client_billings || [], record, billing),
      },
    }
    const payload = buildSectionSavePayload(baseConfig, nextConfig, 'renewal', selectedAgentId)
    setConfigSavingSection('renewal')
    setClientBillingSavingKey(savingKey)
    setConfigError('')
    try {
      const saved = await fetchJSON<ManagedAgentConfig>(`/api/v1/agents/${selectedAgentId}/config`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      })
      const cachedName = cachedCustomerDisplayName(selectedAgentId)
      const savedWithCustomerName = !hasCustomerDisplayNameField(saved) && cachedName !== undefined
        ? { ...saved, customer_display_name: cachedName }
        : saved
      const normalized = normalizeManagedConfig(savedWithCustomerName, selectedAgentId, saved.agent_name || selectedAgent?.agent_name)
      rememberCustomerDisplayName(selectedAgentId, normalized.customer_display_name || '')
      setSavedManagedConfig(normalized)
      managedConfigDirtyRef.current = false
      setManagedConfig(normalized)
      let expirySyncError = ''
      let expirySyncAction: XUIAction | null = null
      if (shouldSyncExpiry && targetAgentID && record.email) {
        try {
          expirySyncAction = await fetchJSON<XUIAction>(`/api/v1/agents/${targetAgentID}/xui/actions`, {
            method: 'POST',
            body: JSON.stringify({
              kind: 'update_client_expiry',
              payload: {
                inbound_id: targetInboundID,
                inbound_tag: targetInboundTag,
                email: record.email,
                expiry_time: expiryTime,
                persist_billing: false,
              },
            }),
          })
          await loadXUIActions(targetAgentID, { silent: true })
          scheduleXUIActionResultRefresh(targetAgentID)
          window.setTimeout(() => {
            void loadOverview(targetAgentID, { silent: true })
            if (selectedAgentId && targetAgentID !== selectedAgentId) {
              void loadOverview(selectedAgentId, { silent: true })
            }
          }, 2500)
        } catch (error) {
          if (isUnauthorized(error)) {
            setAdminUser(null)
          }
          expirySyncError = error instanceof Error ? error.message : '下发 x-ui 到期时间失败'
        }
      }
      if (expirySyncError) {
        message.warning(`客户端收费已保存，但 x-ui 到期时间同步失败：${expirySyncError}`)
      } else if (expirySyncAction) {
        message.success(expirySyncAction.status === 'running'
          ? '客户端收费已保存，x-ui 到期时间已通过 WS 下发'
          : '客户端收费已保存，x-ui 到期时间同步任务已创建')
      } else {
        message.success('客户端收费已保存')
      }
      await loadAgents()
      await loadConfigAudits(selectedAgentId, { silent: true })
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      const detail = error instanceof Error ? error.message : '保存客户端收费失败'
      setConfigError(detail)
      message.error(detail)
    } finally {
      setConfigSavingSection(null)
      setClientBillingSavingKey('')
    }
  }

  useEffect(() => {
    if (!adminUser) {
      return
    }
    const timer = window.setInterval(() => {
      void loadAgents({ silent: true })
      if (selectedAgentId) {
        void loadOverview(selectedAgentId, { silent: true })
      }
      if (selectedAgentId && activeTabKey === 'actions') {
        void loadXUIActions(selectedAgentId, { silent: true })
      }
      if (selectedAgentId && activeTabKey === 'logs') {
        void loadAgentLogs(selectedAgentId, { silent: true })
      }
    }, DASHBOARD_AUTO_REFRESH_MS)
    return () => {
      window.clearInterval(timer)
    }
  }, [activeTabKey, adminUser, selectedAgentId])

  const {
    filteredAgents,
    filteredChains,
    filteredClients,
    filteredTagLinks,
    monthlyFinance,
    offlineAgentCount,
    onlineAgentCount,
    scopedAgentCount,
    scopedFinanceAreaManagers,
    scopedNetwork,
    scopedNodeCount,
    tagFilterOptions,
    workbenchMonthlyFinance,
    workbenchNetwork,
    xuiErrorAgentCount,
  } = buildDashboardScope({
    agents,
    dashboardView,
    selectedTag,
    tagOptions,
    currentOverview,
    deferredClientSearch,
    canManageSystem,
    financeAccountsLoaded,
    financeAccountsError,
    financeCustomers,
    financeAreaManagers,
    costCurrency,
    exchangeRates,
  })
  const agentListAgents = filteredAgents.filter((agent) => agentMatchesHealthFilter(agent, agentHealthFilter))

  if (publicSiteMode) {
    return (
      <Suspense fallback={<div className="login-shell"><Spin size="large" /></div>}>
        <PublicSite />
      </Suspense>
    )
  }

  if (customerMode) {
    return (
      <Suspense fallback={<div className="login-shell"><Spin size="large" /></div>}>
        <CustomerPortal />
      </Suspense>
    )
  }

  if (sessionLoading) {
    return (
      <>
        <VisualEffects />
        <div className="login-shell">
          <Spin size="large" />
        </div>
      </>
    )
  }

  if (!adminUser) {
    return (
      <>
        <VisualEffects />
        <LoginScreen
          loginForm={loginForm}
          loginLoading={loginLoading}
          onChange={setLoginForm}
          onLogin={login}
        />
      </>
    )
  }

  const consoleModalOpen = accountModalOpen ||
    clientInstallModalOpen ||
    customerModalOpen ||
    frontendSettingsModalOpen ||
    Boolean(importURLClient) ||
    telegramBotModalOpen ||
    updateModalOpen ||
    xuiActionModalOpen
  const showWorkbenchDashboard = activeAdminPage === 'dashboard' && !topologyVisible
  const serverVersionLabel = `V${systemInfo?.version || '-'}`
  const openAccountModal = () => {
    setAccountForm({
      current_password: '',
      new_username: adminUser.username,
      new_password: '',
      confirm_password: '',
      avatar_url: adminUser.avatar_url || '',
    })
    setAccountModalOpen(true)
  }
  const openAssetsPage = () => {
    setActiveAdminPage('assets')
    setAgentHealthFilter('all')
    setTopologyVisible(false)
    setSelectedAgentId('')
    setActiveTabKey('overview')
    setSelectedOutboundTag('')
    setSelectedRuleIndex(null)
    setSelectedNodeAnchor('')
  }
  const openCustomersPage = () => setActiveAdminPage('customers')
  const openFrontProxiesPage = () => {
    setActiveAdminPage('front-proxies')
    setTopologyVisible(false)
  }
  const openSupportPage = () => setActiveAdminPage('support')
  const openSettingsPage = () => {
    setActiveAdminPage('settings')
    void openFrontendSettingsModal(false)
  }
  const openAccessLogsPage = () => setActiveAdminPage('access-logs')
  const openSchedulesPage = () => setActiveAdminPage('schedules')
  const shellNavigationProps = {
    adminUser,
    systemInfo,
    canManageSystem,
    isAreaManagerAccount,
    activeAdminPage,
    topologyVisible,
    onlineAgentCount,
    scopedAgentCount,
    supportUnreadCount,
    agentsLoading,
    themeMode,
    effectiveMode,
    heroTitle,
    serverVersionLabel,
    onOpenAccount: openAccountModal,
    onOpenClientInstall: () => void openClientInstallModal(),
    onOpenTelegram: () => setTelegramBotModalOpen(true),
    onOpenCustomers: openCustomersPage,
    onOpenFrontProxies: openFrontProxiesPage,
    onOpenSupport: openSupportPage,
    onOpenFrontendSettings: openSettingsPage,
    onOpenUpdates: () => setUpdateModalOpen(true),
    onLogout: () => void logout(),
    onOpenWorkbench: returnHome,
    onOpenAssets: openAssetsPage,
    onOpenTopology: openTopologyPanel,
    onOpenAccessLogs: openAccessLogsPage,
    onOpenSchedules: openSchedulesPage,
    onRefreshAgents: () => void loadAgents(),
    onThemeModeChange: setThemeMode,
  }

  return (
    <div className="page-shell admin-page-shell">
      <VisualEffects disabled={isAreaManagerAccount} />
      <div className="page-background page-background-left" />
      <div className="page-background page-background-right" />
      <div className={`app-shell admin-oa-shell${centerPanelOpen ? ' topology-open-shell' : ''}`}>
        <AdminShellNavigation {...shellNavigationProps} />

        <section className="admin-oa-main">
          <AdminShellTopbar {...shellNavigationProps} />

          {consoleModalOpen ? <Suspense fallback={null}>
          <ConsoleModals
            accountForm={accountForm}
            accountModalOpen={accountModalOpen}
            accountSaving={accountSaving}
            agents={agents}
            adminUser={adminUser}
            clientInstallCommandKind={clientInstallCommandKind}
            clientInstallForm={clientInstallForm}
            clientInstallLoading={clientInstallLoading}
            clientInstallModalOpen={clientInstallModalOpen}
            clientInstallSaving={clientInstallSaving}
            clientInstallLinuxCommand={clientInstallCommand}
            clientInstallOpenWrtCommand={clientOpenWrtCommand}
            clientInstallWindowsCMDCommand={clientWindowsCMDCommand}
            clientInstallWindowsPowerShellCommand={clientWindowsPowerShellCommand}
            customerModalOpen={customerModalOpen}
            customerAssignmentDraft={customerAssignmentDraft}
            editingTelegramBotId={editingTelegramBotId}
            frontendSettingsForm={frontendSettingsForm}
            frontendSettingsLoading={frontendSettingsLoading}
            frontendSettingsModalOpen={frontendSettingsModalOpen}
            frontendSettingsSaving={frontendSettingsSaving}
            importURLClient={importURLClient}
            outboundActionForm={outboundActionForm}
            outboundSourceLoading={outboundSourceLoading}
            outboundSourceOverview={outboundSourceOverview}
            overview={overview}
            routingActionForm={routingActionForm}
            selectedAgentId={selectedAgentId}
            systemInfo={systemInfo}
            telegramBotForm={telegramBotForm}
            telegramBotModalOpen={telegramBotModalOpen}
            telegramBotSaving={telegramBotSaving}
            telegramBots={telegramBots}
            telegramBotsLoading={telegramBotsLoading}
            updateLatestError={updateLatestError}
            updateLatestInfo={updateLatestInfo}
            updateLatestLoading={updateLatestLoading}
            updateLoading={updateLoading}
            updateModalOpen={updateModalOpen}
            xuiActionKind={xuiActionKind}
            xuiActionModalOpen={xuiActionModalOpen}
            xuiActionSaving={xuiActionSaving}
            addClientActionForm={addClientActionForm}
            addClientActionInbounds={addClientActionInbounds}
            onAccountFormChange={setAccountForm}
            onClientInstallCommandKindChange={setClientInstallCommandKind}
            onClientInstallFormChange={setClientInstallForm}
            onCloseAccount={() => setAccountModalOpen(false)}
            onCloseClientInstall={() => setClientInstallModalOpen(false)}
            onCloseCustomerModal={() => {
              setCustomerModalOpen(false)
              setCustomerAssignmentDraft(null)
            }}
            onCustomerAssignmentDraftApplied={() => setCustomerAssignmentDraft(null)}
            onCloseFrontendSettings={() => setFrontendSettingsModalOpen(false)}
            onCloseImportURL={() => setImportURLClient(null)}
            onCloseTelegramBot={() => setTelegramBotModalOpen(false)}
            onCloseUpdateModal={() => setUpdateModalOpen(false)}
            onCloseXUIActionModal={() => {
              setXUIActionModalOpen(false)
              setXUIActionAgentId('')
              setAddClientActionInbounds([])
            }}
            onConfigChanged={refreshAfterExternalConfigChange}
            onOpenCustomerAssignment={openCustomerAssignment}
            onCopyClientInstallCommand={(command) => void copyClientInstallCommand(command)}
            onCopyImportURL={(client) => void copyImportURL(client)}
            onDeleteTelegramBot={(id) => void deleteTelegramBot(id)}
            onRefreshLatestUpdate={() => void loadUpdateLatestInfo()}
            onRefreshTelegramBots={() => void loadTelegramBots()}
            onSaveAccount={() => void saveAccount()}
            onSaveClientInstallSettings={() => void saveClientInstallSettings()}
            onSaveFrontendSettings={() => void saveFrontendSettings()}
            onSaveTelegramBot={saveTelegramBot}
            onSubmitXUIAction={() => void createXUIAction()}
            onTelegramBotFormChange={setTelegramBotForm}
            onTelegramBotEditIDChange={setEditingTelegramBotId}
            onTestTelegramBot={(id) => void testTelegramBot(id)}
            onUpdateAllClients={() => void updateAllClientsOnline()}
            onUpdateFrontendSettingsFormChange={setFrontendSettingsForm}
            onUpdateAddClientActionForm={setAddClientActionForm}
            onUpdateOutboundActionForm={setOutboundActionForm}
            onUpdateRoutingActionForm={setRoutingActionForm}
            onUpdateServer={() => void updateServerOnline()}
            onXUIActionKindChange={setXUIActionKind}
          />
        </Suspense> : null}

        {activeAdminPage === 'customers' ? (
          <main className="admin-content-page">
            <Suspense fallback={<Spin size="large" />}>
              <CustomerManagementModal
                embedded
                agents={agents}
                adminUser={adminUser}
                initialAssignment={customerAssignmentDraft}
                onInitialAssignmentApplied={() => setCustomerAssignmentDraft(null)}
                onConfigChanged={refreshAfterExternalConfigChange}
                onOpenAssignment={openCustomerAssignment}
              />
            </Suspense>
          </main>
        ) : activeAdminPage === 'front-proxies' ? (
          <Suspense fallback={<Spin size="large" />}>
            <FrontProxyManagementPage canManageNodes={canManageSystem} />
          </Suspense>
        ) : activeAdminPage === 'support' ? (
          <Suspense fallback={<Spin size="large" />}>
            <AdminSupportPage onUnreadCountChange={setSupportUnreadCount} />
          </Suspense>
        ) : activeAdminPage === 'access-logs' ? (
          <AdminAccessLogsPage
            agents={agents}
            filters={accessLogFilters}
            logs={accessLogs}
            loading={accessLogsLoading}
            total={accessLogsTotal}
            onFiltersChange={setAccessLogFilters}
            onLoad={() => void loadAccessLogs()}
          />
        ) : activeAdminPage === 'settings' ? (
          <main className="admin-content-page">
            <FrontendSettingsPanel
              loading={frontendSettingsLoading}
              saving={frontendSettingsSaving}
              form={frontendSettingsForm}
              onSave={() => void saveFrontendSettings()}
              onFormChange={setFrontendSettingsForm}
            />
          </main>
        ) : activeAdminPage === 'schedules' ? (
          <main className="admin-content-page">
            <ScheduledTasksPanel
              loading={scheduledTasksLoading}
              saving={scheduledTasksSaving}
              settings={scheduledTasks}
              onSave={() => void saveScheduledTasks()}
              onChange={(value) => setScheduledTasks(normalizeScheduledTaskSettings(value))}
            />
          </main>
        ) : showWorkbenchDashboard ? (
          <main className="admin-content-page admin-workbench-page">
            <AdminWorkbenchDashboard
              agents={agents}
              dashboardView={dashboardView}
              scopedNetwork={workbenchNetwork}
              monthlyFinance={workbenchMonthlyFinance}
              costCurrency={costCurrency}
              restrictedView={isAreaManagerAccount}
              onSelectAgent={(agentID) => openAgentDetailPanel(agentID)}
              onOpenHealthFilter={openAgentHealthFilter}
              onOpenTopology={openTopologyPanel}
            />
          </main>
        ) : (
        <div className={`workspace-grid${centerPanelOpen ? ' workspace-grid-topology' : ''}${topologyVisible ? ' workspace-grid-topology-open' : ''}${selectedAgent && !topologyVisible ? ' workspace-grid-agent-open' : ''}`}>
          <OverviewSummaryCard
            dashboardView={dashboardView}
            scopedAgentCount={scopedAgentCount}
            scopedNodeCount={scopedNodeCount}
            onlineAgentCount={onlineAgentCount}
            offlineAgentCount={offlineAgentCount}
            xuiErrorAgentCount={xuiErrorAgentCount}
            scopedNetwork={scopedNetwork}
            costCurrency={costCurrency}
            currencyOptions={currencyOptions}
            monthlyFinance={monthlyFinance}
            financeAgents={filteredAgents}
            financeChains={filteredChains}
            financeCustomers={financeCustomers}
            financeAreaManagers={scopedFinanceAreaManagers}
            exchangeRates={exchangeRates}
            selectedTag={selectedTag}
            currentAgentLabel={selectedAgent?.agent_name || selectedAgent?.agent_id || ''}
            currentIPv4={selectedSummary.public_ipv4 || ''}
            compact={centerPanelOpen}
            restrictedView={isAreaManagerAccount}
            onCostCurrencyChange={setCostCurrency}
          />

          <AgentRail
            agents={agentListAgents}
            loading={agentsLoading}
            error={agentsError}
            selectedTag={selectedTag}
            healthFilter={agentHealthFilter}
            selectedAgentId={selectedAgentId}
            tagFilterOptions={tagFilterOptions}
            viewMode={agentViewMode}
            panelExpanded={centerPanelOpen}
            topologyVisible={topologyVisible}
            restrictedView={isAreaManagerAccount}
            onToggleViewMode={toggleAgentViewMode}
            onToggleTopology={() => {
              if (topologyVisible) {
                setTopologyVisible(false)
                setSelectedAgentId('')
                setActiveTabKey('overview')
              } else {
                openTopologyPanel()
              }
            }}
            onRefresh={() => void loadAgents()}
            onSelectTag={selectDashboardTag}
            onHealthFilterChange={setAgentHealthFilter}
            onSelectAgent={(agentID, active) => {
              if (topologyVisible) {
                selectTopologyAgent(agentID)
                return
              }
              if (active && !topologyVisible) {
                setReloadToken((current) => current + 1)
                return
              }
              openAgentDetailPanel(agentID)
            }}
          />

          <main className="main-stage">
            {dashboardView && topologyVisible ? (
              <div id="topology-panel">
                {topologyLoading && !topologyLoaded ? (
                  <Card>
                    <Spin tip="正在计算链路拓扑..." />
                  </Card>
                ) : topologyError && !topologyLoaded ? (
                  <Alert
                    type="error"
                    showIcon
                    message="链路拓扑加载失败"
                    description={topologyError}
                    action={<Button size="small" onClick={() => void loadTopology()}>重试</Button>}
                  />
                ) : renderCNFlowPanel({
                    dashboardView,
                    selectedTag,
                    selectedAgentId,
                    agents: dashboardView.agents,
                    chains: filteredChains,
                    onSelectAgent: (agentID) => {
                      selectTopologyAgent(agentID)
                    },
                    onJumpNode: jumpToNode,
                    restrictedView: isAreaManagerAccount,
                    canOpenXUI: !isAreaManagerAccount && Boolean(managedConfig?.xui?.base_url),
                    onOpenXUI: () => {
                      if (managedConfig?.xui?.base_url) {
                        window.open(managedConfig.xui.base_url, '_blank', 'noopener,noreferrer')
                      }
                    },
                    canRefreshCurrentNode: !isAreaManagerAccount && Boolean(selectedAgentId),
                    currentNodeLoading: overviewLoading || configLoading || agentRefreshLoading,
                    onRefreshCurrentNode: () => {
                      if (selectedAgentId) {
                        void requestAgentSnapshot(selectedAgentId)
                      }
                    },
                    canRestartXUI: !isAreaManagerAccount && Boolean(selectedAgentId),
                    xuiRestartLoading,
                    onRestartXUI: () => {
                      if (selectedAgentId) {
                        void restartXUIService(selectedAgentId)
                      }
                    },
                    canUpdate3XUI: !isAreaManagerAccount && Boolean(selectedAgentId),
                    xuiUpdateLoading,
                    onUpdate3XUI: () => {
                      if (selectedAgentId) {
                        void update3XUI(selectedAgentId)
                      }
                    },
                    searchText: topologySearch,
                    onSearchTextChange: setTopologySearch,
                  })}
              </div>
            ) : null}

            {selectedAgent && !topologyVisible ? (
              <Suspense fallback={<Spin size="large" />}>
                <AgentDetailPanel
                  key={selectedAgentId}
                activeTabKey={activeTabKey}
                agentLogs={agentLogs}
                agentLogsError={agentLogsError}
                agentLogsLoading={agentLogsLoading}
                canOpenXUI={Boolean(managedConfig?.xui?.base_url)}
                canManageConfig={canManageSystem}
                restrictedView={isAreaManagerAccount}
                clientSearch={clientSearch}
                configAudits={configAudits}
                configAuditsLoading={configAuditsLoading}
                configError={configError}
                configLoading={configLoading}
                configSavingSection={configSavingSection}
                clientBillingSavingKey={clientBillingSavingKey}
                currencyOptions={currencyOptions}
                currentAgentLoading={overviewLoading || configLoading || agentRefreshLoading}
                xuiRestartLoading={xuiRestartLoading}
                xuiUpdateLoading={xuiUpdateLoading}
                realmCopyLoading={realmCopyLoading}
                remoteCommandLoading={remoteCommandLoading}
                dashboardView={dashboardView}
                entryAddressInputText={entryAddressInputText}
                filteredAgents={filteredAgents}
                filteredClients={filteredClients}
                filteredTagLinks={filteredTagLinks}
                managedConfig={managedConfig}
                newTagName={newTagName}
                overview={currentOverview}
                overviewError={overviewError}
                overviewLoading={overviewLoading}
                selectedAgent={selectedAgent}
                selectedAgentId={selectedAgentId}
                selectedNodeAnchor={selectedNodeAnchor}
                selectedOutboundTag={selectedOutboundTag}
                selectedRuleIndex={selectedRuleIndex}
                selectedTag={selectedTag}
                tagOptions={tagOptions}
                tagSaving={tagSaving}
                xuiActions={xuiActions}
                xuiActionsLoading={xuiActionsLoading}
                xuiClientDeleteLoadingKey={xuiClientDeleteLoadingKey}
                xuiClientTrafficSavingKey={xuiClientTrafficSavingKey}
                xuiClientToggleLoadingKey={xuiClientToggleLoadingKey}
                agentDeleteLoading={agentDeleteLoading}
                agentReplaceLoading={agentReplaceLoading}
                replacementAgents={agents.filter((agent) => agent.agent_id !== selectedAgentId)}
                onActiveTabChange={setActiveTabKey}
                onClientSearchChange={setClientSearch}
                onCopyImportURL={(client) => void copyImportURL(client)}
                onCreateRoutingAction={() => {
                  setXUIActionAgentId(selectedAgentId)
                  setAddClientActionInbounds([])
                  setXUIActionKind('upsert_routing_rule')
                  setAddClientActionForm(defaultAddClientActionForm())
                  setOutboundActionForm(defaultOutboundActionForm())
                  setRoutingActionForm(defaultRoutingActionForm())
                  setXUIActionModalOpen(true)
                }}
                onCreateNodeClientAction={(node, actionAgentID) => {
                  const targetAgentID = actionAgentID || node.realm_target_agent_id || selectedAgentId
                  const targetNode: XUINodeView = node.realm_target_inbound_id
                    ? {
                        ...node,
                        id: node.realm_target_inbound_id,
                        tag: node.realm_target_inbound_tag || node.tag || '',
                        remark: node.remark || node.realm_target_inbound_tag || `Inbound #${node.realm_target_inbound_id}`,
                      }
                    : node
                  setXUIActionAgentId(targetAgentID)
                  setAddClientActionInbounds([targetNode])
                  setXUIActionKind('add_client')
                  setAddClientActionForm({
                    ...defaultAddClientActionForm(),
                    inbound_id: targetNode.id,
                    inbound_tag: targetNode.tag || '',
                    inbound_name: targetNode.remark || targetNode.tag || `Inbound #${targetNode.id}`,
                    protocol: targetNode.protocol || 'vless',
                  })
                  setXUIActionModalOpen(true)
                }}
                onCreateTag={() => void createTagOption()}
                onEntryAddressesTextChange={(value) => {
                  setEntryAddressInputText(value)
                  updateManagedConfig((current) => ({ ...current, entry: { ...current.entry, addresses: parseAddressInput(value) } }))
                }}
                onEntryChange={(patch) => updateManagedConfig((current) => ({ ...current, entry: { ...current.entry, ...patch } }))}
                onJumpNode={jumpToNode}
                onJumpOutbound={jumpToOutbound}
                onJumpRule={jumpToRule}
                onManagedConfigAgentNameChange={(value) => updateManagedConfig((current) => ({ ...current, agent_name: value }))}
                onManagedConfigCustomerDisplayNameChange={(value) => updateManagedConfig((current) => ({ ...current, customer_display_name: value }))}
                onManagedConfigSortOrderChange={(value) => updateManagedConfig((current) => ({ ...current, sort_order: value }))}
                onNewTagNameChange={setNewTagName}
                onOpenImportURL={setImportURLClient}
                onOpenLogs={() => {
                  setActiveTabKey('logs')
                  void loadAgentLogs(selectedAgentId)
                }}
                onOpenXUI={() => {
                  if (managedConfig?.xui?.base_url) {
                    window.open(managedConfig.xui.base_url, '_blank', 'noopener,noreferrer')
                  }
                }}
                onAuthorizeCustomer={openCustomerAuthorization}
                onDeleteCurrentAgent={() => void deleteAgent(selectedAgentId)}
                onReplaceCurrentAgent={(replacementAgentID) => replaceAgent(replacementAgentID, selectedAgentId)}
                onDeleteXUIClient={(client) => void deleteXUIClient(client, selectedAgentId)}
                onSaveXUIClientTrafficLimit={(client, totalGB) => void saveXUIClientTrafficLimit(client, totalGB, selectedAgentId)}
                onSetXUIClientEnabled={(client, enabled) => void setXUIClientEnabled(client, enabled, selectedAgentId)}
                onRefreshCurrentAgent={() => void requestAgentSnapshot(selectedAgentId)}
                onRestartXUI={() => void restartXUIService(selectedAgentId)}
                onUpdate3XUI={() => void update3XUI(selectedAgentId)}
                onExecuteRemoteCommand={(command, shell, timeoutSeconds) => void executeRemoteCommand(command, shell, timeoutSeconds, selectedAgentId)}
                onRefreshXUIActions={() => void loadXUIActions()}
                onCopyRealmConfig={(targetAgentID) => void copyRealmConfigToAgent(targetAgentID, selectedAgentId)}
                onRenewalChange={(patch) => updateManagedConfig((current) => ({ ...current, renewal: { ...current.renewal, ...patch } }))}
                onReturnHome={returnHome}
                onSaveClientBilling={(record) => void saveClientBilling(record)}
                onSaveManagedConfigSection={(section) => void saveManagedConfigSection(section)}
                onSavePrimaryDomain={savePrimaryDomain}
                onSelectTag={selectDashboardTag}
                onTagsChange={(values) => {
                  setTagOptions((current) => mergeTagOptions(current, values))
                  updateManagedConfig((current) => ({ ...current, tags: values }))
                }}
                onSaveAreaTags={(values) => void saveAreaAgentTags(values)}
                onUpdateClientBillingDraft={updateClientBillingDraft}
                onXUIChange={(patch) => updateManagedConfig((current) => ({ ...current, xui: { ...current.xui, ...patch } }))}
                onFeatureChange={(feature, enabled) => updateManagedConfig((current) => {
                  const features = { ...current.features, [feature]: enabled }
                  const entry = { ...current.entry }
                  if (enabled && feature === 'realm') {
                    features.haproxy = false
                    entry.haproxy = { ...entry.haproxy, enabled: false }
                  }
                  if (enabled && feature === 'haproxy') {
                    features.realm = false
                    entry.port_forwarding = { ...entry.port_forwarding, enabled: false, backend: 'none' }
                  }
                  return { ...current, features, entry }
                })}
                />
              </Suspense>
            ) : null}
          </main>
        </div>
        )}
        </section>
      </div>
    </div>
  )

  function updateManagedConfig(updater: (current: ManagedAgentConfig) => ManagedAgentConfig) {
    managedConfigDirtyRef.current = true
    setManagedConfig((current) => {
      const base = current || createEmptyManagedConfig(selectedAgentId, selectedAgent?.agent_name)
      return updater(base)
    })
  }

  function updateClientBillingDraft(record: XUIClientView, patch: Partial<XUIClientBillingConfig>) {
    updateManagedConfig((current) => {
      const currentBilling = findClientBilling(current.renewal?.client_billings, record) || defaultClientBilling(record)
      return {
        ...current,
        renewal: {
          ...current.renewal,
          client_billings: upsertClientBilling(current.renewal?.client_billings || [], record, {
            ...currentBilling,
            ...patch,
          }),
        },
      }
    })
  }
}
