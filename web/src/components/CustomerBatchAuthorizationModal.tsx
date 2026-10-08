import { useEffect, useMemo, useRef, useState } from 'react'
import { Alert, App, Button, Input, Modal, Space, Table, Tag, Typography } from 'antd'
import type { CustomerAdminView, CustomerAssignment, CustomerAssignmentDraft } from '../types'
import { fetchJSON } from '../lib/appHelpers'
import { authorizeCustomersBatch, customerMatchesSearch, existingCustomerAuthorization } from '../lib/customerBatchAuthorization'

export function CustomerBatchAuthorizationModal(props: {
  draft: CustomerAssignmentDraft
  onClose: () => void
  onConfigChanged: (agentID?: string) => void | Promise<void>
}) {
  const { draft, onClose, onConfigChanged } = props
  const { message } = App.useApp()
  const [customers, setCustomers] = useState<CustomerAdminView[]>([])
  const [search, setSearch] = useState('')
  const [selectedIDs, setSelectedIDs] = useState<number[]>([])
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [failures, setFailures] = useState<{ customerID: number; username: string; error: string }[]>([])
  const [progress, setProgress] = useState({ completed: 0, total: 0 })
  const pending = useRef(false)
  const filtered = useMemo(() => customers.filter(customer => customerMatchesSearch(customer, search)), [customers, search])

  useEffect(() => {
    let active = true
    void fetchJSON<CustomerAdminView[]>('/api/v1/admin/customers').then(data => {
      if (active) setCustomers(data || [])
    }).catch(reason => {
      if (active) setError(reason instanceof Error ? reason.message : '获取用户失败')
    }).finally(() => {
      if (active) setLoading(false)
    })
    return () => { active = false }
  }, [])

  async function submit() {
    if (pending.current || !selectedIDs.length || !draft.agent_id || !draft.inbound_id) return
    pending.current = true
    setSaving(true)
    setError('')
    setFailures([])
    setProgress({ completed: 0, total: selectedIDs.length })
    try {
      // Recheck scope and existing grants immediately before posting/retrying.
      const latest = await fetchJSON<CustomerAdminView[]>('/api/v1/admin/customers')
      setCustomers(latest || [])
      const result = await authorizeCustomersBatch(latest || [], selectedIDs, draft,
        (id, payload) => fetchJSON<CustomerAssignment>(`/api/v1/admin/customers/${id}/assignments`, {
          method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload),
        }),
        (completed, total) => setProgress({ completed, total }),
      )
      setCustomers((latest || []).map(customer => ({ ...customer, assignments: [
        ...(customer.assignments || []), ...result.created.filter(assignment => assignment.customer_id === customer.id),
      ] })))
      setSelectedIDs(result.failed.map(item => item.customerID))
      setFailures(result.failed)
      const summary = `新增授权 ${result.created.length} 位，跳过已授权 ${result.skippedIDs.length} 位`
      if (result.failed.length) message.warning(`${summary}，失败 ${result.failed.length} 位`)
      else message.success(summary)
      if (result.created.length) {
        try { await onConfigChanged(draft.agent_id) }
        catch { message.warning('授权已保存，刷新数据失败，请手动刷新') }
      }
      if (!result.failed.length) onClose()
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '批量授权失败')
    } finally {
      pending.current = false
      setSaving(false)
    }
  }

  return <Modal
    open title="批量授权给用户" width={760} style={{ top: 24 }}
    styles={{ body: { maxHeight: 'calc(100dvh - 200px)', overflowY: 'auto', overflowX: 'hidden' } }}
    onCancel={() => { if (!pending.current) onClose() }}
    closable={!saving} maskClosable={!saving} keyboard={!saving}
    footer={[
      <Button key="cancel" disabled={saving} onClick={onClose}>取消</Button>,
      <Button key="submit" type="primary" loading={saving} disabled={loading || !selectedIDs.length || !draft.agent_id || !draft.inbound_id} onClick={() => void submit()}>
        {saving ? `正在授权 ${progress.completed}/${progress.total}` : `授权给 ${selectedIDs.length} 位用户`}
      </Button>,
    ]}
  >
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Alert type="info" showIcon message={draft.public_client_name || draft.client_email || draft.inbound_tag || '当前节点'} description="多选用户后批量新增授权，费用继承节点默认价；已存在的授权不会覆盖。" />
      {error ? <Alert type="error" showIcon message={error} action={<Button size="small" onClick={onClose}>关闭后重试</Button>} /> : null}
      {failures.length ? <Alert type="warning" showIcon message="以下用户授权失败，已保留勾选，可重试" description={<ul style={{ margin: 0, paddingLeft: 20 }}>{failures.map(item => <li key={item.customerID}>{item.username}：{item.error}</li>)}</ul>} /> : null}
      <Input.Search aria-label="搜索授权用户" placeholder="搜索用户名 / 显示名称" allowClear value={search} disabled={saving} onChange={event => setSearch(event.target.value)} />
      <Space wrap>
        <Button size="small" disabled={loading || saving} onClick={() => setSelectedIDs(current => [...new Set([...current, ...filtered.filter(customer => !existingCustomerAuthorization(customer, draft)).map(customer => customer.id)])])}>全选搜索结果</Button>
        <Button size="small" disabled={saving || !selectedIDs.length} onClick={() => setSelectedIDs([])}>清空选择</Button>
        <Typography.Text type="secondary">已选择 {selectedIDs.length} 位 · 搜索结果 {filtered.length} 位</Typography.Text>
      </Space>
      <Table<CustomerAdminView>
        size="small" loading={loading} rowKey="id" dataSource={filtered}
        pagination={{ pageSize: 8, hideOnSinglePage: true, showSizeChanger: false }}
        scroll={{ x: 460 }}
        rowSelection={{
          selectedRowKeys: selectedIDs, preserveSelectedRowKeys: true,
          onChange: keys => setSelectedIDs(keys.map(Number)),
          getCheckboxProps: customer => ({ disabled: saving || Boolean(existingCustomerAuthorization(customer, draft)) }),
        }}
        columns={[
          { title: '用户名', dataIndex: 'username', render: (username: string) => <span style={{ overflowWrap: 'anywhere' }}>{username}</span> },
          { title: '显示名称', dataIndex: 'display_name', render: (name?: string) => name || '-' },
          { title: '账号状态', width: 100, render: (_, customer) => <Tag color={customer.enabled ? 'blue' : 'default'}>{customer.enabled ? '启用' : '禁用'}</Tag> },
          { title: '授权状态', width: 150, render: (_, customer) => {
            const existing = existingCustomerAuthorization(customer, draft)
            return existing ? <Tag color={existing.enabled ? 'green' : 'default'}>{existing.enabled ? '已授权' : '已授权（禁用）'}</Tag> : <Typography.Text type="secondary">未授权</Typography.Text>
          } },
        ]}
        locale={{ emptyText: error ? '用户列表加载失败' : search ? '未找到匹配用户' : '暂无可管理用户' }}
      />
    </Space>
  </Modal>
}
