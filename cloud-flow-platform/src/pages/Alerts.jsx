import { useState, useEffect, useCallback } from 'react';
import { fetchServiceHealth, fetchSystemStats, fetchLinkMetrics } from '../api';

export default function Alerts() {
  const [alerts, setAlerts] = useState([]);
  const [loading, setLoading] = useState(true);
  const [filter, setFilter] = useState('all');
  const [lastUpdate, setLastUpdate] = useState(null);
  const [stats, setStats] = useState(null);
  const [services, setServices] = useState([]);
  const [linkData, setLinkData] = useState(null);

  const loadData = useCallback(async () => {
    try {
      const [statsRes, svcRes, linkRes] = await Promise.allSettled([
        fetchSystemStats(),
        fetchServiceHealth(),
        fetchLinkMetrics(),
      ]);

      const statsData = statsRes.status === 'fulfilled' ? statsRes.value : null;
      const svcData   = svcRes.status === 'fulfilled' ? svcRes.value : [];
      const linkData  = linkRes.status === 'fulfilled' ? linkRes.value : null;

      setStats(statsData);
      setServices(svcData);
      setLinkData(linkData);

      // 生成告警
      const generatedAlerts = [];

      // 1. 服务异常告警
      svcData.forEach(s => {
        if (s.status === 'down') {
          generatedAlerts.push({
            id: `svc-${s.port}`,
            time: new Date().toLocaleTimeString(),
            level: 'critical',
            source: '服务健康',
            message: `服务异常: ${s.name} (端口 ${s.port}) 不可达`,
            detail: `服务 ${s.name} 健康检查失败，状态: ${s.status}`,
          });
        }
      });

      // 2. 资源超阈值告警
      if (statsData) {
        const thresholds = { cpu: 80, mem: 80, disk: 85 };
        if (statsData.cpu_percent > thresholds.cpu) {
          generatedAlerts.push({
            id: 'res-cpu',
            time: new Date().toLocaleTimeString(),
            level: 'warning',
            source: '系统资源',
            message: `CPU 使用率过高: ${statsData.cpu_percent.toFixed(1)}%`,
            detail: `CPU 使用率超过阈值 ${thresholds.cpu}%，当前: ${statsData.cpu_percent.toFixed(1)}%`,
          });
        }
        if (statsData.mem_percent > thresholds.mem) {
          generatedAlerts.push({
            id: 'res-mem',
            time: new Date().toLocaleTimeString(),
            level: 'warning',
            source: '系统资源',
            message: `内存使用率过高: ${statsData.mem_percent.toFixed(1)}%`,
            detail: `内存使用率超过阈值 ${thresholds.mem}%，当前: ${statsData.mem_percent.toFixed(1)}%`,
          });
        }
        if (statsData.disk_percent > thresholds.disk) {
          generatedAlerts.push({
            id: 'res-disk',
            time: new Date().toLocaleTimeString(),
            level: 'warning',
            source: '系统资源',
            message: `磁盘使用率过高: ${statsData.disk_percent.toFixed(1)}%`,
            detail: `磁盘使用率超过阈值 ${thresholds.disk}%，当前: ${statsData.disk_percent.toFixed(1)}%`,
          });
        }
      }

      // 3. 链路异常告警
      if (linkData?.links) {
        Object.entries(linkData.links).forEach(([key, link]) => {
          if (link.status === 'down') {
            generatedAlerts.push({
              id: `link-${key}`,
              time: new Date().toLocaleTimeString(),
              level: 'critical',
              source: '链路监控',
              message: `链路中断: ${link.from} → ${link.to}`,
              detail: `链路 ${key} 状态异常，描述: ${link.description || '无'}`,
            });
          } else if (link.latency_ms > 500) {
            generatedAlerts.push({
              id: `link-latency-${key}`,
              time: new Date().toLocaleTimeString(),
              level: 'warning',
              source: '链路监控',
              message: `链路延迟过高: ${link.from} → ${link.to} (${link.latency_ms}ms)`,
              detail: `链路 ${key} 延迟 ${link.latency_ms}ms 超过阈值 500ms`,
            });
          }
        });
      }

      // 按时间和级别排序（critical 优先）
      generatedAlerts.sort((a, b) => {
        if (a.level === 'critical' && b.level !== 'critical') return -1;
        if (a.level !== 'critical' && b.level === 'critical') return 1;
        return new Date(b.time) - new Date(a.time);
      });

      setAlerts(generatedAlerts);
      setLastUpdate(new Date().toLocaleTimeString());
    } catch (e) {
      console.error('Alerts load error:', e);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadData();
    const interval = setInterval(loadData, 15000);
    return () => clearInterval(interval);
  }, [loadData]);

  const filtered = filter === 'all' ? alerts : alerts.filter(a => a.level === filter);

  const criticalCount = alerts.filter(a => a.level === 'critical').length;
  const warningCount  = alerts.filter(a => a.level === 'warning').length;
  const infoCount     = alerts.filter(a => a.level === 'info').length;

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <h2>平台告警</h2>
        <div style={{ display: 'flex', gap: 16, alignItems: 'center' }}>
          {lastUpdate && <span style={{ color: '#64748b', fontSize: 13 }}>更新于 {lastUpdate}</span>}
          <button onClick={loadData} className="btn-refresh">↻ 刷新</button>
        </div>
      </div>

      {/* 统计卡片 */}
      <div className="grid-4">
        <div className="stat-card">
          <div className="value">{alerts.length}</div>
          <div className="label">全部告警</div>
        </div>
        <div className="stat-card">
          <div className="value" style={{ color: '#ef4444' }}>{criticalCount}</div>
          <div className="label">严重</div>
        </div>
        <div className="stat-card">
          <div className="value" style={{ color: '#f59e0b' }}>{warningCount}</div>
          <div className="label">警告</div>
        </div>
        <div className="stat-card">
          <div className="value" style={{ color: '#3b82f6' }}>{infoCount}</div>
          <div className="label">信息</div>
        </div>
      </div>

      {/* 过滤栏 */}
      <div className="card" style={{ marginTop: 20 }}>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          <button className={`btn-filter ${filter === 'all' ? 'active' : ''}`} onClick={() => setFilter('all')}>全部 ({alerts.length})</button>
          <button className={`btn-filter ${filter === 'critical' ? 'active' : ''}`} onClick={() => setFilter('critical')}>严重 ({criticalCount})</button>
          <button className={`btn-filter ${filter === 'warning' ? 'active' : ''}`} onClick={() => setFilter('warning')}>警告 ({warningCount})</button>
          <button className={`btn-filter ${filter === 'info' ? 'active' : ''}`} onClick={() => setFilter('info')}>信息 ({infoCount})</button>
        </div>
      </div>

      {/* 告警列表 */}
      <div className="card" style={{ marginTop: 16 }}>
        <h3 style={{ marginTop: 0 }}>告警列表</h3>
        {loading ? (
          <div className="empty-state">加载中...</div>
        ) : filtered.length > 0 ? (
          <table className="table">
            <thead>
              <tr>
                <th>级别</th>
                <th>时间</th>
                <th>来源</th>
                <th>告警信息</th>
                <th>详情</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((a, i) => (
                <tr key={a.id || i}>
                  <td>
                    <span className={`badge ${
                      a.level === 'critical' ? 'badge-critical' :
                      a.level === 'warning'  ? 'badge-warning'  : 'badge-info'
                    }`}>
                      {a.level === 'critical' ? '严重' : a.level === 'warning' ? '警告' : '信息'}
                    </span>
                  </td>
                  <td style={{ color: '#64748b', fontSize: 13, whiteSpace: 'nowrap' }}>{a.time}</td>
                  <td style={{ fontSize: 13 }}>{a.source}</td>
                  <td style={{ fontWeight: 500 }}>{a.message}</td>
                  <td style={{ color: '#94a3b8', fontSize: 13, maxWidth: 300, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{a.detail}</td>
                </tr>
              ))}
            </tbody>
          </table>
        ) : (
          <div className="empty-state">暂无告警 🎉</div>
        )}
      </div>
    </div>
  );
}
