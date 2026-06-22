import { useState, useEffect, useCallback, useRef } from 'react';
import { 
  fetchDashboardStats, 
  fetchEventsByCategory, 
  fetchEventTimeline,
  fetchProbeStatus, 
  fetchServiceHealth,
  fetchRecentEvents,
  fetchNetworkTopology,
  fetchHistoryStats
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
  const [activeTab, setActiveTab] = useState('overview');
  
  // 网络拓扑
  const [topology, setTopology] = useState({ nodes: [], edges: [] });
  const topologyRef = useRef(null);
  
  // 历史趋势
  const [historyStats, setHistoryStats] = useState([]);
  const [historyHours, setHistoryHours] = useState(24);

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

  const loadTopology = useCallback(async () => {
    try {
      const data = await fetchNetworkTopology();
      setTopology(data || { nodes: [], edges: [] });
      renderTopology(data);
    } catch (e) {
      console.error('Topology load error:', e);
    }
  }, []);

  const loadHistory = useCallback(async () => {
    try {
      const data = await fetchHistoryStats(historyHours);
      setHistoryStats(Array.isArray(data) ? data : []);
    } catch (e) {
      console.error('History load error:', e);
    }
  }, [historyHours]);

  useEffect(() => {
    loadData();
    const interval = setInterval(loadData, 15000);
    return () => clearInterval(interval);
  }, [loadData]);

  useEffect(() => {
    if (activeTab === 'topology') {
      loadTopology();
    } else if (activeTab === 'history') {
      loadHistory();
    }
  }, [activeTab, loadTopology, loadHistory]);

  const renderTopology = (data) => {
    if (!topologyRef.current || !data) return;
    
    const canvas = topologyRef.current;
    const ctx = canvas.getContext('2d');
    const width = canvas.width;
    const height = canvas.height;
    
    ctx.clearRect(0, 0, width, height);
    
    const nodes = data.nodes || [];
    const edges = data.edges || [];
    
    if (nodes.length === 0) return;
    
    // 简单布局：圆形排列节点
    const centerX = width / 2;
    const centerY = height / 2;
    const radius = Math.min(width, height) * 0.3;
    
    const nodePositions = {};
    nodes.forEach((node, i) => {
      const angle = (2 * Math.PI * i) / nodes.length;
      const x = centerX + radius * Math.cos(angle);
      const y = centerY + radius * Math.sin(angle);
      nodePositions[node.id] = { x, y };
      
      // 绘制节点
      ctx.beginPath();
      ctx.arc(x, y, 30, 0, 2 * Math.PI);
      ctx.fillStyle = node.status === 'online' ? '#22c55e' : node.status === 'warning' ? '#f59e0b' : '#ef4444';
      ctx.fill();
      ctx.strokeStyle = '#38bdf8';
      ctx.lineWidth = 2;
      ctx.stroke();
      
      // 节点标签
      ctx.fillStyle = '#e2e8f0';
      ctx.font = '12px sans-serif';
      ctx.textAlign = 'center';
      ctx.fillText(node.name || node.id, x, y + 45);
    });
    
    // 绘制边
    edges.forEach(edge => {
      const source = nodePositions[edge.source];
      const target = nodePositions[edge.target];
      if (!source || !target) return;
      
      ctx.beginPath();
      ctx.moveTo(source.x, source.y);
      ctx.lineTo(target.x, target.y);
      ctx.strokeStyle = edge.status === 'normal' ? '#22c55e' : '#f59e0b';
      ctx.lineWidth = 2;
      ctx.stroke();
      
      // 标签
      const midX = (source.x + target.x) / 2;
      const midY = (source.y + target.y) / 2;
      ctx.fillStyle = '#94a3b8';
      ctx.font = '10px sans-serif';
      ctx.fillText(edge.label || '', midX, midY - 5);
    });
  };

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

      {/* Tab 切换 */}
      <div style={{ display: 'flex', gap: 8, marginBottom: 24, borderBottom: '1px solid #334155', paddingBottom: 12 }}>
        {[
          { key: 'overview', label: '📊 总览' },
          { key: 'topology', label: '🌐 网络拓扑' },
          { key: 'history', label: '📈 历史趋势' },
        ].map(tab => (
          <button
            key={tab.key}
            onClick={() => setActiveTab(tab.key)}
            className={`btn ${activeTab === tab.key ? 'btn-primary' : 'btn-secondary'}`}
            style={{ padding: '8px 16px' }}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {activeTab === 'overview' && (
        <>
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
        </>
      )}

      {activeTab === 'topology' && (
        <div className="card">
          <h3>网络拓扑图</h3>
          <div style={{ display: 'flex', justifyContent: 'center' }}>
            <canvas
              ref={topologyRef}
              width={800}
              height={500}
              style={{ background: '#0f172a', borderRadius: 8, border: '1px solid #334155' }}
            />
          </div>
          {topology.nodes.length === 0 && (
            <div className="empty-state" style={{ marginTop: 16 }}>暂无拓扑数据</div>
          )}
          <div style={{ marginTop: 16 }}>
            <h4>节点列表</h4>
            <div style={{ display: 'flex', gap: 12, flexWrap: 'wrap' }}>
              {topology.nodes.map((node, i) => (
                <div key={i} style={{
                  background: '#1e293b',
                  border: '1px solid #334155',
                  borderRadius: 8,
                  padding: '12px 16px',
                  display: 'flex',
                  alignItems: 'center',
                  gap: 8,
                }}>
                  <div style={{
                    width: 8,
                    height: 8,
                    borderRadius: '50%',
                    background: node.status === 'online' ? '#22c55e' : node.status === 'warning' ? '#f59e0b' : '#ef4444',
                  }} />
                  <div>
                    <div style={{ fontWeight: 600, fontSize: 13 }}>{node.name}</div>
                    <div style={{ color: '#64748b', fontSize: 11 }}>{node.ip}</div>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>
      )}

      {activeTab === 'history' && (
        <div className="card">
          <h3>历史趋势 (近{historyHours}小时)</h3>
          <div style={{ display: 'flex', gap: 8, marginBottom: 16 }}>
            {[6, 24, 48, 168].map(h => (
              <button
                key={h}
                onClick={() => setHistoryHours(h)}
                className={`btn ${historyHours === h ? 'btn-primary' : 'btn-secondary'}`}
                style={{ padding: '6px 12px', fontSize: 12 }}
              >
                {h < 24 ? `${h}小时` : `${h / 24}天`}
              </button>
            ))}
          </div>
          {historyStats.length > 0 ? (
            <div>
              <div className="chart-bar" style={{ height: 200 }}>
                {historyStats.map((d, i) => {
                  const maxVal = Math.max(...historyStats.map(x => parseInt(x.total_events) || 0), 1);
                  return (
                    <div key={i} className="chart-bar-item" title={`${d.hour}: ${d.total_events} 事件`}>
                      <div className="chart-bar-fill" style={{
                        height: Math.max((parseInt(d.total_events) / maxVal) * 180, 3) + 'px',
                        background: '#38bdf8',
                      }} />
                      <div className="chart-bar-label" style={{ fontSize: 9 }}>
                        {d.hour ? d.hour.substring(11, 13) + 'h' : ''}
                      </div>
                    </div>
                  );
                })}
              </div>
              <table className="table" style={{ marginTop: 16 }}>
                <thead>
                  <tr><th>时间</th><th>总事件</th><th>活跃探针</th><th>网络事件</th><th>安全事件</th></tr>
                </thead>
                <tbody>
                  {historyStats.slice(0, 10).map((d, i) => (
                    <tr key={i}>
                      <td style={{ fontSize: 12 }}>{d.hour || '-'}</td>
                      <td>{d.total_events || 0}</td>
                      <td>{d.active_probes || 0}</td>
                      <td>{d.network_events || 0}</td>
                      <td>{d.security_events || 0}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <div className="empty-state">暂无历史数据</div>
          )}
        </div>
      )}
    </div>
  );
}
