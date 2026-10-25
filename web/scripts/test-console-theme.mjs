import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { build } from 'esbuild'

const bundled = await build({ entryPoints: [new URL('../src/lib/adminConsoleTheme.ts', import.meta.url).pathname], bundle: true, write: false, format: 'esm', platform: 'node', packages: 'external' })
const { adminConsoleTheme, consolePalette, consoleCSSVariables, isInternalConsoleRoute, isConsoleThemeRoute } = await import(`data:text/javascript;base64,${Buffer.from(bundled.outputFiles[0].text).toString('base64')}`)

for (const route of ['/', '/admin/assets', '/monitor']) assert.equal(isInternalConsoleRoute(route), true)
for (const route of ['/customer', '/recommendations']) assert.equal(isInternalConsoleRoute(route), false)
assert.equal(isInternalConsoleRoute('/', '?page=settings'), true)
for (const route of ['/', '/admin/assets', '/monitor', '/customer']) assert.equal(isConsoleThemeRoute(route), true)
assert.equal(isConsoleThemeRoute('/recommendations'), false)

function luminance(hex) {
  const rgb = hex.slice(1).match(/../g).map(value => parseInt(value, 16) / 255)
    .map(value => value <= .04045 ? value / 12.92 : ((value + .055) / 1.055) ** 2.4)
  return rgb[0] * .2126 + rgb[1] * .7152 + rgb[2] * .0722
}
function contrast(a, b) {
  const values = [luminance(a), luminance(b)].sort((a, b) => b - a)
  return (values[0] + .05) / (values[1] + .05)
}

for (const dark of [false, true]) {
  const theme = adminConsoleTheme(dark)
  const palette = dark ? consolePalette.dark : consolePalette.light
  assert.equal(theme.token.colorBgLayout, palette.background)
  assert.equal(theme.token.colorPrimary, palette.accent)
  assert.notEqual(theme.token.colorSuccess, theme.token.colorPrimary)
  assert.equal(theme.token.borderRadiusLG, 12)
  assert.equal(theme.token.controlHeightSM, 28)
  assert.equal(theme.token.motion, true)
  assert.equal(adminConsoleTheme(dark, true).token.motion, false)
  for (const color of [palette.text, palette.secondary, palette.muted]) {
    assert.ok(contrast(color, palette.raised) >= 4.5, `${color} is readable on ${palette.raised}`)
  }
  assert.ok(contrast(theme.components.Button.primaryColor, palette.accent) >= 4.5, 'primary button text contrast')
  assert.ok(contrast(theme.components.Button.dangerColor, palette.error) >= 4.5, 'danger button text contrast')
  assert.ok(contrast(palette.accent, consoleCSSVariables(dark)['--blue-soft']) >= 4.5, 'selected navigation text contrast')
}

const css = await readFile(new URL('../src/styles/console.css', import.meta.url), 'utf8')
assert.equal(consolePalette.light.background, '#f7f8fa', 'light page backdrop stays near-white')
assert.equal(consolePalette.light.surface, '#f8fafc', 'secondary surfaces retain a light cool tint')
assert.equal(consolePalette.light.sunken, '#f1f5f9', 'filled controls avoid dark gray slabs')
for (const dark of [false, true]) {
  const palette = dark ? consolePalette.dark : consolePalette.light
  const variables = consoleCSSVariables(dark)
  assert.equal(variables['--bg'], palette.background)
  assert.equal(variables['--blue'], palette.accent)
  assert.equal(variables['--green'], palette.success)
  assert.notEqual(variables['--blue'], variables['--green'], 'status and interaction colors remain distinct')
  assert.equal(variables['--console-input'], palette.sunken)
  assert.equal(variables['--page-overlay-strong'], `linear-gradient(${palette.background}, ${palette.background})`)
}
assert.doesNotMatch(css, /--bg:|--blue:|--green:/, 'the console stylesheet cannot override shared theme values')
const provider = await readFile(new URL('../src/components/AdminConsoleTheme.tsx', import.meta.url), 'utf8')
const login = await readFile(new URL('../src/components/LoginScreen.tsx', import.meta.url), 'utf8')
const customer = await readFile(new URL('../src/components/CustomerPortal.tsx', import.meta.url), 'utf8')
const detail = (await readFile(new URL('../src/components/AgentDetailPanel.tsx', import.meta.url), 'utf8')) + '\n' + (await readFile(new URL('../src/components/AgentClientTables.tsx', import.meta.url), 'utf8')) + '\n' + (await readFile(new URL('../src/components/useAgentRoutingTools.tsx', import.meta.url), 'utf8'))
const railCSS = await readFile(new URL('../src/styles/agent-rail.css', import.meta.url), 'utf8')
assert.match(detail, /<Switch\s+className="client-enabled-switch"\s+checked=\{configuredEnabled\}/, 'configured client enablement uses the themed switch')
assert.match(railCSS, /\.client-enabled-switch:focus-visible\s*\{[^}]*outline: 2px solid var\(--blue\);/, 'the switch retains a visible themed keyboard focus')
assert.match(detail, /record\.enabled \? 'blue' : 'default'/, 'account enablement uses a blue badge')
assert.doesNotMatch(detail, /record\.enabled \? 'success'/, 'account enablement is not a financial success color')
const navigation = await readFile(new URL('../src/components/AdminShellNavigation.tsx', import.meta.url), 'utf8')
assert.doesNotMatch(navigation, /<span>ZaneLin \//, 'topbar has no redundant console breadcrumb')
const layout = await readFile(new URL('../src/app/AdminLayout.tsx', import.meta.url), 'utf8')
assert.match(layout, /title=\{current.title\}/, 'the shared page container retains the page title')
assert.match(login, /consoleAppearance = false/)
assert.match(login, /consoleAppearance \? <aside/)
assert.match(customer, /consoleAppearance/, 'customer login opts into shared presentation')
assert.match(customer, /audience="customer"/)
assert.doesNotMatch(customer, /applyCustomerStyle|style_code|页面样式|customer-theme-select/, 'customer custom style feature and theme dropdown are removed')
assert.equal((customer.match(/<CustomerThemeButton \/>/g) || []).length, 2, 'desktop and mobile use the same icon theme switch')
assert.equal((customer.match(/color="#171717" bgColor="#ffffff"/g) || []).length, 2, 'QR codes remain readable in dark mode')
const customerCSS = await readFile(new URL('../src/styles/customer-console.css', import.meta.url), 'utf8')
assert.match(customerCSS, /\.customer-console-theme/)
assert.match(customerCSS, /max-width: 760px/)
assert.match(customerCSS, /\.customer-topology-canvas \{ min-width: 0;/)
assert.doesNotMatch(customerCSS, /https?:\/\/|@font-face/)
assert.match(css, /prefers-reduced-motion: reduce/)
assert.match(css, /max-width: 760px/)
assert.match(css, /data-console-theme='soft'/)
assert.match(provider, /delete document\.documentElement\.dataset\.consoleTheme/)
assert.match(provider, /removeEventListener/)
assert.doesNotMatch(css, /https?:\/\/|@font-face/)
assert.doesNotMatch(css, /ant-table.*(?:width|position):/)
assert.doesNotMatch(css, /\.ant-btn-primary.*background:/, 'ghost/loading states stay managed by Ant Design')
console.log('console route isolation, light/dark contrast, reduced motion and table/style safeguards passed')
