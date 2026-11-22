import type { RealmTrafficUsageResponse } from '../types'
import { useAgentTrafficUsage } from './useAgentTrafficUsage'

export function useRealmTrafficUsage(agentID: string, from: string, to: string) {
  return useAgentTrafficUsage<RealmTrafficUsageResponse>(agentID, 'realm/traffic', from, to)
}
