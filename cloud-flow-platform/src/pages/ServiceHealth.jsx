import { useState, useEffect, useCallback } from 'react';
import { fetchServiceHealth } from '../api';

export default function ServiceHealth() {
  const [services, setServices] = useState([]);
  const [loading, setLoading] = useState(true);
  const [lastUpdate, setLastUpdate] = useState(null);

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

  useEffect(() => {
    loadData();
    const interval = setInterval(loadData, 30000);
    return () => clearInterval(interval);
  }, [loadData]);

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
    </div>
  );
}
