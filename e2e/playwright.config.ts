import { defineConfig, devices } from "@playwright/test";

// P2-19 E2E 起步:针对前端 Web(apps/web,monorepo)冒烟级端到端。
// 默认指向本地 dev server(CI 可用 webServer 启动 vite)。
// 单节点阶段:手动或 workflow_dispatch 触发;迁入多节点后接入 CI。
export default defineConfig({
  testDir: "./",
  timeout: 30000,
  retries: 0,
  use: {
    baseURL: process.env.E2E_BASE_URL || "http://localhost:3000",
    trace: "on-first-retry",
  },
  projects: [
    { name: "chromium", use: { ...devices["Desktop Chrome"] } },
  ],
  webServer: process.env.CI
    ? {
        command: "pnpm --filter @esp/web dev -- --port 3000",
        url: "http://localhost:3000",
        reuseExistingServer: false,
        timeout: 60000,
      }
    : undefined,
});
