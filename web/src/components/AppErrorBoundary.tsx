import { Component, type ErrorInfo, type ReactNode } from 'react'

import { adminEntryPath, appPathname, withAppBasePath } from '../lib/appBasePath'

interface State {
  failed: boolean
  message: string
}

// Keep the fallback independent of Ant Design and theme providers so it can
// still render when an error originates inside those providers.
export class AppErrorBoundary extends Component<{ children: ReactNode }, State> {
  state: State = { failed: false, message: '' }

  static getDerivedStateFromError(error: unknown): State {
    return { failed: true, message: error instanceof Error ? error.message : '发生未知页面异常' }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('[VPSMonitor] 页面渲染失败', error, info.componentStack)
  }

  render() {
    if (!this.state.failed) return this.props.children

    const path = appPathname()
    const home = path === '/customer'
      ? withAppBasePath('/customer')
      : path === '/recommendations'
        ? withAppBasePath('/recommendations')
        : `${adminEntryPath()}?page=assets`

    return (
      <main className="app-error-screen">
        <section className="app-error-card" role="alert" aria-labelledby="app-error-title">
          <svg className="app-error-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" aria-hidden="true">
            <path d="M10.3 3.5 2 18a2 2 0 0 0 1.7 3h16.6a2 2 0 0 0 1.7-3L13.7 3.5a2 2 0 0 0-3.4 0Z" />
            <path d="M12 8v5m0 3v1" />
          </svg>
          <h1 id="app-error-title">页面暂时无法显示</h1>
          <p>页面运行时发生异常，请重新加载当前页面，或返回首页后重新打开。</p>
          <div className="app-error-actions">
            <button type="button" onClick={() => window.location.reload()}>重新加载</button>
            <a href={home}>返回首页</a>
          </div>
          <details className="app-error-details">
            <summary>错误详情</summary>
            <pre>{this.state.message.slice(0, 2000)}</pre>
          </details>
        </section>
      </main>
    )
  }
}
