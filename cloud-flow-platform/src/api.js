import axios from 'axios';

const api = axios.create({
  timeout: 10000,
  headers: { 'Content-Type': 'application/json' },
});

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
  // Parse TabSeparated
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

export async function fetchDashboardStats() {
  const sql = `SELECT count() AS total_events, uniq(category) AS categories, uniq(probe_id) AS active_probes, countIf(timestamp > now() - INTERVAL 1 MINUTE) AS events_1min FROM cloudflow.ebpf_events WHERE timestamp > now() - INTERVAL 5 MINUTE`;
  const rows = await clickhouseTabSeparated(sql);
  return rows[0] || null;
}

export async function fetchEventsByCategory() {
  const sql = `SELECT category, count() AS count FROM cloudflow.ebpf_events WHERE timestamp > now() - INTERVAL 15 MINUTE GROUP BY category ORDER BY count DESC`;
  return await clickhouseTabSeparated(sql);
}

export async function fetchRecentEvents(limit = 50) {
  const sql = `SELECT toString(timestamp) AS time, category, event_type, probe_id, src_ip, dst_ip, protocol, details FROM cloudflow.ebpf_events ORDER BY timestamp DESC LIMIT ${limit}`;
  return await clickhouseQuery(sql);
}

export async function fetchSecurityEvents(limit = 20) {
  const sql = `SELECT toString(timestamp) AS time, event_type, probe_id, src_ip, dst_ip, details FROM cloudflow.ebpf_events WHERE category = 'security' ORDER BY timestamp DESC LIMIT ${limit}`;
  return await clickhouseQuery(sql);
}

export async function fetchEventTimeline(minutes = 30) {
  const sql = `SELECT toStartOfMinute(timestamp) AS minute, count() AS count FROM cloudflow.ebpf_events WHERE timestamp > now() - INTERVAL ${minutes} MINUTE GROUP BY minute ORDER BY minute`;
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
