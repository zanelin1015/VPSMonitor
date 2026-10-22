import { useEffect, useState, type ReactNode } from 'react'
import { App as AntApp, ConfigProvider } from 'antd'
import zhCN from 'antd/locale/zh_CN'
import { useAppTheme } from '../theme'
import { adminConsoleTheme, consoleCSSVariables } from '../lib/adminConsoleTheme'

export function AdminConsoleTheme({ children, customer = false }: { children: ReactNode; customer?: boolean }) {
  const { effectiveMode } = useAppTheme()
  const [reducedMotion, setReducedMotion] = useState(() =>
    Boolean(window.matchMedia?.('(prefers-reduced-motion: reduce)').matches),
  )

  useEffect(() => {
    document.documentElement.dataset.consoleTheme = 'soft'
    const media = window.matchMedia?.('(prefers-reduced-motion: reduce)')
    const update = () => setReducedMotion(Boolean(media?.matches))
    update()
    media?.addEventListener?.('change', update)
    return () => {
      delete document.documentElement.dataset.consoleTheme
      media?.removeEventListener?.('change', update)
    }
  }, [])

  useEffect(() => {
    const root = document.documentElement
    const variables = consoleCSSVariables(effectiveMode === 'dark')
    const previous = Object.fromEntries(Object.keys(variables).map((key) => [key, root.style.getPropertyValue(key)]))
    for (const [key, value] of Object.entries(variables)) root.style.setProperty(key, value)
    return () => {
      for (const [key, value] of Object.entries(previous)) {
        if (value) root.style.setProperty(key, value)
        else root.style.removeProperty(key)
      }
    }
  }, [effectiveMode])

  return (
    <ConfigProvider
      locale={zhCN}
      theme={adminConsoleTheme(effectiveMode === 'dark', reducedMotion)}
      variant="filled"
      button={{ autoInsertSpace: false }}
      modal={{ className: 'admin-console-modal' }}
      dropdown={{ className: 'admin-console-dropdown' }}
    >
      <AntApp className={customer ? 'admin-console-theme customer-console-theme' : 'admin-console-theme'}>
        {children}
      </AntApp>
    </ConfigProvider>
  )
}
