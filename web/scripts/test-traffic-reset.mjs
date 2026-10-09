import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import ts from 'typescript'

const dir = await mkdtemp(join(tmpdir(), 'traffic-reset-'))
try {
  for (const name of ['currency', 'traffic', 'appHelpersBilling', 'scheduledTasks']) {
    const source = await readFile(new URL(`../src/lib/${name}.ts`, import.meta.url), 'utf8')
    const code = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ES2022 } }).outputText.replaceAll("'./currency'", "'./currency.mjs'").replaceAll("'./traffic'", "'./traffic.mjs'")
    await writeFile(join(dir, `${name}.mjs`), code)
  }
  const { dateInputToStartMillis, clientBillingPatchFromStart, effectiveClientBillingExpiryTime, normalizeClientBillings, scaleClientTraffic } = await import(`file://${join(dir, 'appHelpersBilling.mjs')}`)
  const { defaultScheduledTaskSettings, normalizeScheduledTaskSettings } = await import(`file://${join(dir, 'scheduledTasks.mjs')}`)
  for (const tz of ['UTC', 'Asia/Shanghai', 'America/New_York']) {
    process.env.TZ = tz
    const start = dateInputToStartMillis('2024-01-31')
    assert.equal(start, Date.parse('2024-01-31T00:00:00+08:00'), `start date in ${tz}`)
    assert.equal(clientBillingPatchFromStart(start, 'month').expire_time, Date.parse('2024-02-29T00:00:00+08:00'))
    assert.equal(clientBillingPatchFromStart(start, 'semiannual').expire_time, Date.parse('2024-07-31T00:00:00+08:00'))
    const fixed = { client_id: 'uuid', inbound_id: 1, email: 'alice', start_time: start, expire_time: 12345, revenue_cycle: 'month', expire_auto_renew: true }
    assert.equal(normalizeClientBillings([fixed])[0].expire_time, 12345, 'normalization must not auto-renew')
    assert.equal(effectiveClientBillingExpiryTime(fixed, 99999), 12345, 'rendering must preserve expired dates')
    assert.equal(effectiveClientBillingExpiryTime({ ...fixed, expire_time: 0 }, 99999), 0, 'manual unlimited override preserved')
  }
  assert.equal(scaleClientTraffic(200 * 1024 ** 3, 2), 400 * 1024 ** 3)
  assert.equal(scaleClientTraffic(0, 2), 0)
  assert.equal(defaultScheduledTaskSettings().traffic_reset.enabled, false)
  assert.deepEqual(normalizeScheduledTaskSettings({}).traffic_reset, { enabled: false, catch_up_minutes: 360 })
  const panel = await readFile(new URL('../src/components/TrafficResetPanel.tsx', import.meta.url), 'utf8')
  for (const text of ['流量重置日志', '计划执行时间（北京时间）', '详情 / 失败原因', '领取时间，实际执行时间未确认', '结果未确认', '跳过（未执行）']) {
    assert.ok(panel.includes(text), `audit log label: ${text}`)
  }
  assert.ok(panel.includes("timeZone: 'Asia/Shanghai'"), 'audit timestamps must display in Beijing time')
  assert.ok(panel.includes('data.logs || []'), 'older servers without logs must still render')
  console.log('traffic reset timezone, half-year, fixed expiry, defaults and multiplier tests passed')
  console.log('traffic reset audit log result, actual time/fallback and backward compatibility safeguards passed')
} finally { await rm(dir, { recursive: true, force: true }) }
