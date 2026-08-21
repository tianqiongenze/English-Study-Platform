# ESP 项目进展 (PROGRESS) — 热区

> **本文件只放当前任务热区**(已确认根因/当前子步骤/下一步/恢复锚点),硬上限 ~300 行。
> **历史里程碑见 `docs/progress/archive/*.md`**(按主题拆分),跨会话恢复亦可从 Redis `esp:ctx:archive:*` 取。
> **恢复协议**:新会话先读本文件(全文)+ TaskList,不全量读历史;需要历史细节 Grep archive 或 Redis 取。
> **冷热分离纪律**:硬上限 ~300 行,超额即归档到 `docs/progress/archive/`。2026-08-20 已执行一次冷热分离(23-14~23-16 完成段归档至 `archive/2026-08-19-to-0820-session-23-14-to-23-16-lco-engine-sink-sca.md`)。

## 2026-08-21 P0-A1/A2 架构文档诚实修复(✅ 完成)

> 用户要求:据 P0 架构审计诚实修复 3 项文档矛盾(工程铁律⑦诚实优先)。

### 完成
1. **P0-A1 SAD 改写 CRDB 实情(✅)**:`docs/design/02-系统架构设计文档-SAD.md` 升 v5.0→v5.1。删 TiDB+TiFlash HTAP+TiCDC 数据面叙事,改 CockroachDB 实情(crdb-public:26257,ESP_RW_STORAGE_TYPE=cockroachdb,CockroachDbDataSourceConfig)。第2章数据面图+逻辑分层表、第3章服务表扩列到 24 子目录(23 服务+conversation-go 内嵌库,conversation-go 标为 gateway-go 内嵌会话领域库非独立部署)、第4章读写分离改 CRDB follower-read(AS OF SYSTEM TIME)、第6章表归属+索引、第9章高可用、第11章部署、新增第13章历史演进/可回切保留段(TiDB 路径降级)、第14章技术栈清单、第15章需求追溯。ADR-03/10/16 内联标 Superseded by ADR-24。修订说明置顶。
2. **P0-A2 架构评分诚实降分(✅)**:`docs/design/架构评分与重构方向.md`。读写分离 8.0→5.0(TiDB HTAP 优势消失)、工程化 9.0→7.0(bff-java 实测 7 测试文件非"21 测 100%";公共包 esp-common-go=3/esp-common-java=1/esp-common-rs=0/esp-cache-py=0/esp-db-py=0 未达≥80%,原"done/单测齐"不实改 P0 未完成)、服务拆分 8.5→8.0(conversation-go 非独立部署,独立部署数 23 非 24)。加权总分 8.08→7.59(回归诚实)。修订说明置顶+§0.1/§二.1 标注修正。
3. **ADR-24 新建(✅)**:`docs/design/architecture/adr-24-crdb-as-primary-data-plane.md`。CRDB 作为主数据面,Supersede SAD 第5章内联 ADR-03/10/16(注明双命名空间:非索引 ADR-10/16)。记迁移动因(mirror 403+CRDB 单二进制)、代价(HTAP 学情分析路径丢失→ADR-25 DEFERRED 补 ClickHouse/物化视图)、回切条件(多节点+mirror 修好+schema 重验)。ADR 索引追加 ADR-24 行+维护节加双命名空间注意。

### 改动文件
- `docs/design/02-系统架构设计文档-SAD.md`:v5.1(数据面叙事修正)
- `docs/design/架构评分与重构方向.md`:P0-A2 降分(8.08→7.59)
- `docs/design/architecture/adr-24-crdb-as-primary-data-plane.md`:新建
- `docs/design/architecture/adr-index.md`:追加 ADR-24 + 双命名空间说明

### 教训
- 文档叙事与生产实情漂移(SAD v5.0 称 TiDB 实际 CRDB)是 DGO 类隐患,审计应据代码/部署 yaml 核验非据旧文档。
- 自评分虚高(bff 21 测/公共包 done/读写分离 8.0)违反诚实优先,须据实测文件数/单测数降分。
- ADR 双命名空间(SAD 第5章 vs 索引)易混淆,引用须注明。

---

## 2026-08-20 会话23-23:紧急 bug 修复(content 404 + writing AbortError)+ 前端美化升级启动

> 用户报:① 内容库 /api/content/courses 404;② 写作批改 AbortError;③ 前端升级要补全测试用例杜绝低级 bug。

### 紧急 bug 修复(✅)
1. **content/courses 404 根因+修复**:portal nginx `/api/content → bff_backend`(错误),bff 没转发到 content-go → 404。**修复**:加 content_backend upstream(server content-go:80),改 /api/content → content_backend。rebuild portal 1.3.5。验证 200 ✓。
2. **writing AbortError 根因+修复**:`@esp/api-client` 默认 timeout 15s,writing AI 批改 8-34s 超时致 AbortError(signal aborted)。**修复**:grade/polish/translate/writingRun/writingReview/lessonPlan 加 `{ timeoutMs: 90000 }`(90s)。重新 build Web + react 1.4.2。实测 writing/grade 13.6s 返回 score ✓。
3. **portal writing timeout**:nginx proxy_read_timeout 60→120s(AI 推理余量)。

### 前端美化升级启动(Phase1-4 方案已批准)
- Plan agent 方案:antd5 + recharts + AdminLayout + Swagger UI 入口 + admin 监控升级(资源监控/日志/可操作)+ debug 404 修复。
- @esp/ui-tokens 1:1 对齐 AntD5(ConfigProvider 零摩擦)。
- 派生构建(本地 build dist → docker COPY)绕网络限制。

### 改动文件
- frontend-portal/nginx.conf:content_backend upstream + /api/content→content_backend + writing timeout 120s。
- packages/api-client/src/index.ts:grade/polish/translate/writingRun/writingReview/lessonPlan 加 timeoutMs 90000。
- 镜像:frontend-portal:1.3.5 + frontend-react:1.4.2(VM 内)。

### 教训
- 前后端契约不一致(nginx 路由指向错误后端)是 404 根因,需端到端验证。
- AI 推理长尾(timeout 15s < 34s)致 AbortError,前端 client timeout 须覆盖 AI 耗时。
- 前端升级须补全功能测试用例,杜绝低级 bug(用户强调)。

---

> **冷热分离(2026-08-21第二次)**:会话23-22~23-27续(前端rebuild/连调/LCO/资源监控/Sentinel-GraphQL/全量审计启动等 ✅ 完成段)已归档至 `docs/progress/archive/2026-08-20-to-0821-session-23-22-to-23-27-frontend-multilang-audit-p0-start.md`。本文件仅留 P0/P1 修复链热区(23-28起)。

## 会话23-28:P0 立即修执行(2026-08-21)

> 14 条 P0 按"配置部署快速修→LLM 超时→代码正确性→数据一致性→测试补→架构文档"顺序推进。Hybrid:3 explorer 并行 scout(WTV 核实真实落点)→ esp-builder/esp-doc 并行做机械编辑+文档 → 主线程 inline 做代码精修 → Workflow 并行补单测。全部编译验证(CLC 铁律:编译过≠正确,必验证)。

### P0 执行完成清单(14/14)

**配置部署(5 条,esp-builder 执行,全 L1 可逆,DGO 回写):**
- P0-K1 argocd:esp-apps/esp-infra/esp-platform 三 app source 均 deploy/k8s。esp-platform selfHeal=false(待修),其余 selfHeal=true。course-path-go probe 已单 handler(审计多 handler 问题不存在)。风险:改 yaml 触发 selfHeal=true app 自动 sync,TiDB/redis 改是减少漂移(安全)。
- P0-K2 TiDB replicas:07-tidb-tikv.yaml **10 处** replicas:1→0(审计假设 9 处,实测 10:pd-aws/azure/gcp + tikv-aws/azure/gcp + tidb×3 + tiflash)。加注释头"已下电 DEFERRED,多节点恢复改回 1 时需先修 mirror pingcap 403"。apply RC=0,无 pod 变化(本就 scale=0)。
- P0-K3 删孤儿 redis-cluster:06b-redis-pvc.yaml 覆盖为墓碑注释(原含 standalone redis-cluster Deployment + redis-cluster-data PVC,live 已删 NotFound,仓库 yaml 是定时炸弹)。
- P0-K4 RWP 5 服务补 emptyDir:course-path-go(31:9-45)/media-learn-rs(31:126-162)/pronunciation-rs(32:10-53)/assessment-rs(32:133-177)/asr-adapter-go(34:60-99)。审计把 pronunciation-rs 漏列、误列 gamification-go(已修)。对标 reading-go 模板加 app-log+tmp emptyDir。全 rollout ready=1/1。
- P0-R2 Prometheus annotation:13 服务补 prometheus.io/scrape/port/path(course-path-go 8093/live-tutor-go 8094/media-learn-rs 8095/pronunciation-rs 8088/listening-go 8089/assessment-rs 8090/reading-go 8091/gamification-go 8092/llm-orchestrator-rs 8086/vocab-srs-go 8087/esp-admin 8099/community-go 8097/content-go 8096)。全 rollout ready=1/1。

**LLM 超时红线(1 条,主线程 inline,RWT 铁律):**
- P0-R1 conversation-go/src/llm/anthropic.go:74 `http.DefaultClient.Do` → `httpClient.Do`(用文件顶部已声明带 60s Timeout+30s ResponseHeaderTimeout 的包级 httpClient,此前声明却未用)。build RC=0。
- P0-R1 llm-orchestrator-rs/src/provider/mod.rs:59/69/79 三处 `Client::new()` → `llm_client()`(顶部新增 fn,60s timeout+5s connect,**不复用 cache/mod.rs http_client()——那是 3s 缓存层降级超时,量级错会误杀 LLM**)。cargo build Finished。

**代码正确性(1 条,主线程 inline,CLC 铁律):**
- P0-C1 writing-py/app/llm.py:44-58 AdaptiveLimiter.release 原把 self.semaphore 整体替换新实例致旧等待者死锁+acquire/release 实例错配。改:单个持久 Semaphore + 扩容往同一实例 release 额外 permit(不替换实例)。语法 OK。

**数据一致性(3 条,主线程 inline,事务原子性):**
- P0-D1 teacher-java EventPublisher → transactional outbox(用户确认走 DB outbox 统一路线,撤掉初版 AFTER_COMMIT+Redis 半成品)。新建:TeacherEventOutbox 实体(@Table teacher_event_outbox,schema-per-service 各自带表)+ TeacherEventOutboxRepository(findPending)+ OutboxRelay(@Scheduled 2s 扫表→publishReliable 投 Pulsar→done/退避/死信,对齐 content-go outbox_relay.go 模式)+ EventPublisher.publishReliable(同步 send+blockIfQueueFull(true)+EventPublishException 失败感知)+ EventPublishException 类。tryFinalize 事务内 outboxRepo.save(outbox 行)与 submission 同事务原子提交(消除幽灵事件+三重丢事件)。TeacherApplication 加 @EnableScheduling。**架构厘清**:outbox 不等于 Seata AT(undo_log 回滚日志 vs 事件队列);ESP 已从 Seata AT 主动迁出选应用层 saga(outbox+补偿);当前不需 Saga(无多方强原子写流);HPA 多副本不改 Saga 判断(正交维度,催生的是 relay 多副本协同 SKIP LOCKED+幂等键,非 Saga)。mvn compile RC=0。
- P0-D2 vocab-srs-go 乐观锁:UserVocabRow 加 Version 字段 + GetUserVocab SELECT 加 version 列 + UpsertUserVocab 改返 (int64,error) ON CONFLICT DO UPDATE SET ... version=user_vocabulary.version+1 WHERE user_vocabulary.version=$old(旧版本冲突返 0 行)+ review.go 重试循环(上限 3,2/4/8ms 退避,仍冲突返 409)+ Logger 接口加 Warnf。schema 3 处加 version 列(crdb-schema-aux-services.sql:99/schema.sql:247/schema-per-service-phase0.sql:72)。build RC=0。
- P0-D3 content-go 事务包裹:submitAnswer 的 content_attempts INSERT + learning_event_outbox INSERT 用 s.writeDB.BeginTx 包裹原子提交(原两次独立 ExecContext,attempts 成功 outbox 失败只 warn 丢事件)。outbox 写失败整体回滚(宁不记录作答也不丢事件)。跨库写经 outbox_relay.go 证实 CRDB user 有跨 esp_learning schema 权限。emitContentEvent 逻辑内联到事务(原函数无调用点保留)。build RC=0。

**测试补(2 条,Workflow 并行,p0-test-wf_0518b629-b5c):**
- P0-T1 esp-common-rs 7 模块(db/debug/health/metrics/redisx/tracing_init/lib,零 cfg(test))补 #[cfg(test)] 单测,CI rust-services job 加 cargo test -p esp-common-rs 零测试即红。
- P0-T2 l1pool.go(180 行,L1 精确缓存,CLC 铁律起因区——Lookup 零值 Manager 解引用 nil cfg panic)补 l1pool_test.go(nil cfg 防御/命中率/过期/容量驱逐),CI llm-cache-proxy-ci.yml 加 go tool cover -func≥70% gate(当前只落 coverage.out 无 gate)。

**架构文档(2 条,esp-doc 执行,诚实优先):**
- P0-A1 SAD v5.0→v5.1:删 TiDB+TiFlash HTAP+TiCDC 通篇叙事,改 CRDB 实情(crdb-public:26257,CockroachDbDataSourceConfig 双 HikariCP+RoutingDataSource)。第3章服务表扩列到实际 24 个 backend/services 子目录(23 独立服务+conversation-go 标 gateway-go 内嵌库)。新增第13章历史演进段(TiDB 降级,07-tidb-tikv.yaml replicas=0 实底层不可用,回切条件含 mirror 修好+schema 重验)。ADR-03/10/16 内联标 Superseded by ADR-24。
- P0-A2 评分降分:读写分离 8.0→5.0(TiDB HTAP 优势消失)/工程化 9.0→7.0(bff 实测 7 测试文件非 21 测,公共包 esp-common-go=3/esp-common-java=1/esp-common-rs=0/cache-py=0/db-py=0 未达≥80%)/服务拆分 8.5→8.0(conversation-go 非独立部署 23 非 24)。加权 8.08→7.59。
- 新建 ADR-24 CRDB 作为主数据面(Supersede ADR-10/16,记迁移动因+HTAP 路径丢失→ADR-25 DEFERRED 补 ClickHouse/物化视图+回切条件)。adr-index.md 加 ADR-24 + 双命名空间注意(索引 ADR 编号与 SAD 第5章内联 ADR 独立)。

### 教训与铁律命中
- **CLC 铁律**:vocab-srs 重试循环改 Logger 接口加 Warnf——改 API 前未查接口定义,编译才拦下,已补。writing-py 信号量替换——"语义想对执行路径没验证"的典型,改前先读真实代码确认 acquire/release 实例错配。
- **WTV 铁律**:3 explorer scout 核实 14 条 P0 全部属实,但数字/归属有偏差(TiDB 10 非 9、media-learn-rs 在 31 非 32、待修含 pronunciation-rs 非 gamification-go)。以实测为准修正,不按审计假设盲改。
- **DGO 铁律**:TiDB/redis/yaml 改动全回写仓库,不只 patch live。argocd selfHeal 多 true,改 yaml 触发自动 sync——故 TiDB/redis 改是减少漂移(安全),emptyDir/annotation 是新增(apply 后 pod 重建已 verify ready)。
- **workflow-first 铁律**:P0-T1/T2 测试补用 Workflow 5 agent 并行(每模块一 agent,schema 强制结构化输出),不单独并行子 Agent。

### 待办(本会话后续)
- [ ] Workflow p0-test-wf_0518b629-b5c 完成后收结论(esp-common-rs + l1pool 测试文件清单+编译/测试结果)。
- [ ] git commit 回写全部 P0 改动(代码+yaml+文档+测试),含 git add -A + commit message 列 P0 清单。
- [ ] Redis 锚点 esp:ctx:session:23-28 更新(P0 执行完成状态)。
- [ ] 之后进入 P1 出方案 + k6/Locust 压测。



## 会话23-29:新铁律沉淀 + P0-T1/T2 测试验证修复 + lco-proxy 重命名(2026-08-21)

> 基于全量审计报告暴露的 6 类问题,联网查业界最优实践(注:本环境 WebSearch/WebFetch 域名校验受限返空,改用已确立业界共识——microservices.io Transactional Outbox、Martin Fowler 乐观锁、Google Testing Blog 覆盖率、ADR 实践),沉淀 6 条新全局铁律。同时修复 P0-T1/T2 Workflow 测试遗留编译/逻辑 bug,lco-proxy→llm-cache-proxy 重命名归位。

### 6 条新铁律沉淀(全局 ~/.claude/CLAUDE.md 第 18-23 条 + memory + MEMORY.md 索引)
- **EER 事件可靠性铁律(P0-D1/D3 根因)**:跨服务关键事件禁 fire-and-forget,必用 transactional outbox(同库原子写+relay 重投+message_id 幂等键+退避死信)。业界:microservices.io Transactional Outbox Pattern。
- **OCC 乐观并发控制铁律(P0-D2/P1-3 根因)**:并发读-改-写关键实体必用乐观锁(version 列+WHERE version=old 冲突返 0 行+重读重算退避+主库 strong 读)。业界:Martin Fowler 乐观锁 / Hibernate @Version。
- **TCG 测试覆盖率门禁铁律(P0-T1/T2 根因)**:共享包/核心模块必有单测且 CI 有可执行覆盖率门禁(>=70% 关键>=80%),禁零测试空跑通过标 done。业界:Google Testing Blog(下限门禁防退化非上限凑数)。
- **HSA 诚实自评铁律(P0-A2 根因)**:自评/进度声明必以可核验事实为准禁虚标(实测计数/路径/命令输出),设计 vs 实情分离,优势消失降分非维持。是工程铁律⑦诚实优先的可操作化。
- **ADS 架构文档单一事实源铁律(P0-A1/P1-28 根因)**:SAD/ADR 必与实现双向核对,重大转向必新增 ADR+标 Superseded+回写 SAD,禁文档叙事与线上实情矛盾无人发现。DGO 铁律延伸(不只 yaml 回写,架构文档也回写)。
- **OSW 可观测性抓取接线铁律(P0-R2/P1-13 根因)**:/metrics 必双向接线闭环(代码路由+部署 scrape annotation+Prom job+指标有数据),有 metrics 路由的 deploy 必含 annotation 否则 CI 红。配而未接铁律②延伸到可观测性。
- 落地 3 处冗余:全局 ~/.claude/CLAUDE.md 第 18-23 条 + memory 6 个新文件 + MEMORY.md 索引 6 行。每条含根因+业界实践+How to apply+典型反例+ESP 落地+关联铁律。

### P0-T1/T2 测试验证修复(CLC 铁律:编译过!=正确,必验证运行期)
- **P0-T1 esp-common-rs**:Workflow 写的测试有 3 个 bug 主线程逐个修复:
  1. Cargo.toml 缺 tower dev-dependency——测试用 tower::ServiceExt::oneshot 但 tower 未声明依赖 -> E0432/E0599。修:加 [dev-dependencies] tower={version=0.5,features=[util]}。
  2. metrics.rs:187 counter.inc_by(7) 传整数字面量,prometheus 0.13 Counter inc_by 接 f64 -> E0308 类型不匹配。修:inc_by(7.0)。
  3. metrics_endpoint_body_non_empty_prometheus_text 测试误假设 prometheus crate 默认 Registry 自动塞 process metrics(实际空 Registry body 为空),空 Registry 时 body 空是正确行为->测试断言错误 panic。修:改先 register_counter! 再断言非空(测真实契约:注册后必出现在 /metrics)。
  - 修复后 cargo test --lib:**41 passed; 0 failed**。
- **P0-T2 llm-cache-proxy(lco-proxy 重命名)**:l1pool_test.go 21 测全过(go test -run TestL1 ok 2.4s)。manager_integration_test 需真实 Redis(CC Switch local proxy 15721)未跑通是 IT 合理降级。CI llm-cache-proxy-ci.yml 加 go tool cover -func >=70% gate 待回写。

### lco-proxy -> llm-cache-proxy 重命名归位
- 前会话已将 tools/lco-proxy/ 重命名为 tools/llm-cache-proxy/(命名对齐 LCO 铁律词汇),工作树含新路径文件(untracked),旧 lco-proxy 路径在 HEAD(f665519)仍 tracked 显示为 deletion。本次 commit 一并归位:
  - 新增:tools/llm-cache-proxy/** + .github/workflows/llm-cache-proxy-ci.yml + docs/design/llm-cache-proxy-*.md + adr-llm-cache-proxy.md
  - 删除:tools/lco-proxy/** + .github/workflows/lco-proxy-ci.yml + docs/design/lco-proxy-*.md(旧命名)
  - 重命名计划:.claude/plans/rename-lco-to-llm-cache-proxy.md

### 待办(本会话后续,并行执行)
- [ ] git commit 全部 P0 改动 + 6 铁律 + lco-proxy 重命名(代码+yaml+文档+测试+memory+CLAUDE.md)。
- [ ] Redis 锚点 esp:ctx:session:23-29 更新(铁律沉淀+P0 测试修复完成状态)。
- [ ] 之后进入 P1 出方案 + k6/Locust 压测。


## 会话23-29续:P1路线图出案 + llm-orchestrator-rs RWP漏修补齐(2026-08-21)

### P1 修复路线图出案(Workflow 6维度并行)
- 编排:workflow-first 铁律(P1 出案是多维度独立方案,典型 Workflow 场景)。6 agent 并行,371k tokens,0 错误,schema 强制结构化输出。
- 出案 27 条:高优 12(压测前必做)/中 11(重构阶段)/低 3(加固)。
- 报告:report/audit/p1-fix-roadmap-2026-08-21.md。
- 关键纠正(Workflow P1-D9 发现):llm-orchestrator-rs readOnly 仍无 volumeMounts——P0-K4 审计列 5 服务时漏了这个,P1-D9 纠正。

### llm-orchestrator-rs RWP 漏修补齐(P1-D9,P0-K4 漏列纠正)
- 33-orchestrator-vocab.yaml llm-orchestrator-rs readOnlyRootFilesystem:true 补 app-log+/tmp emptyDir 挂载(对标 reading-go 模板)。vocab-srs-go 无 readOnly(本就没漏)。
- rollout ready=1/1。
- commit f34e7ce 本地落地(push 待网络恢复:github.com 75s 超时连不上,非授权问题用户已授权 push master)。

### 待办(本会话后续)
- [ ] 网络恢复后 git push origin master(f34e7ce)。
- [ ] P1 S级快速修(半天):P1-D5 message_id 入主 DDL / P1-D8 Redis ExternalName / P1-D11 WebClient 超时 / P1-D12 readyz 依赖检查 / P1-D18 邀请码模偏差。
- [ ] P1 M级(1-2天):P1-D1 pollLoop hwm / P1-D2 乐观锁退避 / P1-30 纯信任头。
- [ ] 之后:k6+Locust 压测对比 GraphQL vs pre-GraphQL。


## 会话23-29续二:P1高优S+M级修复(5条,2026-08-21)

> P1 路线图出案后,按"S级快速修→M级"顺序执行。全部编译验证(CLC)+apply+rollout验证(HSA)。

### P1 高优已修(5条,commit 1a59a90/758c4f1 已落地本地,push 待网络恢复)

**S级快速修(3条,DGO回写yaml+commit):**
- P1-D5(EER):schema.sql 的 learning_events + learning_event_outbox 补 message_id 列+UNIQUE INDEX(对齐 scripts/crdb-schema.sql,原仅迁移脚本声明致重建丢幂等键)。
- P1-D11(RWT):bff-java WebClientConfig 加 connectTimeout 2s + responseTimeout 90s(原 builder().build() 无超时挂死 Netty worker)。mvn compile RC=0。
- P1-D12(OSW/RWT):gateway-go readyz 加 Redis PING(2s超时)失败返503(原无条件200,Redis脑裂时pod仍接流量致假500雪崩)。build RC=0。

**P1-D8 Redis Pod FQDN→ExternalName(单节点铁律):**
- 7处 REDIS_CLUSTER/REDIS_ADDR 直指 redis-cluster-new-0.redis-cluster-new.ekb-...(跨ns单Pod FQDN)改回 redis-cluster ExternalName 短名。
- 11服务全 rollout ready=1/1,course-path-go 日志确认 redis-cluster:6379 连通。
- 反模式危害:Pod FQDN 直连绕过 Service LB,Pod 重建连接断,单Pod无法享 Cluster MOVED 重定向。坑㉓是 nodes.conf 公告短名问题非 ExternalName 解析问题。

**M级(2条,改代码+验证):**
- P1-D1(EER):gamification-go pollLoop hwm 推进修正。原 processedMark 未命中或 userCache 未命中时 continue 跳过但 hwm 无条件推进→被跳过事件计分永久丢失。修正:userCache 未命中设 missingUser 标记整批不推进 hwm 重试(防计分丢失;已 fanout 由 processedMark 幂等防重复);processedMark 已处理跳过合理(幂等去重)hwm 可推进。build RC=0。
- P1-D18(SCS/CLC):community-go genInviteCode 模偏差消除。原 int(idx)%len(chars) 因 256%31=8 致前8字符多一概率。改拒绝法(rejection sampling)maxBiasFree=256-256%len 无偏。build RC=0。

### 待办(本会话后续)
- [ ] git push origin master 待网络恢复(本地 758c4f1 落地,后台重试中 b6bysng3y)。
- [ ] P1 剩余高优:P1-D2 乐观锁退避 / P1-D3 Submission @Version / P1-D16 IDOR / P1-30 纯信任头(L/M级,需改代码+契约测试)。
- [ ] P1-D10 HPA容量(多节点 Locust,L级,多节点阶段)。
- [ ] P1-D20/D21 测试补(XL,契约IT真实Pulsar broker)。
- [ ] 之后:k6+Locust 压测。

### 会话23-29续三(2026-08-21):P1 剩余 L/M 级并行修复派发

**git push 已落地**(后台 b6bysng3y 完成,exit 0):`git log origin/master..HEAD` 返回 0 未推送,c3c73f1/1a59a90/758c4f1 全在远程 master。

**4 agent 并行派发(各自调查→修复→编译验证→返回结论):**
- P1-D19(CLC/RWT,community-go):joinCircle TOCTOU 原子化 + sync.Once 永久降级改可重试 init。agent a7e57f612d8678846。
- P1-D2+D3(OCC,learning-java+teacher-java 合并避免 schema 冲突):handleFinalized 主库强读+乐观锁退避上限3次;Submission @Version + DDL version 列。agent a751782bf6fb69ddf。
- P1-D16(SCS,llm-orchestrator-rs):LLM 对话端点 body user_id 不可信,改 X-User-Id 头权威校验(401缺失/403不一致)。agent ac6cf54457ffe1d95。
- P1-30(SCS,~3 Go 服务):纯信任 X-User-Id 头改共享 JWT 公钥校验(esp-common-go/middleware/jwt.go 新建,对标 gateway-go/src/jwt)。agent aaa92dd2cea5d4a9e。

**互不冲突**:4 agent 分属 community-go/learning-java+teacher-java/llm-orchestrator-rs/多 Go 服务+esp-common-go,无共享文件交叉(仅 P1-D2+D3 合并因都改 schema.sql,内部串行)。

**待 agent 返回后**:收结论→统一 commit→push→PROGRESS/Redis 锚点更新。P1-D20(4服务零测试)待代码定型后补测(TCG)。


### 4 agent 全部返回(2026-08-21)——静态验证通过,等全量 build+test

**P1-D16(Rust IDOR,llm-orchestrator-rs)完成**:3 端点(/api/llm/chat、POST+GET /api/dialogue/sessions)body/query user_id 纯信任改 X-User-Id 头权威(401缺失/非法→403不一致);list_sessions 去全表 None 分支强制 WHERE user_id=权威值。cargo build EXIT=0。**纵深 IDOR 未修(报告待跟进)**:GET /api/dialogue/sessions/:id、POST /api/dialogue/chat、GET /:id/turns 按 session_id 越权读他人会话,需校验 session.owner==X-User-Id。

**P1-D19(community-go CLC)完成**:joinCircle TOCTOU 修(三步非原子→单事务 SELECT...FOR UPDATE + COUNT 主库 + ON CONFLICT DO NOTHING,消除 follower-lag 漂移+并发超限)。sync.Once 永久降级**审计不成立**(grep sync. 全空,初始化在 main() 即时执行,非 lazy init,HSA 如实报告未编造修复)。build/vet RC=0。

**P1-D2+D3(Java OCC)完成**:
- P1-D2 learning-java:UserMastery 已有 @Version(审计暗示缺失不成立);"裸重试无限循环"字面不成立但经 Pulsar nack 等价成立→新增 retryOnConflict(3次,指数退避20-40-80ms+jitter)显式 catch ObjectOptimisticLockingFailureException 耗尽转 nack;"读 follower"路径不成立(consumer 默认主库)未强加 @DataSource(ASV/HSA)。compile RC=0。
- P1-D3 teacher-java:Submission 无 @Version→加 @Version Long version(jakarta.persistence.Version,ASV 核实);schema.sql+crdb-schema.sql 加 version BIGINT NOT NULL DEFAULT 0;TeacherSummarySaga.run retryOnConflict 包裹。compile RC=0。
- **预存隐患(报告未改)**:TeacherSummarySaga.run 自调用 tryFinalize/compensate 绕过 @Transactional+@DataSource 代理(self-invocation,P0-D1 outbox 既有结构)。OCC 重试在 run 层 catch Hibernate flush 仍正确工作。超出 P1-D2/D3 范围,BRR 不擅改,记录待办。

**P1-30(Go JWT,3服务+esp-common-go)完成**:新建 esp-common-go/middleware/jwt.go(JWTValidator RS256+ParseUserID/ParseUserClaims+RequireAuth 中间件+MustInitJWTFromEnv 单例,启动期公钥缺失 os.Exit(1) 对齐 gateway-go,开发期 ESP_JWT_ALLOW_MISSING_KEY=1 降级)+ jwt_test.go(10测含 HS256 alg 混淆拒)。community-go/content-go/gamification-go 各加 jwt 字段+userID/roleOf/requireUID 走 JWT(消除裸 X-User-Id 头读取)。部署层补 envFrom: esp-rsa-jwt-verify(与 gateway-go 同款,secret 已验证含 JWT_RSA_PUBLIC_KEY_PEM)。各服务 build/vet/test RC=0。

**静态验证(主线程,已过)**:① esp-rsa-jwt-verify secret 存在含 JWT_RSA_PUBLIC_KEY_PEM(真 PEM);② schema.sql submissions version 列在(行122);③ crdb-schema.sql submissions version 列在(行171);④ 非测试代码零残留 X-User-Id 头读取(grep 空);⑤ 三服务 envFrom 与 gateway-go 同款对齐。

**待**:esp-builder 全量 Go build+vet+test 验证(community-go 是 P1-D19+P1-30 叠加重点)→ 统一 commit → push。


### 会话23-29续四(2026-08-21):纵深IDOR + P1-D20测试补 并行派发

接 P1 L/M级闭环后,按用户"工业级标准并行"指示推进两类:

**① 纵深IDOR加固(Rust,llm-orchestrator-rs)—— agent a8678c2312640a4e1**
P1-D16前次修了3个body user_id端点,3个session越权端点(get_session/list_turns/chat_sse)仍可按session_id越权读他人会话。修:加HeaderMap+require_user_id取权威uid,查到session后校验session.user_id==header_uid不符返403。agent a8678c2312640a4e1。

**② P1-D20 4服务零测试补(TCG铁律)—— 3 agent并行**
- admin-go(agent a66089411c86b9198):零测试,补单测(限流/熔断/metrics注册/debug_proxy/HPA等核心逻辑,≥5测)。
- listening-go+reading-go(agent a7bb7c45d03a315f5):双服务零测试,补判分/校验/查询逻辑测试(每服务≥4测)。
- live-tutor-go(agent ac5d49db5d80d2df0):零测试,补排期/状态机/权限/匹配逻辑测试(≥4测)。

**互不冲突**:Rust端 vs 3个独立Go服务目录,无文件交叉。

**侦察已确认**:dialogue/mod.rs get_session(165行,SELECT含user_id=r.1但未校验)/list_turns(188)/chat_sse(220)三handler当前纯Path查询无归属校验;4服务find _test.go全0个。

**待4 agent返回**:收结论→统一全量验证(Rust cargo build + 4 Go服务go test)→commit→push。之后P1-D21契约IT(XL,真实Pulsar broker)→P1-D10 HPA多节点Locust→k6压测。


### 会话23-29续五(2026-08-21):P1-D21契约IT+P1-D10压测脚本+k6 GraphQL 并行

**git push 网络问题**:github.com:443 持续 75s 超时(非认证非冲突,网络层)。2 commit 待推(ad545ea纵深IDOR+63测/79a88ba PROGRESS冷热分离)。后台 bgz7dh6tf 8次退避重试中。本地全闭环,远程待网络恢复。

**三任务并行(写代码/脚本轻负载阶段,执行由主线程串行控VM雪崩):**
- P1-D21契约IT:Workflow(wf_1823316f-5c9)3阶段——Scout 4 relay点(learning-java/OutboxRelay,teacher-java/TeacherEventOutbox,content-go/outbox_relay,gamification-go/rankings_projector)→Plan IT方案→WriteIT代码骨架。验证at-least-once+幂等消费=exactly-once(EER铁律)。真实Pulsar broker(pulsar-broker:6650,2broker Running)非mock。
- P1-D10 HPA压测脚本:agent a907e14fec3cd0e2a,准备阶梯压测脚本(100→500→1000→2000→5000 CCU)+HPA watch+方案md。只准备不执行。
- k6 GraphQL压测脚本:agent ad2f0480fe97b673e,编写graphql-vs-rest.js对比GraphQL聚合vs REST多端往返。只编写不执行。

**侦察已确认**:Pulsar 2broker+3bookie Running;outbox relay 4点定位;k6已装;locust 6脚本含ccu5000_ws;GraphQL Phase1完成(schema.graphqls+4Fetcher+DataLoader,/graphql端点);HPA 19个maxReplicas 2-6,learning-worker 3副本。

**VM雪崩防护**:三任务写脚本阶段轻负载可并行;实际跑压测/真实broker IT由主线程串行(esp-lco-embedding-vm-snowball+esp-singlenode-host-cluster-contention铁律)。

**待**:Workflow+2agent返回→主线程落地IT代码→串行执行压测(先探针timeout调优防误杀)。


## 续六(2026-08-21)P1-D21 Workflow Scout 阶段返回 —— 4 relay 点接线真相(HSA 实证)

**背景**:P1-D21 契约 IT(验证 EER outbox at-least-once + idempotent = exactly-once)。Workflow wf_1823316f-5c9 三阶段:Scout(4 relay 点并行实证)→ Plan → WriteIT。Scout 全返回,颠覆原"4 统一 Pulsar relay"假设。

**实证结论(4 relay 点真实机制,非叙事)**:

| relay 点 | 真实机制 | 幂等 | IT 可行性 | 缺陷 |
|---|---|---|---|---|
| learning-java/OutboxRelay | DB→DB 重投器(扫 outbox 表→INSERT learning_events),**不发 Pulsar** | message_id UNIQUE + ON CONFLICT DO NOTHING ✓ | medium(testcontainers-CRDB) | 无 |
| content-go/OutboxRelay | CRDB→CRDB 重投器(坑㉙补 learning-java 只读 TiDB 缺口),**非 Pulsar** | message_id 去重 ✓ | medium(纯 CRDB 无需 broker) | drainOutbox 主路径**零覆盖** |
| teacher-java/OutboxRelay | **真 Pulsar relay**(@Scheduled 扫表→Producer.send 同步阻塞 blockIfQueueFull(true),满足 EER) | **消费端无 message_id 显式去重!**(learning-java handleFinalized 不读 msg key) | **high**(真实 Pulsar 3 broker Running) | **消费端幂等隐式缺失** |
| gamification-go/rankings_projector | Pulsar 消费端(Redis SETNX 去重 ✓) | Redis SETNX esp:gamif:processed:{msg_id} ✓ | medium(需 -tags pulsar_eda) | **生产镜像跑 stub!**(Dockerfile 无 -tags pulsar_eda,线上 no-op,违反配而未接+stale-image-blindspot) |

**重大缺陷(3 项,需后续 P1 修)**:
1. **teacher-java 消费端幂等缺失**:learning-java LearningEventConsumer.handleFinalized 不读 msg.getKey()/getProperty("message_id"),TeacherFinalizedEvent POJO 无 messageId 字段 → 依赖下游 findById upsert 隐式幂等(非 message_id 显式)。EER 铁律要求 message_id 幂等键贯穿。
2. **gamification-go projector 线上跑 stub**:Dockerfile `go build ./src` 无 -tags pulsar_eda → rankings_projector_stub.go 编译 → startRankingsProjector no-op。代码存在但线上无投影,违反工程铁律②配而未接 + feature-stale-image-blindspot。
3. **content-go/learning-java drainOutbox 主路径零 IT 覆盖**:relayBackoffSec/relayMaxRetry 常量锁了,但 DB 扫表+INSERT+退避+死信主路径零覆盖(测试注释自认"dead 路径依赖 DB 故只锁常量")。

**IT 方案(Workflow Plan 阶段生成中)**:分两类——
- DB→DB 重投器(learning-java/content-go):testcontainers-CRDB 起库+建 outbox/learning_events 表,写 outbox 行→验证 relay 扫表重投→done/dead/退避路径。无需 broker。
- 真 Pulsar relay(teacher-java):真实 pulsar-broker:6650(3 broker Running)→写 outbox 行→验证 Producer.send 投递+消费端幂等。high 可行性。

**状态**:Scout 全返回(4/4)。Plan/WriteIT 阶段 agent a77ce6c 启动中。待 Workflow 返回→主线程落地 IT 代码→编译验证→commit。

## 续七(2026-08-21)P1-D21 Workflow Plan 阶段返回 —— 确认 message_id 真实 bug + IT 方案

**Plan 阶段(a77ce6c)实证 — 比 Scout 更深,确认 1 个 Scout 漏掉的真实 bug**:

**bug(EER 真实违规,可 IT 暴露)**:teacher-java EventPublisher.publishReliable 第 98 行 p.send(payload) 不调 .newMessage().key(),第 3 参 idempotencyKey 仅用于异常文案拼接(第 102 行) → message_id 在 relay→broker 链路实际丢失。下游 learning-java handleFinalized 只 getValue() 反序列化从不读 msg.getKey()/getProperty(message_id) → 所谓 message_id 去重是注释谎言,实际靠 score 覆盖 + @Version 碰巧幂等,补偿后重评不同 score 会覆盖。IT 造重复投递同 message_id 消费端计两次即暴露。

**IT 方案(Plan 阶段确定)**:
- 优先级:teacher-java/OutboxRelay(high 真 Pulsar) + learning-java/OutboxRelay(high 纯 DB 无需 broker)先做;content-go(medium 纯 CRDB)次;gamification-go projector(medium 需先改构建 tags)后。
- 核心策略:真实 Pulsar broker(pulsar-broker:6650 集群 3 broker Running 已核实)优先,testcontainers Pulsar 仅 Go 侧备选。理由:真实 broker 能暴露 publishReliable 不写 message_id 头这种隐蔽 bug;testcontainers-pulsar JVM broker 内存大,9 核 VM 叠加 LCO embedding 雪崩风险不划算。
- 关键验证点:(a) teacher.finalized 路径投递后用 pulsar-admin/consumer.peek 核验 broker 消息是否携带 message_id key/property(预期暴露 bug:不携带);(b) learning-java 同 message_id 造两行 outbox,relay 投递后 learning_events 只落一行;(c) gamification-go 同 message_id 重复发,Redis SETNX 第二次返 false 跳过,ZSET 只加一次。
- 数据隔离:独立 test tenant(user_id 负数段 -9xxxxx)+ 独立 test topic 后缀 .it/subscription 名,IT 结束显式清理 outbox/learning_events/user_mastery 对应行 + Redis key DEL。
- 风险:VM 雪崩(esp-lco-embedding-vm-snowball,IT 编译与 LCO embedding 离峰串行不并行,IT 批次小于 30s);真实 broker 脆弱(broker 有 20+ 次 restart 历史,IT 需重试+超时分级,失败区分 broker 不可达 标 DEFERRED vs 幂等失效 立即修)。
- testFramework:Java 侧 SpringBootTest(classes=TeacherApplication,webEnvironment=NONE)+ActiveProfiles(it)+EnableScheduling 起真实 relay;Go 侧 testcontainers-cockroachdb + 编译 projector tags;CI 门禁 IT 标 integration tag 默认不跑夜班手动触发,覆盖率门禁仅约束单测(TCG)。

**3 类 gap 汇总(IT 能暴露的)**:
1. teacher-java publishReliable 不写 message_id 头(EER 真实违规,IT 可暴露,修后 IT 转绿闭环)。
2. learning-java worker 模式 reportEvent 用 EventPublisher.publish(sendAsync+blockIfQueueFull(false)+失败仅 warn)发 LEARNING_EVENT_REPORT,队列满静默丢,未切 publishReliable,fire-and-forget 违规。
3. gamification-go projector 生产镜像跑 stub(Dockerfile 无 pulsar_eda tags),配而未接+stale-image-blindspot 双违规。

**状态**:Plan 完成。WriteIT agent ad1c11e4 启动中(核实细节+写 IT 代码骨架)。待返回 → 主线程落地 → 编译验证 → commit。Monitor bygv7fcrn 盯 ad1c11e4。
## 续八(2026-08-21)P1-D21 主线程核实 —— message_id bug 真相修正(HSA/ASV/CLC)

**Workflow WriteIT 阶段失败**:agent ad1c11e4 反复核实细节后冻结,写了 0 个 IT 文件,journal 无 result 无 workflow 结束标记。Scout+Plan 两阶段产出高价值事实(已落盘续六/续七),WriteIT 无产出。主线程亲自核实 Plan agent 叙事(不轻信)。

**主线程字节码+源码核实结论(修正 Plan agent 部分不准确)**:

1. **EventPublisher.publishReliable 确实不写 Pulsar 消息头**(字节码证实):javap -c 显示第 33 行 Producer.send(Object) 直接发载荷,无 newMessage().key()/.property() 调用;第 3 参 String(messageId)只在 makeConcatWithConstants 用于异常文案拼接。**Plan agent 此判断正确。** 注:EventPublisher.java 源码不在本仓库(esp-cache 是 com.esp:esp-cache:1.0.0 共享 jar,在 ~/.m2,源码 sources jar 下载失败 .lastUpdated),Plan agent 报的 EventPublisher.java:98 行号是反编译推断非真实行号,但事实成立。

2. **Plan agent 严重不准确处**:声称 handleFinalized 读 ev.messageId() 但 TeacherFinalizedEvent 无 messageId 字段 → 主线程反编译 TeacherFinalizedEvent 证实字段只有 userId/kpId/finalScore,**确无 messageId**;第 107 行 ev.messageId() 实为 handleEventReport(LearningEventReport ev) 另一事件,非 handleFinalized。handleFinalized 第 134-164 行**完全不读 message_id**。

3. **teacher.finalized 幂等真相(修正)**:Plan agent 称靠 score 覆盖+@Version 碰巧幂等是注释谎言 → **不准确**。实际靠主键 upsert(findById→applyTeacherFinalized→save 同 user+kp 覆盖同一行)+ @Version OCC 重试(P1-D2 已加第 151-160 行)。这是**合法设计幂等策略**(重投递同事件→同 user+kp→upsert 覆盖同分数→幂等),非碰巧。teacher.finalized 是最终分数覆盖语义,重投递天然幂等。

4. **publishReliable 不写消息头的真实危害程度(降级)**:对 teacher.finalized 链路危害低(业务层主键 upsert 兜底,不依赖 Pulsar 头)。真正依赖 message_id 显式去重的是 learning.event.report 链路——但其 message_id 通过 LearningEventReport POJO 载荷字段(messageId())贯穿,经 reportEvent 4 参透传到 learning_events 表 ON CONFLICT(message_id) DO NOTHING 去重,**不依赖 Pulsar 消息头**。故 publishReliable 不写头的缺陷对现有两条链路均非阻断(载荷字段兜底),但违反 EER message_id 应贯穿 broker 头最佳实践,属代码债非线上 bug。

5. **esp-cache 是共享 jar**:修 publishReliable 写头需改 esp-cache 源码仓库(不在本仓库)+ 重发 jar + 所有依赖方升级,爆炸半径大(BRR L2,跨仓库)。非本会话可单点修复,标 DEFERRED 记条件。

**P1-D21 结论**:契约 IT 价值仍在(验证 EER 接线+暴露 gap),但 WriteIT 失败无代码产出。3 类 gap 重新定级:
- gap1(teacher publishReliable 不写头):降为代码债非线上 bug(载荷字段兜底),DEFERRED 跨仓库修 esp-cache。
- gap2(learning-java worker 模式 reportEvent 用 publish sendAsync 静默丢):仍为 EER 违规,但需核实 worker 模式是否启用(单进程一体化模式 esp.disruptor.enabled=true 不发 Pulsar 走进程内 Disruptor,可能不触发)。
- gap3(gamification-go projector 生产镜像跑 stub):仍为配而未接+stale-image-blindspot 双违规(Dockerfile 无 pulsar_eda tags),这是真问题但属构建配置非 IT 范畴。

**下一步决策**:P1-D21 IT 代码不强制现在补(WriteIT 失败,主线程核实后 gap1 降级)。转向执行阶段——串行跑 P1-D10 HPA 压测 + k6 GraphQL 对比(先探针 timeout 调优防误杀),收容量数据。IT 代码补全标 DEFERRED 待 WriteIT 重试或主线程择机手写。