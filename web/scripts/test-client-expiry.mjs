import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import ts from 'typescript'

const sourcePath = new URL('../src/lib/clientExpiry.ts', import.meta.url)
const source = await readFile(sourcePath, 'utf8')
const transpiled = ts.transpileModule(source, {
  compilerOptions: {
    target: ts.ScriptTarget.ES2022,
    module: ts.ModuleKind.ES2022,
    importsNotUsedAsValues: ts.ImportsNotUsedAsValues.Remove,
    verbatimModuleSyntax: false,
  },
}).outputText

const dir = await mkdtemp(join(tmpdir(), 'vpsmonitor-client-expiry-'))
try {
  await writeFile(join(dir, 'clientExpiry.mjs'), transpiled, 'utf8')
  const {
    buildClientExpiryRows,
    clientExpiryCycleLabel,
    clientExpiryRemainingLabel,
  } = await import(`file://${join(dir, 'clientExpiry.mjs')}`)

  const now = new Date(2026, 6, 17, 12).getTime()
  const billings = [
    billing(1, 'monthly', 'month-user', 'month'),
    billing(2, 'quarterly', 'quarter-user', 'quarter'),
    billing(9, 'semiannual', 'semiannual-user', 'semiannual'),
    billing(3, 'safe', 'safe-user', 'month'),
    billing(4, 'disabled', 'disabled-user', 'month'),
    billing(5, 'closed-node', 'closed-node-user', 'month'),
    billing(6, 'expired', 'expired-user', 'month'),
  ]
  const agent = {
    agent_id: 'agent-a',
    agent_name: 'HK-A',
    summary: {},
    renewal: { client_billings: billings },
    finance_clients_ready: true,
    finance_clients: [
      client(1, 'monthly', 'month-user', day(now, 5)),
      client(2, 'quarterly', 'quarter-user', day(now, 4)),
      client(9, 'semiannual', 'semiannual-user', day(now, 2)),
      client(3, 'safe', 'safe-user', day(now, 6)),
      client(4, 'disabled', 'disabled-user', day(now, 2), false),
      client(5, 'closed-node', 'closed-node-user', day(now, 3), true, false),
      client(6, 'expired', 'expired-user', day(now, -1)),
      client(7, 'external', 'external-user', day(now, 1)),
    ],
  }
  const fallbackAgent = {
    agent_id: 'agent-b',
    agent_name: 'US-B',
    summary: {},
    renewal: { client_billings: [{ ...billing(8, 'fallback', 'fallback-user', 'quarter'), expire_time: day(now, 3) }] },
    finance_clients_ready: false,
    finance_clients: [],
  }

  const rows = buildClientExpiryRows([agent, fallbackAgent], now)
  assert.deepEqual(rows.map((row) => row.clientName), ['expired-user', 'external-user', 'semiannual-user', 'fallback-user', 'quarter-user', 'month-user'])
  assert.equal(rows.find((row) => row.clientName === 'quarter-user')?.cycle, 'quarter')
  assert.equal(rows.find((row) => row.clientName === 'semiannual-user')?.cycle, 'semiannual')
  assert.equal(rows.find((row) => row.clientName === 'external-user')?.cycle, '')
  assert(!rows.some((row) => row.clientName === 'safe-user'), 'clients beyond five days should not be listed')
  assert(!rows.some((row) => row.clientName === 'disabled-user'), 'disabled clients should not be listed')
  assert(!rows.some((row) => row.clientName === 'closed-node-user'), 'clients on disabled nodes should not be listed')
  assert.equal(clientExpiryCycleLabel('quarter'), '季付')
  assert.equal(clientExpiryCycleLabel('semiannual'), '半年付')
  assert.equal(clientExpiryRemainingLabel(0), '今天到期')
  assert.equal(clientExpiryRemainingLabel(-2), '已过期 2 天')

  console.log('client expiry tests passed')
} finally {
  await rm(dir, { recursive: true, force: true })
}

function billing(inbound_id, inbound_tag, email, revenue_cycle) {
  return { inbound_id, inbound_tag, email, revenue_cycle }
}

function client(inbound_id, inbound_tag, email, expiry_time, enabled = true, node_enabled = true) {
  return { inbound_id, inbound_tag, inbound_remark: inbound_tag, email, comment: email, expiry_time, enabled, node_enabled }
}

function day(value, offset) {
  const date = new Date(value)
  return new Date(date.getFullYear(), date.getMonth(), date.getDate() + offset, 23, 59, 59).getTime()
}
