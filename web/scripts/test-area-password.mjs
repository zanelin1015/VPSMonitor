import assert from 'node:assert/strict'
import { fileURLToPath } from 'node:url'
import { createServer } from 'vite'
import { createElement as h } from 'react'
import { act, create } from 'react-test-renderer'
import { App as AntApp } from 'antd'

const originalWindow = globalThis.window
const originalFetch = globalThis.fetch
const originalUseApp = AntApp.useApp
const messages = []
AntApp.useApp = () => ({ message: { error: text => messages.push(text), success: text => messages.push(text) } })
globalThis.window = { location: new URL('http://localhost/zanelin/monitor') }
const requests = []
let failure = false
const user = { id: 3, username: 'area', role: 'area_manager' }
globalThis.fetch = async (url, init) => {
  requests.push({ url, init, body: JSON.parse(init.body) })
  return { ok: !failure, status: failure ? 400 : 200, json: async () => failure ? { error: '当前密码不正确' } : { user } }
}
const vite = await createServer({
  root: fileURLToPath(new URL('../', import.meta.url)),
  configFile: fileURLToPath(new URL('../vite.config.ts', import.meta.url)),
  logLevel: 'silent',
  optimizeDeps: { noDiscovery: true, include: [] },
  ssr: { optimizeDeps: { noDiscovery: true, include: [] } },
})
let renderer
try {
  const { useAdminSession } = await vite.ssrLoadModule('/src/hooks/useAdminSession.ts')
  const { PersonalCenterDropdown } = await vite.ssrLoadModule('/src/components/AdminModals.tsx')
  let opened = 0
  const dropdown = PersonalCenterDropdown({ adminUser: user, canManageSystem: false, onOpenAccount: () => opened++ })
  assert.equal(dropdown.props.menu.items.find(item => item.key === 'account').label, '修改密码')
  assert.ok(!dropdown.props.menu.items.some(item => item.key === 'updates'))
  dropdown.props.menu.onClick({ key: 'account' })
  assert.equal(opened, 1, 'password menu opens the account dialog')
  let session
  function Harness() { session = useAdminSession(); return null }
  await act(async () => { renderer = create(h(Harness)) })
  await act(async () => session.setAdminUser(user))
  const form = { current_password: 'old-password', new_password: 'new-password', confirm_password: 'new-password', new_username: 'ignored', avatar_url: 'ignored' }
  async function setForm(patch = {}) {
    await act(async () => { session.setAccountForm({ ...form, ...patch }); session.setAccountModalOpen(true) })
  }
  for (const patch of [{ current_password: '' }, { new_password: 'short' }, { confirm_password: 'mismatch' }]) {
    await setForm(patch)
    await act(async () => session.saveAccount())
    assert.equal(requests.length, 0, 'invalid password fields never send a request')
    assert.equal(session.accountModalOpen, true)
  }
  await setForm()
  failure = true
  await act(async () => session.saveAccount())
  assert.equal(session.accountModalOpen, true, 'failed submission keeps the form available')
  assert.equal(session.adminUser.id, user.id, 'wrong current password does not log the user out')
  assert.equal(messages.at(-1), '当前密码不正确')
  failure = false
  await act(async () => session.saveAccount())
  assert.equal(requests.at(-1).url, '/zanelin/api/v1/admin/password')
  assert.equal(requests.at(-1).init.method, 'PUT')
  assert.deepEqual(requests.at(-1).body, { current_password: form.current_password, new_password: form.new_password }, 'area request contains no account or profile fields')
  assert.equal(session.accountModalOpen, false)
  assert.equal(session.accountForm.current_password, '')
  assert.equal(session.accountForm.new_password, '')
  assert.equal(session.accountForm.confirm_password, '')
  assert.equal(messages.at(-1), '密码已修改')
  await act(async () => session.setAdminUser({ ...user, role: 'admin' }))
  await setForm()
  await act(async () => session.saveAccount())
  assert.equal(requests.at(-1).url, '/zanelin/api/v1/admin/account', 'root profile editing keeps its existing API')
  assert.equal(requests.at(-1).body.new_username, form.new_username)
  console.log('area password menu, validation, scoped request, errors and root account compatibility passed')
} finally {
  renderer?.unmount()
  AntApp.useApp = originalUseApp
  if (originalWindow === undefined) delete globalThis.window
  else globalThis.window = originalWindow
  globalThis.fetch = originalFetch
  await vite.close()
}
