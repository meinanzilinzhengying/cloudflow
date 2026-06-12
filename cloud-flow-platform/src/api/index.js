import axios from 'axios'

const api = axios.create({
  baseURL: '/api',
  timeout: 10000
})

api.interceptors.response.use(
  response => response.data,
  error => {
    console.error('API Error:', error.config?.url, error.response?.status)
    // 不抛出错误，返回 null 让调用方自行处理
    return Promise.resolve(null)
  }
)

export default {
  getPlatformStats() {
    return api.get('/control/stats').catch(() => null)
  },

  getSystemMetrics() {
    return api.get('/control/system-metrics').catch(() => null)
  },

  getProbes() {
    return api.get('/control/processes').catch(() => null)
  },

  getHealthStatus() {
    return api.get('/control/health').catch(() => null)
  },

  getConfigs() {
    return api.get('/control/configs').catch(() => null)
  },
  createConfig(data) {
    return api.post('/control/configs', data).catch(() => null)
  },
  updateConfig(data) {
    return api.put('/control/configs', data).catch(() => null)
  },
  deleteConfig(key) {
    return api.delete(`/control/configs?key=${encodeURIComponent(key)}`).catch(() => null)
  },

  getAlerts() {
    return api.get('/alert/alerts').catch(() => null)
  },

  getDataPlaneMetrics() {
    return api.get('/data/system-metrics').catch(() => null)
  }
}
