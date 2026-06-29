import { useEffect, useMemo, useState } from 'react'
import { Alert, Button, Spin } from 'antd'
import { ArrowRightOutlined, GlobalOutlined, ReloadOutlined } from '@ant-design/icons'
import worldMap from '@svg-maps/world'

import type { ClientChainView, DashboardAgentView, GlobalDashboardView, TopologyLinkView } from '../types'
import { fetchJSON, formatDateTime } from '../lib/appHelpers'
import { VisualEffects } from './VisualEffects'

interface PublicCountrySummary {
  code: string
  name: string
  count: number
}

interface PublicCountryPosition {
  x: number
  y: number
}

interface PublicMapLocation {
  id: string
  name: string
  path: string
}

const PUBLIC_COUNTRY_POSITIONS: Record<string, PublicCountryPosition> = {
  AU: { x: 830, y: 545 },
  BR: { x: 340, y: 472 },
  CA: { x: 210, y: 205 },
  CN: { x: 740, y: 350 },
  DE: { x: 505, y: 250 },
  FR: { x: 485, y: 272 },
  GB: { x: 462, y: 232 },
  HK: { x: 798, y: 386 },
  ID: { x: 785, y: 462 },
  IN: { x: 685, y: 392 },
  JP: { x: 855, y: 315 },
  KR: { x: 817, y: 315 },
  MY: { x: 744, y: 432 },
  NL: { x: 492, y: 237 },
  PH: { x: 823, y: 407 },
  RU: { x: 650, y: 205 },
  SG: { x: 754, y: 456 },
  TH: { x: 738, y: 408 },
  TR: { x: 575, y: 305 },
  TW: { x: 817, y: 372 },
  US: { x: 200, y: 342 },
  VN: { x: 758, y: 410 },
}

const PUBLIC_COUNTRY_ALIASES: Record<string, string> = {
  AMERICA: 'US',
  AUSTRALIA: 'AU',
  BRAZIL: 'BR',
  CANADA: 'CA',
  CHINA: 'CN',
  FRANCE: 'FR',
  GERMANY: 'DE',
  'HONG KONG': 'HK',
  INDIA: 'IN',
  INDONESIA: 'ID',
  JAPAN: 'JP',
  KOREA: 'KR',
  MALAYSIA: 'MY',
  NETHERLANDS: 'NL',
  PHILIPPINES: 'PH',
  RUSSIA: 'RU',
  SINGAPORE: 'SG',
  TAIWAN: 'TW',
  THAILAND: 'TH',
  TURKEY: 'TR',
  'UNITED KINGDOM': 'GB',
  'UNITED STATES': 'US',
  'UNITED STATES OF AMERICA': 'US',
  VIETNAM: 'VN',
}

export function PublicSite() {
  const [view, setView] = useState<GlobalDashboardView | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    void load()
  }, [])

  const landingCountries = useMemo(() => collectLandingCountries(view), [view])

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
  const chains = view?.client_chains || []
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
              <Metric label="覆盖区域" value={landingCountries.length || 0} />
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
              <PublicTopologyMap agents={agents} links={visibleLinks} chains={chains} />
            )}
          </div>
        </section>

        <PublicWorldMap countries={landingCountries} />
      </main>
    </>
  )
}

function PublicWorldMap({ countries }: { countries: PublicCountrySummary[] }) {
  const [hoveredCountry, setHoveredCountry] = useState<PublicCountrySummary | null>(null)
  const activeCodes = new Set(countries.map((country) => country.code.toLowerCase()))
  const countryByCode = new Map(countries.map((country) => [country.code.toLowerCase(), country]))
  const visibleMarkers = countries.filter((country) => PUBLIC_COUNTRY_POSITIONS[country.code]).slice(0, 12)
  return (
    <section className="public-section public-world-section">
      <div className="public-world-panel">
        <div className="public-world-title">
          <h2>{countries.length ? `服务器分布在 ${countries.length} 个区域` : '等待拓扑 Geo 数据'}</h2>
          <span>Public Landing Map</span>
        </div>
        <div className="public-world-map" aria-label="全球落地区域地图">
          <svg viewBox={worldMap.viewBox} role="img">
            <defs>
              <radialGradient id="publicMapGlow" cx="50%" cy="50%" r="50%">
                <stop offset="0%" stopColor="#38bdf8" stopOpacity="0.9" />
                <stop offset="62%" stopColor="#14b8a6" stopOpacity="0.22" />
                <stop offset="100%" stopColor="#14b8a6" stopOpacity="0" />
              </radialGradient>
            </defs>
            <g className="public-map-countries">
              {(worldMap.locations as PublicMapLocation[]).map((location) => {
                const summary = countryByCode.get(location.id)
                return (
                  <path
                    key={location.id}
                    className={`public-map-land ${activeCodes.has(location.id) ? 'active' : ''}`}
                    d={location.path}
                    onMouseEnter={() => setHoveredCountry(summary || null)}
                    onMouseLeave={() => setHoveredCountry(null)}
                  >
                    <title>{summary ? `${summary.name} · ${summary.count} 条` : location.name}</title>
                  </path>
                )
              })}
            </g>
            {visibleMarkers.map((country) => {
              const point = PUBLIC_COUNTRY_POSITIONS[country.code]
              if (!point) {
                return null
              }
              return (
                <g
                  key={country.code}
                  className="public-map-point"
                  onMouseEnter={() => setHoveredCountry(country)}
                  onMouseLeave={() => setHoveredCountry(null)}
                >
                  <circle className="public-map-glow" cx={point.x} cy={point.y} r="34" />
                  <circle className="public-map-dot" cx={point.x} cy={point.y} r="6" />
                </g>
              )
            })}
          </svg>
          {hoveredCountry ? (
            <div className="public-map-tooltip">
              <strong>{hoveredCountry.name}</strong>
              <span>{hoveredCountry.count} 个节点</span>
            </div>
          ) : null}
        </div>
        <div className="public-world-list" aria-label="落地区域列表">
          {countries.length ? countries.map((country) => (
            <div key={country.code} className="public-world-chip active">
              <strong>{country.code}</strong>
              <span>{country.name}</span>
              <em>{country.count}</em>
            </div>
          )) : (
            <div className="public-empty-line">等待公开拓扑解析落地国家</div>
          )}
        </div>
      </div>
    </section>
  )
}

function PublicTopologyMap(props: { agents: DashboardAgentView[]; links: TopologyLinkView[]; chains: ClientChainView[] }) {
  const { agents, links, chains } = props
  if (!agents.length && !chains.length) {
    return <div className="public-empty-line">暂无公开拓扑数据</div>
  }
  const agentByID = new Map(agents.map((agent) => [agent.agent_id, agent]))
  const rows = chains.length ? chains.slice(0, 8) : buildAgentFallbackChains(agents, links)
  return (
    <div className="public-topology-map">
      <div className="public-cn-rail">
        <div className="public-cn-orbit">
          <span>CN</span>
          <small>访问入口</small>
        </div>
      </div>
      <div className="public-chain-lanes">
        {rows.map((chain, index) => {
          const outbound = publicPrimaryOutbound(chain)
          const exit = publicExitCountry(chain, agentByID)
          return (
            <div key={chain.key || `${chain.root_agent_id}-${index}`} className="public-chain-row">
              <span className={`public-route-line public-route-line-${index % 4}`} />
              <div className={`public-chain-card public-chain-entry public-topology-node-${index % 4}`}>
                <span className="public-chain-kicker">CLIENT VPS / 入站</span>
                <strong>{chain.root_agent_name || chain.root_agent_id}</strong>
                <small>{publicEntryLabel(chain)}</small>
              </div>
              <ArrowRightOutlined className="public-chain-arrow" />
              <div className="public-chain-card public-chain-outbound">
                <span className="public-chain-kicker">公开出站链路</span>
                <strong>{outbound?.label || 'Direct'}</strong>
                <small>{publicStepDetail(outbound)}</small>
              </div>
              <ArrowRightOutlined className="public-chain-arrow public-chain-arrow-final" />
              <div className="public-chain-card public-chain-country">
                <span className="public-chain-kicker">最终出站国家</span>
                <strong>{exit.label}</strong>
                <small>{exit.detail}</small>
              </div>
            </div>
          )
        })}
      </div>
      <div className="public-topology-summary">
        <strong>{chains.length || links.length}</strong>
        <span>公开链路</span>
      </div>
    </div>
  )
}

function collectLandingCountries(view: GlobalDashboardView | null): PublicCountrySummary[] {
  const values = new Map<string, PublicCountrySummary>()
  const addCountry = (code?: string, name?: string) => {
    const normalizedCode = normalizePublicCountryCode(code, name)
    const normalizedName = (name || code || '').trim()
    if (!normalizedCode && !normalizedName) {
      return
    }
    const key = normalizedCode || normalizedName
    const current = values.get(key)
    if (current) {
      current.count += 1
      if (!current.name && normalizedName) {
        current.name = normalizedName
      }
      return
    }
    values.set(key, {
      code: normalizedCode || normalizedName,
      name: normalizedName || normalizedCode,
      count: 1,
    })
  }
  const agentByID = new Map((view?.agents || []).map((agent) => [agent.agent_id, agent]))
  ;(view?.agents || []).forEach((agent) => addCountry(agent.geo?.country_code, agent.geo?.country_name))
  if (values.size) {
    return sortPublicCountries(values)
  }
  ;(view?.client_chains || []).forEach((chain) => {
    const exitStep = [...(chain.steps || [])].reverse().find((step) => step.target_geo?.country_code || step.target_geo?.country_name)
    if (exitStep?.target_geo) {
      addCountry(exitStep.target_geo.country_code, exitStep.target_geo.country_name)
      return
    }
    const agent = agentByID.get(chain.root_agent_id)
    addCountry(agent?.geo?.country_code, agent?.geo?.country_name)
  })
  ;(view?.links || []).forEach((link) => addCountry(link.source.target_geo?.country_code, link.source.target_geo?.country_name))
  return sortPublicCountries(values)
}

function sortPublicCountries(values: Map<string, PublicCountrySummary>): PublicCountrySummary[] {
  return Array.from(values.values()).sort((a, b) => {
    if (b.count !== a.count) {
      return b.count - a.count
    }
    return a.name.localeCompare(b.name)
  })
}

function normalizePublicCountryCode(code?: string, name?: string): string {
  const rawCode = (code || '').trim().toUpperCase()
  if (rawCode && rawCode.length <= 3) {
    return rawCode
  }
  const rawName = (name || code || '').trim().toUpperCase()
  return PUBLIC_COUNTRY_ALIASES[rawName] || rawCode || rawName
}

function countAgentLinks(agentID: string, links: TopologyLinkView[]): number {
  return links.filter((link) => link.source.agent_id === agentID || link.target.agent_id === agentID).length
}

function buildAgentFallbackChains(agents: DashboardAgentView[], links: TopologyLinkView[]): ClientChainView[] {
  return agents.slice(0, 8).map((agent) => ({
    key: agent.agent_id,
    root_agent_id: agent.agent_id,
    root_agent_name: agent.agent_name || agent.agent_id,
    root_agent_tags: agent.tags || [],
    root_client_enabled: true,
    matched_link_count: countAgentLinks(agent.agent_id, links),
    steps: [],
  }))
}

function publicPrimaryOutbound(chain: ClientChainView) {
  return chain.steps.find((step) => step.step_type === 'outbound' || step.step_type === 'balancer')
}

function publicExitCountry(chain: ClientChainView, agentByID: Map<string, DashboardAgentView>): { label: string; detail: string } {
  const outbound = [...chain.steps].reverse().find((step) => step.target_geo?.country_name || step.target_geo?.country_code)
  if (outbound?.target_geo) {
    const geo = outbound.target_geo
    return {
      label: geo.country_name || geo.country_code || 'Global',
      detail: [geo.region_name, geo.city].filter(Boolean).join(' / ') || '公开出站区域',
    }
  }
  const agent = agentByID.get(chain.root_agent_id)
  if (agent?.geo?.country_name || agent?.geo?.country_code) {
    return {
      label: agent.geo.country_name || agent.geo.country_code || 'Global',
      detail: [agent.geo.region_name, agent.geo.city].filter(Boolean).join(' / ') || 'Client 所在区域',
    }
  }
  const tags = chain.root_agent_tags || []
  return { label: tags[0] || 'Global', detail: '公开出站区域' }
}

function publicEntryLabel(chain: ClientChainView): string {
  const inbound = chain.steps.find((step) => step.step_type === 'inbound' || step.step_type === 'match')
  return inbound?.detail || inbound?.label || chain.root_inbound_tag || '公开入口'
}

function publicStepDetail(step: ClientChainView['steps'][number] | undefined): string {
  if (!step) {
    return '本机 direct 出站'
  }
  return step.detail || [step.protocol, step.port ? `端口 ${step.port}` : ''].filter(Boolean).join(' · ') || '公开规则'
}

function Metric({ label, value }: { label: string; value: number }) {
  return (
    <div className="public-metric">
      <strong>{value}</strong>
      <span>{label}</span>
    </div>
  )
}
