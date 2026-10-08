import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import ts from 'typescript'

const source = await readFile(new URL('../src/lib/adminConsoleTheme.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ES2022 },
}).outputText
const { adminConsoleTheme, consolePalette, isInternalConsoleRoute, isConsoleThemeRoute } = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)

for (const route of ['/', '/admin/assets', '/monitor']) assert.equal(isInternalConsoleRoute(route), true)
for (const route of ['/customer', '/site', '/official']) assert.equal(isInternalConsoleRoute(route), false)
assert.equal(isInternalConsoleRoute('/', '?page=site'), false)
assert.equal(isInternalConsoleRoute('/', '?page=settings'), true)
for (const route of ['/', '/admin/assets', '/monitor', '/customer']) assert.equal(isConsoleThemeRoute(route), true)
for (const route of ['/site', '/official']) assert.equal(isConsoleThemeRoute(route), false)
assert.equal(isConsoleThemeRoute('/customer', '?page=site'), false)

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
  assert.equal(theme.token.borderRadiusLG, 20)
  assert.equal(theme.token.controlHeightSM, 26)
  assert.equal(theme.token.motion, true)
  assert.equal(adminConsoleTheme(dark, true).token.motion, false)
  for (const color of [palette.text, palette.secondary, palette.muted]) {
    assert.ok(contrast(color, palette.raised) >= 4.5, `${color} is readable on ${palette.raised}`)
  }
  assert.ok(contrast(theme.components.Button.primaryColor, palette.accent) >= 4.5, 'primary button text contrast')
  assert.ok(contrast(theme.components.Button.dangerColor, palette.error) >= 4.5, 'danger button text contrast')
  assert.ok(contrast(palette.accent, dark ? '#2b2b44' : '#e9e9ff') >= 4.5, 'selected navigation text contrast')
}

const css = await readFile(new URL('../src/styles/console.css', import.meta.url), 'utf8')
assert.equal(consolePalette.light.background, '#f7f8fa', 'light page backdrop stays near-white')
assert.equal(consolePalette.light.surface, '#f8fafc', 'secondary surfaces retain a light cool tint')
assert.equal(consolePalette.light.sunken, '#f1f5f9', 'filled controls avoid dark gray slabs')
const lightCSS = css.slice(0, css.indexOf(':root.dark:'))
assert.match(lightCSS, /--bg: #f7f8fa;/)
assert.match(lightCSS, /--surface: #ffffff;/)
assert.match(lightCSS, /--surface-soft: #f8fafc;/)
assert.doesNotMatch(lightCSS, /#efefee|#f7f7f6|#e7e7e6/)
assert.match(css, /:root:not\(\.dark\):where\(\[data-console-theme='soft'\]\) \.admin-console-theme \.admin-oa-sider-foot \{[^}]*background: var\(--card\)/, 'light sidebar footer stays white without changing dark mode')
const darkCSS = css.slice(css.indexOf(':root.dark:'), css.indexOf('.admin-console-theme {'))
assert.match(darkCSS, /--console-input: #121216;/)
assert.match(darkCSS, /--page-overlay-strong: linear-gradient\(var\(--bg\), var\(--bg\)\);/, 'near-white light gradient cannot leak into dark mode')
const provider = await readFile(new URL('../src/components/AdminConsoleTheme.tsx', import.meta.url), 'utf8')
const login = await readFile(new URL('../src/components/LoginScreen.tsx', import.meta.url), 'utf8')
const customer = await readFile(new URL('../src/components/CustomerPortal.tsx', import.meta.url), 'utf8')
const detail = await readFile(new URL('../src/components/AgentDetailPanel.tsx', import.meta.url), 'utf8')
assert.equal((detail.match(/record\.enabled \? 'blue' : 'default'/g) || []).length, 2, 'enabled client/account status badges stay blue')
assert.doesNotMatch(detail, /record\.enabled \? 'success'/, 'account enablement is not a financial success color')
const navigation = await readFile(new URL('../src/components/AdminShellNavigation.tsx', import.meta.url), 'utf8')
assert.doesNotMatch(navigation, /<span>ZaneLin \//, 'topbar has no redundant console breadcrumb')
assert.match(navigation, /<strong>\{pageTitles\[props\.activeAdminPage\]\}<\/strong>/, 'page title is retained')
assert.match(login, /consoleAppearance = false/)
assert.match(login, /consoleAppearance \? <aside/)
assert.match(customer, /consoleAppearance/, 'customer login opts into shared presentation')
assert.match(customer, /audience="customer"/)
assert.match(customer, /applyCustomerStyle\(user\?\.style_code/, 'customer custom styles are preserved')
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
