import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'
import React from 'react'
import { act, create } from 'react-test-renderer'
import ts from 'typescript'

const dir = await mkdtemp(join(tmpdir(), 'vpsmonitor-assignment-binding-'))
try {
  for (const name of ['customerAssignmentBinding', 'agentHealth', 'appHelpersAgent', 'traffic', 'useAssignmentBindingOverviews']) {
    const folder = name.startsWith('use') ? 'hooks' : 'lib'
    const source = await readFile(new URL(`../src/${folder}/${name}.ts`, import.meta.url), 'utf8')
    let code = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ES2022 } }).outputText
    code = code.replaceAll("from 'react'", `from '${import.meta.resolve('react')}'`)
      .replaceAll("'../lib/appHelpers'", "'./mock-fetch.mjs'")
      .replaceAll("'../lib/customerAssignmentBinding'", "'./customerAssignmentBinding.mjs'")
    for (const dep of ['agentHealth', 'appHelpersAgent', 'traffic']) code = code.replaceAll(`'./${dep}'`, `'./${dep}.mjs'`)
    await writeFile(join(dir, `${name}.mjs`), code)
  }
  await writeFile(join(dir, 'mock-fetch.mjs'), `
    export const requests = [];
    export function fetchJSON(url) { return new Promise((resolve, reject) => requests.push({url, resolve, reject})); }
  `)
  const load = name => import(pathToFileURL(join(dir, `${name}.mjs`)))
  const { assignmentBindingDiagnostic: diagnose } = await load('customerAssignmentBinding')
  const { useAssignmentBindingOverviews } = await load('useAssignmentBindingOverviews')
  const { requests } = await load('mock-fetch')
  const now = new Date().toISOString()
  const gz = { agent_id: 'gz', last_seen_at: now, summary: {}, finance_clients_ready: false, finance_clients: [], entry: { port_forwarding: { enabled: true, rules: [{ listen_port: 20002, target_agent_id: 'ix', target_port: 15601 }] } } }
  const assignment = { agent_id: 'gz', inbound_id: 20002, inbound_tag: 'YT-CNIX-HK', client_id: 'stable-lei', client_email: 'lei' }
  const client = { inbound_id: 20002, client_id: 'stable-lei', email: 'lei', enabled: true, forward_type: 'realm', realm_target_agent_id: 'dmit', realm_target_inbound_id: 2 }
  const overview = { agent_id: 'gz', nodes: [{ id: 20002 }], clients: [client] }
  const ready = value => ({ gz: { status: 'ready', overview: value } })
  assert.equal(diagnose(assignment, [gz]).status, 'waiting_sync', 'pure Realm entries wait for resolved data instead of reporting deletion')
  assert.equal(diagnose(assignment, [gz], { gz: { status: 'error' } }).status, 'unavailable')
  assert.equal(diagnose(assignment, [gz], ready(overview)).status, 'matched', 'multi-hop client resolves in the entry listener namespace')
  assert.equal(diagnose(assignment, [gz], ready({ ...overview, agent_id: 'other' })).status, 'unavailable')
  assert.equal(diagnose(assignment, [gz], ready({ ...overview, clients: [], nodes: [] })).status, 'unresolved', 'a missing target snapshot/rule must not imply deletion')
  assert.equal(diagnose(assignment, [gz], ready({ ...overview, clients: [] })).status, 'select_client', 'a resolved inbound with no matching client requires selection')
  assert.equal(diagnose(assignment, [gz], ready({ ...overview, clients: [{ ...client, inbound_id: 20003 }] })).status, 'select_client', 'the same UUID on another listener cannot match')
  assert.equal(diagnose({ ...assignment, client_id: '' }, [gz], ready(overview)).status, 'legacy')
  assert.equal(diagnose({ ...assignment, client_id: 'old' }, [gz], ready(overview)).status, 'mismatch')
  assert.equal(diagnose({ ...assignment, client_id: '' }, [gz], ready({ ...overview, clients: [client, { ...client, client_id: 'other' }] })).current, undefined, 'duplicate emails must not suggest an arbitrary rebind')
  const direct = { ...gz, agent_id: 'direct', entry: {}, finance_clients_ready: true, finance_clients: [client] }
  assert.equal(diagnose({ ...assignment, agent_id: 'direct' }, [direct]).status, 'matched')
  assert.equal(diagnose({ ...assignment, agent_id: 'direct', inbound_id: 99 }, [direct]).status, 'node_deleted')
  assert.equal(diagnose({ ...assignment, agent_id: 'direct', inbound_id: 99 }, [{ ...direct, finance_clients_ready: false }]).status, 'waiting_sync')
  assert.deepEqual(gz.finance_clients, [], 'resolved clients never enter finance totals')

  let state
  function Probe(props) { state = useAssignmentBindingOverviews(props.assignments, props.agents, props.active); return null }
  const assignments = [assignment, { ...assignment, client_id: 'second' }, { ...assignment, agent_id: 'direct' }]
  const agents = [gz, direct]
  let renderer
  await act(async () => { renderer = create(React.createElement(Probe, { assignments, agents, active: false })) })
  assert.equal(requests.length, 0, 'closed assignment dialogs issue no requests')
  await act(async () => { renderer.update(React.createElement(Probe, { assignments, agents, active: true })) })
  assert.equal(requests.length, 1, 'one request per forwarding entry, none for direct X-UI grants')
  assert.equal(requests[0].url, '/api/v1/agents/gz/xui/overview?assignment_scope=1')
  assert.equal(state.gz.status, 'loading')
  await act(async () => { requests[0].resolve(overview) })
  assert.equal(diagnose(assignment, agents, state).status, 'matched')
  await act(async () => { renderer.update(React.createElement(Probe, { assignments, agents: [...agents], active: true })) })
  assert.equal(requests.length, 1, 'dashboard realtime updates do not refetch the same overview')
  const refreshed = [...assignments]
  await act(async () => { renderer.update(React.createElement(Probe, { assignments: refreshed, agents, active: true })) })
  assert.equal(requests.length, 2, 'refreshing customer records refreshes diagnostics')
  const other = [{ ...assignment, agent_id: 'other', inbound_tag: 'realm:20002' }]
  await act(async () => { renderer.update(React.createElement(Probe, { assignments: other, agents, active: true })) })
  assert.equal(state.gz, undefined, 'switching customers clears the previous entry namespace')
  await act(async () => { requests[1].resolve(overview) })
  assert.equal(state.gz, undefined, 'late responses from the previous customer are ignored')
  await act(async () => { requests[2].reject(new Error('offline')) })
  assert.equal(state.other.status, 'error')
  await act(async () => { renderer.update(React.createElement(Probe, { assignments: other, agents, active: false })) })
  assert.deepEqual(state, {})
  await act(async () => { renderer.update(React.createElement(Probe, { assignments: other, agents, active: true })) })
  assert.equal(requests.length, 4, 'reopening retries failed loads')
  await act(async () => { renderer.unmount(); requests[3].resolve(overview) })
  console.log('assignment binding: Realm resolution, listener scope, sync/error states, legacy IDs and request lifecycle passed')
} finally {
  await rm(dir, { recursive: true, force: true })
}
