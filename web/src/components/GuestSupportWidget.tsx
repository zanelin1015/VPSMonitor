import { useEffect, useRef, useState } from 'react'
import { Badge, Button, Empty, Input, Spin, Tooltip, Typography } from 'antd'
import { CloseOutlined, CustomerServiceOutlined, SendOutlined } from '@ant-design/icons'

import type { SupportMessage, SupportThreadResponse } from '../types'
import { fetchJSON, formatDateTime } from '../lib/appHelpers'

const { Text } = Typography

const supportEndpoint = '/api/v1/public/support'

/** Guest-only support widget. The server binds its session to the observed IP. */
export function GuestSupportWidget() {
  const [open, setOpen] = useState(false)
  const [thread, setThread] = useState<SupportThreadResponse | null>(null)
  const [draft, setDraft] = useState('')
  const [loading, setLoading] = useState(true)
  const [sending, setSending] = useState(false)
  const [error, setError] = useState('')
  const messageListRef = useRef<HTMLDivElement | null>(null)
  const requestIDRef = useRef(0)

  async function refresh(markRead: boolean) {
    const requestID = ++requestIDRef.current
    try {
      const data = await fetchJSON<SupportThreadResponse>(`${supportEndpoint}?mark_read=${markRead ? '1' : '0'}`)
      if (requestID !== requestIDRef.current) return
      setThread(data)
      setError('')
    } catch (requestError) {
      if (requestID !== requestIDRef.current) return
      setError(requestError instanceof Error ? requestError.message : '客服连接失败')
    } finally {
      if (requestID === requestIDRef.current) setLoading(false)
    }
  }

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
  }, [open])

  useEffect(() => {
    if (!open) return
    const element = messageListRef.current
    if (element) element.scrollTop = element.scrollHeight
  }, [open, thread?.messages.length, guestLastMessageID(thread?.messages)])

  async function sendMessage() {
    const body = draft.trim()
    if (!body || sending) return
    setSending(true)
    try {
      await fetchJSON<SupportMessage>(`${supportEndpoint}/messages`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ body }),
      })
      setDraft('')
      await refresh(true)
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : '消息发送失败')
    } finally {
      setSending(false)
    }
  }

  const unread = thread?.conversation.unread_count || 0
  const online = Boolean(thread?.support_online)

  return (
    <div className={`customer-support guest-support${open ? ' customer-support-open' : ''}`}>
      {open ? (
        <section className="customer-support-panel" aria-label="访客在线客服">
          <header className="customer-support-header">
            <div className="customer-support-heading">
              <span className={`customer-support-status${online ? ' online' : ''}`} />
              <div>
                <strong>访客在线客服</strong>
                <small>{online ? '客服在线 · 未登录临时会话' : '客服离线 · 留言后会通知客服'}</small>
              </div>
            </div>
            <Tooltip title="关闭">
              <Button type="text" shape="circle" icon={<CloseOutlined />} onClick={() => setOpen(false)} />
            </Tooltip>
          </header>
          <div ref={messageListRef} className="customer-support-messages">
            {loading && !thread ? <Spin /> : null}
            {!loading && !thread?.messages.length ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="发送消息联系在线客服" /> : null}
            {(thread?.messages || []).map((message) => <GuestMessageBubble key={message.id} message={message} />)}
          </div>
          <footer className="customer-support-composer">
            {error ? <Text type="danger" className="customer-support-error">{error}</Text> : null}
            <div className="customer-support-compose-row">
              <Input.TextArea
                value={draft}
                maxLength={2000}
                autoSize={{ minRows: 2, maxRows: 4 }}
                placeholder="请输入消息"
                onChange={(event) => setDraft(event.target.value)}
                onPressEnter={(event) => {
                  if (!event.shiftKey) {
                    event.preventDefault()
                    void sendMessage()
                  }
                }}
              />
              <Tooltip title="发送">
                <Button type="primary" shape="circle" icon={<SendOutlined />} loading={sending} disabled={!draft.trim()} onClick={() => void sendMessage()} />
              </Tooltip>
            </div>
          </footer>
        </section>
      ) : null}
      {!open ? (
        <Tooltip title="访客在线客服" placement="left">
          <Badge count={unread} overflowCount={99}>
            <button type="button" className="customer-support-launcher" aria-label="打开访客在线客服" onClick={() => setOpen(true)}>
              <CustomerServiceOutlined />
            </button>
          </Badge>
        </Tooltip>
      ) : null}
    </div>
  )
}

function GuestMessageBubble({ message }: { message: SupportMessage }) {
  const mine = message.sender_role === 'customer'
  return (
    <div className={`customer-support-message${mine ? ' mine' : ''}`}>
      <div className="customer-support-message-meta">
        <span>{mine ? '我' : message.sender_name || '客服'}</span>
        <time>{formatDateTime(message.created_at)}</time>
      </div>
      <p>{message.body}</p>
    </div>
  )
}

function guestLastMessageID(messages?: SupportMessage[]) {
  return messages?.length ? messages[messages.length - 1].id : 0
}
