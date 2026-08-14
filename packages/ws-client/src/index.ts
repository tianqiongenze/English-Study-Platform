// ESP 三端 WS 客户端:重连指数退避 + 心跳 + 发送缓冲。
// 消灭现状:小程序 openSpeechWs 裸 wx.connectSocket 无重连无心跳,
// Web/H5 各自手写 onopen/onmessage。

import type { WsFrame } from "@esp/types";
import { BrowserWsTransport, type WsTransport } from "./transport";

export { BrowserWsTransport } from "./transport";
export type { WsTransport } from "./transport";

export interface WsClientOptions {
  url: string;
  transport?: WsTransport;
  heartbeatIntervalMs?: number;
  heartbeatPayload?: string;
  maxReconnectDelayMs?: number;
  reconnectBaseDelayMs?: number;
  maxBufferedSends?: number;
  onFrame?: (frame: WsFrame) => void;
  onStateChange?: (state: WsState) => void;
}

export type WsState = "connecting" | "open" | "closed" | "reconnecting" | "dead";

export class EspWsClient {
  private transport: WsTransport;
  private opts: Required<
    Pick<
      WsClientOptions,
      | "heartbeatIntervalMs"
      | "heartbeatPayload"
      | "maxReconnectDelayMs"
      | "reconnectBaseDelayMs"
      | "maxBufferedSends"
    >
  > &
    WsClientOptions;
  private state: WsState = "closed";
  private attempts = 0;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private heartbeatTimer: ReturnType<typeof setInterval> | null = null;
  private sendBuffer: string[] = [];
  private manualClose = false;

  constructor(opts: WsClientOptions) {
    this.opts = {
      heartbeatIntervalMs: 15000,
      heartbeatPayload: JSON.stringify({ type: "ping" }),
      maxReconnectDelayMs: 30000,
      reconnectBaseDelayMs: 1000,
      maxBufferedSends: 100,
      ...opts,
    };
    this.transport = opts.transport ?? new BrowserWsTransport();
  }

  connect(): void {
    this.manualClose = false;
    this.setState("connecting");
    this.wire();
    this.transport.connect(this.opts.url);
  }

  send(data: string): void {
    if (this.state === "open") {
      this.transport.send(data);
    } else if (this.sendBuffer.length < this.opts.maxBufferedSends) {
      this.sendBuffer.push(data);
    }
  }

  close(): void {
    this.manualClose = true;
    this.stopTimers();
    this.transport.close(1000, "client_close");
    this.setState("closed");
  }

  getState(): WsState {
    return this.state;
  }

  private wire(): void {
    this.transport.onOpen(() => {
      this.attempts = 0;
      this.setState("open");
      this.startHeartbeat();
      const buf = this.sendBuffer;
      this.sendBuffer = [];
      for (const d of buf) this.transport.send(d);
    });
    this.transport.onMessage((data) => {
      let frame: WsFrame;
      try {
        frame = JSON.parse(data) as WsFrame;
      } catch {
        return;
      }
      if (frame.type === "pong") return;
      this.opts.onFrame?.(frame);
    });
    this.transport.onError(() => {
      // onError 后必触发 onClose,重连逻辑统一放 onClose
    });
    this.transport.onClose(() => {
      this.stopTimers();
      if (this.manualClose) {
        this.setState("closed");
        return;
      }
      this.scheduleReconnect();
    });
  }

  private scheduleReconnect(): void {
    if (this.attempts >= 8) {
      this.setState("dead");
      return;
    }
    const delay = Math.min(
      this.opts.reconnectBaseDelayMs * 2 ** this.attempts,
      this.opts.maxReconnectDelayMs
    );
    this.attempts += 1;
    this.setState("reconnecting");
    this.reconnectTimer = setTimeout(() => {
      this.setState("connecting");
      this.wire();
      this.transport.connect(this.opts.url);
    }, delay);
  }

  private startHeartbeat(): void {
    this.stopHeartbeat();
    this.heartbeatTimer = setInterval(() => {
      this.transport.send(this.opts.heartbeatPayload);
    }, this.opts.heartbeatIntervalMs);
  }

  private stopHeartbeat(): void {
    if (this.heartbeatTimer) {
      clearInterval(this.heartbeatTimer);
      this.heartbeatTimer = null;
    }
  }

  private stopTimers(): void {
    this.stopHeartbeat();
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
  }

  private setState(s: WsState): void {
    this.state = s;
    this.opts.onStateChange?.(s);
  }
}
