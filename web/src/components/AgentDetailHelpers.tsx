import { Card, Empty, Space, Table } from 'antd'
import type { ColumnsType } from 'antd/es/table'

import type { GlobalDashboardView, TopologyLinkView, XUIClientView, XUILocalCertificate, XUINodeView } from '../types'
import { nodeElementId } from '../lib/appHelpers'

export type AgentFeatureKey = 'xui' | 'realm' | 'nat' | 'port_policy'

export type RealmForwardNodeView = XUINodeView & {
  realm_target_agent_id?: string
  realm_target_agent_name?: string
}

export type RealmForwardClientView = XUIClientView & {
  realm_target_agent_id?: string
  realm_target_agent_name?: string
}

export function buildRealmForwardNodes(
  links: TopologyLinkView[],
  dashboardView: GlobalDashboardView | null,
  overviewNodes: XUINodeView[] = [],
): RealmForwardNodeView[] {
  const nodes = new Map<string, RealmForwardNodeView>()
  const agentNameByID = new Map((dashboardView?.agents || []).map((agent) => [agent.agent_id, agent.agent_name || agent.customer_display_name || agent.agent_id]))
  overviewNodes
    .filter((node) => node.realm_target_agent_id || node.route?.match_scope === 'realm')
    .forEach((node) => {
      const key = realmForwardNodeIdentityKey(node)
      nodes.set(key, {
        ...node,
        realm_target_agent_name: node.realm_target_agent_name || agentNameByID.get(node.realm_target_agent_id || '') || node.realm_target_agent_id,
      })
    })
  links.forEach((link, index) => {
    const target = link.target
    const targetInboundID = target.inbound_id || target.port || 0
    const sourceListenPort = link.source.listen_port || target.port || targetInboundID || index + 1
    const sourceTag = link.source.outbound_tag || target.inbound_tag || ''
    const key = realmForwardNodeIdentityKey({
      id: sourceListenPort,
      tag: sourceTag,
      realm_target_agent_id: target.agent_id,
      realm_target_inbound_id: targetInboundID,
      realm_target_inbound_tag: target.inbound_tag || '',
    } as XUINodeView)
    const clientCount = countRealmTargetClients(dashboardView, target.agent_id, target.inbound_tag, target.inbound_name, target.port)
    if (!nodes.has(key)) {
      nodes.set(key, {
        id: sourceListenPort,
        tag: sourceTag,
        remark: target.inbound_name || target.inbound_tag || `${target.agent_name || target.agent_id}:${target.port || '-'}`,
        protocol: target.protocol || '',
        listen: target.agent_name || target.agent_id,
        port: sourceListenPort,
        network: target.network || '',
        security: target.security || '',
        ws_path: target.ws_path || '',
        ws_host: target.ws_host || '',
        enabled: true,
        client_count: clientCount,
        online_count: 0,
        realm_target_agent_id: target.agent_id,
        realm_target_agent_name: target.agent_name,
        realm_target_inbound_id: targetInboundID,
        realm_target_inbound_tag: target.inbound_tag || '',
        route: {
          match_scope: 'realm',
          outbound_tag: link.source.outbound_tag || link.source.target || '',
          note: link.match_reason || link.match_explanation || '',
        },
      })
    }
  })
  return Array.from(nodes.values()).sort((a, b) => (a.listen || '').localeCompare(b.listen || '') || (a.port || 0) - (b.port || 0))
}

export function buildRealmForwardClients(links: TopologyLinkView[], dashboardView: GlobalDashboardView | null, overviewClients: XUIClientView[] = []): RealmForwardClientView[] {
  const clients = new Map<string, RealmForwardClientView>()
  const agentNameByID = new Map((dashboardView?.agents || []).map((agent) => [agent.agent_id, agent.agent_name || agent.customer_display_name || agent.agent_id]))
  overviewClients
    .filter((client) => client.is_realm_forwarded)
    .forEach((client) => {
      const targetAgentID = client.realm_target_agent_id || ''
      const key = `${targetAgentID}:${client.realm_target_inbound_id || client.inbound_id}:${client.realm_target_inbound_tag || client.inbound_tag || ''}:${client.email || client.comment || client.import_url || ''}`
      if (clients.has(key)) {
        return
      }
      clients.set(key, {
        ...client,
        realm_target_agent_id: targetAgentID,
        realm_target_agent_name: client.realm_target_agent_name || agentNameByID.get(targetAgentID) || targetAgentID,
        realm_target_inbound_id: client.realm_target_inbound_id,
        realm_target_inbound_tag: client.realm_target_inbound_tag,
      })
    })
  links.forEach((link) => {
    if (!dashboardView) {
      return
    }
    const target = link.target
    dashboardView.client_chains
      .filter((chain) => chainMatchesRealmTarget(chain, target.agent_id, target.inbound_tag, target.inbound_name, target.port))
      .forEach((chain) => {
        const key = `${target.agent_id}:${target.inbound_id}:${target.inbound_tag || ''}:${chain.root_client_email || chain.key}`
        if (clients.has(key)) {
          return
        }
        clients.set(key, {
          inbound_id: target.inbound_id || target.port || 0,
          inbound_tag: target.inbound_tag || '',
          inbound_remark: target.inbound_name || target.inbound_tag || `${target.agent_name || target.agent_id}:${target.port || '-'}`,
          protocol: target.protocol || '',
          email: chain.root_client_email || '',
          comment: chain.root_client_remark || '',
          enabled: true,
          realm_listen_port: link.source.listen_port || 0,
          realm_listen_tag: link.source.outbound_tag || '',
          realm_source_agent_id: link.source.agent_id,
          realm_target_agent_id: target.agent_id,
          realm_target_agent_name: target.agent_name,
          realm_target_inbound_id: target.inbound_id || target.port || 0,
          realm_target_inbound_tag: target.inbound_tag || '',
          route: {
            match_scope: 'realm',
            outbound_tag: link.source.outbound_tag || link.source.target || '',
            note: link.match_reason || link.match_explanation || '',
          },
        })
      })
  })
  return Array.from(clients.values()).sort((a, b) => (a.inbound_remark || '').localeCompare(b.inbound_remark || '') || (a.email || '').localeCompare(b.email || ''))
}

export function filterRealmForwardClients(clients: RealmForwardClientView[], search: string): RealmForwardClientView[] {
  const keyword = search.trim().toLowerCase()
  if (!keyword) {
    return clients
  }
  return clients.filter((client) => [
    client.email,
    client.comment,
    client.inbound_tag,
    client.inbound_remark,
    client.inbound_id,
    client.realm_target_inbound_id,
    client.realm_target_inbound_tag,
    client.realm_target_agent_id,
    client.realm_target_agent_name,
    client.protocol,
  ].some((value) => String(value || '').toLowerCase().includes(keyword)))
}

function countRealmTargetClients(dashboardView: GlobalDashboardView | null, agentID: string, inboundTag?: string, inboundName?: string, port?: number): number {
  if (!dashboardView) {
    return 0
  }
  return dashboardView.client_chains.filter((chain) => chainMatchesRealmTarget(chain, agentID, inboundTag, inboundName, port)).length
}

function chainMatchesRealmTarget(chain: GlobalDashboardView['client_chains'][number], agentID: string, inboundTag?: string, inboundName?: string, port?: number): boolean {
  if (chain.root_agent_id !== agentID) {
    return false
  }
  if (inboundTag && chain.root_inbound_tag === inboundTag) {
    return true
  }
  return chain.steps.some((step) => step.step_type === 'inbound' && step.agent_id === agentID && (
    (inboundName && step.label === inboundName) ||
    (inboundTag && step.label === inboundTag) ||
    (port && step.port === port)
  ))
}

export function realmForwardNodeKey(node: Pick<XUINodeView, 'id' | 'tag' | 'port' | 'realm_target_agent_id' | 'realm_target_inbound_id' | 'realm_target_inbound_tag'>): string {
  return [
    node.id || node.port || 0,
    node.tag || '',
    node.realm_target_agent_id || '',
    node.realm_target_inbound_id || 0,
    node.realm_target_inbound_tag || '',
  ].join(':')
}

function realmForwardNodeIdentityKey(node: Pick<XUINodeView, 'id' | 'port' | 'realm_target_agent_id' | 'realm_target_inbound_id' | 'realm_target_inbound_tag'>): string {
  return [
    node.id || node.port || 0,
    node.realm_target_agent_id || '',
    node.realm_target_inbound_id || 0,
    node.realm_target_inbound_tag || '',
  ].join(':')
}

export function renderNodeClientHierarchySections(
  nodes: XUINodeView[],
  clients: XUIClientView[],
  nodeColumns: ColumnsType<XUINodeView>,
  _agentLabel: string,
  selectedAgentID: string,
  selectedNodeAnchor: string,
  scrollX: number,
  _clientTabKey: string,
  _onActiveTabChange: (key: string) => void,
  _onClientSearchChange: (value: string) => void,
) {
  if (!nodes.length) {
    return <Empty description="暂无节点数据" />
  }
  return (
    <Space direction="vertical" style={{ width: '100%' }} size="middle">
      {nodes.map((node) => {
        const group = clients.filter((client) => nodeMatchesClient(node, client))
        const nodeLabel = node.remark || node.tag || `Inbound #${node.id || 0}`
        const anchor = nodeElementId(selectedAgentID, nodeLabel)
        const displayNode: XUINodeView = {
          ...node,
          client_count: Math.max(Number(node.client_count || 0), group.length),
        }
        return (
          <Card
            key={realmForwardNodeKey(node)}
            id={anchor}
            size="small"
            className={selectedNodeAnchor === anchor ? 'node-row-selected' : undefined}
          >
            <Table
              size="small"
              rowKey={(record) => realmForwardNodeKey(record)}
              columns={nodeColumns}
              dataSource={[displayNode]}
              pagination={false}
              scroll={{ x: scrollX }}
              rowClassName={() => (selectedNodeAnchor === anchor ? 'node-row-selected' : '')}
            />
          </Card>
        )
      })}
    </Space>
  )
}

export function nodeClientSearchValue(node: XUINodeView): string {
  return String(node.tag || node.remark || node.realm_target_inbound_tag || node.realm_target_inbound_id || node.id || '').trim()
}

function nodeMatchesClient(node: XUINodeView, client: XUIClientView): boolean {
  if (sameInboundIdentity(node.id, node.tag, client.inbound_id, client.inbound_tag)) {
    return true
  }
  const nodeTargetAgentID = node.realm_target_agent_id || ''
  const clientTargetAgentID = client.realm_target_agent_id || ''
  if (nodeTargetAgentID && clientTargetAgentID && nodeTargetAgentID !== clientTargetAgentID) {
    return false
  }
  return sameInboundIdentity(
    node.realm_target_inbound_id || 0,
    node.realm_target_inbound_tag || '',
    client.realm_target_inbound_id || client.inbound_id,
    client.realm_target_inbound_tag || client.inbound_tag,
  )
}

function sameInboundIdentity(expectedID?: number, expectedTag?: string, actualID?: number, actualTag?: string): boolean {
  if (expectedID && actualID && expectedID !== actualID) {
    return false
  }
  if (expectedTag && actualTag && expectedTag !== actualTag) {
    return false
  }
  return Boolean((expectedID && actualID && expectedID === actualID) || (expectedTag && actualTag && expectedTag === actualTag))
}

export function buildPrimaryDomainOptions(certificates: XUILocalCertificate[]) {
  const seen = new Set<string>()
  const options: { value: string; label: string }[] = []
  certificates.forEach((certificate) => {
    certificateDomainCandidates(certificate).forEach((domain) => {
      if (seen.has(domain)) {
        return
      }
      seen.add(domain)
      options.push({
        value: domain,
        label: certificate.name ? `${domain} · ${certificate.name}` : domain,
      })
    })
  })
  return options
}

export function firstCertificateDomainCandidate(certificate: XUILocalCertificate) {
  const candidates = certificateDomainCandidates(certificate)
  return candidates.find((domain) => !domain.startsWith('*.')) || candidates[0] || ''
}

function certificateDomainCandidates(certificate: XUILocalCertificate) {
  const values = [...(certificate.dns_names || []), certificate.subject || '']
  const seen = new Set<string>()
  const result: string[] = []
  values.forEach((value) => {
    const domain = normalizePrimaryDomain(value)
    if (!domain || seen.has(domain)) {
      return
    }
    seen.add(domain)
    result.push(domain)
  })
  return result
}

export function normalizePrimaryDomain(value?: string) {
  let domain = (value || '').trim().toLowerCase().replace(/\.$/, '')
  domain = domain.replace(/^https?:\/\//, '').split('/')[0]
  const portMatch = domain.match(/^([^:[\]]+):\d+$/)
  if (portMatch) {
    domain = portMatch[1]
  }
  if (!domain || domain.includes(' ') || domain.includes('*') || isIPLikeDomain(domain)) {
    return ''
  }
  return domain
}

function isIPLikeDomain(value: string) {
  return /^\d{1,3}(\.\d{1,3}){3}$/.test(value) || (value.includes(':') && /^[0-9a-f:]+$/i.test(value))
}
