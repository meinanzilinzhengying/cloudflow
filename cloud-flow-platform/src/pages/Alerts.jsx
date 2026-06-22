export default function Alerts() {
  const alerts = [
    { id: "ALT-001", service: "control-plane", severity: "warning", msg: "CPU 使用率超过 40%", time: "2024-01-15 10:30" },
    { id: "ALT-002", service: "query-service", severity: "critical", msg: "ClickHouse 连接断开", time: "2024-01-15 09:15" },
  ];

  return (
    <div>
      <h2 style={{ marginBottom: 24 }}>平台告警</h2>
      <div className="card">
        <h3>活跃告警</h3>
        <table className="table">
          <thead>
            <tr><th>告警ID</th><th>服务</th><th>级别</th><th>内容</th><th>时间</th></tr>
          </thead>
          <tbody>
            {alerts.map(a => (
              <tr key={a.id}>
                <td>{a.id}</td>
                <td>{a.service}</td>
                <td className={a.severity === "critical" ? "status-offline" : "status-warning"}>
                  {a.severity}
                </td>
                <td>{a.msg}</td>
                <td>{a.time}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
