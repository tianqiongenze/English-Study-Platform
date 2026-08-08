import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// BFF (8090) 聚合 REST 鉴权;WS 旁路直连 gateway-go (ADR-13)。
// 单服务本地开发时,可把 /api 指向 gateway-go:8080(暂未启动 BFF)。
const API_TARGET = process.env.VITE_API_TARGET || "http://localhost:8090";
const WS_TARGET = process.env.VITE_WS_TARGET || "ws://localhost:8080";

export default defineConfig({
  plugins: [react()],
  server: {
    port: 3000,
    proxy: {
      "/api": API_TARGET,
      "/ws": { target: WS_TARGET, ws: true },
    },
  },
});
