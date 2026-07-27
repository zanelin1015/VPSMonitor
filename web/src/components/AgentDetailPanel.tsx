import { useEffect, useState } from 'react'
import { Alert, AutoComplete, Badge, Button, Card, Empty, Input, InputNumber, Modal, Popconfirm, Select, Space, Switch, Table, Tabs, Tag, Tooltip, Typography } from 'antd'
import type { TabsProps } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { ReloadOutlined, SaveOutlined, SettingOutlined } from '@ant-design/icons'

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
  XUIClientView,
  XUIConfig,
  XUILocalCertificate,
  XUINodeView,
  XUIOverview,
  XUIRoutingRuleView,
} from '../types'
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
  formatDateInputFromMillis,
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
import {
  type RealmForwardClientView,
  type RealmForwardNodeView,
  type AgentFeatureKey,
  buildPrimaryDomainOptions,
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
  clientBillingSavingKey: string
  currencyOptions: CurrencyCode[]
  dashboardView: GlobalDashboardView | null
  entryAddressInputText: string
  filteredAgents: DashboardAgentView[]
  filteredClients: XUIClientView[]
  filteredTagLinks: TopologyLinkView[]
  managedConfig: ManagedAgentConfig | null
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
  xuiClientTrafficSavingKey: string
  xuiClientToggleLoadingKey: string
  agentDeleteLoading: boolean
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
  onDeleteXUIClient: (client: XUIClientView) => void
  onSaveXUIClientTrafficLimit: (client: XUIClientView, totalGB: number) => void
  onSetXUIClientEnabled: (client: XUIClientView, enabled: boolean) => void
  onRefreshCurrentAgent: () => void
  onRestartXUI: () => void
  onUpdate3XUI: () => void
  onExecuteRemoteCommand: (command: string, shell: string, timeoutSeconds: number) => void
  onRefreshXUIActions: () => void
  onCopyRealmConfig: (targetAgentID: string) => void
  onRenewalChange: (patch: Partial<VPSRenewalConfig>) => void
  onReturnHome: () => void
  onSaveClientBilling: (record: XUIClientView) => void
  onSaveManagedConfigSection: (section: ConfigSectionKey, draftOverride?: ManagedAgentConfig) => void
  onSavePrimaryDomain: (value: string) => void
  onSelectTag: (tag: string) => void
  onTagsChange: (values: string[]) => void
  onSaveAreaTags?: (values: string[]) => void
  onUpdateClientBillingDraft: (record: XUIClientView, patch: Partial<XUIClientBillingConfig>) => void
  onXUIChange: (patch: Partial<XUIConfig>) => void
  onFeatureChange: (feature: AgentFeatureKey, enabled: boolean) => void
}

function normalizeBillingCycle(cycle?: string): 'month' | 'quarter' | 'year' {
  return cycle === 'quarter' || cycle === 'year' ? cycle : 'month'
}

function billingCycleLabel(cycle?: string): string {
  switch (normalizeBillingCycle(cycle)) {
    case 'quarter':
      return '季'
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
    clientBillingSavingKey,
    currencyOptions,
    dashboardView,
    entryAddressInputText,
    filteredAgents,
    filteredClients,
    filteredTagLinks,
    managedConfig,
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
    xuiClientTrafficSavingKey,
    xuiClientToggleLoadingKey,
    agentDeleteLoading,
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
    onDeleteXUIClient,
    onSaveXUIClientTrafficLimit,
    onSetXUIClientEnabled,
    onRefreshCurrentAgent,
    onRestartXUI,
    onUpdate3XUI,
    onExecuteRemoteCommand,
    onRefreshXUIActions,
    onCopyRealmConfig,
    onRenewalChange,
    onReturnHome,
    onSaveClientBilling,
    onSaveManagedConfigSection,
    onSavePrimaryDomain,
    onSelectTag,
    onTagsChange,
    onSaveAreaTags,
    onUpdateClientBillingDraft,
    onXUIChange,
    onFeatureChange,
  } = props
  const [remoteCommandOpen, setRemoteCommandOpen] = useState(false)
  const [remoteCommand, setRemoteCommand] = useState('')
  const [remoteShell, setRemoteShell] = useState('bash')
  const [remoteTimeout, setRemoteTimeout] = useState(120)
  const xuiClientActionKey = (record: XUIClientView) => [record.inbound_id, record.inbound_tag || '', record.email || '', record.auth_uuid || record.auth_password || ''].join(':')
  const [clientTrafficLimitDrafts, setClientTrafficLimitDrafts] = useState<Record<string, number>>({})
  const [commandOutputAction, setCommandOutputAction] = useState<XUIAction | null>(null)
  const [terminalOpen, setTerminalOpen] = useState(false)
  const [terminalShell, setTerminalShell] = useState(defaultTerminalShell(selectedAgent.client_os, selectedAgent.system_version))
  const [terminalFontSize, setTerminalFontSize] = useState(13)
  const [terminalExpanded, setTerminalExpanded] = useState(false)
  const currentAgentLinks = filteredTagLinks.filter((link) => link.source.agent_id === selectedAgentId || link.target.agent_id === selectedAgentId)
  const currentAgentRealmLinks = currentAgentLinks.filter((link) => link.source.agent_id === selectedAgentId && (link.source.protocol || '').toLowerCase() === 'realm')
  const realmForwardNodes = buildRealmForwardNodes(currentAgentRealmLinks, dashboardView, overview?.nodes || [])
  const realmForwardClients = buildRealmForwardClients(currentAgentRealmLinks, dashboardView, overview?.clients || [])
  const realmFilteredClients = filterRealmForwardClients(realmForwardClients, clientSearch)
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
      key: 'protocol',
      width: 100,
      render: (value?: string) => <Tag color="geekblue">{value || '-'}</Tag>,
    },
    {
      title: '状态',
      key: 'status',
      width: 120,
      render: (_, record) => {
        const accountBasedProxy = isAccountBasedProxyClient(record)
        return (
          <Space wrap size={[6, 6]}>
            <Tag color={record.enabled ? 'success' : 'default'}>{record.enabled ? '启用' : '停用'}</Tag>
            {accountBasedProxy ? <Tag>节点账号</Tag> : (
              <>
                <Tag color={isClientOnline(record.last_online, overview?.reported_at) ? 'processing' : 'default'}>
                  {isClientOnline(record.last_online, overview?.reported_at) ? '在线' : '离线'}
                </Tag>
                <Switch
                  size="small"
                  checked={record.enabled}
                  checkedChildren="开"
                  unCheckedChildren="关"
                  disabled={!canManageConfig && !restrictedView}
                  loading={xuiClientToggleLoadingKey === xuiClientActionKey(record)}
                  onChange={(checked) => onSetXUIClientEnabled(record, checked)}
                />
              </>
            )}
          </Space>
        )
      },
    },
    {
      title: '流量 / 上传 / 下载',
      key: 'traffic',
      width: 240,
      render: (_, record) => {
        const up = Math.max(0, Number(record.up || 0))
        const down = Math.max(0, Number(record.down || 0))
        const total = clientBidirectionalTrafficTotal(record)
        const limit = Math.max(0, Number(record.total_gb || 0))
        const actionKey = xuiClientActionKey(record)
        const limitGB = clientTrafficLimitDrafts[actionKey] ?? limit / (1024 * 1024 * 1024)
        return (
          <div className="client-traffic-cell">
            <span>已用 {formatBytes(total)}{limit > 0 ? ` / 限额 ${formatBytes(limit)}` : ' / 无上限'}</span>
            {!restrictedView ? (
              <Space.Compact size="small" className="client-traffic-limit-editor">
                <InputNumber
                  min={0}
                  precision={2}
                  step={1}
                  addonAfter="GB"
                  disabled={!canManageConfig}
                  value={limitGB}
                  onChange={(value) => setClientTrafficLimitDrafts((current) => ({
                    ...current,
                    [actionKey]: Math.max(0, Number(value || 0)),
                  }))}
                />
                <Tooltip title="同步到 x-ui；0 GB 表示无上限">
                  <Button
                    type="primary"
                    aria-label="保存流量上限"
                    icon={<SaveOutlined />}
                    disabled={!canManageConfig || !record.email}
                    loading={xuiClientTrafficSavingKey === actionKey}
                    onClick={() => onSaveXUIClientTrafficLimit(record, limitGB)}
                  />
                </Tooltip>
              </Space.Compact>
            ) : null}
            {!restrictedView ? <span>上传 {formatBytes(up)}</span> : null}
            {!restrictedView ? <span>下载 {formatBytes(down)}</span> : null}
          </div>
        )
      },
    },
    {
      title: '收费',
      key: 'billing',
      width: 430,
      render: (_, record) => {
        const billing = findClientBilling(managedConfig?.renewal?.client_billings, record) || defaultClientBilling(record)
        const revenueCycle = normalizeBillingCycle(billing.revenue_cycle || billing.expire_cycle)
        const effectiveStart = effectiveClientBillingStartTime(billing, record.expiry_time || 0)
        const saving = clientBillingSavingKey === billingKeyForClient(record)
        return (
          <Space wrap size={[6, 6]}>
            <InputNumber
              size="small"
              min={0}
              precision={2}
              disabled={!canManageConfig}
              style={{ width: 92 }}
              value={billing.revenue_amount || 0}
              onChange={(value) => onUpdateClientBillingDraft(record, { revenue_amount: Number(value || 0) })}
            />
            <Select
              size="small"
              style={{ width: 78 }}
              disabled={!canManageConfig}
              value={billing.revenue_currency || 'CNY'}
              options={REVENUE_CURRENCIES.map((currency) => ({ value: currency, label: currency }))}
              onChange={(value) => onUpdateClientBillingDraft(record, { revenue_currency: value as 'CNY' | 'USDT' })}
            />
            <Space size={4}>
              <Text type="secondary">流量</Text>
              <InputNumber
                size="small"
                min={0.1}
                max={100}
                precision={2}
                step={0.1}
                disabled={!canManageConfig}
                style={{ width: 76 }}
                value={normalizeClientTrafficMultiplier(billing.traffic_multiplier)}
                onChange={(value) => onUpdateClientBillingDraft(record, { traffic_multiplier: Number(value || 1) })}
              />
              <Text type="secondary">倍</Text>
            </Space>
            <Select
              size="small"
              style={{ width: 78 }}
              disabled={!canManageConfig}
              value={revenueCycle}
              options={[
                { value: 'month', label: '月' },
                { value: 'quarter', label: '季' },
                { value: 'year', label: '年' },
              ]}
              onChange={(value) => {
                const nextCycle = value as 'month' | 'quarter' | 'year'
                onUpdateClientBillingDraft(record, {
                  revenue_cycle: nextCycle,
                  ...clientBillingPatchFromStart(effectiveStart, nextCycle),
                })
              }}
            />
            <Button size="small" type="primary" disabled={!canManageConfig} loading={saving} onClick={() => onSaveClientBilling(record)}>
              保存
            </Button>
          </Space>
        )
      },
    },
    {
      title: '开始 / 到期',
      key: 'expiry',
      width: 360,
      render: (_, record) => {
        const billing = findClientBilling(managedConfig?.renewal?.client_billings, record) || defaultClientBilling(record)
        const effectiveStart = effectiveClientBillingStartTime(billing, record.expiry_time || 0)
        const effectiveExpiry = effectiveClientBillingExpiryTime(billing, record.expiry_time || 0)
        const billingCycle = normalizeBillingCycle(billing.revenue_cycle || billing.expire_cycle)
        const saving = clientBillingSavingKey === billingKeyForClient(record)
        return (
          <Space direction="vertical" size={6} className="client-expiry-cell">
            <Text type="secondary">x-ui 当前：{formatExpiryTime(record.expiry_time)}</Text>
            <Text type="secondary">当前周期到期：{formatExpiryTime(effectiveExpiry)}</Text>
            <Space wrap size={[6, 6]}>
              <Input
                size="small"
                type="date"
                style={{ width: 132 }}
                disabled={!canManageConfig}
                value={formatDateInputFromMillis(effectiveStart)}
                onChange={(event) => onUpdateClientBillingDraft(record, clientBillingPatchFromStart(dateInputToStartMillis(event.target.value), billingCycle))}
              />
              <Tag color="blue">按收费周期：{billingCycleLabel(billingCycle)}</Tag>
              <Button size="small" type="primary" disabled={!canManageConfig} loading={saving} onClick={() => onSaveClientBilling(record)}>
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
      key: 'last_online',
      width: 160,
      render: (value?: number) => formatRelativeTime(value),
    },
    {
      title: '导入',
      key: 'import',
      width: 160,
      render: (_, record) => (
        <Space wrap size={[6, 6]}>
          <Button size="small" disabled={!record.import_url} onClick={() => onCopyImportURL(record)}>
            复制
          </Button>
          <Button size="small" disabled={!record.import_url} onClick={() => onOpenImportURL(record)}>
            URL / 二维码
          </Button>
        </Space>
      ),
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
            inbound_id: record.inbound_id,
            inbound_tag: record.inbound_tag || '',
            client_email: record.email || '',
            public_client_name: [selectedAgent.customer_display_name || selectedAgent.agent_name || selectedAgent.agent_id, record.email || record.comment || record.inbound_tag || `Inbound #${record.inbound_id}`].filter(Boolean).join(' - '),
          })}
        >
          授权给用户
        </Button>
      ),
    },
    {
      title: '操作',
      key: 'actions',
      width: 180,
      render: (_, record) => (
        <Space wrap size={[6, 6]}>
          {restrictedView && !isAccountBasedProxyClient(record) ? (
            <Button
              size="small"
              disabled={!canManageConfig && !restrictedView}
              loading={xuiClientToggleLoadingKey === xuiClientActionKey(record)}
              onClick={() => onSetXUIClientEnabled(record, !record.enabled)}
            >
              {record.enabled ? '停用' : '启用'}
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
        </Space>
      ),
    },
    {
      title: '路由指向',
      key: 'route',
      width: 280,
      render: (_, record) => <RouteBadge route={record.route} onJumpOutbound={onJumpOutbound} onJumpRule={onJumpRule} />,
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
          <Tag color={record.enabled ? 'success' : 'default'}>{record.enabled ? '启用' : '停用'}</Tag>
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
    ? realmClientColumns.filter((column) => !['target_agent', 'inbound'].includes(String(column.key || '')))
    : realmClientColumns

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
            <Tag key={`inbound-${record.index}-${tag}`} color="blue">{tag}</Tag>
          ))}
          {record.users?.map((user) => (
            <Tag key={`user-${record.index}-${user}`} color="gold">{user}</Tag>
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
            <Button type="link" className="route-link" onClick={() => onJumpOutbound(record.outbound_tag)}>
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
        description="导出的单节点链接会优先使用这里保存的主域名替代 IP；如果该 VPS 是 Realm 入口机，转发后导出的节点也会使用入口机的主域名和监听端口。SNI、Reality 等目标节点参数保持不变。"
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

  function renderManagedConfigSection(section: 'basic' | 'xui' | 'nat' | 'network' | 'realm' | 'audit') {
    return (
      <ManagedConfigPanel
        selectedAgent={selectedAgent}
        agents={dashboardView?.agents || filteredAgents}
        managedConfig={managedConfig}
        certificates={overview?.certificates || []}
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
        renderNodeClientHierarchySections(
          overview.nodes,
          filteredClients,
          nodeColumns,
          selectedAgent.customer_display_name || selectedAgent.agent_name || selectedAgent.agent_id,
          selectedAgentId,
          selectedNodeAnchor,
          restrictedView ? 800 : 1600,
          'clients',
          onActiveTabChange,
          onClientSearchChange,
        )
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
            rowKey={(record) => `${record.realm_target_agent_id || ''}-${record.inbound_id}-${record.inbound_tag || ''}-${record.email || record.comment || record.sub_id || ''}`}
            columns={visibleClientColumns}
            dataSource={filteredClients}
            pagination={false}
            scroll={{ x: restrictedView ? 900 : 1700 }}
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
            const linkedNodeLabel = linkedClient ? linkedClient.target.inbound_name || linkedClient.target.inbound_tag || String(linkedClient.target.inbound_id) : ''
            return (
              <section
                key={outbound.tag || 'unknown'}
                id={outboundElementId(outbound.tag || 'unknown')}
                className={`outbound-card${selected ? ' selected' : ''}${linkedClient ? ' linked' : ''}`}
                role={linkedClient ? 'button' : undefined}
                tabIndex={linkedClient ? 0 : undefined}
                onClick={() => {
                  if (outbound.tag) {
                    onJumpOutbound(outbound.tag)
                  }
                  if (linkedClient) {
                    onJumpNode(linkedClient.target.agent_id, linkedNodeLabel)
                  }
                }}
                onKeyDown={(event) => {
                  if (!linkedClient || (event.key !== 'Enter' && event.key !== ' ')) {
                    return
                  }
                  event.preventDefault()
                  onJumpNode(linkedClient.target.agent_id, linkedNodeLabel)
                }}
              >
                <div className="outbound-head">
                  <div>
                    <Text className="outbound-tag">{outbound.tag || '-'}</Text>
                    <div className="muted-line">{outbound.protocol || '-'}{outbound.is_default ? ' · 默认出口' : ''}</div>
                  </div>
                  <Space wrap size={[6, 6]}>
                    {outbound.is_default ? <Tag color="success">默认</Tag> : null}
                    {outbound.send_through ? <Tag>sendThrough:{outbound.send_through}</Tag> : null}
                    {linkedClient ? <Tag color="cyan">关联 {linkedClient.target.agent_name || linkedClient.target.agent_id}</Tag> : null}
                  </Space>
                </div>
                <div className="outbound-target">{outbound.target || '当前配置未提供远端地址'}</div>
                {linkedClient ? <div className="muted-line">点击跳转到 {linkedClient.target.agent_name || linkedClient.target.agent_id} / {linkedNodeLabel}</div> : null}
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
        <Table rowKey={(record) => record.index} columns={routingColumns} dataSource={overview.routing_rules} pagination={false} scroll={{ x: 1100 }} rowClassName={(record) => (selectedRuleIndex === record.index ? 'route-row-selected' : '')} />
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
          realmNodeColumns as ColumnsType<XUINodeView>,
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

  const natTabs: TabsProps['items'] = featureEnabled.nat && canManageConfig ? [{
    key: 'entry-nat',
    label: `NAT 映射 (${natMappingCount})`,
    children: renderManagedConfigSection('nat'),
  }] : []

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
            <Button onClick={onReturnHome}>返回首页</Button>
            {canShowXUIControls && selectedAgent.summary.last_collection_err ? (
              <Tag color="orange" style={{ cursor: 'pointer' }} onClick={onOpenLogs}>x-ui 异常</Tag>
            ) : null}
            {!restrictedView ? <Button onClick={onOpenLogs}>查看日志</Button> : null}
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
