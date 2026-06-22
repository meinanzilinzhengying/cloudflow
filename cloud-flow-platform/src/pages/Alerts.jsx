import { useState, useEffect, useCallback } from 'react';
import { fetchSecurityEvents, fetchEventsByCategory } from '../api';

export default function Alerts() {
  const [events, setEvents] = useState([]);
  const [categories, setCategories] = useState([]);
  const [loading, setLoading] = useState(true);
  const [filter, setFilter] = useState('all');
  const [lastUpdate, setLastUpdate] = useState(null);

  const loadData = useCallback(async () => {
    try {
      const [sec, cat] = await Promise.all([
        fetchSecurityEvents(50).catch(() => []),
        fetchEventsByCategory().catch(() => []),
      ]);
      setEvents(Array.isArray(sec) ? sec : []);
      setCategories(Array.isArray(cat) ? cat : []);
      setLastUpdate(new Date().toLocaleTimeString());
    } catch (e) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadData();
    const interval = setInterval(loadData, 15000);
    return () => clearInterval(interval);
  }, [loadData]);

  const filtered = filter === 'all' ? events : events.filter(e => e.event_type === filter);

  const eventTypes = [...new Set(events.map(e => e.event_type).filter(Boolean))];

  const totalAlerts = events.length;
  const criticalCount = events.filter(e =>
    e.event_type?.includes('block') || e.event_type?.includes('attack') || e.event_type?.includes('malware')
  ).length;

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <h2>安全告警</h2>
        <div style={{ display: 'flex', gap: 16, alignItems: 'center' }}>
          {lastUpdate && <span style={{ color: '#64748b', fontSize: 13 }}>更新于 {lastUpdate}</span>}
          <button onClick={loadData} className="btn-refresh">↻ 刷新</button>
        </div>
      </div>

      <div className="grid-4">
        <div className="stat-card">
          <div className="value">{totalAlerts}</div>
          <div className="label">安全事件总数</div>
        </div>
        <div className="stat-card">
          <div className="value" style={{ color: '#ef4444' }}>{criticalCount}</div>
          <div className="label">高危事件</div>
        </div>
        <div className="stat-card">
          <div className="value">{categories.length}</div>
          <div className="label">事件类别</div>
        </div>
        <div className="stat-card">
          <div className="value">{eventTypes.length}</div>
          <div className="label">事件类型</div>
        </div>
      </div>

      {/* Category Distribution */}
      {categories.length > 0 && (
        <div className="card">
          <h3>事件类别分布</h3>
          <div style={{ display: 'flex', gap: 12, flexWrap: 'wrap' }}>
            {categories.map((c, i) => (
              <div key={i} style={{
                background: '#0f172a',
                border: '1px solid #334155',
                borderRadius: 8,
                padding: '12px 20px',
                textAlign: 'center',
                minWidth: 100,
              }}>
                <div style={{ fontSize: 24, fontWeight: 'bold', color: '#38bdf8' }}>{c.count}</div>
                <div style={{ fontSize: 13, color: '#94a3b8', marginTop: 4 }}>{c.category}</div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Filter */}
      <div className="card">
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
          <h3 style={{ margin: 0 }}>安全事件列表</h3>
          <div style={{ display: 'flex', gap: 8 }}>
            <button
              className={`btn-filter ${filter === 'all' ? 'active' : ''}`}
              onClick={() => setFilter('all')}
            >
              全部 ({events.length})
            </button>
            {eventTypes.slice(0, 5).map((t, i) => (
              <button
                key={i}
                className={`btn-filter ${filter === t ? 'active' : ''}`}
                onClick={() => setFilter(t)}
              >
                {t} ({events.filter(e => e.event_type === t).length})
              </button>
            ))}
          </div>
        </div>

        {loading ? (
          <div className="empty-state">加载中...</div>
        ) : filtered.length > 0 ? (
          <table className="table">
            <thead>
              <tr><th>时间</th><th>类型</th><th>来源IP</th><th>目标IP</th><th>详情</th></tr>
            </thead>
            <tbody>
              {filtered.map((e, i) => (
                <tr key={i}>
                  <td style={{ color: '#64748b', fontSize: 13, whiteSpace: 'nowrap' }}>{e.time}</td>
                  <td>
                    <span className={`badge badge-security ${e.event_type?.includes('block') || e.event_type?.includes('attack') ? 'badge-critical' : ''}`}>
                      {e.event_type}
                    </span>
                  </td>
                  <td style={{ fontFamily: 'monospace', fontSize: 13 }}>{e.src_ip || '-'}</td>
                  <td style={{ fontFamily: 'monospace', fontSize: 13 }}>{e.dst_ip || '-'}</td>
                  <td style={{ color: '#94a3b8', fontSize: 13, maxWidth: 300, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    {typeof e.details === 'string' ? e.details.substring(0, 100) : JSON.stringify(e.details || '').substring(0, 100)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        ) : (
          <div className="empty-state">
            {filter === 'all' ? '暂无安全事件' : '该类型无事件'}
          </div>
        )}
      </div>
    </div>
  );
}
