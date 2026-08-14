# ESP 前端 GUI 连调连测报告(统一 Portal 隧道)

> 日期:2026-08-14
> 方式:Mac → SSH 隧道 localhost:30070 → frontend-portal(nginx)→ 三前端 + 9 后端 ClusterIP
> 用例基线:docs/test/07-人工测试用例.md TC-W(Web)/TC-H(H5)/TC-X(跨端)/TC-P(Portal)
> 注:无浏览器自动化环境,GUI 测以"HTTP 渲染 + API 链路 + SPA 静态资源 + WS 握手"替代真浏览器点按;SPA 首屏 HTML/JS/CSS 可达性 + 后端业务链路贯通 = 渲染可达,JS 逻辑由源码核对。

## 0. 前置:Portal 隧道与路由

- SSH 隧道:`ssh -f -N -L 30070:localhost:30070 -F "$HOME/Library/Application Support/rancher-desktop/lima/0/ssh.config" lima-0`
- frontend-portal:nginx,ClusterIP:80,NodePort 30070。直连各后端 ClusterIP(绕过 traefik ingress controller 未运行)。
- WS /ws/ 反代 gateway-go,已配 Upgrade/Connection 头,proxy_read_timeout 3600s。

## 1. Portal 三端可达性(TC-P-1~4)

| 用例 | 路径 | HTTP | 实测 | 结论 |
|---|---|---|---|---|
| TC-P-1 | GET / (portal 入口) | 200 | DOCTYPE + "ESP 统一入口" 标题 + 样式 | ✅ |
| TC-P-2 | GET /web/ (Web SPA) | 200 | Vite 构建 index.html + JS/CSS bundle 引用 | ✅ |
| TC-P-3 | GET /h5/ (H5 SPA) | 200 | index.html + PWA manifest + registerSW | ✅ |
| TC-P-4 | GET /web/auth/login (SPA 深链 fallback) | 200 | 同 /web/(nginx try_files 回 index.html) | ✅ |
| TC-P-FL | /web/assets/index-CIWPpUjh.js | 200 | JS bundle 可下载 | ✅ |
| TC-P-FL | /web/assets/index-CY0Diupw.css | 200 | CSS bundle 可下载 | ✅ |
| TC-H-FL-1 | /h5/assets/index-Clmbm06m.js | 200 | H5 JS bundle | ✅ |
| TC-H-FL-1b | /h5/manifest.webmanifest | 200 | PWA manifest | ✅ |

## 2. Web 端业务链路(TC-W,经 portal)

| 用例 | 操作 | 实测 | 结论 |
|---|---|---|---|
| TC-W-1 | 登录页可达 /web/auth/login | 200 | ✅ |
| TC-W-1b | POST /api/auth/register(learner) | `{user,token(RS256 532字符)}` | ✅ |
| TC-W-1c | POST /api/auth/login | 同 token | ✅ |
| TC-W-2 | GET /api/scenarios(带 token) | scenarios 数组(airport...) | ✅ |
| TC-W-2b | POST /api/sessions | `{session_id:"sess_..."}` | ✅ |
| TC-W-2c | WS /ws/speech/{sid}?token= | **101 Switching Protocols**(Sec-WebSocket-Accept 正确) | ✅ |
| TC-W-3 | POST /api/writing/grade(exam=toefl) | `{score:24,feedback,grammar[],coherence}` | ✅(注:exam 必填 toefl/ielts) |
| TC-W-4 | GET /api/learning/mastery/weak | mastery 行数组 | ✅ |
| TC-W-5 | POST /api/teacher/lesson-plan(learner token) | 200 passage+exercises | ⚠️ 见 D2(本应 403) |
| TC-W-9 | POST /api/writing/polish | `{polished,suggestions[]}` | ✅ |
| TC-W-10 | POST /api/writing/translate | `{translation:"",vocab:[]}` | ✅(Mock 空翻译) |
| TC-W-11 | POST /api/writing/run(LangGraph) | `{thread_id,status:"done",grade_result}` | ✅ |
| TC-W-13 | POST /api/learning/placement | **500** | ❌ D5 复现 |
| TC-W-14 | POST /api/learning/difficulty | `{next_difficulty:1,speed_multiplier:0.8}` | ✅ |
| TC-W-15 | GET /api/learning/recommend | `{review:[],weak_points:[],similar_scenarios:[]}` | ✅ |
| TC-W-16 | GET /api/learning/behavior/top | `[{user_id:1,cnt:1}]` | ✅ |
| TC-W-17 | POST /api/emotion/sense | `{emotion:"negative",encouragement}` | ✅ |
| TC-W-18 | GET /api/writing-coach/templates | **500** | ❌ D6 复现 |
| TC-W-19b | POST /api/writing-coach/plan | `{plan,weak_points[]}` | ✅ |

## 3. H5 端可达性(TC-H,经 portal)

| 用例 | 路径 | HTTP | 结论 |
|---|---|---|---|
| TC-H-1 | GET /h5/login | 200 | ✅ |
| TC-H-2 | GET /h5/ | 200 | ✅ |
| TC-H-3 | GET /h5/speech(深链) | 200 | ✅ |
| TC-H-4 | H5 复用 /api 链路(scenarios/grade/emotion) | 同 Web 端结果 | ✅ |
| TC-H-5 | POST /api/emotion/visual | `{image_url:placehold.co/...}` | ✅(占位图) |
| TC-H-6 | H5 学习页 placement 触发(源码 useEffect) | 源码触发 placement → 500(D5) | ⚠️ D5 影响 |
| TC-H-7 | H5 写作页 grade/polish | ✅(同 Web) | ✅ |
| TC-H-FL-1 | H5 JS/CSS/manifest | 200 | ✅ |

> H5 Learning.tsx 首屏 useEffect 自动调 placement(Number(uid),[1-5],0.4)→ 命中 D5(500),前端 catch 显示"定级跳过:500",不阻断主流程但定级功能不可用。

## 4. 跨端一致性(TC-X)

| 用例 | 操作 | 实测 | 结论 |
|---|---|---|---|
| TC-X-1 | Web 登录拿 token | RS256 JWT | ✅ |
| TC-X-2 | 同 token 经 H5(/api/scenarios)复用 | 200 scenarios | ✅ JWT 跨端通用 |
| TC-X-3 | WS /ws/speech 经 portal 握手 | **101 Switching Protocols** | ✅(portal nginx WS 反代正确) |

## 5. 安全缺陷复现(经 portal,与后端报告一致)

| 检查 | 操作 | 结果 | 缺陷 |
|---|---|---|---|
| 无 token | GET /api/scenarios | **200** | D1 网关无鉴权 |
| learner 调教师端 | POST /api/teacher/lesson-plan | **200** | D2 RBAC 未强制 |

## 6. 缺陷汇总(前端连测视角)

| # | 缺陷 | 影响端 | 严重度 | 状态 |
|---|---|---|---|---|
| D1 | gateway-go /api/scenarios 无鉴权 | Web/H5 | 🔴 高 | 阻断上线 |
| D2 | teacher-java RBAC 未强制 | Web | 🔴 高 | 阻断上线 |
| D5 | /api/learning/placement 500 | Web/H5 | 🟡 中 | 定级功能不可用 |
| D6 | /api/writing-coach/templates 500 | Web | 🟡 中 | 写作模板功能不可用 |
| D-doc-1 | TC-W-3 文档用例 body 缺 exam 字段 | 文档 | 🟢 低 | 需修正 |
| D-doc-2 | H5 Learning placement 触发后 500 影响首屏提示 | H5 | 🟢 低 | 依赖 D5 修复 |

## 7. 通过项汇总

- Portal 三端静态资源全可达(8/8):入口页/Web SPA/H5 SPA/深链 fallback/JS/CSS/manifest
- Web 端业务链路 16/18 通过(2 个 500 缺陷 D5/D6)
- H5 端可达性 7/7 通过(业务依赖 D5)
- 跨端 JWT 复用 ✅,WS 握手经 portal 101 ✅
- RS256 JWT 颁发 + scenarios + session + WS + writing 闭环 + emotion 全链路经 portal 贯通

## 8. 浏览器自动化缺口(诚实声明)

- **未做真浏览器点按**:本环境无 Playwright/Selenium/Puppeteer,GUI 测以 HTTP 渲染可达 + API 链路 + 源码逻辑核对替代。
- **未验证项**:React 组件挂载后的交互态(按钮点击→状态切换→UI 重渲染)、Canvas/MediaRecorder 音频录制、PWA Service Worker 注册运行态、WebSocket 长连接消息收发(仅握手 101)。
- **建议**:上线前补一轮 Playwright 真浏览器 E2E(覆盖登录→场景→WS 语音→写作→评分→报告全链路交互态),或人工按 09-人工测试操作指南 在浏览器走查。

## 结论

前端 GUI 经统一 Portal 隧道连调,**三端静态资源全可达,Web/H5 业务链路经 portal 贯通**(16/18 端点通过,WS 握手 101,JWT 跨端通用)。但存在 **2 个阻断性安全问题(D1/D2)** 与 **2 个功能缺陷(D5/D6)**,与后端连测报告一致。**D1/D2 必须修复后方可上线**;D5/D6 影响定级与写作模板功能应修复;真浏览器交互态 E2E 为上线前补测项。
