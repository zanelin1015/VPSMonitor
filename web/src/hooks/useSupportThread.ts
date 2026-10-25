import { useCallback, useEffect, useRef, useState } from 'react'
import type { SupportMessage, SupportThreadResponse } from '../types'
import { fetchJSON } from '../lib/appHelpers'

export type SupportIdentity = 'customer' | 'guest'

export function supportURL(identity: SupportIdentity, scopeToken?: string, path = '', markRead?: boolean): string {
  const query = new URLSearchParams()
  if (markRead !== undefined) query.set('mark_read', markRead ? '1' : '0')
  if (identity === 'guest' && scopeToken) query.set('area', scopeToken)
  const suffix = query.toString()
  return `/api/v1/${identity === 'guest' ? 'public' : 'customer'}/support${path}${suffix ? `?${suffix}` : ''}`
}

export function useSupportThread(identity: SupportIdentity, scopeToken?: string) {
  const [open, setOpen] = useState(false)
  const [thread, setThread] = useState<SupportThreadResponse | null>(null)
  const [draft, setDraft] = useState('')
  const [loading, setLoading] = useState(true)
  const [sending, setSending] = useState(false)
  const [error, setError] = useState('')
  const requestIDRef = useRef(0)
  const sendingRef = useRef(false)
  const mountedRef = useRef(true)

  useEffect(() => {
    mountedRef.current = true
    return () => { mountedRef.current = false; requestIDRef.current += 1 }
  }, [])

  const refresh = useCallback(async (markRead: boolean) => {
    const requestID = ++requestIDRef.current
    try {
      const data = await fetchJSON<SupportThreadResponse>(supportURL(identity, scopeToken, '', markRead))
      if (!mountedRef.current || requestID !== requestIDRef.current) return
      setThread(data)
      setError('')
    } catch (requestError) {
      if (!mountedRef.current || requestID !== requestIDRef.current) return
      setError(requestError instanceof Error ? requestError.message : '客服连接失败')
    } finally {
      if (mountedRef.current && requestID === requestIDRef.current) setLoading(false)
    }
  }, [identity, scopeToken])

  useEffect(() => {
    let stopped = false
    let timer: number | undefined
    const poll = async () => {
      await refresh(open)
      if (!stopped) timer = window.setTimeout(() => void poll(), open ? 2000 : 15000)
    }
    void poll()
    return () => {
      stopped = true
      requestIDRef.current += 1
      if (timer !== undefined) window.clearTimeout(timer)
    }
  }, [open, refresh])

  async function sendMessage() {
    const body = draft.trim()
    if (!body || sendingRef.current) return
    sendingRef.current = true
    setSending(true)
    try {
      await fetchJSON<SupportMessage>(supportURL(identity, scopeToken, '/messages'), {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ body }),
      })
      if (!mountedRef.current) return
      setDraft('')
      await refresh(true)
    } catch (requestError) {
      if (mountedRef.current) setError(requestError instanceof Error ? requestError.message : '消息发送失败')
    } finally {
      sendingRef.current = false
      if (mountedRef.current) setSending(false)
    }
  }

  return { open, setOpen, thread, draft, setDraft, loading, sending, error, sendMessage }
}
