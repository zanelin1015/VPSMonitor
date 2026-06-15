import { useCallback, useEffect, useRef } from 'react'

export function useCurrentAgentRequest(selectedAgentId: string) {
  const selectedAgentIdRef = useRef(selectedAgentId)

  useEffect(() => {
    selectedAgentIdRef.current = selectedAgentId
  }, [selectedAgentId])

  const isCurrentAgentRequest = useCallback((agentID: string) => {
    return selectedAgentIdRef.current === agentID
  }, [])

  return isCurrentAgentRequest
}
