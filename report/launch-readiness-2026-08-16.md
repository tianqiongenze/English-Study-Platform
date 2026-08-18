# ESP 上线就绪评估报告(2026-08-16)

> 验收范围:**P2 全功能上线前测试**(Task#174),按 `docs/test/09-人工测试操作指南.md` 阶段 0~7 全量回填实测,对照上线 G-1~G-8 门槛给出 Go/No-Go 判定。
> 环境:Rancher Desktop 单节点(VM 30G/9核,k3s v1.34.6+k3s1 + moby CRI),ns `esp-prod`,基础设施复用 `ekb-m2-aws-us-east-1`(CRDB/Redis Cluster-new/Pulsar/Milvus/Neo4j)。
> 数据来源:`report/p0-env-20260816.txt`、`report/smoke-18svc-20260816.txt`、`e2e/full.spec.ts`、`e2e/h5.spec.ts`、`report/manual-73-run-20260816-141318.json`、`docs/test/06/07`、`report/benchmark-2026-08-16/`。

---

## 0. 结论(执行摘要)

**当前状态:条件性 NO-GO。功能、安全、链路、三端(Web/H5)全部就绪;2 个阻断项 = G-7 种子规模未达标(CRDB 冒烟级)+ G-8 CI 全红。**

- **就绪 ✅**:三端功能(Web 8/8 浏览器 E2E + 07 API 73 用例全 PASS;H5 2/2 + 07 API;小程序代码级核查、WS token P0 bug 已修)、SEC-1~5 全 ✅、16 活跃服务 healthz 全 200、L1-L5 主链路全绿、镜像无 secret。
- **阻断 ✗(2 项)**:① **G-7 数据种子** — 活动数据面 CRDB 仅冒烟级(sessions=500 / session_turns=2000 / learning_events=8000 / users=238;esp_teacher.submissions=2M),30 亿级种子在 TiDB(scaled 0)未灌;② **G-8 CI** — `ci.yml` 最近 5 次全 failure + `security.yml` failure,根因 `platform/pom.xml` 缺失 + pnpm setup,修复路径已定位(见 §5)。
- **条件性 GO 前提**:修 CI(逐模块 install 或建聚合 pom)+ 多节点 D2 重灌种子 → 满足后可重签 GO。真 AI / 5000CCU / 混沌 / 告警为增强项,不阻断 GO。

---

## 1. 三端功能状态

| 端 | 结果 | 证据路径 |
|---|---|---|
| **Web**(frontend-react,/web/auth/login) | **PASS** 浏览器 E2E `full.spec.ts` **8/8 绿**(TC-W-1/2/3/4/4b/5/6/7);TC-W-8~19 + TC-W-FL-1 由 07 API 级 run_manual_73 全 PASS | `e2e/full.spec.ts`、`docs/test/07-人工测试用例.md`、`report/manual-73-run-20260816-141318.json` |
| **H5**(frontend-h5,375×667 经 /h5) | **PASS** 浏览器 E2E `h5.spec.ts` **2/2 绿**(TC-H-1 首页/TC-H-2 登录流);TC-H-1~7 由 07 API PASS + 浏览器冒烟;坑㉟f H5 白屏已修(nginx 补 /h5 静态映射,frontend-h5:1.1.1) | `e2e/h5.spec.ts`、`docs/test/07` |
| **小程序**(frontend-miniprogram,Taro 4) | **条件通过(无环境)** TC-M-1~4 无微信开发者工具 → SKIP(P-0-6);契约级代码核查:14 REST 端点 Bearer 传递一致;**P0 已修** openSpeechWs 原未带 token → 已拼 ?token(gateway-go main.go:245-259 强制);构建待微信环境 + Taro4/Node26 兼容 | `apps/miniprogram/src/api.ts`、`docs/test/09 §3.3` |
| **跨端**(TC-X-1~3) | **PASS** TC-X-1 进度同步、TC-X-2 数字人三端降级可接受、TC-X-3 traceId 贯通(05-traceId 实测:前端 traceparent→BFF→下游,部署侧 Collector→Tempo 实质贯通) | `docs/test/05-traceId实测.md`、`docs/test/07` |

---

## 2. 后端与链路状态

| 项 | 状态 | 证据 |
|---|---|---|
| 全服务健康(阶段 1.2) | **16 活跃全 200**;live-tutor-go/media-learn-rs 0 副本 scaffold 记录在案;learning-worker 无 Service 不适用;otel-collector-gateway 1/2(记 minor,多节点再看) | `report/smoke-18svc-20260816.txt`、`docs/test/09 §1` |
| 鉴权越权 SEC-1~5 | **全 ✅**:伪造 X-User-Id→401/剥离、过期 JWT→401、WS 越权→403、镜像无 secret→clean、JWKS RS256 kid=esp-rs256-1;SEC-6 限流=配置缺口(未发布 Nacos flow-rules,非代码 bug) | `docs/test/06-安全测试报告.md` |
| 73 接口用例(07) | **全 PASS**(run_manual_73,2026-08-16) | `report/manual-73-run-20260816-141318.json` |
| newman 契约 | 19/19 接口级(历史基线) | `deploy/newman/` |
| L1-L5 主链路 | **全绿**:L1 冒烟 200 / L2 scenarios 200 / L3 Dubbo Triple 学情(TC-W-6)/ L4 Disruptor 异步落库(TC-E2E-3 202)/ L5 trace 贯通 | `docs/test/09 §5`、`docs/test/07` |
| L6-L10 增强 | L6 WS 多轮(TC-E2E-7)、L7 Disruptor(TC-E2E-3)、L8 LangGraph(TC-W-11)均 PASS;**L10 优雅停机待多节点**(preStop manifest 就绪) | `docs/test/09 §5` |
| Pact 契约 | teacher-learning 对绿(本地契约测试);全 11 路由待 N-4 扩量 | `services/integration-tests/` |

---

## 3. 数据种子状态(G-7 — 阻断)

**✗ 未达标(冒烟级)**。2026-08-16 实测(活动数据面 **CRDB esp**,非 TiDB):

| 表 | 实测行数 | 达标目标 |
|---|---|---|
| sessions | 500 | ≥50M |
| session_turns | 2,000 | ≥50M |
| learning_events | 8,000 | ≥99M |
| users | 238 | ≥1M |
| esp_teacher.submissions | 2,000,000 | ≥10M(部分达标) |
| 其余 7 表(冒烟/空) | — | 按 SRS |

- 30 亿级种子在 **TiDB(scaled 0)**,本轮未灌。
- **解阻路径**:多节点 Phase D2 重灌(坑㉑ TiKV region 压力、坑㉟b 已知;learning_events 99% pragmatic 达标后精确 100M 亦需多节点)。
- 注:本轮功能/压测均以 CRDB 冒烟级种子完成,业务功能不受影响;仅"生产级数据规模验证"未达。

---

## 4. 工程化状态(G-8 — 阻断)

**✗ CI 全红**。`gh run list` ci.yml 最近 5 次全 failure(2026-08-15 最近);security.yml 最近 failure(0s 无日志,setup 级)。

### 根因(已核实)
1. **`platform/pom.xml` 不存在**。`ci.yml` 后端 job 用 `working-directory: platform` + `mvn -B -q install -DskipTests -pl esp-api,esp-cache,esp-rw-java,esp-disruptor`(ci.yml L77-79,契约 job L222-225 同理)→ Maven 报 `Could not find the selected project in the reactor: esp-api`。
2. **platform/ 下 5 模块相互零依赖**(已逐一 grep 核实):esp-api / esp-cache / esp-disruptor / esp-rw-java / esp-web-common 各自独立 pom,无聚合父 pom;**esp-api、esp-cache、esp-web-common 均未被其他模块 pom 引用**,每个模块可独立构建,构建顺序无关。
3. **Frontend job**:`pnpm/action-setup@v4` 失败(版本 / Node 匹配问题)。
4. **security.yml**:0s setup 级失败,先查 setup-node / action-setup 基座。

### 修复建议(最简路径)
- 后端 job 改为逐模块循环(顺序无关,无需新建聚合父 pom):
  ```yaml
  for m in esp-api esp-cache esp-disruptor esp-rw-java esp-web-common; do
    (cd $m && mvn -B -q install -DskipTests)
  done
  ```
- 可选方案:新建 `platform/pom.xml` 聚合父 pom(modules 按依赖序)后保留 `-pl`。
- 前端 job:对齐 `pnpm/action-setup@v4` 与 Node 版本;security.yml 先修 setup 基座。

---

## 5. 阻断项清单 + 解阻路径

| # | 阻断项 | 解阻路径 | 验收口径 |
|---|---|---|---|
| **G-7** | 12 表种子未达标(CRDB 冒烟级,30亿级 TiDB 未灌) | 多节点 Phase D2 重灌(规避坑㉑ region 压力) | sessions/session_turns≥50M、learning_events≥99M、users≥1M |
| **G-8** | CI 全红(ci.yml + security.yml) | 后端 job 改逐模块 `cd <m> && mvn install`(或建 platform 聚合 pom);前端 pnpm/action-setup 版本对齐;security.yml 修 setup 基座 | ci.yml 6 job + security.yml 3 件套有绿历史 |
| 补充项 | 小程序构建(无微信开发者工具 + Taro4/Node26 不兼容) | 装微信开发者工具 + 降 Node20/Nightly;构建验证 | TC-M-1~4 可跑 |
| 补充项 | 08 前端核查表契约错标 | 更正:scenarios/emotion(表标✅实未调)、placement/difficulty/polish/translate(表缺但实调) | `docs/test/08` 表一致 |

---

## 6. 已知限制(单节点环境,引用 09 附 B)

- **5000 CCU 不可达**:单节点 RD 基线(P1 fullbench 2026-08-16 已执行,k6+locust 混用,CRDB 3 副本,产物 `report/benchmark-2026-08-16/`);真弹性待多节点 + 分布式 Locust(N-7)。
- **混沌注入 / 告警触发**:pod-kill / redis-down / tidb-down / PDB / ServiceDown 告警 单节点未实跑,待多节点(增强项,不阻断 GO)。
- **真 AI 链路**:esp-prod 无 anthropic key → 阶段 4 保持 Mock(Mock 全绿即可上线),真 AI 待多节点 + 配 key。
- **小程序无环境**:无微信开发者工具,Taro4.2.1 + Node26 不兼容(降 Node20/Nightly)。
- **Avatar 数字人** ~80% 降级可接受;**Pact** 仅 teacher-learning 对;**公共包覆盖率 ≥80%** 待 N-3 统计。
- **L10 优雅停机** / 双层 Collector 拓扑为多节点收益。

---

## 7. 证据索引(本次实测)

| 文件 | 内容 |
|---|---|
| `report/p0-env-20260816.txt` | 阶段 0 前置实测:node/k3s/infra/种子/镜像/CI 原始输出 |
| `report/smoke-18svc-20260816.txt` | 18 服务冒烟(16×200,2 scaffold SKIP,learning-worker 无 Service) |
| `e2e/full.spec.ts` | Web 浏览器 E2E,8/8 绿(TC-W-1/2/3/4/4b/5/6/7) |
| `e2e/h5.spec.ts` | H5 浏览器 E2E,2/2 绿(TC-H-1/2) |
| `report/manual-73-run-20260816-141318.json` | 07 API 级 73 用例全 PASS 原始结果 |
| `docs/test/06-安全测试报告.md` | SEC-1~5 全 ✅(2026-08-14 实测) |
| `docs/test/07-人工测试用例.md` | 三端用例 PASS 判定 + TC-E2E-3/7/11 |
| `report/benchmark-2026-08-16/` | P1 fullbench(k6+locust)产物 + `analysis-2026-08-16.md` |
| `docs/test/09-人工测试操作指南.md` | Task#174 全量回填后的执行手册(阶段 0~7 实测) |

---

## 8. 签收

**决议:条件性 NO-GO(阻断 G-7 种子规模、G-8 CI 红)。满足 §5 解阻路径后重签 GO。**

**签收人**:首席架构师 / 技术总监(JoelGong)  **日期:2026-08-16**
