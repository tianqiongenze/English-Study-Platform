import { create } from "zustand";
import type { AuthUser } from "./types";

// P1-13 token 迁移 httpOnly cookie:esp_token 由 BFF Set-Cookie 下发(HttpOnly,JS 读不到),
// 不再写 localStorage(消灭 XSS 窃取面)。token 字段保留在 zustand 内存中(登录响应 body 带回,
// 过渡兼容 authHeader 等旧用法),但刷新页面后为 null —— 鉴权完全靠浏览器自动带 cookie。
const TOKEN_KEY = "esp_token"; // 仅用于清理历史遗留的 localStorage token
const USER_KEY = "esp_user";

interface AuthState {
  token: string | null;
  user: AuthUser | null;
  setAuth: (token: string, user: AuthUser) => void;
  logout: () => void;
  restore: () => void;
}

function readUser(): AuthUser | null {
  try {
    const raw = localStorage.getItem(USER_KEY);
    return raw ? (JSON.parse(raw) as AuthUser) : null;
  } catch {
    return null;
  }
}

export const useAuth = create<AuthState>((set) => ({
  token: null,
  user: readUser(),
  setAuth: (token, user) => {
    // token 仅存内存(cookie 模式下不再落 localStorage);user 仍持久化用于刷新后恢复界面
    localStorage.removeItem(TOKEN_KEY); // 清历史遗留
    localStorage.setItem(USER_KEY, JSON.stringify(user));
    set({ token, user });
  },
  logout: () => {
    // 服务端 Set-Cookie Max-Age=0 清 httpOnly cookie(POST /api/auth/logout 由调用方发起)
    localStorage.removeItem(TOKEN_KEY);
    localStorage.removeItem(USER_KEY);
    set({ token: null, user: null });
  },
  restore: () => set({ token: null, user: readUser() }),
}));

export function authHeader(token: string | null): Record<string, string> {
  return token ? { Authorization: `Bearer ${token}` } : {};
}
