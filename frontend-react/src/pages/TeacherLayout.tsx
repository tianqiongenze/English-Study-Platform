import { NavLink, Outlet, useNavigate } from "react-router-dom";
import { useAuth } from "../auth";

export default function TeacherLayout() {
  const { user, logout } = useAuth();
  const nav = useNavigate();
  return (
    <div className="app">
      <header>
        <h1>ESP · 教师端</h1>
        <nav className="tabs">
          <NavLink to="/teacher/lesson-plan" className={({ isActive }) => `tab ${isActive ? "active" : ""}`}>教案生成</NavLink>
          <NavLink to="/teacher/analytics" className={({ isActive }) => `tab ${isActive ? "active" : ""}`}>学情分析</NavLink>
          <NavLink to="/teacher/grading" className={({ isActive }) => `tab ${isActive ? "active" : ""}`}>作业评阅</NavLink>
        </nav>
        <div className="user">
          <span>{user?.name} (教师)</span>
          <button onClick={() => { logout(); nav("/auth/login"); }}>退出</button>
        </div>
      </header>
      <div className="panel"><Outlet /></div>
    </div>
  );
}
