import { useEffect, useState } from 'react'
import { Alert, App, Checkbox, Form, Modal, Select, Space, Typography } from 'antd'
import type { XUIClientView, XUIOverview } from '../types'
import { fetchJSON } from '../lib/api'
import { routeTypeLabels } from '../lib/agentClassification'

interface EntranceOption {
  key: string
  name: string
  endpoint: string
  route_type?: keyof typeof routeTypeLabels
  granted: boolean
}

function xrayClientKey(client: XUIClientView) {
  return JSON.stringify([client.inbound_id, client.client_id || client.email])
}

export function CustomerEntranceAuthorizationModal({ customerID, agentOptions, onClose, onSaved }: {
  customerID: number
  agentOptions: Array<{ value: string; label: string }>
  onClose: () => void
  onSaved: () => Promise<void>
}) {
  const { message } = App.useApp()
  const [agentID, setAgentID] = useState('')
  const [clientKey, setClientKey] = useState('')
  const [clients, setClients] = useState<XUIClientView[]>([])
  const [entrances, setEntrances] = useState<EntranceOption[]>([])
  const [selected, setSelected] = useState<string[]>([])
  const [loadingClients, setLoadingClients] = useState(false)
  const [loadingEntrances, setLoadingEntrances] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const baseURL = `/api/v1/admin/customers/${customerID}/entrances`
  const client = clients.find(item => xrayClientKey(item) === clientKey)

  useEffect(() => {
    if (!agentID) return
    const controller = new AbortController()
    setLoadingClients(true)
    void fetchJSON<XUIOverview>(`/api/v1/agents/${encodeURIComponent(agentID)}/xui/overview?assignment_scope=1`, { signal: controller.signal })
      .then(overview => {
        if (!controller.signal.aborted) setClients((overview.clients || []).filter(item => !item.forward_type && (item.client_id || item.email)))
      })
      .catch(reason => { if (!controller.signal.aborted) setError(reason instanceof Error ? reason.message : '读取 Xray 客户端失败') })
      .finally(() => { if (!controller.signal.aborted) setLoadingClients(false) })
    return () => controller.abort()
  }, [agentID])

  useEffect(() => {
    if (!client) return
    const controller = new AbortController()
    setLoadingEntrances(true)
    void fetchJSON<EntranceOption[]>(baseURL, {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, signal: controller.signal,
      body: JSON.stringify({ agent_id: agentID, inbound_id: client.inbound_id, client_id: client.client_id || '', client_email: client.email || '', preview: true }),
    })
      .then(data => { if (!controller.signal.aborted) setEntrances(data) })
      .catch(reason => { if (!controller.signal.aborted) setError(reason instanceof Error ? reason.message : '读取入口失败') })
      .finally(() => { if (!controller.signal.aborted) setLoadingEntrances(false) })
    return () => controller.abort()
  }, [agentID, baseURL, client])

  function selectClient(key: string) {
    setClientKey(key)
    setEntrances([])
    setSelected([])
    setLoadingEntrances(false)
    setError('')
  }

  async function save() {
    if (!client || !selected.length || saving) return
    setSaving(true)
    setError('')
    try {
      const result = await fetchJSON<{ created: number }>(baseURL, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ agent_id: agentID, inbound_id: client.inbound_id, client_id: client.client_id || '', client_email: client.email || '', entries: selected }),
      })
      message.success(`已新增 ${result.created} 个入口授权`)
      await onSaved()
      onClose()
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '保存入口授权失败')
    } finally {
      setSaving(false)
    }
  }

  return <Modal open title="授权多个入口" okText="保存授权" cancelText="取消" width={640}
    onCancel={saving ? undefined : onClose} onOk={() => void save()} confirmLoading={saving}
    okButtonProps={{ disabled: !selected.length || loadingEntrances }}>
    <Form layout="vertical" disabled={saving}>
      <Form.Item label="Xray 线路机">
        <Select value={agentID || undefined} showSearch optionFilterProp="label" options={agentOptions} onChange={value => {
          setAgentID(value)
          setClients([])
          selectClient('')
        }} placeholder="选择运行 Xray 的 Client" />
      </Form.Item>
      <Form.Item label="Xray 入站和客户端">
        <Select value={clientKey || undefined} showSearch optionFilterProp="label" loading={loadingClients} disabled={!agentID}
          options={clients.map(item => ({ value: xrayClientKey(item), label: `${item.inbound_remark || item.inbound_tag || item.inbound_id} · ${item.email || '客户端'}` }))}
          onChange={selectClient} placeholder="选择共用流量和到期时间的客户端" />
      </Form.Item>
      {error ? <Alert type="error" message={error} showIcon /> : null}
      {client && !error ? <Form.Item label="可用入口">
        {loadingEntrances ? <Typography.Text type="secondary">正在解析入口</Typography.Text>
          : entrances.length === 0 ? <Typography.Text type="secondary">暂无可授权的入口</Typography.Text>
          : <Checkbox.Group value={selected} onChange={setSelected}>
            <Space direction="vertical">
              {entrances.map(entry => <Checkbox key={entry.key} value={entry.key} disabled={entry.granted}>
                {entry.name} · {entry.endpoint}{entry.route_type ? ` · ${routeTypeLabels[entry.route_type]}` : ''}{entry.granted ? ' · 已授权' : ''}
              </Checkbox>)}
            </Space>
          </Checkbox.Group>}
      </Form.Item> : null}
    </Form>
  </Modal>
}
