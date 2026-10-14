import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import ts from 'typescript'

const source = await readFile(new URL('../src/lib/nodeImport.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ES2022 },
}).outputText
const { nodeImportURL, readNodeImportTarget, saveNodeImportTarget } = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)

const generic = 'vless://uuid@entry.example.com:443?security=reality&fp=chrome#NTT'
const shadowrocket = 'vless://uuid@entry.example.com:443?security=reality&fp=chrome140#NTT'
const node = { import_url: generic, shadowrocket_import_url: shadowrocket }
assert.equal(nodeImportURL(node), shadowrocket, 'new REALITY nodes import into Shadowrocket without a manual fingerprint change')
assert.equal(nodeImportURL(node, 'generic'), generic, 'other clients receive the original link')
assert.equal(nodeImportURL({ import_url: generic }), generic, 'old cores and responses without a companion link still export')
assert.equal(nodeImportURL({}), '', 'unresolved nodes have no import link')

const values = new Map()
globalThis.window = { localStorage: {
  getItem: key => values.get(key),
  setItem: (key, value) => values.set(key, value),
} }
assert.equal(readNodeImportTarget(), 'shadowrocket')
saveNodeImportTarget('generic')
assert.equal(readNodeImportTarget(), 'generic', 'remember another client for subsequent copies and QR codes')
saveNodeImportTarget('shadowrocket')
assert.equal(readNodeImportTarget(), 'shadowrocket')
window.localStorage.getItem = () => { throw new Error('blocked storage') }
window.localStorage.setItem = () => { throw new Error('blocked storage') }
assert.equal(readNodeImportTarget(), 'shadowrocket')
assert.doesNotThrow(() => saveNodeImportTarget('generic'))
delete globalThis.window
console.log('node export target and preference tests passed')
