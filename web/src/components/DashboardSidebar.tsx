import { Alert, Button, Card, Empty, List, Select, Space, Spin, Tag, Typography } from 'antd'
import { BarsOutlined, CloudServerOutlined, ReloadOutlined } from '@ant-design/icons'

import type { DashboardAgentView, DashboardTagView, GlobalDashboardView } from '../types'
import type { AgentViewMode } from '../lib/appHelpers'
import type { CurrencyCode, ExchangeRatesState } from '../lib/currency'
import { formatMoney } from '../lib/currency'
import {
  agentCountryCode,
  agentDisplayStatus,
  calculateRenewalStatus,
  countryFlag,
  formatAgentLocation,
  formatDateTime,
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
  exchangeRates: ExchangeRatesState
  selectedTag: string
  currentAgentLabel: string
  currentIPv4: string
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
    exchangeRates,
    selectedTag,
    currentAgentLabel,
    currentIPv4,
    onCostCurrencyChange,
  } = props

  return (
    <Card className="surface-card summary-card" bordered={false}>
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
              <div className="overview-stat-foot">x-ui 异常 {xuiErrorAgentCount} · 出站 {dashboardView.totals.outbound_count} · 规则 {dashboardView.totals.routing_rule_count}</div>
            </section>
            <section className="overview-stat-card overview-network-card">
              <div className="overview-stat-title">网络</div>
              <div className="overview-network-total">
                <span className="network-up">↑{formatBytes(scopedNetwork.sent)}</span>
                <span className="network-down">↓{formatBytes(scopedNetwork.recv)}</span>
              </div>
              <div className="overview-network-speed">
                <span>⬆ {formatSpeed(scopedNetwork.up)}</span>
                <span>⬇ {formatSpeed(scopedNetwork.down)}</span>
              </div>
            </section>
            <section className="overview-stat-card overview-cost-card">
              <div className="overview-stat-title overview-cost-title">
                <span>财务月览</span>
                <Select
                  className="overview-currency-select"
                  size="small"
                  value={costCurrency}
                  options={currencyOptions.map((currency) => ({ value: currency, label: currency }))}
                  popupMatchSelectWidth={96}
                  onChange={(value) => onCostCurrencyChange(value as CurrencyCode)}
                />
              </div>
              <div className="overview-cost-value">{formatMoney(monthlyFinance.profitTotal, costCurrency)}</div>
              <div className="overview-stat-foot">
                营收 {formatMoney(monthlyFinance.revenueTotal, costCurrency)} · 花销 {formatMoney(monthlyFinance.costTotal, costCurrency)}
              </div>
              {exchangeRates.error ? <div className="overview-stat-foot">汇率加载失败：{exchangeRates.error}</div> : null}
            </section>
          </div>
          <div className="overview-summary-strip">
            <span>已匹配链路 · {dashboardView.totals.link_count}</span>
            <span>前端客户端链路 · {dashboardView.totals.chain_count}</span>
            <span>标签视图 · {selectedTag || '全部'}</span>
            <span>计算时间 · {formatDateTime(dashboardView.generated_at)}</span>
            <span>当前详情节点 · {currentAgentLabel || '-'}</span>
            <span>当前节点 IPv4 · {currentIPv4 || '-'}</span>
          </div>
        </>
      ) : (
        <Empty description="暂无全局概览数据" />
      )}
    </Card>
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
  onToggleViewMode: () => void
  onRefresh: () => void
  onSelectTag: (tag: string) => void
  onSelectAgent: (agentID: string, active: boolean) => void
}) {
  const { agents, loading, error, selectedTag, selectedAgentId, tagFilterOptions, viewMode, onToggleViewMode, onRefresh, onSelectTag, onSelectAgent } = props

  return (
    <aside className="agent-rail">
      <Card
        title={
          <Space>
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
              shape="circle"
              icon={<BarsOutlined />}
              className={`agent-view-mode-button${viewMode === 'list' ? ' active' : ''}`}
              aria-label={viewMode === 'list' ? '关闭列表模式' : '开启列表模式'}
              aria-pressed={viewMode === 'list'}
              title={viewMode === 'list' ? '列表模式已开启，点击切回卡片' : '开启列表模式'}
              onClick={onToggleViewMode}
            />
            <Button size="small" icon={<ReloadOutlined />} onClick={onRefresh} loading={loading}>刷新 VPS 列表</Button>
          </Space>
        }
      >
        {error ? <Alert type="error" showIcon message="加载失败" description={error} className="compact-alert" /> : null}
        <Space wrap style={{ marginBottom: 12 }}>
          <Tag color={!selectedTag ? 'green' : 'default'} className="tag-filter-chip" onClick={() => onSelectTag('')}>全部</Tag>
          {tagFilterOptions.map((tag) => (
            <Tag key={tag.tag} className="tag-filter-chip" style={tagChipStyle(tag.tag, selectedTag === tag.tag)} onClick={() => onSelectTag(tag.tag)}>
              {tag.tag} · {tag.agent_count}
            </Tag>
          ))}
        </Space>
        <Spin spinning={loading}>
          {agents.length ? (
            <List
              className={`agent-list agent-list-${viewMode}`}
              dataSource={agents}
              pagination={{ pageSize: 10, hideOnSinglePage: true, showSizeChanger: false }}
              renderItem={(item, index) => (
                <AgentRailItem
                  key={item.agent_id}
                  item={item}
                  index={index}
                  active={item.agent_id === selectedAgentId}
                  viewMode={viewMode}
                  onSelect={onSelectAgent}
                />
              )}
            />
          ) : (
            <Empty description={selectedTag ? '该标签下暂无 client' : '暂无已注册 client'} />
          )}
        </Spin>
      </Card>
    </aside>
  )
}

function AgentRailItem(props: {
  item: DashboardAgentView
  index: number
  active: boolean
  viewMode: AgentViewMode
  onSelect: (agentID: string, active: boolean) => void
}) {
  const { item, index, active, viewMode, onSelect } = props
  const renewalStatus = calculateRenewalStatus(item.renewal)
  const trafficStatus = calculateTrafficStatus(item)
  const trafficTotalLabel = trafficStatus.isPeriod ? '周期总流量' : '总流量'
  const trafficSummaryValue = `${trafficStatus.total.label} · 上传 ${formatBytes(trafficStatus.upload.used)} · 下载 ${formatBytes(trafficStatus.download.used)}`
  const cpuPercent = clampMetricPercent(item.summary.cpu)
  const memPercent = calculateMemoryPercent(item.summary)
  const displayStatus = agentDisplayStatus(item)
  const statusLevel = displayStatus.level
  const displaySortOrder = item.sort_order || index + 1
  const addressText = item.summary.public_ipv4 || item.summary.observed_ip || item.summary.hostname || item.agent_id
  const countryCode = agentCountryCode(item)
  const locationText = formatAgentLocation(item, countryCode)
  const tags = (item.tags || []).length ? item.tags || [] : ['未分组']
  const activityText = item.realtime_at
    ? `实时 ${formatDateTime(item.realtime_at)}`
    : item.reported_at
      ? `上报 ${formatDateTime(item.reported_at)}`
      : '尚未上报'

  return (
    <List.Item className={`agent-list-item agent-list-item-${viewMode}`}>
      <button className={`agent-button agent-button-${viewMode}${active ? ' active' : ''}`} onClick={() => onSelect(item.agent_id, active)}>
        {viewMode === 'list' ? (
          <>
            <div className="agent-list-main">
              <AgentRailHeader
                item={item}
                countryCode={countryCode}
                locationText={locationText}
                statusLevel={statusLevel}
                statusLabel={displayStatus.label}
                displaySortOrder={displaySortOrder}
              />
              <div className="agent-meta agent-location agent-list-location">
                <span>{addressText}</span>
                {locationText ? <span>{locationText}</span> : null}
              </div>
              <AgentRailTags item={item} tags={tags} />
              <div className="agent-meta agent-footer-line">{item.has_config ? '已托管配置' : '待配置'} · {activityText}</div>
            </div>
            <div className="agent-list-metrics">
              <AgentMeters item={item} cpuPercent={cpuPercent} memPercent={memPercent} />
            </div>
            <div className="agent-list-flow">
              <AgentFlowProgress renewalStatus={renewalStatus} trafficLabel={trafficTotalLabel} trafficValue={trafficSummaryValue} trafficStatus={trafficStatus} />
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
            />
            <div className="agent-meta agent-location">
              <span>{addressText}</span>
              {locationText ? <span>{locationText}</span> : null}
            </div>
            <AgentRailTags item={item} tags={tags} />
            <div className="agent-meter-grid">
              <AgentMeters item={item} cpuPercent={cpuPercent} memPercent={memPercent} />
            </div>
            <div className="agent-traffic-grid">
              <AgentFlowProgress renewalStatus={renewalStatus} trafficLabel={trafficTotalLabel} trafficValue={trafficSummaryValue} trafficStatus={trafficStatus} />
            </div>
            <div className="agent-meta agent-footer-line">{item.has_config ? '已托管配置' : '待配置'} · {activityText}</div>
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
}) {
  const { item, countryCode, locationText, statusLevel, statusLabel, displaySortOrder } = props
  return (
    <div className="agent-card-head">
      <div className="agent-title-line">
        <span className={`agent-state-dot agent-state-${statusLevel}`} />
        <span className="agent-order-chip">#{displaySortOrder}</span>
        <span className="agent-flag" title={locationText || countryCode || '未知地区'}>{countryFlag(countryCode)}</span>
        <span className="agent-name">{item.agent_name || item.agent_id}</span>
      </div>
      <span className={`agent-status-pill agent-status-${statusLevel}`}>{statusLabel}</span>
    </div>
  )
}

function AgentRailTags(props: { item: DashboardAgentView; tags: string[] }) {
  const { item, tags } = props
  return (
    <div className="agent-tag-row">
      {tags.map((tag) => (
        <span className="agent-tag-chip" key={tag} style={tagChipStyle(tag)}>{tag}</span>
      ))}
      {item.renewal?.bandwidth_mbps ? <span className="agent-tag-chip">带宽 {formatBandwidth(item.renewal.bandwidth_mbps)}</span> : null}
      {item.summary.last_collection_err ? <span className="agent-tag-chip agent-tag-warn" title={item.summary.last_collection_err}>x-ui 异常</span> : null}
      {xrayIssueLabel(item) ? <span className="agent-tag-chip agent-tag-warn">{xrayIssueLabel(item)}</span> : null}
    </div>
  )
}

function AgentMeters(props: { item: DashboardAgentView; cpuPercent: number; memPercent: number }) {
  const { item, cpuPercent, memPercent } = props
  return (
    <>
      <MiniProgress label="CPU" value={formatPercent(cpuPercent)} percent={cpuPercent} level={metricLevel(cpuPercent)} />
      <MiniProgress label="内存" value={formatMem(item.summary.mem_used, item.summary.mem_total)} percent={memPercent} level={metricLevel(memPercent)} />
      <MiniProgress label="上行" value={formatSpeed(item.summary.net_io_up)} level="neutral" />
      <MiniProgress label="下行" value={formatSpeed(item.summary.net_io_down)} level="neutral" />
    </>
  )
}

function AgentFlowProgress(props: {
  renewalStatus: ReturnType<typeof calculateRenewalStatus>
  trafficLabel: string
  trafficValue: string
  trafficStatus: ReturnType<typeof calculateTrafficStatus>
}) {
  const { renewalStatus, trafficLabel, trafficValue, trafficStatus } = props
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
        percent={trafficStatus.total.percent}
        showTrack
        level={trafficStatus.total.level}
        className="agent-wide-progress"
      />
    </>
  )
}
