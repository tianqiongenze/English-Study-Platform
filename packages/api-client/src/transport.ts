// 统一传输层:Web/H5 用 fetch,小程序用 wx.request。
// ApiClient 只依赖 Transport 接口,三端各自注入实现。

export interface TransportRequest {
  method: "GET" | "POST" | "PUT" | "DELETE";
  url: string;
  headers?: Record<string, string>;
  body?: string;
  timeoutMs?: number;
}

export interface TransportResponse {
  status: number;
  body: string;
}

export interface Transport {
  request(req: TransportRequest): Promise<TransportResponse>;
}

export class FetchTransport implements Transport {
  async request(req: TransportRequest): Promise<TransportResponse> {
    const ctrl = new AbortController();
    const timer = req.timeoutMs
      ? setTimeout(() => ctrl.abort(), req.timeoutMs)
      : undefined;
    try {
      const res = await fetch(req.url, {
        method: req.method,
        headers: req.headers,
        body: req.body,
        signal: ctrl.signal,
      });
      const body = await res.text();
      return { status: res.status, body };
    } finally {
      if (timer) clearTimeout(timer);
    }
  }
}

export class ApiError extends Error {
  status: number;
  body?: string;
  constructor(status: number, body: string | undefined, message: string) {
    super(message);
    this.status = status;
    this.body = body;
  }
}
