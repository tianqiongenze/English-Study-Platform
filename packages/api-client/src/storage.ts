// token 存储适配器:Web/H5 用 localStorage,小程序用 wx.storage。
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
