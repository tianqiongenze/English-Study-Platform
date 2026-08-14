import { NavLink, Outlet, useNavigate } from "react-router-dom";
import { useAuth } from "../auth";

export default function LearningLayout() {
  const { user, logout } = useAuth();
  const nav = useNavigate();
  return (
    <div className="app">
      <header>
        <h1>ESP · 学习中心</h1>
        <nav className="tabs">
          <NavLink to="/learning/speech" className={({ isActive }) => `tab ${isActive ? "active" : ""}`}>口语陪练</NavLink>
          <NavLink to="/learning/writing" className={({ isActive }) => `tab ${isActive ? "active" : ""}`}>写作批改</NavLink>
          <NavLink to="/learning/knowledge" className={({ isActive }) => `tab ${isActive ? "active" : ""}`}>自适应学习</NavLink>
          <NavLink to="/learning/review" className={({ isActive }) => `tab ${isActive ? "active" : ""}`}>复习清单</NavLink>
          <NavLink to="/learning/difficulty" className={({ isActive }) => `tab ${isActive ? "active" : ""}`}>动态难度</NavLink>
        </nav>
        <div className="user">
          <span>{user?.name} ({user?.role})</span>
          <button onClick={() => { logout(); nav("/auth/login"); }}>退出</button>
        </div>
      </header>
      <div className="panel"><Outlet /></div>
    </div>
  );
}
