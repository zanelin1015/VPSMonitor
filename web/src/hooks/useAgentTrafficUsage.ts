import { useEffect, useState } from 'react'
import { fetchJSON } from '../lib/api'

export function useAgentTrafficUsage<T extends { items: unknown[] }>(agentID: string, path: 'realm/traffic' | 'interfaces/traffic', from: string, to: string) {
  const [revision, setRevision] = useState(0)
  const key = JSON.stringify([path, agentID, from, to, revision])
  const [resource, setResource] = useState<{ key: string; data?: T; error?: string; loading: boolean }>({ key: '', loading: true })
  useEffect(() => {
    const controller = new AbortController()
    let active = true
    setResource({ key, loading: true })
    const query = new URLSearchParams({ from, to })
    void fetchJSON<T>(`/api/v1/agents/${encodeURIComponent(agentID)}/${path}?${query}`, { signal: controller.signal })
      .then(data => { if (active) setResource({ key, data: { ...data, items: data.items || [] }, loading: false }) })
      .catch(error => { if (active) setResource({ key, error: error instanceof Error ? error.message : '读取周期流量失败', loading: false }) })
    return () => { active = false; controller.abort() }
  }, [key])
  return {
    data: resource.key === key ? resource.data : undefined,
    error: resource.key === key ? resource.error : undefined,
    loading: resource.key !== key || resource.loading,
    refresh: () => setRevision(current => current + 1),
  }
}
