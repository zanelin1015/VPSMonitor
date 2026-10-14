import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import ts from 'typescript'

const dir = await mkdtemp(join(tmpdir(), 'vpsmonitor-client-config-'))
try {
  for (const name of ['clientConfig', 'appHelpersBilling', 'clientTrafficReset', 'currency', 'traffic']) {
    const source = await readFile(new URL(`../src/lib/${name}.ts`, import.meta.url), 'utf8')
    let code = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ES2022 } }).outputText
    for (const dependency of ['clientConfig', 'appHelpersBilling', 'clientTrafficReset', 'currency', 'traffic']) code = code.replaceAll(`'./${dependency}'`, `'./${dependency}.mjs'`)
    await writeFile(join(dir, `${name}.mjs`), code)
  }
  const lib = await import(`file://${join(dir, 'clientConfig.mjs')}`)
  const billingLib = await import(`file://${join(dir, 'appHelpersBilling.mjs')}`)
  const record = { inbound_id: 7, inbound_tag: 'test', email: 'alice', client_id: 'stable-uuid', protocol: 'vless', enabled: false, config_enabled: true, total_gb: 50 * 1024 ** 3, expiry_time: Date.parse('2026-04-30T00:00:00+08:00'), traffic_reset_supported: true, traffic_reset: 'never', traffic_reset_day: 1 }
  const billing = { ...billingLib.defaultClientBilling(record), start_time: Date.parse('2026-01-31T00:00:00+08:00'), revenue_cycle: 'quarter', expire_cycle: 'quarter', revenue_amount: 63 }
  const initial = lib.createClientConfigDraft(record, billing)
  assert.equal(initial.enabled, true, 'configured enablement is independent of runtime exhausted/expired state')
  assert.deepEqual(lib.buildClientConfigRequest(record, billing, initial, initial, 'request-unchanged').changes, {}, 'unmodified fields never get dispatched')
  const draft = lib.editClientBilling({ ...initial, enabled: false, totalGB: 100, resetChoice: 'monthly_from_start' }, { start_time: Date.parse('2026-10-10T00:00:00+08:00') })
  const request = lib.buildClientConfigRequest(record, billing, initial, draft, 'request-all')
  assert.equal(request.changes.enabled, false)
  assert.equal(request.changes.total_bytes, 100 * 1024 ** 3)
  assert.equal(request.changes.expiry_time, Date.parse('2027-01-10T00:00:00+08:00'))
  assert.equal(request.changes.traffic_reset, 'monthly')
  assert.equal(request.changes.traffic_reset_day, 10, 'reset shortcut uses the new draft start day')
  assert.equal(request.expected.enabled, true)
  assert.equal(request.previous_billing, billing)
  assert.equal(request.billing.start_time, draft.billing.start_time)
  const endOfMonth = lib.buildClientConfigRequest(record, billing, initial, { ...initial, resetChoice: 'monthly_from_start' }, 'request-month-end')
  assert.equal(endOfMonth.changes.traffic_reset_day, 31, 'the original day is retained for x-ui short-month fallback')
  const manualExpiry = Date.parse('2028-05-15T16:22:33+08:00')
  const manual = lib.editClientBilling({ ...initial, manualExpiry: true, billing: { ...initial.billing, expire_time: manualExpiry } }, { revenue_cycle: 'year' })
  assert.equal(manual.billing.expire_time, manualExpiry, 'cycle changes retain a manual expiry')
  assert.equal(lib.beijingDateInput(manualExpiry, true), '2028-05-15T16:22:33')
  const unlimited = lib.buildClientConfigRequest(record, billing, initial, { ...initial, unlimitedExpiry: true }, 'request-unlimited')
  assert.equal(unlimited.changes.expiry_time, 0)
  assert.equal(unlimited.billing.expire_time, 0)
  const noStored = lib.createClientConfigDraft(record)
  assert.ok(noStored.billing.start_time, 'a start can be inferred for display')
  const feeOnly = lib.buildClientConfigRequest(record, undefined, noStored, lib.editClientBilling(noStored, { revenue_amount: 99 }), 'request-fee-only')
  assert.equal(feeOnly.billing.start_time, 0, 'fee edits do not persist an inferred date')
  assert.deepEqual(feeOnly.changes, {})
  const save = { request_id: 'saved-all', billing: request.billing, billing_saved: true, action: { status: 'failed', result: { field_results: { enable: { verified: true }, totalGB: { verified: true }, expiryTime: { verified: false }, trafficReset: { verified: true }, trafficResetDay: { verified: false } } } } }
  const retry = lib.retryClientConfigRequest(request, save, 'request-retry')
  assert.equal(retry.changes.enabled, undefined)
  assert.equal(retry.changes.total_bytes, undefined)
  assert.equal(retry.changes.expiry_time, request.changes.expiry_time)
  assert.equal(retry.changes.traffic_reset, 'monthly', 'reset keeps its required pair')
  assert.deepEqual(retry.previous_billing, save.billing)
  assert.equal(lib.clientConfigSaveStatus(save).text, '部分失败')
  assert.equal(lib.clientConfigSaveStatus({ ...save, action: { status: 'pending' } }).text, '待下发')
  for (const totalGB of [-1, null, NaN, Infinity, 1e12]) assert.throws(() => lib.buildClientConfigRequest(record, billing, initial, { ...initial, totalGB }, 'request-invalid'))
  assert.throws(() => lib.buildClientConfigRequest(record, billing, initial, { ...initial, resetChoice: 'monthly', resetDay: 32 }, 'request-invalid-day'))
  const account = { ...record, protocol: 'socks' }
  assert.deepEqual(lib.buildClientConfigRequest(account, billing, initial, { ...initial, enabled: false, totalGB: 100, unlimitedExpiry: true }, 'request-account').changes, {}, 'node accounts only save local billing')
  console.log('unified client draft, manual dates, monthly shortcut, safe patch and failed-field retry tests passed')
} finally { await rm(dir, { recursive: true, force: true }) }
