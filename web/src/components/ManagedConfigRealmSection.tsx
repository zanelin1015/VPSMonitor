import type { ReactNode } from 'react'
import { Alert, Button, Card, Col, Input, InputNumber, List, Row, Select, Space, Switch, Typography } from 'antd'
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons'
import type { AgentEntryConfig, AgentListItem, DashboardAgentView, RealmForwardRule, XUIOverview } from '../types'
import { formatRealmEndpoint, realmForwardAddressOptions, realmForwardTargetPortOptions } from '../lib/managedConfigHelpers'
import { formatDateTime } from '../lib/appHelpers'

const { Text, Title } = Typography
type PortForwarding = NonNullable<AgentEntryConfig['port_forwarding']>

interface RealmForwardingSectionProps {
  visible: boolean
  portForwarding: PortForwarding
  observedRealm?: DashboardAgentView['realm']
  observedRealmRules: RealmForwardRule[]
  targetAgentOptions: Array<{ value: string; label: string }>
  agents: AgentListItem[]
  onCopyRealmConfig?: (targetAgentID: string) => void
  realmCopyTargetAgentID: string
  onRealmCopyTargetChange: (agentID: string) => void
  canCopyRealmConfig: boolean
  realmCopyLoading: boolean
  onAddRule: () => void
  saveButton: ReactNode
  onPortForwardingChange: (patch: Partial<PortForwarding>) => void
  targetOverviews: Record<string, XUIOverview>
  targetOverviewLoading: Record<string, boolean>
  onRemoveRule: (index: number) => void
  onSelectTargetAgent: (index: number, agentID: string) => void
  onUpdateRule: (index: number, patch: Partial<RealmForwardRule>) => void
}

export function RealmForwardingSection(props: RealmForwardingSectionProps) {
  const { visible, portForwarding, observedRealm, observedRealmRules, targetAgentOptions, agents, onCopyRealmConfig, realmCopyTargetAgentID, onRealmCopyTargetChange, canCopyRealmConfig, realmCopyLoading, onAddRule, saveButton, onPortForwardingChange, targetOverviews, targetOverviewLoading, onRemoveRule, onSelectTargetAgent, onUpdateRule } = props
  return (
      <Card className="config-section-card" bordered={false} style={visible ? undefined : { display: 'none' }}>
        <div className="section-title-row">
          <Title level={4}>Realm 端口转发</Title>
          <Space wrap>
            {onCopyRealmConfig ? (
              <>
                <Select
                  showSearch
                  allowClear
                  size="small"
                  style={{ minWidth: 240 }}
                  value={realmCopyTargetAgentID || undefined}
                  placeholder="复制到目标 Client"
                  options={targetAgentOptions}
                  filterOption={(input, option) => String(option?.label || '').toLowerCase().includes(input.toLowerCase())}
                  onChange={(value) => onRealmCopyTargetChange(value || '')}
                />
                <Button
                  size="small"
                  loading={realmCopyLoading}
                  disabled={!canCopyRealmConfig}
                  onClick={() => realmCopyTargetAgentID && onCopyRealmConfig(realmCopyTargetAgentID)}
                >
                  复制到 Client 并生效
                </Button>
              </>
            ) : null}
            <Button size="small" icon={<PlusOutlined />} onClick={onAddRule}>
              添加转发
            </Button>
            {saveButton}
          </Space>
        </div>
        <Space wrap style={{ marginBottom: 16 }}>
          <Switch aria-label="统计 Realm 端口流量" checked={Boolean(portForwarding.traffic_enabled)} onChange={(checked) => onPortForwardingChange({ traffic_enabled: checked })} />
          <Text>统计端口流量</Text>
          <Text type="secondary">保存后从 Client 采集，每条规则可在“周期流量”中按日期查询。需要原生 Linux、nftables 及管理网络权限。</Text>
        </Space>
        <Alert
          type="info"
          showIcon
          className="compact-alert"
          message="用于广州 -> HK 这种整端口中转"
          description="Client 会在当前 VPS 本机生成 realm 配置，并把本机监听端口直接转发到目标 Client 的指定端口；这不经过 x-ui 用户和出站规则。需要当前 Client 所在 VPS 已安装 realm，或在这里填写 realm 二进制路径。"
        />
        {observedRealm ? (
          <div className="observed-config-panel">
            <Space direction="vertical" size={8} style={{ width: '100%' }}>
              <Space wrap>
                <Text strong>Client 当前采集配置</Text>
                <Text type="secondary">路径：{observedRealm.config_path || '-'}</Text>
                <Text type="secondary">服务：{observedRealm.service_name || '-'}</Text>
                <Text type="secondary">采集：{formatDateTime(observedRealm.collected_at)}</Text>
              </Space>
              {observedRealm.error ? (
                <Alert type="error" showIcon className="compact-alert" message={observedRealm.error} />
              ) : null}
              <List
                size="small"
                locale={{ emptyText: 'Client 当前没有采集到 Realm endpoint' }}
                dataSource={observedRealmRules}
                renderItem={(rule, index) => (
                  <List.Item key={rule.id || `${rule.listen_port || 0}-${rule.target_address || ''}-${rule.target_port || 0}-${index}`}>
                    <Space wrap>
                      <Text>{rule.name || `endpoint ${index + 1}`}</Text>
                      <Text code>{formatRealmEndpoint(rule.listen_address || '0.0.0.0', rule.listen_port)}</Text>
                      <Text type="secondary">-&gt;</Text>
                      <Text code>{formatRealmEndpoint(rule.target_address || '-', rule.target_port)}</Text>
                    </Space>
                  </List.Item>
                )}
              />
            </Space>
          </div>
        ) : (
          <Alert
            type="warning"
            showIcon
            className="compact-alert"
            message="尚未采集到 Client 实际 Realm 配置"
            description="保存后可点击立即获取 Client 信息，确认最终写入到 Client 的 realm 配置。"
          />
        )}
        <Row gutter={[16, 16]}>
          <Col xs={24} md={6}>
            <div className="switch-row">
              <span>启用 Realm 转发</span>
              <Switch checked={Boolean(portForwarding.enabled)} onChange={(checked) => onPortForwardingChange({ enabled: checked })} />
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
              onChange={(value) => onPortForwardingChange({ backend: value })}
            />
          </Col>
          <Col xs={24} md={6}>
            <Text type="secondary">服务名</Text>
            <Input value={portForwarding.service_name || ''} placeholder="默认 vpsmonitor-realm" onChange={(event) => onPortForwardingChange({ service_name: event.target.value })} />
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
              onChange={(value) => onPortForwardingChange({ log_level: value })}
            />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">realm 二进制路径</Text>
            <Input value={portForwarding.binary_path || ''} placeholder="留空自动查找 realm，例如 /usr/local/bin/realm" onChange={(event) => onPortForwardingChange({ binary_path: event.target.value })} />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">realm 配置路径</Text>
            <Input value={portForwarding.config_path || ''} placeholder="默认 /etc/vpsmonitor/realm.toml" onChange={(event) => onPortForwardingChange({ config_path: event.target.value })} />
          </Col>
          <Col xs={24}>
            <List
              locale={{ emptyText: '暂无 Realm 转发；例如广州 Client 监听 443，转发到 HK Client 的 443/8443' }}
              dataSource={portForwarding.rules || []}
              renderItem={(rule, index) => {
                const targetAgent = agents.find((agent) => agent.agent_id === rule.target_agent_id)
                const currentAddress = String(rule.target_address || '').trim()
                const addressOptions = realmForwardAddressOptions(targetAgent)
                const targetPortOptions = realmForwardTargetPortOptions(targetAgent, targetOverviews[rule.target_agent_id || ''])
                const targetPortLoading = Boolean(rule.target_agent_id && targetOverviewLoading[rule.target_agent_id])
                const currentPort = Number(rule.target_port || 0)
                const knownPortOption = targetPortOptions.find((option) => option.port === currentPort)
                const legacyPortOption = targetPortLoading && currentPort > 0 && !knownPortOption
                  ? { value: `current:${currentPort}`, label: `当前端口 · ${currentPort}` }
                  : undefined
                const customPortSelected = rule.target_port_mode === 'custom' || (!targetPortLoading && currentPort > 0 && !knownPortOption)
                const selectedPortValue = customPortSelected ? 'custom' : knownPortOption?.value || legacyPortOption?.value
                const portOptions = [
                  ...(legacyPortOption ? [legacyPortOption] : []),
                  ...targetPortOptions.map((option) => ({ value: option.value, label: option.label })),
                  { value: 'custom', label: '自定义' },
                ]
                if (currentAddress && !addressOptions.some((option) => option.value === currentAddress)) {
                  addressOptions.unshift({ value: currentAddress, label: `当前配置 · ${currentAddress}` })
                }
                return <List.Item
                  actions={[
                    <Button key="delete" size="small" danger icon={<DeleteOutlined />} onClick={() => onRemoveRule(index)}>
                      删除
                    </Button>,
                  ]}
                >
                  <Row gutter={[12, 12]} style={{ width: '100%' }}>
                    <Col xs={24} md={3}>
                      <Text type="secondary">启用</Text>
                      <Switch checked={rule.enabled !== false} onChange={(checked) => onUpdateRule(index, { enabled: checked })} />
                    </Col>
                    <Col xs={24} md={5}>
                      <Text type="secondary">名称</Text>
                      <Input value={rule.name || ''} placeholder="例如 广州到 HK 443" onChange={(event) => onUpdateRule(index, { name: event.target.value })} />
                    </Col>
                    <Col xs={12} md={4}>
                      <Text type="secondary">监听端口</Text>
                      <InputNumber style={{ width: '100%' }} min={1} max={65535} precision={0} value={rule.listen_port || 0} onChange={(value) => onUpdateRule(index, { listen_port: Number(value || 0) })} />
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
                        onChange={(value) => onSelectTargetAgent(index, value || '')}
                      />
                    </Col>
                    <Col xs={24} md={8}>
                      <Text type="secondary">目标地址</Text>
                      <Select
                        showSearch
                        allowClear
                        style={{ width: '100%' }}
                        value={rule.target_address || undefined}
                        placeholder="选择域名 / 公网 IP / IX 内网 IP"
                        options={addressOptions}
                        optionFilterProp="label"
                        onChange={(value) => onUpdateRule(index, { target_address: value || '' })}
                      />
                    </Col>
                    <Col xs={12} md={4}>
                      <Text type="secondary">目标端口</Text>
                      <Select
                        showSearch
                        style={{ width: '100%' }}
                        loading={targetPortLoading}
                        value={selectedPortValue}
                        placeholder={targetPortLoading ? '正在读取目标节点端口' : '选择节点或 Realm 监听端口'}
                        options={portOptions}
                        optionFilterProp="label"
                        onChange={(value) => {
                          if (value === 'custom') {
                            onUpdateRule(index, { target_port_mode: 'custom' })
                            return
                          }
                          const port = Number(String(value || '').split(':').pop() || 0)
                          onUpdateRule(index, { target_port_mode: 'auto', target_port: port })
                        }}
                      />
                      {customPortSelected && <InputNumber style={{ width: '100%', marginTop: 8 }} min={1} max={65535} precision={0} value={rule.target_port || 0} placeholder="输入目标端口" onChange={(value) => onUpdateRule(index, { target_port_mode: 'custom', target_port: Number(value || 0) })} />}
                    </Col>
                    <Col xs={12} md={4}>
                      <Text type="secondary">协议</Text>
                      <Select
                        style={{ width: '100%' }}
                        value="both"
                        options={[
                          { value: 'both', label: 'TCP + UDP' },
                        ]}
                        onChange={(value) => onUpdateRule(index, { network: value })}
                      />
                    </Col>
                    <Col xs={24} md={8}>
                      <Text type="secondary">备注</Text>
                      <Input value={rule.note || ''} placeholder="可选" onChange={(event) => onUpdateRule(index, { note: event.target.value })} />
                    </Col>
                  </Row>
                </List.Item>
              }}
            />
          </Col>
        </Row>
      </Card>

  )
}
