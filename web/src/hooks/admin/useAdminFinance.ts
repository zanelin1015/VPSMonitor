import type { Dispatch, SetStateAction } from 'react'
import { useEffect, useState } from 'react'
import { fetchJSON, isUnauthorized } from '../../lib/appHelpers'
import {
  COMMON_COST_CURRENCIES,
  COST_CURRENCY_STORAGE_KEY,
  defaultExchangeRatesState,
  mergeCurrencyOptions,
  normalizeCurrencyCode,
  readStoredCostCurrency,
  type CurrencyCode,
  type ExchangeRatesState,
} from '../../lib/currency'
import type { AdminUser, AreaManagerAdminView, CustomerAdminView, ExchangeRatesResponse } from '../../types'

function exchangeRateNotice(data: ExchangeRatesResponse): string {
  if (!data.stale) {
    return ''
  }
  const source = String(data.source || '').toLowerCase()
  if (source.includes('fallback')) {
    return '汇率接口暂不可用，已使用兜底汇率'
  }
  return '汇率接口暂不可用，已使用缓存汇率'
}

interface Options {
  adminUser: AdminUser | null
  setAdminUser: Dispatch<SetStateAction<AdminUser | null>>
}

export function useAdminFinance({ adminUser, setAdminUser }: Options) {
  const [financeCustomers, setFinanceCustomers] = useState<CustomerAdminView[]>([])

  const [financeAreaManagers, setFinanceAreaManagers] = useState<AreaManagerAdminView[]>([])

  const [financeAccountsLoaded, setFinanceAccountsLoaded] = useState(false)

  const [financeAccountsError, setFinanceAccountsError] = useState('')

  const [costCurrency, setCostCurrency] = useState<CurrencyCode>(() => readStoredCostCurrency())

  const [exchangeRates, setExchangeRates] = useState<ExchangeRatesState>(() => defaultExchangeRatesState())

  const [currencyOptions, setCurrencyOptions] = useState<CurrencyCode[]>(() => [...COMMON_COST_CURRENCIES])

  useEffect(() => {
    setFinanceAccountsLoaded(false)
    setFinanceAccountsError('')
    if (!adminUser) {
      setFinanceCustomers([])
      setFinanceAreaManagers([])
    }
  }, [adminUser?.id, adminUser?.username])

  useEffect(() => {
    try {
      window.localStorage.setItem(COST_CURRENCY_STORAGE_KEY, costCurrency)
    } catch {
      // Ignore storage errors; the selector still works for the current session.
    }
  }, [costCurrency])

  async function loadExchangeRates() {
    setExchangeRates((current) => ({ ...current, loading: true, error: '' }))
    try {
      const data = await fetchJSON<ExchangeRatesResponse>('/api/v1/exchange-rates')
      const rates: Record<CurrencyCode, number> = { EUR: 1 }
      for (const [rawCurrency, rawRate] of Object.entries(data.rates || {})) {
        const currency = normalizeCurrencyCode(rawCurrency)
        const value = Number(rawRate)
        if (Number.isFinite(value) && value > 0) {
          rates[currency] = value
        }
      }
      setCurrencyOptions(mergeCurrencyOptions(Object.keys(rates)))
      setExchangeRates({
        base: normalizeCurrencyCode(data.base || 'EUR'),
        date: data.date || '',
        rates,
        loading: false,
        error: exchangeRateNotice(data),
      })
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      setExchangeRates((current) => ({
        ...current,
        loading: false,
        error: '汇率接口暂不可用，已保留当前汇率',
      }))
    }
  }

  async function loadFinanceAccounts() {
    try {
      const [customers, areaManagers] = await Promise.all([
        fetchJSON<CustomerAdminView[]>('/api/v1/admin/customers'),
        fetchJSON<AreaManagerAdminView[]>('/api/v1/admin/area-managers'),
      ])
      setFinanceCustomers(Array.isArray(customers) ? customers : [])
      setFinanceAreaManagers(Array.isArray(areaManagers) ? areaManagers : [])
      setFinanceAccountsLoaded(true)
      setFinanceAccountsError('')
    } catch (error) {
      setFinanceAccountsError(error instanceof Error ? error.message : '财务账号数据加载失败')
    }
  }

  return {
    financeCustomers,
    setFinanceCustomers,
    financeAreaManagers,
    setFinanceAreaManagers,
    financeAccountsLoaded,
    setFinanceAccountsLoaded,
    financeAccountsError,
    setFinanceAccountsError,
    costCurrency,
    setCostCurrency,
    exchangeRates,
    setExchangeRates,
    currencyOptions,
    setCurrencyOptions,
    loadExchangeRates,
    loadFinanceAccounts,
  }
}
