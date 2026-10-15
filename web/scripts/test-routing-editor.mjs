import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import ts from 'typescript'

const source = await readFile(new URL('../src/lib/routingRuleEditor.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ES2022 } }).outputText
const lib = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)
const hash = 'a'.repeat(64)
const raw = { type:'field', user:['alice','missing-user'], outboundTag:'direct', enabled:false, comment:'keep', attrs:{':method':'GET'}, futureOption:{preserve:['all','fields']} }
const record = { index:3, fingerprint:hash, raw_rule:raw, users:raw.user, outbound_tag:'direct' }
const draft = lib.createRoutingRuleDraft(record)
draft.user.push('bob')
assert.deepEqual(raw.user,['alice','missing-user'],'draft edits do not mutate the live overview')
draft.outboundTag = 'relay'
const payload = lib.buildRoutingRuleUpdatePayload(record,hash,draft)
assert.equal(payload.rule_index,3,'editing keeps the saved index')
assert.equal(payload.rule_fingerprint,hash)
assert.equal(payload.rules_fingerprint,hash)
assert.equal(payload.rule.enabled,false,'disabled rules stay disabled unless explicitly changed')
assert.deepEqual(payload.rule.attrs,raw.attrs)
assert.deepEqual(payload.rule.futureOption,raw.futureOption)
assert.equal(payload.rule.comment,'keep')
assert.deepEqual(lib.routingRuleList(['tcp','udp']),['tcp','udp'])
assert.deepEqual(lib.routingRuleList('tcp,udp'),['tcp','udp'])
for (const edited of [raw,Object.fromEntries(Object.entries(raw).reverse())]) assert.throws(()=>lib.buildRoutingRuleUpdatePayload(record,hash,edited),/未变化/)
for (const bad of ['{','[]','null','"rule"']) assert.throws(()=>lib.parseRoutingRuleJSON(bad))
assert.deepEqual(lib.parseRoutingRuleJSON(JSON.stringify(draft)),draft)
for (const incomplete of [{...record,protected:true},{...record,fingerprint:''},{...record,raw_rule:undefined},{...record,index:0}]) assert.throws(()=>lib.createRoutingRuleDraft(incomplete))
for (const edit of [
  {type:'unknown'}, {enabled:'false'}, {balancerTag:'balance'}, {outboundTag:''}, {network:'tcp,bad'}, {port:'65536'}, {sourcePort:'2000-1000'}, {user:'alice'}, {user:[1]},
]) assert.throws(()=>lib.buildRoutingRuleUpdatePayload(record,hash,{...draft,...edit}))
assert.throws(()=>lib.buildRoutingRuleUpdatePayload(record,'',{...draft}))
assert.throws(()=>lib.buildRoutingRuleUpdatePayload(record,hash,{outboundTag:'direct'}),/至少/)
const balancer = {...draft,balancerTag:'balance'};delete balancer.outboundTag
assert.equal(lib.buildRoutingRuleUpdatePayload(record,hash,balancer).rule.balancerTag,'balance')
assert.equal(lib.buildRoutingRuleUpdatePayload(record,hash,{...draft,network:['tcp','udp'],port:443}).rule.port,443)
assert.ok(lib.isPendingRoutingUpdate({kind:'update_routing_rule',status:'running'}))
assert.ok(!lib.isPendingRoutingUpdate({kind:'update_routing_rule',status:'succeeded'}))
console.log('routing editor full-field preservation, guarded payload, draft isolation, JSON and input validation tests passed')
