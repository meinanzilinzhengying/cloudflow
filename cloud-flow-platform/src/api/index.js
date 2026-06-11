import axios from 'axios'

const api = axios.create({
  baseURL: '/api',
  timeout: 10000
})

api.interceptors.response.use(
  response => response.data,
  error => {
    console.error('API Error:', error)
    return Promise.reject(error)
  }
)

export default {
  // 平台概览数据 - 调用控制面 /api/stats 接口
  getPlatformStats() {
    return api.get('/control/stats')
  },

  // 系统指标 - 调用控制面 /api/system-metrics 接口
  getSystemMetrics() {
    return api.get('/control/system-metrics')
  },

  // 探针管理 - 调用控制面 /api/agents 接口
  getProbes() {
    return api.get('/control/agents')
  },

  // 健康检查 - 调用控制面 /health 接口
  getHealthStatus() {
    return api.get('/control/health')
  },

  // 配置管理 - 调用控制面 /api/configs 接口
  getConfigs() {
    return api.get('/control/configs')
  },

  // 告警列表 - 调用告警引擎 /api/alerts 接口
  getAlerts() {
    return api.get('/alert/alerts')
  },

  // Data Plane 系统指标 - 调用 data-plane /api/system-metrics 接口
  getDataPlaneMetrics() {
    return api.get('/data/system-metrics')
  }
}
