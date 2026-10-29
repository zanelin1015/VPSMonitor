import { useEffect, useState } from 'react'
import { Alert, Button, Card, Col, Input, InputNumber, Modal, Row, Select, Space, Table, Tag, Typography, message } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { PlusOutlined, ReloadOutlined } from '@ant-design/icons'

import type { SingBoxOverview, XUIAction } from '../types'
import { fetchJSON } from '../lib/api'
import { actionStatusColor, actionStatusLabel, shortJSON } from '../lib/appHelpers'

const { Text, Title } = Typography

type EditorKind = 'node' | 'client' | 'rule' | null

export function SingBoxActionPanel(props: {
  agentID: string
  enabled: boolean
  canManage?: boolean
  actions: XUIAction[]
  actionsLoading?: boolean
  onRefreshActions: () => void
}) {
  const { agentID, enabled, canManage = true, actions, actionsLoading, onRefreshActions } = props
  const [overview, setOverview] = useState<SingBoxOverview | null>(null)
  const [error, setError] = useState('')
  const [editor, setEditor] = useState<EditorKind>(null)
  const [saving, setSaving] = useState(false)
  const [node, setNode] = useState({ remark: '', type: 'shadowsocks', server: '', server_port: 443, method: 'xchacha20-ietf-poly1305', password: '', psk: '', version: 5 })
  const [client, setClient] = useState({ remark: '', listen: '::', listen_port: 27411, psk: '', version: 5, obfs_mode: 'none' })
  const [rule, setRule] = useState({ domain: '', ip: '', port: '', inbound: '', outbound: '' })

  const load = async () => {
    try {
      const data = await fetchJSON<SingBoxOverview>(`/api/v1/agents/${agentID}/singbox/overview`)
      setOverview(data)
      setError(data.error || '')
    } catch (loadError) {
      setError(loadError instanceof Error ? loadError.message : '加载 sing-box 配置失败')
    }
  }
  useEffect(() => { void load() }, [agentID])

  async function submit() {
    let kind = ''
    let payload: Record<string, unknown> = {}
    if (editor === 'node') {
      if (!node.remark) { message.error('出站节点需要填写备注'); return }
      if (node.type !== 'direct' && !node.server) { message.error('出站节点只允许填写 IP 地址'); return }
      if (node.type === 'shadowsocks' && (!node.method || !node.password)) { message.error('Shadowsocks 节点需要加密方法和密码'); return }
      if (node.type === 'snell' && !node.psk) { message.error('Snell 出站需要 PSK'); return }
      kind = 'singbox_add_node'
      payload = { node: {
        type: node.type,
        remark: node.remark.trim(),
        tag: node.remark.trim(),
        ...(node.type !== 'direct' ? { server: node.server.trim(), server_port: node.server_port } : {}),
        ...(node.type === 'shadowsocks' ? { method: node.method, password: node.password } : {}),
        ...(node.type === 'snell' ? { psk: node.psk, version: node.version } : {}),
      } }
    } else if (editor === 'client') {
      if (!client.remark || !client.psk) { message.error('Snell 客户端需要备注和 PSK'); return }
      kind = 'singbox_add_client'
      payload = { client: { type: 'snell', remark: client.remark.trim(), tag: client.remark.trim(), listen: client.listen.trim() || '::', listen_port: client.listen_port, version: client.version, psk: client.psk, obfs_mode: client.obfs_mode } }
    } else if (editor === 'rule') {
      if (!rule.outbound || !overview?.nodes.some((item) => item.tag === rule.outbound)) { message.error('转发规则需要选择已存在的出站节点'); return }
      const next: Record<string, unknown> = { outbound: rule.outbound.trim() }
      if (rule.domain.trim()) next.domain = rule.domain.split(/[\n,]/).map((value) => value.trim()).filter(Boolean)
      if (rule.ip.trim()) next.ip_cidr = rule.ip.split(/[\n,]/).map((value) => value.trim()).filter(Boolean)
      if (rule.port.trim()) next.port = rule.port.trim()
      if (rule.inbound.trim()) next.inbound = rule.inbound.split(/[\n,]/).map((value) => value.trim()).filter(Boolean)
      kind = 'singbox_add_forwarding_rule'
      payload = { rule: next }
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

  const actionColumns: ColumnsType<XUIAction> = [
    { title: 'ID', dataIndex: 'id', width: 70 },
    { title: '操作', dataIndex: 'kind', render: (value: string) => value.replace('singbox_', '') },
    { title: '状态', dataIndex: 'status', width: 100, render: (value: string) => <Tag color={actionStatusColor(value)}>{actionStatusLabel(value)}</Tag> },
    { title: '结果', key: 'result', render: (_, item) => <Text>{item.error || shortJSON(item.result) || shortJSON(item.payload) || '-'}</Text> },
  ]
  const nodeOptions = (overview?.nodes || []).map((item) => ({ value: String(item.tag || ''), label: `${String(item.tag || '')} · ${String(item.type || '')}` })).filter((item) => item.value)

  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      {!enabled ? <Alert type="warning" showIcon message="sing-box 未启用" description="请先在基础信息 → sing-box 配置中启用并保存。" /> : null}
      {error ? <Alert type="warning" showIcon message="sing-box 采集异常" description={error} /> : null}
      <Card bordered={false} className="config-section-card">
        <div className="section-title-row">
          <Title level={4}>sing-box 配置</Title>
          <Space wrap>
            <Button icon={<ReloadOutlined />} onClick={() => void load()}>刷新</Button>
            <Button type="primary" icon={<PlusOutlined />} disabled={!enabled || !canManage} onClick={() => setEditor('node')}>添加出站节点</Button>
            <Button disabled={!enabled || !canManage} onClick={() => setEditor('client')}>添加 Snell 客户端</Button>
            <Button disabled={!enabled || !canManage} onClick={() => setEditor('rule')}>添加转发规则</Button>
          </Space>
        </div>
        <Text type="secondary">节点 {overview?.nodes.length || 0} · 客户端 {overview?.clients.length || 0} · 转发规则 {overview?.forwarding_rules.length || 0} · 服务 {overview?.running ? '运行中' : '未运行'}</Text>
      </Card>
      <Card bordered={false} className="config-section-card" title="sing-box 操作记录">
        <Table rowKey="id" size="small" loading={actionsLoading} columns={actionColumns} dataSource={actions.filter((item) => item.kind.startsWith('singbox_'))} pagination={{ pageSize: 8, hideOnSinglePage: true }} scroll={{ x: 760 }} />
      </Card>
      <Modal title={editor === 'node' ? '添加出站节点' : editor === 'client' ? '添加 Snell 客户端' : '添加 sing-box 转发规则'} open={Boolean(editor)} onCancel={() => setEditor(null)} onOk={() => void submit()} confirmLoading={saving} okText="下发" cancelText="取消" destroyOnClose>
        {editor === 'node' ? <Row gutter={[12, 12]}>
          <Col xs={24} md={12}><Text type="secondary">备注 / 标签</Text><Input value={node.remark} placeholder="例如 HK-SS-TEST" onChange={(event) => setNode({ ...node, remark: event.target.value })} /></Col>
          <Col xs={24} md={12}><Text type="secondary">类型</Text><Select style={{ width: '100%' }} value={node.type} options={[{ value: 'shadowsocks', label: 'Shadowsocks' }, { value: 'snell', label: 'Snell' }, { value: 'direct', label: 'Direct' }]} onChange={(value) => setNode({ ...node, type: value })} /></Col>
          {node.type !== 'direct' ? <>
            <Col xs={24} md={12}><Text type="secondary">服务器 IP</Text><Input value={node.server} placeholder="只允许 IPv4 / IPv6，不支持域名" onChange={(event) => setNode({ ...node, server: event.target.value })} /></Col>
            <Col xs={24} md={6}><Text type="secondary">端口</Text><InputNumber style={{ width: '100%' }} min={1} max={65535} value={node.server_port} onChange={(value) => setNode({ ...node, server_port: Number(value || 0) })} /></Col>
          </> : null}
          {node.type === 'shadowsocks' ? <>
            <Col xs={24} md={9}><Text type="secondary">加密方法</Text><Input value={node.method} onChange={(event) => setNode({ ...node, method: event.target.value })} /></Col>
            <Col xs={24} md={9}><Text type="secondary">密码</Text><Input.Password value={node.password} onChange={(event) => setNode({ ...node, password: event.target.value })} /></Col>
          </> : null}
          {node.type === 'snell' ? <>
            <Col xs={24} md={6}><Text type="secondary">Snell 版本</Text><InputNumber style={{ width: '100%' }} min={1} max={5} value={node.version} onChange={(value) => setNode({ ...node, version: Number(value || 5) })} /></Col>
            <Col xs={24} md={12}><Text type="secondary">PSK</Text><Input.Password value={node.psk} onChange={(event) => setNode({ ...node, psk: event.target.value })} /></Col>
          </> : null}
        </Row> : null}
        {editor === 'client' ? <Row gutter={[12, 12]}>
          <Col xs={24} md={12}><Text type="secondary">备注 / 标签</Text><Input value={client.remark} placeholder="例如 snell-27411" onChange={(event) => setClient({ ...client, remark: event.target.value })} /></Col>
          <Col xs={24} md={12}><Text type="secondary">PSK</Text><Input.Password value={client.psk} onChange={(event) => setClient({ ...client, psk: event.target.value })} /></Col>
          <Col xs={24} md={8}><Text type="secondary">监听地址</Text><Input value={client.listen} placeholder="默认 ::" onChange={(event) => setClient({ ...client, listen: event.target.value })} /></Col>
          <Col xs={24} md={8}><Text type="secondary">监听端口（唯一）</Text><InputNumber style={{ width: '100%' }} min={1} max={65535} value={client.listen_port} onChange={(value) => setClient({ ...client, listen_port: Number(value || 0) })} /></Col>
          <Col xs={24} md={8}><Text type="secondary">Snell 版本</Text><InputNumber style={{ width: '100%' }} min={1} max={5} value={client.version} onChange={(value) => setClient({ ...client, version: Number(value || 5) })} /></Col>
          <Col xs={24} md={8}><Text type="secondary">混淆模式</Text><Select style={{ width: '100%' }} value={client.obfs_mode} options={[{ value: 'none', label: 'none' }, { value: 'http', label: 'http' }]} onChange={(value) => setClient({ ...client, obfs_mode: value })} /></Col>
          <Col xs={24}><Text type="secondary">一个 Snell 客户端只能绑定一个监听端口；重复端口会被拒绝。</Text></Col>
        </Row> : null}
        {editor === 'rule' ? <Row gutter={[12, 12]}>
          <Col xs={24}><Text type="secondary">出站节点</Text><Select showSearch style={{ width: '100%' }} value={rule.outbound || undefined} options={nodeOptions} onChange={(value) => setRule({ ...rule, outbound: value })} /></Col>
          <Col xs={24} md={12}><Text type="secondary">域名（逗号或换行）</Text><Input.TextArea rows={3} value={rule.domain} onChange={(event) => setRule({ ...rule, domain: event.target.value })} /></Col>
          <Col xs={24} md={12}><Text type="secondary">IP/CIDR（逗号或换行）</Text><Input.TextArea rows={3} value={rule.ip} onChange={(event) => setRule({ ...rule, ip: event.target.value })} /></Col>
          <Col xs={24} md={12}><Text type="secondary">端口</Text><Input value={rule.port} placeholder="443, 8443" onChange={(event) => setRule({ ...rule, port: event.target.value })} /></Col>
          <Col xs={24} md={12}><Text type="secondary">入站标签（可选）</Text><Input value={rule.inbound} onChange={(event) => setRule({ ...rule, inbound: event.target.value })} /></Col>
        </Row> : null}
      </Modal>
    </Space>
  )
}
