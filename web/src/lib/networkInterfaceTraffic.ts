import type { NetworkInterfaceTrafficUsage } from '../types'

export type NetworkInterfaceTrafficMode = 'both' | 'sent' | 'received'

export function suggestedNetworkInterfaceKeys(items: NetworkInterfaceTrafficUsage[]): string[] {
  const available = items.filter(item => item.present && item.up && !item.loopback)
  const selected = available.find(item => item.primary) || available.find(item => item.kind === 'physical') || available[0]
  return selected ? [selected.key] : []
}

export function networkInterfaceTrafficValue(item: Pick<NetworkInterfaceTrafficUsage, 'received_bytes' | 'sent_bytes'>, mode: NetworkInterfaceTrafficMode): number {
  if (mode === 'received') return item.received_bytes
  if (mode === 'sent') return item.sent_bytes
  return item.received_bytes + item.sent_bytes
}

export function selectedNetworkInterfaceTotals(items: NetworkInterfaceTrafficUsage[], keys: string[]) {
  const selected = new Set(keys)
  return items.reduce((sum, item) => selected.has(item.key) ? { received_bytes: sum.received_bytes + item.received_bytes, sent_bytes: sum.sent_bytes + item.sent_bytes } : sum, { received_bytes: 0, sent_bytes: 0 })
}
