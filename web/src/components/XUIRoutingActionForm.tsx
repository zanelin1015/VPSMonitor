import { Card, Col, Input, Row, Select, Space, Tag, Typography } from 'antd'

import type { AgentListItem, XUIBalancerView, XUIClientView, XUINodeView, XUIOverview, XUIRoutingRuleView } from '../types'
import type { XUIOutboundActionForm, XUIRoutingActionForm } from '../lib/appHelpers'
import { renderOutboundActionForm } from './XUIOutboundActionForm'

const { Text, Title } = Typography

export function renderRoutingActionForm(props: {
  form: XUIRoutingActionForm
  outboundForm: XUIOutboundActionForm
  agents: AgentListItem[]
  targetAgentID: string
  currentOverview: XUIOverview | null
  sourceOverview: XUIOverview | null
  sourceLoading: boolean
  inbounds: XUINodeView[]
  clients: XUIClientView[]
  outbounds: { tag?: string }[]
  balancers: XUIBalancerView[]
  rules: XUIRoutingRuleView[]
  onChange: (form: XUIRoutingActionForm) => void
  onOutboundChange: (form: XUIOutboundActionForm) => void
}) {
  const {
    form,
    outboundForm,
    agents,
    targetAgentID,
    currentOverview,
    sourceOverview,
    sourceLoading,
    inbounds,
    clients,
    outbounds,
    balancers,
    rules,
    onChange,
    onOutboundChange,
  } = props
  const update = (patch: Partial<XUIRoutingActionForm>) => onChange({ ...form, ...patch })
  const updateOutbound = (next: XUIOutboundActionForm) => {
    onOutboundChange(next)
    onChange({ ...form, outbound_tag: next.tag, balancer_tag: '' })
  }
  const applyRule = (ruleIndex: number | null) => {
    if (!ruleIndex) {
      update({ rule_index: null, previous_outbound_tag: '' })
      return
    }
    const rule = rules.find((item) => item.index === ruleIndex)
    if (!rule) {
      update({ rule_index: ruleIndex, previous_outbound_tag: '' })
      return
    }
    onChange({
      ...form,
      rule_index: rule.index,
      previous_outbound_tag: rule.outbound_tag || '',
      target_mode: 'existing_outbound',
      outbound_tag: rule.outbound_tag || '',
      balancer_tag: rule.balancer_tag || '',
      inbound_tags: rule.inbound_tags || [],
      users: rule.users || [],
      domains: (rule.domain || []).join('\n'),
      ips: (rule.ip || []).join('\n'),
      ports: (rule.port || []).join(', '),
      source_ips: (rule.source_ip || []).join('\n'),
      source_ports: (rule.source_port || []).join(', '),
      networks: rule.network || [],
      protocols: rule.protocol || [],
    })
  }
  const ruleOptions = [
    { value: 0, label: '新增转发规则', search: '新增 转发 规则 add new create' },
    ...rules.map((rule) => ({
      value: rule.index,
      label: `修改 #${rule.index} · ${rule.summary || rule.outbound_tag || rule.balancer_tag || '未命名规则'}`,
      search: routingRuleSearchText(rule),
    })),
  ]

  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Card className="config-section-card" bordered={false}>
        <Title level={4}>转发规则</Title>
        <Row gutter={[16, 16]}>
          <Col xs={24} md={12}>
            <Text type="secondary">操作</Text>
            <Select
              showSearch
              style={{ width: '100%' }}
              value={form.rule_index || 0}
              options={ruleOptions}
              optionFilterProp="search"
              filterOption={filterSelectOption}
              placeholder="输入编号 / 用户 / 出站 / 域名搜索规则"
              onChange={(value) => applyRule(Number(value) || null)}
            />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">目标类型</Text>
            <Select
              style={{ width: '100%' }}
              value={form.target_mode}
              options={[
                { value: 'existing_outbound', label: '已存在出站规则' },
                { value: 'registered_client', label: '已注册 Client 节点' },
              ]}
              onChange={(value: XUIRoutingActionForm['target_mode']) =>
                update({
                  target_mode: value,
                  outbound_tag: value === 'registered_client' ? outboundForm.tag : form.outbound_tag,
                  balancer_tag: '',
                })}
            />
          </Col>
          {form.target_mode === 'existing_outbound' ? (
            <Col xs={24} md={balancers.length ? 12 : 24}>
              <Text type="secondary">Outbound Tag</Text>
              <Select
                allowClear
                showSearch
                style={{ width: '100%' }}
                value={form.outbound_tag || undefined}
                options={outbounds.filter((item) => item.tag).map((item) => ({ value: item.tag as string, label: item.tag as string }))}
                filterOption={filterSelectOption}
                onChange={(value) => update({ outbound_tag: value || '', balancer_tag: '' })}
              />
            </Col>
          ) : null}
          {form.target_mode === 'existing_outbound' && balancers.length ? (
            <Col xs={24} md={12}>
              <Text type="secondary">Balancer Tag</Text>
              <Select
                allowClear
                showSearch
                style={{ width: '100%' }}
                value={form.balancer_tag || undefined}
                options={balancers.filter((item) => item.tag).map((item) => ({ value: item.tag as string, label: item.tag as string }))}
                filterOption={filterSelectOption}
                onChange={(value) => update({ balancer_tag: value || '', outbound_tag: value ? '' : form.outbound_tag })}
              />
            </Col>
          ) : null}
          {form.target_mode === 'registered_client' ? (
            <Col xs={24}>
              {renderOutboundActionForm({
                form: outboundForm,
                agents,
                targetAgentID,
                currentOverview,
                sourceOverview,
                sourceLoading,
                onChange: updateOutbound,
              })}
            </Col>
          ) : null}
          <Col xs={24}>
            <div className="switch-row">
              <span>提交后重启 Xray</span>
              <Tag color="success">自动执行</Tag>
            </div>
          </Col>
        </Row>
      </Card>

      <Card className="config-section-card" bordered={false}>
        <Title level={4}>匹配条件</Title>
        <Row gutter={[16, 16]}>
          <Col xs={24}>
            <Text type="secondary">匹配入站</Text>
            <Select
              mode="multiple"
              showSearch
              style={{ width: '100%' }}
              value={form.inbound_tags}
              options={inbounds.map((inbound) => ({
                value: inbound.tag || String(inbound.id),
                label: `${inbound.remark || inbound.tag || inbound.id} · ${inbound.tag || '-'}`,
                search: [inbound.id, inbound.tag, inbound.remark, inbound.protocol, inbound.port].filter(Boolean).join(' '),
              }))}
              filterOption={filterSelectOption}
              onChange={(value) => update({ inbound_tags: value })}
            />
          </Col>
          <Col xs={24}>
            <Text type="secondary">匹配用户</Text>
            <Select
              mode="multiple"
              showSearch
              style={{ width: '100%' }}
              value={form.users}
              options={clients.filter((client) => client.email).map((client) => ({
                value: client.email as string,
                label: `${client.email as string} · ${client.inbound_remark || client.inbound_tag || '-'}`,
                search: [client.email, client.comment, client.inbound_remark, client.inbound_tag, client.protocol].filter(Boolean).join(' '),
              }))}
              filterOption={filterSelectOption}
              onChange={(value) => update({ users: value })}
            />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">域名</Text>
            <Input.TextArea value={form.domains} autoSize={{ minRows: 3, maxRows: 6 }} onChange={(event) => update({ domains: event.target.value })} />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">IP / CIDR</Text>
            <Input.TextArea value={form.ips} autoSize={{ minRows: 3, maxRows: 6 }} onChange={(event) => update({ ips: event.target.value })} />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">端口</Text>
            <Input value={form.ports} placeholder="443, 8443" onChange={(event) => update({ ports: event.target.value })} />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">源端口</Text>
            <Input value={form.source_ports} placeholder="10000-20000" onChange={(event) => update({ source_ports: event.target.value })} />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">源 IP</Text>
            <Input.TextArea value={form.source_ips} autoSize={{ minRows: 3, maxRows: 6 }} onChange={(event) => update({ source_ips: event.target.value })} />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">网络协议</Text>
            <Select
              mode="multiple"
              showSearch
              style={{ width: '100%' }}
              value={form.networks}
              options={[
                { value: 'tcp', label: 'tcp' },
                { value: 'udp', label: 'udp' },
              ]}
              filterOption={filterSelectOption}
              onChange={(value) => update({ networks: value })}
            />
          </Col>
          <Col xs={24}>
            <Text type="secondary">协议类型</Text>
            <Select
              mode="multiple"
              showSearch
              style={{ width: '100%' }}
              value={form.protocols}
              options={[
                { value: 'bittorrent', label: 'bittorrent' },
                { value: 'http', label: 'http' },
                { value: 'tls', label: 'tls' },
              ]}
              filterOption={filterSelectOption}
              onChange={(value) => update({ protocols: value })}
            />
          </Col>
        </Row>
      </Card>
    </Space>
  )
}

function routingRuleSearchText(rule: XUIRoutingRuleView): string {
  return [
    `#${rule.index}`,
    `rule ${rule.index}`,
    rule.summary,
    rule.outbound_tag,
    rule.balancer_tag,
    ...(rule.inbound_tags || []),
    ...(rule.users || []),
    ...(rule.domain || []),
    ...(rule.ip || []),
    ...(rule.port || []),
    ...(rule.source_port || []),
    ...(rule.source_ip || []),
    ...(rule.network || []),
    ...(rule.protocol || []),
    ...(rule.vless_route || []),
  ].filter(Boolean).join(' ')
}

function filterSelectOption(input: string, option?: { label?: unknown; value?: unknown; search?: unknown }): boolean {
  const needle = normalizeSearchText(input)
  if (!needle) {
    return true
  }
  return normalizeSearchText([option?.label, option?.value, option?.search].filter(Boolean).join(' ')).includes(needle)
}

function normalizeSearchText(value: unknown): string {
  return String(value || '').trim().toLowerCase()
}
