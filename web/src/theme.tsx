import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { App as AntApp, ConfigProvider, theme } from 'antd'

export type ThemeMode = 'system' | 'light' | 'dark'
export type EffectiveThemeMode = 'light' | 'dark'

interface AppThemeContextValue {
  mode: ThemeMode
  effectiveMode: EffectiveThemeMode
  setMode: (mode: ThemeMode) => void
}

const THEME_STORAGE_KEY = 'vpsmonitor.theme-mode'
const AppThemeContext = createContext<AppThemeContextValue | null>(null)

function readStoredThemeMode(): ThemeMode {
  try {
    const value = window.localStorage.getItem(THEME_STORAGE_KEY)
    return value === 'light' || value === 'dark' || value === 'system' ? value : 'system'
  } catch {
    return 'system'
  }
}

function prefersDarkMode() {
  return Boolean(window.matchMedia?.('(prefers-color-scheme: dark)').matches)
}

export function AppThemeProvider(props: { children: ReactNode }) {
  const [mode, setModeState] = useState<ThemeMode>(() => readStoredThemeMode())
  const [systemDark, setSystemDark] = useState(() => prefersDarkMode())
  const effectiveMode: EffectiveThemeMode = mode === 'system' ? (systemDark ? 'dark' : 'light') : mode

  useEffect(() => {
    const media = window.matchMedia?.('(prefers-color-scheme: dark)')
    if (!media) {
      return
    }
    const handleChange = () => setSystemDark(media.matches)
    handleChange()
    media.addEventListener?.('change', handleChange)
    return () => media.removeEventListener?.('change', handleChange)
  }, [])

  useEffect(() => {
    document.documentElement.classList.toggle('dark', effectiveMode === 'dark')
    document.documentElement.dataset.theme = effectiveMode
  }, [effectiveMode])

  const setMode = (nextMode: ThemeMode) => {
    setModeState(nextMode)
    try {
      window.localStorage.setItem(THEME_STORAGE_KEY, nextMode)
    } catch {
      // The current tab can still switch theme if storage is unavailable.
    }
  }

  const contextValue = useMemo(() => ({ mode, effectiveMode, setMode }), [mode, effectiveMode])
  const isDark = effectiveMode === 'dark'

  return (
    <AppThemeContext.Provider value={contextValue}>
      <ConfigProvider
        theme={{
          algorithm: isDark ? theme.darkAlgorithm : theme.defaultAlgorithm,
          token: {
            colorPrimary: '#2563eb',
            colorInfo: '#2563eb',
            colorSuccess: '#2563eb',
            colorWarning: '#f59e0b',
            colorError: '#ef4444',
            colorBgLayout: isDark ? '#07111f' : '#fafafa',
            colorBgContainer: isDark ? '#101827' : '#ffffff',
            colorText: isDark ? '#f5f5f5' : '#171717',
            colorTextSecondary: isDark ? '#a9bbb4' : '#6b7280',
            colorBorder: isDark ? 'rgba(255,255,255,0.14)' : '#e5e5e5',
            borderRadius: 8,
            fontFamily:
              '"IBM Plex Sans", "Avenir Next", "Segoe UI Variable", "PingFang SC", "Hiragino Sans GB", "Noto Sans SC", sans-serif',
          },
        }}
      >
        <AntApp>{props.children}</AntApp>
      </ConfigProvider>
    </AppThemeContext.Provider>
  )
}

export function useAppTheme() {
  const value = useContext(AppThemeContext)
  if (!value) {
    throw new Error('useAppTheme must be used inside AppThemeProvider')
  }
  return value
}
