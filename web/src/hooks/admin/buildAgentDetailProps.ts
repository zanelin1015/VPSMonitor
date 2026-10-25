import type { AgentDetailPanelProps } from '../../components/AgentDetailPanel'
import type { buildDashboardScope } from '../../lib/appDashboardScope'
import {
  defaultAddClientActionForm,
  defaultOutboundActionForm,
  defaultRoutingActionForm,
  mergeTagOptions,
  parseAddressInput,
  upsertClientBilling,
} from '../../lib/appHelpers'
import type { createAppNavigationHandlers } from '../../lib/appNavigation'
import type { DashboardAgentView, ManagedAgentConfig, XUINodeView, XUIOverview } from '../../types'
import type { useAdminSession } from '../useAdminSession'
import type { useXUIRemoteActions } from '../useXUIRemoteActions'
import type { useAdminFinance } from './useAdminFinance'
import type { useAdminNavigationState } from './useAdminNavigationState'
import type { useAgentDetailData } from './useAgentDetailData'
import type { useAgentOperations } from './useAgentOperations'
import type { useDashboardData } from './useDashboardData'
import type { useXUIActionComposer } from './useXUIActionComposer'

type Options = Pick<
  ReturnType<typeof useAdminNavigationState>,
  | 'activeTabKey'
  | 'clientSearch'
  | 'selectedAgentId'
  | 'selectedNodeAnchor'
  | 'selectedOutboundTag'
  | 'selectedRuleIndex'
  | 'selectedTag'
  | 'setActiveTabKey'
  | 'setClientSearch'
> &
  Pick<
    ReturnType<typeof useAgentDetailData>,
    | 'agentLogs'
    | 'agentLogsError'
    | 'agentLogsLoading'
    | 'managedConfig'
    | 'configAudits'
    | 'configAuditsLoading'
    | 'configError'
    | 'configLoading'
    | 'configSavingSection'
    | 'overviewLoading'
    | 'entryAddressInputText'
    | 'newTagName'
    | 'overviewError'
    | 'tagSaving'
    | 'xuiActions'
    | 'xuiActionsLoading'
    | 'createTagOption'
    | 'setEntryAddressInputText'
    | 'updateManagedConfig'
    | 'setNewTagName'
    | 'loadAgentLogs'
    | 'loadOverview'
    | 'loadXUIActions'
    | 'savedManagedConfig'
    | 'isCurrentAgentRequest'
    | 'setSavedManagedConfig'
    | 'setManagedConfig'
    | 'loadConfigAudits'
    | 'saveManagedConfigSection'
    | 'savePrimaryDomain'
    | 'saveAreaAgentTags'
  > &
  Pick<ReturnType<typeof useAdminFinance>, 'currencyOptions'> &
  Pick<
    ReturnType<typeof useAgentOperations>,
    | 'agentRefreshLoading'
    | 'realmCopyLoading'
    | 'agentDeleteLoading'
    | 'agentReplaceLoading'
    | 'deleteAgent'
    | 'replaceAgent'
    | 'requestAgentSnapshot'
    | 'copyRealmConfigToAgent'
  > &
  Pick<
    ReturnType<typeof useXUIRemoteActions>,
    | 'xuiRestartLoading'
    | 'remoteCommandLoading'
    | 'xuiClientDeleteLoadingKey'
    | 'routingRuleDeleteLoadingAgentId'
    | 'routingRuleUpdateLoadingAgentId'
    | 'outboundDeleteLoadingAgentId'
    | 'xuiClientToggleLoadingKey'
    | 'xuiClientResetLoadingKey'
    | 'deleteXUIClient'
    | 'deleteRoutingRules'
    | 'updateRoutingRule'
    | 'deleteOutbounds'
    | 'setXUIClientEnabled'
    | 'manualResetXUIClientTraffic'
    | 'restartXUIService'
    | 'executeRemoteCommand'
  > &
  Pick<
    ReturnType<typeof useDashboardData>,
    'dashboardView' | 'tagOptions' | 'agents' | 'loadAgents' | 'setTagOptions'
  > &
  Pick<ReturnType<typeof buildDashboardScope>, 'filteredAgents' | 'filteredClients' | 'filteredTagLinks'> &
  Pick<
    ReturnType<typeof useXUIActionComposer>,
    | 'copyImportURL'
    | 'setXUIActionAgentId'
    | 'setAddClientActionInbounds'
    | 'setXUIActionKind'
    | 'setAddClientActionForm'
    | 'setOutboundActionForm'
    | 'setRoutingActionForm'
    | 'setXUIActionModalOpen'
    | 'setImportURLClient'
    | 'scheduleXUIActionResultRefresh'
  > &
  Pick<
    ReturnType<typeof createAppNavigationHandlers>,
    'jumpToNode' | 'jumpToOutbound' | 'jumpToRule' | 'openCustomerAuthorization' | 'selectDashboardTag'
  > &
  Pick<ReturnType<typeof useAdminSession>, 'setAdminUser'> & {
    canManageSystem: boolean
    isAreaManagerAccount: boolean
    currentOverview: XUIOverview | null
    selectedAgent: DashboardAgentView
    openAssetsPage: () => void
  }

export function buildAgentDetailProps({
  activeTabKey,
  agentLogs,
  agentLogsError,
  agentLogsLoading,
  managedConfig,
  canManageSystem,
  isAreaManagerAccount,
  clientSearch,
  configAudits,
  configAuditsLoading,
  configError,
  configLoading,
  configSavingSection,
  overviewLoading,
  currencyOptions,
  agentRefreshLoading,
  xuiRestartLoading,
  realmCopyLoading,
  remoteCommandLoading,
  dashboardView,
  entryAddressInputText,
  filteredAgents,
  filteredClients,
  filteredTagLinks,
  newTagName,
  currentOverview,
  overviewError,
  selectedAgent,
  selectedAgentId,
  selectedNodeAnchor,
  selectedOutboundTag,
  selectedRuleIndex,
  selectedTag,
  tagOptions,
  tagSaving,
  xuiActions,
  xuiActionsLoading,
  xuiClientDeleteLoadingKey,
  routingRuleDeleteLoadingAgentId,
  routingRuleUpdateLoadingAgentId,
  outboundDeleteLoadingAgentId,
  xuiClientToggleLoadingKey,
  xuiClientResetLoadingKey,
  agentDeleteLoading,
  agentReplaceLoading,
  agents,
  openAssetsPage,
  setActiveTabKey,
  setClientSearch,
  copyImportURL,
  setXUIActionAgentId,
  setAddClientActionInbounds,
  setXUIActionKind,
  setAddClientActionForm,
  setOutboundActionForm,
  setRoutingActionForm,
  setXUIActionModalOpen,
  createTagOption,
  setEntryAddressInputText,
  updateManagedConfig,
  jumpToNode,
  jumpToOutbound,
  jumpToRule,
  setNewTagName,
  setImportURLClient,
  loadAgentLogs,
  openCustomerAuthorization,
  deleteAgent,
  replaceAgent,
  deleteXUIClient,
  deleteRoutingRules,
  updateRoutingRule,
  loadOverview,
  deleteOutbounds,
  setXUIClientEnabled,
  manualResetXUIClientTraffic,
  requestAgentSnapshot,
  restartXUIService,
  executeRemoteCommand,
  loadXUIActions,
  copyRealmConfigToAgent,
  savedManagedConfig,
  setAdminUser,
  isCurrentAgentRequest,
  setSavedManagedConfig,
  setManagedConfig,
  loadAgents,
  loadConfigAudits,
  scheduleXUIActionResultRefresh,
  saveManagedConfigSection,
  savePrimaryDomain,
  selectDashboardTag,
  setTagOptions,
  saveAreaAgentTags,
}: Options): AgentDetailPanelProps {
  return {
    activeTabKey: activeTabKey,
    agentLogs: agentLogs,
    agentLogsError: agentLogsError,
    agentLogsLoading: agentLogsLoading,
    canOpenXUI: Boolean(managedConfig?.xui?.base_url),
    canManageConfig: canManageSystem,
    restrictedView: isAreaManagerAccount,
    clientSearch: clientSearch,
    configAudits: configAudits,
    configAuditsLoading: configAuditsLoading,
    configError: configError,
    configLoading: configLoading,
    configSavingSection: configSavingSection,
    currencyOptions: currencyOptions,
    currentAgentLoading: overviewLoading || configLoading || agentRefreshLoading,
    xuiRestartLoading: xuiRestartLoading,
    realmCopyLoading: realmCopyLoading,
    remoteCommandLoading: remoteCommandLoading,
    dashboardView: dashboardView,
    entryAddressInputText: entryAddressInputText,
    filteredAgents: filteredAgents,
    filteredClients: filteredClients,
    filteredTagLinks: filteredTagLinks,
    managedConfig: managedConfig,
    newTagName: newTagName,
    overview: currentOverview,
    overviewError: overviewError,
    selectedAgent,
    selectedAgentId: selectedAgentId,
    selectedNodeAnchor: selectedNodeAnchor,
    selectedOutboundTag: selectedOutboundTag,
    selectedRuleIndex: selectedRuleIndex,
    selectedTag: selectedTag,
    tagOptions: tagOptions,
    tagSaving: tagSaving,
    xuiActions: xuiActions,
    xuiActionsLoading: xuiActionsLoading,
    xuiClientDeleteLoadingKey: xuiClientDeleteLoadingKey,
    routingRuleDeleteLoading: routingRuleDeleteLoadingAgentId === selectedAgentId,
    routingRuleUpdateLoading: routingRuleUpdateLoadingAgentId === selectedAgentId,
    outboundDeleteLoading: outboundDeleteLoadingAgentId === selectedAgentId,
    xuiClientToggleLoadingKey: xuiClientToggleLoadingKey,
    xuiClientResetLoadingKey: xuiClientResetLoadingKey,
    agentDeleteLoading: agentDeleteLoading,
    agentReplaceLoading: agentReplaceLoading,
    replacementAgents: agents.filter((agent) => agent.agent_id !== selectedAgentId),
    onBackToList: openAssetsPage,
    onActiveTabChange: setActiveTabKey,
    onClientSearchChange: setClientSearch,
    onCopyImportURL: (client) => void copyImportURL(client),
    onCreateRoutingAction: () => {
      setXUIActionAgentId(selectedAgentId)
      setAddClientActionInbounds([])
      setXUIActionKind('upsert_routing_rule')
      setAddClientActionForm(defaultAddClientActionForm())
      setOutboundActionForm(defaultOutboundActionForm())
      setRoutingActionForm(defaultRoutingActionForm())
      setXUIActionModalOpen(true)
    },
    onCreateNodeClientAction: (node, actionAgentID) => {
      const targetAgentID = actionAgentID || node.realm_target_agent_id || selectedAgentId
      const targetNode: XUINodeView = node.realm_target_inbound_id
        ? {
            ...node,
            id: node.realm_target_inbound_id,
            tag: node.realm_target_inbound_tag || node.tag || '',
            remark: node.remark || node.realm_target_inbound_tag || `Inbound #${node.realm_target_inbound_id}`,
          }
        : node
      setXUIActionAgentId(targetAgentID)
      setAddClientActionInbounds([targetNode])
      setXUIActionKind('add_client')
      setAddClientActionForm({
        ...defaultAddClientActionForm(),
        inbound_id: targetNode.id,
        inbound_tag: targetNode.tag || '',
        inbound_name: targetNode.remark || targetNode.tag || `Inbound #${targetNode.id}`,
        protocol: targetNode.protocol || 'vless',
      })
      setXUIActionModalOpen(true)
    },
    onCreateTag: () => void createTagOption(),
    onEntryAddressesTextChange: (value) => {
      setEntryAddressInputText(value)
      updateManagedConfig((current) => ({
        ...current,
        entry: { ...current.entry, addresses: parseAddressInput(value) },
      }))
    },
    onEntryChange: (patch) => updateManagedConfig((current) => ({ ...current, entry: { ...current.entry, ...patch } })),
    onJumpNode: jumpToNode,
    onJumpOutbound: jumpToOutbound,
    onJumpRule: jumpToRule,
    onManagedConfigAgentNameChange: (value) => updateManagedConfig((current) => ({ ...current, agent_name: value })),
    onManagedConfigCustomerDisplayNameChange: (value) =>
      updateManagedConfig((current) => ({ ...current, customer_display_name: value })),
    onManagedConfigSortOrderChange: (value) => updateManagedConfig((current) => ({ ...current, sort_order: value })),
    onNewTagNameChange: setNewTagName,
    onOpenImportURL: setImportURLClient,
    onOpenLogs: () => {
      setActiveTabKey('logs')
      void loadAgentLogs(selectedAgentId)
    },
    onOpenXUI: () => {
      if (managedConfig?.xui?.base_url) {
        window.open(managedConfig.xui.base_url, '_blank', 'noopener,noreferrer')
      }
    },
    onAuthorizeCustomer: openCustomerAuthorization,
    onDeleteCurrentAgent: () => void deleteAgent(selectedAgentId),
    onReplaceCurrentAgent: (replacementAgentID) => replaceAgent(replacementAgentID, selectedAgentId),
    onDeleteXUIClient: (client) => void deleteXUIClient(client, selectedAgentId),
    onDeleteRoutingRules: (payload) => deleteRoutingRules(payload, selectedAgentId),
    onUpdateRoutingRule: (payload) => updateRoutingRule(payload, selectedAgentId),
    onRoutingRuleUpdated: () => scheduleXUIActionResultRefresh(selectedAgentId, { delays: [0, 1000, 3000] }),
    onDeleteOutbounds: (payload) => deleteOutbounds(payload, selectedAgentId),
    onSetXUIClientEnabled: (client, enabled) => void setXUIClientEnabled(client, enabled, selectedAgentId),
    onManualResetXUIClientTraffic: (client) => manualResetXUIClientTraffic(client, selectedAgentId),
    onRefreshCurrentAgent: () => void requestAgentSnapshot(selectedAgentId),
    onRestartXUI: () => void restartXUIService(selectedAgentId),
    onExecuteRemoteCommand: (command, shell, timeoutSeconds) =>
      void executeRemoteCommand(command, shell, timeoutSeconds, selectedAgentId),
    onRefreshXUIActions: () => void loadXUIActions(),
    onCopyRealmConfig: (targetAgentID) => void copyRealmConfigToAgent(targetAgentID, selectedAgentId),
    onRenewalChange: (patch) =>
      updateManagedConfig((current) => ({ ...current, renewal: { ...current.renewal, ...patch } })),
    savedClientBillings: savedManagedConfig?.renewal?.client_billings,
    onClientConfigUnauthorized: () => setAdminUser(null),
    onClientConfigSaved: (record, save) => {
      if (!isCurrentAgentRequest(selectedAgentId)) return
      const mergeBilling = (current: ManagedAgentConfig | null) =>
        current
          ? {
              ...current,
              renewal: {
                ...current.renewal,
                client_billings: upsertClientBilling(current.renewal?.client_billings || [], record, save.billing),
              },
            }
          : current
      setSavedManagedConfig(mergeBilling)
      setManagedConfig(mergeBilling)
      void loadAgents({ silent: true })
      void loadOverview(selectedAgentId, { silent: true })
      if (canManageSystem) void loadConfigAudits(selectedAgentId, { silent: true })
      scheduleXUIActionResultRefresh(save.action?.agent_id || selectedAgentId)
      ;[1000, 3000, 8000, 15000].forEach((delay) =>
        window.setTimeout(() => void loadOverview(selectedAgentId, { silent: true }), delay),
      )
    },
    onSaveManagedConfigSection: (section) => void saveManagedConfigSection(section),
    onSavePrimaryDomain: savePrimaryDomain,
    onSelectTag: selectDashboardTag,
    onTagsChange: (values) => {
      setTagOptions((current) => mergeTagOptions(current, values))
      updateManagedConfig((current) => ({ ...current, tags: values }))
    },
    onSaveAreaTags: (values) => void saveAreaAgentTags(values),
    onXUIChange: (patch) => updateManagedConfig((current) => ({ ...current, xui: { ...current.xui, ...patch } })),
    onFeatureChange: (feature, enabled) =>
      updateManagedConfig((current) => {
        const features = { ...current.features, [feature]: enabled }
        const entry = { ...current.entry }
        if (enabled && feature === 'realm') {
          features.haproxy = false
          entry.haproxy = { ...entry.haproxy, enabled: false }
        }
        if (enabled && feature === 'haproxy') {
          features.realm = false
          entry.port_forwarding = { ...entry.port_forwarding, enabled: false, backend: 'none' }
        }
        return { ...current, features, entry }
      }),
  }
}
