import type { useXUIActionResultRefresh } from './useXUIActionResultRefresh'
import { App as AntdApp } from 'antd'
import { useRef, useState } from 'react'

import type { AdminUser, XUIAction, XUIClientView } from '../types'
import { fetchJSON, isUnauthorized } from '../lib/appHelpers'
import type { RoutingRulesDeletePayload } from '../lib/routingRules'
import type { RoutingRuleUpdatePayload } from '../lib/routingRuleEditor'
import type { OutboundsDeletePayload } from '../lib/outbounds'
import { buildManualClientTrafficResetAction } from '../lib/clientTrafficReset'

type LoadOptions = { silent?: boolean }
type LoadAgentResource = (agentID: string, options?: LoadOptions) => Promise<void>

export function useXUIRemoteActions(input: {
  selectedAgentId: string
  setAdminUser: (user: AdminUser | null) => void
  loadOverview: LoadAgentResource
  loadXUIActions: (agentID: string, options?: LoadOptions) => Promise<XUIAction[] | undefined>
} & ReturnType<typeof useXUIActionResultRefresh>) {
  const { message } = AntdApp.useApp()
  const {
    selectedAgentId,
    setAdminUser,
    loadOverview,
    loadXUIActions,
    scheduleXUIActionResultRefresh,
    refreshXUIActionResult,
    waitForXUIAction,
  } = input
  const [remoteCommandLoading, setRemoteCommandLoading] = useState(false)
  const [xuiRestartLoading, setXUIRestartLoading] = useState(false)
  const [xuiClientDeleteLoadingKey, setXUIClientDeleteLoadingKey] = useState('')
  const [xuiClientToggleLoadingKey, setXUIClientToggleLoadingKey] = useState('')
  const xuiClientToggleSubmitting = useRef(false)
  const [xuiClientResetLoadingKey, setXUIClientResetLoadingKey] = useState('')
  const xuiClientResetSubmitting = useRef(false)
  const [routingRuleDeleteLoadingAgentId, setRoutingRuleDeleteLoadingAgentId] = useState('')
  const routingDeleteSubmitting = useRef(false)
  const [routingRuleUpdateLoadingAgentId, setRoutingRuleUpdateLoadingAgentId] = useState('')
  const routingUpdateSubmitting = useRef(false)
  const [outboundDeleteLoadingAgentId, setOutboundDeleteLoadingAgentId] = useState('')
  const outboundDeleteSubmitting = useRef(false)

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
      scheduleXUIActionResultRefresh(agentID, { logs: true, delays: [2500] })
    } catch (error) {
      handleUnauthorized(error)
      message.error(error instanceof Error ? error.message : '下发 x-ui 重启失败')
    } finally {
      setXUIRestartLoading(false)
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
            client_id: record.client_id || record.auth_uuid || record.auth_password || '',
            restart: false,
          },
        }),
      })
      message.success(action.status === 'running' ? '已通过 WS 下发删除 Client 任务，结果会回传到操作记录' : '已创建删除 Client 任务，Client 不在线时会等待轮询执行')
      await loadXUIActions(targetAgentID, { silent: true })
      scheduleXUIActionResultRefresh(targetAgentID, { relatedAgentIDs: [selectedAgentId], agents: true })
    } catch (error) {
      handleUnauthorized(error)
      message.error(error instanceof Error ? error.message : '删除 Client 失败')
    } finally {
      setXUIClientDeleteLoadingKey('')
    }
  }

  async function deleteRoutingRules(payload: RoutingRulesDeletePayload, agentID = selectedAgentId): Promise<XUIAction | null> {
    if (!agentID || routingDeleteSubmitting.current) return null
    routingDeleteSubmitting.current = true
    setRoutingRuleDeleteLoadingAgentId(agentID)
    let action: XUIAction
    try {
      action = await fetchJSON<XUIAction>(`/api/v1/agents/${agentID}/xui/actions`, {
        method: 'POST',
        body: JSON.stringify({ kind: 'delete_routing_rules', payload }),
      })
    } catch (error) {
      handleUnauthorized(error)
      message.error(error instanceof Error ? error.message : '下发删除路由规则任务失败')
      return null
    } finally {
      routingDeleteSubmitting.current = false
      setRoutingRuleDeleteLoadingAgentId('')
    }
    message.success(`已下发删除 ${payload.rules.length} 条路由规则的任务，执行结果请查看 x-ui 操作记录`)
    // Queueing is not completion. Poll the existing action-result refresh flow
    // even if refreshing the operation list temporarily fails.
    scheduleXUIActionResultRefresh(agentID)
    void loadXUIActions(agentID, { silent: true })
    return action
  }

  async function updateRoutingRule(payload: RoutingRuleUpdatePayload, agentID = selectedAgentId): Promise<XUIAction | null> {
    if (!agentID || routingUpdateSubmitting.current) return null
    routingUpdateSubmitting.current = true
    setRoutingRuleUpdateLoadingAgentId(agentID)
    let action: XUIAction
    try {
      action = await fetchJSON<XUIAction>(`/api/v1/agents/${agentID}/xui/actions`, {
        method: 'POST', body: JSON.stringify({ kind: 'update_routing_rule', payload }),
      })
    } catch (error) {
      handleUnauthorized(error)
      // The editor keeps the draft and shows the error next to its fields.
      throw error
    } finally {
      routingUpdateSubmitting.current = false
      setRoutingRuleUpdateLoadingAgentId('')
    }
    scheduleXUIActionResultRefresh(agentID)
    void loadXUIActions(agentID, { silent: true })
    return action
  }

  async function setXUIClientEnabled(record: XUIClientView, enabled: boolean, agentID = selectedAgentId) {
    const targetAgentID = record.realm_target_agent_id || agentID
    const targetInboundID = record.realm_target_inbound_id || record.inbound_id
    const targetInboundTag = record.realm_target_inbound_tag || record.inbound_tag || ''
    if (!targetAgentID || xuiClientToggleSubmitting.current) {
      return
    }
    const key = xuiClientActionKey(record)
    xuiClientToggleSubmitting.current = true
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
            client_id: record.client_id || record.auth_uuid || record.auth_password || '',
            enabled,
            restart: false,
          },
        }),
      })
      message.success(action.status === 'running' ? `已通过 WS 下发${enabled ? '启用' : '停用'} Client 任务，结果会回传到操作记录` : `已创建${enabled ? '启用' : '停用'} Client 任务，Client 不在线时会等待轮询执行`)
      scheduleXUIActionResultRefresh(targetAgentID)
      await loadXUIActions(targetAgentID, { silent: true })
      // Keep the button busy through the first read-back so stale row data
      // cannot immediately dispatch the same state change a second time.
      await new Promise<void>(resolve => window.setTimeout(resolve, 2500))
      await refreshXUIActionResult(targetAgentID, { relatedAgentIDs: [selectedAgentId], agents: true })
    } catch (error) {
      handleUnauthorized(error)
      message.error(error instanceof Error ? error.message : `${enabled ? '启用' : '停用'} Client 失败`)
    } finally {
      xuiClientToggleSubmitting.current = false
      setXUIClientToggleLoadingKey('')
    }
  }

  async function manualResetXUIClientTraffic(record: XUIClientView, agentID = selectedAgentId) {
    if (xuiClientResetSubmitting.current) return
    xuiClientResetSubmitting.current = true
    setXUIClientResetLoadingKey(xuiClientActionKey(record))
    let submitted = false
    let targetAgentID = ''
    try {
      const target = buildManualClientTrafficResetAction(record, agentID)
      targetAgentID = target.agentID
      const submittedAction = await fetchJSON<XUIAction>(`/api/v1/agents/${targetAgentID}/xui/actions`, {
        method: 'POST', body: JSON.stringify(target.request),
      })
      submitted = true
      message.info(`已提交「${record.email}」的流量重置，正在等待执行结果`)
      // Wait on the shared receipt stream; never resend the destructive request.
      const action = await waitForXUIAction(targetAgentID, submittedAction)
      if (!action) { targetAgentID = ''; return }
      if (action.status === 'succeeded' && action.result?.verified === true && action.result?.reset_done === true) {
        message.success(`「${record.email}」的已用流量已重置并核验`)
      } else if (action.status === 'failed') {
        message.error(action.error || '重置未完成，请查看 x-ui 操作记录')
      } else {
        message.warning('重置尚未确认完成，请在 x-ui 操作记录查看结果，勿重复提交')
      }
    } catch (error) {
      handleUnauthorized(error)
      message.error(submitted ? '任务已提交，但暂时无法读取结果，请查看 x-ui 操作记录，勿重复提交' : error instanceof Error ? error.message : '提交流量重置失败，请先查看操作记录')
    } finally {
      if (targetAgentID) {
        await Promise.allSettled([
          loadXUIActions(targetAgentID, { silent: true }),
          loadOverview(targetAgentID, { silent: true }),
          ...(agentID && targetAgentID !== agentID ? [loadOverview(agentID, { silent: true })] : []),
        ])
        scheduleXUIActionResultRefresh(targetAgentID, { relatedAgentIDs: [agentID] })
      }
      xuiClientResetSubmitting.current = false
      setXUIClientResetLoadingKey('')
    }
  }

  async function deleteOutbounds(payload: OutboundsDeletePayload, agentID = selectedAgentId): Promise<XUIAction | null> {
    if (!agentID || outboundDeleteSubmitting.current) return null
    outboundDeleteSubmitting.current = true
    setOutboundDeleteLoadingAgentId(agentID)
    let action: XUIAction
    try {
      action = await fetchJSON<XUIAction>(`/api/v1/agents/${agentID}/xui/actions`, {
        method: 'POST', body: JSON.stringify({ kind: 'delete_outbounds', payload }),
      })
    } catch (error) {
      handleUnauthorized(error)
      message.error(error instanceof Error ? error.message : '下发删除出站任务失败')
      return null
    } finally {
      outboundDeleteSubmitting.current = false
      setOutboundDeleteLoadingAgentId('')
    }
    message.success(`已下发删除 ${payload.outbounds.length} 个出站的任务，执行结果请查看 x-ui 操作记录`)
    scheduleXUIActionResultRefresh(agentID)
    void loadXUIActions(agentID, { silent: true })
    return action
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
      scheduleXUIActionResultRefresh(agentID, { overview: false, delays: [2500] })
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
    xuiClientResetLoadingKey,
    routingRuleDeleteLoadingAgentId,
    routingRuleUpdateLoadingAgentId,
    outboundDeleteLoadingAgentId,
    xuiRestartLoading,
    deleteXUIClient,
    deleteRoutingRules,
    updateRoutingRule,
    deleteOutbounds,
    executeRemoteCommand,
    restartXUIService,
    setXUIClientEnabled,
    manualResetXUIClientTraffic,
  }
}

function xuiClientActionKey(record: XUIClientView): string {
  return [record.inbound_id, record.inbound_tag || '', record.client_id || record.auth_uuid || record.auth_password || '', record.email || ''].join(':')
}
