import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import ts from 'typescript'

const source = await readFile(new URL('../src/lib/clientTrafficReset.ts', import.meta.url), 'utf8')
const output = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ES2022 } }).outputText
const dir = await mkdtemp(join(tmpdir(), 'vpsmonitor-native-reset-'))
try {
  await writeFile(join(dir, 'clientTrafficReset.mjs'), output)
  const { buildManualClientTrafficResetAction, clientTrafficResetSummary, clientTrafficResetStartDay, clientTrafficResetOptions, resolveClientTrafficResetChoice, CLIENT_TRAFFIC_RESET_OPTIONS } = await import(`file://${join(dir, 'clientTrafficReset.mjs')}`)
  const record = { inbound_id: 7, inbound_tag: 'node-7', email: 'alice', client_id: 'stable-id', traffic_reset_supported: true, traffic_reset: 'monthly', traffic_reset_day: 15 }
  assert.equal(clientTrafficResetSummary(record), '每月 15 号')
  assert.equal(clientTrafficResetSummary({ ...record, traffic_reset_day: 0 }), '每月 1 号')
  assert.equal(clientTrafficResetSummary({ ...record, traffic_reset: 'weekly' }), '每周日')
  assert.equal(clientTrafficResetSummary({ ...record, traffic_reset_supported: false }), '未上报')
  for (const { value } of CLIENT_TRAFFIC_RESET_OPTIONS) {
    assert.deepEqual(resolveClientTrafficResetChoice(value, 31), { period: value, day: 31 })
  }
  for (const day of [null, 0, 32, 1.5, NaN]) assert.equal(resolveClientTrafficResetChoice('monthly', day), null)
  for (const timezone of ['UTC', 'Asia/Shanghai', 'America/New_York']) {
    process.env.TZ = timezone
    for (const [startDate, expectedDay] of [['2026-10-01', 1], ['2026-10-10', 10], ['2026-01-31', 31]]) {
      const startTime = Date.parse(`${startDate}T00:00:00+08:00`)
      assert.equal(clientTrafficResetStartDay(startTime), expectedDay, `start day in ${timezone}`)
      const shortcut = clientTrafficResetOptions(startTime).find(option => option.value === 'monthly_from_start')
      assert.equal(shortcut.label, `每月 ${expectedDay} 号（按开始日）`)
      assert.equal(shortcut.disabled, false)
      const settings = resolveClientTrafficResetChoice('monthly_from_start', 28, startTime)
      assert.deepEqual(settings, { period: 'monthly', day: expectedDay }, 'copy start day, not the current or expiry day')
    }
  }
  for (const startTime of [undefined, 0, -1, NaN, Infinity, 9e15]) {
    assert.equal(clientTrafficResetStartDay(startTime), null)
    assert.equal(clientTrafficResetOptions(startTime).find(option => option.value === 'monthly_from_start').disabled, true)
    assert.equal(resolveClientTrafficResetChoice('monthly_from_start', 1, startTime), null)
  }
  assert.deepEqual(resolveClientTrafficResetChoice('monthly', 10), { period: 'monthly', day: 10 }, 'manual day override remains available')
  assert.equal(resolveClientTrafficResetChoice('monthly', null), null)
  assert.deepEqual(resolveClientTrafficResetChoice('daily', null), { period: 'daily', day: 1 })
  assert.equal(resolveClientTrafficResetChoice('month', 10), null)
  const manual = buildManualClientTrafficResetAction(record, 'agent-source')
  assert.deepEqual(manual, { agentID: 'agent-source', request: { kind: 'manual_reset_client_traffic', payload: { inbound_id: 7, inbound_tag: 'node-7', email: 'alice', client_id: 'stable-id' } } })
  const manualForwarded = buildManualClientTrafficResetAction({ ...record, realm_target_agent_id: 'agent-target', realm_target_inbound_id: 9, realm_target_inbound_tag: 'node-9' }, 'agent-source')
  assert.equal(manualForwarded.agentID, 'agent-target')
  assert.equal(manualForwarded.request.payload.inbound_id, 9)
  assert.equal(manualForwarded.request.payload.inbound_tag, 'node-9')
  for (const patch of [{ email: '' }, { client_id: '' }, { inbound_id: 0 }, { inbound_id: 1.5 }, { protocol: 'socks' }, { protocol: 'http' }]) {
    assert.throws(() => buildManualClientTrafficResetAction({ ...record, ...patch }, 'agent-source'))
  }
  console.log('native client traffic reset payload, summary and forwarding tests passed')
  console.log('start-day shortcut, timezone, original day retention, missing start and native API compatibility tests passed')
} finally {
  await rm(dir, { recursive: true, force: true })
}
