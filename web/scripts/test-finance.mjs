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
  const {
    summarizeMonthlyFinance,
    buildMonthlyFinanceExcludedRevenueDetails,
    buildMonthlyFinanceRevenueDetails,
    formatMoney,
    scopeAreaManagersToAgents,
  } = await import(`file://${join(dir, 'currency.mjs')}`)

  const exchangeRates = { base: 'USD', date: '2026-06-18', rates: { USD: 1, USDT: 1, CNY: 0.14 }, loading: false, error: '' }
  const agents = [
    agent('a', 10, [{ inbound_id: 1, inbound_tag: 'node-a', email: 'user-a', revenue_amount: 5, revenue_currency: 'USDT', revenue_cycle: 'month' }]),
    agent('b', 20, [{ inbound_id: 2, inbound_tag: 'node-b', email: 'unassigned-b', revenue_amount: 12, revenue_currency: 'USDT', revenue_cycle: 'month' }]),
    agent('c', 30, [{ inbound_id: 3, inbound_tag: 'node-c', email: 'area-c', revenue_amount: 100, revenue_currency: 'USDT', revenue_cycle: 'month' }]),
  ]
  const chains = [
    chain('a', 'node-a', 'user-a', 1),
    chain('b', 'node-b', 'unassigned-b', 2),
    chain('c', 'node-c', 'area-c', 3),
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


  const summaryWithoutChains = summarizeMonthlyFinance(agents, [], 'USD', exchangeRates, customers, areaManagersWithAccountBilling)
  approx(summaryWithoutChains.revenueTotal, 50.3333333333, 'embedded finance client states keep lightweight dashboard revenue complete')
  const rowsWithoutChains = buildMonthlyFinanceRevenueDetails(agents, [], 'USD', exchangeRates, customers, areaManagersWithAccountBilling)
  assert(rowsWithoutChains.some((row) => row.source === 'client' && row.clientEmail === 'user-a'), 'ordinary billing should be verified from embedded client state')
  assert(rowsWithoutChains.some((row) => row.source === 'client' && row.clientEmail === 'unassigned-b'), 'unassigned billing should be verified from embedded client state')
  assert(!rowsWithoutChains.some((row) => row.clientEmail === 'area-c'), 'area-owned billing should still be skipped when account-level area billing is enabled')

  const areaManagersWithoutAccountBilling = [areaManager(7, false, 100, 'USDT', 'quarter')]
  const summaryWithoutAreaBilling = summarizeMonthlyFinance(agents, chains, 'USD', exchangeRates, customers, areaManagersWithoutAccountBilling)
  approx(summaryWithoutAreaBilling.revenueTotal, 117, 'area-owned node revenue is counted when account-level area billing is disabled')

  const scopedManagers = [
    areaManager(7, true, 100, 'USDT', 'quarter', ['c']),
    areaManager(8, true, 60, 'USDT', 'month', ['outside']),
  ]
  assert.deepEqual(
    scopeAreaManagersToAgents(scopedManagers, [agents[2]], true).map((manager) => manager.id),
    [7],
    'tag-scoped finance only includes area managers assigned to an agent in scope',
  )
  assert.equal(
    scopeAreaManagersToAgents(scopedManagers, [agents[2]], false).length,
    2,
    'global finance keeps every billed area manager',
  )

  const statusBillings = [
    { inbound_id: 1, inbound_tag: 'node-1', email: 'active', revenue_amount: 10, revenue_currency: 'USDT', revenue_cycle: 'month', start_time: monthTimestamp(0, 15), expire_time: cycleExpiry(monthTimestamp(0, 15), 1) },
    { inbound_id: 1, inbound_tag: 'node-1', email: 'disabled', revenue_amount: 20, revenue_currency: 'USDT', revenue_cycle: 'month' },
    { inbound_id: 1, inbound_tag: 'node-1', email: 'missing', revenue_amount: 30, revenue_currency: 'USDT', revenue_cycle: 'month' },
    { inbound_id: 1, inbound_tag: 'node-1', email: 'shared', revenue_amount: 15, revenue_currency: 'USDT', revenue_cycle: 'month' },
    { inbound_id: 3, inbound_tag: 'node-3', email: 'closed-node', revenue_amount: 40, revenue_currency: 'USDT', revenue_cycle: 'month' },
  ]
  const statusClients = [
    financeClient(1, 'node-1', 'active', true),
    financeClient(1, 'node-1', 'disabled', false),
    financeClient(1, 'node-1', 'shared', true),
    financeClient(2, 'node-2', 'shared', true),
    financeClient(3, 'node-3', 'closed-node', true, false),
  ]
  const statusAgent = agent('status', 0, statusBillings, statusClients)
  const statusChains = statusClients.map((client) => chain('status', client.inbound_tag, client.email, client.inbound_id, client.enabled, client.node_enabled))
  const lightweightStatus = summarizeMonthlyFinance([statusAgent], [], 'USD', exchangeRates)
  const topologyStatus = summarizeMonthlyFinance([statusAgent], statusChains, 'USD', exchangeRates)
  approx(lightweightStatus.revenueTotal, 25, 'only enabled, exact client matches are counted')
  approx(topologyStatus.revenueTotal, 25, 'opening topology does not change finance totals')
  assert.equal(lightweightStatus.excludedRevenueCount, 3, 'disabled clients, missing clients, and closed nodes are excluded')
  assert.deepEqual(
    buildMonthlyFinanceExcludedRevenueDetails([statusAgent], [], 'USD', exchangeRates).map((row) => row.reason).sort(),
    ['client_disabled', 'client_not_found', 'node_disabled'],
    'excluded revenue explains stale clients, disabled clients, and closed nodes',
  )
  const activeRow = buildMonthlyFinanceRevenueDetails([statusAgent], [], 'USD', exchangeRates).find((row) => row.clientEmail === 'active')
  assert(activeRow?.payment?.date.endsWith('-15'), 'client payment date follows billing start day')

  const duplicateBillingAgent = agent('duplicate', 0, [statusBillings[0], { ...statusBillings[0] }], [financeClient(1, 'node-1', 'active', true)])
  const duplicateSummary = summarizeMonthlyFinance([duplicateBillingAgent], [], 'USD', exchangeRates)
  approx(duplicateSummary.revenueTotal, 10, 'duplicate billing records are counted once')
  assert.equal(duplicateSummary.excludedRevenueCount, 1, 'duplicate billing is visible as excluded revenue')

  const sharedAgent = agent('shared', 0, [
    { inbound_id: 1, inbound_tag: 'node-1', email: 'same-email', revenue_amount: 100, revenue_currency: 'USDT', revenue_cycle: 'month' },
    { inbound_id: 2, inbound_tag: 'node-2', email: 'same-email', revenue_amount: 50, revenue_currency: 'USDT', revenue_cycle: 'month' },
  ], [
    financeClient(1, 'node-1', 'same-email', true),
    financeClient(2, 'node-2', 'same-email', true),
  ])
  const areaOwnedCustomer = customer(9, 'area_manager', 7, [{ agent_id: 'shared', inbound_id: 1, inbound_tag: 'node-1', client_email: 'same-email' }])
  const strictOwnershipSummary = summarizeMonthlyFinance([sharedAgent], [], 'USD', exchangeRates, [areaOwnedCustomer], [areaManager(7, true, 30, 'USDT', 'month')])
  approx(strictOwnershipSummary.revenueTotal, 80, 'same email on another inbound keeps its independent revenue')
  areaOwnedCustomer.assignments[0].enabled = false
  const disabledAssignmentSummary = summarizeMonthlyFinance([sharedAgent], [], 'USD', exchangeRates, [areaOwnedCustomer], [areaManager(7, true, 30, 'USDT', 'month')])
  approx(disabledAssignmentSummary.revenueTotal, 180, 'disabled assignments do not suppress node revenue')

  const legacyAgent = { ...statusAgent }
  delete legacyAgent.finance_clients
  const legacyLightweight = summarizeMonthlyFinance([legacyAgent], [], 'USD', exchangeRates)
  approx(legacyLightweight.revenueTotal, 0, 'unknown client state is never guessed from billing configuration')
  assert.equal(legacyLightweight.excludedRevenueCount, 5, 'unknown client state is reported instead of counted')
  const legacyTopology = summarizeMonthlyFinance([legacyAgent], statusChains, 'USD', exchangeRates)
  approx(legacyTopology.revenueTotal, 25, 'older servers can verify revenue after topology is available')

  const unavailableAgent = { ...statusAgent, finance_clients: [], finance_clients_ready: false }
  const unavailableRows = buildMonthlyFinanceExcludedRevenueDetails([unavailableAgent], [], 'USD', exchangeRates)
  assert(unavailableRows.every((row) => row.reason === 'client_state_unavailable'), 'failed x-ui collection reports unavailable state instead of deleted clients')

  const coveredQuarterStart = monthTimestamp(-2, 5)
  const coveredSemiannualStart = monthTimestamp(-5, 5)
  const finishedQuarterStart = monthTimestamp(-3, 5)
  const futureMonthStart = monthTimestamp(1, 5)
  const periodBillings = [
    { inbound_id: 1, inbound_tag: 'period', email: 'covered-quarter', revenue_amount: 300, revenue_currency: 'USDT', revenue_cycle: 'quarter', start_time: coveredQuarterStart, expire_time: cycleExpiry(coveredQuarterStart, 3) },
    { inbound_id: 1, inbound_tag: 'period', email: 'covered-semiannual', revenue_amount: 600, revenue_currency: 'USDT', revenue_cycle: 'semiannual', start_time: coveredSemiannualStart, expire_time: cycleExpiry(coveredSemiannualStart, 6) },
    { inbound_id: 1, inbound_tag: 'period', email: 'finished-quarter', revenue_amount: 300, revenue_currency: 'USDT', revenue_cycle: 'quarter', start_time: finishedQuarterStart, expire_time: cycleExpiry(finishedQuarterStart, 3) },
    { inbound_id: 1, inbound_tag: 'period', email: 'renewed-quarter', revenue_amount: 600, revenue_currency: 'USDT', revenue_cycle: 'quarter', start_time: finishedQuarterStart, expire_time: cycleExpiry(finishedQuarterStart, 6) },
    { inbound_id: 1, inbound_tag: 'period', email: 'future-month', revenue_amount: 90, revenue_currency: 'USDT', revenue_cycle: 'month', start_time: futureMonthStart, expire_time: cycleExpiry(futureMonthStart, 1) },
  ]
  const periodAgent = agent('period', 0, periodBillings, periodBillings.map((billing) => financeClient(1, 'period', billing.email, true)))
  const periodSummary = summarizeMonthlyFinance([periodAgent], [], 'USD', exchangeRates)
  approx(periodSummary.revenueTotal, 400, 'quarterly and semiannual revenue is spread only across covered calendar months')
  assert.equal(periodSummary.excludedRevenueCount, 2, 'finished and future billing periods are excluded from the current month')
  assert.deepEqual(
    buildMonthlyFinanceExcludedRevenueDetails([periodAgent], [], 'USD', exchangeRates).map((row) => row.reason),
    ['outside_billing_period', 'outside_billing_period'],
    'out-of-period revenue is explained in finance details',
  )

  assert.match(formatMoney(1410.62, 'USD'), /1[,.]?410[.,]62/, 'financial totals preserve two decimals')

  console.log('finance tests passed')
} finally {
  await rm(dir, { recursive: true, force: true })
}

function agent(agent_id, cost, client_billings, finance_clients = client_billings.map((billing) => ({
  inbound_id: billing.inbound_id,
  inbound_tag: billing.inbound_tag,
  inbound_remark: billing.inbound_tag,
  email: billing.email,
  comment: billing.email,
  enabled: true,
}))) {
  return {
    agent_id,
    agent_name: agent_id.toUpperCase(),
    tags: [],
    renewal: { cost_amount: cost, cost_currency: 'USD', cost_cycle: 'month', client_billings },
    finance_clients,
    summary: {},
  }
}

function chain(agentID, inboundTag, email, inboundID = Number(inboundTag.replace(/\D+/g, '')) || 0, enabled = true, nodeEnabled = true) {
  return {
    key: `${agentID}::${inboundID}::${email}`,
    root_agent_id: agentID,
    root_inbound_id: inboundID,
    root_inbound_tag: inboundTag,
    root_client_email: email,
    root_client_remark: email,
    root_client_enabled: enabled,
    root_inbound_enabled: nodeEnabled,
    steps: [{ step_type: 'inbound', agent_id: agentID, label: inboundTag, detail: inboundTag }],
  }
}

function financeClient(inbound_id, inbound_tag, email, enabled, node_enabled = true) {
  return { inbound_id, inbound_tag, inbound_remark: inbound_tag, email, comment: email, enabled, node_enabled }
}

function monthTimestamp(offset, day) {
  const now = new Date()
  return new Date(now.getFullYear(), now.getMonth() + offset, day).getTime()
}

function cycleExpiry(startTime, months) {
  const start = new Date(startTime)
  return new Date(start.getFullYear(), start.getMonth() + months, start.getDate() - 1, 23, 59, 59).getTime()
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

function areaManager(id, billingEnabled, revenueAmount, revenueCurrency, revenueCycle, agentIDs = []) {
  return {
    id,
    username: `area-${id}`,
    display_name: `Area ${id}`,
    enabled: true,
    billing_enabled: billingEnabled,
    revenue_amount: revenueAmount,
    revenue_currency: revenueCurrency,
    revenue_cycle: revenueCycle,
    agent_ids: agentIDs,
    created_at: '',
    updated_at: '',
  }
}

function approx(actual, expected, message) {
  assert(Math.abs(actual - expected) < 1e-6, `${message}: expected ${expected}, got ${actual}`)
}
