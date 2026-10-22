import { useEffect, useState } from 'react'
import type { useAdminFinance } from '../../hooks/admin/useAdminFinance'
import type { useAdminNavigationState } from '../../hooks/admin/useAdminNavigationState'
import type { useAgentDetailData } from '../../hooks/admin/useAgentDetailData'
import type { useDashboardData } from '../../hooks/admin/useDashboardData'
import type { useAdminSession } from '../../hooks/useAdminSession'
import type { useAdminSystemTools } from '../../hooks/useAdminSystemTools'
import { DASHBOARD_AUTO_REFRESH_MS, fetchJSON } from '../../lib/appHelpers'
import { isOperatorAdminUser } from '../../lib/adminUsers'
import type { SupportUnreadResponse } from '../../types'

type Options = Pick<ReturnType<typeof useAdminSession>, 'loadSession' | 'adminUser' | 'logoutSession'> &
  Pick<
    ReturnType<typeof useDashboardData>,
    'loadAgents' | 'agents' | 'setDashboardView' | 'setAgents' | 'dashboardLoaded' | 'setDashboardLoaded'
  > &
  Pick<
    ReturnType<typeof useAdminSystemTools>,
    | 'loadTelegramBots'
    | 'updateModalOpen'
    | 'loadUpdateLatestInfo'
    | 'loadScheduledTasks'
    | 'openFrontendSettingsModal'
    | 'loadAccessLogs'
    | 'setTelegramBots'
  > &
  Pick<
    ReturnType<typeof useAgentDetailData>,
    | 'loadTagSettings'
    | 'setOverview'
    | 'setManagedConfig'
    | 'setAgentLogs'
    | 'setConfigAudits'
    | 'loadOverview'
    | 'loadXUIActions'
    | 'loadAgentLogs'
  > &
  Pick<
    ReturnType<typeof useAdminFinance>,
    | 'loadExchangeRates'
    | 'loadFinanceAccounts'
    | 'setFinanceCustomers'
    | 'setFinanceAreaManagers'
    | 'setFinanceAccountsLoaded'
    | 'setFinanceAccountsError'
  > &
  Pick<
    ReturnType<typeof useAdminNavigationState>,
    | 'activeAdminPage'
    | 'setActiveAdminPage'
    | 'activeTabKey'
    | 'setActiveTabKey'
    | 'selectedAgentId'
    | 'setSelectedAgentId'
    | 'setSelectedTag'
    | 'setAgentHealthFilter'
    | 'setAgentListSearch'
  > & { canManageSystem: boolean }

/** Session bootstrapping, screen-specific loads and polling cleanup. */
export function useAdminLifecycle(options: Options) {
  const {
    dashboardLoaded,
    setDashboardLoaded,
    loadSession,
    adminUser,
    loadAgents,
    loadTelegramBots,
    loadTagSettings,
    loadExchangeRates,
    loadFinanceAccounts,
    updateModalOpen,
    loadUpdateLatestInfo,
    activeAdminPage,
    setActiveAdminPage,
    activeTabKey,
    setActiveTabKey,
    loadScheduledTasks,
    openFrontendSettingsModal,
    loadAccessLogs,
    agents,
    selectedAgentId,
    setSelectedAgentId,
    logoutSession,
    setDashboardView,
    setSelectedTag,
    setAgentHealthFilter,
    setAgentListSearch,
    setAgents,
    setFinanceCustomers,
    setFinanceAreaManagers,
    setFinanceAccountsLoaded,
    setFinanceAccountsError,
    setOverview,
    setManagedConfig,
    setTelegramBots,
    setAgentLogs,
    setConfigAudits,
    loadOverview,
    loadXUIActions,
    loadAgentLogs,
    canManageSystem,
  } = options
  const [supportUnreadCount, setSupportUnreadCount] = useState(0)

  useEffect(() => {
    void loadSession()
  }, [])

  useEffect(() => {
    if (adminUser) {
      void loadAgents()
      if (canManageSystem) {
        void loadTelegramBots()
        void loadTagSettings()
        void loadExchangeRates()
        void loadFinanceAccounts()
      }
    }
  }, [adminUser, canManageSystem])

  useEffect(() => {
    if (!adminUser) {
      setSupportUnreadCount(0)
      return
    }
    let cancelled = false
    const loadUnread = async () => {
      try {
        const data = await fetchJSON<SupportUnreadResponse>('/api/v1/admin/support/unread')
        if (!cancelled) setSupportUnreadCount(data.unread_count || 0)
      } catch {
        // The session loader handles authentication failures; the badge can stay stale briefly.
      }
    }
    let timer: number | undefined
    const poll = async () => {
      await loadUnread()
      if (!cancelled) timer = window.setTimeout(() => void poll(), 3000)
    }
    void poll()
    return () => {
      cancelled = true
      if (timer !== undefined) window.clearTimeout(timer)
    }
  }, [adminUser?.id, adminUser?.role, adminUser?.username])

  useEffect(() => {
    if (adminUser && updateModalOpen) {
      void loadUpdateLatestInfo()
    }
  }, [adminUser, updateModalOpen])

  useEffect(() => {
    if (adminUser && canManageSystem) {
      void loadUpdateLatestInfo({ silent: true })
    }
  }, [adminUser, canManageSystem])

  useEffect(() => {
    if (!adminUser || canManageSystem) {
      return
    }
    if (isOperatorAdminUser(adminUser)) {
      if (activeAdminPage !== 'customers' && activeAdminPage !== 'support') {
        setActiveAdminPage('customers')
      }
      if (['config', 'logs', 'certificates'].includes(activeTabKey)) {
        setActiveTabKey('overview')
      }
      return
    }
    if (
      activeAdminPage === 'settings' ||
      activeAdminPage === 'schedules' ||
      activeAdminPage === 'access-logs' ||
      activeAdminPage === 'finance'
    ) {
      setActiveAdminPage('dashboard')
    }
    if (['config', 'logs', 'certificates'].includes(activeTabKey)) {
      setActiveTabKey('overview')
    }
  }, [activeAdminPage, activeTabKey, adminUser, canManageSystem])

  useEffect(() => {
    if (adminUser && canManageSystem && activeAdminPage === 'schedules') {
      void loadScheduledTasks()
    }
  }, [activeAdminPage, adminUser, canManageSystem])

  useEffect(() => {
    if (adminUser && canManageSystem && activeAdminPage === 'settings') {
      void openFrontendSettingsModal(false)
    }
  }, [activeAdminPage, adminUser, canManageSystem])

  useEffect(() => {
    if (adminUser && canManageSystem && activeAdminPage === 'access-logs') {
      void loadAccessLogs()
    }
  }, [activeAdminPage, adminUser, canManageSystem])

  useEffect(() => {
    if (dashboardLoaded && selectedAgentId && !agents.some((item) => item.agent_id === selectedAgentId)) {
      setSelectedAgentId('')
    }
  }, [agents, selectedAgentId, dashboardLoaded])

  async function logout() {
    await logoutSession()
    setDashboardView(null)
    setDashboardLoaded(false)
    setSelectedTag('')
    setAgentHealthFilter('all')
    setAgentListSearch('')
    setAgents([])
    setFinanceCustomers([])
    setFinanceAreaManagers([])
    setFinanceAccountsLoaded(false)
    setFinanceAccountsError('')
    setSelectedAgentId('')
    setOverview(null)
    setManagedConfig(null)
    setTelegramBots([])
    setAgentLogs(null)
    setConfigAudits([])
  }

  useEffect(() => {
    if (!adminUser) {
      return
    }
    const timer = window.setInterval(() => {
      void loadAgents({ silent: true })
      if (selectedAgentId) {
        void loadOverview(selectedAgentId, { silent: true })
      }
      if (selectedAgentId && activeTabKey === 'actions') {
        void loadXUIActions(selectedAgentId, { silent: true })
      }
      if (selectedAgentId && activeTabKey === 'logs') {
        void loadAgentLogs(selectedAgentId, { silent: true })
      }
    }, DASHBOARD_AUTO_REFRESH_MS)
    return () => {
      window.clearInterval(timer)
    }
  }, [activeTabKey, adminUser, selectedAgentId])
  return { supportUnreadCount, setSupportUnreadCount, logout }
}
