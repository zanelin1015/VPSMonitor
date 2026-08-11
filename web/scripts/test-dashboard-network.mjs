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

  const { isCNLineEntryAgent, summarizeWorkbenchNetwork } = await import(`file://${join(dir, 'traffic.mjs')}`)
  const agents = [
    agent('gz-primary', '广州-阿里云', 'CN', 100, 200, { haproxy: forwardingConfig() }),
    agent('gz-backup', '广州-阿里云-备用-1', 'CN', 10, 20, { haproxy: forwardingConfig() }),
    agent('hk-relay', 'HK-阿里云', 'HK', 1_000, 2_000, { port_forwarding: forwardingConfig() }),
    agent('us-exit', 'US-DMIT', 'US', 3_000, 4_000),
    agent('istore', 'iStoreOS', 'CN', 5_000, 6_000),
  ]

  assert.equal(isCNLineEntryAgent(agents[0]), true, 'CN HAProxy entry should be included')
  assert.equal(isCNLineEntryAgent(agents[1]), true, 'CN backup entry should be included')
  assert.equal(isCNLineEntryAgent(agents[2]), false, 'overseas Realm relay should not duplicate entry speed')
  assert.equal(isCNLineEntryAgent(agents[4]), false, 'ordinary CN device without forwarding should be excluded')

  const summary = summarizeWorkbenchNetwork(agents)
  assert.equal(summary.up, 110, 'workbench upload only sums CN line entries')
  assert.equal(summary.down, 220, 'workbench download only sums CN line entries')
  assert.equal(summary.sent, 50, 'period upload remains global')
  assert.equal(summary.recv, 100, 'period download remains global')
  assert.equal(summary.used, 150, 'period total remains global')

  const taggedEntry = agent('tagged', 'CN tagged entry', 'CN', 7, 8, undefined, ['国内入口'])
  assert.equal(isCNLineEntryAgent(taggedEntry), true, 'explicit domestic-entry tag should be supported')
  assert.equal(
    isCNLineEntryAgent(agent('gz-realm', '广州 Realm', 'CN', 7, 8, { port_forwarding: forwardingConfig() })),
    true,
    'CN Realm entry should be included',
  )
  assert.equal(
    isCNLineEntryAgent(agent('disabled-entry', '广州停用入口', 'CN', 7, 8, { haproxy: { enabled: true, rules: [{ enabled: false }] } })),
    false,
    'disabled forwarding rules should not mark a Client as an active entry',
  )

  const sanitizedAreaEntry = agent('area-gz-entry', '广州-阿里云', '', 30, 40)
  delete sanitizedAreaEntry.geo
  sanitizedAreaEntry.line_entry = true
  assert.equal(
    isCNLineEntryAgent(sanitizedAreaEntry),
    true,
    'server-derived entry marker should survive area-manager redaction',
  )
  const areaSummary = summarizeWorkbenchNetwork([sanitizedAreaEntry])
  assert.equal(areaSummary.up, 30, 'redacted area-manager entry upload should be included')
  assert.equal(areaSummary.down, 40, 'redacted area-manager entry download should be included')

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
