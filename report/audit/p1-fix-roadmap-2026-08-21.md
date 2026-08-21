# ESP P1 修复路线图(2026-08-21,Workflow 6维度并行出案)

> 27 条 P1（全量审计 P1 28 条,Workflow 出案 27,差 1 为重叠确认项）。6 agent 并行 schema 强制结构化输出,0 错误,371k tokens。
> 编排:workflow-first-ironrule(P1 出案是多维度独立方案,典型 Workflow 场景)。

## 优先级分档

### 高优先(压测前必做,12 条)
| ID | 问题 | effort | 铁律 |
|---|---|---|---|
| P1-D1 | gamification-go pollLoop hwm 无条件推进致跳过事件计分永久丢失 | M | EER |
| P1-D2 | learning-java handleFinalized 无 @DataSource 主库读+乐观锁裸重试无限循环 | M | OCC |
| P1-D3 | teacher-java Submission 无 @Version,compensate/confirm 读-改-写并发 lost update | L | OCC |
| P1-D5 | schema message_id 幂等列未入主 DDL,仅迁移脚本声明致重建丢幂等键 | S | EER |
| P1-D8 | Redis 直连 Pod FQDN 反模式(course-path-go/live-tutor-go/pronunciation-rs/assessment-rs)应走 ExternalName | S | RWT |
| P1-D10 | HPA/容量差距:maxReplicas 3-6/min 1,5000CCU 实测 533rps 7.3%失败,AI 长尾 HPA 跟不上 | L | RWT |
| P1-D11 | bff-java WebClient 聚合零超时,慢下游挂死 Netty worker | S | RWT |
| P1-D12 | gateway-go readyz 无真实依赖检查(Redis 挂仍接流量致假 500 雪崩) | S | OSW |
| P1-D16 | llm-orchestrator-rs IDOR:body user_id 无 X-User-Id 校验可越权 | L | SCS |
| P1-D19 | 限流/缓存重连/降级路径加固(joinCircle TOCTOU + cache sync.Once 永久降级) | L | CLC |
| P1-D20 | admin-go/listening-go/reading-go/live-tutor-go 零测试,admin-go 刚接限流熔断无验证 | L | TCG |
| P1-D21 | 契约测试缺口:Pulsar outbox 中转无真实 broker IT,T3 @Disabled | XL | TCG |
| P1-30 | 三 Go 服务纯信任 BFF 注入 X-User-Id 头无自身 JWT 校验 | M | SCS |

### 中优先(重构阶段,11 条)
P1-D4 learning-java handleRegistered 全表扫描 OOM / P1-D6 多区域纯纸面(CRDB locality 仅 zone 无 region) / P1-D7 HTAP 学情路径丢失(需 ADR-25 ClickHouse/物化视图) / P1-D13 LCO 缓存命中率元数据透传断链 / P1-D14 熔断粒度粗+Dubbo 下游无熔断 / P1-D15 conversation-go 归档 schema 不一致 / P1-D22 覆盖率回填(01 报告占位) / P1-D23 MOCK/REAL 标注不彻底 / P1-27 死配置 MySQL/TiDB env / P1-28 SAD 服务表过期(P0-A1 已部分修) / P1-29 llm-orchestrator-rs 对话会话无鉴权

### 低优先(加固/确认闭环,3 条)
P1-D9 RWP 遗漏确认(本次已补 llm-orchestrator-rs emptyDir,P0-K4 漏修) / P1-D17 conversation-go anthropic DefaultClient(P0-R1 已修确认) / P1-D18 community-go 邀请码模偏差

## 本次已修(P1 侧闭环)
- **P1-D9 llm-orchestrator-rs RWP 漏修补齐**(P0-K4 审计漏列,Workflow P1-D9 纠正发现):33-orchestrator-vocab.yaml llm-orchestrator-rs readOnlyRootFilesystem:true 补 app-log+/tmp emptyDir 挂载。rollout ready=1/1。对标 reading-go 模板。vocab-srs-go 无 readOnly(本就没漏)。

## 建议执行顺序(压测前)
1. **S 级快速修**(半天):P1-D5 message_id 入主 DDL / P1-D8 Redis ExternalName / P1-D11 WebClient 超时 / P1-D12 readyz / P1-D18 模偏差。
2. **M 级**(1-2 天):P1-D1 pollLoop hwm / P1-D2 乐观锁退避 / P1-30 纯信任头。
3. **L 级**(重构阶段):P1-D3 Submission @Version / P1-D16 IDOR / P1-D19 加固 / P1-D20 零测试 / P1-D10 HPA 容量(多节点 Locust)。
4. **XL**:P1-D21 契约 IT(真实 Pulsar broker)。

## 关联铁律
EER(事件可靠性)/OCC(乐观锁)/TCG(测试门禁)/OSW(可观测接线)/RWT(韧性)/CLC(代码正确性)/SCS(供应链安全)/HSA(诚实自评)/ADS(架构文档)
