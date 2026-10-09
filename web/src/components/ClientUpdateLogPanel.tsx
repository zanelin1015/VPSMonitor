import { useEffect, useState } from 'react'
import { Alert, Button, Collapse, Input, Select, Space, Table, Tag, Typography } from 'antd'
import type { ClientUpdateLog, UpdateAgentStatus } from '../types'
import { fetchJSON } from '../lib/api'
import { clientUpdateResult, filterClientUpdateLogs } from '../lib/clientUpdateLogs'

const date = (value?: string) => value ? new Date(value).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai', hour12: false }) : '-'

export function ClientUpdateLogPanel({ open, dispatching, candidates = [], targetVersion }: {
  open: boolean; dispatching: boolean; candidates?: UpdateAgentStatus[]; targetVersion: string
}) {
  const [logs, setLogs] = useState<ClientUpdateLog[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [query, setQuery] = useState('')
  const [result, setResult] = useState('all')
  const load = async () => {
    setLoading(true)
    try {
      const response = await fetchJSON<{ items: ClientUpdateLog[] }>('/api/v1/admin/updates/client-logs')
      setLogs(response.items || []); setError('')
    } catch (err) { setError(err instanceof Error ? err.message : '读取 Client 升级日志失败') }
    finally { setLoading(false) }
  }
  useEffect(() => {
    if (!open || dispatching) return
    let cancelled = false
    const refresh = async () => {
      try {
        const response = await fetchJSON<{ items: ClientUpdateLog[] }>('/api/v1/admin/updates/client-logs')
        if (!cancelled) { setLogs(response.items || []); setError('') }
      } catch (err) { if (!cancelled) setError(err instanceof Error ? err.message : '读取 Client 升级日志失败') }
      finally { if (!cancelled) setLoading(false) }
    }
    setLoading(true); void refresh()
    const timer = window.setInterval(() => void refresh(), 10000)
    return () => { cancelled = true; window.clearInterval(timer) }
  }, [open, dispatching])
  return <Space direction="vertical" size="middle" style={{ width: '100%', minWidth: 0 }}>
    <Typography.Title level={5}>Client 升级日志（北京时间，最近 200 条）</Typography.Title>
    <Typography.Text type="secondary">逐台记录下发、跳过及原因。下发成功不代表升级完成，重启后上报目标版本才标记成功。弹窗打开时每 10 秒刷新。</Typography.Text>
    {candidates.length > 0 && <Collapse items={[{
      key: 'eligibility', label: `当前升级检查结果（${candidates.length} 台，不是历史执行日志）`,
      children: <Table<UpdateAgentStatus> size="small" dataSource={candidates} rowKey="agent_id" pagination={{ pageSize: 10 }} scroll={{ x: 950 }} columns={[
        { title: 'Client', width: 210, render: (_, client) => <>{client.agent_name || client.agent_id}<br /><Typography.Text type="secondary">{client.agent_id}</Typography.Text></> },
        { title: '当前 → 目标版本', width: 180, render: (_, client) => <>{client.version || '未知'} → {targetVersion}</> },
        { title: '检查结果', width: 130, render: (_, client) => <Tag color={client.update_available ? 'blue' : 'default'}>{client.update_available ? '可在线升级' : '不可升级 / 无需升级'}</Tag> },
        { title: '原因', dataIndex: 'reason', width: 370 },
      ]} />,
    }]} />}
    <Space wrap>
      <Input.Search allowClear placeholder="搜索 Client / 版本 / 原因 / 批次" value={query} onChange={(e) => setQuery(e.target.value)} style={{ width: 300, maxWidth: '100%' }} />
      <Select value={result} onChange={setResult} style={{ width: 145 }} options={[
        { value: 'all', label: '全部结果' }, { value: 'skipped', label: '只看跳过' }, { value: 'failed', label: '失败 / 未确认' }, { value: 'dispatched', label: '已下发 / 执行中' }, { value: 'confirmed', label: '升级成功' },
      ]} />
      <Button onClick={() => void load()} loading={loading} disabled={dispatching}>刷新日志</Button>
    </Space>
    {error && <Alert type="error" showIcon message="读取升级日志失败" description={error} />}
    <div style={{ width: '100%', minWidth: 0, maxWidth: '100%' }}>
      <Table<ClientUpdateLog> size="small" loading={loading} dataSource={filterClientUpdateLogs(logs, query, result)} rowKey="id" pagination={{ pageSize: 10 }} scroll={{ x: 1230 }} locale={{ emptyText: '暂无升级日志，日志功能上线后开始记录；历史跳过无法追溯' }} columns={[
        { title: 'Client', width: 210, fixed: 'left', render: (_, log) => <><Typography.Text strong>{log.agent_name || log.agent_id}</Typography.Text>{log.force && <Tag color="volcano" style={{ marginLeft: 6 }}>强制</Tag>}<br /><Typography.Text type="secondary">{log.agent_id}</Typography.Text><br />{log.os || '未知系统'} / {log.arch || '未知架构'}</> },
        { title: '记录时间 / 批次', width: 205, render: (_, log) => <>{date(log.created_at)}<br /><Typography.Text type="secondary">{log.batch_id.slice(0, 8)}</Typography.Text>{!!log.action_id && <><br />任务 #{log.action_id}</>}</> },
        { title: '原版本 → 目标版本', width: 175, render: (_, log) => <>{log.version || '未知'} → {log.target_version}</> },
        { title: '结果', width: 160, render: (_, log) => <Tag color={clientUpdateResult(log).color}>{clientUpdateResult(log).label}</Tag> },
        { title: '详情 / 跳过原因', width: 310, render: (_, log) => <>{clientUpdateResult(log).detail}{log.claimed_at && <div className="muted-line">领取：{date(log.claimed_at)}</div>}{log.completed_at && <div className="muted-line">回执：{date(log.completed_at)}</div>}{log.confirmed_at && <div className="muted-line">版本确认：{date(log.confirmed_at)}</div>}</> },
      ]} />
    </div>
  </Space>
}
