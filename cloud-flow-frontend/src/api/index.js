import axios from 'axios'

// API Base URLs - these would be configured via environment variables in production
const API_BASE = {
  auth: '/api/auth',
  tenant: '/api/tenant',
  controlPlane: '/api/control',
  query: '/api/query',
  alert: '/api/alert',
  dataPlane: '/api/dataplane',
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
  // NOTE: Do NOT redirect on 401 - this SPA has no separate login page.
  // The login is handled inline by components. Redirecting causes infinite reload loops.
  client.interceptors.response.use(
    (response) => response.data,
    (error) => {
      if (error.response?.status === 401) {
        console.warn('[API] Authentication expired or invalid for:', error.config?.url)
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
    authApi.post('/login', { username, password }),

  verify: (token) =>
    authApi.post('/verify', { token }),

  refresh: (token) =>
    authApi.post('/refresh', { token }),

  revoke: (token) =>
    authApi.post('/revoke', { token }),

  getUsers: () =>
    authApi.get('/users'),

  createUser: (userData) =>
    authApi.post('/users/create', userData),

  updateUser: (userData) =>
    authApi.post('/users/update', userData),

  deleteUser: (username) =>
    authApi.delete('/users/delete', { data: { username } }),

  getRoles: () =>
    authApi.get('/roles'),

  checkPermission: (action, resource) =>
    authApi.post('/permissions/check', { action, resource }),
}

// Tenant Service APIs
export const tenantService = {
  getTenants: () =>
    tenantApi.get('/tenants'),

  createTenant: (tenantData) =>
    tenantApi.post('/tenants/create', tenantData),

  updateTenant: (tenantData) =>
    tenantApi.post('/tenants/update', tenantData),

  deleteTenant: (tenantId) =>
    tenantApi.post('/tenants/delete', { tenant_id: tenantId }),

  getProjects: () =>
    tenantApi.get('/projects'),

  getQuotas: () =>
    tenantApi.get('/quotas'),
}

// Control Plane APIs
export const controlPlaneService = {
  getAgents: () =>
    controlPlaneApi.get('/agents'),

  getEdges: () =>
    controlPlaneApi.get('/edges'),
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

// Alert Service APIs
export const alertService = {
  getAlerts: (params) =>
    alertApi.get('/alerts', { params }),

  createAlert: (alertData) =>
    alertApi.post('/alerts/create', alertData),

  updateAlert: (alertData) =>
    alertApi.post('/alerts/update', alertData),

  resolveAlert: (alertId) =>
    alertApi.post('/alerts/resolve', { alert_id: alertId }),

  getRules: () =>
    alertApi.get('/rules'),

  createRule: (ruleData) =>
    alertApi.post('/rules/create', ruleData),

  updateRule: (ruleData) =>
    alertApi.post('/rules/update', ruleData),

  deleteRule: (ruleId) =>
    alertApi.delete('/rules/delete', { data: { rule_id: ruleId } }),
}

// Data Plane APIs
export const dataPlaneService = {
  getHealth: () =>
    dataPlaneApi.get('/health'),

  getMetrics: () =>
    dataPlaneApi.get('/metrics'),

  getSamplingConfig: () =>
    dataPlaneApi.get('/sampling/config'),

  getSamplingStats: () =>
    dataPlaneApi.get('/sampling/stats'),
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
