import type { ThemeMode } from '../theme'

export const customerThemeLabels: Record<ThemeMode, string> = {
  system: '跟随系统',
  light: '明亮',
  dark: '暗黑',
}

export function nextCustomerTheme(mode: ThemeMode): ThemeMode {
  return mode === 'system' ? 'light' : mode === 'light' ? 'dark' : 'system'
}
