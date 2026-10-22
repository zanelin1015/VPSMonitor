import type { MessageInstance } from 'antd/es/message/interface'
import type { Dispatch, MutableRefObject, SetStateAction } from 'react'
import { useState } from 'react'
import { fetchJSON, isUnauthorized } from '../../lib/appHelpers'
import type { AdminUser, AgentRefreshResponse, AgentReplacementResult, DashboardAgentView } from '../../types'

interface LoadOptions {
  silent?: boolean
}

interface Options {
  selectedAgentId: string
  message: MessageInstance
  loadAgents: (options?: LoadOptions) => Promise<void>
  loadOverview: (agentID: string, options?: LoadOptions) => Promise<void>
  loadManagedConfig: (agentID: string, options?: LoadOptions) => Promise<void>
  loadXUIActions: (agentID?: string, options?: LoadOptions) => Promise<void>
  loadAgentLogs: (agentID?: string, options?: LoadOptions) => Promise<void>
  setAdminUser: Dispatch<SetStateAction<AdminUser | null>>
  agents: DashboardAgentView[]
  customerDisplayNameCacheRef: MutableRefObject<Record<string, string>>
  returnHome: () => void
  setSelectedAgentId: Dispatch<SetStateAction<string>>
  setReloadToken: Dispatch<SetStateAction<number>>
  topologyLoaded: boolean
  topologyVisible: boolean
  loadTopology: (options?: LoadOptions) => Promise<void>
  setConfigError: Dispatch<SetStateAction<string>>
  loadConfigAudits: (agentID?: string, options?: LoadOptions) => Promise<void>
}

export function useAgentOperations({
  selectedAgentId,
  message,
  loadAgents,
  loadOverview,
  loadManagedConfig,
  loadXUIActions,
  loadAgentLogs,
  setAdminUser,
  agents,
  customerDisplayNameCacheRef,
  returnHome,
  setSelectedAgentId,
  setReloadToken,
  topologyLoaded,
  topologyVisible,
  loadTopology,
  setConfigError,
  loadConfigAudits,
}: Options) {
  const [agentRefreshLoading, setAgentRefreshLoading] = useState(false)

  const [agentDeleteLoading, setAgentDeleteLoading] = useState(false)

  const [agentReplaceLoading, setAgentReplaceLoading] = useState(false)

  const [realmCopyLoading, setRealmCopyLoading] = useState(false)

  async function requestAgentSnapshot(agentID = selectedAgentId) {
    if (!agentID) {
      return
    }
    setAgentRefreshLoading(true)
    try {
      const data = await fetchJSON<AgentRefreshResponse>(`/api/v1/agents/${agentID}/refresh`, {
        method: 'POST',
      })
      message.success(data.message || '已通知 Client 立即采集')
      window.setTimeout(() => {
        void loadAgents({ silent: true })
        void loadOverview(agentID, { silent: true })
        void loadManagedConfig(agentID, { silent: true })
        void loadXUIActions(agentID, { silent: true })
        void loadAgentLogs(agentID, { silent: true })
      }, 2500)
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      message.error(error instanceof Error ? error.message : '通知 Client 采集失败')
    } finally {
      setAgentRefreshLoading(false)
    }
  }

  async function deleteAgent(agentID = selectedAgentId) {
    if (!agentID) {
      return
    }
    const targetName = agents.find((item) => item.agent_id === agentID)?.agent_name || agentID
    setAgentDeleteLoading(true)
    try {
      const result = await fetchJSON<{ status: string; agent_id: string; client_stop_sent?: boolean }>(
        `/api/v1/agents/${encodeURIComponent(agentID)}`,
        {
          method: 'DELETE',
        },
      )
      customerDisplayNameCacheRef.current = Object.fromEntries(
        Object.entries(customerDisplayNameCacheRef.current).filter(([key]) => key !== agentID),
      )
      if (selectedAgentId === agentID) {
        returnHome()
      }
      message.success(
        result.client_stop_sent
          ? `已删除 Client / VPS：${targetName}，并已通知远端停止 Client 服务`
          : `已删除 Client / VPS：${targetName}；远端不在线，未能即时停止 Client 服务`,
      )
      await loadAgents({ silent: true })
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      message.error(error instanceof Error ? error.message : '删除 Client / VPS 失败')
    } finally {
      setAgentDeleteLoading(false)
    }
  }

  async function replaceAgent(replacementAgentID: string, sourceAgentID = selectedAgentId): Promise<boolean> {
    if (!sourceAgentID || !replacementAgentID || sourceAgentID === replacementAgentID) {
      return false
    }
    const sourceName = agents.find((item) => item.agent_id === sourceAgentID)?.agent_name || sourceAgentID
    const replacementName =
      agents.find((item) => item.agent_id === replacementAgentID)?.agent_name || replacementAgentID
    setAgentReplaceLoading(true)
    try {
      const result = await fetchJSON<AgentReplacementResult>(
        `/api/v1/agents/${encodeURIComponent(sourceAgentID)}/replace`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ replacement_agent_id: replacementAgentID }),
        },
      )
      setSelectedAgentId(replacementAgentID)
      setReloadToken((current) => current + 1)
      await loadAgents({ silent: true })
      if (topologyLoaded || topologyVisible) {
        await loadTopology({ silent: true })
      }
      const permissionCount =
        result.area_manager_agents_migrated +
        result.area_assignments_migrated +
        result.customer_assignments_migrated +
        result.outbound_grants_migrated
      const forwardingCount = result.realm_references_updated + result.haproxy_references_updated
      message.success(
        `已将 ${sourceName} 替换为 ${replacementName}：迁移 ${permissionCount} 条授权，更新 ${forwardingCount} 条转发引用；旧 Client 已保留`,
      )
      return true
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      message.error(error instanceof Error ? error.message : '替换 Client 失败')
      return false
    } finally {
      setAgentReplaceLoading(false)
    }
  }

  async function copyRealmConfigToAgent(targetAgentID: string, sourceAgentID = selectedAgentId) {
    if (!sourceAgentID || !targetAgentID) {
      return
    }
    setRealmCopyLoading(true)
    setConfigError('')
    try {
      const result = await fetchJSON<{ apply_sent?: boolean }>(`/api/v1/agents/${sourceAgentID}/realm/copy`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ target_agent_id: targetAgentID }),
      })
      message.success(
        result.apply_sent
          ? 'Realm 配置已复制到目标 Client，并已通过 WS 通知立即生效'
          : 'Realm 配置已复制到目标 Client；目标 Client 不在线时会在下次轮询后生效',
      )
      await loadAgents()
      if (targetAgentID === selectedAgentId) {
        await loadManagedConfig(targetAgentID, { silent: true })
        await loadConfigAudits(targetAgentID, { silent: true })
      }
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      const detail = error instanceof Error ? error.message : '复制 Realm 配置失败'
      setConfigError(detail)
      message.error(detail)
    } finally {
      setRealmCopyLoading(false)
    }
  }

  return {
    agentRefreshLoading,
    setAgentRefreshLoading,
    agentDeleteLoading,
    setAgentDeleteLoading,
    agentReplaceLoading,
    setAgentReplaceLoading,
    realmCopyLoading,
    setRealmCopyLoading,
    requestAgentSnapshot,
    deleteAgent,
    replaceAgent,
    copyRealmConfigToAgent,
  }
}
