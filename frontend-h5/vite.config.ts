import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { VitePWA } from "vite-plugin-pwa";

// H5 移动端:学习者全功能,教师只读。经 BFF(8090) 鉴权,WS 旁路 gateway-go。
const API_TARGET = process.env.VITE_API_TARGET || "http://localhost:8090";
const WS_TARGET = process.env.VITE_WS_TARGET || "ws://localhost:8080";

export default defineConfig({
  plugins: [react(), VitePWA({ registerType: "autoUpdate", includeAssets: ["favicon.ico"], manifest: { name: "ESP Home-Tutor H5", short_name: "ESP", display: "standalone", background_color: "#0f172a", theme_color: "#3b82f6", start_url: "/" } })],
  server: { port: 3001, host: true, proxy: { "/api": API_TARGET, "/ws": { target: WS_TARGET, ws: true } } },
});
