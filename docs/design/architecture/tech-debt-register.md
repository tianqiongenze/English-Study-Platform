# H1 — 技术债务登记(Tech Debt Register)

> TOGAF ADM Phase H(变更治理)P0 补救产出物。
> 评估视角:教授级高级工程师 / 互联网大厂资深架构师。
> 数据来源:`docs/PROGRESS.md` 遗留项 + 评分 §0 滞后纠正 + C1 反模式 + .trivyignore 38 vulns + 单节点铁律约束。
> 日期:2026-08-13。

## 目的

把散落在 PROGRESS "遗留项"、评分文档、坑链(坑①~㉒)里的技术债结构化登记,按优先级 × 估值 × 偿还计划管理,作为 Phase H 变更治理与 ADM 周期触发的输入。

---

## 技术债登记表

| # | 债项 | 类别 | 优先级 | 估值(人日) | 偿还时间 | 状态 | 关联 |
|---|---|---|---|---|---|---|---|
| TD-01 | learning_events 精确 100M(当前 99% pragmatic,坑㉑ TiKV region unavailable 第4次复发) | 数据/容量 | P1 | 1 | 多节点 Phase D2 | ⏳ 待多节点 | 评分 §0.2 容量6.0 |
| TD-02 | 5000 CCU 单节点不可达(533 rps/P95 25s/7.3% 失败) | 容量/弹性 | P1 | 5 | 多节点 + 分布式 Locust(N-7) | ⏳ 待多节点 | 评分 §三,G-01 |
| TD-03 | Pact 契约仅覆盖 teacher-learning 一对,未及全 11 路由 | 测试/契约 | P1 | 3 | 阶段1 工作包 B-2(N-4) | ⏳ 待扩 | 评分 §0.5 N-4 |
| TD-04 | 公共包严格覆盖率未量化(esp-disruptor/esp-rw-java/esp-cache/esp-db-py/esp-cache-py) | 测试 | P1 | 5 | 阶段1 工作包 B-1(N-3) | ⏳ 待补 jacoco/pytest-cov | 评分 §0.5 N-3 |
| TD-05 | bouncycastle CRITICAL CVE-2025-14813 + netty/jackson HIGH 未升级 | 安全/依赖 | P1 | 2 | 阶段1 工作包 C-1(N-5) | ⏳ 待升级 | .trivyignore 38 vulns |
| TD-06 | 探针三分离未做(缺 startupProbe,liveness 高负载误杀坑⑮) | 可靠性 | P1 | 1 | 阶段1 工作包 C-2(N-1) | ⏳ 待 apply | 评分 §0.5 N-1 |
| TD-07 | preStop graceful shutdown 缺(gateway WS drain/learning Disruptor drain) | 可靠性 | P1 | 1 | 阶段1 工作包 C-3(N-2) | ⏳ 待 apply | 评分 §0.5 N-2 |
| TD-08 | achievements/user_achievements 双 Owner(learning C + gamification CRU)跨域写反模式 | 架构 | P1 | 2 | 事件驱动重构 | ⏳ 待修 | C1 AP-1 |
| TD-09 | learning_events 双写路径文档不清晰(Disruptor↔Pulsar↔worker) | 文档 | P2 | 0.5 | 文档明确 | ⏳ 待补 | C1 AP-2 |
| TD-10 | esp-cache(Java) vs esp-cache-py 契约同步(IMPLEMENTATION 检查表) | 架构/缓存 | P2 | 1 | P2-5 补检查表 | ⏳ 待补 | 评分 §2.2 |
| TD-11 | 数据生命周期(learning_events 30亿归档/分区/冷存)未定义 | 数据 | P2 | 2 | D6 | ⏳ 待补 | C3 §4 |
| TD-12 | 字段级 DLP 分级(public/internal/confidential/secret)未标 | 安全/数据 | P2 | 1 | D5 安全基线 | ⏳ 待补 | C3 §3 |
| TD-13 | 契约版本化(OpenAPI semver + Dubbo esp-api 版本)未做 | 架构/契约 | P2 | 2 | C5 | ⏳ 待补 | B3 |
| TD-14 | 多区域 DR Runbook 未演练 | 可靠性 | P2 | 2 | D4 | ⏳ 待补 | D1 §4 |
| TD-15 | Taro 4.2.1 + Node 26 小程序工具链不兼容 | 前端 | P2 | 0.5 | 降 Node 20 或 Nightly | ⏳ 待处置 | 07 §4 |
| TD-16 | Avatar 数字人三端 ~80% 降级 | 前端/AI | P2 | 3 | 多端资源扩展 | ⏳ 待增强 | 07 §7 |
| TD-17 | OTel 双层 Collector 拓扑(纯多节点收益) | 可观测 | P2 | 2 | 多节点 P2-4 | ⏳ 待多节点 | 评分 §一 |
| TD-18 | HPA metrics-server 单节点不稳定(cpu <unknown>) | 可靠性 | P2 | 1 | 多节点 metrics-server | ⏳ 待多节点 | 本会话观察 |
| TD-19 | TOGAF P1 治理文档(D4 DR/D5 安全/E1 过渡/G3G4 合规+全Pact/R1 三向追溯/C5 契约版本) | 治理 | P2 | 6 | 上线后 | ⏳ 待补 | togaf-assessment §三 |
| TD-20 | SEC-1~4 安全测试实跑回填(06 报告 `_` 未填) | 安全/测试 | P0 | 1 | 阶段3 实跑 | ⏳ 待跑 | 09 §2 |

## 债务分类汇总

| 类别 | 条数 | P0 | P1 | P2 | 总估值(人日) |
|---|---|---|---|---|---|
| 安全/依赖 | 3 | 1(TD-20) | 1(TD-05) | 1(TD-12) | 4 |
| 测试/契约 | 3 | 0 | 2(TD-03,04) | 1(TD-13) | 10 |
| 容量/弹性 | 2 | 0 | 2(TD-01,02) | 0 | 6 |
| 可靠性 | 4 | 0 | 2(TD-06,07) | 2(TD-14,18) | 5 |
| 架构 | 3 | 0 | 1(TD-08) | 2(TD-10,13) | 5 |
| 数据 | 2 | 0 | 0 | 2(TD-09,11) | 2.5 |
| 前端 | 2 | 0 | 0 | 2(TD-15,16) | 3.5 |
| 可观测 | 1 | 0 | 0 | 1(TD-17) | 2 |
| 治理 | 1 | 0 | 0 | 1(TD-19) | 6 |
| **合计** | **20** | **1** | **8** | **11** | **44** |

## 偿还计划(按阶段)

- **阶段1(补灌期并行,本批)**:TD-03/04/05/06/07(N-3/N-4/N-5/N-1/N-2)+ TD-19 部分(9 P0 文档已产出)→ 估 12 人日,纯写代码/manifest/文档不部署。
- **阶段3(连调期)**:TD-20(SEC-1~4 实跑)+ TD-08(achievements 事件驱动重构可选)→ 估 2 人日。
- **多节点阶段**:TD-01/02/14/17/18 + TD-19 剩余 → 估 16 人日,需多节点环境。
- **长期演进**:TD-09/10/11/12/13/15/16 → 估 14 人日,上线后持续。

## ADM 周期触发条件(对接 H3)

任一条件满足 → 再触发 ADM 评估:
1. 多节点落地(TD-01/02/14/17/18 偿还)→ 重评 Phase D 7.0→8.5+。
2. Pact 全 11 路由 + 公共包覆盖率达标(TD-03/04)→ 重评 Phase G 7.0→8.0+。
3. 重大架构变更(如 Seata→Saga 完成 / langgraph 迁移 / 新增服务)→ 新 ADR + 评分迭代。
4. 安全 0-day(bouncycastle 类)→ 紧急 ADM 子周期。

## 出口条件

债项 20 条(已满足 ≥15);每条有优先级 + 估值 + 偿还计划(已满足);分类汇总 + 阶段偿还计划 + ADM 触发条件齐(已满足)。
