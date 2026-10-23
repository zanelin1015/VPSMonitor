// @refresh reset
import type { ComponentProps } from 'react'
import { AdminLayout } from '../app/AdminLayout'
import { AdminModalHost } from '../app/AdminModalHost'
import { AdminPageRouter } from '../app/AdminPageRouter'
import { buildAgentDetailProps } from '../hooks/admin/buildAgentDetailProps'
import { buildConsoleModalProps } from '../hooks/admin/buildConsoleModalProps'
import { useAdminFinance } from '../hooks/admin/useAdminFinance'
import { useAdminLifecycle } from '../hooks/admin/useAdminLifecycle'
import { useAdminNavigationState } from '../hooks/admin/useAdminNavigationState'
import { useAgentDetailData } from '../hooks/admin/useAgentDetailData'
import { useAgentOperations } from '../hooks/admin/useAgentOperations'
import { useDashboardData } from '../hooks/admin/useDashboardData'
import { useXUIActionComposer } from '../hooks/admin/useXUIActionComposer'
import { AgentDetailDrawer } from '../pages/admin/AgentDetailDrawer'
import { TopologyPage } from '../pages/admin/TopologyPage'
// Plain .ts custom hooks are not instrumented by Fast Refresh. Remount this
// development entry on updates instead of reusing an incompatible hook queue.
import { App as AntdApp, Spin } from 'antd'
import { lazy, startTransition } from 'react'

import { useAdminRouteSync } from '../hooks/useAdminRouteSync'
import { useAdminSession } from '../hooks/useAdminSession'
import { useAdminSystemTools } from '../hooks/useAdminSystemTools'
import { useXUIRemoteActions } from '../hooks/useXUIRemoteActions'
import { isAreaManagerAdminUser, isOperatorAdminUser, isRootAdminUser } from '../lib/adminUsers'
import { agentMatchesHealthFilter } from '../lib/agentHealth'
import { agentMatchesSearch, matchingAgentClients } from '../lib/agentSearch'
import { buildDashboardScope } from '../lib/appDashboardScope'
import { createAppNavigationHandlers } from '../lib/appNavigation'
import { normalizeScheduledTaskSettings } from '../lib/scheduledTasks'

import { AdminAccessLogsPage } from '../components/AdminAccessLogsPage'
import { FrontendSettingsPanel, ScheduledTasksPanel } from '../components/AdminModals'
import type { AdminWorkbenchDashboard } from '../components/DashboardWorkbench'
import { AgentRail } from '../components/DashboardAgentRail'
import { OverviewSummaryCard } from '../components/DashboardOverviewSummary'
import { renderCNFlowPanel } from '../components/DashboardTopologyPanels'
import { LoginScreen } from '../components/LoginScreen'
import { VisualEffects } from '../components/VisualEffects'

const AssetsPage = lazy(() => import('../pages/admin/AssetsPage').then((module) => ({ default: module.AssetsPage })))
const CustomerManagementModal = lazy(() =>
  import('../components/CustomerManagementModal').then((module) => ({ default: module.CustomerManagementModal })),
)
const CustomerBatchAuthorizationModal = lazy(() =>
  import('../components/CustomerBatchAuthorizationModal').then((module) => ({
    default: module.CustomerBatchAuthorizationModal,
  })),
)

export function AdminApp() {
  const { message } = AntdApp.useApp()
  const session = useAdminSession()
  const {
    accountModalOpen,
    adminUser,
    loginForm,
    loginLoading,
    sessionLoading,
    systemInfo,
    login,
    setAccountForm,
    setAccountModalOpen,
    setAdminUser,
    setLoginForm,
  } = session

  const isAreaManagerAccount = isAreaManagerAdminUser(adminUser)
  const isOperatorAccount = isOperatorAdminUser(adminUser)
  const canManageSystem = isRootAdminUser(adminUser)
  const canViewInfrastructure = canManageSystem || isAreaManagerAccount

  const navigationState = useAdminNavigationState({ adminUser })
  const {
    selectedTag,
    setSelectedTag,
    agentHealthFilter,
    setAgentHealthFilter,
    topologySearch,
    setTopologySearch,
    activeAdminPage,
    setActiveAdminPage,
    agentViewMode,
    selectedAgentId,
    setSelectedAgentId,
    customerModalOpen,
    customerAssignmentDraft,
    setCustomerAssignmentDraft,
    customerAuthorizationDraft,
    setCustomerAuthorizationDraft,
    setReloadToken,
    setActiveTabKey,
    topologyVisible,
    setTopologyVisible,
    setSelectedOutboundTag,
    setSelectedRuleIndex,
    setSelectedNodeAnchor,
    setClientSearch,
    agentListSearch,
    setAgentListSearch,
    deferredClientSearch,
    deferredAgentListSearch,
    pendingClientSearchRef,
    toggleAgentViewMode,
  } = navigationState

  const finance = useAdminFinance({ adminUser, setAdminUser })
  const { financeCustomers, costCurrency, setCostCurrency, exchangeRates, currencyOptions, loadFinanceAccounts } =
    finance

  const dashboard = useDashboardData({
    ...session,
    ...navigationState,
    message,
    isAreaManagerAccount,
    setOverview: (value) => setOverview(value),
  })
  const {
    agents,
    agentsLoading,
    agentsError,
    dashboardView,
    topologyLoading,
    topologyError,
    topologyLoaded,
    loadAgents,
    loadTopology,
  } = dashboard

  const selectedAgent = agents.find((item) => item.agent_id === selectedAgentId)

  const details = useAgentDetailData({
    ...session,
    ...navigationState,
    ...dashboard,
    selectedAgent,
    message,
    canManageSystem,
  })
  const { overview, setOverview, overviewLoading, managedConfig, configLoading, loadManagedConfig } = details

  const operations = useAgentOperations({
    ...session,
    ...navigationState,
    ...dashboard,
    ...details,
    message,
    returnHome: () => returnHome(),
  })
  const { agentRefreshLoading, requestAgentSnapshot } = operations

  const currentOverview = overview?.agent_id === selectedAgentId ? overview : null

  const composer = useXUIActionComposer({ ...session, ...navigationState, ...details, currentOverview, message })
  const { xuiActionModalOpen, importURLClient } = composer
  const systemTools = useAdminSystemTools(setAdminUser)
  const {
    accessLogFilters,
    accessLogs,
    accessLogsLoading,
    accessLogsTotal,
    clientInstallModalOpen,
    frontendSettingsForm,
    frontendSettingsLoading,
    frontendSettingsModalOpen,
    frontendSettingsSaving,
    scheduledTasks,
    scheduledTasksLoading,
    scheduledTasksSaving,
    telegramBotModalOpen,
    updateModalOpen,
    loadAccessLogs,
    openClientInstallModal,
    openFrontendSettingsModal,
    saveFrontendSettings,
    saveScheduledTasks,
    setAccessLogFilters,
    setScheduledTasks,
    setTelegramBotModalOpen,
    setUpdateModalOpen,
  } = systemTools
  const remote = useXUIRemoteActions({ ...session, ...navigationState, ...dashboard, ...details, ...composer })
  const { xuiRestartLoading, xuiUpdateLoading, restartXUIService, update3XUI } = remote

  const centerPanelOpen = topologyVisible || Boolean(selectedAgent)
  const navigation = createAppNavigationHandlers({ ...navigationState, ...dashboard, runTransition: startTransition })
  const {
    jumpToNode,
    openAgentDetailPanel,
    openAgentHealthFilter,
    openCustomerAssignment,
    openFinancePage,
    openTopologyPanel,
    returnHome,
    selectDashboardTag,
    selectTopologyAgent,
  } = navigation
  useAdminRouteSync({
    ...navigationState,
    ...navigation,
    enabled: Boolean(!sessionLoading && adminUser),
    sessionIdentity: adminUser,
    canManageSystem,
    canViewFrontProxies: canManageSystem || isAreaManagerAccount,
    canViewInfrastructure,
  })

  async function refreshAfterExternalConfigChange(agentID?: string) {
    await loadAgents()
    if (canManageSystem) {
      await loadFinanceAccounts()
    }
    if (agentID && agentID === selectedAgentId) {
      await loadManagedConfig(agentID, { silent: true })
    }
  }

  const scope = buildDashboardScope({
    ...dashboard,
    ...finance,
    selectedTag,
    currentOverview,
    deferredClientSearch,
    canManageSystem,
  })
  const { supportUnreadCount, setSupportUnreadCount, logout } = useAdminLifecycle({
    ...session,
    ...dashboard,
    ...systemTools,
    ...details,
    ...finance,
    ...navigationState,
    canManageSystem,
  })

  const {
    filteredAgents,
    filteredChains,
    monthlyFinance,
    offlineAgentCount,
    onlineAgentCount,
    scopedAgentCount,
    scopedFinanceAreaManagers,
    scopedNetwork,
    scopedNodeCount,
    tagFilterOptions,
    workbenchMonthlyFinance,
    workbenchNetwork,
    xuiErrorAgentCount,
  } = scope
  const agentListAgents = filteredAgents.filter(
    (agent) => agentMatchesHealthFilter(agent, agentHealthFilter) && agentMatchesSearch(agent, deferredAgentListSearch),
  )

  if (sessionLoading) {
    return (
      <>
        <VisualEffects />
        <div className="login-shell">
          <Spin size="large" />
        </div>
      </>
    )
  }

  if (!adminUser) {
    return (
      <>
        <VisualEffects />
        <LoginScreen
          consoleAppearance
          loginForm={loginForm}
          loginLoading={loginLoading}
          onChange={setLoginForm}
          onLogin={login}
        />
      </>
    )
  }

  const consoleModalOpen =
    accountModalOpen ||
    clientInstallModalOpen ||
    customerModalOpen ||
    frontendSettingsModalOpen ||
    Boolean(importURLClient) ||
    telegramBotModalOpen ||
    updateModalOpen ||
    xuiActionModalOpen
  const serverVersionLabel = `V${systemInfo?.version || '-'}`
  const openAccountModal = () => {
    setAccountForm({
      current_password: '',
      new_username: adminUser.username,
      new_password: '',
      confirm_password: '',
      avatar_url: adminUser.avatar_url || '',
    })
    setAccountModalOpen(true)
  }
  const openAssetsPage = () => {
    setActiveAdminPage('assets')
    setAgentHealthFilter('all')
    setTopologyVisible(false)
    setSelectedTag('')
    setClientSearch('')
    setSelectedAgentId('')
    setActiveTabKey('overview')
    setSelectedOutboundTag('')
    setSelectedRuleIndex(null)
    setSelectedNodeAnchor('')
  }
  const openCustomersPage = () => setActiveAdminPage('customers')
  const openFrontProxiesPage = () => {
    setActiveAdminPage('front-proxies')
    setTopologyVisible(false)
  }
  const openSupportPage = () => setActiveAdminPage('support')
  const openSettingsPage = () => {
    setActiveAdminPage('settings')
  }
  const openAccessLogsPage = () => setActiveAdminPage('access-logs')
  const openSchedulesPage = () => setActiveAdminPage('schedules')
  const shellNavigationProps = {
    adminUser,
    systemInfo,
    canManageSystem,
    isAreaManagerAccount,
    isOperatorAccount,
    canViewInfrastructure,
    activeAdminPage,
    topologyVisible,
    onlineAgentCount,
    scopedAgentCount,
    supportUnreadCount,
    agentsLoading,
    serverVersionLabel,
    onOpenAccount: openAccountModal,
    onOpenClientInstall: () => void openClientInstallModal(),
    onOpenTelegram: () => setTelegramBotModalOpen(true),
    onOpenCustomers: openCustomersPage,
    onOpenFrontProxies: openFrontProxiesPage,
    onOpenSupport: openSupportPage,
    onOpenFrontendSettings: () => void openFrontendSettingsModal(true),
    onOpenAnnouncements: openSettingsPage,
    onOpenUpdates: () => setUpdateModalOpen(true),
    onLogout: () => void logout(),
    onOpenWorkbench: returnHome,
    onOpenAssets: openAssetsPage,
    onOpenFinance: openFinancePage,
    onOpenTopology: openTopologyPanel,
    onOpenAccessLogs: openAccessLogsPage,
    onOpenSchedules: openSchedulesPage,
    onRefreshAgents: () => void loadAgents(),
  }

  const consoleModalProps = buildConsoleModalProps({
    ...session,
    ...dashboard,
    ...systemTools,
    ...navigationState,
    ...composer,
    ...details,
    ...navigation,
    refreshAfterExternalConfigChange,
  })

  const agentDetailProps = selectedAgent
    ? buildAgentDetailProps({
        ...navigationState,
        ...details,
        ...finance,
        ...operations,
        ...remote,
        ...dashboard,
        ...scope,
        ...composer,
        ...navigation,
        ...session,
        canManageSystem,
        isAreaManagerAccount,
        currentOverview,
        selectedAgent,
        openAssetsPage,
      })
    : null

  const authorizationProps: ComponentProps<typeof CustomerBatchAuthorizationModal> = {
    draft: customerAuthorizationDraft!,
    onClose: () => setCustomerAuthorizationDraft(null),
    onConfigChanged: refreshAfterExternalConfigChange,
  }

  const agentRailProps: ComponentProps<typeof AgentRail> = {
    agents: agentListAgents,
    loading: agentsLoading,
    error: agentsError,
    selectedTag: selectedTag,
    healthFilter: agentHealthFilter,
    selectedAgentId: selectedAgentId,
    tagFilterOptions: tagFilterOptions,
    viewMode: agentViewMode,
    panelExpanded: centerPanelOpen,
    topologyVisible: topologyVisible,
    restrictedView: isAreaManagerAccount,
    onToggleViewMode: toggleAgentViewMode,
    onToggleTopology: () => {
      if (topologyVisible) {
        setTopologyVisible(false)
        setSelectedAgentId('')
        setActiveTabKey('overview')
      } else {
        openTopologyPanel()
      }
    },
    onRefresh: () => void loadAgents(),
    onSelectTag: selectDashboardTag,
    onHealthFilterChange: setAgentHealthFilter,
    searchText: agentListSearch,
    onSearchTextChange: setAgentListSearch,
    onSelectAgent: (agentID, active) => {
      if (topologyVisible) {
        selectTopologyAgent(agentID)
        return
      }
      const searchAgent = agentListAgents.find((agent) => agent.agent_id === agentID)
      const hasClientMatches = searchAgent && matchingAgentClients(searchAgent, agentListSearch).length > 0
      pendingClientSearchRef.current = hasClientMatches && !active ? { agentID, query: agentListSearch.trim() } : null
      if (hasClientMatches) {
        setClientSearch(agentListSearch.trim())
        setActiveTabKey('clients')
      } else if (agentListSearch.trim()) {
        setClientSearch('')
      }
      if (active && !topologyVisible) {
        setReloadToken((current) => current + 1)
        return
      }
      openAgentDetailPanel(agentID, hasClientMatches ? 'clients' : 'overview')
    },
  }

  const summaryProps: ComponentProps<typeof OverviewSummaryCard> = {
    dashboardView: dashboardView,
    scopedAgentCount: scopedAgentCount,
    scopedNodeCount: scopedNodeCount,
    onlineAgentCount: onlineAgentCount,
    offlineAgentCount: offlineAgentCount,
    xuiErrorAgentCount: xuiErrorAgentCount,
    scopedNetwork: scopedNetwork,
    costCurrency: costCurrency,
    currencyOptions: currencyOptions,
    monthlyFinance: monthlyFinance,
    financeAgents: filteredAgents,
    financeChains: filteredChains,
    financeCustomers: financeCustomers,
    financeAreaManagers: scopedFinanceAreaManagers,
    exchangeRates: exchangeRates,
    selectedTag: selectedTag,
    financePage: true,
    restrictedView: isAreaManagerAccount,
    onCostCurrencyChange: setCostCurrency,
  }

  const workspaceSummaryProps: ComponentProps<typeof OverviewSummaryCard> = {
    dashboardView: dashboardView,
    scopedAgentCount: scopedAgentCount,
    scopedNodeCount: scopedNodeCount,
    onlineAgentCount: onlineAgentCount,
    offlineAgentCount: offlineAgentCount,
    xuiErrorAgentCount: xuiErrorAgentCount,
    scopedNetwork: scopedNetwork,
    costCurrency: costCurrency,
    currencyOptions: currencyOptions,
    monthlyFinance: monthlyFinance,
    financeAgents: filteredAgents,
    financeChains: filteredChains,
    financeCustomers: financeCustomers,
    financeAreaManagers: scopedFinanceAreaManagers,
    exchangeRates: exchangeRates,
    selectedTag: selectedTag,
    compact: centerPanelOpen,
    restrictedView: isAreaManagerAccount,
    onCostCurrencyChange: setCostCurrency,
  }

  const workbenchProps: ComponentProps<typeof AdminWorkbenchDashboard> = {
    agents: agents,
    dashboardView: dashboardView,
    scopedNetwork: workbenchNetwork,
    monthlyFinance: workbenchMonthlyFinance,
    costCurrency: costCurrency,
    restrictedView: isAreaManagerAccount,
    onSelectAgent: (agentID) => openAgentDetailPanel(agentID),
    onOpenHealthFilter: openAgentHealthFilter,
    onOpenTopology: openTopologyPanel,
  }

  const customerPageProps: ComponentProps<typeof CustomerManagementModal> = {
    embedded: true,
    agents: agents,
    adminUser: adminUser,
    initialAssignment: customerAssignmentDraft,
    onInitialAssignmentApplied: () => setCustomerAssignmentDraft(null),
    onConfigChanged: refreshAfterExternalConfigChange,
    onOpenAssignment: openCustomerAssignment,
  }

  const accessLogPageProps: ComponentProps<typeof AdminAccessLogsPage> = {
    agents: agents,
    filters: accessLogFilters,
    logs: accessLogs,
    loading: accessLogsLoading,
    total: accessLogsTotal,
    onFiltersChange: setAccessLogFilters,
    onLoad: () => void loadAccessLogs(),
  }

  const settingsPageProps: ComponentProps<typeof FrontendSettingsPanel> = {
    loading: frontendSettingsLoading,
    saving: frontendSettingsSaving,
    form: frontendSettingsForm,
    onSave: saveFrontendSettings,
  }

  const schedulesPageProps: ComponentProps<typeof ScheduledTasksPanel> = {
    loading: scheduledTasksLoading,
    saving: scheduledTasksSaving,
    settings: scheduledTasks,
    onSave: () => void saveScheduledTasks(),
    onChange: (value) => setScheduledTasks(normalizeScheduledTaskSettings(value)),
  }

  const closeAgentDetails = () => {
    setSelectedAgentId('')
    setSelectedOutboundTag('')
    setSelectedRuleIndex(null)
    setSelectedNodeAnchor('')
    setActiveTabKey('overview')
  }
  const topologyPanel: Parameters<typeof renderCNFlowPanel>[0] | null = dashboardView
    ? {
        dashboardView,
        selectedTag,
        selectedAgentId,
        agents: dashboardView.agents,
        chains: filteredChains,
        onSelectAgent: (agentID) => {
          selectTopologyAgent(agentID)
        },
        onJumpNode: jumpToNode,
        restrictedView: isAreaManagerAccount,
        canOpenXUI: !isAreaManagerAccount && Boolean(managedConfig?.xui?.base_url),
        onOpenXUI: () => {
          if (managedConfig?.xui?.base_url) {
            window.open(managedConfig.xui.base_url, '_blank', 'noopener,noreferrer')
          }
        },
        canRefreshCurrentNode: !isAreaManagerAccount && Boolean(selectedAgentId),
        currentNodeLoading: overviewLoading || configLoading || agentRefreshLoading,
        onRefreshCurrentNode: () => {
          if (selectedAgentId) {
            void requestAgentSnapshot(selectedAgentId)
          }
        },
        canRestartXUI: !isAreaManagerAccount && Boolean(selectedAgentId),
        xuiRestartLoading,
        onRestartXUI: () => {
          if (selectedAgentId) {
            void restartXUIService(selectedAgentId)
          }
        },
        canUpdate3XUI: !isAreaManagerAccount && Boolean(selectedAgentId),
        xuiUpdateLoading,
        onUpdate3XUI: () => {
          if (selectedAgentId) {
            void update3XUI(selectedAgentId)
          }
        },
        searchText: topologySearch,
        onSearchTextChange: setTopologySearch,
      }
    : null
  return (
    <AdminLayout
      navigation={shellNavigationProps}
      overlays={
        <>
          <AdminModalHost
            consoleProps={consoleModalOpen ? consoleModalProps : null}
            authorization={customerAuthorizationDraft ? authorizationProps : null}
          />
          <AgentDetailDrawer
            detail={selectedAgent && !topologyVisible && activeAdminPage === 'assets' ? agentDetailProps : null}
            onClose={closeAgentDetails}
          />
        </>
      }
    >
      <AdminPageRouter
        page={activeAdminPage}
        topology={topologyVisible}
        canManageSystem={canManageSystem}
        customers={customerPageProps}
        onSupportUnread={setSupportUnreadCount}
        accessLogs={accessLogPageProps}
        settings={settingsPageProps}
        schedules={schedulesPageProps}
        finance={summaryProps}
        workbench={workbenchProps}
        workspace={
          topologyVisible ? (
            <div className="workspace-grid workspace-grid-topology workspace-grid-topology-open">
              <OverviewSummaryCard {...workspaceSummaryProps} />
              <AgentRail {...agentRailProps} />
              <section className="main-stage">
                {topologyPanel ? (
                  <TopologyPage
                    panel={topologyPanel}
                    loading={topologyLoading}
                    loaded={topologyLoaded}
                    error={topologyError}
                    onRetry={() => void loadTopology()}
                  />
                ) : (
                  <Spin />
                )}
              </section>
            </div>
          ) : (
            <AssetsPage {...agentRailProps} accountKey={String(adminUser.id)} />
          )
        }
      />
    </AdminLayout>
  )
}
