# A1 — 利益相关者诉求矩阵(Stakeholder Concerns Matrix)

> TOGAF ADM Phase A(架构愿景)P0 补救产出物。
> 评估视角:教授级高级工程师 / 互联网大厂资深架构师(10亿用户/100万并发工业级基线)。
> 数据来源:PDF《AI英语教育平台核心功能》业务愿景 + `01-SRS.md` §2 总体描述 + README + SAD §1 架构目标与原则。
> 日期:2026-08-13。
> 更新:2026-08-17 追加 Phase 1-3 内容库/社区/排行榜对各利益相关者的影响+ Nacos 统一注册影响。

## 目的

明确"谁 cares、care 什么、care 多深、怎么解",作为架构愿景的输入与后续 ADM 阶段的验收基线。一张矩阵覆盖 7 类利益相关者 × (核心诉求 / 影响力 / 关键担忧 / 解决方案指引 / 验收 KPI)。

---

## 利益相关者诉求矩阵(七维)

| # | 角色 | 核心诉求 | 影响力 | 关键担忧 | 解决方案指引(架构/方案) | 验收 KPI |
|---|---|---|---|---|---|---|
| S-1 | **学员(learner)** | 个性化、自适应、随到随学的口语陪练 + 写作批改;三端一致体验 | **高(High)** | 学习效果是否真提升;数据隐私;响应慢 | 自适应学习(学情采集→分析→推荐→复习闭环)+ 三端(Web/H5/小程序)读写分离一致 + LangGraph agent 笔记 + 端到端 traceId 保证可追溯 | 5000 CCU 在线 P95<2s;薄弱点准确率;学习时长留存 D7/D30 |
| S-2 | **教师(teacher)** | 教研效率(自动教案/批改辅助/班级学情);不被 AI 抢饭碗 | **高(High)** | AI 批改可信度;终审主导权 | teacher-java 教案生成 + 作业评阅"AI 自动分 + 教师终审"双轨 + 学情 Top-N 难题 + Seata AT→Saga 跨越全局锁(ADR-Seata→Saga) | 教案生成时延;AI 与教师评分吻合度(终审率) |
| S-3 | **运营(ops/product)** | 数据驱动运营;活动/课程上下架;DAU/转化漏斗 | **中(Med)** | 能力地图不清、价值流断裂致运营盲区 | 业务能力地图(B1)+ 价值流图(B2)+ 业务事件目录(B3)+ app_logs 结构化行为分析 + 协同过滤推荐 | 能力覆盖无 orphan 服务;事件目录全量登记 |
| S-4 | **法务/合规(legal)** | 未成年人数据保护(PII 脱敏) + 内容合规 + 跨境数据 | **高(High)** | 数据出境违规;AI 生成内容不合规 | SAD §12 PII 脱敏 + JWT RS256 + 数据分级(public/internal/confidential/secret 兜底)+ 多区域数据同步方案合规框架 | 安全测试 SEC-1~4 全绿;DLP 分级覆盖每张表 |
| S-5 | **运维/SRE(sre)** | 部署可重复/可观测/可回滚;告警精准;故障可定位 | **高(High)** | 单节点瓶颈;告警风暴;误杀雪崩 | ArgoCD GitOps + CI 6job + 安全 3 件套 + OTel 双层 Collector + tail-sampling + HPA/PDB + 探针三分离(N-1)+ preStop(N-2) | DR RPO/RTO 显式;告警 Signal/Noise;L10 优雅停机无 502 |
| S-6 | **财务(finance)** | 成本可预测/可优化;AI token 预算可控 | **中(Med)** | LLM 长尾成本失控;过度设计浪费 | Capacity Plan(D2,5000CCU 节点数/规格/HPA min-max)+ LLM_MODE mock/real 双模 + llm_call_logs 计费追溯 + 缓存削峰 | 每学习事件成本;token 预算 vs 实耗;缓存命中率 |
| S-7 | **架构治理委员会(board)** | 架构演进有据;决策可追溯;技术债受控 | **高(High)** | 架构漂移;ADR 散落;债务失控 | 18 ADR 完整 + ADR 索引(G2)+ 治理委员会章程(G1)+ 技术 debt 登记(H1)+ TOGAF ADM 周期再触发 | ADR 100% 编号追溯;debt 量化估值;ADM 周期触发 |

---

## 影响力 × 关切度二维分布

```
        高关切
          │
   S-1学员│ S-2教师   S-4法务
          │ S-5 SRE
  ────────┼─────────────── 高影响力
   S-3运营│ S-7委员会
          │
   S-6财务│
          │
        低关切
```

**关键洞察**:
- **高影响力 + 高关切**:学员 S-1、教师 S-2、法务 S-4、SRE S-5、委员会 S-7——这 5 类是上线前**必须签收**的核心利益相关者,对应阶段 7 Go/No-Go 阻断项 G-3(SEC)/G-5(链路)与治理闭环。
- **中影响力**:运营 S-3、财务 S-6——不阻断上线,但影响运营效率与成本可预测性,对应增强项 E-1~E-9。
- **本次架构动作**:学员/教师诉求 → P1 全 6 项(Pact/RS256/langgraph/压测/traceId/缓存统一)已落地;SRE 诉求 → N-1/N-2 探针 + preStop 本轮补;委员会诉求 → 本批 9 份 P0 文档 + 评分修订闭合。

---

## Phase 1-3 新服务对利益相关者的影响(2026-08-17 追加)

> 内容库(content-go)、圈子社区(community-go)、排行榜(gamification-go rankings)三域对七类利益相关者诉求的增量影响。

| # | 角色 | 内容库(content-go)增量 | 社区(circle/community-go)增量 | 排行榜(rankings)增量 |
|---|---|---|---|---|
| S-1 学员 | 课程目录浏览、作答进度跟踪、学习路径个性化(Courses→Lessons→ContentItems) | 圈子互动、发帖评论、举报不良内容、学习社交化 | 排行榜竞争激励、日切/周切/月切快照、percentile 可视化、opt-out 隐私控制 |
| S-2 教师 | 教研内容策展(Courses/Lessons/ContentItems CRUD)、学情从 content_attempts 联动 | 班级圈子管理(教师建圈+邀请码)、学情讨论区 | 班级/年级排行榜,教研数据支撑 |
| S-3 运营 | 内容运营(Courses 状态流转 draft->published->archived),内容审核(content_items.version) | 社区运营(举报阈值->自动隐藏,社区活跃度分析),DAU/帖子量 | 活动排行榜(自定义 scope_type=category),留存指标(排行榜参与率) |
| S-4 法务/合规 | 内容版权标注(source/license 列),未成年人内容审核 | 举报机制(content_reports)、不良内容自动隐藏、UGC 合规审计 | ranking_opt_out 隐私保护,排行榜数据不泄露 PII |
| S-5 SRE | content-go 服务部署可观测,5 表读写延迟监控 | community-go 服务部署,举报阈值->自动隐藏链路可靠性 | Redis ZSET 高可用(排行榜热数据)、日切任务 cron 可靠性、CRDB snapshots 写入监控 |
| S-6 财务 | 内容库 LLM 调用成本(内容生成/批改),5 表存储成本 | 社区存储成本(帖子/评论)、举报审核人工成本 | 排行榜 Redis 内存预算(ZSET 大小)、日切 CRDB snapshots 存储增长 |
| S-7 架构治理委员会 | content-go 独立 schema(esp_content)符合限界上下文原则 | community-go 独立 schema(esp_community)符合原则 | rankings CQRS 架构(Redis ZSET 热 + CRDB snapshots 冷)需 ADR 记录 |

---

## Nacos 统一注册对各利益相关者的影响(ADR-19,2026-08-17 追加)

> 所有 19 个服务(Java/Go/Python/Rust)统一注册到 ekb-m2 共享 Nacos,esp-prod 原 nacos Deployment 已删除(ExternalName 桥接)。

| # | 角色 | 影响要点 | 风险/缓解 |
|---|---|---|---|
| S-5 SRE | **正面**:消除 esp-prod 冗余 Nacos(~512MB 内存释放);统一服务发现入口,一个 Dashboard 查全量实例;ephemeral 模式心跳自动剔除故障实例,无需人工摘流量 | **风险**:Nacos 变为全局单点;**缓解**:ekb-m2 Nacos 本身有多副本 HA + 数据持久化;esp-prod 保留 ExternalName 桥接可快速切回独立实例 |
| S-7 架构治理委员会 | **正面**:19 服务统一注册降低架构异构度;BFF RouteTable 动态化(ADR-20)消除重启 BFF 才能更新路由的技术债;限流熔断统一语义(ADR-21/22)使多语言服务行为一致 | **风险**:跨命名空间(esp-prod→ekb-m2)网络延迟;**缓解**:同集群 VPC 内延迟 <1ms,实测无感知 |
| S-3 运营 | **间接正面**:服务健康状态可统一监控(Nacos Dashboard + OTel),故障发现时间缩短;路由动态化使新功能上线零重启 | 无显著风险 |
| S-1 学员 | **间接正面**:路由动态化缩短功能上线周期;限流熔断保障高峰期学习体验不降级 | 无显著风险 |
| S-6 财务 | **正面**:释放 esp-prod Nacos 节点资源(~512MB),单节点内存更宽裕;共享 Nacos 无额外成本 | 无显著风险 |

## 未结风险登记(回写 H1 技术债)

| 风险 | 关联角色 | 优先级 | 偿还计划 |
|---|---|---|---|
| 单节点 5000 CCU 不可达 | S-1/S-5 | P1 | N-7 多节点分布式 Locust |
| TiKV 单节点 region 压力(坑㉑) | S-5/S-6 | P1 | 多节点 Phase D2 重灌 |
| learning_events 99% pragmatic达标未精确 100M | S-3/S-6 | P2 | 多节点重灌 |
| Pact 仅覆盖 1 对 | S-5/S-7 | P1 | N-4 全 11 路由 |
| Avatar ~80% 降级 | S-1 | P2 | 多端资源扩展 |

## 出口条件

7 类角色行齐,每行 ≥1 诉求 + ≥1 担忧 + ≥1 方案(已满足)。本矩阵为架构愿景(Phase A)、业务架构(Phase B)、治理(Phase G)提供输入基线。
