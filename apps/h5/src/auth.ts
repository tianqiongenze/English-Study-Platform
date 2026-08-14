import { create } from "zustand";
import type { AuthUser } from "./types";

// P1-13 token 迁移 httpOnly cookie:esp_h5_token 不再写 localStorage(消灭 XSS 窃取面),
// cookie 由 BFF Set-Cookie 下发。token 字段保留内存态(登录响应 body 带回,过渡兼容),刷新后为 null。
const TOKEN_KEY = "esp_h5_token"; // 仅用于清理历史遗留的 localStorage token
const USER_KEY = "esp_h5_user";

function readUser(): AuthUser | null {
  try { const r = localStorage.getItem(USER_KEY); return r ? JSON.parse(r) as AuthUser : null; } catch { return null; }
}

export const useAuth = create<{
  token: string | null; user: AuthUser | null;
  setAuth: (t: string, u: AuthUser) => void; logout: () => void;
}>((set) => ({
  token: null, user: readUser(),
  setAuth: (t, u) => { localStorage.removeItem(TOKEN_KEY); localStorage.setItem(USER_KEY, JSON.stringify(u)); set({ token: t, user: u }); },
  logout: () => { localStorage.removeItem(TOKEN_KEY); localStorage.removeItem(USER_KEY); set({ token: null, user: null }); },
}));

export const authHeader = (t: string | null) => t ? { Authorization: `Bearer ${t}` } : {};
