import { useCustomerAccounts } from '../hooks/useCustomerAccounts'
import { useAccountDirectory } from '../hooks/useAccountDirectory';
import { AreaAccountPanel } from './AreaAccountPanel';
import { useEffect, useMemo, useRef, useState } from 'react';
import { Alert, App as AntdApp, Button, Card, Empty, Input, Modal, Select, Space, Spin, Table, Tabs, Typography } from 'antd';
import { PlusOutlined, ReloadOutlined, SyncOutlined } from '@ant-design/icons';

import type { AdminUser, AreaManagerAdminView, AreaManagerAssignment, CustomerAdminView, CustomerAssignment, CustomerAssignmentDraft, CustomerAssignmentSourceView, CustomerBillingDiagnostic, CustomerBillingReconciliationResponse, DashboardAgentView, XUIClientView, XUINodeView, XUIOverview } from '../types';
import { fetchJSON } from '../lib/appHelpers';

import { buildCustomerManagementTables } from './CustomerManagementTables';
import { CustomerAssignmentManagerCard } from './CustomerAssignmentManagerCard';
import { AdminHelpHint } from './AdminHelpHint';
import { DEFAULT_ACCOUNT_PASSWORD, type AreaBatchAssignmentFormState, type AreaManagerAssignmentDraft, type AreaManagerFormState, type AssignmentFormState, type ManagementTabKey, areaAssignmentDraftFromTargetOption, areaAssignmentKey, assignmentFormFromAssignment, assignmentFormFromDraft, assignmentMatchesInbound, billingFormPatch, buildAreaAssignmentTreeData, buildAssignmentTargetOptions, buildClientAssignmentTreeData, buildRealmGrantOptions, clientBilling, clientKey, clientLabel, defaultPublicClientName, defaultPublicNodeName, dedupeAreaManagerAssignmentDrafts, emptyAreaBatchAssignmentForm, emptyAreaManagerForm, emptyAssignmentForm, emptyCustomerForm, findMatchingAssignment, findRealmTargetNode, firstRealmAssignmentAgentID, isRealmAssignmentDraft, nodeKey, nodeLabel, normalizeAreaManagerAssignmentDrafts, normalizeAreaManagerOutboundGrants, normalizeRevenueCycle, realmRuleTargetAgentID, revenueCycleLabel, uniqueStrings } from './CustomerManagementHelpers';
import { CustomerManagementModals } from './CustomerManagementModals';
import { AgentRolePanel } from './AgentRolePanel';
import { OperatorManagementPanel } from './OperatorManagementPanel';

const { Text, Title } = Typography

export function CustomerManagementModal(props: {
  open?: boolean
  agents: DashboardAgentView[]
  onClose?: () => void
  onConfigChanged?: (agentID?: string) => void | Promise<void>
  onOpenAssignment?: (assignment: CustomerAssignment) => void
  initialAssignment?: CustomerAssignmentDraft | null
  onInitialAssignmentApplied?: () => void
  adminUser?: AdminUser | null
  embedded?: boolean
}) {
  const {
    open = false,
    agents,
    onClose = () => undefined,
    onConfigChanged,
    onOpenAssignment,
    initialAssignment = null,
    onInitialAssignmentApplied,
    adminUser = null,
    embedded = false,
  } = props
  const active = embedded || open
  const canManageAreaManagers = adminUser?.role === 'admin'
  const canViewFinance = canManageAreaManagers
  const { message } = AntdApp.useApp()
  const { customers, setCustomers, areaManagers, setAreaManagers, operators, frontProxies, loading, areaManagersLoading, operatorsLoading, resettingOperatorID, savingOperator, selectedCustomerID, setSelectedCustomerID, loadCustomers, loadAreaManagers, copyAreaManagerRecommendationURL, loadOperators, loadFrontProxies, saveOperator, deleteOperator, resetOperatorPassword } = useAccountDirectory(canManageAreaManagers, message)
  const [activeManagementTab, setActiveManagementTab] = useState<ManagementTabKey>(canManageAreaManagers ? 'area' : 'customers')
  const [customerAssignmentSources, setCustomerAssignmentSources] = useState<CustomerAssignmentSourceView[]>([])
  const [savingAssignment, setSavingAssignment] = useState(false)
  const [savingAreaManager, setSavingAreaManager] = useState(false)
  const [resettingAreaManagerID, setResettingAreaManagerID] = useState<number | null>(null)
  const [savingAreaBatchAssignment, setSavingAreaBatchAssignment] = useState(false)
  const [areaBatchModalOpen, setAreaBatchModalOpen] = useState(false)
  const [areaManagerModalOpen, setAreaManagerModalOpen] = useState(false)
  const [assignmentManagerModalOpen, setAssignmentManagerModalOpen] = useState(false)
  const [assignmentViewModalOpen, setAssignmentViewModalOpen] = useState(false)
  const [billingReconciliationModalOpen, setBillingReconciliationModalOpen] = useState(false)
  const [billingReconciliation, setBillingReconciliation] = useState<CustomerBillingReconciliationResponse | null>(null)
  const [billingReconciliationLoading, setBillingReconciliationLoading] = useState(false)
  const [repairingAssignmentID, setRepairingAssignmentID] = useState<number | null>(null)
  const [editingAreaManagerID, setEditingAreaManagerID] = useState<number | null>(null)
  const [editingAssignmentID, setEditingAssignmentID] = useState<number | null>(null)
  const [customerSearch, setCustomerSearch] = useState('')
  const [customerStatusFilter, setCustomerStatusFilter] = useState<'all' | 'enabled' | 'disabled'>('all')
  const [areaManagerForm, setAreaManagerForm] = useState<AreaManagerFormState>(emptyAreaManagerForm)
  const [assignmentForm, setAssignmentForm] = useState<AssignmentFormState>(emptyAssignmentForm)
  const [overview, setOverview] = useState<XUIOverview | null>(null)
  const [overviewLoading, setOverviewLoading] = useState(false)
  const [areaBatchForm, setAreaBatchForm] = useState<AreaBatchAssignmentFormState>(emptyAreaBatchAssignmentForm)
  const [areaBatchOverview, setAreaBatchOverview] = useState<XUIOverview | null>(null)
  const [areaBatchOverviewLoading, setAreaBatchOverviewLoading] = useState(false)
  const skipAssignmentResetRef = useRef(false)

  const {
    savingCustomer,
    changingCustomerStatusIDs,
    resettingCustomerID,
    copyingSubscriptionCustomerID,
    customerCreateModalOpen,
    setCustomerCreateModalOpen,
    customerEditModalOpen,
    setCustomerEditModalOpen,
    customerSubscriptionModalOpen,
    subscriptionCustomer,
    selectedSubscriptionAssignmentIDs,
    setSelectedSubscriptionAssignmentIDs,
    customerForm,
    setCustomerForm,
    customerCreateForm,
    setCustomerCreateForm,
    openCustomerCreateModal,
    openCustomerEditModal,
    openCustomerSubscriptionModal,
    closeCustomerSubscriptionModal,
    createCustomer,
    toggleCustomerStatus,
    saveCustomer,
    deleteCustomer,
    resetCustomerPassword,
    copyCustomerSubscription
  } = useCustomerAccounts({
    canManageAreaManagers, message, selectedCustomerID, setSelectedCustomerID,
    setCustomers, setAreaManagers, loadCustomers, loadAreaManagers,
    onCustomerDeleted: wasSelected => {
      if (wasSelected) { setAssignmentForm(emptyAssignmentForm); setEditingAssignmentID(null) }
      setAssignmentManagerModalOpen(false)
      setAssignmentViewModalOpen(false)
    },
  })

  const selectedCustomer = customers.find((item) => item.id === selectedCustomerID) || null
  const inheritedAssignmentBilling = useMemo(() => clientBilling(
    assignmentForm.agent_id,
    assignmentForm.inbound_id,
    assignmentForm.inbound_tag,
    assignmentForm.client_email,
    agents,
    assignmentForm.client_id,
  ), [agents, assignmentForm.agent_id, assignmentForm.client_email, assignmentForm.client_id, assignmentForm.inbound_id, assignmentForm.inbound_tag])
  const inheritedPriceLabel = inheritedAssignmentBilling && Number(inheritedAssignmentBilling.revenue_amount || 0) > 0
    ? `${inheritedAssignmentBilling.revenue_currency || 'CNY'} ${Number(inheritedAssignmentBilling.revenue_amount || 0).toFixed(2)} / ${revenueCycleLabel(inheritedAssignmentBilling.revenue_cycle)}`
    : '未设置'
  const agentOptions = useMemo(() => agents.map((agent) => ({
    value: agent.agent_id,
    label: agent.agent_name || agent.agent_id,
  })), [agents])
  const realmAgentOptions = useMemo(() => agentOptions.filter(option => buildRealmGrantOptions(option.value, agents).length > 0), [agentOptions, agents])
  const lineAgentOptions = useMemo(() => agentOptions.filter(option => agents.find(agent => agent.agent_id === option.value)?.tags?.includes('线路机')), [agentOptions, agents])
  const landingAgentOptions = useMemo(() => agentOptions.filter(option => agents.find(agent => agent.agent_id === option.value)?.tags?.includes('落地机')), [agentOptions, agents])
  const customerAssignmentAgentOptions = useMemo(() => canManageAreaManagers
    ? agentOptions
    : customerAssignmentSources.map((source) => ({
      value: source.agent_id,
      label: source.agent_name || source.agent_id,
    })), [agentOptions, canManageAreaManagers, customerAssignmentSources])
  const clientOptions = useMemo(() => {
    const clients = (overview?.clients || []).map((client) => ({
      value: clientKey(client),
      label: clientLabel(client),
      client,
      node: undefined as XUINodeView | undefined,
    }))
    const nodes = (overview?.nodes || []).map((node) => ({
      value: nodeKey(node),
      label: nodeLabel(node),
      client: undefined as XUIClientView | undefined,
      node,
    }))
    return [...clients, ...nodes]
  }, [overview])
  const clientTreeData = useMemo(() => buildClientAssignmentTreeData(assignmentForm.agent_id, overview, agents), [assignmentForm.agent_id, overview, agents])
  const areaBatchXUIAgentID = areaBatchForm.xui_agent_id
  const areaBatchClientTreeData = useMemo(() => buildAreaAssignmentTreeData(areaBatchXUIAgentID, areaBatchOverview, agents), [areaBatchXUIAgentID, areaBatchOverview, agents])
  const areaBatchRealmOptions = useMemo(() => buildRealmGrantOptions(areaBatchForm.agent_id, agents), [areaBatchForm.agent_id, agents])
  const areaManagerRealmGrantOptions = useMemo(() => buildRealmGrantOptions(areaManagerForm.grant_agent_id, agents), [areaManagerForm.grant_agent_id, agents])
  const frontProxyOptions = useMemo(() => frontProxies
    .filter((item) => item.enabled)
    .map((item) => ({
      value: item.id,
      label: [item.name, item.protocol].filter(Boolean).join(' / '),
    })), [frontProxies])
  const selectedCustomerFrontProxyOptions = useMemo(() => (selectedCustomer?.front_proxies || [])
    .filter((item) => item.enabled)
    .map((item) => ({
      value: item.id,
      label: [item.name, item.protocol].filter(Boolean).join(' / '),
    })), [selectedCustomer])
  const filteredCustomers = useMemo(() => {
    const normalizedSearch = customerSearch.trim().toLocaleLowerCase()
    return customers.filter((customer) => {
      if (customerStatusFilter !== 'all' && (customer.enabled ? 'enabled' : 'disabled') !== customerStatusFilter) {
        return false
      }
      if (!normalizedSearch) {
        return true
      }
      const searchableText = [
        customer.username,
        customer.display_name,
        ...((customer.assignments || []).flatMap((assignment) => [
          assignment.public_client_name,
          assignment.inbound_tag,
          assignment.client_id,
          assignment.client_email,
          assignment.remark,
        ])),
      ].filter(Boolean).join(' ').toLocaleLowerCase()
      return searchableText.includes(normalizedSearch)
    })
  }, [customerSearch, customerStatusFilter, customers])

  useEffect(() => {
    if (active) {
      void loadCustomers()
      void loadFrontProxies()
      if (canManageAreaManagers) {
        void loadAreaManagers()
        void loadOperators()
      } else {
        void loadCustomerAssignmentSources()
      }
    }
  }, [active, canManageAreaManagers])

  useEffect(() => {
    if (!canManageAreaManagers && activeManagementTab === 'area') {
      setActiveManagementTab('customers')
    }
  }, [activeManagementTab, canManageAreaManagers])

  useEffect(() => {
    if (active && initialAssignment) {
      setActiveManagementTab('customers')
      setAssignmentManagerModalOpen(true)
    }
  }, [active, initialAssignment])

  useEffect(() => {
    if (!selectedCustomer) {
      setCustomerForm(emptyCustomerForm)
      setEditingAssignmentID(null)
      if (skipAssignmentResetRef.current) {
        skipAssignmentResetRef.current = false
        return
      }
      return
    }
    setCustomerForm({
      username: selectedCustomer.username,
      password: '',
      display_name: selectedCustomer.display_name || selectedCustomer.username,
      enabled: selectedCustomer.enabled,
      front_proxy_node_ids: (selectedCustomer.front_proxies || []).map((item) => item.id),
    })
    if (skipAssignmentResetRef.current) {
      skipAssignmentResetRef.current = false
      return
    }
    setEditingAssignmentID(null)
    setAssignmentForm(emptyAssignmentForm)
  }, [selectedCustomerID])

  useEffect(() => {
    if (!active || !initialAssignment) {
      return
    }
    const match = findMatchingAssignment(customers, initialAssignment)
    const nextCustomerID = match?.customer.id || selectedCustomerID || customers[0]?.id || null
    const nextForm = match?.assignment
      ? assignmentFormFromAssignment(match.assignment, agents)
      : assignmentFormFromDraft(initialAssignment, agents)
    if (nextCustomerID !== selectedCustomerID) {
      skipAssignmentResetRef.current = true
      setSelectedCustomerID(nextCustomerID)
    }
    setEditingAssignmentID(match?.assignment.id || null)
    setAssignmentForm(nextForm)
    onInitialAssignmentApplied?.()
  }, [active, agents, customers, initialAssignment, onInitialAssignmentApplied, selectedCustomerID])

  useEffect(() => {
    if (!assignmentForm.agent_id) {
      setOverview(null)
      return
    }
    let cancelled = false
    setOverviewLoading(true)
    void fetchJSON<XUIOverview>(`/api/v1/agents/${assignmentForm.agent_id}/xui/overview?assignment_scope=1`)
      .then((data) => {
        if (!cancelled) {
          setOverview(data)
        }
      })
      .catch((error) => {
        if (!cancelled) {
          setOverview(null)
          message.warning(error instanceof Error ? error.message : '加载 x-ui 客户端失败')
        }
      })
      .finally(() => {
        if (!cancelled) {
          setOverviewLoading(false)
        }
      })
    return () => {
      cancelled = true
    }
  }, [assignmentForm.agent_id])

  useEffect(() => {
    const xuiAgentID = areaBatchForm.xui_agent_id
    if (!xuiAgentID) {
      setAreaBatchOverview(null)
      return
    }
    let cancelled = false
    setAreaBatchOverviewLoading(true)
    void fetchJSON<XUIOverview>(`/api/v1/agents/${xuiAgentID}/xui/overview?assignment_scope=1`)
      .then((data) => {
        if (!cancelled) {
          setAreaBatchOverview(data)
        }
      })
      .catch((error) => {
        if (!cancelled) {
          setAreaBatchOverview(null)
          message.warning(error instanceof Error ? error.message : '加载 x-ui 客户端失败')
        }
      })
      .finally(() => {
        if (!cancelled) {
          setAreaBatchOverviewLoading(false)
        }
      })
    return () => {
      cancelled = true
    }
  }, [areaBatchForm.xui_agent_id])

  const { visibleAssignmentColumns, billingReconciliationColumns, areaManagerColumns, customerColumns, renderAreaManagerCustomers } = buildCustomerManagementTables({
    agents,
    canViewFinance,
    onOpenAssignment,
    editAssignment,
    deleteAssignment,
    repairingAssignmentID,
    rebindBillingAssignment,
    resettingCustomerID,
    openAssignmentManagerModal,
    openCustomerSubscriptionModal,
    resetCustomerPassword,
    editAreaManager,
    copyAreaManagerRecommendationURL,
    resetAreaManagerPassword,
    resettingAreaManagerID,
    deleteAreaManager,
    openAssignmentViewModal,
    openCustomerEditModal,
    deleteCustomer,
    changingCustomerStatusIDs,
    toggleCustomerStatus,
    deleteAreaManagerAssignment,
  })

  async function saveAreaManager() {
    if (!areaManagerForm.username.trim()) {
      message.warning('请填写区域账号登录名')
      return
    }
    const password = areaManagerForm.password || (!editingAreaManagerID ? DEFAULT_ACCOUNT_PASSWORD : '')
    if (!editingAreaManagerID && password.length < 8) {
      message.warning('新区域账号密码至少 8 位')
      return
    }
    const assignments = normalizeAreaManagerAssignmentDrafts(areaManagerForm.assignments, agents).filter(item => {
      if (isRealmAssignmentDraft(item, agents)) return areaManagerForm.realm_enabled
      const tags = agents.find(agent => agent.agent_id === item.agent_id)?.tags || []
      return areaManagerForm.line_enabled && tags.includes('线路机') || areaManagerForm.landing_enabled && tags.includes('落地机')
    })
    const authorizedAgentIDs = new Set(assignments.map((assignment) => assignment.agent_id))
    const outboundGrants = normalizeAreaManagerOutboundGrants(areaManagerForm.outbound_grants)
      .filter((grant) => areaManagerForm.line_enabled && authorizedAgentIDs.has(grant.agent_id))
    if (!assignments.length && !outboundGrants.length && !areaManagerForm.front_proxy_node_ids.length) {
      message.warning('请至少授权一个节点 / 客户端或已有出站')
      return
    }
    setSavingAreaManager(true)
    try {
      const agentIDs = uniqueStrings(assignments.map((assignment) => assignment.agent_id))
      const payload = {
        username: areaManagerForm.username.trim(),
        password,
        display_name: areaManagerForm.display_name.trim(),
        recommendation_token: areaManagerForm.recommendation_token.trim(),
        enabled: areaManagerForm.enabled,
        agent_ids: agentIDs,
        billing_enabled: areaManagerForm.billing_enabled,
        revenue_amount: areaManagerForm.revenue_amount,
        revenue_currency: areaManagerForm.revenue_currency,
        revenue_cycle: areaManagerForm.revenue_cycle,
        outbound_create_enabled: areaManagerForm.landing_enabled,
        outbound_grants: outboundGrants,
        front_proxy_node_ids: areaManagerForm.front_link_enabled ? areaManagerForm.front_proxy_node_ids : [],
        realm_enabled: areaManagerForm.realm_enabled, line_enabled: areaManagerForm.line_enabled,
        landing_enabled: areaManagerForm.landing_enabled, front_link_enabled: areaManagerForm.front_link_enabled,
        line_agent_ids: areaManagerForm.line_enabled ? uniqueStrings(assignments.filter(item => !isRealmAssignmentDraft(item, agents) && agents.find(agent => agent.agent_id === item.agent_id)?.tags?.includes('线路机')).map(item => item.agent_id)) : [],
        landing_agent_ids: areaManagerForm.landing_enabled ? uniqueStrings(assignments.filter(item => !isRealmAssignmentDraft(item, agents) && agents.find(agent => agent.agent_id === item.agent_id)?.tags?.includes('落地机')).map(item => item.agent_id)) : [],
      }
      if (editingAreaManagerID) {
        const updatedManager = await fetchJSON<AreaManagerAdminView>(`/api/v1/admin/area-managers/${editingAreaManagerID}`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        })
        await replaceAreaManagerAssignments(editingAreaManagerID, updatedManager.assignments || [], assignments)
        message.success('区域账号已更新')
      } else {
        const created = await fetchJSON<AreaManagerAdminView>('/api/v1/admin/area-managers', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        })
        await replaceAreaManagerAssignments(created.id, [], assignments)
        message.success('区域账号已创建')
      }
      setEditingAreaManagerID(null)
      setAreaManagerForm(emptyAreaManagerForm)
      setAreaManagerModalOpen(false)
      await loadAreaManagers()
      await onConfigChanged?.()
    } catch (error) {
      message.error(error instanceof Error ? error.message : '保存区域账号失败')
    } finally {
      setSavingAreaManager(false)
    }
  }

  async function replaceAreaManagerAssignments(managerID: number, previous: AreaManagerAssignment[], next: AreaManagerAssignmentDraft[]) {
    const nextKeys = new Set(next.map(areaAssignmentKey))
    const removed = previous.filter((assignment) => !nextKeys.has(areaAssignmentKey(assignment)))
    for (const assignment of removed) {
      await fetchJSON(`/api/v1/admin/area-managers/${managerID}/assignments/${assignment.id}`, { method: 'DELETE' })
    }
    const previousKeys = new Set(previous.map(item => `${areaAssignmentKey(item)}:${Boolean(item.node_only)}`))
    const additions = next.filter(item => !previousKeys.has(`${areaAssignmentKey(item)}:${Boolean(item.node_only)}`))
    if (additions.length) {
      await fetchJSON<AreaManagerAssignment[]>(`/api/v1/admin/area-managers/${managerID}/assignments`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ assignments: additions }),
      })
    }
  }

  async function deleteAreaManager(id: number) {
    setSavingAreaManager(true)
    try {
      await fetchJSON(`/api/v1/admin/area-managers/${id}`, { method: 'DELETE' })
      if (editingAreaManagerID === id) {
        setEditingAreaManagerID(null)
        setAreaManagerForm(emptyAreaManagerForm)
        setAreaManagerModalOpen(false)
      }
      message.success('区域账号已删除')
      await loadAreaManagers()
    } catch (error) {
      message.error(error instanceof Error ? error.message : '删除区域账号失败')
    } finally {
      setSavingAreaManager(false)
    }
  }

  async function resetAreaManagerPassword(id: number) {
    setResettingAreaManagerID(id)
    try {
      await fetchJSON<AreaManagerAdminView>(`/api/v1/admin/area-managers/${id}/reset-password`, { method: 'POST' })
      message.success(`区域账号密码已初始化为 ${DEFAULT_ACCOUNT_PASSWORD}`)
      await loadAreaManagers()
    } catch (error) {
      message.error(error instanceof Error ? error.message : '初始化区域账号密码失败')
    } finally {
      setResettingAreaManagerID(null)
    }
  }

  async function batchAssignAreaManagerTargets() {
    if (savingAreaBatchAssignment) {
      return
    }
    if (!areaBatchForm.manager_id) {
      message.warning('请选择区域账号')
      return
    }
    const xuiAgentID = areaBatchForm.xui_agent_id
    if (areaBatchForm.selected_realm_keys.length && !areaBatchForm.agent_id) {
      message.warning('请选择转发入口 Client')
      return
    }
    if (areaBatchForm.selected_xui_keys.length && !xuiAgentID) {
      message.warning('请选择 x-ui 出口 Client')
      return
    }
    if (!areaBatchForm.selected_realm_keys.length && !areaBatchForm.selected_xui_keys.length) {
      message.warning('请选择要批量授权的转发入口或 x-ui 客户端 / 节点')
      return
    }
    const xuiOptionMap = new Map(buildAssignmentTargetOptions(areaBatchOverview).map((option) => {
      const draft = areaAssignmentDraftFromTargetOption(xuiAgentID, option, agents)
      return [areaAssignmentKey(draft), draft]
    }))
    const realmOptionMap = new Map(areaBatchRealmOptions.map((option) => [option.value, option]))
    const realmAssignments = areaBatchForm.selected_realm_keys.flatMap((key) => {
      const option = realmOptionMap.get(key)
      return option ? [option.assignment] : []
    })
    const xuiAssignments = areaBatchForm.selected_xui_keys.flatMap((key) => {
      const assignment = xuiOptionMap.get(key)
      return assignment ? [assignment] : []
    })
    setSavingAreaBatchAssignment(true)
    try {
      const inferredRealmTargetAssignments = await inferRealmTargetXUIAssignments(areaBatchForm.selected_realm_keys, areaBatchRealmOptions, xuiAssignments)
      const assignments = dedupeAreaManagerAssignmentDrafts([
        ...realmAssignments,
        ...inferredRealmTargetAssignments.filter(item => {
          const manager = areaManagers.find(manager => manager.id === areaBatchForm.manager_id)
          const tags = agents.find(agent => agent.agent_id === item.agent_id)?.tags || []
          return manager && (manager.line_enabled !== false && tags.includes('线路机') || manager.landing_enabled !== false && tags.includes('落地机'))
        }),
        ...xuiAssignments,
      ])
      if (!assignments.length) {
        message.warning('没有可授权的选择项')
        return
      }
      await fetchJSON<AreaManagerAssignment[]>(`/api/v1/admin/area-managers/${areaBatchForm.manager_id}/assignments`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ assignments }),
      })
      message.success(`已批量授权 ${assignments.length} 条范围`)
      setAreaBatchModalOpen(false)
      setAreaBatchForm(emptyAreaBatchAssignmentForm)
      await loadAreaManagers()
    } catch (error) {
      message.error(error instanceof Error ? error.message : '批量授权失败')
    } finally {
      setSavingAreaBatchAssignment(false)
    }
  }

  async function deleteAreaManagerAssignment(managerID: number, assignmentID: number) {
    try {
      await fetchJSON(`/api/v1/admin/area-managers/${managerID}/assignments/${assignmentID}`, { method: 'DELETE' })
      message.success('区域授权已移除')
      await loadAreaManagers()
    } catch (error) {
      message.error(error instanceof Error ? error.message : '移除区域授权失败')
    }
  }

  function editAreaManager(record: AreaManagerAdminView) {
    setEditingAreaManagerID(record.id)
    setAreaManagerForm({
      username: record.username,
      password: '',
      display_name: record.display_name || record.username,
      recommendation_token: record.recommendation_token || '',
      enabled: record.enabled,
      agent_ids: record.agent_ids || [],
      billing_enabled: Boolean(record.billing_enabled),
      revenue_amount: Number(record.revenue_amount || 0),
      revenue_currency: record.revenue_currency === 'USDT' ? 'USDT' : 'CNY',
      revenue_cycle: normalizeRevenueCycle(record.revenue_cycle),
      grant_agent_id: firstRealmAssignmentAgentID(record.assignments || [], agents) || '',
      realm_enabled: record.realm_enabled !== false, line_enabled: record.line_enabled !== false,
      landing_enabled: record.landing_enabled !== false, front_link_enabled: record.front_link_enabled !== false,
      line_agent_ids: record.line_agent_ids || [], landing_agent_ids: record.landing_agent_ids || [],
      outbound_grants: normalizeAreaManagerOutboundGrants(record.outbound_grants || []),
      front_proxy_node_ids: (record.front_proxies || []).map((item) => item.id),
      assignments: normalizeAreaManagerAssignmentDrafts(record.assignments || [], agents),
    })
    setAreaManagerModalOpen(true)
  }

  function openAreaManagerCreateModal() {
    setEditingAreaManagerID(null)
    setAreaManagerForm({
      ...emptyAreaManagerForm,
      password: DEFAULT_ACCOUNT_PASSWORD,
    })
    setAreaManagerModalOpen(true)
  }

  function openAreaBatchModal() {
    setAreaBatchForm(emptyAreaBatchAssignmentForm)
    setAreaBatchModalOpen(true)
  }

  function closeAreaBatchModal() {
    if (savingAreaBatchAssignment) {
      return
    }
    setAreaBatchModalOpen(false)
    setAreaBatchForm(emptyAreaBatchAssignmentForm)
  }

  function closeAreaManagerModal() {
    setAreaManagerModalOpen(false)
    setEditingAreaManagerID(null)
    setAreaManagerForm(emptyAreaManagerForm)
  }

  function openAssignmentManagerModal(record: CustomerAdminView) {
    setSelectedCustomerID(record.id)
    setEditingAssignmentID(null)
    setAssignmentForm(emptyAssignmentForm)
    setAssignmentManagerModalOpen(true)
  }

  function openAssignmentViewModal(record: CustomerAdminView) {
    setSelectedCustomerID(record.id)
    setAssignmentViewModalOpen(true)
  }

  async function loadCustomerAssignmentSources() {
    try {
      const sources = await fetchJSON<CustomerAssignmentSourceView[]>('/api/v1/admin/customers/assignment-sources')
      setCustomerAssignmentSources(sources)
    } catch (error) {
      setCustomerAssignmentSources([])
      message.error(error instanceof Error ? error.message : '加载可分配入口失败')
    }
  }

  async function openBillingReconciliation() {
    setBillingReconciliationModalOpen(true)
    setBillingReconciliationLoading(true)
    try {
      const result = await fetchJSON<CustomerBillingReconciliationResponse>('/api/v1/admin/customers/billing-reconciliation')
      setBillingReconciliation(result)
    } catch (error) {
      setBillingReconciliation(null)
      message.error(error instanceof Error ? error.message : '加载费用对账失败')
    } finally {
      setBillingReconciliationLoading(false)
    }
  }

  async function rebindBillingAssignment(record: CustomerBillingDiagnostic) {
    if (!record.current_client_id) {
      return
    }
    setRepairingAssignmentID(record.assignment_id)
    try {
      await fetchJSON<CustomerAssignment>(`/api/v1/admin/customers/${record.customer_id}/assignments/${record.assignment_id}/rebind`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ client_id: record.current_client_id }),
      })
      message.success('客户端已重新绑定')
      await Promise.all([loadCustomers(), openBillingReconciliation()])
    } catch (error) {
      message.error(error instanceof Error ? error.message : '重新绑定失败')
    } finally {
      setRepairingAssignmentID(null)
    }
  }

  async function saveAssignment(): Promise<boolean> {
    if (!selectedCustomerID) {
      message.warning('请先选择用户')
      return false
    }
    if (!assignmentForm.agent_id || !assignmentForm.inbound_id) {
      message.warning('请选择 Client 和节点')
      return false
    }
    setSavingAssignment(true)
    try {
      const customerID = selectedCustomerID
      const assignmentID = editingAssignmentID
      const payload = {
        agent_id: assignmentForm.agent_id,
        inbound_id: assignmentForm.inbound_id,
        inbound_tag: assignmentForm.inbound_tag,
        client_id: assignmentForm.client_id,
        client_email: assignmentForm.client_email,
        public_client_name: assignmentForm.public_client_name,
        ...(canViewFinance && assignmentForm.price_mode === 'override' ? {
          price_mode: 'override' as const,
          revenue_amount: assignmentForm.revenue_amount,
          revenue_currency: assignmentForm.revenue_currency,
          revenue_cycle: assignmentForm.revenue_cycle,
        } : canViewFinance ? {
          price_mode: 'inherit' as const,
        } : {}),
        enabled: assignmentForm.enabled,
        front_proxy_node_ids: assignmentForm.front_proxy_node_ids,
      }
      let savedAssignment: CustomerAssignment
      if (assignmentID) {
        savedAssignment = await fetchJSON<CustomerAssignment>(`/api/v1/admin/customers/${customerID}/assignments/${assignmentID}`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        })
        message.success('分配已更新')
      } else {
        savedAssignment = await fetchJSON<CustomerAssignment>(`/api/v1/admin/customers/${customerID}/assignments`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        })
        message.success('分配已新增')
      }
      const nextForm = assignmentFormFromAssignment(savedAssignment, agents)
      setSelectedCustomerID(customerID)
      setEditingAssignmentID(savedAssignment.id)
      setAssignmentForm(nextForm)
      await onConfigChanged?.(customerID ? payload.agent_id : undefined)
      await loadCustomers()
      if (canManageAreaManagers) {
        await loadAreaManagers()
      }
      return true
    } catch (error) {
      message.error(error instanceof Error ? error.message : '保存分配失败')
      return false
    } finally {
      setSavingAssignment(false)
    }
  }

  async function deleteAssignment(id: number) {
    if (!selectedCustomerID) {
      return
    }
    setSavingAssignment(true)
    try {
      await fetchJSON(`/api/v1/admin/customers/${selectedCustomerID}/assignments/${id}`, { method: 'DELETE' })
      message.success('分配已删除')
      await loadCustomers()
      if (canManageAreaManagers) {
        await loadAreaManagers()
      }
    } catch (error) {
      message.error(error instanceof Error ? error.message : '删除分配失败')
    } finally {
      setSavingAssignment(false)
    }
  }

  function editAssignment(record: CustomerAssignment) {
    const nextCustomerID = record.customer_id || selectedCustomerID
    if (nextCustomerID !== selectedCustomerID) {
      skipAssignmentResetRef.current = true
      setSelectedCustomerID(nextCustomerID)
    }
    setEditingAssignmentID(record.id)
    setAssignmentForm(assignmentFormFromAssignment(record, agents))
  }

  function selectClient(key: string) {
    const option = clientOptions.find((item) => item.value === key)
    if (!option) {
      setAssignmentForm((current) => ({ ...current, client_key: key }))
      return
    }
    if (option.client) {
      const client = option.client
      setAssignmentForm((current) => ({
        ...current,
        client_key: key,
        inbound_id: client.inbound_id,
        inbound_tag: client.inbound_tag || '',
        client_id: client.client_id || '',
        client_email: client.email || '',
        public_client_name: current.public_client_name || defaultPublicClientName(client, assignmentForm.agent_id, agents),
        ...(canViewFinance ? billingFormPatch(clientBilling(assignmentForm.agent_id, client.inbound_id, client.inbound_tag || '', client.email || '', agents, client.client_id || '')) : {}),
      }))
      return
    }
    if (option.node) {
      const node = option.node
      setAssignmentForm((current) => ({
        ...current,
        client_key: key,
        inbound_id: node.id,
        inbound_tag: node.tag || '',
        client_id: '',
        client_email: '',
        public_client_name: current.public_client_name || defaultPublicNodeName(node, assignmentForm.agent_id, agents),
        ...(canViewFinance ? billingFormPatch(clientBilling(assignmentForm.agent_id, node.id, node.tag || '', '', agents)) : {}),
      }))
    }
  }

  async function updateAreaManagerRealmGrantTargets(keys: string[]) {
    const agentID = areaManagerForm.grant_agent_id
    if (!agentID) {
      return
    }
    const optionMap = new Map(areaManagerRealmGrantOptions.map((option) => [option.value, option.assignment]))
    const nextForAgent = keys
      .map((key) => optionMap.get(key))
      .filter((item): item is AreaManagerAssignmentDraft => Boolean(item))
    const existingXUIAssignments = areaManagerForm.assignments.filter((assignment) => !isRealmAssignmentDraft(assignment, agents))
    const inferredTargetAssignments = (await inferRealmTargetXUIAssignments(keys, areaManagerRealmGrantOptions, existingXUIAssignments)).filter(item => {
      const tags = agents.find(agent => agent.agent_id === item.agent_id)?.tags || []
      return areaManagerForm.line_enabled && tags.includes('线路机') || areaManagerForm.landing_enabled && tags.includes('落地机')
    })
    setAreaManagerForm((current) => {
      const otherAssignments = current.assignments.filter((assignment) => assignment.agent_id !== agentID || !isRealmAssignmentDraft(assignment, agents))
      const assignments = dedupeAreaManagerAssignmentDrafts([...otherAssignments, ...nextForAgent, ...inferredTargetAssignments])
      return {
        ...current,
        assignments,
        agent_ids: uniqueStrings(assignments.map((assignment) => assignment.agent_id)),
        outbound_grants: current.outbound_grants.filter((grant) => assignments.some((assignment) => assignment.agent_id === grant.agent_id)),
      }
    })
  }

  async function inferRealmTargetXUIAssignments(
    selectedRealmKeys: string[],
    options: ReturnType<typeof buildRealmGrantOptions>,
    excludedTargetAssignments: AreaManagerAssignmentDraft[] = [],
  ): Promise<AreaManagerAssignmentDraft[]> {
    const selected = new Set(selectedRealmKeys)
    const overviewCache = new Map<string, XUIOverview | null>()
    const inferred: AreaManagerAssignmentDraft[] = []
    for (const option of options) {
      if (!selected.has(option.value)) {
        continue
      }
      const targetAgentID = realmRuleTargetAgentID(option.rule, agents)
      if (!agents.find(agent => agent.agent_id === targetAgentID)?.tags?.some(tag => tag === '线路机' || tag === '落地机')) continue
      if (!targetAgentID) {
        continue
      }
      let targetOverview = overviewCache.get(targetAgentID)
      if (!overviewCache.has(targetAgentID)) {
        try {
          targetOverview = await fetchJSON<XUIOverview>(`/api/v1/agents/${targetAgentID}/xui/overview?assignment_scope=1`)
        } catch {
          targetOverview = null
        }
        overviewCache.set(targetAgentID, targetOverview || null)
      }
      const node = findRealmTargetNode(targetOverview, option.rule)
      if (!node) {
        continue
      }
      const resolvedTargetAgentID = node.realm_target_agent_id || targetAgentID
      const resolvedNode = node.realm_target_agent_id
        ? {
            ...node,
            id: node.realm_target_inbound_id || node.id,
            tag: node.realm_target_inbound_tag || node.tag,
            port: node.realm_target_inbound_id || node.port,
          }
        : node
      if (excludedTargetAssignments.some((item) => assignmentMatchesInbound(item, resolvedTargetAgentID, resolvedNode.id, resolvedNode.tag))) {
        continue
      }
      inferred.push(areaAssignmentDraftFromTargetOption(resolvedTargetAgentID, { node: resolvedNode }, agents))
    }
    return dedupeAreaManagerAssignmentDrafts(inferred)
  }

  function removeAreaManagerGrant(key: string) {
    setAreaManagerForm((current) => {
      const assignments = current.assignments.filter((assignment) => areaAssignmentKey(assignment) !== key)
      return {
        ...current,
        assignments,
        agent_ids: uniqueStrings(assignments.map((assignment) => assignment.agent_id)),
        outbound_grants: current.outbound_grants.filter((grant) => assignments.some((assignment) => assignment.agent_id === grant.agent_id)),
      }
    })
  }

  function removeAreaManagerOutboundGrant(agentID: string, outboundTag: string) {
    setAreaManagerForm((current) => {
      const outboundGrants = current.outbound_grants.filter((grant) => grant.agent_id !== agentID || grant.outbound_tag !== outboundTag)
      return {
        ...current,
        outbound_grants: outboundGrants,
        agent_ids: uniqueStrings(current.assignments.map((assignment) => assignment.agent_id)),
      }
    })
  }

  const areaManagersPanel = <AreaAccountPanel
    canManageAreaManagers={canManageAreaManagers}
    loadAreaManagers={loadAreaManagers}
    openAreaBatchModal={openAreaBatchModal}
    openAreaManagerCreateModal={openAreaManagerCreateModal}
    areaBatchModalOpen={areaBatchModalOpen}
    closeAreaBatchModal={closeAreaBatchModal}
    savingAreaBatchAssignment={savingAreaBatchAssignment}
    batchAssignAreaManagerTargets={batchAssignAreaManagerTargets}
    areaBatchForm={areaBatchForm}
    areaManagers={areaManagers}
    setAreaBatchForm={setAreaBatchForm}
    realmAgentOptions={realmAgentOptions}
    agentOptions={agentOptions}
    lineAgentOptions={lineAgentOptions}
    landingAgentOptions={landingAgentOptions}
    areaBatchRealmOptions={areaBatchRealmOptions}
    areaBatchXUIAgentID={areaBatchXUIAgentID}
    areaBatchClientTreeData={areaBatchClientTreeData}
    areaBatchOverview={areaBatchOverview}
    agents={agents}
    areaBatchOverviewLoading={areaBatchOverviewLoading}
    areaManagerColumns={areaManagerColumns}
    renderAreaManagerCustomers={renderAreaManagerCustomers}
  />

  const customersPanel = (
    <Card className="customer-admin-card" bordered={false}>
      <div className="customer-admin-card-head">
        <div className="admin-help-title-group">
          <Title level={5}>用户列表</Title>
          <AdminHelpHint title="普通账号以列表方式管理；编辑用户、查看链路和管理链路均通过弹窗完成。" />
        </div>
        <Space>
          <Button icon={<ReloadOutlined />} onClick={() => void loadCustomers()}>刷新用户</Button>
          {canViewFinance ? <Button icon={<SyncOutlined />} onClick={() => void openBillingReconciliation()}>费用对账</Button> : null}
          <Button type="primary" icon={<PlusOutlined />} onClick={openCustomerCreateModal}>新增普通账号</Button>
        </Space>
      </div>
      <Space wrap style={{ width: '100%', marginBottom: 14 }}>
        <Input.Search
          allowClear
          value={customerSearch}
          style={{ minWidth: 280, flex: 1 }}
          placeholder="搜索用户名 / 显示名 / 链路名称 / 客户端"
          onChange={(event) => setCustomerSearch(event.target.value)}
          onSearch={setCustomerSearch}
        />
        <Select
          value={customerStatusFilter}
          style={{ width: 120 }}
          options={[
            { value: 'all', label: '全部状态' },
            { value: 'enabled', label: '仅启用' },
            { value: 'disabled', label: '仅停用' },
          ]}
          onChange={(value: 'all' | 'enabled' | 'disabled') => setCustomerStatusFilter(value)}
        />
        <Text type="secondary">显示 {filteredCustomers.length} / {customers.length} 个用户</Text>
      </Space>
      <Table
        rowKey={(record) => record.id}
        columns={customerColumns}
        dataSource={filteredCustomers}
        pagination={{ pageSize: 8, hideOnSinglePage: true }}
        scroll={{ x: 1400, y: '100%' }}
        locale={{ emptyText: <Empty description={customerSearch.trim() || customerStatusFilter !== 'all' ? '没有匹配的用户' : '暂无用户'} /> }}
      />
    </Card>
  )

  const readOnlyAssignmentColumns = visibleAssignmentColumns.filter((column) => column.key !== 'actions')
  const assignmentManagerContent = (
    <CustomerAssignmentManagerCard
      canViewFinance={canViewFinance}
      selectedCustomer={selectedCustomer}
      selectedCustomerID={selectedCustomerID}
      editingAssignmentID={editingAssignmentID}
      assignmentForm={assignmentForm}
      setAssignmentForm={setAssignmentForm}
      savingAssignment={savingAssignment}
      overviewLoading={overviewLoading}
      agentOptions={customerAssignmentAgentOptions}
      frontProxyOptions={selectedCustomerFrontProxyOptions}
      inheritedPriceLabel={inheritedPriceLabel}
      clientTreeData={clientTreeData}
      visibleAssignmentColumns={visibleAssignmentColumns}
      onReset={() => {
        setEditingAssignmentID(null)
        setAssignmentForm(emptyAssignmentForm)
      }}
      onSelectClient={(value) => selectClient(value)}
      onSaveAssignment={saveAssignment}
    />
  )

  const selectedAreaManagerRealmKeys = areaManagerForm.assignments
    .filter((assignment) => assignment.agent_id === areaManagerForm.grant_agent_id && isRealmAssignmentDraft(assignment, agents))
    .map(areaAssignmentKey)
  const selectedCustomerTitle = selectedCustomer ? selectedCustomer.display_name || selectedCustomer.username : '未选择用户'

  const accountEditorModals = (
    <>
      <CustomerManagementModals
      canManageAreaManagers={canManageAreaManagers}
      realmAgentOptions={realmAgentOptions}
      lineAgentOptions={lineAgentOptions}
      landingAgentOptions={landingAgentOptions}
      onFrontLinksChanged={loadFrontProxies}
      agents={agents}
      agentOptions={agentOptions}
      frontProxyOptions={frontProxyOptions}
      editingAreaManagerID={editingAreaManagerID}
      areaManagerModalOpen={areaManagerModalOpen}
      areaManagerForm={areaManagerForm}
      setAreaManagerForm={setAreaManagerForm}
      savingAreaManager={savingAreaManager}
      onCloseAreaManagerModal={closeAreaManagerModal}
      onSaveAreaManager={() => void saveAreaManager()}
      areaManagerRealmGrantOptions={areaManagerRealmGrantOptions.map((option) => ({ value: option.value, label: option.label }))}
      selectedAreaManagerRealmKeys={selectedAreaManagerRealmKeys}
      onUpdateAreaManagerRealmGrantTargets={(values) => void updateAreaManagerRealmGrantTargets(values)}
      onRemoveAreaManagerGrant={removeAreaManagerGrant}
      onRemoveAreaManagerOutboundGrant={removeAreaManagerOutboundGrant}
      customerCreateModalOpen={customerCreateModalOpen}
      setCustomerCreateModalOpen={setCustomerCreateModalOpen}
      customerCreateForm={customerCreateForm}
      setCustomerCreateForm={setCustomerCreateForm}
      customerEditModalOpen={customerEditModalOpen}
      setCustomerEditModalOpen={setCustomerEditModalOpen}
      customerForm={customerForm}
      setCustomerForm={setCustomerForm}
      selectedCustomerID={selectedCustomerID}
      savingCustomer={savingCustomer}
      onCreateCustomer={() => void createCustomer()}
      onSaveCustomer={() => void saveCustomer()}
      customerSubscriptionModalOpen={customerSubscriptionModalOpen}
      subscriptionCustomer={subscriptionCustomer}
      selectedSubscriptionAssignmentIDs={selectedSubscriptionAssignmentIDs}
      setSelectedSubscriptionAssignmentIDs={setSelectedSubscriptionAssignmentIDs}
      copyingSubscriptionCustomerID={copyingSubscriptionCustomerID}
      onCloseCustomerSubscriptionModal={closeCustomerSubscriptionModal}
      onCopyCustomerSubscription={(customerID, assignmentIDs) => void copyCustomerSubscription(customerID, assignmentIDs)}
      assignmentViewModalOpen={assignmentViewModalOpen}
      setAssignmentViewModalOpen={setAssignmentViewModalOpen}
      assignmentManagerModalOpen={assignmentManagerModalOpen}
      setAssignmentManagerModalOpen={setAssignmentManagerModalOpen}
      setEditingAssignmentID={setEditingAssignmentID}
      setAssignmentForm={setAssignmentForm}
      selectedCustomerTitle={selectedCustomerTitle}
      selectedCustomerAssignments={selectedCustomer?.assignments || []}
      readOnlyAssignmentColumns={readOnlyAssignmentColumns}
      assignmentManagerContent={assignmentManagerContent}
      />
      <Modal
        title="用户费用与客户端绑定对账"
        open={billingReconciliationModalOpen}
        onCancel={() => setBillingReconciliationModalOpen(false)}
        footer={<Button onClick={() => setBillingReconciliationModalOpen(false)}>关闭</Button>}
        width={1080}
      >
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
          <Alert
            type="info"
            showIcon
            message={billingReconciliation ? `已匹配 ${billingReconciliation.matched}，旧数据 ${billingReconciliation.legacy}，需修复 ${billingReconciliation.mismatch}，未找到 ${billingReconciliation.missing}` : '按最新 x-ui 上报检查授权绑定'}
            description="“重新绑定”会保留授权名称、费用模式和前置代理，只把客户端关联更新为当前 x-ui 的稳定 ID。"
          />
          <Table
            rowKey={(record) => record.assignment_id}
            loading={billingReconciliationLoading}
            columns={billingReconciliationColumns}
            dataSource={billingReconciliation?.items || []}
            pagination={{ pageSize: 8, hideOnSinglePage: true }}
            scroll={{ x: 940 }}
            locale={{ emptyText: <Empty description="暂无可对账的授权" /> }}
          />
        </Space>
      </Modal>
    </>
  )

  const managementTabs = [
    ...((canManageAreaManagers || adminUser?.role === 'operator') ? [{ key: 'roles', label: 'Client 角色', children: <AgentRolePanel onChanged={() => onConfigChanged?.()} /> }] : []),
    ...(canManageAreaManagers && areaManagersPanel ? [{ key: 'area', label: '区域账号', children: areaManagersPanel }] : []),
    ...(canManageAreaManagers ? [{
      key: 'operators',
      label: '运营管理员',
      children: (
        <OperatorManagementPanel
          operators={operators}
          loading={operatorsLoading}
          saving={savingOperator}
          resettingID={resettingOperatorID}
          onReload={() => void loadOperators()}
          onSave={saveOperator}
          onDelete={(id) => void deleteOperator(id)}
          onResetPassword={(id) => void resetOperatorPassword(id)}
        />
      ),
    }] : []),
    { key: 'customers', label: '用户账号', children: customersPanel },
  ]

  const content = (
    <Spin spinning={loading || areaManagersLoading || operatorsLoading}>
      <div className="customer-management-body">
        <Tabs
          className="customer-admin-tabs"
          tabBarExtraContent={(
            <span className="admin-help-title-group">
              <Text type="secondary">授权说明</Text>
              <AdminHelpHint title={(
                <>
                  <strong>授权使用规则与停用机制</strong>
                  <br />
                  建议仅给已授权用户开放链路，可用于独享或多人共享场景；禁止滥发、攻击、诈骗、爬虫滥用、扫描爆破等行为。出现异常时，可先停用用户账号或单条授权链路；如需彻底切断连接，再到对应 Client 的 x-ui 客户端列表中删除或停用该 client。
                </>
              )} />
            </span>
          )}
          activeKey={activeManagementTab}
          items={managementTabs}
          onChange={(key) => setActiveManagementTab(key as ManagementTabKey)}
        />
      </div>
    </Spin>
  )

  if (embedded) {
    return (
      <div id="customer-management-panel" className="customer-admin-page">
        {content}
        {accountEditorModals}
      </div>
    )
  }

  return (
    <Modal title="人员管理 / 授权链路" open={open} onCancel={onClose} footer={null} width={1160} destroyOnClose>
      {content}
      {accountEditorModals}
    </Modal>
  )
}
