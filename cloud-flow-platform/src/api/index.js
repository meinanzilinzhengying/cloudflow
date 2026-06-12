import axios from 'axios'

const api = axios.create({
  baseURL: '/api',
  timeout: 10000
})

api.interceptors.response.use(
  response => response.data,
  error => {
    console.error('API Error:', error.config?.url, error.response?.status)
    return Promise.resolve(null)
  }
)

// Loki API client
const lokiApi = axios.create({
  baseURL: '/api/loki',
  timeout: 30000
})

lokiApi.interceptors.response.use(
  response => response.data,
  error => {
    console.error('Loki API Error:', error.config?.url, error.response?.status)
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
    return api.get('/control/alerts').catch(() => null)
  },

  getDataPlaneMetrics() {
    return api.get('/data/system-metrics').catch(() => null)
  },

  // Loki 日志查询
  getLogs({ query = '{service=~".+"}', start, end, limit = 100, direction = 'backward' } = {}) {
    const params = new URLSearchParams()
    params.append('query', query)
    if (start) params.append('start', start)
    if (end) params.append('end', end)
    params.append('limit', String(limit))
    params.append('direction', direction)
    return lokiApi.get(`/loki/api/v1/query_range?${params.toString()}`).catch(() => null)
  },

  // 获取 Loki 标签列表
  getLogLabels() {
    return lokiApi.get('/loki/api/v1/label/job/values').catch(() => null)
  },

  // 获取服务名称列表
  getLogServices() {
    return lokiApi.get('/loki/api/v1/label/service/values').catch(() => null)
  }
}
