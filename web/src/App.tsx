import { startTransition, useDeferredValue, useEffect, useRef, useState } from 'react'
import {
  Alert,
  App as AntdApp,
  Button,
  Card,
  Input,
  Select,
  Space,
  Spin,
  Switch,
  Typography,
} from 'antd'
import {
  ApartmentOutlined,
  CloudServerOutlined,
  DashboardOutlined,
  DeploymentUnitOutlined,
  ReloadOutlined,
  SettingOutlined,
  TeamOutlined,
} from '@ant-design/icons'

import type {
  AdminAuthResponse,
  AdminUser,
  AreaAgentTagsResponse,
  AgentListItem,
  AgentLogsResponse,
  AgentRefreshResponse,
  AgentRealtimeMetrics,
  ConfigAuditLog,
  CustomerAssignment,
  CustomerAssignmentDraft,
  DashboardAgentView,
  DashboardRealtimeMessage,
  ExchangeRatesResponse,
  FrontendSettings,
  ClientInstallInfo,
  GlobalDashboardView,
  ManagedAgentConfig,
  SystemInfo,
  TelegramBot,
  TagSettingsResponse,
  UpdateLatestInfo,
  UpdateResponse,
  VPSSummary,
  XUIAction,
  XUIClientBillingConfig,
  XUIClientView,
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

import type {
  AgentViewMode,
  ClientInstallCommandForm,
  ClientInstallCommandKind,
  ConfigSectionKey,
  FrontendSettingsForm,
  TelegramBotForm,
  XUIOutboundActionForm,
  XUIRoutingActionForm,
} from './lib/appHelpers'
import {
  FrontendSettingsPanel,
  PersonalCenterDropdown,
} from './components/AdminModals'
import { CustomerManagementModal } from './components/CustomerManagementModal'
import { renderCNFlowPanel } from './components/DashboardTopologyPanels'
import { AgentDetailPanel } from './components/AgentDetailPanel'
import { ConsoleModals } from './components/ConsoleModals'
import { AgentRail, AdminWorkbenchDashboard, OverviewSummaryCard } from './components/DashboardSidebar'
import { CustomerPortal } from './components/CustomerPortal'
import { LoginScreen } from './components/LoginScreen'
import { VisualEffects, applyCustomFrontendCode } from './components/VisualEffects'
import { useAppTheme, type ThemeMode } from './theme'
import {
  DASHBOARD_AUTO_REFRESH_MS,
  buildClientInstallCommand,
  buildDashboardRealtimeURL,
  buildSectionSavePayload,
  buildWindowsCMDInstallCommand,
  buildWindowsPowerShellInstallCommand,
  buildXUIActionPayload,
  chainMatchesSelectedTag,
  configSectionLabel,
  configSignature,
  createEmptyManagedConfig,
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
  nodeElementId,
  outboundElementId,
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
  ruleElementId,
  summarizeAgent,
  summarizeConfigAudit,
  summarizeRule,
  topologyMatchesSelectedTag,
  upsertClientBilling,
} from './lib/appHelpers'

const { Paragraph, Text, Title } = Typography
interface LoadOptions {
  silent?: boolean
}

type AdminPageKey = 'dashboard' | 'assets' | 'customers' | 'settings'

interface AdminRouteState {
  page: AdminPageKey
  topology: boolean
  agentId: string
  tabKey: string
  tag: string
  outboundTag: string
  ruleIndex: number | null
  nodeAnchor: string
  topologySearch: string
}

function hasCustomerDisplayNameField(value: unknown): boolean {
  return Boolean(value && typeof value === 'object' && Object.prototype.hasOwnProperty.call(value, 'customer_display_name'))
}

function parseAdminRouteState(canManageSystem: boolean): AdminRouteState {
  const params = new URLSearchParams(window.location.search)
  const path = window.location.pathname.replace(/\/+$/, '')
  const rawPage = (params.get('page') || pageFromAdminPath(path)).toLowerCase()
  const topology = rawPage === 'topology' || params.get('topology') === '1'
  let page: AdminPageKey = topology ? 'dashboard' : normalizeAdminPage(rawPage)
  if (page === 'settings' && !canManageSystem) {
    page = 'dashboard'
  }

  const ruleParam = Number(params.get('rule') || '')
  return {
    page,
    topology,
    agentId: params.get('agent') || agentFromAdminPath(path),
    tabKey: params.get('tab') || 'overview',
    tag: params.get('tag') || '',
    outboundTag: params.get('outbound') || '',
    ruleIndex: Number.isInteger(ruleParam) && ruleParam > 0 ? ruleParam : null,
    nodeAnchor: params.get('node') || '',
    topologySearch: params.get('q') || '',
  }
}

function buildAdminRouteURL(route: AdminRouteState): string {
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
  }
  const query = params.toString()
  return query ? `/?${query}` : '/'
}

function normalizeAdminPage(value: string): AdminPageKey {
  switch (value) {
    case 'assets':
    case 'customers':
    case 'settings':
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
    case '/admin/settings':
      return 'settings'
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

function sanitizeAreaRealtimeMetric(metric: AgentRealtimeMetrics): AgentRealtimeMetrics {
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

function isAreaManagerAdminUser(user: AdminUser | null): boolean {
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
  const [updateModalOpen, setUpdateModalOpen] = useState(false)
  const [updateLoading, setUpdateLoading] = useState(false)
  const [updateLatestLoading, setUpdateLatestLoading] = useState(false)
  const [updateLatestInfo, setUpdateLatestInfo] = useState<UpdateLatestInfo | null>(null)
  const [updateLatestError, setUpdateLatestError] = useState('')
  const [force3XUIUpdate, setForce3XUIUpdate] = useState(false)
  const [reloadToken, setReloadToken] = useState(0)
  const [activeTabKey, setActiveTabKey] = useState('overview')
  const [topologyVisible, setTopologyVisible] = useState(false)
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
  const [remoteCommandLoading, setRemoteCommandLoading] = useState(false)
  const [xuiActionModalOpen, setXUIActionModalOpen] = useState(false)
  const [xuiActionSaving, setXUIActionSaving] = useState(false)
  const [xuiActionKind, setXUIActionKind] = useState('upsert_routing_rule')
  const [outboundActionForm, setOutboundActionForm] = useState<XUIOutboundActionForm>(() => defaultOutboundActionForm())
  const [routingActionForm, setRoutingActionForm] = useState<XUIRoutingActionForm>(() => defaultRoutingActionForm())
  const [outboundSourceOverview, setOutboundSourceOverview] = useState<XUIOverview | null>(null)
  const [outboundSourceLoading, setOutboundSourceLoading] = useState(false)
  const [clientSearch, setClientSearch] = useState('')
  const [importURLClient, setImportURLClient] = useState<XUIClientView | null>(null)
  const deferredClientSearch = useDeferredValue(clientSearch.trim().toLowerCase())
  const applyingRouteRef = useRef(false)
  const lastAdminURLRef = useRef('')

  const selectedAgent = agents.find((item) => item.agent_id === selectedAgentId)
  const selectedSummary = overview?.summary || selectedAgent?.summary || {}
  const centerPanelOpen = topologyVisible || Boolean(selectedAgent)
  const topologyScopeLabel = selectedAgentId ? selectedAgent?.agent_name || selectedAgentId : selectedTag ? `${selectedTag} 标签` : '全部 Client'
  const heroTitle = '南风VPS监控'
  const customerMode = window.location.pathname.replace(/\/+$/, '') === '/customer'
  const isAreaManagerAccount = isAreaManagerAdminUser(adminUser)
  const canManageSystem = Boolean(adminUser && !isAreaManagerAccount)
  useEffect(() => {
    if (customerMode) {
      setSessionLoading(false)
      return
    }
    void loadSession()
  }, [customerMode])

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
    if (activeAdminPage === 'settings') {
      setActiveAdminPage('dashboard')
    }
    if (['config', 'logs', 'certificates'].includes(activeTabKey)) {
      setActiveTabKey('overview')
    }
  }, [activeAdminPage, activeTabKey, adminUser, canManageSystem])

  useEffect(() => {
    if (customerMode || sessionLoading || !adminUser) {
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
  }, [adminUser, canManageSystem, customerMode, sessionLoading])

  useEffect(() => {
    if (customerMode || sessionLoading || !adminUser) {
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
    if (overview && outboundActionForm.source_agent_id === overview.agent_id) {
      setOutboundSourceOverview(overview)
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
  }, [adminUser, outboundActionForm.source_agent_id, overview])

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
    const silent = Boolean(options.silent)
    if (!silent) {
      setAgentsLoading(true)
      setAgentsError('')
    }
    try {
      const data = await fetchJSON<GlobalDashboardView>('/api/v1/dashboard')
      const sortedAgents = sortAgentsByOrder((data.agents || []).map(applyCachedCustomerDisplayName))
      setDashboardView({ ...data, agents: sortedAgents })
      setAgents(sortedAgents)
      setTagOptions((current) => mergeTagOptions(current, sortedAgents.flatMap((agent) => agent.tags || [])))
      setAgentsError('')
      if (!sortedAgents.length || (selectedAgentId && !sortedAgents.some((item) => item.agent_id === selectedAgentId))) {
        setSelectedAgentId('')
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

  async function loadOverview(agentID: string, options: LoadOptions = {}) {
    const silent = Boolean(options.silent)
    if (!silent) {
      setOverviewLoading(true)
      setOverviewError('')
    }
    try {
      const data = await fetchJSON<XUIOverview>(`/api/v1/agents/${agentID}/xui/overview`)
      setOverview(normalizeXUIOverview(data))
      setOverviewError('')
      setSelectedOutboundTag('')
      setSelectedRuleIndex(null)
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      } else if (!silent) {
        setOverview(null)
        setOverviewError(error instanceof Error ? error.message : '加载 x-ui 概览失败')
      }
    } finally {
      if (!silent) {
        setOverviewLoading(false)
      }
    }
  }

  async function loadManagedConfig(agentID: string, options: LoadOptions = {}) {
    const silent = Boolean(options.silent)
    if (!silent) {
      setConfigLoading(true)
      setConfigError('')
    }
    try {
      const data = await fetchJSON<ManagedAgentConfig>(`/api/v1/agents/${agentID}/config`)
      const cachedName = cachedCustomerDisplayName(agentID)
      const dataWithCustomerName = !hasCustomerDisplayNameField(data) && cachedName !== undefined
        ? { ...data, customer_display_name: cachedName }
        : data
      const normalized = normalizeManagedConfig(dataWithCustomerName, agentID, selectedAgent?.agent_name)
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
      } else if (!silent) {
        const emptyConfig = createEmptyManagedConfig(agentID, selectedAgent?.agent_name)
        managedConfigDirtyRef.current = false
        setSavedManagedConfig(emptyConfig)
        setManagedConfig(emptyConfig)
        setEntryAddressInputText(formatAddressInput(emptyConfig.entry?.addresses))
        setConfigError(error instanceof Error ? error.message : '加载托管配置失败')
      }
    } finally {
      if (!silent) {
        setConfigLoading(false)
      }
    }
  }

  async function loadXUIActions(agentID = selectedAgentId, options: LoadOptions = {}) {
    if (!agentID) {
      setXUIActions([])
      return
    }
    const silent = Boolean(options.silent)
    if (!silent) {
      setXUIActionsLoading(true)
    }
    try {
      const data = await fetchJSON<XUIAction[]>(`/api/v1/agents/${agentID}/xui/actions?limit=30`)
      setXUIActions(Array.isArray(data) ? data : [])
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      } else if (!silent) {
        message.error(error instanceof Error ? error.message : '加载 x-ui 操作记录失败')
      }
    } finally {
      if (!silent) {
        setXUIActionsLoading(false)
      }
    }
  }

  async function loadAgentLogs(agentID = selectedAgentId, options: LoadOptions = {}) {
    if (!agentID) {
      setAgentLogs(null)
      setAgentLogsError('')
      return
    }
    const silent = Boolean(options.silent)
    if (!silent) {
      setAgentLogsLoading(true)
      setAgentLogsError('')
    }
    try {
      const data = await fetchJSON<AgentLogsResponse>(`/api/v1/agents/${agentID}/logs`)
      setAgentLogs({ ...data, logs: Array.isArray(data.logs) ? data.logs : [] })
      setAgentLogsError('')
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      } else if (!silent) {
        setAgentLogs(null)
        setAgentLogsError(error instanceof Error ? error.message : '加载日志失败')
      }
    } finally {
      if (!silent) {
        setAgentLogsLoading(false)
      }
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
      await fetchJSON<{ status: string; agent_id: string }>(`/api/v1/agents/${encodeURIComponent(agentID)}`, {
        method: 'DELETE',
      })
      customerDisplayNameCacheRef.current = Object.fromEntries(
        Object.entries(customerDisplayNameCacheRef.current).filter(([key]) => key !== agentID),
      )
      if (selectedAgentId === agentID) {
        returnHome()
      }
      message.success(`已删除 Client / VPS：${targetName}`)
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
    if (!agentID) {
      return
    }
    const key = xuiClientActionKey(record)
    setXUIClientDeleteLoadingKey(key)
    try {
      const action = await fetchJSON<XUIAction>(`/api/v1/agents/${agentID}/xui/actions`, {
        method: 'POST',
        body: JSON.stringify({
          kind: 'delete_client',
          payload: {
            inbound_id: record.inbound_id,
            inbound_tag: record.inbound_tag || '',
            email: record.email || '',
            client_id: record.auth_uuid || record.auth_password || '',
          },
        }),
      })
      message.success(action.status === 'running' ? '已通过 WS 下发删除 Client 任务，结果会回传到操作记录' : '已创建删除 Client 任务，Client 不在线时会等待轮询执行')
      await loadXUIActions(agentID, { silent: true })
      scheduleXUIActionResultRefresh(agentID)
      window.setTimeout(() => {
        void loadOverview(agentID, { silent: true })
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
      setConfigAudits(Array.isArray(data) ? data : [])
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      } else if (!silent) {
        message.error(error instanceof Error ? error.message : '加载配置修改记录失败')
      }
    } finally {
      if (!silent) {
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

  async function updateAll3XUIOnline(force = force3XUIUpdate) {
    setUpdateLoading(true)
    try {
      let latest: UpdateLatestInfo | null = null
      try {
        latest = await fetchJSON<UpdateLatestInfo>('/api/v1/admin/updates/latest')
        setUpdateLatestInfo(latest)
      } catch (error) {
        if (!force) {
          throw error
        }
        message.warning('检查最新版本失败，将按强制升级模式直接下发 3x-ui update.sh')
      }
      const updateCount = Number(latest?.xui_update_available_count || 0)
      const unknownCount = Number(latest?.unknown_xui_count || 0)
      if (!force && updateCount <= 0 && unknownCount <= 0) {
        message.info(latest?.latest_3xui_error ? `暂时无法检测 3x-ui 最新版本：${latest.latest_3xui_error}` : '没有需要升级的 3x-ui')
        return
      }
      const result = await fetchJSON<UpdateResponse>('/api/v1/admin/updates/3xui', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ version: latest?.latest_3xui_tag || latest?.latest_3xui_version || '', force }),
      })
      message.success(`${force ? '已强制下发' : '已下发'} 3x-ui 升级任务：${result.count || 0} 台，跳过 ${result.skipped || 0} 台`)
      await loadUpdateLatestInfo()
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      message.error(error instanceof Error ? error.message : '下发 3x-ui 升级失败')
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
    if (!selectedAgentId) {
      return
    }
    let payload: Record<string, unknown>
    try {
      payload = buildXUIActionPayload(xuiActionKind, {
        outbound: outboundActionForm,
        routing: routingActionForm,
      })
    } catch (error) {
      message.error(error instanceof Error ? error.message : '操作表单不完整')
      return
    }
    setXUIActionSaving(true)
    try {
      const action = await fetchJSON<XUIAction>(`/api/v1/agents/${selectedAgentId}/xui/actions`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ kind: xuiActionKind, payload }),
      })
      setXUIActionModalOpen(false)
      message.success(action.status === 'running' ? 'x-ui 操作已通过 WS 下发，正在等待实时回传结果' : 'x-ui 操作已创建，Client 不在线时会等待轮询执行')
      await loadXUIActions(selectedAgentId)
      scheduleXUIActionResultRefresh(selectedAgentId)
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

  async function saveManagedConfigSection(section: ConfigSectionKey) {
    if (!selectedAgentId || !managedConfig) {
      return
    }
    const baseConfig = savedManagedConfig || createEmptyManagedConfig(selectedAgentId, selectedAgent?.agent_name)
    const payload = buildSectionSavePayload(baseConfig, managedConfig, section, selectedAgentId)
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
      const nextDraft = mergeSavedSectionIntoDraft(managedConfig, normalized, section)
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
        message.success(`${configSectionLabel(section)}已保存，client 下一次轮询会自动生效`)
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

  async function saveClientBilling(record: XUIClientView) {
    if (!selectedAgentId || !managedConfig) {
      return
    }
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
        void loadManagedConfig(selectedAgentId, { silent: true })
        void loadXUIActions(selectedAgentId, { silent: true })
        void loadAgentLogs(selectedAgentId, { silent: true })
      }
    }, DASHBOARD_AUTO_REFRESH_MS)
    return () => {
      window.clearInterval(timer)
    }
  }, [adminUser, selectedAgentId])

  const filteredClients = overview
    ? overview.clients.filter((client) => {
        if (!deferredClientSearch) {
          return true
        }
        const haystack = [client.email, client.comment, client.inbound_tag, client.inbound_remark]
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
  const monthlyFinance = canManageSystem
    ? summarizeMonthlyFinance(filteredAgents, costCurrency, exchangeRates)
    : { costTotal: 0, costCount: 0, revenueTotal: 0, revenueCount: 0, profitTotal: 0, missingCostCount: 0, missingRevenueCount: 0 }
  const filteredTagLinks = (dashboardView?.links || []).filter((link) => topologyMatchesSelectedTag(link, selectedTag))
  const filteredChains = (dashboardView?.client_chains || []).filter((chain) => chainMatchesSelectedTag(chain, selectedTag))

  if (customerMode) {
    return <CustomerPortal />
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
  const showWorkbenchDashboard = activeAdminPage === 'dashboard' && !topologyVisible
  const serverVersionLabel = `V${systemInfo?.version || '-'}`
  return (
    <div className="page-shell admin-page-shell">
      <VisualEffects disabled={isAreaManagerAccount} />
      <div className="page-background page-background-left" />
      <div className="page-background page-background-right" />
      <div className={`app-shell admin-oa-shell${centerPanelOpen ? ' topology-open-shell' : ''}`}>
        <header className="admin-mobile-header">
          <div className="admin-mobile-brand-row">
            <div className="admin-mobile-brand">
              <span className="admin-oa-brand-mark">南</span>
              <div>
                <strong>南风VPS监控</strong>
                <small>
                  在线 {onlineAgentCount}/{scopedAgentCount} · v{systemInfo?.version || '-'}
                </small>
              </div>
            </div>
            <PersonalCenterDropdown
              adminUser={adminUser}
              systemInfo={systemInfo}
              canManageSystem={canManageSystem}
              onOpenAccount={() => {
                setAccountForm({
                  current_password: '',
                  new_username: adminUser.username,
                  new_password: '',
                  confirm_password: '',
                  avatar_url: adminUser.avatar_url || '',
                })
                setAccountModalOpen(true)
              }}
              onOpenClientInstall={() => void openClientInstallModal()}
              onOpenTelegram={() => setTelegramBotModalOpen(true)}
              onOpenCustomers={() => setActiveAdminPage('customers')}
              onOpenFrontendSettings={() => {
                setActiveAdminPage('settings')
                void openFrontendSettingsModal(false)
              }}
              onOpenUpdates={() => setUpdateModalOpen(true)}
              onLogout={() => void logout()}
            />
          </div>
          <nav className="admin-mobile-nav" aria-label="移动端管理导航">
            <button type="button" className={activeAdminPage === 'dashboard' && !topologyVisible ? 'active' : ''} onClick={() => {
              setActiveAdminPage('dashboard')
              returnHome()
            }}>
              <DashboardOutlined />
              <span>工作台</span>
            </button>
            <button type="button" className={activeAdminPage === 'assets' ? 'active' : ''} onClick={() => {
              setActiveAdminPage('assets')
              setTopologyVisible(false)
              setSelectedAgentId('')
              setActiveTabKey('overview')
              setSelectedOutboundTag('')
              setSelectedRuleIndex(null)
              setSelectedNodeAnchor('')
            }}>
              <CloudServerOutlined />
              <span>资产</span>
            </button>
            <button type="button" className={activeAdminPage === 'dashboard' && topologyVisible ? 'active' : ''} onClick={() => {
              setActiveAdminPage('dashboard')
              openTopologyPanel()
            }}>
              <ApartmentOutlined />
              <span>拓扑</span>
            </button>
            <button type="button" className={activeAdminPage === 'customers' ? 'active' : ''} onClick={() => setActiveAdminPage('customers')}>
              <TeamOutlined />
              <span>用户</span>
            </button>
            {canManageSystem ? <button type="button" className={activeAdminPage === 'settings' ? 'active' : ''} onClick={() => {
              setActiveAdminPage('settings')
              void openFrontendSettingsModal(false)
            }}>
              <SettingOutlined />
              <span>设置</span>
            </button> : null}
          </nav>
          <div className="admin-mobile-actions">
            <Button size="small" icon={<ReloadOutlined />} loading={agentsLoading} onClick={() => void loadAgents()}>刷新</Button>
            {canManageSystem ? <Button size="small" icon={<DeploymentUnitOutlined />} onClick={() => void openClientInstallModal()}>安装 Client</Button> : null}
            <Select
              size="small"
              value={themeMode}
              options={[
                { value: 'system', label: '跟随系统' },
                { value: 'light', label: '明亮' },
                { value: 'dark', label: '暗黑' },
              ]}
              onChange={(value) => setThemeMode(value as ThemeMode)}
            />
          </div>
        </header>
        <aside className="admin-oa-sider">
          <div className="admin-oa-brand">
            <span className="admin-oa-brand-mark">南</span>
            <div>
              <strong>南风VPS监控</strong>
              <small>{serverVersionLabel}</small>
            </div>
          </div>
          <nav className="admin-oa-nav" aria-label="管理端导航">
            <button type="button" className={`admin-oa-nav-item${activeAdminPage === 'dashboard' && !topologyVisible ? ' active' : ''}`} onClick={() => {
              setActiveAdminPage('dashboard')
              returnHome()
            }}>
              <DashboardOutlined />
              <span>工作台</span>
              <small>{isAreaManagerAccount ? '授权总览' : '总览与财务'}</small>
            </button>
            <button type="button" className={`admin-oa-nav-item${activeAdminPage === 'assets' ? ' active' : ''}`} onClick={() => {
              setActiveAdminPage('assets')
              setTopologyVisible(false)
              setSelectedAgentId('')
              setActiveTabKey('overview')
              setSelectedOutboundTag('')
              setSelectedRuleIndex(null)
              setSelectedNodeAnchor('')
            }}>
              <CloudServerOutlined />
              <span>Client 资产</span>
              <small>节点列表</small>
            </button>
            <button type="button" className={`admin-oa-nav-item${activeAdminPage === 'dashboard' && topologyVisible ? ' active' : ''}`} onClick={() => {
              setActiveAdminPage('dashboard')
              openTopologyPanel()
            }}>
              <ApartmentOutlined />
              <span>拓扑图</span>
              <small>链路联动</small>
            </button>
            <button type="button" className={`admin-oa-nav-item${activeAdminPage === 'customers' ? ' active' : ''}`} onClick={() => setActiveAdminPage('customers')}>
              <TeamOutlined />
              <span>用户管理</span>
              <small>账号与授权</small>
            </button>
            {canManageSystem ? <button type="button" className={`admin-oa-nav-item${activeAdminPage === 'settings' ? ' active' : ''}`} onClick={() => {
              setActiveAdminPage('settings')
              void openFrontendSettingsModal(false)
            }}>
              <SettingOutlined />
              <span>系统设置</span>
              <small>样式与升级</small>
            </button> : null}
          </nav>
          <div className="admin-oa-sider-foot">
            <span>在线 Client</span>
            <strong>{onlineAgentCount}/{scopedAgentCount}</strong>
            <small className="admin-oa-sider-version-row">
              <span>Server v{systemInfo?.version || '-'}</span>
            </small>
          </div>
        </aside>

        <section className="admin-oa-main">
        <header className="hero-panel admin-oa-topbar">
          <div className="admin-oa-titlebar">
            <div className="eyebrow">{serverVersionLabel} / 工作台</div>
            <Title level={1}>{heroTitle}</Title>
            <Paragraph className="hero-copy">
              {isAreaManagerAccount
                ? '管理已授权 Client、用户账号、区域标签与可见拓扑链路。'
                : '统一管理 Client、x-ui 托管配置、用户账号、财务月览与跨 Client 拓扑联动。'}
            </Paragraph>
          </div>
          <div className="hero-actions hero-actions-column">
            <Button icon={<ReloadOutlined />} loading={agentsLoading} onClick={() => void loadAgents()}>刷新</Button>
            {canManageSystem ? <Button icon={<DeploymentUnitOutlined />} onClick={() => void openClientInstallModal()}>安装 Client</Button> : null}
            <Button icon={<TeamOutlined />} onClick={() => setActiveAdminPage('customers')}>用户</Button>
            <PersonalCenterDropdown
              adminUser={adminUser}
              systemInfo={systemInfo}
              canManageSystem={canManageSystem}
              onOpenAccount={() => {
                setAccountForm({
                  current_password: '',
                  new_username: adminUser.username,
                  new_password: '',
                  confirm_password: '',
                  avatar_url: adminUser.avatar_url || '',
                })
                setAccountModalOpen(true)
              }}
              onOpenClientInstall={() => void openClientInstallModal()}
              onOpenTelegram={() => setTelegramBotModalOpen(true)}
              onOpenCustomers={() => setActiveAdminPage('customers')}
              onOpenFrontendSettings={() => {
                setActiveAdminPage('settings')
                void openFrontendSettingsModal(false)
              }}
              onOpenUpdates={() => setUpdateModalOpen(true)}
              onLogout={() => void logout()}
            />
            <div className="theme-mode-row">
              <Text type="secondary">主题</Text>
              <Select
                size="small"
                value={themeMode}
                options={[
                  { value: 'system', label: `跟随系统（${effectiveMode === 'dark' ? '暗黑' : '明亮'}）` },
                  { value: 'light', label: '明亮' },
                  { value: 'dark', label: '暗黑' },
                ]}
                onChange={(value) => setThemeMode(value as ThemeMode)}
              />
            </div>
          </div>
        </header>

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
          force3XUIUpdate={force3XUIUpdate}
          xuiActionKind={xuiActionKind}
          xuiActionModalOpen={xuiActionModalOpen}
          xuiActionSaving={xuiActionSaving}
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
          onCloseXUIActionModal={() => setXUIActionModalOpen(false)}
          onConfigChanged={() => loadAgents()}
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
          onUpdateAll3XUI={() => void updateAll3XUIOnline(force3XUIUpdate)}
          onForce3XUIUpdateChange={setForce3XUIUpdate}
          onUpdateFrontendSettingsFormChange={setFrontendSettingsForm}
          onUpdateOutboundActionForm={setOutboundActionForm}
          onUpdateRoutingActionForm={setRoutingActionForm}
          onUpdateServer={() => void updateServerOnline()}
          onXUIActionKindChange={setXUIActionKind}
        />

        {activeAdminPage === 'customers' ? (
          <main className="admin-content-page">
            <CustomerManagementModal
              embedded
              agents={agents}
              adminUser={adminUser}
              initialAssignment={customerAssignmentDraft}
              onInitialAssignmentApplied={() => setCustomerAssignmentDraft(null)}
              onConfigChanged={() => loadAgents()}
              onOpenAssignment={openCustomerAssignment}
            />
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
                {renderCNFlowPanel({
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
              <AgentDetailPanel
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
                currencyOptions={currencyOptions}
                currentAgentLoading={overviewLoading || configLoading || agentRefreshLoading}
                xuiRestartLoading={xuiRestartLoading}
                xuiUpdateLoading={xuiUpdateLoading}
                remoteCommandLoading={remoteCommandLoading}
                dashboardView={dashboardView}
                entryAddressInputText={entryAddressInputText}
                filteredAgents={filteredAgents}
                filteredClients={filteredClients}
                filteredTagLinks={filteredTagLinks}
                managedConfig={managedConfig}
                newTagName={newTagName}
                overview={overview}
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
                agentDeleteLoading={agentDeleteLoading}
                onActiveTabChange={setActiveTabKey}
                onClientSearchChange={setClientSearch}
                onCopyImportURL={(client) => void copyImportURL(client)}
                onCreateRoutingAction={() => {
                  setXUIActionKind('upsert_routing_rule')
                  setOutboundActionForm(defaultOutboundActionForm())
                  setRoutingActionForm(defaultRoutingActionForm())
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
                onRefreshCurrentAgent={() => void requestAgentSnapshot(selectedAgentId)}
                onRestartXUI={() => void restartXUIService(selectedAgentId)}
                onUpdate3XUI={() => void update3XUI(selectedAgentId)}
                onExecuteRemoteCommand={(command, shell, timeoutSeconds) => void executeRemoteCommand(command, shell, timeoutSeconds, selectedAgentId)}
                onRefreshXUIActions={() => void loadXUIActions()}
                onRenewalChange={(patch) => updateManagedConfig((current) => ({ ...current, renewal: { ...current.renewal, ...patch } }))}
                onReturnHome={returnHome}
                onSaveClientBilling={(record) => void saveClientBilling(record)}
                onSaveManagedConfigSection={(section) => void saveManagedConfigSection(section)}
                onSelectTag={selectDashboardTag}
                onTagsChange={(values) => {
                  setTagOptions((current) => mergeTagOptions(current, values))
                  updateManagedConfig((current) => ({ ...current, tags: values }))
                }}
                onSaveAreaTags={(values) => void saveAreaAgentTags(values)}
                onUpdateClientBillingDraft={updateClientBillingDraft}
                onXUIChange={(patch) => updateManagedConfig((current) => ({ ...current, xui: { ...current.xui, ...patch } }))}
              />
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

  function applyAdminRoute(route: AdminRouteState) {
    setActiveAdminPage(route.page)
    setTopologyVisible(route.topology)
    setSelectedTag(route.tag)
    setTopologySearch(route.topologySearch)
    setSelectedOutboundTag(route.outboundTag)
    setSelectedRuleIndex(route.ruleIndex)
    setSelectedNodeAnchor(route.nodeAnchor)
    setClientSearch('')

    if (route.page === 'assets' || route.topology) {
      setSelectedAgentId(route.agentId)
      setActiveTabKey(route.topology ? 'overview' : route.tabKey || 'overview')
      return
    }

    setSelectedAgentId('')
    setActiveTabKey('overview')
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

  function openTopologyPanel() {
    setActiveAdminPage('dashboard')
    setTopologyVisible(true)
    window.setTimeout(() => {
      document.getElementById('topology-panel')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
    }, 80)
  }

  function selectDashboardTag(tag: string) {
    setSelectedTag(tag)
    if (topologyVisible) {
      setSelectedAgentId('')
      setSelectedNodeAnchor('')
      setSelectedOutboundTag('')
      setSelectedRuleIndex(null)
    }
  }

  function selectTopologyAgent(agentID: string) {
    setTopologyVisible(true)
    setSelectedNodeAnchor('')
    setSelectedOutboundTag('')
    setSelectedRuleIndex(null)
    startTransition(() => {
      setSelectedAgentId(agentID)
    })
    window.setTimeout(() => {
      document.getElementById('topology-panel')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
    }, 80)
  }

  function returnHome() {
    setActiveAdminPage('dashboard')
    setTopologyVisible(false)
    setSelectedAgentId('')
    setActiveTabKey('overview')
    setSelectedOutboundTag('')
    setSelectedRuleIndex(null)
    setSelectedNodeAnchor('')
  }

  function openAgentDetailPanel(agentID: string, tabKey = 'overview') {
    setActiveAdminPage('assets')
    setTopologyVisible(false)
    setActiveTabKey(tabKey)
    startTransition(() => {
      setSelectedAgentId(agentID)
    })
    window.setTimeout(() => {
      document.getElementById('agent-detail-panel')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
    }, 80)
  }

  function openCustomerAssignment(assignment: CustomerAssignment) {
    if (!assignment.agent_id) {
      return
    }
    setCustomerModalOpen(false)
    setActiveAdminPage('assets')
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

    startTransition(() => {
      setSelectedAgentId(assignment.agent_id)
    })

    let attempts = 0
    const scrollToTarget = () => {
      attempts += 1
      const anchor = assignment.client_email ? 'agent-detail-panel' : nodeElementId(assignment.agent_id, assignment.inbound_tag || String(assignment.inbound_id))
      const element = document.getElementById(anchor) || document.getElementById('agent-detail-panel')
      if (element) {
        element.scrollIntoView({ behavior: 'smooth', block: assignment.client_email ? 'start' : 'center' })
        return
      }
      if (attempts < 20) {
        window.setTimeout(scrollToTarget, 120)
      }
    }
    window.setTimeout(scrollToTarget, 80)
  }

  function openCustomerAuthorization(draft: CustomerAssignmentDraft) {
    setCustomerModalOpen(false)
    setTopologyVisible(false)
    setSelectedOutboundTag('')
    setSelectedRuleIndex(null)
    setCustomerAssignmentDraft(draft)
    setActiveAdminPage('customers')
    window.setTimeout(() => {
      document.getElementById('customer-management-panel')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
    }, 80)
  }

  function jumpToOutbound(tag?: string) {
    if (!tag) {
      return
    }
    setSelectedOutboundTag(tag)
    setActiveTabKey('outbounds')
    window.setTimeout(() => {
      document.getElementById(outboundElementId(tag))?.scrollIntoView({ behavior: 'smooth', block: 'center' })
    }, 60)
  }

  function jumpToRule(index?: number) {
    if (!index) {
      return
    }
    setSelectedRuleIndex(index)
    setActiveTabKey('routes')
    window.setTimeout(() => {
      document.getElementById(ruleElementId(index))?.scrollIntoView({ behavior: 'smooth', block: 'center' })
    }, 60)
  }

  function jumpToNode(agentID?: string, nodeLabel?: string) {
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
      startTransition(() => {
        setSelectedAgentId(agentID)
      })
      window.setTimeout(() => {
        document.getElementById('topology-panel')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
      }, 80)
      return
    }
    setActiveAdminPage('assets')
    setTopologyVisible(false)
    setSelectedAgentId(agentID)
    setActiveTabKey('nodes')

    let attempts = 0
    const scrollToNode = () => {
      attempts += 1
      const element = document.getElementById(anchor)
      if (element) {
        element.scrollIntoView({ behavior: 'smooth', block: 'center' })
        return
      }
      if (attempts < 20) {
        window.setTimeout(scrollToNode, 120)
      }
    }
    window.setTimeout(scrollToNode, 80)
  }
}
