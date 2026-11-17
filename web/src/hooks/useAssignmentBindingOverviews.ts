import { useEffect, useState } from 'react'
import type { CustomerAssignment, DashboardAgentView, XUIOverview } from '../types'
import { fetchJSON } from '../lib/appHelpers'
import { isForwardingAssignment, type AssignmentBindingOverviews } from '../lib/customerAssignmentBinding'

const noAssignments: CustomerAssignment[] = []

export function useAssignmentBindingOverviews(
  assignments: CustomerAssignment[] | null | undefined,
  agents: DashboardAgentView[],
  active: boolean,
): AssignmentBindingOverviews {
  // Older Servers serialize users with no grants as assignments: null.
  // A parameter default only handles undefined, so normalize both here.
  const records = assignments ?? noAssignments
  const agentKey = JSON.stringify([...new Set(records
    .filter(record => isForwardingAssignment(record, agents))
    .map(record => record.agent_id))].sort())
  const [result, setResult] = useState<{ key: string; entries: AssignmentBindingOverviews }>({ key: '', entries: {} })

  useEffect(() => {
    if (!active) return
    const agentIDs: string[] = JSON.parse(agentKey)
    let cancelled = false
    setResult({ key: agentKey, entries: Object.fromEntries(agentIDs.map(id => [id, { status: 'loading' }])) })
    for (const id of agentIDs) {
      void fetchJSON<XUIOverview>(`/api/v1/agents/${encodeURIComponent(id)}/xui/overview?assignment_scope=1`)
        .then(overview => {
          if (!cancelled) setResult(previous => ({ key: agentKey, entries: { ...previous.entries, [id]: { status: 'ready', overview } } }))
        })
        .catch(() => {
          if (!cancelled) setResult(previous => ({ key: agentKey, entries: { ...previous.entries, [id]: { status: 'error' } } }))
        })
    }
    return () => { cancelled = true }
  }, [active, agentKey, records])

  return active && result.key === agentKey ? result.entries : {}
}
