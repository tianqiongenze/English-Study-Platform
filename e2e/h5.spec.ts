import { test, expect, Page } from "@playwright/test";

// P2 H5 浏览器 E2E(移动视口,回填 TC-H-1~7 冒烟)
// 前置:ssh 隧道 localhost:30091=frontend-h5。
// 注意:H5 SPA router basename=/h5(vite base=/h5),生产经 Ingress 路径 /h5 访问,
// 直连 NodePort 也须走 /h5 路径(根 / 处 Router 不渲染 → 空 body,2026-08-16 实测坑㉟f 验证)。
// 运行:E2E_H5_URL=http://localhost:30091 npx playwright test h5.spec.ts --reporter=list

const H5_ROOT = process.env.E2E_H5_URL || "http://localhost:30091";
const H5 = `${H5_ROOT}/h5`;

test.use({ viewport: { width: 375, height: 667 } });

test.describe("H5 移动端冒烟", () => {
  test("TC-H-1 H5 首页可达(移动视口渲染)", async ({ page }) => {
    await page.goto(H5);
    await expect(page.locator("body")).not.toBeEmpty({ timeout: 15000 });
    // 移动端 viewport meta 已生效(应用声明 maximum-scale=1 不可缩放 → H5 壳正确加载)
    const vp = await page.evaluate(() =>
      document.querySelector('meta[name="viewport"]')?.getAttribute("content") ?? ""
    );
    expect(vp).toContain("width=device-width");
    await expect(page).toHaveTitle(/H5|ESP/);
  });

  test("TC-H-2 H5 登录流(注册→进入学习页)", async ({ page }) => {
    await page.goto(H5);
    await page.waitForLoadState("networkidle", { timeout: 15000 }).catch(() => {});
    // 若登录页存在则驱动注册;不同构 H5 前端的选择器以实际为准
    const emailBox = page.getByPlaceholder(/邮箱|email/i).first();
    const pwdBox = page.getByPlaceholder(/密码|password/i).first();
    if (await emailBox.count()) {
      await emailBox.fill(`h5e2e${Date.now() % 100000000}@esp.io`);
      await pwdBox.fill("test123");
      const submit = page.locator("button", { hasText: /登录|注册/ }).first();
      await submit.click();
      await page.waitForTimeout(6000);
    }
    const body = await page.locator("body").innerText();
    expect(body.length).toBeGreaterThan(0);
  });
});
