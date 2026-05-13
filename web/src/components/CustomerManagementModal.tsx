import { useEffect, useMemo, useRef, useState } from 'react'
import { Alert, App as AntdApp, Button, Card, Col, Empty, Input, InputNumber, Modal, Popconfirm, Row, Select, Space, Spin, Switch, Table, Tag, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { DeleteOutlined, EditOutlined, ExportOutlined, PlusOutlined, ReloadOutlined, SaveOutlined } from '@ant-design/icons'

import type { CustomerAdminView, CustomerAssignment, CustomerAssignmentDraft, DashboardAgentView, XUIClientBillingConfig, XUIClientView, XUINodeView, XUIOverview } from '../types'
import { fetchJSON } from '../lib/appHelpers'
import { REVENUE_CURRENCIES } from '../lib/currency'

const { Text, Title } = Typography

interface CustomerFormState {
  username: string
  password: string
  display_name: string
  enabled: boolean
}

interface AssignmentFormState {
  agent_id: string
  client_key: string
  inbound_id: number
  inbound_tag: string
  client_email: string
  public_client_name: string
  revenue_amount: number
  revenue_currency: 'CNY' | 'USDT'
  revenue_cycle: 'month' | 'quarter' | 'year'
  enabled: boolean
}

const emptyCustomerForm: CustomerFormState = {
  username: '',
  password: '',
  display_name: '',
  enabled: true,
}

const emptyAssignmentForm: AssignmentFormState = {
  agent_id: '',
  client_key: '',
  inbound_id: 0,
  inbound_tag: '',
  client_email: '',
  public_client_name: '',
  revenue_amount: 0,
  revenue_currency: 'CNY',
  revenue_cycle: 'month',
  enabled: true,
}

export function CustomerManagementModal(props: {
  open?: boolean
  agents: DashboardAgentView[]
  onClose?: () => void
  onConfigChanged?: () => void | Promise<void>
  onOpenAssignment?: (assignment: CustomerAssignment) => void
  initialAssignment?: CustomerAssignmentDraft | null
  onInitialAssignmentApplied?: () => void
  embedded?: boolean
}) {
  const {
    open = false,
    agents,
    onClose = () => undefined,
    onConfigChanged,
    onOpenAssignment,
    initialAssignment = null,
    onInitialAssignmentApplied,
    embedded = false,
  } = props
  const active = embedded || open
  const { message } = AntdApp.useApp()
  const [customers, setCustomers] = useState<CustomerAdminView[]>([])
  const [loading, setLoading] = useState(false)
  const [savingCustomer, setSavingCustomer] = useState(false)
  const [savingAssignment, setSavingAssignment] = useState(false)
  const [selectedCustomerID, setSelectedCustomerID] = useState<number | null>(null)
  const [editingAssignmentID, setEditingAssignmentID] = useState<number | null>(null)
  const [customerForm, setCustomerForm] = useState<CustomerFormState>(emptyCustomerForm)
  const [assignmentForm, setAssignmentForm] = useState<AssignmentFormState>(emptyAssignmentForm)
  const [overview, setOverview] = useState<XUIOverview | null>(null)
  const [overviewLoading, setOverviewLoading] = useState(false)
  const skipAssignmentResetRef = useRef(false)

  const selectedCustomer = customers.find((item) => item.id === selectedCustomerID) || null
  const agentOptions = useMemo(() => agents.map((agent) => ({
    value: agent.agent_id,
    label: agent.agent_name || agent.agent_id,
  })), [agents])
  const clientOptions = useMemo(() => {
    const clients = (overview?.clients || []).map((client) => ({
      value: clientKey(client),
      label: clientLabel(client),
      client,
      node: undefined as XUINodeView | undefined,
    }))
    const nodes = (overview?.nodes || []).map((node) => ({
      value: nodeKey(node),
      label: nodeLabel(node),
      client: undefined as XUIClientView | undefined,
      node,
    }))
    return [...clients, ...nodes]
  }, [overview])

  useEffect(() => {
    if (active) {
      void loadCustomers()
    }
  }, [active])

  useEffect(() => {
    if (!selectedCustomer) {
      setCustomerForm(emptyCustomerForm)
      setEditingAssignmentID(null)
      if (skipAssignmentResetRef.current) {
        skipAssignmentResetRef.current = false
        return
      }
      return
    }
    setCustomerForm({
      username: selectedCustomer.username,
      password: '',
      display_name: selectedCustomer.display_name || selectedCustomer.username,
      enabled: selectedCustomer.enabled,
    })
    if (skipAssignmentResetRef.current) {
      skipAssignmentResetRef.current = false
      return
    }
    setEditingAssignmentID(null)
    setAssignmentForm(emptyAssignmentForm)
  }, [selectedCustomerID])

  useEffect(() => {
    if (!active || !initialAssignment) {
      return
    }
    const match = findMatchingAssignment(customers, initialAssignment)
    const nextCustomerID = match?.customer.id || selectedCustomerID || customers[0]?.id || null
    const nextForm = match?.assignment
      ? assignmentFormFromAssignment(match.assignment, agents)
      : assignmentFormFromDraft(initialAssignment, agents)
    if (nextCustomerID !== selectedCustomerID) {
      skipAssignmentResetRef.current = true
      setSelectedCustomerID(nextCustomerID)
    }
    setEditingAssignmentID(match?.assignment.id || null)
    setAssignmentForm(nextForm)
    onInitialAssignmentApplied?.()
  }, [active, agents, customers, initialAssignment, onInitialAssignmentApplied, selectedCustomerID])

  useEffect(() => {
    if (!assignmentForm.agent_id) {
      setOverview(null)
      return
    }
    let cancelled = false
    setOverviewLoading(true)
    void fetchJSON<XUIOverview>(`/api/v1/agents/${assignmentForm.agent_id}/xui/overview`)
      .then((data) => {
        if (!cancelled) {
          setOverview(data)
        }
      })
      .catch((error) => {
        if (!cancelled) {
          setOverview(null)
          message.warning(error instanceof Error ? error.message : '加载 x-ui 客户端失败')
        }
      })
      .finally(() => {
        if (!cancelled) {
          setOverviewLoading(false)
        }
      })
    return () => {
      cancelled = true
    }
  }, [assignmentForm.agent_id])

  const assignmentColumns: ColumnsType<CustomerAssignment> = [
    {
      title: '客户可见名称',
      dataIndex: 'public_client_name',
      width: 180,
      render: (value?: string) => <Text strong>{value || '-'}</Text>,
    },
    {
      title: 'Client / 节点',
      width: 260,
      render: (_, record) => (
        <div>
          <Text>{agentName(record.agent_id, agents)}</Text>
          <div className="muted-line">{record.inbound_tag || `Inbound #${record.inbound_id}`} / {record.client_email || '未指定客户端'}</div>
        </div>
      ),
    },
    {
      title: '客户备注',
      dataIndex: 'remark',
      ellipsis: true,
      render: (value?: string) => value || '-',
    },
    {
      title: '节点费用',
      key: 'revenue',
      width: 150,
      render: (_, record) => {
        const billing = assignmentBilling(record, agents)
        return billing && Number(billing.revenue_amount || 0) > 0
          ? `${billing.revenue_currency || 'CNY'} ${Number(billing.revenue_amount || 0).toFixed(2)} / ${revenueCycleLabel(billing.revenue_cycle)}`
          : <Tag>未设置</Tag>
      },
    },
    {
      title: '状态',
      dataIndex: 'enabled',
      width: 90,
      render: (enabled: boolean) => <Tag color={enabled ? 'blue' : 'default'}>{enabled ? '启用' : '停用'}</Tag>,
    },
    {
      title: '操作',
      width: 220,
      render: (_, record) => (
        <Space size={6}>
          {onOpenAssignment ? (
            <Button size="small" icon={<ExportOutlined />} onClick={() => onOpenAssignment(record)}>
              跳转
            </Button>
          ) : null}
          <Button size="small" icon={<EditOutlined />} onClick={() => editAssignment(record)}>编辑</Button>
          <Popconfirm title="删除这条分配？" okText="删除" cancelText="取消" onConfirm={() => void deleteAssignment(record.id)}>
            <Button size="small" danger icon={<DeleteOutlined />} />
          </Popconfirm>
        </Space>
      ),
    },
  ]

  async function loadCustomers() {
    setLoading(true)
    try {
      const data = await fetchJSON<CustomerAdminView[]>('/api/v1/admin/customers')
      setCustomers(data)
      setSelectedCustomerID((current) => {
        if (current && data.some((item) => item.id === current)) {
          return current
        }
        return data[0]?.id || null
      })
    } catch (error) {
      message.error(error instanceof Error ? error.message : '加载人员失败')
    } finally {
      setLoading(false)
    }
  }

  async function saveCustomer() {
    if (!customerForm.username.trim()) {
      message.warning('请填写客户用户名')
      return
    }
    if (!selectedCustomerID && customerForm.password.length < 8) {
      message.warning('新客户密码至少 8 位')
      return
    }
    setSavingCustomer(true)
    try {
      const payload = {
        username: customerForm.username.trim(),
        password: customerForm.password,
        display_name: customerForm.display_name.trim(),
        enabled: customerForm.enabled,
      }
      if (selectedCustomerID) {
        await fetchJSON<CustomerAdminView>(`/api/v1/admin/customers/${selectedCustomerID}`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        })
        message.success('客户已更新')
      } else {
        const created = await fetchJSON<CustomerAdminView>('/api/v1/admin/customers', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        })
        setSelectedCustomerID(created.id)
        message.success('客户已创建')
      }
      setCustomerForm((current) => ({ ...current, password: '' }))
      await loadCustomers()
    } catch (error) {
      message.error(error instanceof Error ? error.message : '保存客户失败')
    } finally {
      setSavingCustomer(false)
    }
  }

  async function deleteCustomer() {
    if (!selectedCustomerID) {
      return
    }
    setSavingCustomer(true)
    try {
      await fetchJSON(`/api/v1/admin/customers/${selectedCustomerID}`, { method: 'DELETE' })
      setSelectedCustomerID(null)
      setCustomerForm(emptyCustomerForm)
      message.success('客户已删除')
      await loadCustomers()
    } catch (error) {
      message.error(error instanceof Error ? error.message : '删除客户失败')
    } finally {
      setSavingCustomer(false)
    }
  }

  async function saveAssignment() {
    if (!selectedCustomerID) {
      message.warning('请先选择客户')
      return
    }
    if (!assignmentForm.agent_id || !assignmentForm.inbound_id) {
      message.warning('请选择 Client 和节点')
      return
    }
    setSavingAssignment(true)
    try {
      const payload = {
        agent_id: assignmentForm.agent_id,
        inbound_id: assignmentForm.inbound_id,
        inbound_tag: assignmentForm.inbound_tag,
        client_email: assignmentForm.client_email,
        public_client_name: assignmentForm.public_client_name,
        revenue_amount: assignmentForm.revenue_amount,
        revenue_currency: assignmentForm.revenue_currency,
        revenue_cycle: assignmentForm.revenue_cycle,
        enabled: assignmentForm.enabled,
      }
      if (editingAssignmentID) {
        await fetchJSON<CustomerAssignment>(`/api/v1/admin/customers/${selectedCustomerID}/assignments/${editingAssignmentID}`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        })
        message.success('分配已更新')
      } else {
        await fetchJSON<CustomerAssignment>(`/api/v1/admin/customers/${selectedCustomerID}/assignments`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        })
        message.success('分配已新增')
      }
      setEditingAssignmentID(null)
      setAssignmentForm(emptyAssignmentForm)
      await onConfigChanged?.()
      await loadCustomers()
    } catch (error) {
      message.error(error instanceof Error ? error.message : '保存分配失败')
    } finally {
      setSavingAssignment(false)
    }
  }

  async function deleteAssignment(id: number) {
    if (!selectedCustomerID) {
      return
    }
    setSavingAssignment(true)
    try {
      await fetchJSON(`/api/v1/admin/customers/${selectedCustomerID}/assignments/${id}`, { method: 'DELETE' })
      message.success('分配已删除')
      await loadCustomers()
    } catch (error) {
      message.error(error instanceof Error ? error.message : '删除分配失败')
    } finally {
      setSavingAssignment(false)
    }
  }

  function editAssignment(record: CustomerAssignment) {
    setEditingAssignmentID(record.id)
    setAssignmentForm(assignmentFormFromAssignment(record, agents))
  }

  function selectClient(key: string) {
    const option = clientOptions.find((item) => item.value === key)
    if (!option) {
      setAssignmentForm((current) => ({ ...current, client_key: key }))
      return
    }
    if (option.client) {
      const client = option.client
      setAssignmentForm((current) => ({
        ...current,
        client_key: key,
        inbound_id: client.inbound_id,
        inbound_tag: client.inbound_tag || '',
        client_email: client.email || '',
        public_client_name: current.public_client_name || defaultPublicClientName(client, assignmentForm.agent_id, agents),
        ...billingFormPatch(clientBilling(assignmentForm.agent_id, client.inbound_id, client.inbound_tag || '', client.email || '', agents)),
      }))
      return
    }
    if (option.node) {
      const node = option.node
      setAssignmentForm((current) => ({
        ...current,
        client_key: key,
        inbound_id: node.id,
        inbound_tag: node.tag || '',
        client_email: '',
        public_client_name: current.public_client_name || defaultPublicNodeName(node, assignmentForm.agent_id, agents),
        ...billingFormPatch(clientBilling(assignmentForm.agent_id, node.id, node.tag || '', '', agents)),
      }))
    }
  }

  const content = (
      <Spin spinning={loading}>
        <Row gutter={[16, 16]}>
          <Col xs={24} md={4}>
            <Card className="customer-admin-card" bordered={false}>
              <div className="customer-admin-card-head">
                <Title level={5}>客户列表</Title>
                <Space>
                  <Button size="small" icon={<ReloadOutlined />} onClick={() => void loadCustomers()} />
                  <Button size="small" type="primary" icon={<PlusOutlined />} onClick={() => {
                    setSelectedCustomerID(null)
                    setCustomerForm(emptyCustomerForm)
                    setAssignmentForm(emptyAssignmentForm)
                    setEditingAssignmentID(null)
                  }}>新建</Button>
                </Space>
              </div>
              <Space direction="vertical" style={{ width: '100%' }}>
                {!customers.length ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无客户" /> : null}
                {customers.map((customer) => (
                  <button
                    key={customer.id}
                    type="button"
                    className={`customer-admin-list-item${selectedCustomerID === customer.id ? ' active' : ''}`}
                    onClick={() => setSelectedCustomerID(customer.id)}
                  >
                    <span>
                      <Text strong>{customer.display_name || customer.username}</Text>
                      <span className="muted-line">{customer.username} · {customer.assignments.length} 条链路</span>
                    </span>
                    <Tag color={customer.enabled ? 'blue' : 'default'}>{customer.enabled ? '启用' : '停用'}</Tag>
                  </button>
                ))}
              </Space>
            </Card>
          </Col>

          <Col xs={24} md={20}>
            <Space direction="vertical" size="middle" style={{ width: '100%' }}>
              <Card className="customer-admin-card" bordered={false}>
                <div className="customer-admin-card-head">
                  <Title level={5}>{selectedCustomerID ? '编辑客户' : '新建客户'}</Title>
                  {selectedCustomerID ? (
                    <Popconfirm title="删除该客户及其全部分配？" okText="删除" cancelText="取消" onConfirm={() => void deleteCustomer()}>
                      <Button danger icon={<DeleteOutlined />}>删除客户</Button>
                    </Popconfirm>
                  ) : null}
                </div>
                <Row gutter={[12, 12]}>
                  <Col xs={24} md={8}>
                    <Text type="secondary">登录用户名</Text>
                    <Input value={customerForm.username} onChange={(event) => setCustomerForm((current) => ({ ...current, username: event.target.value }))} />
                  </Col>
                  <Col xs={24} md={8}>
                    <Text type="secondary">客户显示名</Text>
                    <Input value={customerForm.display_name} onChange={(event) => setCustomerForm((current) => ({ ...current, display_name: event.target.value }))} />
                  </Col>
                  <Col xs={24} md={8}>
                    <Text type="secondary">密码{selectedCustomerID ? '（留空不改）' : ''}</Text>
                    <Input.Password value={customerForm.password} onChange={(event) => setCustomerForm((current) => ({ ...current, password: event.target.value }))} />
                  </Col>
                  <Col xs={24} md={8}>
                    <Text type="secondary">账号状态</Text>
                    <div className="customer-admin-switch-row">
                      <Switch checked={customerForm.enabled} onChange={(checked) => setCustomerForm((current) => ({ ...current, enabled: checked }))} />
                      <Text>{customerForm.enabled ? '启用' : '停用'}</Text>
                    </div>
                  </Col>
                  <Col xs={24} md={16}>
                    <Text type="secondary">客户入口地址</Text>
                    <Input value={`${window.location.origin}/customer`} readOnly />
                  </Col>
                </Row>
                <Button style={{ marginTop: 14 }} type="primary" icon={<SaveOutlined />} loading={savingCustomer} onClick={() => void saveCustomer()}>
                  保存客户
                </Button>
              </Card>

              <Card className="customer-admin-card" bordered={false}>
                <div className="customer-admin-card-head">
                  <Title level={5}>客户端 / 节点分配</Title>
                  <Button onClick={() => {
                    setEditingAssignmentID(null)
                    setAssignmentForm(emptyAssignmentForm)
                  }}>清空表单</Button>
                </div>
                {!selectedCustomerID ? <Alert type="info" showIcon message="先选择或新建客户，再分配链路。" /> : null}
                <Row gutter={[12, 12]}>
                  <Col xs={24} md={8}>
                    <Text type="secondary">入口 Client</Text>
                    <Select
                      style={{ width: '100%' }}
                      showSearch
                      placeholder="选择 client"
                      value={assignmentForm.agent_id || undefined}
                      options={agentOptions}
                      optionFilterProp="label"
                      onChange={(value) => setAssignmentForm({ ...emptyAssignmentForm, agent_id: value })}
                    />
                  </Col>
                  <Col xs={24} md={10}>
                    <Text type="secondary">节点 / 客户端</Text>
                    <Select
                      style={{ width: '100%' }}
                      showSearch
                      placeholder="选择 x-ui 客户端"
                      value={assignmentForm.client_key || undefined}
                      loading={overviewLoading}
                      disabled={!assignmentForm.agent_id}
                      options={clientOptions.map(({ value, label }) => ({ value, label }))}
                      optionFilterProp="label"
                      onChange={selectClient}
                    />
                  </Col>
                  <Col xs={24} md={6}>
                    <Text type="secondary">客户可见名称</Text>
                    <Input value={assignmentForm.public_client_name} onChange={(event) => setAssignmentForm((current) => ({ ...current, public_client_name: event.target.value }))} />
                  </Col>
                  <Col xs={24} md={4}>
                    <Text type="secondary">节点费用</Text>
                    <InputNumber
                      style={{ width: '100%' }}
                      min={0}
                      precision={2}
                      value={assignmentForm.revenue_amount}
                      onChange={(value) => setAssignmentForm((current) => ({ ...current, revenue_amount: Number(value || 0) }))}
                    />
                  </Col>
                  <Col xs={12} md={2}>
                    <Text type="secondary">币种</Text>
                    <Select
                      style={{ width: '100%' }}
                      value={assignmentForm.revenue_currency}
                      options={REVENUE_CURRENCIES.map((currency) => ({ value: currency, label: currency }))}
                      onChange={(value) => setAssignmentForm((current) => ({ ...current, revenue_currency: value as 'CNY' | 'USDT' }))}
                    />
                  </Col>
                  <Col xs={12} md={2}>
                    <Text type="secondary">周期</Text>
                    <Select
                      style={{ width: '100%' }}
                      value={assignmentForm.revenue_cycle}
                      options={[
                        { value: 'month', label: '月' },
                        { value: 'quarter', label: '季' },
                        { value: 'year', label: '年' },
                      ]}
                      onChange={(value) => setAssignmentForm((current) => ({ ...current, revenue_cycle: value as 'month' | 'quarter' | 'year' }))}
                    />
                  </Col>
                  <Col xs={24} md={8}>
                    <Text type="secondary">分配状态</Text>
                    <div className="customer-admin-switch-row">
                      <Switch checked={assignmentForm.enabled} onChange={(checked) => setAssignmentForm((current) => ({ ...current, enabled: checked }))} />
                      <Text>{assignmentForm.enabled ? '启用' : '停用'}</Text>
                    </div>
                  </Col>
                  <Col xs={24} md={16}>
                    <Text type="secondary">选择结果</Text>
                    <Input value={assignmentForm.inbound_id ? `${assignmentForm.inbound_tag || `Inbound #${assignmentForm.inbound_id}`} / ${assignmentForm.client_email || '未指定客户端'}` : ''} readOnly />
                  </Col>
                </Row>
                <Button style={{ marginTop: 14 }} type="primary" icon={<SaveOutlined />} disabled={!selectedCustomerID} loading={savingAssignment} onClick={() => void saveAssignment()}>
                  {editingAssignmentID ? '保存分配' : '新增分配'}
                </Button>
                <Table
                  style={{ marginTop: 14 }}
                  rowKey={(record) => record.id}
                  columns={assignmentColumns}
                  dataSource={selectedCustomer?.assignments || []}
                  pagination={{ pageSize: 5, hideOnSinglePage: true }}
                  locale={{ emptyText: <Empty description="暂无分配" /> }}
                />
              </Card>
            </Space>
          </Col>
        </Row>
      </Spin>
  )

  if (embedded) {
    return (
      <div id="customer-management-panel" className="customer-admin-page">
        <div className="admin-content-title">
          <div>
            <Title level={3}>客户管理</Title>
            <Text type="secondary">管理客户账号、客户可见名称，以及分配给客户的 Client / 节点链路。</Text>
          </div>
          <Button icon={<ReloadOutlined />} loading={loading} onClick={() => void loadCustomers()}>刷新</Button>
        </div>
        {content}
      </div>
    )
  }

  return (
    <Modal title="人员管理 / 客户链路分配" open={open} onCancel={onClose} footer={null} width={1160} destroyOnClose>
      {content}
    </Modal>
  )
}

function clientKey(client: XUIClientView): string {
  return `client:${client.inbound_id}::${client.email || ''}`
}

function clientLabel(client: XUIClientView): string {
  const inbound = client.inbound_remark || client.inbound_tag || `Inbound #${client.inbound_id}`
  const email = client.email || '未指定客户端'
  return `客户端：${inbound} / ${email}`
}

function nodeKey(node: XUINodeView): string {
  return `node:${node.id}::`
}

function nodeLabel(node: XUINodeView): string {
  return `节点：${node.remark || node.tag || `Inbound #${node.id}`} / ${node.protocol || '-'}`
}

function defaultPublicClientName(client: XUIClientView, agentID: string, agents: DashboardAgentView[]): string {
  const agent = customerAgentName(agentID, agents)
  return [agent, client.email || client.inbound_remark || client.inbound_tag].filter(Boolean).join(' - ')
}

function defaultPublicNodeName(node: XUINodeView, agentID: string, agents: DashboardAgentView[]): string {
  const agent = customerAgentName(agentID, agents)
  return [agent, node.remark || node.tag || `Inbound #${node.id}`].filter(Boolean).join(' - ')
}

function agentName(agentID: string, agents: DashboardAgentView[]): string {
  const agent = agents.find((item) => item.agent_id === agentID)
  return agent?.agent_name || agentID
}

function customerAgentName(agentID: string, agents: DashboardAgentView[]): string {
  const agent = agents.find((item) => item.agent_id === agentID)
  return agent?.customer_display_name || agent?.agent_name || agentID
}

function assignmentBilling(record: CustomerAssignment, agents: DashboardAgentView[]): XUIClientBillingConfig | undefined {
  return clientBilling(record.agent_id, record.inbound_id, record.inbound_tag || '', record.client_email || '', agents)
}

function assignmentFormFromAssignment(record: CustomerAssignment, agents: DashboardAgentView[]): AssignmentFormState {
  const billing = assignmentBilling(record, agents)
  return {
    agent_id: record.agent_id,
    client_key: record.client_email ? `client:${record.inbound_id}::${record.client_email}` : `node:${record.inbound_id}::`,
    inbound_id: record.inbound_id,
    inbound_tag: record.inbound_tag || '',
    client_email: record.client_email || '',
    public_client_name: record.public_client_name || '',
    revenue_amount: Number(billing?.revenue_amount || 0),
    revenue_currency: billing?.revenue_currency === 'USDT' ? 'USDT' : 'CNY',
    revenue_cycle: billing?.revenue_cycle === 'quarter' || billing?.revenue_cycle === 'year' ? billing.revenue_cycle : 'month',
    enabled: record.enabled,
  }
}

function assignmentFormFromDraft(draft: CustomerAssignmentDraft, agents: DashboardAgentView[]): AssignmentFormState {
  const inboundTag = draft.inbound_tag || ''
  const clientEmail = draft.client_email || ''
  const billing = clientBilling(draft.agent_id, draft.inbound_id, inboundTag, clientEmail, agents)
  const publicClientName = draft.public_client_name || defaultPublicNameFromDraft(draft, agents)
  return {
    agent_id: draft.agent_id,
    client_key: clientEmail ? `client:${draft.inbound_id}::${clientEmail}` : `node:${draft.inbound_id}::`,
    inbound_id: draft.inbound_id,
    inbound_tag: inboundTag,
    client_email: clientEmail,
    public_client_name: publicClientName,
    revenue_amount: Number(draft.revenue_amount ?? billing?.revenue_amount ?? 0),
    revenue_currency: draft.revenue_currency === 'USDT' ? 'USDT' : billing?.revenue_currency === 'USDT' ? 'USDT' : 'CNY',
    revenue_cycle: draft.revenue_cycle === 'quarter' || draft.revenue_cycle === 'year'
      ? draft.revenue_cycle
      : billing?.revenue_cycle === 'quarter' || billing?.revenue_cycle === 'year'
        ? billing.revenue_cycle
        : 'month',
    enabled: true,
  }
}

function defaultPublicNameFromDraft(draft: CustomerAssignmentDraft, agents: DashboardAgentView[]): string {
  const agent = customerAgentName(draft.agent_id, agents)
  const tail = draft.client_email || draft.inbound_tag || `Inbound #${draft.inbound_id}`
  return [agent, tail].filter(Boolean).join(' - ')
}

function findMatchingAssignment(customers: CustomerAdminView[], draft: CustomerAssignmentDraft): { customer: CustomerAdminView, assignment: CustomerAssignment } | null {
  const exactKey = billingKey(draft.inbound_id, draft.inbound_tag || '', draft.client_email || '')
  const emailKey = draft.client_email ? billingEmailKey(draft.inbound_id, draft.client_email) : ''
  for (const customer of customers) {
    for (const assignment of customer.assignments || []) {
      if (assignment.agent_id !== draft.agent_id) {
        continue
      }
      if (billingKey(assignment.inbound_id, assignment.inbound_tag || '', assignment.client_email || '') === exactKey) {
        return { customer, assignment }
      }
      if (emailKey && billingEmailKey(assignment.inbound_id, assignment.client_email || '') === emailKey) {
        return { customer, assignment }
      }
    }
  }
  return null
}

function clientBilling(agentID: string, inboundID: number, inboundTag: string, email: string, agents: DashboardAgentView[]): XUIClientBillingConfig | undefined {
  const agent = agents.find((item) => item.agent_id === agentID)
  const exactKey = billingKey(inboundID, inboundTag, email)
  const emailKey = email ? billingEmailKey(inboundID, email) : ''
  return (agent?.renewal?.client_billings || []).find((billing) => {
    if (billingKey(Number(billing.inbound_id || 0), billing.inbound_tag || '', billing.email || '') === exactKey) {
      return true
    }
    return Boolean(emailKey && billingEmailKey(Number(billing.inbound_id || 0), billing.email || '') === emailKey)
  })
}

function billingFormPatch(billing?: XUIClientBillingConfig): Pick<AssignmentFormState, 'revenue_amount' | 'revenue_currency' | 'revenue_cycle'> {
  return {
    revenue_amount: Number(billing?.revenue_amount || 0),
    revenue_currency: billing?.revenue_currency === 'USDT' ? 'USDT' : 'CNY',
    revenue_cycle: billing?.revenue_cycle === 'quarter' || billing?.revenue_cycle === 'year' ? billing.revenue_cycle : 'month',
  }
}

function billingKey(inboundID: number, inboundTag: string, email: string): string {
  return `${Number(inboundID || 0)}\u0000${String(inboundTag || '').trim().toLowerCase()}\u0000${String(email || '').trim().toLowerCase()}`
}

function billingEmailKey(inboundID: number, email: string): string {
  return `${Number(inboundID || 0)}\u0000${String(email || '').trim().toLowerCase()}`
}

function revenueCycleLabel(cycle?: string): string {
  switch (cycle) {
    case 'quarter':
      return '季'
    case 'year':
      return '年'
    case 'month':
    default:
      return '月'
  }
}
