import { useAuth } from "../auth";
import { useNavigate } from "react-router-dom";

export default function Profile() {
  const { user, logout } = useAuth();
  const nav = useNavigate();
  return (
    <div className="app">
      <h1>个人中心</h1>
      <pre>{JSON.stringify(user, null, 2)}</pre>
      <button onClick={() => { logout(); nav("/auth/login"); }}>退出登录</button>
    </div>
  );
}
