import type { Dispatch, ReactNode, SetStateAction } from 'react'
import { Button, Card, Checkbox, Col, Empty, Input, InputNumber, Modal, Row, Select, Space, Switch, Table, Tag, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { SaveOutlined } from '@ant-design/icons'

import type { CustomerAdminView, CustomerAssignment } from '../types'
import { REVENUE_CURRENCIES } from '../lib/currency'
import { customerEntryURL } from '../lib/appBasePath'
import type { AreaManagerFormState, AssignmentFormState, CustomerFormState } from './CustomerManagementHelpers'
import {
  emptyAssignmentForm,
  emptyCustomerForm,
  isRealmAssignmentDraft,
  renderAssignmentHierarchy,
} from './CustomerManagementHelpers'

import { AreaFrontLinkCreate } from './AreaFrontLinkCreate'
import { AreaMachineGrantFields, AreaPermissionField, AreaPermissionSection } from './AreaPermissionFields'

const { Text } = Typography

export interface CustomerManagementModalsProps {
  canManageAreaManagers: boolean
  agents: Parameters<typeof renderAssignmentHierarchy>[1]
  agentOptions: Array<{ value: string; label: string }>
  realmAgentOptions: Array<{ value: string; label: string }>
  lineAgentOptions: Array<{ value: string; label: string }>
  landingAgentOptions: Array<{ value: string; label: string }>
  onFrontLinksChanged: () => Promise<void>
  frontProxyOptions: Array<{ value: number; label: string }>
  editingAreaManagerID: number | null
  areaManagerModalOpen: boolean
  areaManagerForm: AreaManagerFormState
  setAreaManagerForm: Dispatch<SetStateAction<AreaManagerFormState>>
  savingAreaManager: boolean
  onCloseAreaManagerModal: () => void
  onSaveAreaManager: () => void
  areaManagerRealmGrantOptions: Array<{ value: string; label: string }>
  selectedAreaManagerRealmKeys: string[]
  onUpdateAreaManagerRealmGrantTargets: (values: string[]) => void
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

  customerSubscriptionModalOpen: boolean
  subscriptionCustomer: CustomerAdminView | null
  selectedSubscriptionAssignmentIDs: number[]
  setSelectedSubscriptionAssignmentIDs: Dispatch<SetStateAction<number[]>>
  copyingSubscriptionCustomerID: number | null
  onCloseCustomerSubscriptionModal: () => void
  onCopyCustomerSubscription: (customerID: number, assignmentIDs: number[]) => void

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
    realmAgentOptions, lineAgentOptions, landingAgentOptions, onFrontLinksChanged,
    frontProxyOptions,
    editingAreaManagerID,
    areaManagerModalOpen,
    areaManagerForm,
    setAreaManagerForm,
    savingAreaManager,
    onCloseAreaManagerModal,
    onSaveAreaManager,
    areaManagerRealmGrantOptions,
    selectedAreaManagerRealmKeys,
    onUpdateAreaManagerRealmGrantTargets,
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
    customerSubscriptionModalOpen,
    subscriptionCustomer,
    selectedSubscriptionAssignmentIDs,
    setSelectedSubscriptionAssignmentIDs,
    copyingSubscriptionCustomerID,
    onCloseCustomerSubscriptionModal,
    onCopyCustomerSubscription,
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
          className="area-manager-modal"
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
          styles={{ body: { maxHeight: 'calc(100vh - 190px)', overflowY: 'auto' } }}
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
              <Text type="secondary">推荐页 URL 标识</Text>
              <Input
                value={areaManagerForm.recommendation_token}
                placeholder={editingAreaManagerID ? '留空保持当前标识' : '留空自动生成'}
                addonBefore="?area="
                onChange={(event) => setAreaManagerForm((current) => ({ ...current, recommendation_token: event.target.value }))}
              />
              <div className="muted-line">仅支持 6–96 位字母、数字、短横线、下划线或点；修改后旧链接立即失效。</div>
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
                        { value: 'semiannual', label: '半年' },
                        { value: 'year', label: '年' },
                      ]}
                      onChange={(value) => setAreaManagerForm((current) => ({ ...current, revenue_cycle: value as 'month' | 'quarter' | 'semiannual' | 'year' }))}
                    />
                  </Col>
                  <Col xs={24} md={4}>
                    <Text type="secondary">说明</Text>
                    <div className="muted-line">admin 财务 = 单用户节点收入 + 区域账号收入 - VPS 总花销</div>
                  </Col>
                </Row>
              </Card>
            </Col>
            <Col xs={24}>
              <Card size="small" title="权限类别">
                <Space wrap size="large">
                  {([{ key: 'realm_enabled', label: 'Realm / HAProxy 前置入口' }, { key: 'line_enabled', label: '线路机' }, { key: 'landing_enabled', label: '落地机' }, { key: 'front_link_enabled', label: '前置链接' }] as const).map(({ key, label }) => <Space key={key}>
                    <Switch aria-label={label + '权限'} checked={areaManagerForm[key]} onChange={checked => setAreaManagerForm(current => {
                      const next = { ...current, [key]: checked }
                      if (!checked) {
                        if (key === 'realm_enabled') { next.grant_agent_id = ''; next.assignments = next.assignments.filter(item => !isRealmAssignmentDraft(item, agents)) }
                        if (key === 'front_link_enabled') next.front_proxy_node_ids = []
                        if (key === 'line_enabled') next.outbound_grants = []
                        if (key === 'line_enabled' || key === 'landing_enabled') {
                          const keep = new Set([...(next.line_enabled ? lineAgentOptions : []), ...(next.landing_enabled ? landingAgentOptions : [])].map(item => item.value))
                          next.assignments = next.assignments.filter(item => isRealmAssignmentDraft(item, agents) || keep.has(item.agent_id))
                        }
                      }
                      return next
                    })} /><Text>{label}</Text>
                  </Space>)}
                </Space>
              </Card>
            </Col>
            {areaManagerForm.realm_enabled ? (
              <AreaPermissionSection title="Realm / HAProxy 前置入口">
                <AreaPermissionField label="前置入口 Client">
                  <Select
                    style={{ width: '100%' }}
                    showSearch
                    placeholder="选择正在运行 Realm / HAProxy 的 Client"
                    disabled={!areaManagerForm.realm_enabled}
                    options={areaManagerForm.realm_enabled ? realmAgentOptions : []}
                    value={areaManagerForm.grant_agent_id || undefined}
                    optionFilterProp="label"
                    onChange={(value) => setAreaManagerForm((current) => ({ ...current, grant_agent_id: value }))}
                  />
                </AreaPermissionField>
                <AreaPermissionField label="转发入口授权" action={
                  <Button size="small" disabled={!areaManagerForm.grant_agent_id || !areaManagerRealmGrantOptions.length} onClick={() => onUpdateAreaManagerRealmGrantTargets(areaManagerRealmGrantOptions.map(option => option.value))}>全选</Button>
                }>
                  <Select
                    mode="multiple"
                    style={{ width: '100%' }}
                    showSearch
                    placeholder="选择 Realm / HAProxy 入口"
                    value={selectedAreaManagerRealmKeys}
                    disabled={!areaManagerForm.realm_enabled || !areaManagerForm.grant_agent_id}
                    options={areaManagerRealmGrantOptions.map((option) => ({ value: option.value, label: option.label }))}
                    optionFilterProp="label"
                    maxTagCount="responsive"
                    onChange={(values) => onUpdateAreaManagerRealmGrantTargets(values)}
                  />
                </AreaPermissionField>
              </AreaPermissionSection>
            ) : null}
            {areaManagerForm.line_enabled ? (
              <AreaPermissionSection title="线路机">
                <AreaMachineGrantFields kind="line" options={lineAgentOptions} form={areaManagerForm} setForm={setAreaManagerForm} agents={agents} />
              </AreaPermissionSection>
            ) : null}
            {areaManagerForm.landing_enabled ? (
              <AreaPermissionSection title="落地机">
                <AreaMachineGrantFields kind="landing" options={landingAgentOptions} form={areaManagerForm} setForm={setAreaManagerForm} agents={agents} />
                <Col xs={24}><Text type="secondary">开启落地机权限后，允许添加已授权 Client 的节点作为落地。</Text></Col>
              </AreaPermissionSection>
            ) : null}
            {areaManagerForm.front_link_enabled ? (
              <AreaPermissionSection title="前置链接">
                <AreaPermissionField label="前置链接授权" action={
                  <AreaFrontLinkCreate agents={agents} onCreated={async id => { await onFrontLinksChanged(); setAreaManagerForm(current => ({ ...current, front_proxy_node_ids: [...current.front_proxy_node_ids, id] })) }} />
                }>
                  <Select
                    mode="multiple"
                    style={{ width: '100%' }}
                    placeholder="选择已启用的本地或第三方前置链接"
                    disabled={!areaManagerForm.front_link_enabled}
                    value={areaManagerForm.front_proxy_node_ids}
                    options={areaManagerForm.front_link_enabled ? frontProxyOptions : []}
                    optionFilterProp="label"
                    maxTagCount="responsive"
                    onChange={(values) => setAreaManagerForm((current) => ({ ...current, front_proxy_node_ids: values }))}
                  />
                </AreaPermissionField>
              </AreaPermissionSection>
            ) : null}
            {(areaManagerForm.realm_enabled || areaManagerForm.line_enabled || areaManagerForm.landing_enabled) ? (
              <Col xs={24}>
                <Text type="secondary">已授权范围</Text>
                <div style={{ marginTop: 6 }}>
                  {areaManagerForm.assignments.length ? renderAssignmentHierarchy(areaManagerForm.assignments, agents, onRemoveAreaManagerGrant) : <Tag>未选择节点</Tag>}
                </div>
              </Col>
            ) : null}
            {(areaManagerForm.line_enabled) ? (
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
            ) : null}
          </Row>
        </Modal>
      ) : null}
      <Modal
        className="customer-account-modal"
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
            <Text type="secondary">第三方前置代理授权</Text>
            <Select
              mode="multiple"
              style={{ width: '100%' }}
              placeholder="选择允许该用户链路使用的前置代理"
              value={customerCreateForm.front_proxy_node_ids}
              options={frontProxyOptions}
              optionFilterProp="label"
              maxTagCount="responsive"
              onChange={(values) => setCustomerCreateForm((current) => ({ ...current, front_proxy_node_ids: values }))}
            />
          </Col>
          <Col xs={24}>
            <Text type="secondary">用户入口地址</Text>
            <Input value={customerEntryURL()} readOnly />
          </Col>
        </Row>
      </Modal>
      <Modal
        title={`导出 Clash 订阅 · ${subscriptionCustomer?.display_name || subscriptionCustomer?.username || ''}`}
        open={customerSubscriptionModalOpen}
        onCancel={onCloseCustomerSubscriptionModal}
        footer={(
          <Space>
            <Button onClick={onCloseCustomerSubscriptionModal}>取消</Button>
            <Button
              type="primary"
              disabled={!subscriptionCustomer || selectedSubscriptionAssignmentIDs.length === 0}
              loading={subscriptionCustomer ? copyingSubscriptionCustomerID === subscriptionCustomer.id : false}
              onClick={() => {
                if (subscriptionCustomer && selectedSubscriptionAssignmentIDs.length > 0) {
                  onCopyCustomerSubscription(subscriptionCustomer.id, selectedSubscriptionAssignmentIDs)
                }
              }}
            >
              复制 Clash 订阅
            </Button>
          </Space>
        )}
        width={760}
        destroyOnClose
      >
        {subscriptionCustomer ? (
          <Space direction="vertical" size="middle" style={{ width: '100%' }}>
            <div>
              <Text strong>选择导出的节点</Text>
              <div className="muted-line">默认全部不选；只会导出下方勾选且已启用的授权链路。</div>
            </div>
            <Space wrap>
              <Button
                size="small"
                disabled={!subscriptionCustomer.assignments?.some((assignment) => assignment.enabled)}
                onClick={() => setSelectedSubscriptionAssignmentIDs((subscriptionCustomer.assignments || []).filter((assignment) => assignment.enabled).map((assignment) => assignment.id))}
              >
                全选
              </Button>
              <Button size="small" onClick={() => setSelectedSubscriptionAssignmentIDs([])}>清空</Button>
              <Text type="secondary">已选择 {selectedSubscriptionAssignmentIDs.length} / {(subscriptionCustomer.assignments || []).filter((assignment) => assignment.enabled).length} 条</Text>
            </Space>
            <Checkbox.Group
              style={{ width: '100%' }}
              value={selectedSubscriptionAssignmentIDs}
              onChange={(values) => setSelectedSubscriptionAssignmentIDs(values.map((value) => Number(value)))}
            >
              <Space direction="vertical" size={10} style={{ width: '100%' }}>
                {(subscriptionCustomer.assignments || []).map((assignment) => (
                  <Checkbox key={assignment.id} value={assignment.id} disabled={!assignment.enabled}>
                    <div>
                      <Text strong>{assignment.public_client_name || `链路 #${assignment.id}`}</Text>
                      <div className="muted-line">
                        {[assignment.inbound_tag, assignment.client_email].filter(Boolean).join(' / ') || '未指定客户端'}{assignment.enabled ? '' : '（已停用）'}
                      </div>
                    </div>
                  </Checkbox>
                ))}
              </Space>
            </Checkbox.Group>
            {!(subscriptionCustomer.assignments || []).length ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无授权链路" /> : null}
          </Space>
        ) : <Empty description="请选择一个用户" />}
      </Modal>
      <Modal
        className="customer-account-modal"
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
            <Text type="secondary">第三方前置代理授权</Text>
            <Select
              mode="multiple"
              style={{ width: '100%' }}
              placeholder="选择允许该用户链路使用的前置代理"
              value={customerForm.front_proxy_node_ids}
              options={frontProxyOptions}
              optionFilterProp="label"
              maxTagCount="responsive"
              onChange={(values) => setCustomerForm((current) => ({ ...current, front_proxy_node_ids: values }))}
            />
          </Col>
          <Col xs={24}>
            <Text type="secondary">用户入口地址</Text>
            <Input value={customerEntryURL()} readOnly />
          </Col>
        </Row>
      </Modal>
      <Modal
        title={`授权链路 · ${selectedCustomerTitle}`}
        open={assignmentViewModalOpen}
        onCancel={() => setAssignmentViewModalOpen(false)}
        footer={<Button onClick={() => setAssignmentViewModalOpen(false)}>关闭</Button>}
        width={1260}
        destroyOnClose
      >
        <Table
          rowKey={(record) => record.id}
          columns={readOnlyAssignmentColumns}
          dataSource={selectedCustomerAssignments}
          pagination={{ pageSize: 8, hideOnSinglePage: true }}
          scroll={{ x: readOnlyAssignmentColumns.reduce((width, column) => width + Number(column.width || 120), 0) }}
          locale={{ emptyText: <Empty description="暂无授权链路" /> }}
        />
      </Modal>
      <Modal
        className="customer-assignment-modal"
        title={`管理授权链路 · ${selectedCustomerTitle}`}
        open={assignmentManagerModalOpen}
        onCancel={() => {
          setAssignmentManagerModalOpen(false)
          setEditingAssignmentID(null)
          setAssignmentForm(emptyAssignmentForm)
        }}
        footer={null}
        width={1260}
        destroyOnClose
      >
        {assignmentManagerContent}
      </Modal>
    </>
  )
}
