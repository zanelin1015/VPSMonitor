import type { AgentListItem, VPSRenewalConfig, VPSSummary, XUIClientView } from '../types'

export interface TrafficMeterStatus {
  label: string
  level: 'ok' | 'warn' | 'bad'
  used: number
  total: number
  percent?: number
}

export interface AgentNetworkSummary {
  sent: number
  recv: number
  up: number
  down: number
}

export function calculateTrafficStatus(agent: AgentListItem): {
  isPeriod: boolean
  total: TrafficMeterStatus
  upload: TrafficMeterStatus
  download: TrafficMeterStatus
} {
  const limit = Number(agent.renewal?.traffic_limit_bytes || 0)
  const currentTotal = Number(agent.summary.net_traffic_total || 0)
  const currentUpload = Number(agent.summary.net_traffic_sent || 0)
  const currentDownload = Number(agent.summary.net_traffic_recv || 0)
  const baselineTotal = Number(agent.renewal?.traffic_baseline_bytes || 0)
  const baselineUpload = Number(agent.renewal?.traffic_sent_baseline_bytes || 0)
  const baselineDownload = Number(agent.renewal?.traffic_recv_baseline_bytes || 0)
  const totalUsed = periodTrafficUsed(currentTotal || currentUpload + currentDownload, baselineTotal)
  const uploadUsed = periodTrafficUsed(currentUpload, baselineUpload)
  const downloadUsed = periodTrafficUsed(currentDownload, baselineDownload)
  return {
    isPeriod: usesRenewalTrafficCycle(agent.renewal),
    total: buildTrafficMeter(totalUsed, limit),
    upload: buildTrafficMeter(uploadUsed, limit),
    download: buildTrafficMeter(downloadUsed, limit),
  }
}

function usesRenewalTrafficCycle(config?: VPSRenewalConfig): boolean {
  return Boolean(
    Number(config?.traffic_limit_bytes || 0) > 0 &&
      config?.auto_renew &&
      config?.cycle &&
      (config?.start_date || config?.expire_date),
  )
}

export function clientTrafficTotal(client: XUIClientView): number {
  const up = Number(client.up || 0)
  const down = Number(client.down || 0)
  const allTime = Number(client.all_time || 0)
  const trafficTotal = Number(client.traffic_total || 0)
  return allTime || up + down || trafficTotal
}

function buildTrafficMeter(used: number, total: number): TrafficMeterStatus {
  if (!total) {
    return {
      label: formatBytes(used),
      level: 'ok',
      used,
      total,
    }
  }
  const percent = total ? (used / total) * 100 : 0
  return {
    label: `${formatBytes(used)} / ${formatBytes(total)} (${Math.min(999, percent).toFixed(1)}%)`,
    level: percent >= 90 ? 'bad' : percent >= 75 ? 'warn' : 'ok',
    used,
    total,
    percent: clampMetricPercent(percent),
  }
}

function periodTrafficUsed(current: number, baseline: number): number {
  if (!current) {
    return 0
  }
  return current >= baseline ? current - baseline : current
}

export function summarizeAgentNetwork(agents: AgentListItem[]): AgentNetworkSummary {
  return agents.reduce<AgentNetworkSummary>(
    (summary, agent) => {
      summary.sent += Number(agent.summary.net_traffic_sent || 0)
      summary.recv += Number(agent.summary.net_traffic_recv || 0)
      summary.up += Number(agent.summary.net_io_up || 0)
      summary.down += Number(agent.summary.net_io_down || 0)
      return summary
    },
    { sent: 0, recv: 0, up: 0, down: 0 },
  )
}

export function gbToBytes(value: number): number {
  return Math.round(Math.max(0, value) * 1024 * 1024 * 1024)
}

export function bytesToGB(value: number): number | undefined {
  if (!value) {
    return undefined
  }
  return Number((value / 1024 / 1024 / 1024).toFixed(2))
}

export function formatBytes(value: number): string {
  if (!value) {
    return '0 B'
  }
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  let current = value
  let unitIndex = 0
  while (current >= 1024 && unitIndex < units.length - 1) {
    current /= 1024
    unitIndex++
  }
  return `${current >= 100 || unitIndex === 0 ? current.toFixed(0) : current.toFixed(1)} ${units[unitIndex]}`
}

export function formatBandwidth(value: number): string {
  if (!value) {
    return '-'
  }
  if (value >= 1000) {
    return `${(value / 1000).toFixed(value % 1000 === 0 ? 0 : 2)} Gbps`
  }
  return `${value.toFixed(value % 1 === 0 ? 0 : 2)} Mbps`
}

export function formatSpeed(value?: number): string {
  return `${formatBytes(Number(value || 0))}/s`
}

export function formatPercent(value?: number): string {
  if (!value) {
    return '0%'
  }
  return `${value.toFixed(1)}%`
}

export function clampMetricPercent(value?: number): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    return 0
  }
  return Math.max(0, Math.min(100, value))
}

export function calculateMemoryPercent(summary?: VPSSummary): number {
  if (!summary?.mem_total) {
    return 0
  }
  return clampMetricPercent(((summary.mem_used || 0) / summary.mem_total) * 100)
}

export function metricLevel(percent: number): 'ok' | 'warn' | 'bad' {
  if (percent >= 90) {
    return 'bad'
  }
  if (percent >= 75) {
    return 'warn'
  }
  return 'ok'
}

export function formatMem(used?: number, total?: number): string {
  if (!used && !total) {
    return '-'
  }
  if (!total) {
    return formatBytes(used || 0)
  }
  const percent = total ? ((used || 0) / total) * 100 : 0
  return `${formatBytes(used || 0)} / ${formatBytes(total)} (${percent.toFixed(1)}%)`
}
