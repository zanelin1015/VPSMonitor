import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import ts from 'typescript'

const dir = await mkdtemp(join(tmpdir(), 'vpsmonitor-routing-delete-'))
try {
  const source = await readFile(new URL('../src/lib/routingRules.ts', import.meta.url), 'utf8')
  const compiled = ts.transpileModule(source, {
    compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ES2022 },
  }).outputText
  await writeFile(join(dir, 'routingRules.mjs'), compiled, 'utf8')
  const { buildRoutingRulesDeletePayload: build, routingRuleDeleteDisabledReason: reason, isPendingRoutingDeletion: pending } = await import(`file://${join(dir, 'routingRules.mjs')}`)
  const fingerprint = 'a'.repeat(64)
  const r2 = { index: 2, fingerprint, outbound_tag: 'direct' }
  const r4 = { index: 4, fingerprint: 'b'.repeat(64) }
  const r3 = { index: 3, fingerprint: 'c'.repeat(64) }
  assert.deepEqual(build([r2], fingerprint), { rules_fingerprint: fingerprint, rules: [{ index: 2, fingerprint }] })
  assert.deepEqual(build([r2, r4, r3], fingerprint).rules.map((item) => item.index), [4, 3, 2])
  assert.equal(r2.index, 2, 'building the payload must not mutate the table data')
  assert.equal(reason(r2), '')
  assert.match(reason({ ...r2, protected: true }), /系统 API/)
  assert.match(reason({ index: 1 }), /刷新/)
  assert.throws(() => build([r2], ''), /校验信息/)
  assert.throws(() => build([], fingerprint), /请选择/)
  assert.throws(() => build(Array(1001).fill(r2), fingerprint), /请选择/)
  assert.throws(() => build([r2, r2], fingerprint), /索引无效/)
  assert.throws(() => build([{ ...r2, index: 1.5 }], fingerprint), /索引无效/)
  assert.throws(() => build([{ ...r2, index: 0 }], fingerprint), /索引无效/)
  assert.throws(() => build([{ ...r2, protected: true }], fingerprint), /系统 API/)
  assert.throws(() => build([{ ...r2, fingerprint: 'bad' }], fingerprint), /刷新/)
  const draft = build([r2, r4], fingerprint)
  r2.fingerprint = 'd'.repeat(64)
  assert.equal(draft.rules[1].fingerprint, fingerprint, 'confirmation keeps the original targets after a background refresh')
  assert.equal(pending({ kind: 'delete_routing_rules', status: 'pending' }), true)
  assert.equal(pending({ kind: 'delete_routing_rules', status: 'running' }), true)
  for (const status of ['succeeded', 'failed']) assert.equal(pending({ kind: 'delete_routing_rules', status }), false)
  assert.equal(pending({ kind: 'delete_client', status: 'running' }), false)
  const outboundSource = await readFile(new URL('../src/lib/outbounds.ts', import.meta.url), 'utf8')
  await writeFile(join(dir, 'outbounds.mjs'), ts.transpileModule(outboundSource, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ES2022 } }).outputText)
  const { buildOutboundsDeletePayload: buildOutbounds, outboundDeleteDisabledReason: outboundReason, isPendingOutboundDeletion: outboundPending } = await import(`file://${join(dir, 'outbounds.mjs')}`)
  const outbound = { tag: 'old-a', fingerprint }
  const second = { tag: 'old-b', fingerprint: 'b'.repeat(64) }
  const selection = buildOutbounds([outbound, second], fingerprint)
  assert.deepEqual(selection.outbounds.map(item => item.tag), ['old-a', 'old-b'])
  assert.equal(selection.config_fingerprint, fingerprint)
  assert.equal(outboundReason(outbound), '')
  assert.throws(() => buildOutbounds([outbound], ''), /校验信息/)
  assert.throws(() => buildOutbounds([], fingerprint), /请选择/)
  assert.throws(() => buildOutbounds([outbound, outbound], fingerprint), /重复/)
  assert.throws(() => buildOutbounds([{ ...outbound, is_default: true }], fingerprint), /默认出口/)
  assert.throws(() => buildOutbounds([{ ...outbound, delete_blocked_reason: '被路由 R2 引用' }], fingerprint), /R2/)
  assert.throws(() => buildOutbounds([{ tag: 'generated' }], fingerprint), /可编辑出站/)
  outbound.fingerprint = 'd'.repeat(64)
  assert.equal(selection.outbounds[0].fingerprint, fingerprint, 'confirmation cannot silently change after a refresh')
  assert.equal(outboundPending({ kind: 'delete_outbounds', status: 'running' }), true)
  assert.equal(outboundPending({ kind: 'delete_outbounds', status: 'failed' }), false)
  assert.equal(outboundPending({ kind: 'add_outbound', status: 'running' }), false)
  console.log('routing and outbound batch deletion tests passed')
} finally {
  await rm(dir, { recursive: true, force: true })
}
