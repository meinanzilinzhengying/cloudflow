import axios from 'axios';

const api = axios.create({
  timeout: 10000,
  headers: { 'Content-Type': 'application/json' },
});

// 平台服务健康检测（基于 link-metrics 数据）
export async function fetchServiceHealth() {
  try {
    const { data } = await api.get('/api/link/');
    const nodes = data?.nodes || {};

    const serviceMap = {
      'data-ingest-vm1': { name: 'Data-Ingest',     port: 9104 },
      'redis-vm1':        { name: 'Redis',            port: 6379 },
      'clickhouse-vm1':   { name: 'ClickHouse',       port: 8123 },
      'nginx-vm1':        { name: 'Nginx',           port: 8080 },
      'ai-service':        { name: 'AI Service',       port: 8082 },
      'control-plane':     { name: 'Control Plane',    port: 8001 },
      'alert-engine':      { name: 'Alert Engine',     port: 9010 },
      'data-plane':        { name: 'Edge Node',        port: 9102 },
      'system-stats':      { name: 'System Stats',     port: 9099 },
      'link-metrics':      { name: 'Link Metrics',     port: 9105 },
      'config-service':    { name: 'Config Service',   port: 9108 },
      'log-service':       { name: 'Log Service',      port: 9106 },
      'edge-health':       { name: 'Edge Health',      port: 8081 },
      'cluster-api':       { name: 'Cluster API',      port: 8083 },
    };

    return Object.entries(serviceMap).map(([key, svc]) => ({
      name: svc.name,
      port: svc.port,
      status: nodes[key]?.status || 'unknown',
      latency: nodes[key]?.latency_ms || 0,
    }));
  } catch (e) {
    console.error('fetchServiceHealth error:', e);
    return [];
  }
}

// 系统资源统计
export async function fetchSystemStats() {
  try {
    const { data } = await api.get('/api/system/stats');
    return data;
  } catch (e) {
    console.error('fetchSystemStats error:', e);
    return null;
  }
}

// 链路指标（供 Topology 页面使用）
export async function fetchLinkMetrics() {
  try {
    const { data } = await api.get('/api/link/');
    return data;
  } catch (e) {
    console.error('fetchLinkMetrics error:', e);
    return null;
  }
}

// 日志查询
export async function fetchLogs(params = {}) {
  try {
    const { data } = await api.get('/api/logs/query', { params });
    return data;
  } catch (e) {
    console.error('fetchLogs error:', e);
    return { logs: [], total: 0 };
  }
}

// 配置管理 API
export async function fetchConfig() {
  try {
    const { data } = await api.get('/api/config');
    return data;
  } catch (e) {
    console.error('fetchConfig error:', e);
    return null;
  }
}

export async function updateConfig(category, key, value) {
  try {
    const { data } = await api.post('/api/config', { category, key, value });
    return data;
  } catch (e) {
    console.error('updateConfig error:', e);
    return { success: false, message: e.message };
  }
}

export async function applyConfig() {
  try {
    const { data } = await api.post('/api/config/apply');
    return data;
  } catch (e) {
    console.error('applyConfig error:', e);
    return { success: false, message: e.message };
  }
}

export default api;
