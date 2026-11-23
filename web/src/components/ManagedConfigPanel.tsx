import { buildCertificateDomainOptions, normalizeNetworkPolicyRules, defaultNetworkPolicyRuleName, shouldUsePortAsNetworkPolicyName, bestAgentAddress, realmForwardAddressOptions, realmForwardTargetPortOptions } from '../lib/managedConfigHelpers'
import { useEffect, useId, useState } from 'react'
import { Alert, Button, Card, Col, Collapse, Empty, Input, InputNumber, List, Row, Select, Space, Spin, Switch, Typography } from 'antd'
import { SaveOutlined } from '@ant-design/icons'

import type { AgentEntryConfig, AgentListItem, ConfigAuditLog, ManagedAgentConfig, NetworkPortPolicyRule, RealmForwardRule, VPSRenewalConfig, XUIConfig, XUILocalCertificate, XUINodeView, XUIOverview } from '../types'
import { DEFAULT_COST_CURRENCY, type CurrencyCode } from '../lib/currency'
import { bytesToGB, gbToBytes } from '../lib/traffic'
import type { ConfigSectionKey } from '../lib/appHelpers'
import { normalizeXUIOverview } from '../lib/appHelpersConfig'
import { AdminHelpHint } from './AdminHelpHint'
import { formatDateTime, formatRenewalHint, summarizeConfigAudit } from '../lib/appHelpers'
import { fetchJSON } from '../lib/api'
import { EntryConnectionSection } from './EntryConnectionSection'
import { HAProxyConfigSection } from './HAProxyConfigSection'
import { NetworkPolicyConfigSection } from './ManagedConfigNetworkSection'
import { RealmForwardingSection } from './ManagedConfigRealmSection'

const { Text, Title } = Typography

export type ConfigPanelSection = 'all' | 'basic' | 'xui' | 'nat' | 'network' | 'realm' | 'haproxy' | 'audit'

export interface ConfigPanelProps {
  selectedAgent?: AgentListItem
  agents?: AgentListItem[]
  managedConfig: ManagedAgentConfig | null
  nodes?: XUINodeView[]
  certificates: XUILocalCertificate[]
  configLoading: boolean
  configSavingSection: ConfigSectionKey | null
  configError: string
  onSave: (section: ConfigSectionKey, draftOverride?: ManagedAgentConfig) => void
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
  onCopyRealmConfig?: (targetAgentID: string) => void
  realmCopyLoading?: boolean
  configAudits: ConfigAuditLog[]
  configAuditsLoading: boolean
  currencyOptions: CurrencyCode[]
  section?: ConfigPanelSection
}

export function ManagedConfigPanel(props: ConfigPanelProps) {
  const {
    selectedAgent,
    agents = [],
    managedConfig,
    certificates,
    nodes = [],
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
    onCopyRealmConfig,
    realmCopyLoading = false,
    configAudits,
    configAuditsLoading,
    currencyOptions,
    section = 'all',
  } = props
  const [realmCopyTargetAgentID, setRealmCopyTargetAgentID] = useState('')
  const [networkPolicyEditedAgentID, setNetworkPolicyEditedAgentID] = useState('')
  const [realmTargetOverviews, setRealmTargetOverviews] = useState<Record<string, XUIOverview>>({})
  const [realmTargetOverviewLoading, setRealmTargetOverviewLoading] = useState<Record<string, boolean>>({})
  const xuiFieldID = useId()

  const realmTargetAgentIDs = Array.from(new Set((managedConfig?.entry?.port_forwarding?.rules || []).map((rule) => rule.target_agent_id || '').filter(Boolean))).sort()
  const realmTargetAgentKey = realmTargetAgentIDs.join('|')
  useEffect(() => {
    let cancelled = false
    realmTargetAgentIDs.forEach((agentID) => {
      if (realmTargetOverviews[agentID] || realmTargetOverviewLoading[agentID]) {
        return
      }
      setRealmTargetOverviewLoading((current) => ({ ...current, [agentID]: true }))
      void fetchJSON<XUIOverview>(`/api/v1/agents/${encodeURIComponent(agentID)}/xui/overview`)
        .then((overview) => {
          if (!cancelled) {
            setRealmTargetOverviews((current) => ({ ...current, [agentID]: normalizeXUIOverview(overview) }))
          }
        })
        .catch(() => undefined)
        .finally(() => {
          if (!cancelled) {
            setRealmTargetOverviewLoading((current) => ({ ...current, [agentID]: false }))
          }
        })
    })
    return () => { cancelled = true }
  }, [realmTargetAgentKey])

  useEffect(() => {
    const rules = managedConfig?.entry?.port_forwarding?.rules || []
    if (!rules.length) {
      return
    }
    let changed = false
    const nextRules = rules.map((rule) => {
      if (!rule.target_agent_id || rule.target_port_mode === 'custom' || Number(rule.target_port || 0) > 0) {
        return rule
      }
      const agent = agents.find((item) => item.agent_id === rule.target_agent_id)
      const option = realmForwardTargetPortOptions(agent, realmTargetOverviews[rule.target_agent_id])[0]
      if (!option) {
        return rule
      }
      changed = true
      return { ...rule, target_port: option.port, target_port_mode: 'auto' as const }
    })
    if (changed) {
      onEntryChange({ port_forwarding: { ...managedConfig?.entry?.port_forwarding, rules: nextRules } })
    }
  }, [managedConfig?.entry?.port_forwarding?.rules, realmTargetOverviews, agents, onEntryChange])

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
    connection: managedConfig.entry?.connection || selectedAgent.entry?.connection || {},
    addresses: managedConfig.entry?.addresses || [],
    import_domain: managedConfig.entry?.import_domain || '',
    mappings: managedConfig.entry?.mappings || [],
    network_policy: managedConfig.entry?.network_policy || { enabled: false, interface: '', firewall_backend: 'auto', rate_limit_backend: 'auto', rules: [] },
    port_forwarding: managedConfig.entry?.port_forwarding || selectedAgent.entry?.port_forwarding || { enabled: false, backend: 'realm', binary_path: '', config_path: '', service_name: '', log_level: 'info', rules: [] },
    haproxy: managedConfig.entry?.haproxy || selectedAgent.entry?.haproxy || { enabled: false, binary_path: '', config_path: '', service_name: '', rules: [] },
  }
  const entryConnection = entryConfig.connection || {}
  const observedNetworkPolicy = selectedAgent.network_policy
  const observedNetworkRules = observedNetworkPolicy?.rules || []
  const observedHasWhitelist = observedNetworkRules.some((rule) => (rule.whitelist_ips || []).length > 0)
  const observedHasRateLimit = observedNetworkRules.some((rule) => Number(rule.rate_limit_mbps || 0) > 0)
  const configuredNetworkPolicy = entryConfig.network_policy || { enabled: false, interface: '', firewall_backend: 'auto', rate_limit_backend: 'auto', rules: [] }
  const configuredNetworkRules = configuredNetworkPolicy.rules || []
  const networkPolicyEdited = networkPolicyEditedAgentID === selectedAgent.agent_id
  const usingObservedNetworkPolicy = !networkPolicyEdited && configuredNetworkRules.length === 0 && observedNetworkRules.length > 0
  const networkPolicy = usingObservedNetworkPolicy
    ? {
        ...configuredNetworkPolicy,
        enabled: true,
        interface: observedNetworkPolicy?.interface || configuredNetworkPolicy.interface || '',
        firewall_backend: observedHasWhitelist ? (observedNetworkPolicy?.firewall_backend || 'ufw') : configuredNetworkPolicy.firewall_backend,
        rate_limit_backend: observedHasRateLimit ? (observedNetworkPolicy?.rate_limit_backend || 'tc') : configuredNetworkPolicy.rate_limit_backend,
        rules: normalizeNetworkPolicyRules(observedNetworkRules),
      }
    : configuredNetworkPolicy
  const portForwarding = entryConfig.port_forwarding || { enabled: false, backend: 'realm', binary_path: '', config_path: '', service_name: '', log_level: 'info', rules: [] }
  const haProxy = entryConfig.haproxy || { enabled: false, binary_path: '', config_path: '', service_name: '', rules: [] }
  const observedRealm = selectedAgent.realm
  const observedRealmRules = observedRealm?.rules || []
  const targetAgentOptions = agents
    .filter((agent) => agent.agent_id && agent.agent_id !== selectedAgent.agent_id)
    .map((agent) => ({ value: agent.agent_id, label: `${agent.agent_name || agent.agent_id} · ${bestAgentAddress(agent) || '未上报 IP'}` }))
  const canCopyRealmConfig = Boolean(onCopyRealmConfig && realmCopyTargetAgentID && (portForwarding.rules || []).length)
  const domainOptions = buildCertificateDomainOptions(certificates)
  const updateNetworkPolicy = (patch: Partial<NonNullable<AgentEntryConfig['network_policy']>>) => {
    setNetworkPolicyEditedAgentID(selectedAgent.agent_id)
    onEntryChange({ network_policy: { ...networkPolicy, ...patch } })
  }
  const updateNetworkPolicyRule = (index: number, patch: Partial<NetworkPortPolicyRule>) => {
    const rules = (networkPolicy.rules || []).map((rule, currentIndex) => {
      if (currentIndex !== index) {
        return rule
      }
      const next = { ...rule, ...patch }
      if (patch.port !== undefined && shouldUsePortAsNetworkPolicyName(rule.name, rule.port)) {
        next.name = defaultNetworkPolicyRuleName(Number(patch.port || 0))
      }
      return next
    })
    updateNetworkPolicy({ rules })
  }
  const addNetworkPolicyRule = () => {
    const port = 443
    updateNetworkPolicy({
      enabled: true,
      rules: [
        ...(networkPolicy.rules || []),
        { id: `rule-${Date.now()}`, enabled: true, name: defaultNetworkPolicyRuleName(port), port, protocol: 'tcp', rate_limit_mbps: 0, whitelist_ips: [] },
      ],
    })
  }
  const removeNetworkPolicyRule = (index: number) => {
    updateNetworkPolicy({ rules: (networkPolicy.rules || []).filter((_, currentIndex) => currentIndex !== index) })
  }
  const saveNetworkPolicy = () => {
    onSave('entry', {
      ...managedConfig,
      entry: {
        ...managedConfig.entry,
        network_policy: networkPolicy,
      },
    })
  }
  const updatePortForwarding = (patch: Partial<NonNullable<AgentEntryConfig['port_forwarding']>>) => {
    const nextPortForwarding = { ...portForwarding, ...patch, ...(patch.enabled ? { backend: 'realm' } : {}) }
    onEntryChange({
      port_forwarding: nextPortForwarding,
      ...(nextPortForwarding.enabled ? { haproxy: { ...haProxy, enabled: false } } : {}),
    })
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
          target_port_mode: 'auto',
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
    const addressOptions = realmForwardAddressOptions(agent)
    const portOption = realmForwardTargetPortOptions(agent, realmTargetOverviews[agentID])[0]
    updatePortForwardRule(index, {
      target_agent_id: agentID,
      target_address: addressOptions[0]?.value || bestAgentAddress(agent) || '',
      target_port: portOption?.port || 0,
      target_port_mode: 'auto',
    })
  }
  const sectionSaving = Boolean(configSavingSection)
  const showSection = (target: ConfigPanelSection) => section === 'all' || section === target
  const hiddenSectionStyle = (target: ConfigPanelSection) => showSection(target) ? undefined : { display: 'none' }
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
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16, width: '100%' }}>
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
        description="client 注册后，server 会保存并下发托管参数。x-ui 配置由 client 使用。"
        className="compact-alert"
        style={hiddenSectionStyle('basic')}
      />

      <Card className="config-section-card" bordered={false} style={hiddenSectionStyle('basic')}>
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
            <AdminHelpHint title="标签先创建再多选；保存 Client 信息后会应用到当前 Client。" />
          </Col>
        </Row>
      </Card>



      <Card className="config-section-card" bordered={false} style={hiddenSectionStyle('basic')}>
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
                { value: 'semiannual', label: '每半年' },
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
                { value: 'semiannual', label: '每半年' },
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
            <Text type="secondary">流量计算方式</Text>
            <Select
              style={{ width: '100%' }}
              value={managedConfig.renewal?.traffic_accounting_mode || 'bidirectional'}
              options={[
                { value: 'bidirectional', label: '双向：上传 + 下载' },
                { value: 'single_direction', label: '单向：取较大方向' },
              ]}
              onChange={(value) => onRenewalChange({ traffic_accounting_mode: value })}
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

      <Card className="config-section-card xui-config-card" bordered={false} style={hiddenSectionStyle('xui')}>
        <div className="section-title-row">
          <Title level={4}>X-UI 托管配置</Title>
          {sectionSaveButton('xui', '保存 X-UI 配置')}
        </div>
        <Row gutter={[16, 12]}>
          <Col xs={24}>
            <div className="xui-config-options">
              <label className="xui-config-option" htmlFor={`${xuiFieldID}-enabled`}>
                <span>启用 x-ui 采集</span>
                <Switch id={`${xuiFieldID}-enabled`} aria-label="启用 x-ui 采集" checked={managedConfig.xui.enabled} onChange={(checked) => onXUIChange({ enabled: checked })} />
              </label>
              <label className="xui-config-option" htmlFor={`${xuiFieldID}-skip-tls`}>
                <span>跳过 TLS 校验</span>
                <Switch id={`${xuiFieldID}-skip-tls`} aria-label="跳过 TLS 校验" checked={managedConfig.xui.skip_tls_verify} onChange={(checked) => onXUIChange({ skip_tls_verify: checked })} />
              </label>
              <label className="xui-config-option" htmlFor={`${xuiFieldID}-access-log-enabled`}>
                <span>采集访问日志</span>
                <Switch id={`${xuiFieldID}-access-log-enabled`} aria-label="采集访问日志" checked={Boolean(managedConfig.xui.access_log_enabled)} onChange={(checked) => onXUIChange({ access_log_enabled: checked })} />
              </label>
              {managedConfig.xui.access_log_enabled && (
                <div className="xui-config-option xui-config-retention">
                  <label htmlFor={`${xuiFieldID}-retention-days`}>日志保留天数</label>
                  <InputNumber
                    id={`${xuiFieldID}-retention-days`}
                    min={1}
                    max={30}
                    precision={0}
                    value={managedConfig.xui.access_log_retention_days || 7}
                    onChange={(value) => onXUIChange({ access_log_retention_days: Number(value || 7) })}
                  />
                </div>
              )}
            </div>
          </Col>
          <Col xs={24} md={8}>
            <div className="xui-config-field">
              <label htmlFor={`${xuiFieldID}-deployment`}><Text type="secondary">安装方式</Text></label>
              <Select
                id={`${xuiFieldID}-deployment`}
                style={{ width: '100%' }}
                value={entryConnection.deployment || 'native'}
                options={[{ value: 'native', label: '直接安装' }, { value: 'docker', label: 'Docker' }]}
                onChange={(value) => onEntryChange({ connection: { ...entryConnection, deployment: value, docker_network: value === 'docker' ? (entryConnection.docker_network || 'host') : undefined } })}
              />
            </div>
          </Col>
          {entryConnection.deployment === 'docker' ? (
            <Col xs={24} md={8}>
              <div className="xui-config-field">
                <label htmlFor={`${xuiFieldID}-docker-network`}><Text type="secondary">Docker 网络</Text></label>
                <Select
                  id={`${xuiFieldID}-docker-network`}
                  style={{ width: '100%' }}
                  value={entryConnection.docker_network || 'host'}
                  options={[{ value: 'host', label: 'Host · 端口直通' }, { value: 'bridge', label: 'Bridge · 端口映射' }]}
                  onChange={(value) => onEntryChange({ connection: { ...entryConnection, deployment: 'docker', docker_network: value } })}
                />
              </div>
            </Col>
          ) : null}
          <Col xs={24} md={8}>
            <Text type="secondary">Docker 配置说明</Text>
            <div className="muted-line">Bridge 网络需要在 X-UI 页的“端口映射”中登记服务器端口；Host 网络无需额外映射。</div>
          </Col>
          <Col xs={24} md={12}>
            <div className="xui-config-field">
              <label htmlFor={`${xuiFieldID}-base-url`}><Text type="secondary">Base URL</Text></label>
              <Input id={`${xuiFieldID}-base-url`} value={managedConfig.xui.base_url || ''} placeholder="用于打开面板/执行操作，例如 https://127.0.0.1:2053/secret/" onChange={(event) => onXUIChange({ base_url: event.target.value })} />
            </div>
          </Col>
          <Col xs={24} md={12}>
            <div className="xui-config-field">
              <label htmlFor={`${xuiFieldID}-api-token`}><Text type="secondary">API Token</Text></label>
              <Input.Password
                id={`${xuiFieldID}-api-token`}
                value={managedConfig.xui.api_token || ''}
                placeholder="新版 3x-ui：Settings → Security → API Token"
                onChange={(event) => onXUIChange({ api_token: event.target.value })}
              />
            </div>
          </Col>
          {managedConfig.xui.access_log_enabled && (
            <Col xs={24}>
              <div className="xui-config-field">
                <label htmlFor={`${xuiFieldID}-access-log-path`}><Text type="secondary">Xray access.log 路径</Text></label>
                <Input
                  id={`${xuiFieldID}-access-log-path`}
                  value={managedConfig.xui.access_log_path || ''}
                  placeholder="例如 /usr/local/x-ui/bin/access.log；Docker 需填写 client 本机可读取的挂载路径"
                  onChange={(event) => onXUIChange({ access_log_path: event.target.value })}
                />
              </div>
            </Col>
          )}
          <Col xs={24}>
            <Collapse
              className="xui-config-advanced"
              ghost
              size="small"
              items={[{
                key: 'local-db',
                label: '高级设置',
                children: (
                  <Space direction="vertical" style={{ width: '100%' }}>
                    <div className="xui-config-field">
                      <label htmlFor={`${xuiFieldID}-db-path`}><Text type="secondary">本地 x-ui 数据库路径</Text></label>
                      <Input
                        id={`${xuiFieldID}-db-path`}
                        value={managedConfig.xui.db_path || ''}
                        placeholder="默认自动读取 /etc/x-ui/x-ui.db"
                        onChange={(event) => onXUIChange({ db_path: event.target.value })}
                      />
                    </div>
                    <Text type="secondary">用于本地兜底读取，常规安装无需填写；Docker 或非标准安装可填写 Client 本机可读取的数据库路径。</Text>
                  </Space>
                ),
              }]}
            />
          </Col>
        </Row>
      </Card>

      <div style={hiddenSectionStyle('nat')}>
        <EntryConnectionSection
          entry={entryConfig}
          nodes={nodes}
          fallbackHost={entryConfig.import_domain || domainOptions.find((option) => !option.value.startsWith('*.'))?.value || entryConfig.addresses?.[0] || bestAgentAddress(selectedAgent) || ''}
          domainOptions={domainOptions}
          addressText={entryAddressInputText}
          saving={configSavingSection === 'entry'}
          onChange={onEntryChange}
          onAddressTextChange={onEntryAddressesTextChange}
          onSave={() => onSave('entry')}
        />
      </div>

      <NetworkPolicyConfigSection
        visible={showSection('network')}
        networkPolicy={networkPolicy}
        observedNetworkPolicy={observedNetworkPolicy}
        usingObservedNetworkPolicy={usingObservedNetworkPolicy}
        configSavingSection={configSavingSection}
        sectionSaving={sectionSaving}
        onAddRule={addNetworkPolicyRule}
        onChange={updateNetworkPolicy}
        onSave={saveNetworkPolicy}
        onRemoveRule={removeNetworkPolicyRule}
        onUpdateRule={updateNetworkPolicyRule}
      />

      <RealmForwardingSection
        visible={showSection('realm')}
        portForwarding={portForwarding}
        observedRealm={observedRealm}
        observedRealmRules={observedRealmRules}
        targetAgentOptions={targetAgentOptions}
        agents={agents}
        onCopyRealmConfig={onCopyRealmConfig}
        realmCopyTargetAgentID={realmCopyTargetAgentID}
        onRealmCopyTargetChange={setRealmCopyTargetAgentID}
        canCopyRealmConfig={canCopyRealmConfig}
        realmCopyLoading={realmCopyLoading}
        onAddRule={addPortForwardRule}
        saveButton={sectionSaveButton('entry', '保存 Realm 转发')}
        onPortForwardingChange={updatePortForwarding}
        targetOverviews={realmTargetOverviews}
        targetOverviewLoading={realmTargetOverviewLoading}
        onRemoveRule={removePortForwardRule}
        onSelectTargetAgent={selectPortForwardTargetAgent}
        onUpdateRule={updatePortForwardRule}
      />

      <HAProxyConfigSection
        visible={showSection('haproxy')}
        selectedAgent={selectedAgent}
        agents={agents}
        config={haProxy}
        runtime={selectedAgent.haproxy}
        saving={configSavingSection === 'entry'}
        saveDisabled={sectionSaving && configSavingSection !== 'entry'}
        onChange={(next) => onEntryChange({
          haproxy: next,
          ...(next.enabled ? { port_forwarding: { ...portForwarding, enabled: false, backend: 'none' } } : {}),
        })}
        onSave={() => onSave('entry')}
      />

      <Card className="config-section-card" bordered={false} style={hiddenSectionStyle('audit')}>
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

    </div>
  )
}
