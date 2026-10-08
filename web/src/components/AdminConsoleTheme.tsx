import { useEffect, useState, type ReactNode } from 'react'
import { App as AntApp, ConfigProvider } from 'antd'
import { useAppTheme } from '../theme'
import { adminConsoleTheme } from '../lib/adminConsoleTheme'

export function AdminConsoleTheme({ children, customer = false }: { children: ReactNode; customer?: boolean }) {
  const { effectiveMode } = useAppTheme()
  const [reducedMotion, setReducedMotion] = useState(() => Boolean(window.matchMedia?.('(prefers-reduced-motion: reduce)').matches))

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

  return (
    <ConfigProvider
      theme={adminConsoleTheme(effectiveMode === 'dark', reducedMotion)}
      variant="filled"
      button={{ autoInsertSpace: false }}
      modal={{ className: 'admin-console-modal' }}
      dropdown={{ className: 'admin-console-dropdown' }}
    >
      <AntApp className={customer ? 'admin-console-theme customer-console-theme' : 'admin-console-theme'}>{children}</AntApp>
    </ConfigProvider>
  )
}
