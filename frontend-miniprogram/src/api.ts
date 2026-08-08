// 小程序统一 API + WS 客户端。wx.login → 后端换 JWT(小程序专用鉴权链路,ADR-14/集成 INT)。
import { traceparent } from "./trace";

const API = TARO_APP_API || "http://localhost:8090";

function getToken(): string { return wx.getStorageSync("esp_token") || ""; }
export function setToken(t: string) { wx.setStorageSync("esp_token", t); }
export function getUser() { const r = wx.getStorageSync("esp_user"); return r ? JSON.parse(r) : null; }
export function setUser(u: unknown) { wx.setStorageSync("esp_user", JSON.stringify(u)); }
export function logout() { wx.removeStorageSync("esp_token"); wx.removeStorageSync("esp_user"); }

export async function request<T>(path: string, init: { method?: string; data?: unknown } = {}): Promise<T> {
  const res = await new Promise<{ statusCode: number; data: T }>((resolve, reject) => {
    wx.request({
      url: `${API}${path}`,
      method: (init.method as any) || "GET",
      data: init.data,
      header: { "Content-Type": "application/json", Authorization: `Bearer ${getToken()}`, traceparent: traceparent() },
      success: resolve, fail: reject,
    });
  });
  if (res.statusCode >= 400) throw new Error(`${res.statusCode}`);
  return res.data;
}

export const api = {
  loginByCode: (code: string) => request<{ token: string; user: { id: number; name: string; role: string } }>("/api/auth/wx-login", { method: "POST", data: { code } }),
  scenarios: () => request<{ code: string; title: string }[]>("/api/scenarios"),
  createSession: (code: string) => request<{ session_id: string }>(`/api/sessions`, { method: "POST", data: { scenario_code: code } }),
  reportEvent: (uid: number) => request<{ accepted: boolean }>("/api/learning/event", { method: "POST", data: { user_id: uid, event_type: "answer", payload: "{}" } }),
  review: (uid: number) => request<unknown[]>(`/api/learning/review?user_id=${uid}`),
  weak: (uid: number) => request<unknown[]>(`/api/learning/mastery/weak?user_id=${uid}`),
  grade: (essay: string) => request<{ score: number; feedback: string }>("/api/writing/grade", { method: "POST", data: { prompt: "Describe your day", essay, exam: "toefl" } }),
  // 润色 / 翻译
  polish: (text: string) => request<{ polished: string; suggestions: { original: string; polished: string; reason: string }[] }>("/api/writing/polish", { method: "POST", data: { text } }),
  translate: (text: string) => request<{ translation: string; vocab: { word: string; pos: string; meaning: string; example: string }[] }>("/api/writing/translate", { method: "POST", data: { text } }),
  // FR-2.1 初测定级 + FR-2.2 动态难度
  placement: (uid: number, kp_ids: number[] = [], score = 0) => request<{ initialized: number }>("/api/learning/placement", { method: "POST", data: { user_id: uid, kp_ids, score } }),
  difficulty: (current: number, correct: boolean) => request<{ next_difficulty: number; speed_multiplier: number; scaffolding: boolean }>("/api/learning/difficulty", { method: "POST", data: { current, correct } }),
  // FR-5.2 视觉辅助 + FR-6.2 LangGraph 写作智能体(human-in-loop)
  visualWord: (word: string, context = "") => request<{ image_url: string; alt: string }>("/api/emotion/visual", { method: "POST", data: { word, context } }),
  senseEmotion: (text: string) => request<{ emotion: string; encouragement: string }>("/api/emotion/sense", { method: "POST", data: { text } }),
  writingRun: (essay: string, human = false) => request<{ thread_id: string; status: string; notes?: string }>("/api/writing/run", { method: "POST", data: { prompt: "Describe your day", essay, exam: "toefl", human } }),
  writingState: (thread_id: string) => request<{ status: string; notes?: string }>(`/api/writing/state/${thread_id}`),
  // 用户行为上报 + 推荐(ADR-observability-rebuild §4)
  reportBehavior: (uid: number, action: string, target?: string, duration_ms?: number, context?: Record<string, unknown>) =>
    request<{ accepted: boolean }>("/api/learning/behavior", { method: "POST", data: { user_id: uid, action, target, duration_ms, context } }),
  recommend: (uid: number) => request<{ review: unknown[]; weak_points: unknown[]; similar_scenarios: unknown[] }>(`/api/learning/recommend?user_id=${uid}`),
};

// 小程序 WS:Taro.connectSocket(wx)
export function openSpeechWs(sessionId: string, onMsg: (f: any) => void): WebSocket {
  const ws: any = wx.connectSocket({ url: `${TARO_APP_WS || "ws://localhost:8080"}/ws/speech/${sessionId}` });
  ws.onMessage((e: any) => { try { onMsg(JSON.parse(e.data)); } catch {} });
  return ws;
}
