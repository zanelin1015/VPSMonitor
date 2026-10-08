import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import ts from 'typescript'

async function load(file, replace = value => value) {
  const source = await readFile(new URL(`../src/lib/${file}.ts`, import.meta.url), 'utf8')
  const compiled = replace(ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ES2022 } }).outputText)
  return import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)
}
const { customerMatchesSearch, existingCustomerAuthorization, customerAuthorizationPayload, authorizeCustomersBatch } = await load('customerBatchAuthorization')
const draft = { agent_id: 'agent-a', inbound_id: 1, client_id: 'client-a', client_email: 'email-a', public_client_name: 'A - email-a' }
const assignment = { ...draft, id: 9, customer_id: 1, enabled: false }
const user = (id, assignments = []) => ({ id, username: `user-${id}`, display_name: `显示名称 ${id}`, enabled: true, assignments })
assert.equal(customerMatchesSearch(user(1), ' USER-1 '), true)
assert.equal(customerMatchesSearch(user(1), '显示名称'), true)
assert.equal(customerMatchesSearch(user(1), 'missing'), false)
assert.equal(existingCustomerAuthorization(user(1, [assignment]), draft), assignment, 'disabled grants are not overwritten')
assert.equal(existingCustomerAuthorization(user(1, [{ ...assignment, client_id: '' }]), draft)?.id, 9, 'legacy email grants match')
for (const changed of [{ agent_id: 'other' }, { inbound_id: 2 }, { client_id: 'other' }]) {
  assert.equal(existingCustomerAuthorization(user(1, [{ ...assignment, ...changed }]), draft), undefined)
}
assert.equal(existingCustomerAuthorization(user(1, [assignment]), { agent_id: 'agent-a', inbound_id: 1 }), undefined, 'whole-node and specific-client grants are distinct')
assert.deepEqual(customerAuthorizationPayload({ ...draft, revenue_amount: 100, traffic_multiplier: 2, price_mode: 'override' }), {
  ...draft, inbound_tag: '', enabled: true, price_mode: 'inherit',
})
const customers = [user(1, [assignment]), ...Array.from({ length: 6 }, (_, index) => user(index + 2))]
const posted = []
let active = 0
let maxActive = 0
let completed = 0
const result = await authorizeCustomersBatch(customers, [1, 2, 2, 3, 4, 5, 6, 7, 999], draft, async (id, payload) => {
  posted.push(id)
  maxActive = Math.max(maxActive, ++active)
  await new Promise(resolve => setTimeout(resolve, 5))
  active--
  if (id === 3) throw new Error('权限不足')
  return { ...payload, customer_id: id, id: id + 100 }
}, (done, total) => { completed = done; assert.equal(total, 8) })
assert.equal(maxActive, 3, 'requests are bounded')
assert.equal(completed, 8)
assert.deepEqual(result.skippedIDs, [1])
assert.equal(result.created.length, 5)
assert.equal(result.failed.length, 2)
assert.match(result.failed.find(item => item.customerID === 3).error, /权限不足/)
assert.equal(posted.filter(id => id === 2).length, 1, 'duplicate selections post once')
assert.ok(!posted.includes(1) && !posted.includes(999))
const latest = customers.map(customer => ({ ...customer, assignments: [...customer.assignments, ...result.created.filter(item => item.customer_id === customer.id)] }))
const retry = await authorizeCustomersBatch(latest, [2, 3], draft, async (id, payload) => ({ ...payload, customer_id: id, id: 200 }))
assert.deepEqual(retry.skippedIDs, [2], 'retry skips writes already persisted')
assert.equal(retry.created[0].customer_id, 3)

const { createAppNavigationHandlers } = await load('appNavigation', code => code.replace(/import \{ nodeElementId, outboundElementId, ruleElementId \} from '.\/appHelpersAgent';/, 'const nodeElementId = () => ""; const outboundElementId = () => ""; const ruleElementId = () => "";'))
const changes = []
const setters = new Proxy({}, { get: (_, name) => (...args) => changes.push([name, ...args]) })
createAppNavigationHandlers(setters).openCustomerAuthorization(draft)
assert.deepEqual(changes, [['setCustomerAuthorizationDraft', draft]], 'authorization only opens modal, with no route/tab/search changes')
console.log('customer batch search, identity, dedupe, partial failure/retry, pricing and no-navigation tests passed')
