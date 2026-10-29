import { useAgentRoutingTools } from './useAgentRoutingTools'
import { useAgentTerminalTools } from './useAgentTerminalTools';
import { buildAgentClientTables } from './AgentClientTables';
import { useEffect, useState } from 'react';
import { Alert, Button, Card, Empty, Input, Modal, Popconfirm, Select, Space, Table, Tabs, Tag, Typography } from 'antd';
import type { TabsProps } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { ArrowLeftOutlined, ReloadOutlined, SettingOutlined, SwapOutlined } from '@ant-design/icons';

import type { AgentLogsResponse, XUIAction, XUIClientView, XUINodeView } from '../types';
import { AgentCertificatesPanel } from './AgentCertificatesPanel';
import { AdminHelpHint } from './AdminHelpHint';
import { formatBytes, formatSpeed } from '../lib/traffic';
import { actionKindLabel, actionStatusColor, actionStatusLabel, billingKeyForClient, findClientBilling, formatDateTime, mergeTagOptions, shortJSON } from '../lib/appHelpers';
import { renderGlobalOverviewPanel } from './DashboardTopologyPanels';
import { FeatureSwitch } from './AgentDetailTerminal';
import { ManagedConfigPanel } from './ManagedConfigPanel';

import { XUINodeList } from './XUINodeList';
import { XUIClientConfigModal } from './XUIClientConfigModal';
import { SingBoxActionPanel } from './SingBoxActionPanel'

import { buildHAProxyForwardClients, buildHAProxyForwardNodes, buildRealmForwardClients, buildRealmForwardNodes, filterRealmForwardClients, renderNodeClientHierarchySections } from './AgentDetailHelpers';
import type { AgentDetailPanelProps } from './AgentDetailPanel.types';
export type { AgentDetailPanelProps } from './AgentDetailPanel.types'

const { Text, Title } = Typography

export function AgentDetailPanel(props: AgentDetailPanelProps) {
  const { routingTabs, routingModals } = useAgentRoutingTools(props)
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
    overviewError,
    selectedAgent,
    selectedAgentId,
    selectedNodeAnchor,
    selectedTag,
    tagOptions,
    tagSaving,
    xuiActions,
    xuiActionsLoading,
    xuiClientDeleteLoadingKey,
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
    realmCopyLoading,
    onActiveTabChange,
    onClientSearchChange,
    onCopyImportURL,
    onCreateRoutingAction,
    onCreateNodeClientAction,
    onCreateTag,
    onEntryAddressesTextChange,
    onEntryChange,
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
    onSetXUIClientEnabled,
    onManualResetXUIClientTraffic,
    onRefreshCurrentAgent,
    onRestartXUI,
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
    onSingBoxChange,
    onFeatureChange,
  } = props
  const canManageClients = canManageConfig || restrictedView
  const [clientConfigEditor, setClientConfigEditor] = useState<XUIClientView | null>(null)
  const [commandOutputAction, setCommandOutputAction] = useState<XUIAction | null>(null)
  const { openRealtimeTerminal, openRemoteCommand, terminalModals } = useAgentTerminalTools({ selectedAgent, selectedAgentId, remoteCommandLoading, onExecuteRemoteCommand, commandOutputAction, setCommandOutputAction })
  const [replacementModalOpen, setReplacementModalOpen] = useState(false)
  const [replacementAgentID, setReplacementAgentID] = useState('')
  useEffect(() => {
    setClientConfigEditor(null)
  }, [selectedAgentId])

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

  const currentPrimaryDomain = managedConfig?.entry?.import_domain || selectedAgent.entry?.import_domain || ''
  const featureEnabled = {
    xui: Boolean(managedConfig?.features?.xui),
    singbox: Boolean(managedConfig?.features?.singbox),
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
      {featureEnabled.singbox ? (
        <section className="overview-stat-card overview-stat-blue">
          <div className="overview-stat-title">sing-box 专线</div>
          <div className="overview-stat-value"><span className="overview-stat-dot" /><strong>{selectedAgent.summary.singbox_node_count || 0}</strong></div>
          <div className="overview-stat-foot">客户端 {selectedAgent.summary.singbox_client_count || 0} · 规则 {selectedAgent.summary.singbox_rule_count || 0}</div>
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

  const { nodeColumns, visibleClientColumns, visibleRealmNodeColumns, visibleRealmClientColumns, visibleHAProxyNodeColumns, visibleHAProxyClientColumns } = buildAgentClientTables({
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
  })

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

  function renderManagedConfigSection(section: 'basic' | 'xui' | 'singbox' | 'nat' | 'network' | 'realm' | 'haproxy' | 'audit') {
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
        onSingBoxChange={onSingBoxChange}
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
        <FeatureSwitch label="sing-box" checked={featureEnabled.singbox} onChange={(checked) => onFeatureChange('singbox', checked)} />
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
    ...routingTabs,
  ] : []

  const singBoxTabs: TabsProps['items'] = featureEnabled.singbox ? [{
    key: 'singbox',
    label: 'sing-box / Snell',
    children: <SingBoxActionPanel agentID={selectedAgentId} enabled={Boolean(managedConfig?.singbox?.enabled)} canManage={canManageConfig && !restrictedView} actions={xuiActions} actionsLoading={xuiActionsLoading} onRefreshActions={onRefreshXUIActions} />,
  }] : []

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
      children: <AgentCertificatesPanel
        certificates={overview?.certificates}
        currentPrimaryDomain={currentPrimaryDomain}
        selectedAgentId={selectedAgentId}
        canManageConfig={canManageConfig}
        configSavingSection={configSavingSection}
        onSavePrimaryDomain={onSavePrimaryDomain}
      />,
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
      {
        key: 'config-singbox',
        label: 'sing-box 配置',
        children: renderManagedConfigSection('singbox'),
      },
    ] : []),
  ]

  const detailTabs: TabsProps['items'] = [
    overviewTab,
    ...xuiTabs,
    ...singBoxTabs,
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
            <Button icon={<ReloadOutlined />} loading={currentAgentLoading} onClick={onRefreshCurrentAgent}>立即获取 Client 信息</Button>
            {!restrictedView ? <Button danger onClick={openRealtimeTerminal}>实时 TTY</Button> : null}
            {!restrictedView ? <Button loading={remoteCommandLoading} onClick={openRemoteCommand}>单次命令</Button> : null}

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
      {routingModals}
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
      {terminalModals}
    </>
  )
}
