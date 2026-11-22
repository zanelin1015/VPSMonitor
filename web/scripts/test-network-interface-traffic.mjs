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
  root: fileURLToPath(new URL('../', import.meta.url)), configFile: fileURLToPath(new URL('../vite.config.ts', import.meta.url)),
  logLevel: 'silent', optimizeDeps: { noDiscovery: true, include: [] }, ssr: { optimizeDeps: { noDiscovery: true, include: [] } }, server: { host: '127.0.0.1', port: 0 },
})
let renderer
try {
  const { useAgentTrafficUsage } = await vite.ssrLoadModule('/src/hooks/useAgentTrafficUsage.ts')
  const { suggestedNetworkInterfaceKeys, selectedNetworkInterfaceTotals, networkInterfaceTrafficValue } = await vite.ssrLoadModule('/src/lib/networkInterfaceTraffic.ts')
  const card = (key, patch = {}) => ({ key, name: key, present: true, up: true, kind: 'physical', loopback: false, primary: false, received_bytes: 100, sent_bytes: 200, ...patch })
  const items = [card('docker0', { kind: 'bridge' }), card('ens3', { primary: true }), card('lo', { loopback: true, kind: 'loopback' }), card('retired', { present: false })]
  assert.deepEqual(suggestedNetworkInterfaceKeys(items), ['ens3'], 'auto selection never sums a bridge, device and loopback together')
  const totals = selectedNetworkInterfaceTotals(items, ['ens3', 'ens3'])
  assert.deepEqual(totals, { received_bytes: 100, sent_bytes: 200 }, 'duplicate selection cannot double count a NIC')
  assert.equal(networkInterfaceTrafficValue(totals, 'received'), 100)
  assert.equal(networkInterfaceTrafficValue(totals, 'sent'), 200)
  assert.equal(networkInterfaceTrafficValue(totals, 'both'), 300)
  assert.deepEqual(selectedNetworkInterfaceTotals(items, []), { received_bytes: 0, sent_bytes: 0 }, 'an explicitly empty selection stays empty')
  assert.deepEqual(suggestedNetworkInterfaceKeys([card('lo', { loopback: true })]), [], 'loopback is never automatically selected')
  assert.deepEqual(suggestedNetworkInterfaceKeys([card('pppoe-wan', { kind: 'virtual', primary: true }), card('eth0')]), ['pppoe-wan'], 'a routed logical WAN can be selected without also adding its parent')
  assert.deepEqual(suggestedNetworkInterfaceKeys([card('docker0', { kind: 'bridge' }), card('ens3')]), ['ens3'], 'fallback prefers an active device')
  assert.equal(networkInterfaceTrafficValue(selectedNetworkInterfaceTotals(items, ['retired']), 'both'), 300, 'removed interfaces remain usable for historical queries')

  let usage
  function Harness({ agent = 'node a', path = 'interfaces/traffic', from = '2026-11-01' }) {
    usage = useAgentTrafficUsage(agent, path, from, '2026-11-18')
    return null
  }
  await act(async () => { renderer = create(h(Harness, {})) })
  assert.equal(pending[0].url, '/zanelin/api/v1/agents/node%20a/interfaces/traffic?from=2026-11-01&to=2026-11-18')
  await act(async () => renderer.update(h(Harness, { agent: 'node-b' })))
  assert.equal(pending[0].signal.aborted, true)
  await act(async () => pending[1].resolve(Response.json({ items: [card('node-b')] })))
  await act(async () => pending[0].resolve(Response.json({ items: [card('stale-node-a')] })))
  assert.equal(usage.data.items[0].key, 'node-b')
  await act(async () => renderer.update(h(Harness, { agent: 'node-b', path: 'realm/traffic' })))
  assert.equal(usage.data, undefined, 'Realm and NIC results must not share a request identity')
  await act(async () => pending[2].resolve(Response.json({ items: null })))
  assert.deepEqual(usage.data.items, [])
  await act(async () => usage.refresh())
  await act(async () => pending[3].resolve(Response.json({ error: 'permission denied' }, { status: 403 })))
  assert.equal(usage.error, 'permission denied')
  assert.equal(usage.data, undefined, 'an error cannot keep showing totals from the previous query')
  await act(async () => renderer.unmount())
  console.log('NIC automatic selection, virtual deduplication, RX/TX/bidirectional modes, historical selection and request isolation passed')
} finally {
  renderer?.unmount()
  if (originalWindow === undefined) delete globalThis.window
  else globalThis.window = originalWindow
  globalThis.fetch = originalFetch
  await vite.close()
}
