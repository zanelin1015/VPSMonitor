import { Select } from 'antd'
import type { AdminUser, SystemInfo } from '../types'
import type { AdminPageKey } from '../lib/adminRoute'
import type { ThemeMode } from '../theme'
import { PersonalCenterDropdown } from './AdminModals'

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
  themeMode: ThemeMode
  effectiveMode: ThemeMode
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
  onOpenFinance: () => void
  onOpenTopology: () => void
  onOpenAccessLogs: () => void
  onOpenSchedules: () => void
  onRefreshAgents: () => void
  onThemeModeChange: (value: ThemeMode) => void
}

/** Shared account and appearance controls for desktop and mobile headers. */
export function AdminHeaderActions(props: AdminShellNavigationProps) {
  return [
    <Select
      key="theme"
      aria-label="界面主题"
      size="small"
      value={props.themeMode}
      style={{ width: 116 }}
      options={[
        { value: 'system', label: '跟随系统' },
        { value: 'light', label: '明亮' },
        { value: 'dark', label: '暗黑' },
      ]}
      onChange={props.onThemeModeChange}
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
