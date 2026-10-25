import { Button, Empty, Popconfirm, Space, Table, Tag, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { CopyOutlined, DeleteOutlined, EditOutlined, ExportOutlined, LinkOutlined, SyncOutlined } from '@ant-design/icons'

import type { AreaManagerAdminView, AreaManagerAssignment, CustomerAdminView, CustomerAssignment, CustomerBillingDiagnostic, DashboardAgentView } from '../types'
import { formatDateTime } from '../lib/appHelpers'
import { DEFAULT_ACCOUNT_PASSWORD, agentName, assignmentBindingDiagnostic, effectiveAssignmentBilling, normalizeAreaManagerAssignmentDrafts, renderAssignmentHierarchy, revenueCycleLabel } from './CustomerManagementHelpers'

const { Text } = Typography

interface CustomerManagementTableOptions {
  agents: DashboardAgentView[]
  canViewFinance: boolean
  onOpenAssignment: ((record: CustomerAssignment) => void) | undefined
  editAssignment: (record: CustomerAssignment) => void
  deleteAssignment: (id: number) => void
  repairingAssignmentID: number | null
  rebindBillingAssignment: (record: CustomerBillingDiagnostic) => void
  resettingCustomerID: number | null
  openAssignmentManagerModal: (record: CustomerAdminView) => void
  openCustomerSubscriptionModal: (record: CustomerAdminView) => void
  resetCustomerPassword: (id: number) => void
  editAreaManager: (record: AreaManagerAdminView) => void
  copyAreaManagerRecommendationURL: (record: AreaManagerAdminView) => void
  resetAreaManagerPassword: (id: number) => void
  resettingAreaManagerID: number | null
  deleteAreaManager: (id: number) => void
  openAssignmentViewModal: (record: CustomerAdminView) => void
  openCustomerEditModal: (record: CustomerAdminView) => void
  deleteCustomer: (id: number) => void
  changingCustomerStatusIDs: number[]
  toggleCustomerStatus: (record: CustomerAdminView) => void
  deleteAreaManagerAssignment: (managerID: number, assignmentID: number) => void
}

export function buildCustomerManagementTables({
  agents,
  canViewFinance,
  onOpenAssignment,
  editAssignment,
  deleteAssignment,
  repairingAssignmentID,
  rebindBillingAssignment,
  resettingCustomerID,
  openAssignmentManagerModal,
  openCustomerSubscriptionModal,
  resetCustomerPassword,
  editAreaManager,
  copyAreaManagerRecommendationURL,
  resetAreaManagerPassword,
  resettingAreaManagerID,
  deleteAreaManager,
  openAssignmentViewModal,
  openCustomerEditModal,
  deleteCustomer,
  changingCustomerStatusIDs,
  toggleCustomerStatus,
  deleteAreaManagerAssignment,
}: CustomerManagementTableOptions) {
  const assignmentColumns: ColumnsType<CustomerAssignment> = [
    {
      title: '授权链路名称',
      dataIndex: 'public_client_name',
      width: 240,
      fixed: 'left',
      render: (value?: string) => <Text strong style={{ wordBreak: 'break-word' }}>{value || '-'}</Text>,
    },
    {
      title: 'Client / 节点 / 客户端',
      width: 320,
      render: (_, record) => renderAssignmentHierarchy([record], agents),
    },
    {
      title: '用户备注',
      dataIndex: 'remark',
      width: 140,
      ellipsis: true,
      onHeaderCell: () => ({ style: { whiteSpace: 'nowrap', textOverflow: 'clip' } }),
      render: (value?: string) => value || '-',
    },
    {
      title: '费用',
      key: 'revenue',
      width: 220,
      render: (_, record) => {
        const billing = effectiveAssignmentBilling(record, agents)
        return billing && Number(billing.revenue_amount || 0) > 0
          ? <Space size={4} wrap={false}><span style={{ whiteSpace: 'nowrap' }}>{billing.revenue_currency || 'CNY'} {Number(billing.revenue_amount || 0).toFixed(2)} / {revenueCycleLabel(billing.revenue_cycle)}</span><Tag color={record.price_mode === 'override' ? 'gold' : 'blue'}>{record.price_mode === 'override' ? '用户覆盖' : '节点默认'}</Tag></Space>
          : <Tag>未设置</Tag>
      },
    },
    {
      title: '绑定状态',
      key: 'binding',
      width: 120,
      render: (_, record) => {
        const diagnostic = assignmentBindingDiagnostic(record, agents)
        const color = diagnostic.status === 'matched' ? 'green' : diagnostic.status === 'legacy' ? 'blue' : diagnostic.status === 'mismatch' ? 'orange' : 'red'
        return <Tag color={color}>{diagnostic.label}</Tag>
      },
    },
    {
      title: '流量倍率',
      key: 'traffic_multiplier',
      width: 100,
      render: (_, record) => `${Number(effectiveAssignmentBilling(record, agents)?.traffic_multiplier || 1)} 倍`,
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
      width: 240,
      fixed: 'right',
      render: (_, record) => (
        <Space size={6} wrap={false}>
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

  const billingReconciliationColumns: ColumnsType<CustomerBillingDiagnostic> = [
    {
      title: '用户 / 授权',
      width: 190,
      render: (_, record) => <div><Text strong>{record.customer_name || `用户 #${record.customer_id}`}</Text><div className="muted-line">{record.agent_id} / {record.inbound_tag || `Inbound #${record.inbound_id}`}</div></div>,
    },
    {
      title: '已存绑定',
      width: 220,
      render: (_, record) => <div>{record.client_email || '节点级授权'}<div className="muted-line">{record.client_id || '未保存稳定 ID'}</div></div>,
    },
    {
      title: '当前 x-ui 客户端',
      width: 220,
      render: (_, record) => record.current_client_id
        ? <div>{record.current_client_name || record.current_client_email}<div className="muted-line">{record.current_client_id}</div></div>
        : '-',
    },
    {
      title: '结果',
      width: 220,
      render: (_, record) => {
        const color = record.status === 'matched' ? 'green' : record.status === 'legacy' ? 'blue' : record.status === 'mismatch' ? 'orange' : 'red'
        const label = record.status === 'matched' ? '已匹配' : record.status === 'legacy' ? '旧数据' : record.status === 'mismatch' ? '需修复' : '未找到'
        return <Space direction="vertical" size={2}><Tag color={color}>{label}</Tag><Text type="secondary">{record.message}</Text></Space>
      },
    },
    {
      title: '操作',
      width: 110,
      render: (_, record) => record.can_rebind
        ? <Button size="small" type="primary" loading={repairingAssignmentID === record.assignment_id} onClick={() => void rebindBillingAssignment(record)}>重新绑定</Button>
        : '-',
    },
  ]

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
      width: 120,
      render: (_, record) => renderCustomerStatusButton(record),
    },
    {
      title: '操作',
      width: 330,
      render: (_, record) => (
        <Space size={6} wrap>
          <Button size="small" onClick={() => openAssignmentManagerModal(record)}>管理链路</Button>
          <Button size="small" icon={<CopyOutlined />} disabled={!record.enabled} onClick={() => openCustomerSubscriptionModal(record)}>导出 Clash</Button>
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
      width: 350,
      render: (_, record) => (
        <Space size={6} wrap>
          <Button size="small" icon={<EditOutlined />} onClick={() => editAreaManager(record)}>编辑</Button>
          <Button size="small" icon={<LinkOutlined />} onClick={() => void copyAreaManagerRecommendationURL(record)}>推荐页 URL</Button>
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
      sorter: (left, right) => left.username.localeCompare(right.username, 'zh-CN'),
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
      sorter: (left, right) => (left.assignments?.length || 0) - (right.assignments?.length || 0),
      render: (_, record) => (
        <Button size="small" onClick={() => openAssignmentViewModal(record)}>
          查看 {record.assignments?.length || 0} 条
        </Button>
      ),
    },
    {
      title: '状态',
      dataIndex: 'enabled',
      width: 120,
      sorter: (left, right) => Number(left.enabled) - Number(right.enabled),
      render: (_, record) => renderCustomerStatusButton(record),
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      width: 180,
      sorter: (left, right) => timestampForSort(left.created_at) - timestampForSort(right.created_at),
      render: (value: string) => formatDateTime(value),
    },
    {
      title: '修改时间',
      dataIndex: 'updated_at',
      width: 180,
      defaultSortOrder: 'descend',
      sorter: (left, right) => timestampForSort(left.updated_at) - timestampForSort(right.updated_at),
      render: (value: string) => formatDateTime(value),
    },
    {
      title: '操作',
      width: 510,
      render: (_, record) => (
        <Space size={6} wrap>
          <Button size="small" icon={<EditOutlined />} onClick={() => openCustomerEditModal(record)}>编辑</Button>
          <Button size="small" type="primary" onClick={() => openAssignmentManagerModal(record)}>管理链路</Button>
          <Button size="small" icon={<CopyOutlined />} disabled={!record.enabled} onClick={() => openCustomerSubscriptionModal(record)}>导出 Clash</Button>
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

  function timestampForSort(value?: string) {
    const timestamp = value ? Date.parse(value) : Number.NaN
    return Number.isFinite(timestamp) ? timestamp : 0
  }

  function renderCustomerStatusButton(record: CustomerAdminView) {
    const nextStatusLabel = record.enabled ? '禁用' : '启用'
    return (
      <Button
        size="small"
        type={record.enabled ? 'primary' : 'default'}
        ghost={record.enabled}
        danger={!record.enabled}
        style={{ minWidth: 72 }}
        loading={changingCustomerStatusIDs.includes(record.id)}
        title={`点击${nextStatusLabel}`}
        aria-label={`${nextStatusLabel}用户 ${record.display_name || record.username}`}
        onClick={() => void toggleCustomerStatus(record)}
      >
        {record.enabled ? '启用' : '禁用'}
      </Button>
    )
  }

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

  return { visibleAssignmentColumns, billingReconciliationColumns, areaManagerColumns, customerColumns, renderAreaManagerCustomers }
}
