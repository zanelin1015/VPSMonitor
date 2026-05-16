import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

const apiTarget = (globalThis as { process?: { env?: Record<string, string | undefined> } }).process?.env?.VPSMONITOR_API_TARGET || 'http://127.0.0.1:8090'

export default defineConfig({
  plugins: [react()],
  server: {
    host: '0.0.0.0',
    port: 5173,
    proxy: {
      '/api': {
        target: apiTarget,
        changeOrigin: true,
      },
      '/healthz': {
        target: apiTarget,
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: '../webui/dist',
    emptyOutDir: true,
    rollupOptions: {
      output: {
        manualChunks: {
          react: ['react', 'react-dom'],
          antd: ['antd', '@ant-design/icons'],
        },
      },
    },
  },
})
