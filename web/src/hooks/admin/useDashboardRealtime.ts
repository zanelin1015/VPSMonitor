import { useEffect, useRef } from 'react'
import { buildDashboardRealtimeURL } from '../../lib/api'
import type { AdminUser, AgentRealtimeMetrics, DashboardRealtimeMessage } from '../../types'

/** Keep a single stream per authenticated account; polling remains the fallback. */
export function useDashboardRealtime(
  adminUser: AdminUser | null,
  onMetrics: (metrics: AgentRealtimeMetrics[]) => void,
) {
  const onMetricsRef = useRef(onMetrics)
  useEffect(() => {
    onMetricsRef.current = onMetrics
  }, [onMetrics])
  const identity = adminUser ? JSON.stringify([adminUser.id, adminUser.username, adminUser.role]) : ''

  useEffect(() => {
    if (!identity) return
    let disposed = false
    let socket: WebSocket | null = null
    let timer: number | undefined
    let failures = 0

    const retry = () => {
      if (disposed) return
      // Repeated upstream resets must not create an endless rapid reconnect loop.
      const delay = Math.min(3000 * 2 ** Math.min(failures++, 4), 30000)
      timer = window.setTimeout(connect, delay)
    }
    const connect = () => {
      if (disposed) return
      let connection: WebSocket
      try {
        connection = new WebSocket(buildDashboardRealtimeURL())
      } catch {
        retry()
        return
      }
      socket = connection
      connection.onmessage = (event) => {
        if (disposed || socket !== connection) return
        try {
          const data = JSON.parse(event.data) as DashboardRealtimeMessage
          const metrics = data.type === 'snapshot' ? data.metrics || [] : data.metric ? [data.metric] : []
          failures = 0
          if (metrics.length) onMetricsRef.current(metrics)
        } catch {
          // Ignore malformed frames; the HTTP polling path remains available.
        }
      }
      connection.onerror = () => {
        if (!disposed && socket === connection) connection.close()
      }
      connection.onclose = () => {
        if (!disposed && socket === connection) {
          socket = null
          retry()
        }
      }
    }

    // StrictMode mounts/cleans effects twice. Defer the handshake so its probe
    // does not immediately abort a CONNECTING socket through the dev proxy.
    timer = window.setTimeout(connect, 0)
    return () => {
      disposed = true
      if (timer !== undefined) window.clearTimeout(timer)
      if (!socket) return
      socket.onmessage = null
      socket.onerror = null
      socket.onclose = null
      if (socket.readyState === WebSocket.CONNECTING) {
        // Let an in-flight upgrade finish before closing it cleanly.
        socket.onopen = () => socket?.close(1000, 'Account changed or page closed')
      } else if (socket.readyState === WebSocket.OPEN) {
        socket.close(1000, 'Account changed or page closed')
      }
    }
  }, [identity])
}
