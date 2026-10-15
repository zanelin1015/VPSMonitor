import assert from 'node:assert/strict'
import { fileURLToPath } from 'node:url'
import { createServer } from 'vite'
import { renderToStaticMarkup } from 'react-dom/server'
import ts from 'typescript'

const originalWindow = globalThis.window
const vite = await createServer({
  root: fileURLToPath(new URL('../', import.meta.url)),
  configFile: fileURLToPath(new URL('../vite.config.ts', import.meta.url)),
  logLevel: 'silent',
  optimizeDeps: { noDiscovery: true, include: [] },
  ssr: { optimizeDeps: { noDiscovery: true, include: [] } },
  server: { host: '127.0.0.1', port: 0 },
})

try {
  await vite.listen()
  const transformed = await vite.transformRequest('/src/App.tsx')
  const ast = ts.createSourceFile('App.js', transformed.code, ts.ScriptTarget.Latest, true, ts.ScriptKind.JS)
  let resetsAppContent = false
  function visit(node) {
    if (ts.isCallExpression(node) && node.arguments[0]?.getText(ast) === 'AppContent' && ts.isStringLiteral(node.arguments[1])) {
      resetsAppContent = node.arguments[2]?.kind === ts.SyntaxKind.TrueKeyword
    }
    ts.forEachChild(node, visit)
  }
  visit(ast)
  assert.ok(resetsAppContent, 'Fast Refresh must remount the app after plain .ts hook edits instead of reusing stale hook queues')

  const { AppErrorBoundary } = await vite.ssrLoadModule('/src/components/AppErrorBoundary.tsx')
  const boundary = new AppErrorBoundary({ children: '正常页面' })
  assert.equal(boundary.render(), '正常页面')
  boundary.state = AppErrorBoundary.getDerivedStateFromError(new Error('<img src=x onerror=alert(1)>'))
  for (const [pathname, search, expectedHome] of [
    ['/', '?page=assets&agent=broken', '/?page=assets'],
    ['/zanelin/monitor', '?page=assets&agent=broken', '/zanelin/monitor?page=assets'],
    ['/customer', '', '/customer'],
    ['/zanelin/customer', '', '/zanelin/customer'],
    ['/zanelin/official', '', '/zanelin/site'],
    ['/', '?page=site', '/site'],
  ]) {
    globalThis.window = { location: { pathname, search } }
    const markup = renderToStaticMarkup(boundary.render())
    assert.ok(markup.includes(`href="${expectedHome}"`), pathname)
    assert.match(markup, /role="alert"/)
    assert.match(markup, /页面暂时无法显示/)
    assert.match(markup, /重新加载/)
    assert.match(markup, /&lt;img/)
    assert.doesNotMatch(markup, /<img/)
  }
  console.log('app Fast Refresh reset, error fallback, escaped details and root/subpath recovery navigation tests passed')
} finally {
  if (originalWindow === undefined) delete globalThis.window
  else globalThis.window = originalWindow
  await vite.close()
}
