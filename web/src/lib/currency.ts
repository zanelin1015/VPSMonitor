import type { AgentListItem, AreaManagerAdminView, ClientChainView, CustomerAdminView, VPSRenewalConfig, XUIClientBillingConfig } from '../types'

export const COMMON_COST_CURRENCIES = ['USD', 'USDT', 'CNY', 'CAD', 'EUR', 'HKD', 'JPY', 'GBP', 'AUD', 'SGD']
export const DEFAULT_COST_CURRENCY = 'USD'
export const COST_CURRENCY_STORAGE_KEY = 'bridge-core.cost-currency'
export const REVENUE_CURRENCIES = ['CNY', 'USDT']

export type CurrencyCode = string

export interface ExchangeRatesState {
  base: CurrencyCode
  date: string
  rates: Record<CurrencyCode, number>
  loading: boolean
  error: string
}

export interface MonthlyCostSummary {
  total: number
  count: number
  missingCount: number
}

export interface MonthlyFinanceSummary {
  costTotal: number
  revenueTotal: number
  profitTotal: number
  costCount: number
  revenueCount: number
  missingCostCount: number
  missingRevenueCount: number
}

export interface MonthlyFinancePaymentInfo {
  date: string
  status: 'paid' | 'today' | 'pending'
}

export interface MonthlyFinanceCostDetail {
  key: string
  agentID: string
  agentName: string
  tags: string[]
  amount: number
  currency: CurrencyCode
  cycle: VPSRenewalConfig['cost_cycle']
  monthlyAmount: number | null
  payment: MonthlyFinancePaymentInfo | null
}

export interface MonthlyFinanceRevenueDetail {
  key: string
  agentID: string
  agentName: string
  inboundID: number
  inboundTag: string
  nodeLabel: string
  nodeDetail: string
  clientEmail: string
  clientLabel: string
  clientRemark: string
  amount: number
  currency: CurrencyCode
  cycle: VPSRenewalConfig['cost_cycle']
  monthlyAmount: number | null
  payment: MonthlyFinancePaymentInfo
  source: 'client' | 'billing' | 'area_account'
}

type BillingConfig = Pick<VPSRenewalConfig, 'cost_amount' | 'cost_currency' | 'cost_cycle'>

export function summarizeMonthlyCost(agents: AgentListItem[], targetCurrency: CurrencyCode, exchangeRates: ExchangeRatesState): MonthlyCostSummary {
  return agents.reduce<MonthlyCostSummary>(
    (summary, agent) => {
      const renewal = normalizeCostConfig(agent.renewal)
      const amount = Number(renewal.cost_amount || 0)
      if (amount <= 0) {
        summary.missingCount += 1
        return summary
      }
      const monthlyAmount = amount / billingCycleMonths(renewal.cost_cycle || 'month')
      const converted = convertCurrency(monthlyAmount, normalizeCurrencyCode(renewal.cost_currency), targetCurrency, exchangeRates)
      if (converted === null) {
        summary.missingCount += 1
        return summary
      }
      summary.total += converted
      summary.count += 1
      return summary
    },
    { total: 0, count: 0, missingCount: 0 },
  )
}

export function summarizeMonthlyFinance(
  agents: AgentListItem[],
  clientChains: ClientChainView[],
  targetCurrency: CurrencyCode,
  exchangeRates: ExchangeRatesState,
  customers: CustomerAdminView[] = [],
  areaManagers: AreaManagerAdminView[] = [],
): MonthlyFinanceSummary {
  const singleUserRevenueRows = buildMonthlyFinanceRevenueDetails(agents, clientChains, targetCurrency, exchangeRates, customers, areaManagers)
  const summary = agents.reduce<MonthlyFinanceSummary>(
    (summary, agent) => {
      const billing = normalizeBillingConfig(agent.renewal)
      const cost = monthlyConvertedAmount(billing.cost_amount, billing.cost_currency, billing.cost_cycle, targetCurrency, exchangeRates)
      if (cost === null) {
        summary.missingCostCount += 1
      } else {
        summary.costTotal += cost
        summary.costCount += 1
      }
      summary.profitTotal = summary.revenueTotal - summary.costTotal
      return summary
    },
    { costTotal: 0, revenueTotal: 0, profitTotal: 0, costCount: 0, revenueCount: 0, missingCostCount: 0, missingRevenueCount: 0 },
  )
  for (const row of singleUserRevenueRows) {
    if (row.monthlyAmount === null) {
      summary.missingRevenueCount += 1
      continue
    }
    summary.revenueTotal += row.monthlyAmount
    summary.revenueCount += 1
  }
  summary.profitTotal = summary.revenueTotal - summary.costTotal
  return summary
}

export function buildMonthlyFinanceCostDetails(agents: AgentListItem[], targetCurrency: CurrencyCode, exchangeRates: ExchangeRatesState): MonthlyFinanceCostDetail[] {
  return agents.map((agent) => {
    const billing = normalizeBillingConfig(agent.renewal)
    return {
      key: agent.agent_id,
      agentID: agent.agent_id,
      agentName: agent.agent_name || agent.summary?.hostname || agent.agent_id,
      tags: agent.tags || [],
      amount: billing.cost_amount,
      currency: billing.cost_currency,
      cycle: billing.cost_cycle,
      monthlyAmount: monthlyConvertedAmount(billing.cost_amount, billing.cost_currency, billing.cost_cycle, targetCurrency, exchangeRates),
      payment: costPaymentInfo(agent.renewal?.start_date, billing.cost_cycle),
    }
  })
}

export function buildMonthlyFinanceRevenueDetails(
  agents: AgentListItem[],
  clientChains: ClientChainView[],
  targetCurrency: CurrencyCode,
  exchangeRates: ExchangeRatesState,
  customers: CustomerAdminView[] = [],
  areaManagers: AreaManagerAdminView[] = [],
): MonthlyFinanceRevenueDetail[] {
  const agentByID = new Map(agents.map((agent) => [agent.agent_id, agent]))
  const billingByClient = new Map<string, RevenueBillingRef>()
  const billingByEmail = new Map<string, RevenueBillingRef>()

  for (const agent of agents) {
    for (const billing of agent.renewal?.client_billings || []) {
      const normalized = normalizeRevenueBilling(billing)
      const ref: RevenueBillingRef = { agent, billing: normalized }
      billingByClient.set(revenueClientKey(agent.agent_id, normalized.inbound_tag, normalized.email), ref)
      if (normalized.email && !billingByEmail.has(revenueEmailKey(agent.agent_id, normalized.email))) {
        billingByEmail.set(revenueEmailKey(agent.agent_id, normalized.email), ref)
      }
    }
  }

  const rows: MonthlyFinanceRevenueDetail[] = []
  const hasCustomerScope = customers.length > 0
  for (const chain of clientChains) {
    if (chain.root_client_enabled === false) {
      continue
    }
    const agent = agentByID.get(chain.root_agent_id)
    if (!agent) {
      continue
    }
    const ref = billingByClient.get(revenueClientKey(chain.root_agent_id, chain.root_inbound_tag || '', chain.root_client_email || ''))
      || billingByEmail.get(revenueEmailKey(chain.root_agent_id, chain.root_client_email || ''))
    const inboundStep = rootInboundStep(chain)
    const row = buildRevenueDetailRow({
      agent,
      billing: ref?.billing,
      key: `client:${chain.key}`,
      inboundTag: chain.root_inbound_tag || ref?.billing.inbound_tag || '',
      inboundID: ref?.billing.inbound_id || 0,
      nodeLabel: inboundStep?.label || '',
      nodeDetail: inboundStep?.detail || '',
      clientEmail: chain.root_client_email || ref?.billing.email || '',
      clientRemark: chain.root_client_remark || '',
      source: 'client',
      targetCurrency,
      exchangeRates,
    })
    if (!hasCustomerScope || isSingleUserRevenueRow(row, customers)) {
      rows.push(row)
    }
  }
  for (const manager of areaManagers) {
    if (!manager.enabled || !manager.billing_enabled || Number(manager.revenue_amount || 0) <= 0) {
      continue
    }
    rows.push(buildAreaManagerRevenueDetailRow(manager, targetCurrency, exchangeRates))
  }
  return rows.filter((row) => row.amount > 0).sort((left, right) => {
    if (left.agentName !== right.agentName) {
      return left.agentName.localeCompare(right.agentName)
    }
    if (left.inboundTag !== right.inboundTag) {
      return left.inboundTag.localeCompare(right.inboundTag)
    }
    return left.clientLabel.localeCompare(right.clientLabel)
  })
}

function isSingleUserRevenueRow(row: MonthlyFinanceRevenueDetail, customers: CustomerAdminView[]): boolean {
  return customers.some((customer) => {
    if (customer.owner_type === 'area_manager') {
      return false
    }
    return (customer.assignments || []).some((assignment) => assignmentMatchesRevenueRow(assignment, row))
  })
}

function assignmentMatchesRevenueRow(assignment: { agent_id: string; inbound_id: number; inbound_tag?: string; client_email?: string }, row: MonthlyFinanceRevenueDetail): boolean {
  if (assignment.agent_id !== row.agentID) {
    return false
  }
  const assignmentEmail = (assignment.client_email || '').toLowerCase()
  const rowEmail = (row.clientEmail || '').toLowerCase()
  if (assignmentEmail && rowEmail && assignmentEmail === rowEmail) {
    return true
  }
  if (assignment.inbound_id > 0 && row.inboundID > 0 && assignment.inbound_id === row.inboundID) {
    return !assignmentEmail || !rowEmail || assignmentEmail === rowEmail
  }
  const assignmentTag = (assignment.inbound_tag || '').toLowerCase()
  const rowTag = (row.inboundTag || '').toLowerCase()
  return Boolean(assignmentTag && rowTag && assignmentTag === rowTag && (!assignmentEmail || !rowEmail || assignmentEmail === rowEmail))
}

function monthlyConvertedAmount(amount: number, currency: CurrencyCode, cycle: VPSRenewalConfig['cost_cycle'], targetCurrency: CurrencyCode, exchangeRates: ExchangeRatesState): number | null {
  if (amount <= 0) {
    return null
  }
  const monthlyAmount = amount / billingCycleMonths(cycle || 'month')
  return convertCurrency(monthlyAmount, normalizeCurrencyCode(currency), targetCurrency, exchangeRates)
}

function enabledRevenueBillingKeys(clientChains: ClientChainView[]): Set<string> {
  const keys = new Set<string>()
  for (const chain of clientChains || []) {
    if (chain.root_client_enabled === false) {
      continue
    }
    keys.add(revenueClientKey(chain.root_agent_id, chain.root_inbound_tag || '', chain.root_client_email || ''))
  }
  return keys
}

function enabledRevenueBillingEmailKeys(clientChains: ClientChainView[]): Set<string> {
  const keys = new Set<string>()
  for (const chain of clientChains || []) {
    if (chain.root_client_enabled === false || !chain.root_client_email) {
      continue
    }
    keys.add(revenueEmailKey(chain.root_agent_id, chain.root_client_email || ''))
  }
  return keys
}

type RevenueBillingRef = {
  agent: AgentListItem
  billing: Required<Pick<XUIClientBillingConfig, 'inbound_id' | 'inbound_tag' | 'email' | 'revenue_amount' | 'revenue_currency' | 'revenue_cycle'>>
}

function normalizeRevenueBilling(billing: XUIClientBillingConfig): RevenueBillingRef['billing'] {
  return {
    inbound_id: Number(billing.inbound_id || 0),
    inbound_tag: billing.inbound_tag || '',
    email: billing.email || '',
    revenue_amount: Math.max(0, Number(billing.revenue_amount || 0)),
    revenue_currency: billing.revenue_currency === 'USDT' ? 'USDT' : 'CNY',
    revenue_cycle: billing.revenue_cycle === 'quarter' || billing.revenue_cycle === 'year' ? billing.revenue_cycle : 'month',
  }
}

function buildRevenueDetailRow(options: {
  agent: AgentListItem
  billing?: RevenueBillingRef['billing']
  key: string
  inboundTag: string
  inboundID?: number
  nodeLabel?: string
  nodeDetail?: string
  clientEmail: string
  clientRemark: string
  source: MonthlyFinanceRevenueDetail['source']
  targetCurrency: CurrencyCode
  exchangeRates: ExchangeRatesState
}): MonthlyFinanceRevenueDetail {
  const amount = Math.max(0, Number(options.billing?.revenue_amount || 0))
  const currency = normalizeCurrencyCode(options.billing?.revenue_currency || 'CNY')
  const cycle = options.billing?.revenue_cycle === 'quarter' || options.billing?.revenue_cycle === 'year' ? options.billing.revenue_cycle : 'month'
  const clientEmail = options.clientEmail || options.billing?.email || ''
  const clientRemark = options.clientRemark || ''
  return {
    key: options.key,
    agentID: options.agent.agent_id,
    agentName: options.agent.agent_name || options.agent.summary?.hostname || options.agent.agent_id,
    inboundID: options.inboundID || options.billing?.inbound_id || 0,
    inboundTag: options.inboundTag || options.billing?.inbound_tag || '',
    nodeLabel: options.nodeLabel || options.inboundTag || options.billing?.inbound_tag || '',
    nodeDetail: options.nodeDetail || '',
    clientEmail,
    clientLabel: clientEmail || clientRemark || 'anonymous-client',
    clientRemark,
    amount,
    currency,
    cycle,
    monthlyAmount: monthlyConvertedAmount(amount, currency, cycle, options.targetCurrency, options.exchangeRates),
    payment: revenuePaymentInfo(cycle),
    source: options.source,
  }
}

function buildAreaManagerRevenueDetailRow(manager: AreaManagerAdminView, targetCurrency: CurrencyCode, exchangeRates: ExchangeRatesState): MonthlyFinanceRevenueDetail {
  const amount = Math.max(0, Number(manager.revenue_amount || 0))
  const currency = normalizeCurrencyCode(manager.revenue_currency || 'CNY')
  const cycle = manager.revenue_cycle === 'quarter' || manager.revenue_cycle === 'year' ? manager.revenue_cycle : 'month'
  const label = manager.display_name || manager.username
  return {
    key: `area:${manager.id}`,
    agentID: '',
    agentName: '区域账号',
    inboundID: 0,
    inboundTag: '',
    nodeLabel: '区域账号整体收费',
    nodeDetail: manager.username,
    clientEmail: '',
    clientLabel: label,
    clientRemark: '区域账号收入',
    amount,
    currency,
    cycle,
    monthlyAmount: monthlyConvertedAmount(amount, currency, cycle, targetCurrency, exchangeRates),
    payment: revenuePaymentInfo(cycle),
    source: 'area_account',
  }
}


function rootInboundStep(chain: ClientChainView) {
  return (chain.steps || []).find((step) => step.step_type === 'inbound' && step.agent_id === chain.root_agent_id)
    || (chain.steps || []).find((step) => step.step_type === 'inbound')
}

function revenueClientKey(agentID: string, inboundTag: string, email: string): string {
  return `${agentID}\u0000${inboundTag.trim().toLowerCase()}\u0000${email.trim().toLowerCase()}`
}

function revenueEmailKey(agentID: string, email: string): string {
  return `${agentID}\u0000${email.trim().toLowerCase()}`
}

function costPaymentInfo(startDate: string | undefined, cycle: VPSRenewalConfig['cost_cycle']): MonthlyFinancePaymentInfo | null {
  const start = parseDateOnly(startDate)
  if (!start) {
    return null
  }
  const today = todayDateOnly()
  const cycleMonths = billingCycleMonths(cycle || 'month')
  const startMonth = monthSerial(start)
  const currentMonth = monthSerial(today)
  let due: Date
  if (currentMonth < startMonth) {
    due = start
  } else {
    const monthsSinceStart = currentMonth - startMonth
    const dueMonth = cycleMonths === 1
      ? currentMonth
      : startMonth + Math.floor(monthsSinceStart / cycleMonths) * cycleMonths
    due = makeDateFromMonthSerial(dueMonth, start.getDate())
  }
  return paymentInfo(due, today)
}

function revenuePaymentInfo(cycle: VPSRenewalConfig['cost_cycle']): MonthlyFinancePaymentInfo {
  const today = todayDateOnly()
  let month = today.getMonth()
  if (cycle === 'quarter') {
    month = Math.floor(month / 3) * 3
  } else if (cycle === 'year') {
    month = 0
  }
  return paymentInfo(new Date(today.getFullYear(), month, 1), today)
}

function paymentInfo(due: Date, today: Date): MonthlyFinancePaymentInfo {
  const comparison = compareDateOnly(today, due)
  return {
    date: formatDateOnly(due),
    status: comparison > 0 ? 'paid' : comparison === 0 ? 'today' : 'pending',
  }
}

function parseDateOnly(value?: string): Date | null {
  const match = String(value || '').match(/^(\d{4})-(\d{2})-(\d{2})$/)
  if (!match) {
    return null
  }
  const year = Number(match[1])
  const month = Number(match[2]) - 1
  const day = Number(match[3])
  if (!Number.isFinite(year) || !Number.isFinite(month) || !Number.isFinite(day)) {
    return null
  }
  return makeDate(year, month, day)
}

function todayDateOnly(): Date {
  const now = new Date()
  return new Date(now.getFullYear(), now.getMonth(), now.getDate())
}

function monthSerial(date: Date): number {
  return date.getFullYear() * 12 + date.getMonth()
}

function makeDateFromMonthSerial(serial: number, day: number): Date {
  return makeDate(Math.floor(serial / 12), serial % 12, day)
}

function makeDate(year: number, month: number, day: number): Date {
  return new Date(year, month, Math.min(day, daysInMonth(year, month)))
}

function daysInMonth(year: number, month: number): number {
  return new Date(year, month + 1, 0).getDate()
}

function compareDateOnly(left: Date, right: Date): number {
  return Number(left) - Number(right)
}

function formatDateOnly(date: Date): string {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

function normalizeCostConfig(config?: VPSRenewalConfig): Pick<VPSRenewalConfig, 'cost_amount' | 'cost_currency' | 'cost_cycle'> {
  return {
    cost_amount: Math.max(0, Number(config?.cost_amount || 0)),
    cost_currency: normalizeCurrencyCode(config?.cost_currency),
    cost_cycle: config?.cost_cycle === 'quarter' || config?.cost_cycle === 'year' ? config.cost_cycle : 'month',
  }
}

function normalizeBillingConfig(config?: VPSRenewalConfig): Required<BillingConfig> {
  return {
    cost_amount: Math.max(0, Number(config?.cost_amount || 0)),
    cost_currency: normalizeCurrencyCode(config?.cost_currency),
    cost_cycle: config?.cost_cycle === 'quarter' || config?.cost_cycle === 'year' ? config.cost_cycle : 'month',
  }
}

function billingCycleMonths(cycle?: VPSRenewalConfig['cost_cycle']): number {
  switch (cycle) {
    case 'quarter':
      return 3
    case 'year':
      return 12
    case 'month':
    default:
      return 1
  }
}

function convertCurrency(amount: number, from: CurrencyCode, to: CurrencyCode, exchangeRates: ExchangeRatesState): number | null {
  if (from === to) {
    return amount
  }
  const fromRate = currencyRate(from, exchangeRates)
  const toRate = currencyRate(to, exchangeRates)
  if (!fromRate || !toRate) {
    return null
  }
  return (amount / fromRate) * toRate
}

function currencyRate(currency: CurrencyCode, exchangeRates: ExchangeRatesState): number | undefined {
  return exchangeRates.rates[currency === 'USDT' ? 'USD' : currency]
}

export function normalizeCurrencyCode(value?: string): CurrencyCode {
  const upper = (value || DEFAULT_COST_CURRENCY).trim().toUpperCase()
  return upper === 'USDT' || /^[A-Z]{3}$/.test(upper) ? upper : DEFAULT_COST_CURRENCY
}

export function mergeCurrencyOptions(currencies: string[]): CurrencyCode[] {
  const merged = new Set([...COMMON_COST_CURRENCIES, ...currencies.map((currency) => normalizeCurrencyCode(currency))])
  return Array.from(merged).sort((left, right) => {
    const leftCommonIndex = COMMON_COST_CURRENCIES.indexOf(left)
    const rightCommonIndex = COMMON_COST_CURRENCIES.indexOf(right)
    if (leftCommonIndex >= 0 || rightCommonIndex >= 0) {
      return (leftCommonIndex >= 0 ? leftCommonIndex : Number.MAX_SAFE_INTEGER) - (rightCommonIndex >= 0 ? rightCommonIndex : Number.MAX_SAFE_INTEGER)
    }
    return left.localeCompare(right)
  })
}

export function defaultExchangeRatesState(): ExchangeRatesState {
  return {
    base: 'EUR',
    date: '',
    rates: { EUR: 1 },
    loading: false,
    error: '',
  }
}

export function readStoredCostCurrency(): CurrencyCode {
  try {
    return normalizeCurrencyCode(window.localStorage.getItem(COST_CURRENCY_STORAGE_KEY) || DEFAULT_COST_CURRENCY)
  } catch {
    return DEFAULT_COST_CURRENCY
  }
}

export function formatMoney(value: number, currency: CurrencyCode): string {
  if (currency === 'USDT') {
    return `USDT ${value.toFixed(value >= 100 ? 0 : 2)}`
  }
  try {
    return new Intl.NumberFormat(undefined, {
      style: 'currency',
      currency,
      maximumFractionDigits: value >= 100 ? 0 : 2,
    }).format(value)
  } catch {
    return `${currency} ${value.toFixed(value >= 100 ? 0 : 2)}`
  }
}
