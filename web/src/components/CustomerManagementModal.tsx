import { useEffect, useMemo, useRef, useState } from 'react'
import { Alert, App as AntdApp, Button, Card, Col, Empty, Modal, Popconfirm, Row, Select, Space, Spin, Table, Tabs, Tag, TreeSelect, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { DeleteOutlined, EditOutlined, ExportOutlined, PlusOutlined, ReloadOutlined, SaveOutlined, SyncOutlined } from '@ant-design/icons'

import type { AdminUser, AreaManagerAdminView, AreaManagerAssignment, CustomerAdminView, CustomerAssignment, CustomerAssignmentDraft, CustomerAssignmentSourceView, DashboardAgentView, XUIClientBillingConfig, XUIClientView, XUINodeView, XUIOverview } from '../types'
import { fetchJSON } from '../lib/appHelpers'
import { CustomerAssignmentManagerCard } from './CustomerAssignmentManagerCard'
import {
  DEFAULT_ACCOUNT_PASSWORD,
  type AreaBatchAssignmentFormState,
  type AreaManagerAssignmentDraft,
  type AreaManagerFormState,
  type AreaManagerOutboundGrantDraft,
  type AssignmentFormState,
  type CustomerFormState,
  type ManagementTabKey,
  areaAssignmentDraftFromTargetOption,
  areaAssignmentKey,
  agentName,
  assignmentBilling,
  assignmentFormFromAssignment,
  assignmentFormFromDraft,
  assignmentMatchesInbound,
  assignmentNodeKeys,
  billingFormPatch,
  buildAreaAssignmentTreeData,
  buildAssignmentTargetOptions,
  buildClientAssignmentTreeData,
  buildRealmGrantOptions,
  clientBilling,
  clientKey,
  clientLabel,
  defaultPublicClientName,
  defaultPublicNodeName,
  dedupeAreaManagerAssignmentDrafts,
  emptyAreaBatchAssignmentForm,
  emptyAreaManagerForm,
  emptyAssignmentForm,
  emptyCustomerForm,
  findMatchingAssignment,
  findRealmTargetNode,
  firstRealmAssignmentAgentID,
  firstXUIAssignmentAgentID,
  isAreaManagerAdminUser,
  isRealmAssignmentDraft,
  nodeKey,
  nodeLabel,
  normalizeAreaManagerAssignmentDrafts,
  normalizeAreaManagerOutboundGrants,
  realmRuleTargetAgentID,
  renderAssignmentHierarchy,
  revenueCycleLabel,
  uniqueStrings,
} from './CustomerManagementHelpers'
import { CustomerManagementModals } from './CustomerManagementModals'

const { Text, Title } = Typography

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
  const [customerAssignmentSources, setCustomerAssignmentSources] = useState<CustomerAssignmentSourceView[]>([])
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
  const [areaManagerOutboundOverview, setAreaManagerOutboundOverview] = useState<XUIOverview | null>(null)
  const [areaManagerOutboundOverviewLoading, setAreaManagerOutboundOverviewLoading] = useState(false)
  const skipAssignmentResetRef = useRef(false)

  const selectedCustomer = customers.find((item) => item.id === selectedCustomerID) || null
  const agentOptions = useMemo(() => agents.map((agent) => ({
    value: agent.agent_id,
    label: agent.agent_name || agent.agent_id,
  })), [agents])
  const customerAssignmentAgentOptions = useMemo(() => canManageAreaManagers
    ? agentOptions
    : customerAssignmentSources.map((source) => ({
      value: source.agent_id,
      label: source.agent_name || source.agent_id,
    })), [agentOptions, canManageAreaManagers, customerAssignmentSources])
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
  const areaManagerOutboundGrantAgentID = areaManagerForm.outbound_grant_agent_id
  const areaManagerGrantOptions = useMemo(() => buildAssignmentTargetOptions(areaManagerOverview), [areaManagerOverview])
  const areaManagerGrantTreeData = useMemo(() => buildAreaAssignmentTreeData(areaManagerXUIGrantAgentID, areaManagerOverview, agents), [areaManagerXUIGrantAgentID, areaManagerOverview, agents])
  const areaManagerRealmGrantOptions = useMemo(() => buildRealmGrantOptions(areaManagerForm.grant_agent_id, agents), [areaManagerForm.grant_agent_id, agents])
  const areaManagerOutboundGrantOptions = useMemo(() => (areaManagerOutboundOverview?.outbounds || [])
    .flatMap((outbound) => outbound.tag ? [{
      value: outbound.tag,
      label: [outbound.tag, outbound.protocol].filter(Boolean).join(' / '),
    }] : []), [areaManagerOutboundOverview])
  const areaManagerOutboundAgentOptions = useMemo(() => {
    const authorizedAgentIDs = new Set(areaManagerForm.assignments.map((assignment) => assignment.agent_id))
    return agentOptions.filter((option) => authorizedAgentIDs.has(option.value))
  }, [agentOptions, areaManagerForm.assignments])

  useEffect(() => {
    if (active) {
      void loadCustomers()
      if (canManageAreaManagers) {
        void loadAreaManagers()
      } else {
        void loadCustomerAssignmentSources()
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

  useEffect(() => {
    if (!areaManagerOutboundGrantAgentID) {
      setAreaManagerOutboundOverview(null)
      return
    }
    let cancelled = false
    setAreaManagerOutboundOverviewLoading(true)
    void fetchJSON<XUIOverview>(`/api/v1/agents/${areaManagerOutboundGrantAgentID}/xui/overview?assignment_scope=1`)
      .then((data) => {
        if (!cancelled) {
          setAreaManagerOutboundOverview(data)
        }
      })
      .catch((error) => {
        if (!cancelled) {
          setAreaManagerOutboundOverview(null)
          message.warning(error instanceof Error ? error.message : '加载 x-ui 出站失败')
        }
      })
      .finally(() => {
        if (!cancelled) {
          setAreaManagerOutboundOverviewLoading(false)
        }
      })
    return () => {
      cancelled = true
    }
  }, [areaManagerOutboundGrantAgentID])

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
      title: '流量倍率',
      key: 'traffic_multiplier',
      width: 100,
      render: (_, record) => `${Number(assignmentBilling(record, agents)?.traffic_multiplier || 1)} 倍`,
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
    : assignmentColumns.filter((column) => !['revenue', 'traffic_multiplier'].includes(String(column.key || '')))

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
        const directCount = normalizeAreaManagerAssignmentDrafts(record.assignments || [], agents).length
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
        const assignments = normalizeAreaManagerAssignmentDrafts(record.assignments || [], agents)
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
      title: '出站权限',
      width: 180,
      render: (_, record) => (
        <Space size={[4, 4]} wrap>
	          <Tag color={record.outbound_create_enabled ? 'green' : 'default'}>{record.outbound_create_enabled ? '允许节点落地' : '禁止节点落地'}</Tag>
          <Tag color={record.outbound_grants?.length ? 'cyan' : 'default'}>{record.outbound_grants?.length || 0} 个已有出站</Tag>
        </Space>
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
    const assignments = normalizeAreaManagerAssignmentDrafts(areaManagerForm.assignments, agents)
    const authorizedAgentIDs = new Set(assignments.map((assignment) => assignment.agent_id))
    const outboundGrants = normalizeAreaManagerOutboundGrants(areaManagerForm.outbound_grants)
      .filter((grant) => authorizedAgentIDs.has(grant.agent_id))
    if (!assignments.length && !outboundGrants.length) {
      message.warning('请至少授权一个节点 / 客户端或已有出站')
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
        outbound_create_enabled: areaManagerForm.outbound_create_enabled,
        outbound_grants: outboundGrants,
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
      message.warning('请选择转发入口 Client')
      return
    }
    if (areaBatchForm.selected_xui_keys.length && !xuiAgentID) {
      message.warning('请选择 x-ui 出口 Client')
      return
    }
    if (!areaBatchForm.selected_realm_keys.length && !areaBatchForm.selected_xui_keys.length) {
      message.warning('请选择要批量授权的转发入口或 x-ui 客户端 / 节点')
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
      outbound_create_enabled: Boolean(record.outbound_create_enabled),
      outbound_grant_agent_id: record.outbound_grants?.[0]?.agent_id || '',
      outbound_grants: normalizeAreaManagerOutboundGrants(record.outbound_grants || []),
      assignments: normalizeAreaManagerAssignmentDrafts(record.assignments || [], agents),
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

  async function loadCustomerAssignmentSources() {
    try {
      const sources = await fetchJSON<CustomerAssignmentSourceView[]>('/api/v1/admin/customers/assignment-sources')
      setCustomerAssignmentSources(sources)
    } catch (error) {
      setCustomerAssignmentSources([])
      message.error(error instanceof Error ? error.message : '加载可分配入口失败')
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
          traffic_multiplier: assignmentForm.traffic_multiplier,
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
          traffic_multiplier: Number(payload.traffic_multiplier ?? assignmentForm.traffic_multiplier ?? 1),
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
        outbound_grants: current.outbound_grants.filter((grant) => assignments.some((assignment) => assignment.agent_id === grant.agent_id)),
        outbound_grant_agent_id: assignments.some((assignment) => assignment.agent_id === current.outbound_grant_agent_id) ? current.outbound_grant_agent_id : '',
      }
    })
  }

  function updateAreaManagerOutboundGrantTargets(tags: string[]) {
    const agentID = areaManagerOutboundGrantAgentID
    if (!agentID) {
      return
    }
    const allowedTags = new Set(areaManagerOutboundGrantOptions.map((option) => option.value))
    const nextForAgent: AreaManagerOutboundGrantDraft[] = tags
      .filter((tag): tag is string => Boolean(tag) && allowedTags.has(tag))
      .map((outboundTag) => ({ agent_id: agentID, outbound_tag: outboundTag }))
    setAreaManagerForm((current) => {
      const outboundGrants = normalizeAreaManagerOutboundGrants([
        ...current.outbound_grants.filter((grant) => grant.agent_id !== agentID),
        ...nextForAgent,
      ])
      return {
        ...current,
        outbound_grants: outboundGrants,
        agent_ids: uniqueStrings(current.assignments.map((assignment) => assignment.agent_id)),
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
        outbound_grants: current.outbound_grants.filter((grant) => assignments.some((assignment) => assignment.agent_id === grant.agent_id)),
        outbound_grant_agent_id: assignments.some((assignment) => assignment.agent_id === current.outbound_grant_agent_id) ? current.outbound_grant_agent_id : '',
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
      const resolvedTargetAgentID = node.realm_target_agent_id || targetAgentID
      const resolvedNode = node.realm_target_agent_id
        ? {
            ...node,
            id: node.realm_target_inbound_id || node.id,
            tag: node.realm_target_inbound_tag || node.tag,
            port: node.realm_target_inbound_id || node.port,
          }
        : node
      if (excludedTargetAssignments.some((item) => assignmentMatchesInbound(item, resolvedTargetAgentID, resolvedNode.id, resolvedNode.tag))) {
        continue
      }
      inferred.push(areaAssignmentDraftFromTargetOption(resolvedTargetAgentID, { node: resolvedNode }, agents))
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
        outbound_grants: current.outbound_grants.filter((grant) => assignments.some((assignment) => assignment.agent_id === grant.agent_id)),
        outbound_grant_agent_id: assignments.some((assignment) => assignment.agent_id === current.outbound_grant_agent_id) ? current.outbound_grant_agent_id : '',
      }
    })
  }

  function removeAreaManagerOutboundGrant(agentID: string, outboundTag: string) {
    setAreaManagerForm((current) => {
      const outboundGrants = current.outbound_grants.filter((grant) => grant.agent_id !== agentID || grant.outbound_tag !== outboundTag)
      return {
        ...current,
        outbound_grants: outboundGrants,
        agent_ids: uniqueStrings(current.assignments.map((assignment) => assignment.agent_id)),
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
            <Text type="secondary">Realm / HAProxy 入口与 x-ui 出口节点可分别选择；HAProxy 主备会自动映射到校验一致的最终节点。</Text>
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
            <Text type="secondary">转发入口 Client</Text>
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
              <Text type="secondary">转发入口</Text>
              <Button size="small" disabled={!areaBatchForm.agent_id || !areaBatchRealmOptions.length} onClick={() => setAreaBatchForm((current) => ({ ...current, selected_realm_keys: areaBatchRealmOptions.map((option) => option.value) }))}>全选</Button>
            </Space>
            <Select
              mode="multiple"
              style={{ width: '100%' }}
              showSearch
              placeholder="选择 Realm / HAProxy 入口"
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

  const readOnlyAssignmentColumns = visibleAssignmentColumns.filter((column) => column.key !== 'actions')
  const assignmentManagerContent = (
    <CustomerAssignmentManagerCard
      canViewFinance={canViewFinance}
      selectedCustomer={selectedCustomer}
      selectedCustomerID={selectedCustomerID}
      editingAssignmentID={editingAssignmentID}
      assignmentForm={assignmentForm}
      setAssignmentForm={setAssignmentForm}
      savingAssignment={savingAssignment}
      overviewLoading={overviewLoading}
      agentOptions={customerAssignmentAgentOptions}
      clientTreeData={clientTreeData}
      visibleAssignmentColumns={visibleAssignmentColumns}
      onReset={() => {
        setEditingAssignmentID(null)
        setAssignmentForm(emptyAssignmentForm)
      }}
      onSelectClient={(value) => selectClient(value)}
      onSaveAssignment={() => void saveAssignment()}
    />
  )

  const selectedAreaManagerRealmKeys = areaManagerForm.assignments
    .filter((assignment) => assignment.agent_id === areaManagerForm.grant_agent_id && isRealmAssignmentDraft(assignment, agents))
    .map(areaAssignmentKey)
  const selectedAreaManagerXUIKeys = areaManagerForm.assignments
    .filter((assignment) => assignment.agent_id === areaManagerXUIGrantAgentID && !isRealmAssignmentDraft(assignment, agents))
    .map(areaAssignmentKey)
  const selectedAreaManagerOutboundTags = areaManagerForm.outbound_grants
    .filter((grant) => grant.agent_id === areaManagerOutboundGrantAgentID)
    .map((grant) => grant.outbound_tag)
  const selectedCustomerTitle = selectedCustomer ? selectedCustomer.display_name || selectedCustomer.username : '未选择用户'

  const accountEditorModals = (
    <CustomerManagementModals
      canManageAreaManagers={canManageAreaManagers}
      agents={agents}
      agentOptions={agentOptions}
      areaManagerOutboundAgentOptions={areaManagerOutboundAgentOptions}
      editingAreaManagerID={editingAreaManagerID}
      areaManagerModalOpen={areaManagerModalOpen}
      areaManagerForm={areaManagerForm}
      setAreaManagerForm={setAreaManagerForm}
      savingAreaManager={savingAreaManager}
      onCloseAreaManagerModal={closeAreaManagerModal}
      onSaveAreaManager={() => void saveAreaManager()}
      areaManagerXUIGrantAgentID={areaManagerXUIGrantAgentID}
      areaManagerOverview={areaManagerOverview}
      areaManagerOverviewLoading={areaManagerOverviewLoading}
      areaManagerOutboundOverviewLoading={areaManagerOutboundOverviewLoading}
      areaManagerGrantTreeData={areaManagerGrantTreeData}
      areaManagerRealmGrantOptions={areaManagerRealmGrantOptions.map((option) => ({ value: option.value, label: option.label }))}
      areaManagerOutboundGrantOptions={areaManagerOutboundGrantOptions}
      selectedAreaManagerRealmKeys={selectedAreaManagerRealmKeys}
      selectedAreaManagerXUIKeys={selectedAreaManagerXUIKeys}
      selectedAreaManagerOutboundTags={selectedAreaManagerOutboundTags}
      onUpdateAreaManagerRealmGrantTargets={(values) => void updateAreaManagerRealmGrantTargets(values)}
      onUpdateAreaManagerGrantTargets={(values) => updateAreaManagerGrantTargets(values)}
      onUpdateAreaManagerOutboundGrantTargets={updateAreaManagerOutboundGrantTargets}
      onRemoveAreaManagerGrant={removeAreaManagerGrant}
      onRemoveAreaManagerOutboundGrant={removeAreaManagerOutboundGrant}
      customerCreateModalOpen={customerCreateModalOpen}
      setCustomerCreateModalOpen={setCustomerCreateModalOpen}
      customerCreateForm={customerCreateForm}
      setCustomerCreateForm={setCustomerCreateForm}
      customerEditModalOpen={customerEditModalOpen}
      setCustomerEditModalOpen={setCustomerEditModalOpen}
      customerForm={customerForm}
      setCustomerForm={setCustomerForm}
      selectedCustomerID={selectedCustomerID}
      savingCustomer={savingCustomer}
      onCreateCustomer={() => void createCustomer()}
      onSaveCustomer={() => void saveCustomer()}
      assignmentViewModalOpen={assignmentViewModalOpen}
      setAssignmentViewModalOpen={setAssignmentViewModalOpen}
      assignmentManagerModalOpen={assignmentManagerModalOpen}
      setAssignmentManagerModalOpen={setAssignmentManagerModalOpen}
      setEditingAssignmentID={setEditingAssignmentID}
      setAssignmentForm={setAssignmentForm}
      selectedCustomerTitle={selectedCustomerTitle}
      selectedCustomerAssignments={selectedCustomer?.assignments || []}
      readOnlyAssignmentColumns={readOnlyAssignmentColumns}
      assignmentManagerContent={assignmentManagerContent}
    />
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
