export default function Dashboard() {
  return (
    <div>
      <h2 style={{ marginBottom: 24 }}>平台自监控 - 概览</h2>
      <div className="grid-4">
        <div className="stat-card">
          <div className="value">12</div>
          <div className="label">活跃节点</div>
        </div>
        <div className="stat-card">
          <div className="value">98.5%</div>
          <div className="label">整体可用率</div>
        </div>
        <div className="stat-card">
          <div className="value">3</div>
          <div className="label">待处理告警</div>
        </div>
        <div className="stat-card">
          <div className="value">1.2ms</div>
          <div className="label">平均延迟</div>
        </div>
      </div>
      <div className="card">
        <h3>服务健康状态</h3>
        <table className="table">
          <thead><tr><th>服务</th><th>状态</th><th>实例数</th><th>CPU</th><th>内存</th></tr></thead>
          <tbody>
            <tr><td>alert-engine</td><td className="status-online">● 正常</td><td>2</td><td>12%</td><td>256MB</td></tr>
            <tr><td>data-plane</td><td className="status-online">● 正常</td><td>3</td><td>8%</td><td>128MB</td></tr>
            <tr><td>query-service</td><td className="status-online">● 正常</td><td>2</td><td>15%</td><td>512MB</td></tr>
            <tr><td>control-plane</td><td className="status-warning">● 降级</td><td>1</td><td>45%</td><td>1.2GB</td></tr>
          </tbody>
        </table>
      </div>
      <div className="card">
        <h3>Leader 选举状态</h3>
        <table className="table">
          <thead><tr><th>服务</th><th>当前 Leader</th><th>租约到期</th></tr></thead>
          <tbody>
            <tr><td>alert-engine</td><td>node-a1b2c3</td><td>30s</td></tr>
          </tbody>
        </table>
      </div>
    </div>
  );
}
