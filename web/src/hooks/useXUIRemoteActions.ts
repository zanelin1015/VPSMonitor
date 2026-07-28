import { App as AntdApp } from 'antd'
import { useState } from 'react'

import type { AdminUser, XUIAction, XUIClientView } from '../types'
import { fetchJSON, isUnauthorized } from '../lib/appHelpers'

type LoadOptions = { silent?: boolean }
type LoadAgentResource = (agentID: string, options?: LoadOptions) => Promise<void>

export function useXUIRemoteActions(input: {
  selectedAgentId: string
  setAdminUser: (user: AdminUser | null) => void
  loadAgents: (options?: LoadOptions) => Promise<void>
  loadAgentLogs: LoadAgentResource
  loadOverview: LoadAgentResource
  loadXUIActions: LoadAgentResource
  scheduleXUIActionResultRefresh: (agentID: string) => void
}) {
  const { message } = AntdApp.useApp()
  const {
    selectedAgentId,
    setAdminUser,
    loadAgents,
    loadAgentLogs,
    loadOverview,
    loadXUIActions,
    scheduleXUIActionResultRefresh,
  } = input
  const [remoteCommandLoading, setRemoteCommandLoading] = useState(false)
  const [xuiRestartLoading, setXUIRestartLoading] = useState(false)
  const [xuiUpdateLoading, setXUIUpdateLoading] = useState(false)
  const [xuiClientDeleteLoadingKey, setXUIClientDeleteLoadingKey] = useState('')
  const [xuiClientToggleLoadingKey, setXUIClientToggleLoadingKey] = useState('')
  const [xuiClientTrafficSavingKey, setXUIClientTrafficSavingKey] = useState('')

  const handleUnauthorized = (error: unknown) => {
    if (isUnauthorized(error)) {
      setAdminUser(null)
    }
  }

  async function restartXUIService(agentID = selectedAgentId) {
    if (!agentID) {
      return
    }
    setXUIRestartLoading(true)
    try {
      await fetchJSON<XUIAction>(`/api/v1/agents/${agentID}/xui/actions`, {
        method: 'POST',
        body: JSON.stringify({ kind: 'restart_xui', payload: { service_name: 'x-ui' } }),
      })
      message.success('已创建 x-ui / Xray 重启任务；在线 Client 会通过 WS 立即执行，失败日志会写入操作记录')
      window.setTimeout(() => {
        void loadXUIActions(agentID, { silent: true })
        void loadAgentLogs(agentID, { silent: true })
        void loadOverview(agentID, { silent: true })
      }, 2500)
    } catch (error) {
      handleUnauthorized(error)
      message.error(error instanceof Error ? error.message : '下发 x-ui 重启失败')
    } finally {
      setXUIRestartLoading(false)
    }
  }

  async function update3XUI(agentID = selectedAgentId) {
    if (!agentID) {
      return
    }
    setXUIUpdateLoading(true)
    try {
      const action = await fetchJSON<XUIAction>(`/api/v1/agents/${agentID}/xui/actions`, {
        method: 'POST',
        body: JSON.stringify({ kind: 'update_3xui', payload: { timeout_seconds: 900 } }),
      })
      message.success(action.status === 'running' ? '已通过 WS 下发 3x-ui 升级任务，结果会实时回传到操作记录' : '已创建 3x-ui 升级任务，Client 不在线时会等待轮询执行')
      await loadXUIActions(agentID, { silent: true })
      scheduleXUIActionResultRefresh(agentID)
    } catch (error) {
      handleUnauthorized(error)
      message.error(error instanceof Error ? error.message : '下发 3x-ui 升级失败')
    } finally {
      setXUIUpdateLoading(false)
    }
  }

  async function deleteXUIClient(record: XUIClientView, agentID = selectedAgentId) {
    const targetAgentID = record.realm_target_agent_id || agentID
    const targetInboundID = record.realm_target_inbound_id || record.inbound_id
    const targetInboundTag = record.realm_target_inbound_tag || record.inbound_tag || ''
    if (!targetAgentID) {
      return
    }
    const key = xuiClientActionKey(record)
    setXUIClientDeleteLoadingKey(key)
    try {
      const action = await fetchJSON<XUIAction>(`/api/v1/agents/${targetAgentID}/xui/actions`, {
        method: 'POST',
        body: JSON.stringify({
          kind: 'delete_client',
          payload: {
            inbound_id: targetInboundID,
            inbound_tag: targetInboundTag,
            protocol: record.protocol || '',
            email: record.email || '',
            client_id: record.auth_uuid || record.auth_password || '',
            restart: false,
          },
        }),
      })
      message.success(action.status === 'running' ? '已通过 WS 下发删除 Client 任务，结果会回传到操作记录' : '已创建删除 Client 任务，Client 不在线时会等待轮询执行')
      await loadXUIActions(targetAgentID, { silent: true })
      scheduleXUIActionResultRefresh(targetAgentID)
      window.setTimeout(() => {
        void loadOverview(targetAgentID, { silent: true })
        if (selectedAgentId && targetAgentID !== selectedAgentId) {
          void loadOverview(selectedAgentId, { silent: true })
        }
        void loadAgents({ silent: true })
      }, 2500)
    } catch (error) {
      handleUnauthorized(error)
      message.error(error instanceof Error ? error.message : '删除 Client 失败')
    } finally {
      setXUIClientDeleteLoadingKey('')
    }
  }

  async function setXUIClientEnabled(record: XUIClientView, enabled: boolean, agentID = selectedAgentId) {
    const targetAgentID = record.realm_target_agent_id || agentID
    const targetInboundID = record.realm_target_inbound_id || record.inbound_id
    const targetInboundTag = record.realm_target_inbound_tag || record.inbound_tag || ''
    if (!targetAgentID) {
      return
    }
    const key = xuiClientActionKey(record)
    setXUIClientToggleLoadingKey(key)
    try {
      const action = await fetchJSON<XUIAction>(`/api/v1/agents/${targetAgentID}/xui/actions`, {
        method: 'POST',
        body: JSON.stringify({
          kind: 'set_client_enabled',
          payload: {
            inbound_id: targetInboundID,
            inbound_tag: targetInboundTag,
            email: record.email || '',
            client_id: record.auth_uuid || record.auth_password || '',
            enabled,
            restart: false,
          },
        }),
      })
      message.success(action.status === 'running' ? `已通过 WS 下发${enabled ? '启用' : '停用'} Client 任务，结果会回传到操作记录` : `已创建${enabled ? '启用' : '停用'} Client 任务，Client 不在线时会等待轮询执行`)
      await loadXUIActions(targetAgentID, { silent: true })
      scheduleXUIActionResultRefresh(targetAgentID)
      window.setTimeout(() => {
        void loadOverview(targetAgentID, { silent: true })
        if (selectedAgentId && targetAgentID !== selectedAgentId) {
          void loadOverview(selectedAgentId, { silent: true })
        }
        void loadAgents({ silent: true })
      }, 2500)
    } catch (error) {
      handleUnauthorized(error)
      message.error(error instanceof Error ? error.message : `${enabled ? '启用' : '停用'} Client 失败`)
    } finally {
      setXUIClientToggleLoadingKey('')
    }
  }

  async function saveXUIClientTrafficLimit(record: XUIClientView, totalGB: number, agentID = selectedAgentId) {
    const targetAgentID = record.realm_target_agent_id || agentID
    const targetInboundID = record.realm_target_inbound_id || record.inbound_id
    const targetInboundTag = record.realm_target_inbound_tag || record.inbound_tag || ''
    if (!targetAgentID || !record.email) {
      return
    }
    const key = xuiClientActionKey(record)
    const normalizedGB = Math.max(0, Number(totalGB || 0))
    const totalBytes = Math.round(normalizedGB * 1024 * 1024 * 1024)
    setXUIClientTrafficSavingKey(key)
    try {
      const action = await fetchJSON<XUIAction>(`/api/v1/agents/${targetAgentID}/xui/actions`, {
        method: 'POST',
        body: JSON.stringify({
          kind: 'update_client_traffic_limit',
          payload: {
            inbound_id: targetInboundID,
            inbound_tag: targetInboundTag,
            email: record.email,
            total_bytes: totalBytes,
          },
        }),
      })
      message.success(action.status === 'running'
        ? `流量上限已通过 WS 下发到 x-ui（${normalizedGB > 0 ? `${normalizedGB} GB` : '无上限'}）`
        : '流量上限同步任务已创建，Client 在线后会自动执行')
      await loadXUIActions(targetAgentID, { silent: true })
      scheduleXUIActionResultRefresh(targetAgentID)
      window.setTimeout(() => {
        void loadOverview(targetAgentID, { silent: true })
        if (selectedAgentId && targetAgentID !== selectedAgentId) {
          void loadOverview(selectedAgentId, { silent: true })
        }
      }, 2500)
    } catch (error) {
      handleUnauthorized(error)
      message.error(error instanceof Error ? error.message : '同步 x-ui 流量上限失败')
    } finally {
      setXUIClientTrafficSavingKey('')
    }
  }

  async function executeRemoteCommand(command: string, shell: string, timeoutSeconds: number, agentID = selectedAgentId) {
    if (!agentID) {
      return
    }
    setRemoteCommandLoading(true)
    try {
      await fetchJSON<XUIAction>(`/api/v1/agents/${agentID}/xui/actions`, {
        method: 'POST',
        body: JSON.stringify({
          kind: 'execute_command',
          payload: { command, shell, timeout_seconds: timeoutSeconds },
        }),
      })
      message.success('命令已下发，Client 会以服务权限执行并回传结果')
      window.setTimeout(() => {
        void loadXUIActions(agentID, { silent: true })
      }, 2500)
    } catch (error) {
      handleUnauthorized(error)
      message.error(error instanceof Error ? error.message : '下发远程命令失败')
    } finally {
      setRemoteCommandLoading(false)
    }
  }

  return {
    remoteCommandLoading,
    xuiClientDeleteLoadingKey,
    xuiClientToggleLoadingKey,
    xuiClientTrafficSavingKey,
    xuiRestartLoading,
    xuiUpdateLoading,
    deleteXUIClient,
    executeRemoteCommand,
    restartXUIService,
    saveXUIClientTrafficLimit,
    setXUIClientEnabled,
    update3XUI,
  }
}

function xuiClientActionKey(record: XUIClientView): string {
  return [record.inbound_id, record.inbound_tag || '', record.email || '', record.auth_uuid || record.auth_password || ''].join(':')
}
