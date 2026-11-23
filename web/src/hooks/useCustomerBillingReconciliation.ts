import { useState } from 'react'
import type { CustomerBillingDiagnostic, CustomerBillingReconciliationResponse } from '../types'
import { fetchJSON } from '../lib/appHelpers'

interface MessageApi {
  error: (content: string) => void
  success: (content: string) => void
}

type LoadCustomers = () => Promise<unknown>

export function useCustomerBillingReconciliation(message: MessageApi, loadCustomers: LoadCustomers) {
  const [modalOpen, setModalOpen] = useState(false)
  const [result, setResult] = useState<CustomerBillingReconciliationResponse | null>(null)
  const [loading, setLoading] = useState(false)
  const [repairingAssignmentID, setRepairingAssignmentID] = useState<number | null>(null)

  async function open() {
    setModalOpen(true)
    setLoading(true)
    try {
      const nextResult = await fetchJSON<CustomerBillingReconciliationResponse>('/api/v1/admin/customers/billing-reconciliation')
      setResult(nextResult)
    } catch (error) {
      setResult(null)
      message.error(error instanceof Error ? error.message : '加载费用对账失败')
    } finally {
      setLoading(false)
    }
  }

  async function rebind(record: CustomerBillingDiagnostic) {
    if (!record.current_client_id) {
      return
    }
    setRepairingAssignmentID(record.assignment_id)
    try {
      await fetchJSON(`/api/v1/admin/customers/${record.customer_id}/assignments/${record.assignment_id}/rebind`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ client_id: record.current_client_id }),
      })
      message.success('客户端已重新绑定')
      await loadCustomers()
      await open()
    } catch (error) {
      message.error(error instanceof Error ? error.message : '重新绑定失败')
    } finally {
      setRepairingAssignmentID(null)
    }
  }

  return {
    modalOpen,
    setModalOpen,
    result,
    loading,
    repairingAssignmentID,
    open,
    rebind,
  }
}
