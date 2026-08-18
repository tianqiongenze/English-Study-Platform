// 小程序 API 层 —— 薄委托到 @esp/api-client(P2-17)+ wx.request 传输适配。
// 历史漂移修正:grade/writingRun 原硬编码 prompt="Describe your day"+exam="toefl",
// 现由调用方显式传参;reportEvent 原硬编码 event_type="answer"。
import { ApiClient, type Transport, type TokenStorage } from "@esp/api-client";
import { traceparent } from "./trace";

const API = TARO_APP_API || "http://localhost:8090";

// wx.request → Transport 适配
const wxTransport: Transport = {
  request(req) {
    return new Promise((resolve, reject) => {
      wx.request({
        url: req.url,
        method: req.method as any,
        data: req.body ? JSON.parse(req.body) : undefined,
        header: { ...req.headers, traceparent: traceparent() },
        success: (res: any) => resolve({ status: res.statusCode, body: typeof res.data === "string" ? res.data : JSON.stringify(res.data) }),
        fail: reject,
      });
    });
  },
};

// P1-13 说明:小程序保持 Bearer token 方案不变 —— wx.storage 无 localStorage 式 XSS 窃取面
// (沙箱隔离、无跨站脚本注入路径),且 wx.request 不自动管理 cookie。token 走 Authorization header
// + WS ?token= 查询参数,服务端 Bearer 优先校验,与 cookie 模式(Web/H5)并存。
const wxStorage: TokenStorage = {
  getToken: () => wx.getStorageSync("esp_token") || null,
  setToken: (t) => wx.setStorageSync("esp_token", t),
  clearToken: () => { wx.removeStorageSync("esp_token"); wx.removeStorageSync("esp_user"); },
};

export const client = new ApiClient({ baseUrl: API, transport: wxTransport, storage: wxStorage });

// 兼容旧导出(页面直接引用这些函数)
export function setToken(t: string) { wxStorage.setToken(t); }
export function getUser() { const r = wx.getStorageSync("esp_user"); return r ? JSON.parse(r) : null; }
export function setUser(u: unknown) { wx.setStorageSync("esp_user", JSON.stringify(u)); }
export function logout() { wxStorage.clearToken(); }
export async function request<T>(path: string, init: { method?: string; data?: unknown } = {}): Promise<T> {
  return client.request(init.method as any || "GET", path, init.data) as Promise<T>;
}

export const api = {
  // wx 专属登录链路保留本地实现
  loginByCode: (code: string) =>
    client.request<{ token: string; user: { id: number; name: string; role: string } }>(
      "POST", "/api/auth/wx-login", { code }, { auth: false }
    ),
  scenarios: client.scenarios.bind(client),
  createSession: client.createSession.bind(client),
  sessionHistory: () => client.sessionHistory(),
  // 显式参数(原硬编码 answer/"{}",调用方显式传)
  reportEvent: client.reportEvent.bind(client),
  review: client.review.bind(client),
  weak: client.weak.bind(client),
  grade: client.grade.bind(client),
  polish: client.polish.bind(client),
  translate: client.translate.bind(client),
  placement: client.placement.bind(client),
  difficulty: client.difficulty.bind(client),
  visualWord: client.visualWord.bind(client),
  senseEmotion: client.senseEmotion.bind(client),
  writingRun: client.writingRun.bind(client),
  writingState: client.writingState.bind(client),
  reportBehavior: client.reportBehavior.bind(client),
  recommend: client.recommend.bind(client),
  // 数字人 persona(功能拓展:per-user 形象,跨设备持久化)
  getDigitalHuman: () => client.getDigitalHuman(),
  setDigitalHuman: (persona_id: string) => client.setDigitalHuman(persona_id),
};

// 小程序 WS:EspWsClient(P2-17,重连+心跳) + wx.connectSocket 传输适配
import { EspWsClient, type WsTransport } from "@esp/ws-client";

class WxSocketTransport implements WsTransport {
  private task: any = null;
  private openCb: (() => void)[] = [];
  private msgCb: ((d: string) => void)[] = [];
  private closeCb: ((ev: { code: number }) => void)[] = [];
  private errCb: ((e: unknown) => void)[] = [];
  connect(url: string): void {
    this.task = wx.connectSocket({ url });
    this.task.onOpen(() => this.openCb.forEach((cb) => cb()));
    this.task.onMessage((e: any) => this.msgCb.forEach((cb) => cb(e.data as string)));
    this.task.onClose((e: any) => this.closeCb.forEach((cb) => cb({ code: e?.code ?? 1006 })));
    this.task.onError((e: any) => this.errCb.forEach((cb) => cb(e)));
  }
  send(data: string): void { this.task?.send({ data }); }
  close(): void { this.task?.close({}); }
  onOpen(cb: () => void): void { this.openCb.push(cb); }
  onMessage(cb: (d: string) => void): void { this.msgCb.push(cb); }
  onClose(cb: (ev: { code: number }) => void): void { this.closeCb.push(cb); }
  onError(cb: (e: unknown) => void): void { this.errCb.push(cb); }
}

export function openSpeechWs(sessionId: string, onMsg: (f: any) => void): EspWsClient {
  // P0-5 修复(2026-08-16 小程序契约核查):gateway-go /ws/speech 强制鉴权(?token= 或 cookie),
  // 原实现未带 token → 口语 WS 握手必 403,链路断。按 P1-13 设计补 ?token=(wx.request 不自动带 cookie)。
  const token = wxStorage.getToken();
  const wsBase = TARO_APP_WS || "ws://localhost:8080";
  const url = token
    ? `${wsBase}/ws/speech/${sessionId}?token=${encodeURIComponent(token)}`
    : `${wsBase}/ws/speech/${sessionId}`;
  const c = new EspWsClient({ url, transport: new WxSocketTransport(), onFrame: (f) => onMsg(f) });
  c.connect();
  return c;
}
