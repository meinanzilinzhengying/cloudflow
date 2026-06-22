import { useState, useEffect, useCallback } from 'react';
import {
  fetchDashboardStats, fetchEventsByCategory, fetchEventTimeline,
  fetchProbeStatus, fetchRecentEvents, fetchServiceHealth,
} from '../api';

export default function Dashboard() {
  const [stats, setStats] = useState(null);
  const [categories, setCategories] = useState([]);
  const [timeline, setTimeline] = useState([]);
  const [probeStatus, setProbeStatus] = useState(null);
  const [serviceHealth, setServiceHealth] = useState([]);
  const [recentEvents, setRecentEvents] = useState([]);
  const [loading, setLoading] = useState(true);
  const [lastUpdate, setLastUpdate] = useState(null);

  const loadData = useCallback(async () => {
    try {
      const [s, c, t, p, sh, re] = await Promise.all([
        fetchDashboardStats().catch(() => null),
        fetchEventsByCategory().catch(() => []),
        fetchEventTimeline(30).catch(() => []),
        fetchProbeStatus().catch(() => null),
        fetchServiceHealth().catch(() => []),
        fetchRecentEvents(10).catch(() => []),
      ]);
      setStats(s);
      setCategories(Array.isArray(c) ? c : []);
      setTimeline(Array.isArray(t) ? t : []);
      setProbeStatus(p);
      setServiceHealth(Array.isArray(sh) ? sh : []);
      setRecentEvents(Array.isArray(re) ? re : []);
      setLastUpdate(new Date().toLocaleTimeString());
    } catch (e) {
      console.error('Dashboard load error:', e);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadData();
    const interval = setInterval(loadData, 15000);
    return () => clearInterval(interval);
  }, [loadData]);

  const upCount = serviceHealth.filter(s => s.status === 'up').length;
  const downCount = serviceHealth.filter(s => s.status === 'down').length;

  const maxCount = timeline.length > 0 ? Math.max(...timeline.map(d => parseInt(d.count) || 0), 1) : 1;

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <h2>平台自监控 - 概览</h2>
        <div style={{ display: 'flex', gap: 16, alignItems: 'center' }}>
          {lastUpdate && <span style={{ color: '#64748b', fontSize: 13 }}>更新于 {lastUpdate}</span>}
          <button onClick={loadData} className="btn-refresh" title="刷新">
            ↻ 刷新
          </button>
        </div>
      </div>

      {/* Stat Cards */}
      <div className="grid-4">
        <div className="stat-card">
          <div className="value">{stats?.events_1min ?? '...'}</div>
          <div className="label">近1分钟事件</div>
        </div>
        <div className="stat-card">
          <div className="value">{stats?.total_events ?? '...'}</div>
          <div className="label">5分钟总事件</div>
        </div>
        <div className="stat-card">
          <div className="value">{stats?.categories ?? '...'}</div>
          <div className="label">事件类别</div>
        </div>
        <div className="stat-card">
          <div className="value">{upCount}/{serviceHealth.length}</div>
          <div className="label">服务健康 ({downCount > 0 ? <span style={{color:'#ef4444'}}>{downCount}异常</span> : '正常'})</div>
        </div>
      </div>

      {/* Event Timeline Chart */}
      <div className="card">
        <h3>事件趋势 (近30分钟)</h3>
        {timeline.length > 0 ? (
          <div className="chart-bar">
            {timeline.map((d, i) => (
              <div key={i} className="chart-bar-item" title={`${d.minute}: ${d.count} 事件`}>
                <div className="chart-bar-fill" style={{
                  height: Math.max((parseInt(d.count) / maxCount) * 120, 3) + 'px',
                }} />
                <div className="chart-bar-label">
                  {d.minute ? d.minute.substring(14, 19) : ''}
                </div>
              </div>
            ))}
          </div>
        ) : (
          <div className="empty-state">暂无数据</div>
        )}
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
        {/* Event Categories */}
        <div className="card">
          <h3>事件类别分布</h3>
          {categories.length > 0 ? (
            <table className="table">
              <thead><tr><th>类别</th><th>数量</th><th>占比</th></tr></thead>
              <tbody>
                {categories.map((c, i) => {
                  const total = categories.reduce((s, x) => s + parseInt(x.count), 0) || 1;
                  const pct = ((parseInt(c.count) / total) * 100).toFixed(1);
                  return (
                    <tr key={i}>
                      <td><span className={`badge badge-${c.category}`}>{c.category}</span></td>
                      <td>{c.count}</td>
                      <td>
                        <div className="pct-bar">
                          <div className="pct-fill" style={{ width: pct + '%' }} />
                          <span>{pct}%</span>
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          ) : (
            <div className="empty-state">暂无数据</div>
          )}
        </div>

        {/* Service Health */}
        <div className="card">
          <h3>服务健康状态</h3>
          {serviceHealth.length > 0 ? (
            <table className="table">
              <thead><tr><th>服务</th><th>端口</th><th>状态</th><th>延迟</th></tr></thead>
              <tbody>
                {serviceHealth.map((s, i) => (
                  <tr key={i}>
                    <td>{s.name}</td>
                    <td style={{ color: '#64748b' }}>{s.port}</td>
                    <td className={s.status === 'up' ? 'status-online' : 'status-offline'}>
                      {s.status === 'up' ? '● 正常' : '● 不可达'}
                    </td>
                    <td style={{ color: '#64748b' }}>
                      {s.latency > 0 ? s.latency + 'ms' : '-'}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          ) : (
            <div className="empty-state">检测中...</div>
          )}
        </div>
      </div>

      {/* Recent Events */}
      <div className="card" style={{ marginTop: 16 }}>
        <h3>最近事件</h3>
        {recentEvents.length > 0 ? (
          <table className="table">
            <thead>
              <tr><th>时间</th><th>类别</th><th>类型</th><th>来源</th><th>目标</th></tr>
            </thead>
            <tbody>
              {recentEvents.slice(0, 10).map((e, i) => (
                <tr key={i}>
                  <td style={{ color: '#64748b', fontSize: 13 }}>{e.time}</td>
                  <td><span className={`badge badge-${e.category}`}>{e.category}</span></td>
                  <td>{e.event_type}</td>
                  <td style={{ color: '#94a3b8', fontSize: 13 }}>{e.src_ip || '-'}</td>
                  <td style={{ color: '#94a3b8', fontSize: 13 }}>{e.dst_ip || '-'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        ) : (
          <div className="empty-state">{loading ? '加载中...' : '暂无数据'}</div>
        )}
      </div>

      {/* Probe Info */}
      {probeStatus && (
        <div className="card" style={{ marginTop: 16 }}>
          <h3>探针状态</h3>
          <div style={{ display: 'flex', gap: 24, flexWrap: 'wrap' }}>
            {probeStatus.collectors?.map((c, i) => (
              <div key={i} className="mini-stat">
                <div className={`status-dot ${c.running ? 'online' : 'offline'}`} />
                <div>
                  <div style={{ fontWeight: 600 }}>{c.name}</div>
                  <div style={{ color: '#64748b', fontSize: 13 }}>
                    {c.category} {c.interface ? '· ' + c.interface : ''}
                  </div>
                </div>
              </div>
            )) || <span className="empty-state">无采集器信息</span>}
          </div>
        </div>
      )}
    </div>
  );
}
