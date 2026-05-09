import { startTransition, useDeferredValue, useEffect, useRef, useState } from 'react'
import {
  Alert,
  App as AntdApp,
  Badge,
  Button,
  Card,
  Col,
  Empty,
  Input,
  InputNumber,
  List,
  Modal,
  QRCode,
  Row,
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
  BarsOutlined,
  CloudServerOutlined,
  BellOutlined,
  CloudDownloadOutlined,
  CopyOutlined,
  DeleteOutlined,
  EditOutlined,
  LockOutlined,
  LogoutOutlined,
  PlusOutlined,
  ReloadOutlined,
  SaveOutlined,
  SafetyCertificateOutlined,
  SettingOutlined,
  UserOutlined,
} from '@ant-design/icons'

import type {
  AdminAuthResponse,
  AdminUser,
  AgentEntryConfig,
  AgentEntryMapping,
  AgentListItem,
  AgentRealtimeMetrics,
  ClientChainStep,
  ClientChainView,
  ClientInstallInfo,
  ConfigAuditLog,
  DashboardAgentView,
  DashboardRealtimeMessage,
  DashboardTagView,
  ExchangeRatesResponse,
  GlobalDashboardView,
  IPGeoView,
  ManagedAgentConfig,
  TopologyLinkView,
  TelegramBot,
  TagSettingsResponse,
  VPSRenewalConfig,
  VPSSummary,
  XUIAction,
  XUIClientView,
  XUIConfig,
  XUILocalCertificate,
  XUINodeView,
  XUIOverview,
  XUIRouteTrace,
  XUIRoutingRuleView,
} from './types'
import {
  COMMON_COST_CURRENCIES,
  COST_CURRENCY_STORAGE_KEY,
  DEFAULT_COST_CURRENCY,
  type CurrencyCode,
  type ExchangeRatesState,
  defaultExchangeRatesState,
  formatMoney,
  mergeCurrencyOptions,
  normalizeCurrencyCode,
  readStoredCostCurrency,
  summarizeMonthlyCost,
} from './lib/currency'
import {
  bytesToGB,
  calculateMemoryPercent,
  calculateTrafficStatus,
  clampMetricPercent,
  clientTrafficTotal,
  formatBandwidth,
  formatBytes,
  formatMem,
  formatPercent,
  formatSpeed,
  gbToBytes,
  metricLevel,
  summarizeAgentNetwork,
} from './lib/traffic'

const { Paragraph, Text, Title } = Typography
const DEFAULT_BACKGROUND_IMAGE = 'https://pic.netbian.com/uploads/allimg/260211/223628-17708205888f2f.jpg'
const DASHBOARD_AUTO_REFRESH_MS = 20_000
const AGENT_VIEW_MODE_STORAGE_PREFIX = 'bridge-core.agent-view-mode.'
const XUI_ACTION_KINDS = [
  { value: 'add_outbound', label: '从内部导入出站' },
  { value: 'add_routing_rule', label: '新增转发 / 路由规则' },
]
type AgentViewMode = 'card' | 'list'
type ConfigSectionKey = 'client' | 'renewal' | 'xui' | 'entry'

interface TLSCertificateSelectionForm {
  mode: 'none' | 'domain_auto' | 'inventory' | 'manual'
  inventory_id: string
  domain: string
  certificate_file: string
  key_file: string
}

interface XUIInboundClientForm {
  email: string
  uuid: string
  password: string
  flow: string
  limit_ip: number
  total_gb: number
  expiry_days: number
  comment: string
  sub_id: string
  enabled: boolean
}

interface XUIInboundActionForm {
  remark: string
  tag: string
  enabled: boolean
  listen: string
  port: number
  protocol: string
  transport: string
  security: string
  server_name: string
  ws_path: string
  ws_host: string
  sniffing: boolean
  tls: TLSCertificateSelectionForm
  clients: XUIInboundClientForm[]
  restart: boolean
}

interface XUIOutboundActionForm {
  tag: string
  protocol: string
  send_through: string
  address: string
  port: number
  uuid: string
  flow: string
  password: string
  method: string
  security: string
  server_name: string
  network: string
  ws_path: string
  ws_host: string
  source_agent_id: string
  source_client_key: string
  restart: boolean
}

interface XUIRoutingActionForm {
  outbound_tag: string
  balancer_tag: string
  inbound_tags: string[]
  users: string[]
  domains: string
  ips: string
  ports: string
  source_ips: string
  source_ports: string
  networks: string[]
  protocols: string[]
  restart: boolean
}

interface TelegramBotForm {
  name: string
  bot_token: string
  chat_id: string
  enabled: boolean
}

interface ClientInstallCommandForm {
  server_url: string
  registration_token: string
  install_script_url: string
  poll_interval: string
  request_timeout_seconds: number
  server_skip_tls_verify: boolean
}

declare global {
  interface Window {
    CustomBackgroundImage?: string
  }
}

interface LoadOptions {
  silent?: boolean
}

export default function App() {
  const { message } = AntdApp.useApp()
  const [sessionLoading, setSessionLoading] = useState(true)
  const [adminUser, setAdminUser] = useState<AdminUser | null>(null)
  const [loginLoading, setLoginLoading] = useState(false)
  const [loginForm, setLoginForm] = useState({ username: '', password: '' })
  const [accountModalOpen, setAccountModalOpen] = useState(false)
  const [personalCenterOpen, setPersonalCenterOpen] = useState(false)
  const [accountSaving, setAccountSaving] = useState(false)
  const [accountForm, setAccountForm] = useState({
    current_password: '',
    new_username: '',
    new_password: '',
    confirm_password: '',
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
  const [clientInstallModalOpen, setClientInstallModalOpen] = useState(false)
  const [clientInstallLoading, setClientInstallLoading] = useState(false)
  const [clientInstallSaving, setClientInstallSaving] = useState(false)
  const [clientInstallForm, setClientInstallForm] = useState<ClientInstallCommandForm>(() => defaultClientInstallCommandForm())
  const [reloadToken, setReloadToken] = useState(0)
  const [activeTabKey, setActiveTabKey] = useState('overview')
  const [topologyVisible, setTopologyVisible] = useState(false)
  const [selectedOutboundTag, setSelectedOutboundTag] = useState('')
  const [selectedRuleIndex, setSelectedRuleIndex] = useState<number | null>(null)
  const [selectedNodeAnchor, setSelectedNodeAnchor] = useState('')
  const [xuiActions, setXUIActions] = useState<XUIAction[]>([])
  const [xuiActionsLoading, setXUIActionsLoading] = useState(false)
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
  const topologyScopeLabel = selectedAgentId ? selectedAgent?.agent_name || selectedAgentId : selectedTag ? `${selectedTag} 标签` : '全部 Client'
  const heroTitle = '南风VPS监控'
  useEffect(() => {
    void loadSession()
  }, [])

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
      setConfigAudits([])
      return
    }

    void loadOverview(selectedAgentId)
    void loadManagedConfig(selectedAgentId)
    void loadXUIActions(selectedAgentId)
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
      setAccountForm((current) => ({ ...current, new_username: data.user.username }))
    } catch {
      setAdminUser(null)
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
      setAccountForm((current) => ({ ...current, new_username: data.user.username }))
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
    setDashboardView(null)
    setSelectedTag('')
    setAgents([])
    setSelectedAgentId('')
    setOverview(null)
    setManagedConfig(null)
    setTelegramBots([])
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
        }),
      })
      setAdminUser(data.user)
      setAccountForm({
        current_password: '',
        new_username: data.user.username,
        new_password: '',
        confirm_password: '',
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
    setPersonalCenterOpen(false)
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

  async function copyClientInstallCommand() {
    if (!clientInstallForm.registration_token.trim()) {
      message.warning('当前 server 未配置注册 Token，安装命令无法完成 Client 注册')
      return
    }
    const command = buildClientInstallCommand(clientInstallForm)
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
  const scopedClientCount = dashboardView ? selectedTagView?.client_count ?? dashboardView.totals.client_count : 0
  const scopedOnlineClientCount = dashboardView ? selectedTagView?.online_client_count ?? dashboardView.totals.online_client_count : 0
  const onlineAgentCount = filteredAgents.filter(isAgentRunning).length
  const offlineAgentCount = Math.max(scopedAgentCount - onlineAgentCount, 0)
  const scopedNetwork = summarizeAgentNetwork(filteredAgents)
  const monthlyCost = summarizeMonthlyCost(filteredAgents, costCurrency, exchangeRates)
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

  return (
    <div className="page-shell">
      <VisualEffects />
      <div className="page-background page-background-left" />
      <div className="page-background page-background-right" />
      <div className="app-shell">
        <header className="hero-panel">
          <div>
            <div className="eyebrow">南风 VPS 监控中心</div>
            <Title level={1}>{heroTitle}</Title>
            <Paragraph className="hero-copy">
              这里统一管理 client 注册、x-ui 托管配置，以及跨 client 的出站与转发编排。节点新增仍在各自 x-ui 面板内完成，中心只负责汇总和联动。
            </Paragraph>
          </div>
          <div className="hero-actions hero-actions-column">
            <Button
              className="personal-center-button"
              icon={<UserOutlined />}
              onClick={() => setPersonalCenterOpen(true)}
            >
              个人中心
              <span>{adminUser.username}</span>
            </Button>
          </div>
        </header>

        <Modal
          title="个人中心"
          open={personalCenterOpen}
          onCancel={() => setPersonalCenterOpen(false)}
          footer={null}
          width={520}
        >
          <div className="personal-center-panel">
            <div className="personal-center-profile">
              <div className="personal-center-avatar">
                <SafetyCertificateOutlined />
              </div>
              <div>
                <Text type="secondary">当前管理员</Text>
                <Title level={3}>{adminUser.username}</Title>
                <Tag color="success">已登录</Tag>
              </div>
            </div>
            <div className="personal-center-actions">
              <Button
                icon={<EditOutlined />}
                onClick={() => {
                  setAccountForm({
                    current_password: '',
                    new_username: adminUser.username,
                    new_password: '',
                    confirm_password: '',
                  })
                  setPersonalCenterOpen(false)
                  setAccountModalOpen(true)
                }}
              >
                修改账号密码
              </Button>
              <Button
                icon={<CloudDownloadOutlined />}
                onClick={() => void openClientInstallModal()}
              >
                Client 安装命令
              </Button>
              <Button
                icon={<BellOutlined />}
                onClick={() => {
                  setPersonalCenterOpen(false)
                  setTelegramBotModalOpen(true)
                }}
              >
                TG 告警机器人
              </Button>
              <Button
                danger
                icon={<LogoutOutlined />}
                onClick={() => {
                  setPersonalCenterOpen(false)
                  void logout()
                }}
              >
                退出登录
              </Button>
            </div>
          </div>
        </Modal>

        <Modal
          title="Client 一键安装命令"
          open={clientInstallModalOpen}
          onCancel={() => setClientInstallModalOpen(false)}
          width={820}
          footer={[
            <Button key="cancel" onClick={() => setClientInstallModalOpen(false)}>
              关闭
            </Button>,
            <Button key="save" loading={clientInstallSaving} onClick={() => void saveClientInstallSettings()}>
              保存参数
            </Button>,
            <Button
              key="copy"
              type="primary"
              icon={<CopyOutlined />}
              disabled={clientInstallLoading}
              onClick={() => void copyClientInstallCommand()}
            >
              复制安装命令
            </Button>,
          ]}
        >
          <Spin spinning={clientInstallLoading}>
            <Space direction="vertical" size="middle" style={{ width: '100%' }}>
              <Alert
                type="info"
                showIcon
                message="复制命令到目标 VPS 上执行"
                description="这里会把 server 地址、注册 Token 和通用 client 参数写进 env，安装脚本会自动生成 client.json 并注册到当前 server。建议在目标 Linux 机器上使用 root 执行。"
              />
              {!clientInstallForm.registration_token.trim() ? (
                <Alert
                  type="warning"
                  showIcon
                  message="缺少注册 Token"
                  description="请先在 server 配置里填写 registration_token，否则 Client 安装后无法完成注册。"
                />
              ) : null}
              <Row gutter={[14, 14]}>
                <Col xs={24} md={12}>
                  <Text type="secondary">Server 地址</Text>
                  <Input
                    value={clientInstallForm.server_url}
                    placeholder="https://panel.example.com"
                    onChange={(event) => setClientInstallForm((current) => ({ ...current, server_url: event.target.value }))}
                  />
                </Col>
                <Col xs={24} md={12}>
                  <Text type="secondary">Client 注册 Token</Text>
                  <Input.Password value={clientInstallForm.registration_token} readOnly />
                </Col>
                <Col xs={24}>
                  <Text type="secondary">安装脚本地址</Text>
                  <Input
                    value={clientInstallForm.install_script_url}
                    onChange={(event) => setClientInstallForm((current) => ({ ...current, install_script_url: event.target.value }))}
                  />
                </Col>
                <Col xs={24} md={8}>
                  <Text type="secondary">轮询间隔</Text>
                  <Input
                    value={clientInstallForm.poll_interval}
                    placeholder="30s"
                    onChange={(event) => setClientInstallForm((current) => ({ ...current, poll_interval: event.target.value }))}
                  />
                </Col>
                <Col xs={24} md={8}>
                  <Text type="secondary">请求超时（秒）</Text>
                  <InputNumber
                    style={{ width: '100%' }}
                    min={1}
                    value={clientInstallForm.request_timeout_seconds}
                    onChange={(value) => setClientInstallForm((current) => ({ ...current, request_timeout_seconds: Number(value || 15) }))}
                  />
                </Col>
                <Col xs={24} md={8}>
                  <Text type="secondary">跳过 TLS 校验</Text>
                  <div className="client-install-switch">
                    <Switch
                      checked={clientInstallForm.server_skip_tls_verify}
                      onChange={(checked) => setClientInstallForm((current) => ({ ...current, server_skip_tls_verify: checked }))}
                    />
                    <Text type="secondary">自签证书时开启</Text>
                  </div>
                </Col>
              </Row>
              <div>
                <div className="client-install-command-title">
                  <Text strong>生成的安装命令</Text>
                  <Button size="small" icon={<CopyOutlined />} onClick={() => void copyClientInstallCommand()}>
                    复制
                  </Button>
                </div>
                <Input.TextArea
                  className="client-install-command"
                  value={clientInstallCommand}
                  readOnly
                  autoSize={{ minRows: 5, maxRows: 8 }}
                />
              </div>
            </Space>
          </Spin>
        </Modal>

        <Modal
          title="修改管理员账号"
          open={accountModalOpen}
          onCancel={() => setAccountModalOpen(false)}
          onOk={() => void saveAccount()}
          confirmLoading={accountSaving}
          okText="保存"
          cancelText="取消"
        >
          <Space direction="vertical" size="middle" style={{ width: '100%' }}>
            <div>
              <Text type="secondary">当前密码</Text>
              <Input.Password
                value={accountForm.current_password}
                onChange={(event) => setAccountForm((current) => ({ ...current, current_password: event.target.value }))}
              />
            </div>
            <div>
              <Text type="secondary">新用户名</Text>
              <Input
                value={accountForm.new_username}
                onChange={(event) => setAccountForm((current) => ({ ...current, new_username: event.target.value }))}
              />
            </div>
            <div>
              <Text type="secondary">新密码</Text>
              <Input.Password
                placeholder="留空表示不修改密码"
                value={accountForm.new_password}
                onChange={(event) => setAccountForm((current) => ({ ...current, new_password: event.target.value }))}
              />
            </div>
            <div>
              <Text type="secondary">确认新密码</Text>
              <Input.Password
                placeholder="留空表示不修改密码"
                value={accountForm.confirm_password}
                onChange={(event) => setAccountForm((current) => ({ ...current, confirm_password: event.target.value }))}
              />
            </div>
          </Space>
        </Modal>

        <Modal
          title="Telegram 告警机器人"
          open={telegramBotModalOpen}
          onCancel={() => setTelegramBotModalOpen(false)}
          footer={null}
          width={920}
        >
          {renderTelegramBotPanel({
            bots: telegramBots,
            loading: telegramBotsLoading,
            saving: telegramBotSaving,
            editingID: editingTelegramBotId,
            form: telegramBotForm,
            onFormChange: setTelegramBotForm,
            onSave: saveTelegramBot,
            onRefresh: () => void loadTelegramBots(),
            onEdit: (bot) => {
              setEditingTelegramBotId(bot.id)
              setTelegramBotForm({
                name: bot.name,
                bot_token: '',
                chat_id: bot.chat_id,
                enabled: bot.enabled,
              })
            },
            onCancelEdit: () => {
              setEditingTelegramBotId(null)
              setTelegramBotForm(defaultTelegramBotForm())
            },
            onDelete: (id) => void deleteTelegramBot(id),
            onTest: (id) => void testTelegramBot(id),
          })}
        </Modal>

        <Modal
          title="下发 x-ui 操作"
          open={xuiActionModalOpen}
          onCancel={() => setXUIActionModalOpen(false)}
          onOk={() => void createXUIAction()}
          confirmLoading={xuiActionSaving}
          okText="下发"
          cancelText="取消"
          width={920}
        >
          <Space direction="vertical" size="middle" style={{ width: '100%' }}>
            <Alert
              type="info"
              showIcon
              message="执行方式"
              description="server 只保存任务；client 下一次轮询领取后，使用已托管的 x-ui 账号密码调用 3x-ui API 执行。这里仅允许把内部 Client 节点导入为出站，再配置转发规则。"
            />
            <div>
              <Text type="secondary">操作类型</Text>
              <Select
                style={{ width: '100%' }}
                value={xuiActionKind}
                options={XUI_ACTION_KINDS}
                onChange={(value) => {
                  setXUIActionKind(value)
                }}
              />
            </div>
            {xuiActionKind === 'add_outbound'
              ? renderOutboundActionForm({
                  form: outboundActionForm,
                  agents,
                  targetAgentID: selectedAgentId,
                  currentOverview: overview,
                  sourceOverview: outboundSourceOverview,
                  sourceLoading: outboundSourceLoading,
                  onChange: setOutboundActionForm,
                })
              : null}
            {xuiActionKind === 'add_routing_rule'
              ? renderRoutingActionForm({
                  form: routingActionForm,
                  inbounds: overview?.nodes || [],
                  clients: overview?.clients || [],
                  outbounds: overview?.outbounds || [],
                  onChange: setRoutingActionForm,
                })
              : null}
          </Space>
        </Modal>

        <Modal
          title="单节点导入 URL"
          open={Boolean(importURLClient)}
          onCancel={() => setImportURLClient(null)}
          footer={
            <Space>
              <Button onClick={() => setImportURLClient(null)}>关闭</Button>
              <Button type="primary" disabled={!importURLClient?.import_url} onClick={() => importURLClient && void copyImportURL(importURLClient)}>
                复制 URL
              </Button>
            </Space>
          }
        >
          {importURLClient?.import_url ? (
            <Space direction="vertical" size="middle" style={{ width: '100%' }}>
              <div>
                <Text strong>{importURLClient.email || 'anonymous-client'}</Text>
                <div className="muted-line">{importURLClient.inbound_remark || importURLClient.inbound_tag || '-'}</div>
              </div>
              <div className="import-url-qr">
                <QRCode value={importURLClient.import_url} bordered={false} />
              </div>
              <Input.TextArea value={importURLClient.import_url} readOnly autoSize={{ minRows: 3, maxRows: 6 }} />
            </Space>
          ) : (
            <Empty description="当前客户端暂不支持生成单节点导入 URL" />
          )}
        </Modal>

        <div className="workspace-grid">
          <Card className="surface-card summary-card" bordered={false}>
            {dashboardView ? (
              <>
                <div className="overview-stat-grid">
                  <section className="overview-stat-card overview-stat-blue">
                    <div className="overview-stat-title">服务器总数</div>
                    <div className="overview-stat-value">
                      <span className="overview-stat-dot" />
                      <strong>{scopedAgentCount}</strong>
                    </div>
                    <div className="overview-stat-foot">节点 {scopedNodeCount} · 标签 {dashboardView.totals.tagged_agent_count}</div>
                  </section>
                  <section className="overview-stat-card overview-stat-green">
                    <div className="overview-stat-title">在线服务器</div>
                    <div className="overview-stat-value">
                      <span className="overview-stat-dot" />
                      <strong>{onlineAgentCount}</strong>
                    </div>
                    <div className="overview-stat-foot">在线客户端 {scopedOnlineClientCount} · 总客户端 {scopedClientCount}</div>
                  </section>
                  <section className="overview-stat-card overview-stat-red">
                    <div className="overview-stat-title">离线服务器</div>
                    <div className="overview-stat-value">
                      <span className="overview-stat-dot" />
                      <strong>{offlineAgentCount}</strong>
                    </div>
                    <div className="overview-stat-foot">出站 {dashboardView.totals.outbound_count} · 转发规则 {dashboardView.totals.routing_rule_count}</div>
                  </section>
                  <section className="overview-stat-card overview-network-card">
                    <div className="overview-stat-title">网络</div>
                    <div className="overview-network-total">
                      <span className="network-up">↑{formatBytes(scopedNetwork.sent)}</span>
                      <span className="network-down">↓{formatBytes(scopedNetwork.recv)}</span>
                    </div>
                    <div className="overview-network-speed">
                      <span>⬆ {formatSpeed(scopedNetwork.up)}</span>
                      <span>⬇ {formatSpeed(scopedNetwork.down)}</span>
                    </div>
                  </section>
                  <section className="overview-stat-card overview-cost-card">
                    <div className="overview-stat-title overview-cost-title">
                      <span>月花销</span>
                      <Select
                        size="small"
                        value={costCurrency}
                        options={currencyOptions.map((currency) => ({ value: currency, label: currency }))}
                        onChange={(value) => setCostCurrency(value as CurrencyCode)}
                      />
                    </div>
                    <div className="overview-cost-value">{formatMoney(monthlyCost.total, costCurrency)}</div>
                    <div className="overview-stat-foot">
                      {exchangeRates.loading ? '汇率加载中' : monthlyCost.missingCount ? `${monthlyCost.count} 台已配置 · ${monthlyCost.missingCount} 台缺少费用/汇率` : `${monthlyCost.count} 台已配置`}
                      {exchangeRates.date ? ` · 汇率 ${exchangeRates.date}` : ''}
                    </div>
                    {exchangeRates.error ? <div className="overview-stat-foot">汇率加载失败：{exchangeRates.error}</div> : null}
                  </section>
                </div>
                <div className="overview-summary-strip">
                  <span>已匹配链路 · {dashboardView.totals.link_count}</span>
                  <span>前端客户端链路 · {dashboardView.totals.chain_count}</span>
                  <span>标签视图 · {selectedTag || '全部'}</span>
                  <span>计算时间 · {formatDateTime(dashboardView.generated_at)}</span>
                  <span>当前详情节点 · {selectedAgent?.agent_name || selectedAgent?.agent_id || '-'}</span>
                  <span>当前节点 IPv4 · {selectedSummary.public_ipv4 || '-'}</span>
                </div>
              </>
            ) : (
              <Empty description="暂无全局概览数据" />
            )}
          </Card>

          <aside className="agent-rail">
            <Card
              title={
                <Space>
                  <CloudServerOutlined />
                  <span>Client 列表</span>
                </Space>
              }
              className="surface-card sidebar-card"
              bordered={false}
              extra={
                <Space size={8} wrap className="agent-rail-toolbar">
                  <Button
                    size="small"
                    shape="circle"
                    icon={<BarsOutlined />}
                    className={`agent-view-mode-button${agentViewMode === 'list' ? ' active' : ''}`}
                    aria-label={agentViewMode === 'list' ? '关闭列表模式' : '开启列表模式'}
                    aria-pressed={agentViewMode === 'list'}
                    title={agentViewMode === 'list' ? '列表模式已开启，点击切回卡片' : '开启列表模式'}
                    onClick={toggleAgentViewMode}
                  />
                  <Button size="small" icon={<ReloadOutlined />} onClick={() => void loadAgents()} loading={agentsLoading}>
                    刷新 VPS 列表
                  </Button>
                </Space>
              }
            >
              {agentsError ? (
                <Alert type="error" showIcon message="加载失败" description={agentsError} className="compact-alert" />
              ) : null}
              <Space wrap style={{ marginBottom: 12 }}>
                <Tag
                  color={!selectedTag ? 'green' : 'default'}
                  className="tag-filter-chip"
                  onClick={() => {
                    setSelectedTag('')
                    setSelectedAgentId('')
                  }}
                >
                  全部
                </Tag>
                {tagFilterOptions.map((tag: DashboardTagView) => (
                  <Tag
                    key={tag.tag}
                    color={selectedTag === tag.tag ? 'green' : 'default'}
                    className="tag-filter-chip"
                    onClick={() => {
                      setSelectedTag(tag.tag)
                      setSelectedAgentId('')
                    }}
                  >
                    {tag.tag} · {tag.agent_count}
                  </Tag>
                ))}
              </Space>
              <Spin spinning={agentsLoading}>
                {filteredAgents.length ? (
                  <List
                    className={`agent-list agent-list-${agentViewMode}`}
                    dataSource={filteredAgents}
                    pagination={{ pageSize: 10, hideOnSinglePage: true, showSizeChanger: false }}
                    renderItem={(item, index) => {
                      const active = item.agent_id === selectedAgentId
                      const renewalStatus = calculateRenewalStatus(item.renewal)
                      const trafficStatus = calculateTrafficStatus(item)
                      const trafficTotalLabel = trafficStatus.isPeriod ? '周期总流量' : '总流量'
                      const trafficSummaryValue = `${trafficStatus.total.label} · 上传 ${formatBytes(trafficStatus.upload.used)} · 下载 ${formatBytes(trafficStatus.download.used)}`
                      const cpuPercent = clampMetricPercent(item.summary.cpu)
                      const memPercent = calculateMemoryPercent(item.summary)
                      const displayStatus = agentDisplayStatus(item)
                      const statusLevel = displayStatus.level
                      const displaySortOrder = item.sort_order || index + 1
                      const addressText = item.summary.public_ipv4 || item.summary.observed_ip || item.summary.hostname || item.agent_id
                      const countryCode = agentCountryCode(item)
                      const locationText = formatAgentLocation(item, countryCode)
                      const tags = (item.tags || []).length ? item.tags || [] : ['未分组']
                      const activityText = item.realtime_at
                        ? `实时 ${formatDateTime(item.realtime_at)}`
                        : item.reported_at
                          ? `上报 ${formatDateTime(item.reported_at)}`
                          : '尚未上报'
                      return (
                        <List.Item className={`agent-list-item agent-list-item-${agentViewMode}`}>
                          <button
                            className={`agent-button agent-button-${agentViewMode}${active ? ' active' : ''}`}
                            onClick={() => {
                              if (active) {
                                setReloadToken((current) => current + 1)
                                return
                              }
                              startTransition(() => {
                                setSelectedAgentId(item.agent_id)
                              })
                            }}
                          >
                            {agentViewMode === 'list' ? (
                              <>
                                <div className="agent-list-main">
                                  <div className="agent-card-head">
                                    <div className="agent-title-line">
                                      <span className={`agent-state-dot agent-state-${statusLevel}`} />
                                      <span className="agent-order-chip">#{displaySortOrder}</span>
                                      <span className="agent-flag" title={locationText || countryCode || '未知地区'}>
                                        {countryFlag(countryCode)}
                                      </span>
                                      <span className="agent-name">{item.agent_name || item.agent_id}</span>
                                    </div>
                                    <span className={`agent-status-pill agent-status-${statusLevel}`}>
                                      {displayStatus.label}
                                    </span>
                                  </div>
                                  <div className="agent-meta agent-location agent-list-location">
                                    <span>{addressText}</span>
                                    {locationText ? <span>{locationText}</span> : null}
                                  </div>
                                  <div className="agent-tag-row agent-list-tags">
                                    {tags.map((tag) => (
                                      <span className="agent-tag-chip" key={tag}>
                                        {tag}
                                      </span>
                                    ))}
                                    {item.renewal?.bandwidth_mbps ? (
                                      <span className="agent-tag-chip">带宽 {formatBandwidth(item.renewal.bandwidth_mbps)}</span>
                                    ) : null}
                                  </div>
                                  <div className="agent-meta agent-footer-line">
                                    {item.has_config ? '已托管配置' : '待配置'} · {activityText}
                                  </div>
                                </div>
                                <div className="agent-list-metrics">
                                  <MiniProgress label="CPU" value={formatPercent(cpuPercent)} percent={cpuPercent} level={metricLevel(cpuPercent)} />
                                  <MiniProgress label="内存" value={formatMem(item.summary.mem_used, item.summary.mem_total)} percent={memPercent} level={metricLevel(memPercent)} />
                                  <MiniProgress label="上行" value={formatSpeed(item.summary.net_io_up)} level="neutral" />
                                  <MiniProgress label="下行" value={formatSpeed(item.summary.net_io_down)} level="neutral" />
                                </div>
                                <div className="agent-list-flow">
                                  {renewalStatus ? (
                                    <MiniProgress
                                      label="周期剩余"
                                      value={`${renewalStatus.remainingLabel} · ${renewalStatus.endLabel} · ${renewalStatus.autoRenew ? '自动刷新' : '不自动刷新'}`}
                                      percent={renewalStatus.percent}
                                      level={renewalStatus.level}
                                      className="agent-wide-progress"
                                    />
                                  ) : null}
                                  <MiniProgress
                                    label={trafficTotalLabel}
                                    value={trafficSummaryValue}
                                    percent={trafficStatus.total.percent}
                                    showTrack
                                    level={trafficStatus.total.level}
                                    className="agent-wide-progress"
                                  />
                                </div>
                              </>
                            ) : (
                              <>
                                <div className="agent-card-head">
                                  <div className="agent-title-line">
                                    <span className={`agent-state-dot agent-state-${statusLevel}`} />
                                    <span className="agent-order-chip">#{displaySortOrder}</span>
                                    <span className="agent-flag" title={locationText || countryCode || '未知地区'}>
                                      {countryFlag(countryCode)}
                                    </span>
                                    <span className="agent-name">{item.agent_name || item.agent_id}</span>
                                  </div>
                                  <span className={`agent-status-pill agent-status-${statusLevel}`}>
                                    {displayStatus.label}
                                  </span>
                                </div>
                                <div className="agent-meta agent-location">
                                  <span>{addressText}</span>
                                  {locationText ? <span>{locationText}</span> : null}
                                </div>
                                <div className="agent-tag-row">
                                  {tags.map((tag) => (
                                    <span className="agent-tag-chip" key={tag}>
                                      {tag}
                                    </span>
                                  ))}
                                  {item.renewal?.bandwidth_mbps ? (
                                    <span className="agent-tag-chip">带宽 {formatBandwidth(item.renewal.bandwidth_mbps)}</span>
                                  ) : null}
                                </div>
                                <div className="agent-meter-grid">
                                  <MiniProgress label="CPU" value={formatPercent(cpuPercent)} percent={cpuPercent} level={metricLevel(cpuPercent)} />
                                  <MiniProgress label="内存" value={formatMem(item.summary.mem_used, item.summary.mem_total)} percent={memPercent} level={metricLevel(memPercent)} />
                                  <MiniProgress label="上行" value={formatSpeed(item.summary.net_io_up)} level="neutral" />
                                  <MiniProgress label="下行" value={formatSpeed(item.summary.net_io_down)} level="neutral" />
                                </div>
                                {renewalStatus ? (
                                  <MiniProgress
                                    label="周期剩余"
                                    value={`${renewalStatus.remainingLabel} · ${renewalStatus.endLabel} · ${renewalStatus.autoRenew ? '自动刷新' : '不自动刷新'}`}
                                    percent={renewalStatus.percent}
                                    level={renewalStatus.level}
                                    className="agent-wide-progress"
                                  />
                                ) : null}
                                <div className="agent-traffic-grid">
                                  <MiniProgress
                                    label={trafficTotalLabel}
                                    value={trafficSummaryValue}
                                    percent={trafficStatus.total.percent}
                                    showTrack
                                    level={trafficStatus.total.level}
                                    className="agent-wide-progress"
                                  />
                                </div>
                                <div className="agent-meta agent-footer-line">
                                  {item.has_config ? '已托管配置' : '待配置'} · {activityText}
                                </div>
                              </>
                            )}
                          </button>
                        </List.Item>
                      )
                    }}
                  />
                ) : (
                  <Empty description={selectedTag ? '该标签下暂无 client' : '暂无已注册 client'} />
                )}
              </Spin>
            </Card>
          </aside>

          <main className="main-stage">
            {dashboardView ? (
              <Card className="surface-card topology-entry-card" bordered={false}>
                <div className="topology-entry">
                  <div>
                    <div className="eyebrow">Topology Map</div>
                    <Title level={4}>链路拓扑图</Title>
                    <Text type="secondary">
                      大型拓扑图已默认隐藏；当前范围 {topologyScopeLabel}，共 {filteredChains.length} 条客户端链路。
                    </Text>
                  </div>
                  <Space wrap>
                    {topologyVisible ? <Button onClick={() => setTopologyVisible(false)}>收起拓扑图</Button> : null}
                    <Button type="primary" onClick={openTopologyPanel}>
                      {topologyVisible ? '跳到拓扑图' : '打开拓扑图'}
                    </Button>
                  </Space>
                </div>
              </Card>
            ) : null}

            {dashboardView && topologyVisible ? (
              <div id="topology-panel">
                {renderCNFlowPanel({
                    dashboardView,
                    selectedTag,
                    selectedAgentId,
                    agents: dashboardView.agents,
                    chains: filteredChains,
                    onSelectAgent: (agentID) => setSelectedAgentId(agentID),
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

            {overviewError && selectedAgentId ? (
              <Alert
                className="surface-card alert-card"
                type="warning"
                showIcon
                message="x-ui 概览暂不可用"
                description={overviewError}
              />
            ) : null}

            {selectedAgent ? (
              <Card className="surface-card tabs-card" bordered={false}>
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
                          scroll={{ x: 1200 }}
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

  function openTopologyPanel() {
    setTopologyVisible(true)
    window.setTimeout(() => {
      document.getElementById('topology-panel')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
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

function VisualEffects() {
  useEffect(() => {
    const backgroundImage = window.CustomBackgroundImage || DEFAULT_BACKGROUND_IMAGE
    document.documentElement.style.setProperty('--custom-bg-image', `url("${backgroundImage}")`)
  }, [])

  return null
}

interface LoginScreenProps {
  loginForm: { username: string; password: string }
  loginLoading: boolean
  onChange: (value: { username: string; password: string }) => void
  onLogin: () => void
}

function LoginScreen({ loginForm, loginLoading, onChange, onLogin }: LoginScreenProps) {
  const canLogin = Boolean(loginForm.username && loginForm.password)

  return (
    <div className="login-shell">
      <section className="login-panel">
        <div className="login-brand">
          <div className="login-mark">
            <LockOutlined />
          </div>
          <div>
            <Title level={2}>南风VPS监控</Title>
            <Text type="secondary">管理员登录</Text>
          </div>
        </div>
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
          <div>
            <Text type="secondary">用户名</Text>
            <Input
              size="large"
              autoFocus
              value={loginForm.username}
              onChange={(event) => onChange({ ...loginForm, username: event.target.value })}
              onPressEnter={() => {
                if (canLogin) {
                  onLogin()
                }
              }}
            />
          </div>
          <div>
            <Text type="secondary">密码</Text>
            <Input.Password
              size="large"
              value={loginForm.password}
              onChange={(event) => onChange({ ...loginForm, password: event.target.value })}
              onPressEnter={() => {
                if (canLogin) {
                  onLogin()
                }
              }}
            />
          </div>
          <Button
            block
            size="large"
            type="primary"
            icon={<LockOutlined />}
            loading={loginLoading}
            disabled={!canLogin}
            onClick={onLogin}
          >
            登录
          </Button>
        </Space>
      </section>
    </div>
  )
}

interface ConfigPanelProps {
  selectedAgent?: AgentListItem
  managedConfig: ManagedAgentConfig | null
  configLoading: boolean
  configSavingSection: ConfigSectionKey | null
  configError: string
  onSave: (section: ConfigSectionKey) => void
  onAgentNameChange: (value: string) => void
  onSortOrderChange: (value: number) => void
  tagOptions: string[]
  newTagName: string
  tagSaving: boolean
  onNewTagNameChange: (value: string) => void
  onCreateTag: () => void
  onTagsChange: (values: string[]) => void
  onRenewalChange: (patch: Partial<VPSRenewalConfig>) => void
  entryAddressInputText: string
  onEntryAddressesTextChange: (value: string) => void
  onEntryChange: (patch: Partial<AgentEntryConfig>) => void
  onXUIChange: (patch: Partial<XUIConfig>) => void
  configAudits: ConfigAuditLog[]
  configAuditsLoading: boolean
  currencyOptions: CurrencyCode[]
}

function renderManagedConfigPanel(props: ConfigPanelProps) {
  const {
    selectedAgent,
    managedConfig,
    configLoading,
    configSavingSection,
    configError,
    onSave,
    onAgentNameChange,
    onSortOrderChange,
    tagOptions,
    newTagName,
    tagSaving,
    onNewTagNameChange,
    onCreateTag,
    onTagsChange,
    onRenewalChange,
    entryAddressInputText,
    onEntryAddressesTextChange,
    onEntryChange,
    onXUIChange,
    configAudits,
    configAuditsLoading,
    currencyOptions,
  } = props

  if (!selectedAgent) {
    return <Empty description="先选择一个 client" />
  }

  if (configLoading && !managedConfig) {
    return (
      <div className="empty-stage">
        <Spin size="large" />
      </div>
    )
  }

  if (!managedConfig) {
    return <Empty description="暂无托管配置" />
  }

  const entryConfig: AgentEntryConfig = {
    addresses: managedConfig.entry?.addresses || [],
    mappings: managedConfig.entry?.mappings || [],
  }
  const updateEntryMapping = (index: number, patch: Partial<AgentEntryMapping>) => {
    const mappings = (entryConfig.mappings || []).map((mapping, currentIndex) => (currentIndex === index ? { ...mapping, ...patch } : mapping))
    onEntryChange({ mappings })
  }
  const addEntryMapping = () => {
    onEntryChange({
      mappings: [
        ...(entryConfig.mappings || []),
        {
          address: entryConfig.addresses?.[0] || managedConfig.xui.base_url || '',
          external_port: 0,
          internal_port: 0,
          protocol: 'vless',
          note: '',
        },
      ],
    })
  }
  const removeEntryMapping = (index: number) => {
    onEntryChange({ mappings: (entryConfig.mappings || []).filter((_, currentIndex) => currentIndex !== index) })
  }
  const sectionSaving = Boolean(configSavingSection)
  const sectionSaveButton = (section: ConfigSectionKey, label: string) => (
    <Button
      type="primary"
      size="small"
      icon={<SaveOutlined />}
      onClick={() => onSave(section)}
      loading={configSavingSection === section}
      disabled={sectionSaving && configSavingSection !== section}
    >
      {label}
    </Button>
  )

  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      {configError ? (
        <Alert
          type="warning"
          showIcon
          message="托管配置状态"
          description={configError}
          className="compact-alert"
        />
      ) : null}

      <Alert
        type="info"
        showIcon
        message="统一配置说明"
        description="client 注册后，server 会保存并下发 x-ui 托管参数。后台修改后，不需要再改 client 本地文件，下一次轮询会自动使用新配置。"
        className="compact-alert"
      />

      <Card className="config-section-card" bordered={false}>
        <div className="section-title-row">
          <Title level={4}>Client 信息</Title>
          {sectionSaveButton('client', '保存 Client 信息')}
        </div>
        <Row gutter={[16, 16]}>
          <Col xs={24} md={8}>
            <Text type="secondary">Agent ID</Text>
            <Input value={managedConfig.agent_id || selectedAgent.agent_id} disabled />
          </Col>
          <Col xs={24} md={8}>
            <Text type="secondary">展示名称</Text>
            <Input value={managedConfig.agent_name || ''} onChange={(event) => onAgentNameChange(event.target.value)} />
          </Col>
          <Col xs={24} md={8}>
            <Text type="secondary">排序序号</Text>
            <InputNumber
              style={{ width: '100%' }}
              min={1}
              precision={0}
              value={managedConfig.sort_order || selectedAgent.sort_order || 1}
              onChange={(value) => onSortOrderChange(Number(value || selectedAgent.sort_order || 1))}
            />
          </Col>
          <Col xs={24}>
            <Text type="secondary">标签</Text>
            <Select
              mode="multiple"
              allowClear
              style={{ width: '100%' }}
              value={managedConfig.tags || []}
              placeholder="选择已创建标签"
              options={tagOptions.map((tag) => ({ value: tag, label: tag }))}
              onChange={(values) => onTagsChange(values)}
            />
            <Space.Compact style={{ width: '100%', marginTop: 8 }}>
              <Input
                value={newTagName}
                placeholder="创建固定标签，例如 PH、家宽、NAT"
                onChange={(event) => onNewTagNameChange(event.target.value)}
                onPressEnter={onCreateTag}
              />
              <Button onClick={onCreateTag} loading={tagSaving}>创建标签</Button>
            </Space.Compact>
            <Text type="secondary">标签先创建再多选；保存 Client 信息后会应用到当前 Client。</Text>
          </Col>
        </Row>
      </Card>

      <Card className="config-section-card" bordered={false}>
        <div className="section-title-row">
          <Title level={4}>VPS 信息</Title>
          {sectionSaveButton('renewal', '保存 VPS 信息')}
        </div>
        <Row gutter={[16, 16]}>
          <Col xs={24} md={8}>
            <div className="switch-row">
              <span>启用自动计算</span>
              <Switch checked={Boolean(managedConfig.renewal?.enabled)} onChange={(checked) => onRenewalChange({ enabled: checked })} />
            </div>
          </Col>
          <Col xs={24} md={8}>
            <div className="switch-row">
              <span>周期到期后自动刷新</span>
              <Switch checked={Boolean(managedConfig.renewal?.auto_renew)} onChange={(checked) => onRenewalChange({ auto_renew: checked })} />
            </div>
          </Col>
          <Col xs={24} md={8}>
            <Text type="secondary">到期时间</Text>
            <Input
              type="date"
              value={managedConfig.renewal?.expire_date || ''}
              onChange={(event) => onRenewalChange({ enabled: true, expire_date: event.target.value })}
            />
          </Col>
          <Col xs={24} md={8}>
            <Text type="secondary">周期开始时间</Text>
            <Input
              type="date"
              value={managedConfig.renewal?.start_date || ''}
              onChange={(event) => onRenewalChange({ enabled: true, start_date: event.target.value })}
            />
          </Col>
          <Col xs={24} md={8}>
            <Text type="secondary">续费周期</Text>
            <Select
              style={{ width: '100%' }}
              value={managedConfig.renewal?.cycle || 'month'}
              options={[
                { value: 'week', label: '每周' },
                { value: 'month', label: '每月' },
                { value: 'quarter', label: '每季' },
                { value: 'year', label: '每年' },
              ]}
              onChange={(value) => onRenewalChange({ cycle: value })}
            />
          </Col>
          <Col xs={24} md={8}>
            <Text type="secondary">费用金额</Text>
            <InputNumber
              style={{ width: '100%' }}
              min={0}
              precision={2}
              value={managedConfig.renewal?.cost_amount || 0}
              onChange={(value) => onRenewalChange({ cost_amount: Number(value || 0) })}
              placeholder="例如 8.99"
            />
          </Col>
          <Col xs={24} md={8}>
            <Text type="secondary">费用币种</Text>
            <Select
              style={{ width: '100%' }}
              value={managedConfig.renewal?.cost_currency || DEFAULT_COST_CURRENCY}
              options={currencyOptions.map((currency) => ({ value: currency, label: currency }))}
              onChange={(value) => onRenewalChange({ cost_currency: value })}
            />
          </Col>
          <Col xs={24} md={8}>
            <Text type="secondary">费用续费周期</Text>
            <Select
              style={{ width: '100%' }}
              value={managedConfig.renewal?.cost_cycle || 'month'}
              options={[
                { value: 'month', label: '每月' },
                { value: 'quarter', label: '每季' },
                { value: 'year', label: '每年' },
              ]}
              onChange={(value) => onRenewalChange({ cost_cycle: value })}
            />
          </Col>
          <Col xs={24} md={8}>
            <Text type="secondary">周期总流量 (GB)</Text>
            <InputNumber
              style={{ width: '100%' }}
              min={0}
              precision={2}
              value={bytesToGB(managedConfig.renewal?.traffic_limit_bytes || 0)}
              onChange={(value) => onRenewalChange({ traffic_limit_bytes: gbToBytes(Number(value || 0)) })}
              placeholder="例如 1024"
            />
          </Col>
          <Col xs={24} md={8}>
            <Text type="secondary">带宽大小 (Mbps)</Text>
            <InputNumber
              style={{ width: '100%' }}
              min={0}
              precision={2}
              value={managedConfig.renewal?.bandwidth_mbps || 0}
              onChange={(value) => onRenewalChange({ bandwidth_mbps: Number(value || 0) })}
              placeholder="例如 1000"
            />
          </Col>
          <Col xs={24}>
            <Text type="secondary">{formatRenewalHint(managedConfig.renewal)}</Text>
          </Col>
        </Row>
      </Card>

      <Card className="config-section-card" bordered={false}>
        <div className="section-title-row">
          <Title level={4}>X-UI 托管配置</Title>
          {sectionSaveButton('xui', '保存 X-UI 配置')}
        </div>
        <Row gutter={[16, 16]}>
          <Col xs={24} md={12}>
            <div className="switch-row">
              <span>启用 x-ui 采集</span>
              <Switch checked={managedConfig.xui.enabled} onChange={(checked) => onXUIChange({ enabled: checked })} />
            </div>
          </Col>
          <Col xs={24} md={12}>
            <div className="switch-row">
              <span>跳过 TLS 校验</span>
              <Switch checked={managedConfig.xui.skip_tls_verify} onChange={(checked) => onXUIChange({ skip_tls_verify: checked })} />
            </div>
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">Base URL</Text>
            <Input value={managedConfig.xui.base_url || ''} placeholder="https://127.0.0.1:2053" onChange={(event) => onXUIChange({ base_url: event.target.value })} />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">用户名</Text>
            <Input value={managedConfig.xui.username || ''} onChange={(event) => onXUIChange({ username: event.target.value })} />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">密码</Text>
            <Input.Password value={managedConfig.xui.password || ''} onChange={(event) => onXUIChange({ password: event.target.value })} />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">二步验证码</Text>
            <Input value={managedConfig.xui.two_factor_code || ''} onChange={(event) => onXUIChange({ two_factor_code: event.target.value })} />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">节点维护方式</Text>
            <Input value="节点请直接在 x-ui 前端手动维护；中心只负责出站与转发编排" disabled />
          </Col>
        </Row>
      </Card>

      <Card className="config-section-card" bordered={false}>
        <div className="section-title-row">
          <Title level={4}>入口地址 / NAT 映射</Title>
          <Space wrap>
            <Button size="small" icon={<PlusOutlined />} onClick={addEntryMapping}>
              添加映射
            </Button>
            {sectionSaveButton('entry', '保存入口/NAT')}
          </Space>
        </div>
        <Alert
          type="info"
          showIcon
          className="compact-alert"
          message="用于 NAT/家宽落地匹配"
          description="当转发配置里填写的是连接 IP/域名，但 Client 查询到的公网 IP 不同，可以在这里配置入口地址和外部端口到内部节点端口的映射。拓扑会优先按入口地址 + 外部端口 + 节点类型匹配。"
        />
        <Row gutter={[16, 16]}>
          <Col xs={24}>
            <Text type="secondary">入口地址</Text>
            <Input.TextArea
              value={entryAddressInputText}
              autoSize={{ minRows: 2, maxRows: 5 }}
              placeholder="每行一个入口域名/IP，例如 att.kynbbz.top 或 1.2.3.4"
              onChange={(event) => onEntryAddressesTextChange(event.target.value)}
            />
            <Text type="secondary">这些地址会加入该 Client 的可匹配入口；映射可以进一步指定端口转换。</Text>
          </Col>
        </Row>
        <Space direction="vertical" size="small" className="entry-mapping-list">
          {(entryConfig.mappings || []).length ? (
            (entryConfig.mappings || []).map((mapping, index) => (
              <div key={`entry-mapping-${index}`} className="entry-mapping-row">
                <Input
                  value={mapping.address || ''}
                  placeholder="入口域名/IP"
                  onChange={(event) => updateEntryMapping(index, { address: event.target.value })}
                />
                <InputNumber
                  min={0}
                  precision={0}
                  value={mapping.external_port || 0}
                  placeholder="外部端口"
                  onChange={(value) => updateEntryMapping(index, { external_port: Number(value || 0) })}
                />
                <InputNumber
                  min={0}
                  precision={0}
                  value={mapping.internal_port || 0}
                  placeholder="内部端口"
                  onChange={(value) => updateEntryMapping(index, { internal_port: Number(value || 0) })}
                />
                <Select
                  value={mapping.protocol || 'vless'}
                  options={[
                    { value: 'vless', label: 'VLESS' },
                    { value: 'vmess', label: 'VMess' },
                    { value: 'http', label: 'HTTP' },
                    { value: 'socks', label: 'Socks' },
                  ]}
                  onChange={(value) => updateEntryMapping(index, { protocol: value })}
                />
                <Input
                  value={mapping.note || ''}
                  placeholder="备注"
                  onChange={(event) => updateEntryMapping(index, { note: event.target.value })}
                />
                <Button danger icon={<DeleteOutlined />} onClick={() => removeEntryMapping(index)}>
                  删除
                </Button>
              </div>
            ))
          ) : (
            <Empty description="暂无端口映射；如果连接 IP 与出口 IP 不同，建议添加一条 NAT 映射" />
          )}
        </Space>
      </Card>

      <Card className="config-section-card" bordered={false}>
        <Title level={4}>配置修改记录</Title>
        <Spin spinning={configAuditsLoading}>
          {configAudits.length ? (
            <List
              dataSource={configAudits}
              renderItem={(item) => (
                <List.Item>
                  <div>
                    <Text strong>{item.actor || 'system'}</Text>
                    <div className="muted-line">
                      {formatDateTime(item.created_at)} · {summarizeConfigAudit(item)}
                    </div>
                  </div>
                </List.Item>
              )}
            />
          ) : (
            <Empty description="暂无配置修改记录" />
          )}
        </Spin>
      </Card>

    </Space>
  )
}

function MiniProgress(props: {
  label: string
  value: string
  percent?: number
  showTrack?: boolean
  level?: 'ok' | 'warn' | 'bad' | 'neutral'
  className?: string
}) {
  const level = props.level || 'ok'
  const hasPercent = typeof props.percent === 'number' && Number.isFinite(props.percent)
  const showTrack = hasPercent || Boolean(props.showTrack)
  const percent = clampMetricPercent(props.percent)

  return (
    <div className={`mini-progress mini-progress-${level}${props.className ? ` ${props.className}` : ''}`}>
      <div className="mini-progress-head">
        <span>{props.label}</span>
        <span>{props.value}</span>
      </div>
      {showTrack ? (
        <div className="mini-progress-track" aria-label={`${props.label} ${percent.toFixed(1)}%`}>
          <span className="mini-progress-fill" style={{ width: `${percent}%` }} />
        </div>
      ) : null}
    </div>
  )
}

function renderTelegramBotPanel(props: {
  bots: TelegramBot[]
  loading: boolean
  saving: boolean
  editingID: number | null
  form: TelegramBotForm
  onFormChange: (form: TelegramBotForm) => void
  onSave: () => void
  onRefresh: () => void
  onEdit: (bot: TelegramBot) => void
  onCancelEdit: () => void
  onDelete: (id: number) => void
  onTest: (id: number) => void
}) {
  const { bots, loading, saving, editingID, form, onFormChange, onSave, onRefresh, onEdit, onCancelEdit, onDelete, onTest } = props
  const update = (patch: Partial<TelegramBotForm>) => onFormChange({ ...form, ...patch })
  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Alert
        type="info"
        showIcon
        message="告警发送方式"
        description="目前会对 Client 离线、X-UI 采集异常、Xray 异常、续费周期临近到期、周期流量超过 75%/90% 发送 Telegram 告警；同一告警默认 6 小时内不会重复刷屏。"
      />
      <Card className="config-section-card" bordered={false}>
        <Title level={4}>{editingID ? '编辑机器人' : '新增机器人'}</Title>
        <Row gutter={[16, 16]}>
          <Col xs={24} md={12}>
            <Text type="secondary">名称</Text>
            <Input value={form.name} placeholder="例如: 主告警群" onChange={(event) => update({ name: event.target.value })} />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">Chat ID</Text>
            <Input value={form.chat_id} placeholder="群/频道/个人 chat_id" onChange={(event) => update({ chat_id: event.target.value })} />
          </Col>
          <Col xs={24} md={16}>
            <Text type="secondary">Bot Token</Text>
            <Input.Password
              value={form.bot_token}
              placeholder={editingID ? '留空表示沿用原 token' : '123456:ABC...'}
              onChange={(event) => update({ bot_token: event.target.value })}
            />
          </Col>
          <Col xs={24} md={8}>
            <div className="switch-row">
              <span>启用告警</span>
              <Switch checked={form.enabled} onChange={(checked) => update({ enabled: checked })} />
            </div>
          </Col>
        </Row>
        <Space style={{ marginTop: 16 }}>
          <Button type="primary" icon={<SaveOutlined />} loading={saving} onClick={onSave}>
            {editingID ? '保存机器人' : '新增机器人'}
          </Button>
          {editingID ? <Button onClick={onCancelEdit}>取消编辑</Button> : null}
        </Space>
      </Card>
      <Card className="config-section-card" bordered={false}>
        <div className="section-title-row">
          <Title level={4}>已配置机器人</Title>
          <Button size="small" icon={<ReloadOutlined />} loading={loading} onClick={onRefresh}>
            刷新
          </Button>
        </div>
        <Spin spinning={loading}>
          {bots.length ? (
            <List
              dataSource={bots}
              renderItem={(bot) => (
                <List.Item className="telegram-bot-list-item">
                  <div className="telegram-bot-main">
                    <Text strong>{bot.name}</Text>
                    <div className="muted-line">
                      Chat {bot.chat_id} · Token {bot.has_bot_token ? '已保存' : '未设置'} · {bot.enabled ? '启用' : '停用'} · 更新 {formatDateTime(bot.updated_at)}
                    </div>
                  </div>
                  <Space wrap>
                    <Button size="small" onClick={() => onTest(bot.id)}>
                      测试
                    </Button>
                    <Button size="small" icon={<EditOutlined />} onClick={() => onEdit(bot)}>
                      编辑
                    </Button>
                    <Button size="small" danger icon={<DeleteOutlined />} onClick={() => onDelete(bot.id)}>
                      删除
                    </Button>
                  </Space>
                </List.Item>
              )}
            />
          ) : (
            <Empty description="还没有 Telegram 告警机器人" />
          )}
        </Spin>
      </Card>
    </Space>
  )
}

function renderGlobalOverviewPanel(props: {
  dashboardView: GlobalDashboardView | null
  selectedTag: string
  links: TopologyLinkView[]
  onSelectTag: (value: string) => void
}) {
  const { dashboardView, selectedTag, links, onSelectTag } = props

  if (!dashboardView) {
    return <Empty description="暂无总览数据" />
  }

  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Alert
        type="info"
        showIcon
        message="链路明细"
        description={`这里保留标签分组、已自动匹配链路和客户端转发链明细。页面会每 ${Math.floor(DASHBOARD_AUTO_REFRESH_MS / 1000)} 秒自动刷新一次统计与匹配结果。`}
      />

      <Card className="config-section-card" bordered={false}>
        <Space wrap>
          <Tag color={!selectedTag ? 'green' : 'default'} className="tag-filter-chip" onClick={() => onSelectTag('')}>
            全部
          </Tag>
          {dashboardView.tags.map((tag) => (
            <Tag
              key={tag.tag}
              color={selectedTag === tag.tag ? 'green' : 'default'}
              className="tag-filter-chip"
              onClick={() => onSelectTag(tag.tag)}
            >
              {tag.tag} · {tag.client_count}
            </Tag>
          ))}
        </Space>
      </Card>

      <Card className="config-section-card" bordered={false}>
        <Title level={4}>跨 Client 已匹配链路</Title>
        {links.length ? (
          <div className="topology-link-list">
            {links.map((link) => (
              <section key={link.key} className="topology-link-card">
                <div className="topology-link-row">
                  <Text strong>{link.source.agent_name || link.source.agent_id}</Text>
                  <Tag color="gold">{link.source.outbound_tag || '-'}</Tag>
                  <span className="topology-arrow">→</span>
                  <Text strong>{link.target.agent_name || link.target.agent_id}</Text>
                  <Tag color="cyan">{link.target.inbound_name || link.target.inbound_tag || '-'}</Tag>
                </div>
                <div className="muted-line">
                  {link.source.target || '-'} → {link.target.entry_addresses?.[0] || link.target.domains?.[0] || link.target.ips?.[0] || '-'}:{link.target.port || 0}
                </div>
                {link.source.resolved_ips?.length || link.target.resolved_ips?.length ? (
                  <div className="muted-line">
                    解析 IP: {(link.source.resolved_ips || []).join(', ') || '-'} → {(link.target.resolved_ips || []).join(', ') || '-'}
                  </div>
                ) : null}
                <div className="muted-line">
                  {link.match_reason || '-'} · {confidenceLabel(link.match_confidence)} · score {link.match_score}
                </div>
                {link.match_explanation ? <div className="muted-line">{link.match_explanation}</div> : null}
              </section>
            ))}
          </div>
        ) : (
          <Empty description="当前没有自动匹配上的跨 client 出站链路" />
        )}
      </Card>
    </Space>
  )
}

function renderCNFlowPanel(props: {
  dashboardView: GlobalDashboardView
  selectedTag: string
  selectedAgentId: string
  agents: DashboardAgentView[]
  chains: ClientChainView[]
  onSelectAgent: (agentID: string) => void
  onJumpNode: (agentID?: string, nodeLabel?: string) => void
  canOpenXUI: boolean
  onOpenXUI: () => void
  canRefreshCurrentNode: boolean
  currentNodeLoading: boolean
  onRefreshCurrentNode: () => void
}) {
  const {
    dashboardView,
    selectedTag,
    selectedAgentId,
    agents,
    chains,
    onSelectAgent,
    onJumpNode,
    canOpenXUI,
    onOpenXUI,
    canRefreshCurrentNode,
    currentNodeLoading,
    onRefreshCurrentNode,
  } = props
  const rows = buildCNFlowRows(chains, agents)
  const selectedAgent = agents.find((agent) => agent.agent_id === selectedAgentId)
  const flowMode: 'cn' | 'agent' | 'tag' = selectedAgentId ? 'agent' : selectedTag ? 'tag' : 'cn'
  const visibleRows = selectedAgentId ? rows.filter((row) => row.rootAgentID === selectedAgentId) : rows
  const toolbarAgents = selectedTag ? agents.filter((agent) => hasSelectedTag(agent.tags, selectedTag)) : agents
  const headerTitle = flowMode === 'agent' ? `${selectedAgent?.agent_name || selectedAgentId} 节点客户端拓扑` : flowMode === 'tag' ? `${selectedTag} 标签链路拓扑` : 'CN 出发链路拓扑'

  const renderSource = () => {
    return (
      <div className="cn-flow-source">
        <div className="cn-source-orb">CN</div>
        <Text>中国大陆出发</Text>
        <small>统一从 CN 访问源进入</small>
      </div>
    )
  }

  const renderAgentNode = (row: CNFlowRow) => (
    <button className="cn-flow-node agent-node" onClick={() => onSelectAgent(row.rootAgentID)}>
      <span className="node-kicker">Client</span>
      <strong>{row.rootAgentName || row.rootAgentID}</strong>
      <small>{row.rootAgentTags?.length ? row.rootAgentTags.join(' · ') : row.rootAgentID}</small>
    </button>
  )

  const renderEntryNode = (row: CNFlowRow) => (
    <button className="cn-flow-node entry-node" onClick={() => onSelectAgent(row.rootAgentID)}>
      <span className="node-kicker">Client 内节点 / 入站</span>
      <strong>{row.rootAgentName || row.rootAgentID}</strong>
      <small>{row.entryLabel}</small>
    </button>
  )

  const renderClientNode = (row: CNFlowRow) => (
    <div className="cn-flow-node client-node">
      <span className="node-kicker">节点客户端</span>
      <strong>{row.clientLabel}</strong>
      <small>{row.clientDetail || '未备注'}</small>
    </div>
  )

  const renderTargetNode = (hop: CNFlowHop, compact = false) =>
    hop.targetAgentID ? (
      <>
        <div className="cn-flow-arrow">→</div>
        <button className={`cn-flow-node target-node${compact ? ' compact-target-node' : ''}`} onClick={() => onJumpNode(hop.targetAgentID, hop.targetInboundLabel)}>
          <span className="node-kicker">{compact ? '后续落地点' : '下一跳 Client / 节点'}</span>
          <strong>{hop.targetAgentName || hop.targetAgentID}</strong>
          <small>
            {hop.targetInboundLabel || '-'}
            {hop.targetClientLabel ? ` · ${hop.targetClientLabel}` : hop.targetProtocol ? ` · ${hop.targetProtocol} 入站` : ''}
          </small>
        </button>
      </>
    ) : null

  const renderForwardNode = (hop: CNFlowHop) =>
    hop.targetAgentID ? (
      <button className="cn-flow-node forward-node" onClick={() => onJumpNode(hop.targetAgentID, hop.targetInboundLabel)}>
        <span className="node-kicker">转发出站 / 下一跳节点</span>
        <strong>{hop.outboundLabel}</strong>
        <small>{hop.outboundDetail}</small>
        <small>
          → {hop.targetAgentName || hop.targetAgentID}
          {hop.targetInboundLabel ? ` / ${hop.targetInboundLabel}` : ''}
          {hop.targetClientLabel ? ` · ${hop.targetClientLabel}` : hop.targetProtocol ? ` · ${hop.targetProtocol} 入站` : ''}
        </small>
      </button>
    ) : (
      <div className="cn-flow-node rule-node">
        <span className="node-kicker">本跳转发出站</span>
        <strong>{hop.outboundLabel}</strong>
        <small>{hop.outboundDetail}</small>
      </div>
    )

  const renderHopSegments = (row: CNFlowRow, showCurrentNode: boolean) =>
    row.hops.length ? (
      row.hops.map((hop, index) => {
        if (index > 0) {
          return hop.targetAgentID ? <div key={`${row.key}-${index}`} className="cn-flow-hop-group compact-hop">{renderTargetNode(hop, true)}</div> : null
        }
        return (
          <div key={`${row.key}-${index}`} className="cn-flow-hop-group">
            <div className="cn-flow-hop-label">
              <b>第 1 跳</b>
            </div>
            <div className="cn-flow-hop-body">
              {showCurrentNode ? (
                <>
                  <button className="cn-flow-node entry-node" onClick={() => onSelectAgent(hop.currentAgentID || row.rootAgentID)}>
                    <span className="node-kicker">本跳 VPS 入站</span>
                    <strong>{hop.currentAgentName || hop.currentAgentID || row.rootAgentName || row.rootAgentID}</strong>
                    <small>{hop.currentInboundLabel || row.entryLabel}</small>
                  </button>
                  <div className="cn-flow-arrow">→</div>
                </>
              ) : null}
              <div className="cn-flow-arrow rule-arrow">
                <span>规则 R{hop.ruleIndex || '?'}</span>
                <em>{hop.routeScope || 'route'}</em>
              </div>
              {renderForwardNode(hop)}
            </div>
          </div>
        )
      })
    ) : (
      <>
        <div className="cn-flow-arrow rule-arrow">
          <span>规则 ?</span>
          <em>unmatched</em>
        </div>
        <div className="cn-flow-node rule-node">
          <span className="node-kicker">未匹配出站</span>
          <strong>-</strong>
          <small>未找到可展示的转发规则</small>
        </div>
      </>
    )

  const renderFlowTail = (row: CNFlowRow) => (
    <>
      {renderClientNode(row)}
      <div className="cn-flow-arrow">→</div>
      <div className="cn-flow-hop-column">{renderHopSegments(row, flowMode === 'cn')}</div>
      <div className="cn-flow-arrow final-arrow">→</div>
      <div className={`cn-flow-node country-node country-${row.exitCountryCode.toLowerCase()}`}>
        <span className="node-kicker">最终出站国家</span>
        <strong>{row.exitCountryLabel}</strong>
        <small>{row.exitReason}</small>
      </div>
      {shouldShowChainWarning(row) ? <Tag color="orange" className="cn-flow-warning">{translateChainReason(row.unresolvedReason || '')}</Tag> : null}
    </>
  )

  const groupRowsByEntry = (inputRows: CNFlowRow[]) =>
    Array.from(
      inputRows.reduce((groups, row) => {
        const key = `${row.rootAgentID}:${row.entryLabel}`
        const group = groups.get(key)
        if (group) {
          group.rows.push(row)
        } else {
          groups.set(key, { key, lead: row, rows: [row] })
        }
        return groups
      }, new Map<string, { key: string; lead: CNFlowRow; rows: CNFlowRow[] }>()),
    ).map(([, group]) => group)

  const renderEntryCluster = (group: { key: string; lead: CNFlowRow; rows: CNFlowRow[] }) => (
    <section key={group.key} className="cn-flow-entry-cluster">
      {renderEntryNode(group.lead)}
      <div className="cn-flow-arrow cluster-arrow">→</div>
      <div className="cn-flow-entry-cluster-rows">
        {group.rows.map((row) => (
          <div key={row.key} className={`cn-flow-lane${row.loopDetected ? ' loop' : ''}`}>
            {renderFlowTail(row)}
          </div>
        ))}
      </div>
    </section>
  )

  const tagAgentGroups = Array.from(
    visibleRows.reduce((groups, row) => {
      const key = row.rootAgentID
      const group = groups.get(key)
      if (group) {
        group.rows.push(row)
      } else {
        groups.set(key, { key, lead: row, rows: [row] })
      }
      return groups
    }, new Map<string, { key: string; lead: CNFlowRow; rows: CNFlowRow[] }>()),
  ).map(([, group]) => ({ ...group, entryGroups: groupRowsByEntry(group.rows) }))

  const selectedEntryGroups = groupRowsByEntry(visibleRows)

  return (
    <Card className={`cn-flow-card cn-flow-card-${flowMode}`} bordered={false}>
      <div className="cn-flow-header">
        <div>
          <div className="eyebrow">CN Access Route Map</div>
          <Title level={3}>{headerTitle}</Title>
        </div>
        <div className="cn-flow-header-actions">
          <Space wrap>
            <Button disabled={!canOpenXUI} onClick={onOpenXUI}>
              打开 x-ui 面板
            </Button>
            <Button icon={<ReloadOutlined />} type="primary" disabled={!canRefreshCurrentNode} loading={currentNodeLoading} onClick={onRefreshCurrentNode}>
              刷新当前节点
            </Button>
          </Space>
          <Space wrap>
            <Tag color="red">起点 CN</Tag>
            <Tag color="green">Client {dashboardView.totals.client_count}</Tag>
            <Tag color="cyan">链路 {dashboardView.totals.link_count}</Tag>
            <Tag color="gold">出口 {uniqueCountries(rows).length}</Tag>
            {selectedTag ? <Tag>{selectedTag}</Tag> : <Tag>全部分组</Tag>}
          </Space>
        </div>
      </div>

      <div className="cn-flow-toolbar">
        <button className={`cn-flow-agent-filter${!selectedAgentId ? ' active' : ''}`} onClick={() => onSelectAgent('')}>
          全部链路
        </button>
        {toolbarAgents.map((agent) => (
          <button
            key={agent.agent_id}
            className={`cn-flow-agent-filter${selectedAgentId === agent.agent_id ? ' active' : ''}`}
            onClick={() => onSelectAgent(agent.agent_id)}
          >
            {agent.agent_name || agent.agent_id}
          </button>
        ))}
      </div>

      {visibleRows.length ? (
        <div className="cn-flow-map">
          {renderSource()}
          <div className="cn-flow-lanes">
            {flowMode === 'tag'
              ? tagAgentGroups.map((group) => (
                  <section key={group.key} className="cn-flow-agent-cluster">
                    {renderAgentNode(group.lead)}
                    <div className="cn-flow-arrow cluster-arrow">→</div>
                    <div className="cn-flow-agent-cluster-rows">
                      {group.entryGroups.map(renderEntryCluster)}
                    </div>
                  </section>
                ))
              : flowMode === 'agent'
                ? selectedEntryGroups.map(renderEntryCluster)
                : visibleRows.map((row) => (
                  <section key={row.key} className={`cn-flow-lane${row.loopDetected ? ' loop' : ''}`}>
                    {renderFlowTail(row)}
                  </section>
                  ))}
          </div>
        </div>
      ) : (
        <Empty description="暂无可展示的 CN 访问链路" />
      )}
    </Card>
  )
}

interface CNFlowHop {
  currentAgentID?: string
  currentAgentName?: string
  currentInboundLabel?: string
  currentDetail?: string
  outboundLabel: string
  outboundDetail: string
  outboundTargetIP?: string
  outboundTargetGeo?: IPGeoView
  routeScope?: string
  ruleIndex?: number
  targetAgentID?: string
  targetAgentName?: string
  targetInboundLabel?: string
  targetDetail?: string
  targetProtocol?: string
  targetClientLabel?: string
}

interface CNFlowRow {
  key: string
  rootAgentID: string
  rootAgentName?: string
  rootAgentTags?: string[]
  clientLabel: string
  clientDetail?: string
  entryLabel: string
  hops: CNFlowHop[]
  exitCountryCode: string
  exitCountryLabel: string
  exitReason: string
  loopDetected?: boolean
  unresolvedReason?: string
}

function shouldShowChainWarning(row: CNFlowRow): boolean {
  if (!row.unresolvedReason) {
    return false
  }
  const lastHop = row.hops[row.hops.length - 1]
  return !isExpectedTerminalExit(lastHop?.outboundLabel, lastHop?.outboundDetail, row.unresolvedReason)
}

function buildCNFlowRows(chains: ClientChainView[], agents: DashboardAgentView[]): CNFlowRow[] {
  const agentByID = new Map(agents.map((agent) => [agent.agent_id, agent]))
  const rows = chains.map((chain) => {
    const clientStep = chain.steps.find((step) => step.step_type === 'client')
    const entryStep = chain.steps.find((step) => step.step_type === 'inbound')
    const hops: CNFlowHop[] = []
    for (let index = 0; index < chain.steps.length; index += 1) {
      const step = chain.steps[index]
      if (step.step_type !== 'outbound') {
        continue
      }
      const nextMatch = chain.steps.slice(index + 1).find((item) => item.step_type === 'match')
      const currentInbound = [...chain.steps.slice(0, index)].reverse().find((item) => item.step_type === 'inbound' || item.step_type === 'match')
      hops.push({
        currentAgentID: currentInbound?.agent_id || step.agent_id || chain.root_agent_id,
        currentAgentName: currentInbound?.agent_name || step.agent_name || chain.root_agent_name,
        currentInboundLabel: currentInbound ? `${currentInbound.label}${currentInbound.port ? `:${currentInbound.port}` : ''}` : entryStep?.label || chain.root_inbound_tag,
        currentDetail: currentInbound?.detail,
        outboundLabel: step.label,
        outboundDetail: step.detail || step.target || '-',
        outboundTargetIP: step.target_ip,
        outboundTargetGeo: step.target_geo,
        routeScope: step.route_scope,
        ruleIndex: step.rule_index,
        targetAgentID: nextMatch?.agent_id,
        targetAgentName: nextMatch?.agent_name,
        targetInboundLabel: nextMatch ? formatStepNodeLabel(nextMatch) : undefined,
        targetDetail: nextMatch?.detail,
        targetProtocol: nextMatch?.protocol,
      })
    }
    const lastHop = hops[hops.length - 1]
    const lastOutbound = [...chain.steps].reverse().find((step) => step.step_type === 'outbound')
    const exitAgentID = lastHop?.targetAgentID || lastOutbound?.agent_id || chain.root_agent_id
    const exitAgent = exitAgentID ? agentByID.get(exitAgentID) : undefined
    const country = inferExitCountry(exitAgent, lastOutbound)
    return {
      key: chain.key,
      rootAgentID: chain.root_agent_id,
      rootAgentName: chain.root_agent_name,
      rootAgentTags: chain.root_agent_tags,
      clientLabel: clientStep?.label || chain.root_client_email || 'anonymous-client',
      clientDetail: clientStep?.detail || chain.root_client_remark,
      entryLabel: entryStep ? `${entryStep.label}${entryStep.port ? `:${entryStep.port}` : ''}` : chain.root_inbound_tag || '-',
      hops,
      exitCountryCode: country.code,
      exitCountryLabel: country.label,
      exitReason: buildExitReason(exitAgent, lastOutbound, chain.unresolved_reason),
      loopDetected: chain.loop_detected,
      unresolvedReason: isExpectedTerminalExit(lastOutbound?.label, lastOutbound?.target || lastOutbound?.detail, chain.unresolved_reason)
        ? undefined
        : chain.unresolved_reason,
    }
  })
  const clientsByEntry = new Map<string, string[]>()
  for (const row of rows) {
    const key = flowEntryKey(row.rootAgentID, row.entryLabel)
    clientsByEntry.set(key, [...(clientsByEntry.get(key) || []), row.clientLabel])
  }
  for (const row of rows) {
    for (const hop of row.hops) {
      if (!hop.targetAgentID || !hop.targetInboundLabel) {
        continue
      }
      const labels = clientsByEntry.get(flowEntryKey(hop.targetAgentID, hop.targetInboundLabel)) || []
      if (labels.length === 1) {
        hop.targetClientLabel = labels[0]
      } else if (labels.length > 1) {
        hop.targetClientLabel = `${labels.length} 个节点客户端`
      }
    }
  }
  return rows
}

function formatStepNodeLabel(step: ClientChainStep): string {
  return `${step.label}${step.port ? `:${step.port}` : ''}`
}

function flowEntryKey(agentID: string, label: string): string {
  return `${agentID}:${normalizeNodeAnchorLabel(label).toLowerCase()}`
}

function isExpectedTerminalExit(label?: string, target?: string, unresolvedReason?: string): boolean {
  if (!unresolvedReason?.includes('did not match')) {
    return false
  }
  const terminalText = `${label || ''} ${target || ''}`.toLowerCase()
  return ['direct', 'freedom', 'blocked', 'blackhole'].some((token) => terminalText.includes(token))
}

function inferExitCountry(agent?: DashboardAgentView, outbound?: ClientChainStep): { code: string; label: string } {
  if (outbound?.label === 'blocked') {
    return { code: 'BLOCK', label: 'Blocked' }
  }
  if (outbound?.target_geo?.country_code || outbound?.target_geo?.country_name) {
    return {
      code: outbound.target_geo.country_code || 'GEO',
      label: outbound.target_geo.country_name || outbound.target_geo.country_code || 'GeoIP',
    }
  }
  const haystack = [agent?.agent_name, agent?.agent_id, ...(agent?.tags || []), outbound?.target, outbound?.detail, outbound?.label]
    .filter(Boolean)
    .join(' ')
    .toLowerCase()
  const countryMap: Array<[string, string[], string]> = [
    ['CN', [' cn', 'china', 'mainland', '中国'], 'China'],
    ['HK', ['hk', 'hong kong', '香港'], 'Hong Kong'],
    ['SG', ['sg', 'singapore', '新加坡'], 'Singapore'],
    ['US', ['us', 'usa', 'united states', 'america', 'cox', '美国'], 'United States'],
    ['JP', ['jp', 'japan', '日本'], 'Japan'],
    ['KR', ['kr', 'korea', '韩国'], 'Korea'],
    ['DE', ['de', 'germany', '德国'], 'Germany'],
    ['GB', ['uk', 'gb', 'britain', '英国'], 'United Kingdom'],
  ]
  for (const [code, tokens, label] of countryMap) {
    if (tokens.some((token) => haystack.includes(token.trim()))) {
      return { code, label }
    }
  }
  return { code: 'UNK', label: agent?.agent_name || outbound?.target || 'Unknown Exit' }
}

function buildExitReason(agent?: DashboardAgentView, outbound?: ClientChainStep, unresolvedReason?: string): string {
  if (outbound?.label === 'blocked') {
    return '流量被 blackhole 阻断'
  }
  if (outbound?.label === 'direct') {
    return `${agent?.agent_name || outbound.agent_name || '当前 VPS'} direct 出站`
  }
  if (outbound?.target_geo?.country_name) {
    const location = [outbound.target_geo.country_name, outbound.target_geo.region_name, outbound.target_geo.city].filter(Boolean).join(' / ')
    return `${outbound.target || outbound.detail || outbound.label} · ${outbound.target_geo.ip || outbound.target_ip || 'resolved IP'} · ${location}`
  }
  if (unresolvedReason?.includes('did not match')) {
    return outbound?.target ? `未匹配到下一跳，按 ${outbound.target} 作为出口` : '未匹配到下一跳，按当前 VPS 出口'
  }
  return outbound?.target || agent?.summary.public_ipv4 || '出口由最后一跳决定'
}

function uniqueCountries(rows: CNFlowRow[]): string[] {
  return Array.from(new Set(rows.map((row) => row.exitCountryCode)))
}

function translateChainReason(reason: string): string {
  if (reason.includes('did not match')) {
    return '最后出站未匹配到下游入站'
  }
  if (reason.includes('loop')) {
    return '检测到循环链路'
  }
  if (reason.includes('missing')) {
    return '配置引用缺失'
  }
  return reason
}

function renderInboundActionForm(props: {
  form: XUIInboundActionForm
  certificates: XUILocalCertificate[]
  onChange: (form: XUIInboundActionForm) => void
}) {
  const { form, certificates, onChange } = props
  const update = (patch: Partial<XUIInboundActionForm>) => onChange({ ...form, ...patch })
  const updateTLS = (patch: Partial<TLSCertificateSelectionForm>) => update({ tls: { ...form.tls, ...patch } })
  const updateClient = (index: number, patch: Partial<XUIInboundClientForm>) =>
    update({
      clients: form.clients.map((client, currentIndex) => (currentIndex === index ? { ...client, ...patch } : client)),
    })

  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Card className="config-section-card" bordered={false}>
        <Title level={4}>入站基础配置</Title>
        <Row gutter={[16, 16]}>
          <Col xs={24} md={12}>
            <Text type="secondary">备注</Text>
            <Input value={form.remark} onChange={(event) => update({ remark: event.target.value })} />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">标签</Text>
            <Input
              placeholder="留空会自动生成"
              value={form.tag}
              onChange={(event) => update({ tag: event.target.value })}
            />
          </Col>
          <Col xs={24} md={8}>
            <Text type="secondary">协议</Text>
            <Select
              style={{ width: '100%' }}
              value={form.protocol}
              options={[
                { value: 'vless', label: 'VLESS' },
                { value: 'vmess', label: 'VMESS' },
                { value: 'trojan', label: 'Trojan' },
              ]}
              onChange={(value) => update({ protocol: value })}
            />
          </Col>
          <Col xs={24} md={8}>
            <Text type="secondary">监听端口</Text>
            <InputNumber style={{ width: '100%' }} min={1} max={65535} value={form.port} onChange={(value) => update({ port: Number(value || 0) })} />
          </Col>
          <Col xs={24} md={8}>
            <Text type="secondary">监听地址</Text>
            <Input value={form.listen} placeholder="默认空" onChange={(event) => update({ listen: event.target.value })} />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">传输层</Text>
            <Select
              style={{ width: '100%' }}
              value={form.transport}
              options={[
                { value: 'tcp', label: 'TCP' },
                { value: 'ws', label: 'WebSocket' },
              ]}
              onChange={(value) => update({ transport: value })}
            />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">安全</Text>
            <Select
              style={{ width: '100%' }}
              value={form.security}
              options={[
                { value: 'none', label: 'None' },
                { value: 'tls', label: 'TLS' },
              ]}
              onChange={(value) => update({ security: value })}
            />
          </Col>
          {form.transport === 'ws' ? (
            <>
              <Col xs={24} md={12}>
                <Text type="secondary">WS Path</Text>
                <Input value={form.ws_path} onChange={(event) => update({ ws_path: event.target.value })} />
              </Col>
              <Col xs={24} md={12}>
                <Text type="secondary">WS Host</Text>
                <Input value={form.ws_host} onChange={(event) => update({ ws_host: event.target.value })} />
              </Col>
            </>
          ) : null}
          <Col xs={24} md={12}>
            <div className="switch-row">
              <span>启用入站</span>
              <Switch checked={form.enabled} onChange={(checked) => update({ enabled: checked })} />
            </div>
          </Col>
          <Col xs={24} md={12}>
            <div className="switch-row">
              <span>提交后重启 Xray</span>
              <Tag color="success">自动执行</Tag>
            </div>
          </Col>
          <Col xs={24}>
            <div className="switch-row">
              <span>启用 Sniffing</span>
              <Switch checked={form.sniffing} onChange={(checked) => update({ sniffing: checked })} />
            </div>
          </Col>
        </Row>
      </Card>

      {form.security === 'tls' ? (
        <Card className="config-section-card" bordered={false}>
          <Title level={4}>TLS / SSL 证书</Title>
          <Row gutter={[16, 16]}>
            <Col xs={24} md={12}>
              <Text type="secondary">Server Name</Text>
              <Input
                value={form.server_name}
                placeholder="例如 hk.example.test"
                onChange={(event) => update({ server_name: event.target.value })}
              />
            </Col>
            <Col xs={24} md={12}>
              <Text type="secondary">证书来源</Text>
              <Select
                style={{ width: '100%' }}
                value={form.tls.mode}
                options={[
                  { value: 'none', label: '暂不注入证书' },
                  { value: 'domain_auto', label: '按域名自动匹配 client 本机证书' },
                  { value: 'inventory', label: '从 client 已发现证书中指定' },
                  { value: 'manual', label: '手动填写证书路径' },
                ]}
                onChange={(value: TLSCertificateSelectionForm['mode']) => updateTLS({ mode: value })}
              />
            </Col>
            {form.tls.mode === 'domain_auto' ? (
              <Col xs={24}>
                <Text type="secondary">自动匹配域名</Text>
                <Input
                  value={form.tls.domain}
                  placeholder="留空时使用上面的 Server Name"
                  onChange={(event) => updateTLS({ domain: event.target.value })}
                />
              </Col>
            ) : null}
            {form.tls.mode === 'inventory' ? (
              <Col xs={24}>
                <Text type="secondary">选择本机证书</Text>
                <Select
                  style={{ width: '100%' }}
                  value={form.tls.inventory_id || undefined}
                  placeholder={certificates.length ? '选择 client 已上报的证书' : '当前 client 暂无已上报证书'}
                  options={certificates.map((certificate) => ({
                    value: certificate.id,
                    label: `${certificate.name || certificate.subject || certificate.id} · ${certificate.cert_path || '-'}`,
                  }))}
                  onChange={(value) => updateTLS({ inventory_id: value })}
                />
              </Col>
            ) : null}
            {form.tls.mode === 'manual' ? (
              <>
                <Col xs={24}>
                  <Text type="secondary">证书文件路径</Text>
                  <Input
                    value={form.tls.certificate_file}
                    placeholder="/etc/letsencrypt/live/example/fullchain.pem"
                    onChange={(event) => updateTLS({ certificate_file: event.target.value })}
                  />
                </Col>
                <Col xs={24}>
                  <Text type="secondary">私钥文件路径</Text>
                  <Input
                    value={form.tls.key_file}
                    placeholder="/etc/letsencrypt/live/example/privkey.pem"
                    onChange={(event) => updateTLS({ key_file: event.target.value })}
                  />
                </Col>
              </>
            ) : null}
          </Row>
        </Card>
      ) : null}

      <Card className="config-section-card" bordered={false}>
        <Space style={{ width: '100%', justifyContent: 'space-between' }}>
          <Title level={4}>客户端账号</Title>
          <Button
            icon={<PlusOutlined />}
            onClick={() => update({ clients: [...form.clients, defaultInboundClientForm()] })}
          >
            新增客户端
          </Button>
        </Space>
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
          {form.clients.map((client, index) => (
            <Card key={`client-${index}`} className="config-section-card" bordered={false}>
              <Space style={{ width: '100%', justifyContent: 'space-between' }}>
                <Text strong>客户端 #{index + 1}</Text>
                <Button
                  danger
                  icon={<DeleteOutlined />}
                  disabled={form.clients.length === 1}
                  onClick={() => update({ clients: form.clients.filter((_, currentIndex) => currentIndex !== index) })}
                >
                  删除
                </Button>
              </Space>
              <Row gutter={[16, 16]}>
                <Col xs={24} md={12}>
                  <Text type="secondary">邮箱 / 标识</Text>
                  <Input value={client.email} onChange={(event) => updateClient(index, { email: event.target.value })} />
                </Col>
                <Col xs={24} md={12}>
                  <Text type="secondary">备注</Text>
                  <Input value={client.comment} onChange={(event) => updateClient(index, { comment: event.target.value })} />
                </Col>
                {form.protocol === 'trojan' ? (
                  <Col xs={24} md={12}>
                    <Text type="secondary">密码</Text>
                    <Input value={client.password} onChange={(event) => updateClient(index, { password: event.target.value })} />
                  </Col>
                ) : (
                  <Col xs={24} md={12}>
                    <Text type="secondary">UUID</Text>
                    <Input value={client.uuid} onChange={(event) => updateClient(index, { uuid: event.target.value })} />
                  </Col>
                )}
                <Col xs={24} md={12}>
                  <Text type="secondary">Sub ID</Text>
                  <Input value={client.sub_id} onChange={(event) => updateClient(index, { sub_id: event.target.value })} />
                </Col>
                <Col xs={24} md={8}>
                  <Text type="secondary">限 IP</Text>
                  <InputNumber style={{ width: '100%' }} min={0} value={client.limit_ip} onChange={(value) => updateClient(index, { limit_ip: Number(value || 0) })} />
                </Col>
                <Col xs={24} md={8}>
                  <Text type="secondary">总流量 GB</Text>
                  <InputNumber style={{ width: '100%' }} min={0} value={client.total_gb} onChange={(value) => updateClient(index, { total_gb: Number(value || 0) })} />
                </Col>
                <Col xs={24} md={8}>
                  <Text type="secondary">到期天数</Text>
                  <InputNumber style={{ width: '100%' }} min={0} value={client.expiry_days} onChange={(value) => updateClient(index, { expiry_days: Number(value || 0) })} />
                </Col>
                {form.protocol !== 'trojan' ? (
                  <Col xs={24} md={12}>
                    <Text type="secondary">Flow</Text>
                    <Input value={client.flow} onChange={(event) => updateClient(index, { flow: event.target.value })} />
                  </Col>
                ) : null}
                <Col xs={24} md={12}>
                  <div className="switch-row">
                    <span>启用此客户端</span>
                    <Switch checked={client.enabled} onChange={(checked) => updateClient(index, { enabled: checked })} />
                  </div>
                </Col>
              </Row>
            </Card>
          ))}
        </Space>
      </Card>
    </Space>
  )
}

function renderOutboundActionForm(props: {
  form: XUIOutboundActionForm
  agents: AgentListItem[]
  targetAgentID: string
  currentOverview: XUIOverview | null
  sourceOverview: XUIOverview | null
  sourceLoading: boolean
  onChange: (form: XUIOutboundActionForm) => void
}) {
  const { form, agents, targetAgentID, currentOverview, sourceOverview, sourceLoading, onChange } = props
  const update = (patch: Partial<XUIOutboundActionForm>) => onChange({ ...form, ...patch })
  const activeSourceOverview = form.source_agent_id && currentOverview?.agent_id === form.source_agent_id ? currentOverview : sourceOverview
  const sourceClientOptions = (activeSourceOverview?.clients || []).map((client) => ({
    key: sourceClientKey(client),
    client,
  }))

  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Card className="config-section-card" bordered={false}>
        <Title level={4}>从内部 Client 节点导入</Title>
        <Alert
          type="info"
          showIcon
          className="compact-alert"
          message="只允许内部导入"
          description="选择一个已有 Client 节点客户端后，系统会自动生成当前 Client 的出站配置；不再开放手动填写协议、地址和端口。"
        />
        <Row gutter={[16, 16]}>
          <Col xs={24} md={12}>
            <Text type="secondary">源 Client</Text>
            <Select
              allowClear
              style={{ width: '100%' }}
              value={form.source_agent_id || undefined}
              options={agents
                .filter((agent) => agent.agent_id !== targetAgentID)
                .map((agent) => ({ value: agent.agent_id, label: agent.agent_name || agent.agent_id }))}
              onChange={(value) =>
                update({
                  source_agent_id: value || '',
                  source_client_key: '',
                })}
            />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">源节点客户端</Text>
            <Select
              allowClear
              style={{ width: '100%' }}
              loading={sourceLoading}
              disabled={!form.source_agent_id}
              value={form.source_client_key || undefined}
              options={sourceClientOptions.map(({ key, client }) => ({
                value: key,
                label: `${client.email || '-'} · ${client.inbound_remark || client.inbound_tag || client.protocol || '-'}`,
              }))}
              onChange={(value) => {
                const nextKey = value || ''
                const patch: Partial<XUIOutboundActionForm> = { source_client_key: nextKey }
                const nextClient = sourceClientOptions.find((item) => item.key === nextKey)?.client
                const nextNode = activeSourceOverview?.nodes.find(
                  (node) => node.id === nextClient?.inbound_id || node.tag === nextClient?.inbound_tag,
                )
                if (activeSourceOverview && nextClient && nextNode) {
                  Object.assign(patch, buildOutboundImportPatch(activeSourceOverview, nextNode, nextClient, form))
                }
                update(patch)
              }}
            />
          </Col>
          <Col xs={24}>
            <div className="switch-row">
              <span>提交后重启 Xray</span>
              <Tag color="success">自动执行</Tag>
            </div>
          </Col>
        </Row>
      </Card>
    </Space>
  )
}

function sourceClientKey(client: XUIClientView): string {
  return [client.inbound_id || 0, client.inbound_tag || '', client.email || ''].join('::')
}

function buildOutboundImportPatch(
  sourceOverview: XUIOverview,
  sourceNode: XUINodeView,
  sourceClient: XUIClientView,
  currentForm: XUIOutboundActionForm,
): Partial<XUIOutboundActionForm> {
  const address = sourceOverview.summary.public_ipv4 || sourceOverview.summary.public_ipv6 || ''
  const protocol = (sourceNode.protocol || sourceClient.protocol || currentForm.protocol || 'freedom').toLowerCase()
  const tagParts = [
    sourceOverview.agent_name || sourceOverview.agent_id,
    sourceNode.tag || sourceNode.remark || String(sourceNode.id),
    sourceClient.email || 'link',
  ]

  return {
    tag: normalizeOutboundTag(tagParts.join('-')),
    protocol,
    address,
    port: sourceNode.port || currentForm.port,
    uuid: protocol === 'socks' ? sourceClient.email || '' : sourceClient.auth_uuid || currentForm.uuid,
    password: sourceClient.auth_password || currentForm.password,
    flow: sourceClient.flow || '',
    security: sourceNode.security || 'none',
    server_name: sourceNode.tls_server_name || sourceNode.ws_host || currentForm.server_name,
    network: sourceNode.network || 'tcp',
    ws_path: sourceNode.ws_path || '/',
    ws_host: sourceNode.ws_host || '',
  }
}

function normalizeOutboundTag(value: string): string {
  const normalized = value
    .toLowerCase()
    .replace(/[^a-z0-9._-]+/g, '-')
    .replace(/-+/g, '-')
    .replace(/^-|-$/g, '')
  return normalized || 'relay-link'
}

function renderRoutingActionForm(props: {
  form: XUIRoutingActionForm
  inbounds: XUINodeView[]
  clients: XUIClientView[]
  outbounds: { tag?: string }[]
  onChange: (form: XUIRoutingActionForm) => void
}) {
  const { form, inbounds, clients, outbounds, onChange } = props
  const update = (patch: Partial<XUIRoutingActionForm>) => onChange({ ...form, ...patch })

  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Card className="config-section-card" bordered={false}>
        <Title level={4}>目标出口</Title>
        <Row gutter={[16, 16]}>
          <Col xs={24} md={12}>
            <Text type="secondary">Outbound Tag</Text>
            <Select
              allowClear
              style={{ width: '100%' }}
              value={form.outbound_tag || undefined}
              options={outbounds.filter((item) => item.tag).map((item) => ({ value: item.tag as string, label: item.tag as string }))}
              onChange={(value) => update({ outbound_tag: value || '' })}
            />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">Balancer Tag</Text>
            <Input value={form.balancer_tag} onChange={(event) => update({ balancer_tag: event.target.value })} />
          </Col>
          <Col xs={24}>
            <div className="switch-row">
              <span>提交后重启 Xray</span>
              <Tag color="success">自动执行</Tag>
            </div>
          </Col>
        </Row>
      </Card>

      <Card className="config-section-card" bordered={false}>
        <Title level={4}>匹配条件</Title>
        <Row gutter={[16, 16]}>
          <Col xs={24}>
            <Text type="secondary">匹配入站</Text>
            <Select
              mode="multiple"
              style={{ width: '100%' }}
              value={form.inbound_tags}
              options={inbounds.map((inbound) => ({
                value: inbound.tag || String(inbound.id),
                label: `${inbound.remark || inbound.tag || inbound.id} · ${inbound.tag || '-'}`,
              }))}
              onChange={(value) => update({ inbound_tags: value })}
            />
          </Col>
          <Col xs={24}>
            <Text type="secondary">匹配用户</Text>
            <Select
              mode="multiple"
              style={{ width: '100%' }}
              value={form.users}
              options={clients.filter((client) => client.email).map((client) => ({
                value: client.email as string,
                label: `${client.email as string} · ${client.inbound_remark || client.inbound_tag || '-'}`,
              }))}
              onChange={(value) => update({ users: value })}
            />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">域名</Text>
            <Input.TextArea value={form.domains} autoSize={{ minRows: 3, maxRows: 6 }} onChange={(event) => update({ domains: event.target.value })} />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">IP / CIDR</Text>
            <Input.TextArea value={form.ips} autoSize={{ minRows: 3, maxRows: 6 }} onChange={(event) => update({ ips: event.target.value })} />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">端口</Text>
            <Input value={form.ports} placeholder="443, 8443" onChange={(event) => update({ ports: event.target.value })} />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">源端口</Text>
            <Input value={form.source_ports} placeholder="10000-20000" onChange={(event) => update({ source_ports: event.target.value })} />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">源 IP</Text>
            <Input.TextArea value={form.source_ips} autoSize={{ minRows: 3, maxRows: 6 }} onChange={(event) => update({ source_ips: event.target.value })} />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">网络协议</Text>
            <Select
              mode="multiple"
              style={{ width: '100%' }}
              value={form.networks}
              options={[
                { value: 'tcp', label: 'tcp' },
                { value: 'udp', label: 'udp' },
              ]}
              onChange={(value) => update({ networks: value })}
            />
          </Col>
          <Col xs={24}>
            <Text type="secondary">协议类型</Text>
            <Select
              mode="multiple"
              style={{ width: '100%' }}
              value={form.protocols}
              options={[
                { value: 'bittorrent', label: 'bittorrent' },
                { value: 'http', label: 'http' },
                { value: 'tls', label: 'tls' },
              ]}
              onChange={(value) => update({ protocols: value })}
            />
          </Col>
        </Row>
      </Card>
    </Space>
  )
}

interface RouteBadgeProps {
  route: XUIRouteTrace
  onJumpOutbound: (tag?: string) => void
  onJumpRule: (index?: number) => void
}

function RouteBadge({ route, onJumpOutbound, onJumpRule }: RouteBadgeProps) {
  const targetLabel = route.outbound_tag || (route.balancer_tag ? `balancer:${route.balancer_tag}` : '未识别')

  return (
    <Space wrap size={[6, 6]}>
      {route.outbound_tag ? (
        <Button type="link" className="route-link" onClick={() => onJumpOutbound(route.outbound_tag)}>
          {targetLabel}
        </Button>
      ) : route.rule_index ? (
        <Button type="link" className="route-link" onClick={() => onJumpRule(route.rule_index)}>
          {targetLabel}
        </Button>
      ) : (
        <Tag>{targetLabel}</Tag>
      )}
      <Tag color={scopeColor(route.match_scope)}>{scopeLabel(route.match_scope)}</Tag>
      {route.rule_index ? (
        <Button type="link" className="route-rule-link" onClick={() => onJumpRule(route.rule_index)}>
          R{route.rule_index}
        </Button>
      ) : null}
      {route.has_global_rules && route.global_rule_indexes?.length ? (
        <Tag>+{route.global_rule_indexes.length} 全局规则</Tag>
      ) : null}
    </Space>
  )
}

class APIError extends Error {
  status: number

  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

async function fetchJSON<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, { ...init, credentials: 'same-origin' })
  if (!response.ok) {
    let detail = response.statusText
    try {
      const payload = (await response.json()) as { error?: string }
      if (payload.error) {
        detail = payload.error
      }
    } catch {
      // ignore invalid json
    }
    throw new APIError(response.status, detail || `request failed: ${response.status}`)
  }
  return (await response.json()) as T
}

function buildDashboardRealtimeURL(): string {
  const url = new URL('/api/v1/dashboard/realtime', window.location.href)
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  return url.toString()
}

function mergeRealtimeMetricsIntoAgents<T extends AgentListItem>(agents: T[], metrics: AgentRealtimeMetrics[]): T[] {
  if (!metrics.length) {
    return agents
  }
  const byAgent = new Map(metrics.map((metric) => [metric.agent_id, metric]))
  let changed = false
  const next = agents.map((agent) => {
    const metric = byAgent.get(agent.agent_id)
    if (!metric) {
      return agent
    }
    changed = true
    return {
      ...agent,
      agent_name: agent.agent_name || metric.agent_name,
      realtime_at: metric.reported_at || agent.realtime_at,
      summary: mergeRealtimeSummary(agent.summary, metric.summary),
    }
  })
  return changed ? next : agents
}

function sortAgentsByOrder<T extends AgentListItem>(agents: T[]): T[] {
  return [...agents].sort((left, right) => {
    const leftOrder = Number(left.sort_order || 0)
    const rightOrder = Number(right.sort_order || 0)
    if (leftOrder > 0 || rightOrder > 0) {
      if (leftOrder <= 0) return 1
      if (rightOrder <= 0) return -1
      if (leftOrder !== rightOrder) return leftOrder - rightOrder
    }
    const leftRegistered = Date.parse(left.registered_at || '')
    const rightRegistered = Date.parse(right.registered_at || '')
    if (!Number.isNaN(leftRegistered) || !Number.isNaN(rightRegistered)) {
      if (Number.isNaN(leftRegistered)) return 1
      if (Number.isNaN(rightRegistered)) return -1
      if (leftRegistered !== rightRegistered) return leftRegistered - rightRegistered
    }
    return left.agent_id.localeCompare(right.agent_id)
  })
}

function mergeRealtimeSummary(current: VPSSummary, realtime: VPSSummary): VPSSummary {
  return {
    ...current,
    hostname: realtime.hostname || current.hostname,
    observed_ip: realtime.observed_ip || current.observed_ip,
    public_ipv4: realtime.public_ipv4 || current.public_ipv4,
    public_ipv6: realtime.public_ipv6 || current.public_ipv6,
    cpu: realtime.cpu ?? current.cpu,
    mem_used: realtime.mem_total ? realtime.mem_used : current.mem_used,
    mem_total: realtime.mem_total || current.mem_total,
    net_traffic_sent: realtime.net_traffic_sent ?? current.net_traffic_sent,
    net_traffic_recv: realtime.net_traffic_recv ?? current.net_traffic_recv,
    net_traffic_total: realtime.net_traffic_total ?? current.net_traffic_total,
    net_io_up: realtime.net_io_up ?? current.net_io_up,
    net_io_down: realtime.net_io_down ?? current.net_io_down,
    xray_state: realtime.xray_state || current.xray_state,
  }
}

function isUnauthorized(error: unknown): boolean {
  return error instanceof APIError && error.status === 401
}

function defaultTLSCertificateSelection(): TLSCertificateSelectionForm {
  return {
    mode: 'none',
    inventory_id: '',
    domain: '',
    certificate_file: '',
    key_file: '',
  }
}

function defaultInboundClientForm(): XUIInboundClientForm {
  return {
    email: 'user@example.com',
    uuid: '',
    password: '',
    flow: '',
    limit_ip: 0,
    total_gb: 0,
    expiry_days: 0,
    comment: '',
    sub_id: '',
    enabled: true,
  }
}

function defaultInboundActionForm(): XUIInboundActionForm {
  return {
    remark: 'vless-auto-443',
    tag: '',
    enabled: true,
    listen: '',
    port: 443,
    protocol: 'vless',
    transport: 'tcp',
    security: 'none',
    server_name: '',
    ws_path: '/',
    ws_host: '',
    sniffing: true,
    tls: defaultTLSCertificateSelection(),
    clients: [defaultInboundClientForm()],
    restart: true,
  }
}

function defaultOutboundActionForm(): XUIOutboundActionForm {
  return {
    tag: 'relay-hk',
    protocol: 'freedom',
    send_through: '',
    address: '',
    port: 443,
    uuid: '',
    flow: '',
    password: '',
    method: 'aes-256-gcm',
    security: 'none',
    server_name: '',
    network: 'tcp',
    ws_path: '/',
    ws_host: '',
    source_agent_id: '',
    source_client_key: '',
    restart: true,
  }
}

function defaultRoutingActionForm(): XUIRoutingActionForm {
  return {
    outbound_tag: '',
    balancer_tag: '',
    inbound_tags: [],
    users: [],
    domains: '',
    ips: '',
    ports: '',
    source_ips: '',
    source_ports: '',
    networks: [],
    protocols: [],
    restart: true,
  }
}

function defaultTelegramBotForm(): TelegramBotForm {
  return {
    name: '',
    bot_token: '',
    chat_id: '',
    enabled: true,
  }
}

function defaultClientInstallCommandForm(): ClientInstallCommandForm {
  return {
    server_url: typeof window !== 'undefined' ? window.location.origin : 'http://SERVER_IP:8090',
    registration_token: '',
    install_script_url: 'https://raw.githubusercontent.com/zanelin1015/VPSMonitor/main/install.sh',
    poll_interval: '30s',
    request_timeout_seconds: 15,
    server_skip_tls_verify: false,
  }
}

function normalizeClientInstallCommandForm(info: ClientInstallInfo): ClientInstallCommandForm {
  return {
    server_url: info.server_url || defaultClientInstallCommandForm().server_url,
    registration_token: info.registration_token || '',
    install_script_url: info.install_script_url || defaultClientInstallCommandForm().install_script_url,
    poll_interval: info.poll_interval || '30s',
    request_timeout_seconds: Number(info.request_timeout_seconds || 15),
    server_skip_tls_verify: Boolean(info.server_skip_tls_verify),
  }
}

function buildClientInstallCommand(form: ClientInstallCommandForm): string {
  const scriptURL = form.install_script_url.trim() || defaultClientInstallCommandForm().install_script_url
  const envValues: Array<[string, string]> = [
    ['VPSMONITOR_SERVER_URL', form.server_url.trim()],
    ['VPSMONITOR_REGISTRATION_TOKEN', form.registration_token.trim()],
    ['VPSMONITOR_SERVER_SKIP_TLS_VERIFY', String(Boolean(form.server_skip_tls_verify))],
    ['VPSMONITOR_POLL_INTERVAL', form.poll_interval.trim() || '30s'],
    ['VPSMONITOR_REQUEST_TIMEOUT_SECONDS', String(Math.max(1, Number(form.request_timeout_seconds || 15)))],
    ['VPSMONITOR_ASSUME_YES', 'true'],
  ]
  const envText = envValues.map(([key, value]) => `${key}=${shellQuote(value)}`).join(' ')
  return `curl -L ${shellQuote(scriptURL)} -o vpsmonitor-install.sh && chmod +x vpsmonitor-install.sh && env ${envText} ./vpsmonitor-install.sh client`
}

function shellQuote(value: string): string {
  return `'${value.replace(/'/g, `'\\''`)}'`
}

function buildXUIActionPayload(
  kind: string,
  forms: {
    outbound: XUIOutboundActionForm
    routing: XUIRoutingActionForm
  },
): Record<string, unknown> {
  switch (kind) {
    case 'add_routing_rule':
      return buildRoutingActionPayload(forms.routing)
    case 'add_outbound':
    default:
      return buildOutboundActionPayload(forms.outbound)
  }
}

function buildInboundActionPayload(form: XUIInboundActionForm): Record<string, unknown> {
  if (!form.port) {
    throw new Error('入站端口不能为空')
  }
  if (!form.protocol) {
    throw new Error('入站协议不能为空')
  }

  const protocol = form.protocol.toLowerCase()
  const tag = form.tag.trim() || `in-${protocol}-${form.port}`
  const clients = form.clients.map((client, index) => buildInboundClientPayload(client, protocol, index))
  const settings: Record<string, unknown> = {
    clients,
    fallbacks: [],
  }
  if (protocol === 'vless') {
    settings.decryption = 'none'
  }

  const streamSettings: Record<string, unknown> = {
    network: form.transport,
    security: form.security,
    tcpSettings: { acceptProxyProtocol: false, header: { type: 'none' } },
  }
  if (form.transport === 'ws') {
    streamSettings.wsSettings = {
      path: form.ws_path || '/',
      headers: form.ws_host ? { Host: form.ws_host } : {},
    }
  }
  if (form.security === 'tls') {
    streamSettings.tlsSettings = {
      serverName: form.server_name.trim(),
      alpn: ['h2', 'http/1.1'],
      certificates: [],
    }
  }

  const payload: Record<string, unknown> = {
    inbound: {
      remark: form.remark.trim(),
      tag,
      enable: form.enabled,
      listen: form.listen.trim(),
      port: form.port,
      protocol,
      total: 0,
      expiryTime: 0,
      settings: JSON.stringify(settings),
      streamSettings: JSON.stringify(streamSettings),
      sniffing: JSON.stringify({
        enabled: form.sniffing,
        destOverride: ['http', 'tls', 'quic', 'fakedns'],
        metadataOnly: false,
        routeOnly: false,
      }),
    },
    restart: true,
  }

  if (form.security === 'tls' && form.tls.mode !== 'none') {
    payload.tls_certificate = {
      mode: form.tls.mode,
      inventory_id: form.tls.inventory_id.trim(),
      domain: (form.tls.domain || form.server_name).trim(),
      certificate_file: form.tls.certificate_file.trim(),
      key_file: form.tls.key_file.trim(),
    }
  }

  return payload
}

function buildInboundClientPayload(client: XUIInboundClientForm, protocol: string, index: number): Record<string, unknown> {
  const email = client.email.trim() || `user-${index + 1}@local`
  const payload: Record<string, unknown> = {
    email,
    enable: client.enabled,
    flow: client.flow.trim(),
    limitIp: Math.max(0, client.limit_ip || 0),
    totalGB: Math.max(0, client.total_gb || 0) * 1024 * 1024 * 1024,
    expiryTime: client.expiry_days > 0 ? Date.now() + client.expiry_days * 24 * 60 * 60 * 1000 : 0,
    subId: client.sub_id.trim(),
    comment: client.comment.trim(),
  }

  if (protocol === 'trojan') {
    payload.password = client.password.trim() || `trojan-${index + 1}`
  } else {
    payload.id = client.uuid.trim() || `00000000-0000-0000-0000-${String(index + 1).padStart(12, '0')}`
  }
  return payload
}

function buildOutboundActionPayload(form: XUIOutboundActionForm): Record<string, unknown> {
  if (!form.source_agent_id || !form.source_client_key) {
    throw new Error('请选择源 Client 和源节点客户端')
  }
  const protocol = form.protocol.toLowerCase()
  const tag = form.tag.trim()
  if (!tag) {
    throw new Error('未能从源节点生成出站标签')
  }

  const outbound: Record<string, unknown> = {
    tag,
    protocol,
  }
  if (form.send_through.trim()) {
    outbound.sendThrough = form.send_through.trim()
  }

  switch (protocol) {
    case 'freedom':
    case 'blackhole':
      outbound.settings = {}
      break
    case 'vless':
    case 'vmess':
      if (!form.address.trim() || !form.port) {
        throw new Error(`${protocol.toUpperCase()} 出站需要远端地址和端口`)
      }
      outbound.settings = {
        vnext: [
          {
            address: form.address.trim(),
            port: form.port,
            users: [
              buildVNextUser(form, protocol),
            ],
          },
        ],
      }
      outbound.streamSettings = buildOutboundStreamSettings(form)
      break
    case 'trojan':
      if (!form.address.trim() || !form.port) {
        throw new Error('Trojan 出站需要远端地址和端口')
      }
      outbound.settings = {
        servers: [
          {
            address: form.address.trim(),
            port: form.port,
            password: form.password.trim() || 'change-me',
          },
        ],
      }
      outbound.streamSettings = buildOutboundStreamSettings(form)
      break
    case 'socks':
      if (!form.address.trim() || !form.port) {
        throw new Error('SOCKS 出站需要远端地址和端口')
      }
      outbound.settings = {
        servers: [
          {
            address: form.address.trim(),
            port: form.port,
            users: form.password.trim()
              ? [{ user: form.uuid.trim() || 'user', pass: form.password.trim() }]
              : [],
          },
        ],
      }
      break
    default:
      throw new Error(`暂不支持该出站协议: ${form.protocol}`)
  }

  return {
    outbound,
    restart: true,
  }
}

function buildVNextUser(form: XUIOutboundActionForm, protocol: string): Record<string, unknown> {
  const user: Record<string, unknown> = {
    id: form.uuid.trim() || '00000000-0000-0000-0000-000000000000',
  }
  if (protocol === 'vless') {
    user.encryption = 'none'
    if (form.flow.trim()) {
      user.flow = form.flow.trim()
    }
    return user
  }
  user.security = 'auto'
  return user
}

function buildOutboundStreamSettings(form: XUIOutboundActionForm): Record<string, unknown> {
  const streamSettings: Record<string, unknown> = {
    network: form.network,
    security: form.security,
  }
  if (form.network === 'ws') {
    streamSettings.wsSettings = {
      path: form.ws_path || '/',
      headers: form.ws_host ? { Host: form.ws_host } : {},
    }
  }
  if (form.security === 'tls') {
    streamSettings.tlsSettings = {
      serverName: form.server_name.trim(),
      allowInsecure: false,
    }
  }
  return streamSettings
}

function buildRoutingActionPayload(form: XUIRoutingActionForm): Record<string, unknown> {
  if (!form.outbound_tag && !form.balancer_tag) {
    throw new Error('路由规则需要选择出站或 balancer')
  }
  const rule: Record<string, unknown> = {
    type: 'field',
  }
  if (form.inbound_tags.length) {
    rule.inboundTag = form.inbound_tags
  }
  if (form.users.length) {
    rule.user = form.users
  }
  if (form.outbound_tag) {
    rule.outboundTag = form.outbound_tag
  }
  if (form.balancer_tag) {
    rule.balancerTag = form.balancer_tag
  }
  setRoutingList(rule, 'domain', form.domains)
  setRoutingList(rule, 'ip', form.ips)
  setRoutingList(rule, 'port', form.ports)
  setRoutingList(rule, 'sourceIP', form.source_ips)
  setRoutingList(rule, 'sourcePort', form.source_ports)
  if (form.networks.length) {
    rule.network = form.networks
  }
  if (form.protocols.length) {
    rule.protocol = form.protocols
  }

  return {
    rule,
    restart: true,
  }
}

function setRoutingList(target: Record<string, unknown>, key: string, rawText: string) {
  const values = splitTextList(rawText)
  if (values.length) {
    target[key] = values
  }
}

function splitTextList(rawText: string): string[] {
  return rawText
    .split(/[\n,]/)
    .map((item) => item.trim())
    .filter(Boolean)
}

function actionKindLabel(kind: string): string {
  return XUI_ACTION_KINDS.find((item) => item.value === kind)?.label || kind
}

function actionStatusLabel(status: string): string {
  switch (status) {
    case 'pending':
      return '等待 client 执行'
    case 'running':
      return '执行中'
    case 'succeeded':
      return '成功'
    case 'failed':
      return '失败'
    default:
      return status || '-'
  }
}

function actionStatusColor(status: string): string {
  switch (status) {
    case 'pending':
      return 'gold'
    case 'running':
      return 'processing'
    case 'succeeded':
      return 'success'
    case 'failed':
      return 'error'
    default:
      return 'default'
  }
}

function shortJSON(value: unknown): string {
  if (!value || (typeof value === 'object' && Object.keys(value as Record<string, unknown>).length === 0)) {
    return ''
  }
  const text = JSON.stringify(value)
  return text.length > 140 ? `${text.slice(0, 140)}...` : text
}

function summarizeConfigAudit(item: ConfigAuditLog): string {
  const before = (item.before || {}) as ManagedAgentConfig
  const after = (item.after || {}) as ManagedAgentConfig
  const changes: string[] = []
  if (before.agent_name !== after.agent_name) {
    changes.push('名称')
  }
  if (JSON.stringify(before.tags || []) !== JSON.stringify(after.tags || [])) {
    changes.push('标签')
  }
  if (JSON.stringify(before.renewal || {}) !== JSON.stringify(after.renewal || {})) {
    changes.push('VPS 周期')
  }
  if (JSON.stringify(before.entry || {}) !== JSON.stringify(after.entry || {})) {
    changes.push('入口/NAT')
  }
  if (JSON.stringify(before.xui || {}) !== JSON.stringify(after.xui || {})) {
    changes.push('X-UI')
  }
  return changes.length ? `修改了 ${changes.join('、')}` : '保存了配置'
}

function confidenceLabel(value?: string): string {
  switch (value) {
    case 'high':
      return '高置信'
    case 'medium':
      return '中置信'
    case 'low':
      return '低置信'
    default:
      return '未标记'
  }
}

function parseTagInput(rawText: string): string[] {
	const seen = new Set<string>()
	const result: string[] = []
	rawText
		.split(/[,\n，、]/)
		.map((item) => item.trim())
		.filter(Boolean)
		.forEach((item) => {
      const key = item.toLowerCase()
      if (!seen.has(key)) {
        seen.add(key)
        result.push(item)
      }
    })
	return result.sort((left, right) => left.localeCompare(right))
}

function formatTagInput(tags: string[] | undefined): string {
	return (tags || []).join(', ')
}

function mergeTagOptions(current: string[], incoming: string[]): string[] {
  return parseTagInput([...current, ...incoming].join(','))
}

function mergeDashboardTagOptions(dashboardTags: DashboardTagView[], tagOptions: string[]): DashboardTagView[] {
  const byTag = new Map<string, DashboardTagView>()
  for (const tag of dashboardTags) {
    byTag.set(tag.tag, tag)
  }
  for (const tag of tagOptions) {
    if (!byTag.has(tag)) {
      byTag.set(tag, { tag, agent_count: 0, node_count: 0, client_count: 0, online_client_count: 0 })
    }
  }
  return Array.from(byTag.values()).sort((left, right) => left.tag.localeCompare(right.tag))
}

function parseAddressInput(rawText: string): string[] {
  const seen = new Set<string>()
  const result: string[] = []
  rawText
    .split(/[,\n，、]/)
    .map((item) => item.trim())
    .filter(Boolean)
    .forEach((item) => {
      const key = item.toLowerCase()
      if (!seen.has(key)) {
        seen.add(key)
        result.push(item)
      }
    })
  return result.sort((left, right) => left.localeCompare(right))
}

function formatAddressInput(addresses: string[] | undefined): string {
  return (addresses || []).join('\n')
}

function normalizeEntryConfig(config?: AgentEntryConfig): AgentEntryConfig {
  return {
    addresses: parseAddressInput((config?.addresses || []).join('\n')),
    mappings: (config?.mappings || []).map((mapping) => ({
      address: mapping.address || '',
      external_port: Math.max(0, Number(mapping.external_port || 0)),
      internal_port: Math.max(0, Number(mapping.internal_port || 0)),
      protocol: normalizeEntryProtocol(mapping.protocol),
      note: mapping.note || '',
    })),
  }
}

function normalizeEntryProtocol(protocol?: string): AgentEntryMapping['protocol'] {
  switch ((protocol || '').toLowerCase()) {
    case 'vless':
      return 'vless'
    case 'vmess':
      return 'vmess'
    case 'http':
      return 'http'
    case 'socks':
    case 'socks5':
      return 'socks'
    default:
      return 'vless'
  }
}

function buildSectionSavePayload(base: ManagedAgentConfig, draft: ManagedAgentConfig, section: ConfigSectionKey, agentID: string): ManagedAgentConfig {
  const payload: ManagedAgentConfig = {
    ...base,
    agent_id: agentID,
    agent_name: base.agent_name || draft.agent_name || agentID,
    sort_order: base.sort_order || draft.sort_order || 0,
    tags: [...(base.tags || [])],
    renewal: { ...(base.renewal || {}) },
    entry: {
      addresses: [...(base.entry?.addresses || [])],
      mappings: (base.entry?.mappings || []).map((mapping) => ({ ...mapping })),
    },
    xui: { ...base.xui },
  }
  switch (section) {
    case 'client':
      payload.agent_name = draft.agent_name || agentID
      payload.sort_order = Number(draft.sort_order || base.sort_order || 0)
      payload.tags = [...(draft.tags || [])]
      break
    case 'renewal':
      payload.renewal = { ...(draft.renewal || {}) }
      break
    case 'xui':
      payload.xui = { ...draft.xui }
      break
    case 'entry':
      payload.entry = {
        addresses: [...(draft.entry?.addresses || [])],
        mappings: (draft.entry?.mappings || []).map((mapping) => ({ ...mapping })),
      }
      break
  }
  return payload
}

function mergeSavedSectionIntoDraft(draft: ManagedAgentConfig, saved: ManagedAgentConfig, section: ConfigSectionKey): ManagedAgentConfig {
  const next: ManagedAgentConfig = {
    ...draft,
    agent_id: saved.agent_id || draft.agent_id,
  }
  switch (section) {
    case 'client':
      next.agent_name = saved.agent_name
      next.sort_order = saved.sort_order
      next.tags = [...(saved.tags || [])]
      break
    case 'renewal':
      next.renewal = { ...(saved.renewal || {}) }
      break
    case 'xui':
      next.xui = { ...saved.xui }
      break
    case 'entry':
      next.entry = {
        addresses: [...(saved.entry?.addresses || [])],
        mappings: (saved.entry?.mappings || []).map((mapping) => ({ ...mapping })),
      }
      break
  }
  return next
}

function configSectionLabel(section: ConfigSectionKey): string {
  switch (section) {
    case 'client':
      return 'Client 信息'
    case 'renewal':
      return 'VPS 信息'
    case 'xui':
      return 'X-UI 配置'
    case 'entry':
      return '入口/NAT 配置'
  }
}

function configSignature(config: ManagedAgentConfig): string {
  return JSON.stringify(config)
}

function normalizeRenewalConfig(config?: VPSRenewalConfig): VPSRenewalConfig {
  const cycle = config?.cycle === 'week' || config?.cycle === 'quarter' || config?.cycle === 'year' ? config.cycle : 'month'
  const trafficLimitBytes = Math.max(0, Number(config?.traffic_limit_bytes || 0))
  const costAmount = Math.max(0, Number(config?.cost_amount || 0))
  const costCurrency = normalizeCurrencyCode(config?.cost_currency)
  const costCycle = config?.cost_cycle === 'quarter' || config?.cost_cycle === 'year' ? config.cost_cycle : 'month'
  return {
    enabled: Boolean(config?.enabled || config?.start_date || config?.expire_date),
    start_date: config?.start_date || '',
    expire_date: config?.expire_date || '',
    cycle,
    auto_renew: Boolean(config?.auto_renew),
    cost_amount: costAmount,
    cost_currency: costCurrency,
    cost_cycle: costCycle,
    traffic_limit_bytes: trafficLimitBytes,
    bandwidth_mbps: Math.max(0, Number(config?.bandwidth_mbps || 0)),
    traffic_baseline_bytes: trafficLimitBytes > 0 ? Math.max(0, Number(config?.traffic_baseline_bytes || 0)) : 0,
    traffic_sent_baseline_bytes: trafficLimitBytes > 0 ? Math.max(0, Number(config?.traffic_sent_baseline_bytes || 0)) : 0,
    traffic_recv_baseline_bytes: trafficLimitBytes > 0 ? Math.max(0, Number(config?.traffic_recv_baseline_bytes || 0)) : 0,
    traffic_baseline_period_start: trafficLimitBytes > 0 ? config?.traffic_baseline_period_start || '' : '',
  }
}

function calculateRenewalStatus(config?: VPSRenewalConfig): {
  remainingLabel: string
  endLabel: string
  level: 'ok' | 'warn' | 'bad'
  autoRenew: boolean
  percent: number
} | null {
  const normalized = normalizeRenewalConfig(config)
  if (!normalized.enabled || (!normalized.start_date && !normalized.expire_date)) {
    return null
  }
  const period = calculateRenewalPeriod(normalized)
  if (!period) {
    return null
  }
  const now = startOfLocalDay(new Date())
  if (now < period.start) {
    const daysUntilStart = daysBetween(now, period.start)
    return {
      remainingLabel: `${daysUntilStart} 天后开始`,
      endLabel: `到期 ${formatLocalDate(period.end)}`,
      level: 'ok',
      autoRenew: Boolean(normalized.auto_renew),
      percent: 0,
    }
  }
  const remainingDays = daysBetween(now, period.end)
  if (remainingDays < 0) {
    const overdueDays = Math.abs(remainingDays)
    return {
      remainingLabel: `已过期 ${overdueDays} 天`,
      endLabel: `到期 ${formatLocalDate(period.end)}`,
      level: 'bad',
      autoRenew: Boolean(normalized.auto_renew),
      percent: 100,
    }
  }
  const totalDays = Math.max(1, daysBetween(period.start, period.end))
  const remainingPercent = (Math.max(0, remainingDays) / totalDays) * 100
  return {
    remainingLabel: remainingDays === 0 ? '今天到期' : `剩余 ${remainingDays} 天`,
    endLabel: `到期 ${formatLocalDate(period.end)}`,
    level: remainingDays <= 3 ? 'bad' : remainingDays <= 7 ? 'warn' : 'ok',
    autoRenew: Boolean(normalized.auto_renew),
    percent: clampMetricPercent(remainingPercent),
  }
}

function formatRenewalHint(config?: VPSRenewalConfig): string {
  const status = calculateRenewalStatus(config)
  if (!status) {
    return '设置后会在 Client 卡片上展示到期、周期刷新、总/上传/下载流量配额和带宽信息。'
  }
  return `当前周期${status.remainingLabel}，${status.endLabel}，${status.autoRenew ? '到期后自动刷新下一周期，并重新计算总/上传/下载流量' : '到期后不自动刷新'}。`
}

function calculateRenewalPeriod(config: VPSRenewalConfig): { start: Date; end: Date } | null {
  const now = startOfLocalDay(new Date())
  if (config.expire_date) {
    let end = parseLocalDate(config.expire_date)
    if (!end) {
      return null
    }
    end = startOfLocalDay(end)
    if (config.auto_renew && config.cycle) {
      while (end <= now) {
        end = addRenewalCycle(end, config.cycle)
      }
    }
    return {
      start: config.cycle ? subtractRenewalCycle(end, config.cycle) : end,
      end,
    }
  }
  const start = parseLocalDate(config.start_date || '')
  if (!start || !config.cycle) {
    return null
  }
  let periodStart = startOfLocalDay(start)
  let periodEnd = addRenewalCycle(periodStart, config.cycle)
  if (config.auto_renew !== false) {
    while (periodEnd <= now) {
      periodStart = periodEnd
      periodEnd = addRenewalCycle(periodStart, config.cycle)
    }
  }
  return { start: periodStart, end: periodEnd }
}

function parseLocalDate(value: string): Date | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value)
  if (!match) {
    return null
  }
  const year = Number(match[1])
  const month = Number(match[2]) - 1
  const day = Number(match[3])
  const date = new Date(year, month, day)
  if (date.getFullYear() !== year || date.getMonth() !== month || date.getDate() !== day) {
    return null
  }
  return date
}

function startOfLocalDay(date: Date): Date {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate())
}

function addRenewalCycle(date: Date, cycle: VPSRenewalConfig['cycle']): Date {
  if (cycle === 'week') {
    return new Date(date.getFullYear(), date.getMonth(), date.getDate() + 7)
  }
  if (cycle === 'quarter') {
    return addClampedMonths(date, 3)
  }
  if (cycle === 'year') {
    return addClampedMonths(date, 12)
  }
  return addClampedMonths(date, 1)
}

function subtractRenewalCycle(date: Date, cycle: VPSRenewalConfig['cycle']): Date {
  if (cycle === 'week') {
    return new Date(date.getFullYear(), date.getMonth(), date.getDate() - 7)
  }
  if (cycle === 'quarter') {
    return addClampedMonths(date, -3)
  }
  if (cycle === 'year') {
    return addClampedMonths(date, -12)
  }
  return addClampedMonths(date, -1)
}

function addClampedMonths(date: Date, months: number): Date {
  const targetMonth = date.getMonth() + months
  const firstOfTarget = new Date(date.getFullYear(), targetMonth, 1)
  const lastDay = new Date(firstOfTarget.getFullYear(), firstOfTarget.getMonth() + 1, 0).getDate()
  return new Date(firstOfTarget.getFullYear(), firstOfTarget.getMonth(), Math.min(date.getDate(), lastDay))
}

function daysBetween(start: Date, end: Date): number {
  const msPerDay = 24 * 60 * 60 * 1000
  return Math.ceil((startOfLocalDay(end).getTime() - startOfLocalDay(start).getTime()) / msPerDay)
}

function formatLocalDate(date: Date): string {
  const year = date.getFullYear()
  const month = `${date.getMonth() + 1}`.padStart(2, '0')
  const day = `${date.getDate()}`.padStart(2, '0')
  return `${year}/${month}/${day}`
}

function hasSelectedTag(tags: string[] | undefined, selectedTag: string): boolean {
  if (!selectedTag) {
    return true
  }
  return (tags || []).some((tag) => tag === selectedTag)
}

function isAgentRunning(agent: AgentListItem): boolean {
  return (agent.summary.xray_state || '').toLowerCase() === 'running'
}

function topologyMatchesSelectedTag(link: TopologyLinkView, selectedTag: string): boolean {
  if (!selectedTag) {
    return true
  }
  return [...(link.source.agent_tags || []), ...(link.target.agent_tags || [])].includes(selectedTag)
}

function findOutboundLinkedClient(view: GlobalDashboardView | null, agentID: string, outboundTag?: string): TopologyLinkView | undefined {
  if (!view || !agentID || !outboundTag) {
    return undefined
  }
  return (view.links || []).find((link) => link.source.agent_id === agentID && link.source.outbound_tag === outboundTag)
}

function chainMatchesSelectedTag(chain: ClientChainView, selectedTag: string): boolean {
  if (!selectedTag) {
    return true
  }
  return (chain.root_agent_tags || []).includes(selectedTag)
}

function createEmptyManagedConfig(agentID: string, agentName?: string): ManagedAgentConfig {
  return {
    agent_id: agentID,
    agent_name: agentName || agentID,
    sort_order: 0,
    tags: [],
    renewal: {
      enabled: false,
      start_date: '',
      expire_date: '',
      cycle: 'month',
      auto_renew: false,
      cost_amount: 0,
      cost_currency: DEFAULT_COST_CURRENCY,
      cost_cycle: 'month',
      traffic_limit_bytes: 0,
      bandwidth_mbps: 0,
      traffic_baseline_bytes: 0,
      traffic_sent_baseline_bytes: 0,
      traffic_recv_baseline_bytes: 0,
      traffic_baseline_period_start: '',
    },
    entry: {
      addresses: [],
      mappings: [],
    },
    xui: {
      enabled: false,
      base_url: '',
      username: '',
      password: '',
      two_factor_code: '',
      skip_tls_verify: false,
    },
  }
}

function normalizeManagedConfig(config: ManagedAgentConfig, agentID: string, agentName?: string): ManagedAgentConfig {
  const base = createEmptyManagedConfig(agentID, agentName)
  return {
    agent_id: config.agent_id || base.agent_id,
    agent_name: config.agent_name || agentName || base.agent_name,
    sort_order: Number(config.sort_order || base.sort_order || 0),
    tags: parseTagInput((config.tags || []).join(',')),
    renewal: normalizeRenewalConfig(config.renewal || base.renewal),
    entry: normalizeEntryConfig(config.entry || base.entry),
    xui: {
      ...base.xui,
      ...config.xui,
      enabled: Boolean(config.xui?.enabled),
      skip_tls_verify: Boolean(config.xui?.skip_tls_verify),
    },
  }
}

function normalizeXUIOverview(overview: XUIOverview): XUIOverview {
  return {
    ...overview,
    nodes: Array.isArray(overview.nodes) ? overview.nodes : [],
    clients: Array.isArray(overview.clients) ? overview.clients : [],
    outbounds: Array.isArray(overview.outbounds) ? overview.outbounds : [],
    routing_rules: Array.isArray(overview.routing_rules) ? overview.routing_rules : [],
    certificates: Array.isArray(overview.certificates) ? overview.certificates : [],
  }
}

function readStoredAgentViewMode(username: string): AgentViewMode {
  try {
    const value = window.localStorage.getItem(agentViewModeStorageKey(username))
    return value === 'list' ? 'list' : 'card'
  } catch {
    return 'card'
  }
}

function storeAgentViewMode(username: string, mode: AgentViewMode) {
  try {
    window.localStorage.setItem(agentViewModeStorageKey(username), mode)
  } catch {
    // Preference storage is optional; the UI still switches for the current session.
  }
}

function agentViewModeStorageKey(username: string): string {
  return `${AGENT_VIEW_MODE_STORAGE_PREFIX}${username || 'default'}`
}

function formatDateTime(value?: string): string {
  if (!value) {
    return '-'
  }
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).format(new Date(value))
}

function formatRelativeTime(value?: number): string {
  if (!value) {
    return '未活跃'
  }
  const target = new Date(value)
  const diffMinutes = Math.round((Date.now() - target.getTime()) / 60000)
  if (diffMinutes < 1) {
    return '刚刚'
  }
  if (diffMinutes < 60) {
    return `${diffMinutes} 分钟前`
  }
  const diffHours = Math.round(diffMinutes / 60)
  if (diffHours < 24) {
    return `${diffHours} 小时前`
  }
  const diffDays = Math.round(diffHours / 24)
  if (diffDays < 30) {
    return `${diffDays} 天前`
  }
  return formatDateTime(target.toISOString())
}

function isClientOnline(lastOnline?: number, reportedAt?: string): boolean {
  if (!lastOnline) {
    return false
  }
  const compareAt = reportedAt ? new Date(reportedAt).getTime() : Date.now()
  return compareAt - lastOnline <= 5 * 60 * 1000
}

function scopeLabel(scope?: string): string {
  switch (scope) {
    case 'user':
      return '用户规则'
    case 'inbound':
      return '节点规则'
    case 'default':
      return '默认出口'
    case 'global':
      return '全局规则'
    default:
      return '未推断'
  }
}

function scopeColor(scope?: string): string {
  switch (scope) {
    case 'user':
      return 'gold'
    case 'inbound':
      return 'blue'
    case 'default':
      return 'green'
    case 'global':
      return 'magenta'
    default:
      return 'default'
  }
}

function summarizeRule(rule: XUIRoutingRuleView): string {
  const segments: string[] = []
  if (rule.domain?.length) {
    segments.push(`domain(${rule.domain.length})`)
  }
  if (rule.ip?.length) {
    segments.push(`ip(${rule.ip.length})`)
  }
  if (rule.protocol?.length) {
    segments.push(`protocol(${rule.protocol.length})`)
  }
  if (rule.port?.length) {
    segments.push(`port(${rule.port.join(',')})`)
  }
  if (rule.network?.length) {
    segments.push(`network(${rule.network.join(',')})`)
  }
  return segments.join(' · ') || '无额外条件'
}

function statusColor(state?: string): string {
  switch ((state || '').toLowerCase()) {
    case 'running':
      return '#0f766e'
    case 'stop':
      return '#c05621'
    case 'error':
      return '#c53030'
    default:
      return '#94a3b8'
  }
}

function agentStatusLevel(state?: string): 'ok' | 'warn' | 'bad' | 'neutral' {
  switch ((state || '').toLowerCase()) {
    case 'running':
      return 'ok'
    case 'stop':
    case 'stopped':
      return 'warn'
    case 'error':
      return 'bad'
    default:
      return 'neutral'
  }
}

function agentDisplayStatus(agent: AgentListItem): { label: string; level: 'ok' | 'warn' | 'bad' | 'neutral' } {
  const xrayState = (agent.summary.xray_state || '').trim()
  if (xrayState) {
    return { label: xrayState, level: agentStatusLevel(xrayState) }
  }
  if (agent.summary.last_collection_err) {
    return { label: '采集异常', level: 'bad' }
  }
  if (agent.realtime_at) {
    return { label: 'client 在线', level: 'ok' }
  }
  if (agent.reported_at) {
    return { label: 'x-ui 未知', level: 'neutral' }
  }
  return { label: '等待上报', level: 'neutral' }
}

function countryFlag(code?: string): string {
  const normalized = (code || '').trim().toUpperCase()
  if (!/^[A-Z]{2}$/.test(normalized)) {
    return '🌐'
  }
  return Array.from(normalized)
    .map((char) => String.fromCodePoint(127397 + char.charCodeAt(0)))
    .join('')
}

function agentCountryCode(agent: AgentListItem): string {
  const explicitCode = explicitAgentCountryCode(agent)
  if (explicitCode) {
    return explicitCode
  }
  const geoCode = normalizeCountryCode(agent.geo?.country_code)
  return geoCode || ''
}

function explicitAgentCountryCode(agent: AgentListItem): string {
  const candidates = [agent.agent_name || '', ...(agent.tags || []), agent.summary.hostname || '', agent.agent_id || '']
  for (const value of candidates) {
    const code = explicitCountryCodeFromText(value)
    if (code) {
      return code
    }
  }
  return ''
}

function explicitCountryCodeFromText(value?: string): string {
  const text = (value || '').trim().toUpperCase()
  if (!text) {
    return ''
  }
  const direct = normalizeCountryCode(text)
  if (direct) {
    return direct
  }
  const match = /(?:^|[^A-Z0-9])(HK|US|CN|JP|SG|TW|PH|CA|DE|FR|GB|AU)(?=$|[^A-Z0-9])/.exec(text)
  return match ? match[1] : ''
}

function normalizeCountryCode(value?: string): string {
  const code = (value || '').trim().toUpperCase()
  if (['HK', 'US', 'CN', 'JP', 'SG', 'TW', 'PH', 'CA', 'DE', 'FR', 'GB', 'AU'].includes(code)) {
    return code
  }
  return ''
}

function formatAgentLocation(agent: AgentListItem, displayCountryCode: string): string {
  const geoCode = normalizeCountryCode(agent.geo?.country_code)
  const geoLabel = formatGeoLabel(agent.geo)
  if (!displayCountryCode) {
    return geoLabel
  }
  const displayCountry = countryName(displayCountryCode)
  if (!geoLabel) {
    return displayCountry
  }
  if (geoCode && geoCode !== displayCountryCode) {
    return `${displayCountry} · IP库: ${geoLabel}`
  }
  return geoLabel || displayCountry
}

function formatGeoLabel(geo?: IPGeoView): string {
  if (!geo) {
    return ''
  }
  return [geo.country_name || geo.country_code, geo.region_name, geo.city].filter(Boolean).join(' · ')
}

function countryName(code: string): string {
  switch (code) {
    case 'HK':
      return 'Hong Kong'
    case 'US':
      return 'United States'
    case 'CN':
      return 'China'
    case 'JP':
      return 'Japan'
    case 'SG':
      return 'Singapore'
    case 'TW':
      return 'Taiwan'
    case 'PH':
      return 'Philippines'
    case 'CA':
      return 'Canada'
    case 'DE':
      return 'Germany'
    case 'FR':
      return 'France'
    case 'GB':
      return 'United Kingdom'
    case 'AU':
      return 'Australia'
    default:
      return code
  }
}

function outboundElementId(tag: string): string {
  return `outbound-${sanitizeFragment(tag)}`
}

function ruleElementId(index: number): string {
  return `rule-${index}`
}

function nodeElementId(agentID: string, nodeLabel: string): string {
  return `node-${sanitizeFragment(agentID)}-${sanitizeFragment(normalizeNodeAnchorLabel(nodeLabel))}`
}

function normalizeNodeAnchorLabel(value: string): string {
  return value.replace(/:\d+$/, '').trim()
}

function sanitizeFragment(value: string): string {
  return value.replace(/[^a-zA-Z0-9_-]/g, '-')
}

function summarizeAgent(summary?: VPSSummary): string {
  if (!summary) {
    return '-'
  }
  return `${formatPercent(summary.cpu)} CPU · ${formatMem(summary.mem_used, summary.mem_total)}`
}
