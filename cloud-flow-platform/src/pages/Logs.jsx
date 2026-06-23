import React, { useState, useEffect, useCallback } from 'react';
import { fetchLogs } from '../api';

export default function Logs() {
  const [logs, setLogs] = useState([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [service, setService] = useState('');
  const [level, setLevel] = useState('');
  const [keyword, setKeyword] = useState('');
  const [start, setStart] = useState('');
  const [end, setEnd] = useState('');
  const [limit, setLimit] = useState(200);

  const LEVELS = ['', 'DEBUG', 'INFO', 'WARN', 'ERROR'];
  const SERVICES = ['', 'ai', 'alert-engine', 'control-plane', 'data-ingest',
                     'edge-health', 'edge', 'link-metrics', 'system-stats', 'data-plane'];

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const params = {};
      if (service) params.service = service;
      if (level)   params.level = level;
      if (keyword) params.keyword = keyword;
      if (start)   params.start = start;
      if (end)     params.end = end;
      params.limit = limit;
      const res = await fetchLogs(params);
      setLogs(res.logs || []);
      setTotal(res.total || 0);
    } catch (e) {
      console.error('Failed to load logs', e);
    } finally {
      setLoading(false);
    }
  }, [service, level, keyword, start, end, limit]);

  useEffect(() => { load(); }, [load]);

  const levelColor = (lv) => ({
    DEBUG: '#9ca3af', INFO: '#60a5fa', WARN: '#fbbf24', ERROR: '#f87171',
  }[lv] || '#9ca3af');

  const formatTime = (ts) => {
    if (!ts) return '-';
    return ts.replace('T', ' ').slice(0, 23);
  };

  return (
    <div style={{ padding: 24, color: '#e2e8f0', height: '100%', display: 'flex', flexDirection: 'column' }}>
      <h2 style={{ margin: '0 0 16px', fontSize: 20 }}>📋 平台日志</h2>

      {/* 筛选栏 */}
      <div style={{
        display: 'flex', gap: 12, flexWrap: 'wrap', marginBottom: 16,
        padding: 16, background: 'rgba(255,255,255,0.05)', borderRadius: 8,
      }}>
        <select value={service} onChange={e => setService(e.target.value)}
          style={{ background: '#1e293b', color: '#e2e8f0', border: '1px solid #334155',
                   borderRadius: 6, padding: '6px 10px', fontSize: 13 }}>
          {SERVICES.map(s => <option key={s} value={s}>{s || '全部服务'}</option>)}
        </select>

        <select value={level} onChange={e => setLevel(e.target.value)}
          style={{ background: '#1e293b', color: '#e2e8f0', border: '1px solid #334155',
                   borderRadius: 6, padding: '6px 10px', fontSize: 13 }}>
          {LEVELS.map(l => <option key={l} value={l}>{l || '全部级别'}</option>)}
        </select>

        <input value={keyword} onChange={e => setKeyword(e.target.value)}
          placeholder="搜索关键字..."
          style={{ flex: 1, minWidth: 200, background: '#1e293b', color: '#e2e8f0',
                   border: '1px solid #334155', borderRadius: 6, padding: '6px 10px', fontSize: 13 }}
        />

        <input type="datetime-local" value={start} onChange={e => setStart(e.target.value)}
          style={{ background: '#1e293b', color: '#e2e8f0', border: '1px solid #334155',
                   borderRadius: 6, padding: '6px 10px', fontSize: 13 }}
        />
        <span style={{ color: '#64748b' }}>~</span>
        <input type="datetime-local" value={end} onChange={e => setEnd(e.target.value)}
          style={{ background: '#1e293b', color: '#e2e8f0', border: '1px solid #334155',
                   borderRadius: 6, padding: '6px 10px', fontSize: 13 }}
        />

        <button onClick={load} disabled={loading}
          style={{ background: loading ? '#334155' : '#3b82f6', color: '#fff',
                   border: 'none', borderRadius: 6, padding: '6px 18px', cursor: 'pointer', fontSize: 13 }}>
          {loading ? '查询中...' : '🔍 查询'}
        </button>
      </div>

      {/* 统计 */}
      <div style={{ marginBottom: 12, fontSize: 13, color: '#64748b' }}>
        共 {total} 条日志
      </div>

      {/* 日志表格 */}
      <div style={{ flex: 1, overflow: 'auto', background: 'rgba(255,255,255,0.03)',
                    borderRadius: 8, border: '1px solid #1e293b' }}>
        {logs.length === 0 ? (
          <div style={{ textAlign: 'center', padding: 40, color: '#64748b' }}>
            {loading ? '加载中...' : '暂无日志数据'}
          </div>
        ) : (
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
            <thead>
              <tr style={{ background: 'rgba(255,255,255,0.05)', position: 'sticky', top: 0 }}>
                <th style={{ padding: '8px 12px', textAlign: 'left', borderBottom: '1px solid #1e293b', width: 180 }}>时间</th>
                <th style={{ padding: '8px 12px', textAlign: 'left', borderBottom: '1px solid #1e293b', width: 120 }}>服务</th>
                <th style={{ padding: '8px 12px', textAlign: 'left', borderBottom: '1px solid #1e293b', width: 80 }}>级别</th>
                <th style={{ padding: '8px 12px', textAlign: 'left', borderBottom: '1px solid #1e293b' }}>消息</th>
              </tr>
            </thead>
            <tbody>
              {logs.map((log, i) => (
                <tr key={i} style={{ borderBottom: '1px solid rgba(255,255,255,0.03)',
                                       background: i % 2 === 0 ? 'transparent' : 'rgba(255,255,255,0.02)' }}>
                  <td style={{ padding: '7px 12px', fontFamily: 'monospace', fontSize: 12, whiteSpace: 'nowrap' }}>
                    {formatTime(log.timestamp)}
                  </td>
                  <td style={{ padding: '7px 12px', color: '#60a5fa' }}>{log.service}</td>
                  <td style={{ padding: '7px 12px' }}>
                    <span style={{ color: levelColor(log.level), fontWeight: 600, fontSize: 12 }}>
                      {log.level}
                    </span>
                  </td>
                  <td style={{ padding: '7px 12px', fontFamily: 'monospace', fontSize: 12,
                                  whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>
                    {log.message}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}
