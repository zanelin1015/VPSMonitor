import type { AgentListItem, VPSRenewalConfig } from '../types'

export const COMMON_COST_CURRENCIES = ['USD', 'CAD', 'EUR', 'CNY', 'HKD', 'JPY', 'GBP', 'AUD', 'SGD']
export const DEFAULT_COST_CURRENCY = 'USD'
export const COST_CURRENCY_STORAGE_KEY = 'bridge-core.cost-currency'

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

function normalizeCostConfig(config?: VPSRenewalConfig): Pick<VPSRenewalConfig, 'cost_amount' | 'cost_currency' | 'cost_cycle'> {
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
  const fromRate = exchangeRates.rates[from]
  const toRate = exchangeRates.rates[to]
  if (!fromRate || !toRate) {
    return null
  }
  return (amount / fromRate) * toRate
}

export function normalizeCurrencyCode(value?: string): CurrencyCode {
  const upper = (value || DEFAULT_COST_CURRENCY).trim().toUpperCase()
  return /^[A-Z]{3}$/.test(upper) ? upper : DEFAULT_COST_CURRENCY
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
