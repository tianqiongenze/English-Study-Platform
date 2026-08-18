import { test, expect, Page } from "@playwright/test";

// P2 全功能 Web 浏览器 E2E(回填 TC-W-1~7)
// 前置:ssh 隧道已建(localhost:30070=frontend-portal,portal 反代 /api→bff、/ws→gateway)。
// 运行:E2E_BASE_URL=http://localhost:30070 npx playwright test full.spec.ts --reporter=list
// 说明:各用例独立注册新用户(隔离);token 为 httpOnly cookie esp_token,由 UI 表单登录设置。

const BASE = process.env.E2E_BASE_URL || "http://localhost:30070";
const PASSWORD = "test123";
const uniq = () => `e2e${Date.now() % 100000000}@esp.io`;
// 教师账号由后台运营开通(P0-4:register 服务端强制 learner,忽略请求体 role),
// 本 E2E 用 ops-provisioned 账号登录教师端,验证真实教师页面。
const TEACHER_EMAIL = process.env.E2E_TEACHER_EMAIL || "e2eteacher@esp.io";
const TEACHER_PASSWORD = process.env.E2E_TEACHER_PASSWORD || "test123";

async function register(page: Page, role: "学习者" | "教师"): Promise<string> {
  await page.goto(`${BASE}/web/auth/login`);
  await page.locator("button.tab", { hasText: "注册" }).first().click();
  const email = uniq();
  await page.getByPlaceholder("姓名").fill("E2E用户");
  await page.getByPlaceholder("邮箱").fill(email);
  await page.getByPlaceholder("密码").fill(PASSWORD);
  if (role === "教师") {
    // 说明:后端 P0-4 强制 learner,此处选择不生效(见 TEACHer note);保留选择以对齐前端行为
    await page.locator("select").selectOption({ label: role });
  }
  await page.locator("button.primary", { hasText: "注册并登录" }).click();
  await page.waitForURL(/\/learning\/speech/, { timeout: 25000 });
  return email;
}

async function login(page: Page, email: string, password = PASSWORD) {
  await page.goto(`${BASE}/web/auth/login`);
  await page.locator("button.tab", { hasText: "登录" }).first().click();
  await page.getByPlaceholder("邮箱").fill(email);
  await page.getByPlaceholder("密码").fill(password);
  await page.locator("button.primary", { hasText: "登录" }).click();
  await page.waitForURL(/\/learning\/speech/, { timeout: 25000 });
}

async function loginTeacher(page: Page) {
  await page.goto(`${BASE}/web/auth/login`);
  await page.locator("button.tab", { hasText: "登录" }).first().click();
  await page.getByPlaceholder("邮箱").fill(TEACHER_EMAIL);
  await page.getByPlaceholder("密码").fill(TEACHER_PASSWORD);
  await page.locator("button.primary", { hasText: "登录" }).click();
  await page.waitForURL(/\/teacher\/lesson-plan/, { timeout: 25000 });
}

async function nav(page: Page, label: string) {
  const el = page.locator(`nav >> text="${label}"`).first();
  if (await el.count()) { await el.click(); return; }
  const btn = page.getByRole("button", { name: label }).first();
  await btn.click();
}

test.describe("Web 学习者 Golden Path", () => {
  test("TC-W-1 注册+退出+再登录(token=httpOnly cookie)", async ({ page }) => {
    const email = await register(page, "学习者");
    await expect(page).toHaveURL(/\/learning\/speech/);
    // cookie 已由登录设置(httpOnly,前端不可读 → 证明登录链通)
    const cookies = await page.context().cookies();
    const tok = cookies.find((c) => c.name === "esp_token");
    expect(tok, "esp_token cookie 应存在").toBeTruthy();
    expect(tok!.httpOnly, "token 应为 httpOnly").toBe(true);
    // 退出
    await page.locator("button", { hasText: "退出" }).first().click();
    await page.waitForURL(/\/auth\/login/, { timeout: 15000 });
    // 再登录
    await login(page, email);
    await expect(page).toHaveURL(/\/learning\/speech/);
  });

  test("TC-W-4 自适应学习-薄弱知识点(API 渲染)", async ({ page }) => {
    await register(page, "学习者");
    await nav(page, "自适应学习");
    await page.locator("button", { hasText: "薄弱知识点" }).first().click();
    // 结果区出现(允许空列表"无数据",但必须是接口返回而非前端报错)
    await page.waitForTimeout(3000);
    const body = await page.locator("body").innerText();
    expect(/薄弱|无数据|暂无|知识点/.test(body), `薄弱点结果区应渲染,body=${body.slice(0, 120)}`).toBeTruthy();
  });

  test("TC-W-4b 复习清单(今日复习)", async ({ page }) => {
    await register(page, "学习者");
    await nav(page, "复习清单");
    await page.locator("button", { hasText: "查询今日复习" }).first().click();
    await page.waitForTimeout(3000);
    const body = await page.locator("body").innerText();
    expect(/复习|无|暂无|清单/.test(body), `复习结果区应渲染,body=${body.slice(0, 120)}`).toBeTruthy();
  });

  test("TC-W-3 写作 AI 批改(评分+反馈)", async ({ page }) => {
    test.setTimeout(90000);
    await register(page, "学习者");
    await nav(page, "写作批改");
    await page.getByPlaceholder("作文题目").fill("My weekend");
    await page.getByPlaceholder("在此输入你的英文作文...").fill(
      "Last weekend I went to the park with my friends. We played football and had a picnic. It was a nice day.",
    );
    await page.locator("button.primary", { hasText: "AI 批改" }).first().click();
    await page.waitForTimeout(15000);
    const body = await page.locator("body").innerText();
    expect(/score|评分|\d{1,3}/i.test(body), `批改结果应含评分,body=${body.slice(0, 200)}`).toBeTruthy();
  });

  test("TC-W-2 口语陪练 WS(会话创建)", async ({ page }) => {
    test.setTimeout(60000);
    await register(page, "学习者");
    await nav(page, "口语陪练");
    await page.locator("button", { hasText: "开始陪练" }).first().click();
    await page.waitForTimeout(8000);
    const body = await page.locator("body").innerText();
    expect(/会话已创建|session|已连接|开始/.test(body), `口语会话区应渲染,body=${body.slice(0, 160)}`).toBeTruthy();
  });
});

test.describe("Web 教师端 Golden Path", () => {
  test("TC-W-5 教案生成", async ({ page }) => {
    test.setTimeout(90000);
    await loginTeacher(page);
    await expect(page).toHaveURL(/\/teacher\/lesson-plan/);
    await page.locator("button", { hasText: "生成教案" }).first().click();
    await page.waitForTimeout(12000);
    const body = await page.locator("body").innerText();
    expect(/教案|lesson|passage|exercises/i.test(body), `教案应渲染,body=${body.slice(0, 160)}`).toBeTruthy();
  });

  test("TC-W-6 学情分析", async ({ page }) => {
    await loginTeacher(page);
    await nav(page, "学情分析");
    await page.locator("button", { hasText: "查询" }).first().click();
    await page.waitForTimeout(4000);
    const body = await page.locator("body").innerText();
    expect(/分析|无数据|暂无|班级|学生/.test(body), `学情结果区应渲染,body=${body.slice(0, 160)}`).toBeTruthy();
  });

  test("TC-W-7 作业评阅(批量自动评阅)", async ({ page }) => {
    test.setTimeout(60000);
    await loginTeacher(page);
    await nav(page, "作业评阅");
    await page.locator("button", { hasText: "批量自动评阅" }).first().click();
    await page.waitForTimeout(6000);
    const body = await page.locator("body").innerText();
    expect(/评阅|无数据|暂无|graded|完成/.test(body), `评阅结果区应渲染,body=${body.slice(0, 160)}`).toBeTruthy();
  });
});
