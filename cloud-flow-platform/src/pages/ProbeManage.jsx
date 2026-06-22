import { useState, useEffect, useCallback } from 'react';
import { fetchProbeStatus, fetchProbeMetrics, probeAction } from '../api';

export default function ProbeManage() {
  const [status, setStatus] = useState(null);
  const [metrics, setMetrics] = useState(null);
  const [loading, setLoading] = useState(true);
  const [actionLoading, setActionLoading] = useState(null);

  const loadData = useCallback(async () => {
    try {
      const [s, m] = await Promise.all([
        fetchProbeStatus().catch(() => null),
        fetchProbeMetrics().catch(() => null),
      ]);
      setStatus(s);
      setMetrics(m);
    } catch (e) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadData();
    const interval = setInterval(loadData, 10000);
    return () => clearInterval(interval);
  }, [loadData]);

  const handleAction = async (action) => {
    setActionLoading(action);
    try {
      await probeAction(action);
      setTimeout(loadData, 2000);
    } catch (e) {
      alert('操作失败: ' + (e.message || '未知错误'));
    } finally {
      setActionLoading(null);
    }
  };

  if (loading) {
    return <div className="card"><div className="empty-state">加载中...</div></div>;
  }

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <h2>探针管理</h2>
        <button onClick={loadData} className="btn-refresh">↻ 刷新</button>
      </div>

      {/* Status Card */}
      <div className="card">
        <h3>探针信息</h3>
        <div className="grid-4">
          <div className="stat-card">
            <div className={`value ${status?.status === 'running' ? 'status-online' : 'status-offline'}`}>
              {status?.status || '未知'}
            </div>
            <div className="label">运行状态</div>
          </div>
          <div className="stat-card">
            <div className="value">{status?.version || '-'}</div>
            <div className="label">版本</div>
          </div>
          <div className="stat-card">
            <div className="value">{status?.uptime || '-'}</div>
            <div className="label">运行时间</div>
          </div>
          <div className="stat-card">
            <div className="value">{status?.collectors?.filter(c => c.running).length ?? '-'}/{status?.collectors?.length ?? '-'}</div>
            <div className="label">采集器(活跃/总数)</div>
          </div>
        </div>
      </div>

      {/* Control Buttons */}
      <div className="card">
        <h3>探针控制</h3>
        <div style={{ display: 'flex', gap: 12 }}>
          <button
            className="btn btn-success"
            onClick={() => handleAction('start')}
            disabled={actionLoading !== null}
          >
            {actionLoading === 'start' ? '启动中...' : '▶ 启动'}
          </button>
          <button
            className="btn btn-warning"
            onClick={() => handleAction('stop')}
            disabled={actionLoading !== null}
          >
            {actionLoading === 'stop' ? '停止中...' : '⏸ 停止'}
          </button>
          <button
            className="btn btn-primary"
            onClick={() => handleAction('restart')}
            disabled={actionLoading !== null}
          >
            {actionLoading === 'restart' ? '重启中...' : '↻ 重启'}
          </button>
        </div>
      </div>

      {/* Collectors */}
      <div className="card">
        <h3>采集器状态</h3>
        {status?.collectors ? (
          <table className="table">
            <thead>
              <tr><th>名称</th><th>类别</th><th>接口</th><th>状态</th></tr>
            </thead>
            <tbody>
              {status.collectors.map((c, i) => (
                <tr key={i}>
                  <td style={{ fontWeight: 600 }}>{c.name}</td>
                  <td><span className={`badge badge-${c.category}`}>{c.category}</span></td>
                  <td style={{ color: '#64748b' }}>{c.interface || '-'}</td>
                  <td className={c.running ? 'status-online' : 'status-offline'}>
                    {c.running ? '● 运行中' : '● 已停止'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        ) : (
          <div className="empty-state">无采集器数据</div>
        )}
      </div>

      {/* Metrics */}
      {metrics && (
        <div className="card">
          <h3>探针指标</h3>
          <div className="grid-4">
            {Object.entries(metrics).slice(0, 8).map(([key, val]) => (
              <div key={key} className="stat-card" style={{ padding: 16 }}>
                <div className="value" style={{ fontSize: 20 }}>{typeof val === 'number' ? val.toFixed(1) : String(val)}</div>
                <div className="label">{key.replace(/_/g, ' ')}</div>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
