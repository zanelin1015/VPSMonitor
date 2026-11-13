import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { Badge, Button, Empty, Input, Modal, Segmented, Select, Spin, Tag, Tooltip, Typography } from 'antd'
import { CheckCircleOutlined, CloseCircleOutlined, CustomerServiceOutlined, MinusOutlined, ReloadOutlined, SendOutlined } from '@ant-design/icons'

import type {
  SupportConversation,
  SupportConversationListResponse,
  SupportConversationStatus,
  SupportMessage,
  SupportThreadResponse,
  CustomerAdminView,
} from '../types'
import { fetchJSON, formatDateTime } from '../lib/appHelpers'
import { useCompactLayout } from '../hooks/useCompactLayout'
import { SupportMessageList, type SupportMessageListHandle } from './SupportMessageList'

const { Text, Title } = Typography

interface AdminSupportPageProps {
  onUnreadCountChange?: (count: number) => void
}

export function AdminSupportPage({ onUnreadCountChange }: AdminSupportPageProps) {
  const compact = useCompactLayout()
  const [conversations, setConversations] = useState<SupportConversation[]>([])
  const [selectedID, setSelectedID] = useState<number>(() => Number(new URLSearchParams(window.location.search).get('conversation')) || 0)
  const [threadOpen, setThreadOpen] = useState(() => selectedID > 0)
  const [thread, setThread] = useState<SupportThreadResponse | null>(null)
  const [listLoading, setListLoading] = useState(true)
  const [threadLoading, setThreadLoading] = useState(false)
  const [sending, setSending] = useState(false)
  const [statusSaving, setStatusSaving] = useState(false)
  const [drafts, setDrafts] = useState<Record<number, string>>({})
  const [search, setSearch] = useState('')
  const [filter, setFilter] = useState<'open' | 'all'>('open')
  const [error, setError] = useState('')
  const [composeOpen, setComposeOpen] = useState(false)
  const [composeLoading, setComposeLoading] = useState(false)
  const [composeError, setComposeError] = useState('')
  const [recipients, setRecipients] = useState<CustomerAdminView[]>([])
  const [recipientID, setRecipientID] = useState<number | undefined>()
  const [composeBody, setComposeBody] = useState('')
  const messageListRef = useRef<SupportMessageListHandle>(null)
  const listRequestIDRef = useRef(0)
  const threadRequestIDRef = useRef(0)
  const activeThreadRef = useRef(0)
  const minimizeButtonRef = useRef<HTMLButtonElement>(null)
  const conversationPaneRef = useRef<HTMLElement>(null)
  const moveFocusRef = useRef(false)
  const showThread = !compact || (threadOpen && selectedID > 0)
  const activeThreadID = showThread ? selectedID : 0
  const currentThread = thread?.conversation.id === selectedID ? thread : null
  const conversation = currentThread?.conversation || conversations.find((item) => item.id === selectedID)
  const draft = drafts[selectedID] || ''

  useEffect(() => {
    const pendingCustomerID = Number(window.sessionStorage.getItem('vpsmonitor-support-compose-customer') || '')
    if (!pendingCustomerID) return
    window.sessionStorage.removeItem('vpsmonitor-support-compose-customer')
    void openCompose(pendingCustomerID)
  }, [])

  useLayoutEffect(() => {
    activeThreadRef.current = activeThreadID
    return () => { activeThreadRef.current = 0 }
  }, [activeThreadID])

  useLayoutEffect(() => {
    if (!moveFocusRef.current) return
    let secondFrame: number | undefined
    const frame = window.requestAnimationFrame(() => {
      // The pane's visibility changes with aria-hidden. Wait one more frame so
      // the browser has applied the new style before moving keyboard focus.
      secondFrame = window.requestAnimationFrame(() => {
        moveFocusRef.current = false
        const pane = conversationPaneRef.current
        const target = showThread
          ? minimizeButtonRef.current
          : pane?.querySelector<HTMLElement>('.support-conversation-item.active') || pane?.querySelector('input')
        target?.focus({ preventScroll: true })
      })
    })
    return () => {
      window.cancelAnimationFrame(frame)
      if (secondFrame !== undefined) window.cancelAnimationFrame(secondFrame)
    }
  }, [showThread])

  async function loadConversations(silent = false) {
    const requestID = ++listRequestIDRef.current
    if (!silent) setListLoading(true)
    try {
      const data = await fetchJSON<SupportConversationListResponse>('/api/v1/admin/support')
      if (requestID !== listRequestIDRef.current) return
      const items = data.conversations || []
      setConversations(items)
      onUnreadCountChange?.(data.unread_count || 0)
      setSelectedID((current) => {
        if (current && items.some((item) => item.id === current)) return current
        return items.find((item) => item.status === 'open')?.id || items[0]?.id || 0
      })
      setError('')
    } catch (requestError) {
      if (requestID !== listRequestIDRef.current) return
      setError(requestError instanceof Error ? requestError.message : '客服会话加载失败')
    } finally {
      if (!silent && requestID === listRequestIDRef.current) setListLoading(false)
    }
  }

  async function openCompose(initialCustomerID?: number) {
    setComposeOpen(true)
    setComposeError('')
    setComposeLoading(true)
    try {
      const users = await fetchJSON<CustomerAdminView[]>('/api/v1/admin/customers')
      setRecipients(users)
      setRecipientID(initialCustomerID && users.some((item) => item.id === initialCustomerID) ? initialCustomerID : undefined)
    } catch (requestError) {
      setComposeError(requestError instanceof Error ? requestError.message : '注册用户加载失败')
    } finally {
      setComposeLoading(false)
    }
  }

  async function sendProactiveMessage() {
    const body = composeBody.trim()
    if (!recipientID || !body || composeLoading) return
    setComposeLoading(true)
    setComposeError('')
    try {
      const message = await fetchJSON<SupportMessage>('/api/v1/admin/support/conversations', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ customer_id: recipientID, body }),
      })
      setComposeOpen(false)
      setComposeBody('')
      setRecipientID(undefined)
      await loadConversations(true)
      if (message.conversation_id) selectConversation(message.conversation_id)
    } catch (requestError) {
      setComposeError(requestError instanceof Error ? requestError.message : '主动消息发送失败')
    } finally {
      setComposeLoading(false)
    }
  }

  async function loadThread(conversationID: number, silent = false) {
    if (activeThreadRef.current !== conversationID) return
    const requestID = ++threadRequestIDRef.current
    if (!silent) setThreadLoading(true)
    try {
      const data = await fetchJSON<SupportThreadResponse>(`/api/v1/admin/support/conversations/${conversationID}`)
      if (requestID !== threadRequestIDRef.current || activeThreadRef.current !== conversationID) return
      setThread(data)
      setConversations((current) => current.map((item) => item.id === conversationID ? { ...data.conversation, unread_count: 0 } : item))
      setError('')
    } catch (requestError) {
      if (requestID !== threadRequestIDRef.current || activeThreadRef.current !== conversationID) return
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
    // Reading a thread clears its unread badge, so only poll while it is visible.
    if (!activeThreadID) return
    let stopped = false
    let timer: number | undefined
    const poll = async (initial = false) => {
      await loadThread(activeThreadID, !initial)
      if (!stopped) timer = window.setTimeout(() => void poll(), 2000)
    }
    void poll(true)
    return () => {
      stopped = true
      threadRequestIDRef.current += 1
      if (timer !== undefined) window.clearTimeout(timer)
    }
  }, [activeThreadID])

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
      setDrafts((current) => current[selectedID] === draft ? { ...current, [selectedID]: '' } : current)
      await Promise.all([loadThread(selectedID, true), loadConversations(true)])
      if (activeThreadRef.current === selectedID) messageListRef.current?.scrollToLatest()
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
    if (!showThread) moveFocusRef.current = true
    setSelectedID(id)
    setThreadOpen(true)
    writeConversationURL(id)
  }

  function minimizeConversation() {
    moveFocusRef.current = true
    setThreadOpen(false)
    writeConversationURL()
  }

  function writeConversationURL(id?: number) {
    const url = new URL(window.location.href)
    url.searchParams.set('page', 'support')
    if (id) url.searchParams.set('conversation', String(id))
    else url.searchParams.delete('conversation')
    window.history.replaceState({}, '', `${url.pathname}?${url.searchParams.toString()}`)
  }

  const filtered = useMemo(() => {
    const keyword = search.trim().toLowerCase()
    return conversations.filter((item) => {
      if (filter === 'open' && item.status !== 'open') return false
      if (!keyword) return true
      return [item.customer_display_name, item.customer_username, item.guest_ip, item.last_message_preview]
        .some((value) => String(value || '').toLowerCase().includes(keyword))
    })
  }, [conversations, filter, search])

  const unreadCount = conversations.reduce((total, item) => total + item.unread_count, 0)

  return (
    <main className="admin-content-page support-admin-page">
      <header className="support-admin-toolbar" hidden={compact && showThread}>
        <div>
          <div className="eyebrow">Customer Support</div>
          <Title level={2}>在线客服</Title>
        </div>
        <div className="support-admin-toolbar-actions">
          <Button type="primary" onClick={() => void openCompose()}>主动联系</Button>
          <Badge count={unreadCount} overflowCount={99}>
            <Tag icon={<CustomerServiceOutlined />} color="blue">待处理消息</Tag>
          </Badge>
          <Tooltip title="刷新">
            <Button aria-label="刷新客服会话" icon={<ReloadOutlined />} loading={listLoading} onClick={() => void loadConversations()} />
          </Tooltip>
        </div>
      </header>

      {error ? <div className="support-admin-error"><Text type="danger">{error}</Text></div> : null}

      <Modal
        title="主动联系注册用户"
        open={composeOpen}
        onCancel={() => { if (!composeLoading) { setComposeOpen(false); setComposeBody(''); setComposeError('') } }}
        onOk={() => void sendProactiveMessage()}
        okText="发送消息"
        cancelText="取消"
        confirmLoading={composeLoading}
        okButtonProps={{ disabled: !recipientID || !composeBody.trim() }}
        destroyOnClose
      >
        <div className="support-compose-form">
          <div>
            <Text type="secondary">注册用户</Text>
            <Select
              showSearch
              allowClear
              value={recipientID}
              loading={composeLoading && !recipients.length}
              placeholder="搜索用户名或显示名"
              optionFilterProp="label"
              style={{ width: '100%', marginTop: 6 }}
              options={recipients.map((item) => ({
                value: item.id,
                label: `${item.display_name || item.username} · @${item.username}${item.enabled ? '' : ' · 已停用'}`,
              }))}
              onChange={(value) => setRecipientID(value)}
            />
          </div>
          <div>
            <Text type="secondary">消息内容</Text>
            <Input.TextArea
              autoSize={{ minRows: 4, maxRows: 8 }}
              maxLength={2000}
              showCount
              value={composeBody}
              placeholder="输入要发送给用户的消息"
              onChange={(event) => setComposeBody(event.target.value)}
            />
          </div>
          {composeError ? <Text type="danger">{composeError}</Text> : null}
          <Text type="secondary">访客不在主动联系列表中，只能回复已有访客会话。</Text>
        </div>
      </Modal>

      <div className={`support-admin-layout${compact ? ' support-admin-layout-compact' : ''}`}>
        <aside ref={conversationPaneRef} className="support-conversation-pane" aria-label="会话列表" aria-hidden={compact && showThread}>
          <div className="support-conversation-filters">
            <Input.Search value={search} allowClear aria-label="搜索用户或消息" placeholder="搜索用户或消息" onChange={(event) => setSearch(event.target.value)} />
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
                    <strong>{conversation.is_guest ? `访客 · ${conversation.guest_ip || '未知 IP'}` : conversation.customer_display_name || conversation.customer_username}</strong>
                    <time>{conversation.last_message_at ? compactTime(conversation.last_message_at) : ''}</time>
                  </span>
                  <span className="support-conversation-preview">{conversation.last_message_preview || '尚未发送消息'}</span>
                  <span className="support-conversation-meta">
                    <small>{conversation.is_guest ? '未登录临时账号' : `@${conversation.customer_username}`}</small>
                    {conversation.is_guest && conversation.referral_area_manager_id ? <Tag color="geekblue">推荐页：{conversation.referral_area_manager_display_name || conversation.referral_area_manager_username}</Tag> : null}
                    {conversation.is_guest ? <Tag color="gold">访客</Tag> : null}
                    {conversation.status === 'closed' ? <Tag>已结束</Tag> : null}
                    {conversation.unread_count > 0 ? <Badge count={conversation.unread_count} overflowCount={99} /> : null}
                  </span>
                </span>
              </button>
            ))}
          </div>
        </aside>

        <section className="support-thread-pane" aria-label="当前对话" aria-hidden={!showThread}>
          {compact || conversation ? (
            <header className="support-thread-header">
              {compact ? (
                <Button ref={minimizeButtonRef} className="support-thread-minimize" icon={<MinusOutlined />}
                  aria-label="最小化对话，返回会话列表" onClick={minimizeConversation}>最小化</Button>
              ) : null}
              <div className="support-thread-identity">
                {conversation ? (
                  <>
                    <strong>{conversation.is_guest ? `访客 · ${conversation.guest_ip || '未知 IP'}` : conversation.customer_display_name || conversation.customer_username}</strong>
                    <small>
                      {conversation.is_guest ? '未登录临时账号' : `@${conversation.customer_username}`}
                      {conversation.is_guest && conversation.referral_area_manager_id ? ` · 推荐页：${conversation.referral_area_manager_display_name || conversation.referral_area_manager_username}` : ''}
                    </small>
                  </>
                ) : <strong>当前对话</strong>}
              </div>
              {currentThread ? (
                <Button
                  icon={currentThread.conversation.status === 'open' ? <CheckCircleOutlined /> : <CloseCircleOutlined />}
                  loading={statusSaving}
                  onClick={() => void setConversationStatus(currentThread.conversation.status === 'open' ? 'closed' : 'open')}
                >{currentThread.conversation.status === 'open' ? '结束会话' : '重新打开'}</Button>
              ) : null}
            </header>
          ) : null}
          {!selectedID ? <Empty description="选择一个客服会话" /> : null}
          {selectedID && threadLoading && !currentThread ? <Spin /> : null}
          {currentThread ? (
            <>
              <SupportMessageList ref={messageListRef} className="support-thread-messages"
                conversationID={currentThread.conversation.id} lastMessageID={lastMessageID(currentThread.messages)} messageCount={currentThread.messages.length}>
                {!currentThread.messages.length ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="等待用户发送消息" /> : null}
                {currentThread.messages.map((message) => <AdminSupportMessage key={message.id} message={message} />)}
              </SupportMessageList>
              <footer className="support-thread-composer">
                <Input.TextArea
                  value={draft}
                  maxLength={2000}
                  autoSize={compact ? { minRows: 1, maxRows: 3 } : { minRows: 2, maxRows: 5 }}
                  aria-label="输入回复"
                  placeholder={compact ? '输入回复' : '输入回复，Enter 发送，Shift + Enter 换行'}
                  onChange={(event) => {
                    const value = event.target.value
                    setDrafts((current) => ({ ...current, [selectedID]: value }))
                  }}
                  onPressEnter={(event) => {
                    if (!compact && !event.shiftKey && !event.nativeEvent.isComposing) {
                      event.preventDefault()
                      void sendMessage()
                    }
                  }}
                />
                <Tooltip title="发送">
                  <Button aria-label="发送回复" type="primary" shape="circle" icon={<SendOutlined />} loading={sending} disabled={!draft.trim()} onClick={() => void sendMessage()} />
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
