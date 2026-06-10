import axios from 'axios'

// 所有请求通过 nginx 反向代理转发到对应后端服务
// nginx 代理规则：
//   /api/     -> query-service:8007/api/
//   /auth/    -> auth-service:8006/
//   /tenant/  -> tenant-service:8010/
//   /control/ -> control-plane:8001/
//   /alert/   -> alert-engine:8009/

const createApiClient = (baseURL) => {
  const client = axios.create({
    baseURL,
    timeout: 10000,
    headers: {
      'Content-Type': 'application/json',
    },
  })

  // Request interceptor to add auth token
  client.interceptors.request.use(
    (config) => {
      const token = localStorage.getItem('cloudflow_token')
      if (token) {
        config.headers.Authorization = `Bearer ${token}`
      }
      const tenantId = localStorage.getItem('cloudflow_tenant_id')
      if (tenantId) {
        config.headers['X-Tenant-Id'] = tenantId
      }
      return config
    },
    (error) => Promise.reject(error)
  )

  // Response interceptor for error handling
  client.interceptors.response.use(
    (response) => response.data,
    (error) => {
      if (error.response?.status === 401) {
        localStorage.removeItem('cloudflow_token')
        window.location.href = '/login'
      }
      return Promise.reject(error)
    }
  )

  return client
}

// 各服务 axios 实例（使用相对路径，由 nginx 代理到对应后端）
const authApi = createApiClient('/auth')
const tenantApi = createApiClient('/tenant')
const controlPlaneApi = createApiClient('/control')
const queryApi = createApiClient('/api')
const alertApi = createApiClient('/alert')

// Auth Service APIs
export const authService = {
  login: (username, password) =>
    authApi.post('/api/login', { username, password }),

  verify: (token) =>
    authApi.post('/api/verify', { token }),

  refresh: (token) =>
    authApi.post('/api/refresh', { token }),

  revoke: (token) =>
    authApi.post('/api/revoke', { token }),

  getUsers: () =>
    authApi.get('/api/users'),

  createUser: (userData) =>
    authApi.post('/api/users/create', userData),

  updateUser: (userData) =>
    authApi.post('/api/users/update', userData),

  deleteUser: (username) =>
    authApi.delete('/api/users/delete', { data: { username } }),

  getRoles: () =>
    authApi.get('/api/roles'),

  checkPermission: (action, resource) =>
    authApi.post('/api/permissions/check', { action, resource }),
}

// Tenant Service APIs
export const tenantService = {
  getTenants: () =>
    tenantApi.get('/api/tenants'),

  createTenant: (tenantData) =>
    tenantApi.post('/api/tenants/create', tenantData),

  updateTenant: (tenantData) =>
    tenantApi.post('/api/tenants/update', tenantData),

  deleteTenant: (tenantId) =>
    tenantApi.post('/api/tenants/delete', { tenant_id: tenantId }),

  getProjects: () =>
    tenantApi.get('/api/projects'),

  getQuotas: () =>
    tenantApi.get('/api/quotas'),
}

// Control Plane APIs
export const controlPlaneService = {
  getAgents: () =>
    controlPlaneApi.get('/api/agents'),

  getEdges: () =>
    controlPlaneApi.get('/api/edges'),
}

// Query Service APIs
export const queryService = {
  getOverview: () =>
    queryApi.get('/overview'),

  getMetrics: (params) =>
    queryApi.get('/metrics', { params }),

  getFlows: (params) =>
    queryApi.get('/flows', { params }),

  getTraces: (params) =>
    queryApi.get('/traces', { params }),

  getTopology: (params) =>
    queryApi.get('/topology', { params }),

  getAlerts: (params) =>
    queryApi.get('/alerts', { params }),

  getOTELTraces: (params) =>
    queryApi.get('/otel/traces', { params }),

  getOTELMetrics: (params) =>
    queryApi.get('/otel/metrics', { params }),

  getOTELLogs: (params) =>
    queryApi.get('/otel/logs', { params }),

  getOTELStats: () =>
    queryApi.get('/otel/stats'),

  getRCA: (params) =>
    queryApi.get('/rca', { params }),

  getCorrelation: (params) =>
    queryApi.get('/correlation', { params }),
}

// Alert Engine APIs
export const alertService = {
  getAlerts: (params) =>
    alertApi.get('/api/alerts', { params }),

  createAlert: (alertData) =>
    alertApi.post('/api/alerts/create', alertData),

  updateAlert: (alertData) =>
    alertApi.post('/api/alerts/update', alertData),

  resolveAlert: (alertId) =>
    alertApi.post('/api/alerts/resolve', { alert_id: alertId }),

  getRules: () =>
    alertApi.get('/api/rules'),

  createRule: (ruleData) =>
    alertApi.post('/api/rules/create', ruleData),

  updateRule: (ruleData) =>
    alertApi.post('/api/rules/update', ruleData),

  deleteRule: (ruleId) =>
    alertApi.delete('/api/rules/delete', { data: { rule_id: ruleId } }),
}

// Health check for all services
export const healthCheck = async () => {
  const services = [
    { name: 'auth', url: '/auth/healthz' },
    { name: 'tenant', url: '/tenant/healthz' },
    { name: 'control-plane', url: '/control/healthz' },
    { name: 'query', url: '/api/healthz' },
    { name: 'alert', url: '/alert/healthz' },
  ]

  const results = await Promise.allSettled(
    services.map(async (svc) => {
      try {
        const response = await axios.get(svc.url, { timeout: 3000 })
        return { ...svc, status: 'healthy', data: response.data }
      } catch {
        return { ...svc, status: 'unhealthy', data: null }
      }
    })
  )

  return results.map((r) => r.value)
}

export default {
  authService,
  tenantService,
  controlPlaneService,
  queryService,
  alertService,
  healthCheck,
}
