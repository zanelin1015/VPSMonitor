import type { AgentListItem, NetworkPortPolicyRule, XUILocalCertificate } from '../types'

export function buildCertificateDomainOptions(certificates: XUILocalCertificate[]) {
  const seen = new Set<string>()
  const options: { value: string; label: string }[] = []
  certificates.forEach((certificate) => {
    const names = certificate.dns_names || []
    names.forEach((name) => {
      const value = normalizeDomainOption(name)
      if (!value || seen.has(value)) {
        return
      }
      seen.add(value)
      options.push({
        value,
        label: certificate.name ? `${value} · ${certificate.name}` : value,
      })
    })
  })
  return options
}

function normalizeDomainOption(value?: string) {
  const domain = (value || '').trim().toLowerCase().replace(/\.$/, '')
  if (!domain || domain.includes(' ') || domain.includes('*') || isIPLike(domain)) {
    return ''
  }
  return domain
}

function isIPLike(value: string) {
  return /^\d{1,3}(\.\d{1,3}){3}$/.test(value) || (value.includes(':') && /^[0-9a-f:]+$/i.test(value))
}

export function normalizeNetworkPolicyRules(rules: NetworkPortPolicyRule[]) {
  return dedupeNetworkPolicyRules(rules.map((rule) => ({
    ...rule,
    enabled: rule.enabled !== false,
    whitelist_ips: rule.whitelist_ips || [],
  })))
}

function dedupeNetworkPolicyRules(rules: NetworkPortPolicyRule[]) {
  const byPort = new Map<number, NetworkPortPolicyRule>()
  rules.forEach((rule) => {
    const port = Number(rule.port || 0)
    if (!port) {
      return
    }
    const existing = byPort.get(port)
    if (!existing) {
      byPort.set(port, {
        ...rule,
        id: rule.id || `port-${port}`,
        name: normalizedNetworkPolicyRuleName(rule.name, port),
        port,
        protocol: normalizeNetworkPolicyProtocol(rule.protocol),
      })
      return
    }
    byPort.set(port, {
      ...existing,
      protocol: mergeNetworkPolicyProtocol(existing.protocol, rule.protocol),
      rate_limit_mbps: mergeNetworkPolicyRate(existing.rate_limit_mbps, rule.rate_limit_mbps),
      whitelist_ips: Array.from(new Set([...(existing.whitelist_ips || []), ...(rule.whitelist_ips || [])])),
      name: shouldUsePortAsNetworkPolicyName(existing.name, port) ? normalizedNetworkPolicyRuleName(rule.name, port) : existing.name,
      id: existing.id || rule.id,
      enabled: existing.enabled !== false || rule.enabled !== false,
    })
  })
  return Array.from(byPort.values()).sort((a, b) => Number(a.port || 0) - Number(b.port || 0))
}

export function defaultNetworkPolicyRuleName(port?: number) {
  const value = Number(port || 0)
  return value > 0 ? String(value) : ''
}

function normalizedNetworkPolicyRuleName(name: string | undefined, port: number) {
  return shouldUsePortAsNetworkPolicyName(name, port) ? defaultNetworkPolicyRuleName(port) : String(name || '').trim()
}

export function shouldUsePortAsNetworkPolicyName(name: string | undefined, port?: number) {
  const value = String(name || '').trim()
  if (!value) {
    return true
  }
  if (port && value === String(port)) {
    return true
  }
  return ['当前 ufw 白名单', '当前 tc 限速', '系统端口策略'].includes(value)
}

function normalizeNetworkPolicyProtocol(protocol?: string) {
  if (protocol === 'udp') {
    return 'udp'
  }
  if (protocol === 'both' || protocol === 'all' || protocol === 'tcp+udp') {
    return 'both'
  }
  return 'tcp'
}

function mergeNetworkPolicyProtocol(a?: string, b?: string) {
  const protocols = [normalizeNetworkPolicyProtocol(a), normalizeNetworkPolicyProtocol(b)]
  const hasTCP = protocols.includes('tcp') || protocols.includes('both')
  const hasUDP = protocols.includes('udp') || protocols.includes('both')
  if (hasTCP && hasUDP) {
    return 'both'
  }
  return hasUDP ? 'udp' : 'tcp'
}

function mergeNetworkPolicyRate(a?: number, b?: number) {
  const first = Number(a || 0)
  const second = Number(b || 0)
  if (!first) {
    return second
  }
  if (!second) {
    return first
  }
  return Math.min(first, second)
}

export function formatRealmEndpoint(host?: string, port?: number) {
  const cleanHost = String(host || '').trim() || '-'
  const cleanPort = Number(port || 0)
  return cleanPort > 0 ? `${cleanHost}:${cleanPort}` : cleanHost
}

export function bestAgentAddress(agent?: AgentListItem) {
  if (!agent) {
    return ''
  }
  return (
    agent.entry?.import_domain ||
    agent.entry?.addresses?.find(Boolean) ||
    agent.summary?.observed_ip ||
    agent.summary?.public_ipv4 ||
    agent.summary?.public_ipv6 ||
    ''
  )
}
