import type { CustomerAdminView, CustomerAssignment, CustomerAssignmentDraft } from '../types'

export function customerMatchesSearch(customer: CustomerAdminView, query: string) {
  const keyword = query.trim().toLocaleLowerCase()
  return !keyword || [customer.username, customer.display_name || ''].some(value => value.toLocaleLowerCase().includes(keyword))
}

export function existingCustomerAuthorization(customer: CustomerAdminView, draft: CustomerAssignmentDraft) {
  return (customer.assignments || []).find(assignment => {
    if (assignment.agent_id !== draft.agent_id || assignment.inbound_id !== draft.inbound_id) return false
    if (draft.client_id && assignment.client_id) return draft.client_id === assignment.client_id
    if (draft.client_email) return draft.client_email === assignment.client_email
    if (draft.client_id) return false
    return !assignment.client_id && !assignment.client_email
  })
}

export function customerAuthorizationPayload(draft: CustomerAssignmentDraft) {
  // Quick authorization inherits pricing; never changes the client's billing.
  return {
    agent_id: draft.agent_id,
    inbound_id: draft.inbound_id,
    inbound_tag: draft.inbound_tag || '',
    client_id: draft.client_id || '',
    client_email: draft.client_email || '',
    public_client_name: draft.public_client_name || '',
    price_mode: 'inherit' as const,
    enabled: true,
  }
}

export async function authorizeCustomersBatch(
  customers: CustomerAdminView[],
  selectedIDs: number[],
  draft: CustomerAssignmentDraft,
  create: (customerID: number, payload: ReturnType<typeof customerAuthorizationPayload>) => Promise<CustomerAssignment>,
  onProgress: (completed: number, total: number) => void = () => undefined,
) {
  const result: {
    created: CustomerAssignment[]
    skippedIDs: number[]
    failed: { customerID: number; username: string; error: string }[]
  } = { created: [], skippedIDs: [], failed: [] }
  const selected = [...new Set(selectedIDs)]
  const byID = new Map(customers.map(customer => [customer.id, customer]))
  let next = 0
  let completed = 0
  const payload = customerAuthorizationPayload(draft)
  async function worker() {
    while (next < selected.length) {
      const id = selected[next++]
      const customer = byID.get(id)
      if (!customer) {
        result.failed.push({ customerID: id, username: `用户 #${id}`, error: '用户已删除或不再具有访问权限' })
      } else if (existingCustomerAuthorization(customer, draft)) {
        result.skippedIDs.push(id)
      } else {
        try {
          result.created.push(await create(id, payload))
        } catch (error) {
          result.failed.push({ customerID: id, username: customer.username, error: error instanceof Error ? error.message : '授权失败' })
        }
      }
      onProgress(++completed, selected.length)
    }
  }
  await Promise.all(Array.from({ length: Math.min(3, selected.length) }, worker))
  return result
}
