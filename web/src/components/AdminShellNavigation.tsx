import { Button, Select, Typography } from 'antd'
import {
  ApartmentOutlined,
  CloudServerOutlined,
  DashboardOutlined,
  DeploymentUnitOutlined,
  FileSearchOutlined,
  ReloadOutlined,
  SettingOutlined,
  TeamOutlined,
} from '@ant-design/icons'

import type { AdminUser, SystemInfo } from '../types'
import { PersonalCenterDropdown } from './AdminModals'
import type { AdminPageKey } from '../lib/adminRoute'
import type { ThemeMode } from '../theme'

const { Paragraph, Text, Title } = Typography

export interface AdminShellNavigationProps {
  adminUser: AdminUser
  systemInfo: SystemInfo | null
  canManageSystem: boolean
  isAreaManagerAccount: boolean
  activeAdminPage: AdminPageKey
  topologyVisible: boolean
  onlineAgentCount: number
  scopedAgentCount: number
  agentsLoading: boolean
  themeMode: ThemeMode
  effectiveMode: ThemeMode
  heroTitle: string
  serverVersionLabel: string
  onOpenAccount: () => void
  onOpenClientInstall: () => void
  onOpenTelegram: () => void
  onOpenCustomers: () => void
  onOpenFrontendSettings: () => void
  onOpenUpdates: () => void
  onLogout: () => void
  onOpenWorkbench: () => void
  onOpenAssets: () => void
  onOpenTopology: () => void
  onOpenAccessLogs: () => void
  onOpenSchedules: () => void
  onRefreshAgents: () => void
  onThemeModeChange: (value: ThemeMode) => void
}

export function AdminShellNavigation(props: AdminShellNavigationProps) {
  const {
    adminUser,
    systemInfo,
    canManageSystem,
    isAreaManagerAccount,
    activeAdminPage,
    topologyVisible,
    onlineAgentCount,
    scopedAgentCount,
    agentsLoading,
    themeMode,
    serverVersionLabel,
    onOpenAccount,
    onOpenClientInstall,
    onOpenTelegram,
    onOpenCustomers,
    onOpenFrontendSettings,
    onOpenUpdates,
    onLogout,
    onOpenWorkbench,
    onOpenAssets,
    onOpenTopology,
    onOpenAccessLogs,
    onOpenSchedules,
    onRefreshAgents,
    onThemeModeChange,
  } = props

  return (
    <>
      <header className="admin-mobile-header">
        <div className="admin-mobile-brand-row">
          <div className="admin-mobile-brand">
            <span className="admin-oa-brand-mark">南</span>
            <div>
              <strong>南风VPS监控</strong>
              <small>
                在线 {onlineAgentCount}/{scopedAgentCount} · v{systemInfo?.version || '-'}
              </small>
            </div>
          </div>
          <PersonalCenterDropdown
            adminUser={adminUser}
            systemInfo={systemInfo}
            canManageSystem={canManageSystem}
            onOpenAccount={onOpenAccount}
            onOpenClientInstall={onOpenClientInstall}
            onOpenTelegram={onOpenTelegram}
            onOpenCustomers={onOpenCustomers}
            onOpenFrontendSettings={onOpenFrontendSettings}
            onOpenUpdates={onOpenUpdates}
            onLogout={onLogout}
          />
        </div>
        <nav className="admin-mobile-nav" aria-label="移动端管理导航">
          <button type="button" className={activeAdminPage === 'dashboard' && !topologyVisible ? 'active' : ''} onClick={onOpenWorkbench}>
            <DashboardOutlined />
            <span>工作台</span>
          </button>
          <button type="button" className={activeAdminPage === 'assets' ? 'active' : ''} onClick={onOpenAssets}>
            <CloudServerOutlined />
            <span>资产</span>
          </button>
          <button type="button" className={activeAdminPage === 'dashboard' && topologyVisible ? 'active' : ''} onClick={onOpenTopology}>
            <ApartmentOutlined />
            <span>拓扑</span>
          </button>
          <button type="button" className={activeAdminPage === 'customers' ? 'active' : ''} onClick={onOpenCustomers}>
            <TeamOutlined />
            <span>用户</span>
          </button>
          {canManageSystem ? <button type="button" className={activeAdminPage === 'access-logs' ? 'active' : ''} onClick={onOpenAccessLogs}>
            <FileSearchOutlined />
            <span>日志</span>
          </button> : null}
          {canManageSystem ? <button type="button" className={activeAdminPage === 'settings' ? 'active' : ''} onClick={onOpenFrontendSettings}>
            <SettingOutlined />
            <span>设置</span>
          </button> : null}
          {canManageSystem ? <button type="button" className={activeAdminPage === 'schedules' ? 'active' : ''} onClick={onOpenSchedules}>
            <ReloadOutlined />
            <span>定时</span>
          </button> : null}
        </nav>
        <div className="admin-mobile-actions">
          <Button size="small" icon={<ReloadOutlined />} loading={agentsLoading} onClick={onRefreshAgents}>刷新</Button>
          {canManageSystem ? <Button size="small" icon={<DeploymentUnitOutlined />} onClick={onOpenClientInstall}>安装 Client</Button> : null}
          <Select
            size="small"
            value={themeMode}
            options={[
              { value: 'system', label: '跟随系统' },
              { value: 'light', label: '明亮' },
              { value: 'dark', label: '暗黑' },
            ]}
            onChange={(value) => onThemeModeChange(value as ThemeMode)}
          />
        </div>
      </header>
      <aside className="admin-oa-sider">
        <div className="admin-oa-brand">
          <span className="admin-oa-brand-mark">南</span>
          <div>
            <strong>南风VPS监控</strong>
            <small>{serverVersionLabel}</small>
          </div>
        </div>
        <nav className="admin-oa-nav" aria-label="管理端导航">
          <button type="button" className={`admin-oa-nav-item${activeAdminPage === 'dashboard' && !topologyVisible ? ' active' : ''}`} onClick={onOpenWorkbench}>
            <DashboardOutlined />
            <span>工作台</span>
            <small>{isAreaManagerAccount ? '授权总览' : '总览与财务'}</small>
          </button>
          <button type="button" className={`admin-oa-nav-item${activeAdminPage === 'assets' ? ' active' : ''}`} onClick={onOpenAssets}>
            <CloudServerOutlined />
            <span>Client 资产</span>
            <small>节点列表</small>
          </button>
          <button type="button" className={`admin-oa-nav-item${activeAdminPage === 'dashboard' && topologyVisible ? ' active' : ''}`} onClick={onOpenTopology}>
            <ApartmentOutlined />
            <span>拓扑图</span>
            <small>链路联动</small>
          </button>
          <button type="button" className={`admin-oa-nav-item${activeAdminPage === 'customers' ? ' active' : ''}`} onClick={onOpenCustomers}>
            <TeamOutlined />
            <span>用户管理</span>
            <small>账号与授权</small>
          </button>
          {canManageSystem ? <button type="button" className={`admin-oa-nav-item${activeAdminPage === 'access-logs' ? ' active' : ''}`} onClick={onOpenAccessLogs}>
            <FileSearchOutlined />
            <span>访问日志</span>
            <small>连接排查</small>
          </button> : null}
          {canManageSystem ? <button type="button" className={`admin-oa-nav-item${activeAdminPage === 'settings' ? ' active' : ''}`} onClick={onOpenFrontendSettings}>
            <SettingOutlined />
            <span>系统设置</span>
            <small>样式与升级</small>
          </button> : null}
          {canManageSystem ? <button type="button" className={`admin-oa-nav-item${activeAdminPage === 'schedules' ? ' active' : ''}`} onClick={onOpenSchedules}>
            <ReloadOutlined />
            <span>定时任务</span>
            <small>时间与频率</small>
          </button> : null}
        </nav>
        <div className="admin-oa-sider-foot">
          <span>在线 Client</span>
          <strong>{onlineAgentCount}/{scopedAgentCount}</strong>
          <small className="admin-oa-sider-version-row">
            <span>Server v{systemInfo?.version || '-'}</span>
          </small>
        </div>
      </aside>

    </>
  )
}

export function AdminShellTopbar(props: AdminShellNavigationProps) {
  const {
    adminUser,
    systemInfo,
    canManageSystem,
    isAreaManagerAccount,
    agentsLoading,
    themeMode,
    effectiveMode,
    heroTitle,
    serverVersionLabel,
    onOpenAccount,
    onOpenClientInstall,
    onOpenTelegram,
    onOpenCustomers,
    onOpenFrontendSettings,
    onOpenUpdates,
    onLogout,
    onRefreshAgents,
    onThemeModeChange,
  } = props

  return (
    <header className="hero-panel admin-oa-topbar">
      <div className="admin-oa-titlebar">
        <div className="eyebrow">{serverVersionLabel} / 工作台</div>
        <Title level={1}>{heroTitle}</Title>
        <Paragraph className="hero-copy">
          {isAreaManagerAccount
            ? '管理已授权 Client、用户账号、区域标签与可见拓扑链路。'
            : '统一管理 Client、x-ui 托管配置、用户账号、财务月览与跨 Client 拓扑联动。'}
        </Paragraph>
      </div>
      <div className="hero-actions hero-actions-column">
        <Button icon={<ReloadOutlined />} loading={agentsLoading} onClick={onRefreshAgents}>刷新</Button>
        {canManageSystem ? <Button icon={<DeploymentUnitOutlined />} onClick={onOpenClientInstall}>安装 Client</Button> : null}
        <Button icon={<TeamOutlined />} onClick={onOpenCustomers}>用户</Button>
        <PersonalCenterDropdown
          adminUser={adminUser}
          systemInfo={systemInfo}
          canManageSystem={canManageSystem}
          onOpenAccount={onOpenAccount}
          onOpenClientInstall={onOpenClientInstall}
          onOpenTelegram={onOpenTelegram}
          onOpenCustomers={onOpenCustomers}
          onOpenFrontendSettings={onOpenFrontendSettings}
          onOpenUpdates={onOpenUpdates}
          onLogout={onLogout}
        />
        <div className="theme-mode-row">
          <Text type="secondary">主题</Text>
          <Select
            size="small"
            value={themeMode}
            options={[
              { value: 'system', label: `跟随系统（${effectiveMode === 'dark' ? '暗黑' : '明亮'}）` },
              { value: 'light', label: '明亮' },
              { value: 'dark', label: '暗黑' },
            ]}
            onChange={(value) => onThemeModeChange(value as ThemeMode)}
          />
        </div>
      </div>
    </header>
  )
}
