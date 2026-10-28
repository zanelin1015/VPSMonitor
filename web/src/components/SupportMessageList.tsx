import { forwardRef, useCallback, useEffect, useImperativeHandle, useLayoutEffect, useRef, useState, type ReactNode } from 'react'

export interface SupportMessageListHandle {
  scrollToLatest: () => void
}

interface SupportMessageListProps {
  conversationID: number
  lastMessageID: number
  messageCount: number
  className: string
  children: ReactNode
}

// All support surfaces share the same scroll behavior, including mobile widgets.
export const SupportMessageList = forwardRef<SupportMessageListHandle, SupportMessageListProps>(function SupportMessageList(
  { conversationID, lastMessageID, messageCount, className, children }, ref,
) {
  const listRef = useRef<HTMLDivElement>(null)
  const followLatestRef = useRef(true)
  const previousRef = useRef({ conversationID, lastMessageID })
  const [showLatest, setShowLatest] = useState(false)
  const [hasNewMessages, setHasNewMessages] = useState(false)

  const scrollToLatest = useCallback(() => {
    followLatestRef.current = true
    const element = listRef.current
    if (element) element.scrollTop = element.scrollHeight
    setShowLatest(false)
    setHasNewMessages(false)
  }, [])

  useImperativeHandle(ref, () => ({ scrollToLatest }), [scrollToLatest])

  useLayoutEffect(() => {
    const previous = previousRef.current
    if (previous.conversationID !== conversationID || followLatestRef.current) {
      scrollToLatest()
    } else if (previous.lastMessageID !== lastMessageID) {
      setHasNewMessages(true)
    }
    previousRef.current = { conversationID, lastMessageID }
  }, [conversationID, lastMessageID, messageCount, scrollToLatest])

  useEffect(() => {
    const element = listRef.current
    if (!element || typeof ResizeObserver === 'undefined') return
    // Keep the latest reply visible when the composer grows or the viewport resizes.
    const observer = new ResizeObserver(() => {
      if (followLatestRef.current) scrollToLatest()
    })
    observer.observe(element)
    return () => observer.disconnect()
  }, [scrollToLatest])

  return (
    <div className="support-message-viewport">
      <div ref={listRef} className={className} role="region" aria-label="聊天记录" tabIndex={0}
        onScroll={(event) => {
          const element = event.currentTarget
          const atBottom = element.scrollHeight - element.clientHeight - element.scrollTop <= 40
          followLatestRef.current = atBottom
          setShowLatest(!atBottom)
          if (atBottom) setHasNewMessages(false)
        }}>
        {children}
      </div>
      {showLatest ? (
        <button type="button" className="support-scroll-latest" onClick={scrollToLatest}>
          {hasNewMessages ? '有新消息 · 回到最新' : '回到最新消息'}
        </button>
      ) : null}
    </div>
  )
})
