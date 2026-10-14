import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import ts from 'typescript'

const dir = await mkdtemp(join(tmpdir(), 'vpsmonitor-customer-session-'))
const originalFetch = globalThis.fetch
const originalWindow = globalThis.window
const events = []
const response = (status, payload = { ok: true }) => new Response(JSON.stringify(payload), { status, headers: { 'Content-Type': 'application/json' } })
const deferred = () => {
  let resolve
  const promise = new Promise(done => { resolve = done })
  return { promise, resolve }
}
try {
  for (const file of ['appBasePath', 'api']) {
    const source = await readFile(new URL(`../src/lib/${file}.ts`, import.meta.url), 'utf8')
    const compiled = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ES2022 } }).outputText.replaceAll("'./appBasePath'", "'./appBasePath.mjs'")
    await writeFile(join(dir, `${file}.mjs`), compiled)
  }
  globalThis.window = {
    location: { pathname: '/zanelin/customer', href: 'https://monitor.example/zanelin/customer', origin: 'https://monitor.example' },
    dispatchEvent: event => { events.push(event.type); return true },
  }
  const api = await import(`file://${join(dir, 'api.mjs')}`)

  // Renewal preserves a mutation's method, body, headers and abort signal.
  let calls = []
  const controller = new AbortController()
  const mutation = { method: 'PUT', body: JSON.stringify({ remark: 'updated' }), headers: { 'Content-Type': 'application/json' }, signal: controller.signal }
  globalThis.fetch = async (url, init) => {
    calls.push({ url, init })
    return response(calls.length === 1 ? 401 : 200)
  }
  assert.deepEqual(await api.fetchJSON('/api/v1/customer/assignments/7/remark', mutation), { ok: true })
  assert.deepEqual(calls.map(call => call.url), ['/zanelin/api/v1/customer/assignments/7/remark', '/zanelin/api/v1/customer/refresh', '/zanelin/api/v1/customer/assignments/7/remark'])
  assert.equal(calls[1].init.method, 'POST')
  for (const call of calls) assert.equal(call.init.credentials, 'same-origin')
  assert.deepEqual(calls[0].init, calls[2].init)
  assert.equal(calls[2].init.signal, controller.signal)
  assert.deepEqual(events, [])

  // Concurrent 401s share a renewal. A late 401 reuses the renewed cookie.
  const started = deferred()
  const release = deferred()
  const late = deferred()
  const attempts = new Map()
  let refreshCount = 0
  globalThis.fetch = async url => {
    if (url.endsWith('/refresh')) {
      refreshCount++
      started.resolve()
      await release.promise
      return response(200)
    }
    const count = (attempts.get(url) || 0) + 1
    attempts.set(url, count)
    if (count === 1 && url.endsWith('/session')) await late.promise
    return response(count === 1 ? 401 : 200)
  }
  const first = api.fetchJSON('/api/v1/customer/overview')
  const second = api.fetchJSON('/api/v1/customer/usage')
  const third = api.fetchJSON('/api/v1/customer/session')
  await started.promise
  release.resolve()
  await Promise.all([first, second])
  late.resolve()
  await third
  assert.equal(refreshCount, 1)
  for (const count of attempts.values()) assert.equal(count, 2)

  // Invalid refresh returns to login; no mutation replay occurs.
  calls = []
  globalThis.fetch = async (url, init) => { calls.push({ url, init }); return response(401, { error: 'customer login required' }) }
  await assert.rejects(api.fetchJSON('/api/v1/customer/account', mutation), error => error instanceof api.APIError && error.status === 401)
  assert.equal(calls.length, 2)
  assert.deepEqual(events.splice(0), [api.CUSTOMER_SESSION_EXPIRED_EVENT])

  // Login, logout, direct refresh, subscription and admin failures never recurse.
  for (const url of ['/api/v1/customer/login', '/api/v1/customer/logout', '/api/v1/customer/refresh', '/api/v1/customer/subscription/token/clash.yaml', '/api/v1/admin/session', 'https://other.example/api/v1/customer/session']) {
    calls = []
    await assert.rejects(api.fetchJSON(url), error => error.status === 401)
    assert.equal(calls.length, 1, url)
    assert.deepEqual(events, [])
  }

  // A renewed access rejected again is retried only once.
  calls = []
  globalThis.fetch = async (url, init) => { calls.push({ url, init }); return response(url.endsWith('/refresh') ? 200 : 401) }
  await assert.rejects(api.fetchJSON('/api/v1/customer/session'), error => error.status === 401)
  assert.equal(calls.length, 3)
  assert.deepEqual(events.splice(0), [api.CUSTOMER_SESSION_EXPIRED_EVENT])

  // Temporary refresh failures must not clear a valid login UI state.
  for (const failure of ['server', 'network']) {
    calls = []
    globalThis.fetch = async (url, init) => {
      calls.push({ url, init })
      if (!url.endsWith('/refresh')) return response(401)
      if (failure === 'network') throw new TypeError('network unavailable')
      return response(503, { error: 'temporarily unavailable' })
    }
    await assert.rejects(api.fetchJSON('/api/v1/customer/overview'))
    assert.equal(calls.length, 2)
    assert.deepEqual(events, [])
  }

  // Logout waits for any renewal, and never renews itself.
  const logoutStarted = deferred()
  const logoutRelease = deferred()
  calls = []
  let protectedCalls = 0
  globalThis.fetch = async (url, init) => {
    calls.push({ url, init })
    if (url.endsWith('/refresh')) {
      logoutStarted.resolve()
      await logoutRelease.promise
      return response(200)
    }
    if (url.endsWith('/logout')) return response(200)
    return response(++protectedCalls === 1 ? 401 : 200)
  }
  const pendingRequest = api.fetchJSON('/api/v1/customer/session')
  await logoutStarted.promise
  const logout = api.fetchJSON('/api/v1/customer/logout', { method: 'POST' })
  assert.ok(!calls.some(call => call.url.endsWith('/logout')))
  logoutRelease.resolve()
  await Promise.all([pendingRequest, logout])
  assert.equal(calls.filter(call => call.url.endsWith('/logout')).length, 1)
  assert.equal(calls.filter(call => call.url.endsWith('/refresh')).length, 1)

  // Root deployments use the same mechanism without a subpath.
  window.location = { pathname: '/customer', href: 'https://monitor.example/customer', origin: 'https://monitor.example' }
  calls = []
  globalThis.fetch = async (url, init) => { calls.push({ url, init }); return response(calls.length === 1 ? 401 : 200) }
  await api.fetchJSON('/api/v1/customer/session')
  assert.deepEqual(calls.map(call => call.url), ['/api/v1/customer/session', '/api/v1/customer/refresh', '/api/v1/customer/session'])
  assert.deepEqual(events, [])
  console.log('Customer access/refresh expiry, single-flight, replay, logout and base-path tests passed')
} finally {
  globalThis.fetch = originalFetch
  globalThis.window = originalWindow
  await rm(dir, { recursive: true, force: true })
}
