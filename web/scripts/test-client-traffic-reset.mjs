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
  const { buildClientTrafficResetAction, clientTrafficResetSummary, CLIENT_TRAFFIC_RESET_OPTIONS } = await import(`file://${join(dir, 'clientTrafficReset.mjs')}`)
  const record = { inbound_id: 7, inbound_tag: 'node-7', email: 'alice', client_id: 'stable-id', traffic_reset_supported: true, traffic_reset: 'monthly', traffic_reset_day: 15 }
  assert.equal(clientTrafficResetSummary(record), '每月 15 号')
  assert.equal(clientTrafficResetSummary({ ...record, traffic_reset_day: 0 }), '每月 1 号')
  assert.equal(clientTrafficResetSummary({ ...record, traffic_reset: 'weekly' }), '每周日')
  assert.equal(clientTrafficResetSummary({ ...record, traffic_reset_supported: false }), '未上报')
  for (const { value } of CLIENT_TRAFFIC_RESET_OPTIONS) {
    const action = buildClientTrafficResetAction(record, value, 31, 'agent-source')
    assert.equal(action.agentID, 'agent-source')
    assert.deepEqual(action.request, { kind: 'update_client_traffic_reset', payload: { inbound_id: 7, inbound_tag: 'node-7', email: 'alice', client_id: 'stable-id', traffic_reset: value, traffic_reset_day: 31 } })
    assert.equal(action.request.payload.expiry_time, undefined)
    assert.equal(action.request.payload.total_bytes, undefined)
  }
  const forwarded = buildClientTrafficResetAction({ ...record, realm_target_agent_id: 'agent-target', realm_target_inbound_id: 9, realm_target_inbound_tag: 'node-9' }, 'monthly', 15, 'agent-source')
  assert.equal(forwarded.agentID, 'agent-target')
  assert.equal(forwarded.request.payload.inbound_id, 9)
  assert.equal(forwarded.request.payload.inbound_tag, 'node-9')
  for (const day of [null, 0, 32, 1.5, NaN]) assert.throws(() => buildClientTrafficResetAction(record, 'monthly', day, 'agent'))
  assert.throws(() => buildClientTrafficResetAction(record, 'month', 15, 'agent'))
  assert.throws(() => buildClientTrafficResetAction({ ...record, email: '' }, 'monthly', 15, 'agent'))
  console.log('native client traffic reset payload, summary and forwarding tests passed')
} finally {
  await rm(dir, { recursive: true, force: true })
}
