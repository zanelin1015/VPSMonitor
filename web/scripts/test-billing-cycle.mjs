import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import ts from 'typescript'

const dir = await mkdtemp(join(tmpdir(), 'billing-cycle-'))
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
  assert.deepEqual(Object.keys(defaultScheduledTaskSettings()).sort(), ['alert_sweep', 'daily_traffic_report'])
  const upgraded = normalizeScheduledTaskSettings({
    traffic_reset: { enabled: true, catch_up_minutes: 360 },
    alert_sweep: { enabled: false, interval_minutes: 10 },
    daily_traffic_report: { enabled: true, time_of_day: '09:00', interval_days: 2 },
  })
  assert.ok(!('traffic_reset' in upgraded), 'legacy reset settings must not be re-saved')
  assert.deepEqual(upgraded.alert_sweep, { enabled: false, interval_minutes: 10 })
  assert.deepEqual(upgraded.daily_traffic_report, { enabled: true, time_of_day: '09:00', interval_days: 2 })
  console.log('billing timezone, half-year, fixed expiry, multiplier and scheduled-task upgrade tests passed')
} finally { await rm(dir, { recursive: true, force: true }) }
