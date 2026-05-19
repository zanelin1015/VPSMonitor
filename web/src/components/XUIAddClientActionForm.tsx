import { Card, Col, Input, InputNumber, Row, Select, Space, Switch, Typography } from 'antd'

import type { XUIAddClientActionForm, XUIInboundClientForm } from '../lib/appHelpers'
import type { XUINodeView } from '../types'

const { Text, Title } = Typography

export function renderAddClientActionForm(props: {
  form: XUIAddClientActionForm
  inbounds: XUINodeView[]
  onChange: (form: XUIAddClientActionForm) => void
}) {
  const { form, inbounds, onChange } = props
  const update = (patch: Partial<XUIAddClientActionForm>) => onChange({ ...form, ...patch })
  const updateClient = (patch: Partial<XUIInboundClientForm>) => update({ client: { ...form.client, ...patch } })

  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Card className="config-section-card" bordered={false}>
        <Title level={4}>选择节点</Title>
        <Row gutter={[16, 16]}>
          <Col xs={24} md={16}>
            <Text type="secondary">目标节点 / Inbound</Text>
            <Select
              style={{ width: '100%' }}
              value={form.inbound_id ? String(form.inbound_id) : undefined}
              placeholder="选择要增加客户端的节点"
              options={inbounds.map((inbound) => ({
                value: String(inbound.id),
                label: `${inbound.remark || inbound.tag || `Inbound #${inbound.id}`} · ${inbound.protocol || '-'} · ${inbound.port || '-'}`,
              }))}
              onChange={(value) => {
                const inbound = inbounds.find((item) => String(item.id) === value)
                if (!inbound) {
                  return
                }
                update({
                  inbound_id: inbound.id,
                  inbound_tag: inbound.tag || '',
                  inbound_name: inbound.remark || inbound.tag || `Inbound #${inbound.id}`,
                  protocol: inbound.protocol || form.protocol || 'vless',
                })
              }}
            />
          </Col>
          <Col xs={24} md={8}>
            <Text type="secondary">协议</Text>
            <Input value={form.protocol || '-'} readOnly />
          </Col>
        </Row>
      </Card>

      <Card className="config-section-card" bordered={false}>
        <Title level={4}>客户端账号</Title>
        <Row gutter={[16, 16]}>
          <Col xs={24} md={12}>
            <Text type="secondary">邮箱 / 标识</Text>
            <Input value={form.client.email} onChange={(event) => updateClient({ email: event.target.value })} />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">备注</Text>
            <Input value={form.client.comment} onChange={(event) => updateClient({ comment: event.target.value })} />
          </Col>
          {form.protocol === 'trojan' ? (
            <Col xs={24} md={12}>
              <Text type="secondary">密码</Text>
              <Input value={form.client.password} placeholder="留空自动生成" onChange={(event) => updateClient({ password: event.target.value })} />
            </Col>
          ) : (
            <Col xs={24} md={12}>
              <Text type="secondary">UUID</Text>
              <Input value={form.client.uuid} placeholder="留空自动生成" onChange={(event) => updateClient({ uuid: event.target.value })} />
            </Col>
          )}
          <Col xs={24} md={12}>
            <Text type="secondary">Sub ID</Text>
            <Input value={form.client.sub_id} placeholder="留空自动生成" onChange={(event) => updateClient({ sub_id: event.target.value })} />
          </Col>
          <Col xs={24} md={8}>
            <Text type="secondary">限 IP</Text>
            <InputNumber style={{ width: '100%' }} min={0} value={form.client.limit_ip} onChange={(value) => updateClient({ limit_ip: Number(value || 0) })} />
          </Col>
          <Col xs={24} md={8}>
            <Text type="secondary">总流量 GB</Text>
            <InputNumber style={{ width: '100%' }} min={0} value={form.client.total_gb} onChange={(value) => updateClient({ total_gb: Number(value || 0) })} />
          </Col>
          <Col xs={24} md={8}>
            <Text type="secondary">到期天数</Text>
            <InputNumber style={{ width: '100%' }} min={0} value={form.client.expiry_days} onChange={(value) => updateClient({ expiry_days: Number(value || 0) })} />
          </Col>
          {form.protocol !== 'trojan' ? (
            <Col xs={24} md={12}>
              <Text type="secondary">Flow</Text>
              <Input value={form.client.flow} placeholder="例如 xtls-rprx-vision，可留空" onChange={(event) => updateClient({ flow: event.target.value })} />
            </Col>
          ) : null}
          <Col xs={24} md={12}>
            <div className="switch-row">
              <span>启用此客户端</span>
              <Switch checked={form.client.enabled} onChange={(checked) => updateClient({ enabled: checked })} />
            </div>
          </Col>
        </Row>
      </Card>
    </Space>
  )
}
