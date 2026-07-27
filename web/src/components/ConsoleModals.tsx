import type { Dispatch, SetStateAction } from 'react'

import type {
  AdminUser,
  CustomerAssignment,
  CustomerAssignmentDraft,
  DashboardAgentView,
  SystemInfo,
  TelegramBot,
  UpdateLatestInfo,
  XUIClientView,
  XUINodeView,
  XUIOverview,
} from '../types'
import type {
  ClientInstallCommandForm,
  ClientInstallCommandKind,
  FrontendSettingsForm,
  TelegramBotForm,
  XUIAddClientActionForm,
  XUIOutboundActionForm,
  XUIRoutingActionForm,
} from '../lib/appHelpers'
import {
  AccountSettingsModal,
  ClientInstallModal,
  FrontendSettingsModal,
  ImportURLModal,
  SystemUpdateModal,
  TelegramBotSettingsModal,
  XUIActionModal,
} from './AdminModals'
import { CustomerManagementModal } from './CustomerManagementModal'

interface AccountFormState {
  current_password: string
  new_username: string
  new_password: string
  confirm_password: string
  avatar_url: string
}

export interface ConsoleModalsProps {
  accountForm: AccountFormState
  accountModalOpen: boolean
  accountSaving: boolean
  agents: DashboardAgentView[]
  adminUser: AdminUser | null
  clientInstallCommandKind: ClientInstallCommandKind
  clientInstallForm: ClientInstallCommandForm
  clientInstallLoading: boolean
  clientInstallModalOpen: boolean
  clientInstallSaving: boolean
  clientInstallLinuxCommand: string
  clientInstallOpenWrtCommand: string
  clientInstallWindowsCMDCommand: string
  clientInstallWindowsPowerShellCommand: string
  customerModalOpen: boolean
  customerAssignmentDraft: CustomerAssignmentDraft | null
  editingTelegramBotId: number | null
  frontendSettingsForm: FrontendSettingsForm
  frontendSettingsLoading: boolean
  frontendSettingsModalOpen: boolean
  frontendSettingsSaving: boolean
  importURLClient: XUIClientView | null
  addClientActionForm: XUIAddClientActionForm
  addClientActionInbounds: XUINodeView[]
  outboundActionForm: XUIOutboundActionForm
  outboundSourceLoading: boolean
  outboundSourceOverview: XUIOverview | null
  overview: XUIOverview | null
  routingActionForm: XUIRoutingActionForm
  selectedAgentId: string
  systemInfo: SystemInfo | null
  telegramBotForm: TelegramBotForm
  telegramBotModalOpen: boolean
  telegramBotSaving: boolean
  telegramBots: TelegramBot[]
  telegramBotsLoading: boolean
  updateLatestError: string
  updateLatestInfo: UpdateLatestInfo | null
  updateLatestLoading: boolean
  updateLoading: boolean
  updateModalOpen: boolean
  xuiActionKind: string
  xuiActionModalOpen: boolean
  xuiActionSaving: boolean
  onAccountFormChange: Dispatch<SetStateAction<AccountFormState>>
  onClientInstallCommandKindChange: Dispatch<SetStateAction<ClientInstallCommandKind>>
  onClientInstallFormChange: Dispatch<SetStateAction<ClientInstallCommandForm>>
  onCloseAccount: () => void
  onCloseClientInstall: () => void
  onCloseCustomerModal: () => void
  onCustomerAssignmentDraftApplied: () => void
  onCloseFrontendSettings: () => void
  onCloseImportURL: () => void
  onCloseTelegramBot: () => void
  onCloseUpdateModal: () => void
  onCloseXUIActionModal: () => void
  onConfigChanged?: (agentID?: string) => void | Promise<void>
  onOpenCustomerAssignment?: (assignment: CustomerAssignment) => void
  onCopyClientInstallCommand: (command: string) => void
  onCopyImportURL: (client: XUIClientView) => void
  onDeleteTelegramBot: (id: number) => void
  onRefreshLatestUpdate: () => void
  onRefreshTelegramBots: () => void
  onSaveAccount: () => void
  onSaveClientInstallSettings: () => void
  onSaveFrontendSettings: () => void
  onSaveTelegramBot: () => void
  onSubmitXUIAction: () => void
  onTelegramBotFormChange: Dispatch<SetStateAction<TelegramBotForm>>
  onTelegramBotEditIDChange: Dispatch<SetStateAction<number | null>>
  onTestTelegramBot: (id: number) => void
  onUpdateAllClients: () => void
  onUpdateFrontendSettingsFormChange: Dispatch<SetStateAction<FrontendSettingsForm>>
  onUpdateAddClientActionForm: Dispatch<SetStateAction<XUIAddClientActionForm>>
  onUpdateOutboundActionForm: Dispatch<SetStateAction<XUIOutboundActionForm>>
  onUpdateRoutingActionForm: Dispatch<SetStateAction<XUIRoutingActionForm>>
  onUpdateServer: () => void
  onXUIActionKindChange: Dispatch<SetStateAction<string>>
}

export function ConsoleModals(props: ConsoleModalsProps) {
  const {
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
    clientInstallLinuxCommand,
    clientInstallOpenWrtCommand,
    clientInstallWindowsCMDCommand,
    clientInstallWindowsPowerShellCommand,
    customerModalOpen,
    customerAssignmentDraft,
    editingTelegramBotId,
    frontendSettingsForm,
    frontendSettingsLoading,
    frontendSettingsModalOpen,
    frontendSettingsSaving,
    importURLClient,
    addClientActionForm,
    addClientActionInbounds,
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
    onAccountFormChange,
    onClientInstallCommandKindChange,
    onClientInstallFormChange,
    onCloseAccount,
    onCloseClientInstall,
    onCloseCustomerModal,
    onCustomerAssignmentDraftApplied,
    onCloseFrontendSettings,
    onCloseImportURL,
    onCloseTelegramBot,
    onCloseUpdateModal,
    onCloseXUIActionModal,
    onConfigChanged,
    onOpenCustomerAssignment,
    onCopyClientInstallCommand,
    onCopyImportURL,
    onDeleteTelegramBot,
    onRefreshLatestUpdate,
    onRefreshTelegramBots,
    onSaveAccount,
    onSaveClientInstallSettings,
    onSaveFrontendSettings,
    onSaveTelegramBot,
    onSubmitXUIAction,
    onTelegramBotFormChange,
    onTelegramBotEditIDChange,
    onTestTelegramBot,
    onUpdateAllClients,
    onUpdateFrontendSettingsFormChange,
    onUpdateAddClientActionForm,
    onUpdateOutboundActionForm,
    onUpdateRoutingActionForm,
    onUpdateServer,
    onXUIActionKindChange,
  } = props

  return (
    <>
      <SystemUpdateModal
        open={updateModalOpen}
        loading={updateLoading}
        latestLoading={updateLatestLoading}
        latestInfo={updateLatestInfo}
        latestError={updateLatestError}
        systemInfo={systemInfo}
        onClose={onCloseUpdateModal}
        onRefreshLatest={onRefreshLatestUpdate}
        onUpdateServer={onUpdateServer}
        onUpdateClients={onUpdateAllClients}
      />

      <ClientInstallModal
        open={clientInstallModalOpen}
        loading={clientInstallLoading}
        saving={clientInstallSaving}
        form={clientInstallForm}
        commandKind={clientInstallCommandKind}
        linuxCommand={clientInstallLinuxCommand}
        openWrtCommand={clientInstallOpenWrtCommand}
        windowsPowerShellCommand={clientInstallWindowsPowerShellCommand}
        windowsCMDCommand={clientInstallWindowsCMDCommand}
        onClose={onCloseClientInstall}
        onSave={onSaveClientInstallSettings}
        onCopy={onCopyClientInstallCommand}
        onFormChange={onClientInstallFormChange}
        onCommandKindChange={onClientInstallCommandKindChange}
      />

      <AccountSettingsModal
        open={accountModalOpen}
        saving={accountSaving}
        form={accountForm}
        onClose={onCloseAccount}
        onSave={onSaveAccount}
        onFormChange={onAccountFormChange}
      />

      <FrontendSettingsModal
        open={frontendSettingsModalOpen}
        loading={frontendSettingsLoading}
        saving={frontendSettingsSaving}
        form={frontendSettingsForm}
        onClose={onCloseFrontendSettings}
        onSave={onSaveFrontendSettings}
        onFormChange={onUpdateFrontendSettingsFormChange}
      />

      <TelegramBotSettingsModal
        open={telegramBotModalOpen}
        bots={telegramBots}
        loading={telegramBotsLoading}
        saving={telegramBotSaving}
        editingID={editingTelegramBotId}
        form={telegramBotForm}
        onClose={onCloseTelegramBot}
        onFormChange={onTelegramBotFormChange}
        onSave={onSaveTelegramBot}
        onRefresh={onRefreshTelegramBots}
        onEditIDChange={onTelegramBotEditIDChange}
        onDelete={onDeleteTelegramBot}
        onTest={onTestTelegramBot}
      />

      <CustomerManagementModal
        open={customerModalOpen}
        agents={agents}
        adminUser={adminUser}
        initialAssignment={customerAssignmentDraft}
        onInitialAssignmentApplied={onCustomerAssignmentDraftApplied}
        onClose={onCloseCustomerModal}
        onConfigChanged={onConfigChanged}
        onOpenAssignment={onOpenCustomerAssignment}
      />

      <XUIActionModal
        open={xuiActionModalOpen}
        saving={xuiActionSaving}
        actionKind={xuiActionKind}
        addClientForm={addClientActionForm}
        addClientInbounds={addClientActionInbounds}
        outboundForm={outboundActionForm}
        routingForm={routingActionForm}
        agents={agents}
        targetAgentID={selectedAgentId}
        currentOverview={overview}
        sourceOverview={outboundSourceOverview}
        sourceLoading={outboundSourceLoading}
        allowCreateOutbound={adminUser?.role !== 'area_manager' || Boolean(adminUser.outbound_create_enabled)}
        authorizedClientNodesOnly={adminUser?.role === 'area_manager'}
        onClose={onCloseXUIActionModal}
        onSubmit={onSubmitXUIAction}
        onActionKindChange={onXUIActionKindChange}
        onAddClientFormChange={onUpdateAddClientActionForm}
        onOutboundFormChange={onUpdateOutboundActionForm}
        onRoutingFormChange={onUpdateRoutingActionForm}
      />

      <ImportURLModal client={importURLClient} onClose={onCloseImportURL} onCopy={onCopyImportURL} />
    </>
  )
}
