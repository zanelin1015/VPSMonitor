import type { ConsoleModalsProps } from '../../components/ConsoleModals'
import type { createAppNavigationHandlers } from '../../lib/appNavigation'
import type { useAdminSession } from '../useAdminSession'
import type { useAdminSystemTools } from '../useAdminSystemTools'
import type { useAdminNavigationState } from './useAdminNavigationState'
import type { useAgentDetailData } from './useAgentDetailData'
import type { useDashboardData } from './useDashboardData'
import type { useXUIActionComposer } from './useXUIActionComposer'

type Options = Pick<
  ReturnType<typeof useAdminSession>,
  | 'accountForm'
  | 'accountModalOpen'
  | 'accountSaving'
  | 'adminUser'
  | 'systemInfo'
  | 'setAccountForm'
  | 'setAccountModalOpen'
  | 'saveAccount'
> &
  Pick<ReturnType<typeof useDashboardData>, 'agents'> &
  Pick<
    ReturnType<typeof useAdminSystemTools>,
    | 'clientInstallCommandKind'
    | 'clientInstallForm'
    | 'clientInstallLoading'
    | 'clientInstallModalOpen'
    | 'clientInstallSaving'
    | 'clientInstallCommand'
    | 'clientOpenWrtCommand'
    | 'clientWindowsCMDCommand'
    | 'clientWindowsPowerShellCommand'
    | 'editingTelegramBotId'
    | 'frontendSettingsForm'
    | 'frontendSettingsLoading'
    | 'frontendSettingsModalOpen'
    | 'frontendSettingsSaving'
    | 'telegramBotForm'
    | 'telegramBotModalOpen'
    | 'telegramBotSaving'
    | 'telegramBots'
    | 'telegramBotsLoading'
    | 'updateLatestError'
    | 'updateLatestInfo'
    | 'updateLatestLoading'
    | 'updateLoading'
    | 'updateModalOpen'
    | 'setClientInstallCommandKind'
    | 'setClientInstallForm'
    | 'setClientInstallModalOpen'
    | 'setFrontendSettingsModalOpen'
    | 'setTelegramBotModalOpen'
    | 'setUpdateModalOpen'
    | 'copyClientInstallCommand'
    | 'deleteTelegramBot'
    | 'loadUpdateLatestInfo'
    | 'loadTelegramBots'
    | 'saveClientInstallSettings'
    | 'saveFrontendSettings'
    | 'saveTelegramBot'
    | 'setTelegramBotForm'
    | 'setEditingTelegramBotId'
    | 'testTelegramBot'
    | 'updateAllClientsOnline'
    | 'setFrontendSettingsForm'
    | 'updateServerOnline'
  > &
  Pick<
    ReturnType<typeof useAdminNavigationState>,
    | 'customerModalOpen'
    | 'customerAssignmentDraft'
    | 'selectedAgentId'
    | 'setCustomerModalOpen'
    | 'setCustomerAssignmentDraft'
  > &
  Pick<
    ReturnType<typeof useXUIActionComposer>,
    | 'importURLClient'
    | 'nodeImportTarget'
    | 'setNodeImportTarget'
    | 'outboundActionForm'
    | 'outboundSourceLoading'
    | 'outboundSourceOverview'
    | 'routingActionForm'
    | 'xuiActionKind'
    | 'xuiActionModalOpen'
    | 'xuiActionSaving'
    | 'addClientActionForm'
    | 'addClientActionInbounds'
    | 'setImportURLClient'
    | 'setXUIActionModalOpen'
    | 'setXUIActionAgentId'
    | 'setAddClientActionInbounds'
    | 'copyImportURL'
    | 'createXUIAction'
    | 'setAddClientActionForm'
    | 'setOutboundActionForm'
    | 'setRoutingActionForm'
    | 'setXUIActionKind'
  > &
  Pick<ReturnType<typeof useAgentDetailData>, 'overview'> &
  Pick<ReturnType<typeof createAppNavigationHandlers>, 'openCustomerAssignment'> & {
    refreshAfterExternalConfigChange: (agentID?: string) => Promise<void>
  }

export function buildConsoleModalProps({
  refreshAfterExternalConfigChange,
  accountForm,
  accountModalOpen,
  accountSaving,
  agents,
  adminUser,
  clientInstallCommandKind,
  clientInstallForm,
  clientInstallLoading,
  clientInstallModalOpen,
  clientInstallSaving,
  clientInstallCommand,
  clientOpenWrtCommand,
  clientWindowsCMDCommand,
  clientWindowsPowerShellCommand,
  customerModalOpen,
  customerAssignmentDraft,
  editingTelegramBotId,
  frontendSettingsForm,
  frontendSettingsLoading,
  frontendSettingsModalOpen,
  frontendSettingsSaving,
  importURLClient,
  nodeImportTarget,
  setNodeImportTarget,
  outboundActionForm,
  outboundSourceLoading,
  outboundSourceOverview,
  overview,
  routingActionForm,
  selectedAgentId,
  systemInfo,
  telegramBotForm,
  telegramBotModalOpen,
  telegramBotSaving,
  telegramBots,
  telegramBotsLoading,
  updateLatestError,
  updateLatestInfo,
  updateLatestLoading,
  updateLoading,
  updateModalOpen,
  xuiActionKind,
  xuiActionModalOpen,
  xuiActionSaving,
  addClientActionForm,
  addClientActionInbounds,
  setAccountForm,
  setClientInstallCommandKind,
  setClientInstallForm,
  setAccountModalOpen,
  setClientInstallModalOpen,
  setCustomerModalOpen,
  setCustomerAssignmentDraft,
  setFrontendSettingsModalOpen,
  setImportURLClient,
  setTelegramBotModalOpen,
  setUpdateModalOpen,
  setXUIActionModalOpen,
  setXUIActionAgentId,
  setAddClientActionInbounds,
  openCustomerAssignment,
  copyClientInstallCommand,
  copyImportURL,
  deleteTelegramBot,
  loadUpdateLatestInfo,
  loadTelegramBots,
  saveAccount,
  saveClientInstallSettings,
  saveFrontendSettings,
  saveTelegramBot,
  createXUIAction,
  setTelegramBotForm,
  setEditingTelegramBotId,
  testTelegramBot,
  updateAllClientsOnline,
  setFrontendSettingsForm,
  setAddClientActionForm,
  setOutboundActionForm,
  setRoutingActionForm,
  updateServerOnline,
  setXUIActionKind,
}: Options): ConsoleModalsProps {
  return {
    accountForm: accountForm,
    accountModalOpen: accountModalOpen,
    accountSaving: accountSaving,
    agents: agents,
    adminUser: adminUser,
    clientInstallCommandKind: clientInstallCommandKind,
    clientInstallForm: clientInstallForm,
    clientInstallLoading: clientInstallLoading,
    clientInstallModalOpen: clientInstallModalOpen,
    clientInstallSaving: clientInstallSaving,
    clientInstallLinuxCommand: clientInstallCommand,
    clientInstallOpenWrtCommand: clientOpenWrtCommand,
    clientInstallWindowsCMDCommand: clientWindowsCMDCommand,
    clientInstallWindowsPowerShellCommand: clientWindowsPowerShellCommand,
    customerModalOpen: customerModalOpen,
    customerAssignmentDraft: customerAssignmentDraft,
    editingTelegramBotId: editingTelegramBotId,
    frontendSettingsForm: frontendSettingsForm,
    frontendSettingsLoading: frontendSettingsLoading,
    frontendSettingsModalOpen: frontendSettingsModalOpen,
    frontendSettingsSaving: frontendSettingsSaving,
    importURLClient: importURLClient,
    nodeImportTarget: nodeImportTarget,
    onNodeImportTargetChange: setNodeImportTarget,
    outboundActionForm: outboundActionForm,
    outboundSourceLoading: outboundSourceLoading,
    outboundSourceOverview: outboundSourceOverview,
    overview: overview,
    routingActionForm: routingActionForm,
    selectedAgentId: selectedAgentId,
    systemInfo: systemInfo,
    telegramBotForm: telegramBotForm,
    telegramBotModalOpen: telegramBotModalOpen,
    telegramBotSaving: telegramBotSaving,
    telegramBots: telegramBots,
    telegramBotsLoading: telegramBotsLoading,
    updateLatestError: updateLatestError,
    updateLatestInfo: updateLatestInfo,
    updateLatestLoading: updateLatestLoading,
    updateLoading: updateLoading,
    updateModalOpen: updateModalOpen,
    xuiActionKind: xuiActionKind,
    xuiActionModalOpen: xuiActionModalOpen,
    xuiActionSaving: xuiActionSaving,
    addClientActionForm: addClientActionForm,
    addClientActionInbounds: addClientActionInbounds,
    onAccountFormChange: setAccountForm,
    onClientInstallCommandKindChange: setClientInstallCommandKind,
    onClientInstallFormChange: setClientInstallForm,
    onCloseAccount: () => setAccountModalOpen(false),
    onCloseClientInstall: () => setClientInstallModalOpen(false),
    onCloseCustomerModal: () => {
      setCustomerModalOpen(false)
      setCustomerAssignmentDraft(null)
    },
    onCustomerAssignmentDraftApplied: () => setCustomerAssignmentDraft(null),
    onCloseFrontendSettings: () => setFrontendSettingsModalOpen(false),
    onCloseImportURL: () => setImportURLClient(null),
    onCloseTelegramBot: () => setTelegramBotModalOpen(false),
    onCloseUpdateModal: () => setUpdateModalOpen(false),
    onCloseXUIActionModal: () => {
      setXUIActionModalOpen(false)
      setXUIActionAgentId('')
      setAddClientActionInbounds([])
    },
    onConfigChanged: refreshAfterExternalConfigChange,
    onOpenCustomerAssignment: openCustomerAssignment,
    onCopyClientInstallCommand: (command) => void copyClientInstallCommand(command),
    onCopyImportURL: (client) => void copyImportURL(client),
    onDeleteTelegramBot: (id) => void deleteTelegramBot(id),
    onRefreshLatestUpdate: () => void loadUpdateLatestInfo(),
    onRefreshTelegramBots: () => void loadTelegramBots(),
    onSaveAccount: () => void saveAccount(),
    onSaveClientInstallSettings: () => void saveClientInstallSettings(),
    onSaveFrontendSettings: saveFrontendSettings,
    onSaveTelegramBot: saveTelegramBot,
    onSubmitXUIAction: () => void createXUIAction(),
    onTelegramBotFormChange: setTelegramBotForm,
    onTelegramBotEditIDChange: setEditingTelegramBotId,
    onTestTelegramBot: (id) => void testTelegramBot(id),
    onUpdateAllClients: () => void updateAllClientsOnline(),
    onForceUpdateAllClients: () => void updateAllClientsOnline(true),
    onUpdateFrontendSettingsFormChange: setFrontendSettingsForm,
    onUpdateAddClientActionForm: setAddClientActionForm,
    onUpdateOutboundActionForm: setOutboundActionForm,
    onUpdateRoutingActionForm: setRoutingActionForm,
    onUpdateServer: () => void updateServerOnline(),
    onXUIActionKindChange: setXUIActionKind,
  }
}
