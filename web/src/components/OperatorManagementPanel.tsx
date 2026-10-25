import { useState } from 'react'
import { App as AntdApp, Button, Card, Input, Modal, Popconfirm, Space, Switch, Table, Tag, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { DeleteOutlined, EditOutlined, PlusOutlined, ReloadOutlined, SaveOutlined } from '@ant-design/icons'

import type { OperatorAdminView } from '../types'

const { Text, Title } = Typography
const DEFAULT_OPERATOR_PASSWORD = '12345678'

export interface OperatorDraft {
  id: number | null
  username: string
  display_name: string
  password: string
  enabled: boolean
}

export function emptyOperatorDraft(): OperatorDraft {
  return { id: null, username: '', display_name: '', password: '', enabled: true }
}

export function OperatorManagementPanel(props: {
  operators: OperatorAdminView[]
  loading: boolean
  saving: boolean
  resettingID: number | null
  onReload: () => void
  onSave: (draft: OperatorDraft) => Promise<boolean> | boolean | void
  onDelete: (id: number) => void
  onResetPassword: (id: number) => void
}) {
  const { operators, loading, saving, resettingID, onReload, onSave, onDelete, onResetPassword } = props
  const { message } = AntdApp.useApp()
  const [modalOpen, setModalOpen] = useState(false)
  const [draft, setDraft] = useState<OperatorDraft>(emptyOperatorDraft)

  const openCreate = () => {
    setDraft(emptyOperatorDraft())
    setModalOpen(true)
  }
  const openEdit = (record: OperatorAdminView) => {
    setDraft({
      id: record.id,
      username: record.username,
      display_name: record.display_name || record.username,
      password: '',
      enabled: record.enabled,
    })
    setModalOpen(true)
  }
  const submit = async () => {
    if (!draft.username.trim()) {
      message.warning('请填写运营管理员用户名')
      return
    }
    if (!draft.id && draft.password && draft.password.length < 8) {
      message.warning('新运营管理员密码至少 8 位')
      return
    }
    const result = await onSave(draft)
    if (result !== false) {
      setModalOpen(false)
    }
  }

  const columns: ColumnsType<OperatorAdminView> = [
    {
      title: '账号',
      key: 'account',
      render: (_, record) => <div><strong>{record.display_name || record.username}</strong><div className="muted-line">{record.username}</div></div>,
    },
    { title: '角色', key: 'role', render: () => <Tag color="blue">运营管理员</Tag> },
    { title: '状态', key: 'enabled', render: (_, record) => <Tag color={record.enabled ? 'success' : 'default'}>{record.enabled ? '启用' : '停用'}</Tag> },
    { title: '创建时间', dataIndex: 'created_at', key: 'created_at', render: (value: string) => value ? new Date(value).toLocaleString() : '-' },
    {
      title: '操作',
      key: 'actions',
      width: 250,
      render: (_, record) => (
        <Space wrap size={6}>
          <Button size="small" icon={<EditOutlined />} onClick={() => openEdit(record)}>编辑</Button>
          <Button size="small" loading={resettingID === record.id} onClick={() => onResetPassword(record.id)}>初始化密码</Button>
          <Popconfirm title="删除运营管理员？" description="删除后该账号的登录会话会立即失效。" okText="删除" cancelText="取消" onConfirm={() => onDelete(record.id)}>
            <Button size="small" danger icon={<DeleteOutlined />}>删除</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <Card className="customer-admin-card" bordered={false}>
      <div className="customer-admin-card-head">
        <div>
          <Title level={5}>运营管理员</Title>
          <Text type="secondary">可管理全部客户、授权和订阅，不允许修改 VPS 与系统配置。</Text>
        </div>
        <Space>
          <Button size="small" icon={<ReloadOutlined />} onClick={onReload}>刷新</Button>
          <Button size="small" type="primary" icon={<PlusOutlined />} onClick={openCreate}>新增运营管理员</Button>
        </Space>
      </div>
      <Table<OperatorAdminView>
        rowKey="id"
        size="small"
        loading={loading}
        columns={columns}
        dataSource={operators}
        scroll={{ x: 1000, y: '100%' }}
        pagination={false}
        locale={{ emptyText: '暂无运营管理员' }}
      />
      <Modal
        title={draft.id ? '编辑运营管理员' : '新增运营管理员'}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        destroyOnClose
        footer={(
          <Space>
            <Button onClick={() => setModalOpen(false)}>取消</Button>
            <Button type="primary" icon={<SaveOutlined />} loading={saving} onClick={submit}>{draft.id ? '保存' : '创建'}</Button>
          </Space>
        )}
      >
        <Space direction="vertical" style={{ width: '100%' }} size="middle">
          <div><Text type="secondary">登录用户名</Text><Input value={draft.username} onChange={(event) => setDraft((current) => ({ ...current, username: event.target.value }))} /></div>
          <div><Text type="secondary">显示名称</Text><Input value={draft.display_name} onChange={(event) => setDraft((current) => ({ ...current, display_name: event.target.value }))} /></div>
          <div><Text type="secondary">密码{draft.id ? '（留空不修改）' : `（留空使用 ${DEFAULT_OPERATOR_PASSWORD}）`}</Text><Input.Password value={draft.password} onChange={(event) => setDraft((current) => ({ ...current, password: event.target.value }))} /></div>
          <div className="customer-admin-switch-row"><Switch checked={draft.enabled} onChange={(enabled) => setDraft((current) => ({ ...current, enabled }))} /><Text>{draft.enabled ? '启用' : '停用'}</Text></div>
        </Space>
      </Modal>
    </Card>
  )
}
