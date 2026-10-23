import { useEffect, useState, type Dispatch, type ReactNode, type SetStateAction } from 'react'
import { App, Button, Col, Divider, Row, Select, TreeSelect, Typography } from 'antd'
import type { DashboardAgentView, XUIOverview } from '../types'
import { fetchJSON } from '../lib/appHelpers'
import {
  areaAssignmentDraftFromTargetOption,
  areaAssignmentKey,
  assignmentNodeKeys,
  buildAreaAssignmentTreeData,
  buildAssignmentTargetOptions,
  isRealmAssignmentDraft,
  normalizeAreaManagerOutboundGrants,
  uniqueStrings,
  type AreaManagerAssignmentDraft,
  type AreaManagerFormState,
} from './CustomerManagementHelpers'

export function AreaPermissionSection({ title, children }: { title: string; children: ReactNode }) {
  return <Col xs={24}>
    <section className="area-permission-section" aria-label={title}>
      <Divider orientation="left"><Typography.Text strong>{title}</Typography.Text></Divider>
      <Row gutter={[12, 16]}>{children}</Row>
    </section>
  </Col>
}

export function AreaPermissionField({ label, action, children }: { label: string; action?: ReactNode; children: ReactNode }) {
  return <Col xs={24} md={12}>
    <div className="area-permission-field-label">
      <Typography.Text type="secondary">{label}</Typography.Text>
      {action}
    </div>
    {children}
  </Col>
}

export function AreaMachineGrantFields({ kind, options, form, setForm, agents }: {
  kind: 'line' | 'landing'
  options: Array<{ value: string; label: string }>
  form: AreaManagerFormState
  setForm: Dispatch<SetStateAction<AreaManagerFormState>>
  agents: DashboardAgentView[]
}) {
  const { message } = App.useApp()
  const [agentID, setAgentID] = useState(() => (kind === 'line' ? options.find(option => form.outbound_grants.some(item => item.agent_id === option.value))?.value : '') || options.find(option => form.assignments.some(item => item.agent_id === option.value && !isRealmAssignmentDraft(item, agents)))?.value || '')
  const [overview, setOverview] = useState<XUIOverview | null>(null)
  const [loading, setLoading] = useState(false)
  const label = kind === 'line' ? '线路机 Client' : '落地机 Client'

  useEffect(() => {
    if (agentID && !options.some(option => option.value === agentID)) setAgentID('')
  }, [agentID, options])

  useEffect(() => {
    setOverview(null)
    if (!agentID) { setLoading(false); return }
    let cancelled = false
    setLoading(true)
    void fetchJSON<XUIOverview>(`/api/v1/agents/${encodeURIComponent(agentID)}/xui/overview?assignment_scope=1`)
      .then(data => { if (!cancelled) setOverview(data) })
      .catch(error => { if (!cancelled) message.error(error instanceof Error ? error.message : '加载 x-ui 节点失败') })
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [agentID, message])

  const currentOverview = overview?.agent_id === agentID ? overview : null
  const treeData = buildAreaAssignmentTreeData(agentID, currentOverview, agents)
  const selectedKeys = form.assignments.filter(item => item.agent_id === agentID && !isRealmAssignmentDraft(item, agents)).map(areaAssignmentKey)
  function update(keys: string[]) {
    if (!agentID || !currentOverview) return
    const drafts = new Map(buildAssignmentTargetOptions(currentOverview).map(option => {
      const draft = areaAssignmentDraftFromTargetOption(agentID, option, agents)
      return [areaAssignmentKey(draft), draft]
    }))
    const selected = keys.map(key => drafts.get(key)).filter((item): item is AreaManagerAssignmentDraft => Boolean(item))
    setForm(current => {
      const assignments = [...current.assignments.filter(item => item.agent_id !== agentID || isRealmAssignmentDraft(item, agents)), ...selected]
      const ids = new Set(assignments.map(item => item.agent_id))
      return {
        ...current, assignments, agent_ids: uniqueStrings([...ids]),
        outbound_grants: current.outbound_grants.filter(grant => ids.has(grant.agent_id)),
      }
    })
  }
  const outboundOptions = (currentOverview?.outbounds || []).flatMap(outbound => outbound.tag ? [{
    value: outbound.tag,
    label: [outbound.tag, outbound.protocol].filter(Boolean).join(' / '),
  }] : [])
  const selectedOutboundTags = form.outbound_grants.filter(grant => grant.agent_id === agentID).map(grant => grant.outbound_tag)
  const canGrantOutbounds = Boolean(agentID && currentOverview && !loading && selectedKeys.length)
  function updateOutbounds(tags: string[]) {
    if (!canGrantOutbounds) return
    const allowed = new Set(outboundOptions.map(option => option.value))
    setForm(current => ({
      ...current,
      outbound_grants: normalizeAreaManagerOutboundGrants([
        ...current.outbound_grants.filter(grant => grant.agent_id !== agentID),
        ...tags.filter(tag => allowed.has(tag)).map(tag => ({ agent_id: agentID, outbound_tag: tag })),
      ]),
    }))
  }
  return <>
    <AreaPermissionField label={label}>
      <Select aria-label={label} style={{ width: '100%' }} showSearch allowClear optionFilterProp="label"
        placeholder={'选择已开启的' + label} options={options} value={agentID || undefined}
        onChange={value => { setOverview(null); setAgentID(value || '') }} />
    </AreaPermissionField>
    <AreaPermissionField label="x-ui 节点 / 客户端授权" action={
      <Button size="small" disabled={!agentID || loading || !treeData.length} onClick={() => update(assignmentNodeKeys(agentID, currentOverview, agents))}>全选</Button>
    }>
      <TreeSelect aria-label={label + '节点授权'} multiple treeCheckable style={{ width: '100%' }} showSearch
        placeholder="按 Client / 节点 / 客户端授权" value={selectedKeys} loading={loading}
        disabled={!agentID || loading || !currentOverview} treeData={treeData} treeDefaultExpandAll
        showCheckedStrategy={TreeSelect.SHOW_PARENT} maxTagCount="responsive" onChange={keys => update(keys as string[])} />
    </AreaPermissionField>
    {kind === 'line' ? <AreaPermissionField label="线路机已有出站规则" action={
      <Button size="small" disabled={!canGrantOutbounds || !outboundOptions.length} onClick={() => updateOutbounds(outboundOptions.map(option => option.value))}>全选</Button>
    }>
      <Select aria-label="线路机已有出站规则" mode="multiple" style={{ width: '100%' }} showSearch
        placeholder={agentID && !selectedKeys.length ? '请先授权该线路机的节点 / 客户端' : '选择允许区域账号使用的出站规则'}
        value={selectedOutboundTags} options={outboundOptions} optionFilterProp="label" loading={loading}
        disabled={!canGrantOutbounds} maxTagCount="responsive" onChange={updateOutbounds} />
    </AreaPermissionField> : null}
  </>
}
