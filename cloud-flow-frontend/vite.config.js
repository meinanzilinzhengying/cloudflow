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
      '/api/auth': {
        target: 'http://localhost:8006',
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/api\/auth/, '')
      },
      // Tenant Service
      '/api/tenants': {
        target: 'http://localhost:8010',
        changeOrigin: true
      },
      '/api/projects': {
        target: 'http://localhost:8010',
        changeOrigin: true
      },
      '/api/quotas': {
        target: 'http://localhost:8010',
        changeOrigin: true
      },
      // Control Plane
      '/api/agents': {
        target: 'http://localhost:8001',
        changeOrigin: true
      },
      '/api/edges': {
        target: 'http://localhost:8001',
        changeOrigin: true
      },
      // Query Service
      '/api/overview': {
        target: 'http://localhost:8007',
        changeOrigin: true
      },
      '/api/metrics': {
        target: 'http://localhost:8007',
        changeOrigin: true
      },
      '/api/flows': {
        target: 'http://localhost:8007',
        changeOrigin: true
      },
      '/api/traces': {
        target: 'http://localhost:8007',
        changeOrigin: true
      },
      '/api/topology': {
        target: 'http://localhost:8007',
        changeOrigin: true
      },
      '/api/alerts': {
        target: 'http://localhost:8007',
        changeOrigin: true
      },
      '/api/otel': {
        target: 'http://localhost:8007',
        changeOrigin: true
      },
      '/api/rca': {
        target: 'http://localhost:8007',
        changeOrigin: true
      },
      '/api/correlation': {
        target: 'http://localhost:8007',
        changeOrigin: true
      },
      // Alert Service
      '/api/rules': {
        target: 'http://localhost:8009',
        changeOrigin: true
      }
    }
  }
})
