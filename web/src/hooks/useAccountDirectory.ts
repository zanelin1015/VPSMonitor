import { useState } from 'react'
import type { MessageInstance } from 'antd/es/message/interface'
import type { AreaManagerAdminView, CustomerAdminView, FrontProxyNode, OperatorAdminView } from '../types'
import type { OperatorDraft } from '../components/OperatorManagementPanel'
import { DEFAULT_ACCOUNT_PASSWORD } from '../components/CustomerManagementHelpers'
import { fetchJSON } from '../lib/appHelpers'
import { withAppBasePath } from '../lib/appBasePath'

export function useAccountDirectory(canManageAreaManagers: boolean, message: MessageInstance) {
  const [customers, setCustomers] = useState<CustomerAdminView[]>([])
  const [areaManagers, setAreaManagers] = useState<AreaManagerAdminView[]>([])
  const [operators, setOperators] = useState<OperatorAdminView[]>([])
  const [frontProxies, setFrontProxies] = useState<FrontProxyNode[]>([])
  const [loading, setLoading] = useState(false)
  const [areaManagersLoading, setAreaManagersLoading] = useState(false)
  const [operatorsLoading, setOperatorsLoading] = useState(false)
  const [resettingOperatorID, setResettingOperatorID] = useState<number | null>(null)
  const [savingOperator, setSavingOperator] = useState(false)
  const [selectedCustomerID, setSelectedCustomerID] = useState<number | null>(null)

  async function loadCustomers() {
    setLoading(true)
    try {
      const data = await fetchJSON<CustomerAdminView[]>('/api/v1/admin/customers')
      setCustomers(data)
      setSelectedCustomerID((current) => {
        if (current && data.some((item) => item.id === current)) {
          return current
        }
        return data[0]?.id || null
      })
    } catch (error) {
      message.error(error instanceof Error ? error.message : '加载人员失败')
    } finally {
      setLoading(false)
    }
  }

  async function loadAreaManagers() {
    if (!canManageAreaManagers) {
      return
    }
    setAreaManagersLoading(true)
    try {
      const data = await fetchJSON<AreaManagerAdminView[]>('/api/v1/admin/area-managers')
      setAreaManagers(Array.isArray(data) ? data : [])
    } catch (error) {
      message.error(error instanceof Error ? error.message : '加载区域账号失败')
    } finally {
      setAreaManagersLoading(false)
    }
  }

  async function copyAreaManagerRecommendationURL(record: AreaManagerAdminView) {
    const token = record.recommendation_token?.trim()
    if (!token) {
      message.error('该区域账号暂未生成推荐页 URL')
      return
    }
    const url = `${window.location.origin}${withAppBasePath('/recommendations')}?area=${encodeURIComponent(token)}`
    try {
      await navigator.clipboard.writeText(url)
      message.success('区域推荐页 URL 已复制')
    } catch (error) {
      message.error(error instanceof Error ? error.message : '复制 URL 失败')
    }
  }

  async function loadOperators() {
    if (!canManageAreaManagers) {
      return
    }
    setOperatorsLoading(true)
    try {
      const data = await fetchJSON<OperatorAdminView[]>('/api/v1/admin/operators')
      setOperators(Array.isArray(data) ? data : [])
    } catch (error) {
      message.error(error instanceof Error ? error.message : '加载运营管理员失败')
    } finally {
      setOperatorsLoading(false)
    }
  }

  async function loadFrontProxies() {
    try {
      const data = await fetchJSON<FrontProxyNode[]>('/api/v1/admin/front-proxies')
      setFrontProxies(Array.isArray(data) ? data : [])
    } catch (error) {
      setFrontProxies([])
      message.error(error instanceof Error ? error.message : '加载前置代理失败')
    }
  }

  async function saveOperator(draft: OperatorDraft): Promise<boolean> {
    setSavingOperator(true)
    try {
      const payload = {
        username: draft.username.trim(),
        display_name: draft.display_name.trim(),
        password: draft.password,
        enabled: draft.enabled,
      }
      if (draft.id) {
        await fetchJSON<OperatorAdminView>(`/api/v1/admin/operators/${draft.id}`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        })
        message.success('运营管理员已更新')
      } else {
        await fetchJSON<OperatorAdminView>('/api/v1/admin/operators', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        })
        message.success('运营管理员已创建')
      }
      await loadOperators()
      return true
    } catch (error) {
      message.error(error instanceof Error ? error.message : '保存运营管理员失败')
      return false
    } finally {
      setSavingOperator(false)
    }
  }

  async function deleteOperator(id: number) {
    setSavingOperator(true)
    try {
      await fetchJSON(`/api/v1/admin/operators/${id}`, { method: 'DELETE' })
      message.success('运营管理员已删除')
      await loadOperators()
    } catch (error) {
      message.error(error instanceof Error ? error.message : '删除运营管理员失败')
    } finally {
      setSavingOperator(false)
    }
  }

  async function resetOperatorPassword(id: number) {
    setResettingOperatorID(id)
    try {
      await fetchJSON<OperatorAdminView>(`/api/v1/admin/operators/${id}/reset-password`, { method: 'POST' })
      message.success(`运营管理员密码已初始化为 ${DEFAULT_ACCOUNT_PASSWORD}`)
      await loadOperators()
    } catch (error) {
      message.error(error instanceof Error ? error.message : '初始化运营管理员密码失败')
    } finally {
      setResettingOperatorID(null)
    }
  }

  return { customers, setCustomers, areaManagers, setAreaManagers, operators, frontProxies, loading, areaManagersLoading, operatorsLoading, resettingOperatorID, savingOperator, selectedCustomerID, setSelectedCustomerID, loadCustomers, loadAreaManagers, copyAreaManagerRecommendationURL, loadOperators, loadFrontProxies, saveOperator, deleteOperator, resetOperatorPassword }
}
