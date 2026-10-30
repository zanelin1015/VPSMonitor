import type { AgentEntryConfig, AgentLogsResponse, ConfigAuditLog, CustomerAssignmentDraft, DashboardAgentView, GlobalDashboardView, ManagedAgentConfig, TopologyLinkView, VPSRenewalConfig, XUIAction, XUIClientBillingConfig, XUIClientConfigSave, XUIClientView, XUIConfig, SingBoxConfig, XUINodeView, XUIOverview } from '../types'
import type { RoutingRulesDeletePayload } from '../lib/routingRules'
import type { RoutingRuleUpdatePayload } from '../lib/routingRuleEditor'
import type { OutboundsDeletePayload } from '../lib/outbounds'
import type { ConfigSectionKey } from '../lib/appHelpers'
import type { CurrencyCode } from '../lib/currency'
import type { AgentFeatureKey } from './AgentDetailHelpers'

export interface AgentDetailPanelProps {
  activeTabKey: string
  agentLogs: AgentLogsResponse | null
  agentLogsError: string
  agentLogsLoading: boolean
  clientSearch: string
  configAudits: ConfigAuditLog[]
  configAuditsLoading: boolean
  configError: string
  configLoading: boolean
  configSavingSection: ConfigSectionKey | null
  currencyOptions: CurrencyCode[]
  dashboardView: GlobalDashboardView | null
  entryAddressInputText: string
  filteredAgents: DashboardAgentView[]
  filteredClients: XUIClientView[]
  filteredTagLinks: TopologyLinkView[]
  managedConfig: ManagedAgentConfig | null
  savedClientBillings?: XUIClientBillingConfig[]
  onClientConfigSaved: (record: XUIClientView, save: XUIClientConfigSave) => void
  onClientConfigUnauthorized: () => void
  newTagName: string
  overview: XUIOverview | null
  overviewError: string
  selectedAgent: DashboardAgentView
  selectedAgentId: string
  selectedNodeAnchor: string
  selectedOutboundTag: string
  selectedRuleIndex: number | null
  selectedTag: string
  tagOptions: string[]
  tagSaving: boolean
  xuiActions: XUIAction[]
  xuiActionsLoading: boolean
  xuiClientDeleteLoadingKey: string
  routingRuleDeleteLoading: boolean
  routingRuleUpdateLoading: boolean
  outboundDeleteLoading: boolean
  xuiClientToggleLoadingKey: string
  xuiClientResetLoadingKey: string
  agentDeleteLoading: boolean
  agentReplaceLoading: boolean
  replacementAgents: DashboardAgentView[]
  onBackToList: () => void
  canOpenXUI: boolean
  canManageConfig: boolean
  restrictedView?: boolean
  currentAgentLoading: boolean
  remoteCommandLoading: boolean
  xuiRestartLoading: boolean
  realmCopyLoading: boolean
  onActiveTabChange: (key: string) => void
  onClientSearchChange: (value: string) => void
  onCopyImportURL: (client: XUIClientView) => void
  onCreateRoutingAction: () => void
  onCreateNodeClientAction: (node: XUINodeView, actionAgentID?: string) => void
  onCreateTag: () => void
  onEntryAddressesTextChange: (value: string) => void
  onEntryChange: (patch: Partial<AgentEntryConfig>) => void
  onJumpNode: (agentID?: string, nodeLabel?: string) => void
  onJumpOutbound: (tag?: string) => void
  onJumpRule: (index?: number) => void
  onManagedConfigAgentNameChange: (value: string) => void
  onManagedConfigCustomerDisplayNameChange: (value: string) => void
  onManagedConfigCountryCodeOverrideChange: (value: string) => void
  onManagedConfigSortOrderChange: (value: number) => void
  onNewTagNameChange: (value: string) => void
  onOpenImportURL: (client: XUIClientView) => void
  onOpenLogs: () => void
  onOpenXUI: () => void
  onAuthorizeCustomer: (draft: CustomerAssignmentDraft) => void
  onDeleteCurrentAgent: () => void
  onReplaceCurrentAgent: (replacementAgentID: string) => Promise<boolean>
  onDeleteXUIClient: (client: XUIClientView) => void
  onDeleteRoutingRules: (payload: RoutingRulesDeletePayload) => Promise<XUIAction | null>
  onUpdateRoutingRule: (payload: RoutingRuleUpdatePayload) => Promise<XUIAction | null>
  onRoutingRuleUpdated?: () => void
  onDeleteOutbounds: (payload: OutboundsDeletePayload) => Promise<XUIAction | null>
  onSetXUIClientEnabled: (client: XUIClientView, enabled: boolean) => void
  onManualResetXUIClientTraffic: (client: XUIClientView) => Promise<void>
  onRefreshCurrentAgent: () => void
  onRestartXUI: () => void
  onExecuteRemoteCommand: (command: string, shell: string, timeoutSeconds: number) => void
  onRefreshXUIActions: () => void
  onCopyRealmConfig: (targetAgentID: string) => void
  onRenewalChange: (patch: Partial<VPSRenewalConfig>) => void
  onSaveManagedConfigSection: (section: ConfigSectionKey, draftOverride?: ManagedAgentConfig) => void
  onSavePrimaryDomain: (value: string) => void
  onSelectTag: (tag: string) => void
  onTagsChange: (values: string[]) => void
  onSaveAreaTags?: (values: string[]) => void
  onXUIChange: (patch: Partial<XUIConfig>) => void
  onSingBoxChange: (patch: Partial<SingBoxConfig>) => void
  onFeatureChange: (feature: AgentFeatureKey, enabled: boolean) => void
}
