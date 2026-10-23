import assert from 'node:assert/strict'
import { fileURLToPath } from 'node:url'
import { createServer } from 'vite'
const vite = await createServer({ root: fileURLToPath(new URL('../', import.meta.url)), configFile: fileURLToPath(new URL('../vite.config.ts', import.meta.url)), logLevel: 'silent', optimizeDeps: { noDiscovery: true, include: [] }, ssr: { optimizeDeps: { noDiscovery: true, include: [] } } })
try {
  const { buildRealmGrantOptions, buildAreaAssignmentTreeData, emptyAreaManagerForm } = await vite.ssrLoadModule('/src/components/CustomerManagementHelpers.tsx')
  const agent = { agent_id: 'entry', last_seen_at: new Date().toISOString(), agent_name: 'Entry', summary: {}, entry: { port_forwarding: { enabled: true, backend: 'realm', rules: [{ enabled: true, listen_port: 10000, target_port: 20000 }] }, haproxy: { enabled: true, rules: [{ enabled: true, listen_port: 10001, primary: { port: 20001 } }] } } }
  assert.equal(buildRealmGrantOptions('entry', [agent]).length, 0, 'configuration alone does not prove a service is running')
  agent.realm = { running: true }
  assert.equal(buildRealmGrantOptions('entry', [agent]).length, 1)
  agent.realm.error = 'read failed'
  assert.equal(buildRealmGrantOptions('entry', [agent]).length, 0)
  agent.realm = { running: false }
  agent.haproxy = { running: true }
  assert.equal(buildRealmGrantOptions('entry', [agent]).length, 1, 'HAProxy runtime independently enables its entries')
  agent.last_seen_at = new Date(Date.now() - 180_000).toISOString()
  assert.equal(buildRealmGrantOptions('entry', [agent]).length, 0, 'stale runtime reports do not enable new grants')
  agent.last_seen_at = new Date().toISOString()
  agent.haproxy.running = false
  assert.equal(buildRealmGrantOptions('entry', [agent]).length, 0)
  const overview = { nodes: [{ id: 1, enabled: true }, { id: 2, enabled: false }], clients: [{ inbound_id: 1, email: 'active', enabled: true }, { inbound_id: 1, email: 'disabled', enabled: false }] }
  const tree = buildAreaAssignmentTreeData('entry', overview, [agent])
  assert.equal(tree[0].children.length, 1, 'disabled nodes do not enter the grant tree')
  assert.equal(tree[0].children[0].children.length, 1, 'disabled clients do not enter the grant tree')
  for (const key of ['realm_enabled', 'line_enabled', 'landing_enabled', 'front_link_enabled']) assert.equal(emptyAreaManagerForm[key], false, 'new account categories require explicit enablement')
  console.log('area permission candidate tests passed')
} finally { await vite.close() }
