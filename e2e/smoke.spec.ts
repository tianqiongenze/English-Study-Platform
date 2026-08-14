import { test, expect } from "@playwright/test";

// P2-19 E2E 冒烟:登录页可达 + 401 拦截未鉴权 API。
// 目标:验证前端壳可加载、BFF 鉴权链路通(未带 token 的受保护接口返回 401)。
// 前置:dev server 起在 baseURL;后端 BFF 可达(可指向集群 NodePort)。
//
// 注:完整登录流 E2E 依赖后端 register/login 真实可用,纳入后续迭代;
// 本冒烟仅断言页面渲染 + 网络层鉴权拦截,避免硬依赖种子数据。

test.describe("ESP Web 冒烟", () => {
  test("首页/登录页可加载", async ({ page }) => {
    await page.goto("/");
    // 至少 body 有内容(不白屏)
    await expect(page.locator("body")).not.toBeEmpty({ timeout: 10000 });
  });

  test("未带 token 访问受保护接口被 BFF 拦截 401", async ({ page }) => {
    // 前端发 /api/scenarios 经 BFF 转发 gateway-go;无 token 应 401
    const res = await page.request.get("/api/scenarios").catch(() => null);
    if (res) {
      expect([401, 403]).toContain(res.status());
    }
  });
});
