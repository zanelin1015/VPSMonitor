import { useEffect, useState } from 'react'
import { Alert, Button, Card, Col, Input, InputNumber, Popconfirm, Row, Select, Space, Tabs, Tag, Typography, message } from 'antd'

import type { AgentListItem, OutboundLinkLibraryItem, XUIClientView, XUINodeView, XUIOverview } from '../types'
import type { XUIOutboundActionForm } from '../lib/appHelpers'
import { buildOutboundImportPatch, buildOutboundPatchFromXrayOutbound, outboundFormToXrayOutbound, parseOutboundImportText, sourceClientKey } from './xuiActionFormShared'
import { fetchJSON } from '../lib/appHelpers'

const { Text, Title } = Typography

export function renderOutboundActionForm(props: {
  form: XUIOutboundActionForm
  agents: AgentListItem[]
  targetAgentID: string
  currentOverview: XUIOverview | null
  sourceOverview: XUIOverview | null
  sourceLoading: boolean
  onChange: (form: XUIOutboundActionForm) => void
}) {
  return <XUIOutboundActionFormPanel {...props} />
}

function XUIOutboundActionFormPanel(props: {
  form: XUIOutboundActionForm
  agents: AgentListItem[]
  targetAgentID: string
  currentOverview: XUIOverview | null
  sourceOverview: XUIOverview | null
  sourceLoading: boolean
  onChange: (form: XUIOutboundActionForm) => void
}) {
  const { form, agents, targetAgentID, currentOverview, sourceOverview, sourceLoading, onChange } = props
  const update = (patch: Partial<XUIOutboundActionForm>) => onChange({ ...form, ...patch })
  const [libraryItems, setLibraryItems] = useState<OutboundLinkLibraryItem[]>([])
  const [selectedLibraryID, setSelectedLibraryID] = useState<string>()
  const [importText, setImportText] = useState('')
  const [librarySaving, setLibrarySaving] = useState(false)
  const [libraryDeleting, setLibraryDeleting] = useState(false)
  const activeSourceOverview = form.source_agent_id && currentOverview?.agent_id === form.source_agent_id ? currentOverview : sourceOverview
  const sourceClientOptions = (activeSourceOverview?.clients || []).map((client) => ({
    key: sourceClientKey(client),
    client,
  }))
  useEffect(() => {
    void fetchJSON<OutboundLinkLibraryItem[]>('/api/v1/admin/outbound-links')
      .then((items) => setLibraryItems(Array.isArray(items) ? items : []))
      .catch(() => setLibraryItems([]))
  }, [])

  const applyImportedText = () => {
    try {
      update(parseOutboundImportText(importText, form))
      message.success('已识别出站配置')
    } catch (error) {
      message.error(error instanceof Error ? error.message : '识别出站配置失败')
    }
  }
  const saveCurrentToLibrary = async () => {
    const outbound = outboundFormToXrayOutbound(form)
    setLibrarySaving(true)
    try {
      const saved = await fetchJSON<OutboundLinkLibraryItem>('/api/v1/admin/outbound-links', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          name: form.tag,
          tag: form.tag,
          protocol: form.protocol,
          outbound,
        }),
      })
      setLibraryItems((items) => [saved, ...items.filter((item) => item.id !== saved.id)])
      message.success('已保存到出口链接库')
    } catch (error) {
      message.error(error instanceof Error ? error.message : '保存出口链接库失败')
    } finally {
      setLibrarySaving(false)
    }
  }
  const applyLibraryItem = (id?: string) => {
    setSelectedLibraryID(id)
    const item = libraryItems.find((entry) => entry.id === id)
    if (!item) {
      return
    }
    update({ ...buildOutboundPatchFromXrayOutbound(item.outbound, form), source_type: 'library' })
  }
  const deleteLibraryItem = async () => {
    if (!selectedLibraryID) {
      return
    }
    setLibraryDeleting(true)
    try {
      await fetchJSON(`/api/v1/admin/outbound-links/${encodeURIComponent(selectedLibraryID)}`, { method: 'DELETE' })
      setLibraryItems((items) => items.filter((item) => item.id !== selectedLibraryID))
      setSelectedLibraryID(undefined)
      message.success('已从出口链接库删除')
    } catch (error) {
      message.error(error instanceof Error ? error.message : '删除出口链接失败')
    } finally {
      setLibraryDeleting(false)
    }
  }

  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Card className="config-section-card" bordered={false}>
        <Title level={4}>出站来源</Title>
        <Alert
          type="info"
          showIcon
          className="compact-alert"
          message="支持内部 Client、现有出站 JSON / 节点链接和公共出口链接库"
          description="提交转发规则时会携带出站配置；client 上已有等价出站会复用，不存在则自动新增。支持 VLESS、VMess、SOCKS、HTTP、Shadowsocks。"
        />
        <Tabs
          activeKey={form.source_type || 'registered_client'}
          onChange={(key) => update({ source_type: key as XUIOutboundActionForm['source_type'] })}
          items={[
            {
              key: 'registered_client',
              label: '已有 Client',
              children: (
                <Row gutter={[16, 16]}>
                  <Col xs={24} md={12}>
                    <Text type="secondary">源 Client</Text>
                    <Select
                      allowClear
                      style={{ width: '100%' }}
                      value={form.source_agent_id || undefined}
                      options={agents
                        .filter((agent) => agent.agent_id !== targetAgentID)
                        .map((agent) => ({ value: agent.agent_id, label: agent.agent_name || agent.agent_id }))}
                      onChange={(value) =>
                        update({
                          source_type: 'registered_client',
                          source_agent_id: value || '',
                          source_client_key: '',
                        })}
                    />
                  </Col>
                  <Col xs={24} md={12}>
                    <Text type="secondary">源节点客户端</Text>
                    <Select
                      allowClear
                      style={{ width: '100%' }}
                      loading={sourceLoading}
                      disabled={!form.source_agent_id}
                      value={form.source_client_key || undefined}
                      options={sourceClientOptions.map(({ key, client }) => ({
                        value: key,
                        label: `${client.email || '-'} · ${client.inbound_remark || client.inbound_tag || client.protocol || '-'}`,
                      }))}
                      onChange={(value) => {
                        const nextKey = value || ''
                        const patch: Partial<XUIOutboundActionForm> = { source_type: 'registered_client', source_client_key: nextKey }
                        const nextClient = sourceClientOptions.find((item) => item.key === nextKey)?.client
                        const nextNode = activeSourceOverview?.nodes.find(
                          (node: XUINodeView) => node.id === nextClient?.inbound_id || node.tag === nextClient?.inbound_tag,
                        )
                        if (activeSourceOverview && nextClient && nextNode) {
                          Object.assign(patch, buildOutboundImportPatch(activeSourceOverview, nextNode, nextClient as XUIClientView, form))
                        }
                        update(patch)
                      }}
                    />
                  </Col>
                </Row>
              ),
            },
            {
              key: 'library',
              label: '出口链接库',
              children: (
                <Row gutter={[16, 16]}>
                  <Col xs={24}>
                    <Text type="secondary">选择出口</Text>
                    <Space.Compact style={{ width: '100%' }}>
                      <Select
                        allowClear
                        showSearch
                        style={{ width: '100%' }}
                        value={selectedLibraryID}
                        options={libraryItems.map((item) => ({ value: item.id || '', label: `${item.name || item.tag} · ${item.protocol} · ${item.tag}` }))}
                        onChange={applyLibraryItem}
                      />
                      <Popconfirm title="删除这个出口链接？" okText="删除" cancelText="取消" onConfirm={() => void deleteLibraryItem()}>
                        <Button danger disabled={!selectedLibraryID} loading={libraryDeleting}>删除</Button>
                      </Popconfirm>
                    </Space.Compact>
                  </Col>
                </Row>
              ),
            },
            {
              key: 'manual',
              label: 'JSON / 链接导入',
              children: (
                <Space direction="vertical" style={{ width: '100%' }}>
                  <Input.TextArea
                    rows={6}
                    value={importText}
                    placeholder="粘贴 x-ui 出站 JSON，或 vless/vmess/socks/http/ss 链接"
                    onChange={(event) => setImportText(event.target.value)}
                  />
                  <Space wrap>
                    <Button type="primary" onClick={applyImportedText}>识别并填入</Button>
                    <Button loading={librarySaving} onClick={() => void saveCurrentToLibrary()}>保存当前配置到出口链接库</Button>
                  </Space>
                </Space>
              ),
            },
          ]}
        />
      </Card>
      <Card className="config-section-card" bordered={false}>
        <Title level={4}>出站配置</Title>
        <Row gutter={[12, 12]}>
          <Col xs={24} md={8}><Text type="secondary">标签</Text><Input value={form.tag} onChange={(event) => update({ tag: event.target.value })} /></Col>
          <Col xs={24} md={8}><Text type="secondary">协议</Text><Select style={{ width: '100%' }} value={form.protocol} options={[
            { value: 'vless', label: 'VLESS' },
            { value: 'vmess', label: 'VMess' },
            { value: 'socks', label: 'SOCKS' },
            { value: 'http', label: 'HTTP' },
            { value: 'shadowsocks', label: 'Shadowsocks' },
            { value: 'freedom', label: 'Freedom' },
            { value: 'blackhole', label: 'Blackhole' },
          ]} onChange={(value) => update({ protocol: value })} /></Col>
          <Col xs={24} md={8}><Text type="secondary">发送通过</Text><Input value={form.send_through} onChange={(event) => update({ send_through: event.target.value })} /></Col>
          <Col xs={24} md={12}><Text type="secondary">地址</Text><Input value={form.address} onChange={(event) => update({ address: event.target.value })} /></Col>
          <Col xs={24} md={6}><Text type="secondary">端口</Text><InputNumber style={{ width: '100%' }} min={0} max={65535} value={form.port} onChange={(value) => update({ port: Number(value || 0) })} /></Col>
          <Col xs={24} md={6}><Text type="secondary">传输</Text><Select style={{ width: '100%' }} value={form.network} options={[
            { value: 'tcp', label: 'TCP' },
            { value: 'ws', label: 'WebSocket' },
            { value: 'grpc', label: 'gRPC' },
            { value: 'h2', label: 'HTTP/2' },
          ]} onChange={(value) => update({ network: value })} /></Col>
          <Col xs={24} md={12}><Text type="secondary">ID / 用户名</Text><Input value={form.uuid} onChange={(event) => update({ uuid: event.target.value })} /></Col>
          <Col xs={24} md={12}><Text type="secondary">密码 / SS 密码</Text><Input value={form.password} onChange={(event) => update({ password: event.target.value })} /></Col>
          <Col xs={24} md={8}><Text type="secondary">加密 / 方法</Text><Input value={form.method} onChange={(event) => update({ method: event.target.value })} /></Col>
          <Col xs={24} md={8}><Text type="secondary">安全</Text><Select style={{ width: '100%' }} value={form.security} options={[
            { value: 'none', label: '无' },
            { value: 'tls', label: 'TLS' },
            { value: 'reality', label: 'Reality' },
          ]} onChange={(value) => update({ security: value })} /></Col>
          <Col xs={24} md={8}><Text type="secondary">SNI / ServerName</Text><Input value={form.server_name} onChange={(event) => update({ server_name: event.target.value })} /></Col>
          {form.security === 'reality' ? (
            <>
              <Col xs={24} md={12}><Text type="secondary">Reality PublicKey</Text><Input value={form.reality_public_key} onChange={(event) => update({ reality_public_key: event.target.value })} /></Col>
              <Col xs={24} md={6}><Text type="secondary">Short ID</Text><Input value={form.reality_short_id} onChange={(event) => update({ reality_short_id: event.target.value })} /></Col>
              <Col xs={24} md={6}><Text type="secondary">Fingerprint</Text><Input value={form.reality_fingerprint} onChange={(event) => update({ reality_fingerprint: event.target.value })} /></Col>
            </>
          ) : null}
          <Col xs={24}>
            <div className="switch-row">
              <span>提交后重启 Xray</span>
              <Tag color="success">自动执行</Tag>
            </div>
          </Col>
        </Row>
      </Card>
    </Space>
  )
}
