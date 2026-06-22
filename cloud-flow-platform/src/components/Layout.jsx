import { NavLink } from "react-router-dom";

export default function Layout({ children }) {
  return (
    <div className="layout">
      <aside className="sidebar">
        <div className="logo">CloudFlow Platform</div>
        <nav>
          <NavLink to="/" end>平台概览</NavLink>
          <NavLink to="/nodes">节点监控</NavLink>
          <NavLink to="/alerts">平台告警</NavLink>
        </nav>
      </aside>
      <main className="main">{children}</main>
    </div>
  );
}
