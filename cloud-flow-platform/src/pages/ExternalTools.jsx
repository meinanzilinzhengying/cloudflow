export default function ExternalTools() {
  const tools = [
    {
      name: 'Grafana',
      desc: '监控仪表盘和可视化',
      url: '/api/grafana/',
      icon: '📊',
      color: '#f46800',
      status: 'available',
    },
    {
      name: 'Prometheus',
      desc: '时序数据库和指标查询',
      url: '/api/prometheus/',
      icon: '📈',
      color: '#e6522c',
      status: 'available',
    },
    {
      name: 'Jaeger',
      desc: '分布式链路追踪',
      url: '/api/jaeger/',
      icon: '🔍',
      color: '#66c0e0',
      status: 'available',
    },
    {
      name: 'AlertManager',
      desc: '告警管理和通知路由',
      url: '/api/alertmanager/',
      icon: '🔔',
      color: '#e89f2e',
      status: 'available',
    },
    {
      name: 'ClickHouse',
      desc: '列式数据库查询',
      url: '/api/clickhouse/play',
      icon: '🗄️',
      color: '#f9d64a',
      status: 'available',
    },
    {
      name: '主监控前端',
      desc: 'CloudFlow 主操作面板',
      url: 'http://' + (window.location.hostname || '192.168.58.130') + ':8080',
      icon: '🖥️',
      color: '#38bdf8',
      status: 'external',
    },
  ];

  return (
    <div>
      <h2 style={{ marginBottom: 24 }}>外部工具</h2>
      <p style={{ color: '#64748b', marginBottom: 24 }}>
        快捷访问 CloudFlow 平台集成的外部运维工具
      </p>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))', gap: 16 }}>
        {tools.map((tool, i) => (
          <a
            key={i}
            href={tool.url}
            target={tool.status === 'external' ? '_blank' : '_self'}
            rel="noopener noreferrer"
            className="tool-card"
            style={{ textDecoration: 'none' }}
          >
            <div className="tool-icon" style={{ background: tool.color + '20', color: tool.color }}>
              {tool.icon}
            </div>
            <div className="tool-info">
              <div className="tool-name">{tool.name}</div>
              <div className="tool-desc">{tool.desc}</div>
            </div>
            <div className="tool-arrow">→</div>
          </a>
        ))}
      </div>

      <div className="card" style={{ marginTop: 24 }}>
        <h3>API 端点总览</h3>
        <table className="table">
          <thead><tr><th>路径</th><th>目标</th><th>说明</th></tr></thead>
          <tbody>
            <tr><td><code>/api/grafana/</code></td><td>localhost:3001</td><td>Grafana 代理</td></tr>
            <tr><td><code>/api/prometheus/</code></td><td>localhost:9091</td><td>Prometheus 代理</td></tr>
            <tr><td><code>/api/jaeger/</code></td><td>localhost:16686</td><td>Jaeger 查询</td></tr>
            <tr><td><code>/api/clickhouse/</code></td><td>localhost:8123</td><td>ClickHouse HTTP</td></tr>
            <tr><td><code>/api/probe/</code></td><td>VM2:9090</td><td>eBPF 探针管理</td></tr>
            <tr><td><code>/api/v1/</code></td><td>VM2:9090</td><td>探针 API v1</td></tr>
            <tr><td><code>/api/ai/</code></td><td>localhost:8082</td><td>AI 分析服务</td></tr>
            <tr><td><code>/api/data-plane/</code></td><td>localhost:9102</td><td>数据面服务</td></tr>
          </tbody>
        </table>
      </div>
    </div>
  );
}
