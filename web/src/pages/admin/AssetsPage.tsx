import { CopyOutlined } from '@ant-design/icons'
import { ProTable, type ProColumns } from '@ant-design/pro-components'
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
  Statistic,
  Tag,
  Typography,
  theme,
} from 'antd'
import { useEffect, useState, type ComponentProps } from 'react'
import type { AgentRail } from '../../components/DashboardAgentRail'
import { CountryFlag } from '../../components/CountryFlag'
import { agentHealthCategory, agentHealthFilterLabel } from '../../lib/agentHealth'
import { agentCountryCode, formatAgentLocation } from '../../lib/appHelpers'
import {
  calculateDiskPercent,
  calculateMemoryPercent,
  calculateTrafficStatus,
  formatBytes,
  formatSpeed,
} from '../../lib/traffic'
import type { DashboardAgentView } from '../../types'

export type AssetsPageProps = ComponentProps<typeof AgentRail> & { accountKey: string }
const healthOptions = (['all', 'healthy', 'warning', 'offline'] as const).map((value) => ({
  value,
  label: agentHealthFilterLabel(value),
}))
const statusText = { healthy: '健康在线', warning: '运行告警', offline: '离线' } as const
const statusBadge = { healthy: 'success', warning: 'warning', offline: 'error' } as const

function ResourceMetric({ value, label }: { value?: number; label: string }) {
  if (value === undefined || !Number.isFinite(value))
    return <Typography.Text type="secondary">暂无数据</Typography.Text>
  const percent = Math.min(100, Math.max(0, value))
  return (
    <div className="admin-assets-metric" aria-label={`${label} ${percent.toFixed(0)}%`}>
      <span>{percent.toFixed(0)}%</span>
      <Progress percent={percent} showInfo={false} size="small" status={percent >= 90 ? 'exception' : 'normal'} />
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

export function AssetsPage(props: AssetsPageProps) {
  const { message } = App.useApp()
  const [selected, setSelected] = useState<React.Key[]>([])
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  useEffect(() => {
    setPage(1)
    setSelected([])
  }, [props.selectedTag, props.healthFilter, props.searchText, props.accountKey])
  useEffect(() => {
    const ids = new Set(props.agents.map((agent) => agent.agent_id))
    setSelected((current) => current.filter((key) => ids.has(String(key))))
    setPage((current) => Math.min(current, Math.max(1, Math.ceil(props.agents.length / pageSize))))
  }, [props.agents, pageSize])
  const online = props.agents.filter((agent) => agentHealthCategory(agent) !== 'offline').length
  const warning = props.agents.filter((agent) => agentHealthCategory(agent) === 'warning').length
  const open = (agent: DashboardAgentView) => props.onSelectAgent(agent.agent_id, false)
  const identity = (agent: DashboardAgentView) => {
    const countryCode = agentCountryCode(agent)
    const location = formatAgentLocation(agent, countryCode)
    return (
      <div className="admin-assets-name">
        <div className="admin-assets-name-row">
          {!props.restrictedView ? <CountryFlag code={countryCode} label={location || countryCode || '未知地区'} /> : null}
          <Button type="link" onClick={() => open(agent)}>
            {agent.agent_name || agent.agent_id}
          </Button>
        </div>
        {!props.restrictedView ? <small>{location}</small> : null}
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
            render: (_: unknown, agent: DashboardAgentView) => <ResourceMetric value={agent.summary.cpu} label="CPU" />,
          },
          {
            title: '内存',
            key: 'memory',
            width: 110,
            sorter: (a: DashboardAgentView, b: DashboardAgentView) =>
              calculateMemoryPercent(a.summary) - calculateMemoryPercent(b.summary),
            render: (_: unknown, agent: DashboardAgentView) => (
              <ResourceMetric
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
                value={agent.summary.disk_total ? calculateDiskPercent(agent.summary) : undefined}
                label="硬盘"
              />
            ),
          },
          {
            title: '实时网速',
            key: 'speed',
            width: 150,
            render: (_: unknown, agent: DashboardAgentView) => (
              <div className="admin-assets-speed">
                <span>↑ {formatSpeed(agent.summary.net_io_up || 0)}</span>
                <Typography.Text type="secondary">↓ {formatSpeed(agent.summary.net_io_down || 0)}</Typography.Text>
              </div>
            ),
          },
        ]
      : []),
    {
      title: '周期流量',
      key: 'traffic',
      width: 190,
      sorter: (a, b) => calculateTrafficStatus(a).total.used - calculateTrafficStatus(b).total.used,
      render: (_, agent) => <TrafficMetric agent={agent} restrictedView={props.restrictedView} />,
    },
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
      fixed: 'right',
      render: (_, agent) => [
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
      {props.searchText || props.selectedTag || props.healthFilter !== 'all' ? (
        <Button
          onClick={() => {
            props.onSearchTextChange('')
            props.onSelectTag('')
            props.onHealthFilterChange('all')
          }}
        >
          重置筛选
        </Button>
      ) : null}
    </div>
  )
  return (
    <section className="admin-assets-page" aria-label="Client 资产列表">
      <div className="admin-assets-summary">
        <Card>
          <Statistic title="当前结果" value={props.agents.length} suffix="台" />
        </Card>
        <Card>
          <Statistic title="在线 Client" value={online} suffix="台" />
        </Card>
        <Card>
          <Statistic title="运行告警" value={warning} suffix="台" />
        </Card>
      </div>
      {props.error ? (
        <Alert type="error" showIcon message={props.error} action={<Button onClick={props.onRefresh}>重试</Button>} />
      ) : null}
      <Card>{filters}</Card>
      <div className="admin-assets-desktop">
        <ProTable<DashboardAgentView>
          rowKey="agent_id"
          headerTitle="Client 列表"
          columns={columns}
          dataSource={props.agents}
          loading={props.loading}
          search={false}
          columnsState={{
            persistenceKey: `vpsmonitor.assets.columns.${props.accountKey}.${props.restrictedView ? 'area' : 'admin'}`,
            persistenceType: 'localStorage',
          }}
          options={{ reload: props.onRefresh, density: true, setting: true, fullScreen: false }}
          rowSelection={{ selectedRowKeys: selected, onChange: setSelected }}
          tableAlertRender={({ selectedRowKeys }) => (
            <Space wrap>
              已选择 {selectedRowKeys.length} 台
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
            current: page,
            pageSize,
            showSizeChanger: true,
            showTotal: (total) => `共 ${total} 台`,
            onChange: (current, size) => {
              setPage(current)
              setPageSize(size)
            },
          }}
          scroll={{ x: props.restrictedView ? 760 : 1250 }}
          locale={{ emptyText: <Empty description="没有匹配的 Client" /> }}
        />
      </div>
      <div className="admin-assets-mobile" aria-busy={props.loading}>
        {props.agents.slice((page - 1) * pageSize, page * pageSize).map((agent) => (
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
                    <ResourceMetric value={agent.summary.cpu} label="CPU" />
                  </div>
                  <div>
                    <Typography.Text type="secondary">内存</Typography.Text>
                    <ResourceMetric
                      value={agent.summary.mem_total ? calculateMemoryPercent(agent.summary) : undefined}
                      label="内存"
                    />
                  </div>
                </>
              ) : null}
              <div>
                <Typography.Text type="secondary">周期流量</Typography.Text>
                <TrafficMetric agent={agent} restrictedView={props.restrictedView} />
              </div>
              {!props.restrictedView ? (
                <div>
                  <Typography.Text type="secondary">实时网速</Typography.Text>
                  <div>
                    ↑{formatSpeed(agent.summary.net_io_up || 0)} ↓{formatSpeed(agent.summary.net_io_down || 0)}
                  </div>
                </div>
              ) : null}
            </div>
            <Space wrap>
              {agent.tags?.map((tag) => (
                <Tag key={tag}>{tag}</Tag>
              ))}
            </Space>
            <Button block onClick={() => open(agent)} style={{ marginTop: 12 }}>
              查看详情
            </Button>
          </Card>
        ))}
        {!props.agents.length ? <Empty description={props.loading ? '正在加载 Client' : '没有匹配的 Client'} /> : null}
        <Pagination
          size="small"
          current={page}
          pageSize={pageSize}
          total={props.agents.length}
          onChange={setPage}
          showSizeChanger={false}
        />
      </div>
    </section>
  )
}
