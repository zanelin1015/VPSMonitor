import type { DashboardAgentView } from '../types'
import { agentCountryCode } from './appHelpersAgent'

export const purposeGroups = [
  { label: '线路机', options: [
    { value: 'domestic_entry', label: '国内入口' },
    { value: 'ix_relay', label: 'IX 中转' },
    { value: 'optimized_relay', label: '优化中转' },
    { value: 'xray_line', label: 'Xray 线路机' },
  ] },
  { label: '落地机', options: [
    { value: 'residential_exit', label: '家宽落地' },
    { value: 'datacenter_exit', label: '机房落地' },
  ] },
]

export const purposeLabels = Object.fromEntries(purposeGroups.flatMap(group => group.options.map(option => [option.value, option.label])))
export const routeTypeLabels = { iepl: 'IEPL 专线', optimized: '优化线路', standard: '普通线路' }

const displayRegions = new Intl.DisplayNames(['zh-CN'], { type: 'region' })

export function classificationCountry(agent: DashboardAgentView) {
  return agent.entry?.classification?.country_code || agentCountryCode(agent)
}

export function classificationCountryLabel(code: string) {
  return code === 'CN' ? '中国大陆' : displayRegions.of(code) || code
}

export function suggestAgentPurposes(agent: DashboardAgentView) {
  const tags = agent.tags || []
  const purposes: string[] = []
  if (tags.includes('线路机') && agent.node_count > 0) purposes.push('xray_line')
  if (tags.includes('落地机')) purposes.push(tags.includes('家宽') ? 'residential_exit' : 'datacenter_exit')
  if (tags.includes('国内入口')) purposes.push('domestic_entry')
  if (tags.includes('IX') || tags.includes('IX 中转')) purposes.push('ix_relay')
  if (tags.includes('优化线路')) purposes.push('optimized_relay')
  return purposes
}
