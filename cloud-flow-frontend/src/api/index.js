import axios from 'axios'

const api = axios.create({
  baseURL: '/api/v1',
  timeout: 30000,
  headers: {
    'Content-Type': 'application/json'
  }
})

api.interceptors.response.use(
  response => response.data,
  error => {
    console.error('API Error:', error)
    return Promise.reject(error)
  }
)

export const overviewApi = {
  getStats() {
    return api.get('/overview')
  }
}

export const trafficApi = {
  getTrend(params) {
    return api.get('/traffic/trend', { params })
  },
  getProtocols() {
    return api.get('/traffic/protocols')
  },
  getTopFlows(params) {
    return api.get('/traffic/top', { params })
  },
  advancedQuery(params) {
    return api.get('/traffic/advanced', { params })
  }
}

export const networkApi = {
  getAnalysis(params) {
    return api.get('/network/analysis', { params })
  }
}

export const alertsApi = {
  getList(params) {
    return api.get('/alerts', { params })
  },
  resolve(id) {
    return api.post(`/alerts/${id}/resolve`)
  }
}

export const filtersApi = {
  getNamespaces() {
    return api.get('/filters/k8s/namespaces')
  },
  getServices(namespace) {
    return api.get('/filters/k8s/services', { params: { namespace } })
  },
  getPods(namespace, service) {
    return api.get('/filters/k8s/pods', { params: { namespace, service } })
  },
  getProtocols() {
    return api.get('/filters/protocols')
  }
}

export const exportApi = {
  exportData(params) {
    return api.get('/export/advanced', { 
      params,
      responseType: 'blob'
    })
  }
}

export const dashboardApi = {
  getList() {
    return api.get('/dashboard')
  },
  create(data) {
    return api.post('/dashboard', data)
  },
  getDetail(id) {
    return api.get(`/dashboard/${id}`)
  },
  update(id, data) {
    return api.put(`/dashboard/${id}`, data)
  },
  delete(id) {
    return api.delete(`/dashboard/${id}`)
  }
}

export default api
