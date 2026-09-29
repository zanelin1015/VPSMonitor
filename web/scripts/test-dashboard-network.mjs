import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import ts from 'typescript'

const dir = await mkdtemp(join(tmpdir(), 'vpsmonitor-dashboard-network-'))
try {
  await transpile('../src/lib/appHelpersAgent.ts', 'appHelpersAgent.mjs')
  await transpile('../src/lib/traffic.ts', 'traffic.mjs', (source) =>
    source.replace("'./appHelpersAgent'", "'./appHelpersAgent.mjs'"),
  )

  const { summarizeAgentNetwork, summarizeDeduplicatedNetwork } = await import(`file://${join(dir, 'traffic.mjs')}`)
  const agents = [
    agent('gz-primary', '广州-阿里云', 'CN', 100, 200, { haproxy: forwardingConfig() }),
    agent('gz-backup', '广州-阿里云-备用-1', 'CN', 10, 20, { haproxy: forwardingConfig() }),
    agent('hk-relay', 'HK-阿里云', 'HK', 1_000, 2_000, { port_forwarding: forwardingConfig() }),
    agent('us-exit', 'US-DMIT', 'US', 3_000, 4_000),
    agent('istore', 'iStoreOS', 'CN', 5_000, 6_000),
  ]

  const summary = summarizeAgentNetwork(agents)
  assert.equal(summary.up, 9_110, 'without topology, all host speeds remain visible')
  assert.equal(summary.down, 12_220, 'without topology, all host speeds remain visible')
  assert.equal(summary.sent, 50, 'period upload remains global')
  assert.equal(summary.recv, 100, 'period download remains global')
  assert.equal(summary.used, 150, 'period total remains global')

  const deduplicated = summarizeDeduplicatedNetwork(agents, [
    {
      key: 'gz-primary:1',
      root_agent_id: 'gz-primary',
      root_inbound_id: 1,
      root_client_enabled: true,
      root_inbound_enabled: true,
      matched_link_count: 1,
      steps: [
        { step_type: 'client', agent_id: 'gz-primary', label: 'entry' },
        { step_type: 'match', agent_id: 'hk-relay', label: 'relay' },
      ],
    },
  ])
  assert.equal(deduplicated.up, 9_010, 'linked hosts use the largest upload sample once')
  assert.equal(deduplicated.down, 12_020, 'linked hosts use the largest download sample once')
  assert.equal(deduplicated.sent, 40, 'linked hosts use the largest period upload once')
  assert.equal(deduplicated.recv, 80, 'linked hosts use the largest period download once')
  assert.equal(deduplicated.used, 120, 'linked hosts use the largest period total once')

  const multiHop = summarizeDeduplicatedNetwork(agents, [
    {
      key: 'gz-primary:1',
      root_agent_id: 'gz-primary',
      root_inbound_id: 1,
      root_client_enabled: true,
      root_inbound_enabled: true,
      matched_link_count: 2,
      steps: [
        { step_type: 'client', agent_id: 'gz-primary', label: 'entry' },
        { step_type: 'match', agent_id: 'hk-relay', label: 'relay' },
        { step_type: 'match', agent_id: 'us-exit', label: 'exit' },
      ],
    },
  ])
  assert.equal(multiHop.up, 8_010, 'multi-hop chain is counted as one connected component')
  assert.equal(multiHop.down, 10_020, 'multi-hop chain is counted as one connected component')

  const mixedTraffic = summarizeDeduplicatedNetwork([
    agent('entry', 'entry', 'CN', 100, 200),
    agent('relay', 'relay', 'HK', 150, 250),
    agent('independent', 'independent', 'US', 40, 60),
  ], [{
    key: 'entry:1',
    root_agent_id: 'entry',
    root_inbound_id: 1,
    root_client_enabled: true,
    root_inbound_enabled: true,
    matched_link_count: 1,
    steps: [
      { step_type: 'client', agent_id: 'entry', label: 'entry' },
      { step_type: 'match', agent_id: 'relay', label: 'relay' },
    ],
  }])
  assert.equal(mixedTraffic.up, 190, 'independent traffic remains additive beside a chain')
  assert.equal(mixedTraffic.down, 310, 'independent traffic remains additive beside a chain')

  console.log('dashboard network tests passed')
} finally {
  await rm(dir, { recursive: true, force: true })
}

async function transpile(sourceName, outputName, transform = (source) => source) {
  const sourcePath = new URL(sourceName, import.meta.url)
  const source = await readFile(sourcePath, 'utf8')
  const transpiled = ts.transpileModule(source, {
    compilerOptions: {
      target: ts.ScriptTarget.ES2022,
      module: ts.ModuleKind.ES2022,
      importsNotUsedAsValues: ts.ImportsNotUsedAsValues.Remove,
      verbatimModuleSyntax: false,
    },
  }).outputText
  await writeFile(join(dir, outputName), transform(transpiled), 'utf8')
}

function agent(agent_id, agent_name, country_code, up, down, entry, tags = []) {
  return {
    agent_id,
    agent_name,
    geo: { country_code },
    tags,
    entry,
    summary: {
      net_io_up: up,
      net_io_down: down,
      net_traffic_sent: 10,
      net_traffic_recv: 20,
      net_traffic_total: 30,
    },
  }
}

function forwardingConfig() {
  return { enabled: true, rules: [{ enabled: true }] }
}
