import { useEffect, useState } from 'react'
import type { AgentEntryConfig, AgentListItem, ManagedAgentConfig, XUIOverview } from '../types'
import { fetchJSON } from '../lib/api'
import { normalizeXUIOverview } from '../lib/appHelpersConfig'
import { realmForwardTargetPortOptions } from '../lib/managedConfigHelpers'

type EntryChange = (patch: Partial<AgentEntryConfig>) => void

export function useRealmTargetOverviews(
  managedConfig: ManagedAgentConfig | null,
  agents: AgentListItem[],
  onEntryChange: EntryChange,
) {
  const [targetOverviews, setTargetOverviews] = useState<Record<string, XUIOverview>>({})
  const [targetOverviewLoading, setTargetOverviewLoading] = useState<Record<string, boolean>>({})
  const targetAgentIDs = Array.from(new Set(
    (managedConfig?.entry?.port_forwarding?.rules || [])
      .map((rule) => rule.target_agent_id || '')
      .filter(Boolean),
  )).sort()
  const targetAgentKey = targetAgentIDs.join('|')

  useEffect(() => {
    let cancelled = false
    targetAgentIDs.forEach((agentID) => {
      if (targetOverviews[agentID] || targetOverviewLoading[agentID]) {
        return
      }
      setTargetOverviewLoading((current) => ({ ...current, [agentID]: true }))
      void fetchJSON<XUIOverview>(`/api/v1/agents/${encodeURIComponent(agentID)}/xui/overview`)
        .then((overview) => {
          if (!cancelled) {
            setTargetOverviews((current) => ({ ...current, [agentID]: normalizeXUIOverview(overview) }))
          }
        })
        .catch(() => undefined)
        .finally(() => {
          if (!cancelled) {
            setTargetOverviewLoading((current) => ({ ...current, [agentID]: false }))
          }
        })
    })
    return () => { cancelled = true }
  }, [targetAgentKey])

  useEffect(() => {
    const rules = managedConfig?.entry?.port_forwarding?.rules || []
    if (!rules.length) {
      return
    }
    let changed = false
    const nextRules = rules.map((rule) => {
      if (!rule.target_agent_id || rule.target_port_mode === 'custom' || Number(rule.target_port || 0) > 0) {
        return rule
      }
      const agent = agents.find((item) => item.agent_id === rule.target_agent_id)
      const option = realmForwardTargetPortOptions(agent, targetOverviews[rule.target_agent_id])[0]
      if (!option) {
        return rule
      }
      changed = true
      return { ...rule, target_port: option.port, target_port_mode: 'auto' as const }
    })
    if (changed) {
      onEntryChange({ port_forwarding: { ...managedConfig?.entry?.port_forwarding, rules: nextRules } })
    }
  }, [managedConfig?.entry?.port_forwarding?.rules, targetOverviews, agents, onEntryChange])

  return { targetOverviews, targetOverviewLoading }
}
