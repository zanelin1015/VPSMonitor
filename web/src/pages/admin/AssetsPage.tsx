import { CopyOutlined, DeploymentUnitOutlined, TagsOutlined } from '@ant-design/icons'
import { ProTable, type ProColumns } from '@ant-design/pro-table'
import {
  Alert,
  App,
  Badge,
  Button,
  Card,
  Empty,
  Input,
  Pagination,
  Progress,
  Select,
  Space,
  Tag,
  Typography,
  theme,
} from 'antd'
import { useEffect, useState } from 'react'
import type { AgentListProps } from '../../lib/agentList'
import { CountryFlag } from '../../components/CountryFlag'
import { AgentClassificationModal } from '../../components/AgentClassificationModal'
import { AgentTagManager } from '../../components/AgentTagManager'
import { useListPagination } from '../../hooks/useListPagination'
import { listPagination } from '../../lib/pagination'
import { agentHealthCategory, agentHealthFilterLabel } from '../../lib/agentHealth'
import { agentCountryCode, calculateRenewalStatus, formatAgentLocation, formatLocalDate } from '../../lib/appHelpers'
import { normalizeCurrencyCode } from '../../lib/currency'
import { classificationCountry, classificationCountryLabel, purposeGroups, purposeLabels } from '../../lib/agentClassification'
import {
  calculateDiskPercent,
  calculateMemoryPercent,
  calculateTrafficStatus,
  formatBytes,
  formatSpeed,
} from '../../lib/traffic'
import type { DashboardAgentView, VPSRenewalConfig } from '../../types'

export type AssetsPageProps = AgentListProps & {
  accountKey: string
  onOpenClientInstall?: () => void
  onAgentTagsChanged?: (tags: string[], deletedTag?: string) => void
}
const healthOptions = (['all', 'healthy', 'warning', 'offline'] as const).map((value) => ({
  value,
  label: agentHealthFilterLabel(value),
}))
const statusText = { healthy: '健康在线', warning: '运行告警', offline: '离线' } as const
const statusBadge = { healthy: 'success', warning: 'warning', offline: 'error' } as const

function AgentPurposeTags({ agent }: { agent: DashboardAgentView }) {
  return <>{agent.entry?.classification?.purposes?.map(purpose => <Tag color="blue" key={purpose}>{purposeLabels[purpose]}</Tag>)}</>
}

function UnavailableMetric({ label, offline }: { label: string; offline: boolean }) {
  const reason = offline ? 'Client 离线，暂无实时数据' : '暂无数据'
  return <Typography.Text type="secondary" title={reason} aria-label={`${label}：${reason}`}>—</Typography.Text>
}

function ResourceMetric({ agent, value, label }: { agent: DashboardAgentView; value?: number; label: string }) {
  const offline = agentHealthCategory(agent) === 'offline'
  if (offline) return <UnavailableMetric label={label} offline />
  if (value === undefined || !Number.isFinite(value))
    return <UnavailableMetric label={label} offline={false} />
  const percent = Math.min(100, Math.max(0, value))
  return (
    <div className="admin-assets-metric" aria-label={`${label} ${percent.toFixed(0)}%`}>
      <span>{percent.toFixed(0)}%</span>
      <Progress percent={percent} showInfo={false} size="small" status={percent >= 90 ? 'exception' : 'normal'} />
    </div>
  )
}

function SpeedMetric({ agent }: { agent: DashboardAgentView }) {
  if (agentHealthCategory(agent) === 'offline') return <UnavailableMetric label="实时网速" offline />
  return (
    <div className="admin-assets-speed">
      <span>↑ {formatSpeed(agent.summary.net_io_up || 0)}</span>
      <Typography.Text type="secondary">↓ {formatSpeed(agent.summary.net_io_down || 0)}</Typography.Text>
    </div>
  )
}

function TrafficMetric({ agent, restrictedView }: { agent: DashboardAgentView; restrictedView?: boolean }) {
  const { token } = theme.useToken()
  const traffic = calculateTrafficStatus(agent).total
  if (restrictedView) return <span>{formatBytes(traffic.used)}</span>
  const hasLimit = traffic.total > 0 && typeof traffic.percent === 'number'
  const color = traffic.level === 'bad' ? token.colorError : traffic.level === 'warn' ? token.colorWarning : token.colorPrimary
  return (
    <div className="admin-assets-metric" title={hasLimit ? traffic.label : '未设置流量额度'} aria-label={`周期流量 ${hasLimit ? traffic.label : formatBytes(traffic.used) + '，未设置额度'}`}>
      <div className="admin-assets-traffic-value">
        <span>{formatBytes(traffic.used)}{hasLimit ? ` / ${formatBytes(traffic.total)}` : ''}</span>
        <Typography.Text type="secondary">{hasLimit ? `${((traffic.used / traffic.total) * 100).toFixed(1)}%` : '未设置额度'}</Typography.Text>
      </div>
      {hasLimit ? <Progress percent={traffic.percent} showInfo={false} size="small" strokeColor={color} status={traffic.level === 'bad' ? 'exception' : 'normal'} /> : null}
    </div>
  )
}

function ExpiryMetric({ agent }: { agent: DashboardAgentView }) {
  const status = calculateRenewalStatus(agent.renewal)
  if (!status) return <Typography.Text type="secondary">未设置</Typography.Text>
  const date = formatLocalDate(new Date(status.endTime))
  return (
    <div className="admin-assets-expiry" title={status.autoRenew ? '到期后自动刷新下一周期' : undefined}>
      <time dateTime={date.replace(/\//g, '-')}>{date}</time>
      <Typography.Text type={status.level === 'ok' ? 'secondary' : 'warning'}>
        {status.remainingLabel}
      </Typography.Text>
    </div>
  )
}

function costCycleLabel(cycle?: VPSRenewalConfig['cost_cycle']) {
  switch (cycle) {
    case 'quarter':
      return '季'
    case 'semiannual':
      return '半年'
    case 'year':
      return '年'
    case 'month':
    default:
      return '月'
  }
}

function costCurrencySymbol(currency: string) {
  switch (currency) {
    case 'CNY':
    case 'JPY':
      return '¥'
    case 'USD':
    case 'USDT':
      return '$'
    case 'CAD':
      return 'CA$'
    case 'EUR':
      return '€'
    case 'HKD':
      return 'HK$'
    case 'GBP':
      return '£'
    case 'AUD':
      return 'A$'
    case 'SGD':
      return 'S$'
    default:
      return currency
  }
}

function CostMetric({ agent }: { agent: DashboardAgentView }) {
  const amount = Number(agent.renewal?.cost_amount)
  if (!Number.isFinite(amount) || amount <= 0) {
    return <Typography.Text type="secondary">未设置</Typography.Text>
  }
  const currency = normalizeCurrencyCode(agent.renewal?.cost_currency)
  const cycle = costCycleLabel(agent.renewal?.cost_cycle)
  const amountLabel = new Intl.NumberFormat('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(amount)
  const costLabel = `${currency} ${costCurrencySymbol(currency)}${amountLabel}/${cycle}`
  return <div className="admin-assets-cost" title={`费用 ${costLabel}`} aria-label={`费用 ${costLabel}`}>{costLabel}</div>
}

export function AssetsPage(props: AssetsPageProps) {
  const { message } = App.useApp()
  const [selected, setSelected] = useState<React.Key[]>([])
  const [purpose, setPurpose] = useState('')
  const [country, setCountry] = useState('')
  const [classificationAgents, setClassificationAgents] = useState<DashboardAgentView[]>([])
  const [tagManagerOpen, setTagManagerOpen] = useState(false)
  const canManageTags = props.canManageConfig && Boolean(props.onAgentTagsChanged)
  const filterClassification = !props.restrictedView
  const agents = props.agents.filter(agent => !filterClassification || (
    (!country || classificationCountry(agent) === country)
    && (!purpose || (purpose === 'unclassified' ? !agent.entry?.classification?.purposes?.length : agent.entry?.classification?.purposes?.includes(purpose)))
  ))
  const countryOptions = [...new Set(props.agents.map(classificationCountry).filter(Boolean))].sort()
    .map(code => ({ value: code, label: classificationCountryLabel(code) }))
  const { page, pageSize, onChange: onPageChange } = useListPagination(
    agents.length, JSON.stringify([props.selectedTag, props.healthFilter, props.searchText, props.accountKey, purpose, country]),
  )
  useEffect(() => {
    setSelected([])
  }, [props.selectedTag, props.healthFilter, props.searchText, props.accountKey, purpose, country])
  useEffect(() => {
    const ids = new Set(props.agents.map((agent) => agent.agent_id))
    setSelected((current) => current.filter((key) => ids.has(String(key))))
  }, [props.agents])
  const online = agents.filter((agent) => agentHealthCategory(agent) !== 'offline').length
  const warning = agents.filter((agent) => agentHealthCategory(agent) === 'warning').length
  const open = (agent: DashboardAgentView) => props.onSelectAgent(agent.agent_id, false)
  const identity = (agent: DashboardAgentView) => {
    const countryCode = agentCountryCode(agent)
    const location = formatAgentLocation(agent, countryCode)
    const address = [agent.summary.observed_ip, agent.summary.public_ipv4, agent.summary.public_ipv6]
      .map((value) => value?.trim())
      .find(Boolean)
    return (
      <div className="admin-assets-name">
        <div className="admin-assets-name-row">
          {!props.restrictedView ? <CountryFlag code={countryCode} label={location || countryCode || '未知地区'} /> : null}
          <Button type="link" onClick={() => open(agent)}>
            {agent.agent_name || agent.agent_id}
          </Button>
        </div>
        {!props.restrictedView ? (
          <>
            <Typography.Text
              className="admin-assets-address"
              type="secondary"
              copyable={address ? { text: address } : false}
            >
              {address || 'IP 未上报'}
            </Typography.Text>
            <small title={location}>{location}</small>
          </>
        ) : null}
      </div>
    )
  }
  const state = (agent: DashboardAgentView) => {
    if (props.restrictedView) return <Tag>已授权</Tag>
    const health = agentHealthCategory(agent)
    return <Badge status={statusBadge[health]} text={statusText[health]} />
  }
  const columns: ProColumns<DashboardAgentView>[] = [
    {
      title: 'Client',
      dataIndex: 'agent_name',
      key: 'name',
      width: 210,
      fixed: 'left',
      render: (_, agent) => identity(agent),
      sorter: (a, b) => (a.agent_name || a.agent_id).localeCompare(b.agent_name || b.agent_id),
    },
    { title: '状态', key: 'health', width: 120, render: (_, agent) => state(agent) },
    {
      title: '标签',
      dataIndex: 'tags',
      key: 'tags',
      width: 160,
      render: (_, agent) => (
        <Space size={[0, 4]} wrap>
          {filterClassification ? <AgentPurposeTags agent={agent} /> : null}
          {agent.tags?.length ? (
            agent.tags.map((tag) => <Tag key={tag}>{tag}</Tag>)
          ) : (
            <Typography.Text type="secondary">—</Typography.Text>
          )}
        </Space>
      ),
    },
    ...(!props.restrictedView
      ? [
          {
            title: 'CPU',
            key: 'cpu',
            width: 110,
            sorter: (a: DashboardAgentView, b: DashboardAgentView) => (a.summary.cpu || 0) - (b.summary.cpu || 0),
            render: (_: unknown, agent: DashboardAgentView) => <ResourceMetric agent={agent} value={agent.summary.cpu} label="CPU" />,
          },
          {
            title: '内存',
            key: 'memory',
            width: 110,
            sorter: (a: DashboardAgentView, b: DashboardAgentView) =>
              calculateMemoryPercent(a.summary) - calculateMemoryPercent(b.summary),
            render: (_: unknown, agent: DashboardAgentView) => (
              <ResourceMetric
                agent={agent}
                value={agent.summary.mem_total ? calculateMemoryPercent(agent.summary) : undefined}
                label="内存"
              />
            ),
          },
          {
            title: '硬盘',
            key: 'disk',
            width: 110,
            sorter: (a: DashboardAgentView, b: DashboardAgentView) =>
              calculateDiskPercent(a.summary) - calculateDiskPercent(b.summary),
            render: (_: unknown, agent: DashboardAgentView) => (
              <ResourceMetric
                agent={agent}
                value={agent.summary.disk_total ? calculateDiskPercent(agent.summary) : undefined}
                label="硬盘"
              />
            ),
          },
        ]
      : []),
    {
      title: '实时网速',
      key: 'speed',
      width: 150,
      render: (_, agent) => <SpeedMetric agent={agent} />,
    },
    {
      title: '周期流量',
      key: 'traffic',
      width: 190,
      sorter: (a, b) => calculateTrafficStatus(a).total.used - calculateTrafficStatus(b).total.used,
      render: (_, agent) => <TrafficMetric agent={agent} restrictedView={props.restrictedView} />,
    },
    ...(!props.restrictedView
      ? [{
          title: '费用',
          key: 'cost',
          width: 150,
          sorter: (a: DashboardAgentView, b: DashboardAgentView) => (a.renewal?.cost_amount || 0) - (b.renewal?.cost_amount || 0),
          render: (_: unknown, agent: DashboardAgentView) => <CostMetric agent={agent} />,
        }, {
          title: '到期时间',
          key: 'expiry',
          width: 145,
          sorter: (a: DashboardAgentView, b: DashboardAgentView, order?: 'ascend' | 'descend' | null) => {
            const aTime = calculateRenewalStatus(a.renewal)?.endTime
            const bTime = calculateRenewalStatus(b.renewal)?.endTime
            // Keep unconfigured dates last in either sorting direction.
            if (aTime === undefined && bTime === undefined) return 0
            if (aTime === undefined) return order === 'descend' ? -1 : 1
            if (bTime === undefined) return order === 'descend' ? 1 : -1
            return aTime - bTime
          },
          render: (_: unknown, agent: DashboardAgentView) => <ExpiryMetric agent={agent} />,
        }]
      : []),
    {
      title: '节点 / 客户端',
      key: 'nodes',
      width: 135,
      render: (_, agent) => `${agent.node_count} / ${agent.client_count}`,
    },
    {
      title: '操作',
      key: 'actions',
      valueType: 'option',
      width: 92,
      render: (_, agent) => [
        ...(props.canManageConfig ? [<Button key="classification" type="link" onClick={() => setClassificationAgents([agent])}>分类</Button>] : []),
        <Button key="open" type="link" onClick={() => open(agent)}>
          详情
        </Button>,
      ],
    },
  ]
  const copySelection = async () => {
    const rows = props.agents.filter((agent) => selected.includes(agent.agent_id))
    try {
      await navigator.clipboard.writeText(
        rows.map((agent) => `${agent.agent_name || agent.agent_id}\t${agent.agent_id}`).join('\n'),
      )
      message.success(`已复制 ${rows.length} 个 Client 名称与 ID`)
    } catch {
      message.error('复制失败，请允许浏览器访问剪贴板后重试')
    }
  }
  const filters = (
    <div className="admin-assets-filters">
      <Input
        allowClear
        aria-label="搜索 Client"
        placeholder="搜索服务器、节点或客户端"
        value={props.searchText}
        onChange={(event) => props.onSearchTextChange(event.target.value)}
      />
      <Select
        aria-label="健康状态"
        value={props.healthFilter}
        options={healthOptions}
        onChange={props.onHealthFilterChange}
        style={{ minWidth: 140 }}
      />
      <Select
        aria-label="标签筛选"
        value={props.selectedTag}
        options={[
          { value: '', label: '全部标签' },
          ...props.tagFilterOptions.map((tag) => ({ value: tag.tag, label: `${tag.tag} (${tag.agent_count})` })),
        ]}
        onChange={props.onSelectTag}
        style={{ minWidth: 160 }}
      />
      {filterClassification ? <>
        <Select aria-label="线路用途" value={purpose} onChange={setPurpose} style={{ minWidth: 145 }}
          options={[{ value: '', label: '全部线路用途' }, { value: 'unclassified', label: '未分类' }, ...purposeGroups]} />
        <Select aria-label="国家或地区" value={country} onChange={setCountry} style={{ minWidth: 140 }}
          options={[{ value: '', label: '全部国家／地区' }, ...countryOptions]} />
      </> : null}
      {props.searchText || props.selectedTag || props.healthFilter !== 'all' || purpose || country ? (
        <Button
          onClick={() => {
            props.onSearchTextChange('')
            props.onSelectTag('')
            props.onHealthFilterChange('all')
            setPurpose('')
            setCountry('')
          }}
        >
          重置筛选
        </Button>
      ) : null}
      <Space className="admin-assets-summary" size={16} wrap>
        <Typography.Text type="secondary">当前结果 <strong>{agents.length}</strong> 台</Typography.Text>
        <Typography.Text type="secondary">在线 <strong>{online}</strong> 台</Typography.Text>
        <Typography.Text type={warning ? 'warning' : 'secondary'}>告警 <strong>{warning}</strong> 台</Typography.Text>
      </Space>
    </div>
  )
  return (
    <section className="admin-assets-page" aria-label="Client 资产列表">
      {props.error ? (
        <Alert type="error" showIcon message={props.error} action={<Button onClick={props.onRefresh}>重试</Button>} />
      ) : null}
      <Card className="admin-assets-filter-card">
        {filters}
        {canManageTags ? <Button className="admin-assets-mobile-install" icon={<TagsOutlined />} onClick={() => setTagManagerOpen(true)}>标签管理</Button> : null}
        {props.onOpenClientInstall ? (
          <Button className="admin-assets-mobile-install" icon={<DeploymentUnitOutlined />} onClick={props.onOpenClientInstall}>
            安装 Client
          </Button>
        ) : null}
      </Card>
      <div className="admin-assets-desktop">
        <ProTable<DashboardAgentView>
          rowKey="agent_id"
          headerTitle="Client 列表"
          defaultSize="small"
          columns={columns}
          dataSource={agents}
          loading={props.loading}
          search={false}
          columnsState={{
            persistenceKey: `vpsmonitor.assets.columns.${props.accountKey}.${props.restrictedView ? 'area' : 'admin'}`,
            persistenceType: 'localStorage',
          }}
          options={{ reload: props.onRefresh, density: true, setting: true, fullScreen: false }}
          toolBarRender={() => [
            ...(canManageTags ? [<Button key="tags" icon={<TagsOutlined />} onClick={() => setTagManagerOpen(true)}>标签管理</Button>] : []),
            ...(props.onOpenClientInstall ? [<Button key="install" type="primary" icon={<DeploymentUnitOutlined />} onClick={props.onOpenClientInstall}>
              安装 Client
            </Button>] : []),
          ]}
          rowSelection={{ selectedRowKeys: selected, onChange: setSelected }}
          tableAlertRender={({ selectedRowKeys }) => (
            <Space wrap>
              已选择 {selectedRowKeys.length} 台
              {props.canManageConfig ? <Button size="small" onClick={() => setClassificationAgents(agents.filter(agent => selectedRowKeys.includes(agent.agent_id)))}>批量分类</Button> : null}
              <Button icon={<CopyOutlined />} size="small" onClick={() => void copySelection()}>
                复制名称与 ID
              </Button>
            </Space>
          )}
          tableAlertOptionRender={() => (
            <Button type="link" size="small" onClick={() => setSelected([])}>
              取消选择
            </Button>
          )}
          pagination={{
            ...listPagination,
            current: page,
            pageSize,
            showTotal: (total) => `共 ${total} 台`,
            onChange: onPageChange,
          }}
          // Let rc-table size the complete column set and keep the header and
          // body measurements in sync while the table is dragged horizontally.
          scroll={{ x: 'max-content', y: '100%' }}
          locale={{ emptyText: <Empty description="没有匹配的 Client" /> }}
        />
      </div>
      <div className="admin-assets-mobile" aria-busy={props.loading}>
        {agents.slice((page - 1) * pageSize, page * pageSize).map((agent) => (
          <Card key={agent.agent_id}>
            <div className="admin-assets-mobile-head">
              {identity(agent)}
              {state(agent)}
            </div>
            <div className="admin-assets-mobile-metrics">
              {!props.restrictedView ? (
                <>
                  <div>
                    <Typography.Text type="secondary">CPU</Typography.Text>
                    <ResourceMetric agent={agent} value={agent.summary.cpu} label="CPU" />
                  </div>
                  <div>
                    <Typography.Text type="secondary">内存</Typography.Text>
                    <ResourceMetric
                      agent={agent}
                      value={agent.summary.mem_total ? calculateMemoryPercent(agent.summary) : undefined}
                      label="内存"
                    />
                  </div>
                  <div>
                    <Typography.Text type="secondary">硬盘</Typography.Text>
                    <ResourceMetric
                      agent={agent}
                      value={agent.summary.disk_total ? calculateDiskPercent(agent.summary) : undefined}
                      label="硬盘"
                    />
                  </div>
                </>
              ) : null}
              <div>
                <Typography.Text type="secondary">周期流量</Typography.Text>
                <TrafficMetric agent={agent} restrictedView={props.restrictedView} />
              </div>
              <div>
                <Typography.Text type="secondary">实时网速</Typography.Text>
                <SpeedMetric agent={agent} />
              </div>
              <div>
                <Typography.Text type="secondary">节点 / 客户端</Typography.Text>
                <Typography.Text>{agent.node_count} / {agent.client_count}</Typography.Text>
              </div>
              {!props.restrictedView ? (
                <>
                  <div>
                    <Typography.Text type="secondary">费用</Typography.Text>
                    <CostMetric agent={agent} />
                  </div>
                  <div>
                    <Typography.Text type="secondary">到期时间</Typography.Text>
                    <ExpiryMetric agent={agent} />
                  </div>
                </>
              ) : null}
            </div>
            <Space wrap>
              {filterClassification ? <AgentPurposeTags agent={agent} /> : null}
              {agent.tags?.map((tag) => (
                <Tag key={tag}>{tag}</Tag>
              ))}
            </Space>
            {props.canManageConfig ? <Button type="link" onClick={() => setClassificationAgents([agent])}>编辑分类</Button> : null}
            <Button block onClick={() => open(agent)} style={{ marginTop: 12 }}>
              查看详情
            </Button>
          </Card>
        ))}
        {!agents.length ? <Empty description={props.loading ? '正在加载 Client' : '没有匹配的 Client'} /> : null}
        <Pagination
          {...listPagination}
          size="small"
          current={page}
          pageSize={pageSize}
          total={agents.length}
          onChange={onPageChange}
        />
      </div>
      {classificationAgents.length > 0 ? <AgentClassificationModal agents={classificationAgents}
        onClose={() => setClassificationAgents([])} onSaved={() => { setClassificationAgents([]); props.onRefresh() }} /> : null}
      {tagManagerOpen && canManageTags && props.onAgentTagsChanged ? <AgentTagManager
        onClose={() => setTagManagerOpen(false)} onChanged={props.onAgentTagsChanged} /> : null}
    </section>
  )
}
