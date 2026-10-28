import assert from 'node:assert/strict'
import { fileURLToPath } from 'node:url'
import { createElement as h, createRef } from 'react'
import { act, create } from 'react-test-renderer'
import { createServer } from 'vite'

const originalResizeObserver = globalThis.ResizeObserver
let resize, disconnected = false
globalThis.ResizeObserver = class {
  constructor(callback) { resize = callback }
  observe() {}
  disconnect() { disconnected = true }
}
const vite = await createServer({
  root: fileURLToPath(new URL('../', import.meta.url)),
  configFile: fileURLToPath(new URL('../vite.config.ts', import.meta.url)),
  logLevel: 'silent', optimizeDeps: { noDiscovery: true, include: [] },
  ssr: { optimizeDeps: { noDiscovery: true, include: [] } },
})
let renderer
try {
  const { SupportMessageList } = await vite.ssrLoadModule('/src/components/SupportMessageList.tsx')
  const ref = createRef()
  let scrollTop = 0
  const element = {
    scrollHeight: 400, clientHeight: 400,
    get scrollTop() { return scrollTop },
    set scrollTop(value) { scrollTop = Math.max(0, Math.min(value, this.scrollHeight - this.clientHeight)) },
  }
  const props = { ref, conversationID: 1, lastMessageID: 0, messageCount: 0, className: 'support-thread-messages' }
  const render = () => h(SupportMessageList, props, h('p', {}, 'Chat history'))
  const update = async (lastID, count, height) => {
    props.lastMessageID = lastID
    props.messageCount = count
    element.scrollHeight = height
    await act(async () => renderer.update(render()))
  }
  const scroll = async top => {
    element.scrollTop = top
    await act(async () => renderer.root.findByProps({ role: 'region' }).props.onScroll({ currentTarget: element }))
  }
  await act(async () => { renderer = create(render(), { createNodeMock: () => element }) })
  await update(40, 40, 2000)
  assert.equal(scrollTop, 1600, 'first loaded history opens at the latest message')
  await scroll(600)
  await update(41, 41, 2050)
  assert.equal(scrollTop, 600, 'incoming messages do not pull the reader away from history')
  assert.equal(renderer.root.findByType('button').children.join(''), '有新消息 · 回到最新')
  await act(async () => resize())
  assert.equal(scrollTop, 600, 'resize preserves history reading position')
  await act(async () => renderer.root.findByType('button').props.onClick())
  assert.equal(scrollTop, 1650, 'latest button reaches the bottom')
  assert.equal(renderer.root.findAllByType('button').length, 0)
  await update(42, 42, 2100)
  assert.equal(scrollTop, 1700, 'incoming messages follow when already at the bottom')
  element.clientHeight = 300
  await act(async () => resize())
  assert.equal(scrollTop, 1800, 'growing composer keeps the newest reply in view')
  await scroll(0)
  await act(async () => ref.current.scrollToLatest())
  assert.equal(scrollTop, 1800, 'successful sends can explicitly reveal the reply')
  await update(43, 42, 2150)
  assert.equal(scrollTop, 1850, 'fixed-size API message windows still follow the last message ID')
  await scroll(300)
  props.conversationID = 2
  await update(43, 42, 2200)
  assert.equal(scrollTop, 1900, 'switching conversations resets scrolling even with matching last ID and count')
  await act(async () => renderer.unmount())
  renderer = null
  assert.ok(disconnected, 'resize observer is disconnected on unmount')
  console.log('Support chat initial position, history preservation, follow-latest, resize and conversation switching passed')
} finally {
  if (renderer) await act(async () => renderer.unmount())
  globalThis.ResizeObserver = originalResizeObserver
  await vite.close()
}
