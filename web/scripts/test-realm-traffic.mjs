import assert from 'node:assert/strict'
import { fileURLToPath } from 'node:url'
import { createServer } from 'vite'
import { createElement as h } from 'react'
import { act, create } from 'react-test-renderer'

const originalWindow = globalThis.window
const originalFetch = globalThis.fetch
globalThis.window = { location: new URL('http://localhost/zanelin/monitor') }
const pending = []
globalThis.fetch = (url, init) => new Promise(resolve => pending.push({ url, signal: init.signal, resolve }))
const vite = await createServer({
  root: fileURLToPath(new URL('../', import.meta.url)),
  configFile: fileURLToPath(new URL('../vite.config.ts', import.meta.url)),
  logLevel: 'silent', optimizeDeps: { noDiscovery: true, include: [] }, ssr: { optimizeDeps: { noDiscovery: true, include: [] } },
  server: { host: '127.0.0.1', port: 0 },
})
let renderer
try {
  const { useRealmTrafficUsage } = await vite.ssrLoadModule('/src/hooks/useRealmTrafficUsage.ts')
  const { normalizeEntryConfig } = await vite.ssrLoadModule('/src/lib/appHelpersConfig.ts')
  assert.equal(normalizeEntryConfig({ port_forwarding: { enabled: true, traffic_enabled: true } }).port_forwarding.traffic_enabled, true, 'loading and saving an entry preserve the traffic switch')
  assert.equal(normalizeEntryConfig({ port_forwarding: {} }).port_forwarding.traffic_enabled, false, 'existing installations do not enable kernel accounting automatically')
  let usage
  function Harness({ agent = 'gz', from = '2026-11-01', to = '2026-11-18' }) {
    usage = useRealmTrafficUsage(agent, from, to)
    return null
  }
  await act(async () => { renderer = create(h(Harness, {})) })
  assert.equal(pending[0].url, '/zanelin/api/v1/agents/gz/realm/traffic?from=2026-11-01&to=2026-11-18')
  assert.equal(usage.loading, true)
  await act(async () => { renderer.update(h(Harness, { agent: 'hk' })) })
  assert.equal(pending[0].signal.aborted, true)
  const result = label => ({ items: [{ key: label, total_bytes: 123 }], from: '2026-11-01', to: '2026-11-18' })
  await act(async () => pending[1].resolve(Response.json(result('hk'))))
  assert.equal(usage.data.items[0].key, 'hk')
  await act(async () => pending[0].resolve(Response.json(result('stale-gz'))))
  assert.equal(usage.data.items[0].key, 'hk', 'late responses cannot replace the current agent statistics')
  await act(async () => { renderer.update(h(Harness, { agent: 'hk', from: '2026-11-18' })) })
  assert.equal(usage.data, undefined, 'changing the period clears the previous totals')
  await act(async () => pending[2].resolve(Response.json({ error: '权限不足' }, { status: 403 })))
  assert.equal(usage.loading, false)
  assert.equal(usage.error, '权限不足')
  assert.equal(usage.data, undefined)
  await act(async () => usage.refresh())
  await act(async () => pending[3].resolve(Response.json({ items: null })))
  assert.deepEqual(usage.data.items, [], 'an empty response cannot crash the table')
  await act(async () => usage.refresh())
  const last = pending.at(-1)
  await act(async () => renderer.unmount())
  assert.equal(last.signal.aborted, true, 'closing the view cancels the request')
  await act(async () => last.resolve(Response.json(result('closed'))))
  console.log('Realm traffic switch persistence, default-off behavior, periods, subpaths, stale reads, errors and cancellation passed')
} finally {
  renderer?.unmount()
  if (originalWindow === undefined) delete globalThis.window
  else globalThis.window = originalWindow
  globalThis.fetch = originalFetch
  await vite.close()
}
