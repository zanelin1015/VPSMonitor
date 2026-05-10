import { Alert, Card, Col, Row, Select, Space, Tag, Typography } from 'antd'

import type { AgentListItem, XUIClientView, XUINodeView, XUIOverview } from '../types'
import type { XUIOutboundActionForm } from '../lib/appHelpers'
import { buildOutboundImportPatch, sourceClientKey } from './xuiActionFormShared'

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
  const { form, agents, targetAgentID, currentOverview, sourceOverview, sourceLoading, onChange } = props
  const update = (patch: Partial<XUIOutboundActionForm>) => onChange({ ...form, ...patch })
  const activeSourceOverview = form.source_agent_id && currentOverview?.agent_id === form.source_agent_id ? currentOverview : sourceOverview
  const sourceClientOptions = (activeSourceOverview?.clients || []).map((client) => ({
    key: sourceClientKey(client),
    client,
  }))

  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Card className="config-section-card" bordered={false}>
        <Title level={4}>从内部 Client 节点导入</Title>
        <Alert
          type="info"
          showIcon
          className="compact-alert"
          message="只允许内部导入"
          description="选择一个已有 Client 节点客户端后，系统会自动生成当前 Client 的出站配置；不再开放手动填写协议、地址和端口。"
        />
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
                const patch: Partial<XUIOutboundActionForm> = { source_client_key: nextKey }
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
