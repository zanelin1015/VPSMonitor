import { useEffect, useState } from 'react'
import { Alert, App, Button, Empty, Input, Modal, Space, Table, Tag } from 'antd'
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons'
import { fetchJSON } from '../lib/api'
import { parseTagInput } from '../lib/appHelpersTags'
import type { AgentTagInventoryItem, TagSettingsResponse } from '../types'

const endpoint = '/api/v1/admin/tags'

export function AgentTagManager({ onClose, onChanged }: {
  onClose: () => void
  onChanged: (tags: string[], deletedTag?: string) => void
}) {
  const { message, modal } = App.useApp()
  const [items, setItems] = useState<AgentTagInventoryItem[]>([])
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [draft, setDraft] = useState('')
  const [search, setSearch] = useState('')
  const [error, setError] = useState('')
  const shownItems = items.filter(item => item.tag.toLowerCase().includes(search.trim().toLowerCase()))

  useEffect(() => {
    const controller = new AbortController()
    void fetchJSON<AgentTagInventoryItem[]>(`${endpoint}?details=1`, { signal: controller.signal })
      .then(data => { if (!controller.signal.aborted) setItems(data) })
      .catch(reason => { if (!controller.signal.aborted) setError(reason instanceof Error ? reason.message : '加载标签失败') })
      .finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [])

  async function addTags() {
    const tags = parseTagInput(draft)
    if (!tags.length || saving) return
    setSaving(true)
    setError('')
    try {
      const result = await fetchJSON<TagSettingsResponse>(endpoint, {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ tags }),
      })
      onChanged(result.tags || [])
      setDraft('')
      message.success('标签已保存')
      setItems(await fetchJSON<AgentTagInventoryItem[]>(`${endpoint}?details=1`))
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '创建标签失败')
    } finally {
      setSaving(false)
    }
  }

  function confirmDelete(item: AgentTagInventoryItem) {
    modal.confirm({
      title: `删除标签「${item.tag}」？`,
      content: item.agent_count
        ? `当前有 ${item.agent_count} 台 Client 使用该标签。删除时会从标签库及所有引用它的 Client 中移除。`
        : '删除后，此标签会从标签库和下拉选项中移除。',
      okText: '删除',
      okType: 'danger',
      cancelText: '取消',
      onOk: async () => {
        setSaving(true)
        setError('')
        try {
          const result = await fetchJSON<{ items: AgentTagInventoryItem[]; removed_clients: number }>(endpoint, {
            method: 'DELETE', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ tag: item.tag }),
          })
          setItems(result.items)
          onChanged(result.items.map(entry => entry.tag), item.tag)
          message.success(result.removed_clients ? `标签已删除，已从 ${result.removed_clients} 台 Client 移除` : '标签已删除')
        } catch (reason) {
          setError(reason instanceof Error ? reason.message : '删除标签失败')
        } finally {
          setSaving(false)
        }
      },
    })
  }

  return (
    <Modal title="标签管理" open width={660} footer={<Button onClick={onClose} disabled={saving}>关闭</Button>}
      onCancel={saving ? undefined : onClose}>
      <Space direction="vertical" size="middle" style={{ width: '100%' }}>
        {error ? <Alert showIcon type="error" message={error} /> : null}
        <Space.Compact block>
          <Input aria-label="新标签名称" value={draft} maxLength={120} disabled={saving || loading}
            onChange={event => setDraft(event.target.value)} onPressEnter={() => void addTags()} placeholder="输入标签名称" />
          <Button type="primary" icon={<PlusOutlined />} disabled={!draft.trim() || loading} loading={saving} onClick={() => void addTags()}>创建标签</Button>
        </Space.Compact>
        <Input allowClear aria-label="搜索标签" value={search} onChange={event => setSearch(event.target.value)} placeholder="搜索标签" />
        <Table<AgentTagInventoryItem> rowKey="tag" size="small" dataSource={shownItems} loading={loading} pagination={{ pageSize: 8 }}
          locale={{ emptyText: <Empty description={search ? '没有匹配的标签' : '暂无标签'} /> }}
          columns={[
            { title: '标签', dataIndex: 'tag', render: (_, item) => <span style={{ overflowWrap: 'anywhere' }}>{item.tag} {item.protected ? <Tag>系统角色</Tag> : null}</span> },
            { title: '使用 Client 数', dataIndex: 'agent_count', width: 125, sorter: (a, b) => a.agent_count - b.agent_count },
            { title: '操作', width: 130, render: (_, item) => item.protected ? '在 Client 角色中管理' :
              <Button type="link" danger size="small" icon={<DeleteOutlined />} disabled={saving} onClick={() => confirmDelete(item)} aria-label={`删除标签 ${item.tag}`}>删除</Button> },
          ]} />
      </Space>
    </Modal>
  )
}
