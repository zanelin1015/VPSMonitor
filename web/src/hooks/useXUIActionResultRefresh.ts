import { useCallback, useEffect, useRef } from 'react'
import type { XUIAction } from '../types'

export interface ActionRefreshOptions {
  relatedAgentIDs?: string[]
  agents?: boolean
  logs?: boolean
  overview?: boolean
  delays?: readonly number[]
}

const defaultDelays = [500, 1000, 2000, 4000, 8000, 15000]
const isPending = (action: XUIAction) => action.status === 'pending' || action.status === 'running'
const isRoutingAction = (action: XUIAction) => ['delete_routing_rules', 'update_routing_rule', 'delete_outbounds'].includes(action.kind)
type LoadAgentResource = (agentID: string, options?: { silent?: boolean }) => Promise<void>
interface Resources {
  loadXUIActions: (agentID: string, options?: { silent?: boolean }) => Promise<XUIAction[] | undefined>
  loadOverview: LoadAgentResource
  loadAgents?: (options?: { silent?: boolean }) => Promise<void>
  loadAgentLogs?: LoadAgentResource
  observedAgentID?: string
  actions?: XUIAction[]
}
interface Waiter {
  action: XUIAction
  deadline: number
  resolve: (action: XUIAction | null) => void
}
interface RefreshEntry {
  timer?: number
  running: boolean
  due: number[]
  options: ActionRefreshOptions
  relatedAt: number
  relatedRead: boolean
  followPending: boolean
  pending: boolean
  waiters: Set<Waiter>
}

/** Scheduled read-backs, pending routes and reset receipts share one timer per agent. */
export function useXUIActionResultRefresh(resources: Resources) {
  const resourcesRef = useRef(resources)
  resourcesRef.current = resources
  const pendingRef = useRef(new Map<string, RefreshEntry>())
  const mountedRef = useRef(true)

  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
      for (const entry of pendingRef.current.values()) {
        window.clearTimeout(entry.timer)
        entry.waiters.forEach(waiter => waiter.resolve(null))
      }
      pendingRef.current.clear()
    }
  }, [])

  const readResult = useCallback(async (agentID: string, options: ActionRefreshOptions) => {
    const loaders = resourcesRef.current
    const ids = options.overview === false ? [] : [...new Set([agentID, ...(options.relatedAgentIDs || [])].filter(Boolean))]
    const [receipts] = await Promise.allSettled([
      loaders.loadXUIActions(agentID, { silent: true }),
      ...ids.map(id => loaders.loadOverview(id, { silent: true })),
      ...(options.agents && loaders.loadAgents ? [loaders.loadAgents({ silent: true })] : []),
      ...(options.logs && loaders.loadAgentLogs ? [loaders.loadAgentLogs(agentID, { silent: true })] : []),
    ])
    return receipts.status === 'fulfilled' ? receipts.value : undefined
  }, [])

  const refreshXUIActionResult = useCallback(async (agentID: string, options: ActionRefreshOptions = {}) => {
    if (mountedRef.current) await readResult(agentID, options)
  }, [readResult])

  function arm(agentID: string, entry: RefreshEntry) {
    if (!mountedRef.current || entry.running || pendingRef.current.get(agentID) !== entry) return
    window.clearTimeout(entry.timer)
    const now = Date.now()
    const next = Math.min(
      entry.due[0] ?? Infinity,
      entry.followPending && entry.pending ? now + 3000 : Infinity,
      entry.waiters.size ? Math.min(now + 2000, ...[...entry.waiters].map(waiter => waiter.deadline)) : Infinity,
    )
    if (!Number.isFinite(next)) {
      pendingRef.current.delete(agentID)
      return
    }
    entry.timer = window.setTimeout(() => void run(agentID, entry), Math.max(0, next - now))
  }

  async function run(agentID: string, entry: RefreshEntry) {
    entry.running = true
    const now = Date.now()
    entry.due = entry.due.filter(at => at > now)
    const includeRelated = !entry.relatedRead && now >= entry.relatedAt
    if (includeRelated) entry.relatedRead = true
    const actions = await readResult(agentID, includeRelated ? entry.options : { overview: entry.options.overview })
    if (!mountedRef.current) return
    if (actions) entry.pending = actions.some(action => isRoutingAction(action) && isPending(action))
    for (const waiter of entry.waiters) {
      waiter.action = actions?.find(action => action.id === waiter.action.id) || waiter.action
      if (!isPending(waiter.action) || Date.now() >= waiter.deadline) {
        entry.waiters.delete(waiter)
        waiter.resolve(waiter.action)
      }
    }
    entry.running = false
    arm(agentID, entry)
  }

  const scheduleXUIActionResultRefresh = useCallback((agentID: string, options: ActionRefreshOptions = {}) => {
    if (!agentID || !mountedRef.current) return
    const pending = pendingRef.current.get(agentID)
    if (pending) options = {
      agents: pending.options.agents || options.agents,
      logs: pending.options.logs || options.logs,
      overview: pending.options.overview !== false || options.overview !== false,
      delays: [...new Set([...(pending.options.delays || defaultDelays), ...(options.delays || defaultDelays)])].sort((a, b) => a - b),
      relatedAgentIDs: [...new Set([...(pending.options.relatedAgentIDs || []), ...(options.relatedAgentIDs || [])])],
    }
    const delays = options.delays?.length ? [...options.delays].sort((a, b) => a - b) : defaultDelays
    const now = Date.now()
    const entry: RefreshEntry = pending || { running: false, due: [], options, relatedAt: now, relatedRead: false, followPending: false, pending: false, waiters: new Set() }
    entry.options = options
    entry.due = delays.map(delay => now + delay)
    entry.relatedAt = now + delays[Math.min(2, delays.length - 1)]
    entry.relatedRead = false
    pendingRef.current.set(agentID, entry)
    arm(agentID, entry)
  }, [])

  const waitForXUIAction = useCallback((agentID: string, action: XUIAction): Promise<XUIAction | null> => {
    if (!mountedRef.current) return Promise.resolve(null)
    if (!isPending(action)) return Promise.resolve(action)
    if (!pendingRef.current.has(agentID)) scheduleXUIActionResultRefresh(agentID, { delays: [2000] })
    const entry = pendingRef.current.get(agentID)!
    return new Promise(resolve => {
      entry.waiters.add({ action, deadline: Date.now() + 90_000, resolve })
      arm(agentID, entry)
    })
  }, [scheduleXUIActionResultRefresh])

  const pendingActionIDs = (resources.actions || []).filter(action => action.agent_id === resources.observedAgentID && isRoutingAction(action) && isPending(action)).map(action => action.id).join(',')
  useEffect(() => {
    const agentID = resources.observedAgentID
    if (!agentID || !pendingActionIDs) return
    if (!pendingRef.current.has(agentID)) scheduleXUIActionResultRefresh(agentID, { delays: [3000] })
    const entry = pendingRef.current.get(agentID)!
    entry.followPending = entry.pending = true
    arm(agentID, entry)
    return () => { entry.followPending = false; arm(agentID, entry) }
  }, [resources.observedAgentID, pendingActionIDs, scheduleXUIActionResultRefresh])

  return { scheduleXUIActionResultRefresh, refreshXUIActionResult, waitForXUIAction }
}
