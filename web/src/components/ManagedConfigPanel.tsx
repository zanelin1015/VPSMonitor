import { Alert, Button, Card, Col, Empty, Input, InputNumber, List, Row, Select, Space, Spin, Switch, Typography } from 'antd'
import { DeleteOutlined, PlusOutlined, SaveOutlined } from '@ant-design/icons'

import type { AgentEntryConfig, AgentEntryMapping, AgentListItem, ConfigAuditLog, ManagedAgentConfig, VPSRenewalConfig, XUIConfig } from '../types'
import { DEFAULT_COST_CURRENCY, type CurrencyCode } from '../lib/currency'
import { bytesToGB, gbToBytes } from '../lib/traffic'
import type { ConfigSectionKey } from '../lib/appHelpers'
import { formatDateTime, formatRenewalHint, summarizeConfigAudit } from '../lib/appHelpers'

const { Text, Title } = Typography

export interface ConfigPanelProps {
  selectedAgent?: AgentListItem
  managedConfig: ManagedAgentConfig | null
  configLoading: boolean
  configSavingSection: ConfigSectionKey | null
  configError: string
  onSave: (section: ConfigSectionKey) => void
  onAgentNameChange: (value: string) => void
  onSortOrderChange: (value: number) => void
  tagOptions: string[]
  newTagName: string
  tagSaving: boolean
  onNewTagNameChange: (value: string) => void
  onCreateTag: () => void
  onTagsChange: (values: string[]) => void
  onRenewalChange: (patch: Partial<VPSRenewalConfig>) => void
  entryAddressInputText: string
  onEntryAddressesTextChange: (value: string) => void
  onEntryChange: (patch: Partial<AgentEntryConfig>) => void
  onXUIChange: (patch: Partial<XUIConfig>) => void
  configAudits: ConfigAuditLog[]
  configAuditsLoading: boolean
  currencyOptions: CurrencyCode[]
}

export function ManagedConfigPanel(props: ConfigPanelProps) {
  const {
    selectedAgent,
    managedConfig,
    configLoading,
    configSavingSection,
    configError,
    onSave,
    onAgentNameChange,
    onSortOrderChange,
    tagOptions,
    newTagName,
    tagSaving,
    onNewTagNameChange,
    onCreateTag,
    onTagsChange,
    onRenewalChange,
    entryAddressInputText,
    onEntryAddressesTextChange,
    onEntryChange,
    onXUIChange,
    configAudits,
    configAuditsLoading,
    currencyOptions,
  } = props

  if (!selectedAgent) {
    return <Empty description="先选择一个 client" />
  }

  if (configLoading && !managedConfig) {
    return (
      <div className="empty-stage">
        <Spin size="large" />
      </div>
    )
  }

  if (!managedConfig) {
    return <Empty description="暂无托管配置" />
  }

  const entryConfig: AgentEntryConfig = {
    addresses: managedConfig.entry?.addresses || [],
    mappings: managedConfig.entry?.mappings || [],
  }
  const updateEntryMapping = (index: number, patch: Partial<AgentEntryMapping>) => {
    const mappings = (entryConfig.mappings || []).map((mapping, currentIndex) => (currentIndex === index ? { ...mapping, ...patch } : mapping))
    onEntryChange({ mappings })
  }
  const addEntryMapping = () => {
    onEntryChange({
      mappings: [
        ...(entryConfig.mappings || []),
        {
          address: entryConfig.addresses?.[0] || managedConfig.xui.base_url || '',
          external_port: 0,
          internal_port: 0,
          protocol: 'vless',
          note: '',
        },
      ],
    })
  }
  const removeEntryMapping = (index: number) => {
    onEntryChange({ mappings: (entryConfig.mappings || []).filter((_, currentIndex) => currentIndex !== index) })
  }
  const sectionSaving = Boolean(configSavingSection)
  const sectionSaveButton = (section: ConfigSectionKey, label: string) => (
    <Button
      type="primary"
      size="small"
      icon={<SaveOutlined />}
      onClick={() => onSave(section)}
      loading={configSavingSection === section}
      disabled={sectionSaving && configSavingSection !== section}
    >
      {label}
    </Button>
  )

  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      {configError ? (
        <Alert
          type="warning"
          showIcon
          message="托管配置状态"
          description={configError}
          className="compact-alert"
        />
      ) : null}

      <Alert
        type="info"
        showIcon
        message="统一配置说明"
        description="client 注册后，server 会保存并下发 x-ui 托管参数。后台修改后，不需要再改 client 本地文件，下一次轮询会自动使用新配置。"
        className="compact-alert"
      />

      <Card className="config-section-card" bordered={false}>
        <div className="section-title-row">
          <Title level={4}>Client 信息</Title>
          {sectionSaveButton('client', '保存 Client 信息')}
        </div>
        <Row gutter={[16, 16]}>
          <Col xs={24} md={8}>
            <Text type="secondary">Agent ID</Text>
            <Input value={managedConfig.agent_id || selectedAgent.agent_id} disabled />
          </Col>
          <Col xs={24} md={8}>
            <Text type="secondary">展示名称</Text>
            <Input value={managedConfig.agent_name || ''} onChange={(event) => onAgentNameChange(event.target.value)} />
          </Col>
          <Col xs={24} md={8}>
            <Text type="secondary">排序序号</Text>
            <InputNumber
              style={{ width: '100%' }}
              min={1}
              precision={0}
              value={managedConfig.sort_order || selectedAgent.sort_order || 1}
              onChange={(value) => onSortOrderChange(Number(value || selectedAgent.sort_order || 1))}
            />
          </Col>
          <Col xs={24}>
            <Text type="secondary">标签</Text>
            <Select
              mode="multiple"
              allowClear
              style={{ width: '100%' }}
              value={managedConfig.tags || []}
              placeholder="选择已创建标签"
              options={tagOptions.map((tag) => ({ value: tag, label: tag }))}
              onChange={(values) => onTagsChange(values)}
            />
            <Space.Compact style={{ width: '100%', marginTop: 8 }}>
              <Input
                value={newTagName}
                placeholder="创建固定标签，例如 PH、家宽、NAT"
                onChange={(event) => onNewTagNameChange(event.target.value)}
                onPressEnter={onCreateTag}
              />
              <Button onClick={onCreateTag} loading={tagSaving}>创建标签</Button>
            </Space.Compact>
            <Text type="secondary">标签先创建再多选；保存 Client 信息后会应用到当前 Client。</Text>
          </Col>
        </Row>
      </Card>

      <Card className="config-section-card" bordered={false}>
        <div className="section-title-row">
          <Title level={4}>VPS 信息</Title>
          {sectionSaveButton('renewal', '保存 VPS 信息')}
        </div>
        <Row gutter={[16, 16]}>
          <Col xs={24} md={8}>
            <div className="switch-row">
              <span>启用自动计算</span>
              <Switch checked={Boolean(managedConfig.renewal?.enabled)} onChange={(checked) => onRenewalChange({ enabled: checked })} />
            </div>
          </Col>
          <Col xs={24} md={8}>
            <div className="switch-row">
              <span>周期到期后自动刷新</span>
              <Switch checked={Boolean(managedConfig.renewal?.auto_renew)} onChange={(checked) => onRenewalChange({ auto_renew: checked })} />
            </div>
          </Col>
          <Col xs={24} md={8}>
            <Text type="secondary">到期时间</Text>
            <Input
              type="date"
              value={managedConfig.renewal?.expire_date || ''}
              onChange={(event) => onRenewalChange({ enabled: true, expire_date: event.target.value })}
            />
          </Col>
          <Col xs={24} md={8}>
            <Text type="secondary">周期开始时间</Text>
            <Input
              type="date"
              value={managedConfig.renewal?.start_date || ''}
              onChange={(event) => onRenewalChange({ enabled: true, start_date: event.target.value })}
            />
          </Col>
          <Col xs={24} md={8}>
            <Text type="secondary">续费周期</Text>
            <Select
              style={{ width: '100%' }}
              value={managedConfig.renewal?.cycle || 'month'}
              options={[
                { value: 'week', label: '每周' },
                { value: 'month', label: '每月' },
                { value: 'quarter', label: '每季' },
                { value: 'year', label: '每年' },
              ]}
              onChange={(value) => onRenewalChange({ cycle: value })}
            />
          </Col>
          <Col xs={24} md={8}>
            <Text type="secondary">费用金额</Text>
            <InputNumber
              style={{ width: '100%' }}
              min={0}
              precision={2}
              value={managedConfig.renewal?.cost_amount || 0}
              onChange={(value) => onRenewalChange({ cost_amount: Number(value || 0) })}
              placeholder="例如 8.99"
            />
          </Col>
          <Col xs={24} md={8}>
            <Text type="secondary">费用币种</Text>
            <Select
              style={{ width: '100%' }}
              value={managedConfig.renewal?.cost_currency || DEFAULT_COST_CURRENCY}
              options={currencyOptions.map((currency) => ({ value: currency, label: currency }))}
              onChange={(value) => onRenewalChange({ cost_currency: value })}
            />
          </Col>
          <Col xs={24} md={8}>
            <Text type="secondary">费用续费周期</Text>
            <Select
              style={{ width: '100%' }}
              value={managedConfig.renewal?.cost_cycle || 'month'}
              options={[
                { value: 'month', label: '每月' },
                { value: 'quarter', label: '每季' },
                { value: 'year', label: '每年' },
              ]}
              onChange={(value) => onRenewalChange({ cost_cycle: value })}
            />
          </Col>
          <Col xs={24} md={8}>
            <Text type="secondary">周期总流量 (GB)</Text>
            <InputNumber
              style={{ width: '100%' }}
              min={0}
              precision={2}
              value={bytesToGB(managedConfig.renewal?.traffic_limit_bytes || 0)}
              onChange={(value) => onRenewalChange({ traffic_limit_bytes: gbToBytes(Number(value || 0)) })}
              placeholder="例如 1024"
            />
          </Col>
          <Col xs={24} md={8}>
            <Text type="secondary">带宽大小 (Mbps)</Text>
            <InputNumber
              style={{ width: '100%' }}
              min={0}
              precision={2}
              value={managedConfig.renewal?.bandwidth_mbps || 0}
              onChange={(value) => onRenewalChange({ bandwidth_mbps: Number(value || 0) })}
              placeholder="例如 1000"
            />
          </Col>
          <Col xs={24}>
            <Text type="secondary">{formatRenewalHint(managedConfig.renewal)}</Text>
          </Col>
        </Row>
      </Card>

      <Card className="config-section-card" bordered={false}>
        <div className="section-title-row">
          <Title level={4}>X-UI 托管配置</Title>
          {sectionSaveButton('xui', '保存 X-UI 配置')}
        </div>
        <Row gutter={[16, 16]}>
          <Col xs={24} md={12}>
            <div className="switch-row">
              <span>启用 x-ui 采集</span>
              <Switch checked={managedConfig.xui.enabled} onChange={(checked) => onXUIChange({ enabled: checked })} />
            </div>
          </Col>
          <Col xs={24} md={12}>
            <div className="switch-row">
              <span>跳过 TLS 校验</span>
              <Switch checked={managedConfig.xui.skip_tls_verify} onChange={(checked) => onXUIChange({ skip_tls_verify: checked })} />
            </div>
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">Base URL</Text>
            <Input value={managedConfig.xui.base_url || ''} placeholder="https://127.0.0.1:2053" onChange={(event) => onXUIChange({ base_url: event.target.value })} />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">用户名</Text>
            <Input value={managedConfig.xui.username || ''} onChange={(event) => onXUIChange({ username: event.target.value })} />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">密码</Text>
            <Input.Password value={managedConfig.xui.password || ''} onChange={(event) => onXUIChange({ password: event.target.value })} />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">二步验证码</Text>
            <Input value={managedConfig.xui.two_factor_code || ''} onChange={(event) => onXUIChange({ two_factor_code: event.target.value })} />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">节点维护方式</Text>
            <Input value="节点请直接在 x-ui 前端手动维护；中心只负责出站与转发编排" disabled />
          </Col>
        </Row>
      </Card>

      <Card className="config-section-card" bordered={false}>
        <div className="section-title-row">
          <Title level={4}>入口地址 / NAT 映射</Title>
          <Space wrap>
            <Button size="small" icon={<PlusOutlined />} onClick={addEntryMapping}>
              添加映射
            </Button>
            {sectionSaveButton('entry', '保存入口/NAT')}
          </Space>
        </div>
        <Alert
          type="info"
          showIcon
          className="compact-alert"
          message="用于 NAT/家宽落地匹配"
          description="当转发配置里填写的是连接 IP/域名，但 Client 查询到的公网 IP 不同，可以在这里配置入口地址和外部端口到内部节点端口的映射。拓扑会优先按入口地址 + 外部端口 + 节点类型匹配。"
        />
        <Row gutter={[16, 16]}>
          <Col xs={24}>
            <Text type="secondary">入口地址</Text>
            <Input.TextArea
              value={entryAddressInputText}
              autoSize={{ minRows: 2, maxRows: 5 }}
              placeholder="每行一个入口域名/IP，例如 att.kynbbz.top 或 1.2.3.4"
              onChange={(event) => onEntryAddressesTextChange(event.target.value)}
            />
            <Text type="secondary">这些地址会加入该 Client 的可匹配入口；映射可以进一步指定端口转换。</Text>
          </Col>
        </Row>
        <Space direction="vertical" size="small" className="entry-mapping-list">
          {(entryConfig.mappings || []).length ? (
            (entryConfig.mappings || []).map((mapping, index) => (
              <div key={`entry-mapping-${index}`} className="entry-mapping-row">
                <Input
                  value={mapping.address || ''}
                  placeholder="入口域名/IP"
                  onChange={(event) => updateEntryMapping(index, { address: event.target.value })}
                />
                <InputNumber
                  min={0}
                  precision={0}
                  value={mapping.external_port || 0}
                  placeholder="外部端口"
                  onChange={(value) => updateEntryMapping(index, { external_port: Number(value || 0) })}
                />
                <InputNumber
                  min={0}
                  precision={0}
                  value={mapping.internal_port || 0}
                  placeholder="内部端口"
                  onChange={(value) => updateEntryMapping(index, { internal_port: Number(value || 0) })}
                />
                <Select
                  value={mapping.protocol || 'vless'}
                  options={[
                    { value: 'vless', label: 'VLESS' },
                    { value: 'vmess', label: 'VMess' },
                    { value: 'http', label: 'HTTP' },
                    { value: 'socks', label: 'Socks' },
                  ]}
                  onChange={(value) => updateEntryMapping(index, { protocol: value })}
                />
                <Input
                  value={mapping.note || ''}
                  placeholder="备注"
                  onChange={(event) => updateEntryMapping(index, { note: event.target.value })}
                />
                <Button danger icon={<DeleteOutlined />} onClick={() => removeEntryMapping(index)}>
                  删除
                </Button>
              </div>
            ))
          ) : (
            <Empty description="暂无端口映射；如果连接 IP 与出口 IP 不同，建议添加一条 NAT 映射" />
          )}
        </Space>
      </Card>

      <Card className="config-section-card" bordered={false}>
        <Title level={4}>配置修改记录</Title>
        <Spin spinning={configAuditsLoading}>
          {configAudits.length ? (
            <List
              dataSource={configAudits}
              renderItem={(item) => (
                <List.Item>
                  <div>
                    <Text strong>{item.actor || 'system'}</Text>
                    <div className="muted-line">
                      {formatDateTime(item.created_at)} · {summarizeConfigAudit(item)}
                    </div>
                  </div>
                </List.Item>
              )}
            />
          ) : (
            <Empty description="暂无配置修改记录" />
          )}
        </Spin>
      </Card>

    </Space>
  )
}

