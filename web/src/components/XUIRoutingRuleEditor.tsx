import { useEffect, useRef, useState } from 'react'
import { Alert, App as AntdApp, Button, Form, Input, Modal, Select, Space, Switch, Tabs, Typography } from 'antd'

import type { XUIAction, XUIOverview, XUIRoutingRuleView } from '../types'
import { buildRoutingRuleUpdatePayload, createRoutingRuleDraft, isPendingRoutingUpdate, parseRoutingRuleJSON, routingRuleChanged, routingRuleList } from '../lib/routingRuleEditor'
import type { RoutingRuleUpdatePayload } from '../lib/routingRuleEditor'

export function XUIRoutingRuleEditor(props: {
  record: XUIRoutingRuleView
  fingerprint: string
  currentFingerprint: string
  overview: XUIOverview
  actions: XUIAction[]
  busy: boolean
  onSave: (payload: RoutingRuleUpdatePayload) => Promise<XUIAction | null>
  onClose: () => void
}) {
  const { message, modal } = AntdApp.useApp()
  const [rule, setRule] = useState(() => createRoutingRuleDraft(props.record))
  const [json, setJSON] = useState(() => JSON.stringify(rule, null, 2))
  const [lineInputs, setLineInputs] = useState<Record<string, string>>({})
  const [mode, setMode] = useState('form')
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const submittingRef = useRef(false)
  const [submitted, setSubmitted] = useState<XUIAction | null>(null)
  const submittedAction = props.actions.find(action => action.id === submitted?.id) || submitted
  const pending = Boolean(submittedAction && isPendingRoutingUpdate(submittedAction))
  const uncertain = submittedAction?.status === 'failed' && (submittedAction.result?.saved === true || submittedAction.result?.submitted === true)
  const stale = props.fingerprint !== props.currentFingerprint
  const locked = submitting || pending || Boolean(uncertain)
  const dirty = mode === 'json' ? json !== JSON.stringify(props.record.raw_rule, null, 2) : routingRuleChanged(props.record.raw_rule!, rule)
  const onCloseRef = useRef(props.onClose)
  onCloseRef.current = props.onClose

  useEffect(() => {
    if (submittedAction?.status === 'succeeded') {
      message.success(`路由 R${props.record.index} 已保存、核验并重载，规则顺序已保留`)
      onCloseRef.current()
    }
  }, [submittedAction?.status, message, props.record.index])

  function patch(key: string, value: unknown) {
    setRule(current => {
      const next = { ...current }
      if (value === '' || value === undefined || (Array.isArray(value) && !value.length)) delete next[key]
      else next[key] = value
      return next
    })
    setError('')
  }

  function changeMode(next: string) {
    if (next === 'json') setJSON(JSON.stringify(rule, null, 2))
    else {
      try { setRule(parseRoutingRuleJSON(json)); setLineInputs({}) }
      catch (error) { setError((error as Error).message); return }
    }
    setError('')
    setMode(next)
  }

  async function save() {
    if (locked || props.busy || stale || submittingRef.current) return
    try {
      const payload = buildRoutingRuleUpdatePayload(props.record, props.fingerprint, mode === 'json' ? parseRoutingRuleJSON(json) : rule)
      submittingRef.current = true
      setSubmitting(true)
      setError('')
      const action = await props.onSave(payload)
      if (action) setSubmitted(action)
    } catch (error) { setError(error instanceof Error ? error.message : '保存路由失败') }
    finally { submittingRef.current = false; setSubmitting(false) }
  }

  function close() {
    if (submitting) return
    if (dirty && !pending && !uncertain) {
      modal.confirm({ title: '放弃未保存的修改？', content: '关闭后，这次填写的路由修改不会保存。', okText: '放弃修改', cancelText: '继续编辑', onOk: props.onClose })
      return
    }
    props.onClose()
  }

  const list = (key: string) => routingRuleList(rule[key])
  const lines = (key: string) => lineInputs[key] ?? list(key).join('\n')
  const fromLines = (value: string) => value.split('\n').map(item => item.trim()).filter(Boolean)
  function patchLines(key: string, value: string) {
    // Keep the typed newline while the stored condition list omits blank rows.
    setLineInputs(current => ({ ...current, [key]: value }))
    patch(key, fromLines(value))
  }
  const text = (key: string) => rule[key] === undefined ? '' : String(rule[key])
  const optionsWithCurrent = (options: { value: string; label: string }[], current: string[]) => [...options, ...current.filter(value => !options.some(option => option.value === value)).map(value => ({ value, label: `${value}（当前配置）` }))]
  const apiTags = new Set((props.overview.editable_routing_rules || []).filter(item => item.protected).map(item => item.outbound_tag))
  const outboundOptions = props.overview.outbounds.filter(item => item.tag && item.tag !== 'api' && !apiTags.has(item.tag)).map(item => ({ value: item.tag!, label: `${item.tag} · ${item.protocol || '-'}` }))
  const balancerOptions = (props.overview.balancers || []).filter(item => item.tag).map(item => ({ value: item.tag!, label: item.tag! }))
  const targetType = rule.balancerTag !== undefined && !rule.outboundTag ? 'balancerTag' : 'outboundTag'
  const sourceKey = 'sourceIP' in rule ? 'sourceIP' : ('source' in rule || 'source' in props.record.raw_rule!) ? 'source' : 'sourceIP'

  const form = <Form layout="vertical" className="routing-editor-grid" disabled={locked}>
    <Form.Item label="规则状态"><Switch checked={rule.enabled !== false} checkedChildren="启用" unCheckedChildren="停用" onChange={value => patch('enabled', value)} aria-label="规则状态" /></Form.Item>
    <Form.Item label="备注"><Input value={text('comment')} onChange={event => patch('comment', event.target.value)} aria-label="规则备注" /></Form.Item>
    <Form.Item label="目标类型"><Select value={targetType} options={[{value:'outboundTag',label:'出站'},{value:'balancerTag',label:'均衡器'}]} onChange={value => { setRule(current => { const next={...current}; delete next.outboundTag; delete next.balancerTag; next[value]=''; return next }); setError('') }} aria-label="路由目标类型" /></Form.Item>
    <Form.Item label="目标" required><Select showSearch optionFilterProp="label" value={text(targetType) || undefined} options={optionsWithCurrent(targetType === 'balancerTag' ? balancerOptions : outboundOptions, text(targetType) ? [text(targetType)] : [])} onChange={value => patch(targetType, value)} placeholder="选择目标出站或均衡器" aria-label="路由目标" /></Form.Item>
    <Form.Item label="匹配入站" className="routing-editor-full"><Select mode="tags" value={list('inboundTag')} options={props.overview.nodes.filter(item => item.tag).map(item => ({value:item.tag!,label:`${item.remark || item.tag} · ${item.tag}`}))} onChange={value => patch('inboundTag', value)} placeholder="选择入站；可输入标签" aria-label="匹配入站" /></Form.Item>
    <Form.Item label="匹配客户端" className="routing-editor-full"><Select mode="tags" value={list('user')} options={[...new Set(props.overview.clients.map(item => item.email).filter((value): value is string => Boolean(value)))].map(value => ({value,label:value}))} onChange={value => patch('user', value)} placeholder="选择客户端邮箱；可输入 regexp: 正则" aria-label="匹配客户端" /></Form.Item>
    <Form.Item label="域名"><Input.TextArea value={lines('domain')} onChange={event => patchLines('domain', event.target.value)} autoSize={{minRows:3,maxRows:6}} placeholder={'每行一项，例如\ndomain:example.com\ngeosite:cn'} aria-label="匹配域名" /></Form.Item>
    <Form.Item label="目标 IP / CIDR"><Input.TextArea value={lines('ip')} onChange={event => patchLines('ip', event.target.value)} autoSize={{minRows:3,maxRows:6}} placeholder={'每行一项，例如\n10.0.0.0/8\ngeoip:private'} aria-label="目标 IP" /></Form.Item>
    <Form.Item label="目标端口"><Input value={text('port')} onChange={event => patch('port', event.target.value)} placeholder="443,1000-2000" aria-label="目标端口" /></Form.Item>
    <Form.Item label="源端口"><Input value={text('sourcePort')} onChange={event => patch('sourcePort', event.target.value)} placeholder="10000-20000" aria-label="源端口" /></Form.Item>
    <Form.Item label="源 IP"><Input.TextArea value={lines(sourceKey)} onChange={event => patchLines(sourceKey, event.target.value)} autoSize={{minRows:2,maxRows:5}} placeholder="每行一个 IP / CIDR" aria-label="源 IP" /></Form.Item>
    <Form.Item label="传输协议"><Select mode="multiple" value={list('network')} options={['tcp','udp'].map(value=>({value,label:value.toUpperCase()}))} onChange={value => patch('network', value.join(','))} aria-label="传输协议" /></Form.Item>
    <Form.Item label="应用协议" className="routing-editor-full"><Select mode="tags" value={list('protocol')} options={['http','tls','quic','bittorrent'].map(value=>({value,label:value}))} onChange={value=>patch('protocol',value)} aria-label="应用协议" /></Form.Item>
  </Form>

  return <Modal open width={800} title={`编辑路由规则 · R${props.record.index}`} className="xui-routing-editor-modal" onCancel={close} maskClosable={!locked} keyboard={!submitting} closable={!submitting} footer={<Space><Button disabled={submitting} onClick={close}>{pending || uncertain ? '关闭' : '取消'}</Button><Button type="primary" loading={submitting || pending} disabled={props.busy || stale || !dirty || Boolean(uncertain)} onClick={() => void save()}>保存并重载</Button></Space>}>
    <Typography.Paragraph type="secondary" className="routing-editor-help">修改保留当前规则顺序，未编辑的字段保持原值。多个匹配条件需同时满足；保存后重载 Xray，连接可能短暂中断。</Typography.Paragraph>
    {stale && !pending && !uncertain && submittedAction?.status !== 'succeeded' ? <Alert type="warning" showIcon message="规则已变化，请关闭弹窗，刷新后重新选择编辑" className="routing-editor-feedback" /> : null}
    {pending ? <Alert type="info" showIcon message={submittedAction?.status === 'pending' ? '任务等待 Client 执行' : '任务执行中，正在等待保存、回读核验和重载结果'} description="关闭窗口不会取消已下发的任务。" className="routing-editor-feedback" /> : null}
    {submittedAction?.status === 'failed' ? <Alert type="error" showIcon message={submittedAction.error || '修改未成功，请查看 x-ui 操作记录'} description={uncertain ? '修改可能已保存，请先刷新核对；若已保存但重载失败，请手动重启 x-ui / Xray。' : undefined} className="routing-editor-feedback" /> : null}
    {error ? <Alert type="error" showIcon message={error} className="routing-editor-feedback" /> : null}
    <Tabs activeKey={mode} onChange={changeMode} items={[{key:'form',label:'常用条件',children:form},{key:'json',label:'高级 JSON',children:<><Typography.Paragraph type="secondary">编辑完整单条规则，可维护表单未列出的条件。不要填写整个 routing 配置。</Typography.Paragraph><Input.TextArea className="routing-editor-json" value={json} disabled={locked} onChange={event=>{setJSON(event.target.value);setError('')}} autoSize={{minRows:12,maxRows:22}} aria-label="路由规则 JSON" spellCheck={false} /></>}]} />
  </Modal>
}
