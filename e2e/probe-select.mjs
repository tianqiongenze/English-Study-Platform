import { chromium } from "@playwright/test";
const b = await chromium.launch();
const p = await b.newPage();
await p.goto("http://localhost:30070/web/auth/login");
await p.waitForTimeout(1500);
console.log("selects:", await p.locator("select").count());
await p.locator("button.tab", { hasText: "注册" }).first().click();
await p.waitForTimeout(800);
const n = await p.locator("select").count();
console.log("selects after register tab:", n);
if (n) {
  const opts = await p.locator("select option").allTextContents();
  console.log("options:", JSON.stringify(opts));
  await p.locator("select").selectOption({ label: "教师" });
  const val = await p.locator("select").inputValue();
  console.log("selected value:", val);
}
await b.close();
