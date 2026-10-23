// @refresh reset
import { Spin } from 'antd'
import { lazy, Suspense } from 'react'
import { AdminConsoleTheme } from './components/AdminConsoleTheme'
import { isConsoleThemeRoute } from './lib/adminConsoleTheme'
import { appPathname } from './lib/appBasePath'

const AdminApp = lazy(() => import('./app/AdminApp').then((module) => ({ default: module.AdminApp })))
const CustomerPortal = lazy(() =>
  import('./components/CustomerPortal').then((module) => ({ default: module.CustomerPortal })),
)
const PublicSite = lazy(() => import('./components/PublicSite').then((module) => ({ default: module.PublicSite })))
const RecommendationsPage = lazy(() =>
  import('./pages/admin/RecommendationsPage').then((module) => ({ default: module.RecommendationsPage })),
)

export default function App() {
  const pathname = appPathname()
  const publicMode =
    pathname === '/site' ||
    pathname === '/official' ||
    new URLSearchParams(window.location.search).get('page') === 'site'
  const recommendationMode = pathname === '/recommendations'
  const content = (
    <Suspense
      fallback={
        <div className="login-shell">
          <Spin size="large" />
        </div>
      }
    >
      {recommendationMode ? <RecommendationsPage publicPage /> : publicMode ? <PublicSite /> : pathname === '/customer' ? <CustomerPortal /> : <AdminApp />}
    </Suspense>
  )
  return isConsoleThemeRoute(pathname, window.location.search) ? (
    <AdminConsoleTheme customer={pathname === '/customer'}>{content}</AdminConsoleTheme>
  ) : (
    content
  )
}
