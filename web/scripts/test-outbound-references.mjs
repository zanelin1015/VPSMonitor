import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import ts from 'typescript'

const dir = await mkdtemp(join(tmpdir(), 'vpsmonitor-outbound-references-'))
try {
  const source = await readFile(new URL('../src/lib/outboundRuleReferences.ts', import.meta.url), 'utf8')
  await writeFile(join(dir, 'references.mjs'), ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ES2022 } }).outputText)
  const { buildOutboundRuleReferences: build } = await import(`file://${join(dir, 'references.mjs')}`)
  const outbounds = [{ tag: 'direct', is_default: true }, { tag: 'relay-a' }, { tag: 'relay-aa' }, { tag: 'unused' }]
  const rules = [
    { index: 1, outbound_tag: 'relay-a', validity: 'valid' },
    { index: 2, outbound_tag: 'relay-a', enabled: false },
    { index: 3, outbound_tag: 'relay-a', validity: 'invalid' },
    { index: 4, balancer_tag: 'pool' },
    { index: 5, balancer_tag: 'fallback' },
    { index: 6, balancer_tag: 'cycle-a' },
    { index: 7, outbound_tag: 'removed' },
    { index: 8, balancer_tag: 'removed' },
  ]
  const balancers = [
    { tag: 'pool', outbound_tags: ['relay-a', 'relay-a', 'relay-aa', 'removed'], fallback_tag: 'direct' },
    { tag: 'fallback', outbound_tags: [], fallback_tag: 'pool' },
    { tag: 'cycle-a', outbound_tags: [], fallback_tag: 'cycle-b' },
    { tag: 'cycle-b', outbound_tags: ['relay-aa'], fallback_tag: 'cycle-a' },
  ]
  const original = JSON.stringify({ outbounds, rules, balancers })
  const refs = build(outbounds, rules, balancers)
  assert.deepEqual(refs.get('relay-a').map(item => item.rule.index), [1, 2, 3, 4, 5], 'include all explicit references in original rule order, including disabled and invalid rules')
  assert.equal(refs.get('relay-a')[1].rule.enabled, false, 'keep status available for the association detail')
  assert.equal(refs.get('relay-a')[2].rule.validity, 'invalid')
  assert.deepEqual(refs.get('relay-aa').map(item => item.rule.index), [4, 5, 6], 'exact direct tags must not match similarly named outbounds')
  assert.deepEqual(refs.get('direct').map(item => item.rule.index), [4, 5], 'resolve outbound fallback and nested balancer fallback')
  assert.equal(refs.get('relay-a')[0].direct, true)
  assert.deepEqual(refs.get('relay-a')[0].balancers, [])
  assert.equal(refs.get('relay-a')[3].direct, false)
  assert.deepEqual(refs.get('relay-a')[3].balancers, ['pool'])
  assert.deepEqual(refs.get('unused'), [], 'unused outbounds still have an explicit empty result')
  assert.equal(refs.has('removed'), false, 'do not create phantom outbound records')
  assert.equal(JSON.stringify({ outbounds, rules, balancers }), original, 'view derivation cannot change configuration data')
  assert.equal(build([], rules, balancers).size, 0)
  assert.deepEqual(build(outbounds, []).get('direct'), [], 'a default outbound does not imply an explicit route')
  assert.equal(build(outbounds, [{ index: 9, outbound_tag: 'relay-a', balancer_tag: 'pool' }], balancers).get('relay-a').length, 1, 'duplicate paths count a rule once')
  console.log('outbound direct/balancer/fallback references, rule status, exact tags, cycles and default/empty cases passed')
} finally {
  await rm(dir, { recursive: true, force: true })
}
