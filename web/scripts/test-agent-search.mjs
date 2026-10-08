import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import ts from 'typescript'

const dir = await mkdtemp(join(tmpdir(), 'vpsmonitor-agent-search-'))
try {
  const source = await readFile(new URL('../src/lib/agentSearch.ts', import.meta.url), 'utf8')
  const transpiled = ts.transpileModule(source, {
    compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ES2022 },
  }).outputText
  await writeFile(join(dir, 'agentSearch.mjs'), transpiled, 'utf8')
  const { agentMatchesSearch, matchingAgentClients, clientMatchesSearch } = await import(`file://${join(dir, 'agentSearch.mjs')}`)

  const agent = {
    agent_id: 'agent-1',
    agent_name: 'HK-DMIT-香港',
    customer_display_name: '香港出口',
    tags: ['线路机'],
    summary: { hostname: 'debian-a', public_ipv4: '192.0.2.10' },
    finance_clients: [
      { client_id: 'client-a', inbound_id: 1001, inbound_tag: 'in-20001-tcp', inbound_remark: '游戏节点', email: 'Alice@example.com', comment: '小王', enabled: true },
      { client_id: 'client-b', inbound_id: 1002, inbound_tag: 'in-20002-tcp', inbound_remark: '备用节点', email: 'Bob@example.com', comment: '停用账号', enabled: false },
    ],
    renewal: { client_billings: [{ email: 'deleted@example.com' }] },
  }
  for (const query of ['', '  ', ' hK-dMiT ', '香港', '香港出口', '线路机', 'debian-a', '192.0.2.10', '小王', ' ALICE@EXAMPLE.COM ', '游戏节点', 'in-20002', '1002', '停用账号', 'client-a']) {
    assert.equal(agentMatchesSearch(agent, query), true, `query should match: ${query}`)
  }
  assert.equal(agentMatchesSearch(agent, 'missing-user'), false)
  assert.equal(agentMatchesSearch(agent, 'deleted@example.com'), false, 'stale billing entries must not produce search results')
  assert.equal(agentMatchesSearch({ agent_id: 'empty', summary: {} }, 'alice'), false, 'missing snapshot should not crash')
  assert.equal(agentMatchesSearch({ agent_id: 'empty', summary: {} }, 'empty'), true)
  assert.deepEqual(matchingAgentClients(agent, '').map((client) => client.client_id), [], 'empty query should not mark every client as a search hit')
  assert.deepEqual(matchingAgentClients(agent, '小王').map((client) => client.client_id), ['client-a'])
  assert.deepEqual(matchingAgentClients(agent, '备用节点').map((client) => client.client_id), ['client-b'])
  assert.equal(clientMatchesSearch(agent.finance_clients[0], ' alice '), true, 'detail search uses the same case-insensitive matching')
  assert.equal(clientMatchesSearch(agent.finance_clients[0], '游戏节点'), true)
  assert.equal(clientMatchesSearch(agent.finance_clients[0], ''), true)
  assert.equal(clientMatchesSearch(agent.finance_clients[0], 'bob'), false)
  const agents = [agent, { agent_id: 'other', summary: {}, finance_clients: [] }]
  assert.deepEqual(agents.filter((item) => agentMatchesSearch(item, 'Alice')).map((item) => item.agent_id), ['agent-1'])
  assert.deepEqual(agents.filter((item) => agentMatchesSearch(item, '')).map((item) => item.agent_id), ['agent-1', 'other'], 'clearing search restores the full list')
  console.log('agent search tests passed')
} finally {
  await rm(dir, { recursive: true, force: true })
}
