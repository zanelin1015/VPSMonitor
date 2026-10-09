import { useEffect, useState } from 'react'
import { Alert, Button, Card, Input, InputNumber, Modal, Select, Space, Switch, Table, Tag, Typography, message } from 'antd'
import type { ScheduledTaskSettings } from '../types'
import { fetchJSON } from '../lib/api'

interface Candidate {
  agent_id: string; agent_name: string; client_id: string; email: string; inbound_id: number
  start_time: number; cycle: string; expiry_time: number; supported: boolean; fresh: boolean; config_enabled: boolean
}
interface Policy {
  id: string; agent_id: string; client_id: string; email: string; inbound_id: number
  enabled: boolean; follow_billing: boolean; start_time: number; cycle: string
}
interface Job { id: string; agent_id: string; email: string; run_at: number; boundary: number; status: string; message: string }
interface ResetLog {
  id: number; agent_id: string; agent_name: string; client_name: string; email: string
  started_at?: string; completed_at?: string; recorded_at: string; time_source: string; completed_time_source: string
  operation: string; outcome: string; message: string
}
interface Data { clients: Candidate[]; policies: Policy[]; preview: Job[]; jobs: Job[]; logs: ResetLog[] }
const empty: Data = { clients: [], policies: [], preview: [], jobs: [], logs: [] }
const cycles = [{ value: 'month', label: '月' }, { value: 'quarter', label: '季' }, { value: 'semiannual', label: '半年' }, { value: 'year', label: '年' }]
const statuses: Record<string, string> = { prepared: '待执行', dispatched: '已下发', succeeded: '完成', failed: '失败', skipped: '已跳过', uncertain: '需人工核查', needs_enable: '接口已确认，需核验结果' }
const outcomes: Record<string, string> = { succeeded: '成功', failed: '失败', uncertain: '结果未确认', unverified: '清零已确认，核验失败', skipped: '跳过（未执行）' }
const date = (ms: number) => ms > 0 ? new Date(ms).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai', hour12: false }) : '未设置'
const day = (ms: number) => ms > 0 ? new Date(ms + 8 * 3600000).toISOString().slice(0, 10) : ''

export function TrafficResetPanel({ settings, onChange }: {
  settings: ScheduledTaskSettings['traffic_reset']; onChange: (settings: ScheduledTaskSettings['traffic_reset']) => void
}) {
  const [data, setData] = useState<Data>(empty)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [draft, setDraft] = useState<Policy | null>(null)
  const [query, setQuery] = useState('')
  const [logQuery, setLogQuery] = useState('')
  const refresh = async () => {
    setLoading(true)
    try { setData(await fetchJSON<Data>('/api/v1/admin/traffic-reset')) }
    catch (error) { message.error(error instanceof Error ? error.message : '读取重置计划失败') }
    finally { setLoading(false) }
  }
  useEffect(() => { void refresh() }, [])
  const save = async () => {
    if (!draft) return
    setSaving(true)
    try {
      await fetchJSON('/api/v1/admin/traffic-reset', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(draft) })
      setDraft(null); message.success('客户端重置策略已保存'); await refresh()
    } catch (error) { message.error(error instanceof Error ? error.message : '保存失败') }
    finally { setSaving(false) }
  }
  const scan = async () => {
    setLoading(true)
    try { await fetchJSON('/api/v1/admin/traffic-reset', { method: 'POST' }); message.success('扫描完成，未到执行时间不会清零'); await refresh() }
    catch (error) { message.error(error instanceof Error ? error.message : '扫描失败'); setLoading(false) }
  }
  const missingClients: Candidate[] = data.policies.filter((p) => !data.clients.some((c) => c.agent_id === p.agent_id && c.client_id === p.client_id)).map((p) => ({ ...p, agent_name: p.agent_id, expiry_time: 0, config_enabled: false, supported: false, fresh: false }))
  const policyRows = [...data.clients, ...missingClients].filter((c) => `${c.agent_name} ${c.agent_id} ${c.email}`.toLowerCase().includes(query.trim().toLowerCase())).map((client) => ({ ...client, policy: data.policies.find((p) => p.agent_id === client.agent_id && p.client_id === client.client_id) }))
  const verify = async (id: string) => {
    try { await fetchJSON(`/api/v1/admin/traffic-reset?verify_job=${encodeURIComponent(id)}`, { method: 'POST' }); message.success('已下发只读核验，不会再次清零'); await refresh() }
    catch (error) { message.error(error instanceof Error ? error.message : '核验失败') }
  }
  return <Card bordered={false} className="config-section-card">
    <Space direction="vertical" size="middle" style={{ width: '100%', minWidth: 0 }}>
      <div className="admin-content-title compact">
        <Typography.Title level={4}>客户端周期流量重置</Typography.Title>
        <Switch checked={settings.enabled} onChange={(enabled) => onChange({ ...settings, enabled })} />
      </div>
      <Alert type="info" showIcon message="北京时间：周期结束前 5 分钟准备，结束后 1 分钟执行" description="只重置本周期上传/下载，不修改额度、倍率、费用、路由或到期时间。流量耗尽但未到期的客户端会恢复；人工禁用或已过期的客户端不会自动开启。全局开关修改后，请点击页面上方保存任务配置。" />
      <Space wrap>
        <Typography.Text>离线补执行窗口（分钟）</Typography.Text>
        <InputNumber min={1} max={1440} value={settings.catch_up_minutes} onChange={(v) => onChange({ ...settings, catch_up_minutes: Number(v || 360) })} />
        <Button onClick={() => void refresh()} loading={loading}>刷新计划</Button>
        <Button onClick={() => void scan()} disabled={!settings.enabled} loading={loading}>扫描数据库</Button>
      </Space>
      <Input.Search allowClear placeholder="搜索 Client 名称 / 客户端邮箱" value={query} onChange={(e) => setQuery(e.target.value)} />
      <Table size="small" loading={loading} dataSource={policyRows} rowKey={(r) => `${r.agent_id}:${r.client_id}`} scroll={{ x: 1050 }} pagination={{ pageSize: 10 }} columns={[
        { title: '客户端', fixed: 'left', width: 230, render: (_, r) => <><strong>{r.email}</strong><br /><Typography.Text type="secondary">{r.agent_name}</Typography.Text></> },
        { title: '兼容 / 在线', width: 160, render: (_, r) => <Tag color={r.supported && r.fresh ? 'blue' : 'orange'}>{!r.supported ? '请升级 Client / 检查配置' : !r.config_enabled ? '配置已禁用' : r.expiry_time > 0 && r.expiry_time <= Date.now() ? '已到期，需手动续期' : r.fresh ? '已升级 / 上报正常' : '上报过期'}</Tag> },
        { title: '策略', width: 140, render: (_, r) => r.policy?.enabled ? `${cycles.find((c) => c.value === r.policy?.cycle)?.label || '月'} / 已启用` : '未启用' },
        { title: '下次执行（北京时间）', width: 230, render: (_, r) => date(data.preview.filter((j) => j.agent_id === r.agent_id && j.email === r.email).sort((a, b) => a.run_at - b.run_at)[0]?.run_at || 0) },
        { title: '操作', fixed: 'right', width: 100, render: (_, r) => <Button size="small" onClick={() => setDraft(r.policy || { id: '', agent_id: r.agent_id, client_id: r.client_id, inbound_id: r.inbound_id, email: r.email, enabled: false, follow_billing: true, start_time: r.start_time, cycle: r.cycle || 'month' })}>配置</Button> },
      ]} />
      <Typography.Title level={5}>最近执行记录</Typography.Title>
      <Table size="small" dataSource={data.jobs} rowKey="id" scroll={{ x: 900 }} pagination={{ pageSize: 5 }} columns={[
        { title: '客户端', dataIndex: 'email', width: 190 },
        { title: '计划执行时间（北京时间）', width: 220, render: (_, r) => date(r.run_at) },
        { title: '状态', width: 210, render: (_, r) => <Tag color={r.status === 'succeeded' ? 'blue' : r.status === 'uncertain' ? 'red' : 'default'}>{statuses[r.status] || r.status}</Tag> },
        { title: '结果 / 原因', dataIndex: 'message' },
        { title: '操作', width: 110, render: (_, r) => r.status === 'needs_enable' ? <Button size="small" onClick={() => void verify(r.id)}>只读核验</Button> : null },
      ]} />
      <Typography.Title level={5}>流量重置日志（北京时间，最近 100 条）</Typography.Title>
      <Input.Search allowClear placeholder="搜索 Client / 客户端 / 失败原因" value={logQuery} onChange={(e) => setLogQuery(e.target.value)} />
      <Table size="small" dataSource={(data.logs || []).filter((r) => `${r.agent_name} ${r.agent_id} ${r.client_name} ${r.email} ${r.message}`.toLowerCase().includes(logQuery.trim().toLowerCase()))}
        rowKey="id" scroll={{ x: 1350 }} pagination={{ pageSize: 10 }} columns={[
          { title: '执行时间', width: 220, render: (_, r) => <>{r.started_at ? date(Date.parse(r.started_at)) : r.time_source === 'not_executed' ? '未执行' : '未确认'}{r.time_source === 'server_claim' && <><br /><Typography.Text type="secondary">领取时间，实际执行时间未确认</Typography.Text></>}</> },
          { title: '完成 / 记录时间', width: 220, render: (_, r) => <>{date(Date.parse(r.completed_at || r.recorded_at))}{r.completed_time_source !== 'client' && <><br /><Typography.Text type="secondary">Server 记录时间</Typography.Text></>}</> },
          { title: 'Client', width: 180, render: (_, r) => <>{r.agent_name}<br /><Typography.Text type="secondary">{r.agent_id}</Typography.Text></> },
          { title: '客户端', width: 180, render: (_, r) => <>{r.client_name}{r.client_name !== r.email && <><br /><Typography.Text type="secondary">{r.email}</Typography.Text></>}</> },
          { title: '操作', width: 110, render: (_, r) => r.operation === 'verify' ? '只读核验' : r.operation === 'skip' ? '跳过重置' : '重置流量' },
          { title: '结果', width: 190, render: (_, r) => <Tag color={r.outcome === 'succeeded' ? 'blue' : r.outcome === 'failed' ? 'red' : r.outcome === 'skipped' ? 'default' : 'orange'}>{outcomes[r.outcome] || r.outcome}</Tag> },
          { title: '详情 / 失败原因', dataIndex: 'message' },
        ]} />
    </Space>
    <Modal title={`配置流量重置 · ${draft?.email || ''}`} open={!!draft} onCancel={() => setDraft(null)} onOk={() => void save()} confirmLoading={saving} okText="保存策略" cancelText="取消">
      {draft && <Space direction="vertical" size="middle" style={{ width: '100%' }}>
        <Space>启用自动重置<Switch checked={draft.enabled} onChange={(enabled) => setDraft({ ...draft, enabled })} /></Space>
        <Space>跟随客户端开始日期和收费周期<Switch checked={draft.follow_billing} onChange={(follow_billing) => setDraft({ ...draft, follow_billing })} /></Space>
        {!draft.follow_billing && <>
          <Typography.Text>重置开始日期（北京时间 00:00:00）</Typography.Text>
          <Input type="date" value={day(draft.start_time)} onChange={(e) => setDraft({ ...draft, start_time: e.target.value ? Date.parse(`${e.target.value}T00:00:00+08:00`) : 0 })} />
          <Select style={{ width: '100%' }} value={draft.cycle} options={cycles} onChange={(cycle) => setDraft({ ...draft, cycle })} />
        </>}
        <Typography.Text type="secondary">未设置开始日期时，请先在客户端配置信息中设置，或关闭跟随并单独指定重置日期。此策略不会修改服务到期时间。</Typography.Text>
      </Space>}
    </Modal>
  </Card>
}
