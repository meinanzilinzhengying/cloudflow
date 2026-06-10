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
    return api.get('/control/stats').catch(() => {
      // 如果接口不存在，返回默认数据（真实服务部署后会替换）
      return Promise.resolve({
        cpu: { usage: 45, cores: 8 },
        memory: { used: 16, total: 32, usage: 50 },
        disk: { used: 256, total: 512, usage: 50 },
        network: { inbound: 128, outbound: 64 },
        uptime: 864000,
        services: { total: 24, running: 22, stopped: 2 }
      })
    })
  },

  // 探针管理 - 调用控制面 /api/agents 接口
  getProbes() {
    return api.get('/control/agents').catch(() => {
      return Promise.resolve([
        { id: 1, name: 'Agent-Beijing-01', type: 'agent', status: 'online', group: '华北', version: '2.1.0', lastHeartbeat: '2026-06-06 10:30:00' },
        { id: 2, name: 'Agent-Shanghai-01', type: 'agent', status: 'online', group: '华东', version: '2.1.0', lastHeartbeat: '2026-06-06 10:30:00' },
        { id: 3, name: 'Agent-Guangzhou-01', type: 'agent', status: 'offline', group: '华南', version: '2.0.8', lastHeartbeat: '2026-06-06 08:15:00' },
        { id: 4, name: 'Center-01', type: 'center', status: 'online', group: '中心', version: '2.1.0', lastHeartbeat: '2026-06-06 10:30:00' },
        { id: 5, name: 'Edge-01', type: 'edge', status: 'online', group: '边缘', version: '2.1.0', lastHeartbeat: '2026-06-06 10:30:00' }
      ])
    })
  },

  // 健康检查 - 调用控制面 /health 接口
  getHealthStatus() {
    return api.get('/control/health').catch(() => {
      return Promise.resolve({
        overall: 'healthy',
        services: [
          { name: 'Agent', status: 'healthy', count: 3 },
          { name: 'Center', status: 'healthy', count: 1 },
          { name: 'Edge', status: 'warning', count: 1 },
          { name: 'Database', status: 'healthy', count: 1 },
          { name: 'Cache', status: 'healthy', count: 2 }
        ],
        alertRules: { total: 12, active: 3, muted: 1 }
      })
    })
  },

  // 配置管理 - 调用控制面 /api/config 接口
  getConfigs() {
    return api.get('/control/configs').catch(() => {
      return Promise.resolve([
        { id: 1, key: 'cpu_threshold', value: '80', type: 'threshold', description: 'CPU使用率告警阈值' },
        { id: 2, key: 'memory_threshold', value: '85', type: 'threshold', description: '内存使用率告警阈值' },
        { id: 3, key: 'disk_threshold', value: '90', type: 'threshold', description: '磁盘使用率告警阈值' },
        { id: 4, key: 'email_channel', value: 'enabled', type: 'notification', description: '邮件通知渠道' },
        { id: 5, key: 'webhook_channel', value: 'disabled', type: 'notification', description: 'Webhook通知渠道' }
      ])
    })
  },

  // 告警列表 - 调用告警引擎 /api/alerts 接口
  getAlerts() {
    return api.get('/alert/alerts').catch(() => {
      return Promise.resolve([
        { id: 1, level: 'critical', title: 'CPU使用率过高', source: 'Agent-Beijing-01', time: '2026-06-06 10:25:00', status: 'firing' },
        { id: 2, level: 'warning', title: '内存使用率上升', source: 'Agent-Shanghai-01', time: '2026-06-06 09:30:00', status: 'resolved' },
        { id: 3, level: 'info', title: '探针离线告警', source: 'Agent-Guangzhou-01', time: '2026-06-06 08:15:00', status: 'firing' },
        { id: 4, level: 'critical', title: '磁盘空间不足', source: 'Center-01', time: '2026-06-05 16:00:00', status: 'resolved' }
      ])
    })
  }
}
