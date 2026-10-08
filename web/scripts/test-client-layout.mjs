import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import ts from 'typescript'

const source = await readFile(new URL('../src/components/AgentDetailPanel.tsx', import.meta.url), 'utf8')
const ast = ts.createSourceFile('AgentDetailPanel.tsx', source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX)
let columns
const rows = new Map()
function visit(node) {
  if (ts.isVariableDeclaration(node) && node.name.getText(ast) === 'clientColumns') columns = node.initializer
  if (ts.isJsxElement(node)) {
    const attribute = node.openingElement.attributes.properties.find(item => item.name?.getText(ast) === 'className')
    if (attribute?.initializer && ts.isStringLiteral(attribute.initializer)) rows.set(attribute.initializer.text, node)
  }
  ts.forEachChild(node, visit)
}
visit(ast)
assert.ok(columns && ts.isArrayLiteralExpression(columns))
const firstColumn = columns.elements[0].getText(ast)
assert.match(firstColumn, /key: 'client'/)
assert.match(firstColumn, /fixed: 'left'/, 'client identity stays pinned when scrolling')
const billing = columns.elements.find(node => /key: 'billing'/.test(node.getText(ast))).getText(ast)
assert.match(billing, /title: '配置信息'/)
assert.match(billing, /width: 380/, 'billing column has room for the second row')
const price = rows.get('client-billing-price-row').getText(ast)
const cycle = rows.get('client-billing-cycle-row').getText(ast)
assert.match(price, /<Text type="secondary">费用<\/Text>\s*<InputNumber/, 'fee label precedes the amount')
assert.match(price, /revenue_amount/)
assert.match(price, /revenue_currency/)
assert.doesNotMatch(price, /traffic_multiplier|revenueCycle|<Button/)
assert.match(cycle, /traffic_multiplier/)
assert.match(cycle, /value=\{revenueCycle\}/)
assert.match(cycle, /width: 64/)
assert.match(cycle, /loading=\{saving\}/)
assert.match(cycle, /onSaveClientBilling\(record\)/)
assert.doesNotMatch(cycle, /revenue_amount|revenue_currency/)
assert.match(source, /tableLayout="fixed"[\s\S]*visibleClientColumns\.reduce/, 'table scroll width respects declared column widths')
const css = await readFile(new URL('../src/styles/agent-rail.css', import.meta.url), 'utf8')
assert.match(css, /\.client-billing-controls\s*\{[^}]*display: grid;[^}]*gap: 6px;/)
assert.match(css, /\.xui-client-table \.ant-table-cell-fix-left\s*\{[^}]*z-index: 5;/, 'compact inputs cannot paint over the pinned column')
assert.match(css, /\.client-billing-cycle-row\s*\{[^}]*flex-wrap: nowrap;[^}]*white-space: nowrap;/)
assert.match(css, /\.client-billing-cycle-row > \*\s*\{[^}]*flex: 0 0 auto;/)
console.log('client fixed column and two-row billing layout safeguards passed')
