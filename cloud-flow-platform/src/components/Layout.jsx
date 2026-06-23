import { NavLink } from "react-router-dom";

const navItems = [
  { to: "/",          label: "平台概览", icon: "📊", end: true },
  { to: "/topology",  label: "服务拓扑", icon: "🗼️" },
  { to: "/services",  label: "服务健康", icon: "💚" },
  { to: "/alerts",    label: "平台告警", icon: "🔔" },
  { to: "/logs",      label: "平台日志", icon: "📋" },
  { to: "/config",    label: "配置管理", icon: "⚙️" },
  { to: "/ai",        label: "AI 分析", icon: "🤖" },
  { to: "/tools",     label: "外部工具", icon: "🔧" },
];

export default function Layout({ children }) {
  return (
    <div className="layout">
      <aside className="sidebar">
        <div className="logo">☁️ CloudFlow Platform</div>
        <nav>
          {navItems.map((item) => (
            <NavLink key={item.to} to={item.to} end={item.end}>
              <span className="nav-icon">{item.icon}</span>
              {item.label}
            </NavLink>
          ))}
        </nav>
        <div className="sidebar-footer">
          <div style={{ fontSize: 12, color: '#475569' }}>v3.2.0</div>
          <div style={{ fontSize: 12, color: '#475569' }}>平台自监控</div>
        </div>
      </aside>
      <main className="main">{children}</main>
    </div>
  );
}
