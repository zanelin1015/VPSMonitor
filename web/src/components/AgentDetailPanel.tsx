import { useEffect, useRef, useState } from 'react'
import { Alert, Badge, Button, Card, Empty, Input, InputNumber, Modal, Popconfirm, Select, Space, Switch, Table, Tabs, Tag, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { ReloadOutlined, SettingOutlined } from '@ant-design/icons'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'

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
import { clientTrafficTotal, formatBytes } from '../lib/traffic'
import {
  actionKindLabel,
  actionStatusColor,
  actionStatusLabel,
  buildAgentTerminalURL,
  dateInputToExpiryMillis,
  defaultClientBilling,
  findClientBilling,
  findOutboundLinkedClient,
  formatDateInputFromMillis,
  formatDateTime,
  formatExpiryTime,
  formatRelativeTime,
  hasSelectedTag,
  isClientOnline,
  mergeTagOptions,
  nodeElementId,
  outboundElementId,
  parseAddressInput,
  ruleElementId,
  shortJSON,
  summarizeRule,
} from '../lib/appHelpers'
import { renderGlobalOverviewPanel } from './DashboardTopologyPanels'
import { ManagedConfigPanel as renderManagedConfigPanel } from './ManagedConfigPanel'
import { RouteBadge } from './RouteBadge'

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
  agentDeleteLoading: boolean
  canOpenXUI: boolean
  canManageConfig: boolean
  restrictedView?: boolean
  currentAgentLoading: boolean
  remoteCommandLoading: boolean
  xuiRestartLoading: boolean
  xuiUpdateLoading: boolean
  onActiveTabChange: (key: string) => void
  onClientSearchChange: (value: string) => void
  onCopyImportURL: (client: XUIClientView) => void
  onCreateRoutingAction: () => void
  onCreateNodeClientAction: (node: XUINodeView) => void
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
  onRefreshCurrentAgent: () => void
  onRestartXUI: () => void
  onUpdate3XUI: () => void
  onExecuteRemoteCommand: (command: string, shell: string, timeoutSeconds: number) => void
  onRefreshXUIActions: () => void
  onRenewalChange: (patch: Partial<VPSRenewalConfig>) => void
  onReturnHome: () => void
  onSaveClientBilling: (record: XUIClientView) => void
  onSaveManagedConfigSection: (section: ConfigSectionKey) => void
  onSelectTag: (tag: string) => void
  onTagsChange: (values: string[]) => void
  onSaveAreaTags?: (values: string[]) => void
  onUpdateClientBillingDraft: (record: XUIClientView, patch: Partial<XUIClientBillingConfig>) => void
  onXUIChange: (patch: Partial<XUIConfig>) => void
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
    agentDeleteLoading,
    canOpenXUI,
    canManageConfig,
    restrictedView = false,
    currentAgentLoading,
    remoteCommandLoading,
    xuiRestartLoading,
    xuiUpdateLoading,
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
    onRefreshCurrentAgent,
    onRestartXUI,
    onUpdate3XUI,
    onExecuteRemoteCommand,
    onRefreshXUIActions,
    onRenewalChange,
    onReturnHome,
    onSaveClientBilling,
    onSaveManagedConfigSection,
    onSelectTag,
    onTagsChange,
    onSaveAreaTags,
    onUpdateClientBillingDraft,
    onXUIChange,
  } = props
  const [remoteCommandOpen, setRemoteCommandOpen] = useState(false)
  const [remoteCommand, setRemoteCommand] = useState('')
  const [remoteShell, setRemoteShell] = useState('bash')
  const [remoteTimeout, setRemoteTimeout] = useState(120)
  const xuiClientActionKey = (record: XUIClientView) => [record.inbound_id, record.inbound_tag || '', record.email || '', record.auth_uuid || record.auth_password || ''].join(':')
  const [commandOutputAction, setCommandOutputAction] = useState<XUIAction | null>(null)
  const [terminalOpen, setTerminalOpen] = useState(false)
  const [terminalShell, setTerminalShell] = useState(defaultTerminalShell(selectedAgent.client_os))
  const [terminalFontSize, setTerminalFontSize] = useState(13)
  const [terminalExpanded, setTerminalExpanded] = useState(false)
  const currentAgentLinks = filteredTagLinks.filter((link) => link.source.agent_id === selectedAgentId || link.target.agent_id === selectedAgentId)
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

  function submitRemoteCommand() {
    const command = remoteCommand.trim()
    if (!command) {
      return
    }
    onExecuteRemoteCommand(command, remoteShell, remoteTimeout)
    setRemoteCommandOpen(false)
  }

  function openRealtimeTerminal() {
    setTerminalShell(defaultTerminalShell(selectedAgent.client_os))
    setTerminalOpen(true)
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
          <Button size="small" type="link" disabled={!canManageConfig} onClick={() => onCreateNodeClientAction(record)}>
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
            <span>{restrictedView ? '已用' : '总'} {formatBytes(total)}</span>
            {!restrictedView ? <span>上传 {formatBytes(up)}</span> : null}
            {!restrictedView ? <span>下载 {formatBytes(down)}</span> : null}
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
            <Select
              size="small"
              style={{ width: 78 }}
              disabled={!canManageConfig}
              value={billing.revenue_cycle || 'month'}
              options={[
                { value: 'month', label: '月' },
                { value: 'quarter', label: '季' },
                { value: 'year', label: '年' },
              ]}
              onChange={(value) => onUpdateClientBillingDraft(record, { revenue_cycle: value as 'month' | 'quarter' | 'year' })}
            />
            <Button size="small" type="primary" disabled={!canManageConfig} loading={configSavingSection === 'renewal'} onClick={() => onSaveClientBilling(record)}>
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
                disabled={!canManageConfig}
                value={formatDateInputFromMillis(effectiveExpiry)}
                onChange={(event) => onUpdateClientBillingDraft(record, { expire_time: dateInputToExpiryMillis(event.target.value) })}
              />
              <Select
                size="small"
                style={{ width: 78 }}
                disabled={!canManageConfig}
                value={billing.expire_cycle || 'month'}
                options={[
                  { value: 'month', label: '月' },
                  { value: 'quarter', label: '季' },
                  { value: 'year', label: '年' },
                ]}
                onChange={(value) => onUpdateClientBillingDraft(record, { expire_cycle: value as 'month' | 'quarter' | 'year', expire_time: effectiveExpiry })}
              />
              <Switch
                size="small"
                disabled={!canManageConfig}
                checked={Boolean(billing.expire_auto_renew)}
                onChange={(checked: boolean) => onUpdateClientBillingDraft(record, { expire_auto_renew: checked, expire_time: effectiveExpiry })}
              />
              <Text type="secondary">周期刷新</Text>
              <Button size="small" type="primary" disabled={!canManageConfig} loading={configSavingSection === 'renewal'} onClick={() => onSaveClientBilling(record)}>
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
      width: 110,
      render: (_, record) => (
        <Popconfirm
          title="删除这个 Client？"
          description={`将从 x-ui 入站 ${record.inbound_remark || record.inbound_tag || record.inbound_id} 删除 ${record.email || '该客户端'}，删除后会重启 Xray。`}
          okText="删除"
          cancelText="取消"
          okButtonProps={{ danger: true }}
          onConfirm={() => onDeleteXUIClient(record)}
        >
          <Button
            size="small"
            danger
            disabled={!canManageConfig}
            loading={xuiClientDeleteLoadingKey === xuiClientActionKey(record)}
          >
            删除
          </Button>
        </Popconfirm>
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
    ? clientColumns.filter((column) => !['protocol', 'status', 'billing', 'expiry', 'last_online', 'actions', 'route'].includes(String(column.key || '')))
    : clientColumns

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
  ]

  return (
    <>
      {overviewError ? (
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
            {!restrictedView && selectedAgent.summary.last_collection_err ? (
              <Tag color="orange" style={{ cursor: 'pointer' }} onClick={onOpenLogs}>x-ui 异常</Tag>
            ) : null}
            {!restrictedView ? <Button onClick={onOpenLogs}>查看日志</Button> : null}
            {!restrictedView ? <Button disabled={!canOpenXUI} onClick={onOpenXUI}>打开 x-ui 面板</Button> : null}
            {!restrictedView ? <Button icon={<ReloadOutlined />} loading={currentAgentLoading} onClick={onRefreshCurrentAgent}>立即获取 Client 信息</Button> : null}
            {!restrictedView ? <Button danger onClick={openRealtimeTerminal}>实时 TTY</Button> : null}
            {!restrictedView ? <Button loading={remoteCommandLoading} onClick={() => setRemoteCommandOpen(true)}>单次命令</Button> : null}
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
            {!restrictedView ? (
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
                description="会从后台移除该 VPS 的配置、快照、授权和操作记录；不会卸载远端 VPS 上的 Client 服务，远端服务仍运行时可能会重新注册。"
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
          activeKey={activeTabKey}
          onChange={onActiveTabChange}
          items={[
            {
              key: 'overview',
              label: `总览 (${selectedAgentId ? 1 : 0})`,
              children: renderGlobalOverviewPanel({
                dashboardView: currentAgentDashboardView,
                selectedTag,
                links: currentAgentLinks,
                onSelectTag: (value) => onSelectTag(value),
                scopeAgentName: selectedAgent.agent_name || selectedAgent.agent_id,
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
                    message="这里现在通过 WS 实时下发 x-ui 指令"
                    description="入站节点请直接通过 x-ui 面板手动新增；中心会把其它 client 的节点导入为当前 client 的出站，并通过在线 Client 实时执行和回传结果。Client 不在线时会保留任务等待轮询。"
                  />
                  <Space wrap>
                    <Button type="primary" disabled={!selectedAgentId} onClick={onCreateRoutingAction}>新增操作</Button>
                    {!restrictedView ? <Button danger onClick={openRealtimeTerminal}>实时 TTY</Button> : null}
                    {!restrictedView ? <Button loading={remoteCommandLoading} onClick={() => setRemoteCommandOpen(true)}>单次命令</Button> : null}
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
              children: overview ? (
                <Table rowKey={(record) => record.id} columns={certificateColumns} dataSource={overview.certificates} pagination={{ pageSize: 8, hideOnSinglePage: true }} scroll={{ x: 1200 }} />
              ) : (
                <Empty description="暂无本机证书数据" />
              ),
            }] : []),
            ...(canManageConfig ? [{
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
                certificates: overview?.certificates || [],
                configLoading,
                configSavingSection,
                configError,
                onSave: onSaveManagedConfigSection,
                onAgentNameChange: onManagedConfigAgentNameChange,
                onCustomerDisplayNameChange: onManagedConfigCustomerDisplayNameChange,
                onSortOrderChange: onManagedConfigSortOrderChange,
                tagOptions,
                newTagName,
                tagSaving,
                onNewTagNameChange,
                onCreateTag,
                onTagsChange: (values) => {
                  const tags = mergeTagOptions([], values)
                  onTagsChange(tags)
                },
                onRenewalChange,
                entryAddressInputText,
                onEntryAddressesTextChange,
                onEntryChange,
                onXUIChange,
                configAudits,
                configAuditsLoading,
                currencyOptions,
              }),
            }] : []),
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
                  <Input.Search allowClear placeholder="按邮箱、备注、节点标签筛选客户端" value={clientSearch} onChange={(event) => onClientSearchChange(event.target.value)} />
                  <Table rowKey={(record) => `${record.inbound_tag}-${record.email}`} columns={visibleClientColumns} dataSource={filteredClients} pagination={{ pageSize: 12, hideOnSinglePage: true }} scroll={{ x: restrictedView ? 980 : 1780 }} />
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
          ]}
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

interface TerminalWSMessage {
  type: string
  session_id?: string
  data?: string
  error?: string
  exit_code?: number
  rows?: number
  cols?: number
  shell?: string
}

function defaultTerminalShell(clientOS?: string): string {
  return String(clientOS || '').toLowerCase().includes('windows') ? 'powershell' : 'bash'
}

function RemoteTTYTerminal(props: { agentID: string; shell: string; active: boolean; fontSize: number; expanded: boolean }) {
  const { agentID, shell, active, fontSize, expanded } = props
  const containerRef = useRef<HTMLDivElement | null>(null)
  const terminalRef = useRef<Terminal | null>(null)
  const fitAddonRef = useRef<FitAddon | null>(null)
  const sendRef = useRef<(message: TerminalWSMessage) => void>(() => undefined)

  useEffect(() => {
    const terminal = terminalRef.current
    const fitAddon = fitAddonRef.current
    if (!active || !terminal || !fitAddon) {
      return
    }
    terminal.options.fontSize = fontSize
    window.setTimeout(() => {
      fitAddon.fit()
      sendRef.current({ type: 'resize', cols: terminal.cols, rows: terminal.rows })
      terminal.focus()
    }, 0)
  }, [active, expanded, fontSize])

  useEffect(() => {
    if (!active || !agentID || !containerRef.current) {
      return undefined
    }
    const terminal = new Terminal({
      cursorBlink: true,
      convertEol: true,
      fontFamily: '"JetBrains Mono", "SFMono-Regular", Consolas, monospace',
      fontSize,
      rows: 36,
      scrollback: 5000,
      theme: {
        background: '#07111f',
        foreground: '#dbeafe',
        cursor: '#f97316',
        selectionBackground: '#334155',
      },
    })
    const fitAddon = new FitAddon()
    terminalRef.current = terminal
    fitAddonRef.current = fitAddon
    terminal.loadAddon(fitAddon)
    terminal.open(containerRef.current)
    fitAddon.fit()
    terminal.focus()
    terminal.writeln('Connecting to VPSMonitor Client realtime TTY...')

    let closed = false
    let currentSessionID = ''
    const socket = new WebSocket(buildAgentTerminalURL(agentID, shell, terminal.cols, terminal.rows))
    const send = (message: TerminalWSMessage) => {
      if (socket.readyState === WebSocket.OPEN) {
        socket.send(JSON.stringify({ ...message, session_id: currentSessionID || message.session_id }))
      }
    }
    sendRef.current = send
    const dataDisposable = terminal.onData((data) => send({ type: 'input', data: normalizeTerminalInput(data, shell) }))
    const resizeDisposable = terminal.onResize((size) => send({ type: 'resize', cols: size.cols, rows: size.rows }))
    const resizeWindow = () => {
      fitAddon.fit()
      send({ type: 'resize', cols: terminal.cols, rows: terminal.rows })
    }
    window.addEventListener('resize', resizeWindow)

    socket.onopen = () => {
      terminal.writeln('\r\nConnected, waiting for remote shell...')
    }
    socket.onmessage = (event) => {
      const message = JSON.parse(event.data) as TerminalWSMessage
      if (message.session_id) {
        currentSessionID = message.session_id
      }
      switch (message.type) {
        case 'terminal_opened':
          terminal.writeln(`\r\nTTY opened (${message.shell || shell}, ${message.cols || terminal.cols}x${message.rows || terminal.rows})\r\n`)
          break
        case 'terminal_output':
          terminal.write(message.data || '')
          break
        case 'terminal_error':
          terminal.writeln(`\r\n[error] ${message.error || 'unknown error'}\r\n`)
          break
        case 'terminal_closed':
          terminal.writeln(`\r\n[closed] exit=${message.exit_code ?? '-'} ${message.error || ''}\r\n`)
          break
        default:
          break
      }
    }
    socket.onerror = () => {
      terminal.writeln('\r\n[error] WebSocket connection failed\r\n')
    }
    socket.onclose = () => {
      if (!closed) {
        terminal.writeln('\r\n[disconnected]\r\n')
      }
    }

    window.setTimeout(resizeWindow, 80)
    return () => {
      closed = true
      send({ type: 'close' })
      window.removeEventListener('resize', resizeWindow)
      dataDisposable.dispose()
      resizeDisposable.dispose()
      socket.close()
      terminal.dispose()
      terminalRef.current = null
      fitAddonRef.current = null
      sendRef.current = () => undefined
    }
  }, [active, agentID, shell])

  return <div ref={containerRef} className={`remote-tty-terminal${expanded ? ' remote-tty-terminal-expanded' : ''}`} />
}

function normalizeTerminalInput(data: string, shell: string): string {
  if (data !== '\r') {
    return data
  }
  const normalizedShell = String(shell || '').toLowerCase()
  return normalizedShell === 'powershell' || normalizedShell === 'pwsh' || normalizedShell === 'cmd' ? '\r\n' : '\n'
}
