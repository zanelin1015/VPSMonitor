import { useEffect, useMemo, useRef, useState } from 'react'
import { App as AntdApp, Alert, AutoComplete, Badge, Button, Card, Checkbox, Empty, Input, InputNumber, Modal, Popconfirm, Segmented, Select, Space, Switch, Table, Tabs, Tag, Tooltip, Typography } from 'antd'
import type { TabsProps } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { AppstoreOutlined, ArrowLeftOutlined, BranchesOutlined, DeleteOutlined, EditOutlined, ReloadOutlined, SaveOutlined, SettingOutlined, SwapOutlined, UnorderedListOutlined } from '@ant-design/icons'

import type {
  AgentEntryConfig,
  AgentLogsResponse,
  ConfigAuditLog,
  CustomerAssignmentDraft,
  DashboardAgentView,
  GlobalDashboardView,
  ManagedAgentConfig,
  TopologyLinkView,
  VPSRenewalConfig,
  XUIAction,
  XUIClientBillingConfig,
  XUIClientConfigSave,
  XUIClientView,
  XUIConfig,
  XUILocalCertificate,
  XUINodeView,
  XUIOverview,
  XUIOutboundView,
  XUIRoutingRuleView,
} from '../types'
import { AdminHelpHint } from './AdminHelpHint'
import { buildRoutingRulesDeletePayload, isPendingRoutingDeletion, routingRuleDeleteDisabledReason } from '../lib/routingRules'
import type { RoutingRulesDeletePayload } from '../lib/routingRules'
import { isPendingRoutingUpdate, routingRuleEditDisabledReason } from '../lib/routingRuleEditor'
import type { RoutingRuleUpdatePayload } from '../lib/routingRuleEditor'
import { XUIRoutingRuleEditor } from './XUIRoutingRuleEditor'
import { buildOutboundsDeletePayload, isPendingOutboundDeletion, outboundDeleteDisabledReason } from '../lib/outbounds'
import type { OutboundsDeletePayload } from '../lib/outbounds'
import { buildOutboundRuleReferences } from '../lib/outboundRuleReferences'
import type { ConfigSectionKey } from '../lib/appHelpers'
import type { CurrencyCode } from '../lib/currency'
import { REVENUE_CURRENCIES } from '../lib/currency'
import { clientBidirectionalTrafficTotal, formatBytes, formatSpeed } from '../lib/traffic'
import {
  actionKindLabel,
  actionStatusColor,
  actionStatusLabel,
  billingKeyForClient,
  clientBillingPatchFromStart,
  dateInputToStartMillis,
  defaultClientBilling,
  effectiveClientBillingExpiryTime,
  effectiveClientBillingStartTime,
  findClientBilling,
  findOutboundLinkedClient,
  formatDateTime,
  formatExpiryTime,
  formatRelativeTime,
  hasSelectedTag,
  isClientOnline,
  mergeTagOptions,
  normalizeClientTrafficMultiplier,
  nodeElementId,
  outboundElementId,
  parseAddressInput,
  ruleElementId,
  shortJSON,
  summarizeRule,
} from '../lib/appHelpers'
import { renderGlobalOverviewPanel } from './DashboardTopologyPanels'
import { defaultTerminalShell, FeatureSwitch, RemoteTTYTerminal } from './AgentDetailTerminal'
import { ManagedConfigPanel } from './ManagedConfigPanel'
import { RouteBadge } from './RouteBadge'
import { XUINodeList } from './XUINodeList'
import { XUIClientConfigModal } from './XUIClientConfigModal'
import { beijingDateInput, clientConfigSaveStatus } from '../lib/clientConfig'
import { clientTrafficResetSummary } from '../lib/clientTrafficReset'
import {
  type HAProxyForwardClientView,
  type HAProxyForwardNodeView,
  type RealmForwardClientView,
  type RealmForwardNodeView,
  type AgentFeatureKey,
  buildPrimaryDomainOptions,
  buildHAProxyForwardClients,
  buildHAProxyForwardNodes,
  buildRealmForwardClients,
  buildRealmForwardNodes,
  filterRealmForwardClients,
  firstCertificateDomainCandidate,
  realmForwardNodeKey,
  renderNodeClientHierarchySections,
} from './AgentDetailHelpers'

const { Text, Title } = Typography

export interface AgentDetailPanelProps {
  activeTabKey: string
  agentLogs: AgentLogsResponse | null
  agentLogsError: string
  agentLogsLoading: boolean
  clientSearch: string
  configAudits: ConfigAuditLog[]
  configAuditsLoading: boolean
  configError: string
  configLoading: boolean
  configSavingSection: ConfigSectionKey | null
  currencyOptions: CurrencyCode[]
  dashboardView: GlobalDashboardView | null
  entryAddressInputText: string
  filteredAgents: DashboardAgentView[]
  filteredClients: XUIClientView[]
  filteredTagLinks: TopologyLinkView[]
  managedConfig: ManagedAgentConfig | null
  savedClientBillings?: XUIClientBillingConfig[]
  onClientConfigSaved: (record: XUIClientView, save: XUIClientConfigSave) => void
  onClientConfigUnauthorized: () => void
  newTagName: string
  overview: XUIOverview | null
  overviewLoading: boolean
  overviewError: string
  selectedAgent: DashboardAgentView
  selectedAgentId: string
  selectedNodeAnchor: string
  selectedOutboundTag: string
  selectedRuleIndex: number | null
  selectedTag: string
  tagOptions: string[]
  tagSaving: boolean
  xuiActions: XUIAction[]
  xuiActionsLoading: boolean
  xuiClientDeleteLoadingKey: string
  routingRuleDeleteLoading: boolean
  routingRuleUpdateLoading: boolean
  outboundDeleteLoading: boolean
  xuiClientToggleLoadingKey: string
  xuiClientResetLoadingKey: string
  agentDeleteLoading: boolean
  agentReplaceLoading: boolean
  replacementAgents: DashboardAgentView[]
  onBackToList: () => void
  canOpenXUI: boolean
  canManageConfig: boolean
  restrictedView?: boolean
  currentAgentLoading: boolean
  remoteCommandLoading: boolean
  xuiRestartLoading: boolean
  xuiUpdateLoading: boolean
  realmCopyLoading: boolean
  onActiveTabChange: (key: string) => void
  onClientSearchChange: (value: string) => void
  onCopyImportURL: (client: XUIClientView) => void
  onCreateRoutingAction: () => void
  onCreateNodeClientAction: (node: XUINodeView, actionAgentID?: string) => void
  onCreateTag: () => void
  onEntryAddressesTextChange: (value: string) => void
  onEntryChange: (patch: Partial<AgentEntryConfig>) => void
  onJumpNode: (agentID?: string, nodeLabel?: string) => void
  onJumpOutbound: (tag?: string) => void
  onJumpRule: (index?: number) => void
  onManagedConfigAgentNameChange: (value: string) => void
  onManagedConfigCustomerDisplayNameChange: (value: string) => void
  onManagedConfigSortOrderChange: (value: number) => void
  onNewTagNameChange: (value: string) => void
  onOpenImportURL: (client: XUIClientView) => void
  onOpenLogs: () => void
  onOpenXUI: () => void
  onAuthorizeCustomer: (draft: CustomerAssignmentDraft) => void
  onDeleteCurrentAgent: () => void
  onReplaceCurrentAgent: (replacementAgentID: string) => Promise<boolean>
  onDeleteXUIClient: (client: XUIClientView) => void
  onDeleteRoutingRules: (payload: RoutingRulesDeletePayload) => Promise<XUIAction | null>
  onUpdateRoutingRule: (payload: RoutingRuleUpdatePayload) => Promise<XUIAction | null>
  onRoutingRuleUpdated?: () => void
  onDeleteOutbounds: (payload: OutboundsDeletePayload) => Promise<XUIAction | null>
  onSetXUIClientEnabled: (client: XUIClientView, enabled: boolean) => void
  onManualResetXUIClientTraffic: (client: XUIClientView) => Promise<void>
  onRefreshCurrentAgent: () => void
  onRestartXUI: () => void
  onUpdate3XUI: () => void
  onExecuteRemoteCommand: (command: string, shell: string, timeoutSeconds: number) => void
  onRefreshXUIActions: () => void
  onCopyRealmConfig: (targetAgentID: string) => void
  onRenewalChange: (patch: Partial<VPSRenewalConfig>) => void
  onSaveManagedConfigSection: (section: ConfigSectionKey, draftOverride?: ManagedAgentConfig) => void
  onSavePrimaryDomain: (value: string) => void
  onSelectTag: (tag: string) => void
  onTagsChange: (values: string[]) => void
  onSaveAreaTags?: (values: string[]) => void
  onXUIChange: (patch: Partial<XUIConfig>) => void
  onFeatureChange: (feature: AgentFeatureKey, enabled: boolean) => void
}

function normalizeBillingCycle(cycle?: string): 'month' | 'quarter' | 'semiannual' | 'year' {
  return cycle === 'quarter' || cycle === 'semiannual' || cycle === 'year' ? cycle : 'month'
}

function billingCycleLabel(cycle?: string): string {
  switch (normalizeBillingCycle(cycle)) {
    case 'quarter':
      return '季'
    case 'semiannual':
      return '半年'
    case 'year':
      return '年'
    default:
      return '月'
  }
}

function isAccountBasedProxyClient(client: XUIClientView): boolean {
  return ['http', 'socks', 'socks5'].includes((client.protocol || '').toLowerCase())
}

export function AgentDetailPanel(props: AgentDetailPanelProps) {
  const { message } = AntdApp.useApp()
  const {
    activeTabKey,
    agentLogs,
    agentLogsError,
    agentLogsLoading,
    clientSearch,
    configAudits,
    configAuditsLoading,
    configError,
    configLoading,
    configSavingSection,
    currencyOptions,
    dashboardView,
    entryAddressInputText,
    filteredAgents,
    filteredClients,
    filteredTagLinks,
    managedConfig,
    savedClientBillings,
    onClientConfigSaved,
    onClientConfigUnauthorized,
    newTagName,
    overview,
    overviewLoading,
    overviewError,
    selectedAgent,
    selectedAgentId,
    selectedNodeAnchor,
    selectedOutboundTag,
    selectedRuleIndex,
    selectedTag,
    tagOptions,
    tagSaving,
    xuiActions,
    xuiActionsLoading,
    xuiClientDeleteLoadingKey,
    routingRuleDeleteLoading,
    routingRuleUpdateLoading,
    outboundDeleteLoading,
    xuiClientToggleLoadingKey,
    xuiClientResetLoadingKey,
    agentDeleteLoading,
    agentReplaceLoading,
    replacementAgents,
    onBackToList,
    canOpenXUI,
    canManageConfig,
    restrictedView = false,
    currentAgentLoading,
    remoteCommandLoading,
    xuiRestartLoading,
    xuiUpdateLoading,
    realmCopyLoading,
    onActiveTabChange,
    onClientSearchChange,
    onCopyImportURL,
    onCreateRoutingAction,
    onCreateNodeClientAction,
    onCreateTag,
    onEntryAddressesTextChange,
    onEntryChange,
    onJumpNode,
    onJumpOutbound,
    onJumpRule,
    onManagedConfigAgentNameChange,
    onManagedConfigCustomerDisplayNameChange,
    onManagedConfigSortOrderChange,
    onNewTagNameChange,
    onOpenImportURL,
    onOpenLogs,
    onOpenXUI,
    onAuthorizeCustomer,
    onDeleteCurrentAgent,
    onReplaceCurrentAgent,
    onDeleteXUIClient,
    onDeleteRoutingRules,
    onUpdateRoutingRule,
    onRoutingRuleUpdated,
    onDeleteOutbounds,
    onSetXUIClientEnabled,
    onManualResetXUIClientTraffic,
    onRefreshCurrentAgent,
    onRestartXUI,
    onUpdate3XUI,
    onExecuteRemoteCommand,
    onRefreshXUIActions,
    onCopyRealmConfig,
    onRenewalChange,
    onSaveManagedConfigSection,
    onSavePrimaryDomain,
    onSelectTag,
    onTagsChange,
    onSaveAreaTags,
    onXUIChange,
    onFeatureChange,
  } = props
  const [remoteCommandOpen, setRemoteCommandOpen] = useState(false)
  const [clientConfigEditor, setClientConfigEditor] = useState<XUIClientView | null>(null)
  const [remoteCommand, setRemoteCommand] = useState('')
  const [remoteShell, setRemoteShell] = useState('bash')
  const [remoteTimeout, setRemoteTimeout] = useState(120)
  const xuiClientActionKey = (record: XUIClientView) => [record.inbound_id, record.inbound_tag || '', record.client_id || record.auth_uuid || record.auth_password || '', record.email || ''].join(':')
  const [commandOutputAction, setCommandOutputAction] = useState<XUIAction | null>(null)
  const [terminalOpen, setTerminalOpen] = useState(false)
  const [replacementModalOpen, setReplacementModalOpen] = useState(false)
  const [replacementAgentID, setReplacementAgentID] = useState('')
  const [terminalShell, setTerminalShell] = useState(defaultTerminalShell(selectedAgent.client_os, selectedAgent.system_version))
  const [terminalFontSize, setTerminalFontSize] = useState(13)
  const [terminalExpanded, setTerminalExpanded] = useState(false)
  const [selectedRoutingRuleKeys, setSelectedRoutingRuleKeys] = useState<number[]>([])
  const [routingDeleteDraft, setRoutingDeleteDraft] = useState<RoutingRulesDeletePayload | null>(null)
  const [submittedRoutingDeleteID, setSubmittedRoutingDeleteID] = useState<number | null>(null)
  const [routingEditor, setRoutingEditor] = useState<{ rule: XUIRoutingRuleView; fingerprint: string } | null>(null)
  const [submittedRoutingUpdateID, setSubmittedRoutingUpdateID] = useState<number | null>(null)
  const [selectedOutboundTags, setSelectedOutboundTags] = useState<string[]>([])
  const [outboundView, setOutboundView] = useState<'list' | 'card'>('list')
  const [routingView, setRoutingView] = useState<'list' | 'card'>('list')
  const [referenceOutboundTag, setReferenceOutboundTag] = useState<string | null>(null)
  const [outboundDeleteDraft, setOutboundDeleteDraft] = useState<OutboundsDeletePayload | null>(null)
  const [submittedOutboundDeleteID, setSubmittedOutboundDeleteID] = useState<number | null>(null)
  const refreshRoutingActionsRef = useRef(onRefreshXUIActions)
  refreshRoutingActionsRef.current = onRefreshXUIActions
  const refreshRoutingOverviewRef = useRef(onRoutingRuleUpdated)
  refreshRoutingOverviewRef.current = onRoutingRuleUpdated
  const routingFingerprint = overview?.routing_rules_fingerprint || ''
  const routingRules = overview?.editable_routing_rules ?? overview?.routing_rules ?? []
  const outboundRuleReferences = useMemo(() => buildOutboundRuleReferences(overview?.outbounds || [], overview?.editable_routing_rules ?? overview?.routing_rules ?? [], overview?.balancers), [overview])
  const pendingRoutingDelete = submittedRoutingDeleteID !== null
    || xuiActions.some((action) => action.agent_id === selectedAgentId && isPendingRoutingDeletion(action))
  const outboundFingerprint = overview?.outbounds_fingerprint || ''
  const pendingOutboundDelete = submittedOutboundDeleteID !== null
    || xuiActions.some(action => action.agent_id === selectedAgentId && isPendingOutboundDeletion(action))
  const pendingRoutingUpdate = submittedRoutingUpdateID !== null || xuiActions.some(action => action.agent_id === selectedAgentId && isPendingRoutingUpdate(action))
  const routingDeleteBusy = routingRuleDeleteLoading || routingRuleUpdateLoading || outboundDeleteLoading || xuiActionsLoading || pendingRoutingDelete || pendingOutboundDelete || pendingRoutingUpdate
  const outboundDeleteBusy = routingDeleteBusy
  const canDeleteRoutes = canManageConfig && !restrictedView
  const canDeleteOutbounds = canDeleteRoutes
  const deletableOutbounds = overview?.outbounds.filter(outbound => !outboundDeleteDisabledReason(outbound)) || []

  useEffect(() => {
    setClientConfigEditor(null)
  }, [selectedAgentId])

  useEffect(() => {
    setSelectedRoutingRuleKeys([])
  }, [selectedAgentId, routingFingerprint])

  useEffect(() => { setSelectedOutboundTags([]) }, [selectedAgentId, outboundFingerprint])
  useEffect(() => { setReferenceOutboundTag(null) }, [selectedAgentId])
  useEffect(() => { setRoutingEditor(null); setSubmittedRoutingUpdateID(null) }, [selectedAgentId])
  useEffect(() => {
    const action = xuiActions.find(item => item.id === submittedRoutingUpdateID)
    if (action && !isPendingRoutingUpdate(action)) {
      setSubmittedRoutingUpdateID(null)
      refreshRoutingOverviewRef.current?.()
    }
  }, [submittedRoutingUpdateID, xuiActions])
  useEffect(() => { setOutboundDeleteDraft(null); setSubmittedOutboundDeleteID(null) }, [selectedAgentId])
  useEffect(() => {
    const action = xuiActions.find(item => item.id === submittedOutboundDeleteID)
    if (action && !isPendingOutboundDeletion(action)) setSubmittedOutboundDeleteID(null)
  }, [submittedOutboundDeleteID, xuiActions])

  useEffect(() => {
    const action = xuiActions.find((item) => item.id === submittedRoutingDeleteID)
    if (action && !isPendingRoutingDeletion(action)) setSubmittedRoutingDeleteID(null)
  }, [submittedRoutingDeleteID, xuiActions])

  useEffect(() => {
    if (!pendingRoutingDelete && !pendingOutboundDelete && !pendingRoutingUpdate) return
    const timer = window.setInterval(() => refreshRoutingActionsRef.current(), 3000)
    return () => window.clearInterval(timer)
  }, [pendingRoutingDelete, pendingOutboundDelete, pendingRoutingUpdate, selectedAgentId])

  async function submitRoutingUpdate(payload: RoutingRuleUpdatePayload) {
    if (!canDeleteRoutes || routingDeleteBusy || payload.rules_fingerprint !== routingFingerprint) return null
    const action = await onUpdateRoutingRule(payload)
    if (action && isPendingRoutingUpdate(action)) setSubmittedRoutingUpdateID(action.id)
    return action
  }

  function confirmOutboundDeletion(outbounds: XUIOutboundView[]) {
    if (!canDeleteOutbounds || outboundDeleteBusy || !outbounds.length) return
    try { setOutboundDeleteDraft(buildOutboundsDeletePayload(outbounds, outboundFingerprint)) }
    catch (error) { message.error(error instanceof Error ? error.message : '请选择可删除的出站') }
  }

  async function submitOutboundDeletion() {
    if (!outboundDeleteDraft || !canDeleteOutbounds || outboundDeleteBusy || outboundDeleteDraft.config_fingerprint !== outboundFingerprint) return
    const action = await onDeleteOutbounds(outboundDeleteDraft)
    if (action) {
      if (isPendingOutboundDeletion(action)) setSubmittedOutboundDeleteID(action.id)
      setOutboundDeleteDraft(null)
      setSelectedOutboundTags([])
    }
  }

  function confirmRoutingRuleDeletion(rules: XUIRoutingRuleView[]) {
    if (!canDeleteRoutes || routingDeleteBusy || !rules.length) return
    // Capture exact targets when the dialog opens. A background refresh must
    // never silently change the rules the user is about to confirm.
    try {
      setRoutingDeleteDraft(buildRoutingRulesDeletePayload(rules, routingFingerprint))
    } catch (error) {
      message.error(error instanceof Error ? error.message : '请选择有效的路由规则')
    }
  }

  async function submitRoutingRuleDeletion() {
    if (!routingDeleteDraft || !canDeleteRoutes || routingDeleteBusy || routingDeleteDraft.rules_fingerprint !== routingFingerprint) return
    const action = await onDeleteRoutingRules(routingDeleteDraft)
    if (action) {
      if (isPendingRoutingDeletion(action)) setSubmittedRoutingDeleteID(action.id)
      setRoutingDeleteDraft(null)
      setSelectedRoutingRuleKeys([])
    }
  }

  async function submitAgentReplacement() {
    if (!replacementAgentID) {
      return
    }
    if (await onReplaceCurrentAgent(replacementAgentID)) {
      setReplacementModalOpen(false)
      setReplacementAgentID('')
    }
  }
  const currentAgentLinks = filteredTagLinks.filter((link) => link.source.agent_id === selectedAgentId || link.target.agent_id === selectedAgentId)
  const currentAgentRealmLinks = currentAgentLinks.filter((link) => link.source.agent_id === selectedAgentId && (link.source.protocol || '').toLowerCase() === 'realm')
  const realmForwardNodes = buildRealmForwardNodes(currentAgentRealmLinks, dashboardView, overview?.nodes || [])
  const realmForwardClients = buildRealmForwardClients(currentAgentRealmLinks, dashboardView, overview?.clients || [])
  const realmFilteredClients = filterRealmForwardClients(realmForwardClients, clientSearch)
  const haProxyForwardNodes = buildHAProxyForwardNodes(dashboardView, overview?.nodes || [])
  const haProxyForwardClients = buildHAProxyForwardClients(dashboardView, overview?.clients || [])
  const haProxyFilteredClients = filterRealmForwardClients(haProxyForwardClients, clientSearch)
  const currentAgentTagSet = new Set(selectedAgent.tags || [])
  const currentAgentDashboardView = dashboardView
    ? {
        ...dashboardView,
        tags: dashboardView.tags.filter((tag) => currentAgentTagSet.has(tag.tag)),
      }
    : null

  const commandResult = commandOutputAction?.result || {}
  const commandPayload = commandOutputAction?.payload || {}
  const commandText = String(commandResult.command || commandPayload.command || '')
  const stdoutText = String(commandResult.stdout || '')
  const stderrText = String(commandResult.stderr || commandOutputAction?.error || '')
  const currentPrimaryDomain = managedConfig?.entry?.import_domain || selectedAgent.entry?.import_domain || ''
  const certificateDomainOptions = buildPrimaryDomainOptions(overview?.certificates || [])
  const [primaryDomainDraft, setPrimaryDomainDraft] = useState(currentPrimaryDomain)
  const featureEnabled = {
    xui: Boolean(managedConfig?.features?.xui),
    realm: Boolean(managedConfig?.features?.realm),
    haproxy: Boolean(managedConfig?.features?.haproxy),
    nat: Boolean(managedConfig?.features?.nat),
    port_policy: Boolean(managedConfig?.features?.port_policy || selectedAgent.network_policy?.rules?.length),
  }
  const realmRuleCount = managedConfig?.entry?.port_forwarding?.rules?.length || selectedAgent.entry?.port_forwarding?.rules?.length || 0
  const natMappingCount = managedConfig?.entry?.mappings?.length || selectedAgent.entry?.mappings?.length || 0
  const portPolicyCount = managedConfig?.entry?.network_policy?.rules?.length || selectedAgent.entry?.network_policy?.rules?.length || selectedAgent.network_policy?.rules?.length || 0
  const internalNodeCount = overview?.nodes.length ?? selectedAgent.node_count ?? 0
  const internalClientCount = overview?.clients.length ?? selectedAgent.client_count ?? 0
  const internalOnlineClientCount = overview?.online_client_count ?? selectedAgent.online_client_count ?? 0
  const internalSummary = overview?.summary || selectedAgent.summary
  const internalTrafficSent = Number(internalSummary.net_traffic_sent || 0)
  const internalTrafficRecv = Number(internalSummary.net_traffic_recv || 0)
  const internalTrafficTotal = internalTrafficSent + internalTrafficRecv
  const internalTrafficMax = Math.max(1, internalTrafficSent, internalTrafficRecv)
  const internalOverviewStats = (
    <div className="overview-stat-grid client-internal-overview-grid">
      {featureEnabled.xui ? (
        <section className="overview-stat-card overview-stat-blue">
          <div className="overview-stat-title">x-ui 节点</div>
          <div className="overview-stat-value"><span className="overview-stat-dot" /><strong>{internalNodeCount}</strong></div>
          <div className="overview-stat-foot">客户端 {internalClientCount} · 在线 {internalOnlineClientCount}</div>
        </section>
      ) : null}
      {featureEnabled.xui ? (
        <section className="overview-stat-card overview-network-card">
          <div className="overview-stat-title">实时速度</div>
          <div className="overview-network-total">
            <span className="network-up">↑ {formatSpeed(Number(internalSummary.net_io_up || 0))}</span>
            <span className="network-down">↓ {formatSpeed(Number(internalSummary.net_io_down || 0))}</span>
          </div>
          <div className="overview-stat-foot">总速 {formatSpeed(Number(internalSummary.net_io_up || 0) + Number(internalSummary.net_io_down || 0))}</div>
        </section>
      ) : null}
      {featureEnabled.realm && realmRuleCount > 0 ? (
        <section className="overview-stat-card overview-stat-green">
          <div className="overview-stat-title">Realm 转发</div>
          <div className="overview-stat-value"><span className="overview-stat-dot" /><strong>{realmRuleCount}</strong></div>
          <div className="overview-stat-foot">当前 Client 已启用转发规则</div>
        </section>
      ) : null}
      {featureEnabled.port_policy && portPolicyCount > 0 ? (
        <section className="overview-stat-card overview-stat-red">
          <div className="overview-stat-title">端口策略</div>
          <div className="overview-stat-value"><span className="overview-stat-dot" /><strong>{portPolicyCount}</strong></div>
          <div className="overview-stat-foot">端口限速 / 白名单策略数量</div>
        </section>
      ) : null}
      <section className="overview-stat-card overview-traffic-chart-card client-internal-traffic-card">
        <div className="overview-stat-title">已用流量图</div>
        <div className="overview-network-total">
          <span className="network-up">{formatBytes(internalTrafficTotal)}</span>
          <span className="network-down">↑{formatBytes(internalTrafficSent)} · ↓{formatBytes(internalTrafficRecv)}</span>
        </div>
        <div className="overview-traffic-bars">
          <div className="overview-traffic-row">
            <span>上传</span>
            <div><i style={{ width: `${Math.max(4, Math.min(100, (internalTrafficSent / internalTrafficMax) * 100))}%` }} /></div>
            <strong>{formatBytes(internalTrafficSent)}</strong>
          </div>
          <div className="overview-traffic-row">
            <span>下载</span>
            <div><i style={{ width: `${Math.max(4, Math.min(100, (internalTrafficRecv / internalTrafficMax) * 100))}%` }} /></div>
            <strong>{formatBytes(internalTrafficRecv)}</strong>
          </div>
        </div>
      </section>
    </div>
  )

  useEffect(() => {
    setPrimaryDomainDraft(currentPrimaryDomain)
  }, [currentPrimaryDomain, selectedAgentId])

  function submitRemoteCommand() {
    const command = remoteCommand.trim()
    if (!command) {
      return
    }
    onExecuteRemoteCommand(command, remoteShell, remoteTimeout)
    setRemoteCommandOpen(false)
  }

  function openRealtimeTerminal() {
    setTerminalShell(defaultTerminalShell(selectedAgent.client_os, selectedAgent.system_version))
    setTerminalOpen(true)
  }

  function openRemoteCommand() {
    setRemoteShell(defaultTerminalShell(selectedAgent.client_os, selectedAgent.system_version))
    setRemoteCommandOpen(true)
  }

  function zoomTerminal(delta: number) {
    setTerminalFontSize((value) => Math.min(22, Math.max(10, value + delta)))
  }

  function resetTerminalZoom() {
    setTerminalFontSize(13)
    setTerminalExpanded(false)
  }

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
      width: 150,
      render: (value: number | undefined, record) => (
        <Space wrap size={[6, 6]}>
          <Tag>{value ?? 0}</Tag>
          <Button size="small" type="link" disabled={!canManageConfig && !restrictedView} onClick={() => onCreateNodeClientAction(record)}>
            新增客户端
          </Button>
        </Space>
      ),
    },
    {
      title: '近 5 分钟在线',
      dataIndex: 'online_count',
      width: 130,
      render: (value?: number) => <Badge color={value ? '#0f766e' : '#94a3b8'} text={String(value ?? 0)} />,
    },
    {
      title: '累计流量',
      key: 'traffic',
      width: 130,
      render: (_, record) => formatBytes(record.all_time || record.total || 0),
    },
    {
      title: '用户授权',
      key: 'customer',
      width: 120,
      render: (_, record) => (
        <Button
          size="small"
          onClick={() => onAuthorizeCustomer({
            agent_id: selectedAgentId,
            inbound_id: record.id,
            inbound_tag: record.tag || '',
            public_client_name: [selectedAgent.customer_display_name || selectedAgent.agent_name || selectedAgent.agent_id, record.remark || record.tag || `Inbound #${record.id}`].filter(Boolean).join(' - '),
          })}
        >
          授权给用户
        </Button>
      ),
    },
    {
      title: '路由指向',
      key: 'route',
      width: 280,
      render: (_, record) => <RouteBadge route={record.route} onJumpOutbound={onJumpOutbound} onJumpRule={onJumpRule} />,
    },
  ]

  const clientColumns: ColumnsType<XUIClientView> = [
    {
      title: '客户端',
      key: 'client',
      width: 200,
      fixed: 'left',
      render: (_, record) => (
        <div className="client-identity-cell">
          <Text strong>{record.email || '-'}</Text>
          <div className="muted-line">{record.comment || record.sub_id || '未备注'}</div>
        </div>
      ),
    },
    {
      title: '所在节点',
      key: 'inbound',
      // Let this column absorb spare width in the compact account view.
      // Otherwise table-layout: fixed expands the action column as well.
      width: restrictedView ? undefined : 180,
      render: (_, record) => (
        <div>
          <Text>{record.inbound_remark || record.inbound_tag || '-'}</Text>
          <div className="muted-line">{record.inbound_tag || '-'}</div>
          {record.export_endpoint?.error && <Tooltip title={record.export_endpoint.error}><Tag color="warning">连接映射待配置</Tag></Tooltip>}
        </div>
      ),
    },
    {
      title: '协议',
      dataIndex: 'protocol',
      key: 'protocol',
      width: 84,
      render: (value?: string) => <Tooltip title={value}><Tag color="geekblue" className="client-protocol-tag">{value === 'shadowsocks' ? 'SS' : value || '-'}</Tag></Tooltip>,
    },
    {
      title: '状态',
      key: 'status',
      width: 90,
      render: (_, record) => {
        const accountBasedProxy = isAccountBasedProxyClient(record)
        const configuredEnabled = record.config_enabled ?? record.enabled
        const online = isClientOnline(record.last_online, overview?.reported_at)
        return (
          <div className="client-status-cell">
            <Tooltip title={accountBasedProxy ? '节点账号请在 x-ui 入站中维护' : !canManageConfig ? '当前账号没有修改权限' : `点击${configuredEnabled ? '禁用' : '启用'}客户端`}>
              <span className="client-enabled-control">
                <Switch
                  className="client-enabled-switch"
                  checked={configuredEnabled}
                  checkedChildren="启用"
                  unCheckedChildren="禁用"
                  disabled={!canManageConfig || accountBasedProxy || Boolean(xuiClientResetLoadingKey) || Boolean(xuiClientToggleLoadingKey && xuiClientToggleLoadingKey !== xuiClientActionKey(record))}
                  loading={xuiClientToggleLoadingKey === xuiClientActionKey(record)}
                  aria-label={`${record.email || record.comment || '客户端'}：启用状态`}
                  aria-busy={xuiClientToggleLoadingKey === xuiClientActionKey(record)}
                  onChange={(checked) => onSetXUIClientEnabled(record, checked)}
                />
              </span>
            </Tooltip>
            {accountBasedProxy ? <Tag>节点账号</Tag> : (
              <Tag color={online ? 'processing' : 'default'}>{online ? '在线' : '离线'}</Tag>
            )}
          </div>
        )
      },
    },
    {
      title: '流量',
      key: 'traffic',
      width: 210,
      render: (_, record) => {
        const total = clientBidirectionalTrafficTotal(record)
        const limit = Math.max(0, Number(record.total_gb || 0))
        const billing = findClientBilling(savedClientBillings || managedConfig?.renewal?.client_billings, record) || defaultClientBilling(record)
        const resetPending = xuiActions.some(action => action.kind === 'manual_reset_client_traffic'
          && action.agent_id === (record.realm_target_agent_id || selectedAgentId)
          && action.payload?.email === record.email
          && ['pending', 'running'].includes(action.status))
        const resetLoading = resetPending || xuiClientResetLoadingKey === xuiClientActionKey(record)
        return <div className="client-traffic-cell">
          <div className="client-traffic-usage">
            <span className="client-summary-label">已用</span>
            <strong>{formatBytes(total)}</strong>
            <Tooltip title="流量上限"><span className="client-summary-chip">{limit > 0 ? `/ ${formatBytes(limit)}` : '无上限'}</span></Tooltip>
          </div>
          {!restrictedView ? <div className="client-traffic-directions">
            <Tooltip title="上传"><span aria-label={`上传 ${formatBytes(record.up || 0)}`}><span className="client-traffic-arrow" aria-hidden="true">↑</span>{formatBytes(record.up || 0)}</span></Tooltip>
            <Tooltip title="下载"><span aria-label={`下载 ${formatBytes(record.down || 0)}`}><span className="client-traffic-arrow" aria-hidden="true">↓</span>{formatBytes(record.down || 0)}</span></Tooltip>
          </div> : null}
          {!restrictedView ? <div className="client-traffic-reset">
            <span className="client-summary-label">重置</span>
            <span className="client-summary-chip">{clientTrafficResetSummary(record)}</span>
            <Tooltip title="流量计费倍率"><span className="client-summary-chip">{normalizeClientTrafficMultiplier(billing.traffic_multiplier)} 倍</span></Tooltip>
            {canManageConfig && !isAccountBasedProxyClient(record) ? (
              <Popconfirm
                title={`重置「${record.email || '客户端'}」的流量？`}
                description={<div className="client-traffic-reset-confirm">清空该客户端的已用上传、下载流量；不修改流量上限、到期时间和自动重置周期。若同一客户端关联多个入站，其共享流量会一并重置。此操作无法撤销。</div>}
                okText="确认重置"
                cancelText="取消"
                okButtonProps={{ danger: true }}
                cancelButtonProps={{ disabled: Boolean(xuiClientResetLoadingKey) }}
                disabled={Boolean(xuiClientResetLoadingKey || xuiClientToggleLoadingKey) || resetPending || !record.email || !(record.client_id || record.auth_uuid || record.auth_password)}
                onConfirm={() => onManualResetXUIClientTraffic(record)}
              >
                <Button
                  size="small"
                  className="client-manual-reset-button"
                  icon={<ReloadOutlined />}
                  loading={resetLoading}
                  disabled={Boolean(xuiClientResetLoadingKey && xuiClientResetLoadingKey !== xuiClientActionKey(record)) || Boolean(xuiClientToggleLoadingKey) || !record.email || !(record.client_id || record.auth_uuid || record.auth_password)}
                  aria-label={`手动重置 ${record.email || '客户端'} 的流量`}
                >手动重置</Button>
              </Popconfirm>
            ) : null}
          </div> : null}
        </div>
      },
    },
    {
      title: '配置信息',
      key: 'billing',
      width: 250,
      render: (_, record) => {
        const billing = findClientBilling(savedClientBillings || managedConfig?.renewal?.client_billings, record) || defaultClientBilling(record)
        const start = effectiveClientBillingStartTime(billing, record.expiry_time || 0)
        const expiry = effectiveClientBillingExpiryTime(billing, record.expiry_time || 0)
        const save = overview?.client_config_saves?.find(item => item.client_id ? item.client_id === record.client_id : item.inbound_id === record.inbound_id && item.inbound_tag === record.inbound_tag && item.email === record.email)
        const status = clientConfigSaveStatus(save)
        return <div className="client-config-summary">
          <div className="client-config-price">
            <strong><span className="client-config-currency">{billing.revenue_currency || 'CNY'}</span> {Number(billing.revenue_amount || 0).toFixed(2)}</strong>
            <span className="client-summary-label">/ {billingCycleLabel(billing.revenue_cycle)}</span>
          </div>
          <div className="client-config-dates">
            <div className="client-config-date"><span className="client-summary-label">开始</span><span className={!start ? 'client-summary-empty' : undefined}>{beijingDateInput(start) || '未设置'}</span></div>
            <Tooltip title={`北京时间：${formatExpiryTime(expiry)}；x-ui 当前：${formatExpiryTime(record.expiry_time)}`}>
              <div className="client-config-date"><span className="client-summary-label">到期</span><span className={!expiry ? 'client-summary-empty' : 'client-config-expiry'}>{beijingDateInput(expiry) || '不限期'}</span></div>
            </Tooltip>
          </div>
          {status ? <Tooltip title={save?.action?.error || (save?.action ? '以远端回读核验结果为准' : '本系统收费配置已保存')}><Tag color={status.color}>{status.text}</Tag></Tooltip> : null}
        </div>
      },
    },
    {
      title: '最近活跃',
      dataIndex: 'last_online',
      key: 'last_online',
      width: 160,
      render: (value?: number) => formatRelativeTime(value),
    },
    {
      title: '路由指向',
      key: 'route',
      width: 280,
      render: (_, record) => <RouteBadge route={record.route} onJumpOutbound={onJumpOutbound} onJumpRule={onJumpRule} />,
    },
    {
      title: '操作',
      key: 'actions',
      width: restrictedView ? 230 : 240,
      fixed: 'right',
      render: (_, record) => (
        <div className="client-action-controls">
          <div className="client-action-row">
            {!restrictedView ? <Button size="small" disabled={!canManageConfig || !record.email} onClick={() => setClientConfigEditor(record)}>编辑</Button> : null}
            <Button size="small" disabled={!record.import_url} onClick={() => onCopyImportURL(record)}>
              复制
            </Button>
            <Button size="small" disabled={!record.import_url} onClick={() => onOpenImportURL(record)}>
              URL / 二维码
            </Button>
          </div>
          <div className="client-action-row">
            <Button
              size="small"
              onClick={() => onAuthorizeCustomer({
                agent_id: selectedAgentId,
                inbound_id: record.inbound_id,
                inbound_tag: record.inbound_tag || '',
                client_id: record.client_id || '',
                client_email: record.email || '',
                public_client_name: [selectedAgent.customer_display_name || selectedAgent.agent_name || selectedAgent.agent_id, record.email || record.comment || record.inbound_tag || `Inbound #${record.inbound_id}`].filter(Boolean).join(' - '),
              })}
            >
              授权给用户
            </Button>
            {restrictedView && !isAccountBasedProxyClient(record) ? (
              <Button
                size="small"
                disabled={Boolean(xuiClientToggleLoadingKey && xuiClientToggleLoadingKey !== xuiClientActionKey(record))}
                loading={xuiClientToggleLoadingKey === xuiClientActionKey(record)}
                onClick={() => onSetXUIClientEnabled(record, !(record.config_enabled ?? record.enabled))}
              >
                {(record.config_enabled ?? record.enabled) ? '停用' : '启用'}
              </Button>
            ) : null}
            <Popconfirm
              title="删除这个 Client？"
              description={`将从 x-ui 入站 ${record.inbound_remark || record.inbound_tag || record.inbound_id} 删除 ${record.email || '该客户端'}，不会重启 Xray。`}
              okText="删除"
              cancelText="取消"
              okButtonProps={{ danger: true }}
              onConfirm={() => onDeleteXUIClient(record)}
            >
              <Button
                size="small"
                danger
                disabled={!canManageConfig && !restrictedView}
                loading={xuiClientDeleteLoadingKey === xuiClientActionKey(record)}
              >
                删除
              </Button>
            </Popconfirm>
          </div>
        </div>
      ),
    },
  ]
  const visibleClientColumns = restrictedView
    ? clientColumns.filter((column) => !['protocol', 'status', 'billing', 'expiry', 'last_online', 'route'].includes(String(column.key || '')))
    : clientColumns
  const realmNodeColumns: ColumnsType<RealmForwardNodeView> = [
    {
      title: '命中节点',
      key: 'node',
      width: 260,
      render: (_, record) => (
        <div>
          <Text strong>{record.remark || record.tag || `Node #${record.id}`}</Text>
          <div className="muted-line">{record.tag || '-'}</div>
        </div>
      ),
    },
    {
      title: '目标 Client',
      key: 'target_agent',
      width: 200,
      render: (_, record) => <Tag color="blue">{record.realm_target_agent_name || record.realm_target_agent_id || record.listen || '-'}</Tag>,
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
      title: '命中客户端',
      dataIndex: 'client_count',
      width: 120,
      render: (value?: number) => <Tag>{value ?? 0}</Tag>,
    },
    {
      title: '操作',
      key: 'actions',
      width: 130,
      render: (_, record) => (
        <Button
          size="small"
          disabled={!record.realm_target_agent_id || (!canManageConfig && !restrictedView)}
          onClick={() => onCreateNodeClientAction(record, record.realm_target_agent_id)}
        >
          新增客户端
        </Button>
      ),
    },
    {
      title: 'Realm 来源',
      key: 'route',
      width: 260,
      render: (_, record) => <RouteBadge route={record.route} onJumpOutbound={onJumpOutbound} onJumpRule={onJumpRule} />,
    },
  ]
  const visibleRealmNodeColumns = restrictedView
    ? realmNodeColumns.filter((column) => column.key !== 'route')
    : realmNodeColumns
  const realmClientColumns: ColumnsType<RealmForwardClientView> = [
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
      title: '目标 Client',
      key: 'target_agent',
      width: 200,
      render: (_, record) => <Tag color="blue">{record.realm_target_agent_name || record.realm_target_agent_id || '-'}</Tag>,
    },
    {
      title: '命中节点',
      key: 'inbound',
      width: 220,
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
      width: 110,
      render: (value?: string) => <Tag color="geekblue">{value || '-'}</Tag>,
    },
    {
      title: '状态',
      key: 'status',
      width: 120,
      render: (_, record) => (
        <Space wrap size={[6, 6]}>
          <Tag color={record.enabled ? 'blue' : 'default'}>{record.enabled ? '启用' : '停用'}</Tag>
        </Space>
      ),
    },
    {
      title: '导出',
      key: 'import',
      width: 150,
      render: (_, record) => (
        <Space wrap size={[6, 6]}>
          <Button size="small" disabled={!record.import_url} onClick={() => onCopyImportURL(record)}>
            复制
          </Button>
          <Button size="small" disabled={!record.import_url} onClick={() => onOpenImportURL(record)}>
            URL
          </Button>
        </Space>
      ),
    },
    {
      title: 'Realm 来源',
      key: 'route',
      width: 280,
      render: (_, record) => <RouteBadge route={record.route} onJumpOutbound={onJumpOutbound} onJumpRule={onJumpRule} />,
    },
  ]
  const visibleRealmClientColumns = restrictedView
    ? realmClientColumns.filter((column) => !['target_agent', 'inbound', 'route'].includes(String(column.key || '')))
    : realmClientColumns
  const haProxyNodeColumns: ColumnsType<HAProxyForwardNodeView> = realmNodeColumns.filter((column) => column.key !== 'route')
  const visibleHAProxyNodeColumns = haProxyNodeColumns
  const haProxyClientColumns: ColumnsType<HAProxyForwardClientView> = realmClientColumns.filter((column) => column.key !== 'route')
  const visibleHAProxyClientColumns = restrictedView
    ? haProxyClientColumns.filter((column) => !['target_agent', 'inbound'].includes(String(column.key || '')))
    : haProxyClientColumns

  function renderRoutingValidity(record: XUIRoutingRuleView) {
    const state = record.enabled === false ? 'disabled' : record.validity || (!record.inbound_tags?.length && !record.users?.length || record.protected ? 'valid' : 'unknown')
    const labels = { valid: '有效', partial: '部分有效', invalid: '无效', disabled: '停用', unknown: '待判断' }
    const colors = { valid: 'success', partial: 'warning', invalid: 'error', disabled: 'default', unknown: 'default' }
    const reason = record.validity_reasons?.join('；') || (state === 'unknown' ? '请更新 Server 并刷新 Client 信息' : state === 'valid' ? '当前配置可匹配' : '')
    const visibleReason = (record.validity_reasons?.length || 0) > 2 ? `${record.validity_reasons!.slice(0, 2).join('；')}；另 ${record.validity_reasons!.length - 2} 项` : reason
    return <div className="routing-validity-cell">
      <Tag color={colors[state]}>{labels[state]}</Tag>
      {reason ? <Tooltip title={reason}><span className="routing-validity-reason">{visibleReason}</span></Tooltip> : null}
    </div>
  }

  function renderRoutingMatchers(record: XUIRoutingRuleView) {
    return <Space wrap size={[6, 6]}>
      {record.inbound_tags?.map(tag => <Tag key={`inbound-${tag}`} color="blue">{tag}</Tag>)}
      {record.users?.map(user => <Tag key={`user-${user}`} color="gold">{user}</Tag>)}
      {!record.inbound_tags?.length && !record.users?.length ? <Tag>全局条件</Tag> : null}
    </Space>
  }

  function renderRoutingTarget(record: XUIRoutingRuleView) {
    return <Space wrap size={[6, 6]}>
      {record.outbound_tag ? <Button type="link" className="route-link" onClick={() => onJumpOutbound(record.outbound_tag)}>{record.outbound_tag}</Button> : null}
      {record.balancer_tag ? <Tag color="magenta">balancer:{record.balancer_tag}</Tag> : null}
    </Space>
  }

  function renderRoutingActions(record: XUIRoutingRuleView) {
    return <Space size={8} className="routing-rule-actions">
      <Tooltip title={routingRuleEditDisabledReason(record) || (!routingFingerprint ? '请刷新 Client 信息以读取面板保存的规则' : '编辑当前规则，保留原有顺序')}>
        <Button size="small" icon={<EditOutlined />} disabled={routingDeleteBusy || !routingFingerprint || Boolean(routingRuleEditDisabledReason(record))} onClick={() => setRoutingEditor({ rule: record, fingerprint: routingFingerprint })}>编辑</Button>
      </Tooltip>
      <Tooltip title={routingRuleDeleteDisabledReason(record) || (!routingFingerprint ? '请升级 Bridge Client，并立即获取 Client 信息以读取面板保存的规则' : undefined)}>
        <Button size="small" danger icon={<DeleteOutlined />} disabled={routingDeleteBusy || !routingFingerprint || Boolean(routingRuleDeleteDisabledReason(record))} onClick={() => confirmRoutingRuleDeletion([record])}>删除</Button>
      </Tooltip>
    </Space>
  }

  function renderOutboundReferences(outbound: XUIOutboundView) {
    const references = outboundRuleReferences.get(outbound.tag || '') || []
    return references.length ? <div className="outbound-rule-association">
      <Button size="small" icon={<BranchesOutlined />} aria-label={`查看出站 ${outbound.tag} 的 ${references.length} 条关联路由`} onClick={() => setReferenceOutboundTag(outbound.tag!)}>{references.length} 条规则</Button>
      <Text type="secondary">{references.slice(0, 3).map(item => `R${item.rule.index}`).join('、')}{references.length > 3 ? ` 等 ${references.length} 条` : ''}</Text>
    </div> : <Tooltip title={outbound.is_default ? '默认出口仍可处理未命中路由规则的流量' : '当前上报的规则没有直接或通过均衡器指向此出站'}><Tag className="outbound-unreferenced">无显式关联</Tag></Tooltip>
  }

  function renderOutboundMetrics(outbound: XUIOutboundView) {
    return <div className="outbound-traffic-cell">
      <Text>累计 <strong>{formatBytes(outbound.total || 0)}</strong></Text>
      <div className="outbound-metrics"><span title="上传">↑ {formatBytes(outbound.up || 0)}</span><span title="下载">↓ {formatBytes(outbound.down || 0)}</span></div>
    </div>
  }

  function renderOutboundTarget(outbound: XUIOutboundView) {
    const linked = findOutboundLinkedClient(dashboardView, selectedAgentId, outbound.tag)
    const label = linked ? linked.target.inbound_name || linked.target.inbound_tag || String(linked.target.inbound_id) : ''
    return <div className="outbound-target-cell">
      <div className="outbound-target">{outbound.target || '当前配置未提供远端地址'}</div>
      {linked ? <Button type="link" className="outbound-jump" onClick={() => onJumpNode(linked.target.agent_id, label)}>跳转到 {linked.target.agent_name || linked.target.agent_id} / {label}</Button> : null}
      {outbound.send_through ? <Tag>sendThrough:{outbound.send_through}</Tag> : null}
    </div>
  }

  function renderOutboundDelete(outbound: XUIOutboundView) {
    const reason = outboundDeleteDisabledReason(outbound)
    return <Tooltip title={reason || undefined}><Button size="small" danger icon={<DeleteOutlined />} disabled={outboundDeleteBusy || !outboundFingerprint || Boolean(reason)} onClick={() => confirmOutboundDeletion([outbound])}>删除</Button></Tooltip>
  }

  const outboundColumns: ColumnsType<XUIOutboundView> = [
    { title: '出站', key: 'outbound', width: 260, render: (_, outbound) => <div id={outboundElementId(outbound.tag || 'unknown')} className="outbound-identity">
      <Text className="outbound-tag">{outbound.tag || '-'}</Text>
      <Space wrap size={[6, 6]}><Tag>{outbound.protocol || '-'}</Tag>{outbound.is_default ? <Tag color="success">默认出口</Tag> : null}</Space>
    </div> },
    { title: '目标 / 关联节点', key: 'target', width: 290, render: (_, outbound) => renderOutboundTarget(outbound) },
    { title: '关联路由', key: 'rules', width: 180, render: (_, outbound) => renderOutboundReferences(outbound) },
    { title: '流量', key: 'traffic', width: 220, render: (_, outbound) => renderOutboundMetrics(outbound) },
  ]
  if (canDeleteOutbounds) outboundColumns.push({ title: '操作', key: 'actions', width: 100, fixed: 'right', render: (_, outbound) => renderOutboundDelete(outbound) })

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
      title: '有效状态',
      key: 'validity',
      width: 230,
      render: (_, record) => renderRoutingValidity(record),
    },
    {
      title: '入口匹配',
      key: 'matchers',
      width: 280,
      render: (_, record) => renderRoutingMatchers(record),
    },
    {
      title: '出站 / 均衡器',
      key: 'route',
      width: 220,
      render: (_, record) => renderRoutingTarget(record),
    },
    {
      title: '条件摘要',
      dataIndex: 'summary',
      render: (value: string | undefined, record) => value || summarizeRule(record),
    },
  ]
  if (canDeleteRoutes) {
    routingColumns.push({
      title: '操作',
      key: 'actions',
      width: 180,
      fixed: 'right',
      render: (_, record) => renderRoutingActions(record),
    })
  }

  const xuiActionColumns: ColumnsType<XUIAction> = [
    { title: 'ID', dataIndex: 'id', width: 76 },
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
          {record.error && shortJSON(record.result) ? <div className="muted-line">{shortJSON(record.result)}</div> : null}
          {['execute_command', 'update_3xui'].includes(record.kind) ? (
            <div>
              <Button size="small" type="link" onClick={() => setCommandOutputAction(record)}>
                查看输出
              </Button>
            </div>
          ) : null}
          <div className="muted-line">创建 {formatDateTime(record.created_at)}{record.completed_at ? ` · 完成 ${formatDateTime(record.completed_at)}` : ''}</div>
        </div>
      ),
    },
  ]

  const agentLogColumns: ColumnsType<NonNullable<AgentLogsResponse['logs']>[number]> = [
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
    {
      title: '主域名',
      key: 'primary_domain',
      width: 150,
      fixed: 'right',
      render: (_, record) => {
        const domain = firstCertificateDomainCandidate(record)
        if (!domain) {
          return <Text type="secondary">无域名</Text>
        }
        const selected = currentPrimaryDomain === domain
        return (
          <Button
            size="small"
            type={selected ? 'primary' : 'default'}
            disabled={!canManageConfig || configSavingSection === 'entry'}
            loading={configSavingSection === 'entry' && selected}
            onClick={() => onSavePrimaryDomain(domain)}
          >
            {selected ? '当前主域名' : '设为主域名'}
          </Button>
        )
      },
    },
  ]

  const certificateDomainPanel = overview ? (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Alert
        type="info"
        showIcon
        message="VPS 主域名"
        description="导出的单节点链接会优先使用这里保存的主域名替代 IP；如果该 VPS 是 Realm 或 HAProxy 入口机，转发后的节点也会使用入口机的主域名和监听端口。SNI、Reality 等目标节点参数保持不变。"
      />
      <Space.Compact style={{ width: '100%' }}>
        <AutoComplete
          allowClear
          value={primaryDomainDraft}
          options={certificateDomainOptions}
          placeholder="可从证书域名选择，也可以手动输入域名"
          onChange={setPrimaryDomainDraft}
          filterOption={(inputValue, option) => String(option?.value || '').toLowerCase().includes(inputValue.toLowerCase())}
          style={{ width: '100%' }}
          disabled={!canManageConfig}
        />
        <Button
          type="primary"
          loading={configSavingSection === 'entry'}
          disabled={!canManageConfig || configSavingSection === 'entry' || primaryDomainDraft.trim() === currentPrimaryDomain}
          onClick={() => onSavePrimaryDomain(primaryDomainDraft)}
        >
          保存主域名
        </Button>
      </Space.Compact>
      <Text type="secondary">
        {currentPrimaryDomain ? `当前主域名：${currentPrimaryDomain}` : '当前未设置主域名；未设置时会继续按证书域名、入口地址、公网 IP 的顺序回退。'}
      </Text>
      <Table rowKey={(record) => record.id} columns={certificateColumns} dataSource={overview.certificates} pagination={{ pageSize: 8, hideOnSinglePage: true }} scroll={{ x: 1360 }} />
    </Space>
  ) : (
    <Empty description="暂无本机证书数据" />
  )

  function renderManagedConfigSection(section: 'basic' | 'xui' | 'nat' | 'network' | 'realm' | 'haproxy' | 'audit') {
    return (
      <ManagedConfigPanel
        selectedAgent={selectedAgent}
        agents={dashboardView?.agents || filteredAgents}
        managedConfig={managedConfig}
        certificates={overview?.certificates || []}
        nodes={overview?.nodes || []}
        configLoading={configLoading}
        configSavingSection={configSavingSection}
        configError={configError}
        onSave={onSaveManagedConfigSection}
        onAgentNameChange={onManagedConfigAgentNameChange}
        onCustomerDisplayNameChange={onManagedConfigCustomerDisplayNameChange}
        onSortOrderChange={onManagedConfigSortOrderChange}
        tagOptions={tagOptions}
        newTagName={newTagName}
        tagSaving={tagSaving}
        onNewTagNameChange={onNewTagNameChange}
        onCreateTag={onCreateTag}
        onTagsChange={(values) => {
          const tags = mergeTagOptions([], values)
          onTagsChange(tags)
        }}
        onRenewalChange={onRenewalChange}
        entryAddressInputText={entryAddressInputText}
        onEntryAddressesTextChange={onEntryAddressesTextChange}
        onEntryChange={onEntryChange}
        onXUIChange={onXUIChange}
        onCopyRealmConfig={onCopyRealmConfig}
        realmCopyLoading={realmCopyLoading}
        configAudits={configAudits}
        configAuditsLoading={configAuditsLoading}
        currencyOptions={currencyOptions}
        section={section}
      />
    )
  }

  const featureSwitchPanel = canManageConfig ? (
    <div className="agent-feature-switch-panel">
      <div>
        <Text strong>启用功能</Text>
        <div className="muted-line">只打开当前 VPS 拥有的能力；关闭后对应操作入口会从详情页隐藏。</div>
      </div>
      <Space wrap>
        <FeatureSwitch label="x-ui" checked={featureEnabled.xui} onChange={(checked) => onFeatureChange('xui', checked)} />
        <FeatureSwitch label="Realm" checked={featureEnabled.realm} onChange={(checked) => onFeatureChange('realm', checked)} />
        <FeatureSwitch label="HAProxy" checked={featureEnabled.haproxy} onChange={(checked) => onFeatureChange('haproxy', checked)} />
        <FeatureSwitch label="NAT" checked={featureEnabled.nat} onChange={(checked) => onFeatureChange('nat', checked)} />
        <FeatureSwitch label="端口限速" checked={featureEnabled.port_policy} onChange={(checked) => onFeatureChange('port_policy', checked)} />
        <Button size="small" type="primary" loading={configSavingSection === 'client'} onClick={() => onSaveManagedConfigSection('client')}>保存功能选择</Button>
      </Space>
    </div>
  ) : null

  const overviewTab: NonNullable<TabsProps['items']>[number] = {
    key: 'overview',
    label: `总览 (${selectedAgentId ? 1 : 0})`,
    children: (
      <Space direction="vertical" size="middle" style={{ width: '100%' }}>
        {featureSwitchPanel}
        {internalOverviewStats}
        {!restrictedView ? renderGlobalOverviewPanel({
          dashboardView: currentAgentDashboardView,
          selectedTag,
          links: currentAgentLinks,
          onSelectTag: (value) => onSelectTag(value),
          scopeAgentID: selectedAgentId,
          scopeAgentName: selectedAgent.agent_name || selectedAgent.agent_id,
          showRealm: featureEnabled.realm,
          showMatchedLinks: featureEnabled.xui,
        }) : null}
      </Space>
    ),
  }

  const xuiTabs: TabsProps['items'] = featureEnabled.xui ? [
    {
      key: 'actions',
      label: `x-ui 操作 (${xuiActions.length})`,
      children: (
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
          <Alert
            type="warning"
            showIcon
            message="这里现在通过 WS 实时下发 x-ui 指令"
            description="入站节点请直接通过 x-ui 面板手动新增；中心会把其它 client 的节点导入为当前 client 的出站，并通过在线 Client 实时执行和回传结果。Client 不在线时会保留任务等待轮询。"
          />
          <Space wrap>
            <Button type="primary" disabled={!selectedAgentId} onClick={onCreateRoutingAction}>新增操作</Button>
            {!restrictedView ? <Button danger onClick={openRealtimeTerminal}>实时 TTY</Button> : null}
            {!restrictedView ? <Button loading={remoteCommandLoading} onClick={openRemoteCommand}>单次命令</Button> : null}
            {!restrictedView ? (
              <Popconfirm
                title="升级 3x-ui？"
                description="将通过在线 Client 执行 3x-ui 官方 update.sh 升级脚本，过程中可能短暂影响 x-ui / Xray。"
                okText="升级"
                cancelText="取消"
                onConfirm={onUpdate3XUI}
              >
                <Button loading={xuiUpdateLoading}>升级 3x-ui</Button>
              </Popconfirm>
            ) : null}
            <Button icon={<ReloadOutlined />} disabled={!selectedAgentId} loading={xuiActionsLoading} onClick={onRefreshXUIActions}>刷新操作记录</Button>
          </Space>
          <Table rowKey={(record) => record.id} columns={xuiActionColumns} dataSource={xuiActions} loading={xuiActionsLoading} pagination={{ pageSize: 8, hideOnSinglePage: true }} scroll={{ x: 820 }} />
        </Space>
      ),
    },
    ...(canManageConfig ? [{
      key: 'xui-config',
      label: 'X-ui 配置',
      children: renderManagedConfigSection('xui'),
    }] : []),
    {
      key: 'nodes',
      label: `节点 (${overview?.nodes.length || 0})`,
      children: overview ? (
        <XUINodeList
          key={selectedAgentId}
          nodes={overview.nodes}
          agentID={selectedAgentId}
          selectedNodeAnchor={selectedNodeAnchor}
          active={activeTabKey === 'nodes'}
        >
          {(nodes) => renderNodeClientHierarchySections(
            nodes,
            filteredClients,
            nodeColumns,
            selectedAgent.customer_display_name || selectedAgent.agent_name || selectedAgent.agent_id,
            selectedAgentId,
            selectedNodeAnchor,
            restrictedView ? 800 : 1600,
            'clients',
            onActiveTabChange,
            onClientSearchChange,
          )}
        </XUINodeList>
      ) : (
        <Empty description="暂无 x-ui 节点数据" />
      ),
    },
    {
      key: 'clients',
      label: `客户端 (${overview?.clients.length || 0})`,
      children: overview ? (
        <Space direction="vertical" style={{ width: '100%' }} size="middle">
          <Input.Search allowClear style={{ minWidth: 280 }} placeholder="按邮箱、备注、节点标签筛选客户端" value={clientSearch} onChange={(event) => onClientSearchChange(event.target.value)} />
          <Table
            className="xui-client-table"
            rowKey={(record) => `${record.realm_target_agent_id || ''}-${record.inbound_id}-${record.inbound_tag || ''}-${record.email || record.comment || record.sub_id || ''}`}
            columns={visibleClientColumns}
            dataSource={filteredClients}
            pagination={false}
            tableLayout="fixed"
            // Let rc-table measure the declared column widths intrinsically. A
            // numeric x value equal to their sum can leave the sticky layer
            // with a stale scroll range after a fast right-to-left fling.
            scroll={{ x: 'max-content' }}
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
        <>
          <div className="xui-collection-toolbar">
          {canDeleteOutbounds ? <Space wrap className="outbound-batch-toolbar">
            <Button danger icon={<DeleteOutlined />} loading={outboundDeleteLoading} disabled={outboundDeleteBusy || !outboundFingerprint || !selectedOutboundTags.length}
              onClick={() => confirmOutboundDeletion(overview.outbounds.filter(outbound => selectedOutboundTags.includes(outbound.tag || '')))}>
              批量删除{selectedOutboundTags.length ? ` (${selectedOutboundTags.length})` : ''}
            </Button>
            <Button disabled={outboundDeleteBusy || !outboundFingerprint || !deletableOutbounds.length} onClick={() => setSelectedOutboundTags(deletableOutbounds.map(outbound => outbound.tag!))}>全选可删除</Button>
            {selectedOutboundTags.length ? <Button type="link" disabled={outboundDeleteBusy} onClick={() => setSelectedOutboundTags([])}>取消选择</Button> : null}
            <AdminHelpHint title="批量移除所选出站，一次保存完整配置并回读核验，然后重载 Xray。默认出口、被路由/均衡器/代理链引用的出站不可删除；请先调整引用。运行时生成的出站需在来源面板维护。需要目标 VPSMonitor Client v0.3.49 或更高版本。" />
            {outboundDeleteBusy ? <Text type="secondary">删除任务下发中或执行中，请查看 x-ui 操作记录</Text> : null}
            {!outboundFingerprint ? <Text type="secondary">请升级目标 VPSMonitor Client 并立即获取 Client 信息</Text> : null}
          </Space> : <span />}
          <Segmented aria-label="出站查看方式" value={outboundView} options={[{ value: 'list', label: '列表', icon: <UnorderedListOutlined aria-hidden /> }, { value: 'card', label: '卡片', icon: <AppstoreOutlined aria-hidden /> }]} onChange={value => setOutboundView(value as 'list' | 'card')} />
          </div>
          {overview.editable_routing_rules === undefined ? <div className="routing-validity-help"><Text type="secondary">关联按当前运行时规则展示，停用规则可能未上报；刷新采集后可查看面板保存的完整规则。</Text></div> : null}
          {outboundView === 'list' ? <Table<XUIOutboundView>
            className="xui-outbound-table"
            rowKey={outbound => outbound.tag || 'unknown'} columns={outboundColumns} dataSource={overview.outbounds} pagination={false} tableLayout="fixed" scroll={{ x: 1100 }}
            locale={{ emptyText: <Empty description="暂无出站数据" /> }}
            rowClassName={outbound => outbound.tag === selectedOutboundTag ? 'outbound-row-selected' : ''}
            rowSelection={canDeleteOutbounds ? {
              selectedRowKeys: selectedOutboundTags,
              onChange: keys => setSelectedOutboundTags(keys.map(String)),
              getCheckboxProps: outbound => ({ disabled: outboundDeleteBusy || !outboundFingerprint || Boolean(outboundDeleteDisabledReason(outbound)), title: outboundDeleteDisabledReason(outbound), 'aria-label': `选择出站 ${outbound.tag || '-'}` }),
            } : undefined}
          /> : overview.outbounds.length ? <div className="outbound-grid">
          {overview.outbounds.map((outbound) => {
            const selected = outbound.tag && outbound.tag === selectedOutboundTag
            const checked = selectedOutboundTags.includes(outbound.tag || '')
            const blockedReason = outboundDeleteDisabledReason(outbound)
            const linkedClient = findOutboundLinkedClient(dashboardView, selectedAgentId, outbound.tag)
            return (
              <section
                key={outbound.tag || 'unknown'}
                id={outboundElementId(outbound.tag || 'unknown')}
                className={`outbound-card${selected ? ' selected' : ''}${checked ? ' checked' : ''}${linkedClient ? ' linked' : ''}`}
              >
                {canDeleteOutbounds ? <div className="outbound-select-row">
                  <Tooltip title={blockedReason || undefined}>
                    <Checkbox aria-label={`选择出站 ${outbound.tag || '-'}`} checked={checked} disabled={outboundDeleteBusy || !outboundFingerprint || Boolean(blockedReason)}
                      onChange={event => setSelectedOutboundTags(current => event.target.checked ? [...current, outbound.tag!] : current.filter(tag => tag !== outbound.tag))}>选择</Checkbox>
                  </Tooltip>
                  {renderOutboundDelete(outbound)}
                </div> : null}
                <div className="outbound-head">
                  <div>
                    <Text className="outbound-tag">{outbound.tag || '-'}</Text>
                    <div className="muted-line">{outbound.protocol || '-'}{outbound.is_default ? ' · 默认出口' : ''}</div>
                  </div>
                  <Space wrap size={[6, 6]} className="outbound-head-meta">
                    {outbound.is_default ? <Tag color="success">默认</Tag> : null}
                    {linkedClient ? <Tag color="cyan">关联 {linkedClient.target.agent_name || linkedClient.target.agent_id}</Tag> : null}
                  </Space>
                </div>
                {renderOutboundTarget(outbound)}
                {renderOutboundMetrics(outbound)}
                <div className="outbound-card-rules"><Text type="secondary">关联路由</Text>{renderOutboundReferences(outbound)}</div>
                {canDeleteOutbounds && blockedReason ? <Text type="secondary" className="outbound-delete-reason">{blockedReason}</Text> : null}
              </section>
            )
          })}
          </div> : <Empty description="暂无出站数据" />}
        </>
      ) : (
        <Empty description="暂无出站数据" />
      ),
    },
    {
      key: 'routes',
      label: `路由规则 (${routingRules.length})`,
      children: overview ? (
        <>
          <div className="routing-validity-help">
            <Text type="secondary">按当前入站、客户端及出站引用判断，全局条件与系统 API 路由视为有效；不依据在线状态或流量。</Text>
          </div>
          <div className="xui-collection-toolbar">
          {canDeleteRoutes ? (
            <Space wrap>
              <Button
                danger
                icon={<DeleteOutlined />}
                loading={routingRuleDeleteLoading}
                disabled={routingDeleteBusy || !selectedRoutingRuleKeys.length || !routingFingerprint}
                onClick={() => confirmRoutingRuleDeletion(routingRules.filter((rule) => selectedRoutingRuleKeys.includes(rule.index)))}
              >批量删除{selectedRoutingRuleKeys.length ? ` (${selectedRoutingRuleKeys.length})` : ''}</Button>
              <Button disabled={routingDeleteBusy || !routingFingerprint || !routingRules.some(rule => !routingRuleDeleteDisabledReason(rule))} onClick={() => setSelectedRoutingRuleKeys(routingRules.filter(rule => !routingRuleDeleteDisabledReason(rule)).map(rule => rule.index))}>全选可删除</Button>
              {selectedRoutingRuleKeys.length ? <Button type="link" disabled={routingDeleteBusy} onClick={() => setSelectedRoutingRuleKeys([])}>取消选择</Button> : null}
              <AdminHelpHint title="勾选后可批量删除。按倒序逐条移除，统一保存并重载 Xray；不删除客户端或出站。系统 API 路由不可删除，规则变化时会取消本次操作。执行结果可在 x-ui 操作记录查看。需升级 Bridge Client 后使用。" />
              {routingDeleteBusy ? <Text type="secondary">路由或出站任务下发中或执行中，请查看 x-ui 操作记录</Text> : null}
            </Space>
          ) : <span />}
          <Segmented aria-label="路由规则查看方式" value={routingView} options={[{ value: 'list', label: '列表', icon: <UnorderedListOutlined aria-hidden /> }, { value: 'card', label: '卡片', icon: <AppstoreOutlined aria-hidden /> }]} onChange={value => setRoutingView(value as 'list' | 'card')} />
          </div>
          {routingView === 'list' ? <Table
            rowKey={(record) => record.index}
            columns={routingColumns}
            dataSource={routingRules}
            pagination={false}
            locale={{ emptyText: <Empty description="暂无路由规则数据" /> }}
            scroll={{ x: 1280 }}
            rowClassName={(record) => [selectedRuleIndex === record.index ? 'route-row-selected' : '', record.validity === 'invalid' ? 'route-row-invalid' : ''].filter(Boolean).join(' ')}
            rowSelection={canDeleteRoutes ? {
              selectedRowKeys: selectedRoutingRuleKeys,
              onChange: (keys) => setSelectedRoutingRuleKeys(keys.map(Number)),
              getCheckboxProps: (record) => ({
                disabled: routingDeleteBusy || !routingFingerprint || Boolean(routingRuleDeleteDisabledReason(record)),
                title: routingRuleDeleteDisabledReason(record),
                'aria-label': `选择路由 R${record.index}`,
              }),
            } : undefined}
          /> : routingRules.length ? <div className="routing-rule-grid">
            {routingRules.map(rule => <section key={rule.index} className={`routing-rule-card${selectedRuleIndex === rule.index ? ' selected' : ''}${selectedRoutingRuleKeys.includes(rule.index) ? ' checked' : ''}`}>
              <div className="routing-rule-card-head">
                <Space size={10}>
                  {canDeleteRoutes ? <Tooltip title={routingRuleDeleteDisabledReason(rule) || undefined}><Checkbox aria-label={`选择路由 R${rule.index}`} checked={selectedRoutingRuleKeys.includes(rule.index)} disabled={routingDeleteBusy || !routingFingerprint || Boolean(routingRuleDeleteDisabledReason(rule))} onChange={event => setSelectedRoutingRuleKeys(current => event.target.checked ? [...current, rule.index] : current.filter(index => index !== rule.index))} /></Tooltip> : null}
                  <span id={ruleElementId(rule.index)} className={selectedRuleIndex === rule.index ? 'route-anchor active' : 'route-anchor'}>R{rule.index}</span>
                </Space>
                {canDeleteRoutes ? renderRoutingActions(rule) : null}
              </div>
              {renderRoutingValidity(rule)}
              <div className="routing-rule-card-field"><Text type="secondary">入口匹配</Text>{renderRoutingMatchers(rule)}</div>
              <div className="routing-rule-card-field"><Text type="secondary">出站 / 均衡器</Text>{renderRoutingTarget(rule)}</div>
              <div className="routing-rule-card-summary">{rule.summary || summarizeRule(rule)}</div>
            </section>)}
          </div> : <Empty description="暂无路由规则数据" />}
        </>
      ) : (
        <Empty description="暂无路由规则数据" />
      ),
    },
  ] : []

  const realmTabs: TabsProps['items'] = featureEnabled.realm ? [
    ...(canManageConfig ? [{
      key: 'realm-forwarding',
      label: 'Realm 转发',
      children: renderManagedConfigSection('realm'),
    }] : []),
    {
      key: 'realm-nodes',
      label: `节点 (${realmForwardNodes.length})`,
      children: realmForwardNodes.length ? (
        renderNodeClientHierarchySections(
          realmForwardNodes,
          realmFilteredClients,
          visibleRealmNodeColumns as ColumnsType<XUINodeView>,
          selectedAgent.customer_display_name || selectedAgent.agent_name || selectedAgent.agent_id,
          selectedAgentId,
          selectedNodeAnchor,
          restrictedView ? 760 : 1120,
          'realm-clients',
          onActiveTabChange,
          onClientSearchChange,
        )
      ) : (
        <Empty description="暂无通过 Realm 转发命中的节点" />
      ),
    },
    {
      key: 'realm-clients',
      label: `客户端 (${realmForwardClients.length})`,
      children: realmForwardClients.length ? (
        <Space direction="vertical" style={{ width: '100%' }} size="middle">
          <Input.Search allowClear style={{ minWidth: 280 }} placeholder="按邮箱、备注、节点标签筛选 Realm 命中客户端" value={clientSearch} onChange={(event) => onClientSearchChange(event.target.value)} />
          <Table
            rowKey={(record) => `${record.realm_target_agent_id || ''}-${record.inbound_id}-${record.inbound_tag || ''}-${record.email || record.comment || record.sub_id || ''}`}
            columns={visibleRealmClientColumns}
            dataSource={realmFilteredClients}
            pagination={false}
            scroll={{ x: restrictedView ? 760 : 1120 }}
          />
        </Space>
      ) : (
        <Empty description="暂无通过 Realm 转发命中的客户端" />
      ),
    },
  ] : []

  const natTabs: TabsProps['items'] = (featureEnabled.nat || featureEnabled.xui || featureEnabled.realm || featureEnabled.haproxy) && canManageConfig ? [{
    key: 'entry-nat',
    label: `连接与端口 (${natMappingCount})`,
    children: renderManagedConfigSection('nat'),
  }] : []

  const haProxyTabs: TabsProps['items'] = featureEnabled.haproxy ? [
    ...(canManageConfig ? [{
      key: 'haproxy-forwarding',
      label: `HAProxy 主备 (${managedConfig?.entry?.haproxy?.rules?.length || 0})`,
      children: renderManagedConfigSection('haproxy'),
    }] : []),
    {
      key: 'haproxy-nodes',
      label: `节点 (${haProxyForwardNodes.length})`,
      children: haProxyForwardNodes.length ? (
        renderNodeClientHierarchySections(
          haProxyForwardNodes,
          haProxyFilteredClients,
          visibleHAProxyNodeColumns as ColumnsType<XUINodeView>,
          selectedAgent.customer_display_name || selectedAgent.agent_name || selectedAgent.agent_id,
          selectedAgentId,
          selectedNodeAnchor,
          restrictedView ? 760 : 1120,
          'haproxy-clients',
          onActiveTabChange,
          onClientSearchChange,
        )
      ) : (
        <Empty description="暂无通过 HAProxy 转发命中的节点" />
      ),
    },
    {
      key: 'haproxy-clients',
      label: `客户端 (${haProxyForwardClients.length})`,
      children: haProxyForwardClients.length ? (
        <Space direction="vertical" style={{ width: '100%' }} size="middle">
          <Input.Search allowClear style={{ minWidth: 280 }} placeholder="按邮箱、备注、节点标签筛选 HAProxy 命中客户端" value={clientSearch} onChange={(event) => onClientSearchChange(event.target.value)} />
          <Table
            rowKey={(record) => `${record.realm_target_agent_id || ''}-${record.inbound_id}-${record.inbound_tag || ''}-${record.email || record.comment || record.sub_id || ''}`}
            columns={visibleHAProxyClientColumns}
            dataSource={haProxyFilteredClients}
            pagination={false}
            scroll={{ x: restrictedView ? 760 : 1120 }}
          />
        </Space>
      ) : (
        <Empty description="暂无通过 HAProxy 转发命中的客户端" />
      ),
    },
  ] : []

  const portPolicyTabs: TabsProps['items'] = featureEnabled.port_policy && canManageConfig ? [{
    key: 'network-policy',
    label: `端口策略 (${portPolicyCount})`,
    children: renderManagedConfigSection('network'),
  }] : []

  const supportTabs: TabsProps['items'] = [
    ...(!restrictedView ? [{
      key: 'logs',
      label: `日志 (${agentLogs?.logs.length || 0})`,
      children: (
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
          {agentLogs?.last_collection_err ? <Alert type="warning" showIcon message="最近一次 x-ui 采集异常" description={agentLogs.last_collection_err} /> : null}
          {agentLogsError ? <Alert type="error" showIcon message={agentLogsError} /> : null}
          <Space wrap>
            <Button icon={<ReloadOutlined />} disabled={!selectedAgentId} loading={agentLogsLoading} onClick={onOpenLogs}>刷新日志</Button>
            <AdminHelpHint title="当前显示 client 最近一次上报附带的异常日志" />
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
      children: certificateDomainPanel,
    }] : []),
    ...(canManageConfig ? [
      {
        key: 'config',
        label: (
          <Space size={6}>
            <SettingOutlined />
            <span>基础信息</span>
          </Space>
        ),
        children: renderManagedConfigSection('basic'),
      },
      {
        key: 'config-audits',
        label: `配置记录 (${configAudits.length})`,
        children: renderManagedConfigSection('audit'),
      },
    ] : []),
  ]

  const detailTabs: TabsProps['items'] = [
    overviewTab,
    ...xuiTabs,
    ...realmTabs,
    ...haProxyTabs,
    ...natTabs,
    ...portPolicyTabs,
    ...supportTabs,
  ]
  const effectiveActiveTabKey = detailTabs.some((item) => item?.key === activeTabKey) ? activeTabKey : 'overview'
  const canShowXUIControls = !restrictedView && featureEnabled.xui

  return (
    <>
      {featureEnabled.xui && overviewError ? (
        <Alert className="surface-card alert-card" type="warning" showIcon message="x-ui 概览暂不可用" description={overviewError} />
      ) : null}

      <Card id="agent-detail-panel" className="surface-card tabs-card" bordered={false}>
        <div className="selected-agent-toolbar">
          <div>
            <Text type="secondary">当前 Client</Text>
            <Title level={4}>{selectedAgent.agent_name || selectedAgent.agent_id}</Title>
          </div>
          <Space wrap>
            {canShowXUIControls && selectedAgent.summary.last_collection_err ? (
              <Tag color="orange" style={{ cursor: 'pointer' }} onClick={onOpenLogs}>x-ui 异常</Tag>
            ) : null}
            <Button icon={<ArrowLeftOutlined />} onClick={onBackToList}>返回 Client 列表</Button>
            {canShowXUIControls ? <Button disabled={!canOpenXUI} onClick={onOpenXUI}>打开 x-ui 面板</Button> : null}
            {!restrictedView ? <Button icon={<ReloadOutlined />} loading={currentAgentLoading} onClick={onRefreshCurrentAgent}>立即获取 Client 信息</Button> : null}
            {!restrictedView ? <Button danger onClick={openRealtimeTerminal}>实时 TTY</Button> : null}
            {!restrictedView ? <Button loading={remoteCommandLoading} onClick={openRemoteCommand}>单次命令</Button> : null}
            {canShowXUIControls ? (
              <Popconfirm
                title="升级 3x-ui？"
                description="将通过在线 Client 执行 3x-ui 官方 update.sh 升级脚本，过程中可能短暂影响 x-ui / Xray。"
                okText="升级"
                cancelText="取消"
                onConfirm={onUpdate3XUI}
              >
                <Button loading={xuiUpdateLoading}>升级 3x-ui</Button>
              </Popconfirm>
            ) : null}
            {canShowXUIControls ? (
              <Popconfirm
                title="重启 x-ui / Xray？"
                description="将通过在线 Client 的 WebSocket 执行 x-ui 服务重启；失败日志会写入操作记录。"
                okText="重启"
                cancelText="取消"
                onConfirm={onRestartXUI}
              >
                <Button danger loading={xuiRestartLoading}>重启 x-ui / Xray</Button>
              </Popconfirm>
            ) : null}
            {!restrictedView && canManageConfig ? (
              <Button
                icon={<SwapOutlined />}
                loading={agentReplaceLoading}
                onClick={() => {
                  setReplacementAgentID('')
                  setReplacementModalOpen(true)
                }}
              >
                替换 Client
              </Button>
            ) : null}
            {!restrictedView && canManageConfig ? (
              <Popconfirm
                title="删除这个 VPS / Client？"
                description="会从后台移除该 VPS 的配置、快照、授权和操作记录；如果远端 Client 当前在线，会同时下发停止服务并关闭开机自启。"
                okText="删除"
                cancelText="取消"
                okButtonProps={{ danger: true }}
                onConfirm={onDeleteCurrentAgent}
              >
                <Button danger loading={agentDeleteLoading}>删除 VPS</Button>
              </Popconfirm>
            ) : null}
          </Space>
        </div>
        {restrictedView ? (
          <div className="selected-agent-toolbar area-agent-tags-toolbar">
            <div>
              <Text type="secondary">区域账号私有标签</Text>
              <div className="muted-line">标签只对当前区域账号生效，不会影响 Admin 或其他区域账号。</div>
            </div>
            <Space wrap>
              <Select
                mode="tags"
                allowClear
                style={{ minWidth: 280 }}
                placeholder="输入并回车添加标签"
                value={managedConfig?.tags || selectedAgent.tags || []}
                options={mergeTagOptions(tagOptions, managedConfig?.tags || selectedAgent.tags || []).map((tag) => ({ value: tag, label: tag }))}
                onChange={(values) => onTagsChange(mergeTagOptions([], values))}
              />
              <Button type="primary" loading={tagSaving} onClick={() => onSaveAreaTags?.(managedConfig?.tags || selectedAgent.tags || [])}>保存标签</Button>
            </Space>
          </div>
        ) : null}
        <Tabs
          activeKey={effectiveActiveTabKey}
          onChange={onActiveTabChange}
          items={detailTabs}
        />
      </Card>
      {clientConfigEditor ? <XUIClientConfigModal key={`${selectedAgentId}:${billingKeyForClient(clientConfigEditor)}`} agentID={selectedAgentId} record={overview?.clients.find(item => item.inbound_id === clientConfigEditor.inbound_id && item.inbound_tag === clientConfigEditor.inbound_tag && item.email === clientConfigEditor.email && item.realm_target_agent_id === clientConfigEditor.realm_target_agent_id) || clientConfigEditor}
        billing={findClientBilling(savedClientBillings || managedConfig?.renewal?.client_billings, clientConfigEditor)}
        initialSave={overview?.client_config_saves?.find(item => item.client_id ? item.client_id === clientConfigEditor.client_id : item.inbound_id === clientConfigEditor.inbound_id && item.inbound_tag === clientConfigEditor.inbound_tag && item.email === clientConfigEditor.email)}
        onClose={() => setClientConfigEditor(null)} onSaved={save => onClientConfigSaved(clientConfigEditor, save)} onUnauthorized={onClientConfigUnauthorized} /> : null}
      {routingEditor && overview ? <XUIRoutingRuleEditor key={`${selectedAgentId}:${routingEditor.rule.index}:${routingEditor.fingerprint}`} record={routingEditor.rule} fingerprint={routingEditor.fingerprint} currentFingerprint={routingFingerprint} overview={overview} actions={xuiActions} busy={routingDeleteBusy} onSave={submitRoutingUpdate} onClose={() => setRoutingEditor(null)} /> : null}
      <Modal className="xui-reference-modal" title={`关联路由 · ${referenceOutboundTag || ''}`} open={referenceOutboundTag !== null} width={800} onCancel={() => setReferenceOutboundTag(null)} footer={<Button onClick={() => setReferenceOutboundTag(null)}>关闭</Button>}>
        <Typography.Paragraph type="secondary">按配置引用展示，包含停用或无效规则；均衡器关联表示候选出站，不代表当前流量命中。</Typography.Paragraph>
        <div className="outbound-reference-list">
          {(outboundRuleReferences.get(referenceOutboundTag || '') || []).map(({ rule, direct, balancers }) => <section key={rule.index} className="outbound-reference-item">
            <div className="outbound-reference-head"><Space wrap><Tag color="blue">R{rule.index}</Tag>{direct ? <Tag>直接关联</Tag> : null}{balancers.map(tag => <Tag color="magenta" key={tag}>经均衡器 {tag}</Tag>)}</Space>
              <Button size="small" disabled={rule.index <= 0} onClick={() => { setReferenceOutboundTag(null); onJumpRule(rule.index) }}>查看规则</Button>
            </div>
            {renderRoutingValidity(rule)}
            <div className="routing-rule-card-field"><Text type="secondary">入口匹配</Text>{renderRoutingMatchers(rule)}</div>
            <div className="routing-rule-card-summary">{rule.summary || summarizeRule(rule)}</div>
          </section>)}
          {!outboundRuleReferences.get(referenceOutboundTag || '')?.length ? <Empty description="当前没有关联路由规则" /> : null}
        </div>
      </Modal>
      <Modal
        className="xui-delete-modal"
        title={`删除 ${outboundDeleteDraft?.outbounds.length || 0} 个出站？`}
        open={outboundDeleteDraft !== null}
        okText="确认删除"
        cancelText="取消"
        okButtonProps={{ danger: true, disabled: outboundDeleteBusy || outboundDeleteDraft?.config_fingerprint !== outboundFingerprint }}
        confirmLoading={outboundDeleteLoading}
        closable={!outboundDeleteLoading}
        maskClosable={!outboundDeleteLoading}
        onCancel={() => { if (!outboundDeleteLoading) setOutboundDeleteDraft(null) }}
        onOk={() => void submitOutboundDeletion()}
      >
        <Space wrap>{outboundDeleteDraft?.outbounds.map(outbound => <Tag key={outbound.tag}>{outbound.tag}</Tag>)}</Space>
        <p>将一次保存所选出站的删除并回读核验，随后重载 Xray，可能短暂影响连接。不会删除客户端或路由规则。此操作不能直接撤销。</p>
        {outboundDeleteDraft?.config_fingerprint !== outboundFingerprint ? <Alert type="warning" showIcon message="Xray 配置已变化，请取消并重新选择。" /> : null}
      </Modal>
      <Modal
        className="xui-delete-modal"
        title={`删除 ${routingDeleteDraft?.rules.length || 0} 条路由规则？`}
        open={routingDeleteDraft !== null}
        okText="确认删除"
        cancelText="取消"
        okButtonProps={{ danger: true, disabled: routingDeleteBusy || routingDeleteDraft?.rules_fingerprint !== routingFingerprint }}
        confirmLoading={routingRuleDeleteLoading}
        closable={!routingRuleDeleteLoading}
        maskClosable={!routingRuleDeleteLoading}
        onCancel={() => { if (!routingRuleDeleteLoading) setRoutingDeleteDraft(null) }}
        onOk={() => void submitRoutingRuleDeletion()}
      >
        <p>选中的规则：{routingDeleteDraft?.rules.map((rule) => `R${rule.index}`).join('、')}</p>
        <p>将逐条移除并统一保存，随后重载 Xray，可能短暂影响连接。只删除路由规则，不删除客户端或出站。此操作不能直接撤销。</p>
        {routingDeleteDraft?.rules_fingerprint !== routingFingerprint ? <Alert type="warning" showIcon message="路由规则已变化，请取消并重新选择。" /> : null}
      </Modal>
      <Modal
        title="替换 Client"
        open={replacementModalOpen}
        okText="确认替换"
        cancelText="取消"
        confirmLoading={agentReplaceLoading}
        okButtonProps={{ disabled: !replacementAgentID }}
        onOk={() => void submitAgentReplacement()}
        onCancel={() => {
          if (!agentReplaceLoading) {
            setReplacementModalOpen(false)
            setReplacementAgentID('')
          }
        }}
        destroyOnClose
      >
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
          <Alert
            type="warning"
            showIcon
            message={`将 ${selectedAgent.agent_name || selectedAgent.agent_id} 的关联关系迁移到新 Client`}
            description="会迁移区域账号权限、节点/客户端/转发端口授权、出站授权、区域私有标签、下属用户授权，以及其他 Client 中引用旧 Client 的 Realm/HAProxy 目标。"
          />
          <div>
            <Text type="secondary">新 Client</Text>
            <Select
              showSearch
              optionFilterProp="label"
              style={{ width: '100%', marginTop: 6 }}
              placeholder="选择用于接替的 Client"
              value={replacementAgentID || undefined}
              options={replacementAgents.map((agent) => ({
                value: agent.agent_id,
                label: `${agent.agent_name || agent.agent_id} (${agent.agent_id})`,
              }))}
              onChange={setReplacementAgentID}
            />
          </div>
          <Alert
            type="info"
            showIcon
            message="新 Client 的 x-ui、Realm、HAProxy、续费和运行配置不会被覆盖；旧 Client 也不会自动删除。"
          />
        </Space>
      </Modal>
      <Modal
        title="实时 TTY"
        open={terminalOpen}
        footer={null}
        onCancel={() => setTerminalOpen(false)}
        width={terminalExpanded ? 'min(1500px, 98vw)' : 'min(1100px, 96vw)'}
        destroyOnClose
      >
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
          <Alert
            type="warning"
            showIcon
            message="这是直接连到 Client 服务权限的实时终端"
            description="Linux 使用 PTY，会以 vpsmonitor-client 服务用户打开交互式 shell；Windows 使用管理员服务权限下的 PowerShell/CMD 管道。请只在可信设备上操作。"
          />
          <Space wrap>
            <Text type="secondary">Shell</Text>
            <Select
              style={{ width: 190 }}
              value={terminalShell}
              options={[
                { value: 'bash', label: 'Linux bash' },
                { value: 'sh', label: 'Linux sh' },
                { value: 'powershell', label: 'Windows PowerShell' },
                { value: 'cmd', label: 'Windows CMD' },
                { value: 'pwsh', label: 'PowerShell Core' },
              ]}
              onChange={setTerminalShell}
            />
            <Text type="secondary">切换 Shell 会重新建立终端连接</Text>
            <Space.Compact>
              <Button onClick={() => zoomTerminal(-1)} disabled={terminalFontSize <= 10}>
                缩小
              </Button>
              <Button disabled>{terminalFontSize}px</Button>
              <Button onClick={() => zoomTerminal(1)} disabled={terminalFontSize >= 22}>
                放大
              </Button>
            </Space.Compact>
            <Button onClick={resetTerminalZoom} disabled={terminalFontSize === 13 && !terminalExpanded}>
              重置缩放
            </Button>
            <Button onClick={() => setTerminalExpanded((value) => !value)}>
              {terminalExpanded ? '普通窗口' : '放大窗口'}
            </Button>
          </Space>
          <RemoteTTYTerminal agentID={selectedAgentId} shell={terminalShell} active={terminalOpen} fontSize={terminalFontSize} expanded={terminalExpanded} />
        </Space>
      </Modal>
      <Modal
        title="后台命令行"
        open={remoteCommandOpen}
        okText="下发执行"
        cancelText="取消"
        confirmLoading={remoteCommandLoading}
        okButtonProps={{ danger: true, disabled: !remoteCommand.trim() }}
        onOk={submitRemoteCommand}
        onCancel={() => setRemoteCommandOpen(false)}
        destroyOnClose
      >
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
          <Alert
            type="warning"
            showIcon
            message="命令会直接通过 Client 执行"
            description="Linux 通常以 vpsmonitor-client 服务用户执行（安装为 root 时就是 root 权限）；Windows 取决于 Client 服务是否以管理员权限运行。命令和输出会记录在 x-ui 操作记录中。"
          />
          <Space wrap>
            <div>
              <Text type="secondary">Shell</Text>
              <Select
                style={{ width: 170, display: 'block', marginTop: 6 }}
                value={remoteShell}
                options={[
                  { value: 'bash', label: 'Linux bash' },
                  { value: 'sh', label: 'Linux sh' },
                  { value: 'powershell', label: 'Windows PowerShell' },
                  { value: 'cmd', label: 'Windows CMD' },
                  { value: 'pwsh', label: 'PowerShell Core' },
                ]}
                onChange={setRemoteShell}
              />
            </div>
            <div>
              <Text type="secondary">超时时间（秒）</Text>
              <InputNumber
                min={1}
                max={600}
                style={{ width: 140, display: 'block', marginTop: 6 }}
                value={remoteTimeout}
                onChange={(value) => setRemoteTimeout(Number(value || 120))}
              />
            </div>
          </Space>
          <Input.TextArea
            autoSize={{ minRows: 6, maxRows: 12 }}
            placeholder="例如：systemctl status x-ui --no-pager"
            value={remoteCommand}
            onChange={(event) => setRemoteCommand(event.target.value)}
          />
        </Space>
      </Modal>
      <Modal
        title={commandOutputAction?.kind === 'update_3xui' ? '3x-ui 升级输出' : '远程命令输出'}
        open={Boolean(commandOutputAction)}
        footer={<Button onClick={() => setCommandOutputAction(null)}>关闭</Button>}
        onCancel={() => setCommandOutputAction(null)}
        width={860}
      >
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
          <div>
            <Text type="secondary">命令</Text>
            <pre className="command-output-block">{commandText || '-'}</pre>
          </div>
          <Space wrap>
            <Tag color={commandOutputAction?.status === 'succeeded' ? 'success' : 'error'}>{actionStatusLabel(commandOutputAction?.status || '')}</Tag>
            <Tag>Shell {String(commandResult.shell || commandPayload.shell || '-')}</Tag>
            <Tag>退出码 {String(commandResult.exit_code ?? '-')}</Tag>
            <Tag>执行用户 {String(commandResult.run_as || commandResult.uid || '-')}</Tag>
            <Tag>耗时 {String(commandResult.duration || '-')}</Tag>
          </Space>
          <div>
            <Text type="secondary">STDOUT</Text>
            <pre className="command-output-block">{stdoutText || '-'}</pre>
          </div>
          <div>
            <Text type="secondary">STDERR / 错误</Text>
            <pre className="command-output-block">{stderrText || '-'}</pre>
          </div>
        </Space>
      </Modal>
    </>
  )
}
