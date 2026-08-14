# E2E 测试(Playwright)— P2-19 工程化补齐起步

针对前端 Web(frontend-react)的冒烟级端到端测试。

## 运行

### 前置
1. 安装依赖(项目根):
   ```bash
   npm init playwright@latest -- --quiet --browser=chromium
   # 或手动:npm i -D @playwright/test && npx playwright install chromium
   ```
2. 起前端 dev server:
   ```bash
   cd frontend-react && npm run dev   # 默认 5173;E2E 默认指 3000,用 E2E_BASE_URL 覆盖
   ```
3. 后端 BFF 可达(默认指向 dev server 同源 `/api/*` 代理;或设 `E2E_BASE_URL` 指向集群 NodePort)。

### 执行
```bash
# 默认指 http://localhost:3000
E2E_BASE_URL=http://localhost:5173 npx playwright test

# CI 模式(自动起 dev server)
CI=1 npx playwright test
```

## 范围(当前)
- `smoke.spec.ts`:首页加载 + 未鉴权接口 401 拦截(验证 BFF 鉴权链路通)。

## 后续迭代
- 完整登录流 E2E(register→login→token 持有→受保护页可达)。
- 三端(Web/H5/小程序)共享 E2E 契约。
- 接入 CI workflow_dispatch(需 runner 可达集群)。
