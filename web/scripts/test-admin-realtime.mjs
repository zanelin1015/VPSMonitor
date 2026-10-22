import assert from 'node:assert/strict'
import { fileURLToPath } from 'node:url'
import { createServer } from 'vite'
import { createElement as h } from 'react'
import { act, create } from 'react-test-renderer'

const originalWindow = globalThis.window
const originalWebSocket = globalThis.WebSocket
const timers = new Map()
let nextTimer = 0
globalThis.window = {
  location: new URL('http://localhost:5173/'),
  setTimeout(fn, delay) {
    const id = ++nextTimer
    timers.set(id, { fn, delay })
    return id
  },
  clearTimeout(id) {
    timers.delete(id)
  },
}
class FakeSocket {
  static CONNECTING = 0
  static OPEN = 1
  static CLOSED = 3
  static instances = []
  static failNext = false
  readyState = FakeSocket.CONNECTING
  closes = []
  onopen = null
  onclose = null
  onerror = null
  onmessage = null
  constructor(url) {
    if (FakeSocket.failNext) {
      FakeSocket.failNext = false
      throw new Error('simulated websocket constructor failure')
    }
    this.url = url
    FakeSocket.instances.push(this)
  }
  open() {
    this.readyState = FakeSocket.OPEN
    this.onopen?.({})
  }
  close(code, reason) {
    this.closes.push({ code, reason, readyState: this.readyState })
    this.readyState = FakeSocket.CLOSED
    this.onclose?.({ code: code || 1006 })
  }
  reset() {
    this.readyState = FakeSocket.CLOSED
    this.onclose?.({ code: 1006 })
  }
  message(payload) {
    this.onmessage?.({ data: JSON.stringify(payload) })
  }
}
globalThis.WebSocket = FakeSocket
const vite = await createServer({
  root: fileURLToPath(new URL('../', import.meta.url)),
  configFile: fileURLToPath(new URL('../vite.config.ts', import.meta.url)),
  logLevel: 'silent',
  optimizeDeps: { noDiscovery: true, include: [] },
  ssr: { optimizeDeps: { noDiscovery: true, include: [] } },
})
let renderer
const user = { id: 1, username: 'preview', role: 'admin' }
async function tick(delay) {
  assert.equal(timers.size, 1, 'only one handshake/reconnect can be pending')
  const [id, timer] = [...timers][0]
  assert.equal(timer.delay, delay)
  timers.delete(id)
  await act(async () => timer.fn())
}
try {
  const { useDashboardRealtime } = await vite.ssrLoadModule('/src/hooks/admin/useDashboardRealtime.ts')
  function Harness({ account, onMetrics }) {
    useDashboardRealtime(account, onMetrics)
    return null
  }
  const received = []
  const props = { account: user, onMetrics: (metrics) => received.push(['original', metrics]) }
  // Simulate the setup/cleanup/setup probe performed by React StrictMode.
  await act(async () => {
    renderer = create(h(Harness, props))
  })
  await act(async () => {
    renderer.unmount()
  })
  assert.equal(timers.size, 0)
  assert.equal(FakeSocket.instances.length, 0, 'a StrictMode probe does not abort an in-flight handshake')
  await act(async () => {
    renderer = create(h(Harness, props))
  })
  await tick(0)
  const first = FakeSocket.instances.at(-1)
  assert.equal(first.url, 'ws://localhost:5173/api/v1/dashboard/realtime')
  await act(async () => first.open())
  await act(async () => {
    renderer.update(
      h(Harness, {
        account: { ...user, cost_currency: 'USD' },
        onMetrics: (metrics) => received.push(['latest', metrics]),
      }),
    )
  })
  assert.equal(FakeSocket.instances.length, 1, 'updates to the same account reuse the live connection')
  assert.equal(first.closes.length, 0)
  await act(async () => first.message({ type: 'snapshot', metrics: [{ agent_id: 'sample-a' }] }))
  assert.deepEqual(received, [['latest', [{ agent_id: 'sample-a' }]]], 'the stream uses the latest callback')
  first.onmessage({ data: '{malformed' })
  assert.equal(timers.size, 0)

  for (const delay of [3000, 6000, 12000, 24000, 30000, 30000]) {
    FakeSocket.instances.at(-1).reset()
    await tick(delay)
  }
  const recovered = FakeSocket.instances.at(-1)
  await act(async () => recovered.message({ type: 'snapshot', metrics: [] }))
  recovered.reset()
  await tick(3000)
  const connecting = FakeSocket.instances.at(-1)
  const lateMessage = connecting.onmessage
  const lateClose = connecting.onclose
  await act(async () => renderer.update(h(Harness, { ...props, account: null })))
  assert.equal(timers.size, 0, 'logout cancels reconnects')
  assert.equal(connecting.closes.length, 0, 'cleanup does not abort a pending upgrade')
  connecting.open()
  assert.equal(connecting.closes[0].code, 1000, 'a pending upgrade closes gracefully after opening')
  assert.equal(connecting.closes[0].readyState, FakeSocket.OPEN)
  lateMessage({ data: JSON.stringify({ type: 'metrics', metric: { agent_id: 'stale' } }) })
  lateClose({ code: 1006 })
  assert.equal(received.length, 1, 'old account frames cannot update the new session')
  assert.equal(timers.size, 0, 'old account closes cannot restart its stream')

  FakeSocket.failNext = true
  await act(async () => renderer.update(h(Harness, props)))
  await tick(0)
  await tick(3000)
  const active = FakeSocket.instances.at(-1)
  active.open()
  await act(async () => renderer.update(h(Harness, { ...props, account: { ...user, role: 'area_manager' } })))
  assert.equal(active.closes[0].code, 1000, 'role changes replace the account stream')
  await act(async () => renderer.unmount())
  assert.equal(timers.size, 0, 'unmount clears the scheduled handshake')
  console.log('realtime stable identity, StrictMode cleanup, stale frames, recovery and capped backoff passed')
} finally {
  renderer?.unmount()
  if (originalWindow === undefined) delete globalThis.window
  else globalThis.window = originalWindow
  if (originalWebSocket === undefined) delete globalThis.WebSocket
  else globalThis.WebSocket = originalWebSocket
  await vite.close()
}
