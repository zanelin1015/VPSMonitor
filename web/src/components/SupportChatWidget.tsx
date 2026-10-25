import { useEffect, useRef } from 'react'
import { Badge, Button, Empty, Input, Spin, Tooltip, Typography } from 'antd'
import { CloseOutlined, CustomerServiceOutlined, SendOutlined } from '@ant-design/icons'
import type { SupportMessage } from '../types'
import { formatDateTime } from '../lib/appHelpers'
import { useSupportThread, type SupportIdentity } from '../hooks/useSupportThread'

const labels = {
  guest: { title: '访客在线客服', online: '客服在线 · 未登录临时会话', offline: '客服离线 · 留言后会通知客服' },
  customer: { title: '在线客服', online: '客服在线', offline: '客服离线，消息将通知客服' },
}

export function SupportChatWidget({ identity, scopeToken }: { identity: SupportIdentity; scopeToken?: string }) {
  const { open, setOpen, thread, draft, setDraft, loading, sending, error, sendMessage } = useSupportThread(identity, scopeToken)
  const messageListRef = useRef<HTMLDivElement | null>(null)
  const lastMessageID = thread?.messages[thread.messages.length - 1]?.id || 0
  const online = Boolean(thread?.support_online)
  const copy = labels[identity]

  useEffect(() => {
    const element = messageListRef.current
    if (open && element) element.scrollTop = element.scrollHeight
  }, [open, thread?.messages.length, lastMessageID])

  return (
    <div className={`customer-support${identity === 'guest' ? ' guest-support' : ''}${open ? ' customer-support-open' : ''}`}>
      {open ? (
        <section className="customer-support-panel" aria-label={copy.title}>
          <header className="customer-support-header">
            <div className="customer-support-heading">
              <span className={`customer-support-status${online ? ' online' : ''}`} />
              <div><strong>{copy.title}</strong><small>{online ? copy.online : copy.offline}</small></div>
            </div>
            <Tooltip title="关闭"><Button type="text" shape="circle" icon={<CloseOutlined />} onClick={() => setOpen(false)} /></Tooltip>
          </header>
          <div ref={messageListRef} className="customer-support-messages">
            {loading && !thread ? <Spin /> : null}
            {!loading && !thread?.messages.length ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="发送消息联系在线客服" /> : null}
            {(thread?.messages || []).map((message) => <SupportMessageBubble key={message.id} message={message} />)}
          </div>
          <footer className="customer-support-composer">
            {error ? <Typography.Text type="danger" className="customer-support-error">{error}</Typography.Text> : null}
            <div className="customer-support-compose-row">
              <Input.TextArea value={draft} maxLength={2000} autoSize={{ minRows: 2, maxRows: 4 }} placeholder="请输入消息"
                onChange={(event) => setDraft(event.target.value)} onPressEnter={(event) => {
                  if (!event.shiftKey) { event.preventDefault(); void sendMessage() }
                }} />
              <Tooltip title="发送"><Button type="primary" shape="circle" icon={<SendOutlined />} loading={sending} disabled={!draft.trim()} onClick={() => void sendMessage()} /></Tooltip>
            </div>
          </footer>
        </section>
      ) : (
        <Tooltip title={copy.title} placement="left">
          <Badge count={thread?.conversation.unread_count || 0} overflowCount={99}>
            <button type="button" className="customer-support-launcher" aria-label={`打开${copy.title}`} onClick={() => setOpen(true)}><CustomerServiceOutlined /></button>
          </Badge>
        </Tooltip>
      )}
    </div>
  )
}

function SupportMessageBubble({ message }: { message: SupportMessage }) {
  const mine = message.sender_role === 'customer'
  return (
    <div className={`customer-support-message${mine ? ' mine' : ''}`}>
      <div className="customer-support-message-meta"><span>{mine ? '我' : message.sender_name || '客服'}</span><time>{formatDateTime(message.created_at)}</time></div>
      <p>{message.body}</p>
    </div>
  )
}
