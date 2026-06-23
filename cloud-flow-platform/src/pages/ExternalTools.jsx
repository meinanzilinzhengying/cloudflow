import { useState, useEffect } from 'react';

// 平台实际可用的工具（基于VM1当前运行的服务）
const TOOLS = [
  {
    name: 'ClickHouse 查询',
    desc: '列式数据库，平台核心存储',
    icon: '🗄️',
    color: '#f9d64a',
    url: '/api/clickhouse/',
    internal: true,
    category: 'storage',
  },
  {
    name: '主监控面板',
    desc: 'CloudFlow 业务监控前端 (8080)',
    icon: '🖥️',
    color: '#38bdf8',
    url: () => 'http://' + (window.location.hostname || '192.168.58.130') + ':8080',
    internal: false,
    category: 'frontend',
  },
  {
    name: 'AI 分析服务',
    desc: 'AI 智能分析接口 (8082)',
    icon: '🤖',
    color: '#a78bfa',
    url: '/api/ai/v1/models',
    internal: true,
    category: 'control',
  },
  {
    name: 'eBPF 探针 API',
    desc: '探针管理接口 (VM2:9090)',
    icon: '📡',
    color: '#3b82f6',
    url: '/api/probe/status',
    internal: true,
    category: 'external',
  },
  {
    name: '数据面服务',
    desc: 'Edge 数据面 (9102)',
    icon: '🔀',
    color: '#10b981',
    url: '/api/data-plane/health',
    internal: true,
    category: 'control',
  },
];

// 实际存在的 API 路由（基于 nginx 配置）
const API_ENDPOINTS = [
  { path: '/api/system/',     target: ':9099',   desc: '系统资源采集' },
  { path: '/api/link/',       target: ':9105',   desc: '链路指标采集' },
  { path: '/api/clickhouse/', target: ':8123',   desc: 'ClickHouse HTTP' },
  { path: '/api/probe/',      target: 'VM2:9090', desc: 'eBPF 探针管理' },
  { path: '/api/v1/',         target: 'VM2:9090', desc: '探针数据查询 API' },
  { path: '/api/ai/',         target: ':8082',    desc: 'AI 分析服务' },
  { path: '/api/data-plane/', target: ':9102',    desc: '数据面服务' },
];

export default function ExternalTools() {
  const [statuses, setStatuses] = useState({});

  // 检测各工具可达性
  useEffect(() => {
    const checks = TOOLS.map(async (tool) => {
      try {
        const url = typeof tool.url === 'function' ? tool.url() : tool.url;
        const res = await fetch(url, {
          method: 'HEAD',
          timeout: 3000,
          signal: AbortSignal.timeout(3000),
        });
        return [tool.name, res.ok ? 'up' : 'degraded'];
      } catch {
        return [tool.name, 'unknown'];
      }
    });

    Promise.all(checks).then((results) => {
      setStatuses(Object.fromEntries(results));
    });
  }, []);

  const statusIcon = (s) => {
    switch (s) {
      case 'up': return '●';
      case 'degraded': return '◐';
      default: return '○';
    }
  };
  const statusColor = (s) => {
    switch (s) {
      case 'up': return '#22c55e';
      case 'degraded': return '#f59e0b';
      default: return '#64748b';
    }
  };

  const catLabel = {
    storage: '数据存储',
    frontend: '前端界面',
    control: '控制面服务',
    external: '外部节点',
  };

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <h2>外部工具</h2>
        <button onClick={() => window.location.reload()} className="btn-refresh">↻ 检测状态</button>
      </div>
      <p style={{ color: '#64748b', marginBottom: 24 }}>
        CloudFlow 平台集成的运维工具快捷入口（仅显示实际运行中的服务）
      </p>

      {/* 工具卡片 */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(300px, 1fr))', gap: 16 }}>
        {TOOLS.map((tool, i) => {
          const st = statuses[tool.name] || 'checking';
          const toolUrl = typeof tool.url === 'function' ? tool.url() : tool.url;
          return (
            <a
              key={i}
              href={toolUrl}
              target={tool.internal ? '_self' : '_blank'}
              rel="noopener noreferrer"
              className="tool-card"
              style={{ textDecoration: 'none', opacity: st === 'unknown' ? 0.6 : 1 }}
            >
              <div style={{ display: 'flex', alignItems: 'center', gap: 14 }}>
                <div style={{
                  fontSize: 32,
                  width: 52, height: 52,
                  borderRadius: 12,
                  background: tool.color + '18',
                  display: 'flex', alignItems: 'center', justifyContent: 'center',
                }}>
                  {tool.icon}
                </div>
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                    <span style={{ fontWeight: 600, fontSize: 15 }}>{tool.name}</span>
                    <span style={{ fontSize: 11, color: statusColor(st), fontWeight: 600 }}>{statusIcon(st)} {st === 'up' ? '在线' : st === 'degraded' ? '异常' : '检测中'}</span>
                  </div>
                  <div style={{ fontSize: 13, color: '#94a3b8', marginTop: 2 }}>{tool.desc}</div>
                  <div style={{ fontSize: 11, color: '#64748b', marginTop: 4 }}>
                    <span style={{
                      background: '#1e293a', padding: '1px 8px', borderRadius: 4,
                      border: '1px solid #334155',
                    }}>{catLabel[tool.category] || tool.category}</span>
                  </div>
                </div>
                <div style={{ color: '#475569', fontSize: 18 }}>→</div>
              </div>
            </a>
          );
        })}
      </div>

      {/* API 端点总览 */}
      <div className="card" style={{ marginTop: 28 }}>
        <h3>平台 API 路由总览</h3>
        <p style={{ color: '#94a3b8', fontSize: 13, margin: '0 0 16px' }}>
          以下为 3003 端口 Nginx 反向代理配置的实际路由，均映射到 VM1 本地或 VM2 的后端服务
        </p>
        <table className="table">
          <thead><tr><th>路由路径</th><th>代理目标</th><th>说明</th></tr></thead>
          <tbody>
            {API_ENDPOINTS.map((ep, i) => (
              <tr key={i}>
                <td>
                  <code style={{
                    background: '#0f172a', padding: '2px 8px', borderRadius: 4,
                    fontSize: 13, color: '#38bdf8',
                  }}>{ep.path}</code>
                </td>
                <td style={{ fontFamily: 'monospace', fontSize: 13 }}>{ep.target}</td>
                <td style={{ color: '#94a3b8', fontSize: 13 }}>{ep.desc}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {/* 说明 */}
      <div className="card" style={{ marginTop: 16, background: 'rgba(56,189,248,0.04)', borderColor: 'rgba(56,189,248,0.15)' }}>
        <div style={{ fontSize: 13, color: '#94a3b8', lineHeight: 1.7 }}>
          <b style={{ color: '#38bdf8' }}>说明：</b>
          Grafana / Prometheus / Jaeger / AlertManager 等 Docker 监控栈服务当前未启动，
          故不在此页面显示。如需启用这些服务，请在 VM1 上启动对应的 Docker 容器。
          此页面仅展示<strong>当前实际运行中</strong>的平台服务。
        </div>
      </div>
    </div>
  );
}
