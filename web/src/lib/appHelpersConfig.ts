import type { AgentEntryConfig, AgentEntryMapping, HAProxyConfig, ManagedAgentConfig, RealmForwardConfig, VPSRenewalConfig, XUIOverview } from '../types'
import type { ConfigSectionKey } from './appHelperTypes'
import { DEFAULT_COST_CURRENCY } from './currency'
import { normalizeRenewalConfig } from './appHelpersBilling'
import { parseAddressInput, parseTagInput } from './appHelpersTags'

export function normalizeEntryConfig(config?: AgentEntryConfig): AgentEntryConfig {
  return {
    addresses: parseAddressInput((config?.addresses || []).join('\n')),
    import_domain: normalizeImportDomain(config?.import_domain),
    mappings: (config?.mappings || []).map((mapping) => ({
      address: mapping.address || '',
      external_port: Math.max(0, Number(mapping.external_port || 0)),
      internal_port: Math.max(0, Number(mapping.internal_port || 0)),
      protocol: normalizeEntryProtocol(mapping.protocol),
      note: mapping.note || '',
    })),
    network_policy: normalizeNetworkPolicyConfig(config?.network_policy),
    port_forwarding: normalizeRealmForwardConfig(config?.port_forwarding),
    haproxy: normalizeHAProxyConfig(config?.haproxy),
  }
}

export function normalizeImportDomain(value?: string): string {
  let domain = (value || '').trim().toLowerCase()
  domain = domain.replace(/^https?:\/\//, '').split('/')[0].replace(/\.$/, '')
  const portMatch = domain.match(/^([^:[\]]+):\d+$/)
  if (portMatch) {
    domain = portMatch[1]
  }
  if (!domain || domain.includes(' ') || domain.includes('*') || domain.includes(':')) {
    return ''
  }
  return domain
}

export function normalizeEntryProtocol(protocol?: string): AgentEntryMapping['protocol'] {
  switch ((protocol || '').toLowerCase()) {
    case 'vless':
      return 'vless'
    case 'vmess':
      return 'vmess'
    case 'http':
      return 'http'
    case 'socks':
    case 'socks5':
      return 'socks'
    default:
      return 'vless'
  }
}

export function buildSectionSavePayload(base: ManagedAgentConfig, draft: ManagedAgentConfig, section: ConfigSectionKey, agentID: string): ManagedAgentConfig {
  const payload: ManagedAgentConfig = {
    ...base,
    agent_id: agentID,
    agent_name: base.agent_name || draft.agent_name || agentID,
    customer_display_name: base.customer_display_name || '',
    sort_order: base.sort_order || draft.sort_order || 0,
    tags: [...(base.tags || [])],
    features: { ...(base.features || {}) },
    renewal: { ...(base.renewal || {}) },
    entry: {
      addresses: [...(base.entry?.addresses || [])],
      import_domain: base.entry?.import_domain || '',
      mappings: (base.entry?.mappings || []).map((mapping) => ({ ...mapping })),
      network_policy: normalizeNetworkPolicyConfig(base.entry?.network_policy),
      port_forwarding: normalizeRealmForwardConfig(base.entry?.port_forwarding),
      haproxy: normalizeHAProxyConfig(base.entry?.haproxy),
    },
    xui: { ...base.xui },
  }
  switch (section) {
    case 'client':
      payload.agent_name = draft.agent_name || agentID
      payload.customer_display_name = draft.customer_display_name || ''
      payload.sort_order = Number(draft.sort_order || base.sort_order || 0)
      payload.tags = [...(draft.tags || [])]
      payload.features = { ...(draft.features || {}) }
      break
    case 'renewal':
      payload.renewal = { ...(draft.renewal || {}) }
      break
    case 'xui':
      payload.xui = { ...draft.xui }
      break
    case 'entry':
      payload.entry = {
        addresses: [...(draft.entry?.addresses || [])],
        import_domain: draft.entry?.import_domain || '',
        mappings: (draft.entry?.mappings || []).map((mapping) => ({ ...mapping })),
        network_policy: normalizeNetworkPolicyConfig(draft.entry?.network_policy),
        port_forwarding: normalizeRealmForwardConfig(draft.entry?.port_forwarding),
        haproxy: normalizeHAProxyConfig(draft.entry?.haproxy),
      }
      break
  }
  return payload
}

export function mergeSavedSectionIntoDraft(draft: ManagedAgentConfig, saved: ManagedAgentConfig, section: ConfigSectionKey): ManagedAgentConfig {
  const next: ManagedAgentConfig = {
    ...draft,
    agent_id: saved.agent_id || draft.agent_id,
  }
  switch (section) {
    case 'client':
      next.agent_name = saved.agent_name
      next.customer_display_name = saved.customer_display_name || ''
      next.sort_order = saved.sort_order
      next.tags = [...(saved.tags || [])]
      next.features = { ...(saved.features || {}) }
      break
    case 'renewal':
      next.renewal = { ...(saved.renewal || {}) }
      break
    case 'xui':
      next.xui = { ...saved.xui }
      break
    case 'entry':
      next.entry = {
        addresses: [...(saved.entry?.addresses || [])],
        import_domain: saved.entry?.import_domain || '',
        mappings: (saved.entry?.mappings || []).map((mapping) => ({ ...mapping })),
        network_policy: normalizeNetworkPolicyConfig(saved.entry?.network_policy),
        port_forwarding: normalizeRealmForwardConfig(saved.entry?.port_forwarding),
        haproxy: normalizeHAProxyConfig(saved.entry?.haproxy),
      }
      break
  }
  return next
}

export function configSectionLabel(section: ConfigSectionKey): string {
  switch (section) {
    case 'client':
      return 'Client 信息'
    case 'renewal':
      return 'VPS 信息'
    case 'xui':
      return 'X-UI 配置'
    case 'entry':
      return '入口/NAT 配置'
  }
}

export function configSignature(config: ManagedAgentConfig): string {
  return JSON.stringify(config)
}

export function createEmptyManagedConfig(agentID: string, agentName?: string): ManagedAgentConfig {
  return {
    agent_id: agentID,
    agent_name: agentName || agentID,
    customer_display_name: '',
    sort_order: 0,
    tags: [],
    features: {
      xui: false,
      realm: false,
      haproxy: false,
      nat: false,
      port_policy: false,
    },
    renewal: {
      enabled: false,
      start_date: '',
      expire_date: '',
      cycle: 'month',
      auto_renew: false,
      cost_amount: 0,
      cost_currency: DEFAULT_COST_CURRENCY,
      cost_cycle: 'month',
      client_billings: [],
      traffic_limit_bytes: 0,
      traffic_accounting_mode: 'bidirectional',
      bandwidth_mbps: 0,
      traffic_baseline_bytes: 0,
      traffic_sent_baseline_bytes: 0,
      traffic_recv_baseline_bytes: 0,
      traffic_baseline_period_start: '',
    },
    entry: {
      addresses: [],
      import_domain: '',
      mappings: [],
      network_policy: {
        enabled: false,
        interface: '',
        firewall_backend: 'auto',
        rate_limit_backend: 'auto',
        rules: [],
      },
      port_forwarding: {
        enabled: false,
        backend: 'realm',
        binary_path: '',
        config_path: '',
        service_name: '',
        log_level: 'info',
        rules: [],
      },
      haproxy: {
        enabled: false,
        binary_path: '',
        config_path: '',
        service_name: '',
        rules: [],
      },
    },
    xui: {
      enabled: false,
      base_url: '',
      db_path: '',
      username: '',
      password: '',
      api_token: '',
      two_factor_code: '',
      skip_tls_verify: false,
      access_log_enabled: false,
      access_log_path: '',
      access_log_retention_days: 7,
    },
  }
}

export function normalizeManagedConfig(config: ManagedAgentConfig, agentID: string, agentName?: string): ManagedAgentConfig {
  const base = createEmptyManagedConfig(agentID, agentName)
  return {
    agent_id: config.agent_id || base.agent_id,
    agent_name: config.agent_name || agentName || base.agent_name,
    customer_display_name: config.customer_display_name || '',
    sort_order: Number(config.sort_order || base.sort_order || 0),
    tags: parseTagInput((config.tags || []).join(',')),
    features: normalizeAgentFeatures(config.features, config),
    renewal: normalizeRenewalConfig(config.renewal || base.renewal),
    entry: normalizeEntryConfig(config.entry || base.entry),
    xui: {
      ...base.xui,
      ...config.xui,
      enabled: Boolean(config.xui?.enabled),
      skip_tls_verify: Boolean(config.xui?.skip_tls_verify),
      access_log_enabled: Boolean(config.xui?.access_log_enabled),
      access_log_retention_days: Math.min(30, Math.max(1, Number(config.xui?.access_log_retention_days || 7))),
    },
  }
}

export function normalizeXUIOverview(overview: XUIOverview): XUIOverview {
  return {
    ...overview,
    nodes: Array.isArray(overview.nodes) ? overview.nodes : [],
    clients: Array.isArray(overview.clients) ? overview.clients : [],
    outbounds: Array.isArray(overview.outbounds) ? overview.outbounds : [],
    routing_rules: Array.isArray(overview.routing_rules) ? overview.routing_rules : [],
    certificates: Array.isArray(overview.certificates) ? overview.certificates : [],
  }
}

function normalizeRealmForwardConfig(config?: RealmForwardConfig): RealmForwardConfig {
  return {
    enabled: Boolean(config?.enabled),
    backend: config?.backend === 'none' ? 'none' : 'realm',
    binary_path: (config?.binary_path || '').trim(),
    config_path: (config?.config_path || '').trim(),
    service_name: (config?.service_name || '').trim(),
    log_level: ['trace', 'debug', 'warn', 'error'].includes(String(config?.log_level || '')) ? config?.log_level : 'info',
    rules: (config?.rules || []).map((rule, index) => ({
      id: rule.id || `realm-${rule.listen_port || 0}-${rule.target_port || 0}-${index}`,
      name: rule.name || '',
      enabled: rule.enabled !== false,
      listen_address: '0.0.0.0',
      listen_port: Math.max(0, Number(rule.listen_port || 0)),
      target_agent_id: rule.target_agent_id || '',
      target_address: rule.target_address || '',
      target_port: Math.max(0, Number(rule.target_port || 0)),
      network: 'both',
      note: rule.note || '',
    })),
  }
}

function normalizeHAProxyConfig(config?: HAProxyConfig): HAProxyConfig {
  return {
    enabled: Boolean(config?.enabled),
    binary_path: (config?.binary_path || '').trim(),
    config_path: (config?.config_path || '').trim(),
    service_name: (config?.service_name || '').trim(),
    rules: (config?.rules || []).map((rule, index) => ({
      id: rule.id || `haproxy-${rule.listen_port || 0}-${index}`,
      name: rule.name || '',
      enabled: rule.enabled !== false,
      listen_address: (rule.listen_address || '').trim() || '0.0.0.0',
      listen_port: Math.max(0, Number(rule.listen_port || 0)),
      primary: normalizeHAProxyTarget(rule.primary),
      backups: (rule.backups || []).map(normalizeHAProxyTarget),
      check_interval_seconds: Math.min(300, Math.max(1, Number(rule.check_interval_seconds || 3))),
      connect_timeout_seconds: Math.min(60, Math.max(1, Number(rule.connect_timeout_seconds || 5))),
      fall: Math.min(20, Math.max(1, Number(rule.fall || 3))),
      rise: Math.min(20, Math.max(1, Number(rule.rise || 2))),
    })),
  }
}

function normalizeHAProxyTarget(target: NonNullable<HAProxyConfig['rules']>[number]['primary']) {
  return {
    agent_id: target?.agent_id || '',
    realm_rule_id: target?.realm_rule_id || '',
    address: target?.address || '',
    port: Math.max(0, Number(target?.port || 0)),
  }
}

function normalizeNetworkPolicyConfig(config?: AgentEntryConfig['network_policy']): NonNullable<AgentEntryConfig['network_policy']> {
  return {
    enabled: Boolean(config?.enabled),
    interface: (config?.interface || '').trim(),
    firewall_backend: ['ufw', 'iptables', 'none'].includes(String(config?.firewall_backend || '')) ? config?.firewall_backend : 'auto',
    rate_limit_backend: ['tc', 'none'].includes(String(config?.rate_limit_backend || '')) ? config?.rate_limit_backend : 'auto',
    rules: (config?.rules || []).map((rule, index) => ({
      id: rule.id || `${rule.protocol || 'tcp'}-${rule.port || 0}-${index}`,
      name: rule.name || '',
      enabled: rule.enabled !== false,
      port: Math.max(0, Number(rule.port || 0)),
      protocol: rule.protocol === 'udp' || rule.protocol === 'both' ? rule.protocol : 'tcp',
      rate_limit_mbps: Math.max(0, Number(rule.rate_limit_mbps || 0)),
      whitelist_ips: parseAddressInput((rule.whitelist_ips || []).join('\n')),
    })),
  }
}

function normalizeAgentFeatures(features: ManagedAgentConfig['features'], config?: ManagedAgentConfig): NonNullable<ManagedAgentConfig['features']> {
  const entry = config?.entry
  const xui = config?.xui
  return {
    xui: features?.xui ?? Boolean(xui?.enabled || xui?.base_url || xui?.db_path || xui?.api_token),
    realm: features?.realm ?? Boolean(entry?.port_forwarding?.enabled || (entry?.port_forwarding?.rules || []).length),
    haproxy: features?.haproxy ?? Boolean(entry?.haproxy?.enabled || (entry?.haproxy?.rules || []).length),
    nat: features?.nat ?? Boolean((entry?.mappings || []).length || (entry?.addresses || []).length || entry?.import_domain),
    port_policy: features?.port_policy ?? Boolean(entry?.network_policy?.enabled || (entry?.network_policy?.rules || []).length),
  }
}
