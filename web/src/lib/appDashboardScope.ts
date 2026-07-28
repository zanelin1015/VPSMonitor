import type {
  AreaManagerAdminView,
  CustomerAdminView,
  DashboardAgentView,
  GlobalDashboardView,
  XUIOverview,
} from '../types'
import type { CurrencyCode, ExchangeRatesState } from './currency'
import { scopeAreaManagersToAgents, summarizeMonthlyFinance } from './currency'
import {
  chainMatchesSelectedTag,
  hasSelectedTag,
  isAgentRunning,
  mergeDashboardTagOptions,
  topologyMatchesSelectedTag,
} from './appHelpers'
import { summarizeAgentNetwork } from './traffic'

type DashboardScopeInput = {
  agents: DashboardAgentView[]
  dashboardView: GlobalDashboardView | null
  selectedTag: string
  tagOptions: string[]
  currentOverview: XUIOverview | null
  deferredClientSearch: string
  canManageSystem: boolean
  financeAccountsLoaded: boolean
  financeAccountsError: string
  financeCustomers: CustomerAdminView[]
  financeAreaManagers: AreaManagerAdminView[]
  costCurrency: CurrencyCode
  exchangeRates: ExchangeRatesState
}

export function buildDashboardScope(input: DashboardScopeInput) {
  const {
    agents,
    dashboardView,
    selectedTag,
    tagOptions,
    currentOverview,
    deferredClientSearch,
    canManageSystem,
    financeAccountsLoaded,
    financeAccountsError,
    financeCustomers,
    financeAreaManagers,
    costCurrency,
    exchangeRates,
  } = input

  const filteredClients = currentOverview
    ? currentOverview.clients.filter((client) => {
        if (!deferredClientSearch) {
          return true
        }
        const haystack = [client.email, client.comment, client.inbound_tag, client.inbound_remark, client.inbound_id]
          .filter(Boolean)
          .join(' ')
          .toLowerCase()
        return haystack.includes(deferredClientSearch)
      })
    : []

  const filteredAgents = agents.filter((item) => hasSelectedTag(item.tags, selectedTag))
  const tagFilterOptions = mergeDashboardTagOptions(dashboardView?.tags || [], tagOptions)
  const selectedTagView = selectedTag ? dashboardView?.tags.find((tag) => tag.tag === selectedTag) : undefined
  const scopedAgentCount = dashboardView ? selectedTagView?.agent_count ?? filteredAgents.length : filteredAgents.length
  const scopedNodeCount = dashboardView ? selectedTagView?.node_count ?? dashboardView.totals.node_count : 0
  const onlineAgentCount = filteredAgents.filter(isAgentRunning).length
  const offlineAgentCount = Math.max(scopedAgentCount - onlineAgentCount, 0)
  const xuiErrorAgentCount = filteredAgents.filter((agent) => Boolean(agent.summary.last_collection_err)).length
  const scopedNetwork = summarizeAgentNetwork(filteredAgents)
  const filteredTagLinks = (dashboardView?.links || []).filter((link) => topologyMatchesSelectedTag(link, selectedTag))
  const filteredChains = (dashboardView?.client_chains || []).filter((chain) => chainMatchesSelectedTag(chain, selectedTag))
  const scopedFinanceAreaManagers = scopeAreaManagersToAgents(financeAreaManagers, filteredAgents, Boolean(selectedTag))
  const monthlyFinance = canManageSystem && financeAccountsLoaded
    ? {
        ...summarizeMonthlyFinance(filteredAgents, filteredChains, costCurrency, exchangeRates, financeCustomers, scopedFinanceAreaManagers),
        error: financeAccountsError || undefined,
      }
    : {
        available: false,
        error: financeAccountsError,
        costTotal: 0,
        costCount: 0,
        revenueTotal: 0,
        revenueCount: 0,
        profitTotal: 0,
        missingCostCount: 0,
        missingRevenueCount: 0,
        excludedRevenueCount: 0,
      }
  const workbenchNetwork = summarizeAgentNetwork(agents)
  const workbenchMonthlyFinance = canManageSystem && financeAccountsLoaded
    ? {
        ...summarizeMonthlyFinance(agents, dashboardView?.client_chains || [], costCurrency, exchangeRates, financeCustomers, financeAreaManagers),
        error: financeAccountsError || undefined,
      }
    : monthlyFinance

  return {
    filteredAgents,
    filteredChains,
    filteredClients,
    filteredTagLinks,
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
  }
}
