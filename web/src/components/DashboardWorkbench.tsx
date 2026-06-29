import { useEffect, useMemo, useRef, useState } from 'react'
import { Button, Card, Empty, Tag, Typography } from 'antd'
import { ApartmentOutlined } from '@ant-design/icons'
import { Line } from '@ant-design/plots'
import type { LineConfig } from '@ant-design/plots'

import type { DashboardAgentView, GlobalDashboardView } from '../types'
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
  scopedNetwork: AgentNetworkSummary
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
        <WorkbenchKpi label={restrictedView ? '授权周期已用' : '周期总流量'} value={formatBytes(scopedNetwork.used)} note={restrictedView ? '仅统计有权限的 Client' : `↑${formatBytes(scopedNetwork.sent)} · ↓${formatBytes(scopedNetwork.recv)}`} tone="traffic" />
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
