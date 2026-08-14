// WS 传输抽象:Web/H5 用浏览器 WebSocket,小程序用 wx.connectSocket。
// 重连/心跳逻辑只写在 EspWsClient,不感知具体传输。

export interface WsTransport {
  connect(url: string): void;
  send(data: string): void;
  close(code?: number, reason?: string): void;
  onOpen(cb: () => void): void;
  onMessage(cb: (data: string) => void): void;
  onClose(cb: (ev: { code: number; reason?: string }) => void): void;
  onError(cb: (err: unknown) => void): void;
}

export class BrowserWsTransport implements WsTransport {
  private ws: WebSocket | null = null;
  connect(url: string): void {
    this.ws = new WebSocket(url);
  }
  send(data: string): void {
    this.ws?.send(data);
  }
  close(code?: number, reason?: string): void {
    this.ws?.close(code, reason);
  }
  onOpen(cb: () => void): void {
    if (this.ws) this.ws.onopen = cb;
  }
  onMessage(cb: (data: string) => void): void {
    if (this.ws) this.ws.onmessage = (e) => cb(e.data as string);
  }
  onClose(cb: (ev: { code: number; reason?: string }) => void): void {
    if (this.ws) this.ws.onclose = (e) => cb({ code: e.code, reason: e.reason });
  }
  onError(cb: (err: unknown) => void): void {
    if (this.ws) this.ws.onerror = cb;
  }
}
