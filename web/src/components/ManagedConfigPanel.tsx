import { Alert, AutoComplete, Button, Card, Col, Empty, Input, InputNumber, List, Row, Select, Space, Spin, Switch, Typography } from 'antd'
import { DeleteOutlined, PlusOutlined, SaveOutlined } from '@ant-design/icons'

import type { AgentEntryConfig, AgentEntryMapping, AgentListItem, ConfigAuditLog, ManagedAgentConfig, NetworkPortPolicyRule, RealmForwardRule, VPSRenewalConfig, XUIConfig, XUILocalCertificate } from '../types'
import { DEFAULT_COST_CURRENCY, type CurrencyCode } from '../lib/currency'
import { bytesToGB, gbToBytes } from '../lib/traffic'
import type { ConfigSectionKey } from '../lib/appHelpers'
import { formatDateTime, formatRenewalHint, summarizeConfigAudit } from '../lib/appHelpers'

const { Text, Title } = Typography

export interface ConfigPanelProps {
  selectedAgent?: AgentListItem
  agents?: AgentListItem[]
  managedConfig: ManagedAgentConfig | null
  certificates: XUILocalCertificate[]
  configLoading: boolean
  configSavingSection: ConfigSectionKey | null
  configError: string
  onSave: (section: ConfigSectionKey) => void
  onAgentNameChange: (value: string) => void
  onCustomerDisplayNameChange: (value: string) => void
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
    agents = [],
    managedConfig,
    certificates,
    configLoading,
    configSavingSection,
    configError,
    onSave,
    onAgentNameChange,
    onCustomerDisplayNameChange,
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
    import_domain: managedConfig.entry?.import_domain || '',
    mappings: managedConfig.entry?.mappings || [],
    network_policy: managedConfig.entry?.network_policy || { enabled: false, interface: '', firewall_backend: 'auto', rate_limit_backend: 'auto', rules: [] },
    port_forwarding: managedConfig.entry?.port_forwarding || selectedAgent.entry?.port_forwarding || { enabled: false, backend: 'realm', binary_path: '', config_path: '', service_name: '', log_level: 'info', rules: [] },
  }
  const networkPolicy = entryConfig.network_policy || { enabled: false, interface: '', firewall_backend: 'auto', rate_limit_backend: 'auto', rules: [] }
  const portForwarding = entryConfig.port_forwarding || { enabled: false, backend: 'realm', binary_path: '', config_path: '', service_name: '', log_level: 'info', rules: [] }
  const targetAgentOptions = agents
    .filter((agent) => agent.agent_id && agent.agent_id !== selectedAgent.agent_id)
    .map((agent) => ({ value: agent.agent_id, label: `${agent.agent_name || agent.agent_id} · ${bestAgentAddress(agent) || '未上报 IP'}` }))
  const domainOptions = buildCertificateDomainOptions(certificates)
  const defaultImportDomain = domainOptions.find((option) => !option.value.startsWith('*.'))?.value || ''
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
  const updateNetworkPolicy = (patch: Partial<NonNullable<AgentEntryConfig['network_policy']>>) => {
    onEntryChange({ network_policy: { ...networkPolicy, ...patch } })
  }
  const updateNetworkPolicyRule = (index: number, patch: Partial<NetworkPortPolicyRule>) => {
    const rules = (networkPolicy.rules || []).map((rule, currentIndex) => (currentIndex === index ? { ...rule, ...patch } : rule))
    updateNetworkPolicy({ rules })
  }
  const addNetworkPolicyRule = () => {
    updateNetworkPolicy({
      enabled: true,
      rules: [
        ...(networkPolicy.rules || []),
        { id: `rule-${Date.now()}`, enabled: true, name: '', port: 443, protocol: 'tcp', rate_limit_mbps: 0, whitelist_ips: [] },
      ],
    })
  }
  const removeNetworkPolicyRule = (index: number) => {
    updateNetworkPolicy({ rules: (networkPolicy.rules || []).filter((_, currentIndex) => currentIndex !== index) })
  }
  const updatePortForwarding = (patch: Partial<NonNullable<AgentEntryConfig['port_forwarding']>>) => {
    onEntryChange({ port_forwarding: { ...portForwarding, ...patch } })
  }
  const updatePortForwardRule = (index: number, patch: Partial<RealmForwardRule>) => {
    const rules = (portForwarding.rules || []).map((rule, currentIndex) => (currentIndex === index ? { ...rule, ...patch } : rule))
    updatePortForwarding({ rules })
  }
  const addPortForwardRule = () => {
    updatePortForwarding({
      enabled: true,
      backend: 'realm',
      rules: [
        ...(portForwarding.rules || []),
        {
          id: `realm-${Date.now()}`,
          enabled: true,
          name: '',
          listen_address: '0.0.0.0',
          listen_port: 443,
          target_agent_id: '',
          target_address: '',
          target_port: 443,
          network: 'both',
          note: '',
        },
      ],
    })
  }
  const removePortForwardRule = (index: number) => {
    updatePortForwarding({ rules: (portForwarding.rules || []).filter((_, currentIndex) => currentIndex !== index) })
  }
  const selectPortForwardTargetAgent = (index: number, agentID: string) => {
    const agent = agents.find((item) => item.agent_id === agentID)
    updatePortForwardRule(index, {
      target_agent_id: agentID,
      target_address: bestAgentAddress(agent) || '',
    })
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
          <Col xs={24} md={6}>
            <Text type="secondary">Agent ID</Text>
            <Input value={managedConfig.agent_id || selectedAgent.agent_id} disabled />
          </Col>
          <Col xs={24} md={6}>
            <Text type="secondary">展示名称</Text>
            <Input value={managedConfig.agent_name || ''} onChange={(event) => onAgentNameChange(event.target.value)} />
          </Col>
          <Col xs={24} md={6}>
            <Text type="secondary">用户展示名称</Text>
            <Input
              value={managedConfig.customer_display_name || ''}
              placeholder="用户页面只显示这个名称"
              onChange={(event) => onCustomerDisplayNameChange(event.target.value)}
            />
          </Col>
          <Col xs={24} md={6}>
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
            <Input value={managedConfig.xui.base_url || ''} placeholder="用于打开面板/执行操作，例如 https://127.0.0.1:2053/secret/" onChange={(event) => onXUIChange({ base_url: event.target.value })} />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">本地 x-ui 数据库路径</Text>
            <Input value={managedConfig.xui.db_path || ''} placeholder="默认自动读取 /etc/x-ui/x-ui.db" onChange={(event) => onXUIChange({ db_path: event.target.value })} />
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
            <Text type="secondary">API Token</Text>
            <Input.Password
              value={managedConfig.xui.api_token || ''}
              placeholder="新版 3x-ui：Settings → Security → API Token"
              onChange={(event) => onXUIChange({ api_token: event.target.value })}
            />
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
          <Col xs={24} md={12}>
            <Text type="secondary">VPS 主域名 / 导入链接域名</Text>
            <AutoComplete
              allowClear
              value={entryConfig.import_domain || ''}
              options={domainOptions}
              placeholder={defaultImportDomain ? `可选择证书域名：${defaultImportDomain}` : '无域名证书也可以手动输入域名'}
              onChange={(value) => onEntryChange({ import_domain: value })}
              filterOption={(inputValue, option) => String(option?.value || '').toLowerCase().includes(inputValue.toLowerCase())}
              style={{ width: '100%' }}
            />
            <Text type="secondary">
              {entryConfig.import_domain
                ? '生成 VLESS/VMess/Trojan/SS 等导入链接时固定使用该域名；Realm 转发导出时也使用入口 VPS 的主域名和监听端口。'
                : defaultImportDomain
                  ? '未填写时后端会自动使用第一个可连接的域名证书；建议显式选择主域名，避免多证书时选错。'
                  : '未填写且没有域名证书时会回退到入口地址或公网 IP；需要固定域名时请手动输入后保存。'}
            </Text>
          </Col>
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
        <div className="section-title-row">
          <Title level={4}>端口限速 / IP 白名单</Title>
          <Space wrap>
            <Button size="small" icon={<PlusOutlined />} onClick={addNetworkPolicyRule}>
              添加端口规则
            </Button>
            {sectionSaveButton('entry', '保存端口策略')}
          </Space>
        </div>
        <Alert
          type="warning"
          showIcon
          className="compact-alert"
          message="由 Client 在 VPS 本机执行"
          description="IP 白名单会优先在 Debian/Ubuntu 使用 ufw，其他系统自动回退 iptables；端口限速使用 Linux tc，对服务端出方向按源端口限速。启用前请确认防火墙不会锁死 SSH。"
        />
        <Row gutter={[16, 16]}>
          <Col xs={24} md={6}>
            <div className="switch-row">
              <span>启用端口策略</span>
              <Switch checked={Boolean(networkPolicy.enabled)} onChange={(checked) => updateNetworkPolicy({ enabled: checked })} />
            </div>
          </Col>
          <Col xs={24} md={6}>
            <Text type="secondary">网卡</Text>
            <Input value={networkPolicy.interface || ''} placeholder="留空自动识别，例如 eth0" onChange={(event) => updateNetworkPolicy({ interface: event.target.value })} />
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
              onChange={(value) => updateNetworkPolicy({ firewall_backend: value })}
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
              onChange={(value) => updateNetworkPolicy({ rate_limit_backend: value })}
            />
          </Col>
          <Col xs={24}>
            <List
              locale={{ emptyText: '暂无端口策略' }}
              dataSource={networkPolicy.rules || []}
              renderItem={(rule, index) => (
                <List.Item
                  actions={[
                    <Button key="delete" size="small" danger icon={<DeleteOutlined />} onClick={() => removeNetworkPolicyRule(index)}>
                      删除
                    </Button>,
                  ]}
                >
                  <Row gutter={[12, 12]} style={{ width: '100%' }}>
                    <Col xs={24} md={4}>
                      <Text type="secondary">启用</Text>
                      <Switch checked={rule.enabled !== false} onChange={(checked) => updateNetworkPolicyRule(index, { enabled: checked })} />
                    </Col>
                    <Col xs={24} md={5}>
                      <Text type="secondary">名称</Text>
                      <Input value={rule.name || ''} placeholder="例如 HK 入站" onChange={(event) => updateNetworkPolicyRule(index, { name: event.target.value })} />
                    </Col>
                    <Col xs={12} md={4}>
                      <Text type="secondary">端口</Text>
                      <InputNumber style={{ width: '100%' }} min={1} max={65535} precision={0} value={rule.port || 0} onChange={(value) => updateNetworkPolicyRule(index, { port: Number(value || 0) })} />
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
                        onChange={(value) => updateNetworkPolicyRule(index, { protocol: value })}
                      />
                    </Col>
                    <Col xs={24} md={7}>
                      <Text type="secondary">限速 Mbps（0 不限速）</Text>
                      <InputNumber style={{ width: '100%' }} min={0} precision={2} value={rule.rate_limit_mbps || 0} onChange={(value) => updateNetworkPolicyRule(index, { rate_limit_mbps: Number(value || 0) })} />
                    </Col>
                    <Col xs={24}>
                      <Text type="secondary">IP 白名单 / CIDR（一行一个；为空不限制）</Text>
                      <Input.TextArea
                        value={(rule.whitelist_ips || []).join('\n')}
                        autoSize={{ minRows: 2, maxRows: 5 }}
                        placeholder={'1.2.3.4\n10.0.0.0/24'}
                        onChange={(event) => updateNetworkPolicyRule(index, { whitelist_ips: event.target.value.split(/\r?\n|,/).map((item) => item.trim()).filter(Boolean) })}
                      />
                    </Col>
                  </Row>
                </List.Item>
              )}
            />
          </Col>
        </Row>
      </Card>

      <Card className="config-section-card" bordered={false}>
        <div className="section-title-row">
          <Title level={4}>Realm 端口转发</Title>
          <Space wrap>
            <Button size="small" icon={<PlusOutlined />} onClick={addPortForwardRule}>
              添加转发
            </Button>
            {sectionSaveButton('entry', '保存 Realm 转发')}
          </Space>
        </div>
        <Alert
          type="info"
          showIcon
          className="compact-alert"
          message="用于广州 -> HK 这种整端口中转"
          description="Client 会在当前 VPS 本机生成 realm 配置，并把本机监听端口直接转发到目标 Client 的指定端口；这不经过 x-ui 用户和出站规则。需要当前 Client 所在 VPS 已安装 realm，或在这里填写 realm 二进制路径。"
        />
        <Row gutter={[16, 16]}>
          <Col xs={24} md={6}>
            <div className="switch-row">
              <span>启用 Realm 转发</span>
              <Switch checked={Boolean(portForwarding.enabled)} onChange={(checked) => updatePortForwarding({ enabled: checked })} />
            </div>
          </Col>
          <Col xs={24} md={6}>
            <Text type="secondary">后端</Text>
            <Select
              style={{ width: '100%' }}
              value={portForwarding.backend || 'realm'}
              options={[
                { value: 'realm', label: 'realm' },
                { value: 'none', label: '不处理' },
              ]}
              onChange={(value) => updatePortForwarding({ backend: value })}
            />
          </Col>
          <Col xs={24} md={6}>
            <Text type="secondary">服务名</Text>
            <Input value={portForwarding.service_name || ''} placeholder="默认 vpsmonitor-realm" onChange={(event) => updatePortForwarding({ service_name: event.target.value })} />
          </Col>
          <Col xs={24} md={6}>
            <Text type="secondary">日志等级</Text>
            <Select
              style={{ width: '100%' }}
              value={portForwarding.log_level || 'info'}
              options={[
                { value: 'info', label: 'info' },
                { value: 'debug', label: 'debug' },
                { value: 'warn', label: 'warn' },
                { value: 'error', label: 'error' },
                { value: 'trace', label: 'trace' },
              ]}
              onChange={(value) => updatePortForwarding({ log_level: value })}
            />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">realm 二进制路径</Text>
            <Input value={portForwarding.binary_path || ''} placeholder="留空自动查找 realm，例如 /usr/local/bin/realm" onChange={(event) => updatePortForwarding({ binary_path: event.target.value })} />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">realm 配置路径</Text>
            <Input value={portForwarding.config_path || ''} placeholder="默认 /etc/vpsmonitor/realm.toml" onChange={(event) => updatePortForwarding({ config_path: event.target.value })} />
          </Col>
          <Col xs={24}>
            <List
              locale={{ emptyText: '暂无 Realm 转发；例如广州 Client 监听 443，转发到 HK Client 的 443/8443' }}
              dataSource={portForwarding.rules || []}
              renderItem={(rule, index) => (
                <List.Item
                  actions={[
                    <Button key="delete" size="small" danger icon={<DeleteOutlined />} onClick={() => removePortForwardRule(index)}>
                      删除
                    </Button>,
                  ]}
                >
                  <Row gutter={[12, 12]} style={{ width: '100%' }}>
                    <Col xs={24} md={3}>
                      <Text type="secondary">启用</Text>
                      <Switch checked={rule.enabled !== false} onChange={(checked) => updatePortForwardRule(index, { enabled: checked })} />
                    </Col>
                    <Col xs={24} md={5}>
                      <Text type="secondary">名称</Text>
                      <Input value={rule.name || ''} placeholder="例如 广州到 HK 443" onChange={(event) => updatePortForwardRule(index, { name: event.target.value })} />
                    </Col>
                    <Col xs={12} md={4}>
                      <Text type="secondary">监听端口</Text>
                      <InputNumber style={{ width: '100%' }} min={1} max={65535} precision={0} value={rule.listen_port || 0} onChange={(value) => updatePortForwardRule(index, { listen_port: Number(value || 0) })} />
                    </Col>
                    <Col xs={24} md={8}>
                      <Text type="secondary">目标 Client</Text>
                      <Select
                        allowClear
                        showSearch
                        style={{ width: '100%' }}
                        value={rule.target_agent_id || undefined}
                        placeholder="选择 HK Client，可自动填目标地址"
                        options={targetAgentOptions}
                        filterOption={(input, option) => String(option?.label || '').toLowerCase().includes(input.toLowerCase())}
                        onChange={(value) => selectPortForwardTargetAgent(index, value || '')}
                      />
                    </Col>
                    <Col xs={24} md={8}>
                      <Text type="secondary">目标地址</Text>
                      <Input value={rule.target_address || ''} placeholder="HK 公网 IP / 域名" onChange={(event) => updatePortForwardRule(index, { target_address: event.target.value })} />
                    </Col>
                    <Col xs={12} md={4}>
                      <Text type="secondary">目标端口</Text>
                      <InputNumber style={{ width: '100%' }} min={1} max={65535} precision={0} value={rule.target_port || 0} onChange={(value) => updatePortForwardRule(index, { target_port: Number(value || 0) })} />
                    </Col>
                    <Col xs={12} md={4}>
                      <Text type="secondary">协议</Text>
                      <Select
                        style={{ width: '100%' }}
                        value="both"
                        options={[
                          { value: 'both', label: 'TCP + UDP' },
                        ]}
                        onChange={(value) => updatePortForwardRule(index, { network: value })}
                      />
                    </Col>
                    <Col xs={24} md={8}>
                      <Text type="secondary">备注</Text>
                      <Input value={rule.note || ''} placeholder="可选" onChange={(event) => updatePortForwardRule(index, { note: event.target.value })} />
                    </Col>
                  </Row>
                </List.Item>
              )}
            />
          </Col>
        </Row>
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

function buildCertificateDomainOptions(certificates: XUILocalCertificate[]) {
  const seen = new Set<string>()
  const options: { value: string; label: string }[] = []
  certificates.forEach((certificate) => {
    const names = certificate.dns_names || []
    names.forEach((name) => {
      const value = normalizeDomainOption(name)
      if (!value || seen.has(value)) {
        return
      }
      seen.add(value)
      options.push({
        value,
        label: certificate.name ? `${value} · ${certificate.name}` : value,
      })
    })
  })
  return options
}

function normalizeDomainOption(value?: string) {
  const domain = (value || '').trim().toLowerCase().replace(/\.$/, '')
  if (!domain || domain.includes(' ') || domain.includes('*') || isIPLike(domain)) {
    return ''
  }
  return domain
}

function isIPLike(value: string) {
  return /^\d{1,3}(\.\d{1,3}){3}$/.test(value) || (value.includes(':') && /^[0-9a-f:]+$/i.test(value))
}

function bestAgentAddress(agent?: AgentListItem) {
  if (!agent) {
    return ''
  }
  return (
    agent.entry?.addresses?.find(Boolean) ||
    agent.summary?.observed_ip ||
    agent.summary?.public_ipv4 ||
    agent.summary?.public_ipv6 ||
    ''
  )
}
