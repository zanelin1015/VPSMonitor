export const FINANCE_VISIBILITY_STORAGE_KEY = 'bridge-core.profit-visible'

export function readFinanceVisibilityPreference(): boolean {
  try {
    return window.localStorage.getItem(FINANCE_VISIBILITY_STORAGE_KEY) !== 'false'
  } catch {
    return true
  }
}

export function writeFinanceVisibilityPreference(visible: boolean): void {
  try {
    window.localStorage.setItem(FINANCE_VISIBILITY_STORAGE_KEY, String(visible))
  } catch {
    // Visibility preference is optional; the current page still toggles.
  }
}
