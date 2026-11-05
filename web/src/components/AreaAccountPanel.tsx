import type { AreaManagerAdminView, DashboardAgentView, XUIOverview } from '../types'
import type { AreaBatchAssignmentFormState } from './CustomerManagementHelpers'
import { Button, Card, Col, Empty, Modal, Row, Select, Space, Table, TreeSelect, Typography } from 'antd';
import { PlusOutlined, ReloadOutlined, SaveOutlined } from '@ant-design/icons';
import { AdminHelpHint } from './AdminHelpHint';
import { assignmentTreeKeys, buildAreaAssignmentTreeData, normalizeAreaAssignmentTreeKeys } from './CustomerManagementHelpers';
import type { Dispatch, ReactNode, SetStateAction } from 'react'
import type { ColumnsType } from 'antd/es/table'

const { Text, Title } = Typography
type AgentOptions = Array<{ value: string; label: string }>
interface AreaAccountPanelProps {
  canManageAreaManagers: boolean
  loadAreaManagers: () => Promise<void>
  openAreaBatchModal: () => void
  openAreaManagerCreateModal: () => void
  areaBatchModalOpen: boolean
  closeAreaBatchModal: () => void
  savingAreaBatchAssignment: boolean
  batchAssignAreaManagerTargets: () => Promise<void>
  areaBatchForm: AreaBatchAssignmentFormState
  areaManagers: AreaManagerAdminView[]
  setAreaBatchForm: Dispatch<SetStateAction<AreaBatchAssignmentFormState>>
  realmAgentOptions: AgentOptions
  agentOptions: AgentOptions
  lineAgentOptions: AgentOptions
  landingAgentOptions: AgentOptions
  areaBatchRealmOptions: Array<{ value: string; label: string }>
  areaBatchXUIAgentID: string
  areaBatchClientTreeData: ReturnType<typeof buildAreaAssignmentTreeData>
  areaBatchOverview: XUIOverview | null
  agents: DashboardAgentView[]
  areaBatchOverviewLoading: boolean
  areaManagerColumns: ColumnsType<AreaManagerAdminView>
  renderAreaManagerCustomers: (record: AreaManagerAdminView) => ReactNode
}

export function AreaAccountPanel({
  canManageAreaManagers,
  loadAreaManagers,
  openAreaBatchModal,
  openAreaManagerCreateModal,
  areaBatchModalOpen,
  closeAreaBatchModal,
  savingAreaBatchAssignment,
  batchAssignAreaManagerTargets,
  areaBatchForm,
  areaManagers,
  setAreaBatchForm,
  realmAgentOptions,
  agentOptions,
  lineAgentOptions,
  landingAgentOptions,
  areaBatchRealmOptions,
  areaBatchXUIAgentID,
  areaBatchClientTreeData,
  areaBatchOverview,
  agents,
  areaBatchOverviewLoading,
  areaManagerColumns,
  renderAreaManagerCustomers,
}: AreaAccountPanelProps) {
  return canManageAreaManagers ? (
    <Card className="customer-admin-card" bordered={false}>
      <div className="customer-admin-card-head">
        <div className="admin-help-title-group">
          <Title level={5}>区域管理账号</Title>
          <AdminHelpHint title={(
            <>
              <strong>区域账号权限</strong>
              <br />
              区域账号只能查看被分配的 Client，能下发 x-ui 转发规则，并且只能管理自己创建的普通用户；Admin 可见全部用户与区域账号。展开区域账号可直接查看其下属用户与链路。
            </>
          )} />
        </div>
        <Space>
          <Button size="small" icon={<ReloadOutlined />} onClick={() => void loadAreaManagers()}>刷新区域账号</Button>
          <Button size="small" icon={<PlusOutlined />} onClick={openAreaBatchModal}>新增授权</Button>
          <Button size="small" type="primary" icon={<PlusOutlined />} onClick={openAreaManagerCreateModal}>新增区域账号</Button>
        </Space>
      </div>
      <Modal
        title={(
          <div className="admin-help-title-group">
            <span>新增授权 · 批量授权入口 / 出口</span>
            <AdminHelpHint title="Realm / HAProxy 入口与 x-ui 出口节点可分别选择；HAProxy 主备会自动映射到校验一致的最终节点。" />
          </div>
        )}
        open={areaBatchModalOpen}
        onCancel={closeAreaBatchModal}
        maskClosable={!savingAreaBatchAssignment}
        keyboard={!savingAreaBatchAssignment}
        closable={!savingAreaBatchAssignment}
        width={1000}
        styles={{ body: { maxHeight: 'calc(100vh - 220px)', overflowY: 'auto' } }}
        destroyOnClose
        footer={(
          <Space>
            <Button disabled={savingAreaBatchAssignment} onClick={closeAreaBatchModal}>取消</Button>
            <Button type="primary" icon={<SaveOutlined />} loading={savingAreaBatchAssignment} onClick={() => void batchAssignAreaManagerTargets()}>
              保存授权
            </Button>
          </Space>
        )}
      >
        <Row gutter={[12, 12]}>
          <Col xs={24}>
            <Text type="secondary">区域账号</Text>
            <Select
              style={{ width: '100%' }}
              showSearch
              placeholder="选择区域账号"
              value={areaBatchForm.manager_id ?? undefined}
              options={areaManagers.map((item) => ({ value: item.id, label: `${item.username} · ${item.display_name || item.username}` }))}
              optionFilterProp="label"
              onChange={(value) => setAreaBatchForm((current) => ({ ...current, manager_id: value, agent_id: '', xui_agent_id: '', selected_realm_keys: [], selected_xui_keys: [] }))}
            />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">转发入口 Client</Text>
            <Select
              style={{ width: '100%' }}
              showSearch
              placeholder="选择 GZ 入口"
              value={areaBatchForm.agent_id || undefined}
              options={areaManagers.find(item => item.id === areaBatchForm.manager_id)?.realm_enabled === false ? [] : realmAgentOptions}
              disabled={!areaBatchForm.manager_id || areaManagers.find(item => item.id === areaBatchForm.manager_id)?.realm_enabled === false}
              optionFilterProp="label"
              onChange={(value) => setAreaBatchForm((current) => ({ ...current, agent_id: value, selected_realm_keys: [] }))}
            />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">x-ui 出口 Client</Text>
            <Select
              style={{ width: '100%' }}
              showSearch
              placeholder="选择 HK 出口"
              value={areaBatchForm.xui_agent_id || undefined}
              options={agentOptions.filter(option => {
                const manager = areaManagers.find(item => item.id === areaBatchForm.manager_id)
                return manager && (manager.line_enabled !== false && lineAgentOptions.some(item => item.value === option.value) || manager.landing_enabled !== false && landingAgentOptions.some(item => item.value === option.value))
              })}
              disabled={!areaBatchForm.manager_id}
              optionFilterProp="label"
              onChange={(value) => setAreaBatchForm((current) => ({ ...current, xui_agent_id: value, selected_xui_keys: [] }))}
            />
          </Col>
          <Col xs={24} md={12}>
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
          <Col xs={24} md={12}>
            <Space style={{ width: '100%', justifyContent: 'space-between' }}>
              <Text type="secondary">x-ui 客户端 / 节点</Text>
              <Button size="small" disabled={!areaBatchXUIAgentID || !areaBatchClientTreeData.length} onClick={() => setAreaBatchForm((current) => ({ ...current, selected_xui_keys: assignmentTreeKeys(areaBatchXUIAgentID, areaBatchOverview, agents) }))}>全选</Button>
            </Space>
            <TreeSelect
              multiple
              treeCheckable
              treeCheckStrictly
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
              onChange={(values) => setAreaBatchForm((current) => ({ ...current, selected_xui_keys: normalizeAreaAssignmentTreeKeys(areaBatchXUIAgentID, values as string[], areaBatchOverview, agents) }))}
            />
          </Col>
        </Row>
      </Modal>
      <Table
        style={{ marginTop: 14 }}
        rowKey={(record) => record.id}
        columns={areaManagerColumns}
        dataSource={areaManagers}
        scroll={{ x: 1200, y: '100%' }}
        pagination={{ pageSize: 5, hideOnSinglePage: true }}
        expandable={{
          expandedRowRender: renderAreaManagerCustomers,
          rowExpandable: (record) => Boolean(record.customers?.length || record.assignments?.length),
        }}
        locale={{ emptyText: <Empty description="暂无区域账号" /> }}
      />
    </Card>
  ) : null

}
