import assert from 'node:assert/strict'
import { fileURLToPath } from 'node:url'
import { createServer } from 'vite'
import { createElement as h } from 'react'
import { act, create } from 'react-test-renderer'
import { App as AntApp } from 'antd'

const originalWindow = globalThis.window
const originalFetch = globalThis.fetch
const originalUseApp = AntApp.useApp
const notices = []
AntApp.useApp = () => ({ message: Object.fromEntries(['success', 'warning', 'error', 'info'].map(kind => [kind, text => notices.push([kind, text])])) })
globalThis.window = { location: new URL('http://localhost/zanelin/monitor') }
const vite = await createServer({
  root: fileURLToPath(new URL('../', import.meta.url)),
  configFile: fileURLToPath(new URL('../vite.config.ts', import.meta.url)),
  logLevel: 'silent', optimizeDeps: { noDiscovery: true, include: [] }, ssr: { optimizeDeps: { noDiscovery: true, include: [] } },
})
let renderer
try {
  const { PersonalCenterDropdown } = await vite.ssrLoadModule('/src/components/AdminModals.tsx')
  const area = { id: 3, username: 'area', role: 'area_manager' }
  let opened = 0
  const menu = PersonalCenterDropdown({ adminUser: area, canManageSystem: false, onOpenTelegram: () => opened++ }).props.menu
  assert.ok(menu.items.some(item => item?.key === 'telegram'), 'areas can open their own TG settings')
  menu.onClick({ key: 'telegram' })
  assert.equal(opened, 1)
  assert.ok(!PersonalCenterDropdown({ adminUser: { ...area, role: 'operator' }, canManageSystem: false }).props.menu.items.some(item => item?.key === 'telegram'), 'operators do not gain bot access')

  const { useAdminSystemTools } = await vite.ssrLoadModule('/src/hooks/useAdminSystemTools.ts')
  let tools, signedOut = 0
  function Harness({ user }) { tools = useAdminSystemTools(() => signedOut++, user); return null }
  let releaseOld
  globalThis.fetch = async () => new Promise(resolve => { releaseOld = () => resolve(Response.json([{ id: 1, name: 'admin-private-chat' }])) })
  await act(async () => { renderer = create(h(Harness, { user: { id: 1, username: 'admin', role: 'admin' } })) })
  const oldRead = tools.loadTelegramBots()
  await act(async () => { tools.setTelegramBotForm({ name: 'admin secret', bot_token: 'admin-token', chat_id: 'admin-chat', enabled: true, notify_expiry: true, notify_support: true }) })
  await act(async () => { renderer.update(h(Harness, { user: area })) })
  assert.equal(tools.telegramBotForm.bot_token, '', 'account changes clear secret input')
  assert.deepEqual(tools.telegramBots, [], 'previous account bot metadata is hidden')
  const requests = []
  const bot = { id: 3, name: 'area-bot', has_bot_token: true, chat_id: 'area-chat', enabled: true, notify_expiry: false, notify_support: true }
  globalThis.fetch = async (url, init) => {
    requests.push({ url: String(url), method: init?.method || 'GET', body: init?.body ? JSON.parse(init.body) : undefined })
    return Response.json(init?.method ? bot : [bot])
  }
  await act(async () => { await tools.loadTelegramBots() })
  await act(async () => { releaseOld(); await oldRead })
  assert.deepEqual(tools.telegramBots, [bot], 'late root response cannot replace area bot metadata')
  await act(async () => { tools.setTelegramBotForm({ name: 'area-bot', bot_token: 'area-token', chat_id: 'area-chat', enabled: true, notify_expiry: false, notify_support: true }) })
  await act(async () => { await tools.saveTelegramBot() })
  const created = requests.find(item => item.method === 'POST')
  assert.equal(created.url, '/zanelin/api/v1/admin/telegram-bots')
  assert.deepEqual(created.body, { name: 'area-bot', bot_token: 'area-token', chat_id: 'area-chat', enabled: true, notify_expiry: false, notify_support: true })
  assert.ok(!('owner_id' in created.body) && !('area_manager_id' in created.body), 'ownership is always assigned by the authenticated server session')
  assert.equal(tools.telegramBotForm.bot_token, '', 'successful save clears the token input')
  assert.equal(signedOut, 0)
  await act(async () => { renderer.unmount() }); renderer = null
  console.log('area TG menu, scoped state, late response isolation and notification preferences passed')
} finally {
  if (renderer) await act(async () => renderer.unmount())
  globalThis.window = originalWindow
  globalThis.fetch = originalFetch
  AntApp.useApp = originalUseApp
  await vite.close()
}
