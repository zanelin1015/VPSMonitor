import type { XUIOutboundView } from '../types'

export interface OutboundsDeletePayload {
  config_fingerprint: string
  outbounds: { tag: string; fingerprint: string }[]
}

const fingerprintPattern = /^[a-f0-9]{64}$/

export function outboundDeleteDisabledReason(outbound: XUIOutboundView): string {
  if (outbound.is_default) return '默认出口不能删除'
  if (outbound.delete_blocked_reason) return outbound.delete_blocked_reason
  if (!outbound.tag || !outbound.fingerprint || !fingerprintPattern.test(outbound.fingerprint)) return '请升级 VPSMonitor Client 并刷新采集，以读取可编辑出站'
  return ''
}

export function buildOutboundsDeletePayload(outbounds: XUIOutboundView[], fingerprint?: string): OutboundsDeletePayload {
  if (!fingerprint || !fingerprintPattern.test(fingerprint)) throw new Error('出站校验信息缺失，请刷新 Client 信息')
  if (!outbounds.length || outbounds.length > 1000) throw new Error('请选择 1 至 1000 个出站')
  const seen = new Set<string>()
  const targets = outbounds.map(outbound => {
    const reason = outboundDeleteDisabledReason(outbound)
    if (reason) throw new Error(reason)
    if (seen.has(outbound.tag!)) throw new Error('出站标签重复，请刷新后重新选择')
    seen.add(outbound.tag!)
    return { tag: outbound.tag!, fingerprint: outbound.fingerprint! }
  })
  return { config_fingerprint: fingerprint, outbounds: targets }
}

export function isPendingOutboundDeletion(action: { kind: string; status: string }): boolean {
  return action.kind === 'delete_outbounds' && ['pending', 'running'].includes(action.status)
}
