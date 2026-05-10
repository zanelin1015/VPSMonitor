import { Button, Card, Col, Input, InputNumber, Row, Select, Space, Switch, Tag, Typography } from 'antd'
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons'

import type { XUILocalCertificate } from '../types'
import type { TLSCertificateSelectionForm, XUIInboundActionForm, XUIInboundClientForm } from '../lib/appHelpers'
import { defaultInboundClientForm } from '../lib/appHelpers'

const { Text, Title } = Typography

export function renderInboundActionForm(props: {
  form: XUIInboundActionForm
  certificates: XUILocalCertificate[]
  onChange: (form: XUIInboundActionForm) => void
}) {
  const { form, certificates, onChange } = props
  const update = (patch: Partial<XUIInboundActionForm>) => onChange({ ...form, ...patch })
  const updateTLS = (patch: Partial<TLSCertificateSelectionForm>) => update({ tls: { ...form.tls, ...patch } })
  const updateClient = (index: number, patch: Partial<XUIInboundClientForm>) =>
    update({
      clients: form.clients.map((client, currentIndex) => (currentIndex === index ? { ...client, ...patch } : client)),
    })

  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Card className="config-section-card" bordered={false}>
        <Title level={4}>入站基础配置</Title>
        <Row gutter={[16, 16]}>
          <Col xs={24} md={12}>
            <Text type="secondary">备注</Text>
            <Input value={form.remark} onChange={(event) => update({ remark: event.target.value })} />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">标签</Text>
            <Input
              placeholder="留空会自动生成"
              value={form.tag}
              onChange={(event) => update({ tag: event.target.value })}
            />
          </Col>
          <Col xs={24} md={8}>
            <Text type="secondary">协议</Text>
            <Select
              style={{ width: '100%' }}
              value={form.protocol}
              options={[
                { value: 'vless', label: 'VLESS' },
                { value: 'vmess', label: 'VMESS' },
                { value: 'trojan', label: 'Trojan' },
              ]}
              onChange={(value) => update({ protocol: value })}
            />
          </Col>
          <Col xs={24} md={8}>
            <Text type="secondary">监听端口</Text>
            <InputNumber style={{ width: '100%' }} min={1} max={65535} value={form.port} onChange={(value) => update({ port: Number(value || 0) })} />
          </Col>
          <Col xs={24} md={8}>
            <Text type="secondary">监听地址</Text>
            <Input value={form.listen} placeholder="默认空" onChange={(event) => update({ listen: event.target.value })} />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">传输层</Text>
            <Select
              style={{ width: '100%' }}
              value={form.transport}
              options={[
                { value: 'tcp', label: 'TCP' },
                { value: 'ws', label: 'WebSocket' },
              ]}
              onChange={(value) => update({ transport: value })}
            />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">安全</Text>
            <Select
              style={{ width: '100%' }}
              value={form.security}
              options={[
                { value: 'none', label: 'None' },
                { value: 'tls', label: 'TLS' },
              ]}
              onChange={(value) => update({ security: value })}
            />
          </Col>
          {form.transport === 'ws' ? (
            <>
              <Col xs={24} md={12}>
                <Text type="secondary">WS Path</Text>
                <Input value={form.ws_path} onChange={(event) => update({ ws_path: event.target.value })} />
              </Col>
              <Col xs={24} md={12}>
                <Text type="secondary">WS Host</Text>
                <Input value={form.ws_host} onChange={(event) => update({ ws_host: event.target.value })} />
              </Col>
            </>
          ) : null}
          <Col xs={24} md={12}>
            <div className="switch-row">
              <span>启用入站</span>
              <Switch checked={form.enabled} onChange={(checked) => update({ enabled: checked })} />
            </div>
          </Col>
          <Col xs={24} md={12}>
            <div className="switch-row">
              <span>提交后重启 Xray</span>
              <Tag color="success">自动执行</Tag>
            </div>
          </Col>
          <Col xs={24}>
            <div className="switch-row">
              <span>启用 Sniffing</span>
              <Switch checked={form.sniffing} onChange={(checked) => update({ sniffing: checked })} />
            </div>
          </Col>
        </Row>
      </Card>

      {form.security === 'tls' ? (
        <Card className="config-section-card" bordered={false}>
          <Title level={4}>TLS / SSL 证书</Title>
          <Row gutter={[16, 16]}>
            <Col xs={24} md={12}>
              <Text type="secondary">Server Name</Text>
              <Input
                value={form.server_name}
                placeholder="例如 hk.example.test"
                onChange={(event) => update({ server_name: event.target.value })}
              />
            </Col>
            <Col xs={24} md={12}>
              <Text type="secondary">证书来源</Text>
              <Select
                style={{ width: '100%' }}
                value={form.tls.mode}
                options={[
                  { value: 'none', label: '暂不注入证书' },
                  { value: 'domain_auto', label: '按域名自动匹配 client 本机证书' },
                  { value: 'inventory', label: '从 client 已发现证书中指定' },
                  { value: 'manual', label: '手动填写证书路径' },
                ]}
                onChange={(value: TLSCertificateSelectionForm['mode']) => updateTLS({ mode: value })}
              />
            </Col>
            {form.tls.mode === 'domain_auto' ? (
              <Col xs={24}>
                <Text type="secondary">自动匹配域名</Text>
                <Input
                  value={form.tls.domain}
                  placeholder="留空时使用上面的 Server Name"
                  onChange={(event) => updateTLS({ domain: event.target.value })}
                />
              </Col>
            ) : null}
            {form.tls.mode === 'inventory' ? (
              <Col xs={24}>
                <Text type="secondary">选择本机证书</Text>
                <Select
                  style={{ width: '100%' }}
                  value={form.tls.inventory_id || undefined}
                  placeholder={certificates.length ? '选择 client 已上报的证书' : '当前 client 暂无已上报证书'}
                  options={certificates.map((certificate) => ({
                    value: certificate.id,
                    label: `${certificate.name || certificate.subject || certificate.id} · ${certificate.cert_path || '-'}`,
                  }))}
                  onChange={(value) => updateTLS({ inventory_id: value })}
                />
              </Col>
            ) : null}
            {form.tls.mode === 'manual' ? (
              <>
                <Col xs={24}>
                  <Text type="secondary">证书文件路径</Text>
                  <Input
                    value={form.tls.certificate_file}
                    placeholder="/etc/letsencrypt/live/example/fullchain.pem"
                    onChange={(event) => updateTLS({ certificate_file: event.target.value })}
                  />
                </Col>
                <Col xs={24}>
                  <Text type="secondary">私钥文件路径</Text>
                  <Input
                    value={form.tls.key_file}
                    placeholder="/etc/letsencrypt/live/example/privkey.pem"
                    onChange={(event) => updateTLS({ key_file: event.target.value })}
                  />
                </Col>
              </>
            ) : null}
          </Row>
        </Card>
      ) : null}

      <Card className="config-section-card" bordered={false}>
        <Space style={{ width: '100%', justifyContent: 'space-between' }}>
          <Title level={4}>客户端账号</Title>
          <Button
            icon={<PlusOutlined />}
            onClick={() => update({ clients: [...form.clients, defaultInboundClientForm()] })}
          >
            新增客户端
          </Button>
        </Space>
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
          {form.clients.map((client, index) => (
            <Card key={`client-${index}`} className="config-section-card" bordered={false}>
              <Space style={{ width: '100%', justifyContent: 'space-between' }}>
                <Text strong>客户端 #{index + 1}</Text>
                <Button
                  danger
                  icon={<DeleteOutlined />}
                  disabled={form.clients.length === 1}
                  onClick={() => update({ clients: form.clients.filter((_, currentIndex) => currentIndex !== index) })}
                >
                  删除
                </Button>
              </Space>
              <Row gutter={[16, 16]}>
                <Col xs={24} md={12}>
                  <Text type="secondary">邮箱 / 标识</Text>
                  <Input value={client.email} onChange={(event) => updateClient(index, { email: event.target.value })} />
                </Col>
                <Col xs={24} md={12}>
                  <Text type="secondary">备注</Text>
                  <Input value={client.comment} onChange={(event) => updateClient(index, { comment: event.target.value })} />
                </Col>
                {form.protocol === 'trojan' ? (
                  <Col xs={24} md={12}>
                    <Text type="secondary">密码</Text>
                    <Input value={client.password} onChange={(event) => updateClient(index, { password: event.target.value })} />
                  </Col>
                ) : (
                  <Col xs={24} md={12}>
                    <Text type="secondary">UUID</Text>
                    <Input value={client.uuid} onChange={(event) => updateClient(index, { uuid: event.target.value })} />
                  </Col>
                )}
                <Col xs={24} md={12}>
                  <Text type="secondary">Sub ID</Text>
                  <Input value={client.sub_id} onChange={(event) => updateClient(index, { sub_id: event.target.value })} />
                </Col>
                <Col xs={24} md={8}>
                  <Text type="secondary">限 IP</Text>
                  <InputNumber style={{ width: '100%' }} min={0} value={client.limit_ip} onChange={(value) => updateClient(index, { limit_ip: Number(value || 0) })} />
                </Col>
                <Col xs={24} md={8}>
                  <Text type="secondary">总流量 GB</Text>
                  <InputNumber style={{ width: '100%' }} min={0} value={client.total_gb} onChange={(value) => updateClient(index, { total_gb: Number(value || 0) })} />
                </Col>
                <Col xs={24} md={8}>
                  <Text type="secondary">到期天数</Text>
                  <InputNumber style={{ width: '100%' }} min={0} value={client.expiry_days} onChange={(value) => updateClient(index, { expiry_days: Number(value || 0) })} />
                </Col>
                {form.protocol !== 'trojan' ? (
                  <Col xs={24} md={12}>
                    <Text type="secondary">Flow</Text>
                    <Input value={client.flow} onChange={(event) => updateClient(index, { flow: event.target.value })} />
                  </Col>
                ) : null}
                <Col xs={24} md={12}>
                  <div className="switch-row">
                    <span>启用此客户端</span>
                    <Switch checked={client.enabled} onChange={(checked) => updateClient(index, { enabled: checked })} />
                  </div>
                </Col>
              </Row>
            </Card>
          ))}
        </Space>
      </Card>
    </Space>
  )
}
