import { App as AntdApp } from 'antd'
import { useState } from 'react'

import type {
  AccessLogEntry,
  AccessLogListResponse,
  AdminUser,
  ClientInstallInfo,
  FrontendSettings,
  ScheduledTaskSettings,
  TelegramBot,
  UpdateLatestInfo,
  UpdateResponse,
} from '../types'
import type {
  ClientInstallCommandForm,
  ClientInstallCommandKind,
  FrontendSettingsForm,
  TelegramBotForm,
} from '../lib/appHelpers'
import {
  buildClientInstallCommand,
  buildOpenWrtInstallCommand,
  buildWindowsCMDInstallCommand,
  buildWindowsPowerShellInstallCommand,
  defaultClientInstallCommandForm,
  defaultFrontendSettingsForm,
  defaultTelegramBotForm,
  fetchJSON,
  isUnauthorized,
  normalizeClientInstallCommandForm,
  normalizeFrontendSettingsForm,
  serializeFrontendSettingsForm,
} from '../lib/appHelpers'
import { defaultScheduledTaskSettings, normalizeScheduledTaskSettings } from '../lib/scheduledTasks'
import { applyCustomFrontendCode } from '../components/VisualEffects'

interface LoadOptions {
  silent?: boolean
}

export type AccessLogFilters = {
  agent_id: string
  source_ip: string
  target: string
  client_email: string
  limit: number
}

const defaultAccessLogFilters: AccessLogFilters = {
  agent_id: '',
  source_ip: '',
  target: '',
  client_email: '',
  limit: 100,
}

export function useAdminSystemTools(setAdminUser: (user: AdminUser | null) => void) {
  const { message } = AntdApp.useApp()
  const [telegramBots, setTelegramBots] = useState<TelegramBot[]>([])
  const [telegramBotsLoading, setTelegramBotsLoading] = useState(false)
  const [telegramBotModalOpen, setTelegramBotModalOpen] = useState(false)
  const [telegramBotSaving, setTelegramBotSaving] = useState(false)
  const [editingTelegramBotId, setEditingTelegramBotId] = useState<number | null>(null)
  const [telegramBotForm, setTelegramBotForm] = useState<TelegramBotForm>(() => defaultTelegramBotForm())
  const [clientInstallModalOpen, setClientInstallModalOpen] = useState(false)
  const [clientInstallLoading, setClientInstallLoading] = useState(false)
  const [clientInstallSaving, setClientInstallSaving] = useState(false)
  const [clientInstallForm, setClientInstallForm] = useState<ClientInstallCommandForm>(() => defaultClientInstallCommandForm())
  const [clientInstallCommandKind, setClientInstallCommandKind] = useState<ClientInstallCommandKind>('linux')
  const [frontendSettingsModalOpen, setFrontendSettingsModalOpen] = useState(false)
  const [frontendSettingsLoading, setFrontendSettingsLoading] = useState(false)
  const [frontendSettingsSaving, setFrontendSettingsSaving] = useState(false)
  const [frontendSettingsForm, setFrontendSettingsForm] = useState<FrontendSettingsForm>(() => defaultFrontendSettingsForm())
  const [scheduledTasksLoading, setScheduledTasksLoading] = useState(false)
  const [scheduledTasksSaving, setScheduledTasksSaving] = useState(false)
  const [scheduledTasks, setScheduledTasks] = useState<ScheduledTaskSettings>(() => defaultScheduledTaskSettings())
  const [accessLogs, setAccessLogs] = useState<AccessLogEntry[]>([])
  const [accessLogsTotal, setAccessLogsTotal] = useState(0)
  const [accessLogsLoading, setAccessLogsLoading] = useState(false)
  const [accessLogFilters, setAccessLogFilters] = useState<AccessLogFilters>(defaultAccessLogFilters)
  const [updateModalOpen, setUpdateModalOpen] = useState(false)
  const [updateLoading, setUpdateLoading] = useState(false)
  const [updateLatestLoading, setUpdateLatestLoading] = useState(false)
  const [updateLatestInfo, setUpdateLatestInfo] = useState<UpdateLatestInfo | null>(null)
  const [updateLatestError, setUpdateLatestError] = useState('')

  const handleUnauthorized = (error: unknown): boolean => {
    if (isUnauthorized(error)) {
      setAdminUser(null)
      return true
    }
    return false
  }

  async function openClientInstallModal() {
    setClientInstallModalOpen(true)
    setClientInstallLoading(true)
    try {
      const data = await fetchJSON<ClientInstallInfo>('/api/v1/admin/client-install')
      setClientInstallForm(normalizeClientInstallCommandForm(data))
    } catch (error) {
      handleUnauthorized(error)
      message.error(error instanceof Error ? error.message : '加载 Client 安装信息失败')
    } finally {
      setClientInstallLoading(false)
    }
  }

  async function saveClientInstallSettings() {
    setClientInstallSaving(true)
    try {
      const data = await fetchJSON<ClientInstallInfo>('/api/v1/admin/client-install', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          server_url: clientInstallForm.server_url,
          install_script_url: clientInstallForm.install_script_url,
          poll_interval: clientInstallForm.poll_interval,
          request_timeout_seconds: clientInstallForm.request_timeout_seconds,
          server_skip_tls_verify: clientInstallForm.server_skip_tls_verify,
          realm_auto_install: clientInstallForm.realm_auto_install,
          realm_version: clientInstallForm.realm_version,
          realm_download_base_url: clientInstallForm.realm_download_base_url,
          haproxy_auto_install: clientInstallForm.haproxy_auto_install,
          xui_auto_install: false,
        }),
      })
      setClientInstallForm(normalizeClientInstallCommandForm(data))
      message.success('Client 安装参数已保存')
    } catch (error) {
      handleUnauthorized(error)
      message.error(error instanceof Error ? error.message : '保存 Client 安装参数失败')
    } finally {
      setClientInstallSaving(false)
    }
  }

  async function openFrontendSettingsModal(showModal = true) {
    if (showModal) {
      setFrontendSettingsModalOpen(true)
    }
    setFrontendSettingsLoading(true)
    try {
      const data = await fetchJSON<FrontendSettings>('/api/v1/admin/frontend-settings')
      setFrontendSettingsForm(normalizeFrontendSettingsForm(data))
    } catch (error) {
      handleUnauthorized(error)
      message.error(error instanceof Error ? error.message : '加载前端样式设置失败')
    } finally {
      setFrontendSettingsLoading(false)
    }
  }

  async function saveFrontendSettings() {
    setFrontendSettingsSaving(true)
    try {
      const data = await fetchJSON<FrontendSettings>('/api/v1/admin/frontend-settings', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(serializeFrontendSettingsForm(frontendSettingsForm)),
      })
      setFrontendSettingsForm(normalizeFrontendSettingsForm(data))
      applyCustomFrontendCode(data.custom_code || '')
      message.success('前端自定义样式已保存')
    } catch (error) {
      handleUnauthorized(error)
      message.error(error instanceof Error ? error.message : '保存前端样式设置失败')
    } finally {
      setFrontendSettingsSaving(false)
    }
  }

  async function loadScheduledTasks() {
    setScheduledTasksLoading(true)
    try {
      const data = await fetchJSON<ScheduledTaskSettings>('/api/v1/admin/scheduled-tasks')
      setScheduledTasks(normalizeScheduledTaskSettings(data))
    } catch (error) {
      handleUnauthorized(error)
      message.error(error instanceof Error ? error.message : '加载定时任务失败')
    } finally {
      setScheduledTasksLoading(false)
    }
  }

  async function saveScheduledTasks() {
    setScheduledTasksSaving(true)
    try {
      const data = await fetchJSON<ScheduledTaskSettings>('/api/v1/admin/scheduled-tasks', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(normalizeScheduledTaskSettings(scheduledTasks)),
      })
      setScheduledTasks(normalizeScheduledTaskSettings(data))
      message.success('定时任务配置已保存')
    } catch (error) {
      handleUnauthorized(error)
      message.error(error instanceof Error ? error.message : '保存定时任务失败')
    } finally {
      setScheduledTasksSaving(false)
    }
  }

  async function loadAccessLogs() {
    setAccessLogsLoading(true)
    try {
      const params = new URLSearchParams()
      Object.entries(accessLogFilters).forEach(([key, value]) => {
        if (String(value || '').trim()) {
          params.set(key, String(value).trim())
        }
      })
      const data = await fetchJSON<AccessLogListResponse>(`/api/v1/admin/access-logs?${params.toString()}`)
      setAccessLogs(data.items || [])
      setAccessLogsTotal(data.total || 0)
    } catch (error) {
      handleUnauthorized(error)
      message.error(error instanceof Error ? error.message : '加载访问日志失败')
    } finally {
      setAccessLogsLoading(false)
    }
  }

  async function copyClientInstallCommand(command = buildClientInstallCommand(clientInstallForm)) {
    if (!clientInstallForm.registration_token.trim()) {
      message.warning('当前 server 未配置注册 Token，安装命令无法完成 Client 注册')
      return
    }
    try {
      await navigator.clipboard.writeText(command)
      message.success('Client 安装命令已复制')
    } catch {
      message.warning('浏览器不允许直接复制，请手动复制命令')
    }
  }

  async function loadTelegramBots(options: LoadOptions = {}) {
    const silent = Boolean(options.silent)
    if (!silent) {
      setTelegramBotsLoading(true)
    }
    try {
      const data = await fetchJSON<TelegramBot[]>('/api/v1/admin/telegram-bots')
      setTelegramBots(Array.isArray(data) ? data : [])
    } catch (error) {
      if (!handleUnauthorized(error) && !silent) {
        message.error(error instanceof Error ? error.message : '加载 Telegram 机器人失败')
      }
    } finally {
      if (!silent) {
        setTelegramBotsLoading(false)
      }
    }
  }

  async function loadUpdateLatestInfo(options: LoadOptions = {}) {
    const silent = Boolean(options.silent)
    if (!silent) {
      setUpdateLatestLoading(true)
      setUpdateLatestError('')
    }
    try {
      const data = await fetchJSON<UpdateLatestInfo>('/api/v1/admin/updates/latest')
      setUpdateLatestInfo(data)
    } catch (error) {
      handleUnauthorized(error)
      if (!silent) {
        setUpdateLatestError(error instanceof Error ? error.message : '获取最新版本失败')
      }
    } finally {
      if (!silent) {
        setUpdateLatestLoading(false)
      }
    }
  }

  async function updateServerOnline() {
    if (!updateLatestInfo?.server_update_available) {
      message.info('当前 Server 已是最新版本')
      return
    }
    setUpdateLoading(true)
    try {
      await fetchJSON<UpdateResponse>('/api/v1/admin/updates/server', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ version: updateLatestInfo.latest_server_tag || updateLatestInfo.latest_server_version || updateLatestInfo.latest_tag || updateLatestInfo.latest_version }),
      })
      message.success('Server 升级已启动，服务会自动重启')
      await loadUpdateLatestInfo()
    } catch (error) {
      handleUnauthorized(error)
      message.error(error instanceof Error ? error.message : '启动 Server 升级失败')
    } finally {
      setUpdateLoading(false)
    }
  }

  async function updateAllClientsOnline() {
    if (!updateLatestInfo?.client_update_available_count) {
      message.info('没有需要升级的 Client')
      return
    }
    setUpdateLoading(true)
    try {
      const result = await fetchJSON<UpdateResponse>('/api/v1/admin/updates/clients', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ version: updateLatestInfo.latest_client_tag || updateLatestInfo.latest_client_version || updateLatestInfo.latest_tag || updateLatestInfo.latest_version }),
      })
      message.success(`已下发 Client 升级任务：${result.count || 0} 台，跳过 ${result.skipped || 0} 台`)
      await loadUpdateLatestInfo()
    } catch (error) {
      handleUnauthorized(error)
      message.error(error instanceof Error ? error.message : '下发 Client 升级失败')
    } finally {
      setUpdateLoading(false)
    }
  }

  async function saveTelegramBot() {
    if (!telegramBotForm.name.trim() || !telegramBotForm.chat_id.trim()) {
      message.error('机器人名称和 Chat ID 必填')
      return
    }
    if (!editingTelegramBotId && !telegramBotForm.bot_token.trim()) {
      message.error('新增机器人时 Bot Token 必填')
      return
    }
    setTelegramBotSaving(true)
    try {
      const payload = {
        name: telegramBotForm.name.trim(),
        bot_token: telegramBotForm.bot_token.trim(),
        chat_id: telegramBotForm.chat_id.trim(),
        enabled: telegramBotForm.enabled,
      }
      if (editingTelegramBotId) {
        await fetchJSON<TelegramBot>(`/api/v1/admin/telegram-bots/${editingTelegramBotId}`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        })
        message.success('Telegram 机器人已更新')
      } else {
        await fetchJSON<TelegramBot>('/api/v1/admin/telegram-bots', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        })
        message.success('Telegram 机器人已新增')
      }
      setEditingTelegramBotId(null)
      setTelegramBotForm(defaultTelegramBotForm())
      await loadTelegramBots()
    } catch (error) {
      handleUnauthorized(error)
      message.error(error instanceof Error ? error.message : '保存 Telegram 机器人失败')
    } finally {
      setTelegramBotSaving(false)
    }
  }

  async function deleteTelegramBot(id: number) {
    try {
      await fetchJSON<{ status: string }>(`/api/v1/admin/telegram-bots/${id}`, { method: 'DELETE' })
      message.success('Telegram 机器人已删除')
      await loadTelegramBots()
    } catch (error) {
      handleUnauthorized(error)
      message.error(error instanceof Error ? error.message : '删除 Telegram 机器人失败')
    }
  }

  async function testTelegramBot(id: number) {
    try {
      await fetchJSON<{ status: string }>(`/api/v1/admin/telegram-bots/${id}/test`, { method: 'POST' })
      message.success('测试消息已发送')
    } catch (error) {
      handleUnauthorized(error)
      message.error(error instanceof Error ? error.message : '发送测试消息失败')
    }
  }

  return {
    accessLogFilters,
    accessLogs,
    accessLogsLoading,
    accessLogsTotal,
    clientInstallCommand: buildClientInstallCommand(clientInstallForm),
    clientInstallCommandKind,
    clientInstallForm,
    clientInstallLoading,
    clientInstallModalOpen,
    clientInstallSaving,
    clientOpenWrtCommand: buildOpenWrtInstallCommand(clientInstallForm),
    clientWindowsCMDCommand: buildWindowsCMDInstallCommand(clientInstallForm),
    clientWindowsPowerShellCommand: buildWindowsPowerShellInstallCommand(clientInstallForm),
    editingTelegramBotId,
    frontendSettingsForm,
    frontendSettingsLoading,
    frontendSettingsModalOpen,
    frontendSettingsSaving,
    scheduledTasks,
    scheduledTasksLoading,
    scheduledTasksSaving,
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
    copyClientInstallCommand,
    deleteTelegramBot,
    loadAccessLogs,
    loadScheduledTasks,
    loadTelegramBots,
    loadUpdateLatestInfo,
    openClientInstallModal,
    openFrontendSettingsModal,
    saveClientInstallSettings,
    saveFrontendSettings,
    saveScheduledTasks,
    saveTelegramBot,
    setAccessLogFilters,
    setClientInstallCommandKind,
    setClientInstallForm,
    setClientInstallModalOpen,
    setEditingTelegramBotId,
    setFrontendSettingsForm,
    setFrontendSettingsModalOpen,
    setScheduledTasks,
    setTelegramBotForm,
    setTelegramBotModalOpen,
    setTelegramBots,
    setUpdateModalOpen,
    testTelegramBot,
    updateAllClientsOnline,
    updateServerOnline,
  }
}
