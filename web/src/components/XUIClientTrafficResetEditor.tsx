import { useId, useState } from 'react'
import { Button, Form, InputNumber, Modal, Select, Space, Tooltip, Typography } from 'antd'

import type { XUIClientView } from '../types'
import { CLIENT_TRAFFIC_RESET_OPTIONS, clientTrafficResetDay, clientTrafficResetOptions, clientTrafficResetStartDay, clientTrafficResetSummary, resolveClientTrafficResetChoice } from '../lib/clientTrafficReset'
import type { ClientTrafficResetChoice, ClientTrafficResetPeriod } from '../lib/clientTrafficReset'

export function XUIClientTrafficResetEditor({ record, startTime, canManage, onSave }: {
  record: XUIClientView
  startTime?: number
  canManage: boolean
  onSave: (record: XUIClientView, period: ClientTrafficResetPeriod, day: number) => Promise<boolean>
}) {
  const id = useId()
  const [open, setOpen] = useState(false)
  const [choice, setChoice] = useState<ClientTrafficResetChoice>('never')
  const [day, setDay] = useState<number | null>(1)
  const [saving, setSaving] = useState(false)
  const fromStart = choice === 'monthly_from_start'
  const period = fromStart ? 'monthly' : choice
  const startDay = clientTrafficResetStartDay(startTime)
  const displayedDay = fromStart ? startDay : day
  const validDay = displayedDay !== null && Number.isInteger(displayedDay) && displayedDay >= 1 && displayedDay <= 31
  const settings = resolveClientTrafficResetChoice(choice, day, startTime)
  const canSave = canManage && Boolean(record.email) && record.traffic_reset_supported === true

  async function save() {
    if (!canSave || saving || !settings) return
    setSaving(true)
    try {
      if (await onSave(record, settings.period, settings.day)) setOpen(false)
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
          setChoice(CLIENT_TRAFFIC_RESET_OPTIONS.find((option) => option.value === record.traffic_reset)?.value || 'never')
          setDay(clientTrafficResetDay(record.traffic_reset_day))
          setOpen(true)
        }}>设置</Button>
      </Tooltip>
    </Space>
    <Modal className="xui-traffic-reset-modal" title={`自动流量重置 · ${record.email || '客户端'}`} width={440} open={open}
      okText="保存并下发" cancelText="取消" onOk={() => void save()} onCancel={() => setOpen(false)}
      confirmLoading={saving} maskClosable={!saving} closable={!saving}
      cancelButtonProps={{ disabled: saving }} okButtonProps={{ disabled: !canSave || !settings }}>
      <Form layout="vertical">
        <Form.Item label="重置周期" htmlFor={`${id}-period`} required
          extra={startDay === null ? '设置客户端开始日期后，可选择按开始日每月重置。' : undefined}>
          <Select id={`${id}-period`} aria-label="重置周期" value={choice} options={clientTrafficResetOptions(startTime)}
            disabled={saving} onChange={(nextChoice) => {
              setChoice(nextChoice)
              if (nextChoice === 'monthly_from_start') setDay(startDay)
            }} />
        </Form.Item>
        {period === 'monthly' ? <Form.Item label="每月重置日" htmlFor={`${id}-day`} required
          validateStatus={!validDay ? 'error' : undefined} help={!validDay ? fromStart ? '请先设置客户端开始日期' : '请输入 1–31 的整数' : undefined}>
          <InputNumber id={`${id}-day`} aria-label="每月重置日" min={1} max={31} precision={0} value={displayedDay}
            disabled={saving} addonAfter="号" onChange={(nextDay) => {
              setDay(nextDay)
              if (fromStart) setChoice('monthly')
            }} style={{ width: '100%' }} />
        </Form.Item> : null}
        <Typography.Paragraph type="secondary" style={{ color: 'var(--muted)' }}>
          {period === 'never' ? '关闭客户端自动重置。' : '由 x-ui 按面板时区清零已用流量。'}
          保存设置不会立即清零，也不修改总流量上限或到期时间。
          {fromStart ? '按当前开始日保存每月重置日，收费周期不影响重置周期；开始日期调整后需重新选择并保存。' : null}
          {period === 'monthly' ? '在指定日 00:00 执行；当月没有该日期时在月底执行，后续月份仍使用原日号。' : null}
          {period === 'weekly' ? '在每周日 00:00 执行。' : null}
          同一客户端关联多个入站时会同步更新。
        </Typography.Paragraph>
      </Form>
    </Modal>
  </>
}
