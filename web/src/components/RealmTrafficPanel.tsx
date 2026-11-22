import { useState } from 'react'
import { Alert, Button, Card, Col, Input, Row, Space, Statistic, Table, Typography } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { RealmTrafficUsage } from '../types'
import { useRealmTrafficUsage } from '../hooks/useRealmTrafficUsage'
import { formatBytes } from '../lib/traffic'
import { formatDateTime } from '../lib/appHelpers'
import { formatRealmEndpoint } from '../lib/managedConfigHelpers'
import { listPagination } from '../lib/pagination'

const { Text } = Typography
const beijingDate = (at: number) => new Intl.DateTimeFormat('sv-SE', { timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit' }).format(at)

export function RealmTrafficPanel({ agentID }: { agentID: string }) {
  const today = beijingDate(Date.now())
  const [range, setRange] = useState(() => ({ from: beijingDate(Date.now() - 6 * 86400000), to: today }))
  const [draft, setDraft] = useState(range)
  const [rangeError, setRangeError] = useState('')
  const { data, error, loading, refresh } = useRealmTrafficUsage(agentID, range.from, range.to)
  const items = data?.items || []
  const totals = items.reduce((sum, item) => ({ up: sum.up + item.upload_bytes, down: sum.down + item.download_bytes }), { up: 0, down: 0 })
  const columns: ColumnsType<RealmTrafficUsage> = [
    { title: 'Realm 规则', key: 'name', render: (_, item) => <Space direction="vertical" size={0}><Text strong>{item.name || item.rule_id}</Text><Text type="secondary">{formatRealmEndpoint(item.listen_address, item.listen_port)} · {item.network}</Text></Space> },
    { title: '上行', dataIndex: 'upload_bytes', width: 140, render: (value: number) => formatBytes(value), sorter: (a, b) => a.upload_bytes - b.upload_bytes },
    { title: '下行', dataIndex: 'download_bytes', width: 140, render: (value: number) => formatBytes(value), sorter: (a, b) => a.download_bytes - b.download_bytes },
    { title: '合计', dataIndex: 'total_bytes', width: 140, render: (value: number) => formatBytes(value), defaultSortOrder: 'descend', sorter: (a, b) => a.total_bytes - b.total_bytes },
    { title: '统计起点', dataIndex: 'first_observed_at', width: 185, render: (value: string) => formatDateTime(value) },
    { title: '最近采集', dataIndex: 'last_observed_at', width: 185, render: (value: string) => formatDateTime(value) },
  ]
  function applyRange(next: { from: string; to: string }) {
    const days = (Date.parse(next.to) - Date.parse(next.from)) / 86400000
    if (!next.from || !next.to || !Number.isFinite(days) || days < 0 || days > 365) {
      setRangeError('请选择有效的起止日期，单次最多查询 366 天')
      return
    }
    setRangeError('')
    setDraft(next)
    if (next.from === range.from && next.to === range.to) refresh()
    setRange(next)
  }
  return <Card bordered={false} title="Realm 周期流量" extra={<Button icon={<ReloadOutlined />} loading={loading} onClick={refresh}>刷新</Button>}>
    <Space wrap style={{ marginBottom: 16 }}>
      <Input aria-label="Realm 流量开始日期" type="date" value={draft.from} style={{ width: 160 }} onChange={event => setDraft(current => ({ ...current, from: event.target.value }))} />
      <Text>至</Text>
      <Input aria-label="Realm 流量结束日期" type="date" value={draft.to} style={{ width: 160 }} onChange={event => setDraft(current => ({ ...current, to: event.target.value }))} />
      <Button type="primary" onClick={() => applyRange(draft)}>查询</Button>
      <Button onClick={() => applyRange({ from: today, to: today })}>今日</Button>
      <Button onClick={() => applyRange({ from: beijingDate(Date.now() - 6 * 86400000), to: today })}>近 7 天</Button>
      <Button onClick={() => applyRange({ from: `${today.slice(0, 7)}-01`, to: today })}>本月</Button>
    </Space>
    {(rangeError || error) && <Alert showIcon type="error" message={rangeError || error} style={{ marginBottom: 16 }} />}
    {data && <>
      {!data.reported && <Alert showIcon type="info" message="尚未上报 Realm 流量。请更新 Client，并在 Realm 转发配置中开启“统计端口流量”后保存。" style={{ marginBottom: 16 }} />}
      {data.reported && !data.enabled && <Alert showIcon type="info" message="当前未启用采集；已记录的历史流量仍可查询。" style={{ marginBottom: 16 }} />}
      {data.error && <Alert showIcon type="warning" message={data.error} style={{ marginBottom: 16 }} />}
      <div style={{ marginBottom: 12 }}><Text type="secondary">统计周期：{data.from} 至 {data.to}（北京时间）</Text></div>
      <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
        <Col xs={24} md={8}><Statistic title="所选周期上行" value={formatBytes(totals.up)} /></Col>
        <Col xs={24} md={8}><Statistic title="所选周期下行" value={formatBytes(totals.down)} /></Col>
        <Col xs={24} md={8}><Statistic title="所选周期合计" value={formatBytes(totals.up + totals.down)} /></Col>
      </Row>
    </>}
    <Table rowKey="key" columns={columns} dataSource={items} loading={loading} pagination={listPagination} scroll={{ x: 1000 }} locale={{ emptyText: '暂无已采集的 Realm 流量数据' }} />
    <Text type="secondary">按北京时间统计，起止日期均包含当天。上行为客户端发往入口，下行为入口返回客户端；仅计监听侧网络字节，包含包头及重传。多跳链路各跳独立展示，查询整条链路时请选定一跳作为统计口径。启用前的数据无法补算，日期边界精度取决于 Client 上报间隔。</Text>
  </Card>
}
