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
  EditOutlined,
  ReloadOutlined,
} from '@ant-design/icons'

import type {
  AdminAuthResponse,
  AdminUser,
  AgentListItem,
  AgentLogsResponse,
  AgentRealtimeMetrics,
  ConfigAuditLog,
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
  PersonalCenterDropdown,
} from './components/AdminModals'
import { renderCNFlowPanel } from './components/DashboardTopologyPanels'
import { AgentDetailPanel } from './components/AgentDetailPanel'
import { ConsoleModals } from './components/ConsoleModals'
import { AgentRail, OverviewSummaryCard } from './components/DashboardSidebar'
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

function hasCustomerDisplayNameField(value: unknown): boolean {
  return Boolean(value && typeof value === 'object' && Object.prototype.hasOwnProperty.call(value, 'customer_display_name'))
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
  const [xuiActionKind, setXUIActionKind] = useState('upsert_routing_rule')
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
        message.warning('Customer 展示名称已保留在当前页面；客户侧显示需要后端服务更新到新版后生效')
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
  const monthlyFinance = summarizeMonthlyFinance(filteredAgents, costCurrency, exchangeRates)
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

        <ConsoleModals
          accountForm={accountForm}
          accountModalOpen={accountModalOpen}
          accountSaving={accountSaving}
          agents={agents}
          clientInstallCommandKind={clientInstallCommandKind}
          clientInstallForm={clientInstallForm}
          clientInstallLoading={clientInstallLoading}
          clientInstallModalOpen={clientInstallModalOpen}
          clientInstallSaving={clientInstallSaving}
          clientInstallLinuxCommand={clientInstallCommand}
          clientInstallWindowsCMDCommand={clientWindowsCMDCommand}
          clientInstallWindowsPowerShellCommand={clientWindowsPowerShellCommand}
          customerModalOpen={customerModalOpen}
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
          onAccountFormChange={setAccountForm}
          onClientInstallCommandKindChange={setClientInstallCommandKind}
          onClientInstallFormChange={setClientInstallForm}
          onCloseAccount={() => setAccountModalOpen(false)}
          onCloseClientInstall={() => setClientInstallModalOpen(false)}
          onCloseCustomerModal={() => setCustomerModalOpen(false)}
          onCloseFrontendSettings={() => setFrontendSettingsModalOpen(false)}
          onCloseImportURL={() => setImportURLClient(null)}
          onCloseTelegramBot={() => setTelegramBotModalOpen(false)}
          onCloseUpdateModal={() => setUpdateModalOpen(false)}
          onCloseXUIActionModal={() => setXUIActionModalOpen(false)}
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
          onUpdateOutboundActionForm={setOutboundActionForm}
          onUpdateRoutingActionForm={setRoutingActionForm}
          onUpdateServer={() => void updateServerOnline()}
          onXUIActionKindChange={setXUIActionKind}
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

            {selectedAgent && !topologyVisible ? (
              <AgentDetailPanel
                activeTabKey={activeTabKey}
                agentLogs={agentLogs}
                agentLogsError={agentLogsError}
                agentLogsLoading={agentLogsLoading}
                canOpenXUI={Boolean(managedConfig?.xui?.base_url)}
                clientSearch={clientSearch}
                configAudits={configAudits}
                configAuditsLoading={configAuditsLoading}
                configError={configError}
                configLoading={configLoading}
                configSavingSection={configSavingSection}
                currencyOptions={currencyOptions}
                currentAgentLoading={overviewLoading || configLoading}
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
                onRefreshCurrentAgent={() => setReloadToken((current) => current + 1)}
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
                onUpdateClientBillingDraft={updateClientBillingDraft}
                onXUIChange={(patch) => updateManagedConfig((current) => ({ ...current, xui: { ...current.xui, ...patch } }))}
              />
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
    setTopologyVisible(false)
    setSelectedAgentId('')
    setActiveTabKey('overview')
    setSelectedOutboundTag('')
    setSelectedRuleIndex(null)
    setSelectedNodeAnchor('')
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
