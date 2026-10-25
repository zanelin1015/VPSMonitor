import { Alert, Button, Card, Col, Empty, Input, List, Row, Space, Spin, Switch, Typography } from 'antd'
import { DeleteOutlined, EditOutlined, ReloadOutlined, SaveOutlined } from '@ant-design/icons'

import type { TelegramBot } from '../types'
import type { TelegramBotForm } from '../lib/appHelpers'
import { formatDateTime } from '../lib/appHelpers'

const { Text, Title } = Typography

export function TelegramBotPanel(props: {
  areaManager?: boolean
  bots: TelegramBot[]
  loading: boolean
  saving: boolean
  editingID: number | null
  form: TelegramBotForm
  onFormChange: (form: TelegramBotForm) => void
  onSave: () => void
  onRefresh: () => void
  onEdit: (bot: TelegramBot) => void
  onCancelEdit: () => void
  onDelete: (id: number) => void
  onTest: (id: number) => void
}) {
  const { areaManager = false, bots, loading, saving, editingID, form, onFormChange, onSave, onRefresh, onEdit, onCancelEdit, onDelete, onTest } = props
  const update = (patch: Partial<TelegramBotForm>) => onFormChange({ ...form, ...patch })
  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Alert
        type="info"
        showIcon
        message="告警发送方式"
        description={areaManager ? "仅通知当前区域账号有权限的客户端，以及归属本区域的访客和用户消息。客户端到期在每天北京时间 09:00 推送，提前 7 天提醒、3 天内紧急提醒；访客和用户留言即时推送，同一会话每 30 秒最多推送一次，可在客服页面查看完整消息。" : "Client 离线和用户在线客服消息会即时推送；X-UI 采集异常、Xray 异常、X-UI Client 到期、续费周期和流量告警统一在每天北京时间 09:00 推送。同一告警默认 6 小时内不会重复刷屏。"}
      />
      <Card className="config-section-card" bordered={false}>
        <Title level={4}>{editingID ? '编辑机器人' : '新增机器人'}</Title>
        <Row gutter={[16, 16]}>
          <Col xs={24} md={12}>
            <Text type="secondary">名称</Text>
            <Input value={form.name} placeholder="例如: 主告警群" onChange={(event) => update({ name: event.target.value })} />
          </Col>
          <Col xs={24} md={12}>
            <Text type="secondary">Chat ID</Text>
            <Input value={form.chat_id} placeholder="群/频道/个人 chat_id" onChange={(event) => update({ chat_id: event.target.value })} />
          </Col>
          <Col xs={24} md={16}>
            <Text type="secondary">Bot Token</Text>
            <Input.Password
              value={form.bot_token}
              placeholder={editingID ? '留空表示沿用原 token' : '123456:ABC...'}
              onChange={(event) => update({ bot_token: event.target.value })}
            />
          </Col>
          <Col xs={24} md={8}>
            <div className="switch-row">
              <span>启用通知</span>
              <Switch checked={form.enabled} onChange={(checked) => update({ enabled: checked })} />
            </div>
          </Col>
          {areaManager ? <>
            <Col xs={24} md={12}><div className="switch-row"><span>客户端到期提醒</span><Switch aria-label="客户端到期提醒" checked={form.notify_expiry} onChange={checked => update({ notify_expiry: checked })} /></div></Col>
            <Col xs={24} md={12}><div className="switch-row"><span>访客 / 用户消息</span><Switch aria-label="访客和用户消息通知" checked={form.notify_support} onChange={checked => update({ notify_support: checked })} /></div></Col>
          </> : null}
        </Row>
        <Space style={{ marginTop: 16 }}>
          <Button type="primary" icon={<SaveOutlined />} loading={saving} onClick={onSave}>
            {editingID ? '保存机器人' : '新增机器人'}
          </Button>
          {editingID ? <Button onClick={onCancelEdit}>取消编辑</Button> : null}
        </Space>
      </Card>
      <Card className="config-section-card" bordered={false}>
        <div className="section-title-row">
          <Title level={4}>已配置机器人</Title>
          <Button size="small" icon={<ReloadOutlined />} loading={loading} onClick={onRefresh}>
            刷新
          </Button>
        </div>
        <Spin spinning={loading}>
          {bots.length ? (
            <List
              dataSource={bots}
              renderItem={(bot) => (
                <List.Item className="telegram-bot-list-item">
                  <div className="telegram-bot-main">
                    <Text strong>{bot.name}</Text>
                    <div className="muted-line">
                      Chat {bot.chat_id} · Token {bot.has_bot_token ? '已保存' : '未设置'} · {bot.enabled ? '启用' : '停用'} · 更新 {formatDateTime(bot.updated_at)}
                    </div>
                  </div>
                  <Space wrap>
                    <Button size="small" onClick={() => onTest(bot.id)}>
                      测试
                    </Button>
                    <Button size="small" icon={<EditOutlined />} onClick={() => onEdit(bot)}>
                      编辑
                    </Button>
                    <Button size="small" danger icon={<DeleteOutlined />} onClick={() => onDelete(bot.id)}>
                      删除
                    </Button>
                  </Space>
                </List.Item>
              )}
            />
          ) : (
            <Empty description="还没有 Telegram 告警机器人" />
          )}
        </Spin>
      </Card>
    </Space>
  )
}
