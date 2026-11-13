import { useEffect, useState } from 'react'
import { Alert, App, Button, Pagination, Space, Switch, Table } from 'antd'
import { fetchJSON } from '../lib/appHelpers'
import { listPagination } from '../lib/pagination'
import { useCompactLayout } from '../hooks/useCompactLayout'
import { useListPagination } from '../hooks/useListPagination'
import { CompactDataCardList } from './AgentClientTables'

export interface AgentRoleView { agent_id: string; agent_name?: string; line_enabled: boolean; landing_enabled: boolean }
export function AgentRolePanel({ onChanged }: { onChanged?: () => void | Promise<void> }) {
  const { message } = App.useApp()
  const [items, setItems] = useState<AgentRoleView[]>([])
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState('')
  const compact = useCompactLayout()
  const { page, pageSize, onChange: onPageChange } = useListPagination(items.length, 'agent-roles')
  async function load() {
    setLoading(true)
    try { setItems(await fetchJSON<AgentRoleView[]>('/api/v1/admin/agent-roles')) }
    catch (error) { message.error(error instanceof Error ? error.message : '加载 Client 角色失败') }
    finally { setLoading(false) }
  }
  useEffect(() => { void load() }, [])
  async function save(item: AgentRoleView, patch: Partial<AgentRoleView>) {
    setSaving(item.agent_id)
    try {
      const updated = await fetchJSON<AgentRoleView>('/api/v1/admin/agent-roles', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ ...item, ...patch }) })
      setItems(current => current.map(row => row.agent_id === updated.agent_id ? updated : row))
      await onChanged?.()
      message.success('Client 角色已保存')
    } catch (error) { message.error(error instanceof Error ? error.message : '保存失败') }
    finally { setSaving('') }
  }
  const columns = [
    { title: 'Client', key: 'client', render: (_: unknown, item: AgentRoleView) => item.agent_name || item.agent_id },
    ...(['line_enabled', 'landing_enabled'] as const).map(key => ({ title: key === 'line_enabled' ? '线路机' : '落地机', key, render: (_: unknown, item: AgentRoleView) => <Switch aria-label={`${item.agent_name || item.agent_id} ${key === 'line_enabled' ? '线路机' : '落地机'}`} checked={item[key]} loading={saving === item.agent_id} disabled={Boolean(saving)} onChange={checked => void save(item, { [key]: checked })} /> })),
  ]
  return <Space direction="vertical" style={{ width: '100%' }} size="middle">
    <Alert type="info" showIcon message="Client 角色设置" description="Admin 和运营管理员可开启线路机、落地机角色。只有开启对应角色的 Client 才能出现在区域账号授权下拉框中。一个 Client 可同时承担两种角色。" />
    <Button onClick={() => void load()} loading={loading}>刷新</Button>
    {compact ? (
      <>
        <CompactDataCardList columns={columns} dataSource={items.slice((page - 1) * pageSize, page * pageSize)} rowKey={(item) => item.agent_id} />
        {items.length ? <Pagination {...listPagination} current={page} pageSize={pageSize} total={items.length} onChange={onPageChange} /> : null}
      </>
    ) : <Table rowKey="agent_id" loading={loading} dataSource={items} pagination={listPagination} columns={columns} />}
  </Space>
}
