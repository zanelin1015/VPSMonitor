import type { DashboardTagView } from '../types'

const TAG_COLOR_PALETTE = [
  { bg: '#dbeafe', border: '#93c5fd', text: '#1d4ed8' },
  { bg: '#dcfce7', border: '#86efac', text: '#15803d' },
  { bg: '#fef3c7', border: '#fcd34d', text: '#b45309' },
  { bg: '#fce7f3', border: '#f9a8d4', text: '#be185d' },
  { bg: '#ede9fe', border: '#c4b5fd', text: '#6d28d9' },
  { bg: '#ccfbf1', border: '#5eead4', text: '#0f766e' },
  { bg: '#fee2e2', border: '#fca5a5', text: '#b91c1c' },
  { bg: '#e0f2fe', border: '#7dd3fc', text: '#0369a1' },
]

export function parseTagInput(rawText: string): string[] {
  const seen = new Set<string>()
  const result: string[] = []
  rawText
    .split(/[,\n，、]/)
    .map((item) => item.trim())
    .filter(Boolean)
    .forEach((item) => {
      const key = item.toLowerCase()
      if (!seen.has(key)) {
        seen.add(key)
        result.push(item)
      }
    })
  return result.sort((left, right) => left.localeCompare(right))
}

export function formatTagInput(tags: string[] | undefined): string {
  return (tags || []).join(', ')
}

export function mergeTagOptions(current: string[], incoming: string[]): string[] {
  return parseTagInput([...current, ...incoming].join(','))
}

export function mergeDashboardTagOptions(dashboardTags: DashboardTagView[], tagOptions: string[]): DashboardTagView[] {
  const byTag = new Map<string, DashboardTagView>()
  for (const tag of dashboardTags) {
    byTag.set(tag.tag, tag)
  }
  for (const tag of tagOptions) {
    if (!byTag.has(tag)) {
      byTag.set(tag, { tag, agent_count: 0, node_count: 0, client_count: 0, online_client_count: 0 })
    }
  }
  return Array.from(byTag.values()).sort((left, right) => left.tag.localeCompare(right.tag))
}

export function parseAddressInput(rawText: string): string[] {
  const seen = new Set<string>()
  const result: string[] = []
  rawText
    .split(/[,\n，、]/)
    .map((item) => item.trim())
    .filter(Boolean)
    .forEach((item) => {
      const key = item.toLowerCase()
      if (!seen.has(key)) {
        seen.add(key)
        result.push(item)
      }
    })
  return result.sort((left, right) => left.localeCompare(right))
}

export function formatAddressInput(addresses: string[] | undefined): string {
  return (addresses || []).join('\n')
}

export function tagChipStyle(tag: string, active = false) {
  const normalized = tag.trim().toLowerCase()
  let hash = 0
  for (let index = 0; index < normalized.length; index += 1) {
    hash = (hash * 31 + normalized.charCodeAt(index)) >>> 0
  }
  const color = TAG_COLOR_PALETTE[hash % TAG_COLOR_PALETTE.length]
  return {
    backgroundColor: active ? color.text : color.bg,
    borderColor: color.border,
    color: active ? '#ffffff' : color.text,
    boxShadow: active ? `0 0 0 3px ${color.bg}` : undefined,
  }
}
