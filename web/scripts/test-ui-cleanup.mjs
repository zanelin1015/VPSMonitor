import assert from 'node:assert/strict'
import { fileURLToPath } from 'node:url'
import { createServer } from 'vite'
import { createElement as h } from 'react'
import { act, create } from 'react-test-renderer'
import { App as AntApp } from 'antd'

const originalWindow = globalThis.window
const originalFetch = globalThis.fetch
const originalUseApp = AntApp.useApp
const originalNow = Date.now
let now = 0
const dueAt = new Map()
const timers = new Map()
let nextTimer = 0
const cancelled = []
globalThis.window = {
  location: new URL('http://localhost/'),
  setTimeout(fn, delay = 0) { const id = ++nextTimer; timers.set(id, fn); dueAt.set(id, now + delay); return id },
  clearTimeout(id) { cancelled.push(id); timers.delete(id) },
}
const vite = await createServer({
  root: fileURLToPath(new URL('../', import.meta.url)),
  configFile: fileURLToPath(new URL('../vite.config.ts', import.meta.url)),
  logLevel: 'silent', optimizeDeps: { noDiscovery: true, include: [] }, ssr: { optimizeDeps: { noDiscovery: true, include: [] } },
})
let renderer
try {
  const { useSupportThread, supportURL } = await vite.ssrLoadModule('/src/hooks/useSupportThread.ts')
  assert.equal(supportURL('customer', 'area-a', '/messages'), '/api/v1/customer/support/messages', 'authenticated support never uses guest ownership')
  assert.equal(new URL(supportURL('guest', 'area a', '', true), 'http://localhost').searchParams.get('area'), 'area a')
  const { GuestSupportWidget } = await vite.ssrLoadModule('/src/components/GuestSupportWidget.tsx')
  assert.notEqual(GuestSupportWidget({ scopeToken: 'area-a' }).key, GuestSupportWidget({ scopeToken: 'area-b' }).key, 'changing area remounts guest state')
  assert.notEqual(GuestSupportWidget({}).key, GuestSupportWidget({ scopeToken: 'default' }).key, 'a custom area slug cannot collide with the default guest identity')

  const requests = []
  const thread = id => ({ conversation: { id, unread_count: 0 }, messages: [], support_online: true })
  globalThis.fetch = async (url, init) => {
    requests.push({ url: String(url), method: init?.method || 'GET', body: init?.body })
    return Response.json(init?.method === 'POST' ? { id: 1 } : thread(String(url).includes('/public/') ? 202 : 101))
  }
  let customer, guest
  function SupportHarness() {
    customer = useSupportThread('customer')
    guest = useSupportThread('guest', 'area-a')
    return null
  }
  await act(async () => { renderer = create(h(SupportHarness)) })
  assert.equal(customer.thread.conversation.id, 101)
  assert.equal(guest.thread.conversation.id, 202)
  assert.ok(requests.some(r => r.url.includes('/public/support?mark_read=0&area=area-a')))
  await act(async () => { guest.setDraft('Guest message') })
  await act(async () => { await Promise.all([guest.sendMessage(), guest.sendMessage()]) })
  const posts = requests.filter(r => r.method === 'POST')
  assert.equal(posts.length, 1, 'rapid duplicate clicks send one message')
  assert.equal(posts[0].url, '/api/v1/public/support/messages?area=area-a')
  assert.equal(JSON.parse(posts[0].body).body, 'Guest message')
  assert.equal(customer.thread.conversation.id, 101, 'guest send cannot change authenticated thread')
  await act(async () => { renderer.unmount() })
  assert.equal(timers.size, 0, 'all support polls stop on unmount')

  let releaseOld
  let reads = 0
  globalThis.fetch = async () => {
    if (++reads === 1) return new Promise(resolve => { releaseOld = () => resolve(Response.json(thread(303))) })
    return Response.json(thread(404))
  }
  function StaleHarness() { guest = useSupportThread('guest', 'area-a'); return null }
  await act(async () => { renderer = create(h(StaleHarness)) })
  await act(async () => { guest.setOpen(true) })
  assert.equal(guest.thread.conversation.id, 404)
  await act(async () => { releaseOld() })
  assert.equal(guest.thread.conversation.id, 404, 'late closed-widget response cannot replace newer data')
  await act(async () => { renderer.unmount() })

  const { useXUIActionResultRefresh } = await vite.ssrLoadModule('/src/hooks/useXUIActionResultRefresh.ts')
  let refresh
  const calls = []
  const receipts = new Map()
  Date.now = () => now
  async function tick() {
    const id = [...timers.keys()].sort((a, b) => dueAt.get(a) - dueAt.get(b))[0]
    assert.ok(id, 'a read-back is scheduled')
    now = dueAt.get(id)
    const fn = timers.get(id)
    timers.delete(id)
    await act(async () => { fn() })
  }
  async function drain() {
    let count = 0
    while (timers.size && ++count < 100) await tick()
    assert.ok(count < 100, 'finite refresh schedules stop')
  }
  function RefreshHarness({ observedAgentID, actions } = {}) {
    refresh = useXUIActionResultRefresh({
      loadXUIActions: async id => { calls.push(['actions', id]); return receipts.get(id) || [] },
      loadOverview: async id => calls.push(['overview', id]),
      loadAgents: async () => calls.push(['agents']),
      loadAgentLogs: async id => calls.push(['logs', id]),
      observedAgentID, actions,
    })
    return null
  }
  await act(async () => { renderer = create(h(RefreshHarness)) })
  refresh.scheduleXUIActionResultRefresh('entry', { relatedAgentIDs: ['exit'], agents: true })
  refresh.scheduleXUIActionResultRefresh('entry', { relatedAgentIDs: ['entry', 'exit'], logs: true })
  assert.equal(timers.size, 1, 'mutations on one agent share one timer')
  await drain()
  assert.equal(calls.filter(c => c[0] === 'actions').length, 6)
  assert.equal(calls.filter(c => c[0] === 'overview' && c[1] === 'entry').length, 6)
  assert.equal(calls.filter(c => c[0] === 'overview' && c[1] === 'exit').length, 1)
  assert.equal(calls.filter(c => c[0] === 'agents').length, 1, 'directory refreshes are not multiplied by receipt retries')
  assert.equal(calls.filter(c => c[0] === 'logs').length, 1)
  const readsBeforeCommand = calls.length
  refresh.scheduleXUIActionResultRefresh('command', { overview: false, delays: [2500] })
  assert.equal(timers.size, 1, 'command output needs one receipt read-back')
  await drain()
  assert.deepEqual(calls.slice(readsBeforeCommand), [['actions', 'command']])
  const pending = { id: 7, agent_id: 'entry', kind: 'update_routing_rule', status: 'pending' }
  receipts.set('entry', [pending])
  await act(async () => { renderer.update(h(RefreshHarness, { observedAgentID: 'entry', actions: [pending] })) })
  await tick()
  assert.equal(timers.size, 1, 'pending routes keep refreshing after initial read-backs')
  refresh.scheduleXUIActionResultRefresh('entry')
  assert.equal(timers.size, 1, 'pending-route polling and mutation retries use the same timer')
  receipts.set('entry', [{ ...pending, status: 'succeeded' }])
  await act(async () => { renderer.update(h(RefreshHarness)) })
  await drain()

  const reset = { id: 8, agent_id: 'entry', kind: 'manual_reset_client_traffic', status: 'running' }
  receipts.set('entry', [reset])
  refresh.scheduleXUIActionResultRefresh('entry')
  const resetResult = refresh.waitForXUIAction('entry', reset)
  assert.equal(timers.size, 1, 'reset verification joins the existing receipt schedule')
  await tick()
  const verified = { ...reset, status: 'succeeded', result: { verified: true, reset_done: true } }
  receipts.set('entry', [verified])
  await tick()
  assert.deepEqual(await resetResult, verified, 'reset verification returns the exact server receipt')
  await drain()

  const offline = { ...reset, id: 9, agent_id: 'offline', status: 'pending' }
  receipts.set('offline', [offline])
  const started = now
  const timeoutResult = refresh.waitForXUIAction('offline', offline)
  await drain()
  assert.equal(now - started, 90_000, 'offline reset waits are bounded without resubmitting the action')
  assert.deepEqual(await timeoutResult, offline)

  const cancelledResult = refresh.waitForXUIAction('entry', reset)
  await act(async () => { renderer.unmount() })
  assert.equal(await cancelledResult, null, 'leaving the console cancels receipt waits')
  assert.equal(timers.size, 0, 'action retry timers stop when the console unmounts')
  refresh.scheduleXUIActionResultRefresh('entry')
  assert.equal(timers.size, 0, 'a late action response cannot restart timers after unmount')
  assert.ok(cancelled.length > 0)
  Date.now = originalNow

  const notices = []
  const message = Object.fromEntries(['success', 'warning', 'info', 'error'].map(kind => [kind, text => notices.push([kind, text])]))
  AntApp.useApp = () => ({ message })
  const { useXUIRemoteActions } = await vite.ssrLoadModule('/src/hooks/useXUIRemoteActions.ts')
  const resetPosts = []
  let remote, resolveReset, resultPromise, resultReads = 0, schedules = 0
  globalThis.fetch = async (url, init) => {
    resetPosts.push({ url: String(url), body: JSON.parse(init.body) })
    return Response.json({ id: 88, agent_id: 'exit', kind: 'manual_reset_client_traffic', status: 'running' })
  }
  function RemoteHarness() {
    remote = useXUIRemoteActions({
      selectedAgentId: 'entry', setAdminUser() {},
      loadOverview: async () => { resultReads++ },
      loadXUIActions: async () => { resultReads++; return [] },
      scheduleXUIActionResultRefresh: () => { schedules++ },
      refreshXUIActionResult: async () => {},
      waitForXUIAction: () => resultPromise,
    })
    return null
  }
  await act(async () => { renderer = create(h(RemoteHarness)) })
  const resetClient = { inbound_id: 7, inbound_tag: 'entry-7', email: 'alice', client_id: 'stable-id', protocol: 'vless', realm_target_agent_id: 'exit', realm_target_inbound_id: 9 }
  resultPromise = new Promise(resolve => { resolveReset = resolve })
  let resetOperation
  await act(async () => { resetOperation = remote.manualResetXUIClientTraffic(resetClient); await remote.manualResetXUIClientTraffic(resetClient) })
  assert.equal(resetPosts.length, 1, 'double clicking reset creates exactly one destructive action')
  assert.equal(resetPosts[0].url, '/api/v1/agents/exit/xui/actions')
  assert.equal(resetPosts[0].body.payload.inbound_id, 9, 'forwarded clients reset their real target')
  await act(async () => { resolveReset({ id: 88, status: 'succeeded', result: { verified: true, reset_done: true } }); await resetOperation })
  assert.ok(notices.some(([kind, text]) => kind === 'success' && text.includes('核验')))
  for (const receipt of [
    { status: 'succeeded', result: { verified: false, reset_done: true } },
    { status: 'pending' },
    { status: 'failed', error: 'reset rejected' },
  ]) {
    notices.length = 0
    resultPromise = Promise.resolve(receipt)
    await act(async () => { await remote.manualResetXUIClientTraffic(resetClient) })
    assert.ok(!notices.some(([kind]) => kind === 'success'), 'unverified, pending or failed receipts cannot report reset success')
  }
  const readsBeforeCancel = resultReads, schedulesBeforeCancel = schedules
  resultPromise = Promise.resolve(null)
  await act(async () => { await remote.manualResetXUIClientTraffic(resetClient) })
  assert.equal(resultReads, readsBeforeCancel, 'cancelled waits do not reload resources after navigation')
  assert.equal(schedules, schedulesBeforeCancel)
  await act(async () => { renderer.unmount() })
  AntApp.useApp = originalUseApp

  const history = await vite.ssrLoadModule('/src/lib/speedHistory.ts')
  const initial = history.createInitialSpeedHistory(Number.NaN, -50)
  assert.equal(initial.length, 24)
  assert.ok(initial.every(row => row.up === 0 && row.down === 0))
  assert.equal(initial[1].time - initial[0].time, 5 * 60_000)
  const rows = [{ time: 0, up: 1024, down: 2048, samples: 1 }, { time: 300_000, up: 4096, down: 1024, samples: 1 }]
  const axis = history.speedAxisInfo(rows)
  assert.ok(axis.maxValue >= 4096)
  assert.equal(axis.unit, 'KB')
  const { SpeedHistoryChart } = await vite.ssrLoadModule('/src/components/SpeedHistoryChart.tsx')
  await act(async () => { renderer = create(h(SpeedHistoryChart, { rows, axis })) })
  for (const path of renderer.root.findAllByType('path')) assert.doesNotMatch(path.props.d, /NaN|Infinity/)
  await act(async () => { renderer.root.findByType('svg').props.onKeyDown({ key: 'End', preventDefault() {} }) })
  const tooltip = renderer.root.findByProps({ role: 'status' })
  assert.ok(tooltip.findAllByType('span').some(child => child.children.join('') === '上传 4.0 KB/s'), 'keyboard tooltip shows the selected sample')
  await act(async () => { renderer.unmount() })
  renderer = null
  console.log('support identity isolation, stale reads, shared action refreshes and SVG chart interaction passed')
} finally {
  if (renderer) await act(async () => renderer.unmount())
  globalThis.window = originalWindow
  globalThis.fetch = originalFetch
  Date.now = originalNow
  AntApp.useApp = originalUseApp
  await vite.close()
}
