import { Alert, Button, Card, Col, Input, InputNumber, List, Row, Select, Space, Switch, Typography } from 'antd'
import { DeleteOutlined, PlusOutlined, SaveOutlined } from '@ant-design/icons'
import type { AgentEntryConfig, DashboardAgentView, NetworkPortPolicyRule } from '../types'

const { Text, Title } = Typography

type NetworkPolicy = NonNullable<AgentEntryConfig['network_policy']>

interface NetworkPolicyConfigSectionProps {
  visible: boolean
  networkPolicy: NetworkPolicy
  observedNetworkPolicy?: DashboardAgentView['network_policy']
  usingObservedNetworkPolicy: boolean
  configSavingSection: string | null
  sectionSaving: boolean
  onAddRule: () => void
  onChange: (patch: Partial<NetworkPolicy>) => void
  onSave: () => void
  onRemoveRule: (index: number) => void
  onUpdateRule: (index: number, patch: Partial<NetworkPortPolicyRule>) => void
}

export function NetworkPolicyConfigSection(props: NetworkPolicyConfigSectionProps) {
  const { visible, networkPolicy, observedNetworkPolicy, usingObservedNetworkPolicy, configSavingSection, sectionSaving, onAddRule, onChange, onSave, onRemoveRule, onUpdateRule } = props
  return (
      <Card className="config-section-card" bordered={false} style={visible ? undefined : { display: 'none' }}>
        <div className="section-title-row">
          <Title level={4}>端口限速 / IP 白名单</Title>
          <Space wrap>
            <Button size="small" icon={<PlusOutlined />} onClick={onAddRule}>
              添加端口规则
            </Button>
            <Button
              type="primary"
              size="small"
              icon={<SaveOutlined />}
              onClick={onSave}
              loading={configSavingSection === 'entry'}
              disabled={sectionSaving && configSavingSection !== 'entry'}
            >
              保存端口策略
            </Button>
          </Space>
        </div>
        <Alert
          type="warning"
          showIcon
          className="compact-alert"
          message="由 Client 在 VPS 本机执行"
          description="IP 白名单会优先在 Debian/Ubuntu 使用 ufw，其他系统自动回退 iptables；端口限速使用 Linux tc，对服务端出方向按源端口限速。启用前请确认防火墙不会锁死 SSH。"
        />
        {observedNetworkPolicy?.error ? (
          <Alert type="warning" showIcon className="compact-alert" message="读取当前端口策略失败" description={observedNetworkPolicy.error} />
        ) : null}
        {usingObservedNetworkPolicy ? (
          <Alert
            type="info"
            showIcon
            className="compact-alert"
            message="已读取当前系统端口策略作为可编辑配置"
            description="可读取 VPS 当前 ufw 白名单和 tc 限速；修改下方端口策略并保存后，Client 会按托管配置重建对应规则。"
          />
        ) : null}
        <Row gutter={[16, 16]}>
          <Col xs={24} md={6}>
            <div className="switch-row">
              <span>启用端口策略</span>
              <Switch checked={Boolean(networkPolicy.enabled)} onChange={(checked) => onChange({ enabled: checked })} />
            </div>
          </Col>
          <Col xs={24} md={6}>
            <Text type="secondary">网卡</Text>
            <Input value={networkPolicy.interface || ''} placeholder="留空自动识别，例如 eth0" onChange={(event) => onChange({ interface: event.target.value })} />
          </Col>
          <Col xs={24} md={6}>
            <Text type="secondary">白名单后端</Text>
            <Select
              style={{ width: '100%' }}
              value={networkPolicy.firewall_backend || 'auto'}
              options={[
                { value: 'auto', label: '自动：ufw/iptables' },
                { value: 'ufw', label: 'ufw' },
                { value: 'iptables', label: 'iptables' },
                { value: 'none', label: '不处理白名单' },
              ]}
              onChange={(value) => onChange({ firewall_backend: value })}
            />
          </Col>
          <Col xs={24} md={6}>
            <Text type="secondary">限速后端</Text>
            <Select
              style={{ width: '100%' }}
              value={networkPolicy.rate_limit_backend || 'auto'}
              options={[
                { value: 'auto', label: '自动：tc' },
                { value: 'tc', label: 'tc' },
                { value: 'none', label: '不处理限速' },
              ]}
              onChange={(value) => onChange({ rate_limit_backend: value })}
            />
          </Col>
          <Col xs={24}>
            <List
              locale={{ emptyText: '暂无端口策略' }}
              dataSource={networkPolicy.rules || []}
              renderItem={(rule, index) => (
                <List.Item
                  actions={[
                    <Button key="delete" size="small" danger icon={<DeleteOutlined />} onClick={() => onRemoveRule(index)}>
                      删除
                    </Button>,
                  ]}
                >
                  <Row gutter={[12, 12]} style={{ width: '100%' }}>
                    <Col xs={24} md={4}>
                      <Text type="secondary">启用</Text>
                      <Switch checked={rule.enabled !== false} onChange={(checked) => onUpdateRule(index, { enabled: checked })} />
                    </Col>
                    <Col xs={24} md={5}>
                      <Text type="secondary">名称</Text>
                      <Input value={rule.name || ''} placeholder="默认使用端口号，可自定义" onChange={(event) => onUpdateRule(index, { name: event.target.value })} />
                    </Col>
                    <Col xs={12} md={4}>
                      <Text type="secondary">端口</Text>
                      <InputNumber style={{ width: '100%' }} min={1} max={65535} precision={0} value={rule.port || 0} onChange={(value) => onUpdateRule(index, { port: Number(value || 0) })} />
                    </Col>
                    <Col xs={12} md={4}>
                      <Text type="secondary">协议</Text>
                      <Select
                        style={{ width: '100%' }}
                        value={rule.protocol || 'tcp'}
                        options={[
                          { value: 'tcp', label: 'TCP' },
                          { value: 'udp', label: 'UDP' },
                          { value: 'both', label: 'TCP + UDP' },
                        ]}
                        onChange={(value) => onUpdateRule(index, { protocol: value })}
                      />
                    </Col>
                    <Col xs={24} md={7}>
                      <Text type="secondary">限速 Mbps（0 不限速）</Text>
                      <InputNumber style={{ width: '100%' }} min={0} precision={2} value={rule.rate_limit_mbps || 0} onChange={(value) => onUpdateRule(index, { rate_limit_mbps: Number(value || 0) })} />
                    </Col>
                    <Col xs={24}>
                      <Text type="secondary">IP 白名单 / CIDR（一行一个；为空不限制）</Text>
                      <Input.TextArea
                        value={(rule.whitelist_ips || []).join('\n')}
                        autoSize={{ minRows: 2, maxRows: 5 }}
                        placeholder={'1.2.3.4\n10.0.0.0/24'}
                        onChange={(event) => onUpdateRule(index, { whitelist_ips: event.target.value.split(/\r?\n|,/).map((item) => item.trim()).filter(Boolean) })}
                      />
                    </Col>
                  </Row>
                </List.Item>
              )}
            />
          </Col>
        </Row>
      </Card>

  )
}
