import { useEffect, useState } from 'react'
import { Alert, Button, Card, Col, Input, InputNumber, Modal, Popconfirm, Row, Select, Space, Table, Tabs, Tag, Typography, message } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { DeleteOutlined, PlusOutlined, ReloadOutlined, SettingOutlined } from '@ant-design/icons'

import type { AgentListItem, ManagedAgentConfig, SingBoxClientMetadata, SingBoxConfig, SingBoxOverview, XUIAction, XUIOverview } from '../types'
import { fetchJSON } from '../lib/api'
import { actionStatusColor, actionStatusLabel, formatExpiryTime, formatRelativeTime, shortJSON } from '../lib/appHelpers'
import { formatBytes } from '../lib/traffic'
import { defaultOutboundActionForm, type XUIOutboundActionForm } from '../lib/xuiActionForms'
import { buildOutboundImportPatch } from './xuiActionFormShared'

const { Text, Title } = Typography
type EditorKind = 'inbound' | 'outbound' | 'user' | 'rule' | null

export function SingBoxActionPanel(props: {
  agentID: string
  agents: AgentListItem[]
  enabled: boolean
  canManage?: boolean
  managedConfig: ManagedAgentConfig | null
  configSaving?: boolean
  onSingBoxChange: (patch: Partial<SingBoxConfig>) => void
  onSaveConfig: () => void
  actions: XUIAction[]
  actionsLoading?: boolean
  onRefreshActions: () => void
}) {
  const { agentID, agents, enabled, canManage = true, managedConfig, configSaving = false, onSingBoxChange, onSaveConfig, actions, actionsLoading, onRefreshActions } = props
  const [overview, setOverview] = useState<SingBoxOverview | null>(null)
  const [sourceOverview, setSourceOverview] = useState<SingBoxOverview | null>(null)
  const [sourceXUIOverview, setSourceXUIOverview] = useState<XUIOverview | null>(null)
  const [sourceLoading, setSourceLoading] = useState(false)
  const [sourceError, setSourceError] = useState('')
  const [error, setError] = useState('')
  const [editor, setEditor] = useState<EditorKind>(null)
  const [configOpen, setConfigOpen] = useState(false)
  const [saving, setSaving] = useState(false)
  const [inbound, setInbound] = useState({ tag: '', type: 'snell', listen: '::', listen_port: 1080, version: 5, psk: '', obfs_mode: 'none', mode: 'default', raw: '' })
  const [outbound, setOutbound] = useState({ tag: '', type: 'snell', server: '', server_port: 443, version: 4, psk: '', userkey: '', reuse: false, network: '', obfs_mode: 'none', obfs_host: 'bing.com', mode: 'default', raw: '' })
  const [outboundSourceAgentID, setOutboundSourceAgentID] = useState(agentID)
  const [outboundSourceTag, setOutboundSourceTag] = useState('')
  const [outboundSourceXUIInboundKey, setOutboundSourceXUIInboundKey] = useState('')
  const [outboundSourceXUIClientKey, setOutboundSourceXUIClientKey] = useState('')
  const [user, setUser] = useState({ inbound_tag: '', name: '', userkey: '' })
  const [ruleSourceSelection, setRuleSourceSelection] = useState('')
  const [ruleXUIInboundKey, setRuleXUIInboundKey] = useState('')
  const [ruleXUIClientKey, setRuleXUIClientKey] = useState('')
  const [metadataUser, setMetadataUser] = useState<Record<string, unknown> | null>(null)
  const [metadataDraft, setMetadataDraft] = useState({ remark: '', totalGB: 0, trafficMultiplier: 1, expiryTime: 0, lastActive: 0 })

  const openInboundEditor = () => {
    setInbound({ ...inbound, type: 'snell', psk: randomPSK() })
    setEditor('inbound')
  }

  const openOutboundEditor = () => {
    setOutbound({ ...outbound, type: 'snell', psk: randomPSK() })
    setOutboundSourceAgentID(agentID)
    setOutboundSourceTag('')
    setOutboundSourceXUIInboundKey('')
    setOutboundSourceXUIClientKey('')
    setEditor('outbound')
  }

  const selectOutboundSource = (value?: string) => {
    const selected = value || ''
    setOutboundSourceTag(selected)
    if (!selected) return
    const separator = selected.indexOf(':')
    const sourceKind = separator >= 0 ? selected.slice(0, separator) : ''
    const sourceTag = separator >= 0 ? selected.slice(separator + 1) : ''
    if (sourceKind === 'singbox') {
      const node = (outboundSourceAgentID === agentID ? overview?.nodes || [] : sourceOverview?.nodes || []).find((item) => String(item.tag || '') === sourceTag)
      if (!node) return
      setOutbound((current) => ({
        ...current,
        tag: String(node.tag || current.tag),
        type: String(node.type || current.type),
        server: String(node.server || current.server),
        server_port: Number(node.server_port || current.server_port),
        version: Number(node.version || current.version),
        psk: String(node.psk || current.psk),
        userkey: String(node.userkey || current.userkey),
        raw: '',
      }))
      return
    }
    if (sourceKind === 'xui-node') {
      const [nodeIDText, ...tagParts] = sourceTag.split(':')
      const nodeID = Number(nodeIDText || 0)
      const nodeTag = tagParts.join(':')
      const node = sourceXUIInboundNodes(sourceXUIOverview).find((item) => item.id === nodeID && String(item.tag || '') === nodeTag)
      if (!node) return
      setOutboundSourceXUIInboundKey(`${node.id}::${node.tag || ''}`)
      setOutboundSourceXUIClientKey('')
      const targetHost = String(node.export_endpoint?.address || '').trim()
      setOutbound((current) => ({
        ...current,
        tag: String(node.tag || current.tag),
        type: normalizeSingBoxType(node.protocol),
        server: targetHost || current.server,
        server_port: Number(node.export_endpoint?.port || node.port || current.server_port),
        raw: '',
      }))
      message.info('已填充 x-ui 入站的协议、地址和端口；如该入站有客户端，请继续选择客户端以填充凭据')
    }
  }

  const selectXUIClientSource = (value?: string) => {
    const selected = value || ''
    setOutboundSourceXUIClientKey(selected)
    const source = sourceXUIOverview
    const client = source?.clients.find((item) => xuiClientSourceKey(item) === selected)
    if (!client) return
    const [inboundIDText, inboundTag] = outboundSourceXUIInboundKey.split('::')
    const node = sourceXUIInboundNodes(source).find((item) => item.id === Number(inboundIDText) && String(item.tag || '') === inboundTag)
    if (!node) return
    try {
      const imported = buildOutboundImportPatch(source!, node, client, defaultOutboundActionForm())
      const credentials: Record<string, unknown> = {}
      if (imported.uuid) credentials.uuid = imported.uuid
      if (imported.password) credentials.password = imported.password
      if (imported.method) credentials.method = imported.method
      if (imported.flow) credentials.flow = imported.flow
      setOutbound((current) => ({
        ...current,
        type: imported.protocol || normalizeSingBoxType(client.protocol || outbound.type),
        tag: imported.tag || current.tag,
        server: imported.address || current.server,
        server_port: imported.port || current.server_port,
        raw: JSON.stringify(credentials, null, 2),
      }))
    } catch (error) {
      message.warning(error instanceof Error ? error.message : '来源客户端节点暂时无法导入')
    }
  }

  const selectRuleOutboundSource = (value?: string) => {
    const selected = value || ''
    setRuleSourceSelection(selected)
    if (!selected) return
    const separator = selected.indexOf(':')
    const sourceKind = separator >= 0 ? selected.slice(0, separator) : ''
    const sourceTag = separator >= 0 ? selected.slice(separator + 1) : ''
    if (sourceKind === 'singbox') {
      setRuleXUIInboundKey('')
      setRuleXUIClientKey('')
      setRule((current) => ({ ...current, outbound: sourceTag }))
      return
    }
    if (sourceKind === 'xui-node') {
      const [nodeIDText, ...tagParts] = sourceTag.split(':')
      const nodeID = Number(nodeIDText || 0)
      const nodeTag = tagParts.join(':')
      const node = sourceXUIInboundNodes(sourceXUIOverview).find((item) => item.id === nodeID && String(item.tag || '') === nodeTag)
      if (!node) return
      const inboundKey = `${node.id}::${node.tag || ''}`
      const generatedTag = `${sourceXUIOverview?.agent_name || rule.source_agent_id}-${node.tag || node.id}`.replace(/[^a-zA-Z0-9._-]+/g, '-')
      setRuleXUIInboundKey(inboundKey)
      setRuleXUIClientKey('')
      setRule((current) => ({ ...current, outbound: generatedTag }))
    }
  }

  const selectRuleXUIClientSource = (value?: string) => {
    setRuleXUIClientKey(value || '')
  }
  const [rule, setRule] = useState({ source_agent_id: agentID, domain: '', ip: '', port: '', inbound: '', outbound: '' })

  const load = async () => {
    try {
      const data = await fetchJSON<SingBoxOverview>(`/api/v1/agents/${agentID}/singbox/overview`)
      setOverview(data)
      setError(data.error || '')
      if (rule.source_agent_id === agentID) setSourceOverview(data)
    } catch (loadError) {
      setError(loadError instanceof Error ? loadError.message : '加载 sing-box 配置失败')
    }
  }
  useEffect(() => { void load() }, [agentID])

  useEffect(() => {
    const sourceAgentID = editor === 'rule' ? rule.source_agent_id : editor === 'outbound' ? outboundSourceAgentID : ''
    if (!sourceAgentID) return
    let cancelled = false
    setSourceLoading(true)
    setSourceOverview(null)
    setSourceXUIOverview(null)
    setSourceError('')
    const singboxRequest = sourceAgentID === agentID && overview
      ? Promise.resolve(overview)
      : fetchJSON<SingBoxOverview>(`/api/v1/agents/${sourceAgentID}/singbox/overview`)
    const xuiRequest = fetchJSON<XUIOverview>(`/api/v1/agents/${sourceAgentID}/xui/overview`)
    void Promise.allSettled([singboxRequest, xuiRequest]).then(([singboxResult, xuiResult]) => {
      if (cancelled) return
      const singboxOK = singboxResult.status === 'fulfilled'
      const xuiOK = xuiResult.status === 'fulfilled'
      setSourceOverview(singboxOK ? singboxResult.value : null)
      setSourceXUIOverview(xuiOK ? xuiResult.value : null)
      if (!singboxOK && !xuiOK) {
        const reason = singboxResult.status === 'rejected' ? singboxResult.reason : xuiResult.status === 'rejected' ? xuiResult.reason : undefined
        setSourceError(reason instanceof Error ? reason.message : '来源 Client 的 x-ui / sing-box 配置读取失败')
      } else {
        setSourceError('')
      }
    }).finally(() => { if (!cancelled) setSourceLoading(false) })
    return () => { cancelled = true }
  }, [agentID, editor, overview, rule.source_agent_id, outboundSourceAgentID])

  async function submit() {
    let kind = ''
    let payload: Record<string, unknown> = {}
    if (editor === 'inbound') {
      if (!inbound.tag.trim()) { message.error('入站节点需要填写标签'); return }
      if (inbound.listen_port < 1 || inbound.listen_port > 65535) { message.error('监听端口必须在 1 到 65535 之间'); return }
      const client = parseJSONDraft(inbound.raw, '入站节点')
      if (!client) return
      Object.assign(client, { type: inbound.type.trim(), tag: inbound.tag.trim(), listen: inbound.listen.trim() || '::', listen_port: inbound.listen_port })
      if (inbound.type.trim().toLowerCase() === 'snell') {
        Object.assign(client, { version: inbound.version, psk: inbound.psk })
        if (inbound.version === 6) { client.mode = inbound.mode; delete client.obfs_mode } else { client.obfs_mode = inbound.obfs_mode; delete client.mode }
      }
      kind = 'singbox_add_client'
      payload = { client }
    } else if (editor === 'outbound') {
      if (!outbound.tag.trim()) { message.error('出站节点需要填写标签'); return }
      const node = parseJSONDraft(outbound.raw, '出站节点')
      if (!node) return
      Object.assign(node, { type: outbound.type.trim(), tag: outbound.tag.trim() })
      if (outbound.server.trim()) node.server = outbound.server.trim()
      if (outbound.server_port) node.server_port = outbound.server_port
      if (outbound.type.trim().toLowerCase() === 'snell') {
        Object.assign(node, { version: outbound.version, psk: outbound.psk, userkey: outbound.userkey, reuse: outbound.reuse })
        if (outbound.network) node.network = outbound.network
        if (outbound.version === 6) { node.mode = outbound.mode; delete node.obfs_mode; delete node.obfs_host } else { node.obfs_mode = outbound.obfs_mode; node.obfs_host = outbound.obfs_host; delete node.mode }
      }
      kind = 'singbox_add_node'
      payload = { node }
      const [outboundSourceKind, outboundSourceNodeTag] = outboundSourceTag.split(':')
      if (outboundSourceKind === 'singbox' && outboundSourceNodeTag) {
        payload.source_agent_id = outboundSourceAgentID
        payload.outbound_tag = outboundSourceNodeTag
        payload.target_tag = outbound.tag.trim()
      }
      if (outboundSourceKind === 'xui-node') {
        payload.source_agent_id = outboundSourceAgentID
        payload.source_inbound_key = outboundSourceXUIInboundKey
        payload.source_client_key = outboundSourceXUIClientKey
        payload.target_tag = outbound.tag.trim()
      }
    } else if (editor === 'user') {
      if (!user.inbound_tag.trim()) { message.error('请选择入站节点'); return }
      if (!user.userkey.trim()) { message.error('Snell 客户端必须填写 userkey'); return }
      const nextUser: Record<string, unknown> = { userkey: user.userkey.trim() }
      if (user.name.trim()) nextUser.name = user.name.trim()
      if (user.userkey.trim()) nextUser.userkey = user.userkey.trim()
      kind = 'singbox_add_user'
      payload = { inbound_tag: user.inbound_tag.trim(), user: nextUser }
    } else if (editor === 'rule') {
      const source = rule.source_agent_id === agentID ? overview : sourceOverview
      const sourceOutbound = source?.nodes.find((item) => item.tag === rule.outbound)
      if (!ruleSourceSelection.startsWith('xui-node:') && !sourceOutbound) { message.error('请选择来源 Client 中已有的出站节点'); return }
      const next: Record<string, unknown> = { outbound: rule.outbound.trim() }
      if (rule.domain.trim()) next.domain = splitList(rule.domain)
      if (rule.ip.trim()) next.ip_cidr = splitList(rule.ip)
      if (rule.port.trim()) next.port = rule.port.trim()
      if (rule.inbound.trim()) next.inbound = splitList(rule.inbound)
      kind = 'singbox_add_forwarding_rule'
      payload = { rule: next }
      if (ruleSourceSelection.startsWith('xui-node:')) {
        const separator = ruleSourceSelection.indexOf(':')
        const sourceTag = separator >= 0 ? ruleSourceSelection.slice(separator + 1) : ''
        const [nodeIDText, ...tagParts] = sourceTag.split(':')
        const nodeID = Number(nodeIDText || 0)
        const nodeTag = tagParts.join(':')
          const sourceNode = sourceXUIInboundNodes(sourceXUIOverview).find((item) => item.id === nodeID && String(item.tag || '') === nodeTag)
        if (!sourceNode) { message.error('来源 Client 入站节点不存在'); return }
        const sourceClient = sourceXUIOverview?.clients.find((item) => xuiClientSourceKey(item) === ruleXUIClientKey) || {
          inbound_id: sourceNode.id,
          inbound_tag: sourceNode.tag || '',
          protocol: sourceNode.protocol || '',
          enabled: true,
          route: { match_scope: '' },
        }
        try {
          const imported = buildOutboundImportPatch(sourceXUIOverview!, sourceNode, sourceClient, defaultOutboundActionForm())
          payload.outbound = buildSingBoxOutboundFromImport(imported, rule.outbound)
          payload.source_agent_id = rule.source_agent_id
          payload.source_inbound_key = ruleXUIInboundKey
          payload.source_client_key = ruleXUIClientKey
        } catch (error) {
          message.error(error instanceof Error ? error.message : '来源 Client 入站无法转换为 sing-box 出站')
          return
        }
      } else if (rule.source_agent_id !== agentID) {
        payload.source_agent_id = rule.source_agent_id
        payload.outbound_tag = rule.outbound.trim()
        payload.outbound = sourceOutbound
      }
    } else return
    setSaving(true)
    try {
      await fetchJSON(`/api/v1/agents/${agentID}/singbox/actions`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ kind, payload }) })
      message.success('sing-box 操作已下发')
      setEditor(null)
      onRefreshActions()
      ;[1000, 3000, 8000].forEach((delay) => window.setTimeout(() => { onRefreshActions(); void load() }, delay))
    } catch (submitError) {
      message.error(submitError instanceof Error ? submitError.message : 'sing-box 操作下发失败')
    } finally { setSaving(false) }
  }

  const singBoxConfig = managedConfig?.singbox || { enabled: false, config_path: '', binary_path: '', service_name: '' }
  const sourceOptions = agents.filter((agent) => agent.agent_id).map((agent) => ({ value: agent.agent_id, label: agent.agent_id === agentID ? `${agent.agent_name || agent.agent_id}（当前）` : (agent.agent_name || agent.agent_id) }))
  const singBoxOutboundOptions = (rule.source_agent_id === agentID ? overview?.nodes || [] : sourceOverview?.nodes || []).map((item) => ({ value: String(item.tag || ''), label: `[sing-box] ${String(item.tag || '')} · ${String(item.type || '')}` })).filter((item) => item.value)
  const xuiInboundOptions = sourceXUIInboundNodes(sourceXUIOverview).map((item) => ({ value: `xui-node:${item.id}:${String(item.tag || '')}`, label: `[x-ui 入站] ${String(item.remark || item.tag || item.id)} · ${String(item.protocol || '')}` })).filter((item) => item.value !== 'xui-node:0:')
  const outboundSourceOptions = [
    ...(outboundSourceAgentID === agentID ? overview?.nodes || [] : sourceOverview?.nodes || []).map((item) => ({ value: `singbox:${String(item.tag || '')}`, label: `[sing-box] ${String(item.tag || '')} · ${String(item.type || '')}` })).filter((item) => item.value !== 'singbox:'),
    ...xuiInboundOptions,
  ]
  const xuiSourceClients = outboundSourceXUIInboundKey
    ? (sourceXUIOverview?.clients || []).filter((item) => `${item.inbound_id}::${item.inbound_tag || ''}` === outboundSourceXUIInboundKey)
    : []
  const ruleOutboundOptions = [
    ...singBoxOutboundOptions,
    ...xuiInboundOptions,
  ]
  const ruleXUIClients = ruleXUIInboundKey
    ? (sourceXUIOverview?.clients || []).filter((item) => `${item.inbound_id}::${item.inbound_tag || ''}` === ruleXUIInboundKey)
    : []
  const singBoxUsers = flattenSingBoxUsers(overview?.clients || []).map((item) => {
    const metadata = (singBoxConfig.clients || []).find((client) => client.inbound_tag === String(item.inbound_tag || '') && client.client_key === String(item.user_key || ''))
    return metadata ? { ...item, ...metadata, total: metadata.total_bytes || item.total, traffic_multiplier: metadata.traffic_multiplier || item.traffic_multiplier, expiry_time: metadata.expiry_time || item.expiry_time, last_active: metadata.last_active || item.last_active } : item
  })
  const inboundColumns: ColumnsType<Record<string, unknown>> = [
    { title: '节点标签', dataIndex: 'tag', render: (value) => valueText(value) },
    { title: '类型', dataIndex: 'type', width: 120, render: (value) => valueText(value) },
    { title: '监听地址', dataIndex: 'listen', render: (value) => valueText(value) },
    { title: '监听端口', dataIndex: 'listen_port', width: 110, render: (value) => valueText(value) },
    { title: '客户端', key: 'users', width: 150, render: (_, item) => <Space wrap><Tag>{Array.isArray(item.users) ? item.users.length : 0}</Tag><Button size="small" type="link" disabled={!canManage} onClick={() => { setUser({ inbound_tag: String(item.tag || ''), name: '', userkey: '' }); setEditor('user') }}>添加客户端</Button></Space> },
  ]
  const outboundColumns: ColumnsType<Record<string, unknown>> = [
    { title: '标签', dataIndex: 'tag', render: (value) => valueText(value) },
    { title: '类型', dataIndex: 'type', width: 130, render: (value) => valueText(value) },
    { title: '服务器', dataIndex: 'server', render: (value) => valueText(value) },
    { title: '端口', dataIndex: 'server_port', width: 100, render: (value) => valueText(value) },
    { title: '方法', dataIndex: 'method', render: (value) => valueText(value) },
  ]
  const ruleColumns: ColumnsType<Record<string, unknown>> = [
    { title: '入站', dataIndex: 'inbound', render: (value) => valueText(value) },
    { title: '域名 / IP', key: 'match', render: (_, item) => [valueText(item.domain), valueText(item.ip_cidr)].filter((value) => value !== '-').join(' · ') || '-' },
    { title: '端口', dataIndex: 'port', render: (value) => valueText(value) },
    { title: '出站', dataIndex: 'outbound', render: (value) => valueText(value) },
    { title: '动作', dataIndex: 'action', render: (value) => valueText(value) },
  ]
  const userColumns: ColumnsType<Record<string, unknown>> = [
    { title: '客户端', key: 'identity', width: 190, render: (_, item) => <div><Text strong>{valueText(item.remark || item.name || item.userkey || item.username || item.email || item.uuid || item.password)}</Text><div className="muted-line">{valueText(item.userkey || item.email || item.username || item.uuid)}</div></div> },
    { title: '所在节点', dataIndex: 'inbound_tag', width: 150, render: (value) => valueText(value) },
    { title: '状态', key: 'status', width: 100, render: (_, item) => <Tag color={item.enabled === false ? 'default' : 'processing'}>{item.enabled === false ? '停用' : '启用'}</Tag> },
    { title: '流量', key: 'traffic', width: 190, render: (_, item) => <div><Text>已用 {formatBytes(Number(item.total || item.all_time || item.traffic_total || 0))}</Text><div className="muted-line">↑ {formatBytes(Number(item.up || 0))} · ↓ {formatBytes(Number(item.down || 0))}</div></div> },
    { title: '到期时间', key: 'expiry', width: 150, render: (_, item) => formatExpiryTime(Number(item.expiry_time || 0)) },
    { title: '流量倍数', key: 'multiplier', width: 100, render: (_, item) => item.traffic_multiplier ? `${item.traffic_multiplier} 倍` : '-' },
    { title: '最近活跃', key: 'active', width: 140, render: (_, item) => formatRelativeTime(Number(item.last_online || item.last_active || 0)) },
    { title: '操作', key: 'actions', width: 170, render: (_, item) => <Space><Button size="small" disabled={!canManage} onClick={() => { setMetadataUser(item); setMetadataDraft({ remark: String(item.remark || ''), totalGB: Number(item.total_bytes || 0) / 1024 ** 3, trafficMultiplier: Number(item.traffic_multiplier || 1), expiryTime: Number(item.expiry_time || 0), lastActive: Number(item.last_active || item.last_online || 0) }) }}>编辑</Button><Popconfirm title="删除这个 sing-box 客户端？" okText="删除" cancelText="取消" okButtonProps={{ danger: true }} onConfirm={() => void removeUser(item)}><Button size="small" danger icon={<DeleteOutlined />} disabled={!canManage || saving}>删除</Button></Popconfirm></Space> },
  ]
  const actionColumns: ColumnsType<XUIAction> = [
    { title: 'ID', dataIndex: 'id', width: 70 },
    { title: '操作', dataIndex: 'kind', render: (value: string) => value.replace('singbox_', '') },
    { title: '状态', dataIndex: 'status', width: 100, render: (value: string) => <Tag color={actionStatusColor(value)}>{actionStatusLabel(value)}</Tag> },
    { title: '结果', key: 'result', render: (_, item) => <Text>{item.error || shortJSON(item.result) || shortJSON(item.payload) || '-'}</Text> },
  ]

  const removeUser = async (item: Record<string, unknown>) => {
    const inboundTag = String(item.inbound_tag || '')
    const userKey = String(item.user_key || '')
    if (!inboundTag || !userKey) return
    setSaving(true)
    try {
      await fetchJSON(`/api/v1/agents/${agentID}/singbox/actions`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ kind: 'singbox_delete_user', payload: { inbound_tag: inboundTag, user_key: userKey } }) })
      message.success('sing-box 客户端删除操作已下发')
      onRefreshActions()
      setTimeout(() => { onRefreshActions(); void load() }, 1000)
    } catch (submitError) {
      message.error(submitError instanceof Error ? submitError.message : 'sing-box 客户端删除失败')
    } finally { setSaving(false) }
  }

  const saveMetadata = () => {
    if (!metadataUser) return
    const inboundTag = String(metadataUser.inbound_tag || '')
    const clientKey = String(metadataUser.user_key || '')
    if (!inboundTag || !clientKey) return
    const next: SingBoxClientMetadata[] = [...(singBoxConfig.clients || []).filter((client) => !(client.inbound_tag === inboundTag && client.client_key === clientKey)), {
      inbound_tag: inboundTag,
      client_key: clientKey,
      remark: metadataDraft.remark.trim(),
      total_bytes: Math.max(0, metadataDraft.totalGB) * 1024 ** 3,
      traffic_multiplier: Math.max(0.1, metadataDraft.trafficMultiplier),
      expiry_time: Math.max(0, metadataDraft.expiryTime),
      last_active: Math.max(0, metadataDraft.lastActive),
    }]
    onSingBoxChange({ clients: next })
    onSaveConfig()
    setMetadataUser(null)
  }

  const tabs = [
    { key: 'inbounds', label: `入站节点 (${overview?.clients.length || 0})`, children: <Table rowKey={(item, index) => `${valueText(item.tag)}-${valueText(item.listen_port)}-${index}`} size="small" columns={inboundColumns} dataSource={overview?.clients || []} pagination={{ pageSize: 8, hideOnSinglePage: true }} scroll={{ x: 920 }} /> },
    { key: 'users', label: `客户端 (${singBoxUsers.length})`, children: <Table className="xui-client-table" rowKey={(item, index) => `${valueText(item.inbound_tag)}-${valueText(item.user_key)}-${index}`} size="small" columns={userColumns} dataSource={singBoxUsers} pagination={{ pageSize: 8, hideOnSinglePage: true }} scroll={{ x: 1050 }} /> },
    { key: 'outbounds', label: `出站节点 (${overview?.nodes.length || 0})`, children: <Table rowKey={(item, index) => `${valueText(item.tag)}-${index}`} size="small" columns={outboundColumns} dataSource={overview?.nodes || []} pagination={{ pageSize: 8, hideOnSinglePage: true }} scroll={{ x: 760 }} /> },
    { key: 'rules', label: `路由规则 (${overview?.forwarding_rules.length || 0})`, children: <Table rowKey={(_, index) => String(index)} size="small" columns={ruleColumns} dataSource={overview?.forwarding_rules || []} pagination={{ pageSize: 8, hideOnSinglePage: true }} scroll={{ x: 900 }} /> },
    { key: 'actions', label: `操作历史 (${actions.filter((item) => item.kind.startsWith('singbox_')).length})`, children: <Table rowKey="id" size="small" loading={actionsLoading} columns={actionColumns} dataSource={actions.filter((item) => item.kind.startsWith('singbox_'))} pagination={{ pageSize: 8, hideOnSinglePage: true }} scroll={{ x: 760 }} /> },
  ]

  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      {!enabled ? <Alert type="warning" showIcon message="sing-box 未启用" description="请先在本页的托管配置中启用并保存。" /> : null}
      {error ? <Alert type="warning" showIcon message="sing-box 采集异常" description={error} /> : null}
      <Card bordered={false} className="config-section-card">
        <div className="section-title-row">
          <Title level={4}>sing-box</Title>
          <Space wrap>
            <Button icon={<ReloadOutlined />} onClick={() => void load()}>刷新</Button>
            <Button icon={<SettingOutlined />} disabled={!canManage} onClick={() => setConfigOpen(true)}>配置</Button>
            <Button type="primary" icon={<PlusOutlined />} disabled={!enabled || !canManage} onClick={openInboundEditor}>添加入站节点</Button>
            <Button disabled={!enabled || !canManage} onClick={openOutboundEditor}>添加出站节点</Button>
            <Button disabled={!enabled || !canManage || !(overview?.clients.length)} onClick={() => { setUser({ inbound_tag: String(overview?.clients[0]?.tag || ''), name: '', userkey: '' }); setEditor('user') }}>添加客户端</Button>
            <Button disabled={!enabled || !canManage} onClick={() => { setRule({ source_agent_id: agentID, domain: '', ip: '', port: '', inbound: '', outbound: '' }); setRuleSourceSelection(''); setRuleXUIInboundKey(''); setRuleXUIClientKey(''); setEditor('rule') }}>添加转发规则</Button>
          </Space>
        </div>
        <Text type="secondary">入站 {overview?.clients.length || 0} · 出站 {overview?.nodes.length || 0} · 路由规则 {overview?.forwarding_rules.length || 0} · 服务 {overview?.running ? '运行中' : '未运行'}</Text>
      </Card>
      <Card bordered={false} className="config-section-card"><Tabs items={tabs} /></Card>
      <Modal title="sing-box 托管配置" open={configOpen} onCancel={() => setConfigOpen(false)} onOk={() => { onSaveConfig(); setConfigOpen(false) }} confirmLoading={configSaving} okText="保存配置" cancelText="取消">
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
          <div><Text type="secondary">启用采集与操作</Text><Select style={{ width: '100%' }} disabled={!canManage} value={singBoxConfig.enabled ? 'enabled' : 'disabled'} options={[{ value: 'enabled', label: '启用' }, { value: 'disabled', label: '停用' }]} onChange={(value) => onSingBoxChange({ enabled: value === 'enabled' })} /></div>
          <div><Text type="secondary">配置文件路径</Text><Input disabled={!canManage} value={singBoxConfig.config_path || ''} placeholder="默认 /etc/sing-box/config.json" onChange={(event) => onSingBoxChange({ config_path: event.target.value })} /></div>
          <div><Text type="secondary">服务名</Text><Input disabled={!canManage} value={singBoxConfig.service_name || ''} placeholder="默认 sing-box" onChange={(event) => onSingBoxChange({ service_name: event.target.value })} /></div>
          <div><Text type="secondary">二进制路径（可选）</Text><Input disabled={!canManage} value={singBoxConfig.binary_path || ''} placeholder="留空时从 PATH 查找 sing-box" onChange={(event) => onSingBoxChange({ binary_path: event.target.value })} /></div>
        </Space>
      </Modal>
      <Modal title="添加入站节点" open={editor === 'inbound'} onCancel={() => setEditor(null)} onOk={() => void submit()} confirmLoading={saving} okText="下发" cancelText="取消">
        <Row gutter={[12, 12]}>
          <Col xs={24} md={12}><Text type="secondary">标签</Text><Input value={inbound.tag} placeholder="例如 proxy-1080" onChange={(event) => setInbound({ ...inbound, tag: event.target.value })} /></Col>
          <Col xs={24} md={12}><Text type="secondary">类型</Text><Select style={{ width: '100%' }} value={outbound.type.trim().toLowerCase() || 'snell'} options={[{ value: 'snell', label: 'Snell' }, ...(outbound.type.trim().toLowerCase() && outbound.type.trim().toLowerCase() !== 'snell' ? [{ value: outbound.type.trim().toLowerCase(), label: `${outbound.type}（来源配置）` }] : [])]} disabled /></Col>
          <Col xs={24} md={12}><Text type="secondary">监听地址</Text><Input value={inbound.listen} placeholder="默认 ::" onChange={(event) => setInbound({ ...inbound, listen: event.target.value })} /></Col>
          <Col xs={24} md={12}><Text type="secondary">监听端口</Text><InputNumber style={{ width: '100%' }} min={1} max={65535} value={inbound.listen_port} onChange={(value) => setInbound({ ...inbound, listen_port: Number(value || 0) })} /></Col>
          {inbound.type.trim().toLowerCase() === 'snell' ? <>
            <Col xs={24} md={12}><Text type="secondary">Snell 版本</Text><Select style={{ width: '100%' }} value={inbound.version} options={[{ value: 5, label: '版本 5 · obfs_mode' }, { value: 6, label: '版本 6 · traffic shaping' }]} onChange={(value) => setInbound({ ...inbound, version: value })} /></Col>
            <Col xs={24} md={12}><Text type="secondary">PSK（自动生成）</Text><Space.Compact style={{ width: '100%' }}><Input readOnly value={inbound.psk} title={inbound.psk} /><Button aria-label="重新生成入站 PSK" title="重新生成 PSK" icon={<ReloadOutlined />} onClick={() => setInbound({ ...inbound, psk: randomPSK() })} /></Space.Compact></Col>
            {inbound.version === 6 ? <Col xs={24} md={12}><Text type="secondary">Traffic shaping mode</Text><Select style={{ width: '100%' }} value={inbound.mode} options={['default', 'unshaped', 'unsafe-raw'].map((value) => ({ value }))} onChange={(value) => setInbound({ ...inbound, mode: value })} /></Col> : <Col xs={24} md={12}><Text type="secondary">HTTP obfuscation</Text><Select style={{ width: '100%' }} value={inbound.obfs_mode} options={[{ value: 'none', label: 'none' }, { value: 'http', label: 'http' }]} onChange={(value) => setInbound({ ...inbound, obfs_mode: value })} /></Col>}
          </> : null}
          <Col xs={24}><Text type="secondary">高级 JSON（可选，用于 users、TLS、传输层等协议字段）</Text><Input.TextArea rows={6} value={inbound.raw} placeholder={'例如 {"users":[{"name":"alice","uuid":"..."}]}' } onChange={(event) => setInbound({ ...inbound, raw: event.target.value })} /></Col>
        </Row>
      </Modal>
      <Modal title="添加出站节点" open={editor === 'outbound'} onCancel={() => setEditor(null)} onOk={() => void submit()} confirmLoading={saving} okText="下发" cancelText="取消">
        <Row gutter={[12, 12]}>
          <Col xs={24}><Text type="secondary">出站来源（可选，按 x-ui 转发规则方式复制已有出站）</Text><Select showSearch style={{ width: '100%' }} options={sourceOptions} value={outboundSourceAgentID} onChange={(value) => { setOutboundSourceAgentID(value); setOutboundSourceTag(''); setOutboundSourceXUIInboundKey(''); setOutboundSourceXUIClientKey('') }} /></Col>
          <Col xs={24}><Text type="secondary">来源出站节点</Text><Select showSearch allowClear style={{ width: '100%' }} loading={sourceLoading} options={outboundSourceOptions} value={outboundSourceTag || undefined} placeholder="选择来源 Client 中已有的出站；留空使用下方手动配置" onChange={selectOutboundSource} notFoundContent={sourceLoading ? '正在读取来源配置…' : '来源 Client 没有可用的 sing-box 出站或 x-ui 入站'} /></Col>
          {outboundSourceXUIInboundKey ? <Col xs={24}><Text type="secondary">来源入站客户端（可选）</Text><Select showSearch allowClear style={{ width: '100%' }} options={xuiSourceClients.map((item) => ({ value: xuiClientSourceKey(item), label: `${item.email || item.comment || item.client_id || '客户端'} · ${item.protocol || ''}` }))} value={outboundSourceXUIClientKey || undefined} placeholder="选择客户端以自动填充 UUID / 密码等凭据" onChange={selectXUIClientSource} /></Col> : null}
          {sourceError && outboundSourceAgentID !== agentID ? <Col xs={24}><Alert type="warning" showIcon message="来源 Client 读取失败" description={sourceError} /></Col> : null}
          {outboundSourceTag ? <Col xs={24}><Alert type="info" showIcon message="下发时会复制来源 Client 的完整出站配置" description="目标标签仍使用下方的“标签 / 备注”；来源配置中的密码、PSK、TLS 和协议字段会由 Server 一并复制。" /></Col> : null}
          <Col xs={24} md={12}><Text type="secondary">标签 / 备注</Text><Input value={outbound.tag} placeholder="例如 HK-SS-TEST" onChange={(event) => setOutbound({ ...outbound, tag: event.target.value })} /></Col>
          <Col xs={24} md={12}><Text type="secondary">类型</Text><Select style={{ width: '100%' }} value="snell" options={[{ value: 'snell', label: 'Snell' }]} disabled /></Col>
          <Col xs={24} md={12}><Text type="secondary">服务器 IP</Text><Input value={outbound.server} placeholder="仅允许 IPv4 / IPv6" onChange={(event) => setOutbound({ ...outbound, server: event.target.value })} /></Col>
          <Col xs={24} md={12}><Text type="secondary">服务器端口</Text><InputNumber style={{ width: '100%' }} min={1} max={65535} value={outbound.server_port} onChange={(value) => setOutbound({ ...outbound, server_port: Number(value || 0) })} /></Col>
          {outbound.type.trim().toLowerCase() === 'snell' ? <>
            <Col xs={24} md={12}><Text type="secondary">Snell 版本</Text><Select style={{ width: '100%' }} value={outbound.version} options={[{ value: 4, label: '版本 4 · obfs_mode' }, { value: 6, label: '版本 6 · traffic shaping' }]} onChange={(value) => setOutbound({ ...outbound, version: value })} /></Col>
            <Col xs={24} md={12}><Text type="secondary">PSK（自动生成）</Text><Space.Compact style={{ width: '100%' }}><Input readOnly value={outbound.psk} title={outbound.psk} /><Button aria-label="重新生成出站 PSK" title="重新生成 PSK" icon={<ReloadOutlined />} onClick={() => setOutbound({ ...outbound, psk: randomPSK() })} /></Space.Compact></Col>
            <Col xs={24} md={12}><Text type="secondary">User key（多用户服务端）</Text><Input value={outbound.userkey} onChange={(event) => setOutbound({ ...outbound, userkey: event.target.value })} /></Col>
            <Col xs={24} md={12}><Text type="secondary">Network</Text><Select allowClear style={{ width: '100%' }} value={outbound.network || undefined} options={[{ value: 'tcp', label: 'TCP' }, { value: 'udp', label: 'UDP' }]} onChange={(value) => setOutbound({ ...outbound, network: value || '' })} /></Col>
            {outbound.version === 6 ? <Col xs={24} md={12}><Text type="secondary">Traffic shaping mode</Text><Select style={{ width: '100%' }} value={outbound.mode} options={['default', 'unshaped', 'unsafe-raw'].map((value) => ({ value }))} onChange={(value) => setOutbound({ ...outbound, mode: value })} /></Col> : <><Col xs={24} md={12}><Text type="secondary">HTTP obfuscation</Text><Select style={{ width: '100%' }} value={outbound.obfs_mode} options={[{ value: 'none', label: 'none' }, { value: 'http', label: 'http' }]} onChange={(value) => setOutbound({ ...outbound, obfs_mode: value })} /></Col><Col xs={24} md={12}><Text type="secondary">Obfs Host</Text><Input value={outbound.obfs_host} onChange={(event) => setOutbound({ ...outbound, obfs_host: event.target.value })} /></Col></>}
          </> : null}
          <Col xs={24}><Text type="secondary">高级 JSON（可选，用于 method、password、TLS、UUID、传输层等协议字段）</Text><Input.TextArea rows={8} value={outbound.raw} placeholder={'例如 {"method":"xchacha20-ietf-poly1305","password":"..."}' } onChange={(event) => setOutbound({ ...outbound, raw: event.target.value })} /></Col>
        </Row>
      </Modal>
      <Modal title="添加客户端" open={editor === 'user'} onCancel={() => setEditor(null)} onOk={() => void submit()} confirmLoading={saving} okText="下发" cancelText="取消">
        <Row gutter={[12, 12]}>
          <Col xs={24}><Text type="secondary">所在入站节点</Text><Select showSearch style={{ width: '100%' }} value={user.inbound_tag || undefined} options={(overview?.clients || []).map((item) => ({ value: String(item.tag || ''), label: `${String(item.tag || '')} · ${String(item.type || '')}` })).filter((item) => item.value)} onChange={(value) => setUser({ ...user, inbound_tag: value })} /></Col>
          <Col xs={24} md={12}><Text type="secondary">名称</Text><Input value={user.name} placeholder="例如 alice" onChange={(event) => setUser({ ...user, name: event.target.value })} /></Col>
          <Col xs={24}><Text type="secondary">User key（必填）</Text><Input value={user.userkey} placeholder="Snell 用户密钥" onChange={(event) => setUser({ ...user, userkey: event.target.value })} /></Col>
        </Row>
      </Modal>
      <Modal title={`编辑客户端 · ${String(metadataUser?.name || metadataUser?.username || metadataUser?.user_key || '')}`} open={metadataUser !== null} onCancel={() => setMetadataUser(null)} onOk={saveMetadata} confirmLoading={configSaving} okText="保存记录" cancelText="取消">
        <Row gutter={[12, 12]}>
          <Col xs={24}><Text type="secondary">备注</Text><Input value={metadataDraft.remark} onChange={(event) => setMetadataDraft({ ...metadataDraft, remark: event.target.value })} /></Col>
          <Col xs={24} md={12}><Text type="secondary">流量上限（GB）</Text><InputNumber style={{ width: '100%' }} min={0} precision={2} value={metadataDraft.totalGB} onChange={(value) => setMetadataDraft({ ...metadataDraft, totalGB: Number(value || 0) })} /></Col>
          <Col xs={24} md={12}><Text type="secondary">流量倍数</Text><InputNumber style={{ width: '100%' }} min={0.1} max={100} step={0.1} precision={2} value={metadataDraft.trafficMultiplier} onChange={(value) => setMetadataDraft({ ...metadataDraft, trafficMultiplier: Number(value || 1) })} /></Col>
          <Col xs={24} md={12}><Text type="secondary">到期时间</Text><Input type="datetime-local" value={metadataDateInput(metadataDraft.expiryTime)} onChange={(event) => setMetadataDraft({ ...metadataDraft, expiryTime: parseMetadataDate(event.target.value) })} /></Col>
          <Col xs={24} md={12}><Text type="secondary">最近活跃时间</Text><Input type="datetime-local" value={metadataDateInput(metadataDraft.lastActive)} onChange={(event) => setMetadataDraft({ ...metadataDraft, lastActive: parseMetadataDate(event.target.value) })} /></Col>
          <Col xs={24}><Text type="secondary">流量和活跃时间如果 sing-box 未上报，将保留为手动记录；后续可继续编辑。</Text></Col>
        </Row>
      </Modal>
      <Modal title="添加转发规则" open={editor === 'rule'} onCancel={() => setEditor(null)} onOk={() => void submit()} confirmLoading={saving || sourceLoading} okText="下发" cancelText="取消">
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
          <div><Text type="secondary">来源 Client</Text><Select showSearch style={{ width: '100%' }} options={sourceOptions} value={rule.source_agent_id} onChange={(value) => { setRule({ ...rule, source_agent_id: value, outbound: '' }); setRuleSourceSelection(''); setRuleXUIInboundKey(''); setRuleXUIClientKey('') }} /></div>
          <div><Text type="secondary">出站节点 / 来源入站</Text><Select showSearch style={{ width: '100%' }} loading={sourceLoading} options={ruleOutboundOptions} value={ruleSourceSelection || undefined} placeholder="选择 sing-box 出站或其他 Client 的 x-ui 入站" onChange={selectRuleOutboundSource} notFoundContent={sourceLoading ? '正在读取来源配置…' : '来源 Client 没有可用的 sing-box 出站或 x-ui 入站'} /></div>
          {ruleXUIInboundKey ? <div><Text type="secondary">来源入站客户端（可选）</Text><Select showSearch allowClear style={{ width: '100%' }} options={ruleXUIClients.map((item) => ({ value: xuiClientSourceKey(item), label: `${item.email || item.comment || item.client_id || '客户端'} · ${item.protocol || ''}` }))} value={ruleXUIClientKey || undefined} placeholder="选择客户端以自动填充凭据" onChange={selectRuleXUIClientSource} /></div> : null}
          {sourceLoading ? <Text type="secondary">正在读取来源 Client 的 sing-box 出站...</Text> : null}
          {sourceError ? <Alert type="warning" showIcon message="来源 Client 读取失败" description={sourceError} /> : null}
          {ruleSourceSelection.startsWith('xui-node:') ? <Alert type="info" showIcon message="地址自动解析" description="来源地址如果是域名，目标 Client 下发时会自动解析为 IP；无需手动填写 IP。" /> : null}
          <Row gutter={[12, 12]}>
            <Col xs={24} md={12}><Text type="secondary">域名</Text><Input.TextArea rows={3} value={rule.domain} onChange={(event) => setRule({ ...rule, domain: event.target.value })} /></Col>
            <Col xs={24} md={12}><Text type="secondary">IP/CIDR</Text><Input.TextArea rows={3} value={rule.ip} onChange={(event) => setRule({ ...rule, ip: event.target.value })} /></Col>
            <Col xs={24} md={12}><Text type="secondary">端口</Text><Input value={rule.port} placeholder="443, 8443" onChange={(event) => setRule({ ...rule, port: event.target.value })} /></Col>
            <Col xs={24} md={12}><Text type="secondary">入站标签</Text><Input value={rule.inbound} onChange={(event) => setRule({ ...rule, inbound: event.target.value })} /></Col>
          </Row>
          {rule.source_agent_id !== agentID ? <Text type="secondary">来源 Client 的 outbound 会随规则一起复制到当前 Client。</Text> : null}
        </Space>
      </Modal>
    </Space>
  )
}

function splitList(value: string): string[] {
  return value.split(/[\n,]/).map((item) => item.trim()).filter(Boolean)
}

function parseJSONDraft(value: string, label: string): Record<string, unknown> | null {
  if (!value.trim()) return {}
  try {
    const parsed = JSON.parse(value)
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) throw new Error('必须是 JSON 对象')
    return parsed as Record<string, unknown>
  } catch (error) {
    message.error(`${label}高级 JSON 无效：${error instanceof Error ? error.message : '格式错误'}`)
    return null
  }
}

function randomPSK(): string {
  const bytes = new Uint8Array(32)
  crypto.getRandomValues(bytes)
  let binary = ''
  for (const byte of bytes) binary += String.fromCharCode(byte)
  return btoa(binary)
}

function flattenSingBoxUsers(inbounds: Record<string, unknown>[]): Record<string, unknown>[] {
  return inbounds.flatMap((inbound) => {
    const users = Array.isArray(inbound.users) ? inbound.users : []
    return users.filter((user): user is Record<string, unknown> => Boolean(user && typeof user === 'object' && !Array.isArray(user))).map((user) => ({
      ...user,
      inbound_tag: inbound.tag,
      inbound_type: inbound.type,
      user_key: user.name || user.username || user.email || user.uuid || user.password || '',
    }))
  })
}

function metadataDateInput(value?: number): string {
  if (!value) return ''
  const date = new Date(value)
  if (!Number.isFinite(date.getTime())) return ''
  return new Date(date.getTime() - date.getTimezoneOffset() * 60_000).toISOString().slice(0, 16)
}

function xuiClientSourceKey(client: { inbound_id: number; inbound_tag?: string; email?: string; client_id?: string }): string {
  return `${client.inbound_id}::${client.inbound_tag || ''}::${client.email || client.client_id || ''}`
}

function sourceXUIInboundNodes(overview?: XUIOverview | null): XUIOverview['nodes'] {
  if (!overview) return []
  const nodes = [...(overview.nodes || [])]
  for (const client of overview.clients || []) {
    if (nodes.some((node) => node.id === client.inbound_id && String(node.tag || '') === String(client.inbound_tag || ''))) continue
    nodes.push({
      id: client.inbound_id,
      tag: client.inbound_tag,
      remark: client.inbound_remark,
      protocol: client.protocol,
      enabled: client.enabled,
      export_endpoint: client.export_endpoint,
      port: client.export_endpoint?.port,
      route: client.route,
    })
  }
  return nodes
}

function normalizeSingBoxType(protocol?: string): string {
  const value = String(protocol || '').trim().toLowerCase()
  if (value === 'freedom') return 'direct'
  if (value === 'blackhole') return 'block'
  if (value === 'shadowsocks') return 'shadowsocks'
  if (value === 'vmess') return 'vmess'
  if (value === 'vless') return 'vless'
  if (value === 'trojan') return 'trojan'
  if (value === 'socks' || value === 'http') return value
  return value || 'snell'
}

function buildSingBoxOutboundFromImport(imported: Partial<XUIOutboundActionForm>, tag: string): Record<string, unknown> {
  const type = String(imported.protocol || 'vless').trim().toLowerCase()
  const node: Record<string, unknown> = {
    type,
    tag: tag.trim(),
    server: imported.address || '',
    server_port: Number(imported.port || 0),
  }
  if (imported.uuid) node.uuid = imported.uuid
  if (imported.password) node.password = imported.password
  if (type === 'vmess' && imported.method) node.security = imported.method
  else if (imported.method) node.method = imported.method
  if (imported.flow) node.flow = imported.flow
  if (imported.network) node.network = imported.network
  if (imported.security && imported.security !== 'none') {
    node.tls = {
      enabled: true,
      server_name: imported.server_name || undefined,
      alpn: imported.alpn ? imported.alpn.split(',').map((item) => item.trim()).filter(Boolean) : undefined,
    }
  }
  const network = String(imported.network || '').trim().toLowerCase()
  if (network === 'ws') {
    node.transport = {
      type: 'ws',
      path: imported.ws_path || '/',
      headers: imported.ws_host ? { Host: imported.ws_host } : undefined,
    }
  } else if (network === 'grpc') {
    node.transport = { type: 'grpc', service_name: imported.grpc_service || '' }
  } else if (network === 'http' || network === 'h2') {
    node.transport = { type: 'http', host: imported.ws_host ? [imported.ws_host] : undefined, path: imported.ws_path || '/' }
  }
  return node
}

function parseMetadataDate(value: string): number {
  const parsed = value ? Date.parse(value) : 0
  return Number.isFinite(parsed) ? parsed : 0
}

function valueText(value: unknown): string {
  if (Array.isArray(value)) return value.map((item) => valueText(item)).join(', ')
  if (value && typeof value === 'object') return JSON.stringify(value)
  if (value === undefined || value === null || value === '') return '-'
  return String(value)
}
