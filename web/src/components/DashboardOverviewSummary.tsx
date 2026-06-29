import { useEffect, useMemo, useState } from 'react'
import { Button, Card, Empty, Select, Space, Table, Tabs, Tag, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'

import type { AreaManagerAdminView, ClientChainView, CustomerAdminView, CustomerAssignment, DashboardAgentView, GlobalDashboardView } from '../types'
import type { CurrencyCode, ExchangeRatesState, MonthlyFinanceCostDetail, MonthlyFinancePaymentInfo, MonthlyFinanceRevenueDetail } from '../lib/currency'
import { buildMonthlyFinanceCostDetails, buildMonthlyFinanceRevenueDetails, formatMoney } from '../lib/currency'
import { fetchJSON, formatDateTime } from '../lib/appHelpers'
import { type AgentNetworkSummary, formatBytes, formatSpeed } from '../lib/traffic'

const { Text } = Typography

type FinanceRevenueGroupRow = {
  key: string
  label: string
  detail: string
  clients: string[]
  count: number
  monthlyAmount: number
}

export function OverviewSummaryCard(props: {
  dashboardView: GlobalDashboardView | null
  scopedAgentCount: number
  scopedNodeCount: number
  onlineAgentCount: number
  offlineAgentCount: number
  xuiErrorAgentCount: number
  scopedNetwork: AgentNetworkSummary
  costCurrency: CurrencyCode
  currencyOptions: CurrencyCode[]
  monthlyFinance: { profitTotal: number; revenueTotal: number; costTotal: number }
  financeAgents: DashboardAgentView[]
  financeChains: ClientChainView[]
  financeCustomers: CustomerAdminView[]
  financeAreaManagers: AreaManagerAdminView[]
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
    financeCustomers,
    financeAreaManagers,
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
  const effectiveCustomerRows = financeCustomers.length ? financeCustomers : customerRows
  const costRows = useMemo(
    () => buildMonthlyFinanceCostDetails(financeAgents, costCurrency, exchangeRates),
    [costCurrency, exchangeRates, financeAgents],
  )
  const revenueRows = useMemo(
    () => buildMonthlyFinanceRevenueDetails(financeAgents, financeChains, costCurrency, exchangeRates, effectiveCustomerRows, financeAreaManagers),
    [costCurrency, effectiveCustomerRows, exchangeRates, financeAgents, financeAreaManagers, financeChains],
  )
  const customerRevenueRows = useMemo(
    () => buildCustomerRevenueRows(revenueRows, effectiveCustomerRows),
    [effectiveCustomerRows, revenueRows],
  )
  const nodeRevenueRows = useMemo(
    () => buildNodeRevenueRows(revenueRows),
    [revenueRows],
  )

  useEffect(() => {
    if (restrictedView || !financeDetailOpen || effectiveCustomerRows.length || customerRowsLoading) {
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
  }, [customerRowsLoading, effectiveCustomerRows.length, financeDetailOpen, restrictedView])

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
          {record.source === 'area_account' ? <Tag color="gold">区域账号收入</Tag> : null}
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
                {restrictedView ? <span className="network-up">{formatBytes(scopedNetwork.used)}</span> : <span className="network-up">↑{formatBytes(scopedNetwork.sent)}</span>}
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
                  <Text type="secondary">admin 财务 = 单用户节点收入 + 区域账号收入 - VPS 总花销</Text>
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
                    label: `收入 · 用户/区域 ${revenueRows.length}`,
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
    if (row.source === 'area_account') {
      const current = groups.get(row.key) || {
        key: row.key,
        label: row.clientLabel,
        detail: row.clientRemark || '区域账号收入',
        clients: [],
        count: 0,
        monthlyAmount: 0,
      }
      current.count += 1
      current.monthlyAmount += amount
      groups.set(row.key, current)
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
