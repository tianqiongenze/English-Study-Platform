import { create } from "zustand";
import type { AuthUser } from "./types";

const TOKEN_KEY = "esp_h5_token";
const USER_KEY = "esp_h5_user";

function readUser(): AuthUser | null {
  try { const r = localStorage.getItem(USER_KEY); return r ? JSON.parse(r) as AuthUser : null; } catch { return null; }
}

export const useAuth = create<{
  token: string | null; user: AuthUser | null;
  setAuth: (t: string, u: AuthUser) => void; logout: () => void;
}>((set) => ({
  token: localStorage.getItem(TOKEN_KEY), user: readUser(),
  setAuth: (t, u) => { localStorage.setItem(TOKEN_KEY, t); localStorage.setItem(USER_KEY, JSON.stringify(u)); set({ token: t, user: u }); },
  logout: () => { localStorage.removeItem(TOKEN_KEY); localStorage.removeItem(USER_KEY); set({ token: null, user: null }); },
}));

export const authHeader = (t: string | null) => t ? { Authorization: `Bearer ${t}` } : {};
