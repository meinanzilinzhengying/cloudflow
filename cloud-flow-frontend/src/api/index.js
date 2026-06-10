import axios from 'axios'

// API Base URLs - these would be configured via environment variables in production
const API_BASE = {
  auth: 'http://localhost:8006',
  tenant: 'http://localhost:8010',
  controlPlane: 'http://localhost:8001',
  query: 'http://localhost:8007',
  alert: 'http://localhost:8009',
  dataPlane: 'http://localhost:9102',
}

// Create axios instances for each service
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

const authApi = createApiClient(API_BASE.auth)
const tenantApi = createApiClient(API_BASE.tenant)
const controlPlaneApi = createApiClient(API_BASE.controlPlane)
const queryApi = createApiClient(API_BASE.query)
const alertApi = createApiClient(API_BASE.alert)
const dataPlaneApi = createApiClient(API_BASE.dataPlane)

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
    queryApi.get('/api/overview'),

  getMetrics: (params) =>
    queryApi.get('/api/metrics', { params }),

  getFlows: (params) =>
    queryApi.get('/api/flows', { params }),

  getTraces: (params) =>
    queryApi.get('/api/traces', { params }),

  getTopology: (params) =>
    queryApi.get('/api/topology', { params }),

  getAlerts: (params) =>
    queryApi.get('/api/alerts', { params }),

  getOTELTraces: (params) =>
    queryApi.get('/api/otel/traces', { params }),

  getOTELMetrics: (params) =>
    queryApi.get('/api/otel/metrics', { params }),

  getOTELLogs: (params) =>
    queryApi.get('/api/otel/logs', { params }),

  getOTELStats: () =>
    queryApi.get('/api/otel/stats'),

  getRCA: (params) =>
    queryApi.get('/api/rca', { params }),

  getCorrelation: (params) =>
    queryApi.get('/api/correlation', { params }),
}

// Alert Service APIs
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

// Data Plane APIs
export const dataPlaneService = {
  getHealth: () =>
    dataPlaneApi.get('/health'),

  getMetrics: () =>
    dataPlaneApi.get('/metrics'),

  getSamplingConfig: () =>
    dataPlaneApi.get('/api/sampling/config'),

  getSamplingStats: () =>
    dataPlaneApi.get('/api/sampling/stats'),
}

// Health check for all services
export const healthCheck = async () => {
  const services = [
    { name: 'auth', url: `${API_BASE.auth}/healthz` },
    { name: 'tenant', url: `${API_BASE.tenant}/healthz` },
    { name: 'control-plane', url: `${API_BASE.controlPlane}/healthz` },
    { name: 'query', url: `${API_BASE.query}/healthz` },
    { name: 'alert', url: `${API_BASE.alert}/healthz` },
    { name: 'data-plane', url: `${API_BASE.dataPlane}/health` },
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
  dataPlaneService,
  healthCheck,
}
