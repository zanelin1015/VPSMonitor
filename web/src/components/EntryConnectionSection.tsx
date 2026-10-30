import { useId } from 'react'
import { Alert, AutoComplete, Button, Card, Col, Collapse, Empty, Input, InputNumber, Row, Select, Space, Switch, Table, Tag, Typography } from 'antd'
import { DeleteOutlined, PlusOutlined, SaveOutlined } from '@ant-design/icons'
import type { AgentEntryConfig, AgentEntryMapping, XUINodeView } from '../types'
import { entryTargets, entryUsesDockerMapping, formatEntryEndpoint, resolveEntryEndpoint, validateEntryConnection } from '../lib/entryConnection'
import './EntryConnectionSection.css'

const { Text, Title } = Typography
interface Props {
  entry: AgentEntryConfig
  nodes: XUINodeView[]
  fallbackHost: string
  domainOptions: { value: string }[]
  addressText: string
  saving: boolean
  onChange: (patch: Partial<AgentEntryConfig>) => void
  onAddressTextChange: (text: string) => void
  onSave: () => void
}

export function EntryConnectionSection({ entry, nodes, fallbackHost, domainOptions, addressText, saving, onChange, onAddressTextChange, onSave }: Props) {
  const id = useId()
  const connection = entry.connection || {}
  const mappings = entry.mappings || []
  const targets = entryTargets(entry, nodes)
  const error = validateEntryConnection(entry)
  const nat = connection.public_access === 'nat'
  const docker = connection.deployment === 'docker'
  const needsPortMapping = docker || nat
  const preview = targets.map((target) => ({ ...target, key: `${target.type}:${target.id}`, endpoint: resolveEntryEndpoint(entry, target, entry.import_domain || fallbackHost) }))
  const update = (index: number, patch: Partial<AgentEntryMapping>) => onChange({ mappings: mappings.map((m, i) => i === index ? { ...m, ...patch } : m) })
  const remove = (index: number) => onChange({ mappings: mappings.filter((_, i) => i !== index) })
  const add = () => onChange({ mappings: [...mappings, { use_for_export: true, target_type: 'xui', target_id: '', transport: 'tcp', internal_port: 0, host_port: 0, external_port: 0 }] })
  const bind = (index: number, key: string) => {
    const target = targets.find((item) => `${item.type}:${item.id}` === key)
    if (target) update(index, { target_type: target.type, target_id: target.id, internal_port: target.port, transport: target.transport, protocol: target.protocol })
  }
  const activeCount = mappings.filter((m) => m.use_for_export).length

  return (
    <Card className="config-section-card entry-connection" bordered={false}>
      <div className="section-title-row">
        <div><Title level={4}>连接与端口</Title><Text type="secondary">设置节点对外连接地址，预览导出的实际入口。</Text></div>
        <Button type="primary" icon={<SaveOutlined />} loading={saving} disabled={!!error} onClick={onSave}>保存连接配置</Button>
      </div>
      <Row gutter={[16, 16]}>
        <Col xs={24} md={8}>
          <label htmlFor={`${id}-deploy`}>x-ui 部署方式</label>
          <Select id={`${id}-deploy`} value={connection.deployment || 'native'} options={[{ value: 'native', label: '直接安装' }, { value: 'docker', label: 'Docker' }]} onChange={(value) => onChange({ connection: { ...connection, deployment: value, docker_network: connection.docker_network || 'host' } })} />
        </Col>
        {docker && <Col xs={24} md={8}>
          <label htmlFor={`${id}-docker`}>Docker 网络方式</label>
          <Select id={`${id}-docker`} value={connection.docker_network || 'host'} options={[{ value: 'host', label: 'Host · 端口直通' }, { value: 'bridge', label: 'Bridge · 端口映射' }]} onChange={(value) => onChange({ connection: { ...connection, docker_network: value } })} />
        </Col>}
        <Col xs={24} md={8}>
          <label htmlFor={`${id}-public`}>公网接入方式</label>
          <Select id={`${id}-public`} value={connection.public_access || 'direct'} options={[{ value: 'direct', label: '直接公网访问' }, { value: 'nat', label: 'NAT · 外层端口映射' }]} onChange={(value) => onChange({ connection: { ...connection, public_access: value } })} />
        </Col>
        {nat ? <Col xs={24} md={16}>
          <label htmlFor={`${id}-host`}>公网入口域名 / IP</label>
          <AutoComplete id={`${id}-host`} allowClear value={connection.public_address || ''} options={domainOptions} placeholder="例如 nat.example.com，填写供应商提供的入站连接地址" onChange={(value) => onChange({ connection: { ...connection, public_address: value } })} />
          <Text type="secondary" className="entry-connection-hint">只填写域名或 IP。NAT 的出口 IP 可能与入口不同，请使用供应商提供的入口地址。</Text>
        </Col> : null}
      </Row>
      <div className="entry-connection-note">
        {docker && connection.docker_network === 'bridge'
          ? '连接路径：公网入口 → 服务器端口 → x-ui 容器内端口。映射在实际服务器配置后，再登记到这里。'
          : docker
            ? '连接路径：公网入口 → Docker Host 端口直通 → 节点监听端口。需要为节点登记入口时，可在下方添加映射。'
            : nat
              ? '连接路径：公网入口 → NAT 外层端口 → 节点监听端口。映射在供应商控制台配置后，再登记到这里。'
              : '连接路径：公网入口 → 节点监听端口。直接安装与直接公网访问无需配置端口映射。'}
      </div>

      {needsPortMapping ? <>
        <div className="section-title-row"><Title level={5}>节点端口映射 <Text type="secondary">({activeCount})</Text></Title><Button icon={<PlusOutlined />} onClick={add}>添加映射</Button></div>
        {!activeCount && <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="为需要导出的节点添加映射，同一节点的所有客户端共用。" />}
        <div className="entry-connection-mappings">
          {mappings.map((mapping, index) => {
            if (!mapping.use_for_export) return null
            const bridge = entryUsesDockerMapping(entry, mapping.target_type)
            const target = targets.find((item) => item.type === mapping.target_type && item.id === mapping.target_id)
            const endpoint = target ? resolveEntryEndpoint(entry, target, entry.import_domain || fallbackHost) : undefined
            const key = `${mapping.target_type}:${mapping.target_id}`
            const fieldID = `${id}-${index}`
            return <div className="entry-connection-mapping" key={index}>
              <div className="entry-connection-mapping-head"><Space><Tag color="blue">映射 {index + 1}</Tag><Text>{target?.name || '选择节点'}</Text></Space><Button danger size="small" icon={<DeleteOutlined />} onClick={() => remove(index)}>删除</Button></div>
              <Row gutter={[12, 12]}>
                <Col xs={24} lg={8}>
                  <label htmlFor={`${fieldID}-target`}>绑定节点</label>
                  <Select id={`${fieldID}-target`} showSearch optionFilterProp="label" value={mapping.target_id ? key : undefined} placeholder="选择 x-ui 入站或转发入口" options={[
                    ...targets.map((item) => ({ value: `${item.type}:${item.id}`, label: `${item.type === 'xui' ? 'x-ui' : item.type.toUpperCase()} · ${item.name} · :${item.port}` })),
                    ...(mapping.target_id && !target ? [{ value: key, label: `节点已不存在 · ${mapping.target_id}`, disabled: true }] : []),
                  ]} onChange={(value) => bind(index, value)} />
                </Col>
                <Col xs={12} lg={4}>
                  <label htmlFor={`${fieldID}-internal`}>{bridge ? '容器内端口' : '监听端口'}</label>
                  <InputNumber id={`${fieldID}-internal`} disabled value={mapping.internal_port || undefined} placeholder="选择节点后读取" />
                  {target && target.port !== mapping.internal_port && <Button type="link" size="small" onClick={() => bind(index, key)}>更新为 {target.port}</Button>}
                </Col>
                {bridge && <Col xs={12} lg={4}><label htmlFor={`${fieldID}-server`}>服务器端口</label><InputNumber id={`${fieldID}-server`} min={1} max={65535} precision={0} value={mapping.host_port || undefined} placeholder="如 30001" onChange={(value) => update(index, { host_port: Number(value || 0) })} /></Col>}
                {nat && <Col xs={12} lg={4}><label htmlFor={`${fieldID}-external`}>公网端口</label><InputNumber id={`${fieldID}-external`} min={1} max={65535} precision={0} value={mapping.external_port || undefined} placeholder="如 333" onChange={(value) => update(index, { external_port: Number(value || 0) })} /></Col>}
                <Col xs={12} lg={4}><label htmlFor={`${fieldID}-transport`}>传输协议</label><Select id={`${fieldID}-transport`} value={mapping.transport || 'tcp'} options={[{ value: 'tcp', label: 'TCP' }, { value: 'udp', label: 'UDP' }, { value: 'both', label: 'TCP + UDP' }]} onChange={(value) => update(index, { transport: value })} /></Col>
                <Col xs={24} lg={12}><label htmlFor={`${fieldID}-address`}>独立入口（可选）</label><Input id={`${fieldID}-address`} value={mapping.address || ''} placeholder="留空使用上方公网入口" onChange={(event) => update(index, { address: event.target.value })} /></Col>
                <Col xs={24} lg={8}><label htmlFor={`${fieldID}-note`}>备注</label><Input id={`${fieldID}-note`} value={mapping.note || ''} onChange={(event) => update(index, { note: event.target.value })} /></Col>
                <Col xs={24} lg={4}><label htmlFor={`${fieldID}-default`}>默认导出入口</label><Switch id={`${fieldID}-default`} checked={!!mapping.default_export} onChange={(checked) => onChange({ mappings: mappings.map((row, i) => i === index ? { ...row, default_export: checked } : checked && row.target_type === mapping.target_type && row.target_id === mapping.target_id ? { ...row, default_export: false } : row) })} /></Col>
              </Row>
              <div className="entry-connection-preview">{!target ? <Text type="warning">请选择当前存在的节点。</Text> : endpoint?.error ? <Text type="warning">{endpoint.error}</Text> : <><Text type="secondary">导出入口</Text> <Text strong copyable>{formatEntryEndpoint(endpoint)}</Text> <Text type="secondary">→ {bridge ? `服务器 :${mapping.host_port} → 容器` : '监听'} :{target.port}</Text></>}</div>
            </div>
          })}
        </div>
      </> : null}
      {error && <Alert type="error" showIcon message={error} />}
      <div className="entry-connection-preview-table">
        <Title level={5}>导出预览</Title>
        <Table size="small" pagination={{ pageSize: 10, hideOnSinglePage: true }} dataSource={preview} scroll={{ x: 640 }} columns={[
          { title: '节点', dataIndex: 'name', width: 220 },
          { title: '监听端口', dataIndex: 'port', width: 110 },
          { title: '对外连接 / 待处理', render: (_, item) => item.endpoint.error ? <Text type="warning">{item.endpoint.error}</Text> : <Text copyable>{formatEntryEndpoint(item.endpoint)}</Text> },
        ]} locale={{ emptyText: needsPortMapping ? '等待节点上报后可绑定映射并预览。' : '等待节点上报后可预览。' }} />
      </div>
      <Collapse items={[{ key: 'legacy', label: `原有地址与拓扑映射 (${mappings.filter((m) => !m.use_for_export).length})`, children: <Space direction="vertical" size="middle" style={{ width: '100%' }}>
        <Text type="secondary">旧映射继续用于拓扑匹配。选择“用于节点导出”后，请绑定节点并确认端口。</Text>
        <div><label htmlFor={`${id}-legacy-domain`}>原有导入域名</label><AutoComplete id={`${id}-legacy-domain`} value={entry.import_domain || ''} options={domainOptions} allowClear onChange={(value) => onChange({ import_domain: value })} /></div>
        <div><label htmlFor={`${id}-addresses`}>拓扑入口地址（每行一个）</label><Input.TextArea id={`${id}-addresses`} value={addressText} autoSize={{ minRows: 2, maxRows: 5 }} onChange={(event) => onAddressTextChange(event.target.value)} /></div>
        {mappings.map((m, index) => !m.use_for_export && <div className="entry-connection-legacy" key={index}><Text>{m.address}:{m.external_port} → :{m.internal_port || m.external_port} · {m.protocol}</Text><Space wrap><Button size="small" onClick={() => update(index, { use_for_export: true, target_type: 'xui', target_id: '', transport: 'tcp' })}>用于节点导出</Button><Button size="small" danger onClick={() => remove(index)}>删除</Button></Space></div>)}
      </Space> }]} />
      <Text type="secondary" className="entry-connection-hint">保存后，新链接、二维码、订阅和新建出站使用新的入口；已导入的静态节点需重新导入，现有出站需重新选择来源并保存。</Text>
    </Card>
  )
}
