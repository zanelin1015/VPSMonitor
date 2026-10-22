import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import ts from 'typescript'

const source = await readFile(new URL('../src/components/xuiActionFormShared.ts', import.meta.url), 'utf8')
let compiled = ts.transpileModule(source, {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ES2022 },
}).outputText
compiled = compiled.replace(/from ['"]\.\.\/types['"];?/g, '').replace(/from ['"]\.\.\/lib\/appHelpers['"];?/g, '')
globalThis.window = { atob }
const { parseOutboundImportText } = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)

const form = {
  tag: '', protocol: 'vless', source_type: '', source_agent_id: '', source_inbound_id: 0,
  source_inbound_tag: '', source_client_email: '', source_client_key: '', send_through: '',
  address: '', port: 0, uuid: '', flow: '', password: '', method: 'aes-256-gcm', security: 'none',
  server_name: '', alpn: '', grpc_service: '', reality_public_key: '', reality_short_id: '',
  reality_fingerprint: 'chrome', reality_spider_x: '', network: 'tcp', ws_path: '/', ws_host: '',
}

const explicit2022 = 'ss://2022-blake3-aes-256-gcm:server%2Bkey%2Fvalue%3D:user%2Bkey%2Fvalue%3D@entry.example.com:34919?type=tcp#test'
const parsed = parseOutboundImportText(explicit2022, form)
assert.equal(parsed.protocol, 'shadowsocks')
assert.equal(parsed.address, 'entry.example.com')
assert.equal(parsed.port, 34919)
assert.equal(parsed.method, '2022-blake3-aes-256-gcm')
assert.equal(parsed.password, 'server+key/value=:user+key/value=')
assert.equal(parsed.uuid, '')

const wholeBase64 = `ss://${Buffer.from('aes-256-gcm:password').toString('base64')}@entry.example.com:8388#legacy`
const legacy = parseOutboundImportText(wholeBase64, form)
assert.equal(legacy.method, 'aes-256-gcm')
assert.equal(legacy.password, 'password')
assert.equal(legacy.address, 'entry.example.com')
assert.equal(legacy.port, 8388)

console.log('Shadowsocks explicit 2022 and whole-base64 import parsing passed')
