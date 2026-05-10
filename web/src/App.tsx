import { startTransition, useDeferredValue, useEffect, useRef, useState } from 'react'
import {
  Alert,
  App as AntdApp,
  Badge,
  Button,
  Card,
  Empty,
  Input,
  InputNumber,
  Select,
  Space,
  Spin,
  Switch,
  Table,
  Tabs,
  Tag,
  Typography,
} from 'antd'
import type { ColumnsType } from 'antd/es/table'
import {
  EditOutlined,
  ReloadOutlined,
  SettingOutlined,
} from '@ant-design/icons'

import type {
  AdminAuthResponse,
  AdminUser,
  AgentListItem,
  AgentLogEntry,
  AgentLogsResponse,
  AgentRealtimeMetrics,
  ClientInstallInfo,
  ConfigAuditLog,
  DashboardAgentView,
  DashboardRealtimeMessage,
  ExchangeRatesResponse,
  FrontendSettings,
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
  XUILocalCertificate,
  XUINodeView,
  XUIOverview,
  XUIRoutingRuleView,
} from './types'
import {
  COMMON_COST_CURRENCIES,
  COST_CURRENCY_STORAGE_KEY,
  DEFAULT_COST_CURRENCY,
  REVENUE_CURRENCIES,
  type CurrencyCode,
  type ExchangeRatesState,
  defaultExchangeRatesState,
  mergeCurrencyOptions,
  normalizeCurrencyCode,
  readStoredCostCurrency,
  summarizeMonthlyFinance,
} from './lib/currency'
import {
  bytesToGB,
  clientTrafficTotal,
  formatBytes,
  gbToBytes,
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
  AccountSettingsModal,
  ClientInstallModal,
  FrontendSettingsModal,
  ImportURLModal,
  PersonalCenterDropdown,
  SystemUpdateModal,
  TelegramBotSettingsModal,
  XUIActionModal,
} from './components/AdminModals'
import { renderCNFlowPanel, renderGlobalOverviewPanel } from './components/DashboardTopologyPanels'
import { AgentRail, OverviewSummaryCard } from './components/DashboardSidebar'
import { CustomerManagementModal } from './components/CustomerManagementModal'
import { CustomerPortal } from './components/CustomerPortal'
import { LoginScreen } from './components/LoginScreen'
import { ManagedConfigPanel as renderManagedConfigPanel } from './components/ManagedConfigPanel'
import { RouteBadge } from './components/RouteBadge'
import { VisualEffects, applyCustomFrontendCode } from './components/VisualEffects'
import { useAppTheme, type ThemeMode } from './theme'
import {
  DASHBOARD_AUTO_REFRESH_MS,
  actionKindLabel,
  actionStatusColor,
  actionStatusLabel,
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
  formatDateInputFromMillis,
  formatDateTime,
  formatExpiryTime,
  formatRelativeTime,
  dateInputToExpiryMillis,
  formatRenewalHint,
  formatTagInput,
  hasSelectedTag,
  isAgentRunning,
  isClientOnline,
  isUnauthorized,
  mergeDashboardTagOptions,
  mergeRealtimeMetricsIntoAgents,
  mergeRealtimeSummary,
  mergeSavedSectionIntoDraft,
  mergeTagOptions,
  nodeElementId,
  normalizeEntryConfig,
  normalizeClientInstallCommandForm,
  normalizeFrontendSettingsForm,
  normalizeManagedConfig,
  normalizeNodeAnchorLabel,
  normalizeXUIOverview,
  outboundElementId,
  parseAddressInput,
  parseTagInput,
  readStoredAgentViewMode,
  ruleElementId,
  scopeColor,
  scopeLabel,
  shortJSON,
  sortAgentsByOrder,
  statusColor,
  storeAgentViewMode,
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
  const [agentViewMode, setAgentViewMode] = useState<AgentViewMode>('card')
  const [selectedAgentId, setSelectedAgentId] = useState('')
  const [overview, setOverview] = useState<XUIOverview | null>(null)
  const [overviewLoading, setOverviewLoading] = useState(false)
  const [overviewError, setOverviewError] = useState('')
  const [managedConfig, setManagedConfig] = useState<ManagedAgentConfig | null>(null)
  const [savedManagedConfig, setSavedManagedConfig] = useState<ManagedAgentConfig | null>(null)
  const managedConfigDirtyRef = useRef(false)
  const [tagInputText, setTagInputText] = useState('')
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
  const [xuiActionModalOpen, setXUIActionModalOpen] = useState(false)
  const [xuiActionSaving, setXUIActionSaving] = useState(false)
  const [xuiActionKind, setXUIActionKind] = useState('add_outbound')
  const [outboundActionForm, setOutboundActionForm] = useState<XUIOutboundActionForm>(() => defaultOutboundActionForm())
  const [routingActionForm, setRoutingActionForm] = useState<XUIRoutingActionForm>(() => defaultRoutingActionForm())
  const [outboundSourceOverview, setOutboundSourceOverview] = useState<XUIOverview | null>(null)
  const [outboundSourceLoading, setOutboundSourceLoading] = useState(false)
  const [clientSearch, setClientSearch] = useState('')
  const [importURLClient, setImportURLClient] = useState<XUIClientView | null>(null)
  const deferredClientSearch = useDeferredValue(clientSearch.trim().toLowerCase())

  const selectedAgent = agents.find((item) => item.agent_id === selectedAgentId)
  const selectedSummary = overview?.summary || selectedAgent?.summary || {}
  const centerPanelOpen = topologyVisible || Boolean(selectedAgent)
  const topologyScopeLabel = selectedAgentId ? selectedAgent?.agent_name || selectedAgentId : selectedTag ? `${selectedTag} 标签` : '全部 Client'
  const heroTitle = '南风VPS监控'
  const customerMode = window.location.pathname.replace(/\/+$/, '') === '/customer'
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
      void loadTelegramBots()
      void loadTagSettings()
      void loadExchangeRates()
    }
  }, [adminUser])

  useEffect(() => {
    if (adminUser && updateModalOpen) {
      void loadUpdateLatestInfo()
    }
  }, [adminUser, updateModalOpen])

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
      setTagInputText('')
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
    const visibleAgents = agents.filter((item) => hasSelectedTag(item.tags, selectedTag))
    if (!visibleAgents.length || (selectedAgentId && !visibleAgents.some((item) => item.agent_id === selectedAgentId))) {
      setSelectedAgentId('')
    }
  }, [agents, selectedTag, selectedAgentId])

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
      const sortedAgents = sortAgentsByOrder(data.agents || [])
      setDashboardView({ ...data, agents: sortedAgents })
      setAgents(sortedAgents)
      setTagOptions((current) => mergeTagOptions(current, sortedAgents.flatMap((agent) => agent.tags || [])))
      setAgentsError('')
      const filtered = sortedAgents.filter((item) => hasSelectedTag(item.tags, selectedTag))
      if (!filtered.length || (selectedAgentId && !filtered.some((item) => item.agent_id === selectedAgentId))) {
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
    setAgents((current) => mergeRealtimeMetricsIntoAgents(current, metrics))
    setDashboardView((current) =>
      current
        ? {
            ...current,
            agents: mergeRealtimeMetricsIntoAgents(current.agents, metrics),
          }
        : current,
    )
    setOverview((current) => {
      if (!current) {
        return current
      }
      const metric = metrics.find((item) => item.agent_id === current.agent_id)
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
      const normalized = normalizeManagedConfig(data, agentID, selectedAgent?.agent_name)
      setSavedManagedConfig(normalized)
      if (silent && managedConfigDirtyRef.current) {
        setConfigError('')
        return
      }
      managedConfigDirtyRef.current = false
      setManagedConfig(normalized)
      setTagInputText(formatTagInput(normalized.tags))
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
        setTagInputText(formatTagInput(emptyConfig.tags))
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

  async function createTagOption() {
    const parsed = parseTagInput(newTagName)
    if (!parsed.length) {
      message.warning('请输入标签名称')
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

  async function openFrontendSettingsModal() {
    setFrontendSettingsModalOpen(true)
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


  async function loadUpdateLatestInfo() {
    setUpdateLatestLoading(true)
    setUpdateLatestError('')
    try {
      const data = await fetchJSON<UpdateLatestInfo>('/api/v1/admin/updates/latest')
      setUpdateLatestInfo(data)
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      setUpdateLatestError(error instanceof Error ? error.message : '获取最新版本失败')
    } finally {
      setUpdateLatestLoading(false)
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
      message.success('Server 更新已启动，服务会自动重启')
      await loadUpdateLatestInfo()
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      message.error(error instanceof Error ? error.message : '启动 Server 更新失败')
    } finally {
      setUpdateLoading(false)
    }
  }

  async function updateAllClientsOnline() {
    if (!updateLatestInfo?.client_update_available_count) {
      message.info('没有需要更新的 Client')
      return
    }
    setUpdateLoading(true)
    try {
      const result = await fetchJSON<UpdateResponse>('/api/v1/admin/updates/clients', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ version: updateLatestInfo.latest_client_tag || updateLatestInfo.latest_client_version || updateLatestInfo.latest_tag || updateLatestInfo.latest_version }),
      })
      message.success(`已下发 Client 更新任务：${result.count || 0} 台，跳过 ${result.skipped || 0} 台`)
      await loadUpdateLatestInfo()
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      message.error(error instanceof Error ? error.message : '下发 Client 更新失败')
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
      await fetchJSON<XUIAction>(`/api/v1/agents/${selectedAgentId}/xui/actions`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ kind: xuiActionKind, payload }),
      })
      setXUIActionModalOpen(false)
      message.success('x-ui 操作已下发，client 下一次轮询会自动执行')
      await loadXUIActions(selectedAgentId)
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      message.error(error instanceof Error ? error.message : '创建 x-ui 操作失败')
    } finally {
      setXUIActionSaving(false)
    }
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
      const normalized = normalizeManagedConfig(saved, selectedAgentId, saved.agent_name || selectedAgent?.agent_name)
      setSavedManagedConfig(normalized)
      const nextDraft = mergeSavedSectionIntoDraft(managedConfig, normalized, section)
      managedConfigDirtyRef.current = configSignature(nextDraft) !== configSignature(normalized)
      setManagedConfig(nextDraft)
      if (section === 'client') {
        setTagInputText(formatTagInput(normalized.tags))
        setTagOptions((current) => mergeTagOptions(current, normalized.tags || []))
      }
      if (section === 'entry') {
        setEntryAddressInputText(formatAddressInput(normalized.entry?.addresses))
      }
      message.success(`${configSectionLabel(section)}已保存，client 下一次轮询会自动生效`)
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
      const normalized = normalizeManagedConfig(saved, selectedAgentId, saved.agent_name || selectedAgent?.agent_name)
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
  const monthlyFinance = summarizeMonthlyFinance(filteredAgents, costCurrency, exchangeRates)
  const filteredTagLinks = (dashboardView?.links || []).filter((link) => topologyMatchesSelectedTag(link, selectedTag))
  const filteredChains = (dashboardView?.client_chains || []).filter((chain) => chainMatchesSelectedTag(chain, selectedTag))

  const nodeColumns: ColumnsType<XUINodeView> = [
    {
      title: '节点',
      key: 'node',
      width: 240,
      render: (_, record) => (
        <div>
          <Text strong>{record.remark || record.tag || `Node #${record.id}`}</Text>
          <div className="muted-line">{record.tag || '-'}</div>
        </div>
      ),
    },
    {
      title: '协议',
      dataIndex: 'protocol',
      width: 110,
      render: (value?: string) => <Tag color="cyan">{value || '-'}</Tag>,
    },
    {
      title: '端口',
      dataIndex: 'port',
      width: 90,
      render: (value?: number) => value || '-',
    },
    {
      title: '客户端',
      dataIndex: 'client_count',
      width: 100,
      render: (value?: number) => value ?? 0,
    },
    {
      title: '近 5 分钟在线',
      dataIndex: 'online_count',
      width: 130,
      render: (value?: number) => (
        <Badge color={value ? '#0f766e' : '#94a3b8'} text={String(value ?? 0)} />
      ),
    },
    {
      title: '累计流量',
      key: 'traffic',
      width: 130,
      render: (_, record) => formatBytes(record.all_time || record.total || 0),
    },
    {
      title: '路由指向',
      key: 'route',
      width: 280,
      render: (_, record) => (
        <RouteBadge route={record.route} onJumpOutbound={jumpToOutbound} onJumpRule={jumpToRule} />
      ),
    },
  ]

  const clientColumns: ColumnsType<XUIClientView> = [
    {
      title: '客户端',
      key: 'client',
      width: 260,
      render: (_, record) => (
        <div>
          <Text strong>{record.email || '-'}</Text>
          <div className="muted-line">{record.comment || record.sub_id || '未备注'}</div>
        </div>
      ),
    },
    {
      title: '所在节点',
      key: 'inbound',
      width: 180,
      render: (_, record) => (
        <div>
          <Text>{record.inbound_remark || record.inbound_tag || '-'}</Text>
          <div className="muted-line">{record.inbound_tag || '-'}</div>
        </div>
      ),
    },
    {
      title: '协议',
      dataIndex: 'protocol',
      width: 100,
      render: (value?: string) => <Tag color="geekblue">{value || '-'}</Tag>,
    },
    {
      title: '状态',
      key: 'status',
      width: 120,
      render: (_, record) => (
        <Space wrap size={[6, 6]}>
          <Tag color={record.enabled ? 'success' : 'default'}>{record.enabled ? '启用' : '停用'}</Tag>
          <Tag color={isClientOnline(record.last_online, overview?.reported_at) ? 'processing' : 'default'}>
            {isClientOnline(record.last_online, overview?.reported_at) ? '在线' : '离线'}
          </Tag>
        </Space>
      ),
    },
    {
      title: '总 / 上传 / 下载',
      key: 'traffic',
      width: 170,
      render: (_, record) => {
        const up = Number(record.up || 0)
        const down = Number(record.down || 0)
        const total = clientTrafficTotal(record)
        return (
          <div className="client-traffic-cell">
            <span>总 {formatBytes(total)}</span>
            <span>上传 {formatBytes(up)}</span>
            <span>下载 {formatBytes(down)}</span>
          </div>
        )
      },
    },
    {
      title: '收费',
      key: 'billing',
      width: 300,
      render: (_, record) => {
        const billing = findClientBilling(managedConfig?.renewal?.client_billings, record) || defaultClientBilling(record)
        return (
          <Space wrap size={[6, 6]}>
            <InputNumber
              size="small"
              min={0}
              precision={2}
              style={{ width: 92 }}
              value={billing.revenue_amount || 0}
              onChange={(value) => updateClientBillingDraft(record, { revenue_amount: Number(value || 0) })}
            />
            <Select
              size="small"
              style={{ width: 78 }}
              value={billing.revenue_currency || 'CNY'}
              options={REVENUE_CURRENCIES.map((currency) => ({ value: currency, label: currency }))}
              onChange={(value) => updateClientBillingDraft(record, { revenue_currency: value as 'CNY' | 'USDT' })}
            />
            <Select
              size="small"
              style={{ width: 78 }}
              value={billing.revenue_cycle || 'month'}
              options={[
                { value: 'month', label: '月' },
                { value: 'quarter', label: '季' },
                { value: 'year', label: '年' },
              ]}
              onChange={(value) => updateClientBillingDraft(record, { revenue_cycle: value as 'month' | 'quarter' | 'year' })}
            />
            <Button size="small" type="primary" loading={configSavingSection === 'renewal'} onClick={() => void saveClientBilling(record)}>
              保存
            </Button>
          </Space>
        )
      },
    },
    {
      title: '过期 / 自动刷新',
      key: 'expiry',
      width: 360,
      render: (_, record) => {
        const billing = findClientBilling(managedConfig?.renewal?.client_billings, record) || defaultClientBilling(record)
        const effectiveExpiry = billing.expire_time || record.expiry_time || 0
        return (
          <Space direction="vertical" size={6} className="client-expiry-cell">
            <Text type="secondary">x-ui 当前：{formatExpiryTime(record.expiry_time)}</Text>
            <Space wrap size={[6, 6]}>
              <Input
                size="small"
                type="date"
                style={{ width: 132 }}
                value={formatDateInputFromMillis(effectiveExpiry)}
                onChange={(event) => updateClientBillingDraft(record, { expire_time: dateInputToExpiryMillis(event.target.value) })}
              />
              <Select
                size="small"
                style={{ width: 78 }}
                value={billing.expire_cycle || 'month'}
                options={[
                  { value: 'month', label: '月' },
                  { value: 'quarter', label: '季' },
                  { value: 'year', label: '年' },
                ]}
                onChange={(value) => updateClientBillingDraft(record, { expire_cycle: value as 'month' | 'quarter' | 'year', expire_time: effectiveExpiry })}
              />
              <Switch
                size="small"
                checked={Boolean(billing.expire_auto_renew)}
                onChange={(checked: boolean) => updateClientBillingDraft(record, { expire_auto_renew: checked, expire_time: effectiveExpiry })}
              />
              <Text type="secondary">周期刷新</Text>
              <Button size="small" type="primary" loading={configSavingSection === 'renewal'} onClick={() => void saveClientBilling(record)}>
                保存
              </Button>
            </Space>
          </Space>
        )
      },
    },
    {
      title: '最近活跃',
      dataIndex: 'last_online',
      width: 160,
      render: (value?: number) => formatRelativeTime(value),
    },
    {
      title: '导入',
      key: 'import',
      width: 160,
      render: (_, record) => (
        <Space wrap size={[6, 6]}>
          <Button size="small" disabled={!record.import_url} onClick={() => copyImportURL(record)}>
            复制
          </Button>
          <Button size="small" disabled={!record.import_url} onClick={() => setImportURLClient(record)}>
            URL / 二维码
          </Button>
        </Space>
      ),
    },
    {
      title: '路由指向',
      key: 'route',
      width: 280,
      render: (_, record) => (
        <RouteBadge route={record.route} onJumpOutbound={jumpToOutbound} onJumpRule={jumpToRule} />
      ),
    },
  ]

  const routingColumns: ColumnsType<XUIRoutingRuleView> = [
    {
      title: '#',
      dataIndex: 'index',
      width: 70,
      render: (value: number) => (
        <span id={ruleElementId(value)} className={selectedRuleIndex === value ? 'route-anchor active' : 'route-anchor'}>
          R{value}
        </span>
      ),
    },
    {
      title: '入口匹配',
      key: 'matchers',
      width: 280,
      render: (_, record) => (
        <Space wrap size={[6, 6]}>
          {record.inbound_tags?.map((tag) => (
            <Tag key={`inbound-${record.index}-${tag}`} color="blue">
              {tag}
            </Tag>
          ))}
          {record.users?.map((user) => (
            <Tag key={`user-${record.index}-${user}`} color="gold">
              {user}
            </Tag>
          ))}
          {!record.inbound_tags?.length && !record.users?.length ? <Tag>全局条件</Tag> : null}
        </Space>
      ),
    },
    {
      title: '出站 / 均衡器',
      key: 'route',
      width: 220,
      render: (_, record) => (
        <Space wrap size={[6, 6]}>
          {record.outbound_tag ? (
            <Button type="link" className="route-link" onClick={() => jumpToOutbound(record.outbound_tag)}>
              {record.outbound_tag}
            </Button>
          ) : null}
          {record.balancer_tag ? <Tag color="magenta">balancer:{record.balancer_tag}</Tag> : null}
        </Space>
      ),
    },
    {
      title: '条件摘要',
      dataIndex: 'summary',
      render: (value: string | undefined, record) => value || summarizeRule(record),
    },
  ]

  const xuiActionColumns: ColumnsType<XUIAction> = [
    {
      title: 'ID',
      dataIndex: 'id',
      width: 76,
    },
    {
      title: '操作',
      dataIndex: 'kind',
      width: 160,
      render: (value: string) => actionKindLabel(value),
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 120,
      render: (value: string) => <Tag color={actionStatusColor(value)}>{actionStatusLabel(value)}</Tag>,
    },
    {
      title: '结果',
      key: 'result',
      render: (_, record) => (
        <div>
          <Text>{record.error || shortJSON(record.result) || shortJSON(record.payload) || '-'}</Text>
          <div className="muted-line">创建 {formatDateTime(record.created_at)}{record.completed_at ? ` · 完成 ${formatDateTime(record.completed_at)}` : ''}</div>
        </div>
      ),
    },
  ]

  const agentLogColumns: ColumnsType<AgentLogEntry> = [
    {
      title: '时间',
      dataIndex: 'time',
      width: 180,
      render: (value?: string) => formatDateTime(value),
    },
    {
      title: '来源',
      dataIndex: 'source',
      width: 100,
      render: (value?: string) => value || '-',
    },
    {
      title: '级别',
      dataIndex: 'level',
      width: 100,
      render: (value?: string) => <Tag color={value === 'error' ? 'red' : 'default'}>{value || 'info'}</Tag>,
    },
    {
      title: '内容',
      dataIndex: 'message',
      render: (value?: string) => <Text copyable={Boolean(value)}>{value || '-'}</Text>,
    },
  ]

  const certificateColumns: ColumnsType<XUILocalCertificate> = [
    {
      title: '证书',
      key: 'certificate',
      width: 240,
      render: (_, record) => (
        <div>
          <Text strong>{record.name || record.subject || record.id}</Text>
          <div className="muted-line">{record.subject || '-'}</div>
        </div>
      ),
    },
    {
      title: '域名',
      dataIndex: 'dns_names',
      width: 280,
      render: (value?: string[]) => value?.length ? value.join(', ') : '-',
    },
    {
      title: '到期时间',
      dataIndex: 'not_after',
      width: 180,
      render: (value?: string) => formatDateTime(value),
    },
    {
      title: '证书路径',
      dataIndex: 'cert_path',
      width: 260,
      render: (value?: string) => value || '-',
    },
    {
      title: '私钥路径',
      dataIndex: 'key_path',
      width: 260,
      render: (value?: string) => value || '-',
    },
  ]

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
  return (
    <div className="page-shell">
      <VisualEffects />
      <div className="page-background page-background-left" />
      <div className="page-background page-background-right" />
      <div className={`app-shell${centerPanelOpen ? ' topology-open-shell' : ''}`}>
        <header className="hero-panel">
          <div>
            <div className="eyebrow">南风 VPS 监控中心</div>
            <Title level={1}>{heroTitle}</Title>
            <Paragraph className="hero-copy">
              这里统一管理 client 注册、x-ui 托管配置，以及跨 client 的出站与转发编排。节点新增仍在各自 x-ui 面板内完成，中心只负责汇总和联动。
            </Paragraph>
          </div>
          <div className="hero-actions hero-actions-column">
            <PersonalCenterDropdown
              adminUser={adminUser}
              systemInfo={systemInfo}
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
              onOpenCustomers={() => setCustomerModalOpen(true)}
              onOpenFrontendSettings={() => void openFrontendSettingsModal()}
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

        <SystemUpdateModal
          open={updateModalOpen}
          loading={updateLoading}
          latestLoading={updateLatestLoading}
          latestInfo={updateLatestInfo}
          latestError={updateLatestError}
          systemInfo={systemInfo}
          onClose={() => setUpdateModalOpen(false)}
          onRefreshLatest={() => void loadUpdateLatestInfo()}
          onUpdateServer={() => void updateServerOnline()}
          onUpdateClients={() => void updateAllClientsOnline()}
        />

        <ClientInstallModal
          open={clientInstallModalOpen}
          loading={clientInstallLoading}
          saving={clientInstallSaving}
          form={clientInstallForm}
          commandKind={clientInstallCommandKind}
          linuxCommand={clientInstallCommand}
          windowsPowerShellCommand={clientWindowsPowerShellCommand}
          windowsCMDCommand={clientWindowsCMDCommand}
          onClose={() => setClientInstallModalOpen(false)}
          onSave={() => void saveClientInstallSettings()}
          onCopy={(command) => void copyClientInstallCommand(command)}
          onFormChange={setClientInstallForm}
          onCommandKindChange={setClientInstallCommandKind}
        />

        <AccountSettingsModal
          open={accountModalOpen}
          saving={accountSaving}
          form={accountForm}
          onClose={() => setAccountModalOpen(false)}
          onSave={() => void saveAccount()}
          onFormChange={setAccountForm}
        />

        <FrontendSettingsModal
          open={frontendSettingsModalOpen}
          loading={frontendSettingsLoading}
          saving={frontendSettingsSaving}
          form={frontendSettingsForm}
          onClose={() => setFrontendSettingsModalOpen(false)}
          onSave={() => void saveFrontendSettings()}
          onFormChange={setFrontendSettingsForm}
        />

        <TelegramBotSettingsModal
          open={telegramBotModalOpen}
          bots={telegramBots}
          loading={telegramBotsLoading}
          saving={telegramBotSaving}
          editingID={editingTelegramBotId}
          form={telegramBotForm}
          onClose={() => setTelegramBotModalOpen(false)}
          onFormChange={setTelegramBotForm}
          onSave={saveTelegramBot}
          onRefresh={() => void loadTelegramBots()}
          onEditIDChange={setEditingTelegramBotId}
          onDelete={(id) => void deleteTelegramBot(id)}
          onTest={(id) => void testTelegramBot(id)}
        />

        <CustomerManagementModal
          open={customerModalOpen}
          agents={agents}
          onClose={() => setCustomerModalOpen(false)}
        />

        <XUIActionModal
          open={xuiActionModalOpen}
          saving={xuiActionSaving}
          actionKind={xuiActionKind}
          outboundForm={outboundActionForm}
          routingForm={routingActionForm}
          agents={agents}
          targetAgentID={selectedAgentId}
          currentOverview={overview}
          sourceOverview={outboundSourceOverview}
          sourceLoading={outboundSourceLoading}
          onClose={() => setXUIActionModalOpen(false)}
          onSubmit={() => void createXUIAction()}
          onActionKindChange={setXUIActionKind}
          onOutboundFormChange={setOutboundActionForm}
          onRoutingFormChange={setRoutingActionForm}
        />

        <ImportURLModal
          client={importURLClient}
          onClose={() => setImportURLClient(null)}
          onCopy={(client) => void copyImportURL(client)}
        />

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
            exchangeRates={exchangeRates}
            selectedTag={selectedTag}
            currentAgentLabel={selectedAgent?.agent_name || selectedAgent?.agent_id || ''}
            currentIPv4={selectedSummary.public_ipv4 || ''}
            compact={centerPanelOpen}
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
            onToggleViewMode={toggleAgentViewMode}
            onToggleTopology={() => {
              if (topologyVisible) {
                setTopologyVisible(false)
              } else {
                openTopologyPanel()
              }
            }}
            onRefresh={() => void loadAgents()}
            onSelectTag={(tag) => {
              setSelectedTag(tag)
              setSelectedAgentId('')
            }}
            onSelectAgent={(agentID, active) => {
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
                    onSelectAgent: openAgentDetailPanel,
                    onJumpNode: jumpToNode,
                    canOpenXUI: Boolean(managedConfig?.xui?.base_url),
                    onOpenXUI: () => {
                      if (managedConfig?.xui?.base_url) {
                        window.open(managedConfig.xui.base_url, '_blank', 'noopener,noreferrer')
                      }
                    },
                    canRefreshCurrentNode: Boolean(selectedAgentId),
                    currentNodeLoading: overviewLoading || configLoading,
                    onRefreshCurrentNode: () => {
                      if (selectedAgentId) {
                        setReloadToken((current) => current + 1)
                      }
                    },
                  })}
              </div>
            ) : null}

            {overviewError && selectedAgentId && !topologyVisible ? (
              <Alert
                className="surface-card alert-card"
                type="warning"
                showIcon
                message="x-ui 概览暂不可用"
                description={overviewError}
              />
            ) : null}

            {selectedAgent && !topologyVisible ? (
              <Card id="agent-detail-panel" className="surface-card tabs-card" bordered={false}>
                <div className="selected-agent-toolbar">
                  <div>
                    <Text type="secondary">当前 Client</Text>
                    <Title level={4}>{selectedAgent.agent_name || selectedAgent.agent_id}</Title>
                  </div>
                  <Space wrap>
                    {selectedAgent.summary.last_collection_err ? (
                      <Tag
                        color="orange"
                        style={{ cursor: 'pointer' }}
                        onClick={() => {
                          setActiveTabKey('logs')
                          void loadAgentLogs(selectedAgentId)
                        }}
                      >
                        x-ui 异常
                      </Tag>
                    ) : null}
                    <Button
                      onClick={() => {
                        setActiveTabKey('logs')
                        void loadAgentLogs(selectedAgentId)
                      }}
                    >
                      查看日志
                    </Button>
                    <Button
                      disabled={!managedConfig?.xui?.base_url}
                      onClick={() => {
                        if (managedConfig?.xui?.base_url) {
                          window.open(managedConfig.xui.base_url, '_blank', 'noopener,noreferrer')
                        }
                      }}
                    >
                      打开 x-ui 面板
                    </Button>
                    <Button icon={<ReloadOutlined />} loading={overviewLoading || configLoading} onClick={() => setReloadToken((current) => current + 1)}>
                      刷新当前 Client
                    </Button>
                  </Space>
                </div>
                <Tabs
                  activeKey={activeTabKey}
                  onChange={setActiveTabKey}
                  items={[
                  {
                    key: 'overview',
                    label: `总览 (${filteredAgents.length})`,
                    children: renderGlobalOverviewPanel({
                      dashboardView,
                      selectedTag,
                      links: filteredTagLinks,
                      onSelectTag: (value) => {
                        setSelectedTag(value)
                        setSelectedAgentId('')
                      },
                    }),
                  },
                  {
                    key: 'actions',
                    label: `x-ui 操作 (${xuiActions.length})`,
                    children: (
                      <Space direction="vertical" size="middle" style={{ width: '100%' }}>
                        <Alert
                          type="warning"
                          showIcon
                          message="这里现在只负责出站和转发编排"
                          description="入站节点请直接通过 x-ui 面板手动新增；中心只负责把其它 client 的节点导入为当前 client 的出站，以及给当前 client 下发转发规则。"
                        />
                        <Space wrap>
                          <Button
                            type="primary"
                            disabled={!selectedAgentId}
                            onClick={() => {
                              setXUIActionKind('add_outbound')
                              setOutboundActionForm(defaultOutboundActionForm())
                              setXUIActionModalOpen(true)
                            }}
                          >
                            新增操作
                          </Button>
                          <Button
                            icon={<ReloadOutlined />}
                            disabled={!selectedAgentId}
                            loading={xuiActionsLoading}
                            onClick={() => void loadXUIActions()}
                          >
                            刷新操作记录
                          </Button>
                        </Space>
                        <Table
                          rowKey={(record) => record.id}
                          columns={xuiActionColumns}
                          dataSource={xuiActions}
                          loading={xuiActionsLoading}
                          pagination={{ pageSize: 8, hideOnSinglePage: true }}
                          scroll={{ x: 820 }}
                        />
                      </Space>
                    ),
                  },
                  {
                    key: 'logs',
                    label: `日志 (${agentLogs?.logs.length || 0})`,
                    children: (
                      <Space direction="vertical" size="middle" style={{ width: '100%' }}>
                        {agentLogs?.last_collection_err ? (
                          <Alert
                            type="warning"
                            showIcon
                            message="最近一次 x-ui 采集异常"
                            description={agentLogs.last_collection_err}
                          />
                        ) : null}
                        {agentLogsError ? <Alert type="error" showIcon message={agentLogsError} /> : null}
                        <Space wrap>
                          <Button
                            icon={<ReloadOutlined />}
                            disabled={!selectedAgentId}
                            loading={agentLogsLoading}
                            onClick={() => void loadAgentLogs()}
                          >
                            刷新日志
                          </Button>
                          <Text type="secondary">当前显示 client 最近一次上报附带的异常日志</Text>
                        </Space>
                        <Table
                          rowKey={(record, index) => `${record.time}-${record.source || 'log'}-${index}`}
                          columns={agentLogColumns}
                          dataSource={agentLogs?.logs || []}
                          loading={agentLogsLoading}
                          pagination={{ pageSize: 10, hideOnSinglePage: true }}
                          scroll={{ x: 760 }}
                          locale={{ emptyText: <Empty description="暂无异常日志" /> }}
                        />
                      </Space>
                    ),
                  },
                  {
                    key: 'certificates',
                    label: `本机证书 (${overview?.certificates.length || 0})`,
                    children: overview ? (
                      <Table
                        rowKey={(record) => record.id}
                        columns={certificateColumns}
                        dataSource={overview.certificates}
                        pagination={{ pageSize: 8, hideOnSinglePage: true }}
                        scroll={{ x: 1200 }}
                      />
                    ) : (
                      <Empty description="暂无本机证书数据" />
                    ),
                  },
                    {
                      key: 'config',
                      label: (
                        <Space size={6}>
                          <SettingOutlined />
                          <span>托管配置</span>
                        </Space>
                      ),
                      children: renderManagedConfigPanel({
                        selectedAgent,
                        managedConfig,
                        configLoading,
                        configSavingSection,
                        configError,
                        onSave: saveManagedConfigSection,
                        onAgentNameChange: (value) => updateManagedConfig((current) => ({ ...current, agent_name: value })),
                        onSortOrderChange: (value) => updateManagedConfig((current) => ({ ...current, sort_order: value })),
                        tagOptions,
                        newTagName,
                        tagSaving,
                        onNewTagNameChange: setNewTagName,
                        onCreateTag: createTagOption,
                        onTagsChange: (values) => {
                          const tags = mergeTagOptions([], values)
                          setTagInputText(formatTagInput(tags))
                          setTagOptions((current) => mergeTagOptions(current, tags))
                          updateManagedConfig((current) => ({ ...current, tags }))
                        },
                        onRenewalChange: (patch) => updateManagedConfig((current) => ({ ...current, renewal: { ...current.renewal, ...patch } })),
                        entryAddressInputText,
                        onEntryAddressesTextChange: (value) => {
                          setEntryAddressInputText(value)
                          updateManagedConfig((current) => ({ ...current, entry: { ...current.entry, addresses: parseAddressInput(value) } }))
                        },
                        onEntryChange: (patch) => updateManagedConfig((current) => ({ ...current, entry: { ...current.entry, ...patch } })),
                        onXUIChange: (patch) => updateManagedConfig((current) => ({ ...current, xui: { ...current.xui, ...patch } })),
                        configAudits,
                        configAuditsLoading,
                        currencyOptions,
                      }),
                    },
                  {
                    key: 'nodes',
                    label: `节点 (${overview?.nodes.length || 0})`,
                    children: overview ? (
                      <Table
                        rowKey={(record) => record.tag || String(record.id)}
                        columns={nodeColumns}
                        dataSource={overview.nodes}
                        pagination={false}
                        scroll={{ x: 1000 }}
                        onRow={(record) => {
                          const anchor = nodeElementId(selectedAgentId, record.remark || record.tag || String(record.id))
                          return {
                            id: anchor,
                            className: selectedNodeAnchor === anchor ? 'node-row-selected' : '',
                          }
                        }}
                      />
                    ) : (
                      <Empty description="暂无 x-ui 节点数据" />
                    ),
                  },
                  {
                    key: 'clients',
                    label: `客户端 (${overview?.clients.length || 0})`,
                    children: overview ? (
                      <Space direction="vertical" style={{ width: '100%' }} size="middle">
                        <Input.Search
                          allowClear
                          placeholder="按邮箱、备注、节点标签筛选客户端"
                          value={clientSearch}
                          onChange={(event) => setClientSearch(event.target.value)}
                        />
                        <Table
                          rowKey={(record) => `${record.inbound_tag}-${record.email}`}
                          columns={clientColumns}
                          dataSource={filteredClients}
                          pagination={{ pageSize: 12, hideOnSinglePage: true }}
                          scroll={{ x: 1780 }}
                        />
                      </Space>
                    ) : (
                      <Empty description="暂无客户端数据" />
                    ),
                  },
                  {
                    key: 'outbounds',
                    label: `出站 (${overview?.outbounds.length || 0})`,
                    children: overview ? (
                      <div className="outbound-grid">
                        {overview.outbounds.map((outbound) => {
                          const selected = outbound.tag && outbound.tag === selectedOutboundTag
                          const linkedClient = findOutboundLinkedClient(dashboardView, selectedAgentId, outbound.tag)
                          const linkedNodeLabel = linkedClient
                            ? linkedClient.target.inbound_name || linkedClient.target.inbound_tag || String(linkedClient.target.inbound_id)
                            : ''
                          return (
                            <section
                              key={outbound.tag || 'unknown'}
                              id={outboundElementId(outbound.tag || 'unknown')}
                              className={`outbound-card${selected ? ' selected' : ''}${linkedClient ? ' linked' : ''}`}
                              role={linkedClient ? 'button' : undefined}
                              tabIndex={linkedClient ? 0 : undefined}
                              onClick={() => {
                                if (outbound.tag) {
                                  setSelectedOutboundTag(outbound.tag)
                                }
                                if (linkedClient) {
                                  jumpToNode(linkedClient.target.agent_id, linkedNodeLabel)
                                }
                              }}
                              onKeyDown={(event) => {
                                if (!linkedClient || (event.key !== 'Enter' && event.key !== ' ')) {
                                  return
                                }
                                event.preventDefault()
                                jumpToNode(linkedClient.target.agent_id, linkedNodeLabel)
                              }}
                            >
                              <div className="outbound-head">
                                <div>
                                  <Text className="outbound-tag">{outbound.tag || '-'}</Text>
                                  <div className="muted-line">
                                    {outbound.protocol || '-'}
                                    {outbound.is_default ? ' · 默认出口' : ''}
                                  </div>
                                </div>
                                <Space wrap size={[6, 6]}>
                                  {outbound.is_default ? <Tag color="success">默认</Tag> : null}
                                  {outbound.send_through ? <Tag>sendThrough:{outbound.send_through}</Tag> : null}
                                  {linkedClient ? <Tag color="cyan">关联 {linkedClient.target.agent_name || linkedClient.target.agent_id}</Tag> : null}
                                </Space>
                              </div>
                              <div className="outbound-target">{outbound.target || '当前配置未提供远端地址'}</div>
                              {linkedClient ? (
                                <div className="muted-line">
                                  点击跳转到 {linkedClient.target.agent_name || linkedClient.target.agent_id} / {linkedNodeLabel}
                                </div>
                              ) : null}
                              <div className="outbound-metrics">
                                <span>上行 {formatBytes(outbound.up || 0)}</span>
                                <span>下行 {formatBytes(outbound.down || 0)}</span>
                                <span>累计 {formatBytes(outbound.total || 0)}</span>
                              </div>
                            </section>
                          )
                        })}
                      </div>
                    ) : (
                      <Empty description="暂无出站数据" />
                    ),
                  },
                  {
                    key: 'routes',
                    label: `路由规则 (${overview?.routing_rules.length || 0})`,
                    children: overview ? (
                      <Table
                        rowKey={(record) => record.index}
                        columns={routingColumns}
                        dataSource={overview.routing_rules}
                        pagination={false}
                        scroll={{ x: 1100 }}
                        rowClassName={(record) => (selectedRuleIndex === record.index ? 'route-row-selected' : '')}
                      />
                    ) : (
                      <Empty description="暂无路由规则数据" />
                    ),
                  },
                  ]}
                />
              </Card>
            ) : null}
          </main>
        </div>
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

  function openTopologyPanel() {
    setTopologyVisible(true)
    window.setTimeout(() => {
      document.getElementById('topology-panel')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
    }, 80)
  }

  function openAgentDetailPanel(agentID: string, tabKey = 'overview') {
    setTopologyVisible(false)
    setActiveTabKey(tabKey)
    startTransition(() => {
      setSelectedAgentId(agentID)
    })
    window.setTimeout(() => {
      document.getElementById('agent-detail-panel')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
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
