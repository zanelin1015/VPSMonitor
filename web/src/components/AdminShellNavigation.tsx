import type { AdminUser, SystemInfo } from '../types'
import type { AdminPageKey } from '../lib/adminRoute'
import { PersonalCenterDropdown } from './AdminModals'
import { CustomerThemeButton } from './CustomerThemeButton'

export interface AdminShellNavigationProps {
  adminUser: AdminUser
  systemInfo: SystemInfo | null
  canManageSystem: boolean
  isAreaManagerAccount: boolean
  isOperatorAccount: boolean
  canViewInfrastructure: boolean
  activeAdminPage: AdminPageKey
  topologyVisible: boolean
  onlineAgentCount: number
  scopedAgentCount: number
  supportUnreadCount: number
  agentsLoading: boolean
  serverVersionLabel: string
  onOpenAccount: () => void
  onOpenClientInstall: () => void
  onOpenTelegram: () => void
  onOpenCustomers: () => void
  onOpenFrontProxies: () => void
  onOpenSupport: () => void
  onOpenFrontendSettings: () => void
  onOpenAnnouncements: () => void
  onOpenUpdates: () => void
  onLogout: () => void
  onOpenWorkbench: () => void
  onOpenAssets: () => void
  onOpenRecommendations: () => void
  onOpenFinance: () => void
  onOpenTopology: () => void
  onOpenAccessLogs: () => void
  onOpenSchedules: () => void
  onRefreshAgents: () => void
}

/** Shared account and appearance controls for desktop and mobile headers. */
export function AdminHeaderActions(props: AdminShellNavigationProps) {
  return [
    <CustomerThemeButton
      key="theme"
    />,
    <PersonalCenterDropdown
      key="account"
      adminUser={props.adminUser}
      systemInfo={props.systemInfo}
      canManageSystem={props.canManageSystem}
      onOpenAccount={props.onOpenAccount}
      onOpenClientInstall={props.onOpenClientInstall}
      onOpenTelegram={props.onOpenTelegram}
      onOpenCustomers={props.onOpenCustomers}
      onOpenFrontendSettings={props.onOpenFrontendSettings}
      onOpenUpdates={props.onOpenUpdates}
      onLogout={props.onLogout}
    />,
  ]
}
