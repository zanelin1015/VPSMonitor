import type { MessageInstance } from 'antd/es/message/interface'
import type { Dispatch, SetStateAction } from 'react'
import { useEffect, useState } from 'react'
import type { XUIAddClientActionForm, XUIOutboundActionForm, XUIRoutingActionForm } from '../../lib/appHelpers'
import {
  buildXUIActionPayload,
  defaultAddClientActionForm,
  defaultOutboundActionForm,
  defaultRoutingActionForm,
  fetchJSON,
  isUnauthorized,
  normalizeXUIOverview,
} from '../../lib/appHelpers'
import { nodeImportURL, readNodeImportTarget, saveNodeImportTarget } from '../../lib/nodeImport'
import type { AdminUser, XUIAction, XUIClientView, XUINodeView, XUIOverview } from '../../types'

interface LoadOptions {
  silent?: boolean
}

interface Options {
  adminUser: AdminUser | null
  currentOverview: XUIOverview | null
  selectedAgentId: string
  message: MessageInstance
  loadXUIActions: (agentID?: string, options?: LoadOptions) => Promise<void>
  setAdminUser: Dispatch<SetStateAction<AdminUser | null>>
}

export function useXUIActionComposer({
  adminUser,
  currentOverview,
  selectedAgentId,
  message,
  loadXUIActions,
  setAdminUser,
}: Options) {
  const [xuiActionModalOpen, setXUIActionModalOpen] = useState(false)

  const [xuiActionSaving, setXUIActionSaving] = useState(false)

  const [xuiActionKind, setXUIActionKind] = useState('upsert_routing_rule')

  const [xuiActionAgentId, setXUIActionAgentId] = useState('')

  const [addClientActionInbounds, setAddClientActionInbounds] = useState<XUINodeView[]>([])

  const [addClientActionForm, setAddClientActionForm] = useState<XUIAddClientActionForm>(() =>
    defaultAddClientActionForm(),
  )

  const [outboundActionForm, setOutboundActionForm] = useState<XUIOutboundActionForm>(() => defaultOutboundActionForm())

  const [routingActionForm, setRoutingActionForm] = useState<XUIRoutingActionForm>(() => defaultRoutingActionForm())

  const [outboundSourceOverview, setOutboundSourceOverview] = useState<XUIOverview | null>(null)

  const [outboundSourceLoading, setOutboundSourceLoading] = useState(false)

  const [importURLClient, setImportURLClient] = useState<XUIClientView | null>(null)

  const [nodeImportTarget, setNodeImportTarget] = useState(readNodeImportTarget)

  useEffect(() => {
    saveNodeImportTarget(nodeImportTarget)
  }, [nodeImportTarget])

  useEffect(() => {
    if (!adminUser || !outboundActionForm.source_agent_id) {
      setOutboundSourceOverview(null)
      setOutboundSourceLoading(false)
      return
    }
    if (currentOverview && outboundActionForm.source_agent_id === currentOverview.agent_id) {
      setOutboundSourceOverview(currentOverview)
      setOutboundSourceLoading(false)
      return
    }

    let cancelled = false
    setOutboundSourceLoading(true)
    void fetchJSON<XUIOverview>(`/api/v1/agents/${outboundActionForm.source_agent_id}/xui/overview`)
      .then((data) => {
        if (!cancelled) {
          setOutboundSourceOverview(normalizeXUIOverview(data))
        }
      })
      .catch(() => {
        if (!cancelled) {
          setOutboundSourceOverview(null)
        }
      })
      .finally(() => {
        if (!cancelled) {
          setOutboundSourceLoading(false)
        }
      })
    return () => {
      cancelled = true
    }
  }, [adminUser, outboundActionForm.source_agent_id, currentOverview])

  async function createXUIAction() {
    const actionAgentID = xuiActionAgentId || selectedAgentId
    if (!actionAgentID) {
      return
    }
    let payload: Record<string, unknown>
    try {
      payload = buildXUIActionPayload(xuiActionKind, {
        addClient: addClientActionForm,
        outbound: outboundActionForm,
        routing: routingActionForm,
      })
    } catch (error) {
      message.error(error instanceof Error ? error.message : '操作表单不完整')
      return
    }
    setXUIActionSaving(true)
    try {
      const action = await fetchJSON<XUIAction>(`/api/v1/agents/${actionAgentID}/xui/actions`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ kind: xuiActionKind, payload }),
      })
      setXUIActionModalOpen(false)
      message.success(
        action.status === 'running'
          ? 'x-ui 操作已通过 WS 下发，正在等待实时回传结果'
          : 'x-ui 操作已创建，Client 不在线时会等待轮询执行',
      )
      await loadXUIActions(actionAgentID)
      scheduleXUIActionResultRefresh(actionAgentID)
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      message.error(error instanceof Error ? error.message : '创建 x-ui 操作失败')
    } finally {
      setXUIActionSaving(false)
    }
  }

  function scheduleXUIActionResultRefresh(agentID: string) {
    ;[500, 1000, 2000, 4000, 8000, 15000].forEach((delay) => {
      window.setTimeout(() => {
        void loadXUIActions(agentID, { silent: true })
      }, delay)
    })
  }

  async function copyImportURL(client: XUIClientView) {
    const importURL = nodeImportURL(client, nodeImportTarget)
    if (!importURL) {
      message.warning('当前客户端暂不支持生成单节点导入 URL')
      return
    }
    try {
      await navigator.clipboard.writeText(importURL)
      message.success('导入 URL 已复制')
    } catch {
      setImportURLClient(client)
      message.warning('浏览器不允许直接复制，已打开 URL 弹窗')
    }
  }

  return {
    xuiActionModalOpen,
    setXUIActionModalOpen,
    xuiActionSaving,
    setXUIActionSaving,
    xuiActionKind,
    setXUIActionKind,
    xuiActionAgentId,
    setXUIActionAgentId,
    addClientActionInbounds,
    setAddClientActionInbounds,
    addClientActionForm,
    setAddClientActionForm,
    outboundActionForm,
    setOutboundActionForm,
    routingActionForm,
    setRoutingActionForm,
    outboundSourceOverview,
    setOutboundSourceOverview,
    outboundSourceLoading,
    setOutboundSourceLoading,
    importURLClient,
    setImportURLClient,
    nodeImportTarget,
    setNodeImportTarget,
    createXUIAction,
    scheduleXUIActionResultRefresh,
    copyImportURL,
  }
}
