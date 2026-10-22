import assert from 'node:assert/strict'
import { fileURLToPath } from 'node:url'
import { createServer } from 'vite'
import { createElement as h } from 'react'
import { act, create } from 'react-test-renderer'

const originalWindow = globalThis.window
const originalFetch = globalThis.fetch
const timers = new Map()
let nextTimer = 1
const cancelled = []
globalThis.window = {
  location: new URL('http://localhost/?page=assets&agent=sample-a'),
  setTimeout(fn) {
    const id = nextTimer++
    timers.set(id, { fn, type: 'timeout' })
    return id
  },
  clearTimeout(id) {
    cancelled.push(id)
    timers.delete(id)
  },
  setInterval(fn) {
    const id = nextTimer++
    timers.set(id, { fn, type: 'interval' })
    return id
  },
  clearInterval(id) {
    cancelled.push(id)
    timers.delete(id)
  },
}
const json = (body) => ({ ok: true, status: 200, json: async () => body })
globalThis.fetch = async () => json({ unread_count: 2 })
const vite = await createServer({
  root: fileURLToPath(new URL('../', import.meta.url)),
  configFile: fileURLToPath(new URL('../vite.config.ts', import.meta.url)),
  logLevel: 'silent',
  optimizeDeps: { noDiscovery: true, include: [] },
  ssr: { optimizeDeps: { noDiscovery: true, include: [] } },
})
let renderer
try {
  const { useAdminLifecycle } = await vite.ssrLoadModule('/src/hooks/admin/useAdminLifecycle.ts')
  const calls = []
  const stub =
    (name) =>
    (...args) => {
      calls.push([name, ...args])
    }
  const options = Object.fromEntries(
    [
      'loadSession',
      'loadAgents',
      'loadTelegramBots',
      'loadTagSettings',
      'loadExchangeRates',
      'loadFinanceAccounts',
      'loadUpdateLatestInfo',
      'setActiveAdminPage',
      'setActiveTabKey',
      'loadScheduledTasks',
      'openFrontendSettingsModal',
      'loadAccessLogs',
      'setSelectedAgentId',
      'logoutSession',
      'setDashboardView',
      'setDashboardLoaded',
      'setSelectedTag',
      'setAgentHealthFilter',
      'setAgentListSearch',
      'setAgents',
      'setFinanceCustomers',
      'setFinanceAreaManagers',
      'setFinanceAccountsLoaded',
      'setFinanceAccountsError',
      'setOverview',
      'setManagedConfig',
      'setTelegramBots',
      'setAgentLogs',
      'setConfigAudits',
      'loadOverview',
      'loadXUIActions',
      'loadAgentLogs',
    ].map((name) => [name, stub(name)]),
  )
  Object.assign(options, {
    adminUser: { id: 1, username: 'preview', role: 'admin' },
    canManageSystem: true,
    updateModalOpen: false,
    activeAdminPage: 'assets',
    activeTabKey: 'overview',
    agents: [],
    selectedAgentId: 'sample-a',
    dashboardLoaded: false,
  })
  function LifecycleHarness({ value }) {
    useAdminLifecycle(value)
    return null
  }
  await act(async () => {
    renderer = create(h(LifecycleHarness, { value: options }))
  })
  assert.ok(calls.some((call) => call[0] === 'loadSession'))
  assert.ok(
    !calls.some((call) => call[0] === 'setSelectedAgentId'),
    'a deep link survives while the first dashboard request is pending',
  )
  assert.equal([...timers.values()].filter((timer) => timer.type === 'interval').length, 1)
  calls.length = 0
  await act(async () => {
    renderer.update(h(LifecycleHarness, { value: { ...options, dashboardLoaded: true } }))
  })
  assert.ok(
    calls.some((call) => call[0] === 'setSelectedAgentId' && call[1] === ''),
    'a removed agent is cleared after a successful dashboard load',
  )
  calls.length = 0
  const area = {
    ...options,
    adminUser: { id: 2, username: 'area', role: 'area_manager' },
    canManageSystem: false,
    activeAdminPage: 'finance',
    activeTabKey: 'logs',
  }
  await act(async () => {
    renderer.update(h(LifecycleHarness, { value: area }))
  })
  assert.ok(calls.some((call) => call[0] === 'setActiveAdminPage' && call[1] === 'dashboard'))
  assert.ok(calls.some((call) => call[0] === 'setActiveTabKey' && call[1] === 'overview'))
  assert.ok(!calls.some((call) => ['loadFinanceAccounts', 'loadTelegramBots', 'loadAccessLogs'].includes(call[0])))
  calls.length = 0
  const operator = {
    ...options,
    adminUser: { id: 3, username: 'operator', role: 'operator' },
    canManageSystem: false,
    activeAdminPage: 'assets',
    activeTabKey: 'overview',
  }
  await act(async () => {
    renderer.update(h(LifecycleHarness, { value: operator }))
  })
  assert.ok(calls.some((call) => call[0] === 'setActiveAdminPage' && call[1] === 'customers'))
  assert.ok(!calls.some((call) => ['loadFinanceAccounts', 'loadTelegramBots', 'loadAccessLogs'].includes(call[0])))
  await act(async () => {
    renderer.unmount()
  })
  assert.equal(timers.size, 0, 'polling and unread timers are released on unmount')
  assert.ok(cancelled.length >= 2)

  const { useAgentDetailData } = await vite.ssrLoadModule('/src/hooks/admin/useAgentDetailData.ts')
  let releaseOld
  globalThis.fetch = async (raw) => {
    const url = String(raw)
    if (url.includes('/sample-a/xui/overview'))
      return new Promise((resolve) => {
        releaseOld = () => resolve(json({ agent_id: 'sample-a', summary: {}, clients: [], nodes: [] }))
      })
    if (url.endsWith('/xui/overview')) return json({ agent_id: 'sample-b', summary: {}, clients: [], nodes: [] })
    if (url.endsWith('/config')) return json({ agent_id: 'sample-b', agent_name: 'remote-b' })
    if (url.endsWith('/logs')) return json({ logs: [] })
    return json([])
  }
  const no = () => {}
  const detailOptions = {
    selectedAgentId: 'sample-a',
    adminUser: options.adminUser,
    pendingClientSearchRef: { current: null },
    setActiveTabKey: no,
    setClientSearch: no,
    setSelectedOutboundTag: no,
    setSelectedRuleIndex: no,
    setSelectedNodeAnchor: no,
    reloadToken: 0,
    inFlightRequestsRef: { current: new Set() },
    setAdminUser: no,
    cachedCustomerDisplayName: () => undefined,
    agents: [],
    rememberCustomerDisplayName: no,
    message: { error: no, success: no, warning: no },
    setTagOptions: no,
    loadAgents: async () => {},
    canManageSystem: true,
    tagOptions: [],
    selectedAgent: undefined,
  }
  let detail
  function DetailHarness({ value }) {
    detail = useAgentDetailData(value)
    return null
  }
  await act(async () => {
    renderer = create(h(DetailHarness, { value: detailOptions }))
  })
  await act(async () => {
    renderer.update(h(DetailHarness, { value: { ...detailOptions, selectedAgentId: 'sample-b' } }))
  })
  assert.equal(detail.overview.agent_id, 'sample-b')
  await act(async () => {
    releaseOld()
  })
  assert.equal(
    detail.overview.agent_id,
    'sample-b',
    'a late response for the previous Client cannot overwrite the current detail',
  )
  await act(async () => {
    detail.updateManagedConfig((current) => ({ ...current, agent_name: 'unsaved-draft' }))
  })
  await act(async () => {
    await detail.loadManagedConfig('sample-b', { silent: true })
  })
  assert.equal(detail.managedConfig.agent_name, 'unsaved-draft', 'background refresh preserves an unsaved config draft')
  assert.equal(detail.savedManagedConfig.agent_name, 'remote-b')
  await act(async () => {
    renderer.unmount()
  })
  console.log('admin deep links, role restrictions, timer cleanup, stale responses and unsaved drafts passed')
} finally {
  renderer?.unmount()
  if (originalWindow === undefined) delete globalThis.window
  else globalThis.window = originalWindow
  globalThis.fetch = originalFetch
  await vite.close()
}
