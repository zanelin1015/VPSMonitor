import { useEffect, useRef, useState } from 'react'
import { Alert, App as AntdApp, Button, Form, Input, InputNumber, Modal, Select, Space, Switch, Tag, Typography } from 'antd'
import type { XUIClientBillingConfig, XUIClientConfigChanges, XUIClientConfigRequest, XUIClientConfigSave, XUIClientView } from '../types'
import { fetchJSON, isUnauthorized } from '../lib/api'
import { clientBillingPatchFromStart, dateInputToStartMillis } from '../lib/appHelpersBilling'
import { clientTrafficResetOptions, clientTrafficResetStartDay } from '../lib/clientTrafficReset'
import { accountBasedClient, beijingDateInput, buildClientConfigRequest, CLIENT_CONFIG_FIELD_LABELS, clientConfigSaveStatus, createClientConfigDraft, editClientBilling, retryClientConfigRequest } from '../lib/clientConfig'

export function XUIClientConfigModal({ agentID, record, billing, initialSave, onClose, onSaved, onUnauthorized }: {
  agentID: string
  record: XUIClientView
  billing?: XUIClientBillingConfig
  initialSave?: XUIClientConfigSave
  onClose: () => void
  onSaved: (save: XUIClientConfigSave) => void
  onUnauthorized: () => void
}) {
  const { modal } = AntdApp.useApp()
  const ongoingSave = initialSave && ['pending', 'running', 'failed'].includes(initialSave.action?.status || '') ? initialSave : undefined
  const [baselineRecord, setBaselineRecord] = useState(record)
  const [previousBilling, setPreviousBilling] = useState(billing)
  const [initial, setInitial] = useState(() => {
    const value = createClientConfigDraft(record, ongoingSave?.billing || billing)
    const changes = ongoingSave?.action?.payload?.changes as XUIClientConfigChanges | undefined
    if (changes?.enabled !== undefined) value.enabled = changes.enabled
    if (changes?.total_bytes !== undefined) value.totalGB = changes.total_bytes / 1024 ** 3
    if (changes?.expiry_time !== undefined) { value.billing.expire_time = changes.expiry_time; value.unlimitedExpiry = !changes.expiry_time }
    if (changes?.traffic_reset !== undefined) value.resetChoice = changes.traffic_reset as typeof value.resetChoice
    if (changes?.traffic_reset_day !== undefined) value.resetDay = changes.traffic_reset_day
    return value
  })
  const [draft, setDraft] = useState(initial)
  const [save, setSave] = useState<XUIClientConfigSave | undefined>(ongoingSave)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [pollError, setPollError] = useState('')
  const submitted = useRef<{ signature: string; request: XUIClientConfigRequest; uncertain: boolean } | null>(ongoingSave?.action ? {
    signature: '', uncertain: false,
    request: { request_id: ongoingSave.request_id, client_id: ongoingSave.client_id, inbound_id: ongoingSave.inbound_id, inbound_tag: ongoingSave.inbound_tag, email: ongoingSave.email, billing: ongoingSave.billing, previous_billing: ongoingSave.billing, changes: (ongoingSave.action.payload?.changes || {}) as XUIClientConfigChanges, expected: (ongoingSave.action.payload?.expected || {}) as XUIClientConfigChanges },
  } : null)
  const callbacks = useRef({ onSaved, onUnauthorized })
  callbacks.current = { onSaved, onUnauthorized }
  const pending = save?.action?.status === 'pending' || save?.action?.status === 'running'
  const locked = saving || Boolean(save)
  const account = accountBasedClient(record)
  const dirty = !save && JSON.stringify(draft) !== JSON.stringify(initial)
  const startDay = clientTrafficResetStartDay(draft.billing.start_time)
  const monthly = draft.resetChoice === 'monthly' || draft.resetChoice === 'monthly_from_start'
  const status = clientConfigSaveStatus(save)
  const resultFields = save?.action?.result?.field_results as Record<string, { verified?: boolean; error?: string }> | undefined

  useEffect(() => {
    if (!pending || !save) return
    const controller = new AbortController()
    let busy = false
    const poll = async () => {
      if (busy) return
      busy = true
      try {
        const updated = await fetchJSON<XUIClientConfigSave>(`/api/v1/agents/${encodeURIComponent(agentID)}/xui/client-config/${encodeURIComponent(save.request_id)}`, { signal: controller.signal })
        if (controller.signal.aborted) return
        setSave(updated)
        setPollError('')
        if (!['pending', 'running'].includes(updated.action?.status || '')) callbacks.current.onSaved(updated)
      } catch (error) {
        if (controller.signal.aborted) return
        if (isUnauthorized(error)) callbacks.current.onUnauthorized()
        setPollError('暂时无法查询执行结果，正在重试；可关闭弹窗后在列表查看。')
      } finally { busy = false }
    }
    void poll()
    const timer = window.setInterval(() => void poll(), 3000)
    return () => { controller.abort(); window.clearInterval(timer) }
  }, [agentID, pending, save?.request_id])

  function close() {
    if (saving) return
    if (dirty) modal.confirm({ title: '放弃未保存的修改？', content: '当前修改还没有保存。', okText: '放弃修改', cancelText: '继续编辑', onOk: onClose })
    else onClose()
  }

  async function submit(retry = false) {
    if (saving || pending) return
    setError('')
    let request: XUIClientConfigRequest
    try {
      if (retry && save && submitted.current) {
        request = retryClientConfigRequest(submitted.current.request, save, crypto.randomUUID())
      } else {
        request = buildClientConfigRequest(baselineRecord, previousBilling, initial, draft, crypto.randomUUID())
      }
      const signature = JSON.stringify({ ...request, request_id: '' })
      if (submitted.current?.signature === signature && submitted.current.uncertain) request = submitted.current.request
      submitted.current = { signature, request, uncertain: true }
    } catch (error) { setError(error instanceof Error ? error.message : '请检查填写内容'); return }
    setSaving(true)
    try {
      const saved = await fetchJSON<XUIClientConfigSave>(`/api/v1/agents/${encodeURIComponent(agentID)}/xui/client-config`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(request),
      })
      setSave(saved)
      if (submitted.current) submitted.current.uncertain = false
      callbacks.current.onSaved(saved)
    } catch (error) {
      if (isUnauthorized(error)) callbacks.current.onUnauthorized()
      setError(error instanceof Error ? error.message : '提交结果暂时无法确认，请重试保存')
    } finally { setSaving(false) }
  }

  function editAgain() {
    const fresh = createClientConfigDraft(record, { ...billing, expire_time: Math.max(0, Number(record.expiry_time || 0)) })
    setBaselineRecord(record)
    setPreviousBilling(billing)
    setInitial(fresh)
    setDraft(fresh)
    setSave(undefined)
    setError('')
    setPollError('')
    submitted.current = null
  }

  return <Modal className="xui-client-config-modal" title={`编辑客户端 · ${record.email || '客户端'}`} width={760} open
    onCancel={close} closable={!saving} maskClosable={!saving}
    footer={<Space>
      <Button disabled={saving} onClick={close}>{save ? '关闭' : '取消'}</Button>
      {save?.action?.status === 'failed' ? <Button disabled={saving} onClick={editAgain}>重新编辑</Button> : null}
      {save?.action?.status === 'failed' && submitted.current ? <Button type="primary" loading={saving} onClick={() => void submit(true)}>重试失败项</Button> : !save ? <Button type="primary" loading={saving} onClick={() => void submit()}>保存全部</Button> : null}
    </Space>}>
    <div className="client-config-context"><Tag>{record.inbound_remark || record.inbound_tag}</Tag><Typography.Text type="secondary">{record.protocol} · {record.email}</Typography.Text></div>
    {error ? <Alert type="error" showIcon message="保存未完成" description={error} className="client-config-feedback" /> : null}
    {save ? <Alert className="client-config-feedback" showIcon type={save.action?.status === 'failed' ? 'warning' : pending ? 'info' : 'success'}
      message={status?.text} description={save.action?.status === 'failed' ? <>
        <div>收费配置已保存；远端有修改未确认生效，草稿保留，可重试失败项。</div>
        <div>{save.action.error}</div>
        {Object.entries(resultFields || {}).map(([field, item]) => <div key={field}>{CLIENT_CONFIG_FIELD_LABELS[field] || field}：{item.verified ? '已生效' : '未确认生效'}</div>)}
      </> : pending ? '收费配置已保存，等待 Client 执行并回读核验。可关闭弹窗，列表会保留执行状态。' : save.action ? '收费配置和远端修改均已确认生效。' : '收费配置已保存。'} /> : null}
    {pollError ? <Alert className="client-config-feedback" type="warning" message={pollError} /> : null}
    <Form layout="vertical" disabled={locked} className="client-config-form">
      <section className="client-config-section">
        <Typography.Text strong>使用配置</Typography.Text>
        <div className="client-config-grid">
          <Form.Item label="启用状态"><Space><Switch aria-label="启用状态" checked={draft.enabled} disabled={locked || account} onChange={enabled => setDraft(current => ({ ...current, enabled }))} /><span>{draft.enabled ? '启用' : '禁用'}</span></Space></Form.Item>
          <Form.Item label="总流量上限" extra={account ? '节点账号不支持单独设置流量上限' : '0 GB 表示无上限'}><InputNumber aria-label="总流量上限" min={0} max={8388607} precision={2} addonAfter="GB" value={draft.totalGB} disabled={locked || account} onChange={totalGB => setDraft(current => ({ ...current, totalGB }))} /></Form.Item>
        </div>
      </section>
      <section className="client-config-section">
        <Typography.Text strong>收费配置</Typography.Text>
        <div className="client-config-grid">
          <Form.Item label="收费金额"><InputNumber aria-label="收费金额" min={0} precision={2} value={draft.billing.revenue_amount} onChange={value => setDraft(current => editClientBilling(current, { revenue_amount: value ?? NaN }))} /></Form.Item>
          <Form.Item label="币种"><Select aria-label="币种" value={draft.billing.revenue_currency} options={[{ value: 'CNY', label: 'CNY' }, { value: 'USDT', label: 'USDT' }]} onChange={value => setDraft(current => editClientBilling(current, { revenue_currency: value }))} /></Form.Item>
          <Form.Item label="收费周期"><Select aria-label="收费周期" value={draft.billing.revenue_cycle} options={[{ value: 'month', label: '月付' }, { value: 'quarter', label: '季付' }, { value: 'semiannual', label: '半年付' }, { value: 'year', label: '年付' }]} onChange={value => setDraft(current => editClientBilling(current, { revenue_cycle: value }))} /></Form.Item>
          <Form.Item label="流量倍数"><InputNumber aria-label="流量倍数" min={0.1} max={100} step={0.1} precision={2} addonAfter="倍" value={draft.billing.traffic_multiplier} onChange={value => setDraft(current => editClientBilling(current, { traffic_multiplier: value ?? NaN }))} /></Form.Item>
        </div>
      </section>
      <section className="client-config-section">
        <Typography.Text strong>时间配置</Typography.Text>
        <div className="client-config-grid">
          <Form.Item label="开始日期（北京时间）"><Input aria-label="开始日期" type="date" value={beijingDateInput(draft.billing.start_time)} onChange={event => { const start = dateInputToStartMillis(event.target.value); setDraft(current => editClientBilling(current, { start_time: start })) }} /></Form.Item>
          <Form.Item label="到期限制"><Space><Switch aria-label="不限期" checked={draft.unlimitedExpiry} onChange={unlimitedExpiry => setDraft(current => ({ ...current, unlimitedExpiry, manualExpiry: unlimitedExpiry, billing: { ...current.billing, expire_time: unlimitedExpiry ? 0 : clientBillingPatchFromStart(current.billing.start_time || 0, current.billing.revenue_cycle).expire_time } }))} /><span>不限期</span></Space></Form.Item>
          {!draft.unlimitedExpiry ? <Form.Item className="client-config-full" label="到期时间（北京时间，精确到秒）" extra={account ? '节点账号的到期日期仅用于本系统收费记录' : '手动填写的到期时间优先；保存不会自动续期'}>
            <div className="client-config-expiry-input"><Input aria-label="到期时间" type="datetime-local" step={1} value={beijingDateInput(draft.billing.expire_time, true)} onChange={event => { const expiry = event.target.value ? Date.parse(`${event.target.value}+08:00`) : 0; setDraft(current => ({ ...current, manualExpiry: true, billing: { ...current.billing, expire_time: expiry } })) }} />
              <Button disabled={locked || !draft.billing.start_time} onClick={() => setDraft(current => ({ ...current, manualExpiry: false, billing: { ...current.billing, expire_time: clientBillingPatchFromStart(current.billing.start_time || 0, current.billing.revenue_cycle).expire_time } }))}>按周期计算</Button></div>
          </Form.Item> : null}
        </div>
      </section>
      {!account ? <section className="client-config-section">
        <Typography.Text strong>自动重置</Typography.Text>
        <div className="client-config-grid">
          <Form.Item label="重置周期" extra={!record.traffic_reset_supported ? '面板未上报自动重置能力，可修改其他配置' : undefined}><Select aria-label="重置周期" value={draft.resetChoice} options={clientTrafficResetOptions(draft.billing.start_time)} disabled={locked || !record.traffic_reset_supported} onChange={resetChoice => setDraft(current => ({ ...current, resetChoice }))} /></Form.Item>
          {monthly ? <Form.Item label="每月重置日"><InputNumber aria-label="每月重置日" min={1} max={31} precision={0} addonAfter="号" value={draft.resetChoice === 'monthly_from_start' ? startDay : draft.resetDay} disabled={locked || !record.traffic_reset_supported || draft.resetChoice === 'monthly_from_start'} onChange={resetDay => setDraft(current => ({ ...current, resetDay }))} /></Form.Item> : null}
        </div>
        <Typography.Paragraph type="secondary" className="client-config-help">由 x-ui 按面板时区执行。保存规则不会立即清空已用流量。{monthly ? '当月没有该日号时在月底重置，后续月份仍按原日号执行。' : ''}</Typography.Paragraph>
      </section> : null}
    </Form>
  </Modal>
}
