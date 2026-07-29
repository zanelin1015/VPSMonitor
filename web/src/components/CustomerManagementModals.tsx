import type { Dispatch, ReactNode, SetStateAction } from 'react'
import { Button, Card, Col, Empty, Input, InputNumber, Modal, Row, Select, Space, Switch, Table, Tag, TreeSelect, Typography } from 'antd'
import type { TreeSelectProps } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { SaveOutlined } from '@ant-design/icons'

import type { AreaManagerAssignment, CustomerAssignment, XUIOverview } from '../types'
import { REVENUE_CURRENCIES } from '../lib/currency'
import type { AreaManagerFormState, AssignmentFormState, CustomerFormState } from './CustomerManagementHelpers'
import {
  areaAssignmentKey,
  assignmentNodeKeys,
  DEFAULT_ACCOUNT_PASSWORD,
  emptyAssignmentForm,
  emptyCustomerForm,
  isRealmAssignmentDraft,
  renderAssignmentHierarchy,
} from './CustomerManagementHelpers'

const { Text } = Typography

export interface CustomerManagementModalsProps {
  canManageAreaManagers: boolean
  agents: Parameters<typeof renderAssignmentHierarchy>[1]
  agentOptions: Array<{ value: string; label: string }>
  areaManagerOutboundAgentOptions: Array<{ value: string; label: string }>
  editingAreaManagerID: number | null
  areaManagerModalOpen: boolean
  areaManagerForm: AreaManagerFormState
  setAreaManagerForm: Dispatch<SetStateAction<AreaManagerFormState>>
  savingAreaManager: boolean
  onCloseAreaManagerModal: () => void
  onSaveAreaManager: () => void
  areaManagerXUIGrantAgentID: string
  areaManagerOverview: XUIOverview | null
  areaManagerOverviewLoading: boolean
  areaManagerOutboundOverviewLoading: boolean
  areaManagerGrantTreeData: TreeSelectProps['treeData']
  areaManagerRealmGrantOptions: Array<{ value: string; label: string }>
  areaManagerOutboundGrantOptions: Array<{ value: string; label: string }>
  selectedAreaManagerRealmKeys: string[]
  selectedAreaManagerXUIKeys: string[]
  selectedAreaManagerOutboundTags: string[]
  onUpdateAreaManagerRealmGrantTargets: (values: string[]) => void
  onUpdateAreaManagerGrantTargets: (values: string[]) => void
  onUpdateAreaManagerOutboundGrantTargets: (values: string[]) => void
  onRemoveAreaManagerGrant: (key: string) => void
  onRemoveAreaManagerOutboundGrant: (agentID: string, outboundTag: string) => void

  customerCreateModalOpen: boolean
  setCustomerCreateModalOpen: Dispatch<SetStateAction<boolean>>
  customerCreateForm: CustomerFormState
  setCustomerCreateForm: Dispatch<SetStateAction<CustomerFormState>>
  customerEditModalOpen: boolean
  setCustomerEditModalOpen: Dispatch<SetStateAction<boolean>>
  customerForm: CustomerFormState
  setCustomerForm: Dispatch<SetStateAction<CustomerFormState>>
  selectedCustomerID: number | null
  savingCustomer: boolean
  onCreateCustomer: () => void
  onSaveCustomer: () => void

  assignmentViewModalOpen: boolean
  setAssignmentViewModalOpen: Dispatch<SetStateAction<boolean>>
  assignmentManagerModalOpen: boolean
  setAssignmentManagerModalOpen: Dispatch<SetStateAction<boolean>>
  setEditingAssignmentID: Dispatch<SetStateAction<number | null>>
  setAssignmentForm: Dispatch<SetStateAction<AssignmentFormState>>
  selectedCustomerTitle: string
  selectedCustomerAssignments: CustomerAssignment[]
  readOnlyAssignmentColumns: ColumnsType<CustomerAssignment>
  assignmentManagerContent: ReactNode
}

export function CustomerManagementModals(props: CustomerManagementModalsProps) {
  const {
    canManageAreaManagers,
    agents,
    agentOptions,
    areaManagerOutboundAgentOptions,
    editingAreaManagerID,
    areaManagerModalOpen,
    areaManagerForm,
    setAreaManagerForm,
    savingAreaManager,
    onCloseAreaManagerModal,
    onSaveAreaManager,
    areaManagerXUIGrantAgentID,
    areaManagerOverview,
    areaManagerOverviewLoading,
    areaManagerOutboundOverviewLoading,
    areaManagerGrantTreeData,
    areaManagerRealmGrantOptions,
    areaManagerOutboundGrantOptions,
    selectedAreaManagerRealmKeys,
    selectedAreaManagerXUIKeys,
    selectedAreaManagerOutboundTags,
    onUpdateAreaManagerRealmGrantTargets,
    onUpdateAreaManagerGrantTargets,
    onUpdateAreaManagerOutboundGrantTargets,
    onRemoveAreaManagerGrant,
    onRemoveAreaManagerOutboundGrant,
    customerCreateModalOpen,
    setCustomerCreateModalOpen,
    customerCreateForm,
    setCustomerCreateForm,
    customerEditModalOpen,
    setCustomerEditModalOpen,
    customerForm,
    setCustomerForm,
    selectedCustomerID,
    savingCustomer,
    onCreateCustomer,
    onSaveCustomer,
    assignmentViewModalOpen,
    setAssignmentViewModalOpen,
    assignmentManagerModalOpen,
    setAssignmentManagerModalOpen,
    setEditingAssignmentID,
    setAssignmentForm,
    selectedCustomerTitle,
    selectedCustomerAssignments,
    readOnlyAssignmentColumns,
    assignmentManagerContent,
  } = props

  return (
    <>
      {canManageAreaManagers ? (
        <Modal
          title={editingAreaManagerID ? '编辑区域账号' : '新增区域账号'}
          open={areaManagerModalOpen}
          onCancel={onCloseAreaManagerModal}
          footer={(
            <Space>
              <Button onClick={onCloseAreaManagerModal}>取消</Button>
              <Button type="primary" icon={<SaveOutlined />} loading={savingAreaManager} onClick={onSaveAreaManager}>
                {editingAreaManagerID ? '保存区域账号' : '新增区域账号'}
              </Button>
            </Space>
          )}
          width={920}
          styles={{ body: { maxHeight: 'calc(100vh - 190px)', overflowY: 'auto', paddingRight: 6 } }}
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
              <Text type="secondary">转发入口 Client</Text>
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
                <Text type="secondary">转发入口授权</Text>
                <Button size="small" disabled={!areaManagerForm.grant_agent_id || !areaManagerRealmGrantOptions.length} onClick={() => onUpdateAreaManagerRealmGrantTargets(areaManagerRealmGrantOptions.map((option) => option.value))}>全选</Button>
              </Space>
              <Select
                mode="multiple"
                style={{ width: '100%' }}
                showSearch
                placeholder="选择 Realm / HAProxy 入口"
                value={selectedAreaManagerRealmKeys}
                disabled={!areaManagerForm.grant_agent_id}
                options={areaManagerRealmGrantOptions.map((option) => ({ value: option.value, label: option.label }))}
                optionFilterProp="label"
                maxTagCount="responsive"
                onChange={(values) => onUpdateAreaManagerRealmGrantTargets(values)}
              />
            </Col>
            <Col xs={24} md={12}>
              <Space style={{ width: '100%', justifyContent: 'space-between' }}>
                <Text type="secondary">x-ui 节点 / 客户端授权</Text>
                <Button
                  size="small"
                  disabled={!areaManagerXUIGrantAgentID || !(areaManagerGrantTreeData?.length)}
                  onClick={() => onUpdateAreaManagerGrantTargets(assignmentNodeKeys(areaManagerXUIGrantAgentID, areaManagerOverview, agents))}
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
                value={selectedAreaManagerXUIKeys}
                loading={areaManagerOverviewLoading}
                disabled={!areaManagerXUIGrantAgentID}
                treeData={areaManagerGrantTreeData}
                treeDefaultExpandAll
                showCheckedStrategy={TreeSelect.SHOW_PARENT}
                maxTagCount="responsive"
                onChange={(values) => onUpdateAreaManagerGrantTargets(values as string[])}
              />
            </Col>
            <Col xs={24}>
              <Text type="secondary">节点落地权限</Text>
              <div className="customer-admin-switch-row">
                <Switch
                  checked={areaManagerForm.outbound_create_enabled}
                  onChange={(checked) => setAreaManagerForm((current) => ({ ...current, outbound_create_enabled: checked }))}
                />
                <Text>允许添加已授权 Client 的节点作为落地</Text>
              </div>
            </Col>
            <Col xs={24} md={12}>
              <Text type="secondary">当前 Client</Text>
              <Select
                style={{ width: '100%' }}
                showSearch
                allowClear
                placeholder="选择已授权的 Client"
                options={areaManagerOutboundAgentOptions}
                value={areaManagerForm.outbound_grant_agent_id || undefined}
                optionFilterProp="label"
                onChange={(value) => setAreaManagerForm((current) => ({ ...current, outbound_grant_agent_id: value || '' }))}
              />
            </Col>
            <Col xs={24} md={12}>
              <Space style={{ width: '100%', justifyContent: 'space-between' }}>
                <Text type="secondary">当前 Client 已有出站规则</Text>
                <Button
                  size="small"
                  disabled={!areaManagerForm.outbound_grant_agent_id || !areaManagerOutboundGrantOptions.length}
                  onClick={() => onUpdateAreaManagerOutboundGrantTargets(areaManagerOutboundGrantOptions.map((option) => option.value))}
                >
                  全选
                </Button>
              </Space>
              <Select
                mode="multiple"
                style={{ width: '100%' }}
                showSearch
                placeholder="选择允许区域账号使用的出站规则"
                value={selectedAreaManagerOutboundTags}
                options={areaManagerOutboundGrantOptions}
                optionFilterProp="label"
                loading={areaManagerOutboundOverviewLoading}
                disabled={!areaManagerForm.outbound_grant_agent_id}
                maxTagCount="responsive"
                onChange={onUpdateAreaManagerOutboundGrantTargets}
              />
            </Col>
            <Col xs={24}>
              <Text type="secondary">已授权范围</Text>
              <div style={{ marginTop: 6 }}>
                {areaManagerForm.assignments.length ? renderAssignmentHierarchy(areaManagerForm.assignments, agents, onRemoveAreaManagerGrant) : <Tag>未选择节点</Tag>}
              </div>
            </Col>
            <Col xs={24}>
              <Text type="secondary">已授权出站</Text>
              <div style={{ marginTop: 6 }}>
                {areaManagerForm.outbound_grants.length ? (
                  <Space wrap size={[6, 6]}>
                    {areaManagerForm.outbound_grants.map((grant) => (
                      <Tag
                        key={`${grant.agent_id}::${grant.outbound_tag}`}
                        color="cyan"
                        closable
                        onClose={() => onRemoveAreaManagerOutboundGrant(grant.agent_id, grant.outbound_tag)}
                      >
                        {(agents.find((agent) => agent.agent_id === grant.agent_id)?.agent_name || grant.agent_id)} / {grant.outbound_tag}
                      </Tag>
                    ))}
                  </Space>
                ) : <Tag>未授权出站</Tag>}
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
            <Button type="primary" icon={<SaveOutlined />} loading={savingCustomer} onClick={onCreateCustomer}>
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
            <Button type="primary" icon={<SaveOutlined />} loading={savingCustomer} disabled={!selectedCustomerID} onClick={onSaveCustomer}>
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
        title={`授权链路 · ${selectedCustomerTitle}`}
        open={assignmentViewModalOpen}
        onCancel={() => setAssignmentViewModalOpen(false)}
        footer={<Button onClick={() => setAssignmentViewModalOpen(false)}>关闭</Button>}
        width={980}
        destroyOnClose
      >
        <Table
          rowKey={(record) => record.id}
          columns={readOnlyAssignmentColumns}
          dataSource={selectedCustomerAssignments}
          pagination={{ pageSize: 8, hideOnSinglePage: true }}
          locale={{ emptyText: <Empty description="暂无授权链路" /> }}
        />
      </Modal>
      <Modal
        title={`管理授权链路 · ${selectedCustomerTitle}`}
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
}
