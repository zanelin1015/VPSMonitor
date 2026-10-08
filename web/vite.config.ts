import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

const apiTarget = (globalThis as { process?: { env?: Record<string, string | undefined> } }).process?.env?.VPSMONITOR_API_TARGET || 'http://127.0.0.1:8090'
const apiOrigin = new URL(apiTarget).origin

export default defineConfig({
  base: './',
  plugins: [react()],
  server: {
    host: '0.0.0.0',
    port: 5173,
    proxy: {
      '/api': {
        target: apiTarget,
        changeOrigin: true,
        ws: true,
        // The local UI uses /api; deployed sessions may be scoped to /zanelin.
        // Keep HttpOnly/Secure/SameSite intact and use localhost for HTTPS cookies.
        cookieDomainRewrite: '',
        cookiePathRewrite: '/',
        headers: { Origin: apiOrigin },
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
