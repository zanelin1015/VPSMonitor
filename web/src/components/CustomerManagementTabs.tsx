import type { ReactNode } from 'react'
import { Spin, Tabs, Typography } from 'antd'
import type { OperatorAdminView } from '../types'
import type { ManagementTabKey } from './CustomerManagementHelpers'
import { AdminHelpHint } from './AdminHelpHint'
import { AgentRolePanel } from './AgentRolePanel'
import { OperatorManagementPanel } from './OperatorManagementPanel'

const { Text } = Typography

interface CustomerManagementTabsProps {
  activeKey: ManagementTabKey
  onChange: (key: ManagementTabKey) => void
  canManageAreaManagers: boolean
  showRolePanel: boolean
  areaManagersPanel: ReactNode
  customersPanel: ReactNode
  operators: OperatorAdminView[]
  operatorsLoading: boolean
  savingOperator: boolean
  resettingOperatorID: number | null
  onConfigChanged?: () => void | Promise<void>
  onReloadOperators: () => void
  onSaveOperator: Parameters<typeof OperatorManagementPanel>[0]['onSave']
  onDeleteOperator: (id: number) => void
  onResetOperatorPassword: (id: number) => void
  loading: boolean
  areaManagersLoading: boolean
}

export function CustomerManagementTabs({
  activeKey,
  onChange,
  canManageAreaManagers,
  showRolePanel,
  areaManagersPanel,
  customersPanel,
  operators,
  operatorsLoading,
  savingOperator,
  resettingOperatorID,
  onConfigChanged,
  onReloadOperators,
  onSaveOperator,
  onDeleteOperator,
  onResetOperatorPassword,
  loading,
  areaManagersLoading,
}: CustomerManagementTabsProps) {
  const managementTabs = [
    ...(showRolePanel ? [{ key: 'roles', label: 'Client 角色', children: <AgentRolePanel onChanged={() => onConfigChanged?.()} /> }] : []),
    ...(canManageAreaManagers ? [{ key: 'area', label: '区域账号', children: areaManagersPanel }] : []),
    ...(canManageAreaManagers ? [{
      key: 'operators',
      label: '运营管理员',
      children: (
        <OperatorManagementPanel
          operators={operators}
          loading={operatorsLoading}
          saving={savingOperator}
          resettingID={resettingOperatorID}
          onReload={onReloadOperators}
          onSave={onSaveOperator}
          onDelete={onDeleteOperator}
          onResetPassword={onResetOperatorPassword}
        />
      ),
    }] : []),
    { key: 'customers', label: '用户账号', children: customersPanel },
  ]

  return (
    <Spin spinning={loading || areaManagersLoading || operatorsLoading}>
      <div className="customer-management-body">
        <Tabs
          className="customer-admin-tabs"
          tabBarExtraContent={(
            <span className="admin-help-title-group customer-admin-tab-help">
              <Text type="secondary" className="customer-admin-help-label">授权说明</Text>
              <AdminHelpHint title={(
                <>
                  <strong>授权使用规则与停用机制</strong>
                  <br />
                  建议仅给已授权用户开放链路，可用于独享或多人共享场景；禁止滥发、攻击、诈骗、爬虫滥用、扫描爆破等行为。出现异常时，可先停用用户账号或单条授权链路；如需彻底切断连接，再到对应 Client 的 x-ui 客户端列表中删除或停用该 client。
                </>
              )} />
            </span>
          )}
          activeKey={activeKey}
          items={managementTabs}
          onChange={(key) => onChange(key as ManagementTabKey)}
        />
      </div>
    </Spin>
  )
}
