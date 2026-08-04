import { useEffect, useMemo, useRef, useState } from 'react'
import { Badge, Button, Empty, Input, Segmented, Spin, Tag, Tooltip, Typography } from 'antd'
import { CheckCircleOutlined, CloseCircleOutlined, CustomerServiceOutlined, ReloadOutlined, SendOutlined } from '@ant-design/icons'

import type {
  SupportConversation,
  SupportConversationListResponse,
  SupportConversationStatus,
  SupportMessage,
  SupportThreadResponse,
} from '../types'
import { fetchJSON, formatDateTime } from '../lib/appHelpers'

const { Text, Title } = Typography

interface AdminSupportPageProps {
  onUnreadCountChange?: (count: number) => void
}

export function AdminSupportPage({ onUnreadCountChange }: AdminSupportPageProps) {
  const [conversations, setConversations] = useState<SupportConversation[]>([])
  const [selectedID, setSelectedID] = useState<number>(() => Number(new URLSearchParams(window.location.search).get('conversation')) || 0)
  const [thread, setThread] = useState<SupportThreadResponse | null>(null)
  const [listLoading, setListLoading] = useState(true)
  const [threadLoading, setThreadLoading] = useState(false)
  const [sending, setSending] = useState(false)
  const [statusSaving, setStatusSaving] = useState(false)
  const [draft, setDraft] = useState('')
  const [search, setSearch] = useState('')
  const [filter, setFilter] = useState<'open' | 'all'>('open')
  const [error, setError] = useState('')
  const messageListRef = useRef<HTMLDivElement | null>(null)
  const listRequestIDRef = useRef(0)
  const threadRequestIDRef = useRef(0)

  async function loadConversations(silent = false) {
    const requestID = ++listRequestIDRef.current
    if (!silent) setListLoading(true)
    try {
      const data = await fetchJSON<SupportConversationListResponse>('/api/v1/admin/support')
      if (requestID !== listRequestIDRef.current) return
      setConversations(data.conversations || [])
      onUnreadCountChange?.(data.unread_count || 0)
      setSelectedID((current) => {
        if (current && data.conversations.some((item) => item.id === current)) return current
        return data.conversations.find((item) => item.status === 'open')?.id || data.conversations[0]?.id || 0
      })
      setError('')
    } catch (requestError) {
      if (requestID !== listRequestIDRef.current) return
      setError(requestError instanceof Error ? requestError.message : '客服会话加载失败')
    } finally {
      if (!silent && requestID === listRequestIDRef.current) setListLoading(false)
    }
  }

  async function loadThread(conversationID: number, silent = false) {
    if (!conversationID) {
      setThread(null)
      return
    }
    const requestID = ++threadRequestIDRef.current
    if (!silent) setThreadLoading(true)
    try {
      const data = await fetchJSON<SupportThreadResponse>(`/api/v1/admin/support/conversations/${conversationID}`)
      if (requestID !== threadRequestIDRef.current) return
      setThread(data)
      setConversations((current) => current.map((item) => item.id === conversationID ? { ...data.conversation, unread_count: 0 } : item))
      setError('')
    } catch (requestError) {
      if (requestID !== threadRequestIDRef.current) return
      setError(requestError instanceof Error ? requestError.message : '会话内容加载失败')
    } finally {
      if (!silent && requestID === threadRequestIDRef.current) setThreadLoading(false)
    }
  }

  useEffect(() => {
    let stopped = false
    let timer: number | undefined
    const poll = async (initial = false) => {
      await loadConversations(!initial)
      if (!stopped) timer = window.setTimeout(() => void poll(), 2500)
    }
    void poll(true)
    return () => {
      stopped = true
      listRequestIDRef.current += 1
      if (timer !== undefined) window.clearTimeout(timer)
    }
  }, [])

  useEffect(() => {
    if (!selectedID) {
      setThread(null)
      return
    }
    let stopped = false
    let timer: number | undefined
    const poll = async (initial = false) => {
      await loadThread(selectedID, !initial)
      if (!stopped) timer = window.setTimeout(() => void poll(), 2000)
    }
    void poll(true)
    return () => {
      stopped = true
      threadRequestIDRef.current += 1
      if (timer !== undefined) window.clearTimeout(timer)
    }
  }, [selectedID])

  useEffect(() => {
    const element = messageListRef.current
    if (element) element.scrollTop = element.scrollHeight
  }, [thread?.messages.length, lastMessageID(thread?.messages)])

  async function sendMessage() {
    const body = draft.trim()
    if (!selectedID || !body || sending) return
    setSending(true)
    try {
      await fetchJSON<SupportMessage>(`/api/v1/admin/support/conversations/${selectedID}/messages`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ body }),
      })
      setDraft('')
      await Promise.all([loadThread(selectedID, true), loadConversations(true)])
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : '回复发送失败')
    } finally {
      setSending(false)
    }
  }

  async function setConversationStatus(status: SupportConversationStatus) {
    if (!selectedID || statusSaving) return
    setStatusSaving(true)
    try {
      await fetchJSON<SupportConversation>(`/api/v1/admin/support/conversations/${selectedID}/status`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ status }),
      })
      await Promise.all([loadThread(selectedID, true), loadConversations(true)])
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : '会话状态更新失败')
    } finally {
      setStatusSaving(false)
    }
  }

  function selectConversation(id: number) {
    setSelectedID(id)
    const url = new URL(window.location.href)
    url.searchParams.set('page', 'support')
    url.searchParams.set('conversation', String(id))
    window.history.replaceState({}, '', `${url.pathname}?${url.searchParams.toString()}`)
  }

  const filtered = useMemo(() => {
    const keyword = search.trim().toLowerCase()
    return conversations.filter((item) => {
      if (filter === 'open' && item.status !== 'open') return false
      if (!keyword) return true
      return [item.customer_display_name, item.customer_username, item.last_message_preview]
        .some((value) => String(value || '').toLowerCase().includes(keyword))
    })
  }, [conversations, filter, search])

  const unreadCount = conversations.reduce((total, item) => total + item.unread_count, 0)

  return (
    <main className="admin-content-page support-admin-page">
      <header className="support-admin-toolbar">
        <div>
          <div className="eyebrow">Customer Support</div>
          <Title level={2}>在线客服</Title>
        </div>
        <div className="support-admin-toolbar-actions">
          <Badge count={unreadCount} overflowCount={99}>
            <Tag icon={<CustomerServiceOutlined />} color="blue">待处理消息</Tag>
          </Badge>
          <Tooltip title="刷新">
            <Button icon={<ReloadOutlined />} loading={listLoading} onClick={() => void loadConversations()} />
          </Tooltip>
        </div>
      </header>

      {error ? <div className="support-admin-error"><Text type="danger">{error}</Text></div> : null}

      <div className="support-admin-layout">
        <aside className="support-conversation-pane">
          <div className="support-conversation-filters">
            <Input.Search value={search} allowClear placeholder="搜索用户或消息" onChange={(event) => setSearch(event.target.value)} />
            <Segmented
              block
              value={filter}
              options={[{ value: 'open', label: '进行中' }, { value: 'all', label: '全部' }]}
              onChange={(value) => setFilter(value as 'open' | 'all')}
            />
          </div>
          <div className="support-conversation-list">
            {listLoading && conversations.length === 0 ? <Spin /> : null}
            {!listLoading && filtered.length === 0 ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无客服会话" /> : null}
            {filtered.map((conversation) => (
              <button
                type="button"
                key={conversation.id}
                className={`support-conversation-item${selectedID === conversation.id ? ' active' : ''}`}
                onClick={() => selectConversation(conversation.id)}
              >
                <span className="support-conversation-avatar">{supportInitial(conversation)}</span>
                <span className="support-conversation-copy">
                  <span className="support-conversation-title-row">
                    <strong>{conversation.customer_display_name || conversation.customer_username}</strong>
                    <time>{conversation.last_message_at ? compactTime(conversation.last_message_at) : ''}</time>
                  </span>
                  <span className="support-conversation-preview">{conversation.last_message_preview || '尚未发送消息'}</span>
                  <span className="support-conversation-meta">
                    <small>@{conversation.customer_username}</small>
                    {conversation.status === 'closed' ? <Tag>已结束</Tag> : null}
                    {conversation.unread_count > 0 ? <Badge count={conversation.unread_count} overflowCount={99} /> : null}
                  </span>
                </span>
              </button>
            ))}
          </div>
        </aside>

        <section className="support-thread-pane">
          {!selectedID ? <Empty description="选择一个客服会话" /> : null}
          {selectedID && threadLoading && thread?.conversation.id !== selectedID ? <Spin /> : null}
          {thread?.conversation.id === selectedID ? (
            <>
              <header className="support-thread-header">
                <div>
                  <strong>{thread.conversation.customer_display_name || thread.conversation.customer_username}</strong>
                  <small>@{thread.conversation.customer_username}</small>
                </div>
                {thread.conversation.status === 'open' ? (
                  <Button
                    icon={<CheckCircleOutlined />}
                    loading={statusSaving}
                    onClick={() => void setConversationStatus('closed')}
                  >结束会话</Button>
                ) : (
                  <Button
                    icon={<CloseCircleOutlined />}
                    loading={statusSaving}
                    onClick={() => void setConversationStatus('open')}
                  >重新打开</Button>
                )}
              </header>
              <div ref={messageListRef} className="support-thread-messages">
                {!thread.messages.length ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="等待用户发送消息" /> : null}
                {thread.messages.map((message) => <AdminSupportMessage key={message.id} message={message} />)}
              </div>
              <footer className="support-thread-composer">
                <Input.TextArea
                  value={draft}
                  maxLength={2000}
                  autoSize={{ minRows: 2, maxRows: 5 }}
                  placeholder="输入回复，Enter 发送，Shift + Enter 换行"
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
              </footer>
            </>
          ) : null}
        </section>
      </div>
    </main>
  )
}

function AdminSupportMessage({ message }: { message: SupportMessage }) {
  const fromCustomer = message.sender_role === 'customer'
  return (
    <div className={`support-thread-message${fromCustomer ? ' customer' : ' operator'}`}>
      <div className="support-thread-message-meta">
        <span>{fromCustomer ? message.sender_name || 'Customer' : message.sender_name || '客服'}</span>
        <time>{formatDateTime(message.created_at)}</time>
      </div>
      <p>{message.body}</p>
    </div>
  )
}

function lastMessageID(messages?: SupportMessage[]): number {
  return messages?.length ? messages[messages.length - 1].id : 0
}

function supportInitial(conversation: SupportConversation): string {
  const value = (conversation.customer_display_name || conversation.customer_username || 'C').trim()
  return Array.from(value)[0]?.toUpperCase() || 'C'
}

function compactTime(value: string): string {
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) return ''
  const now = new Date()
  if (parsed.toDateString() === now.toDateString()) {
    return parsed.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
  }
  return parsed.toLocaleDateString('zh-CN', { month: '2-digit', day: '2-digit' })
}
