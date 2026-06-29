import { lazy, startTransition, Suspense, useDeferredValue, useEffect, useRef, useState } from 'react'
import {
  Alert,
  App as AntdApp,
  Button,
  Card,
  Col,
  Input,
  InputNumber,
  Row,
  Select,
  Space,
  Spin,
  Switch,
  Table,
  Typography,
} from 'antd'
import { ReloadOutlined } from '@ant-design/icons'

import type {
  AdminAuthResponse,
  AdminUser,
  AccessLogEntry,
  AccessLogListResponse,
  AreaManagerAdminView,
  AreaAgentTagsResponse,
  AgentListItem,
  AgentLogsResponse,
  AgentRefreshResponse,
  AgentRealtimeMetrics,
  ConfigAuditLog,
  CustomerAdminView,
  CustomerAssignmentDraft,
  DashboardAgentView,
  DashboardRealtimeMessage,
  ExchangeRatesResponse,
  FrontendSettings,
  ClientInstallInfo,
  GlobalDashboardView,
  ManagedAgentConfig,
  ScheduledTaskSettings,
  SystemInfo,
  TelegramBot,
  TagSettingsResponse,
  UpdateLatestInfo,
  UpdateResponse,
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
  summarizeMonthlyFinance,
} from './lib/currency'
import {
  summarizeAgentNetwork,
} from './lib/traffic'
import { useCurrentAgentRequest } from './hooks/useCurrentAgentRequest'
import { buildAdminRouteURL, parseAdminRouteState, type AdminPageKey } from './lib/adminRoute'
import { isAreaManagerAdminUser, sanitizeAreaRealtimeMetric } from './lib/adminUsers'
import { createAppNavigationHandlers } from './lib/appNavigation'
import { defaultScheduledTaskSettings, normalizeScheduledTaskSettings } from './lib/scheduledTasks'

import type {
  AgentViewMode,
  ClientInstallCommandForm,
  ClientInstallCommandKind,
  ConfigSectionKey,
  FrontendSettingsForm,
  TelegramBotForm,
  XUIAddClientActionForm,
  XUIOutboundActionForm,
  XUIRoutingActionForm,
} from './lib/appHelpers'
import {
  FrontendSettingsPanel,
  ScheduledTasksPanel,
} from './components/AdminModals'
import { AdminShellNavigation } from './components/AdminShellNavigation'
import { renderCNFlowPanel } from './components/DashboardTopologyPanels'
import { AgentRail, AdminWorkbenchDashboard, OverviewSummaryCard } from './components/DashboardSidebar'
import { LoginScreen } from './components/LoginScreen'
import { VisualEffects, applyCustomFrontendCode } from './components/VisualEffects'
import { useAppTheme } from './theme'
import {
  DASHBOARD_AUTO_REFRESH_MS,
  buildClientInstallCommand,
  billingKeyForClient,
  buildDashboardRealtimeURL,
  buildSectionSavePayload,
  buildWindowsCMDInstallCommand,
  buildWindowsPowerShellInstallCommand,
  buildXUIActionPayload,
  chainMatchesSelectedTag,
  configSectionLabel,
  configSignature,
  createEmptyManagedConfig,
  defaultAddClientActionForm,
  defaultClientBilling,
  defaultClientInstallCommandForm,
  defaultFrontendSettingsForm,
  defaultOutboundActionForm,
  defaultRoutingActionForm,
  defaultTelegramBotForm,
  findOutboundLinkedClient,
  findClientBilling,
  fetchJSON,
  formatAddressInput,
  formatDateTime,
  formatRenewalHint,
  formatTagInput,
  hasSelectedTag,
  isAgentRunning,
  isUnauthorized,
  mergeDashboardTagOptions,
  mergeRealtimeMetricsIntoAgents,
  mergeRealtimeSummary,
  mergeSavedSectionIntoDraft,
  mergeTagOptions,
  normalizeEntryConfig,
  normalizeClientInstallCommandForm,
  normalizeFrontendSettingsForm,
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
  topologyMatchesSelectedTag,
  upsertClientBilling,
} from './lib/appHelpers'

const { Text, Title } = Typography
interface LoadOptions {
  silent?: boolean
}

function hasCustomerDisplayNameField(value: unknown): boolean {
  return Boolean(value && typeof value === 'object' && Object.prototype.hasOwnProperty.call(value, 'customer_display_name'))
}

const AgentDetailPanel = lazy(() => import('./components/AgentDetailPanel').then((module) => ({ default: module.AgentDetailPanel })))
const ConsoleModals = lazy(() => import('./components/ConsoleModals').then((module) => ({ default: module.ConsoleModals })))
const CustomerManagementModal = lazy(() => import('./components/CustomerManagementModal').then((module) => ({ default: module.CustomerManagementModal })))
const CustomerPortal = lazy(() => import('./components/CustomerPortal').then((module) => ({ default: module.CustomerPortal })))
const PublicSite = lazy(() => import('./components/PublicSite').then((module) => ({ default: module.PublicSite })))

export default function App() {
  const { message } = AntdApp.useApp()
  const { mode: themeMode, effectiveMode, setMode: setThemeMode } = useAppTheme()
  const [sessionLoading, setSessionLoading] = useState(true)
  const [adminUser, setAdminUser] = useState<AdminUser | null>(null)
  const [systemInfo, setSystemInfo] = useState<SystemInfo | null>(null)
  const [loginLoading, setLoginLoading] = useState(false)
  const [loginForm, setLoginForm] = useState({ username: '', password: '' })
  const [accountModalOpen, setAccountModalOpen] = useState(false)
  const [accountSaving, setAccountSaving] = useState(false)
  const [accountForm, setAccountForm] = useState({
    current_password: '',
    new_username: '',
    new_password: '',
    confirm_password: '',
    avatar_url: '',
  })
  const [agents, setAgents] = useState<DashboardAgentView[]>([])
  const [agentsLoading, setAgentsLoading] = useState(false)
  const [agentsError, setAgentsError] = useState('')
  const [dashboardView, setDashboardView] = useState<GlobalDashboardView | null>(null)
  const [financeCustomers, setFinanceCustomers] = useState<CustomerAdminView[]>([])
  const [financeAreaManagers, setFinanceAreaManagers] = useState<AreaManagerAdminView[]>([])
  const [costCurrency, setCostCurrency] = useState<CurrencyCode>(() => readStoredCostCurrency())
  const [exchangeRates, setExchangeRates] = useState<ExchangeRatesState>(() => defaultExchangeRatesState())
  const [currencyOptions, setCurrencyOptions] = useState<CurrencyCode[]>(() => [...COMMON_COST_CURRENCIES])
  const [selectedTag, setSelectedTag] = useState('')
  const [topologySearch, setTopologySearch] = useState('')
  const [activeAdminPage, setActiveAdminPage] = useState<AdminPageKey>('dashboard')
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
  const [telegramBots, setTelegramBots] = useState<TelegramBot[]>([])
  const [telegramBotsLoading, setTelegramBotsLoading] = useState(false)
  const [telegramBotModalOpen, setTelegramBotModalOpen] = useState(false)
  const [telegramBotSaving, setTelegramBotSaving] = useState(false)
  const [editingTelegramBotId, setEditingTelegramBotId] = useState<number | null>(null)
  const [telegramBotForm, setTelegramBotForm] = useState<TelegramBotForm>(() => defaultTelegramBotForm())
  const [customerModalOpen, setCustomerModalOpen] = useState(false)
  const [customerAssignmentDraft, setCustomerAssignmentDraft] = useState<CustomerAssignmentDraft | null>(null)
  const [clientInstallModalOpen, setClientInstallModalOpen] = useState(false)
  const [clientInstallLoading, setClientInstallLoading] = useState(false)
  const [clientInstallSaving, setClientInstallSaving] = useState(false)
  const [clientInstallForm, setClientInstallForm] = useState<ClientInstallCommandForm>(() => defaultClientInstallCommandForm())
  const [clientInstallCommandKind, setClientInstallCommandKind] = useState<ClientInstallCommandKind>('linux')
  const [frontendSettingsModalOpen, setFrontendSettingsModalOpen] = useState(false)
  const [frontendSettingsLoading, setFrontendSettingsLoading] = useState(false)
  const [frontendSettingsSaving, setFrontendSettingsSaving] = useState(false)
  const [frontendSettingsForm, setFrontendSettingsForm] = useState<FrontendSettingsForm>(() => defaultFrontendSettingsForm())
  const [scheduledTasksLoading, setScheduledTasksLoading] = useState(false)
  const [scheduledTasksSaving, setScheduledTasksSaving] = useState(false)
  const [scheduledTasks, setScheduledTasks] = useState<ScheduledTaskSettings>(() => defaultScheduledTaskSettings())
  const [accessLogs, setAccessLogs] = useState<AccessLogEntry[]>([])
  const [accessLogsTotal, setAccessLogsTotal] = useState(0)
  const [accessLogsLoading, setAccessLogsLoading] = useState(false)
  const [accessLogFilters, setAccessLogFilters] = useState({
    agent_id: '',
    source_ip: '',
    target: '',
    client_email: '',
    limit: 100,
  })
  const [updateModalOpen, setUpdateModalOpen] = useState(false)
  const [updateLoading, setUpdateLoading] = useState(false)
  const [updateLatestLoading, setUpdateLatestLoading] = useState(false)
  const [updateLatestInfo, setUpdateLatestInfo] = useState<UpdateLatestInfo | null>(null)
  const [updateLatestError, setUpdateLatestError] = useState('')
  const [reloadToken, setReloadToken] = useState(0)
  const [activeTabKey, setActiveTabKey] = useState('overview')
  const [topologyVisible, setTopologyVisible] = useState(false)
  const [topologyLoading, setTopologyLoading] = useState(false)
  const [topologyError, setTopologyError] = useState('')
  const [topologyLoaded, setTopologyLoaded] = useState(false)
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
  const [xuiRestartLoading, setXUIRestartLoading] = useState(false)
  const [xuiUpdateLoading, setXUIUpdateLoading] = useState(false)
  const [xuiClientDeleteLoadingKey, setXUIClientDeleteLoadingKey] = useState('')
  const [xuiClientToggleLoadingKey, setXUIClientToggleLoadingKey] = useState('')
  const [realmCopyLoading, setRealmCopyLoading] = useState(false)
  const [remoteCommandLoading, setRemoteCommandLoading] = useState(false)
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
  const applyingRouteRef = useRef(false)
  const lastAdminURLRef = useRef('')
  const inFlightRequestsRef = useRef<Set<string>>(new Set())
  const lastDetailAgentIdRef = useRef('')
  const isCurrentAgentRequest = useCurrentAgentRequest(selectedAgentId)

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
  const heroTitle = '南风VPS监控'
  const normalizedPath = window.location.pathname.replace(/\/+$/, '') || '/'
  const customerMode = normalizedPath === '/customer'
  const publicSiteMode = normalizedPath === '/site' || normalizedPath === '/official' || new URLSearchParams(window.location.search).get('page') === 'site'
  const isAreaManagerAccount = isAreaManagerAdminUser(adminUser)
  const canManageSystem = Boolean(adminUser && !isAreaManagerAccount)
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
    if (customerMode || publicSiteMode || sessionLoading || !adminUser) {
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
  }, [adminUser, canManageSystem, customerMode, publicSiteMode, sessionLoading])

  useEffect(() => {
    if (customerMode || publicSiteMode || sessionLoading || !adminUser) {
      return
    }
    if (applyingRouteRef.current) {
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
    if (applyingRouteRef.current) {
      window.history.replaceState(null, '', nextURL)
    } else {
      window.history.pushState(null, '', nextURL)
    }
    lastAdminURLRef.current = nextURL
  }, [
    activeAdminPage,
    activeTabKey,
    adminUser,
    customerMode,
    publicSiteMode,
    selectedAgentId,
    selectedNodeAnchor,
    selectedOutboundTag,
    selectedRuleIndex,
    selectedTag,
    sessionLoading,
    topologySearch,
    topologyVisible,
  ])

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

  async function loadSession() {
    setSessionLoading(true)
    try {
      const data = await fetchJSON<AdminAuthResponse>('/api/v1/admin/session')
      setAdminUser(data.user)
      setSystemInfo(data.system || null)
      setAccountForm((current) => ({ ...current, new_username: data.user.username, avatar_url: data.user.avatar_url || '' }))
    } catch {
      setAdminUser(null)
      setSystemInfo(null)
    } finally {
      setSessionLoading(false)
    }
  }

  async function login() {
    setLoginLoading(true)
    try {
      const data = await fetchJSON<AdminAuthResponse>('/api/v1/admin/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(loginForm),
      })
      setAdminUser(data.user)
      setSystemInfo(data.system || null)
      setAccountForm((current) => ({ ...current, new_username: data.user.username, avatar_url: data.user.avatar_url || '' }))
      setLoginForm({ username: data.user.username, password: '' })
      message.success('登录成功')
    } catch (error) {
      message.error(error instanceof Error ? error.message : '登录失败')
    } finally {
      setLoginLoading(false)
    }
  }

  async function logout() {
    try {
      await fetchJSON<{ status: string }>('/api/v1/admin/logout', { method: 'POST' })
    } catch {
      // Session cleanup is best-effort; local state is cleared either way.
    }
    setAdminUser(null)
    setSystemInfo(null)
    setDashboardView(null)
    setSelectedTag('')
    setAgents([])
    setFinanceCustomers([])
    setFinanceAreaManagers([])
    setSelectedAgentId('')
    setOverview(null)
    setManagedConfig(null)
    setTelegramBots([])
    setAgentLogs(null)
    setConfigAudits([])
  }

  async function saveAccount() {
    if (accountForm.new_password && accountForm.new_password !== accountForm.confirm_password) {
      message.error('两次输入的新密码不一致')
      return
    }
    setAccountSaving(true)
    try {
      const data = await fetchJSON<AdminAuthResponse>('/api/v1/admin/account', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          current_password: accountForm.current_password,
          new_username: accountForm.new_username,
          new_password: accountForm.new_password,
          avatar_url: accountForm.avatar_url,
        }),
      })
      setAdminUser(data.user)
      setSystemInfo(data.system || systemInfo)
      setAccountForm({
        current_password: '',
        new_username: data.user.username,
        new_password: '',
        confirm_password: '',
        avatar_url: data.user.avatar_url || '',
      })
      setAccountModalOpen(false)
      message.success('管理员账号已更新')
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      message.error(error instanceof Error ? error.message : '更新账号失败')
    } finally {
      setAccountSaving(false)
    }
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
        if (!topologyVisible || !current || !topologyLoaded) {
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
      if (topologyVisible) {
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
        error: data.stale && data.error ? `使用缓存汇率：${data.error}` : '',
      })
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      setExchangeRates((current) => ({
        ...current,
        loading: false,
        error: error instanceof Error ? error.message : '加载汇率失败',
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
    } catch {
      setFinanceCustomers([])
      setFinanceAreaManagers([])
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

  async function restartXUIService(agentID = selectedAgentId) {
    if (!agentID) {
      return
    }
    setXUIRestartLoading(true)
    try {
      await fetchJSON<XUIAction>(`/api/v1/agents/${agentID}/xui/actions`, {
        method: 'POST',
        body: JSON.stringify({ kind: 'restart_xui', payload: { service_name: 'x-ui' } }),
      })
      message.success('已创建 x-ui / Xray 重启任务；在线 Client 会通过 WS 立即执行，失败日志会写入操作记录')
      window.setTimeout(() => {
        void loadXUIActions(agentID, { silent: true })
        void loadAgentLogs(agentID, { silent: true })
        void loadOverview(agentID, { silent: true })
      }, 2500)
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      message.error(error instanceof Error ? error.message : '下发 x-ui 重启失败')
    } finally {
      setXUIRestartLoading(false)
    }
  }

  async function update3XUI(agentID = selectedAgentId) {
    if (!agentID) {
      return
    }
    setXUIUpdateLoading(true)
    try {
      const action = await fetchJSON<XUIAction>(`/api/v1/agents/${agentID}/xui/actions`, {
        method: 'POST',
        body: JSON.stringify({ kind: 'update_3xui', payload: { timeout_seconds: 900 } }),
      })
      message.success(action.status === 'running' ? '已通过 WS 下发 3x-ui 升级任务，结果会实时回传到操作记录' : '已创建 3x-ui 升级任务，Client 不在线时会等待轮询执行')
      await loadXUIActions(agentID, { silent: true })
      scheduleXUIActionResultRefresh(agentID)
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      message.error(error instanceof Error ? error.message : '下发 3x-ui 升级失败')
    } finally {
      setXUIUpdateLoading(false)
    }
  }

  function xuiClientActionKey(record: XUIClientView): string {
    return [record.inbound_id, record.inbound_tag || '', record.email || '', record.auth_uuid || record.auth_password || ''].join(':')
  }

  async function deleteXUIClient(record: XUIClientView, agentID = selectedAgentId) {
    const targetAgentID = record.realm_target_agent_id || agentID
    const targetInboundID = record.realm_target_inbound_id || record.inbound_id
    const targetInboundTag = record.realm_target_inbound_tag || record.inbound_tag || ''
    if (!targetAgentID) {
      return
    }
    const key = xuiClientActionKey(record)
    setXUIClientDeleteLoadingKey(key)
    try {
      const action = await fetchJSON<XUIAction>(`/api/v1/agents/${targetAgentID}/xui/actions`, {
        method: 'POST',
        body: JSON.stringify({
          kind: 'delete_client',
          payload: {
            inbound_id: targetInboundID,
            inbound_tag: targetInboundTag,
            email: record.email || '',
            client_id: record.auth_uuid || record.auth_password || '',
            restart: false,
          },
        }),
      })
      message.success(action.status === 'running' ? '已通过 WS 下发删除 Client 任务，结果会回传到操作记录' : '已创建删除 Client 任务，Client 不在线时会等待轮询执行')
      await loadXUIActions(targetAgentID, { silent: true })
      scheduleXUIActionResultRefresh(targetAgentID)
      window.setTimeout(() => {
        void loadOverview(targetAgentID, { silent: true })
        if (selectedAgentId && targetAgentID !== selectedAgentId) {
          void loadOverview(selectedAgentId, { silent: true })
        }
        void loadAgents({ silent: true })
      }, 2500)
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      message.error(error instanceof Error ? error.message : '删除 Client 失败')
    } finally {
      setXUIClientDeleteLoadingKey('')
    }
  }

  async function setXUIClientEnabled(record: XUIClientView, enabled: boolean, agentID = selectedAgentId) {
    const targetAgentID = record.realm_target_agent_id || agentID
    const targetInboundID = record.realm_target_inbound_id || record.inbound_id
    const targetInboundTag = record.realm_target_inbound_tag || record.inbound_tag || ''
    if (!targetAgentID) {
      return
    }
    const key = xuiClientActionKey(record)
    setXUIClientToggleLoadingKey(key)
    try {
      const action = await fetchJSON<XUIAction>(`/api/v1/agents/${targetAgentID}/xui/actions`, {
        method: 'POST',
        body: JSON.stringify({
          kind: 'set_client_enabled',
          payload: {
            inbound_id: targetInboundID,
            inbound_tag: targetInboundTag,
            email: record.email || '',
            client_id: record.auth_uuid || record.auth_password || '',
            enabled,
            restart: false,
          },
        }),
      })
      message.success(action.status === 'running' ? `已通过 WS 下发${enabled ? '启用' : '停用'} Client 任务，结果会回传到操作记录` : `已创建${enabled ? '启用' : '停用'} Client 任务，Client 不在线时会等待轮询执行`)
      await loadXUIActions(targetAgentID, { silent: true })
      scheduleXUIActionResultRefresh(targetAgentID)
      window.setTimeout(() => {
        void loadOverview(targetAgentID, { silent: true })
        if (selectedAgentId && targetAgentID !== selectedAgentId) {
          void loadOverview(selectedAgentId, { silent: true })
        }
        void loadAgents({ silent: true })
      }, 2500)
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      message.error(error instanceof Error ? error.message : `${enabled ? '启用' : '停用'} Client 失败`)
    } finally {
      setXUIClientToggleLoadingKey('')
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

  async function executeRemoteCommand(command: string, shell: string, timeoutSeconds: number, agentID = selectedAgentId) {
    if (!agentID) {
      return
    }
    setRemoteCommandLoading(true)
    try {
      await fetchJSON<XUIAction>(`/api/v1/agents/${agentID}/xui/actions`, {
        method: 'POST',
        body: JSON.stringify({
          kind: 'execute_command',
          payload: {
            command,
            shell,
            timeout_seconds: timeoutSeconds,
          },
        }),
      })
      message.success('命令已下发，Client 会以服务权限执行并回传结果')
      window.setTimeout(() => {
        void loadXUIActions(agentID, { silent: true })
      }, 2500)
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      message.error(error instanceof Error ? error.message : '下发远程命令失败')
    } finally {
      setRemoteCommandLoading(false)
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

  async function openClientInstallModal() {
    setClientInstallModalOpen(true)
    setClientInstallLoading(true)
    try {
      const data = await fetchJSON<ClientInstallInfo>('/api/v1/admin/client-install')
      setClientInstallForm(normalizeClientInstallCommandForm(data))
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      message.error(error instanceof Error ? error.message : '加载 Client 安装信息失败')
    } finally {
      setClientInstallLoading(false)
    }
  }

  async function saveClientInstallSettings() {
    setClientInstallSaving(true)
    try {
      const data = await fetchJSON<ClientInstallInfo>('/api/v1/admin/client-install', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          server_url: clientInstallForm.server_url,
          install_script_url: clientInstallForm.install_script_url,
          poll_interval: clientInstallForm.poll_interval,
          request_timeout_seconds: clientInstallForm.request_timeout_seconds,
          server_skip_tls_verify: clientInstallForm.server_skip_tls_verify,
          xui_auto_install: false,
        }),
      })
      setClientInstallForm(normalizeClientInstallCommandForm(data))
      message.success('Client 安装参数已保存')
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      message.error(error instanceof Error ? error.message : '保存 Client 安装参数失败')
    } finally {
      setClientInstallSaving(false)
    }
  }

  async function openFrontendSettingsModal(showModal = true) {
    if (showModal) {
      setFrontendSettingsModalOpen(true)
    }
    setFrontendSettingsLoading(true)
    try {
      const data = await fetchJSON<FrontendSettings>('/api/v1/admin/frontend-settings')
      setFrontendSettingsForm(normalizeFrontendSettingsForm(data))
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      message.error(error instanceof Error ? error.message : '加载前端样式设置失败')
    } finally {
      setFrontendSettingsLoading(false)
    }
  }

  async function saveFrontendSettings() {
    setFrontendSettingsSaving(true)
    try {
      const data = await fetchJSON<FrontendSettings>('/api/v1/admin/frontend-settings', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(frontendSettingsForm),
      })
      setFrontendSettingsForm(normalizeFrontendSettingsForm(data))
      applyCustomFrontendCode(data.custom_code || '')
      message.success('前端自定义样式已保存')
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      message.error(error instanceof Error ? error.message : '保存前端样式设置失败')
    } finally {
      setFrontendSettingsSaving(false)
    }
  }

  async function loadScheduledTasks() {
    setScheduledTasksLoading(true)
    try {
      const data = await fetchJSON<ScheduledTaskSettings>('/api/v1/admin/scheduled-tasks')
      setScheduledTasks(normalizeScheduledTaskSettings(data))
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      message.error(error instanceof Error ? error.message : '加载定时任务失败')
    } finally {
      setScheduledTasksLoading(false)
    }
  }

  async function saveScheduledTasks() {
    setScheduledTasksSaving(true)
    try {
      const data = await fetchJSON<ScheduledTaskSettings>('/api/v1/admin/scheduled-tasks', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(normalizeScheduledTaskSettings(scheduledTasks)),
      })
      setScheduledTasks(normalizeScheduledTaskSettings(data))
      message.success('定时任务配置已保存')
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      message.error(error instanceof Error ? error.message : '保存定时任务失败')
    } finally {
      setScheduledTasksSaving(false)
    }
  }

  async function loadAccessLogs() {
    setAccessLogsLoading(true)
    try {
      const params = new URLSearchParams()
      Object.entries(accessLogFilters).forEach(([key, value]) => {
        if (String(value || '').trim()) {
          params.set(key, String(value).trim())
        }
      })
      const data = await fetchJSON<AccessLogListResponse>(`/api/v1/admin/access-logs?${params.toString()}`)
      setAccessLogs(data.items || [])
      setAccessLogsTotal(data.total || 0)
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      message.error(error instanceof Error ? error.message : '加载访问日志失败')
    } finally {
      setAccessLogsLoading(false)
    }
  }

  async function copyClientInstallCommand(command = buildClientInstallCommand(clientInstallForm)) {
    if (!clientInstallForm.registration_token.trim()) {
      message.warning('当前 server 未配置注册 Token，安装命令无法完成 Client 注册')
      return
    }
    try {
      await navigator.clipboard.writeText(command)
      message.success('Client 安装命令已复制')
    } catch {
      message.warning('浏览器不允许直接复制，请手动复制命令')
    }
  }

  async function loadTelegramBots(options: LoadOptions = {}) {
    const silent = Boolean(options.silent)
    if (!silent) {
      setTelegramBotsLoading(true)
    }
    try {
      const data = await fetchJSON<TelegramBot[]>('/api/v1/admin/telegram-bots')
      setTelegramBots(Array.isArray(data) ? data : [])
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      } else if (!silent) {
        message.error(error instanceof Error ? error.message : '加载 Telegram 机器人失败')
      }
    } finally {
      if (!silent) {
        setTelegramBotsLoading(false)
      }
    }
  }


  async function loadUpdateLatestInfo(options: LoadOptions = {}) {
    const silent = Boolean(options.silent)
    if (!silent) {
      setUpdateLatestLoading(true)
      setUpdateLatestError('')
    }
    try {
      const data = await fetchJSON<UpdateLatestInfo>('/api/v1/admin/updates/latest')
      setUpdateLatestInfo(data)
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      if (!silent) {
        setUpdateLatestError(error instanceof Error ? error.message : '获取最新版本失败')
      }
    } finally {
      if (!silent) {
        setUpdateLatestLoading(false)
      }
    }
  }

  async function updateServerOnline() {
    if (!updateLatestInfo?.server_update_available) {
      message.info('当前 Server 已是最新版本')
      return
    }
    setUpdateLoading(true)
    try {
      await fetchJSON<UpdateResponse>('/api/v1/admin/updates/server', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ version: updateLatestInfo.latest_server_tag || updateLatestInfo.latest_server_version || updateLatestInfo.latest_tag || updateLatestInfo.latest_version }),
      })
      message.success('Server 升级已启动，服务会自动重启')
      await loadUpdateLatestInfo()
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      message.error(error instanceof Error ? error.message : '启动 Server 升级失败')
    } finally {
      setUpdateLoading(false)
    }
  }

  async function updateAllClientsOnline() {
    if (!updateLatestInfo?.client_update_available_count) {
      message.info('没有需要升级的 Client')
      return
    }
    setUpdateLoading(true)
    try {
      const result = await fetchJSON<UpdateResponse>('/api/v1/admin/updates/clients', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ version: updateLatestInfo.latest_client_tag || updateLatestInfo.latest_client_version || updateLatestInfo.latest_tag || updateLatestInfo.latest_version }),
      })
      message.success(`已下发 Client 升级任务：${result.count || 0} 台，跳过 ${result.skipped || 0} 台`)
      await loadUpdateLatestInfo()
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      message.error(error instanceof Error ? error.message : '下发 Client 升级失败')
    } finally {
      setUpdateLoading(false)
    }
  }

  async function saveTelegramBot() {
    if (!telegramBotForm.name.trim() || !telegramBotForm.chat_id.trim()) {
      message.error('机器人名称和 Chat ID 必填')
      return
    }
    if (!editingTelegramBotId && !telegramBotForm.bot_token.trim()) {
      message.error('新增机器人时 Bot Token 必填')
      return
    }
    setTelegramBotSaving(true)
    try {
      const payload = {
        name: telegramBotForm.name.trim(),
        bot_token: telegramBotForm.bot_token.trim(),
        chat_id: telegramBotForm.chat_id.trim(),
        enabled: telegramBotForm.enabled,
      }
      if (editingTelegramBotId) {
        await fetchJSON<TelegramBot>(`/api/v1/admin/telegram-bots/${editingTelegramBotId}`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        })
        message.success('Telegram 机器人已更新')
      } else {
        await fetchJSON<TelegramBot>('/api/v1/admin/telegram-bots', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        })
        message.success('Telegram 机器人已新增')
      }
      setEditingTelegramBotId(null)
      setTelegramBotForm(defaultTelegramBotForm())
      await loadTelegramBots()
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      message.error(error instanceof Error ? error.message : '保存 Telegram 机器人失败')
    } finally {
      setTelegramBotSaving(false)
    }
  }

  async function deleteTelegramBot(id: number) {
    try {
      await fetchJSON<{ status: string }>(`/api/v1/admin/telegram-bots/${id}`, { method: 'DELETE' })
      message.success('Telegram 机器人已删除')
      await loadTelegramBots()
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      message.error(error instanceof Error ? error.message : '删除 Telegram 机器人失败')
    }
  }

  async function testTelegramBot(id: number) {
    try {
      await fetchJSON<{ status: string }>(`/api/v1/admin/telegram-bots/${id}/test`, { method: 'POST' })
      message.success('测试消息已发送')
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      message.error(error instanceof Error ? error.message : '发送测试消息失败')
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
      message.success('客户端收费已保存')
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

  const filteredClients = currentOverview
    ? currentOverview.clients.filter((client) => {
        if (!deferredClientSearch) {
          return true
        }
        const haystack = [client.email, client.comment, client.inbound_tag, client.inbound_remark, client.inbound_id]
          .filter(Boolean)
          .join(' ')
          .toLowerCase()
        return haystack.includes(deferredClientSearch)
      })
    : []

  const filteredAgents = agents.filter((item) => hasSelectedTag(item.tags, selectedTag))
  const tagFilterOptions = mergeDashboardTagOptions(dashboardView?.tags || [], tagOptions)
  const selectedTagView = selectedTag ? dashboardView?.tags.find((tag) => tag.tag === selectedTag) : undefined
  const scopedAgentCount = dashboardView ? selectedTagView?.agent_count ?? filteredAgents.length : filteredAgents.length
  const scopedNodeCount = dashboardView ? selectedTagView?.node_count ?? dashboardView.totals.node_count : 0
  const onlineAgentCount = filteredAgents.filter(isAgentRunning).length
  const offlineAgentCount = Math.max(scopedAgentCount - onlineAgentCount, 0)
  const xuiErrorAgentCount = filteredAgents.filter((agent) => Boolean(agent.summary.last_collection_err)).length
  const scopedNetwork = summarizeAgentNetwork(filteredAgents)
  const filteredTagLinks = (dashboardView?.links || []).filter((link) => topologyMatchesSelectedTag(link, selectedTag))
  const filteredChains = (dashboardView?.client_chains || []).filter((chain) => chainMatchesSelectedTag(chain, selectedTag))
  const monthlyFinance = canManageSystem
    ? summarizeMonthlyFinance(filteredAgents, filteredChains, costCurrency, exchangeRates, financeCustomers, financeAreaManagers)
    : { costTotal: 0, costCount: 0, revenueTotal: 0, revenueCount: 0, profitTotal: 0, missingCostCount: 0, missingRevenueCount: 0 }
  const accessLogAgentOptions = [
    { value: '', label: '全部 Client' },
    ...agents.map((agent) => ({ value: agent.agent_id, label: agent.agent_name || agent.agent_id })),
  ]
  const accessLogColumns = [
    {
      title: '时间',
      dataIndex: 'created_at',
      width: 180,
      render: (value: string) => formatDateTime(value),
    },
    {
      title: 'Client',
      dataIndex: 'agent_id',
      width: 180,
      render: (value: string, record: AccessLogEntry) => record.agent_name || agents.find((agent) => agent.agent_id === value)?.agent_name || value,
    },
    {
      title: '来源',
      width: 170,
      render: (_: unknown, record: AccessLogEntry) => `${record.source_ip || '-'}${record.source_port ? `:${record.source_port}` : ''}`,
    },
    {
      title: '目标',
      width: 220,
      render: (_: unknown, record: AccessLogEntry) => `${record.target_host || record.target_ip || '-'}${record.target_port ? `:${record.target_port}` : ''}`,
    },
    {
      title: '协议',
      width: 90,
      render: (_: unknown, record: AccessLogEntry) => (record.network || record.protocol || '-').toUpperCase(),
    },
    {
      title: '出站',
      dataIndex: 'outbound_tag',
      width: 140,
      render: (value: string) => value || '-',
    },
    {
      title: '客户端',
      dataIndex: 'client_email',
      width: 180,
      render: (value: string) => value || '-',
    },
    {
      title: '摘要',
      dataIndex: 'raw_summary',
      ellipsis: true,
      render: (value: string) => value || '-',
    },
  ]

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

  const clientInstallCommand = buildClientInstallCommand(clientInstallForm)
  const clientWindowsPowerShellCommand = buildWindowsPowerShellInstallCommand(clientInstallForm)
  const clientWindowsCMDCommand = buildWindowsCMDInstallCommand(clientInstallForm)
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
    setTopologyVisible(false)
    setSelectedAgentId('')
    setActiveTabKey('overview')
    setSelectedOutboundTag('')
    setSelectedRuleIndex(null)
    setSelectedNodeAnchor('')
  }
  const openCustomersPage = () => setActiveAdminPage('customers')
  const openSettingsPage = () => {
    setActiveAdminPage('settings')
    void openFrontendSettingsModal(false)
  }
  const openAccessLogsPage = () => setActiveAdminPage('access-logs')
  const openSchedulesPage = () => setActiveAdminPage('schedules')
  return (
    <div className="page-shell admin-page-shell">
      <VisualEffects disabled={isAreaManagerAccount} />
      <div className="page-background page-background-left" />
      <div className="page-background page-background-right" />
      <div className={`app-shell admin-oa-shell${centerPanelOpen ? ' topology-open-shell' : ''}`}>
        <AdminShellNavigation
          adminUser={adminUser}
          systemInfo={systemInfo}
          canManageSystem={canManageSystem}
          isAreaManagerAccount={isAreaManagerAccount}
          activeAdminPage={activeAdminPage}
          topologyVisible={topologyVisible}
          onlineAgentCount={onlineAgentCount}
          scopedAgentCount={scopedAgentCount}
          agentsLoading={agentsLoading}
          themeMode={themeMode}
          effectiveMode={effectiveMode}
          heroTitle={heroTitle}
          serverVersionLabel={serverVersionLabel}
          onOpenAccount={openAccountModal}
          onOpenClientInstall={() => void openClientInstallModal()}
          onOpenTelegram={() => setTelegramBotModalOpen(true)}
          onOpenCustomers={openCustomersPage}
          onOpenFrontendSettings={openSettingsPage}
          onOpenUpdates={() => setUpdateModalOpen(true)}
          onLogout={() => void logout()}
          onOpenWorkbench={returnHome}
          onOpenAssets={openAssetsPage}
          onOpenTopology={openTopologyPanel}
          onOpenAccessLogs={openAccessLogsPage}
          onOpenSchedules={openSchedulesPage}
          onRefreshAgents={() => void loadAgents()}
          onThemeModeChange={setThemeMode}
        />

        <section className="admin-oa-main">

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
        ) : activeAdminPage === 'access-logs' ? (
          <main className="admin-content-page">
            <Card className="config-section-card" bordered={false}>
              <div className="section-title-row">
                <Title level={4}>访问日志</Title>
                <Space>
                  <Text type="secondary">显示 {accessLogs.length} / {accessLogsTotal}</Text>
                  <Button type="primary" icon={<ReloadOutlined />} loading={accessLogsLoading} onClick={() => void loadAccessLogs()}>查询</Button>
                </Space>
              </div>
              <Row gutter={[12, 12]}>
                <Col xs={24} md={6}>
                  <Text type="secondary">Client</Text>
                  <Select
                    showSearch
                    style={{ width: '100%' }}
                    value={accessLogFilters.agent_id}
                    options={accessLogAgentOptions}
                    optionFilterProp="label"
                    onChange={(value) => setAccessLogFilters((current) => ({ ...current, agent_id: value }))}
                  />
                </Col>
                <Col xs={24} md={5}>
                  <Text type="secondary">来源 IP</Text>
                  <Input value={accessLogFilters.source_ip} onChange={(event) => setAccessLogFilters((current) => ({ ...current, source_ip: event.target.value }))} />
                </Col>
                <Col xs={24} md={5}>
                  <Text type="secondary">目标域名/IP</Text>
                  <Input value={accessLogFilters.target} onChange={(event) => setAccessLogFilters((current) => ({ ...current, target: event.target.value }))} />
                </Col>
                <Col xs={24} md={5}>
                  <Text type="secondary">客户端 Email</Text>
                  <Input value={accessLogFilters.client_email} onChange={(event) => setAccessLogFilters((current) => ({ ...current, client_email: event.target.value }))} />
                </Col>
                <Col xs={24} md={3}>
                  <Text type="secondary">条数</Text>
                  <InputNumber
                    style={{ width: '100%' }}
                    min={20}
                    max={500}
                    precision={0}
                    value={accessLogFilters.limit}
                    onChange={(value) => setAccessLogFilters((current) => ({ ...current, limit: Number(value || 100) }))}
                  />
                </Col>
              </Row>
              <Table
                style={{ marginTop: 16 }}
                rowKey={(record) => String(record.id || `${record.agent_id}-${record.created_at}-${record.raw_summary}`)}
                size="small"
                loading={accessLogsLoading}
                columns={accessLogColumns}
                dataSource={accessLogs}
                pagination={false}
                scroll={{ x: 1200 }}
              />
            </Card>
          </main>
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
              agents={filteredAgents}
              dashboardView={dashboardView}
              selectedTag={selectedTag}
              scopedNetwork={scopedNetwork}
              monthlyFinance={monthlyFinance}
              costCurrency={costCurrency}
              restrictedView={isAreaManagerAccount}
              onSelectAgent={(agentID) => openAgentDetailPanel(agentID)}
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
            financeAreaManagers={financeAreaManagers}
            exchangeRates={exchangeRates}
            selectedTag={selectedTag}
            currentAgentLabel={selectedAgent?.agent_name || selectedAgent?.agent_id || ''}
            currentIPv4={selectedSummary.public_ipv4 || ''}
            compact={centerPanelOpen}
            restrictedView={isAreaManagerAccount}
            onCostCurrencyChange={setCostCurrency}
          />

          <AgentRail
            agents={filteredAgents}
            loading={agentsLoading}
            error={agentsError}
            selectedTag={selectedTag}
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
                xuiClientToggleLoadingKey={xuiClientToggleLoadingKey}
                agentDeleteLoading={agentDeleteLoading}
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
                onDeleteXUIClient={(client) => void deleteXUIClient(client, selectedAgentId)}
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
                onFeatureChange={(feature, enabled) => updateManagedConfig((current) => ({ ...current, features: { ...current.features, [feature]: enabled } }))}
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
