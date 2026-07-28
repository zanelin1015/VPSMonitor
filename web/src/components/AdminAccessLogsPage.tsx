import type { Dispatch, SetStateAction } from 'react'
import { Button, Card, Col, Input, InputNumber, Row, Select, Space, Table, Typography } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'

import type { AccessLogEntry, DashboardAgentView } from '../types'
import type { AccessLogFilters } from '../hooks/useAdminSystemTools'
import { formatDateTime } from '../lib/appHelpers'

const { Text, Title } = Typography

export function AdminAccessLogsPage(props: {
  agents: DashboardAgentView[]
  filters: AccessLogFilters
  logs: AccessLogEntry[]
  loading: boolean
  total: number
  onFiltersChange: Dispatch<SetStateAction<AccessLogFilters>>
  onLoad: () => void
}) {
  const { agents, filters, logs, loading, total, onFiltersChange, onLoad } = props
  const agentOptions = [
    { value: '', label: '全部 Client' },
    ...agents.map((agent) => ({ value: agent.agent_id, label: agent.agent_name || agent.agent_id })),
  ]
  const columns = [
    {
      title: '时间',
      dataIndex: 'created_at',
      width: 180,
      render: (value: string) => formatDateTime(value),
    },
    {
      title: 'Client',
      dataIndex: 'agent_id',
      width: 180,
      render: (value: string, record: AccessLogEntry) => record.agent_name || agents.find((agent) => agent.agent_id === value)?.agent_name || value,
    },
    {
      title: '来源',
      width: 170,
      render: (_: unknown, record: AccessLogEntry) => `${record.source_ip || '-'}${record.source_port ? `:${record.source_port}` : ''}`,
    },
    {
      title: '目标',
      width: 220,
      render: (_: unknown, record: AccessLogEntry) => `${record.target_host || record.target_ip || '-'}${record.target_port ? `:${record.target_port}` : ''}`,
    },
    {
      title: '协议',
      width: 90,
      render: (_: unknown, record: AccessLogEntry) => (record.network || record.protocol || '-').toUpperCase(),
    },
    {
      title: '出站',
      dataIndex: 'outbound_tag',
      width: 140,
      render: (value: string) => value || '-',
    },
    {
      title: '客户端',
      dataIndex: 'client_email',
      width: 180,
      render: (value: string) => value || '-',
    },
    {
      title: '摘要',
      dataIndex: 'raw_summary',
      ellipsis: true,
      render: (value: string) => value || '-',
    },
  ]

  return (
    <main className="admin-content-page">
      <Card className="config-section-card" bordered={false}>
        <div className="section-title-row">
          <Title level={4}>访问日志</Title>
          <Space>
            <Text type="secondary">显示 {logs.length} / {total}</Text>
            <Button type="primary" icon={<ReloadOutlined />} loading={loading} onClick={onLoad}>查询</Button>
          </Space>
        </div>
        <Row gutter={[12, 12]}>
          <Col xs={24} md={6}>
            <Text type="secondary">Client</Text>
            <Select
              showSearch
              style={{ width: '100%' }}
              value={filters.agent_id}
              options={agentOptions}
              optionFilterProp="label"
              onChange={(value) => onFiltersChange((current) => ({ ...current, agent_id: value }))}
            />
          </Col>
          <Col xs={24} md={5}>
            <Text type="secondary">来源 IP</Text>
            <Input value={filters.source_ip} onChange={(event) => onFiltersChange((current) => ({ ...current, source_ip: event.target.value }))} />
          </Col>
          <Col xs={24} md={5}>
            <Text type="secondary">目标域名/IP</Text>
            <Input value={filters.target} onChange={(event) => onFiltersChange((current) => ({ ...current, target: event.target.value }))} />
          </Col>
          <Col xs={24} md={5}>
            <Text type="secondary">客户端 Email</Text>
            <Input value={filters.client_email} onChange={(event) => onFiltersChange((current) => ({ ...current, client_email: event.target.value }))} />
          </Col>
          <Col xs={24} md={3}>
            <Text type="secondary">条数</Text>
            <InputNumber
              style={{ width: '100%' }}
              min={20}
              max={500}
              precision={0}
              value={filters.limit}
              onChange={(value) => onFiltersChange((current) => ({ ...current, limit: Number(value || 100) }))}
            />
          </Col>
        </Row>
        <Table
          style={{ marginTop: 16 }}
          rowKey={(record) => String(record.id || `${record.agent_id}-${record.created_at}-${record.raw_summary}`)}
          size="small"
          loading={loading}
          columns={columns}
          dataSource={logs}
          pagination={false}
          scroll={{ x: 1200 }}
        />
      </Card>
    </main>
  )
}
