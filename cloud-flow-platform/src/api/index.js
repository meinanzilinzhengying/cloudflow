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
    return api.get('/control/alerts').catch(() => null)
  },

  getDataPlaneMetrics() {
    return api.get('/data/system-metrics').catch(() => null)
  },

  // ========== 探针管理 API (agent-manager :8099) ==========
  getAgents() {
    return api.get('/agents').catch(() => null)
  },

  getAgentDetail(id) {
    return api.get(`/agents/${id}`).catch(() => null)
  },

  agentHeartbeat(data) {
    return api.post('/agent/heartbeat', data).catch(() => null)
  },

  updateAgentConfig(id, config) {
    return api.put(`/agents/${id}/config`, config).catch(() => null)
  },

  installAgentLocal(data) {
    return api.post('/agents/install', data).catch(() => null)
  },

  installAgentSSH(data) {
    return api.post('/agents/ssh-install', data).catch(() => null)
  },

  uninstallAgent(id) {
    return api.delete(`/agents/${id}`).catch(() => null)
  },
  // ========= eBPF 探针管理 (VM2 :9090) =========
  getEBPFStatus() {
    return fetch('http://192.168.58.131:9090/api/probe/status').then(r => r.json()).catch(() => null)
  },

  controlEBPF(action) {
    return fetch(`http://192.168.58.131:9090/api/probe/${action}`, { method: 'POST' })
      .then(r => r.json())
      .catch(() => null)
  }
}
