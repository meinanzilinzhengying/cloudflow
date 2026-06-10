import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': '/src'
    }
  },
  server: {
    port: 5173,
    proxy: {
      // Auth Service
      '/auth': {
        target: 'http://localhost:8006',
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/auth/, '')
      },
      // Tenant Service
      '/tenant': {
        target: 'http://localhost:8010',
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/tenant/, '')
      },
      // Control Plane
      '/control': {
        target: 'http://localhost:8001',
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/control/, '')
      },
      // Query Service（/api/ 前缀，后端路由本身就是 /api/*）
      '/api': {
        target: 'http://localhost:8007',
        changeOrigin: true
      },
      // Alert Engine
      '/alert': {
        target: 'http://localhost:8009',
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/alert/, '')
      }
    }
  }
})
