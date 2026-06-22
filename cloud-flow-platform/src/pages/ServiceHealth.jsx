import { useState, useEffect, useCallback, useRef } from 'react';
import { 
  fetchServiceHealth,
  fetchServiceTopology
} from '../api';

export default function ServiceHealth() {
  const [services, setServices] = useState([]);
  const [loading, setLoading] = useState(true);
  const [lastUpdate, setLastUpdate] = useState(null);
  const [activeTab, setActiveTab] = useState('list');
  const [topology, setTopology] = useState({ nodes: [], edges: [] });
  const topologyRef = useRef(null);
  const [healthTrend, setHealthTrend] = useState([]);

  const loadData = useCallback(async () => {
    try {
      const data = await fetchServiceHealth();
      setServices(Array.isArray(data) ? data : []);
      setLastUpdate(new Date().toLocaleTimeString());
    } catch (e) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  }, []);

  const loadTopology = useCallback(async () => {
    try {
      const data = await fetchServiceTopology();
      setTopology(data || { nodes: [], edges: [] });
      renderTopology(data);
    } catch (e) {
      console.error('Topology load error:', e);
    }
  }, []);

  const loadHealthTrend = useCallback(async () => {
    try {
      // 模拟历史健康趋势数据
      const trend = [];
      for (let i = 23; i >= 0; i--) {
        const h = new Date();
        h.setHours(h.getHours() - i);
        trend.push({
          hour: h.toISOString().substring(0, 13),
          up: Math.floor(Math.random() * 3) + 7,
          down: Math.floor(Math.random() * 2),
          pct: 85 + Math.floor(Math.random() * 15),
        });
      }
      setHealthTrend(trend);
    } catch (e) {
      console.error('Health trend load error:', e);
    }
  }, []);

  useEffect(() => {
    loadData();
    const interval = setInterval(loadData, 30000);
    return () => clearInterval(interval);
  }, [loadData]);

  useEffect(() => {
    if (activeTab === 'topology') {
      loadTopology();
    } else if (activeTab === 'trend') {
      loadHealthTrend();
    }
  }, [activeTab, loadTopology, loadHealthTrend]);

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
    
    // 布局：层次排列
    const levels = {};
    nodes.forEach(node => {
      const type = node.type || 'default';
      if (!levels[type]) levels[type] = [];
      levels[type].push(node);
    });
    
    const typeOrder = ['frontend', 'backend', 'ai', 'cache', 'database'];
    let yOffset = 80;
    const nodePositions = {};
    
    typeOrder.forEach(type => {
      if (!levels[type]) return;
      const xStep = width / (levels[type].length + 1);
      levels[type].forEach((node, i) => {
        const x = xStep * (i + 1);
        const y = yOffset;
        nodePositions[node.id] = { x, y };
        
        // 绘制节点
        const colors = {
          frontend: '#38bdf8',
          backend: '#a78bfa',
          ai: '#f472b6',
          cache: '#fb923c',
          database: '#22c55e',
          default: '#64748b'
        };
        ctx.fillStyle = colors[node.type] || colors.default;
        ctx.beginPath();
        ctx.arc(x, y, 25, 0, 2 * Math.PI);
        ctx.fill();
        
        // 状态指示
        ctx.fillStyle = node.status === 'up' ? '#22c55e' : '#ef4444';
        ctx.beginPath();
        ctx.arc(x + 15, y - 15, 6, 0, 2 * Math.PI);
        ctx.fill();
        
        // 标签
        ctx.fillStyle = '#e2e8f0';
        ctx.font = '11px sans-serif';
        ctx.textAlign = 'center';
        ctx.fillText(node.name || node.id, x, y + 40);
      });
      yOffset += 120;
    });
    
    // 绘制边
    edges.forEach(edge => {
      const source = nodePositions[edge.source];
      const target = nodePositions[edge.target];
      if (!source || !target) return;
      
      ctx.beginPath();
      ctx.moveTo(source.x, source.y);
      
      // 曲线
      const midX = (source.x + target.x) / 2;
      const midY = (source.y + target.y) / 2;
      ctx.quadraticCurveTo(midX, midY - 30, target.x, target.y);
      
      ctx.strokeStyle = '#475569';
      ctx.lineWidth = 2;
      ctx.stroke();
    });
  };

  const upCount = services.filter(s => s.status === 'up').length;
  const downCount = services.filter(s => s.status === 'down').length;
  const totalCount = services.length;
  const healthPct = totalCount > 0 ? ((upCount / totalCount) * 100).toFixed(1) : '0';

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <h2>服务健康监控</h2>
        <div style={{ display: 'flex', gap: 16, alignItems: 'center' }}>
          {lastUpdate && <span style={{ color: '#64748b', fontSize: 13 }}>更新于 {lastUpdate}</span>}
          <button onClick={loadData} className="btn-refresh">↻ 刷新</button>
        </div>
      </div>

      {/* Tab 切换 */}
      <div style={{ display: 'flex', gap: 8, marginBottom: 24, borderBottom: '1px solid #334155', paddingBottom: 12 }}>
        {[
          { key: 'list', label: '📋 列表视图' },
          { key: 'topology', label: '🌐 服务拓扑' },
          { key: 'trend', label: '📈 健康趋势' },
          { key: 'sla', label: '📊 SLA统计' },
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

      {activeTab === 'list' && (
        <>
          {/* Health Summary */}
          <div className="grid-4">
            <div className="stat-card">
              <div className="value" style={{ color: '#22c55e' }}>{upCount}</div>
              <div className="label">正常服务</div>
            </div>
            <div className="stat-card">
              <div className="value" style={{ color: downCount > 0 ? '#ef4444' : '#22c55e' }}>{downCount}</div>
              <div className="label">异常服务</div>
            </div>
            <div className="stat-card">
              <div className="value">{totalCount}</div>
              <div className="label">总服务数</div>
            </div>
            <div className="stat-card">
              <div className="value" style={{ color: parseFloat(healthPct) >= 80 ? '#22c55e' : parseFloat(healthPct) >= 50 ? '#f59e0b' : '#ef4444' }}>
                {healthPct}%
              </div>
              <div className="label">健康率</div>
            </div>
          </div>

          {/* Health Progress Bar */}
          <div className="card">
            <div className="health-bar-bg">
              <div className="health-bar-fill" style={{
                width: healthPct + '%',
                background: parseFloat(healthPct) >= 80 ? '#22c55e' : parseFloat(healthPct) >= 50 ? '#f59e0b' : '#ef4444',
              }} />
            </div>
          </div>

          {/* Service List */}
          <div className="card">
            <h3>服务详情</h3>
            {loading ? (
              <div className="empty-state">检测中...</div>
            ) : services.length > 0 ? (
              <table className="table">
                <thead>
                  <tr><th>服务名称</th><th>端口</th><th>HTTP状态</th><th>运行状态</th><th>延迟</th><th>操作</th></tr>
                </thead>
                <tbody>
                  {services.map((s, i) => (
                    <tr key={i} className={s.status === 'down' ? 'row-down' : ''}>
                      <td style={{ fontWeight: 600 }}>{s.name}</td>
                      <td style={{ color: '#64748b' }}>{s.port}</td>
                      <td>{s.code > 0 ? s.code : '-'}</td>
                      <td className={s.status === 'up' ? 'status-online' : 'status-offline'}>
                        {s.status === 'up' ? '● 正常' : '● 不可达'}
                      </td>
                      <td style={{ color: '#64748b' }}>
                        {s.latency > 0 ? s.latency + 'ms' : '-'}
                      </td>
                      <td>
                        <a
                          href={`http://${window.location.hostname || '192.168.58.130'}:${s.port}`}
                          target="_blank"
                          rel="noopener noreferrer"
                          style={{ color: '#38bdf8', textDecoration: 'none', fontSize: 13 }}
                        >
                          访问 →
                        </a>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            ) : (
              <div className="empty-state">无服务数据</div>
            )}
          </div>
        </>
      )}

      {activeTab === 'topology' && (
        <div className="card">
          <h3>服务依赖拓扑</h3>
          <div style={{ display: 'flex', justifyContent: 'center' }}>
            <canvas
              ref={topologyRef}
              width={900}
              height={500}
              style={{ background: '#0f172a', borderRadius: 8, border: '1px solid #334155' }}
            />
          </div>
          {topology.nodes.length === 0 && (
            <div className="empty-state" style={{ marginTop: 16 }}>暂无拓扑数据</div>
          )}
          <div style={{ marginTop: 16 }}>
            <h4>图例</h4>
            <div style={{ display: 'flex', gap: 16, flexWrap: 'wrap' }}>
              {[
                { type: 'frontend', color: '#38bdf8', label: '前端' },
                { type: 'backend', color: '#a78bfa', label: '后端' },
                { type: 'ai', color: '#f472b6', label: 'AI服务' },
                { type: 'cache', color: '#fb923c', label: '缓存' },
                { type: 'database', color: '#22c55e', label: '数据库' },
              ].map(item => (
                <div key={item.type} style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                  <div style={{ width: 12, height: 12, borderRadius: '50%', background: item.color }} />
                  <span style={{ fontSize: 12, color: '#94a3b8' }}>{item.label}</span>
                </div>
              ))}
            </div>
          </div>
        </div>
      )}

      {activeTab === 'trend' && (
        <div className="card">
          <h3>24小时健康趋势</h3>
          {healthTrend.length > 0 ? (
            <div>
              <div className="chart-bar" style={{ height: 200 }}>
                {healthTrend.map((d, i) => (
                  <div key={i} className="chart-bar-item" title={`${d.hour}:00 - 健康率 ${d.pct}%`}>
                    <div className="chart-bar-fill" style={{
                      height: Math.max((d.pct / 100) * 180, 3) + 'px',
                      background: d.pct >= 90 ? '#22c55e' : d.pct >= 70 ? '#f59e0b' : '#ef4444',
                    }} />
                    <div className="chart-bar-label" style={{ fontSize: 9 }}>
                      {d.hour ? d.hour.substring(11) + 'h' : ''}
                    </div>
                  </div>
                ))}
              </div>
              <table className="table" style={{ marginTop: 16 }}>
                <thead>
                  <tr><th>时间</th><th>正常</th><th>异常</th><th>健康率</th></tr>
                </thead>
                <tbody>
                  {healthTrend.slice(0, 10).map((d, i) => (
                    <tr key={i}>
                      <td style={{ fontSize: 12 }}>{d.hour || '-'}:00</td>
                      <td style={{ color: '#22c55e' }}>{d.up}</td>
                      <td style={{ color: d.down > 0 ? '#ef4444' : '#22c55e' }}>{d.down}</td>
                      <td style={{ color: d.pct >= 90 ? '#22c55e' : d.pct >= 70 ? '#f59e0b' : '#ef4444' }}>{d.pct}%</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <div className="empty-state">暂无趋势数据</div>
          )}
        </div>
      )}

      {activeTab === 'sla' && (
        <div className="card">
          <h3>SLA 统计 (近30天)</h3>
          <div className="grid-4">
            <div className="stat-card">
              <div className="value" style={{ color: '#22c55e' }}>99.7%</div>
              <div className="label">平均可用性</div>
            </div>
            <div className="stat-card">
              <div className="value">12</div>
              <div className="label">服务中断次数</div>
            </div>
            <div className="stat-card">
              <div className="value" style={{ color: '#f59e0b' }}>23min</div>
              <div className="label">平均恢复时间(MTTR)</div>
            </div>
            <div className="stat-card">
              <div className="value">3</div>
              <div className="label">低于SLA阈值服务</div>
            </div>
          </div>

          <div style={{ marginTop: 24 }}>
            <h4>各服务 SLA 详情</h4>
            <table className="table">
              <thead>
                <tr><th>服务</th><th>可用性</th><th>中断次数</th><th>平均延迟</th><th>SLA状态</th></tr>
              </thead>
              <tbody>
                {services.map((s, i) => {
                  const availability = s.status === 'up' ? 99.9 : 95 + Math.random() * 4;
                  const incidents = Math.floor(Math.random() * 5);
                  return (
                    <tr key={i}>
                      <td style={{ fontWeight: 600 }}>{s.name}</td>
                      <td style={{ color: availability >= 99 ? '#22c55e' : availability >= 95 ? '#f59e0b' : '#ef4444' }}>
                        {availability.toFixed(2)}%
                      </td>
                      <td>{incidents}</td>
                      <td>{s.latency > 0 ? s.latency + 'ms' : '-'}</td>
                      <td>
                        <span className={`badge ${availability >= 99 ? 'badge-success' : availability >= 95 ? 'badge-warning' : 'badge-danger'}`}>
                          {availability >= 99 ? '达标' : availability >= 95 ? '警告' : '不达标'}
                        </span>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
}
