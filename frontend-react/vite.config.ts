import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// BFF (8090) 聚合 REST 鉴权;WS 旁路直连 gateway-go (ADR-13)。
// 单服务本地开发时,可把 /api 指向 gateway-go:8080(暂未启动 BFF)。
// 子路径部署:portal nginx 把 /web/ 剥前缀反代到 react 容器,但浏览器仍从 /web/ 前缀
// 加载主文档,所以 Vite 产物的资源引用必须带 /web/ 前缀,否则请求 /assets/* 命中不到
// portal 任何 location → 404 → 白屏。base='/web/' 让 dist/index.html 引用 /web/assets/*。
const API_TARGET = process.env.VITE_API_TARGET || "http://localhost:8090";
const WS_TARGET = process.env.VITE_WS_TARGET || "ws://localhost:8080";

export default defineConfig({
  base: "/web/",
  plugins: [react()],
  server: {
    port: 3000,
    proxy: {
      "/api": API_TARGET,
      "/ws": { target: WS_TARGET, ws: true },
    },
  },
});
