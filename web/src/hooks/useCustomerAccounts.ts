import { useRef, useState } from 'react'
import type { MessageInstance } from 'antd/es/message/interface'
import type { CustomerAdminView, CustomerSubscriptionURLResponse } from '../types'
import { fetchJSON } from '../lib/appHelpers'
import { DEFAULT_ACCOUNT_PASSWORD, emptyCustomerForm, type CustomerFormState } from '../components/CustomerManagementHelpers'
import type { useAccountDirectory } from './useAccountDirectory'

type Options = Pick<ReturnType<typeof useAccountDirectory>, 'selectedCustomerID' | 'setSelectedCustomerID' | 'setCustomers' | 'setAreaManagers' | 'loadCustomers' | 'loadAreaManagers'> & {
  canManageAreaManagers: boolean
  message: MessageInstance
  onCustomerDeleted: (wasSelected: boolean) => void
}

export function useCustomerAccounts({ canManageAreaManagers, message, selectedCustomerID, setSelectedCustomerID, setCustomers, setAreaManagers, loadCustomers, loadAreaManagers, onCustomerDeleted }: Options) {
  const [savingCustomer, setSavingCustomer] = useState(false)
  const [changingCustomerStatusIDs, setChangingCustomerStatusIDs] = useState<number[]>([])
  const customerStatusPendingRef = useRef(new Set<number>())
  const [resettingCustomerID, setResettingCustomerID] = useState<number | null>(null)
  const [copyingSubscriptionCustomerID, setCopyingSubscriptionCustomerID] = useState<number | null>(null)
  const [customerCreateModalOpen, setCustomerCreateModalOpen] = useState(false)
  const [customerEditModalOpen, setCustomerEditModalOpen] = useState(false)
  const [customerSubscriptionModalOpen, setCustomerSubscriptionModalOpen] = useState(false)
  const [subscriptionCustomer, setSubscriptionCustomer] = useState<CustomerAdminView | null>(null)
  const [selectedSubscriptionAssignmentIDs, setSelectedSubscriptionAssignmentIDs] = useState<number[]>([])
  const [customerForm, setCustomerForm] = useState<CustomerFormState>(emptyCustomerForm)
  const [customerCreateForm, setCustomerCreateForm] = useState<CustomerFormState>(emptyCustomerForm)

  function openCustomerCreateModal() {
    setCustomerCreateForm({
      ...emptyCustomerForm,
      password: DEFAULT_ACCOUNT_PASSWORD,
    })
    setCustomerCreateModalOpen(true)
  }

  function openCustomerEditModal(record: CustomerAdminView) {
    setSelectedCustomerID(record.id)
    setCustomerForm({
      username: record.username,
      password: '',
      display_name: record.display_name || record.username,
      enabled: record.enabled,
      front_proxy_node_ids: (record.front_proxies || []).map((item) => item.id),
    })
    setCustomerEditModalOpen(true)
  }

  function openCustomerSubscriptionModal(record: CustomerAdminView) {
    setSubscriptionCustomer(record)
    setSelectedSubscriptionAssignmentIDs([])
    setCustomerSubscriptionModalOpen(true)
  }

  function closeCustomerSubscriptionModal() {
    setCustomerSubscriptionModalOpen(false)
    setSubscriptionCustomer(null)
    setSelectedSubscriptionAssignmentIDs([])
  }

  async function createCustomer() {
    if (!customerCreateForm.username.trim()) {
      message.warning('请填写用户用户名')
      return
    }
    if (customerCreateForm.password.length < 8) {
      message.warning('新用户密码至少 8 位')
      return
    }
    setSavingCustomer(true)
    try {
      const created = await fetchJSON<CustomerAdminView>('/api/v1/admin/customers', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          username: customerCreateForm.username.trim(),
          password: customerCreateForm.password,
          display_name: customerCreateForm.display_name.trim(),
          enabled: customerCreateForm.enabled,
          front_proxy_node_ids: customerCreateForm.front_proxy_node_ids,
        }),
      })
      message.success('用户已创建')
      setCustomerCreateModalOpen(false)
      setCustomerCreateForm(emptyCustomerForm)
      await loadCustomers()
      setSelectedCustomerID(created.id)
      if (canManageAreaManagers) {
        await loadAreaManagers()
      }
    } catch (error) {
      message.error(error instanceof Error ? error.message : '创建用户失败')
    } finally {
      setSavingCustomer(false)
    }
  }

  async function toggleCustomerStatus(record: CustomerAdminView) {
    if (customerStatusPendingRef.current.has(record.id)) {
      return
    }
    customerStatusPendingRef.current.add(record.id)
    setChangingCustomerStatusIDs((current) => [...current, record.id])
    try {
      const updated = await fetchJSON<CustomerAdminView>(`/api/v1/admin/customers/${record.id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ enabled: !record.enabled }),
      })
      setCustomers((current) => current.map((item) => item.id === updated.id ? updated : item))
      setAreaManagers((current) => current.map((manager) => ({
        ...manager,
        customers: manager.customers?.map((item) => item.id === updated.id ? updated : item),
      })))
      setCustomerForm((current) => current.username === updated.username ? { ...current, enabled: updated.enabled } : current)
      message.success(`用户已${updated.enabled ? '启用' : '禁用'}`)
    } catch (error) {
      message.error(error instanceof Error ? error.message : '切换用户状态失败')
    } finally {
      customerStatusPendingRef.current.delete(record.id)
      setChangingCustomerStatusIDs((current) => current.filter((id) => id !== record.id))
    }
  }

  async function saveCustomer() {
    if (!customerForm.username.trim()) {
      message.warning('请填写用户用户名')
      return
    }
    if (!selectedCustomerID && customerForm.password.length < 8) {
      message.warning('新用户密码至少 8 位')
      return
    }
    setSavingCustomer(true)
    try {
      const payload = {
        username: customerForm.username.trim(),
        password: customerForm.password,
        display_name: customerForm.display_name.trim(),
        enabled: customerForm.enabled,
        front_proxy_node_ids: customerForm.front_proxy_node_ids,
      }
      if (selectedCustomerID) {
        await fetchJSON<CustomerAdminView>(`/api/v1/admin/customers/${selectedCustomerID}`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        })
        message.success('用户已更新')
      } else {
        const created = await fetchJSON<CustomerAdminView>('/api/v1/admin/customers', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        })
        setSelectedCustomerID(created.id)
        message.success('用户已创建')
      }
      setCustomerForm((current) => ({ ...current, password: '' }))
      setCustomerEditModalOpen(false)
      await loadCustomers()
      if (canManageAreaManagers) {
        await loadAreaManagers()
      }
    } catch (error) {
      message.error(error instanceof Error ? error.message : '保存用户失败')
    } finally {
      setSavingCustomer(false)
    }
  }

  async function deleteCustomer(customerID = selectedCustomerID) {
    if (!customerID) {
      return
    }
    setSavingCustomer(true)
    try {
      await fetchJSON(`/api/v1/admin/customers/${customerID}`, { method: 'DELETE' })
      if (selectedCustomerID === customerID) {
        setSelectedCustomerID(null)
        setCustomerForm(emptyCustomerForm)
      }
      setCustomerEditModalOpen(false)
      onCustomerDeleted(selectedCustomerID === customerID)
      message.success('用户已删除')
      await loadCustomers()
      if (canManageAreaManagers) {
        await loadAreaManagers()
      }
    } catch (error) {
      message.error(error instanceof Error ? error.message : '删除用户失败')
    } finally {
      setSavingCustomer(false)
    }
  }

  async function resetCustomerPassword(customerID: number) {
    setResettingCustomerID(customerID)
    try {
      await fetchJSON<CustomerAdminView>(`/api/v1/admin/customers/${customerID}/reset-password`, { method: 'POST' })
      message.success(`普通用户密码已初始化为 ${DEFAULT_ACCOUNT_PASSWORD}`)
      await loadCustomers()
      if (canManageAreaManagers) {
        await loadAreaManagers()
      }
    } catch (error) {
      message.error(error instanceof Error ? error.message : '初始化普通用户密码失败')
    } finally {
      setResettingCustomerID(null)
    }
  }

  async function copyCustomerSubscription(customerID: number, assignmentIDs: number[]) {
    if (assignmentIDs.length === 0) {
      message.warning('请至少选择一个导出节点')
      return
    }
    setCopyingSubscriptionCustomerID(customerID)
    try {
      const query = new URLSearchParams({ assignments: assignmentIDs.join(',') })
      const data = await fetchJSON<CustomerSubscriptionURLResponse>(`/api/v1/admin/customers/${customerID}/subscription?${query.toString()}`)
      const subscriptionURL = data.clash_subscription_url || data.mihomo_subscription_url || ''
      if (!subscriptionURL) {
        message.warning('当前用户没有可用的 Clash 订阅地址')
        return
      }
      await navigator.clipboard.writeText(subscriptionURL)
      message.success('Clash 订阅地址已复制')
      closeCustomerSubscriptionModal()
    } catch (error) {
      message.error(error instanceof Error ? error.message : '复制 Clash 订阅地址失败')
    } finally {
      setCopyingSubscriptionCustomerID(null)
    }
  }

  return {
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
  }
}
