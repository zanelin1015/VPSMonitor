import { Col, Input, Select, Typography } from 'antd'
import type { CustomerRouteConfig } from '../types'
import { routeTypeLabels } from '../lib/agentClassification'

export function CustomerRouteFields({ value, onChange }: {
  value?: CustomerRouteConfig
  onChange: (value: CustomerRouteConfig) => void
}) {
  return <>
    <Col xs={24} md={8}>
      <Typography.Text type="secondary">公开线路名称</Typography.Text>
      <Input aria-label="公开线路名称" value={value?.name || ''} maxLength={120} placeholder="例如 广州 IEPL"
        onChange={event => onChange({ ...value, name: event.target.value })} />
    </Col>
    <Col xs={12} md={6}>
      <Typography.Text type="secondary">线路类型</Typography.Text>
      <Select aria-label="线路类型" style={{ width: '100%' }} value={value?.type || ''} options={[
        { value: '', label: '未指定' }, ...Object.entries(routeTypeLabels).map(([code, label]) => ({ value: code, label })),
      ]} onChange={(type: NonNullable<CustomerRouteConfig['type']> | '') => onChange({ ...value, type: type || undefined })} />
    </Col>
    <Col xs={12} md={6}>
      <Typography.Text type="secondary">客户可见性</Typography.Text>
      <Select aria-label="客户可见性" style={{ width: '100%' }} value={value?.visibility || ''} options={[
        { value: '', label: '按 Client 用途' }, { value: 'public', label: '可授权客户' }, { value: 'internal', label: '仅内部转发' },
      ]} onChange={(visibility: NonNullable<CustomerRouteConfig['visibility']> | '') => onChange({ ...value, visibility: visibility || undefined })} />
    </Col>
  </>
}
