import type { Dispatch, SetStateAction } from 'react'
import { Alert, Button, Card, Col, Empty, Input, InputNumber, Row, Select, Switch, Table, TreeSelect, Typography } from 'antd'
import type { TreeSelectProps } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { SaveOutlined } from '@ant-design/icons'

import type { CustomerAdminView, CustomerAssignment, CustomerAssignmentDraft } from '../types'
import { REVENUE_CURRENCIES } from '../lib/currency'
import type { AssignmentFormState } from './CustomerManagementHelpers'
import { emptyAssignmentForm } from './CustomerManagementHelpers'

const { Text, Title } = Typography

export interface CustomerAssignmentManagerCardProps {
  canViewFinance: boolean
  selectedCustomer: CustomerAdminView | null
  selectedCustomerID: number | null
  editingAssignmentID: number | null
  assignmentForm: AssignmentFormState
  setAssignmentForm: Dispatch<SetStateAction<AssignmentFormState>>
  savingAssignment: boolean
  overviewLoading: boolean
  agentOptions: Array<{ value: string; label: string }>
  clientTreeData: TreeSelectProps['treeData']
  visibleAssignmentColumns: ColumnsType<CustomerAssignment>
  onReset: () => void
  onSelectClient: (value: string) => void
  onSaveAssignment: () => void
}

export function CustomerAssignmentManagerCard(props: CustomerAssignmentManagerCardProps) {
  const {
    canViewFinance,
    selectedCustomer,
    selectedCustomerID,
    editingAssignmentID,
    assignmentForm,
    setAssignmentForm,
    savingAssignment,
    overviewLoading,
    agentOptions,
    clientTreeData,
    visibleAssignmentColumns,
    onReset,
    onSelectClient,
    onSaveAssignment,
  } = props

  return (
    <Card className="customer-admin-card" bordered={false}>
      <div className="customer-admin-card-head">
        <div>
          <Title level={5}>授权链路分配</Title>
          <Text type="secondary">当前用户：{selectedCustomer ? selectedCustomer.display_name || selectedCustomer.username : '未选择'}</Text>
        </div>
        <Button onClick={onReset}>清空表单</Button>
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
            onChange={(value) => onSelectClient(String(value || ''))}
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
      <Button style={{ marginTop: 14 }} type="primary" icon={<SaveOutlined />} disabled={!selectedCustomerID} loading={savingAssignment} onClick={onSaveAssignment}>
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
}
