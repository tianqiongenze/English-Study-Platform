import { NavLink, Outlet, useNavigate } from "react-router-dom";
import { useAuth } from "../auth";

export default function Home() {
  const { user, logout } = useAuth(); const nav = useNavigate();
  return (
    <div className="app">
      <header>
        <span className="title">ESP</span>
        <span style={{ fontSize: "0.8rem" }}>{user?.name} <button onClick={() => { logout(); nav("/login"); }}>退出</button></span>
      </header>
      <nav className="tabs">
        <NavLink to="/speech" className={({ isActive }) => `tab ${isActive ? "active" : ""}`}>口语</NavLink>
        <NavLink to="/writing" className={({ isActive }) => `tab ${isActive ? "active" : ""}`}>写作</NavLink>
        <NavLink to="/learning" className={({ isActive }) => `tab ${isActive ? "active" : ""}`}>学习</NavLink>
        <NavLink to="/profile" className={({ isActive }) => `tab ${isActive ? "active" : ""}`}>我的</NavLink>
      </nav>
      <Outlet />
    </div>
  );
}
