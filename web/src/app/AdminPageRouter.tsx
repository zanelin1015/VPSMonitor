import { Spin } from 'antd'
import { lazy, Suspense, type ComponentProps, type ReactNode } from 'react'
import { AdminAccessLogsPage } from '../components/AdminAccessLogsPage'
import { FrontendSettingsPanel, ScheduledTasksPanel } from '../components/AdminModals'
import { OverviewSummaryCard } from '../components/DashboardOverviewSummary'
const AdminWorkbenchDashboard = lazy(() =>
  import('../components/DashboardWorkbench').then((module) => ({ default: module.AdminWorkbenchDashboard })),
)
import type { AdminPageKey } from '../lib/adminRoute'
const Customers = lazy(() =>
  import('../components/CustomerManagementModal').then((module) => ({ default: module.CustomerManagementModal })),
)
const FrontProxies = lazy(() =>
  import('../components/FrontProxyManagementPage').then((module) => ({ default: module.FrontProxyManagementPage })),
)
const Recommendations = lazy(() =>
  import('../pages/admin/RecommendationsPage').then((module) => ({ default: module.RecommendationsPage })),
)
const Support = lazy(() =>
  import('../components/AdminSupportPage').then((module) => ({ default: module.AdminSupportPage })),
)

export interface AdminPageRouterProps {
  page: AdminPageKey
  topology: boolean
  customers: ComponentProps<typeof Customers>
  canManageSystem: boolean
  onSupportUnread: (count: number) => void
  accessLogs: ComponentProps<typeof AdminAccessLogsPage>
  settings: ComponentProps<typeof FrontendSettingsPanel>
  schedules: ComponentProps<typeof ScheduledTasksPanel>
  finance: ComponentProps<typeof OverviewSummaryCard>
  workbench: ComponentProps<typeof AdminWorkbenchDashboard>
  workspace: ReactNode
}
export function AdminPageRouter(props: AdminPageRouterProps) {
  let content: ReactNode
  switch (props.page) {
    case 'customers':
      content = <Customers {...props.customers} />
      break
    case 'front-proxies':
      content = <FrontProxies canManageNodes={props.canManageSystem} />
      break
    case 'recommendations':
      content = <Recommendations />
      break
    case 'support':
      content = <Support onUnreadCountChange={props.onSupportUnread} />
      break
    case 'access-logs':
      content = <AdminAccessLogsPage {...props.accessLogs} />
      break
    case 'settings':
      content = <FrontendSettingsPanel {...props.settings} />
      break
    case 'schedules':
      content = <ScheduledTasksPanel {...props.schedules} />
      break
    case 'finance':
      content = <OverviewSummaryCard {...props.finance} />
      break
    case 'dashboard':
      content = props.topology ? props.workspace : <AdminWorkbenchDashboard {...props.workbench} />
      break
    default:
      content = props.workspace
  }
  return (
    <main className="admin-pro-route">
      <Suspense
        fallback={
          <Spin tip="正在加载页面">
            <div style={{ minHeight: 240 }} />
          </Spin>
        }
      >
        {content}
      </Suspense>
    </main>
  )
}
