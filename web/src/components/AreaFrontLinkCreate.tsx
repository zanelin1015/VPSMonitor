import { useRef, useState } from 'react'
import { App, Button, Input, Modal, Radio, Select, Space, Typography } from 'antd'
import type { DashboardAgentView, FrontProxyNode, XUIOverview } from '../types'
import { fetchJSON } from '../lib/appHelpers'

export function AreaFrontLinkCreate({ agents, onCreated }: { agents: DashboardAgentView[]; onCreated: (id: number) => void | Promise<void> }) {
  const { message } = App.useApp()
  const [open, setOpen] = useState(false)
  const [mode, setMode] = useState('third-party')
  const [name, setName] = useState('')
  const [url, setURL] = useState('')
  const [agentID, setAgentID] = useState('')
  const [links, setLinks] = useState<Array<{ value: string; label: string }>>([])
  const requestRef = useRef(0)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  async function load(id: string) {
    const request = ++requestRef.current
    setAgentID(id); setURL(''); setLinks([]); setLoading(true)
    try {
      const overview = await fetchJSON<XUIOverview>(`/api/v1/agents/${encodeURIComponent(id)}/xui/overview`)
      if (request !== requestRef.current) return
      setLinks((overview.clients || []).flatMap(client => client.import_url ? [{ value: client.import_url, label: client.email || client.inbound_tag || String(client.inbound_id) }] : []))
    } catch (error) { message.error(error instanceof Error ? error.message : '加载本地链接失败') }
    finally { if (request === requestRef.current) setLoading(false) }
  }
  async function save() {
    if (!name.trim() || !url.trim()) { message.warning('请填写名称并选择或输入链接'); return }
    setSaving(true)
    try {
      const node = await fetchJSON<FrontProxyNode>('/api/v1/admin/front-proxies', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name: name.trim(), share_url: url.trim(), enabled: true }) })
      await onCreated(node.id); setOpen(false); setName(''); setURL(''); message.success('前置链接已添加并选中')
    } catch (error) { message.error(error instanceof Error ? error.message : '新增前置链接失败') }
    finally { setSaving(false) }
  }
  return <><Button size="small" onClick={() => setOpen(true)}>添加前置链接</Button><Modal title="添加前置链接" open={open} onCancel={() => setOpen(false)} onOk={() => void save()} confirmLoading={saving} okText="添加并授权" destroyOnClose>
    <Space direction="vertical" style={{ width: '100%' }} size="middle">
      <Typography.Text type="secondary">用于链式代理，导出 Clash 时通过前置代理组连接。</Typography.Text>
      <Input aria-label="前置链接名称" placeholder="前置链接名称" value={name} onChange={event => setName(event.target.value)} />
      <Radio.Group value={mode} onChange={event => { setMode(event.target.value); setURL('') }} options={[{ value: 'third-party', label: '第三方链接' }, { value: 'local', label: '本地 Client 链接' }]} />
      {mode === 'local' ? <><Select aria-label="本地 Client" style={{ width: '100%' }} value={agentID || undefined} showSearch optionFilterProp="label" placeholder="选择本地 Client" options={agents.map(agent => ({ value: agent.agent_id, label: agent.agent_name || agent.agent_id }))} onChange={id => void load(id)} />
        <Select aria-label="本地节点链接" style={{ width: '100%' }} loading={loading} disabled={loading || !agentID} showSearch optionFilterProp="label" placeholder="选择节点 / 客户端链接" value={url || undefined} options={links} onChange={setURL} /></> : <Input.TextArea aria-label="第三方分享链接" placeholder="SS / VLESS / VMess / Trojan / HTTP / SOCKS 分享链接" value={url} onChange={event => setURL(event.target.value)} autoSize={{ minRows: 3, maxRows: 6 }} />}
    </Space>
  </Modal></>
}
