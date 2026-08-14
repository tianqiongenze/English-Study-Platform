// token 存储适配器:Web/H5 用 httpOnly cookie(P1-13),小程序用 wx.storage,本地开发可回落 localStorage。
// 应用启动时注入实现,api-client 本体不感知宿主环境。

export interface TokenStorage {
  getToken(): string | null;
  setToken(token: string): void;
  clearToken(): void;
}

export class LocalStorageTokenStorage implements TokenStorage {
  constructor(private tokenKey: string, private userKey?: string) {}
  getToken(): string | null {
    return localStorage.getItem(this.tokenKey);
  }
  setToken(token: string): void {
    localStorage.setItem(this.tokenKey, token);
  }
  clearToken(): void {
    localStorage.removeItem(this.tokenKey);
    if (this.userKey) localStorage.removeItem(this.userKey);
  }
}

// P1-13 httpOnly cookie 模式(Web/H5):JWT 存 HttpOnly cookie,JS 不可读 → 消灭 XSS 窃取面。
// - getToken 恒 null:ApiClient 不注入 Authorization header,请求靠浏览器同源自动携带 cookie
//   (BFF/网关侧已兼容:无 Bearer 时从 esp_token cookie 验签)。
// - setToken no-op:cookie 由 BFF login/register 响应 Set-Cookie 下发。
// - clearToken no-op:HttpOnly cookie JS 无法清除,注销须调 POST /api/auth/logout(服务端 Max-Age=0)。
export class CookieTokenStorage implements TokenStorage {
  getToken(): string | null {
    return null; // token 在 httpOnly cookie 里,JS 读不到(这正是安全收益)
  }
  setToken(_token: string): void {
    /* cookie 由服务端 Set-Cookie 下发,客户端不写 */
  }
  clearToken(): void {
    /* HttpOnly cookie 不可由 JS 清除;调 POST /api/auth/logout 由服务端清 */
  }
}
