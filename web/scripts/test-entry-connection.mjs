import assert from 'node:assert/strict'
import { build } from 'esbuild'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'

const dir = await mkdtemp(join(tmpdir(), 'entry-connection-'))
try {
  const load = async (name, path) => {
    const outfile = join(dir, `${name}.mjs`)
    await build({ entryPoints: [fileURLToPath(new URL(path, import.meta.url))], outfile, bundle: true, platform: 'node', format: 'esm' })
    return import(pathToFileURL(outfile).href)
  }
  const lib = await load('entry', '../src/lib/entryConnection.ts')
  const config = await load('config', '../src/lib/appHelpersConfig.ts')
  const outbound = await load('outbound', '../src/components/xuiActionFormShared.ts')
  const target = { type: 'xui', id: '7', port: 20001, transport: 'tcp' }
  const entry = { connection: { deployment: 'docker', docker_network: 'bridge', public_access: 'nat', public_address: 'nat.example.com' }, mappings: [{ use_for_export: true, target_type: 'xui', target_id: '7', internal_port: 20001, host_port: 30001, external_port: 333, transport: 'tcp', default_export: true }] }
  for (const [deployment, docker_network, public_access, port] of [['native', '', 'direct', 20001], ['native', '', 'nat', 333], ['docker', 'host', 'direct', 20001], ['docker', 'host', 'nat', 333], ['docker', 'bridge', 'direct', 30001], ['docker', 'bridge', 'nat', 333]]) {
    const result = lib.resolveEntryEndpoint({ ...entry, connection: { ...entry.connection, deployment, docker_network, public_access } }, target, 'wrong.example.com')
    assert.equal(result.error, undefined)
    assert.equal(result.port, port)
    assert.equal(result.address, 'nat.example.com')
  }
  assert.match(lib.resolveEntryEndpoint({ ...entry, mappings: [] }, target).error, /缺少/)
  assert.match(lib.resolveEntryEndpoint(entry, { ...target, port: 20002 }).error, /已变化/)
  assert.match(lib.resolveEntryEndpoint(entry, { ...target, transport: 'udp' }).error, /未覆盖/)
  assert.equal(lib.resolveEntryEndpoint({ mappings: [{ address: 'legacy.example.com', external_port: 333, internal_port: 20001 }] }, target, 'old.example.com').port, 20001)
  const base = config.createEmptyManagedConfig('agent')
  base.entry = config.normalizeEntryConfig(entry)
  for (const section of ['client', 'xui', 'entry', 'renewal']) {
    const saved = config.buildSectionSavePayload(base, structuredClone(base), section, 'agent')
    const restored = config.mergeSavedSectionIntoDraft(base, saved, section)
    assert.deepEqual(restored.entry.connection, base.entry.connection)
    assert.deepEqual(restored.entry.mappings, base.entry.mappings, `${section} save preserves bindings and ports`)
  }
  const node = { id: 7, port: 20001, protocol: 'vless', export_endpoint: lib.resolveEntryEndpoint(entry, target), tls_server_name: 'tls.example.com' }
  const client = { inbound_id: 7, email: 'alice', import_url: 'vless://uuid@nat.example.com:333?security=reality&sni=tls.example.com', auth_uuid: 'uuid' }
  const patch = outbound.buildOutboundImportPatch({ agent_id: 'source', summary: {} }, node, client, {})
  assert.equal(patch.port, 333, 'new outbounds must not use internal port')
  assert.equal(patch.address, 'nat.example.com')
  assert.equal(patch.server_name, 'tls.example.com')
  assert.throws(() => outbound.buildOutboundImportPatch({ summary: {} }, { ...node, export_endpoint: { error: '缺少映射' } }, client, {}), /缺少映射/)
  assert.equal(lib.formatEntryEndpoint({ address: '2001:db8::1', port: 333 }), '[2001:db8::1]:333')
  for (const host of ['nat.example.com', '1.161.63.29', '2001:db8::1', '[2001:db8::1]']) assert.ok(lib.validEntryHost(host), host)
  for (const host of ['https://example.com', 'example.com:333', '999.1.2.3', '0.0.0.0', '::', '::1', '127.0.0.1']) assert.equal(lib.validEntryHost(host), false, host)
  console.log('NAT/Docker endpoint, config preservation and outbound import tests passed')
} finally { await rm(dir, { recursive: true, force: true }) }
