import { useEffect, useMemo, useRef, useState } from 'react'
import { App as AntdApp, Alert, Button, Checkbox, Empty, Modal, Segmented, Space, Table, Tag, Tooltip, Typography } from 'antd'
import type { TabsProps } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { AppstoreOutlined, BranchesOutlined, DeleteOutlined, EditOutlined, UnorderedListOutlined } from '@ant-design/icons'
import type { XUIOutboundView, XUIRoutingRuleView } from '../types'
import { buildRoutingRulesDeletePayload, isPendingRoutingDeletion, routingRuleDeleteDisabledReason } from '../lib/routingRules'
import type { RoutingRulesDeletePayload } from '../lib/routingRules'
import { isPendingRoutingUpdate, routingRuleEditDisabledReason } from '../lib/routingRuleEditor'
import type { RoutingRuleUpdatePayload } from '../lib/routingRuleEditor'
import { buildOutboundsDeletePayload, isPendingOutboundDeletion, outboundDeleteDisabledReason } from '../lib/outbounds'
import type { OutboundsDeletePayload } from '../lib/outbounds'
import { buildOutboundRuleReferences } from '../lib/outboundRuleReferences'
import { findOutboundLinkedClient, outboundElementId, ruleElementId, summarizeRule } from '../lib/appHelpers'
import { formatBytes } from '../lib/traffic'
import { AdminHelpHint } from './AdminHelpHint'
import { XUIRoutingRuleEditor } from './XUIRoutingRuleEditor'
import type { AgentDetailPanelProps } from './AgentDetailPanel.types'

const { Text } = Typography

type RoutingProps = Pick<AgentDetailPanelProps, 'overview' | 'selectedAgentId' | 'xuiActions' | 'xuiActionsLoading' | 'routingRuleDeleteLoading' | 'routingRuleUpdateLoading' | 'outboundDeleteLoading' | 'canManageConfig' | 'restrictedView' | 'dashboardView' | 'selectedRuleIndex' | 'selectedOutboundTag' | 'onDeleteRoutingRules' | 'onUpdateRoutingRule' | 'onRoutingRuleUpdated' | 'onDeleteOutbounds' | 'onJumpOutbound' | 'onJumpNode' | 'onJumpRule'>

/** Route and outbound editing share pending-action guards and captured fingerprints. */
export function useAgentRoutingTools(props: RoutingProps) {
  const { message } = AntdApp.useApp()
  const { overview, selectedAgentId, xuiActions, xuiActionsLoading, routingRuleDeleteLoading, routingRuleUpdateLoading, outboundDeleteLoading, canManageConfig, restrictedView, dashboardView, selectedRuleIndex, selectedOutboundTag, onDeleteRoutingRules, onUpdateRoutingRule, onRoutingRuleUpdated, onDeleteOutbounds, onJumpOutbound, onJumpNode, onJumpRule } = props
  const [selectedRoutingRuleKeys, setSelectedRoutingRuleKeys] = useState<number[]>([])
  const [routingDeleteDraft, setRoutingDeleteDraft] = useState<RoutingRulesDeletePayload | null>(null)
  const [submittedRoutingDeleteID, setSubmittedRoutingDeleteID] = useState<number | null>(null)
  const [routingEditor, setRoutingEditor] = useState<{ rule: XUIRoutingRuleView; fingerprint: string } | null>(null)
  const [submittedRoutingUpdateID, setSubmittedRoutingUpdateID] = useState<number | null>(null)
  const [selectedOutboundTags, setSelectedOutboundTags] = useState<string[]>([])
  const [outboundView, setOutboundView] = useState<'list' | 'card'>('list')
  const [routingView, setRoutingView] = useState<'list' | 'card'>('list')
  const [referenceOutboundTag, setReferenceOutboundTag] = useState<string | null>(null)
  const [outboundDeleteDraft, setOutboundDeleteDraft] = useState<OutboundsDeletePayload | null>(null)
  const [submittedOutboundDeleteID, setSubmittedOutboundDeleteID] = useState<number | null>(null)
  const refreshRoutingOverviewRef = useRef(onRoutingRuleUpdated)
  refreshRoutingOverviewRef.current = onRoutingRuleUpdated
  const routingFingerprint = overview?.routing_rules_fingerprint || ''
  const routingRules = overview?.editable_routing_rules ?? overview?.routing_rules ?? []
  const outboundRuleReferences = useMemo(() => buildOutboundRuleReferences(overview?.outbounds || [], overview?.editable_routing_rules ?? overview?.routing_rules ?? [], overview?.balancers), [overview])
  const pendingRoutingDelete = submittedRoutingDeleteID !== null
    || xuiActions.some((action) => action.agent_id === selectedAgentId && isPendingRoutingDeletion(action))
  const outboundFingerprint = overview?.outbounds_fingerprint || ''
  const pendingOutboundDelete = submittedOutboundDeleteID !== null
    || xuiActions.some(action => action.agent_id === selectedAgentId && isPendingOutboundDeletion(action))
  const pendingRoutingUpdate = submittedRoutingUpdateID !== null || xuiActions.some(action => action.agent_id === selectedAgentId && isPendingRoutingUpdate(action))
  const routingDeleteBusy = routingRuleDeleteLoading || routingRuleUpdateLoading || outboundDeleteLoading || xuiActionsLoading || pendingRoutingDelete || pendingOutboundDelete || pendingRoutingUpdate
  const outboundDeleteBusy = routingDeleteBusy
  const canDeleteRoutes = canManageConfig && !restrictedView
  const canDeleteOutbounds = canDeleteRoutes
  const deletableOutbounds = overview?.outbounds.filter(outbound => !outboundDeleteDisabledReason(outbound)) || []

  useEffect(() => {
    setSelectedRoutingRuleKeys([])
  }, [selectedAgentId, routingFingerprint])

  useEffect(() => { setSelectedOutboundTags([]) }, [selectedAgentId, outboundFingerprint])
  useEffect(() => { setReferenceOutboundTag(null) }, [selectedAgentId])
  useEffect(() => { setRoutingEditor(null); setSubmittedRoutingUpdateID(null) }, [selectedAgentId])
  useEffect(() => {
    const action = xuiActions.find(item => item.id === submittedRoutingUpdateID)
    if (action && !isPendingRoutingUpdate(action)) {
      setSubmittedRoutingUpdateID(null)
      refreshRoutingOverviewRef.current?.()
    }
  }, [submittedRoutingUpdateID, xuiActions])
  useEffect(() => { setOutboundDeleteDraft(null); setSubmittedOutboundDeleteID(null) }, [selectedAgentId])
  useEffect(() => {
    const action = xuiActions.find(item => item.id === submittedOutboundDeleteID)
    if (action && !isPendingOutboundDeletion(action)) setSubmittedOutboundDeleteID(null)
  }, [submittedOutboundDeleteID, xuiActions])

  useEffect(() => {
    const action = xuiActions.find((item) => item.id === submittedRoutingDeleteID)
    if (action && !isPendingRoutingDeletion(action)) setSubmittedRoutingDeleteID(null)
  }, [submittedRoutingDeleteID, xuiActions])

  async function submitRoutingUpdate(payload: RoutingRuleUpdatePayload) {
    if (!canDeleteRoutes || routingDeleteBusy || payload.rules_fingerprint !== routingFingerprint) return null
    const action = await onUpdateRoutingRule(payload)
    if (action && isPendingRoutingUpdate(action)) setSubmittedRoutingUpdateID(action.id)
    return action
  }

  function confirmOutboundDeletion(outbounds: XUIOutboundView[]) {
    if (!canDeleteOutbounds || outboundDeleteBusy || !outbounds.length) return
    try { setOutboundDeleteDraft(buildOutboundsDeletePayload(outbounds, outboundFingerprint)) }
    catch (error) { message.error(error instanceof Error ? error.message : '请选择可删除的出站') }
  }

  async function submitOutboundDeletion() {
    if (!outboundDeleteDraft || !canDeleteOutbounds || outboundDeleteBusy || outboundDeleteDraft.config_fingerprint !== outboundFingerprint) return
    const action = await onDeleteOutbounds(outboundDeleteDraft)
    if (action) {
      if (isPendingOutboundDeletion(action)) setSubmittedOutboundDeleteID(action.id)
      setOutboundDeleteDraft(null)
      setSelectedOutboundTags([])
    }
  }

  function confirmRoutingRuleDeletion(rules: XUIRoutingRuleView[]) {
    if (!canDeleteRoutes || routingDeleteBusy || !rules.length) return
    // Capture exact targets when the dialog opens. A background refresh must
    // never silently change the rules the user is about to confirm.
    try {
      setRoutingDeleteDraft(buildRoutingRulesDeletePayload(rules, routingFingerprint))
    } catch (error) {
      message.error(error instanceof Error ? error.message : '请选择有效的路由规则')
    }
  }

  async function submitRoutingRuleDeletion() {
    if (!routingDeleteDraft || !canDeleteRoutes || routingDeleteBusy || routingDeleteDraft.rules_fingerprint !== routingFingerprint) return
    const action = await onDeleteRoutingRules(routingDeleteDraft)
    if (action) {
      if (isPendingRoutingDeletion(action)) setSubmittedRoutingDeleteID(action.id)
      setRoutingDeleteDraft(null)
      setSelectedRoutingRuleKeys([])
    }
  }

  function renderRoutingValidity(record: XUIRoutingRuleView) {
    const state = record.enabled === false ? 'disabled' : record.validity || (!record.inbound_tags?.length && !record.users?.length || record.protected ? 'valid' : 'unknown')
    const labels = { valid: '有效', partial: '部分有效', invalid: '无效', disabled: '停用', unknown: '待判断' }
    const colors = { valid: 'success', partial: 'warning', invalid: 'error', disabled: 'default', unknown: 'default' }
    const reason = record.validity_reasons?.join('；') || (state === 'unknown' ? '请更新 Server 并刷新 Client 信息' : state === 'valid' ? '当前配置可匹配' : '')
    const visibleReason = (record.validity_reasons?.length || 0) > 2 ? `${record.validity_reasons!.slice(0, 2).join('；')}；另 ${record.validity_reasons!.length - 2} 项` : reason
    return <div className="routing-validity-cell">
      <Tag color={colors[state]}>{labels[state]}</Tag>
      {reason ? <Tooltip title={reason}><span className="routing-validity-reason">{visibleReason}</span></Tooltip> : null}
    </div>
  }

  function renderRoutingMatchers(record: XUIRoutingRuleView) {
    return <Space wrap size={[6, 6]}>
      {record.inbound_tags?.map(tag => <Tag key={`inbound-${tag}`} color="blue">{tag}</Tag>)}
      {record.users?.map(user => <Tag key={`user-${user}`} color="gold">{user}</Tag>)}
      {!record.inbound_tags?.length && !record.users?.length ? <Tag>全局条件</Tag> : null}
    </Space>
  }

  function renderRoutingTarget(record: XUIRoutingRuleView) {
    return <Space wrap size={[6, 6]}>
      {record.outbound_tag ? <Button type="link" className="route-link" onClick={() => onJumpOutbound(record.outbound_tag)}>{record.outbound_tag}</Button> : null}
      {record.balancer_tag ? <Tag color="magenta">balancer:{record.balancer_tag}</Tag> : null}
    </Space>
  }

  function renderRoutingActions(record: XUIRoutingRuleView) {
    return <Space size={8} className="routing-rule-actions">
      <Tooltip title={routingRuleEditDisabledReason(record) || (!routingFingerprint ? '请刷新 Client 信息以读取面板保存的规则' : '编辑当前规则，保留原有顺序')}>
        <Button size="small" icon={<EditOutlined />} disabled={routingDeleteBusy || !routingFingerprint || Boolean(routingRuleEditDisabledReason(record))} onClick={() => setRoutingEditor({ rule: record, fingerprint: routingFingerprint })}>编辑</Button>
      </Tooltip>
      <Tooltip title={routingRuleDeleteDisabledReason(record) || (!routingFingerprint ? '请升级 Bridge Client，并立即获取 Client 信息以读取面板保存的规则' : undefined)}>
        <Button size="small" danger icon={<DeleteOutlined />} disabled={routingDeleteBusy || !routingFingerprint || Boolean(routingRuleDeleteDisabledReason(record))} onClick={() => confirmRoutingRuleDeletion([record])}>删除</Button>
      </Tooltip>
    </Space>
  }

  function renderOutboundReferences(outbound: XUIOutboundView) {
    const references = outboundRuleReferences.get(outbound.tag || '') || []
    return references.length ? <div className="outbound-rule-association">
      <Button size="small" icon={<BranchesOutlined />} aria-label={`查看出站 ${outbound.tag} 的 ${references.length} 条关联路由`} onClick={() => setReferenceOutboundTag(outbound.tag!)}>{references.length} 条规则</Button>
      <Text type="secondary">{references.slice(0, 3).map(item => `R${item.rule.index}`).join('、')}{references.length > 3 ? ` 等 ${references.length} 条` : ''}</Text>
    </div> : <Tooltip title={outbound.is_default ? '默认出口仍可处理未命中路由规则的流量' : '当前上报的规则没有直接或通过均衡器指向此出站'}><Tag className="outbound-unreferenced">无显式关联</Tag></Tooltip>
  }

  function renderOutboundMetrics(outbound: XUIOutboundView) {
    return <div className="outbound-traffic-cell">
      <Text>累计 <strong>{formatBytes(outbound.total || 0)}</strong></Text>
      <div className="outbound-metrics"><span title="上传">↑ {formatBytes(outbound.up || 0)}</span><span title="下载">↓ {formatBytes(outbound.down || 0)}</span></div>
    </div>
  }

  function renderOutboundTarget(outbound: XUIOutboundView) {
    const linked = findOutboundLinkedClient(dashboardView, selectedAgentId, outbound.tag)
    const label = linked ? linked.target.inbound_name || linked.target.inbound_tag || String(linked.target.inbound_id) : ''
    return <div className="outbound-target-cell">
      <div className="outbound-target">{outbound.target || '当前配置未提供远端地址'}</div>
      {linked ? <Button type="link" className="outbound-jump" onClick={() => onJumpNode(linked.target.agent_id, label)}>跳转到 {linked.target.agent_name || linked.target.agent_id} / {label}</Button> : null}
      {outbound.send_through ? <Tag>sendThrough:{outbound.send_through}</Tag> : null}
    </div>
  }

  function renderOutboundDelete(outbound: XUIOutboundView) {
    const reason = outboundDeleteDisabledReason(outbound)
    return <Tooltip title={reason || undefined}><Button size="small" danger icon={<DeleteOutlined />} disabled={outboundDeleteBusy || !outboundFingerprint || Boolean(reason)} onClick={() => confirmOutboundDeletion([outbound])}>删除</Button></Tooltip>
  }

  const outboundColumns: ColumnsType<XUIOutboundView> = [
    { title: '出站', key: 'outbound', width: 260, render: (_, outbound) => <div id={outboundElementId(outbound.tag || 'unknown')} className="outbound-identity">
      <Text className="outbound-tag">{outbound.tag || '-'}</Text>
      <Space wrap size={[6, 6]}><Tag>{outbound.protocol || '-'}</Tag>{outbound.is_default ? <Tag color="success">默认出口</Tag> : null}</Space>
    </div> },
    { title: '目标 / 关联节点', key: 'target', width: 290, render: (_, outbound) => renderOutboundTarget(outbound) },
    { title: '关联路由', key: 'rules', width: 180, render: (_, outbound) => renderOutboundReferences(outbound) },
    { title: '流量', key: 'traffic', width: 220, render: (_, outbound) => renderOutboundMetrics(outbound) },
  ]
  if (canDeleteOutbounds) outboundColumns.push({ title: '操作', key: 'actions', width: 100, fixed: 'right', render: (_, outbound) => renderOutboundDelete(outbound) })

  const routingColumns: ColumnsType<XUIRoutingRuleView> = [
    {
      title: '#',
      dataIndex: 'index',
      width: 70,
      render: (value: number) => (
        <span id={ruleElementId(value)} className={selectedRuleIndex === value ? 'route-anchor active' : 'route-anchor'}>
          R{value}
        </span>
      ),
    },
    {
      title: '有效状态',
      key: 'validity',
      width: 230,
      render: (_, record) => renderRoutingValidity(record),
    },
    {
      title: '入口匹配',
      key: 'matchers',
      width: 280,
      render: (_, record) => renderRoutingMatchers(record),
    },
    {
      title: '出站 / 均衡器',
      key: 'route',
      width: 220,
      render: (_, record) => renderRoutingTarget(record),
    },
    {
      title: '条件摘要',
      dataIndex: 'summary',
      render: (value: string | undefined, record) => value || summarizeRule(record),
    },
  ]
  if (canDeleteRoutes) {
    routingColumns.push({
      title: '操作',
      key: 'actions',
      width: 180,
      fixed: 'right',
      render: (_, record) => renderRoutingActions(record),
    })
  }

  const routingTabs: NonNullable<TabsProps['items']> = [
    {
      key: 'outbounds',
      label: `出站 (${overview?.outbounds.length || 0})`,
      children: overview ? (
        <>
          <div className="xui-collection-toolbar">
          {canDeleteOutbounds ? <Space wrap className="outbound-batch-toolbar">
            <Button danger icon={<DeleteOutlined />} loading={outboundDeleteLoading} disabled={outboundDeleteBusy || !outboundFingerprint || !selectedOutboundTags.length}
              onClick={() => confirmOutboundDeletion(overview.outbounds.filter(outbound => selectedOutboundTags.includes(outbound.tag || '')))}>
              批量删除{selectedOutboundTags.length ? ` (${selectedOutboundTags.length})` : ''}
            </Button>
            <Button disabled={outboundDeleteBusy || !outboundFingerprint || !deletableOutbounds.length} onClick={() => setSelectedOutboundTags(deletableOutbounds.map(outbound => outbound.tag!))}>全选可删除</Button>
            {selectedOutboundTags.length ? <Button type="link" disabled={outboundDeleteBusy} onClick={() => setSelectedOutboundTags([])}>取消选择</Button> : null}
            <AdminHelpHint title="批量移除所选出站，一次保存完整配置并回读核验，然后重载 Xray。默认出口、被路由/均衡器/代理链引用的出站不可删除；请先调整引用。运行时生成的出站需在来源面板维护。需要目标 VPSMonitor Client v0.3.49 或更高版本。" />
            {outboundDeleteBusy ? <Text type="secondary">删除任务下发中或执行中，请查看 x-ui 操作记录</Text> : null}
            {!outboundFingerprint ? <Text type="secondary">请升级目标 VPSMonitor Client 并立即获取 Client 信息</Text> : null}
          </Space> : <span />}
          <Segmented aria-label="出站查看方式" value={outboundView} options={[{ value: 'list', label: '列表', icon: <UnorderedListOutlined aria-hidden /> }, { value: 'card', label: '卡片', icon: <AppstoreOutlined aria-hidden /> }]} onChange={value => setOutboundView(value as 'list' | 'card')} />
          </div>
          {overview.editable_routing_rules === undefined ? <div className="routing-validity-help"><Text type="secondary">关联按当前运行时规则展示，停用规则可能未上报；刷新采集后可查看面板保存的完整规则。</Text></div> : null}
          {outboundView === 'list' ? <Table<XUIOutboundView>
            className="xui-outbound-table"
            rowKey={outbound => outbound.tag || 'unknown'} columns={outboundColumns} dataSource={overview.outbounds} pagination={false} tableLayout="fixed" scroll={{ x: 1100 }}
            locale={{ emptyText: <Empty description="暂无出站数据" /> }}
            rowClassName={outbound => outbound.tag === selectedOutboundTag ? 'outbound-row-selected' : ''}
            rowSelection={canDeleteOutbounds ? {
              selectedRowKeys: selectedOutboundTags,
              onChange: keys => setSelectedOutboundTags(keys.map(String)),
              getCheckboxProps: outbound => ({ disabled: outboundDeleteBusy || !outboundFingerprint || Boolean(outboundDeleteDisabledReason(outbound)), title: outboundDeleteDisabledReason(outbound), 'aria-label': `选择出站 ${outbound.tag || '-'}` }),
            } : undefined}
          /> : overview.outbounds.length ? <div className="outbound-grid">
          {overview.outbounds.map((outbound) => {
            const selected = outbound.tag && outbound.tag === selectedOutboundTag
            const checked = selectedOutboundTags.includes(outbound.tag || '')
            const blockedReason = outboundDeleteDisabledReason(outbound)
            const linkedClient = findOutboundLinkedClient(dashboardView, selectedAgentId, outbound.tag)
            return (
              <section
                key={outbound.tag || 'unknown'}
                id={outboundElementId(outbound.tag || 'unknown')}
                className={`outbound-card${selected ? ' selected' : ''}${checked ? ' checked' : ''}${linkedClient ? ' linked' : ''}`}
              >
                {canDeleteOutbounds ? <div className="outbound-select-row">
                  <Tooltip title={blockedReason || undefined}>
                    <Checkbox aria-label={`选择出站 ${outbound.tag || '-'}`} checked={checked} disabled={outboundDeleteBusy || !outboundFingerprint || Boolean(blockedReason)}
                      onChange={event => setSelectedOutboundTags(current => event.target.checked ? [...current, outbound.tag!] : current.filter(tag => tag !== outbound.tag))}>选择</Checkbox>
                  </Tooltip>
                  {renderOutboundDelete(outbound)}
                </div> : null}
                <div className="outbound-head">
                  <div>
                    <Text className="outbound-tag">{outbound.tag || '-'}</Text>
                    <div className="muted-line">{outbound.protocol || '-'}{outbound.is_default ? ' · 默认出口' : ''}</div>
                  </div>
                  <Space wrap size={[6, 6]} className="outbound-head-meta">
                    {outbound.is_default ? <Tag color="success">默认</Tag> : null}
                    {linkedClient ? <Tag color="cyan">关联 {linkedClient.target.agent_name || linkedClient.target.agent_id}</Tag> : null}
                  </Space>
                </div>
                {renderOutboundTarget(outbound)}
                {renderOutboundMetrics(outbound)}
                <div className="outbound-card-rules"><Text type="secondary">关联路由</Text>{renderOutboundReferences(outbound)}</div>
                {canDeleteOutbounds && blockedReason ? <Text type="secondary" className="outbound-delete-reason">{blockedReason}</Text> : null}
              </section>
            )
          })}
          </div> : <Empty description="暂无出站数据" />}
        </>
      ) : (
        <Empty description="暂无出站数据" />
      ),
    },
    {
      key: 'routes',
      label: `路由规则 (${routingRules.length})`,
      children: overview ? (
        <>
          <div className="routing-validity-help">
            <Text type="secondary">按当前入站、客户端及出站引用判断，全局条件与系统 API 路由视为有效；不依据在线状态或流量。</Text>
          </div>
          <div className="xui-collection-toolbar">
          {canDeleteRoutes ? (
            <Space wrap>
              <Button
                danger
                icon={<DeleteOutlined />}
                loading={routingRuleDeleteLoading}
                disabled={routingDeleteBusy || !selectedRoutingRuleKeys.length || !routingFingerprint}
                onClick={() => confirmRoutingRuleDeletion(routingRules.filter((rule) => selectedRoutingRuleKeys.includes(rule.index)))}
              >批量删除{selectedRoutingRuleKeys.length ? ` (${selectedRoutingRuleKeys.length})` : ''}</Button>
              <Button disabled={routingDeleteBusy || !routingFingerprint || !routingRules.some(rule => !routingRuleDeleteDisabledReason(rule))} onClick={() => setSelectedRoutingRuleKeys(routingRules.filter(rule => !routingRuleDeleteDisabledReason(rule)).map(rule => rule.index))}>全选可删除</Button>
              {selectedRoutingRuleKeys.length ? <Button type="link" disabled={routingDeleteBusy} onClick={() => setSelectedRoutingRuleKeys([])}>取消选择</Button> : null}
              <AdminHelpHint title="勾选后可批量删除。按倒序逐条移除，统一保存并重载 Xray；不删除客户端或出站。系统 API 路由不可删除，规则变化时会取消本次操作。执行结果可在 x-ui 操作记录查看。需升级 Bridge Client 后使用。" />
              {routingDeleteBusy ? <Text type="secondary">路由或出站任务下发中或执行中，请查看 x-ui 操作记录</Text> : null}
            </Space>
          ) : <span />}
          <Segmented aria-label="路由规则查看方式" value={routingView} options={[{ value: 'list', label: '列表', icon: <UnorderedListOutlined aria-hidden /> }, { value: 'card', label: '卡片', icon: <AppstoreOutlined aria-hidden /> }]} onChange={value => setRoutingView(value as 'list' | 'card')} />
          </div>
          {routingView === 'list' ? <Table
            rowKey={(record) => record.index}
            columns={routingColumns}
            dataSource={routingRules}
            pagination={false}
            locale={{ emptyText: <Empty description="暂无路由规则数据" /> }}
            scroll={{ x: 1280 }}
            rowClassName={(record) => [selectedRuleIndex === record.index ? 'route-row-selected' : '', record.validity === 'invalid' ? 'route-row-invalid' : ''].filter(Boolean).join(' ')}
            rowSelection={canDeleteRoutes ? {
              selectedRowKeys: selectedRoutingRuleKeys,
              onChange: (keys) => setSelectedRoutingRuleKeys(keys.map(Number)),
              getCheckboxProps: (record) => ({
                disabled: routingDeleteBusy || !routingFingerprint || Boolean(routingRuleDeleteDisabledReason(record)),
                title: routingRuleDeleteDisabledReason(record),
                'aria-label': `选择路由 R${record.index}`,
              }),
            } : undefined}
          /> : routingRules.length ? <div className="routing-rule-grid">
            {routingRules.map(rule => <section key={rule.index} className={`routing-rule-card${selectedRuleIndex === rule.index ? ' selected' : ''}${selectedRoutingRuleKeys.includes(rule.index) ? ' checked' : ''}`}>
              <div className="routing-rule-card-head">
                <Space size={10}>
                  {canDeleteRoutes ? <Tooltip title={routingRuleDeleteDisabledReason(rule) || undefined}><Checkbox aria-label={`选择路由 R${rule.index}`} checked={selectedRoutingRuleKeys.includes(rule.index)} disabled={routingDeleteBusy || !routingFingerprint || Boolean(routingRuleDeleteDisabledReason(rule))} onChange={event => setSelectedRoutingRuleKeys(current => event.target.checked ? [...current, rule.index] : current.filter(index => index !== rule.index))} /></Tooltip> : null}
                  <span id={ruleElementId(rule.index)} className={selectedRuleIndex === rule.index ? 'route-anchor active' : 'route-anchor'}>R{rule.index}</span>
                </Space>
                {canDeleteRoutes ? renderRoutingActions(rule) : null}
              </div>
              {renderRoutingValidity(rule)}
              <div className="routing-rule-card-field"><Text type="secondary">入口匹配</Text>{renderRoutingMatchers(rule)}</div>
              <div className="routing-rule-card-field"><Text type="secondary">出站 / 均衡器</Text>{renderRoutingTarget(rule)}</div>
              <div className="routing-rule-card-summary">{rule.summary || summarizeRule(rule)}</div>
            </section>)}
          </div> : <Empty description="暂无路由规则数据" />}
        </>
      ) : (
        <Empty description="暂无路由规则数据" />
      ),
    },
  ]

  const routingModals = <>
      {routingEditor && overview ? <XUIRoutingRuleEditor key={`${selectedAgentId}:${routingEditor.rule.index}:${routingEditor.fingerprint}`} record={routingEditor.rule} fingerprint={routingEditor.fingerprint} currentFingerprint={routingFingerprint} overview={overview} actions={xuiActions} busy={routingDeleteBusy} onSave={submitRoutingUpdate} onClose={() => setRoutingEditor(null)} /> : null}
      <Modal className="xui-reference-modal" title={`关联路由 · ${referenceOutboundTag || ''}`} open={referenceOutboundTag !== null} width={800} onCancel={() => setReferenceOutboundTag(null)} footer={<Button onClick={() => setReferenceOutboundTag(null)}>关闭</Button>}>
        <Typography.Paragraph type="secondary">按配置引用展示，包含停用或无效规则；均衡器关联表示候选出站，不代表当前流量命中。</Typography.Paragraph>
        <div className="outbound-reference-list">
          {(outboundRuleReferences.get(referenceOutboundTag || '') || []).map(({ rule, direct, balancers }) => <section key={rule.index} className="outbound-reference-item">
            <div className="outbound-reference-head"><Space wrap><Tag color="blue">R{rule.index}</Tag>{direct ? <Tag>直接关联</Tag> : null}{balancers.map(tag => <Tag color="magenta" key={tag}>经均衡器 {tag}</Tag>)}</Space>
              <Button size="small" disabled={rule.index <= 0} onClick={() => { setReferenceOutboundTag(null); onJumpRule(rule.index) }}>查看规则</Button>
            </div>
            {renderRoutingValidity(rule)}
            <div className="routing-rule-card-field"><Text type="secondary">入口匹配</Text>{renderRoutingMatchers(rule)}</div>
            <div className="routing-rule-card-summary">{rule.summary || summarizeRule(rule)}</div>
          </section>)}
          {!outboundRuleReferences.get(referenceOutboundTag || '')?.length ? <Empty description="当前没有关联路由规则" /> : null}
        </div>
      </Modal>
      <Modal
        className="xui-delete-modal"
        title={`删除 ${outboundDeleteDraft?.outbounds.length || 0} 个出站？`}
        open={outboundDeleteDraft !== null}
        okText="确认删除"
        cancelText="取消"
        okButtonProps={{ danger: true, disabled: outboundDeleteBusy || outboundDeleteDraft?.config_fingerprint !== outboundFingerprint }}
        confirmLoading={outboundDeleteLoading}
        closable={!outboundDeleteLoading}
        maskClosable={!outboundDeleteLoading}
        onCancel={() => { if (!outboundDeleteLoading) setOutboundDeleteDraft(null) }}
        onOk={() => void submitOutboundDeletion()}
      >
        <Space wrap>{outboundDeleteDraft?.outbounds.map(outbound => <Tag key={outbound.tag}>{outbound.tag}</Tag>)}</Space>
        <p>将一次保存所选出站的删除并回读核验，随后重载 Xray，可能短暂影响连接。不会删除客户端或路由规则。此操作不能直接撤销。</p>
        {outboundDeleteDraft?.config_fingerprint !== outboundFingerprint ? <Alert type="warning" showIcon message="Xray 配置已变化，请取消并重新选择。" /> : null}
      </Modal>
      <Modal
        className="xui-delete-modal"
        title={`删除 ${routingDeleteDraft?.rules.length || 0} 条路由规则？`}
        open={routingDeleteDraft !== null}
        okText="确认删除"
        cancelText="取消"
        okButtonProps={{ danger: true, disabled: routingDeleteBusy || routingDeleteDraft?.rules_fingerprint !== routingFingerprint }}
        confirmLoading={routingRuleDeleteLoading}
        closable={!routingRuleDeleteLoading}
        maskClosable={!routingRuleDeleteLoading}
        onCancel={() => { if (!routingRuleDeleteLoading) setRoutingDeleteDraft(null) }}
        onOk={() => void submitRoutingRuleDeletion()}
      >
        <p>选中的规则：{routingDeleteDraft?.rules.map((rule) => `R${rule.index}`).join('、')}</p>
        <p>将逐条移除并统一保存，随后重载 Xray，可能短暂影响连接。只删除路由规则，不删除客户端或出站。此操作不能直接撤销。</p>
        {routingDeleteDraft?.rules_fingerprint !== routingFingerprint ? <Alert type="warning" showIcon message="路由规则已变化，请取消并重新选择。" /> : null}
      </Modal>
  </>

  return { routingTabs, routingModals }
}
