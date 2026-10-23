import assert from 'node:assert/strict'
import { fileURLToPath } from 'node:url'
import { createServer } from 'vite'
import { createElement as h } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { App as AntApp, ConfigProvider } from 'antd'
import { getMenuData } from '@ant-design/pro-components'

const originalWindow = globalThis.window
const vite = await createServer({
  root: fileURLToPath(new URL('../', import.meta.url)),
  configFile: fileURLToPath(new URL('../vite.config.ts', import.meta.url)),
  logLevel: 'silent',
  optimizeDeps: { noDiscovery: true, include: [] },
  ssr: { optimizeDeps: { noDiscovery: true, include: [] } },
})
try {
  const { visibleAdminMenu, currentAdminMenu, adminMenu, adminMenuRoutes } =
    await vite.ssrLoadModule('/src/app/adminMenu.ts')
  const admin = visibleAdminMenu(true, false).map((item) => item.key)
  const area = visibleAdminMenu(false, true).map((item) => item.key)
  const operator = visibleAdminMenu(false, false, true).map((item) => item.key)
  assert.equal(new Set(admin).size, adminMenu.length)
  for (const key of ['finance', 'access-logs', 'settings', 'schedules']) {
    assert.ok(admin.includes(key))
    assert.ok(!area.includes(key), `area accounts cannot navigate to ${key}`)
  }
  assert.ok(area.includes('front-proxies'))
  assert.deepEqual(operator, ['customers', 'support'], 'operator accounts only see customer operations and support')
  assert.equal(currentAdminMenu('dashboard', true).key, 'topology')
  assert.equal(currentAdminMenu('assets', true).key, 'assets')

  // Exercise ProLayout's actual conversion: raw configuration checks miss dropped groups.
  const converted = getMenuData(adminMenuRoutes(true, false), { locale: false }).menuData
  assert.equal(converted.length, 4, 'all navigation groups survive ProLayout conversion')
  assert.deepEqual(
    converted.flatMap((group) => group.children.map((item) => item.path)),
    admin.map((key) => `/${key}`),
  )
  const convertedArea = getMenuData(adminMenuRoutes(false, true), { locale: false }).menuData
  assert.deepEqual(
    convertedArea.flatMap((group) => group.children.map((item) => item.path)),
    area.map((key) => `/${key}`),
  )
  const convertedOperator = getMenuData(adminMenuRoutes(false, false, true), { locale: false }).menuData
  assert.deepEqual(
    convertedOperator.flatMap((group) => group.children.map((item) => item.path)),
    operator.map((key) => `/${key}`),
  )

  const { AdminLayout } = await vite.ssrLoadModule('/src/app/AdminLayout.tsx')
  const { AppThemeProvider } = await vite.ssrLoadModule('/src/theme.tsx')
  const navigation = {
    adminUser: { id: 1, username: 'preview', role: 'admin' },
    systemInfo: null,
    canManageSystem: true,
    isAreaManagerAccount: false,
    activeAdminPage: 'assets',
    topologyVisible: false,
    onlineAgentCount: 1,
    scopedAgentCount: 1,
    supportUnreadCount: 0,
    agentsLoading: false,
    themeMode: 'dark',
    effectiveMode: 'dark',
    serverVersionLabel: 'test',
  }
  const renderLayout = (restricted) => {
    // This client-only layout legitimately uses layout effects. Only filter the
    // known SSR warning while checking the actual rendered menu, not other errors.
    const error = console.error
    console.error = (message, ...args) => {
      if (typeof message === 'string' && message.startsWith('Warning: useLayoutEffect does nothing on the server'))
        return
      error(message, ...args)
    }
    globalThis.window = {
      location: new URL('http://localhost/zanelin/monitor?page=assets'),
      matchMedia: () => ({ matches: false, addListener() {}, removeListener() {} }),
    }
    try {
      return renderToStaticMarkup(
        h(
          AppThemeProvider,
          {},
          h(
            ConfigProvider,
            {},
            h(AntApp, {}, h(AdminLayout, {
              navigation: { ...navigation, canManageSystem: !restricted, isAreaManagerAccount: restricted },
            }, 'page content')),
          ),
        ),
      )
    } finally {
      console.error = error
      if (originalWindow === undefined) delete globalThis.window
      else globalThis.window = originalWindow
    }
  }
  const layout = renderLayout(false)
  assert.match(layout, /role="menuitem"/, 'the real layout must render navigation items')
  assert.match(layout, /customer-theme-button/, 'the admin header must use the customer theme button')
  assert.match(layout, /href="\/zanelin\/monitor\?page=assets"/)
  assert.match(layout, /href="\/zanelin\/monitor\?page=finance"/)
  const areaLayout = renderLayout(true)
  assert.match(areaLayout, /href="\/zanelin\/monitor\?page=assets"/)
  assert.doesNotMatch(areaLayout, /href="[^"\s]+\?page=(finance|access-logs|schedules|settings)"/)

  const { AssetsPage } = await vite.ssrLoadModule('/src/pages/admin/AssetsPage.tsx')
  const agent = {
    agent_id: 'sample-hk',
    agent_name: '香港示例 Client',
    has_config: true,
    tags: ['演示'],
    node_count: 3,
    client_count: 7,
    online_client_count: 4,
    outbound_count: 1,
    routing_rule_count: 2,
    reported_at: new Date().toISOString(),
    last_seen_at: new Date().toISOString(),
    summary: { cpu: 83, mem_total: 8000, mem_used: 4000, net_io_up: 1024, net_io_down: 2048 },
    geo: { country_code: 'HK', country_name: 'admin-only-location' },
  }
  const props = {
    accountKey: 'preview',
    agents: [agent],
    loading: false,
    error: '',
    selectedTag: '',
    healthFilter: 'all',
    selectedAgentId: '',
    tagFilterOptions: [],
    viewMode: 'list',
    panelExpanded: false,
    topologyVisible: false,
    searchText: '',
    onToggleViewMode() {},
    onToggleTopology() {},
    onRefresh() {},
    onSelectTag() {},
    onHealthFilterChange() {},
    onSelectAgent() {},
    onSearchTextChange() {},
  }
  const render = (restrictedView) =>
    renderToStaticMarkup(h(ConfigProvider, {}, h(AntApp, {}, h(AssetsPage, { ...props, restrictedView }))))
  const full = render(false)
  assert.match(full, /香港示例 Client/)
  assert.match(full, /搜索 Client/)
  assert.match(full, /健康状态/)
  assert.match(full, /CPU/)
  assert.match(full, /硬盘/)
  assert.match(full, /admin-only-location/)
  assert.match(full, /class="country-flag"/)
  assert.match(full, /aria-label="admin-only-location"/)
  assert.match(full, /查看详情/)
  const restricted = render(true)
  assert.doesNotMatch(restricted, /CPU|内存|硬盘|实时网速|admin-only-location|mem_total/)
  assert.doesNotMatch(restricted, /class="country-flag"/)
  assert.match(restricted, /已授权/)
  assert.match(restricted, /香港示例 Client/)
  const { adminMenuHref } = await vite.ssrLoadModule('/src/app/adminMenu.ts')
  assert.equal(adminMenuHref('assets', '/'), '/?page=assets')
  assert.equal(adminMenuHref('assets', '/zanelin/monitor'), '/zanelin/monitor?page=assets')
  assert.equal(adminMenuHref('dashboard', '/zanelin/monitor'), '/zanelin/monitor')
  assert.equal(adminMenuHref('topology', '/zanelin/monitor'), '/zanelin/monitor?page=topology')
  console.log('ProLayout rendered menu/conversion, permissions, deployment prefixes and asset rendering tests passed')
} finally {
  if (originalWindow === undefined) delete globalThis.window
  else globalThis.window = originalWindow
  await vite.close()
}
