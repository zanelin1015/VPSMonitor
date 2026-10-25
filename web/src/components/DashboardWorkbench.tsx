import { WorkbenchSpeedTrend } from './WorkbenchSpeedTrend'
import { useAnimatedNumber } from '../hooks/useAnimatedNumber'
import { useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { Button, Card, Empty, Modal, Tag, Typography } from 'antd'
import { EyeInvisibleOutlined, EyeOutlined } from '@ant-design/icons'

import type { DashboardAgentView, GlobalDashboardView } from '../types'
import {
  buildClientExpiryRows,
  CLIENT_EXPIRY_WARNING_DAYS,
  clientExpiryCycleLabel,
  clientExpiryRemainingLabel,
  type ClientExpiryRow,
} from '../lib/clientExpiry'
import type { CurrencyCode, MonthlyFinanceSummary } from '../lib/currency'
import { formatMoney } from '../lib/currency'
import { agentDisplayStatus, calculateRenewalStatus, formatDateTime } from '../lib/appHelpers'
import {
  type AgentNetworkSummary,
  calculateMemoryPercent,
  calculateTrafficStatus,
  clampMetricPercent,
  formatBytes,
  formatPercent,
  formatSpeed,
} from '../lib/traffic'
import { MiniProgress } from './MiniProgress'
import { agentHealthCategory, type AgentHealthCategory } from '../lib/agentHealth'
import { readFinanceVisibilityPreference, writeFinanceVisibilityPreference } from '../lib/financeVisibility'

const { Text } = Typography

type WorkbenchMetricRow = {
  agent: DashboardAgentView
  status: ReturnType<typeof agentDisplayStatus>
  memoryPercent: number
  traffic: ReturnType<typeof calculateTrafficStatus>
  renewal: ReturnType<typeof calculateRenewalStatus>
  renewalDays: number | null
}

type WorkbenchAlertRow = {
  key: string
  agentID?: string
  level: 'warn' | 'bad'
  tag: string
  title: string
  detail: string
}

type WorkbenchFinanceBar = {
  label: string
  value: number
  tone: 'income' | 'cost' | 'profit' | 'loss'
}

export function AdminWorkbenchDashboard(props: {
  agents: DashboardAgentView[]
  dashboardView: GlobalDashboardView | null
  scopedNetwork: AgentNetworkSummary
  monthlyFinance: MonthlyFinanceSummary
  costCurrency: CurrencyCode
  restrictedView?: boolean
  onSelectAgent: (agentID: string) => void
  onOpenHealthFilter: (filter: AgentHealthCategory) => void
  onOpenTopology: () => void
}) {
  const { agents, dashboardView, scopedNetwork, monthlyFinance, costCurrency, restrictedView = false, onSelectAgent, onOpenHealthFilter, onOpenTopology } = props
  const [profitVisible, setProfitVisible] = useState(readFinanceVisibilityPreference)
  const [clientExpiryOpen, setClientExpiryOpen] = useState(false)
  const statusRows = useMemo<WorkbenchMetricRow[]>(() => agents.map((agent) => {
    const renewal = calculateRenewalStatus(agent.renewal)
    return {
      agent,
      status: agentDisplayStatus(agent),
      memoryPercent: calculateMemoryPercent(agent.summary),
      traffic: calculateTrafficStatus(agent),
      renewal,
      renewalDays: renewalDaysFromStatus(renewal),
    }
  }), [agents])

  const scopedClientCount = dashboardView?.totals.client_count ?? 0
  const scopedOnlineClientCount = dashboardView?.totals.online_client_count ?? 0
  const scopedNodeCount = dashboardView?.totals.node_count ?? 0
  const totalAgents = agents.length
  const healthyRows = statusRows.filter((row) => agentHealthCategory(row.agent) === 'healthy')
  const warningRows = statusRows.filter((row) => agentHealthCategory(row.agent) === 'warning')
  const offlineRows = statusRows.filter((row) => agentHealthCategory(row.agent) === 'offline')
  const onlineRows = [...healthyRows, ...warningRows]
  const highLoadRows = statusRows.filter((row) => row.status.level !== 'bad' && (row.memoryPercent >= 75 || Number(row.agent.summary.cpu || 0) >= 75))
  const xuiErrorRows = statusRows.filter((row) => Boolean(row.agent.summary.last_collection_err))
  const healthAlertCount = warningRows.length + offlineRows.length
  const onlinePercent = totalAgents ? (onlineRows.length / totalAgents) * 100 : 0
  const clientOnlinePercent = scopedClientCount ? (scopedOnlineClientCount / scopedClientCount) * 100 : 0

  const trafficCandidates = restrictedView
    ? statusRows.filter((row) => row.traffic.total.used > 0)
    : statusRows
  const maxTraffic = Math.max(1, ...trafficCandidates.map((row) => row.traffic.total.used))
  const trafficRows = [...trafficCandidates]
    .sort((left, right) => trafficRiskScore(right, maxTraffic) - trafficRiskScore(left, maxTraffic))
    .slice(0, 5)
  const trafficRiskRows = trafficCandidates.filter((row) => row.traffic.total.total > 0 && row.traffic.total.level !== 'ok')

  const renewalRows = statusRows
    .filter((row) => row.renewal)
    .sort((left, right) => renewalSortValue(left) - renewalSortValue(right))
  const renewalRiskRows = renewalRows
    .filter((row) => row.renewal?.level !== 'ok' || (row.renewalDays !== null && row.renewalDays <= 30))
    .slice(0, 5)
  const renewalRiskCount = renewalRows.filter((row) => row.renewal?.level !== 'ok').length
  const renewalWithin30Count = renewalRows.filter((row) => row.renewalDays !== null && row.renewalDays <= 30).length
  const clientExpiryRows = useMemo(() => buildClientExpiryRows(agents), [agents])
  const expiredClientCount = clientExpiryRows.filter((row) => row.remainingDays < 0).length
  const clientExpirySummary = `已过期 ${expiredClientCount} 个 · ${CLIENT_EXPIRY_WARNING_DAYS} 天内到期 ${clientExpiryRows.length - expiredClientCount} 个`

  const alertRows = buildWorkbenchAlerts({
    offlineRows,
    xuiErrorRows,
    highLoadRows,
    trafficRiskRows,
    renewalRows,
  }).slice(0, 5)

  const financeBars: WorkbenchFinanceBar[] = [
    { label: '月收入', value: monthlyFinance.revenueTotal, tone: 'income' },
    { label: '月成本', value: monthlyFinance.costTotal, tone: 'cost' },
    { label: '月利润', value: monthlyFinance.profitTotal, tone: monthlyFinance.profitTotal >= 0 ? 'profit' : 'loss' },
  ]
  const maxFinance = Math.max(1, ...financeBars.map((bar) => Math.abs(bar.value)))
  const toggleProfitVisibility = () => {
    setProfitVisible((visible) => {
      const next = !visible
      writeFinanceVisibilityPreference(next)
      return next
    })
  }
  return (
    <section className="admin-workbench">
      <div className="admin-workbench-strip">
        <div className="admin-workbench-heading">
          <Text type="secondary">运营驾驶舱</Text>
          <Typography.Title level={3}>全部服务器</Typography.Title>
          <small>{dashboardView ? `数据时间 ${formatDateTime(dashboardView.generated_at)}` : '等待全局概览数据'}</small>
        </div>
        <WorkbenchKpi label="服务器在线率" value={`${onlinePercent.toFixed(0)}%`} note={`${onlineRows.length}/${totalAgents} 在线`} tone={offlineRows.length ? 'warn' : 'ok'} />
        <WorkbenchKpi label="客户端在线率" value={`${clientOnlinePercent.toFixed(0)}%`} note={`${scopedOnlineClientCount}/${scopedClientCount} 在线`} tone={scopedClientCount && scopedOnlineClientCount < scopedClientCount ? 'warn' : 'ok'} />
        <WorkbenchRealtimeKpi up={scopedNetwork.up} down={scopedNetwork.down} />
        <WorkbenchKpi label={restrictedView ? '授权周期已用' : '周期总流量'} value={formatBytes(scopedNetwork.used)} note={restrictedView ? '仅统计有权限的 Client' : `↑${formatBytes(scopedNetwork.sent)} · ↓${formatBytes(scopedNetwork.recv)}`} tone="traffic" />
        {!restrictedView ? <WorkbenchKpi label="续费风险" value={`${renewalRiskCount}`} note={`30天内 ${renewalWithin30Count}`} tone={renewalRiskCount ? 'bad' : 'ok'} /> : null}
        {!restrictedView ? <WorkbenchKpi
          label="本月利润"
          value={monthlyFinance.available ? (profitVisible ? formatMoney(monthlyFinance.profitTotal, costCurrency) : '••••••') : '--'}
          note={monthlyFinance.available ? (profitVisible ? `收入 ${formatMoney(monthlyFinance.revenueTotal, costCurrency)}` : '收入 ••••••') : monthlyFinance.error || '财务数据加载中'}
          tone={monthlyFinance.available && monthlyFinance.profitTotal >= 0 ? 'profit' : 'bad'}
          action={(
            <Button
              type="text"
              size="small"
              className="workbench-profit-visibility"
              aria-label={profitVisible ? '隐藏利润' : '显示利润'}
              title={profitVisible ? '隐藏利润' : '显示利润'}
              icon={profitVisible ? <EyeOutlined /> : <EyeInvisibleOutlined />}
              onClick={toggleProfitVisibility}
            />
          )}
        /> : null}
      </div>

      <div className="admin-workbench-grid">
        <Card bordered={false} className="surface-card admin-workbench-card workbench-health-card">
          <div className="admin-workbench-card-title">
            <Text strong>系统健康</Text>
            <Tag color={healthAlertCount ? 'orange' : 'green'}>{healthAlertCount ? `告警 ${healthAlertCount}` : '运行平稳'}</Tag>
          </div>
          <div className="workbench-health-chart">
            <WorkbenchStatusDonut
              online={healthyRows.length}
              warning={warningRows.length}
              offline={offlineRows.length}
              total={totalAgents}
            />
            <div className="workbench-status-legend">
              <WorkbenchHealthLink tone="online" count={healthyRows.length} label="健康在线" onClick={() => onOpenHealthFilter('healthy')} />
              <WorkbenchHealthLink tone="warning" count={warningRows.length} label="负载/采集告警" onClick={() => onOpenHealthFilter('warning')} />
              <WorkbenchHealthLink tone="offline" count={offlineRows.length} label="离线 Client" onClick={() => onOpenHealthFilter('offline')} />
            </div>
          </div>
          <div className="workbench-health-foot">
            <span>CPU/内存高负载 {highLoadRows.length}</span>
            <span>x-ui 异常 {xuiErrorRows.length}</span>
          </div>
        </Card>

        <Card bordered={false} className="surface-card admin-workbench-card workbench-speed-card">
          <div className="admin-workbench-card-title">
            <Text strong>实时网速监控</Text>
            <Tag color="geekblue">总速 <AnimatedSpeedText value={scopedNetwork.up + scopedNetwork.down} /></Tag>
          </div>
          <WorkbenchSpeedTrend up={scopedNetwork.up} down={scopedNetwork.down} />
        </Card>

        <Card bordered={false} className="surface-card admin-workbench-card workbench-traffic-card">
          <div className="admin-workbench-card-title">
            <Text strong>流量水位 Top</Text>
            <Tag color={trafficRiskRows.length ? 'orange' : 'blue'}>{trafficRiskRows.length ? `风险 ${trafficRiskRows.length}` : '正常'}</Tag>
          </div>
          <WorkbenchTrafficList rows={trafficRows} maxValue={maxTraffic} restrictedView={restrictedView} onSelectAgent={onSelectAgent} />
        </Card>

        {!restrictedView ? <Card bordered={false} className="surface-card admin-workbench-card workbench-renewal-card">
          <div className="admin-workbench-card-title">
            <Text strong>续费 / 到期风险</Text>
            <Tag color={renewalRiskCount ? 'red' : 'green'}>{renewalRows.length ? `已配置 ${renewalRows.length}` : '未配置'}</Tag>
          </div>
          <WorkbenchRenewalList rows={renewalRiskRows} onSelectAgent={onSelectAgent} />
        </Card> : null}

        {!restrictedView ? <Card bordered={false} className="surface-card admin-workbench-card workbench-client-expiry-card">
          <div className="admin-workbench-card-title">
            <Text strong>客户端即将到期</Text>
            <Tag color={expiredClientCount ? 'red' : clientExpiryRows.length ? 'orange' : 'green'}>
              共 {clientExpiryRows.length} 个
            </Tag>
          </div>
          <WorkbenchClientExpiryList rows={clientExpiryRows} onSelectAgent={onSelectAgent} />
          {clientExpiryRows.length ? <div className="workbench-client-expiry-footer">
            <Text type="secondary">{clientExpirySummary}</Text>
            <Button size="small" onClick={() => setClientExpiryOpen(true)}>查看全部（{clientExpiryRows.length}）</Button>
          </div> : null}
        </Card> : null}

        {!restrictedView ? <Card bordered={false} className="surface-card admin-workbench-card workbench-finance-card">
          <div className="admin-workbench-card-title">
            <Text strong>财务月览</Text>
            <Tag color={!monthlyFinance.available ? 'default' : monthlyFinance.profitTotal >= 0 ? 'green' : 'red'}>{monthlyFinance.available ? costCurrency : '未就绪'}</Tag>
          </div>
          <div className={profitVisible ? 'workbench-finance-private' : 'workbench-finance-private workbench-finance-private-hidden'}>
            <div className="workbench-finance-total">
              <span>预计月利润</span>
              <strong className={monthlyFinance.profitTotal >= 0 ? 'finance-positive' : 'finance-negative'}>{monthlyFinance.available ? formatMoney(monthlyFinance.profitTotal, costCurrency) : '--'}</strong>
            </div>
            <div className="workbench-finance-bars">
              {financeBars.map((bar) => (
                <div className={`workbench-finance-bar workbench-finance-${bar.tone}`} key={bar.label}>
                  <div><span>{bar.label}</span><strong>{monthlyFinance.available ? formatMoney(bar.value, costCurrency) : '--'}</strong></div>
                  <i><b style={{ width: `${monthlyFinance.available ? percentOf(Math.abs(bar.value), maxFinance) : 0}%` }} /></i>
                </div>
              ))}
            </div>
            <div className="workbench-finance-foot">
              {monthlyFinance.available ? <>
                <span>成本 {monthlyFinance.costCount}/{agents.length}</span>
                <span>收费客户端 {monthlyFinance.revenueCount}</span>
                {monthlyFinance.excludedRevenueCount ? <span>未计收入 {monthlyFinance.excludedRevenueCount}</span> : null}
              </> : <span>{monthlyFinance.error || '财务数据加载中'}</span>}
            </div>
          </div>
        </Card> : null}

        {!restrictedView ? <Card bordered={false} className="surface-card admin-workbench-card workbench-ops-card">
          <div className="admin-workbench-card-title">
            <Text strong>业务拓扑与告警</Text>
            <Button size="small" type="link" onClick={onOpenTopology}>查看拓扑</Button>
          </div>
          <div className="workbench-ops-grid">
            <div><strong>{scopedNodeCount}</strong><span>入站节点</span></div>
            <div><strong>{dashboardView?.totals.outbound_count || 0}</strong><span>出站出口</span></div>
            <div><strong>{dashboardView?.totals.link_count || 0}</strong><span>匹配链路</span></div>
            <div><strong>{dashboardView?.totals.routing_rule_count || 0}</strong><span>路由规则</span></div>
          </div>
          <div className="workbench-alert-list">
            {alertRows.length ? alertRows.map((alert) => (
              <button
                type="button"
                key={alert.key}
                disabled={!alert.agentID}
                onClick={() => alert.agentID && onSelectAgent(alert.agentID)}
              >
                <Tag color={alert.level === 'bad' ? 'red' : 'orange'}>{alert.tag}</Tag>
                <span><strong>{alert.title}</strong><small>{alert.detail}</small></span>
              </button>
            )) : <div className="workbench-alert-empty">暂无离线、流量、续费或采集告警</div>}
          </div>
        </Card> : null}
      </div>
      {!restrictedView ? <Modal
        className="workbench-client-expiry-modal"
        title={`客户端到期提醒（${clientExpiryRows.length}）`}
        open={clientExpiryOpen}
        onCancel={() => setClientExpiryOpen(false)}
        footer={<Button onClick={() => setClientExpiryOpen(false)}>关闭</Button>}
        width={760}
        centered
      >
        <p className="workbench-client-expiry-summary">{clientExpirySummary}，按到期时间排序。点击条目查看所属 Client。</p>
        <WorkbenchClientExpiryList rows={clientExpiryRows} onSelectAgent={(agentID) => {
          setClientExpiryOpen(false)
          onSelectAgent(agentID)
        }} />
      </Modal> : null}
    </section>
  )
}

function WorkbenchHealthLink(props: {
  tone: 'online' | 'warning' | 'offline'
  count: number
  label: string
  onClick: () => void
}) {
  return (
    <button type="button" className="workbench-health-link" onClick={props.onClick} aria-label={`查看${props.label}`}>
      <i className={`legend-dot legend-${props.tone}`} />
      <strong>{props.count}</strong>
      <span>{props.label}</span>
    </button>
  )
}

function WorkbenchKpi(props: { label: string; value: string; note: string; tone: 'ok' | 'warn' | 'bad' | 'speed' | 'traffic' | 'profit'; action?: ReactNode }) {
  return (
    <div className={`admin-workbench-metric workbench-kpi-${props.tone}`}>
      <div className="workbench-kpi-label"><span>{props.label}</span>{props.action}</div>
      <strong>{props.value}</strong>
      <small>{props.note}</small>
    </div>
  )
}

function WorkbenchRealtimeKpi(props: { up: number; down: number }) {
  const animatedUp = useAnimatedNumber(props.up)
  const animatedDown = useAnimatedNumber(props.down)
  return (
    <div className="admin-workbench-metric workbench-kpi-speed">
      <span>实时总网速</span>
      <strong>{formatSpeed(animatedUp + animatedDown)}</strong>
      <small>↑{formatSpeed(animatedUp)} · ↓{formatSpeed(animatedDown)}</small>
    </div>
  )
}

function AnimatedSpeedText(props: { value: number }) {
  const animatedValue = useAnimatedNumber(props.value)
  return <>{formatSpeed(animatedValue)}</>
}

function WorkbenchStatusDonut(props: { online: number; warning: number; offline: number; total: number }) {
  const total = Math.max(0, props.total)
  const safeTotal = total || 1
  const onlineDeg = (props.online / safeTotal) * 360
  const warningDeg = (props.warning / safeTotal) * 360
  const onlinePercent = total ? ((props.online + props.warning) / total) * 100 : 0
  const background = total
    ? `conic-gradient(var(--blue) 0deg ${onlineDeg}deg, var(--amber) ${onlineDeg}deg ${onlineDeg + warningDeg}deg, var(--red) ${onlineDeg + warningDeg}deg 360deg)`
    : 'conic-gradient(var(--progress-bg) 0deg 360deg)'

  return (
    <div className="workbench-donut" style={{ background }} aria-label={`健康在线 ${props.online}，告警 ${props.warning}，离线 ${props.offline}`}>
      <div className="workbench-donut-center">
        <strong>{onlinePercent.toFixed(0)}%</strong>
        <span>在线率</span>
      </div>
    </div>
  )
}

function WorkbenchTrafficList(props: {
  rows: WorkbenchMetricRow[]
  maxValue: number
  restrictedView?: boolean
  onSelectAgent: (agentID: string) => void
}) {
  const { rows, maxValue, restrictedView = false, onSelectAgent } = props
  if (!rows.length) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无流量数据" />
  }
  return (
    <div className="workbench-progress-list">
      {rows.map((row) => {
        const percent = typeof row.traffic.total.percent === 'number' ? row.traffic.total.percent : percentOf(row.traffic.total.used, maxValue)
        const value = restrictedView
          ? `已用 ${formatBytes(row.traffic.total.used)}`
          : row.traffic.total.total > 0
            ? `${formatBytes(row.traffic.total.used)} / ${formatBytes(row.traffic.total.total)}`
            : `${formatBytes(row.traffic.total.used)} · 无上限`
        return (
          <button type="button" key={row.agent.agent_id} onClick={() => onSelectAgent(row.agent.agent_id)}>
            <MiniProgress
              label={agentWorkbenchName(row.agent)}
              value={value}
              percent={restrictedView ? percentOf(row.traffic.total.used, maxValue) : percent}
              level={restrictedView ? 'neutral' : row.traffic.total.total > 0 ? row.traffic.total.level : 'neutral'}
            />
          </button>
        )
      })}
    </div>
  )
}

function WorkbenchRenewalList(props: {
  rows: WorkbenchMetricRow[]
  onSelectAgent: (agentID: string) => void
}) {
  const { rows, onSelectAgent } = props
  if (!rows.length) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无 30 天内到期风险" />
  }
  return (
    <div className="workbench-progress-list workbench-renewal-list">
      {rows.map((row) => {
        const renewal = row.renewal
        if (!renewal) {
          return null
        }
        const riskPercent = row.renewalDays !== null && row.renewalDays < 0 ? 100 : clampMetricPercent(100 - renewal.percent)
        return (
          <button type="button" key={row.agent.agent_id} onClick={() => onSelectAgent(row.agent.agent_id)}>
            <MiniProgress
              label={agentWorkbenchName(row.agent)}
              value={`${renewal.remainingLabel} · ${renewal.endLabel}${renewal.autoRenew ? ' · 自动' : ''}`}
              percent={riskPercent}
              level={renewal.level}
            />
          </button>
        )
      })}
    </div>
  )
}

function WorkbenchClientExpiryList(props: {
  rows: ClientExpiryRow[]
  onSelectAgent: (agentID: string) => void
}) {
  const { rows, onSelectAgent } = props
  if (!rows.length) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={`未来 ${CLIENT_EXPIRY_WARNING_DAYS} 天暂无客户端到期`} />
  }
  return (
    <div className="workbench-client-expiry-list">
      {rows.map((row) => (
        <button type="button" key={row.key} onClick={() => onSelectAgent(row.agentID)}>
          <span className="workbench-client-expiry-identity">
            <strong title={row.clientName}>{row.clientName}</strong>
            <small title={`${row.agentName} · ${row.inboundName}`}>{row.agentName} · {row.inboundName}</small>
          </span>
          <span className="workbench-client-expiry-status">
            <Tag color={row.cycle === 'quarter' ? 'geekblue' : row.cycle === 'semiannual' ? 'cyan' : row.cycle === 'year' ? 'purple' : 'blue'}>{clientExpiryCycleLabel(row.cycle)}</Tag>
            <strong className={row.level === 'bad' ? 'finance-negative' : ''}>{clientExpiryRemainingLabel(row.remainingDays)}</strong>
            <small>{row.expiryDate}</small>
          </span>
        </button>
      ))}
    </div>
  )
}

function buildWorkbenchAlerts(options: {
  offlineRows: WorkbenchMetricRow[]
  xuiErrorRows: WorkbenchMetricRow[]
  highLoadRows: WorkbenchMetricRow[]
  trafficRiskRows: WorkbenchMetricRow[]
  renewalRows: WorkbenchMetricRow[]
}): WorkbenchAlertRow[] {
  const alerts: WorkbenchAlertRow[] = []
  for (const row of options.offlineRows) {
    alerts.push({
      key: `offline:${row.agent.agent_id}`,
      agentID: row.agent.agent_id,
      level: 'bad',
      tag: '离线',
      title: agentWorkbenchName(row.agent),
      detail: row.status.label,
    })
  }
  for (const row of options.xuiErrorRows) {
    alerts.push({
      key: `xui:${row.agent.agent_id}`,
      agentID: row.agent.agent_id,
      level: 'warn',
      tag: '采集',
      title: agentWorkbenchName(row.agent),
      detail: row.agent.summary.last_collection_err || 'x-ui 采集异常',
    })
  }
  for (const row of options.trafficRiskRows) {
    alerts.push({
      key: `traffic:${row.agent.agent_id}`,
      agentID: row.agent.agent_id,
      level: row.traffic.total.level === 'bad' ? 'bad' : 'warn',
      tag: '流量',
      title: agentWorkbenchName(row.agent),
      detail: row.traffic.total.label,
    })
  }
  for (const row of options.renewalRows) {
    if (!row.renewal || row.renewal.level === 'ok') {
      continue
    }
    alerts.push({
      key: `renewal:${row.agent.agent_id}`,
      agentID: row.agent.agent_id,
      level: row.renewal.level === 'bad' ? 'bad' : 'warn',
      tag: '续费',
      title: agentWorkbenchName(row.agent),
      detail: `${row.renewal.remainingLabel} · ${row.renewal.endLabel}`,
    })
  }
  for (const row of options.highLoadRows) {
    alerts.push({
      key: `load:${row.agent.agent_id}`,
      agentID: row.agent.agent_id,
      level: 'warn',
      tag: '负载',
      title: agentWorkbenchName(row.agent),
      detail: `CPU ${formatPercent(Number(row.agent.summary.cpu || 0))} · 内存 ${formatPercent(row.memoryPercent)}`,
    })
  }
  return alerts.sort((left, right) => alertSeverity(right) - alertSeverity(left))
}

function alertSeverity(alert: WorkbenchAlertRow): number {
  return alert.level === 'bad' ? 2 : 1
}

function renewalDaysFromStatus(status: ReturnType<typeof calculateRenewalStatus>): number | null {
  if (!status) {
    return null
  }
  if (status.remainingLabel === '今天到期') {
    return 0
  }
  const remaining = status.remainingLabel.match(/^剩余\s+(\d+)\s+天/)
  if (remaining) {
    return Number(remaining[1])
  }
  const overdue = status.remainingLabel.match(/^已过期\s+(\d+)\s+天/)
  if (overdue) {
    return -Number(overdue[1])
  }
  return null
}

function renewalSortValue(row: WorkbenchMetricRow): number {
  if (row.renewalDays !== null) {
    return row.renewalDays
  }
  return row.renewal?.level === 'bad' ? -1 : row.renewal?.level === 'warn' ? 7 : Number.MAX_SAFE_INTEGER
}

function trafficRiskScore(row: WorkbenchMetricRow, maxTraffic: number): number {
  if (typeof row.traffic.total.percent === 'number') {
    return row.traffic.total.percent + (row.traffic.total.level === 'bad' ? 200 : row.traffic.total.level === 'warn' ? 100 : 0)
  }
  return percentOf(row.traffic.total.used, maxTraffic)
}

function agentWorkbenchName(agent: DashboardAgentView): string {
  return agent.agent_name || agent.summary.hostname || agent.agent_id
}

function percentOf(value: number, maxValue: number): number {
  if (!value || !maxValue) {
    return 0
  }
  return clampMetricPercent((value / maxValue) * 100)
}
