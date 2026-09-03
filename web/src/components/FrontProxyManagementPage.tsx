import { useEffect, useState } from 'react'
import { Alert, App as AntdApp, Button, Card, Col, Empty, Input, Popconfirm, Row, Space, Switch, Table, Tag, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { DeleteOutlined, EditOutlined, PlusOutlined, ReloadOutlined, SaveOutlined } from '@ant-design/icons'

import type { FrontProxyNode } from '../types'
import { fetchJSON, formatDateTime } from '../lib/appHelpers'

const { Text, Title } = Typography

interface FrontProxyFormState {
  name: string
  share_url: string
  enabled: boolean
  remark: string
}

const emptyFrontProxyForm: FrontProxyFormState = {
  name: '',
  share_url: '',
  enabled: true,
  remark: '',
}

export function FrontProxyManagementPage(props: { canManageNodes?: boolean }) {
  const canManageNodes = props.canManageNodes ?? true
  const { message } = AntdApp.useApp()
  const [frontProxies, setFrontProxies] = useState<FrontProxyNode[]>([])
  const [frontProxyForm, setFrontProxyForm] = useState<FrontProxyFormState>(emptyFrontProxyForm)
  const [editingFrontProxyID, setEditingFrontProxyID] = useState<number | null>(null)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    void loadFrontProxies()
  }, [])

  async function loadFrontProxies() {
    setLoading(true)
    try {
      const data = await fetchJSON<FrontProxyNode[]>('/api/v1/admin/front-proxies')
      setFrontProxies(Array.isArray(data) ? data : [])
    } catch (error) {
      setFrontProxies([])
      message.error(error instanceof Error ? error.message : '加载前置代理失败')
    } finally {
      setLoading(false)
    }
  }

  function clearForm() {
    setEditingFrontProxyID(null)
    setFrontProxyForm(emptyFrontProxyForm)
  }

  async function saveFrontProxy() {
    if (!frontProxyForm.name.trim() || !frontProxyForm.share_url.trim()) {
      message.warning('请填写前置代理名称和分享链接')
      return
    }
    setSaving(true)
    try {
      const payload = {
        name: frontProxyForm.name.trim(),
        share_url: frontProxyForm.share_url.trim(),
        enabled: frontProxyForm.enabled,
        remark: frontProxyForm.remark.trim(),
      }
      if (editingFrontProxyID) {
        await fetchJSON<FrontProxyNode>(`/api/v1/admin/front-proxies/${editingFrontProxyID}`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        })
        message.success('前置代理已更新')
      } else {
        await fetchJSON<FrontProxyNode>('/api/v1/admin/front-proxies', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        })
        message.success('前置代理已新增')
      }
      clearForm()
      await loadFrontProxies()
    } catch (error) {
      message.error(error instanceof Error ? error.message : '保存前置代理失败')
    } finally {
      setSaving(false)
    }
  }

  async function deleteFrontProxy(id: number) {
    setSaving(true)
    try {
      await fetchJSON(`/api/v1/admin/front-proxies/${id}`, { method: 'DELETE' })
      if (editingFrontProxyID === id) {
        clearForm()
      }
      message.success('前置代理已删除')
      await loadFrontProxies()
    } catch (error) {
      message.error(error instanceof Error ? error.message : '删除前置代理失败')
    } finally {
      setSaving(false)
    }
  }

  const columns: ColumnsType<FrontProxyNode> = [
    {
      title: '名称',
      dataIndex: 'name',
      width: 180,
      sorter: (left, right) => left.name.localeCompare(right.name, 'zh-CN'),
      render: (value: string, record) => (
        <div>
          <Text strong>{value}</Text>
          <div className="muted-line">{record.protocol || '-'}</div>
        </div>
      ),
    },
    {
      title: '分享链接',
      dataIndex: 'share_url',
      ellipsis: true,
      render: (value?: string) => <Text code>{value || '-'}</Text>,
    },
    {
      title: '备注',
      dataIndex: 'remark',
      width: 220,
      ellipsis: true,
      render: (value?: string) => value || '-',
    },
    {
      title: '状态',
      dataIndex: 'enabled',
      width: 90,
      sorter: (left, right) => Number(left.enabled) - Number(right.enabled),
      render: (enabled: boolean) => <Tag color={enabled ? 'blue' : 'default'}>{enabled ? '启用' : '停用'}</Tag>,
    },
    {
      title: '修改时间',
      dataIndex: 'updated_at',
      width: 180,
      defaultSortOrder: 'descend',
      sorter: (left, right) => Date.parse(left.updated_at) - Date.parse(right.updated_at),
      render: (value: string) => formatDateTime(value),
    },
    ...(canManageNodes ? [{
      title: '操作',
      width: 150,
      render: (_value: unknown, record: FrontProxyNode) => (
        <Space size={6}>
          <Button
            size="small"
            icon={<EditOutlined />}
            onClick={() => {
              setEditingFrontProxyID(record.id)
              setFrontProxyForm({
                name: record.name,
                share_url: record.share_url || '',
                enabled: record.enabled,
                remark: record.remark || '',
              })
            }}
          >
            编辑
          </Button>
          <Popconfirm title="删除该前置代理？" okText="删除" cancelText="取消" onConfirm={() => void deleteFrontProxy(record.id)}>
            <Button size="small" danger icon={<DeleteOutlined />} loading={saving} />
          </Popconfirm>
        </Space>
      ),
    }] : []),
  ]

  return (
    <main className="admin-content-page">
      <Card className="customer-admin-card" bordered={false}>
        <div className="customer-admin-card-head">
          <div>
            <Title level={4}>{canManageNodes ? '第三方前置代理' : '已授权前置代理'}</Title>
            <Text type="secondary">{canManageNodes ? '导入 SS / VLESS / VMess / Trojan / HTTP 等分享链接，授权后可作为客户订阅的前置代理组。' : '以下为管理员已授权给当前区域账号、且处于启用状态的前置代理，可在用户和链路授权中选择使用。'}</Text>
          </div>
          <Space>
            <Button icon={<ReloadOutlined />} loading={loading} onClick={() => void loadFrontProxies()}>刷新</Button>
            {canManageNodes ? <Button onClick={clearForm}>清空</Button> : null}
          </Space>
        </div>
        <Alert
          style={{ marginBottom: 14 }}
          type="info"
          showIcon
          message="前置代理只负责客户订阅链路的前置转发"
          description={canManageNodes ? '新增后还需要在用户或区域账号的授权配置中选择对应代理组；停用或删除会影响引用该代理的订阅链路。' : '区域账号不能新增、编辑或删除前置代理。若列表为空或缺少代理，请联系管理员先启用代理并在区域账号授权配置中分配。'}
        />
        {canManageNodes ? <Row gutter={[12, 12]} align="middle">
          <Col xs={24} md={5}>
            <Text type="secondary">名称</Text>
            <Input value={frontProxyForm.name} onChange={(event) => setFrontProxyForm((current) => ({ ...current, name: event.target.value }))} />
          </Col>
          <Col xs={24} md={10}>
            <Text type="secondary">分享链接</Text>
            <Input value={frontProxyForm.share_url} placeholder="支持 SS / VLESS / VMess / Trojan / HTTP" onChange={(event) => setFrontProxyForm((current) => ({ ...current, share_url: event.target.value }))} />
          </Col>
          <Col xs={24} md={4}>
            <Text type="secondary">状态</Text>
            <div className="customer-admin-switch-row">
              <Switch checked={frontProxyForm.enabled} onChange={(checked) => setFrontProxyForm((current) => ({ ...current, enabled: checked }))} />
              <Text>{frontProxyForm.enabled ? '启用' : '停用'}</Text>
            </div>
          </Col>
          <Col xs={24} md={5}>
            <Button block type="primary" icon={editingFrontProxyID ? <SaveOutlined /> : <PlusOutlined />} loading={saving} onClick={() => void saveFrontProxy()}>
              {editingFrontProxyID ? '保存前置' : '新增前置'}
            </Button>
          </Col>
          <Col xs={24}>
            <Text type="secondary">备注（可选）</Text>
            <Input value={frontProxyForm.remark} placeholder="例如：香港入口 / 仅供某区域账号使用" onChange={(event) => setFrontProxyForm((current) => ({ ...current, remark: event.target.value }))} />
          </Col>
        </Row> : null}
        <Table
          style={{ marginTop: 16 }}
          rowKey={(record) => record.id}
          loading={loading}
          columns={columns}
          dataSource={frontProxies}
          pagination={{ pageSize: 8, hideOnSinglePage: true }}
          scroll={{ x: 1000 }}
          locale={{ emptyText: <Empty description={canManageNodes ? '暂无第三方前置代理' : '暂无已授权前置代理'} /> }}
        />
      </Card>
    </main>
  )
}
