import { useId, useState } from 'react'
import { Button, Form, InputNumber, Modal, Select, Space, Tooltip, Typography } from 'antd'

import type { XUIClientView } from '../types'
import { CLIENT_TRAFFIC_RESET_OPTIONS, clientTrafficResetDay, clientTrafficResetSummary } from '../lib/clientTrafficReset'
import type { ClientTrafficResetPeriod } from '../lib/clientTrafficReset'

export function XUIClientTrafficResetEditor({ record, canManage, onSave }: {
  record: XUIClientView
  canManage: boolean
  onSave: (record: XUIClientView, period: ClientTrafficResetPeriod, day: number) => Promise<boolean>
}) {
  const id = useId()
  const [open, setOpen] = useState(false)
  const [period, setPeriod] = useState<ClientTrafficResetPeriod>('never')
  const [day, setDay] = useState<number | null>(1)
  const [saving, setSaving] = useState(false)
  const validDay = day !== null && Number.isInteger(day) && day >= 1 && day <= 31
  const canSave = canManage && Boolean(record.email) && record.traffic_reset_supported === true

  async function save() {
    if (!canSave || saving || (period === 'monthly' && !validDay)) return
    setSaving(true)
    try {
      if (await onSave(record, period, validDay ? day! : 1)) setOpen(false)
    } finally {
      setSaving(false)
    }
  }

  return <>
    <Space size={4} wrap>
      <Typography.Text type="secondary" style={{ color: 'var(--muted)' }}>自动重置</Typography.Text>
      <Typography.Text>{clientTrafficResetSummary(record)}</Typography.Text>
      <Tooltip title={!record.traffic_reset_supported ? '当前上报中没有可用的客户端重置配置，请刷新采集或升级 x-ui 面板' : '设置 x-ui 客户端自动流量重置'}>
        <Button size="small" disabled={!canSave} onClick={() => {
          setPeriod(CLIENT_TRAFFIC_RESET_OPTIONS.find((option) => option.value === record.traffic_reset)?.value || 'never')
          setDay(clientTrafficResetDay(record.traffic_reset_day))
          setOpen(true)
        }}>设置</Button>
      </Tooltip>
    </Space>
    <Modal className="xui-traffic-reset-modal" title={`自动流量重置 · ${record.email || '客户端'}`} width={440} open={open}
      okText="保存并下发" cancelText="取消" onOk={() => void save()} onCancel={() => setOpen(false)}
      confirmLoading={saving} maskClosable={!saving} closable={!saving}
      cancelButtonProps={{ disabled: saving }} okButtonProps={{ disabled: !canSave || (period === 'monthly' && !validDay) }}>
      <Form layout="vertical">
        <Form.Item label="重置周期" htmlFor={`${id}-period`} required>
          <Select id={`${id}-period`} aria-label="重置周期" value={period} options={[...CLIENT_TRAFFIC_RESET_OPTIONS]}
            disabled={saving} onChange={setPeriod} />
        </Form.Item>
        {period === 'monthly' ? <Form.Item label="每月重置日" htmlFor={`${id}-day`} required
          validateStatus={!validDay ? 'error' : undefined} help={!validDay ? '请输入 1–31 的整数' : undefined}>
          <InputNumber id={`${id}-day`} aria-label="每月重置日" min={1} max={31} precision={0} value={day}
            disabled={saving} addonAfter="号" onChange={setDay} style={{ width: '100%' }} />
        </Form.Item> : null}
        <Typography.Paragraph type="secondary" style={{ color: 'var(--muted)' }}>
          {period === 'never' ? '关闭客户端自动重置。' : '由 x-ui 按面板时区清零已用流量。'}
          保存设置不会立即清零，也不修改总流量上限或到期时间。
          {period === 'monthly' ? '在指定日 00:00 执行；当月没有该日期时在月底执行。' : null}
          {period === 'weekly' ? '在每周日 00:00 执行。' : null}
          同一客户端关联多个入站时会同步更新。
        </Typography.Paragraph>
      </Form>
    </Modal>
  </>
}
