import { Alert, Badge, Button, Card, Empty, Input, InputNumber, Select, Space, Switch, Table, Tabs, Tag, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { ReloadOutlined, SettingOutlined } from '@ant-design/icons'

import type {
  AgentEntryConfig,
  AgentLogsResponse,
  ConfigAuditLog,
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
  canOpenXUI: boolean
  currentAgentLoading: boolean
  onActiveTabChange: (key: string) => void
  onClientSearchChange: (value: string) => void
  onCopyImportURL: (client: XUIClientView) => void
  onCreateRoutingAction: () => void
  onCreateTag: () => void
  onEntryAddressesTextChange: (value: string) => void
  onEntryChange: (patch: Partial<AgentEntryConfig>) => void
  onJumpNode: (agentID?: string, nodeLabel?: string) => void
  onJumpOutbound: (tag?: string) => void
  onJumpRule: (index?: number) => void
  onManagedConfigAgentNameChange: (value: string) => void
  onManagedConfigSortOrderChange: (value: number) => void
  onNewTagNameChange: (value: string) => void
  onOpenImportURL: (client: XUIClientView) => void
  onOpenLogs: () => void
  onOpenXUI: () => void
  onRefreshCurrentAgent: () => void
  onRefreshXUIActions: () => void
  onRenewalChange: (patch: Partial<VPSRenewalConfig>) => void
  onReturnHome: () => void
  onSaveClientBilling: (record: XUIClientView) => void
  onSaveManagedConfigSection: (section: ConfigSectionKey) => void
  onSelectTag: (tag: string) => void
  onTagsChange: (values: string[]) => void
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
    canOpenXUI,
    currentAgentLoading,
    onActiveTabChange,
    onClientSearchChange,
    onCopyImportURL,
    onCreateRoutingAction,
    onCreateTag,
    onEntryAddressesTextChange,
    onEntryChange,
    onJumpNode,
    onJumpOutbound,
    onJumpRule,
    onManagedConfigAgentNameChange,
    onManagedConfigSortOrderChange,
    onNewTagNameChange,
    onOpenImportURL,
    onOpenLogs,
    onOpenXUI,
    onRefreshCurrentAgent,
    onRefreshXUIActions,
    onRenewalChange,
    onReturnHome,
    onSaveClientBilling,
    onSaveManagedConfigSection,
    onSelectTag,
    onTagsChange,
    onUpdateClientBillingDraft,
    onXUIChange,
  } = props

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
      render: (value?: number) => <Badge color={value ? '#0f766e' : '#94a3b8'} text={String(value ?? 0)} />,
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
              onChange={(value) => onUpdateClientBillingDraft(record, { revenue_amount: Number(value || 0) })}
            />
            <Select
              size="small"
              style={{ width: 78 }}
              value={billing.revenue_currency || 'CNY'}
              options={REVENUE_CURRENCIES.map((currency) => ({ value: currency, label: currency }))}
              onChange={(value) => onUpdateClientBillingDraft(record, { revenue_currency: value as 'CNY' | 'USDT' })}
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
              onChange={(value) => onUpdateClientBillingDraft(record, { revenue_cycle: value as 'month' | 'quarter' | 'year' })}
            />
            <Button size="small" type="primary" loading={configSavingSection === 'renewal'} onClick={() => onSaveClientBilling(record)}>
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
                onChange={(event) => onUpdateClientBillingDraft(record, { expire_time: dateInputToExpiryMillis(event.target.value) })}
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
                onChange={(value) => onUpdateClientBillingDraft(record, { expire_cycle: value as 'month' | 'quarter' | 'year', expire_time: effectiveExpiry })}
              />
              <Switch
                size="small"
                checked={Boolean(billing.expire_auto_renew)}
                onChange={(checked: boolean) => onUpdateClientBillingDraft(record, { expire_auto_renew: checked, expire_time: effectiveExpiry })}
              />
              <Text type="secondary">周期刷新</Text>
              <Button size="small" type="primary" loading={configSavingSection === 'renewal'} onClick={() => onSaveClientBilling(record)}>
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
      title: '路由指向',
      key: 'route',
      width: 280,
      render: (_, record) => <RouteBadge route={record.route} onJumpOutbound={onJumpOutbound} onJumpRule={onJumpRule} />,
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
            {selectedAgent.summary.last_collection_err ? (
              <Tag color="orange" style={{ cursor: 'pointer' }} onClick={onOpenLogs}>x-ui 异常</Tag>
            ) : null}
            <Button onClick={onOpenLogs}>查看日志</Button>
            <Button disabled={!canOpenXUI} onClick={onOpenXUI}>打开 x-ui 面板</Button>
            <Button icon={<ReloadOutlined />} loading={currentAgentLoading} onClick={onRefreshCurrentAgent}>刷新当前 Client</Button>
          </Space>
        </div>
        <Tabs
          activeKey={activeTabKey}
          onChange={onActiveTabChange}
          items={[
            {
              key: 'overview',
              label: `总览 (${filteredAgents.length})`,
              children: renderGlobalOverviewPanel({
                dashboardView,
                selectedTag,
                links: filteredTagLinks,
                onSelectTag: (value) => onSelectTag(value),
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
                    <Button type="primary" disabled={!selectedAgentId} onClick={onCreateRoutingAction}>新增操作</Button>
                    <Button icon={<ReloadOutlined />} disabled={!selectedAgentId} loading={xuiActionsLoading} onClick={onRefreshXUIActions}>刷新操作记录</Button>
                  </Space>
                  <Table rowKey={(record) => record.id} columns={xuiActionColumns} dataSource={xuiActions} loading={xuiActionsLoading} pagination={{ pageSize: 8, hideOnSinglePage: true }} scroll={{ x: 820 }} />
                </Space>
              ),
            },
            {
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
                onSave: onSaveManagedConfigSection,
                onAgentNameChange: onManagedConfigAgentNameChange,
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
                  <Input.Search allowClear placeholder="按邮箱、备注、节点标签筛选客户端" value={clientSearch} onChange={(event) => onClientSearchChange(event.target.value)} />
                  <Table rowKey={(record) => `${record.inbound_tag}-${record.email}`} columns={clientColumns} dataSource={filteredClients} pagination={{ pageSize: 12, hideOnSinglePage: true }} scroll={{ x: 1780 }} />
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
    </>
  )
}
