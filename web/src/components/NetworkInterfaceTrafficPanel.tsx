import { useState } from 'react'
import { Alert, Button, Card, Col, Input, Row, Select, Space, Statistic, Table, Tag, Typography } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { NetworkInterfaceTrafficResponse, NetworkInterfaceTrafficUsage } from '../types'
import { useAgentTrafficUsage } from '../hooks/useAgentTrafficUsage'
import { networkInterfaceTrafficValue, selectedNetworkInterfaceTotals, suggestedNetworkInterfaceKeys, type NetworkInterfaceTrafficMode } from '../lib/networkInterfaceTraffic'
import { formatBytes } from '../lib/traffic'
import { formatDateTime } from '../lib/appHelpers'
import { listPagination } from '../lib/pagination'

const { Text } = Typography
const beijingDate = (at: number) => new Intl.DateTimeFormat('sv-SE', { timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit' }).format(at)
const modeLabels = { both: '双向（接收＋发送）', received: '单向接收 RX', sent: '单向发送 TX' }
const kinds: Record<string, string> = { physical: '设备网卡', virtual: '虚拟接口', bridge: '网桥', bond: '聚合接口', loopback: '回环' }

export function NetworkInterfaceTrafficPanel({ agentID }: { agentID: string }) {
  const today = beijingDate(Date.now())
  const [range, setRange] = useState(() => ({ from: beijingDate(Date.now() - 6 * 86400000), to: today }))
  const [draft, setDraft] = useState(range)
  const [rangeError, setRangeError] = useState('')
  const [mode, setMode] = useState<NetworkInterfaceTrafficMode>('both')
  const [manualSelection, setManualSelection] = useState<{ agentID: string; keys: string[] } | null>(null)
  const { data, error, loading, refresh } = useAgentTrafficUsage<NetworkInterfaceTrafficResponse>(agentID, 'interfaces/traffic', range.from, range.to)
  const items = data?.items || []
  const keys = manualSelection?.agentID === agentID ? manualSelection.keys.filter(key => items.some(item => item.key === key)) : suggestedNetworkInterfaceKeys(items)
  const totals = selectedNetworkInterfaceTotals(items, keys)
  const columns: ColumnsType<NetworkInterfaceTrafficUsage> = [
    { title: '自动识别的网卡', key: 'name', width: 240, render: (_, item) => <Space direction="vertical" size={0}><Space wrap><Text strong>{item.name}</Text>{item.primary && <Tag color="blue">默认选择</Tag>}<Tag>{kinds[item.kind] || item.kind}</Tag></Space>{item.mac && <Text type="secondary">{item.mac}</Text>}</Space> },
    { title: '最近状态', key: 'status', width: 130, render: (_, item) => <Tag color={!item.present ? 'default' : item.up ? 'green' : 'orange'}>{!item.present ? '本次未上报' : item.up ? '已启用' : '未启用'}</Tag> },
    { title: '接收 RX', dataIndex: 'received_bytes', width: 130, render: (value: number) => formatBytes(value), sorter: (a,b) => a.received_bytes-b.received_bytes },
    { title: '发送 TX', dataIndex: 'sent_bytes', width: 130, render: (value: number) => formatBytes(value), sorter: (a,b) => a.sent_bytes-b.sent_bytes },
    { title: `统计值 · ${modeLabels[mode]}`, key: 'counted', width: 190, render: (_, item) => formatBytes(networkInterfaceTrafficValue(item, mode)), sorter: (a,b) => networkInterfaceTrafficValue(a,mode)-networkInterfaceTrafficValue(b,mode) },
    { title: '统计起点', dataIndex: 'first_observed_at', width: 185, render: (value: string) => formatDateTime(value) },
    { title: '最近采集', dataIndex: 'last_observed_at', width: 185, render: (value: string) => formatDateTime(value) },
  ]
  function applyRange(next: { from: string; to: string }) {
    const days = (Date.parse(next.to) - Date.parse(next.from)) / 86400000
    if (!next.from || !next.to || !Number.isFinite(days) || days < 0 || days > 365) { setRangeError('请选择有效的起止日期，单次最多查询 366 天'); return }
    setRangeError('')
    setDraft(next)
    if (next.from === range.from && next.to === range.to) refresh()
    setRange(next)
  }
  return <Card bordered={false} title="网卡周期流量" extra={<Button icon={<ReloadOutlined />} loading={loading} onClick={refresh}>刷新</Button>}>
    <Space wrap style={{ marginBottom: 16 }}>
      <Input aria-label="网卡流量开始日期" type="date" value={draft.from} style={{ width: 160 }} onChange={event => setDraft(current => ({ ...current, from: event.target.value }))} />
      <Text>至</Text>
      <Input aria-label="网卡流量结束日期" type="date" value={draft.to} style={{ width: 160 }} onChange={event => setDraft(current => ({ ...current, to: event.target.value }))} />
      <Button type="primary" onClick={() => applyRange(draft)}>查询</Button>
      <Button onClick={() => applyRange({ from: today, to: today })}>今日</Button>
      <Button onClick={() => applyRange({ from: beijingDate(Date.now() - 6*86400000), to: today })}>近 7 天</Button>
      <Button onClick={() => applyRange({ from: `${today.slice(0,7)}-01`, to: today })}>本月</Button>
    </Space>
    {(rangeError || error) && <Alert showIcon type="error" message={rangeError || error} style={{ marginBottom: 16 }} />}
    {data && <>
      {!data.reported && <Alert showIcon type="info" message="尚未上报逐网卡数据，请更新 Client；自动发现后将开始保存周期流量。" style={{ marginBottom: 16 }} />}
      {data.error && <Alert showIcon type="warning" message="最近一次网卡采集失败，当前展示已保存的历史" description={data.error} style={{ marginBottom: 16 }} />}
      {!!data.warnings?.length && <Alert showIcon type="warning" message="部分网卡本次未采集" description={data.warnings.join('；')} style={{ marginBottom: 16 }} />}
      <Space wrap style={{ marginBottom: 12 }}>
        <Text type="secondary">统计周期：{data.from} 至 {data.to}（北京时间）</Text>
        <Text>统计口径</Text>
        <Select aria-label="网卡流量统计口径" value={mode} onChange={setMode} style={{ width: 200 }} options={Object.entries(modeLabels).map(([value,label]) => ({ value,label }))} />
        <Text>已选 {keys.length} 张网卡</Text>
        <Button onClick={() => setManualSelection(null)}>自动选择主出网卡</Button>
      </Space>
      <Row gutter={[16,16]} style={{ marginBottom: 16 }}>
        <Col xs={24} md={8}><Statistic title="选定网卡接收 RX" value={formatBytes(totals.received_bytes)} /></Col>
        <Col xs={24} md={8}><Statistic title="选定网卡发送 TX" value={formatBytes(totals.sent_bytes)} /></Col>
        <Col xs={24} md={8}><Statistic title={modeLabels[mode]} value={formatBytes(networkInterfaceTrafficValue(totals,mode))} /></Col>
      </Row>
    </>}
    <Table rowKey="key" columns={columns} dataSource={items} loading={loading} pagination={listPagination} scroll={{ x: 1200 }}
      rowSelection={{ selectedRowKeys: keys, onChange: values => setManualSelection({ agentID, keys: values.map(String) }), preserveSelectedRowKeys: true }}
      locale={{ emptyText: '暂无逐网卡采集记录' }} />
    <Text type="secondary">自动发现 Client 所在系统的接口，默认按路由建议选择一张主出网卡。每张网卡独立保存收发字节，包含所有程序的流量。网桥、VLAN、隧道与其底层网卡可能重复经过同一份数据，请按统计目的选择，避免重复汇总。历史从首次有效采样开始，日期边界精度取决于上报间隔。</Text>
  </Card>
}
