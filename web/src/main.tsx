import React from 'react'
import ReactDOM from 'react-dom/client'
import 'antd/dist/reset.css'

import App from './App'
import { AppErrorBoundary } from './components/AppErrorBoundary'
import { AppThemeProvider } from './theme'
import './styles.css'

ReactDOM.createRoot(document.getElementById('root') as HTMLElement).render(
  <React.StrictMode>
    <AppErrorBoundary>
      <AppThemeProvider>
        <App />
      </AppThemeProvider>
    </AppErrorBoundary>
  </React.StrictMode>,
)
