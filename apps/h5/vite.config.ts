import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { VitePWA } from "vite-plugin-pwa";

// H5 移动端:学习者全功能,教师只读。经 BFF(8090) 鉴权,WS 旁路 gateway-go。
// 子路径部署:portal nginx 把 /h5/ 剥前缀反代到 h5 容器,Vite 产物资源必须带 /h5/ 前缀,
// 否则浏览器请求 /assets/* → 404 → 白屏。PWA start_url/manifest 也要带 /h5/ 前缀。
const API_TARGET = process.env.VITE_API_TARGET || "http://localhost:8090";
const WS_TARGET = process.env.VITE_WS_TARGET || "ws://localhost:8080";

export default defineConfig({
  base: "/h5/",
  plugins: [react(), VitePWA({ registerType: "autoUpdate", includeAssets: ["favicon.ico"], manifest: { name: "Verity H5", short_name: "Verity", display: "standalone", background_color: "#0f172a", theme_color: "#3b82f6", start_url: "/h5/" } })],
  server: { port: 3001, host: true, proxy: { "/api": API_TARGET, "/ws": { target: WS_TARGET, ws: true } } },
});
