import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import ts from 'typescript'

const dir = await mkdtemp(join(tmpdir(), 'vpsmonitor-app-base-'))
try {
  for (const file of ['appBasePath', 'api', 'adminRoute']) {
    const source = await readFile(new URL(`../src/lib/${file}.ts`, import.meta.url), 'utf8')
    let compiled = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ES2022 } }).outputText
    compiled = compiled.replaceAll("'./appBasePath'", "'./appBasePath.mjs'").replaceAll("'./agentHealth'", "'./agentHealth.mjs'")
    await writeFile(join(dir, `${file}.mjs`), compiled)
  }
  await writeFile(join(dir, 'agentHealth.mjs'), 'export function normalizeAgentHealthFilter(v) { return v || "all" }')
  globalThis.window = { location: { pathname: '/', origin: 'https://monitor.zanelin.top', href: 'https://monitor.zanelin.top/', search: '' } }
  const base = await import(`file://${join(dir, 'appBasePath.mjs')}`)
  const api = await import(`file://${join(dir, 'api.mjs')}`)
  const route = await import(`file://${join(dir, 'adminRoute.mjs')}`)
  for (const entry of ['/', '/customer', '/admin/assets']) assert.equal(base.appBasePath(entry), '')
  assert.equal(base.appBasePath('/zanelin-other'), '')
  assert.equal(base.adminEntryPath(), '/')
  assert.equal(base.customerEntryURL(), 'https://monitor.zanelin.top/customer')
  assert.equal(api.buildDashboardRealtimeURL(), 'wss://monitor.zanelin.top/api/v1/dashboard/realtime')
  window.location = { pathname: '/zanelin/monitor', origin: 'https://monitor.appleaccount.top', href: 'https://monitor.appleaccount.top/zanelin/monitor', search: '?page=assets&agent=test' }
  assert.equal(base.appPathname('/zanelin/customer/'), '/customer')
  assert.equal(base.adminEntryPath(), '/zanelin/monitor')
  assert.equal(base.customerEntryURL(), 'https://monitor.appleaccount.top/zanelin/customer')
  assert.equal(base.withAppBasePath('/zanelin/api/v1/admin/session'), '/zanelin/api/v1/admin/session')
  assert.equal(base.withAppBasePath('https://external.example/api'), 'https://external.example/api')
  assert.equal(base.withAppBasePath('//external.example/api'), '//external.example/api')
  assert.equal(api.buildDashboardRealtimeURL(), 'wss://monitor.appleaccount.top/zanelin/api/v1/dashboard/realtime')
  assert.match(api.buildAgentTerminalURL('a/b', 'bash'), /^wss:\/\/monitor.appleaccount.top\/zanelin\/api\/v1\/agents\/a%2Fb\/terminal\/ws\?/)
  const state = route.parseAdminRouteState(true)
  assert.equal(state.page, 'assets')
  assert.equal(state.agentId, 'test')
  assert.equal(route.buildAdminRouteURL(state), '/zanelin/monitor?page=assets&agent=test')
  let fetched
  globalThis.fetch = async (url, init) => { fetched = { url, init }; return { ok: true, json: async () => ({ ok: true }) } }
  await api.fetchJSON('/api/v1/customer/session')
  assert.equal(fetched.url, '/zanelin/api/v1/customer/session')
  assert.equal(fetched.init.credentials, 'same-origin')
  console.log('root and subpath navigation/API/WebSocket tests passed')
} finally {
  await rm(dir, { recursive: true, force: true })
}
