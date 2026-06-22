import { useState, useEffect, useCallback } from 'react';
import { 
  fetchProbeStatus, 
  fetchProbeMetrics, 
  probeAction,
  fetchProbes,
  fetchProbeConfig,
  updateProbeConfig,
  fetchProbeVersion,
  upgradeProbe,
  rollbackProbe
} from '../api';

export default function ProbeManage() {
  const [status, setStatus] = useState(null);
  const [metrics, setMetrics] = useState(null);
  const [probes, setProbes] = useState([]);
  const [selectedProbe, setSelectedProbe] = useState(null);
  const [loading, setLoading] = useState(true);
  const [actionLoading, setActionLoading] = useState(null);
  
  // 配置编辑
  const [showConfig, setShowConfig] = useState(false);
  const [config, setConfig] = useState({});
  const [configLoading, setConfigLoading] = useState(false);
  
  // 版本管理
  const [showVersion, setShowVersion] = useState(false);
  const [versionInfo, setVersionInfo] = useState(null);
  const [upgradeLoading, setUpgradeLoading] = useState(false);
  
  // 批量操作
  const [selectedProbes, setSelectedProbes] = useState([]);
  const [showBatch, setShowBatch] = useState(false);

  const loadData = useCallback(async () => {
    try {
      const [s, m, p] = await Promise.all([
        fetchProbeStatus().catch(() => null),
        fetchProbeMetrics().catch(() => null),
        fetchProbes().catch(() => []),
      ]);
      setStatus(s);
      setMetrics(m);
      setProbes(Array.isArray(p) ? p : []);
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

  const handleBatchAction = async (action) => {
    setActionLoading(action);
    try {
      await Promise.all(selectedProbes.map(id => probeAction(action)));
      setSelectedProbes([]);
      setShowBatch(false);
      setTimeout(loadData, 2000);
    } catch (e) {
      alert('批量操作失败: ' + (e.message || '未知错误'));
    } finally {
      setActionLoading(null);
    }
  };

  const loadConfig = async (probeId) => {
    setConfigLoading(true);
    try {
      const cfg = await fetchProbeConfig(probeId);
      setConfig(cfg || {});
      setSelectedProbe(probeId);
      setShowConfig(true);
    } catch (e) {
      alert('加载配置失败: ' + (e.message || '未知错误'));
    } finally {
      setConfigLoading(false);
    }
  };

  const saveConfig = async () => {
    try {
      await updateProbeConfig(selectedProbe, config);
      alert('配置保存成功');
      setShowConfig(false);
      loadData();
    } catch (e) {
      alert('保存配置失败: ' + (e.message || '未知错误'));
    }
  };

  const loadVersion = async (probeId) => {
    setUpgradeLoading(true);
    try {
      const info = await fetchProbeVersion();
      setVersionInfo(info);
      setSelectedProbe(probeId);
      setShowVersion(true);
    } catch (e) {
      alert('加载版本信息失败: ' + (e.message || '未知错误'));
    } finally {
      setUpgradeLoading(false);
    }
  };

  const handleUpgrade = async () => {
    setUpgradeLoading(true);
    try {
      await upgradeProbe(selectedProbe);
      alert('升级成功');
      setShowVersion(false);
      loadData();
    } catch (e) {
      alert('升级失败: ' + (e.message || '未知错误'));
    } finally {
      setUpgradeLoading(false);
    }
  };

  const handleRollback = async (targetVersion) => {
    setUpgradeLoading(true);
    try {
      await rollbackProbe(selectedProbe, targetVersion);
      alert('回滚成功');
      setShowVersion(false);
      loadData();
    } catch (e) {
      alert('回滚失败: ' + (e.message || '未知错误'));
    } finally {
      setUpgradeLoading(false);
    }
  };

  const toggleProbeSelection = (probeId) => {
    setSelectedProbes(prev => 
      prev.includes(probeId) 
        ? prev.filter(id => id !== probeId) 
        : [...prev, probeId]
    );
  };

  if (loading) {
    return <div className="card"><div className="empty-state">加载中...</div></div>;
  }

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <h2>探针管理</h2>
        <div style={{ display: 'flex', gap: 8 }}>
          <button onClick={loadData} className="btn-refresh">↻ 刷新</button>
          {probes.length > 1 && (
            <button 
              onClick={() => setShowBatch(!showBatch)}
              className="btn-btn-primary"
            >
              ☑ 批量操作 ({selectedProbes.length})
            </button>
          )}
        </div>
      </div>

      {/* 批量操作面板 */}
      {showBatch && (
        <div className="card" style={{ marginBottom: 16, background: '#1e293b' }}>
          <h3>批量操作</h3>
          <div style={{ display: 'flex', gap: 8 }}>
            <button 
              className="btn btn-success" 
              onClick={() => handleBatchAction('start')}
              disabled={actionLoading !== null}
            >
              ▶ 批量启动
            </button>
            <button 
              className="btn btn-warning" 
              onClick={() => handleBatchAction('stop')}
              disabled={actionLoading !== null}
            >
              ⏸ 批量停止
            </button>
            <button 
              className="btn btn-primary" 
              onClick={() => handleBatchAction('restart')}
              disabled={actionLoading !== null}
            >
              ↻ 批量重启
            </button>
          </div>
        </div>
      )}

      {/* Status Card */}
      <div className="card">
        <h3>探针概览</h3>
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
            <div className="value">
              {status?.collectors?.filter(c => c.running).length ?? '-'}/
              {status?.collectors?.length ?? '-'}
            </div>
            <div className="label">采集器(活跃/总数)</div>
          </div>
        </div>
      </div>

      {/* Probes List */}
      <div className="card">
        <h3>探针列表 ({probes.length})</h3>
        {probes.length > 0 ? (
          <table className="table">
            <thead>
              <tr>
                {probes.length > 1 && (
                  <th>
                    <input 
                      type="checkbox" 
                      checked={selectedProbes.length === probes.length}
                      onChange={(e) => {
                        if (e.target.checked) {
                          setSelectedProbes(probes.map(p => p.id));
                        } else {
                          setSelectedProbes([]);
                        }
                      }}
                    />
                  </th>
                )}
                <th>名称</th>
                <th>IP地址</th>
                <th>主机名</th>
                <th>版本</th>
                <th>状态</th>
                <th>采集器</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              {probes.map((probe, i) => (
                <tr key={i}>
                  {probes.length > 1 && (
                    <td>
                      <input 
                        type="checkbox" 
                        checked={selectedProbes.includes(probe.id)}
                        onChange={() => toggleProbeSelection(probe.id)}
                      />
                    </td>
                  )}
                  <td style={{ fontWeight: 600 }}>{probe.id}</td>
                  <td style={{ fontFamily: 'monospace' }}>{probe.ip || '-'}</td>
                  <td>{probe.hostname || '-'}</td>
                  <td>
                    <span className="badge badge-info">{probe.version || '-'}</span>
                  </td>
                  <td className={probe.status === 'online' ? 'status-online' : 'status-offline'}>
                    {probe.status === 'online' ? '● 在线' : '● 离线'}
                  </td>
                  <td>
                    {Array.isArray(probe.collectors) && probe.collectors.map((c, j) => (
                      <span key={j} className={`badge ${c.running ? 'badge-success' : 'badge-danger'}`} style={{ marginRight: 4 }}>
                        {c}
                      </span>
                    ))}
                  </td>
                  <td>
                    <div style={{ display: 'flex', gap: 4 }}>
                      <button 
                        className="btn-btn-sm btn-primary"
                        onClick={() => loadConfig(probe.id)}
                        title="编辑配置"
                      >
                        ⚙ 配置
                      </button>
                      <button 
                        className="btn-btn-sm btn-info"
                        onClick={() => loadVersion(probe.id)}
                        title="版本管理"
                      >
                        📦 版本
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        ) : (
          <div className="empty-state">无探针数据</div>
        )}
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
                  <td style={{ fontWeight: 600 }}>{c.name || c}</td>
                  <td><span className={`badge badge-${c.category || 'default'}`}>{c.category || 'default'}</span></td>
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

      {/* 配置编辑模态框 */}
      {showConfig && (
        <div className="modal-overlay">
          <div className="modal" style={{ maxWidth: 700 }}>
            <div className="modal-header">
              <h3>编辑探针配置 - {selectedProbe}</h3>
              <button className="modal-close" onClick={() => setShowConfig(false)}>×</button>
            </div>
            <div className="modal-body" style={{ maxHeight: '60vh', overflowY: 'auto' }}>
              {configLoading ? (
                <div className="empty-state">加载中...</div>
              ) : (
                <div style={{ display: 'grid', gap: 16 }}>
                  {Object.entries(config).map(([key, value]) => (
                    <div key={key}>
                      <label style={{ display: 'block', marginBottom: 4, color: '#94a3b8', fontSize: 13 }}>
                        {key.replace(/_/g, ' ')}
                      </label>
                      {typeof value === 'boolean' ? (
                        <select 
                          value={value ? 'true' : 'false'}
                          onChange={(e) => setConfig({...config, [key]: e.target.value === 'true'})}
                          style={{ width: '100%', padding: 8, background: '#0f172a', color: '#e2e8f0', border: '1px solid #334155', borderRadius: 4 }}
                        >
                          <option value="true">true</option>
                          <option value="false">false</option>
                        </select>
                      ) : typeof value === 'number' ? (
                        <input 
                          type="number" 
                          value={value}
                          onChange={(e) => setConfig({...config, [key]: parseFloat(e.target.value)})}
                          style={{ width: '100%', padding: 8, background: '#0f172a', color: '#e2e8f0', border: '1px solid #334155', borderRadius: 4 }}
                        />
                      ) : (
                        <input 
                          type="text" 
                          value={value || ''}
                          onChange={(e) => setConfig({...config, [key]: e.target.value})}
                          style={{ width: '100%', padding: 8, background: '#0f172a', color: '#e2e8f0', border: '1px solid #334155', borderRadius: 4 }}
                        />
                      )}
                    </div>
                  ))}
                </div>
              )}
            </div>
            <div className="modal-footer">
              <button className="btn btn-primary" onClick={saveConfig}>保存配置</button>
              <button className="btn" onClick={() => setShowConfig(false)}>取消</button>
            </div>
          </div>
        </div>
      )}

      {/* 版本管理模态框 */}
      {showVersion && (
        <div className="modal-overlay">
          <div className="modal" style={{ maxWidth: 600 }}>
            <div className="modal-header">
              <h3>版本管理 - {selectedProbe}</h3>
              <button className="modal-close" onClick={() => setShowVersion(false)}>×</button>
            </div>
            <div className="modal-body">
              {versionInfo ? (
                <div>
                  <div className="stat-card" style={{ marginBottom: 16 }}>
                    <div className="value">{versionInfo.version || '-'}</div>
                    <div className="label">当前版本</div>
                  </div>
                  <div className="stat-card" style={{ marginBottom: 16 }}>
                    <div className="value">{versionInfo.build_time || '-'}</div>
                    <div className="label">构建时间</div>
                  </div>
                  <div style={{ marginBottom: 16 }}>
                    <h4>可用版本</h4>
                    <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                      {['3.1.0', '3.0.0', '2.9.0'].map(v => (
                        <button 
                          key={v}
                          className={`btn ${v === versionInfo.version ? 'btn-success' : 'btn-primary'}`}
                          onClick={() => {
                            if (v !== versionInfo.version) {
                              if (confirm(`确定要${v > versionInfo.version ? '升级' : '回滚'}到版本 ${v} 吗？`)) {
                                v > versionInfo.version ? handleUpgrade() : handleRollback(v);
                              }
                            }
                          }}
                        >
                          {v} {v === versionInfo.version && '(当前)'}
                        </button>
                      ))}
                    </div>
                  </div>
                </div>
              ) : (
                <div className="empty-state">加载中...</div>
              )}
            </div>
            <div className="modal-footer">
              <button 
                className="btn btn-primary" 
                onClick={handleUpgrade}
                disabled={upgradeLoading}
              >
                {upgradeLoading ? '升级中...' : '检查更新'}
              </button>
              <button className="btn" onClick={() => setShowVersion(false)}>关闭</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
