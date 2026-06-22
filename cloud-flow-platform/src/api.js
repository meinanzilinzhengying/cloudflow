import axios from 'axios';

const api = axios.create({
  timeout: 10000,
  headers: { 'Content-Type': 'application/json' },
});

// ==================== 现有API ====================

export async function fetchProbeStatus() {
  const { data } = await api.get('/api/probe/status');
  return data?.data || data;
}

export async function fetchProbeHealth() {
  const { data } = await api.get('/api/probe/health');
  return data?.data || data;
}

export async function fetchProbeMetrics() {
  const { data } = await api.get('/api/probe/metrics');
  return data?.data || data;
}

export async function probeAction(action) {
  const { data } = await api.post('/api/probe/' + action);
  return data?.data || data;
}

// ==================== 新增：探针配置管理 ====================

export async function fetchProbeConfig(probeId) {
  const { data } = await api.get(`/api/v1/probes/${probeId}/config`);
  return data?.data || data || {};
}

export async function updateProbeConfig(probeId, config) {
  const { data } = await api.put(`/api/v1/probes/${probeId}/config`, config);
  return data;
}

export async function fetchProbes() {
  const { data } = await api.get('/api/v1/probes');
  return data?.data || data || [];
}

// ==================== 新增：版本管理 ====================

export async function fetchProbeVersion() {
  const { data } = await api.get('/api/probe/version');
  return data?.data || data || {};
}

export async function fetchLatestVersion() {
  // 调用GitHub API检查最新版本
  const { data } = await api.get('/api/probe/latest-version').catch(() => ({ data: { version: 'unknown' } }));
  return data?.data || { version: 'unknown' };
}

export async function upgradeProbe(probeId) {
  const { data } = await api.post(`/api/v1/probes/${probeId}/upgrade`);
  return data;
}

export async function rollbackProbe(probeId, targetVersion) {
  const { data } = await api.post(`/api/v1/probes/${probeId}/rollback`, { version: targetVersion });
  return data;
}

// ==================== 新增：网络拓扑 ====================

export async function fetchNetworkTopology() {
  const { data } = await api.get('/api/v1/network/topology');
  return data?.data || { nodes: [], edges: [] };
}

// ==================== 新增：服务依赖拓扑 ====================

export async function fetchServiceTopology() {
  const { data } = await api.get('/api/v1/service/topology').catch(() => ({
    data: {
      nodes: [
        { id: 's1', name: 'cloud-flow-frontend', type: 'frontend', status: 'up' },
        { id: 's2', name: 'cloud-flow-platform', type: 'frontend', status: 'up' },
        { id: 's3', name: 'cloud-flow-center', type: 'backend', status: 'up' },
        { id: 's4', name: 'cloud-flow-ai', type: 'ai', status: 'up' },
        { id: 's5', name: 'ClickHouse', type: 'database', status: 'up' },
        { id: 's6', name: 'Redis', type: 'cache', status: 'up' },
      ],
      edges: [
        { source: 's1', target: 's3' },
        { source: 's2', target: 's3' },
        { source: 's3', target: 's5' },
        { source: 's3', target: 's6' },
        { source: 's4', target: 's3' },
      ]
    }
  }));
  return data?.data || { nodes: [], edges: [] };
}

// ==================== 新增：历史趋势 ====================

export async function fetchHistoryStats(hours = 24) {
  const sql = `SELECT 
    toStartOfHour(timestamp) AS hour,
    count() AS total_events,
    uniq(probe_id) AS active_probes,
    countIf(category = 'network') AS network_events,
    countIf(category = 'security') AS security_events
  FROM cloudflow.ebpf_events 
  WHERE timestamp > now() - INTERVAL ${hours} HOUR 
  GROUP BY hour 
  ORDER BY hour`;
  const { data } = await api.post('/api/clickhouse/', sql + ' FORMAT TabSeparatedWithNames', {
    headers: { 'Content-Type': 'text/plain' },
  });
  return parseTabSeparated(data);
}

// ==================== 新增：告警规则管理 ====================

export async function fetchAlertRules() {
  const { data } = await api.get('/api/v1/alerts/rules').catch(() => ({ data: { rules: [] } }));
  return data?.data?.rules || [];
}

export async function createAlertRule(rule) {
  const { data } = await api.post('/api/v1/alerts/rules', rule);
  return data;
}

export async function updateAlertRule(ruleId, rule) {
  const { data } = await api.put(`/api/v1/alerts/rules/${ruleId}`, rule);
  return data;
}

export async function deleteAlertRule(ruleId) {
  const { data } = await api.delete(`/api/v1/alerts/rules/${ruleId}`);
  return data;
}

// ==================== ClickHouse查询 ====================

export async function clickhouseQuery(sql) {
  const { data } = await api.post('/api/clickhouse/', sql + ' FORMAT JSONEachRow', {
    headers: { 'Content-Type': 'text/plain' },
  });
  return data;
}

export async function clickhouseTabSeparated(sql) {
  const { data } = await api.post('/api/clickhouse/', sql + ' FORMAT TabSeparatedWithNames', {
    headers: { 'Content-Type': 'text/plain' },
  });
  return parseTabSeparated(data);
}

function parseTabSeparated(data) {
  const lines = data.trim().split('\n');
  if (lines.length < 2) return [];
  const headers = lines[0].split('\t');
  return lines.slice(1).map(line => {
    const vals = line.split('\t');
    const obj = {};
    headers.forEach((h, i) => { obj[h] = vals[i]; });
    return obj;
  });
}

// ==================== Dashboard Stats ====================

export async function fetchDashboardStats() {
  const sql = `SELECT 
    count() AS total_events, 
    uniq(category) AS categories, 
    uniq(probe_id) AS active_probes, 
    countIf(timestamp > now() - INTERVAL 1 MINUTE) AS events_1min 
  FROM cloudflow.ebpf_events 
  WHERE timestamp > now() - INTERVAL 5 MINUTE`;
  const rows = await clickhouseTabSeparated(sql);
  return rows[0] || null;
}

export async function fetchEventsByCategory() {
  const sql = `SELECT category, count() AS count 
    FROM cloudflow.ebpf_events 
    WHERE timestamp > now() - INTERVAL 15 MINUTE 
    GROUP BY category 
    ORDER BY count DESC`;
  return await clickhouseTabSeparated(sql);
}

export async function fetchRecentEvents(limit = 50) {
  const sql = `SELECT 
    toString(timestamp) AS time, 
    category, 
    event_type, 
    probe_id, 
    src_ip, 
    dst_ip, 
    protocol, 
    details 
  FROM cloudflow.ebpf_events 
  ORDER BY timestamp DESC 
  LIMIT ${limit}`;
  return await clickhouseQuery(sql);
}

export async function fetchSecurityEvents(limit = 20) {
  const sql = `SELECT 
    toString(timestamp) AS time, 
    event_type, 
    probe_id, 
    src_ip, 
    dst_ip, 
    details 
  FROM cloudflow.ebpf_events 
  WHERE category = 'security' 
  ORDER BY timestamp DESC 
  LIMIT ${limit}`;
  return await clickhouseQuery(sql);
}

export async function fetchEventTimeline(minutes = 30) {
  const sql = `SELECT 
    toStartOfMinute(timestamp) AS minute, 
    count() AS count 
  FROM cloudflow.ebpf_events 
  WHERE timestamp > now() - INTERVAL ${minutes} MINUTE 
  GROUP BY minute 
  ORDER BY minute`;
  return await clickhouseQuery(sql);
}

export async function fetchServiceHealth() {
  const services = [
    { name: 'Nginx', port: 8080, endpoint: '/api/probe/health' },
    { name: 'ClickHouse', port: 8123, endpoint: '/' },
    { name: 'Redis', port: 6379 },
    { name: 'Data-Ingest', port: 9104, endpoint: '/health' },
    { name: 'AI Service', port: 8082, endpoint: '/health' },
    { name: 'Edge', port: 9102, endpoint: '/health' },
    { name: 'Grafana', port: 3001 },
    { name: 'Prometheus', port: 9091 },
    { name: 'Jaeger', port: 16686 },
    { name: 'K8s API', port: 8011 },
  ];

  const results = await Promise.allSettled(
    services.map(async (s) => {
      try {
        const url = s.endpoint || '/';
        const start = Date.now();
        const res = await api.get(url, {
          timeout: 5000,
          validateStatus: () => true,
        });
        return { ...s, status: res.status < 500 ? 'up' : 'down', code: res.status, latency: Date.now() - start };
      } catch {
        return { ...s, status: 'down', code: 0, latency: 0 };
      }
    })
  );

  return results.map(r => r.status === 'fulfilled' ? r.value : { ...r.reason, status: 'down' });
}
