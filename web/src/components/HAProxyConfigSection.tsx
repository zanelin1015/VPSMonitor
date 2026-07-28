import { Button, Card, Col, Input, InputNumber, List, Row, Select, Space, Switch, Typography } from 'antd'
import { DeleteOutlined, PlusOutlined, SaveOutlined } from '@ant-design/icons'

import type { AgentListItem, HAProxyConfig, HAProxyRealmTarget, HAProxyRule } from '../types'

const { Text, Title } = Typography

interface RealmTargetOption {
  value: string
  label: string
  target: HAProxyRealmTarget
}

interface HAProxyConfigSectionProps {
  visible: boolean
  selectedAgent: AgentListItem
  agents: AgentListItem[]
  config: HAProxyConfig
  saving: boolean
  saveDisabled: boolean
  onChange: (config: HAProxyConfig) => void
  onSave: () => void
}

export function HAProxyConfigSection(props: HAProxyConfigSectionProps) {
  const { visible, selectedAgent, agents, config, saving, saveDisabled, onChange, onSave } = props
  const targetOptions = buildRealmTargetOptions(agents, selectedAgent.agent_id)
  const updateRule = (index: number, patch: Partial<HAProxyRule>) => {
    onChange({ ...config, rules: (config.rules || []).map((rule, current) => current === index ? { ...rule, ...patch } : rule) })
  }
  const addRule = () => {
    onChange({
      ...config,
      enabled: true,
      rules: [
        ...(config.rules || []),
        {
          id: `haproxy-${Date.now()}`,
          name: '',
          enabled: true,
          listen_address: '0.0.0.0',
          listen_port: 20001,
          primary: emptyTarget(),
          backups: [emptyTarget()],
          check_interval_seconds: 3,
          connect_timeout_seconds: 5,
          fall: 3,
          rise: 2,
        },
      ],
    })
  }
  const removeRule = (index: number) => onChange({ ...config, rules: (config.rules || []).filter((_, current) => current !== index) })
  const selectPrimary = (index: number, value?: string) => updateRule(index, { primary: targetFromValue(targetOptions, value) })
  const selectBackup = (ruleIndex: number, backupIndex: number, value?: string) => {
    const rule = (config.rules || [])[ruleIndex]
    if (!rule) return
    updateRule(ruleIndex, {
      backups: (rule.backups || []).map((target, current) => current === backupIndex ? targetFromValue(targetOptions, value) : target),
    })
  }
  const addBackup = (ruleIndex: number) => {
    const rule = (config.rules || [])[ruleIndex]
    if (!rule) return
    updateRule(ruleIndex, { backups: [...(rule.backups || []), emptyTarget()] })
  }
  const removeBackup = (ruleIndex: number, backupIndex: number) => {
    const rule = (config.rules || [])[ruleIndex]
    if (!rule) return
    updateRule(ruleIndex, { backups: (rule.backups || []).filter((_, current) => current !== backupIndex) })
  }

  return (
    <Card className="config-section-card" bordered={false} style={visible ? undefined : { display: 'none' }}>
      <div className="section-title-row">
        <Title level={4}>HAProxy 主备转发</Title>
        <Space wrap>
          <Button size="small" icon={<PlusOutlined />} onClick={addRule}>添加主备规则</Button>
          <Button type="primary" size="small" icon={<SaveOutlined />} loading={saving} disabled={saveDisabled} onClick={onSave}>保存 HAProxy</Button>
        </Space>
      </div>
      <Row gutter={[16, 16]}>
        <Col xs={24} md={6}>
          <div className="switch-row">
            <span>启用 HAProxy</span>
            <Switch checked={Boolean(config.enabled)} onChange={(enabled) => onChange({ ...config, enabled })} />
          </div>
        </Col>
        <Col xs={24} md={6}>
          <Text type="secondary">服务名</Text>
          <Input value={config.service_name || ''} placeholder="vpsmonitor-haproxy" onChange={(event) => onChange({ ...config, service_name: event.target.value })} />
        </Col>
        <Col xs={24} md={6}>
          <Text type="secondary">二进制路径</Text>
          <Input value={config.binary_path || ''} placeholder="自动查找 haproxy" onChange={(event) => onChange({ ...config, binary_path: event.target.value })} />
        </Col>
        <Col xs={24} md={6}>
          <Text type="secondary">配置路径</Text>
          <Input value={config.config_path || ''} placeholder="/etc/vpsmonitor/haproxy.cfg" onChange={(event) => onChange({ ...config, config_path: event.target.value })} />
        </Col>
        <Col xs={24}>
          <List
            locale={{ emptyText: '暂无 HAProxy 主备规则' }}
            dataSource={config.rules || []}
            renderItem={(rule, ruleIndex) => {
              const selectedTargets = new Set([
                realmTargetValue(rule.primary),
                ...(rule.backups || []).map(realmTargetValue),
              ].filter(Boolean))
              return (
                <List.Item
                  actions={[
                    <Button key="delete" size="small" danger icon={<DeleteOutlined />} onClick={() => removeRule(ruleIndex)}>删除</Button>,
                  ]}
                >
                  <Row gutter={[12, 12]} style={{ width: '100%' }}>
                    <Col xs={24} md={2}>
                      <Text type="secondary">启用</Text>
                      <Switch checked={rule.enabled !== false} onChange={(enabled) => updateRule(ruleIndex, { enabled })} />
                    </Col>
                    <Col xs={24} md={6}>
                      <Text type="secondary">名称</Text>
                      <Input value={rule.name || ''} placeholder="广州 20001 主备" onChange={(event) => updateRule(ruleIndex, { name: event.target.value })} />
                    </Col>
                    <Col xs={12} md={4}>
                      <Text type="secondary">监听端口</Text>
                      <InputNumber style={{ width: '100%' }} min={1} max={65535} precision={0} value={rule.listen_port || 0} onChange={(value) => updateRule(ruleIndex, { listen_port: Number(value || 0) })} />
                    </Col>
                    <Col xs={24} md={12}>
                      <Text type="secondary">主节点</Text>
                      <Select
                        allowClear
                        showSearch
                        style={{ width: '100%' }}
                        value={realmTargetValue(rule.primary) || undefined}
                        placeholder="选择 Client · Realm 监听端口"
                        options={targetOptions.map((option) => ({ ...option, disabled: selectedTargets.has(option.value) && option.value !== realmTargetValue(rule.primary) }))}
                        filterOption={(input, option) => String(option?.label || '').toLowerCase().includes(input.toLowerCase())}
                        onChange={(value) => selectPrimary(ruleIndex, value)}
                      />
                    </Col>
                    <Col xs={12} md={4}>
                      <Text type="secondary">检查间隔（秒）</Text>
                      <InputNumber style={{ width: '100%' }} min={1} max={300} precision={0} value={rule.check_interval_seconds || 3} onChange={(value) => updateRule(ruleIndex, { check_interval_seconds: Number(value || 3) })} />
                    </Col>
                    <Col xs={12} md={4}>
                      <Text type="secondary">连接超时（秒）</Text>
                      <InputNumber style={{ width: '100%' }} min={1} max={60} precision={0} value={rule.connect_timeout_seconds || 5} onChange={(value) => updateRule(ruleIndex, { connect_timeout_seconds: Number(value || 5) })} />
                    </Col>
                    <Col xs={12} md={4}>
                      <Text type="secondary">失败次数</Text>
                      <InputNumber style={{ width: '100%' }} min={1} max={20} precision={0} value={rule.fall || 3} onChange={(value) => updateRule(ruleIndex, { fall: Number(value || 3) })} />
                    </Col>
                    <Col xs={12} md={4}>
                      <Text type="secondary">恢复次数</Text>
                      <InputNumber style={{ width: '100%' }} min={1} max={20} precision={0} value={rule.rise || 2} onChange={(value) => updateRule(ruleIndex, { rise: Number(value || 2) })} />
                    </Col>
                    <Col xs={24} md={8}>
                      <Space direction="vertical" size={8} style={{ width: '100%' }}>
                        <Space style={{ width: '100%', justifyContent: 'space-between' }}>
                          <Text type="secondary">备用节点（按顺序接管）</Text>
                          <Button size="small" icon={<PlusOutlined />} onClick={() => addBackup(ruleIndex)}>添加备用</Button>
                        </Space>
                        {(rule.backups || []).map((target, backupIndex) => (
                          <Space.Compact key={`${rule.id || ruleIndex}-backup-${backupIndex}`} style={{ width: '100%' }}>
                            <Select
                              allowClear
                              showSearch
                              style={{ width: 'calc(100% - 34px)' }}
                              value={realmTargetValue(target) || undefined}
                              placeholder={`备用 ${backupIndex + 1}`}
                              options={targetOptions.map((option) => ({ ...option, disabled: selectedTargets.has(option.value) && option.value !== realmTargetValue(target) }))}
                              filterOption={(input, option) => String(option?.label || '').toLowerCase().includes(input.toLowerCase())}
                              onChange={(value) => selectBackup(ruleIndex, backupIndex, value)}
                            />
                            <Button danger icon={<DeleteOutlined />} title="删除备用" onClick={() => removeBackup(ruleIndex, backupIndex)} />
                          </Space.Compact>
                        ))}
                      </Space>
                    </Col>
                  </Row>
                </List.Item>
              )
            }}
          />
        </Col>
      </Row>
    </Card>
  )
}

function buildRealmTargetOptions(agents: AgentListItem[], sourceAgentID: string): RealmTargetOption[] {
  return agents
    .filter((agent) => agent.agent_id && agent.agent_id !== sourceAgentID && agent.entry?.port_forwarding?.enabled !== false)
    .flatMap((agent) => (agent.entry?.port_forwarding?.rules || [])
      .filter((rule) => rule.enabled !== false && Number(rule.listen_port || 0) > 0 && (rule.network || 'tcp').toLowerCase() !== 'udp')
      .map((rule, index) => {
        const realmRuleID = rule.id || `realm-port-${rule.listen_port || 0}-${index}`
        const target: HAProxyRealmTarget = {
          agent_id: agent.agent_id,
          realm_rule_id: realmRuleID,
          address: bestAgentAddress(agent),
          port: Number(rule.listen_port || 0),
        }
        return {
          value: realmTargetValue(target),
          label: `${agent.agent_name || agent.agent_id} · Realm :${rule.listen_port || 0}${rule.name ? ` · ${rule.name}` : ''}`,
          target,
        }
      }))
}

function targetFromValue(options: RealmTargetOption[], value?: string): HAProxyRealmTarget {
  return options.find((option) => option.value === value)?.target || emptyTarget()
}

function realmTargetValue(target?: HAProxyRealmTarget): string {
  if (!target?.agent_id || (!target.realm_rule_id && !target.port)) return ''
  return JSON.stringify([target.agent_id, target.realm_rule_id || '', Number(target.port || 0)])
}

function emptyTarget(): HAProxyRealmTarget {
  return { agent_id: '', realm_rule_id: '', address: '', port: 0 }
}

function bestAgentAddress(agent: AgentListItem): string {
  return agent.entry?.import_domain || agent.entry?.addresses?.find(Boolean) || agent.summary?.observed_ip || agent.summary?.public_ipv4 || agent.summary?.public_ipv6 || ''
}
