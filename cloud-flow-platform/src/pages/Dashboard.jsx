import { useState, useEffect, useCallback } from 'react';
import { fetchSystemStats, fetchServiceHealth } from '../api';

export default function Dashboard() {
  const [sys, setSys] = useState(null);
  const [services, setServices] = useState([]);
  const [loading, setLoading] = useState(true);
  const [lastUpdate, setLastUpdate] = useState(null);

  const loadData = useCallback(async () => {
    try {
      const [s, sh] = await Promise.all([
        fetchSystemStats().catch(() => null),
        fetchServiceHealth().catch(() => []),
      ]);
      setSys(s);
      setServices(Array.isArray(sh) ? sh : []);
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

  const upCount = services.filter(s => s.status === 'up').length;
  const downCount = services.filter(s => s.status === 'down').length;
  const totalCount = services.length;
  const healthPct = totalCount > 0 ? ((upCount / totalCount) * 100).toFixed(1) : '0';

  const fmtUptime = (sec) => {
    if (!sec) return '-';
    const h = Math.floor(sec / 3600);
    const m = Math.floor((sec % 3600) / 60);
    return `${h}小时${m}分钟`;
  };

  const fmtNet = (mb) => {
    if (mb == null) return '-';
    if (mb > 1024) return (mb / 1024).toFixed(1) + ' GB';
    return mb.toFixed(1) + ' MB';
  };

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <h2>平台自监控 - 概览</h2>
        <div style={{ display: 'flex', gap: 16, alignItems: 'center' }}>
          {lastUpdate && <span style={{ color: '#64748b', fontSize: 13 }}>更新于 {lastUpdate}</span>}
          <button onClick={loadData} className="btn-refresh">↻ 刷新</button>
        </div>
      </div>

      {/* 系统资源 KPI */}
      {sys && (
        <div className="grid-4" style={{ marginBottom: 16 }}>
          <div className="stat-card">
            <div className="value" style={{ color: parseFloat(sys.cpu_pct) > 80 ? '#ef4444' : parseFloat(sys.cpu_pct) > 60 ? '#f59e0b' : '#22c55e' }}>
              {sys.cpu_pct}%
            </div>
            <div className="label">CPU 使用率</div>
          </div>
          <div className="stat-card">
            <div className="value" style={{ color: parseFloat(sys.mem_pct) > 80 ? '#ef4444' : parseFloat(sys.mem_pct) > 60 ? '#f59e0b' : '#22c55e' }}>
              {sys.mem_pct}%
            </div>
            <div className="label">内存使用率 ({sys.mem_used_mb}/{sys.mem_total_mb} MB)</div>
          </div>
          <div className="stat-card">
            <div className="value" style={{ color: parseFloat(sys.disk_pct) > 80 ? '#ef4444' : parseFloat(sys.disk_pct) > 60 ? '#f59e0b' : '#22c55e' }}>
              {sys.disk_pct}%
            </div>
            <div className="label">磁盘使用率 ({sys.disk_used_gb}/{sys.disk_total_gb} GB)</div>
          </div>
          <div className="stat-card">
            <div className="value">{healthPct}%</div>
            <div className="label">服务健康率 ({upCount}/{totalCount})</div>
          </div>
        </div>
      )}

      {/* 系统详情 + 服务健康 双栏 */}
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
        {/* 系统资源详情 */}
        <div className="card">
          <h3>系统资源详情</h3>
          {sys ? (
            <table className="table" style={{ fontSize: 13 }}>
              <tbody>
                <tr>
                  <td style={{ fontWeight: 600, width: 100 }}>CPU 使用率</td>
                  <td>
                    <div className="pct-bar" style={{ width: 200 }}>
                      <div className="pct-fill" style={{
                        width: sys.cpu_pct + '%',
                        background: parseFloat(sys.cpu_pct) > 80 ? '#ef4444' : parseFloat(sys.cpu_pct) > 60 ? '#f59e0b' : '#22c55e',
                      }} />
                      <span style={{ marginLeft: 8 }}>{sys.cpu_pct}%</span>
                    </div>
                  </td>
                </tr>
                <tr>
                  <td style={{ fontWeight: 600 }}>内存</td>
                  <td>{sys.mem_used_mb} MB / {sys.mem_total_mb} MB（{sys.mem_pct}%）</td>
                </tr>
                <tr>
                  <td style={{ fontWeight: 600 }}>磁盘</td>
                  <td>{sys.disk_used_gb} GB / {sys.disk_total_gb} GB（{sys.disk_pct}%）</td>
                </tr>
                <tr>
                  <td style={{ fontWeight: 600 }}>系统负载</td>
                  <td>{sys.load_1}（1分钟）/ {sys.load_5}（5分钟）/ {sys.load_15}（15分钟）</td>
                </tr>
                <tr>
                  <td style={{ fontWeight: 600 }}>运行时长</td>
                  <td>{fmtUptime(sys.uptime_sec)}</td>
                </tr>
                <tr>
                  <td style={{ fontWeight: 600 }}>网络接收</td>
                  <td>{fmtNet(sys.net_rx_mb)}</td>
                </tr>
                <tr>
                  <td style={{ fontWeight: 600 }}>网络发送</td>
                  <td>{fmtNet(sys.net_tx_mb)}</td>
                </tr>
                <tr>
                  <td style={{ fontWeight: 600 }}>更新时间</td>
                  <td style={{ color: '#64748b' }}>{sys.timestamp}</td>
                </tr>
              </tbody>
            </table>
          ) : (
            <div className="empty-state">{loading ? '加载中...' : '暂无数据'}</div>
          )}
        </div>

        {/* 服务健康状态 */}
        <div className="card">
          <h3>服务健康状态</h3>
          {services.length > 0 ? (
            <>
              <div className="health-bar-bg" style={{ marginBottom: 12 }}>
                <div className="health-bar-fill" style={{
                  width: healthPct + '%',
                  background: parseFloat(healthPct) >= 80 ? '#22c55e' : parseFloat(healthPct) >= 50 ? '#f59e0b' : '#ef4444',
                }} />
              </div>
              <table className="table" style={{ fontSize: 13 }}>
                <thead><tr><th>服务</th><th>端口</th><th>状态</th><th>延迟</th></tr></thead>
                <tbody>
                  {services.map((s, i) => (
                    <tr key={i} className={s.status === 'down' ? 'row-down' : ''}>
                      <td style={{ fontWeight: 600 }}>{s.name}</td>
                      <td style={{ color: '#64748b' }}>{s.port}</td>
                      <td className={s.status === 'up' ? 'status-online' : 'status-offline'}>
                        {s.status === 'up' ? '● 正常' : '● 不可达'}
                      </td>
                      <td style={{ color: '#64748b' }}>{s.latency > 0 ? s.latency + 'ms' : '-'}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </>
          ) : (
            <div className="empty-state">{loading ? '检测中...' : '无服务数据'}</div>
          )}
        </div>
      </div>
    </div>
  );
}
