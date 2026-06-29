import type { AgentListItem, ClientChainView, GlobalDashboardView, IPGeoView, TopologyLinkView } from '../types'
import type { AgentViewMode } from './appHelperTypes'

const AGENT_VIEW_MODE_STORAGE_PREFIX = 'bridge-core.agent-view-mode.'

export function hasSelectedTag(tags: string[] | undefined, selectedTag: string): boolean {
  if (!selectedTag) {
    return true
  }
  return (tags || []).some((tag) => tag === selectedTag)
}

export function isAgentRunning(agent: AgentListItem): boolean {
  return isRecentTimestamp(agent.realtime_at || agent.last_seen_at || agent.reported_at, 5 * 60 * 1000)
}

export function topologyMatchesSelectedTag(link: TopologyLinkView, selectedTag: string): boolean {
  if (!selectedTag) {
    return true
  }
  return [...(link.source.agent_tags || []), ...(link.target.agent_tags || [])].includes(selectedTag)
}

export function findOutboundLinkedClient(view: GlobalDashboardView | null, agentID: string, outboundTag?: string): TopologyLinkView | undefined {
  if (!view || !agentID || !outboundTag) {
    return undefined
  }
  return (view.links || []).find((link) => link.source.agent_id === agentID && link.source.outbound_tag === outboundTag)
}

export function chainMatchesSelectedTag(chain: ClientChainView, selectedTag: string): boolean {
  if (!selectedTag) {
    return true
  }
  return (chain.root_agent_tags || []).includes(selectedTag)
}

export function readStoredAgentViewMode(username: string): AgentViewMode {
  try {
    const value = window.localStorage.getItem(agentViewModeStorageKey(username))
    return value === 'list' ? 'list' : 'card'
  } catch {
    return 'card'
  }
}

export function storeAgentViewMode(username: string, mode: AgentViewMode) {
  try {
    window.localStorage.setItem(agentViewModeStorageKey(username), mode)
  } catch {
    // Preference storage is optional; the UI still switches for the current session.
  }
}

export function agentViewModeStorageKey(username: string): string {
  return `${AGENT_VIEW_MODE_STORAGE_PREFIX}${username || 'default'}`
}

export function isClientOnline(lastOnline?: number, reportedAt?: string): boolean {
  if (!lastOnline) {
    return false
  }
  const compareAt = reportedAt ? parseTimestampMillis(reportedAt) : Date.now()
  if (!Number.isFinite(compareAt)) {
    return false
  }
  return compareAt - lastOnline <= 5 * 60 * 1000
}

export function xrayIssueLabel(agent: AgentListItem): string {
  const xrayState = (agent.summary.xray_state || '').trim()
  if (!xrayState || xrayState.toLowerCase() === 'running') {
    return ''
  }
  return `Xray ${xrayState}`
}

export function agentDisplayStatus(agent: AgentListItem): { label: string; level: 'ok' | 'warn' | 'bad' | 'neutral' } {
  if (!isAgentRunning(agent)) {
    return { label: 'client 离线', level: 'bad' }
  }
  if (agent.summary.last_collection_err || xrayIssueLabel(agent)) {
    return { label: 'client 警告', level: 'warn' }
  }
  return { label: 'running', level: 'ok' }
}

export function countryFlag(code?: string): string {
  const normalized = (code || '').trim().toUpperCase()
  if (!/^[A-Z]{2}$/.test(normalized)) {
    return '🌐'
  }
  return Array.from(normalized)
    .map((char) => String.fromCodePoint(127397 + char.charCodeAt(0)))
    .join('')
}

export function agentCountryCode(agent: AgentListItem): string {
  const explicitCode = explicitAgentCountryCode(agent)
  if (explicitCode) {
    return explicitCode
  }
  const geoCode = normalizeCountryCode(agent.geo?.country_code)
  return geoCode || ''
}

export function explicitAgentCountryCode(agent: AgentListItem): string {
  const candidates = [agent.agent_name || '', ...(agent.tags || []), agent.summary.hostname || '', agent.agent_id || '']
  for (const value of candidates) {
    const code = explicitCountryCodeFromText(value)
    if (code) {
      return code
    }
  }
  return ''
}

export function explicitCountryCodeFromText(value?: string): string {
  const text = (value || '').trim().toUpperCase()
  if (!text) {
    return ''
  }
  const direct = normalizeCountryCode(text)
  if (direct) {
    return direct
  }
  const match = /(?:^|[^A-Z0-9])(TH|MY|VN|IN|SG|HK|MO|TW|JP|KR|CA|US|CN|PH|DE|FR|GB|AU)(?=$|[^A-Z0-9])/.exec(text)
  return match ? match[1] : ''
}

export function normalizeCountryCode(value?: string): string {
  const code = (value || '').trim().toUpperCase()
  if (['TH', 'MY', 'VN', 'IN', 'SG', 'HK', 'MO', 'TW', 'JP', 'KR', 'CA', 'US', 'CN', 'PH', 'DE', 'FR', 'GB', 'AU'].includes(code)) {
    return code
  }
  return ''
}

export function formatAgentLocation(agent: AgentListItem, displayCountryCode: string): string {
  const geoCode = normalizeCountryCode(agent.geo?.country_code)
  const geoLabel = formatGeoLabel(agent.geo)
  if (!displayCountryCode) {
    return geoLabel
  }
  const displayCountry = countryName(displayCountryCode)
  if (!geoLabel) {
    return displayCountry
  }
  if (geoCode && geoCode !== displayCountryCode) {
    return `${displayCountry} · IP库: ${geoLabel}`
  }
  return geoLabel || displayCountry
}

export function formatGeoLabel(geo?: IPGeoView): string {
  if (!geo) {
    return ''
  }
  return [geo.country_name || geo.country_code, geo.region_name, geo.city].filter(Boolean).join(' · ')
}

export function countryName(code: string): string {
  switch (code) {
    case 'TH':
      return 'Thailand'
    case 'MY':
      return 'Malaysia'
    case 'VN':
      return 'Vietnam'
    case 'IN':
      return 'India'
    case 'SG':
      return 'Singapore'
    case 'HK':
      return 'Hong Kong'
    case 'MO':
      return 'Macao'
    case 'TW':
      return 'Taiwan'
    case 'JP':
      return 'Japan'
    case 'KR':
      return 'South Korea'
    case 'CA':
      return 'Canada'
    case 'US':
      return 'United States'
    case 'CN':
      return 'China'
    case 'PH':
      return 'Philippines'
    case 'DE':
      return 'Germany'
    case 'FR':
      return 'France'
    case 'GB':
      return 'United Kingdom'
    case 'AU':
      return 'Australia'
    default:
      return code
  }
}

export function outboundElementId(tag: string): string {
  return `outbound-${sanitizeFragment(tag)}`
}

export function ruleElementId(index: number): string {
  return `rule-${index}`
}

export function nodeElementId(agentID: string, nodeLabel: string): string {
  return `node-${sanitizeFragment(agentID)}-${sanitizeFragment(normalizeNodeAnchorLabel(nodeLabel))}`
}

export function normalizeNodeAnchorLabel(value: string): string {
  return value.replace(/:\d+$/, '').trim()
}

export function sanitizeFragment(value: string): string {
  return value.replace(/[^a-zA-Z0-9_-]/g, '-')
}

function parseTimestampMillis(value?: string): number {
  const candidates = parseTimestampMillisCandidates(value)
  return candidates.length ? candidates[0] : Number.NaN
}

function isRecentTimestamp(value: string | undefined, ttlMs: number): boolean {
  const now = Date.now()
  return parseTimestampMillisCandidates(value).some((seenAt) => {
    const diff = now - seenAt
    return diff >= -60_000 && diff <= ttlMs
  })
}

function parseTimestampMillisCandidates(value?: string): number[] {
  const text = String(value || '').trim()
  if (!text) {
    return []
  }
  const candidates: number[] = []
  const add = (candidate: string) => {
    const parsed = Date.parse(candidate)
    if (Number.isFinite(parsed) && !candidates.includes(parsed)) {
      candidates.push(parsed)
    }
  }

  add(text)

  const normalized = text.replace(/^(\d{4})\/(\d{2})\/(\d{2})/, '$1-$2-$3').replace(' ', 'T')
  const hasTime = /T\d{2}:\d{2}/.test(normalized)
  const hasTimezone = /(?:Z|[+-]\d{2}:?\d{2})$/i.test(normalized)
  if (hasTime && !hasTimezone) {
    add(`${normalized}Z`)
  }
  return candidates
}
