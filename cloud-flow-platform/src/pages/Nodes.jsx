export default function Nodes() {
  const nodes = [
    { id: "node-01", ip: "192.168.1.10", role: "agent", status: "online", cpu: 15, mem: 60 },
    { id: "node-02", ip: "192.168.1.11", role: "agent", status: "online", cpu: 22, mem: 70 },
    { id: "node-03", ip: "192.168.1.12", role: "edge", status: "offline", cpu: 0, mem: 0 },
  ];

  return (
    <div>
      <h2 style={{ marginBottom: 24 }}>节点监控</h2>
      <div className="card">
        <h3>探针节点列表</h3>
        <table className="table">
          <thead>
            <tr><th>节点ID</th><th>IP地址</th><th>角色</th><th>状态</th><th>CPU</th><th>内存</th></tr>
          </thead>
          <tbody>
            {nodes.map(n => (
              <tr key={n.id}>
                <td>{n.id}</td>
                <td>{n.ip}</td>
                <td>{n.role}</td>
                <td className={n.status === "online" ? "status-online" : "status-offline"}>
                  {n.status === "online" ? "● 在线" : "● 离线"}
                </td>
                <td>{n.cpu}%</td>
                <td>{n.mem}%</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
