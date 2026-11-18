import assert from 'node:assert/strict'
import { fileURLToPath } from 'node:url'
import { createServer } from 'vite'
import { createElement as h, useState } from 'react'
import { act, create } from 'react-test-renderer'

const originalWindow = globalThis.window
const originalFetch = globalThis.fetch
globalThis.window = { location: new URL('http://localhost/') }
const requests = []
const messages = []
let failure = false
globalThis.fetch = async (url, init) => {
  const body = JSON.parse(init.body)
  requests.push({ url, method: init.method, body })
  return Response.json(failure ? { error: '链路超出接收区域的授权范围' } : { id: 12, ...body }, { status: failure ? 400 : 200 })
}
const vite = await createServer({
  root: fileURLToPath(new URL('../', import.meta.url)),
  configFile: fileURLToPath(new URL('../vite.config.ts', import.meta.url)),
  logLevel: 'silent',
  optimizeDeps: { noDiscovery: true, include: [] },
  ssr: { optimizeDeps: { noDiscovery: true, include: [] } },
  server: { host: '127.0.0.1', port: 0 },
})
let renderer
try {
  const { useCustomerAccounts } = await vite.ssrLoadModule('/src/hooks/useCustomerAccounts.ts')
  const { CustomerManagementModals } = await vite.ssrLoadModule('/src/components/CustomerManagementModals.tsx')
  const { emptyAreaManagerForm, emptyCustomerForm } = await vite.ssrLoadModule('/src/components/CustomerManagementHelpers.tsx')
  let accounts
  let customerLoads = 0
  let areaLoads = 0
  const noop = () => {}
  function Harness({ root }) {
    const [selectedCustomerID, setSelectedCustomerID] = useState(null)
    accounts = useCustomerAccounts({
      canManageAreaManagers: root,
      selectedCustomerID, setSelectedCustomerID, setCustomers: noop, setAreaManagers: noop,
      loadCustomers: async () => customerLoads++, loadAreaManagers: async () => areaLoads++, onCustomerDeleted: noop,
      message: { success: noop, warning: text => messages.push(text), error: text => messages.push(text) },
    })
    return null
  }
  await act(async () => { renderer = create(h(Harness, { root: true })) })
  const record = { id: 12, username: 'alice', enabled: true, owner_type: 'area_manager', owner_id: 4, front_proxies: [{ id: 6 }] }
  await act(async () => accounts.openCustomerEditModal(record))
  assert.equal(accounts.customerForm.owner_area_manager_id, 4, 'editing loads the existing owner instead of defaulting to root')
  await act(async () => accounts.setCustomerForm(current => ({ ...current, owner_area_manager_id: 5 })))
  failure = true
  await act(async () => accounts.saveCustomer())
  assert.equal(requests.at(-1).body.owner_area_manager_id, 5)
  assert.equal(accounts.customerEditModalOpen, true, 'a blocked transfer leaves the editor open')
  assert.equal(accounts.customerForm.owner_area_manager_id, 5, 'a blocked transfer preserves the selection')
  assert.equal(messages.at(-1), '链路超出接收区域的授权范围')
  assert.equal(customerLoads, 0, 'a rejected save cannot refresh away the pending form')
  failure = false
  await act(async () => accounts.saveCustomer())
  assert.equal(accounts.customerEditModalOpen, false)
  assert.equal(customerLoads, 1)
  assert.equal(areaLoads, 1, 'a successful transfer refreshes the area customer groups')
  await act(async () => accounts.openCustomerEditModal(record))
  await act(async () => accounts.setCustomerForm(current => ({ ...current, owner_area_manager_id: 0 })))
  await act(async () => accounts.saveCustomer())
  assert.equal(requests.at(-1).body.owner_area_manager_id, 0, 'zero explicitly transfers back to the root')
  await act(async () => accounts.openCustomerCreateModal())
  assert.equal(accounts.customerCreateForm.owner_area_manager_id, 0)
  await act(async () => accounts.setCustomerCreateForm(current => ({ ...current, username: 'new-a', owner_area_manager_id: 4 })))
  await act(async () => accounts.createCustomer())
  assert.equal(requests.at(-1).method, 'POST')
  assert.equal(requests.at(-1).body.owner_area_manager_id, 4)
  await act(async () => accounts.toggleCustomerStatus(record))
  assert.deepEqual(requests.at(-1).body, { enabled: false }, 'status changes leave ownership untouched')

  await act(async () => renderer.update(h(Harness, { root: false })))
  await act(async () => accounts.openCustomerEditModal(record))
  await act(async () => accounts.saveCustomer())
  assert.ok(!Object.hasOwn(requests.at(-1).body, 'owner_area_manager_id'), 'area/operator updates never request ownership changes')
  await act(async () => accounts.openCustomerCreateModal())
  await act(async () => accounts.setCustomerCreateForm(current => ({ ...current, username: 'area-new', owner_area_manager_id: 99 })))
  await act(async () => accounts.createCustomer())
  assert.ok(!Object.hasOwn(requests.at(-1).body, 'owner_area_manager_id'), 'area/operator creation uses server-assigned ownership')

  const props = {
    agents: [], areaManagerForm: emptyAreaManagerForm, customerForm: emptyCustomerForm, customerCreateForm: emptyCustomerForm,
    areaManagerRealmGrantOptions: [], selectedSubscriptionAssignmentIDs: [], customerOwnerOptions: [{ value: 0, label: '总管理员' }, { value: 4, label: '区域 B' }],
    selectedCustomerAssignments: [], readOnlyAssignmentColumns: [],
    setCustomerForm: update => accounts.setCustomerForm(update), setCustomerCreateForm: update => accounts.setCustomerCreateForm(update),
  }
  function ownerSelectors(element, result = []) {
    if (Array.isArray(element)) element.forEach(child => ownerSelectors(child, result))
    else if (element && typeof element === 'object') {
      if (['新用户所属区域', '用户所属区域'].includes(element.props?.['aria-label'])) result.push(element)
      ownerSelectors(element.props?.children, result)
    }
    return result
  }
  const selectors = ownerSelectors(CustomerManagementModals({ ...props, canManageAreaManagers: true }))
  assert.equal(selectors.length, 2, 'root has ownership selectors in both create and edit dialogs')
  assert.equal(ownerSelectors(CustomerManagementModals({ ...props, canManageAreaManagers: false })).length, 0, 'area/operator has no ownership control')
  await act(async () => selectors.find(select => select.props['aria-label'] === '用户所属区域').props.onChange(4))
  assert.equal(accounts.customerForm.owner_area_manager_id, 4, 'the selector changes the submitted form state')
  console.log('customer owner selection, transfer requests, failure recovery, directory refresh and area isolation passed')
} finally {
  renderer?.unmount()
  if (originalWindow === undefined) delete globalThis.window
  else globalThis.window = originalWindow
  globalThis.fetch = originalFetch
  await vite.close()
}
