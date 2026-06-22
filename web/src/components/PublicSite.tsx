import { useEffect, useMemo, useState } from 'react'
import { Alert, Button, Spin, Tag } from 'antd'
import { ArrowRightOutlined, GlobalOutlined, ReloadOutlined } from '@ant-design/icons'

import type { DashboardAgentView, GlobalDashboardView, TopologyLinkView } from '../types'
import { fetchJSON, formatDateTime } from '../lib/appHelpers'
import { VisualEffects } from './VisualEffects'

export function PublicSite() {
  const [view, setView] = useState<GlobalDashboardView | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    void load()
  }, [])

  const countries = useMemo(() => {
    const values = new Map<string, string>()
    ;(view?.agents || []).forEach((agent) => {
      const code = agent.geo?.country_code || ''
      const name = agent.geo?.country_name || ''
      if (code || name) {
        values.set(code || name, name || code)
      }
    })
    ;(view?.links || []).forEach((link) => {
      const code = link.source.target_geo?.country_code || ''
      const name = link.source.target_geo?.country_name || ''
      if (code || name) {
        values.set(code || name, name || code)
      }
    })
    return Array.from(values.values()).filter(Boolean)
  }, [view])

  async function load() {
    setLoading(true)
    setError('')
    try {
      const data = await fetchJSON<GlobalDashboardView>('/api/v1/public/topology')
      setView(data)
    } catch (loadError) {
      setError(loadError instanceof Error ? loadError.message : '拓扑加载失败')
    } finally {
      setLoading(false)
    }
  }

  const agents = view?.agents || []
  const links = view?.links || []
  const visibleLinks = links.slice(0, 8)

  return (
    <>
      <VisualEffects />
      <main className="public-site-shell">
        <section className="public-hero">
          <div className="public-hero-copy">
            <div className="public-brand-row">
              <span className="public-brand-mark">南</span>
              <span>VPSMonitor</span>
            </div>
            <h1>VPSMonitor</h1>
            <p>跨区域 VPS、Realm 中转与 X-UI 节点的统一拓扑视图。</p>
            <div className="public-hero-actions">
              <Button type="primary" size="large" href="/admin">
                进入控制台
              </Button>
              <Button size="large" icon={<ReloadOutlined />} loading={loading} onClick={() => void load()}>
                刷新拓扑
              </Button>
            </div>
            <div className="public-metrics">
              <Metric label="拓扑节点" value={view?.totals.agent_count || agents.length || 0} />
              <Metric label="链路" value={view?.totals.link_count || links.length || 0} />
              <Metric label="覆盖区域" value={countries.length || 0} />
            </div>
          </div>

          <div className="public-topology-scene" aria-label="公开拓扑图">
            <div className="public-topology-scene-header">
              <span><GlobalOutlined /> Public Route Map</span>
              <small>{view?.generated_at ? formatDateTime(view.generated_at) : '等待数据'}</small>
            </div>
            {loading && !view ? (
              <div className="public-topology-loading">
                <Spin />
              </div>
            ) : error ? (
              <Alert type="warning" showIcon message="拓扑暂不可用" description={error} />
            ) : (
              <PublicTopologyMap agents={agents} links={visibleLinks} />
            )}
          </div>
        </section>

        <section className="public-section public-section-stats">
          <div>
            <h2>链路状态</h2>
            <p>公开页面仅展示脱敏后的节点关系，不显示后台 Client 原始名称、agent_id、客户客户端、IP 或域名。</p>
          </div>
          <div className="public-stat-strip">
            <Metric label="X-UI 节点" value={view?.totals.node_count || 0} />
            <Metric label="出站规则" value={view?.totals.outbound_count || 0} />
            <Metric label="路由规则" value={view?.totals.routing_rule_count || 0} />
          </div>
        </section>

        <section className="public-section public-link-section">
          <div className="public-section-heading">
            <h2>实时拓扑</h2>
            <span>{links.length ? `展示 ${Math.min(links.length, visibleLinks.length)} / ${links.length} 条链路` : '暂无链路'}</span>
          </div>
          <div className="public-link-list">
            {visibleLinks.length ? visibleLinks.map((link) => <PublicLinkRow key={link.key} link={link} />) : (
              <div className="public-empty-line">等待 Client 上报拓扑数据</div>
            )}
          </div>
        </section>
      </main>
    </>
  )
}

function PublicTopologyMap(props: { agents: DashboardAgentView[]; links: TopologyLinkView[] }) {
  const { agents, links } = props
  if (!agents.length) {
    return <div className="public-empty-line">暂无公开拓扑数据</div>
  }
  const activeIDs = new Set<string>()
  links.forEach((link) => {
    activeIDs.add(link.source.agent_id)
    activeIDs.add(link.target.agent_id)
  })
  const nodes = agents.filter((agent) => activeIDs.size === 0 || activeIDs.has(agent.agent_id)).slice(0, 10)
  return (
    <div className="public-topology-map">
      <div className="public-cn-orbit">CN</div>
      <div className="public-node-cloud">
        {nodes.map((agent, index) => (
          <div key={agent.agent_id} className={`public-topology-node public-topology-node-${index % 5}`}>
            <span>{agent.agent_name || agent.agent_id}</span>
            <small>{agent.geo?.country_name || agent.tags?.[0] || 'Edge'}</small>
          </div>
        ))}
      </div>
      <div className="public-link-beams">
        {links.slice(0, 6).map((link, index) => (
          <span key={link.key} className={`public-link-beam public-link-beam-${index % 4}`} />
        ))}
      </div>
    </div>
  )
}

function PublicLinkRow({ link }: { link: TopologyLinkView }) {
  return (
    <div className="public-link-row">
      <strong>{link.source.agent_name || link.source.agent_id}</strong>
      <Tag color="gold">{link.source.protocol || 'outbound'}</Tag>
      <ArrowRightOutlined />
      <strong>{link.target.agent_name || link.target.agent_id}</strong>
      <Tag color="cyan">{link.target.protocol || 'inbound'}</Tag>
      <span>{link.source.target_geo?.country_name || link.target.agent_tags?.[0] || 'Global'}</span>
    </div>
  )
}

function Metric({ label, value }: { label: string; value: number }) {
  return (
    <div className="public-metric">
      <strong>{value}</strong>
      <span>{label}</span>
    </div>
  )
}
