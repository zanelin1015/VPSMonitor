import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import ts from 'typescript'

const source = await readFile(new URL('../src/lib/clientUpdateLogs.ts', import.meta.url), 'utf8')
const code = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ES2022 } }).outputText
const { clientUpdateResult, filterClientUpdateLogs } = await import(`data:text/javascript;base64,${Buffer.from(code).toString('base64')}`)
const base = { id: 1, batch_id: 'batch-one', agent_id: 'agent1', agent_name: 'HK Client', version: '0.3.35', target_version: '0.3.36', decision: 'dispatched', reason: '已下发', created_at: '2026-10-09T01:00:00Z' }
assert.equal(clientUpdateResult(base).label, '待领取')
assert.equal(clientUpdateResult({ ...base, task_status: 'running' }).label, '执行中')
assert.equal(clientUpdateResult({ ...base, task_status: 'succeeded' }).label, '已启动，待确认', 'installer launch must not claim upgrade success')
assert.equal(clientUpdateResult({ ...base, task_status: 'succeeded', confirmed_at: base.created_at }).label, '升级成功')
assert.equal(clientUpdateResult({ ...base, task_status: 'failed', task_error: 'download failed' }).detail, 'download failed')
assert.equal(clientUpdateResult({ ...base, task_status: 'failed', task_error: 'execution lease expired; retry manually' }).label, '结果未确认')
assert.equal(clientUpdateResult({ ...base, force: true }).label, '强制待领取')
const skipped = { ...base, id: 2, agent_name: 'US Client', decision: 'skipped', reason: '旧 Client 需要手动升级一次' }
assert.equal(clientUpdateResult(skipped).detail, skipped.reason)
assert.deepEqual(filterClientUpdateLogs([base, skipped], '', 'skipped'), [skipped])
assert.deepEqual(filterClientUpdateLogs([base, skipped], '手动升级', 'all'), [skipped])
assert.deepEqual(filterClientUpdateLogs([base, skipped], ' hk ', 'dispatched'), [base])
assert.equal(filterClientUpdateLogs([base], '0.3.36', 'all').length, 1)
const panel = await readFile(new URL('../src/components/ClientUpdateLogPanel.tsx', import.meta.url), 'utf8')
const modal = await readFile(new URL('../src/components/AdminModals.tsx', import.meta.url), 'utf8')
assert.ok(panel.includes("timeZone: 'Asia/Shanghai'"))
assert.ok(panel.includes('window.clearInterval(timer)'), 'close modal must stop polling')
assert.ok(panel.includes('/api/v1/admin/updates/client-logs'))
assert.ok(panel.includes('历史跳过无法追溯'))
assert.ok(panel.includes('不是历史执行日志'), 'current eligibility must not masquerade as historical logs')
assert.ok(panel.includes('强制'), 'force update logs must be visibly marked')
assert.ok(modal.includes('强制升级 Client'), 'force update button must be present')
assert.ok(modal.includes('确认强制升级'), 'force update must require confirmation')
console.log('client update log skip/search/filter, installer launch vs confirmed success and polling safeguards passed')
