import { useEffect, useMemo, useRef, useState } from 'react'
import { Alert, Button, Card, Empty, List, Select, Space, Table, Tabs, Tag, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { ApartmentOutlined, BarsOutlined, CloudServerOutlined, ReloadOutlined } from '@ant-design/icons'
import { Line } from '@ant-design/plots'
import type { LineConfig } from '@ant-design/plots'

import type { AgentListItem, ClientChainView, CustomerAdminView, CustomerAssignment, DashboardAgentView, DashboardTagView, GlobalDashboardView } from '../types'
import type { AgentViewMode } from '../lib/appHelpers'
import type { CurrencyCode, ExchangeRatesState, MonthlyFinanceCostDetail, MonthlyFinancePaymentInfo, MonthlyFinanceRevenueDetail, MonthlyFinanceSummary } from '../lib/currency'
import { buildMonthlyFinanceCostDetails, buildMonthlyFinanceRevenueDetails, formatMoney } from '../lib/currency'
import {
  agentCountryCode,
  agentDisplayStatus,
  calculateRenewalStatus,
  countryFlag,
  formatAgentLocation,
  formatDateTime,
  fetchJSON,
  tagChipStyle,
  xrayIssueLabel,
} from '../lib/appHelpers'
import {
  calculateMemoryPercent,
  calculateTrafficStatus,
  clampMetricPercent,
  formatBandwidth,
  formatBytes,
  formatMem,
  formatPercent,
  formatSpeed,
  metricLevel,
} from '../lib/traffic'
import { MiniProgress } from './MiniProgress'

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

type FinanceRevenueGroupRow = {
  key: string
  label: string
  detail: string
  clients: string[]
  count: number
  monthlyAmount: number
}

type SpeedHistoryPoint = {
  up: number
  down: number
  time: number
  samples: number
}

type SpeedChartPoint = {
  name: '上传' | '下载'
  speed: number
  time: string
}

type SpeedAxisInfo = {
  maxValue: number
  unit: string
  divisor: number
}

const SPEED_BUCKET_MS = 5 * 60 * 1000
const SPEED_HISTORY_POINTS = 24

export function AdminWorkbenchDashboard(props: {
  agents: DashboardAgentView[]
  dashboardView: GlobalDashboardView | null
  selectedTag: string
  scopedNetwork: { sent: number; recv: number; up: number; down: number }
  monthlyFinance: MonthlyFinanceSummary
  costCurrency: CurrencyCode
  restrictedView?: boolean
  onSelectAgent: (agentID: string) => void
  onOpenTopology: () => void
}) {
  const { agents, dashboardView, selectedTag, scopedNetwork, monthlyFinance, costCurrency, restrictedView = false, onSelectAgent, onOpenTopology } = props
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

  const selectedTagView = selectedTag ? dashboardView?.tags.find((tag) => tag.tag === selectedTag) : undefined
  const scopedClientCount = selectedTagView?.client_count ?? dashboardView?.totals.client_count ?? 0
  const scopedOnlineClientCount = selectedTagView?.online_client_count ?? dashboardView?.totals.online_client_count ?? 0
  const scopedNodeCount = selectedTagView?.node_count ?? dashboardView?.totals.node_count ?? 0
  const totalAgents = agents.length
  const onlineRows = statusRows.filter((row) => row.status.level !== 'bad')
  const offlineRows = statusRows.filter((row) => row.status.level === 'bad')
  const highLoadRows = statusRows.filter((row) => row.status.level !== 'bad' && (row.memoryPercent >= 75 || Number(row.agent.summary.cpu || 0) >= 75))
  const xuiErrorRows = statusRows.filter((row) => Boolean(row.agent.summary.last_collection_err))
  const warningRows = statusRows.filter((row) => row.status.level !== 'bad' && (
    row.status.level === 'warn' ||
    row.memoryPercent >= 75 ||
    Number(row.agent.summary.cpu || 0) >= 75 ||
    Boolean(row.agent.summary.last_collection_err)
  ))
  const healthyRows = onlineRows.filter((row) => !warningRows.includes(row))
  const onlinePercent = totalAgents ? (onlineRows.length / totalAgents) * 100 : 0
  const clientOnlinePercent = scopedClientCount ? (scopedOnlineClientCount / scopedClientCount) * 100 : 0

  const maxTraffic = Math.max(1, ...statusRows.map((row) => row.traffic.total.used))
  const trafficRows = [...statusRows]
    .sort((left, right) => trafficRiskScore(right, maxTraffic) - trafficRiskScore(left, maxTraffic))
    .slice(0, 5)
  const trafficRiskRows = statusRows.filter((row) => row.traffic.total.total > 0 && row.traffic.total.level !== 'ok')

  const renewalRows = statusRows
    .filter((row) => row.renewal)
    .sort((left, right) => renewalSortValue(left) - renewalSortValue(right))
  const renewalRiskRows = renewalRows
    .filter((row) => row.renewal?.level !== 'ok' || (row.renewalDays !== null && row.renewalDays <= 30))
    .slice(0, 5)
  const renewalRiskCount = renewalRows.filter((row) => row.renewal?.level !== 'ok').length
  const renewalWithin30Count = renewalRows.filter((row) => row.renewalDays !== null && row.renewalDays <= 30).length

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
  const currentScope = selectedTag || '全部服务器'

  return (
    <section className="admin-workbench">
      <div className="admin-workbench-strip">
        <div className="admin-workbench-heading">
          <Text type="secondary">运营驾驶舱</Text>
          <Typography.Title level={3}>{currentScope}</Typography.Title>
          <small>{dashboardView ? `数据时间 ${formatDateTime(dashboardView.generated_at)}` : '等待全局概览数据'}</small>
        </div>
        <WorkbenchKpi label="服务器在线率" value={`${onlinePercent.toFixed(0)}%`} note={`${onlineRows.length}/${totalAgents} 在线`} tone={offlineRows.length ? 'warn' : 'ok'} />
        <WorkbenchKpi label="客户端在线率" value={`${clientOnlinePercent.toFixed(0)}%`} note={`${scopedOnlineClientCount}/${scopedClientCount} 在线`} tone={scopedClientCount && scopedOnlineClientCount < scopedClientCount ? 'warn' : 'ok'} />
        <WorkbenchRealtimeKpi up={scopedNetwork.up} down={scopedNetwork.down} />
        <WorkbenchKpi label={restrictedView ? '授权周期已用' : '周期总流量'} value={formatBytes(scopedNetwork.sent + scopedNetwork.recv)} note={restrictedView ? '仅统计有权限的 Client' : `↑${formatBytes(scopedNetwork.sent)} · ↓${formatBytes(scopedNetwork.recv)}`} tone="traffic" />
        {!restrictedView ? <WorkbenchKpi label="续费风险" value={`${renewalRiskCount}`} note={`30天内 ${renewalWithin30Count}`} tone={renewalRiskCount ? 'bad' : 'ok'} /> : null}
        {!restrictedView ? <WorkbenchKpi label="本月利润" value={formatMoney(monthlyFinance.profitTotal, costCurrency)} note={`收入 ${formatMoney(monthlyFinance.revenueTotal, costCurrency)}`} tone={monthlyFinance.profitTotal >= 0 ? 'profit' : 'bad'} /> : null}
        <Button size="small" onClick={onOpenTopology} icon={<ApartmentOutlined />}>拓扑</Button>
      </div>

      <div className="admin-workbench-grid">
        <Card bordered={false} className="surface-card admin-workbench-card workbench-health-card">
          <div className="admin-workbench-card-title">
            <Text strong>系统健康</Text>
            <Tag color={alertRows.length ? 'orange' : 'green'}>{alertRows.length ? `告警 ${alertRows.length}` : '运行平稳'}</Tag>
          </div>
          <div className="workbench-health-chart">
            <WorkbenchStatusDonut
              online={healthyRows.length}
              warning={warningRows.length}
              offline={offlineRows.length}
              total={totalAgents}
            />
            <div className="workbench-status-legend">
              <div><i className="legend-dot legend-online" /><strong>{healthyRows.length}</strong><span>健康在线</span></div>
              <div><i className="legend-dot legend-warning" /><strong>{warningRows.length}</strong><span>负载/采集告警</span></div>
              <div><i className="legend-dot legend-offline" /><strong>{offlineRows.length}</strong><span>离线 Client</span></div>
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

        {!restrictedView ? <Card bordered={false} className="surface-card admin-workbench-card workbench-finance-card">
          <div className="admin-workbench-card-title">
            <Text strong>财务月览</Text>
            <Tag color={monthlyFinance.profitTotal >= 0 ? 'green' : 'red'}>{costCurrency}</Tag>
          </div>
          <div className="workbench-finance-total">
            <span>预计月利润</span>
            <strong className={monthlyFinance.profitTotal >= 0 ? 'finance-positive' : 'finance-negative'}>{formatMoney(monthlyFinance.profitTotal, costCurrency)}</strong>
          </div>
          <div className="workbench-finance-bars">
            {financeBars.map((bar) => (
              <div className={`workbench-finance-bar workbench-finance-${bar.tone}`} key={bar.label}>
                <div><span>{bar.label}</span><strong>{formatMoney(bar.value, costCurrency)}</strong></div>
                <i><b style={{ width: `${percentOf(Math.abs(bar.value), maxFinance)}%` }} /></i>
              </div>
            ))}
          </div>
          <div className="workbench-finance-foot">
            <span>成本 Client VPS {monthlyFinance.costCount}</span>
            <span>收费客户端 {monthlyFinance.revenueCount}</span>
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
    </section>
  )
}

function WorkbenchKpi(props: { label: string; value: string; note: string; tone: 'ok' | 'warn' | 'bad' | 'speed' | 'traffic' | 'profit' }) {
  return (
    <div className={`admin-workbench-metric workbench-kpi-${props.tone}`}>
      <span>{props.label}</span>
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

function WorkbenchSpeedTrend(props: { up: number; down: number }) {
  const { up, down } = props
  const animatedUp = useAnimatedNumber(up)
  const animatedDown = useAnimatedNumber(down)
  const [history, setHistory] = useState<SpeedHistoryPoint[]>(() => createInitialSpeedHistory(up, down))
  const animatedTotal = animatedUp + animatedDown
  const chartRows = history.map((point, index) => (
    index === history.length - 1 && point.time === speedBucketStart(Date.now())
      ? { ...point, up: animatedUp, down: animatedDown }
      : point
  ))
  const axis = speedAxisInfo(chartRows)
  const chartData = chartRows.flatMap<SpeedChartPoint>((point) => {
    const time = formatChartTime(point.time)
    return [
      { name: '上传', speed: point.up / axis.divisor, time },
      { name: '下载', speed: point.down / axis.divisor, time },
    ]
  })
  const chartConfig: LineConfig = {
    data: chartData,
    xField: 'time',
    yField: 'speed',
    colorField: 'name',
    shapeField: 'smooth',
    autoFit: true,
    padding: [18, 16, 34, 48],
    scale: {
      y: {
        domain: [0, axis.maxValue / axis.divisor],
        nice: false,
      },
      color: {
        range: ['#0f9f8f', '#f97316'],
      },
    },
    axis: {
      x: {
        title: false,
        tick: false,
        labelAutoHide: true,
        labelAutoRotate: false,
        labelFill: 'rgba(100, 116, 139, 0.82)',
        labelFontSize: 10,
        line: true,
        lineStroke: 'rgba(148, 163, 184, 0.34)',
      },
      y: {
        title: false,
        labelFormatter: (value: string | number) => formatChartAxisValue(Number(value), axis.unit),
        labelFill: 'rgba(100, 116, 139, 0.82)',
        labelFontSize: 10,
        grid: true,
        gridStroke: 'rgba(148, 163, 184, 0.18)',
        gridLineDash: [3, 6],
      },
    },
    legend: false,
    tooltip: {
      title: 'time',
      items: [
        {
          channel: 'y',
          valueFormatter: (value: number) => formatChartAxisValue(value, axis.unit),
        },
      ],
    },
    style: {
      lineWidth: 1.2,
      lineCap: 'round',
      lineJoin: 'round',
    },
    point: {
      shapeField: 'circle',
      sizeField: 2.2,
      style: {
        stroke: 'rgba(255,255,255,0.82)',
        lineWidth: 1,
      },
    },
    interaction: {
      tooltip: {
        shared: true,
        crosshairs: true,
      },
    },
  }

  useEffect(() => {
    setHistory((current) => {
      const now = Date.now()
      const bucket = speedBucketStart(now)
      const safeUp = Number.isFinite(up) ? Math.max(0, up) : 0
      const safeDown = Number.isFinite(down) ? Math.max(0, down) : 0
      const last = current[current.length - 1]
      if (last?.time === bucket) {
        const samples = Math.max(1, last.samples || 1)
        const nextSamples = samples + 1
        return [
          ...current.slice(0, -1),
          {
            time: bucket,
            samples: nextSamples,
            up: ((last.up * samples) + safeUp) / nextSamples,
            down: ((last.down * samples) + safeDown) / nextSamples,
          },
        ]
      }
      return [...current, { up: safeUp, down: safeDown, time: bucket, samples: 1 }].slice(-SPEED_HISTORY_POINTS)
    })
  }, [down, up])

  return (
    <div className="workbench-speed-trend">
      <div className="workbench-speed-current">
        <div className="workbench-speed-current-total">
          <span>当前总网速</span>
          <strong>{formatSpeed(animatedTotal)}</strong>
        </div>
        <div className="workbench-speed-legend">
          <span className="workbench-speed-upload-pill"><i className="legend-dot legend-upload" />上传 {formatSpeed(animatedUp)}</span>
          <span className="workbench-speed-download-pill"><i className="legend-dot legend-download" />下载 {formatSpeed(animatedDown)}</span>
        </div>
      </div>
      <div className="workbench-speed-chart" aria-label={`实时网速，上传 ${formatSpeed(animatedUp)}，下载 ${formatSpeed(animatedDown)}`}>
        <Line {...chartConfig} />
      </div>
      <div className="workbench-speed-scale">
        <span>最近 {SPEED_HISTORY_POINTS} 个 5 分钟均值</span>
        <span>纵轴单位 {axis.unit}/s · 峰值 {formatSpeed(axis.maxValue)}</span>
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

function createInitialSpeedHistory(up: number, down: number): SpeedHistoryPoint[] {
  const currentBucket = speedBucketStart(Date.now())
  const safeUp = Number.isFinite(up) ? Math.max(0, up) : 0
  const safeDown = Number.isFinite(down) ? Math.max(0, down) : 0
  return Array.from({ length: SPEED_HISTORY_POINTS }, (_, index) => ({
    up: safeUp,
    down: safeDown,
    samples: 1,
    time: currentBucket - (SPEED_HISTORY_POINTS - 1 - index) * SPEED_BUCKET_MS,
  }))
}

function speedBucketStart(value: number): number {
  return Math.floor(value / SPEED_BUCKET_MS) * SPEED_BUCKET_MS
}

function speedAxisInfo(rows: SpeedHistoryPoint[]): SpeedAxisInfo {
  const rawMax = Math.max(1, ...rows.flatMap((row) => [row.up, row.down]))
  const unitInfo = speedUnitInfo(rawMax)
  const niceMax = niceAxisMax(rawMax / unitInfo.divisor) * unitInfo.divisor
  return {
    maxValue: niceMax,
    unit: unitInfo.unit,
    divisor: unitInfo.divisor,
  }
}

function speedUnitInfo(maxValue: number): { unit: string; divisor: number } {
  if (maxValue >= 1024 * 1024 * 1024) {
    return { unit: 'GB', divisor: 1024 * 1024 * 1024 }
  }
  if (maxValue >= 1024 * 1024) {
    return { unit: 'MB', divisor: 1024 * 1024 }
  }
  if (maxValue >= 1024) {
    return { unit: 'KB', divisor: 1024 }
  }
  return { unit: 'B', divisor: 1 }
}

function niceAxisMax(value: number): number {
  if (value <= 0) {
    return 1
  }
  const exponent = Math.floor(Math.log10(value))
  const base = 10 ** exponent
  const normalized = value / base
  const nice = normalized <= 1 ? 1 : normalized <= 2 ? 2 : normalized <= 5 ? 5 : 10
  return nice * base
}

function formatChartAxisValue(value: number, unit: string): string {
  const safeValue = Number.isFinite(value) ? value : 0
  if (safeValue === 0) {
    return `0 ${unit}/s`
  }
  return `${safeValue >= 10 ? safeValue.toFixed(0) : safeValue.toFixed(1)} ${unit}/s`
}

function formatChartTime(value: number): string {
  return new Intl.DateTimeFormat('zh-CN', {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  }).format(new Date(value))
}

function agentWorkbenchName(agent: DashboardAgentView): string {
  return agent.agent_name || agent.summary.hostname || agent.agent_id
}

function useAnimatedNumber(target: number, duration = 950): number {
  const safeTarget = Number.isFinite(target) ? Math.max(0, target) : 0
  const [value, setValue] = useState(safeTarget)
  const currentRef = useRef(safeTarget)

  useEffect(() => {
    const from = currentRef.current
    const delta = safeTarget - from
    if (Math.abs(delta) < 0.5) {
      currentRef.current = safeTarget
      setValue(safeTarget)
      return undefined
    }

    let frame = 0
    const startedAt = performance.now()
    const tick = (now: number) => {
      const progress = Math.min(1, Math.max(0, (now - startedAt) / duration))
      const eased = 1 - Math.pow(1 - progress, 3)
      const next = from + delta * eased
      currentRef.current = next
      setValue(next)
      if (progress < 1) {
        frame = requestAnimationFrame(tick)
      } else {
        currentRef.current = safeTarget
        setValue(safeTarget)
      }
    }

    frame = requestAnimationFrame(tick)
    return () => {
      cancelAnimationFrame(frame)
    }
  }, [duration, safeTarget])

  return value
}

function percentOf(value: number, maxValue: number): number {
  if (!value || !maxValue) {
    return 0
  }
  return clampMetricPercent((value / maxValue) * 100)
}

export function OverviewSummaryCard(props: {
  dashboardView: GlobalDashboardView | null
  scopedAgentCount: number
  scopedNodeCount: number
  onlineAgentCount: number
  offlineAgentCount: number
  xuiErrorAgentCount: number
  scopedNetwork: { sent: number; recv: number; up: number; down: number }
  costCurrency: CurrencyCode
  currencyOptions: CurrencyCode[]
  monthlyFinance: { profitTotal: number; revenueTotal: number; costTotal: number }
  financeAgents: AgentListItem[]
  financeChains: ClientChainView[]
  exchangeRates: ExchangeRatesState
  selectedTag: string
  currentAgentLabel: string
  currentIPv4: string
  compact?: boolean
  restrictedView?: boolean
  onCostCurrencyChange: (currency: CurrencyCode) => void
}) {
  const {
    dashboardView,
    scopedAgentCount,
    scopedNodeCount,
    onlineAgentCount,
    offlineAgentCount,
    xuiErrorAgentCount,
    scopedNetwork,
    costCurrency,
    currencyOptions,
    monthlyFinance,
    financeAgents,
    financeChains,
    exchangeRates,
    selectedTag,
    currentAgentLabel,
    currentIPv4,
    compact,
    restrictedView = false,
    onCostCurrencyChange,
  } = props
  const [financeDetailOpen, setFinanceDetailOpen] = useState(false)
  const [customerRows, setCustomerRows] = useState<CustomerAdminView[]>([])
  const [customerRowsLoading, setCustomerRowsLoading] = useState(false)
  const costRows = useMemo(
    () => buildMonthlyFinanceCostDetails(financeAgents, costCurrency, exchangeRates),
    [costCurrency, exchangeRates, financeAgents],
  )
  const revenueRows = useMemo(
    () => buildMonthlyFinanceRevenueDetails(financeAgents, financeChains, costCurrency, exchangeRates),
    [costCurrency, exchangeRates, financeAgents, financeChains],
  )
  const customerRevenueRows = useMemo(
    () => buildCustomerRevenueRows(revenueRows, customerRows),
    [customerRows, revenueRows],
  )
  const nodeRevenueRows = useMemo(
    () => buildNodeRevenueRows(revenueRows),
    [revenueRows],
  )

  useEffect(() => {
    if (restrictedView || !financeDetailOpen || customerRows.length || customerRowsLoading) {
      return
    }
    let cancelled = false
    setCustomerRowsLoading(true)
    void fetchJSON<CustomerAdminView[]>('/api/v1/admin/customers')
      .then((rows) => {
        if (!cancelled) {
          setCustomerRows(rows)
        }
      })
      .catch(() => {
        if (!cancelled) {
          setCustomerRows([])
        }
      })
      .finally(() => {
        if (!cancelled) {
          setCustomerRowsLoading(false)
        }
      })
    return () => {
      cancelled = true
    }
  }, [customerRows.length, customerRowsLoading, financeDetailOpen, restrictedView])
  const costColumns: ColumnsType<MonthlyFinanceCostDetail> = [
    {
      title: 'Client VPS',
      dataIndex: 'agentName',
      key: 'agentName',
      width: 220,
      render: (_, record) => (
        <Space direction="vertical" size={2}>
          <Text strong>{record.agentName}</Text>
          <Text type="secondary">{record.agentID}</Text>
          {record.tags.length ? <Space size={[4, 4]} wrap>{record.tags.map((tag) => <Tag key={tag}>{tag}</Tag>)}</Space> : null}
        </Space>
      ),
    },
    {
      title: '原始成本',
      key: 'sourceCost',
      width: 150,
      render: (_, record) => record.amount > 0 ? formatMoney(record.amount, record.currency) : <Tag>未设置</Tag>,
    },
    {
      title: '周期',
      dataIndex: 'cycle',
      key: 'cycle',
      width: 90,
      render: (cycle: MonthlyFinanceCostDetail['cycle']) => cycleLabel(cycle),
    },
    {
      title: '缴费日',
      key: 'payment',
      width: 160,
      render: (_, record) => renderPaymentInfo(record.payment),
    },
    {
      title: `折算月成本 (${costCurrency})`,
      key: 'monthlyAmount',
      width: 170,
      align: 'right',
      render: (_, record) => renderMonthlyAmount(record.monthlyAmount, costCurrency, record.amount > 0),
    },
  ]
  const revenueColumns: ColumnsType<MonthlyFinanceRevenueDetail> = [
    {
      title: '客户端',
      key: 'client',
      width: 260,
      render: (_, record) => (
        <Space direction="vertical" size={2}>
          <Text strong>{record.clientLabel}</Text>
          <Text type="secondary">{record.clientRemark || record.inboundTag || '未备注'}</Text>
          {record.source === 'billing' ? <Tag color="blue">仅收费配置</Tag> : null}
        </Space>
      ),
    },
    {
      title: '所属 Client VPS',
      key: 'agent',
      width: 180,
      render: (_, record) => (
        <Space direction="vertical" size={0}>
          <Text>{record.agentName}</Text>
          <Text type="secondary">{record.agentID}</Text>
        </Space>
      ),
    },
    {
      title: '入站',
      dataIndex: 'inboundTag',
      key: 'inboundTag',
      width: 140,
      render: (value: string) => value || '-',
    },
    {
      title: '原始收入',
      key: 'sourceRevenue',
      width: 150,
      render: (_, record) => record.amount > 0 ? formatMoney(record.amount, record.currency) : <Tag>未设置</Tag>,
    },
    {
      title: '周期',
      dataIndex: 'cycle',
      key: 'cycle',
      width: 90,
      render: (cycle: MonthlyFinanceRevenueDetail['cycle']) => cycleLabel(cycle),
    },
    {
      title: '缴费日',
      key: 'payment',
      width: 160,
      render: (_, record) => renderPaymentInfo(record.payment),
    },
    {
      title: `折算月收入 (${costCurrency})`,
      key: 'monthlyAmount',
      width: 170,
      align: 'right',
      render: (_, record) => renderMonthlyAmount(record.monthlyAmount, costCurrency, record.amount > 0),
    },
  ]
  const revenueGroupColumns: ColumnsType<FinanceRevenueGroupRow> = [
    {
      title: '维度',
      key: 'label',
      width: 260,
      render: (_, record) => (
        <Space direction="vertical" size={2}>
          <Text strong>{record.label}</Text>
          <Text type="secondary">{record.detail}</Text>
        </Space>
      ),
    },
    {
      title: '计费项',
      dataIndex: 'count',
      key: 'count',
      width: 120,
      render: (value: number) => `${value} 项`,
    },
    {
      title: `折算月收入 (${costCurrency})`,
      key: 'monthlyAmount',
      width: 180,
      align: 'right',
      render: (_, record) => formatMoney(record.monthlyAmount, costCurrency),
    },
  ]
  const nodeRevenueColumns: ColumnsType<FinanceRevenueGroupRow> = [
    {
      title: '节点',
      key: 'label',
      width: 260,
      render: (_, record) => (
        <Space direction="vertical" size={2}>
          <Text strong>{record.label}</Text>
          <Text type="secondary">{record.detail}</Text>
        </Space>
      ),
    },
    {
      title: 'Client 信息',
      key: 'clients',
      width: 320,
      render: (_, record) => (
        <Space size={[4, 4]} wrap>
          {record.clients.slice(0, 4).map((client) => <Tag key={client}>{client}</Tag>)}
          {record.clients.length > 4 ? <Tag>+{record.clients.length - 4}</Tag> : null}
        </Space>
      ),
    },
    {
      title: '计费项',
      dataIndex: 'count',
      key: 'count',
      width: 120,
      render: (value: number) => `${value} 项`,
    },
    {
      title: `折算月收入 (${costCurrency})`,
      key: 'monthlyAmount',
      width: 180,
      align: 'right',
      render: (_, record) => formatMoney(record.monthlyAmount, costCurrency),
    },
  ]

  return (
    <Card className={`surface-card summary-card${compact ? ' compact-summary-card' : ''}`} bordered={false}>
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
                <div className="overview-stat-title">Client 状态</div>
                <div className="overview-network-total">
                  <span className="network-down">在线 {onlineAgentCount}</span>
                  <span className="network-up">离线 {offlineAgentCount}</span>
                </div>
                <div className="overview-stat-foot">
                  {restrictedView ? `仅显示已授权 Client · 链路 ${dashboardView.totals.chain_count}` : `x-ui 异常 ${xuiErrorAgentCount} · 出站 ${dashboardView.totals.outbound_count} · 规则 ${dashboardView.totals.routing_rule_count}`}
                </div>
              </section>
              <section className="overview-stat-card overview-network-card">
                <div className="overview-stat-title">{restrictedView ? '周期已用流量' : '本周期流量'}</div>
                <div className="overview-network-total">
                  {restrictedView ? <span className="network-up">{formatBytes(scopedNetwork.sent + scopedNetwork.recv)}</span> : <span className="network-up">↑{formatBytes(scopedNetwork.sent)}</span>}
                  {!restrictedView ? <span className="network-down">↓{formatBytes(scopedNetwork.recv)}</span> : null}
                </div>
                {!restrictedView ? <div className="overview-network-speed">
                  <span>⬆ {formatSpeed(scopedNetwork.up)}</span>
                  <span>⬇ {formatSpeed(scopedNetwork.down)}</span>
                </div> : <div className="overview-stat-foot">不展示总额度，仅展示授权范围内已用总量</div>}
              </section>
              {!restrictedView ? <section
                className="overview-stat-card overview-cost-card overview-cost-card-clickable"
                role="button"
                tabIndex={0}
                aria-expanded={financeDetailOpen}
                title={financeDetailOpen ? '点击收起成本与收入明细' : '点击查看成本与收入明细'}
                onClick={() => setFinanceDetailOpen((open) => !open)}
                onKeyDown={(event) => {
                  if (event.key === 'Enter' || event.key === ' ') {
                    event.preventDefault()
                    setFinanceDetailOpen((open) => !open)
                  }
                }}
              >
                <div className="overview-stat-title overview-cost-title">
                  <span>财务月览</span>
                  <Select
                    className="overview-currency-select"
                    size="small"
                    value={costCurrency}
                    options={currencyOptions.map((currency) => ({ value: currency, label: currency }))}
                    popupMatchSelectWidth={96}
                    onClick={(event) => event.stopPropagation()}
                    onKeyDown={(event) => event.stopPropagation()}
                    onChange={(value) => onCostCurrencyChange(value as CurrencyCode)}
                  />
                </div>
                <div className="overview-cost-value">{formatMoney(monthlyFinance.profitTotal, costCurrency)}</div>
                <div className="overview-stat-foot">
                  营收 {formatMoney(monthlyFinance.revenueTotal, costCurrency)} · 花销 {formatMoney(monthlyFinance.costTotal, costCurrency)}
                </div>
                {exchangeRates.error ? <div className="overview-stat-foot">汇率加载失败：{exchangeRates.error}</div> : null}
              </section> : null}
            </div>
            <div className="overview-summary-strip">
              <span>已匹配链路 · {dashboardView.totals.link_count}</span>
              <span>前端客户端链路 · {dashboardView.totals.chain_count}</span>
              <span>标签视图 · {selectedTag || '全部'}</span>
              <span>计算时间 · {formatDateTime(dashboardView.generated_at)}</span>
              <span>当前详情节点 · {currentAgentLabel || '-'}</span>
              {!restrictedView ? <span>当前节点 IPv4 · {currentIPv4 || '-'}</span> : null}
            </div>
            {!restrictedView && financeDetailOpen ? (
              <div className="finance-detail-panel">
                <div className="finance-detail-head">
                  <div>
                    <Text strong>财务月览明细</Text>
                    <Text type="secondary">成本按 Client VPS，收入按已配置收费的客户端统计</Text>
                  </div>
                  <Button size="small" onClick={() => setFinanceDetailOpen(false)}>收起</Button>
                </div>
                <div className="finance-detail-summary">
                  <div>
                    <span>月利润</span>
                    <strong className={monthlyFinance.profitTotal >= 0 ? 'finance-positive' : 'finance-negative'}>{formatMoney(monthlyFinance.profitTotal, costCurrency)}</strong>
                  </div>
                  <div>
                    <span>月收入</span>
                    <strong>{formatMoney(monthlyFinance.revenueTotal, costCurrency)}</strong>
                  </div>
                  <div>
                    <span>月成本</span>
                    <strong>{formatMoney(monthlyFinance.costTotal, costCurrency)}</strong>
                  </div>
                  <div>
                    <span>范围</span>
                    <strong>{selectedTag || '全部标签'}</strong>
                  </div>
                </div>
                <Tabs
                  items={[
                    {
                      key: 'cost',
                      label: `成本 · Client VPS ${costRows.length}`,
                      children: (
                        <Table
                          size="small"
                          rowKey="key"
                          columns={costColumns}
                          dataSource={costRows}
                          pagination={{ pageSize: 8, showSizeChanger: false }}
                          scroll={{ x: 880 }}
                        />
                      ),
                    },
                    {
                      key: 'revenue',
                      label: `收入 · 客户端 ${revenueRows.length}`,
                      children: (
                        <Table
                          size="small"
                          rowKey="key"
                          columns={revenueColumns}
                          dataSource={revenueRows}
                          pagination={{ pageSize: 8, showSizeChanger: false }}
                          scroll={{ x: 1120 }}
                        />
                      ),
                    },
                    {
                      key: 'revenue-customer',
                      label: `收入 · 用户 ${customerRevenueRows.length}`,
                      children: (
                        <Table
                          size="small"
                          rowKey="key"
                          columns={revenueGroupColumns}
                          dataSource={customerRevenueRows}
                          loading={customerRowsLoading}
                          pagination={{ pageSize: 8, showSizeChanger: false }}
                          scroll={{ x: 620 }}
                        />
                      ),
                    },
                    {
                      key: 'revenue-node',
                      label: `收入 · 节点 ${nodeRevenueRows.length}`,
                      children: (
                        <Table
                          size="small"
                          rowKey="key"
                          columns={nodeRevenueColumns}
                          dataSource={nodeRevenueRows}
                          pagination={{ pageSize: 8, showSizeChanger: false }}
                          scroll={{ x: 880 }}
                        />
                      ),
                    },
                  ]}
                />
              </div>
            ) : null}
        </>
      ) : (
        <Empty description="暂无全局概览数据" />
      )}
    </Card>
  )
}

function cycleLabel(cycle?: string): string {
  switch (cycle) {
    case 'quarter':
      return '每季'
    case 'year':
      return '每年'
    case 'month':
    default:
      return '每月'
  }
}

function buildCustomerRevenueRows(revenueRows: MonthlyFinanceRevenueDetail[], customers: CustomerAdminView[]): FinanceRevenueGroupRow[] {
  const groups = new Map<string, FinanceRevenueGroupRow>()
  for (const row of revenueRows) {
    const amount = row.monthlyAmount || 0
    if (amount <= 0) {
      continue
    }
    const matchedCustomers = customers.filter((customer) => (
      (customer.assignments || []).some((assignment) => assignmentMatchesRevenue(assignment, row))
    ))
    if (!matchedCustomers.length) {
      const key = 'unassigned'
      const current = groups.get(key) || {
        key,
        label: '未分配用户',
        detail: '收费已设置，但没有匹配的用户分配',
        clients: [],
        count: 0,
        monthlyAmount: 0,
      }
      current.count += 1
      current.monthlyAmount += amount
      groups.set(key, current)
      continue
    }
    for (const customer of matchedCustomers) {
      const key = String(customer.id)
      const current = groups.get(key) || {
        key,
        label: customer.display_name || customer.username,
        detail: customer.username,
        clients: [],
        count: 0,
        monthlyAmount: 0,
      }
      current.count += 1
      current.monthlyAmount += amount
      groups.set(key, current)
    }
  }
  return [...groups.values()].sort((left, right) => right.monthlyAmount - left.monthlyAmount)
}

function buildNodeRevenueRows(revenueRows: MonthlyFinanceRevenueDetail[]): FinanceRevenueGroupRow[] {
  const groups = new Map<string, FinanceRevenueGroupRow>()
  for (const row of revenueRows) {
    const amount = row.monthlyAmount || 0
    if (amount <= 0) {
      continue
    }
    const nodeLabel = row.nodeLabel || row.inboundTag || (row.inboundID ? `Inbound #${row.inboundID}` : '未指定入站')
    const nodeDetail = [row.inboundTag || (row.inboundID ? `Inbound #${row.inboundID}` : ''), row.agentName].filter(Boolean).join(' · ')
    const clientLabel = row.clientRemark && row.clientEmail
      ? `${row.clientRemark} / ${row.clientEmail}`
      : row.clientRemark || row.clientEmail || row.clientLabel
    const key = `${row.agentID}\u0000${row.inboundID}\u0000${row.inboundTag}`
    const current = groups.get(key) || {
      key,
      label: nodeLabel,
      detail: row.nodeDetail ? `${nodeDetail} · ${row.nodeDetail}` : nodeDetail,
      clients: [],
      count: 0,
      monthlyAmount: 0,
    }
    current.count += 1
    current.monthlyAmount += amount
    if (clientLabel && !current.clients.includes(clientLabel)) {
      current.clients.push(clientLabel)
    }
    groups.set(key, current)
  }
  return [...groups.values()].sort((left, right) => right.monthlyAmount - left.monthlyAmount)
}

function assignmentMatchesRevenue(assignment: CustomerAssignment, row: MonthlyFinanceRevenueDetail): boolean {
  if (assignment.agent_id !== row.agentID) {
    return false
  }
  const assignmentEmail = (assignment.client_email || '').toLowerCase()
  const rowEmail = (row.clientEmail || '').toLowerCase()
  if (assignmentEmail && rowEmail && assignmentEmail === rowEmail) {
    return true
  }
  if (assignment.inbound_id > 0 && row.inboundID > 0 && assignment.inbound_id === row.inboundID) {
    return !assignmentEmail || !rowEmail || assignmentEmail === rowEmail
  }
  const assignmentTag = (assignment.inbound_tag || '').toLowerCase()
  const rowTag = (row.inboundTag || '').toLowerCase()
  if (assignmentTag && rowTag && assignmentTag === rowTag) {
    return !assignmentEmail || !rowEmail || assignmentEmail === rowEmail
  }
  return false
}

function renderMonthlyAmount(amount: number | null, currency: CurrencyCode, hasSourceAmount: boolean) {
  if (amount !== null) {
    return <Text strong>{formatMoney(amount, currency)}</Text>
  }
  return hasSourceAmount ? <Tag color="orange">汇率缺失</Tag> : <Tag>未设置</Tag>
}

function renderPaymentInfo(payment: MonthlyFinancePaymentInfo | null) {
  if (!payment) {
    return <Tag>未设置日期</Tag>
  }
  const color = payment.status === 'paid' ? 'success' : payment.status === 'today' ? 'processing' : 'warning'
  const label = payment.status === 'paid' ? '已缴费' : payment.status === 'today' ? '今日缴费' : '待缴费'
  return (
    <Space direction="vertical" size={2}>
      <Text>{payment.date}</Text>
      <Tag color={color}>{label}</Tag>
    </Space>
  )
}

export function AgentRail(props: {
  agents: DashboardAgentView[]
  loading: boolean
  error: string
  selectedTag: string
  selectedAgentId: string
  tagFilterOptions: DashboardTagView[]
  viewMode: AgentViewMode
  panelExpanded: boolean
  topologyVisible: boolean
  restrictedView?: boolean
  onToggleViewMode: () => void
  onToggleTopology: () => void
  onRefresh: () => void
  onSelectTag: (tag: string) => void
  onSelectAgent: (agentID: string, active: boolean) => void
}) {
  const { agents, loading, error, selectedTag, selectedAgentId, tagFilterOptions, viewMode, panelExpanded, topologyVisible, restrictedView = false, onToggleViewMode, onToggleTopology, onRefresh, onSelectTag, onSelectAgent } = props
  const effectiveViewMode: AgentViewMode = panelExpanded ? 'list' : viewMode
  const [agentPage, setAgentPage] = useState(1)
  const [agentPageSize, setAgentPageSize] = useState(10)

  useEffect(() => {
    setAgentPage(1)
  }, [selectedTag, effectiveViewMode])

  useEffect(() => {
    const maxPage = Math.max(1, Math.ceil(agents.length / agentPageSize))
    setAgentPage((current) => Math.min(current, maxPage))
  }, [agents.length, agentPageSize])

  return (
    <aside className="agent-rail">
      <Card
        title={
          <Space className="agent-rail-title">
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
              type={topologyVisible ? 'default' : 'primary'}
              icon={<ApartmentOutlined />}
              className="topology-toggle-button"
              onClick={onToggleTopology}
            >
              {topologyVisible ? '收起' : '打开拓扑图'}
            </Button>
            {!panelExpanded ? (
              <Button
                size="small"
                shape="circle"
                icon={<BarsOutlined />}
                className={`agent-view-mode-button${effectiveViewMode === 'list' ? ' active' : ''}`}
                aria-label={effectiveViewMode === 'list' ? '关闭列表模式' : '开启列表模式'}
                aria-pressed={effectiveViewMode === 'list'}
                title={effectiveViewMode === 'list' ? '列表模式已开启，点击切回卡片' : '开启列表模式'}
                onClick={onToggleViewMode}
              />
            ) : null}
            <Button
              size="small"
              shape={panelExpanded ? 'circle' : undefined}
              icon={<ReloadOutlined />}
              title="刷新 VPS 列表"
              onClick={onRefresh}
              loading={loading}
            >
              {panelExpanded ? null : '刷新 VPS 列表'}
            </Button>
          </Space>
        }
      >
        {error ? <Alert type="error" showIcon message="加载失败" description={error} className="compact-alert" /> : null}
        <div className="agent-filter-strip">
          <Space wrap>
            <Tag color={!selectedTag ? 'blue' : 'default'} className="tag-filter-chip" onClick={() => onSelectTag('')}>全部</Tag>
            {tagFilterOptions.map((tag) => (
              <Tag key={tag.tag} className="tag-filter-chip" style={tagChipStyle(tag.tag, selectedTag === tag.tag)} onClick={() => onSelectTag(tag.tag)}>
                {tag.tag} · {tag.agent_count}
              </Tag>
            ))}
          </Space>
        </div>
        <div className="agent-list-wrap">
          <List
            className={`agent-list agent-list-${effectiveViewMode}`}
            dataSource={agents}
            loading={loading}
            locale={{ emptyText: <Empty description={selectedTag ? '该标签下暂无 client' : '暂无已注册 client'} /> }}
            pagination={{
              current: agentPage,
              pageSize: agentPageSize,
              hideOnSinglePage: false,
              showSizeChanger: {
                labelRender: ({ value }) => `${value}/页`,
                optionRender: (option) => `${option.value}/页`,
              },
              pageSizeOptions: [5, 10, 20],
              locale: { items_per_page: '/页' },
              onChange: (page, nextPageSize) => {
                if (nextPageSize !== agentPageSize) {
                  setAgentPageSize(nextPageSize)
                  setAgentPage(1)
                  return
                }
                setAgentPage(page)
              },
              onShowSizeChange: (_, nextPageSize) => {
                setAgentPageSize(nextPageSize)
                setAgentPage(1)
              },
              showTotal: (total, range) => `第 ${range[0]}-${range[1]} 台 / 共 ${total} 台`,
            }}
            renderItem={(item, index) => (
              <AgentRailItem
                key={item.agent_id}
                item={item}
                index={index}
                active={item.agent_id === selectedAgentId}
                viewMode={effectiveViewMode}
                compact={panelExpanded}
                restrictedView={restrictedView}
                onSelect={onSelectAgent}
              />
            )}
          />
        </div>
      </Card>
    </aside>
  )
}

function AgentRailItem(props: {
  item: DashboardAgentView
  index: number
  active: boolean
  viewMode: AgentViewMode
  compact?: boolean
  restrictedView?: boolean
  onSelect: (agentID: string, active: boolean) => void
}) {
  const { item, index, active, viewMode, compact = false, restrictedView = false, onSelect } = props
  const renewalStatus = calculateRenewalStatus(item.renewal)
  const trafficStatus = calculateTrafficStatus(item)
  const trafficTotalLabel = restrictedView ? '周期已用流量' : trafficStatus.isPeriod ? '周期总流量' : '总流量'
  const trafficSummaryValue = restrictedView
    ? formatBytes(trafficStatus.total.used)
    : `${trafficStatus.total.label} · 上传 ${formatBytes(trafficStatus.upload.used)} · 下载 ${formatBytes(trafficStatus.download.used)}`
  const cpuPercent = clampMetricPercent(item.summary.cpu)
  const memPercent = calculateMemoryPercent(item.summary)
  const displayStatus = agentDisplayStatus(item)
  const statusLevel = displayStatus.level
  const showStatusText = statusLevel !== 'ok'
  const displaySortOrder = item.sort_order || index + 1
  const addressText = item.summary.observed_ip || item.summary.public_ipv4 || item.summary.hostname || item.agent_id
  const countryCode = agentCountryCode(item)
  const locationText = formatAgentLocation(item, countryCode)
  const tags = (item.tags || []).length ? item.tags || [] : ['未分组']
  const activityText = item.realtime_at
    ? `实时 ${formatDateTime(item.realtime_at)}`
    : item.reported_at
      ? `上报 ${formatDateTime(item.reported_at)}`
      : '尚未上报'
  const footerText = restrictedView ? `已用 ${formatBytes(trafficStatus.total.used)}` : compact ? activityText : `${item.has_config ? '已托管配置' : '待配置'} · ${activityText}`
  const compactTags = tags.slice(0, 3)
  const compactExtraTagCount = Math.max(0, tags.length - compactTags.length)

  if (compact) {
    return (
      <List.Item className="agent-list-item agent-list-item-compact">
        <button className={`agent-button agent-button-compact${active ? ' active' : ''}`} onClick={() => onSelect(item.agent_id, active)}>
          <AgentRailHeader
            item={item}
            countryCode={countryCode}
            locationText={locationText}
            statusLevel={statusLevel}
            statusLabel={displayStatus.label}
            displaySortOrder={displaySortOrder}
            showStatus={!restrictedView && showStatusText}
            restrictedView={restrictedView}
          />
          {!restrictedView ? <div className="agent-meta agent-compact-location">
            <span>{addressText}</span>
            {locationText ? <span title={locationText}>· {locationText}</span> : null}
          </div> : null}
          <div className="agent-compact-tags">
            <AgentRailTags item={item} tags={compactTags} restrictedView={restrictedView} />
            {compactExtraTagCount ? <span className="agent-tag-chip">+{compactExtraTagCount}</span> : null}
          </div>
          <div className="agent-meta agent-footer-line agent-compact-footer">{footerText}</div>
        </button>
      </List.Item>
    )
  }

  return (
    <List.Item className={`agent-list-item agent-list-item-${viewMode}`}>
      <button className={`agent-button agent-button-${viewMode}${active ? ' active' : ''}`} onClick={() => onSelect(item.agent_id, active)}>
        {viewMode === 'list' ? (
          <>
            <div className="agent-list-identity">
              <AgentRailHeader
                item={item}
                countryCode={countryCode}
                locationText={locationText}
                statusLevel={statusLevel}
                statusLabel={displayStatus.label}
                displaySortOrder={displaySortOrder}
                showStatus={false}
                restrictedView={restrictedView}
              />
              {!restrictedView ? <div className="agent-meta agent-location agent-list-location">
                <span>{addressText}</span>
                {locationText ? <span>· {locationText}</span> : null}
              </div> : null}
              <div className="agent-meta agent-footer-line agent-list-footer">{footerText}</div>
            </div>
            <div className="agent-list-runtime">
              {!restrictedView && showStatusText ? <span className={`agent-status-pill agent-status-${statusLevel}`}>{displayStatus.label}</span> : null}
              {!restrictedView ? <AgentRailRuntime item={item} /> : null}
              {!restrictedView ? <AgentSpeedTags item={item} /> : null}
              <div className="agent-list-top-tags">
                <AgentRailTags item={item} tags={tags} restrictedView={restrictedView} />
              </div>
            </div>
            <div className="agent-list-metrics">
              {!restrictedView ? <AgentMeters item={item} cpuPercent={cpuPercent} memPercent={memPercent} showNetwork={false} /> : null}
              <AgentFlowProgress renewalStatus={restrictedView ? null : renewalStatus} trafficLabel={trafficTotalLabel} trafficValue={trafficSummaryValue} trafficStatus={trafficStatus} restrictedView={restrictedView} />
            </div>
          </>
        ) : (
          <>
            <AgentRailHeader
              item={item}
              countryCode={countryCode}
              locationText={locationText}
              statusLevel={statusLevel}
              statusLabel={displayStatus.label}
              displaySortOrder={displaySortOrder}
              showStatus={!restrictedView && showStatusText}
              restrictedView={restrictedView}
            />
            {!restrictedView ? <div className="agent-meta agent-location">
              <span>{addressText}</span>
              {locationText ? <span>· {locationText}</span> : null}
            </div> : null}
            <AgentRailTags item={item} tags={tags} restrictedView={restrictedView} />
            {!restrictedView ? <AgentRailRuntime item={item} /> : null}
            {!restrictedView ? <div className="agent-meter-grid">
              <AgentMeters item={item} cpuPercent={cpuPercent} memPercent={memPercent} />
            </div> : null}
            <div className="agent-traffic-grid">
              <AgentFlowProgress renewalStatus={restrictedView ? null : renewalStatus} trafficLabel={trafficTotalLabel} trafficValue={trafficSummaryValue} trafficStatus={trafficStatus} restrictedView={restrictedView} />
            </div>
            <div className="agent-meta agent-footer-line">{footerText}</div>
          </>
        )}
      </button>
    </List.Item>
  )
}

function AgentRailHeader(props: {
  item: DashboardAgentView
  countryCode: string
  locationText: string
  statusLevel: string
  statusLabel: string
  displaySortOrder: number
  showStatus?: boolean
  restrictedView?: boolean
}) {
  const { item, countryCode, locationText, statusLevel, statusLabel, displaySortOrder, showStatus = true, restrictedView = false } = props
  return (
    <div className="agent-card-head">
      <div className="agent-title-line">
        {!restrictedView ? <span className={`agent-state-dot agent-state-${statusLevel}`} /> : null}
        <span className="agent-order-chip">#{displaySortOrder}</span>
        {!restrictedView ? <span className="agent-flag" title={locationText || countryCode || '未知地区'}>{countryFlag(countryCode)}</span> : null}
        <span className="agent-name">{item.agent_name || item.agent_id}</span>
      </div>
      {showStatus ? <span className={`agent-status-pill agent-status-${statusLevel}`}>{statusLabel}</span> : null}
    </div>
  )
}

function AgentRailTags(props: { item: DashboardAgentView; tags: string[]; restrictedView?: boolean }) {
  const { item, tags, restrictedView = false } = props
  return (
    <div className="agent-tag-row">
      {tags.map((tag) => (
        <span className="agent-tag-chip" key={tag} style={tagChipStyle(tag)}>{tag}</span>
      ))}
      {!restrictedView && item.summary.last_collection_err ? <span className="agent-tag-chip agent-tag-warn" title={item.summary.last_collection_err}>x-ui 异常</span> : null}
      {!restrictedView && xrayIssueLabel(item) ? <span className="agent-tag-chip agent-tag-warn">{xrayIssueLabel(item)}</span> : null}
    </div>
  )
}

function AgentRailRuntime(props: { item: DashboardAgentView }) {
  const { item } = props
  const systemLabel = displayClientSystem(item)
  const clientVersion = item.client_version ? `v${item.client_version}` : ''
  return (
    <div className="agent-runtime-row">
      <div className="agent-runtime-pill" title={formatClientPlatform(item)}>
        <SystemIcon item={item} />
        <span className="agent-runtime-text">{systemLabel || '未知系统'}</span>
        {clientVersion ? <span className="agent-runtime-divider">·</span> : null}
        {clientVersion ? <span className="agent-runtime-subtle">{clientVersion}</span> : null}
      </div>
      {item.renewal?.bandwidth_mbps ? <div className="agent-runtime-pill agent-runtime-pill-bandwidth">带宽 {formatBandwidth(item.renewal.bandwidth_mbps)}</div> : null}
    </div>
  )
}

function AgentSpeedTags(props: { item: DashboardAgentView }) {
  const { item } = props
  return (
    <div className="agent-speed-tags">
      <span className="agent-speed-chip">上行 {formatSpeed(item.summary.net_io_up)}</span>
      <span className="agent-speed-chip">下行 {formatSpeed(item.summary.net_io_down)}</span>
    </div>
  )
}

function SystemIcon(props: { item: DashboardAgentView; compact?: boolean; title?: string }) {
  const { item, compact = false, title } = props
  const flavor = resolveSystemFlavor(item)
  return (
    <span
      className={`agent-system-icon agent-system-${flavor.key}${compact ? ' compact' : ''}`}
      title={title}
      aria-label={flavor.label}
    >
      {flavor.key === 'debian' ? <DebianSwirlIcon /> : compact ? flavor.compactMark : flavor.mark}
    </span>
  )
}

function DebianSwirlIcon() {
  return (
    <svg className="agent-system-svg" viewBox="0 0 64 64" aria-hidden="true" focusable="false">
      <path
        d="M33.8 10.4c-9.6-.8-20.4 5.4-22.6 16-2.4 11.3 6.2 19.2 15.1 21.4 7.8 1.9 17.5-.6 21.9-8.5 3.8-6.9 1.2-15.4-5.2-18.5-5.4-2.6-12.8-.9-15.3 4.6-2.1 4.7.3 9.2 4.2 10.6 3.1 1.1 7-.2 8.2-3.3.9-2.3-.1-4.6-1.9-5.7 3.9.7 6.7 4.4 5.7 8.9-1.3 6.1-8.7 9.3-15.7 7.4-7.9-2.1-14.2-8.8-12.1-17.1 1.9-7.5 10.3-12.5 18.2-11.7 5.6.6 10 3.5 12.1 7.2-1.4-6.2-6.3-10.8-12.6-11.3Z"
        fill="currentColor"
      />
      <path
        d="M28.1 51.4c-5.7-.6-10.3-3-13.8-6.4 2.9 5.6 9.1 9.6 16.4 10.2 10.6.9 20.1-5.6 21.8-15.3-3.6 8-13 12.6-24.4 11.5Z"
        fill="currentColor"
        opacity="0.8"
      />
    </svg>
  )
}

function resolveSystemFlavor(item: DashboardAgentView) {
  const systemVersion = (item.system_version || '').trim()
  const clientOS = (item.client_os || '').trim().toLowerCase()
  const value = `${systemVersion} ${clientOS}`.toLowerCase()
  if (value.includes('debian')) return { key: 'debian', mark: 'DEB', compactMark: 'D', label: 'Debian' }
  if (value.includes('ubuntu')) return { key: 'ubuntu', mark: 'UBU', compactMark: 'U', label: 'Ubuntu' }
  if (value.includes('alpine')) return { key: 'alpine', mark: 'ALP', compactMark: 'A', label: 'Alpine' }
  if (value.includes('windows')) return { key: 'windows', mark: 'WIN', compactMark: 'W', label: 'Windows' }
  if (value.includes('centos')) return { key: 'centos', mark: 'COS', compactMark: 'C', label: 'CentOS' }
  if (value.includes('rocky')) return { key: 'rocky', mark: 'RKY', compactMark: 'R', label: 'Rocky Linux' }
  if (value.includes('alma')) return { key: 'alma', mark: 'ALM', compactMark: 'A', label: 'AlmaLinux' }
  if (value.includes('fedora')) return { key: 'fedora', mark: 'FED', compactMark: 'F', label: 'Fedora' }
  if (value.includes('mac') || value.includes('darwin')) return { key: 'macos', mark: 'MAC', compactMark: 'M', label: 'macOS' }
  if (clientOS === 'linux') return { key: 'linux', mark: 'LNX', compactMark: 'L', label: 'Linux' }
  if (clientOS === 'windows') return { key: 'windows', mark: 'WIN', compactMark: 'W', label: 'Windows' }
  if (clientOS === 'darwin') return { key: 'macos', mark: 'MAC', compactMark: 'M', label: 'macOS' }
  return { key: 'generic', mark: 'SYS', compactMark: 'S', label: 'System' }
}

function displayClientSystem(item: DashboardAgentView) {
  const systemVersion = item.system_version?.trim()
  if (systemVersion) return systemVersion
  return humanizeClientOS(item.client_os)
}

function humanizeClientOS(os?: string) {
  const value = (os || '').trim().toLowerCase()
  if (value === 'linux') return 'Linux'
  if (value === 'windows') return 'Windows'
  if (value === 'darwin') return 'macOS'
  return os?.trim() || ''
}

function formatClientPlatform(item: DashboardAgentView) {
  return [displayClientSystem(item), item.client_os, item.client_arch].filter(Boolean).join(' / ')
}

function AgentMeters(props: { item: DashboardAgentView; cpuPercent: number; memPercent: number; showNetwork?: boolean }) {
  const { item, cpuPercent, memPercent, showNetwork = true } = props
  return (
    <>
      <MiniProgress label="CPU" value={formatPercent(cpuPercent)} percent={cpuPercent} level={metricLevel(cpuPercent)} />
      <MiniProgress label="内存" value={formatMem(item.summary.mem_used, item.summary.mem_total)} percent={memPercent} level={metricLevel(memPercent)} />
      {showNetwork ? <MiniProgress label="上行" value={formatSpeed(item.summary.net_io_up)} level="neutral" /> : null}
      {showNetwork ? <MiniProgress label="下行" value={formatSpeed(item.summary.net_io_down)} level="neutral" /> : null}
    </>
  )
}

function AgentFlowProgress(props: {
  renewalStatus: ReturnType<typeof calculateRenewalStatus>
  trafficLabel: string
  trafficValue: string
  trafficStatus: ReturnType<typeof calculateTrafficStatus>
  restrictedView?: boolean
}) {
  const { renewalStatus, trafficLabel, trafficValue, trafficStatus, restrictedView = false } = props
  return (
    <>
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
        label={trafficLabel}
        value={trafficValue}
        percent={restrictedView ? undefined : trafficStatus.total.percent}
        showTrack={!restrictedView}
        level={restrictedView ? 'neutral' : trafficStatus.total.level}
        className="agent-wide-progress"
      />
    </>
  )
}
