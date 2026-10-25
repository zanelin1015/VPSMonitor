import { Badge, Button, Popconfirm, Space, Switch, Tag, Tooltip, Typography } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { ReloadOutlined } from '@ant-design/icons';
import type { XUIClientView, XUINodeView } from '../types';
import { clientBidirectionalTrafficTotal, formatBytes } from '../lib/traffic';
import { defaultClientBilling, effectiveClientBillingExpiryTime, effectiveClientBillingStartTime, findClientBilling, formatExpiryTime, formatRelativeTime, isClientOnline, normalizeClientTrafficMultiplier } from '../lib/appHelpers';
import { RouteBadge } from './RouteBadge';
import { beijingDateInput, clientConfigSaveStatus } from '../lib/clientConfig';
import { clientTrafficResetSummary } from '../lib/clientTrafficReset';
import { type HAProxyForwardClientView, type HAProxyForwardNodeView, type RealmForwardClientView, type RealmForwardNodeView } from './AgentDetailHelpers';
import type { Dispatch, SetStateAction } from 'react'
import type { AgentDetailPanelProps } from './AgentDetailPanel.types'

const { Text } = Typography

type AgentClientTableOptions = Pick<AgentDetailPanelProps, 'canManageConfig' | 'onCreateNodeClientAction' | 'onAuthorizeCustomer' | 'selectedAgentId' | 'selectedAgent' | 'onJumpOutbound' | 'onJumpRule' | 'overview' | 'xuiClientResetLoadingKey' | 'xuiClientToggleLoadingKey' | 'onSetXUIClientEnabled' | 'savedClientBillings' | 'managedConfig' | 'xuiActions' | 'onManualResetXUIClientTraffic' | 'onCopyImportURL' | 'onOpenImportURL' | 'onDeleteXUIClient' | 'xuiClientDeleteLoadingKey'> & {
  restrictedView: boolean
  canManageClients: boolean
  setClientConfigEditor: Dispatch<SetStateAction<XUIClientView | null>>
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

export function buildAgentClientTables({
  canManageConfig,
  onCreateNodeClientAction,
  onAuthorizeCustomer,
  selectedAgentId,
  selectedAgent,
  onJumpOutbound,
  onJumpRule,
  overview,
  xuiClientResetLoadingKey,
  xuiClientToggleLoadingKey,
  onSetXUIClientEnabled,
  savedClientBillings,
  managedConfig,
  xuiActions,
  onManualResetXUIClientTraffic,
  onCopyImportURL,
  onOpenImportURL,
  onDeleteXUIClient,
  xuiClientDeleteLoadingKey,
  restrictedView,
  canManageClients,
  setClientConfigEditor,
}: AgentClientTableOptions) {
  const xuiClientActionKey = (record: XUIClientView) => [record.inbound_id, record.inbound_tag || '', record.client_id || record.auth_uuid || record.auth_password || '', record.email || ''].join(':')
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
            <Tooltip title={accountBasedProxy ? '节点账号请在 x-ui 入站中维护' : !canManageClients ? '当前账号没有修改权限' : `点击${configuredEnabled ? '禁用' : '启用'}客户端`}>
              <span className="client-enabled-control">
                <Switch
                  className="client-enabled-switch"
                  checked={configuredEnabled}
                  checkedChildren="启用"
                  unCheckedChildren="禁用"
                  disabled={!canManageClients || accountBasedProxy || Boolean(xuiClientResetLoadingKey) || Boolean(xuiClientToggleLoadingKey && xuiClientToggleLoadingKey !== xuiClientActionKey(record))}
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
          <div className="client-traffic-directions">
            <Tooltip title="上传"><span aria-label={`上传 ${formatBytes(record.up || 0)}`}><span className="client-traffic-arrow" aria-hidden="true">↑</span>{formatBytes(record.up || 0)}</span></Tooltip>
            <Tooltip title="下载"><span aria-label={`下载 ${formatBytes(record.down || 0)}`}><span className="client-traffic-arrow" aria-hidden="true">↓</span>{formatBytes(record.down || 0)}</span></Tooltip>
          </div>
          <div className="client-traffic-reset">
            <span className="client-summary-label">重置</span>
            <span className="client-summary-chip">{clientTrafficResetSummary(record)}</span>
            <Tooltip title="流量计费倍率"><span className="client-summary-chip">{normalizeClientTrafficMultiplier(billing.traffic_multiplier)} 倍</span></Tooltip>
            {!restrictedView && canManageConfig && !isAccountBasedProxyClient(record) ? (
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
          </div>
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
            <Button size="small" disabled={!canManageClients || !record.email} onClick={() => setClientConfigEditor(record)}>编辑</Button>
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
    ? clientColumns.filter((column) => !['expiry', 'route'].includes(String(column.key || '')))
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

  return { nodeColumns, visibleClientColumns, visibleRealmNodeColumns, visibleRealmClientColumns, visibleHAProxyNodeColumns, visibleHAProxyClientColumns }
}
