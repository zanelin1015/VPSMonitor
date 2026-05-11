import { Alert, Button, Card, Empty, Space, Tag, Typography } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'

import type { ClientChainStep, ClientChainView, DashboardAgentView, GlobalDashboardView, IPGeoView, TopologyLinkView } from '../types'
import { DASHBOARD_AUTO_REFRESH_MS, confidenceLabel, hasSelectedTag, normalizeNodeAnchorLabel, tagChipStyle } from '../lib/appHelpers'

const { Text, Title } = Typography

export function renderGlobalOverviewPanel(props: {
  dashboardView: GlobalDashboardView | null
  selectedTag: string
  links: TopologyLinkView[]
  onSelectTag: (value: string) => void
}) {
  const { dashboardView, selectedTag, links, onSelectTag } = props

  if (!dashboardView) {
    return <Empty description="暂无总览数据" />
  }

  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Alert
        type="info"
        showIcon
        message="链路明细"
        description={`这里保留标签分组、已自动匹配链路和客户端转发链明细。页面会每 ${Math.floor(DASHBOARD_AUTO_REFRESH_MS / 1000)} 秒自动刷新一次统计与匹配结果。`}
      />

      <Card className="config-section-card" bordered={false}>
        <Space wrap>
          <Tag color={!selectedTag ? 'blue' : 'default'} className="tag-filter-chip" onClick={() => onSelectTag('')}>
            全部
          </Tag>
          {dashboardView.tags.map((tag) => (
            <Tag
              key={tag.tag}
              className="tag-filter-chip"
              style={tagChipStyle(tag.tag, selectedTag === tag.tag)}
              onClick={() => onSelectTag(tag.tag)}
            >
              {tag.tag} · {tag.client_count}
            </Tag>
          ))}
        </Space>
      </Card>

      <Card className="config-section-card" bordered={false}>
        <Title level={4}>跨 Client 已匹配链路</Title>
        {links.length ? (
          <div className="topology-link-list">
            {links.map((link) => (
              <section key={link.key} className="topology-link-card">
                <div className="topology-link-row">
                  <Text strong>{link.source.agent_name || link.source.agent_id}</Text>
                  <Tag color="gold">{link.source.outbound_tag || '-'}</Tag>
                  <span className="topology-arrow">→</span>
                  <Text strong>{link.target.agent_name || link.target.agent_id}</Text>
                  <Tag color="cyan">{link.target.inbound_name || link.target.inbound_tag || '-'}</Tag>
                </div>
                <div className="muted-line">
                  {link.source.target || '-'} → {link.target.entry_addresses?.[0] || link.target.domains?.[0] || link.target.ips?.[0] || '-'}:{link.target.port || 0}
                </div>
                {link.source.resolved_ips?.length || link.target.resolved_ips?.length ? (
                  <div className="muted-line">
                    解析 IP: {(link.source.resolved_ips || []).join(', ') || '-'} → {(link.target.resolved_ips || []).join(', ') || '-'}
                  </div>
                ) : null}
                <div className="muted-line">
                  {link.match_reason || '-'} · {confidenceLabel(link.match_confidence)} · score {link.match_score}
                </div>
                {link.match_explanation ? <div className="muted-line">{link.match_explanation}</div> : null}
              </section>
            ))}
          </div>
        ) : (
          <Empty description="当前没有自动匹配上的跨 client 出站链路" />
        )}
      </Card>
    </Space>
  )
}

export function renderCNFlowPanel(props: {
  dashboardView: GlobalDashboardView
  selectedTag: string
  selectedAgentId: string
  agents: DashboardAgentView[]
  chains: ClientChainView[]
  onSelectAgent: (agentID: string) => void
  onJumpNode: (agentID?: string, nodeLabel?: string) => void
  canOpenXUI: boolean
  onOpenXUI: () => void
  canRefreshCurrentNode: boolean
  currentNodeLoading: boolean
  onRefreshCurrentNode: () => void
}) {
  const {
    dashboardView,
    selectedTag,
    selectedAgentId,
    agents,
    chains,
    onSelectAgent,
    onJumpNode,
    canOpenXUI,
    onOpenXUI,
    canRefreshCurrentNode,
    currentNodeLoading,
    onRefreshCurrentNode,
  } = props
  const rows = buildCNFlowRows(chains, agents)
  const selectedAgent = agents.find((agent) => agent.agent_id === selectedAgentId)
  const flowMode: 'cn' | 'agent' | 'tag' = selectedAgentId ? 'agent' : selectedTag ? 'tag' : 'cn'
  const visibleRows = selectedAgentId ? rows.filter((row) => row.rootAgentID === selectedAgentId) : rows
  const toolbarAgents = selectedTag ? agents.filter((agent) => hasSelectedTag(agent.tags, selectedTag)) : agents
  const headerTitle = flowMode === 'agent' ? `${selectedAgent?.agent_name || selectedAgentId} 节点客户端拓扑` : flowMode === 'tag' ? `${selectedTag} 标签链路拓扑` : 'CN 出发链路拓扑'

  const renderSource = () => {
    return (
      <div className="cn-flow-source">
        <div className="cn-source-orb">CN</div>
      </div>
    )
  }

  const renderAgentNode = (row: CNFlowRow) => (
    <button className="cn-flow-node agent-node" onClick={() => onSelectAgent(row.rootAgentID)}>
      <span className="node-kicker">Client</span>
      <strong>{row.rootAgentName || row.rootAgentID}</strong>
      <small>{row.rootAgentTags?.length ? row.rootAgentTags.join(' · ') : row.rootAgentID}</small>
    </button>
  )

  const renderEntryNode = (row: CNFlowRow) => (
    <button className="cn-flow-node entry-node" onClick={() => onSelectAgent(row.rootAgentID)}>
      <span className="node-kicker">Client VPS / 入站</span>
      <strong>{row.rootAgentName || row.rootAgentID}</strong>
      <small>{row.entryLabel}</small>
    </button>
  )

  const renderClientNode = (row: CNFlowRow) => (
    <div className="cn-flow-node client-node">
      <span className="node-kicker">节点客户端</span>
      <strong>{row.clientLabel}</strong>
      <small>{row.clientDetail || '未备注'}</small>
    </div>
  )

  const renderTargetNode = (hop: CNFlowHop, compact = false) =>
    hop.targetAgentID ? (
      <>
        <div className="cn-flow-arrow">→</div>
        <button className={`cn-flow-node target-node${compact ? ' compact-target-node' : ''}`} onClick={() => onJumpNode(hop.targetAgentID, hop.targetInboundLabel)}>
          <span className="node-kicker">{compact ? '后续落地点' : '下一跳 Client / 节点'}</span>
          <strong>{hop.targetAgentName || hop.targetAgentID}</strong>
          <small>
            {hop.targetInboundLabel || '-'}
            {hop.targetClientLabel ? ` · ${hop.targetClientLabel}` : hop.targetProtocol ? ` · ${hop.targetProtocol} 入站` : ''}
          </small>
        </button>
      </>
    ) : null

  const renderForwardNode = (hop: CNFlowHop) =>
    hop.targetAgentID ? (
      <button className="cn-flow-node forward-node" onClick={() => onJumpNode(hop.targetAgentID, hop.targetInboundLabel)}>
        <span className="node-kicker">转发出站 / 下一跳节点</span>
        <strong>{hop.outboundLabel}</strong>
        <small>{hop.outboundDetail}</small>
        <small>
          → {hop.targetAgentName || hop.targetAgentID}
          {hop.targetInboundLabel ? ` / ${hop.targetInboundLabel}` : ''}
          {hop.targetClientLabel ? ` · ${hop.targetClientLabel}` : hop.targetProtocol ? ` · ${hop.targetProtocol} 入站` : ''}
        </small>
      </button>
    ) : (
      <div className="cn-flow-node rule-node">
        <span className="node-kicker">本跳转发出站</span>
        <strong>{hop.outboundLabel}</strong>
        <small>{hop.outboundDetail}</small>
      </div>
    )

  const renderHopSegments = (row: CNFlowRow, showCurrentNode: boolean) =>
    row.hops.length ? (
      row.hops.map((hop, index) => {
        if (index > 0) {
          return hop.targetAgentID ? <div key={`${row.key}-${index}`} className="cn-flow-hop-group compact-hop">{renderTargetNode(hop, true)}</div> : null
        }
        return (
          <div key={`${row.key}-${index}`} className="cn-flow-hop-group">
            <div className="cn-flow-hop-label">
              <b>第 1 跳</b>
            </div>
            <div className="cn-flow-hop-body">
              {showCurrentNode ? (
                <>
                  <button className="cn-flow-node entry-node" onClick={() => onSelectAgent(hop.currentAgentID || row.rootAgentID)}>
                    <span className="node-kicker">Client VPS / 入站</span>
                    <strong>{hop.currentAgentName || hop.currentAgentID || row.rootAgentName || row.rootAgentID}</strong>
                    <small>{hop.currentInboundLabel || row.entryLabel}</small>
                  </button>
                  <div className="cn-flow-arrow">→</div>
                </>
              ) : null}
              <div className="cn-flow-arrow rule-arrow">
                <span>规则 R{hop.ruleIndex || '?'}</span>
                <em>{hop.routeScope || 'route'}</em>
              </div>
              {renderForwardNode(hop)}
            </div>
          </div>
        )
      })
    ) : (
      <>
        <div className="cn-flow-arrow rule-arrow">
          <span>规则 ?</span>
          <em>unmatched</em>
        </div>
        <div className="cn-flow-node rule-node">
          <span className="node-kicker">未匹配出站</span>
          <strong>-</strong>
          <small>未找到可展示的转发规则</small>
        </div>
      </>
    )

  const renderFlowTail = (row: CNFlowRow) => (
    <>
      {flowMode === 'cn' ? (
        <>
          {renderEntryNode(row)}
          <div className="cn-flow-arrow">→</div>
          <div className="cn-flow-hop-column cn-flow-hop-column-cn">
            {renderClientNode(row)}
            <div className="cn-flow-arrow">→</div>
            {renderHopSegments(row, false)}
          </div>
        </>
      ) : (
        <>
          {renderClientNode(row)}
          <div className="cn-flow-arrow">→</div>
          <div className="cn-flow-hop-column">{renderHopSegments(row, false)}</div>
        </>
      )}
      <div className="cn-flow-arrow final-arrow">→</div>
      <div className={`cn-flow-node country-node country-${row.exitCountryCode.toLowerCase()}`}>
        <span className="node-kicker">最终出站国家</span>
        <strong>{row.exitCountryLabel}</strong>
        <small>{row.exitReason}</small>
      </div>
      {shouldShowChainWarning(row) ? <Tag color="orange" className="cn-flow-warning">{translateChainReason(row.unresolvedReason || '')}</Tag> : null}
    </>
  )

  const groupRowsByEntry = (inputRows: CNFlowRow[]) =>
    Array.from(
      inputRows.reduce((groups, row) => {
        const key = `${row.rootAgentID}:${row.entryLabel}`
        const group = groups.get(key)
        if (group) {
          group.rows.push(row)
        } else {
          groups.set(key, { key, lead: row, rows: [row] })
        }
        return groups
      }, new Map<string, { key: string; lead: CNFlowRow; rows: CNFlowRow[] }>()),
    ).map(([, group]) => group)

  const renderEntryCluster = (group: { key: string; lead: CNFlowRow; rows: CNFlowRow[] }) => (
    <section key={group.key} className="cn-flow-entry-cluster">
      {renderEntryNode(group.lead)}
      <div className="cn-flow-arrow cluster-arrow">→</div>
      <div className="cn-flow-entry-cluster-rows">
        {group.rows.map((row) => (
          <div key={row.key} className={`cn-flow-lane${row.loopDetected ? ' loop' : ''}`}>
            {renderFlowTail(row)}
          </div>
        ))}
      </div>
    </section>
  )

  const tagAgentGroups = Array.from(
    visibleRows.reduce((groups, row) => {
      const key = row.rootAgentID
      const group = groups.get(key)
      if (group) {
        group.rows.push(row)
      } else {
        groups.set(key, { key, lead: row, rows: [row] })
      }
      return groups
    }, new Map<string, { key: string; lead: CNFlowRow; rows: CNFlowRow[] }>()),
  ).map(([, group]) => ({ ...group, entryGroups: groupRowsByEntry(group.rows) }))

  const selectedEntryGroups = groupRowsByEntry(visibleRows)

  return (
    <Card className={`cn-flow-card cn-flow-card-${flowMode}`} bordered={false}>
      <div className="cn-flow-header">
        <div>
          <div className="eyebrow">CN Access Route Map</div>
          <Title level={3}>{headerTitle}</Title>
        </div>
        <div className="cn-flow-header-actions">
          <Space wrap className="cn-flow-header-buttons">
            <Button disabled={!canOpenXUI} onClick={onOpenXUI}>
              打开 x-ui 面板
            </Button>
            <Button icon={<ReloadOutlined />} type="primary" disabled={!canRefreshCurrentNode} loading={currentNodeLoading} onClick={onRefreshCurrentNode}>
              刷新当前节点
            </Button>
          </Space>
          <Space wrap className="cn-flow-header-meta">
            <Tag color="cyan">链路 {dashboardView.totals.link_count}</Tag>
            <Tag color="gold">出口 {uniqueCountries(rows).length}</Tag>
            <Tag color="blue">客户端 {dashboardView.totals.client_count}</Tag>
            <Tag>节点总数 {dashboardView.totals.node_count}</Tag>
          </Space>
        </div>
      </div>

      <div className="cn-flow-toolbar">
        <button className={`cn-flow-agent-filter${!selectedAgentId ? ' active' : ''}`} onClick={() => onSelectAgent('')}>
          全部链路
        </button>
        {toolbarAgents.map((agent) => (
          <button
            key={agent.agent_id}
            className={`cn-flow-agent-filter${selectedAgentId === agent.agent_id ? ' active' : ''}`}
            onClick={() => onSelectAgent(agent.agent_id)}
          >
            {agent.agent_name || agent.agent_id}
          </button>
        ))}
      </div>

      {visibleRows.length ? (
        <div className="cn-flow-map">
          {renderSource()}
          <div className="cn-flow-lanes">
            {flowMode === 'tag'
              ? tagAgentGroups.map((group) => (
                  <section key={group.key} className="cn-flow-agent-cluster">
                    {renderAgentNode(group.lead)}
                    <div className="cn-flow-arrow cluster-arrow">→</div>
                    <div className="cn-flow-agent-cluster-rows">
                      {group.entryGroups.map(renderEntryCluster)}
                    </div>
                  </section>
                ))
              : flowMode === 'agent'
                ? selectedEntryGroups.map(renderEntryCluster)
                : visibleRows.map((row) => (
                  <section key={row.key} className={`cn-flow-lane${row.loopDetected ? ' loop' : ''}`}>
                    {renderFlowTail(row)}
                  </section>
                  ))}
          </div>
        </div>
      ) : (
        <Empty description="暂无可展示的 CN 访问链路" />
      )}
    </Card>
  )
}

interface CNFlowHop {
  currentAgentID?: string
  currentAgentName?: string
  currentInboundLabel?: string
  currentDetail?: string
  outboundLabel: string
  outboundDetail: string
  outboundTargetIP?: string
  outboundTargetGeo?: IPGeoView
  routeScope?: string
  ruleIndex?: number
  targetAgentID?: string
  targetAgentName?: string
  targetInboundLabel?: string
  targetDetail?: string
  targetProtocol?: string
  targetClientLabel?: string
}

interface CNFlowRow {
  key: string
  rootAgentID: string
  rootAgentName?: string
  rootAgentTags?: string[]
  clientLabel: string
  clientDetail?: string
  entryLabel: string
  hops: CNFlowHop[]
  exitCountryCode: string
  exitCountryLabel: string
  exitReason: string
  loopDetected?: boolean
  unresolvedReason?: string
}

function shouldShowChainWarning(row: CNFlowRow): boolean {
  if (!row.unresolvedReason) {
    return false
  }
  const lastHop = row.hops[row.hops.length - 1]
  return !isExpectedTerminalExit(lastHop?.outboundLabel, lastHop?.outboundDetail, row.unresolvedReason)
}

function buildCNFlowRows(chains: ClientChainView[], agents: DashboardAgentView[]): CNFlowRow[] {
  const agentByID = new Map(agents.map((agent) => [agent.agent_id, agent]))
  const rows = chains.map((chain) => {
    const clientStep = chain.steps.find((step) => step.step_type === 'client')
    const entryStep = chain.steps.find((step) => step.step_type === 'inbound')
    const hops: CNFlowHop[] = []
    for (let index = 0; index < chain.steps.length; index += 1) {
      const step = chain.steps[index]
      if (step.step_type !== 'outbound') {
        continue
      }
      const nextMatch = chain.steps.slice(index + 1).find((item) => item.step_type === 'match')
      const currentInbound = [...chain.steps.slice(0, index)].reverse().find((item) => item.step_type === 'inbound' || item.step_type === 'match')
      hops.push({
        currentAgentID: currentInbound?.agent_id || step.agent_id || chain.root_agent_id,
        currentAgentName: currentInbound?.agent_name || step.agent_name || chain.root_agent_name,
        currentInboundLabel: currentInbound ? `${currentInbound.label}${currentInbound.port ? `:${currentInbound.port}` : ''}` : entryStep?.label || chain.root_inbound_tag,
        currentDetail: currentInbound?.detail,
        outboundLabel: step.label,
        outboundDetail: step.detail || step.target || '-',
        outboundTargetIP: step.target_ip,
        outboundTargetGeo: step.target_geo,
        routeScope: step.route_scope,
        ruleIndex: step.rule_index,
        targetAgentID: nextMatch?.agent_id,
        targetAgentName: nextMatch?.agent_name,
        targetInboundLabel: nextMatch ? formatStepNodeLabel(nextMatch) : undefined,
        targetDetail: nextMatch?.detail,
        targetProtocol: nextMatch?.protocol,
      })
    }
    const lastHop = hops[hops.length - 1]
    const lastOutbound = [...chain.steps].reverse().find((step) => step.step_type === 'outbound')
    const exitAgentID = lastHop?.targetAgentID || lastOutbound?.agent_id || chain.root_agent_id
    const exitAgent = exitAgentID ? agentByID.get(exitAgentID) : undefined
    const country = inferExitCountry(exitAgent, lastOutbound)
    return {
      key: chain.key,
      rootAgentID: chain.root_agent_id,
      rootAgentName: chain.root_agent_name,
      rootAgentTags: chain.root_agent_tags,
      clientLabel: clientStep?.label || chain.root_client_email || 'anonymous-client',
      clientDetail: clientStep?.detail || chain.root_client_remark,
      entryLabel: entryStep ? `${entryStep.label}${entryStep.port ? `:${entryStep.port}` : ''}` : chain.root_inbound_tag || '-',
      hops,
      exitCountryCode: country.code,
      exitCountryLabel: country.label,
      exitReason: buildExitReason(exitAgent, lastOutbound, chain.unresolved_reason),
      loopDetected: chain.loop_detected,
      unresolvedReason: isExpectedTerminalExit(lastOutbound?.label, lastOutbound?.target || lastOutbound?.detail, chain.unresolved_reason)
        ? undefined
        : chain.unresolved_reason,
    }
  })
  const clientsByEntry = new Map<string, string[]>()
  for (const row of rows) {
    const key = flowEntryKey(row.rootAgentID, row.entryLabel)
    clientsByEntry.set(key, [...(clientsByEntry.get(key) || []), row.clientLabel])
  }
  for (const row of rows) {
    for (const hop of row.hops) {
      if (!hop.targetAgentID || !hop.targetInboundLabel) {
        continue
      }
      const labels = clientsByEntry.get(flowEntryKey(hop.targetAgentID, hop.targetInboundLabel)) || []
      if (labels.length === 1) {
        hop.targetClientLabel = labels[0]
      } else if (labels.length > 1) {
        hop.targetClientLabel = `${labels.length} 个节点客户端`
      }
    }
  }
  return rows
}

function formatStepNodeLabel(step: ClientChainStep): string {
  return `${step.label}${step.port ? `:${step.port}` : ''}`
}

function flowEntryKey(agentID: string, label: string): string {
  return `${agentID}:${normalizeNodeAnchorLabel(label).toLowerCase()}`
}

function isExpectedTerminalExit(label?: string, target?: string, unresolvedReason?: string): boolean {
  if (!unresolvedReason?.includes('did not match')) {
    return false
  }
  const terminalText = `${label || ''} ${target || ''}`.toLowerCase()
  return ['direct', 'freedom', 'blocked', 'blackhole'].some((token) => terminalText.includes(token))
}

function inferExitCountry(agent?: DashboardAgentView, outbound?: ClientChainStep): { code: string; label: string } {
  if (outbound?.label === 'blocked') {
    return { code: 'BLOCK', label: 'Blocked' }
  }
  if (outbound?.target_geo?.country_code || outbound?.target_geo?.country_name) {
    return {
      code: outbound.target_geo.country_code || 'GEO',
      label: outbound.target_geo.country_name || outbound.target_geo.country_code || 'GeoIP',
    }
  }
  const haystack = [agent?.agent_name, agent?.agent_id, ...(agent?.tags || []), outbound?.target, outbound?.detail, outbound?.label]
    .filter(Boolean)
    .join(' ')
    .toLowerCase()
  const countryMap: Array<[string, string[], string]> = [
    ['CN', [' cn', 'china', 'mainland', '中国'], 'China'],
    ['HK', ['hk', 'hong kong', '香港'], 'Hong Kong'],
    ['SG', ['sg', 'singapore', '新加坡'], 'Singapore'],
    ['US', ['us', 'usa', 'united states', 'america', 'cox', '美国'], 'United States'],
    ['JP', ['jp', 'japan', '日本'], 'Japan'],
    ['KR', ['kr', 'korea', '韩国'], 'Korea'],
    ['DE', ['de', 'germany', '德国'], 'Germany'],
    ['GB', ['uk', 'gb', 'britain', '英国'], 'United Kingdom'],
  ]
  for (const [code, tokens, label] of countryMap) {
    if (tokens.some((token) => haystack.includes(token.trim()))) {
      return { code, label }
    }
  }
  return { code: 'UNK', label: agent?.agent_name || outbound?.target || 'Unknown Exit' }
}

function buildExitReason(agent?: DashboardAgentView, outbound?: ClientChainStep, unresolvedReason?: string): string {
  if (outbound?.label === 'blocked') {
    return '流量被 blackhole 阻断'
  }
  if (outbound?.label === 'direct') {
    return `${agent?.agent_name || outbound.agent_name || '当前 VPS'} direct 出站`
  }
  if (outbound?.target_geo?.country_name) {
    const location = [outbound.target_geo.country_name, outbound.target_geo.region_name, outbound.target_geo.city].filter(Boolean).join(' / ')
    return `${outbound.target || outbound.detail || outbound.label} · ${outbound.target_geo.ip || outbound.target_ip || 'resolved IP'} · ${location}`
  }
  if (unresolvedReason?.includes('did not match')) {
    return outbound?.target ? `未匹配到下一跳，按 ${outbound.target} 作为出口` : '未匹配到下一跳，按当前 VPS 出口'
  }
  return outbound?.target || agent?.summary.observed_ip || agent?.summary.public_ipv4 || '出口由最后一跳决定'
}

function uniqueCountries(rows: CNFlowRow[]): string[] {
  return Array.from(new Set(rows.map((row) => row.exitCountryCode)))
}

function translateChainReason(reason: string): string {
  if (reason.includes('did not match')) {
    return '最后出站未匹配到下游入站'
  }
  if (reason.includes('loop')) {
    return '检测到循环链路'
  }
  if (reason.includes('missing')) {
    return '配置引用缺失'
  }
  return reason
}
