import { useEffect, useState } from 'react'
import { Alert, Button, Card, Empty, List, Select, Space, Tag, Typography } from 'antd'
import { ApartmentOutlined, BarsOutlined, CloudServerOutlined, ReloadOutlined } from '@ant-design/icons'

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
  compact?: boolean
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
    compact,
    onCostCurrencyChange,
  } = props

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
              <div className="overview-stat-foot">x-ui 异常 {xuiErrorAgentCount} · 出站 {dashboardView.totals.outbound_count} · 规则 {dashboardView.totals.routing_rule_count}</div>
            </section>
            <section className="overview-stat-card overview-network-card">
              <div className="overview-stat-title">本周期流量</div>
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
  panelExpanded: boolean
  topologyVisible: boolean
  onToggleViewMode: () => void
  onToggleTopology: () => void
  onRefresh: () => void
  onSelectTag: (tag: string) => void
  onSelectAgent: (agentID: string, active: boolean) => void
}) {
  const { agents, loading, error, selectedTag, selectedAgentId, tagFilterOptions, viewMode, panelExpanded, topologyVisible, onToggleViewMode, onToggleTopology, onRefresh, onSelectTag, onSelectAgent } = props
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
  onSelect: (agentID: string, active: boolean) => void
}) {
  const { item, index, active, viewMode, compact = false, onSelect } = props
  const renewalStatus = calculateRenewalStatus(item.renewal)
  const trafficStatus = calculateTrafficStatus(item)
  const trafficTotalLabel = trafficStatus.isPeriod ? '周期总流量' : '总流量'
  const trafficSummaryValue = `${trafficStatus.total.label} · 上传 ${formatBytes(trafficStatus.upload.used)} · 下载 ${formatBytes(trafficStatus.download.used)}`
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
  const footerText = compact ? activityText : `${item.has_config ? '已托管配置' : '待配置'} · ${activityText}`
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
            showStatus={showStatusText}
          />
          <div className="agent-meta agent-compact-location">
            <span>{addressText}</span>
            {locationText ? <span title={locationText}>{locationText}</span> : null}
          </div>
          <div className="agent-compact-tags">
            <AgentRailTags item={item} tags={compactTags} />
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
              />
              <div className="agent-meta agent-location agent-list-location">
                <span>{addressText}</span>
                {locationText ? <span>{locationText}</span> : null}
              </div>
              <div className="agent-meta agent-footer-line agent-list-footer">{footerText}</div>
            </div>
            <div className="agent-list-runtime">
              {showStatusText ? <span className={`agent-status-pill agent-status-${statusLevel}`}>{displayStatus.label}</span> : null}
              <AgentRailRuntime item={item} />
              <AgentSpeedTags item={item} />
              <div className="agent-list-top-tags">
                <AgentRailTags item={item} tags={tags} />
              </div>
            </div>
            <div className="agent-list-metrics">
              <AgentMeters item={item} cpuPercent={cpuPercent} memPercent={memPercent} showNetwork={false} />
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
              showStatus={showStatusText}
            />
            <div className="agent-meta agent-location">
              <span>{addressText}</span>
              {locationText ? <span>{locationText}</span> : null}
            </div>
            <AgentRailTags item={item} tags={tags} />
            <AgentRailRuntime item={item} />
            <div className="agent-meter-grid">
              <AgentMeters item={item} cpuPercent={cpuPercent} memPercent={memPercent} />
            </div>
            <div className="agent-traffic-grid">
              <AgentFlowProgress renewalStatus={renewalStatus} trafficLabel={trafficTotalLabel} trafficValue={trafficSummaryValue} trafficStatus={trafficStatus} />
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
}) {
  const { item, countryCode, locationText, statusLevel, statusLabel, displaySortOrder, showStatus = true } = props
  return (
    <div className="agent-card-head">
      <div className="agent-title-line">
        <span className={`agent-state-dot agent-state-${statusLevel}`} />
        <span className="agent-order-chip">#{displaySortOrder}</span>
        <span className="agent-flag" title={locationText || countryCode || '未知地区'}>{countryFlag(countryCode)}</span>
        <span className="agent-name">{item.agent_name || item.agent_id}</span>
      </div>
      {showStatus ? <span className={`agent-status-pill agent-status-${statusLevel}`}>{statusLabel}</span> : null}
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
      {item.summary.last_collection_err ? <span className="agent-tag-chip agent-tag-warn" title={item.summary.last_collection_err}>x-ui 异常</span> : null}
      {xrayIssueLabel(item) ? <span className="agent-tag-chip agent-tag-warn">{xrayIssueLabel(item)}</span> : null}
    </div>
  )
}

function AgentRailRuntime(props: { item: DashboardAgentView }) {
  const { item } = props
  const systemLabel = displayClientSystem(item)
  const clientVersion = item.client_version ? `Client v${item.client_version}` : ''
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
