import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import ts from 'typescript'

const sourcePath = new URL('../src/lib/currency.ts', import.meta.url)
const source = await readFile(sourcePath, 'utf8')
const transpiled = ts.transpileModule(source, {
  compilerOptions: {
    target: ts.ScriptTarget.ES2022,
    module: ts.ModuleKind.ES2022,
    importsNotUsedAsValues: ts.ImportsNotUsedAsValues.Remove,
    verbatimModuleSyntax: false,
  },
}).outputText

const dir = await mkdtemp(join(tmpdir(), 'vpsmonitor-finance-'))
try {
  await writeFile(join(dir, 'currency.mjs'), transpiled, 'utf8')
  const { summarizeMonthlyFinance, buildMonthlyFinanceRevenueDetails } = await import(`file://${join(dir, 'currency.mjs')}`)

  const exchangeRates = { base: 'USD', date: '2026-06-18', rates: { USD: 1, USDT: 1, CNY: 0.14 }, loading: false, error: '' }
  const agents = [
    agent('a', 10, [{ inbound_id: 1, inbound_tag: 'node-a', email: 'user-a', revenue_amount: 5, revenue_currency: 'USDT', revenue_cycle: 'month' }]),
    agent('b', 20, [{ inbound_id: 2, inbound_tag: 'node-b', email: 'unassigned-b', revenue_amount: 12, revenue_currency: 'USDT', revenue_cycle: 'month' }]),
    agent('c', 30, [{ inbound_id: 3, inbound_tag: 'node-c', email: 'area-c', revenue_amount: 100, revenue_currency: 'USDT', revenue_cycle: 'month' }]),
  ]
  const chains = [
    chain('a', 'node-a', 'user-a'),
    chain('b', 'node-b', 'unassigned-b'),
    chain('c', 'node-c', 'area-c'),
  ]
  const customers = [
    customer(1, 'admin', 1, [{ agent_id: 'a', inbound_id: 1, inbound_tag: 'node-a', client_email: 'user-a' }]),
    customer(2, 'area_manager', 7, [{ agent_id: 'c', inbound_id: 3, inbound_tag: 'node-c', client_email: 'area-c' }]),
  ]

  const areaManagersWithAccountBilling = [areaManager(7, true, 100, 'USDT', 'quarter')]
  const summaryWithAreaBilling = summarizeMonthlyFinance(agents, chains, 'USD', exchangeRates, customers, areaManagersWithAccountBilling)
  approx(summaryWithAreaBilling.costTotal, 60, 'all VPS costs are included')
  approx(summaryWithAreaBilling.revenueTotal, 50.3333333333, 'single-user, unassigned, and area-account revenues are included without duplicated area nodes')
  approx(summaryWithAreaBilling.profitTotal, -9.6666666667, 'profit is all revenue minus all VPS costs')

  const rowsWithAreaBilling = buildMonthlyFinanceRevenueDetails(agents, chains, 'USD', exchangeRates, customers, areaManagersWithAccountBilling)
  assert.deepEqual(rowsWithAreaBilling.map((row) => row.source).sort(), ['area_account', 'client', 'client'])
  assert(rowsWithAreaBilling.some((row) => row.clientEmail === 'unassigned-b'), 'unassigned charged node should stay in admin revenue')
  assert(!rowsWithAreaBilling.some((row) => row.clientEmail === 'area-c'), 'area-owned node should not duplicate account-level area revenue')

  const areaManagersWithoutAccountBilling = [areaManager(7, false, 100, 'USDT', 'quarter')]
  const summaryWithoutAreaBilling = summarizeMonthlyFinance(agents, chains, 'USD', exchangeRates, customers, areaManagersWithoutAccountBilling)
  approx(summaryWithoutAreaBilling.revenueTotal, 117, 'area-owned node revenue is counted when account-level area billing is disabled')

  console.log('finance tests passed')
} finally {
  await rm(dir, { recursive: true, force: true })
}

function agent(agent_id, cost, client_billings) {
  return {
    agent_id,
    agent_name: agent_id.toUpperCase(),
    tags: [],
    renewal: { cost_amount: cost, cost_currency: 'USD', cost_cycle: 'month', client_billings },
    summary: {},
  }
}

function chain(agentID, inboundTag, email) {
  return {
    key: `${agentID}:${inboundTag}:${email}`,
    root_agent_id: agentID,
    root_inbound_tag: inboundTag,
    root_client_email: email,
    root_client_remark: email,
    root_client_enabled: true,
    steps: [{ step_type: 'inbound', agent_id: agentID, label: inboundTag, detail: inboundTag }],
  }
}

function customer(id, ownerType, ownerID, assignments) {
  return {
    id,
    username: `customer-${id}`,
    display_name: `Customer ${id}`,
    owner_type: ownerType,
    owner_id: ownerID,
    enabled: true,
    assignments: assignments.map((assignment, index) => ({ id: index + 1, customer_id: id, enabled: true, ...assignment })),
    created_at: '',
    updated_at: '',
  }
}

function areaManager(id, billingEnabled, revenueAmount, revenueCurrency, revenueCycle) {
  return {
    id,
    username: `area-${id}`,
    display_name: `Area ${id}`,
    enabled: true,
    billing_enabled: billingEnabled,
    revenue_amount: revenueAmount,
    revenue_currency: revenueCurrency,
    revenue_cycle: revenueCycle,
    created_at: '',
    updated_at: '',
  }
}

function approx(actual, expected, message) {
  assert(Math.abs(actual - expected) < 1e-6, `${message}: expected ${expected}, got ${actual}`)
}
