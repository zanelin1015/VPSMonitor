import { useEffect, useMemo, useRef, useState } from 'react'
import { Alert, App as AntdApp, Button, Card, Col, Empty, Input, InputNumber, Modal, Popconfirm, Row, Select, Space, Spin, Switch, Table, Tabs, Tag, TreeSelect, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { DeleteOutlined, EditOutlined, ExportOutlined, PlusOutlined, ReloadOutlined, SaveOutlined, SyncOutlined } from '@ant-design/icons'

import type { AdminUser, AreaManagerAdminView, AreaManagerAssignment, CustomerAdminView, CustomerAssignment, CustomerAssignmentDraft, DashboardAgentView, RealmForwardRule, XUIClientBillingConfig, XUIClientView, XUINodeView, XUIOverview } from '../types'
import { fetchJSON } from '../lib/appHelpers'
import { REVENUE_CURRENCIES } from '../lib/currency'

const { Text, Title } = Typography
const DEFAULT_ACCOUNT_PASSWORD = '12345678'

type ManagementTabKey = 'area' | 'customers'

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

interface AreaManagerAssignmentDraft {
  agent_id: string
  inbound_id: number
  inbound_tag: string
  client_email: string
  public_client_name: string
  enabled: boolean
}

interface AreaManagerFormState {
  username: string
  password: string
  display_name: string
  enabled: boolean
  agent_ids: string[]
  billing_enabled: boolean
  revenue_amount: number
  revenue_currency: 'CNY' | 'USDT'
  revenue_cycle: 'month' | 'quarter' | 'year'
  grant_agent_id: string
  xui_grant_agent_id: string
  assignments: AreaManagerAssignmentDraft[]
}

interface AreaBatchAssignmentFormState {
  manager_id: number | null
  agent_id: string
  xui_agent_id: string
  selected_realm_keys: string[]
  selected_xui_keys: string[]
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

const emptyAreaManagerForm: AreaManagerFormState = {
  username: '',
  password: '',
  display_name: '',
  enabled: true,
  agent_ids: [],
  billing_enabled: false,
  revenue_amount: 0,
  revenue_currency: 'CNY',
  revenue_cycle: 'month',
  grant_agent_id: '',
  xui_grant_agent_id: '',
  assignments: [],
}

const emptyAreaBatchAssignmentForm: AreaBatchAssignmentFormState = {
  manager_id: null,
  agent_id: '',
  xui_agent_id: '',
  selected_realm_keys: [],
  selected_xui_keys: [],
}

function isAreaManagerAdminUser(user: AdminUser | null): boolean {
  if (!user) {
    return false
  }
  if (user.role === 'area_manager') {
    return true
  }
  if (user.role === 'admin') {
    return false
  }
  return Boolean((user.agent_ids || []).length || (user.id && user.id !== 1))
}

export function CustomerManagementModal(props: {
  open?: boolean
  agents: DashboardAgentView[]
  onClose?: () => void
  onConfigChanged?: (agentID?: string) => void | Promise<void>
  onOpenAssignment?: (assignment: CustomerAssignment) => void
  initialAssignment?: CustomerAssignmentDraft | null
  onInitialAssignmentApplied?: () => void
  adminUser?: AdminUser | null
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
    adminUser = null,
    embedded = false,
  } = props
  const active = embedded || open
  const canManageAreaManagers = !isAreaManagerAdminUser(adminUser)
  const canViewFinance = canManageAreaManagers
  const { message } = AntdApp.useApp()
  const [activeManagementTab, setActiveManagementTab] = useState<ManagementTabKey>(canManageAreaManagers ? 'area' : 'customers')
  const [customers, setCustomers] = useState<CustomerAdminView[]>([])
  const [areaManagers, setAreaManagers] = useState<AreaManagerAdminView[]>([])
  const [loading, setLoading] = useState(false)
  const [areaManagersLoading, setAreaManagersLoading] = useState(false)
  const [savingCustomer, setSavingCustomer] = useState(false)
  const [resettingCustomerID, setResettingCustomerID] = useState<number | null>(null)
  const [savingAssignment, setSavingAssignment] = useState(false)
  const [savingAreaManager, setSavingAreaManager] = useState(false)
  const [resettingAreaManagerID, setResettingAreaManagerID] = useState<number | null>(null)
  const [savingAreaBatchAssignment, setSavingAreaBatchAssignment] = useState(false)
  const [areaManagerModalOpen, setAreaManagerModalOpen] = useState(false)
  const [customerCreateModalOpen, setCustomerCreateModalOpen] = useState(false)
  const [customerEditModalOpen, setCustomerEditModalOpen] = useState(false)
  const [assignmentManagerModalOpen, setAssignmentManagerModalOpen] = useState(false)
  const [assignmentViewModalOpen, setAssignmentViewModalOpen] = useState(false)
  const [selectedCustomerID, setSelectedCustomerID] = useState<number | null>(null)
  const [editingAreaManagerID, setEditingAreaManagerID] = useState<number | null>(null)
  const [editingAssignmentID, setEditingAssignmentID] = useState<number | null>(null)
  const [customerForm, setCustomerForm] = useState<CustomerFormState>(emptyCustomerForm)
  const [customerCreateForm, setCustomerCreateForm] = useState<CustomerFormState>(emptyCustomerForm)
  const [areaManagerForm, setAreaManagerForm] = useState<AreaManagerFormState>(emptyAreaManagerForm)
  const [assignmentForm, setAssignmentForm] = useState<AssignmentFormState>(emptyAssignmentForm)
  const [overview, setOverview] = useState<XUIOverview | null>(null)
  const [overviewLoading, setOverviewLoading] = useState(false)
  const [areaBatchForm, setAreaBatchForm] = useState<AreaBatchAssignmentFormState>(emptyAreaBatchAssignmentForm)
  const [areaBatchOverview, setAreaBatchOverview] = useState<XUIOverview | null>(null)
  const [areaBatchOverviewLoading, setAreaBatchOverviewLoading] = useState(false)
  const [areaManagerOverview, setAreaManagerOverview] = useState<XUIOverview | null>(null)
  const [areaManagerOverviewLoading, setAreaManagerOverviewLoading] = useState(false)
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
  const clientTreeData = useMemo(() => buildClientAssignmentTreeData(assignmentForm.agent_id, overview, agents), [assignmentForm.agent_id, overview, agents])
  const areaBatchXUIAgentID = areaBatchForm.xui_agent_id
  const areaBatchClientTreeData = useMemo(() => buildAreaAssignmentTreeData(areaBatchXUIAgentID, areaBatchOverview, agents), [areaBatchXUIAgentID, areaBatchOverview, agents])
  const areaBatchRealmOptions = useMemo(() => buildRealmGrantOptions(areaBatchForm.agent_id, agents), [areaBatchForm.agent_id, agents])
  const areaManagerXUIGrantAgentID = areaManagerForm.xui_grant_agent_id
  const areaManagerGrantOptions = useMemo(() => buildAssignmentTargetOptions(areaManagerOverview), [areaManagerOverview])
  const areaManagerGrantTreeData = useMemo(() => buildAreaAssignmentTreeData(areaManagerXUIGrantAgentID, areaManagerOverview, agents), [areaManagerXUIGrantAgentID, areaManagerOverview, agents])
  const areaManagerRealmGrantOptions = useMemo(() => buildRealmGrantOptions(areaManagerForm.grant_agent_id, agents), [areaManagerForm.grant_agent_id, agents])

  useEffect(() => {
    if (active) {
      void loadCustomers()
      if (canManageAreaManagers) {
        void loadAreaManagers()
      }
    }
  }, [active, canManageAreaManagers])

  useEffect(() => {
    if (!canManageAreaManagers && activeManagementTab === 'area') {
      setActiveManagementTab('customers')
    }
  }, [activeManagementTab, canManageAreaManagers])

  useEffect(() => {
    if (active && initialAssignment) {
      setActiveManagementTab('customers')
      setAssignmentManagerModalOpen(true)
    }
  }, [active, initialAssignment])

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
    void fetchJSON<XUIOverview>(`/api/v1/agents/${assignmentForm.agent_id}/xui/overview?assignment_scope=1`)
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

  useEffect(() => {
    const xuiAgentID = areaBatchForm.xui_agent_id
    if (!xuiAgentID) {
      setAreaBatchOverview(null)
      return
    }
    let cancelled = false
    setAreaBatchOverviewLoading(true)
    void fetchJSON<XUIOverview>(`/api/v1/agents/${xuiAgentID}/xui/overview?assignment_scope=1`)
      .then((data) => {
        if (!cancelled) {
          setAreaBatchOverview(data)
        }
      })
      .catch((error) => {
        if (!cancelled) {
          setAreaBatchOverview(null)
          message.warning(error instanceof Error ? error.message : '加载 x-ui 客户端失败')
        }
      })
      .finally(() => {
        if (!cancelled) {
          setAreaBatchOverviewLoading(false)
        }
      })
    return () => {
      cancelled = true
    }
  }, [areaBatchForm.xui_agent_id])

  useEffect(() => {
    if (!areaManagerXUIGrantAgentID) {
      setAreaManagerOverview(null)
      return
    }
    let cancelled = false
    setAreaManagerOverviewLoading(true)
    void fetchJSON<XUIOverview>(`/api/v1/agents/${areaManagerXUIGrantAgentID}/xui/overview?assignment_scope=1`)
      .then((data) => {
        if (!cancelled) {
          setAreaManagerOverview(data)
        }
      })
      .catch((error) => {
        if (!cancelled) {
          setAreaManagerOverview(null)
          message.warning(error instanceof Error ? error.message : '加载 x-ui 节点失败')
        }
      })
      .finally(() => {
        if (!cancelled) {
          setAreaManagerOverviewLoading(false)
        }
      })
    return () => {
      cancelled = true
    }
  }, [areaManagerXUIGrantAgentID])

  const assignmentColumns: ColumnsType<CustomerAssignment> = [
    {
      title: '授权链路名称',
      dataIndex: 'public_client_name',
      width: 180,
      render: (value?: string) => <Text strong>{value || '-'}</Text>,
    },
    {
      title: 'Client / 节点 / 客户端',
      width: 320,
      render: (_, record) => renderAssignmentHierarchy([record], agents),
    },
    {
      title: '用户备注',
      dataIndex: 'remark',
      ellipsis: true,
      render: (value?: string) => value || '-',
    },
    {
      title: '费用',
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
      key: 'actions',
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
  const visibleAssignmentColumns = canViewFinance
    ? assignmentColumns
    : assignmentColumns.filter((column) => String(column.key || '') !== 'revenue')

  const areaCustomerColumns: ColumnsType<CustomerAdminView> = [
    {
      title: '下属用户',
      width: 180,
      render: (_, record) => (
        <div>
          <Text strong>{record.username}</Text>
          <div className="muted-line">{record.display_name || record.username}</div>
        </div>
      ),
    },
    {
      title: '链路',
      render: (_, record) => {
        const assignments = record.assignments || []
        return renderAssignmentHierarchy(assignments, agents)
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
      width: 230,
      render: (_, record) => (
        <Space size={6} wrap>
          <Button size="small" onClick={() => openAssignmentManagerModal(record)}>管理链路</Button>
          <Popconfirm title={`将 ${record.username} 的密码初始化为 ${DEFAULT_ACCOUNT_PASSWORD}？`} okText="初始化" cancelText="取消" onConfirm={() => void resetCustomerPassword(record.id)}>
            <Button size="small" icon={<SyncOutlined />} loading={resettingCustomerID === record.id}>初始化密码</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  const areaManagerColumns: ColumnsType<AreaManagerAdminView> = [
    {
      title: '区域账号',
      width: 180,
      render: (_, record) => (
        <div>
          <Text strong>{record.username}</Text>
          <div className="muted-line">{record.display_name || record.username}</div>
        </div>
      ),
    },
    {
      title: '下属用户 / 链路',
      width: 150,
      render: (_, record) => {
        const ownedCustomers = record.customers || []
        const assignmentCount = ownedCustomers.reduce((sum, customer) => sum + (customer.assignments?.length || 0), 0)
        const directCount = normalizeAreaManagerAssignmentDrafts(record.assignments || []).length
        return (
          <Space size={6}>
            <Tag color="geekblue">{ownedCustomers.length} 用户</Tag>
            <Tag color={assignmentCount ? 'cyan' : 'default'}>{assignmentCount} 链路</Tag>
            <Tag color={directCount ? 'blue' : 'default'}>{directCount} 授权</Tag>
          </Space>
        )
      },
    },
    {
      title: '可管理节点',
      render: (_, record) => {
        const assignments = normalizeAreaManagerAssignmentDrafts(record.assignments || [])
        if (assignments.length) {
          return renderAssignmentHierarchy(assignments, agents)
        }
        const agentIDs = record.agent_ids || []
        return agentIDs.length ? (
          <Space size={[4, 4]} wrap>
            {agentIDs.map((agentID) => <Tag key={agentID}>{agentName(agentID, agents)}</Tag>)}
          </Space>
        ) : <Tag>未细化授权</Tag>
      },
    },
    {
      title: '区域收入',
      width: 170,
      render: (_, record) => record.billing_enabled && Number(record.revenue_amount || 0) > 0
        ? `${record.revenue_currency || 'CNY'} ${Number(record.revenue_amount || 0).toFixed(2)} / ${revenueCycleLabel(record.revenue_cycle)}`
        : <Tag>未统计</Tag>,
    },
    {
      title: '状态',
      dataIndex: 'enabled',
      width: 90,
      render: (enabled: boolean) => <Tag color={enabled ? 'blue' : 'default'}>{enabled ? '启用' : '停用'}</Tag>,
    },
    {
      title: '操作',
      width: 260,
      render: (_, record) => (
        <Space size={6}>
          <Button size="small" icon={<EditOutlined />} onClick={() => editAreaManager(record)}>编辑</Button>
          <Popconfirm title={`将 ${record.username} 的密码初始化为 ${DEFAULT_ACCOUNT_PASSWORD}？`} okText="初始化" cancelText="取消" onConfirm={() => void resetAreaManagerPassword(record.id)}>
            <Button size="small" icon={<SyncOutlined />} loading={resettingAreaManagerID === record.id}>初始化密码</Button>
          </Popconfirm>
          <Popconfirm title="删除该区域账号？" okText="删除" cancelText="取消" onConfirm={() => void deleteAreaManager(record.id)}>
            <Button size="small" danger icon={<DeleteOutlined />} />
          </Popconfirm>
        </Space>
      ),
    },
  ]

  const customerColumns: ColumnsType<CustomerAdminView> = [
    {
      title: '用户账号',
      width: 220,
      render: (_, record) => (
        <div>
          <Text strong>{record.username}</Text>
          <div className="muted-line">{record.display_name || record.username}</div>
        </div>
      ),
    },
    {
      title: '已授权链路',
      width: 160,
      render: (_, record) => (
        <Button size="small" onClick={() => openAssignmentViewModal(record)}>
          查看 {record.assignments?.length || 0} 条
        </Button>
      ),
    },
    {
      title: '状态',
      dataIndex: 'enabled',
      width: 90,
      render: (enabled: boolean) => <Tag color={enabled ? 'blue' : 'default'}>{enabled ? '启用' : '停用'}</Tag>,
    },
    {
      title: '操作',
      width: 430,
      render: (_, record) => (
        <Space size={6} wrap>
          <Button size="small" icon={<EditOutlined />} onClick={() => openCustomerEditModal(record)}>编辑</Button>
          <Button size="small" type="primary" onClick={() => openAssignmentManagerModal(record)}>管理链路</Button>
          <Popconfirm title={`将 ${record.username} 的密码初始化为 ${DEFAULT_ACCOUNT_PASSWORD}？`} okText="初始化" cancelText="取消" onConfirm={() => void resetCustomerPassword(record.id)}>
            <Button size="small" icon={<SyncOutlined />} loading={resettingCustomerID === record.id}>初始化密码</Button>
          </Popconfirm>
          <Popconfirm title="删除该用户及其全部分配？" okText="删除" cancelText="取消" onConfirm={() => void deleteCustomer(record.id)}>
            <Button size="small" danger icon={<DeleteOutlined />}>删除</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  const areaAssignmentColumns: ColumnsType<AreaManagerAssignment> = [
    {
      title: '已授权 Client / 节点 / 客户端',
      render: (_, record) => renderAssignmentHierarchy([record], agents),
    },
    {
      title: '操作',
      width: 90,
      render: (_, record) => (
        <Popconfirm title="移除该区域授权？" okText="移除" cancelText="取消" onConfirm={() => void deleteAreaManagerAssignment(record.manager_id, record.id)}>
          <Button size="small" danger icon={<DeleteOutlined />} />
        </Popconfirm>
      ),
    },
  ]

  function renderAreaManagerCustomers(record: AreaManagerAdminView) {
    const ownedCustomers = record.customers || []
    const directAssignments = record.assignments || []
    if (!ownedCustomers.length && !directAssignments.length) {
      return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="该区域账号暂未创建下属用户或授权" />
    }
    return (
      <Space direction="vertical" style={{ width: '100%' }}>
        {directAssignments.length ? (
          <Table
            size="small"
            rowKey={(assignment) => assignment.id}
            columns={areaAssignmentColumns}
            dataSource={directAssignments}
            pagination={false}
          />
        ) : null}
        {ownedCustomers.length ? <Table
          size="small"
          rowKey={(customer) => customer.id}
          columns={areaCustomerColumns}
          dataSource={ownedCustomers}
          pagination={false}
        /> : null}
      </Space>
    )
  }

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

  async function loadAreaManagers() {
    if (!canManageAreaManagers) {
      return
    }
    setAreaManagersLoading(true)
    try {
      const data = await fetchJSON<AreaManagerAdminView[]>('/api/v1/admin/area-managers')
      setAreaManagers(Array.isArray(data) ? data : [])
    } catch (error) {
      message.error(error instanceof Error ? error.message : '加载区域账号失败')
    } finally {
      setAreaManagersLoading(false)
    }
  }

  async function saveAreaManager() {
    if (!areaManagerForm.username.trim()) {
      message.warning('请填写区域账号登录名')
      return
    }
    const password = areaManagerForm.password || (!editingAreaManagerID ? DEFAULT_ACCOUNT_PASSWORD : '')
    if (!editingAreaManagerID && password.length < 8) {
      message.warning('新区域账号密码至少 8 位')
      return
    }
    const assignments = normalizeAreaManagerAssignmentDrafts(areaManagerForm.assignments)
    if (!assignments.length) {
      message.warning('请选择区域账号允许管理的具体节点 / 客户端')
      return
    }
    setSavingAreaManager(true)
    try {
      const agentIDs = uniqueStrings(assignments.map((assignment) => assignment.agent_id))
      const payload = {
        username: areaManagerForm.username.trim(),
        password,
        display_name: areaManagerForm.display_name.trim(),
        enabled: areaManagerForm.enabled,
        agent_ids: agentIDs,
        billing_enabled: areaManagerForm.billing_enabled,
        revenue_amount: areaManagerForm.revenue_amount,
        revenue_currency: areaManagerForm.revenue_currency,
        revenue_cycle: areaManagerForm.revenue_cycle,
      }
      if (editingAreaManagerID) {
        const currentManager = areaManagers.find((item) => item.id === editingAreaManagerID)
        await fetchJSON<AreaManagerAdminView>(`/api/v1/admin/area-managers/${editingAreaManagerID}`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        })
        await replaceAreaManagerAssignments(editingAreaManagerID, currentManager?.assignments || [], assignments)
        message.success('区域账号已更新')
      } else {
        const created = await fetchJSON<AreaManagerAdminView>('/api/v1/admin/area-managers', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        })
        await replaceAreaManagerAssignments(created.id, [], assignments)
        message.success('区域账号已创建')
      }
      setEditingAreaManagerID(null)
      setAreaManagerForm(emptyAreaManagerForm)
      setAreaManagerModalOpen(false)
      await loadAreaManagers()
      await onConfigChanged?.()
    } catch (error) {
      message.error(error instanceof Error ? error.message : '保存区域账号失败')
    } finally {
      setSavingAreaManager(false)
    }
  }

  async function replaceAreaManagerAssignments(managerID: number, previous: AreaManagerAssignment[], next: AreaManagerAssignmentDraft[]) {
    const nextKeys = new Set(next.map(areaAssignmentKey))
    const removed = previous.filter((assignment) => !nextKeys.has(areaAssignmentKey(assignment)))
    for (const assignment of removed) {
      await fetchJSON(`/api/v1/admin/area-managers/${managerID}/assignments/${assignment.id}`, { method: 'DELETE' })
    }
    if (next.length) {
      await fetchJSON<AreaManagerAssignment[]>(`/api/v1/admin/area-managers/${managerID}/assignments`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ assignments: next }),
      })
    }
  }

  async function deleteAreaManager(id: number) {
    setSavingAreaManager(true)
    try {
      await fetchJSON(`/api/v1/admin/area-managers/${id}`, { method: 'DELETE' })
      if (editingAreaManagerID === id) {
        setEditingAreaManagerID(null)
        setAreaManagerForm(emptyAreaManagerForm)
        setAreaManagerModalOpen(false)
      }
      message.success('区域账号已删除')
      await loadAreaManagers()
    } catch (error) {
      message.error(error instanceof Error ? error.message : '删除区域账号失败')
    } finally {
      setSavingAreaManager(false)
    }
  }

  async function resetAreaManagerPassword(id: number) {
    setResettingAreaManagerID(id)
    try {
      await fetchJSON<AreaManagerAdminView>(`/api/v1/admin/area-managers/${id}/reset-password`, { method: 'POST' })
      message.success(`区域账号密码已初始化为 ${DEFAULT_ACCOUNT_PASSWORD}`)
      await loadAreaManagers()
    } catch (error) {
      message.error(error instanceof Error ? error.message : '初始化区域账号密码失败')
    } finally {
      setResettingAreaManagerID(null)
    }
  }

  async function batchAssignAreaManagerTargets() {
    if (!areaBatchForm.manager_id) {
      message.warning('请选择区域账号')
      return
    }
    const xuiAgentID = areaBatchForm.xui_agent_id
    if (areaBatchForm.selected_realm_keys.length && !areaBatchForm.agent_id) {
      message.warning('请选择 Realm 入口 Client')
      return
    }
    if (areaBatchForm.selected_xui_keys.length && !xuiAgentID) {
      message.warning('请选择 x-ui 出口 Client')
      return
    }
    if (!areaBatchForm.selected_realm_keys.length && !areaBatchForm.selected_xui_keys.length) {
      message.warning('请选择要批量授权的 Realm 端口或 x-ui 客户端 / 节点')
      return
    }
    const xuiOptionMap = new Map(buildAssignmentTargetOptions(areaBatchOverview).map((option) => {
      const draft = areaAssignmentDraftFromTargetOption(xuiAgentID, option, agents)
      return [areaAssignmentKey(draft), draft]
    }))
    const realmOptionMap = new Map(areaBatchRealmOptions.map((option) => [option.value, option]))
    const realmAssignments = areaBatchForm.selected_realm_keys.flatMap((key) => {
      const option = realmOptionMap.get(key)
      return option ? [option.assignment] : []
    })
    const xuiAssignments = areaBatchForm.selected_xui_keys.flatMap((key) => {
      const assignment = xuiOptionMap.get(key)
      return assignment ? [assignment] : []
    })
    const inferredRealmTargetAssignments = await inferRealmTargetXUIAssignments(areaBatchForm.selected_realm_keys, areaBatchRealmOptions, xuiAssignments)
    const assignments = dedupeAreaManagerAssignmentDrafts([
      ...realmAssignments,
      ...inferredRealmTargetAssignments,
      ...xuiAssignments,
    ])
    if (!assignments.length) {
      message.warning('没有可授权的选择项')
      return
    }
    setSavingAreaBatchAssignment(true)
    try {
      await fetchJSON<AreaManagerAssignment[]>(`/api/v1/admin/area-managers/${areaBatchForm.manager_id}/assignments`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ assignments }),
      })
      message.success(`已批量授权 ${assignments.length} 条范围`)
      setAreaBatchForm((current) => ({ ...current, selected_realm_keys: [], selected_xui_keys: [] }))
      await loadAreaManagers()
    } catch (error) {
      message.error(error instanceof Error ? error.message : '批量授权失败')
    } finally {
      setSavingAreaBatchAssignment(false)
    }
  }

  async function deleteAreaManagerAssignment(managerID: number, assignmentID: number) {
    try {
      await fetchJSON(`/api/v1/admin/area-managers/${managerID}/assignments/${assignmentID}`, { method: 'DELETE' })
      message.success('区域授权已移除')
      await loadAreaManagers()
    } catch (error) {
      message.error(error instanceof Error ? error.message : '移除区域授权失败')
    }
  }

  function editAreaManager(record: AreaManagerAdminView) {
    setEditingAreaManagerID(record.id)
    setAreaManagerForm({
      username: record.username,
      password: '',
      display_name: record.display_name || record.username,
      enabled: record.enabled,
      agent_ids: record.agent_ids || [],
      billing_enabled: Boolean(record.billing_enabled),
      revenue_amount: Number(record.revenue_amount || 0),
      revenue_currency: record.revenue_currency === 'USDT' ? 'USDT' : 'CNY',
      revenue_cycle: record.revenue_cycle === 'quarter' || record.revenue_cycle === 'year' ? record.revenue_cycle : 'month',
      grant_agent_id: firstRealmAssignmentAgentID(record.assignments || [], agents) || '',
      xui_grant_agent_id: firstXUIAssignmentAgentID(record.assignments || [], agents) || '',
      assignments: normalizeAreaManagerAssignmentDrafts(record.assignments || []),
    })
    setAreaManagerModalOpen(true)
  }

  function openAreaManagerCreateModal() {
    setEditingAreaManagerID(null)
    setAreaManagerForm({
      ...emptyAreaManagerForm,
      password: DEFAULT_ACCOUNT_PASSWORD,
    })
    setAreaManagerModalOpen(true)
  }

  function closeAreaManagerModal() {
    setAreaManagerModalOpen(false)
    setEditingAreaManagerID(null)
    setAreaManagerForm(emptyAreaManagerForm)
  }

  function openCustomerCreateModal() {
    setCustomerCreateForm({
      ...emptyCustomerForm,
      password: DEFAULT_ACCOUNT_PASSWORD,
    })
    setCustomerCreateModalOpen(true)
  }

  function openCustomerEditModal(record: CustomerAdminView) {
    setSelectedCustomerID(record.id)
    setCustomerForm({
      username: record.username,
      password: '',
      display_name: record.display_name || record.username,
      enabled: record.enabled,
    })
    setCustomerEditModalOpen(true)
  }

  function openAssignmentManagerModal(record: CustomerAdminView) {
    setSelectedCustomerID(record.id)
    setEditingAssignmentID(null)
    setAssignmentForm(emptyAssignmentForm)
    setAssignmentManagerModalOpen(true)
  }

  function openAssignmentViewModal(record: CustomerAdminView) {
    setSelectedCustomerID(record.id)
    setAssignmentViewModalOpen(true)
  }

  async function createCustomer() {
    if (!customerCreateForm.username.trim()) {
      message.warning('请填写用户用户名')
      return
    }
    if (customerCreateForm.password.length < 8) {
      message.warning('新用户密码至少 8 位')
      return
    }
    setSavingCustomer(true)
    try {
      const created = await fetchJSON<CustomerAdminView>('/api/v1/admin/customers', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          username: customerCreateForm.username.trim(),
          password: customerCreateForm.password,
          display_name: customerCreateForm.display_name.trim(),
          enabled: customerCreateForm.enabled,
        }),
      })
      message.success('用户已创建')
      setCustomerCreateModalOpen(false)
      setCustomerCreateForm(emptyCustomerForm)
      await loadCustomers()
      setSelectedCustomerID(created.id)
      if (canManageAreaManagers) {
        await loadAreaManagers()
      }
    } catch (error) {
      message.error(error instanceof Error ? error.message : '创建用户失败')
    } finally {
      setSavingCustomer(false)
    }
  }

  async function saveCustomer() {
    if (!customerForm.username.trim()) {
      message.warning('请填写用户用户名')
      return
    }
    if (!selectedCustomerID && customerForm.password.length < 8) {
      message.warning('新用户密码至少 8 位')
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
        message.success('用户已更新')
      } else {
        const created = await fetchJSON<CustomerAdminView>('/api/v1/admin/customers', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        })
        setSelectedCustomerID(created.id)
        message.success('用户已创建')
      }
      setCustomerForm((current) => ({ ...current, password: '' }))
      setCustomerEditModalOpen(false)
      await loadCustomers()
      if (canManageAreaManagers) {
        await loadAreaManagers()
      }
    } catch (error) {
      message.error(error instanceof Error ? error.message : '保存用户失败')
    } finally {
      setSavingCustomer(false)
    }
  }

  async function deleteCustomer(customerID = selectedCustomerID) {
    if (!customerID) {
      return
    }
    setSavingCustomer(true)
    try {
      await fetchJSON(`/api/v1/admin/customers/${customerID}`, { method: 'DELETE' })
      if (selectedCustomerID === customerID) {
        setSelectedCustomerID(null)
        setCustomerForm(emptyCustomerForm)
        setAssignmentForm(emptyAssignmentForm)
        setEditingAssignmentID(null)
      }
      setCustomerEditModalOpen(false)
      setAssignmentManagerModalOpen(false)
      setAssignmentViewModalOpen(false)
      message.success('用户已删除')
      await loadCustomers()
      if (canManageAreaManagers) {
        await loadAreaManagers()
      }
    } catch (error) {
      message.error(error instanceof Error ? error.message : '删除用户失败')
    } finally {
      setSavingCustomer(false)
    }
  }

  async function resetCustomerPassword(customerID: number) {
    setResettingCustomerID(customerID)
    try {
      await fetchJSON<CustomerAdminView>(`/api/v1/admin/customers/${customerID}/reset-password`, { method: 'POST' })
      message.success(`普通用户密码已初始化为 ${DEFAULT_ACCOUNT_PASSWORD}`)
      await loadCustomers()
      if (canManageAreaManagers) {
        await loadAreaManagers()
      }
    } catch (error) {
      message.error(error instanceof Error ? error.message : '初始化普通用户密码失败')
    } finally {
      setResettingCustomerID(null)
    }
  }

  async function saveAssignment() {
    if (!selectedCustomerID) {
      message.warning('请先选择用户')
      return
    }
    if (!assignmentForm.agent_id || !assignmentForm.inbound_id) {
      message.warning('请选择 Client 和节点')
      return
    }
    setSavingAssignment(true)
    try {
      const customerID = selectedCustomerID
      const assignmentID = editingAssignmentID
      const payload = {
        agent_id: assignmentForm.agent_id,
        inbound_id: assignmentForm.inbound_id,
        inbound_tag: assignmentForm.inbound_tag,
        client_email: assignmentForm.client_email,
        public_client_name: assignmentForm.public_client_name,
        ...(canViewFinance ? {
          revenue_amount: assignmentForm.revenue_amount,
          revenue_currency: assignmentForm.revenue_currency,
          revenue_cycle: assignmentForm.revenue_cycle,
        } : {}),
        enabled: assignmentForm.enabled,
      }
      let savedAssignment: CustomerAssignment
      if (assignmentID) {
        savedAssignment = await fetchJSON<CustomerAssignment>(`/api/v1/admin/customers/${customerID}/assignments/${assignmentID}`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        })
        message.success('分配已更新')
      } else {
        savedAssignment = await fetchJSON<CustomerAssignment>(`/api/v1/admin/customers/${customerID}/assignments`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        })
        message.success('分配已新增')
      }
      const nextForm = assignmentFormFromAssignment(savedAssignment, agents)
      setSelectedCustomerID(customerID)
      setEditingAssignmentID(savedAssignment.id)
      setAssignmentForm({
        ...nextForm,
        ...(canViewFinance ? {
          revenue_amount: Number(payload.revenue_amount ?? assignmentForm.revenue_amount ?? 0),
          revenue_currency: payload.revenue_currency === 'USDT' ? 'USDT' : 'CNY',
          revenue_cycle: payload.revenue_cycle === 'quarter' || payload.revenue_cycle === 'year' ? payload.revenue_cycle : 'month',
        } : {}),
      })
      await onConfigChanged?.(customerID ? payload.agent_id : undefined)
      await loadCustomers()
      if (canManageAreaManagers) {
        await loadAreaManagers()
      }
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
      if (canManageAreaManagers) {
        await loadAreaManagers()
      }
    } catch (error) {
      message.error(error instanceof Error ? error.message : '删除分配失败')
    } finally {
      setSavingAssignment(false)
    }
  }

  function editAssignment(record: CustomerAssignment) {
    const nextCustomerID = record.customer_id || selectedCustomerID
    if (nextCustomerID !== selectedCustomerID) {
      skipAssignmentResetRef.current = true
      setSelectedCustomerID(nextCustomerID)
    }
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
        ...(canViewFinance ? billingFormPatch(clientBilling(assignmentForm.agent_id, client.inbound_id, client.inbound_tag || '', client.email || '', agents)) : {}),
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
        ...(canViewFinance ? billingFormPatch(clientBilling(assignmentForm.agent_id, node.id, node.tag || '', '', agents)) : {}),
      }))
    }
  }

  function updateAreaManagerGrantTargets(keys: string[]) {
    const agentID = areaManagerXUIGrantAgentID
    if (!agentID) {
      return
    }
    const optionMap = new Map(areaManagerGrantOptions.map((option) => {
      const draft = areaAssignmentDraftFromTargetOption(agentID, option, agents)
      return [areaAssignmentKey(draft), draft]
    }))
    const nextForAgent = keys
      .map((key) => optionMap.get(key))
      .filter((item): item is AreaManagerAssignmentDraft => Boolean(item))
    setAreaManagerForm((current) => {
      const otherAssignments = current.assignments.filter((assignment) => assignment.agent_id !== agentID || isRealmAssignmentDraft(assignment, agents))
      const assignments = [...otherAssignments, ...nextForAgent]
      return {
        ...current,
        assignments,
        agent_ids: uniqueStrings(assignments.map((assignment) => assignment.agent_id)),
      }
    })
  }

  async function updateAreaManagerRealmGrantTargets(keys: string[]) {
    const agentID = areaManagerForm.grant_agent_id
    if (!agentID) {
      return
    }
    const optionMap = new Map(areaManagerRealmGrantOptions.map((option) => [option.value, option.assignment]))
    const nextForAgent = keys
      .map((key) => optionMap.get(key))
      .filter((item): item is AreaManagerAssignmentDraft => Boolean(item))
    const existingXUIAssignments = areaManagerForm.assignments.filter((assignment) => !isRealmAssignmentDraft(assignment, agents))
    const inferredTargetAssignments = await inferRealmTargetXUIAssignments(keys, areaManagerRealmGrantOptions, existingXUIAssignments)
    setAreaManagerForm((current) => {
      const otherAssignments = current.assignments.filter((assignment) => assignment.agent_id !== agentID || !isRealmAssignmentDraft(assignment, agents))
      const assignments = dedupeAreaManagerAssignmentDrafts([...otherAssignments, ...nextForAgent, ...inferredTargetAssignments])
      return {
        ...current,
        assignments,
        agent_ids: uniqueStrings(assignments.map((assignment) => assignment.agent_id)),
      }
    })
  }

  async function inferRealmTargetXUIAssignments(
    selectedRealmKeys: string[],
    options: ReturnType<typeof buildRealmGrantOptions>,
    excludedTargetAssignments: AreaManagerAssignmentDraft[] = [],
  ): Promise<AreaManagerAssignmentDraft[]> {
    const selected = new Set(selectedRealmKeys)
    const overviewCache = new Map<string, XUIOverview | null>()
    const inferred: AreaManagerAssignmentDraft[] = []
    for (const option of options) {
      if (!selected.has(option.value)) {
        continue
      }
      const targetAgentID = realmRuleTargetAgentID(option.rule, agents)
      if (!targetAgentID) {
        continue
      }
      let targetOverview = overviewCache.get(targetAgentID)
      if (!overviewCache.has(targetAgentID)) {
        try {
          targetOverview = await fetchJSON<XUIOverview>(`/api/v1/agents/${targetAgentID}/xui/overview?assignment_scope=1`)
        } catch {
          targetOverview = null
        }
        overviewCache.set(targetAgentID, targetOverview || null)
      }
      const node = findRealmTargetNode(targetOverview, option.rule)
      if (!node) {
        continue
      }
      if (excludedTargetAssignments.some((item) => assignmentMatchesInbound(item, targetAgentID, node.id, node.tag))) {
        continue
      }
      inferred.push(areaAssignmentDraftFromTargetOption(targetAgentID, { node }, agents))
    }
    return dedupeAreaManagerAssignmentDrafts(inferred)
  }

  function removeAreaManagerGrant(key: string) {
    setAreaManagerForm((current) => {
      const assignments = current.assignments.filter((assignment) => areaAssignmentKey(assignment) !== key)
      return {
        ...current,
        assignments,
        agent_ids: uniqueStrings(assignments.map((assignment) => assignment.agent_id)),
      }
    })
  }

  const areaManagersPanel = canManageAreaManagers ? (
    <Card className="customer-admin-card" bordered={false}>
      <div className="customer-admin-card-head">
        <Title level={5}>区域管理账号</Title>
        <Space>
          <Button size="small" icon={<ReloadOutlined />} onClick={() => void loadAreaManagers()}>刷新区域账号</Button>
          <Button size="small" type="primary" icon={<PlusOutlined />} onClick={openAreaManagerCreateModal}>新增区域账号</Button>
        </Space>
      </div>
      <Alert
        style={{ marginBottom: 12 }}
        type="info"
        showIcon
        message="区域账号权限"
        description="区域账号只能查看被分配的 Client，能下发 x-ui 转发规则，并且只能管理自己创建的普通用户；Admin 可见全部用户与区域账号。展开区域账号可直接查看其下属用户与链路。"
      />
      <Card size="small" style={{ marginTop: 14 }} bordered={false}>
        <div className="customer-admin-card-head">
          <div>
            <Title level={5}>批量授权入口 / 出口</Title>
            <Text type="secondary">Realm 入口端口与 x-ui 出口节点可分别选择；广州入口转发 HK 时，先授权 GZ Realm 端口，再授权 HK x-ui 节点或客户端。</Text>
          </div>
          <Button
            type="primary"
            icon={<SaveOutlined />}
            loading={savingAreaBatchAssignment}
            onClick={() => void batchAssignAreaManagerTargets()}
          >
            批量授权
          </Button>
        </div>
        <Row gutter={[12, 12]}>
          <Col xs={24} md={5}>
            <Text type="secondary">区域账号</Text>
            <Select
              style={{ width: '100%' }}
              showSearch
              placeholder="选择区域账号"
              value={areaBatchForm.manager_id ?? undefined}
              options={areaManagers.map((item) => ({ value: item.id, label: `${item.username} · ${item.display_name || item.username}` }))}
              optionFilterProp="label"
              onChange={(value) => setAreaBatchForm((current) => ({ ...current, manager_id: value }))}
            />
          </Col>
          <Col xs={24} md={5}>
            <Text type="secondary">Realm 入口 Client</Text>
            <Select
              style={{ width: '100%' }}
              showSearch
              placeholder="选择 GZ 入口"
              value={areaBatchForm.agent_id || undefined}
              options={agentOptions}
              optionFilterProp="label"
              onChange={(value) => setAreaBatchForm((current) => ({ ...current, agent_id: value, selected_realm_keys: [] }))}
            />
          </Col>
          <Col xs={24} md={5}>
            <Text type="secondary">x-ui 出口 Client</Text>
            <Select
              style={{ width: '100%' }}
              showSearch
              placeholder="选择 HK 出口"
              value={areaBatchForm.xui_agent_id || undefined}
              options={agentOptions}
              optionFilterProp="label"
              onChange={(value) => setAreaBatchForm((current) => ({ ...current, xui_agent_id: value, selected_xui_keys: [] }))}
            />
          </Col>
          <Col xs={24} md={4}>
            <Space style={{ width: '100%', justifyContent: 'space-between' }}>
              <Text type="secondary">Realm 端口</Text>
              <Button size="small" disabled={!areaBatchForm.agent_id || !areaBatchRealmOptions.length} onClick={() => setAreaBatchForm((current) => ({ ...current, selected_realm_keys: areaBatchRealmOptions.map((option) => option.value) }))}>全选</Button>
            </Space>
            <Select
              mode="multiple"
              style={{ width: '100%' }}
              showSearch
              placeholder="选择 Realm 中转端口"
              value={areaBatchForm.selected_realm_keys}
              disabled={!areaBatchForm.agent_id}
              options={areaBatchRealmOptions.map(({ value, label }) => ({ value, label }))}
              optionFilterProp="label"
              maxTagCount="responsive"
              onChange={(values) => setAreaBatchForm((current) => ({ ...current, selected_realm_keys: values }))}
            />
          </Col>
          <Col xs={24} md={5}>
            <Space style={{ width: '100%', justifyContent: 'space-between' }}>
              <Text type="secondary">x-ui 客户端 / 节点</Text>
              <Button size="small" disabled={!areaBatchXUIAgentID || !areaBatchClientTreeData.length} onClick={() => setAreaBatchForm((current) => ({ ...current, selected_xui_keys: assignmentNodeKeys(areaBatchXUIAgentID, areaBatchOverview, agents) }))}>全选</Button>
            </Space>
            <TreeSelect
              multiple
              treeCheckable
              style={{ width: '100%' }}
              showSearch
              placeholder="按 Client / 节点 / 客户端授权"
              value={areaBatchForm.selected_xui_keys}
              loading={areaBatchOverviewLoading}
              disabled={!areaBatchXUIAgentID}
              treeData={areaBatchClientTreeData}
              treeDefaultExpandAll
              showCheckedStrategy={TreeSelect.SHOW_PARENT}
              maxTagCount="responsive"
              onChange={(values) => setAreaBatchForm((current) => ({ ...current, selected_xui_keys: values as string[] }))}
            />
          </Col>
        </Row>
      </Card>
      <Table
        style={{ marginTop: 14 }}
        rowKey={(record) => record.id}
        columns={areaManagerColumns}
        dataSource={areaManagers}
        pagination={{ pageSize: 5, hideOnSinglePage: true }}
        expandable={{
          expandedRowRender: renderAreaManagerCustomers,
          rowExpandable: (record) => Boolean(record.customers?.length || record.assignments?.length),
        }}
        locale={{ emptyText: <Empty description="暂无区域账号" /> }}
      />
    </Card>
  ) : null

  const customersPanel = (
    <Card className="customer-admin-card" bordered={false}>
      <div className="customer-admin-card-head">
        <div>
          <Title level={5}>用户列表</Title>
          <Text type="secondary">普通账号以列表方式管理；编辑用户、查看链路和管理链路均通过弹窗完成。</Text>
        </div>
        <Space>
          <Button icon={<ReloadOutlined />} onClick={() => void loadCustomers()}>刷新用户</Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={openCustomerCreateModal}>新增普通账号</Button>
        </Space>
      </div>
      <Table
        rowKey={(record) => record.id}
        columns={customerColumns}
        dataSource={customers}
        pagination={{ pageSize: 8, hideOnSinglePage: true }}
        locale={{ emptyText: <Empty description="暂无用户" /> }}
      />
    </Card>
  )

  const assignmentManagerContent = (
    <Card className="customer-admin-card" bordered={false}>
        <div className="customer-admin-card-head">
          <div>
            <Title level={5}>授权链路分配</Title>
            <Text type="secondary">当前用户：{selectedCustomer ? selectedCustomer.display_name || selectedCustomer.username : '未选择'}</Text>
          </div>
          <Button onClick={() => {
            setEditingAssignmentID(null)
            setAssignmentForm(emptyAssignmentForm)
          }}>清空表单</Button>
        </div>
        {!selectedCustomerID ? <Alert type="info" showIcon message="先选择或新建用户，再分配链路。" /> : null}
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
            <Text type="secondary">Client / 节点 / 客户端</Text>
            <TreeSelect
              style={{ width: '100%' }}
              showSearch
              placeholder="按 Client / 节点 / 客户端选择"
              value={assignmentForm.client_key || undefined}
              loading={overviewLoading}
              disabled={!assignmentForm.agent_id}
              treeData={clientTreeData}
              treeDefaultExpandAll
              allowClear
              onChange={(value) => selectClient(String(value || ''))}
            />
          </Col>
          <Col xs={24} md={6}>
            <Text type="secondary">授权链路名称</Text>
            <Input value={assignmentForm.public_client_name} onChange={(event) => setAssignmentForm((current) => ({ ...current, public_client_name: event.target.value }))} />
          </Col>
          {canViewFinance ? <Col xs={24} md={4}>
            <Text type="secondary">费用</Text>
            <InputNumber
              style={{ width: '100%' }}
              min={0}
              precision={2}
              value={assignmentForm.revenue_amount}
              onChange={(value) => setAssignmentForm((current) => ({ ...current, revenue_amount: Number(value || 0) }))}
            />
          </Col> : null}
          {canViewFinance ? <Col xs={12} md={2}>
            <Text type="secondary">币种</Text>
            <Select
              style={{ width: '100%' }}
              value={assignmentForm.revenue_currency}
              options={REVENUE_CURRENCIES.map((currency) => ({ value: currency, label: currency }))}
              onChange={(value) => setAssignmentForm((current) => ({ ...current, revenue_currency: value as 'CNY' | 'USDT' }))}
            />
          </Col> : null}
          {canViewFinance ? <Col xs={12} md={2}>
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
          </Col> : null}
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
          {editingAssignmentID ? '保存授权' : '新增授权'}
        </Button>
        <Table
          style={{ marginTop: 14 }}
          rowKey={(record) => record.id}
          columns={visibleAssignmentColumns}
          dataSource={selectedCustomer?.assignments || []}
          pagination={{ pageSize: 8, hideOnSinglePage: true }}
          locale={{ emptyText: <Empty description="暂无授权链路" /> }}
        />
    </Card>
  )

  const readOnlyAssignmentColumns = visibleAssignmentColumns.filter((column) => column.key !== 'actions')

  const accountEditorModals = (
    <>
      {canManageAreaManagers ? (
        <Modal
          title={editingAreaManagerID ? '编辑区域账号' : '新增区域账号'}
          open={areaManagerModalOpen}
          onCancel={closeAreaManagerModal}
          footer={(
            <Space>
              <Button onClick={closeAreaManagerModal}>取消</Button>
              <Button type="primary" icon={<SaveOutlined />} loading={savingAreaManager} onClick={() => void saveAreaManager()}>
                {editingAreaManagerID ? '保存区域账号' : '新增区域账号'}
              </Button>
            </Space>
          )}
          width={860}
          destroyOnClose
        >
          <Row gutter={[12, 12]}>
            <Col xs={24} md={12}>
              <Text type="secondary">登录用户名</Text>
              <Input value={areaManagerForm.username} onChange={(event) => setAreaManagerForm((current) => ({ ...current, username: event.target.value }))} />
            </Col>
            <Col xs={24} md={12}>
              <Text type="secondary">显示名</Text>
              <Input value={areaManagerForm.display_name} onChange={(event) => setAreaManagerForm((current) => ({ ...current, display_name: event.target.value }))} />
            </Col>
            <Col xs={24} md={12}>
              <Text type="secondary">密码{editingAreaManagerID ? '（留空不改，可初始化为 12345678）' : '（默认 12345678）'}</Text>
              <Input.Password value={areaManagerForm.password} onChange={(event) => setAreaManagerForm((current) => ({ ...current, password: event.target.value }))} />
            </Col>
            <Col xs={24} md={12}>
              <Text type="secondary">状态</Text>
              <div className="customer-admin-switch-row">
                <Switch checked={areaManagerForm.enabled} onChange={(checked) => setAreaManagerForm((current) => ({ ...current, enabled: checked }))} />
                <Text>{areaManagerForm.enabled ? '启用' : '停用'}</Text>
              </div>
            </Col>
            <Col xs={24}>
              <Card size="small" bordered className="customer-admin-card">
                <Row gutter={[12, 12]} align="middle">
                  <Col xs={24} md={6}>
                    <Text type="secondary">区域账号财务</Text>
                    <div className="customer-admin-switch-row">
                      <Switch checked={areaManagerForm.billing_enabled} onChange={(checked) => setAreaManagerForm((current) => ({ ...current, billing_enabled: checked }))} />
                      <Text>{areaManagerForm.billing_enabled ? '计入 admin 财务' : '不统计'}</Text>
                    </div>
                  </Col>
                  <Col xs={24} md={6}>
                    <Text type="secondary">账号收入</Text>
                    <InputNumber
                      style={{ width: '100%' }}
                      min={0}
                      precision={2}
                      disabled={!areaManagerForm.billing_enabled}
                      value={areaManagerForm.revenue_amount}
                      onChange={(value) => setAreaManagerForm((current) => ({ ...current, revenue_amount: Number(value || 0) }))}
                    />
                  </Col>
                  <Col xs={12} md={4}>
                    <Text type="secondary">币种</Text>
                    <Select
                      style={{ width: '100%' }}
                      disabled={!areaManagerForm.billing_enabled}
                      value={areaManagerForm.revenue_currency}
                      options={REVENUE_CURRENCIES.map((currency) => ({ value: currency, label: currency }))}
                      onChange={(value) => setAreaManagerForm((current) => ({ ...current, revenue_currency: value as 'CNY' | 'USDT' }))}
                    />
                  </Col>
                  <Col xs={12} md={4}>
                    <Text type="secondary">周期</Text>
                    <Select
                      style={{ width: '100%' }}
                      disabled={!areaManagerForm.billing_enabled}
                      value={areaManagerForm.revenue_cycle}
                      options={[
                        { value: 'month', label: '月' },
                        { value: 'quarter', label: '季' },
                        { value: 'year', label: '年' },
                      ]}
                      onChange={(value) => setAreaManagerForm((current) => ({ ...current, revenue_cycle: value as 'month' | 'quarter' | 'year' }))}
                    />
                  </Col>
                  <Col xs={24} md={4}>
                    <Text type="secondary">说明</Text>
                    <div className="muted-line">admin 财务 = 单用户节点收入 + 区域账号收入 - VPS 总花销</div>
                  </Col>
                </Row>
              </Card>
            </Col>
            <Col xs={24} md={12}>
              <Text type="secondary">Realm 入口 Client</Text>
              <Select
                style={{ width: '100%' }}
                showSearch
                placeholder="选择 GZ 入口 Client"
                options={agentOptions}
                value={areaManagerForm.grant_agent_id || undefined}
                optionFilterProp="label"
                onChange={(value) => setAreaManagerForm((current) => ({ ...current, grant_agent_id: value }))}
              />
            </Col>
            <Col xs={24} md={12}>
              <Text type="secondary">x-ui 出口 Client</Text>
              <Select
                style={{ width: '100%' }}
                showSearch
                placeholder="选择 HK 出口 Client"
                options={agentOptions}
                value={areaManagerXUIGrantAgentID || undefined}
                optionFilterProp="label"
                onChange={(value) => setAreaManagerForm((current) => ({ ...current, xui_grant_agent_id: value }))}
              />
            </Col>
            <Col xs={24} md={12}>
              <Space style={{ width: '100%', justifyContent: 'space-between' }}>
                <Text type="secondary">Realm 端口授权</Text>
                <Button size="small" disabled={!areaManagerForm.grant_agent_id || !areaManagerRealmGrantOptions.length} onClick={() => void updateAreaManagerRealmGrantTargets(areaManagerRealmGrantOptions.map((option) => option.value))}>全选</Button>
              </Space>
              <Select
                mode="multiple"
                style={{ width: '100%' }}
                showSearch
                placeholder="选择 Realm 中转端口"
                value={areaManagerForm.assignments
                  .filter((assignment) => assignment.agent_id === areaManagerForm.grant_agent_id && isRealmAssignmentDraft(assignment, agents))
                  .map(areaAssignmentKey)}
                disabled={!areaManagerForm.grant_agent_id}
                options={areaManagerRealmGrantOptions.map((option) => ({ value: option.value, label: option.label }))}
                optionFilterProp="label"
                maxTagCount="responsive"
                onChange={(values) => void updateAreaManagerRealmGrantTargets(values)}
              />
            </Col>
            <Col xs={24} md={12}>
              <Space style={{ width: '100%', justifyContent: 'space-between' }}>
                <Text type="secondary">x-ui 节点 / 客户端授权</Text>
                <Button
                  size="small"
                  disabled={!areaManagerXUIGrantAgentID || !areaManagerGrantOptions.length}
                  onClick={() => updateAreaManagerGrantTargets(assignmentNodeKeys(areaManagerXUIGrantAgentID, areaManagerOverview, agents))}
                >
                  全选
                </Button>
              </Space>
              <TreeSelect
                multiple
                treeCheckable
                style={{ width: '100%' }}
                showSearch
                placeholder="按 Client / 节点 / 客户端授权"
                value={areaManagerForm.assignments
                  .filter((assignment) => assignment.agent_id === areaManagerXUIGrantAgentID && !isRealmAssignmentDraft(assignment, agents))
                  .map(areaAssignmentKey)}
                loading={areaManagerOverviewLoading}
                disabled={!areaManagerXUIGrantAgentID}
                treeData={areaManagerGrantTreeData}
                treeDefaultExpandAll
                showCheckedStrategy={TreeSelect.SHOW_PARENT}
                maxTagCount="responsive"
                onChange={(values) => updateAreaManagerGrantTargets(values as string[])}
              />
            </Col>
            <Col xs={24}>
              <Text type="secondary">已授权范围</Text>
              <div style={{ marginTop: 6 }}>
                {areaManagerForm.assignments.length ? renderAssignmentHierarchy(areaManagerForm.assignments, agents, removeAreaManagerGrant) : <Tag>未选择节点</Tag>}
              </div>
            </Col>
          </Row>
        </Modal>
      ) : null}
      <Modal
        title="新增普通账号"
        open={customerCreateModalOpen}
        onCancel={() => {
          setCustomerCreateModalOpen(false)
          setCustomerCreateForm(emptyCustomerForm)
        }}
        footer={(
          <Space>
            <Button onClick={() => {
              setCustomerCreateModalOpen(false)
              setCustomerCreateForm(emptyCustomerForm)
            }}>取消</Button>
            <Button type="primary" icon={<SaveOutlined />} loading={savingCustomer} onClick={() => void createCustomer()}>
              新增普通账号
            </Button>
          </Space>
        )}
        width={680}
        destroyOnClose
      >
        <Row gutter={[12, 12]}>
          <Col xs={24} md={12}>
            <Text type="secondary">登录用户名</Text>
            <Input value={customerCreateForm.username} onChange={(event) => setCustomerCreateForm((current) => ({ ...current, username: event.target.value }))} />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">用户显示名</Text>
            <Input value={customerCreateForm.display_name} onChange={(event) => setCustomerCreateForm((current) => ({ ...current, display_name: event.target.value }))} />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">密码（默认 12345678）</Text>
            <Input.Password value={customerCreateForm.password} onChange={(event) => setCustomerCreateForm((current) => ({ ...current, password: event.target.value }))} />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">账号状态</Text>
            <div className="customer-admin-switch-row">
              <Switch checked={customerCreateForm.enabled} onChange={(checked) => setCustomerCreateForm((current) => ({ ...current, enabled: checked }))} />
              <Text>{customerCreateForm.enabled ? '启用' : '停用'}</Text>
            </div>
          </Col>
          <Col xs={24}>
            <Text type="secondary">用户入口地址</Text>
            <Input value={`${window.location.origin}/customer`} readOnly />
          </Col>
        </Row>
      </Modal>
      <Modal
        title="编辑普通账号"
        open={customerEditModalOpen}
        onCancel={() => {
          setCustomerEditModalOpen(false)
          setCustomerForm((current) => ({ ...current, password: '' }))
        }}
        footer={(
          <Space>
            <Button onClick={() => {
              setCustomerEditModalOpen(false)
              setCustomerForm((current) => ({ ...current, password: '' }))
            }}>取消</Button>
            <Button type="primary" icon={<SaveOutlined />} loading={savingCustomer} disabled={!selectedCustomerID} onClick={() => void saveCustomer()}>
              保存普通账号
            </Button>
          </Space>
        )}
        width={760}
        destroyOnClose
      >
        <Row gutter={[12, 12]}>
          <Col xs={24} md={12}>
            <Text type="secondary">登录用户名</Text>
            <Input value={customerForm.username} onChange={(event) => setCustomerForm((current) => ({ ...current, username: event.target.value }))} />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">用户显示名</Text>
            <Input value={customerForm.display_name} onChange={(event) => setCustomerForm((current) => ({ ...current, display_name: event.target.value }))} />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">密码（留空不改）</Text>
            <Input.Password value={customerForm.password} onChange={(event) => setCustomerForm((current) => ({ ...current, password: event.target.value }))} />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">账号状态</Text>
            <div className="customer-admin-switch-row">
              <Switch checked={customerForm.enabled} onChange={(checked) => setCustomerForm((current) => ({ ...current, enabled: checked }))} />
              <Text>{customerForm.enabled ? '启用' : '停用'}</Text>
            </div>
          </Col>
          <Col xs={24}>
            <Text type="secondary">用户入口地址</Text>
            <Input value={`${window.location.origin}/customer`} readOnly />
          </Col>
        </Row>
      </Modal>
      <Modal
        title={`授权链路 · ${selectedCustomer ? selectedCustomer.display_name || selectedCustomer.username : '未选择用户'}`}
        open={assignmentViewModalOpen}
        onCancel={() => setAssignmentViewModalOpen(false)}
        footer={<Button onClick={() => setAssignmentViewModalOpen(false)}>关闭</Button>}
        width={980}
        destroyOnClose
      >
        <Table
          rowKey={(record) => record.id}
          columns={readOnlyAssignmentColumns}
          dataSource={selectedCustomer?.assignments || []}
          pagination={{ pageSize: 8, hideOnSinglePage: true }}
          locale={{ emptyText: <Empty description="暂无授权链路" /> }}
        />
      </Modal>
      <Modal
        title={`管理授权链路 · ${selectedCustomer ? selectedCustomer.display_name || selectedCustomer.username : '未选择用户'}`}
        open={assignmentManagerModalOpen}
        onCancel={() => {
          setAssignmentManagerModalOpen(false)
          setEditingAssignmentID(null)
          setAssignmentForm(emptyAssignmentForm)
        }}
        footer={null}
        width={1160}
        destroyOnClose
      >
        {assignmentManagerContent}
      </Modal>
    </>
  )

  const managementTabs = [
    ...(canManageAreaManagers && areaManagersPanel ? [{ key: 'area', label: '区域账号', children: areaManagersPanel }] : []),
    { key: 'customers', label: '用户账号', children: customersPanel },
  ]

  const content = (
    <Spin spinning={loading || areaManagersLoading}>
      <Space direction="vertical" size="middle" style={{ width: '100%' }}>
        <Alert
          type="info"
          showIcon
          message="授权使用规则与停用机制"
          description="建议仅给已授权用户开放链路，可用于独享或多人共享场景；禁止滥发、攻击、诈骗、爬虫滥用、扫描爆破等行为。出现异常时，可先停用用户账号或单条授权链路；如需彻底切断连接，再到对应 Client 的 x-ui 客户端列表中删除或停用该 client。"
        />
        <Tabs
          className="customer-admin-tabs"
          activeKey={activeManagementTab}
          items={managementTabs}
          onChange={(key) => setActiveManagementTab(key as ManagementTabKey)}
        />
      </Space>
    </Spin>
  )

  if (embedded) {
    return (
      <div id="customer-management-panel" className="customer-admin-page">
        {content}
        {accountEditorModals}
      </div>
    )
  }

  return (
    <Modal title="人员管理 / 授权链路" open={open} onCancel={onClose} footer={null} width={1160} destroyOnClose>
      {content}
      {accountEditorModals}
    </Modal>
  )
}

function clientKey(client: XUIClientView): string {
  return `client:${client.inbound_id}::${client.email || ''}`
}

function buildAssignmentTargetOptions(overview: XUIOverview | null) {
  const clients = (overview?.clients || [])
    .filter((client) => !isRealmForwardedClientOption(client))
    .map((client) => ({
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
}

function buildClientAssignmentTreeData(agentID: string, overview: XUIOverview | null, agents: DashboardAgentView[]) {
  if (!agentID || !overview) {
    return []
  }
  return [{
    title: agentName(agentID, agents),
    value: `agent:${agentID}`,
    selectable: false,
    children: overviewNodeGroups(overview, false).map(({ node, clients }) => ({
      title: nodeLabel(node),
      value: nodeKey(node),
      children: clients.map((client) => ({
        title: clientTreeTitle(client),
        value: clientKey(client),
      })),
    })),
  }]
}

function buildAreaAssignmentTreeData(agentID: string, overview: XUIOverview | null, agents: DashboardAgentView[]) {
  if (!agentID || !overview) {
    return []
  }
  return [{
    title: agentName(agentID, agents),
    value: `agent:${agentID}`,
    selectable: false,
    children: overviewNodeGroups(overview, true).map(({ node, clients }) => ({
      title: nodeLabel(node),
      value: areaAssignmentKey(areaAssignmentDraftFromTargetOption(agentID, { node }, agents)),
      children: clients.map((client) => ({
        title: clientTreeTitle(client),
        value: areaAssignmentKey(areaAssignmentDraftFromTargetOption(agentID, { client }, agents)),
      })),
    })),
  }]
}

function overviewNodeGroups(overview: XUIOverview | null, excludeRealmForwarded: boolean) {
  const nodes = [...(overview?.nodes || [])]
  const clients = (overview?.clients || []).filter((client) => !excludeRealmForwarded || !isRealmForwardedClientOption(client))
  const nodeIDs = new Set(nodes.map((node) => Number(node.id || 0)))
  for (const client of clients) {
    if (!nodeIDs.has(Number(client.inbound_id || 0))) {
      nodes.push({
        id: client.inbound_id,
        tag: client.inbound_tag || '',
        remark: client.inbound_remark || client.inbound_tag || `Inbound #${client.inbound_id}`,
        protocol: client.protocol || '',
        enabled: client.enabled !== false,
        route: client.route,
      })
      nodeIDs.add(Number(client.inbound_id || 0))
    }
  }
  return nodes.map((node) => ({
    node,
    clients: clients.filter((client) => Number(client.inbound_id || 0) === Number(node.id || 0)),
  }))
}

function assignmentNodeKeys(agentID: string, overview: XUIOverview | null, agents: DashboardAgentView[]): string[] {
  if (!agentID) {
    return []
  }
  return (overview?.nodes || []).map((node) => areaAssignmentKey(areaAssignmentDraftFromTargetOption(agentID, { node }, agents)))
}

function buildRealmGrantOptions(agentID: string, agents: DashboardAgentView[]) {
  const agent = agents.find((item) => item.agent_id === agentID)
  const rules = agent?.entry?.port_forwarding?.rules || []
  const seenPorts = new Set<number>()
  return rules
    .filter((rule) => rule.enabled !== false && Number(rule.listen_port || 0) > 0)
    .filter((rule) => {
      const port = Number(rule.listen_port || 0)
      if (seenPorts.has(port)) {
        return false
      }
      seenPorts.add(port)
      return true
    })
    .map((rule) => {
      const assignment = areaAssignmentDraftFromRealmRule(agentID, rule, agents)
      return {
        value: areaAssignmentKey(assignment),
        label: realmGrantLabel(rule),
        assignment,
        rule,
      }
    })
}

function realmRuleTargetAgentID(rule: RealmForwardRule, agents: DashboardAgentView[]): string {
  const explicit = (rule.target_agent_id || '').trim()
  if (explicit) {
    return explicit
  }
  const target = normalizeEndpointHost(rule.target_address || '')
  if (!target) {
    return ''
  }
  return agents.find((agent) => agentAddressCandidates(agent).some((candidate) => normalizeEndpointHost(candidate) === target))?.agent_id || ''
}

function agentAddressCandidates(agent: DashboardAgentView): string[] {
  return [
    agent.agent_id,
    agent.agent_name || '',
    agent.customer_display_name || '',
    agent.entry?.import_domain || '',
    ...(agent.entry?.addresses || []),
    agent.summary?.public_ipv4 || '',
    agent.summary?.public_ipv6 || '',
    agent.summary?.observed_ip || '',
    agent.summary?.server_seen_ip || '',
    agent.summary?.hostname || '',
  ].filter(Boolean)
}

function normalizeEndpointHost(value: string): string {
  let text = String(value || '').trim().toLowerCase()
  if (!text) {
    return ''
  }
  if (text.includes('://')) {
    try {
      text = new URL(text).hostname
    } catch {
      text = text.replace(/^[a-z][a-z0-9+.-]*:\/\//, '')
    }
  }
  text = text.replace(/^\[/, '').replace(/\]$/, '')
  if (text.includes('/') || text.includes('?') || text.includes('#')) {
    text = text.split(/[/?#]/)[0]
  }
  if (text.includes(':') && !text.includes('::')) {
    text = text.split(':')[0]
  }
  return text
}

function findRealmTargetNode(overview: XUIOverview | null | undefined, rule: RealmForwardRule): XUINodeView | null {
  const targetPort = Number(rule.target_port || 0)
  if (!overview || targetPort <= 0) {
    return null
  }
  const node = (overview.nodes || []).find((item) => Number(item.port || 0) === targetPort) ||
    (overview.nodes || []).find((item) => Number(item.id || 0) === targetPort)
  if (node) {
    return node
  }
  const client = (overview.clients || []).find((item) => Number(item.inbound_id || 0) === targetPort)
  if (!client) {
    return null
  }
  return {
    id: client.inbound_id,
    tag: client.inbound_tag || '',
    remark: client.inbound_remark || client.inbound_tag || `Inbound #${client.inbound_id}`,
    protocol: client.protocol || '',
    enabled: client.enabled !== false,
    route: client.route,
  }
}

function areaAssignmentDraftFromRealmRule(agentID: string, rule: RealmForwardRule, agents: DashboardAgentView[]): AreaManagerAssignmentDraft {
  const listenPort = Number(rule.listen_port || 0)
  const label = realmGrantLabel(rule)
  return {
    agent_id: agentID,
    inbound_id: listenPort,
    inbound_tag: `realm:${listenPort}`,
    client_email: '',
    public_client_name: `${agentName(agentID, agents)} / ${label}`,
    enabled: true,
  }
}

function realmGrantLabel(rule: RealmForwardRule) {
  const listen = rule.listen_port || '-'
  const name = (rule.name || '').trim()
  return name ? `${name} (${listen})` : `Realm 端口 ${listen}`
}

function realmAssignmentDisplayName(item: { agent_id: string; inbound_id: number; public_client_name?: string }, agents: DashboardAgentView[]): string {
  const agentPrefix = `${agentName(item.agent_id, agents)} / `
  const publicName = (item.public_client_name || '').trim()
  if (publicName) {
    return publicName.startsWith(agentPrefix) ? publicName : `${agentPrefix}${publicName}`
  }
  return `${agentPrefix}Realm 端口 ${item.inbound_id}`
}

function areaAssignmentDraftFromTargetOption(
  agentID: string,
  option: { client?: XUIClientView; node?: XUINodeView },
  agents: DashboardAgentView[],
): AreaManagerAssignmentDraft {
  if (option.client) {
    const client = option.client
    return {
      agent_id: agentID,
      inbound_id: client.inbound_id,
      inbound_tag: client.inbound_tag || '',
      client_email: client.email || '',
      public_client_name: defaultPublicClientName(client, agentID, agents),
      enabled: true,
    }
  }
  const node = option.node
  return {
    agent_id: agentID,
    inbound_id: node?.id || 0,
    inbound_tag: node?.tag || '',
    client_email: '',
    public_client_name: node ? defaultPublicNodeName(node, agentID, agents) : agentName(agentID, agents),
    enabled: true,
  }
}

function normalizeAreaManagerAssignmentDrafts(items: Array<AreaManagerAssignment | AreaManagerAssignmentDraft>): AreaManagerAssignmentDraft[] {
  const result: AreaManagerAssignmentDraft[] = []
  const seen = new Set<string>()
  for (const item of items || []) {
    if (isLegacyRealmForwardedClientAssignment(item)) {
      continue
    }
    const draft: AreaManagerAssignmentDraft = {
      agent_id: item.agent_id,
      inbound_id: Number(item.inbound_id || 0),
      inbound_tag: item.inbound_tag || '',
      client_email: item.client_email || '',
      public_client_name: item.public_client_name || item.client_email || item.inbound_tag || `Inbound #${item.inbound_id}`,
      enabled: item.enabled !== false,
    }
    if (!draft.agent_id || !draft.inbound_id) {
      continue
    }
    if (!draft.client_email && isRealmAssignmentTagValue(draft.inbound_tag)) {
      draft.inbound_tag = `realm:${draft.inbound_id}`
      draft.public_client_name = draft.public_client_name || `Realm ${draft.inbound_id}`
    }
    const key = areaAssignmentKey(draft)
    if (seen.has(key)) {
      continue
    }
    seen.add(key)
    result.push(draft)
  }
  return dedupeAreaManagerAssignmentDrafts(result)
}

function dedupeAreaManagerAssignmentDrafts(items: AreaManagerAssignmentDraft[]): AreaManagerAssignmentDraft[] {
  const result: AreaManagerAssignmentDraft[] = []
  const seen = new Set<string>()
  const exactClientAssignments = items.filter((item) => item.client_email && !isRealmAssignmentTagValue(item.inbound_tag || ''))
  for (const item of items) {
    if (!item.client_email && !isRealmAssignmentTagValue(item.inbound_tag || '') && exactClientAssignments.some((exact) => assignmentMatchesInbound(exact, item.agent_id, item.inbound_id, item.inbound_tag))) {
      continue
    }
    const key = areaAssignmentKey(item)
    if (seen.has(key)) {
      continue
    }
    seen.add(key)
    result.push(item)
  }
  return result
}

function assignmentMatchesInbound(item: { agent_id: string; inbound_id: number; inbound_tag?: string }, agentID: string, inboundID: number, inboundTag?: string): boolean {
  if ((item.agent_id || '') !== agentID || Number(item.inbound_id || 0) !== Number(inboundID || 0)) {
    return false
  }
  const leftTag = (item.inbound_tag || '').trim().toLowerCase()
  const rightTag = (inboundTag || '').trim().toLowerCase()
  return !leftTag || !rightTag || leftTag === rightTag
}

function firstRealmAssignmentAgentID(items: AreaManagerAssignment[], agents: DashboardAgentView[]): string {
  return normalizeAreaManagerAssignmentDrafts(items).find((item) => isRealmAssignmentDraft(item, agents))?.agent_id || ''
}

function firstXUIAssignmentAgentID(items: AreaManagerAssignment[], agents: DashboardAgentView[]): string {
  return normalizeAreaManagerAssignmentDrafts(items).find((item) => !isRealmAssignmentDraft(item, agents))?.agent_id || ''
}

function isRealmForwardedClientOption(client: XUIClientView): boolean {
  return Boolean(
    client.is_realm_forwarded ||
    client.realm_source_agent_id ||
    client.realm_target_agent_id ||
    looksLikeRealmForwardedInboundTag(client.inbound_tag || '') ||
    looksLikeRealmForwardedInboundTag(client.inbound_remark || ''),
  )
}

function isLegacyRealmForwardedClientAssignment(item: { inbound_tag?: string; client_email?: string; public_client_name?: string }): boolean {
  if (!item.client_email) {
    return false
  }
  return looksLikeRealmForwardedInboundTag(item.inbound_tag || '') || looksLikeRealmForwardedInboundTag(item.public_client_name || '')
}

function looksLikeRealmForwardedInboundTag(value: string): boolean {
  return /^realm\s+\d+\s*->/i.test(value.trim())
}

function isRealmAssignmentDraft(item: { agent_id: string; inbound_id: number; inbound_tag?: string; client_email?: string }, agents: DashboardAgentView[]) {
  if (item.client_email) {
    return false
  }
  if (isRealmAssignmentTagValue(item.inbound_tag || '')) {
    return true
  }
  const rules = agents.find((agent) => agent.agent_id === item.agent_id)?.entry?.port_forwarding?.rules || []
  return rules.some((rule) => Number(rule.listen_port || 0) === Number(item.inbound_id || 0))
}

function areaAssignmentKey(item: { agent_id: string; inbound_id: number; inbound_tag?: string; client_email?: string }): string {
  return [
    item.agent_id || '',
    String(item.inbound_id || 0),
    item.inbound_tag || '',
    item.client_email || '',
  ].map((part) => encodeURIComponent(part)).join('::')
}

function areaAssignmentLabel(item: { agent_id: string; inbound_id: number; inbound_tag?: string; client_email?: string; public_client_name?: string }, agents: DashboardAgentView[]): string {
  if (isRealmAssignmentDraft(item, agents)) {
    return realmAssignmentDisplayName(item, agents)
  }
  const scope = item.client_email ? item.client_email : '整个节点'
  const name = item.public_client_name || item.inbound_tag || `Inbound #${item.inbound_id}`
  return `${agentName(item.agent_id, agents)} / ${name} / ${scope}`
}

function renderAssignmentHierarchy(
  items: Array<{ agent_id: string; inbound_id: number; inbound_tag?: string; client_email?: string; public_client_name?: string }>,
  agents: DashboardAgentView[],
  onRemove?: (key: string) => void,
) {
  if (!items.length) {
    return <Tag>未分配</Tag>
  }
  const grouped = new Map<string, Map<string, Array<{ key: string; label: string; nodeLabel: string }>>>()
  for (const item of items) {
    const agentID = item.agent_id || ''
    const realm = isRealmAssignmentDraft(item, agents)
    const nodeLabelText = realm ? realmAssignmentDisplayName(item, agents) : item.inbound_tag || `Inbound #${item.inbound_id}`
    const nodeKeyText = `${item.inbound_id}\x00${nodeLabelText}`
    const clientLabelText = realm ? '端口授权' : item.client_email || '整个节点'
    if (!grouped.has(agentID)) {
      grouped.set(agentID, new Map())
    }
    const nodeMap = grouped.get(agentID)!
    if (!nodeMap.has(nodeKeyText)) {
      nodeMap.set(nodeKeyText, [])
    }
    nodeMap.get(nodeKeyText)!.push({
      key: areaAssignmentKey(item),
      label: clientLabelText,
      nodeLabel: nodeLabelText,
    })
  }
  return (
    <Space direction="vertical" size={4} style={{ width: '100%' }}>
      {Array.from(grouped.entries()).map(([agentID, nodeMap]) => (
        <div key={agentID}>
          <Text strong>{agentName(agentID, agents)}</Text>
          <Space direction="vertical" size={3} style={{ width: '100%', marginTop: 4, paddingLeft: 10 }}>
            {Array.from(nodeMap.entries()).map(([nodeKeyText, clients]) => (
              <div key={nodeKeyText}>
                <Tag color="blue">{clients[0]?.nodeLabel || '-'}</Tag>
                <Space size={[4, 4]} wrap>
                  {clients.map((client) => (
                    <Tag
                      key={client.key}
                      closable={Boolean(onRemove)}
                      onClose={(event) => {
                        event.preventDefault()
                        onRemove?.(client.key)
                      }}
                    >
                      {client.label}
                    </Tag>
                  ))}
                </Space>
              </div>
            ))}
          </Space>
        </div>
      ))}
    </Space>
  )
}

function isRealmAssignmentTagValue(value: string): boolean {
  return value.trim().toLowerCase().startsWith('realm:')
}

function uniqueStrings(values: string[]): string[] {
  return Array.from(new Set(values.map((value) => value.trim()).filter(Boolean)))
}

function clientLabel(client: XUIClientView): string {
  const inbound = client.inbound_remark || client.inbound_tag || `Inbound #${client.inbound_id}`
  const email = client.email || '未指定客户端'
  return `客户端：${inbound} / ${email}`
}

function clientTreeTitle(client: XUIClientView): string {
  return ['客户端', client.email || '未指定客户端', client.comment || client.sub_id || ''].filter(Boolean).join(' / ')
}

function nodeKey(node: XUINodeView): string {
  return `node:${node.id}::`
}

function nodeLabel(node: XUINodeView): string {
  return `节点：${node.remark || node.tag || `Inbound #${node.id}`} / ${node.protocol || '-'}`
}

function defaultPublicClientName(client: XUIClientView, agentID: string, agents: DashboardAgentView[]): string {
  const agent = customerAgentName(agentID, agents)
  const node = client.inbound_remark || client.inbound_tag || `Inbound #${client.inbound_id}`
  return [agent, node, client.email || client.comment || client.sub_id].filter(Boolean).join(' - ')
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
