import React from 'react'
import ReactDOM from 'react-dom/client'
import { App as AntApp, ConfigProvider, theme } from 'antd'
import 'antd/dist/reset.css'

import App from './App'
import './styles.css'

const prefersDark = window.matchMedia?.('(prefers-color-scheme: dark)').matches
const algorithm = prefersDark ? theme.darkAlgorithm : theme.defaultAlgorithm
document.documentElement.classList.toggle('dark', prefersDark)

ReactDOM.createRoot(document.getElementById('root') as HTMLElement).render(
  <React.StrictMode>
    <ConfigProvider
      theme={{
        algorithm,
        token: {
          colorPrimary: '#16a34a',
          colorInfo: '#0284c7',
          colorSuccess: '#16a34a',
          colorWarning: '#f59e0b',
          colorError: '#ef4444',
          colorBgLayout: prefersDark ? '#171717' : '#fafafa',
          colorBgContainer: prefersDark ? '#1f1f1f' : '#ffffff',
          colorText: prefersDark ? '#f5f5f5' : '#171717',
          colorBorder: prefersDark ? '#333333' : '#e5e5e5',
          borderRadius: 8,
          fontFamily:
            '"IBM Plex Sans", "Avenir Next", "Segoe UI Variable", "PingFang SC", "Hiragino Sans GB", "Noto Sans SC", sans-serif',
        },
      }}
    >
      <AntApp>
        <App />
      </AntApp>
    </ConfigProvider>
  </React.StrictMode>,
)
