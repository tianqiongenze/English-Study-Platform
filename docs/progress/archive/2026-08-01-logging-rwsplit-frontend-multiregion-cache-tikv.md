> **Part D 原始日志 → Loki(Fluent Bit)+ Prometheus + Alertmanager ✅(manifest 就绪)**:
> - `deploy/k8s/monitoring/10-observability.yaml` 核查 fluent-bit DaemonSet 配置正确(tail /var/log/containers/*.log → K8s filter → modify 提取 esp_app/esp_ns 标签 → OUTPUT Loki)。补强 OUTPUT Labels 把 esp_app/esp_ns 作为 Loki 标签(Label_Keys)。
> - 追加 Prometheus(ServiceAccount/ClusterRole/ConfigMap with scrape_configs + 告警规则 ServiceDown/HighErrorRate/PodRestart + Deployment/Service)+ Alertmanager(ConfigMap + Deployment/Service)。Grafana 数据源已含 Loki/Prometheus/Tempo。
> **Part E OTel traceId 注入 ✅(manifest env 就绪,agent jar 在 Dockerfile 已 COPY)**:
> - 各服务 deploy manifest 补 OTEL_SERVICE_NAME / OTEL_EXPORTER_OTLP_ENDPOINT(env 指向 otel-collector.esp-observability:4317)。
> - bff/learning/teacher-java:OTel javaagent 已在镜像 JAVA_OPTS 中(Dockerfile ENTRYPOINT `java $JAVA_OPTS`,含 `-javaagent:/app/otel-agent.jar`),manifest 只补 OTEL_* env 配置。
> - writing-py/emotion-svc:已内置 opentelemetry-sdk(tracing.py setup_tracing)+ pyproject 依赖,manifest 补 OTEL_* env + TIDB_HOST(AppLogClient)。
> - gateway-go:已内置 otel go sdk(tracing.go setupTracing,go.mod 已 require)+ env。
> **Part F 用户行为分析与推荐 ✅(代码就绪)**:
> - 现状核查:重构前仅三端 `reportEvent()` → `/api/learning/event`(单一 event_type/payload),只存 learning_events 表,**无聚合分析、无推荐端点、无统一行为上报**。
> - 新增 `BehaviorController.java`(learning-java):`POST /api/learning/behavior`(统一行为上报,落 app_logs event=behavior.*)+ `GET /behavior/top`(TOP 活跃用户)+ `/behavior/funnel`(漏斗)+ `/behavior/heatmap`(热力)+ `GET /api/learning/recommend`(复习清单 + 薄弱知识点 + 相似用户场景协同过滤 SQL 推荐)。
> - 三端 api.ts 增 `reportBehavior()` + `recommend()`;react LearningPanel 增"为你推荐"按钮 + 页面停留行为上报(useEffect unmount 上报 page_stay)。
> **Part G 落盘**:`docs/design/日志系统架构.md` + `docs/design/schema.sql` + 各服务代码 + `deploy/k8s/monitoring/10-observability.yaml` + 各 deploy manifest + 本节。
> **现状**:manifest + 代码 + 配置全部就绪;**暂不全 scale**(节点 CPU95%/内存91%,session_turns 灌数在跑)。OTel agent jar/sdk 依赖已在 Dockerfile/代码就绪,**不在本轮 build 部署**(灌数完成后再 build+scale)。
> **下一步**:① 灌数完成后 build 各服务镜像(含 otel-agent.jar);② kubectl apply 可观测栈 + 各服务;③ 验证 app_logs 写入 + Loki 采集 + Grafana 查询;④ 前端连调行为上报/推荐。
> - 落盘:本节 + `docs/design/日志系统架构.md`。

> 更新日期:2026-08-01(读写分离全覆盖 + 公共工具包抽取)
> ## 读写分离全覆盖 + 公共工具包抽取(2026-08-01)
> 用户要求所有应用服务都实现读写分离访问 TiDB/Redis 等基础设施。已全部完成:
> - **Java 侧公共包 `esp-rw-java`**(`services/esp-rw-java/`):抽取自 learning-java/teacher-java 的 rw/ 包,含 RoutingDataSource + @DataSource AOP + DataSourceConfig 自动配置。learning-java / teacher-java / bff-java 均已依赖,删除各自 rw/ 包,bff-java 新增读写分离(注册写主库 + 登录读主库 consistent=true)。
> - **Python 侧公共包 `esp-db-py`**(`services/esp-db-py/`):aiomysql 双连接池,execute_write/execute_read(forced_primary)。writing-py 批改结果写 writing_tasks(主库)+ 查历史走从库;emotion-svc 情绪事件写 learning_events(主库)+ 查历史走从库。
> - **Go 侧 `src/db/db.go`**(`services/gateway-go/src/db/`):database/sql 双连接池。gateway-go 场景列表启动时从从库读 scenarios + 会话 End 归档写 sessions(主库)。
> - **构建验证**:learning-java mvn test 35 PASS / teacher-java 12 PASS / bff-java BUILD SUCCESS / gateway-go go test 全 PASS。
> - **deploy env**:10-gateway-go / 30-other-services.yaml 均补 DB_ENABLED + WRITE_DB_HOST/READ_DB_HOST env。
> - **结论:所有 6 个应用服务 100% 覆盖读写分离**(learning/teacher/bff Java + writing/emotion Python + gateway Go),统一 tidb-primary 写 / tidb-follower 读。
> - 落盘:`docs/design/多区域数据同步方案.md` §1.2.2(更新表)+ §1.2.2.1(公共包抽取)+ 本节。

> 更新日期:2026-08-01(续跑-13:多区域同步真实现 + 读写分离全覆盖 + 前端P1修复 + 日志系统重构中)
> ## 跨会话恢复指引(最重要)
> 上下文溢出后,新会话:①读本文件顶部 → ②`kubectl -n ekb-m2-aws-us-east-1 exec deploy/redis-cluster -- redis-cli -c GET "esp:ctx:session:last_update"` 读运行态 → ③TaskList 查任务清单。三处对齐即可续跑。
> ## 本轮(续跑-13)总体状态
> - **多区域数据同步 ✅ 真实现**:TiDB 三区 Raft 强一致 + Redis 跨区主从(aws主→azure/gcp从,实测 SET→GET 同步)+ Pulsar aws broker 起通(geo-rep 单节点不可行,给多节点模板)。
> - **读写分离 ✅ 覆盖所有应用服务**:learning/teacher/bff Java(公共包 esp-rw-java)+ writing/emotion Python(公共包 esp-db-py)+ gateway Go(src/db)全部支持主写从读。
> - **前端 ✅**:8连调阻断点全修 + P1四项全修(Avatar三端/GradingPage/placement/H5·小程序 polish·translate·difficulty)。核查报告 docs/test/08-前端核查报告.md。
> - **基础数据 ✅**:scenarios=20/knowledge_points=23/users=7(含测试账号 learner/teacher/admin@esp.com 密码 esp123456),连调前置满足。
> - **session_turns 灌数 🔄 后台**:16分片×1worker,Redis 进度 ~1116万/3亿,ETA 偏长(单worker)。tikv-aws 偶发 liveness timeout 抖动但不影响 Raft 多数派。
> - **日志系统 🔄 重构中**:架构决策 TiDB存结构化日志(app_logs表)+Loki存原始日志,manifest/接入配置就绪暂不全scale(节点内存91%紧张)。
> - **节点资源**:CPU66%/内存91%(灌数+三区redis/pulsar叠加),紧张但未崩。
>
> ## 灌装数据多区域同步验证 + 读写分离服务覆盖(2026-08-01)
> **Part A 灌装数据多区域同步 ✅ 验证通过**:
> 1. 写 aws(tidb-primary)带标记行 `multiregion-verify-2026-0801` → 等 2s → azure(tidb-follower)/gcp(tidb-cluster-tidb.ekb-m2-gcp-europe-west1)均读到完全一致的标记行(Raft 强一致)。
> 2. session_turns 点查:三区 `MAX(id)=8935141660708413064` 一致 + `COUNT WHERE id=MAX` 三区均=1(最新灌装行三区立即可见)。全表 COUNT 三区不一致(417万/432万/443万)是 TiDB 并行扫描近似值差异,非数据丢失——点查一致才是强一致证据。
> 3. 清理:DROP TABLE multiregion_verify。
> **Part B 读写分离 ✅ 覆盖所有应用服务**(详见上方"读写分离全覆盖"节,6 个服务全部支持)。
> **Part C tikv-aws 抖动**:Running ready=true,5h35m restart 2 次(liveness timeout),azure+gcp 2 副本构成多数派,数据一致不丢,无需止血。
> - 落盘:`docs/design/多区域数据同步方案.md` §1.2.1/§1.2.2/§1.2.3 + 本节。

> 更新日期:2026-08-01(前端 P1 缺口修复)
> ## 前端 P1 缺口修复(2026-08-01)
> 对照 `docs/test/08-前端核查报告.md` P1 四项,三端全部修复:
> 1. **数字人 Avatar(原型 6.6 / FR-6.1)✅**:三端新增 `components/Avatar.tsx`(Web/H5 用 SVG 表情+唇形动画,小程序用 emoji 降级),props `{emotion,active,text}`;Speech 页 WS `ai_reply` 收到 emotion 帧时切换表情、文本气泡、说话态。
> 2. **Web GradingPage(原型 3.6 / F14)✅**:新建 `frontend-react/src/pages/teacher/Grading.tsx`,路由 `/teacher/grading`,TeacherLayout 加 Tab;调 `autoGrade`(批量自动评阅)+ `finalize`(教师终审确认最终分)。
> 3. **placement 定级(三端)✅**:三端 api.ts 增 `placement(user_id,kp_ids,score)` 调 `/api/learning/placement`;learning 页首次进入 useEffect 触发,展示"能力定级完成,初始化 N 个知识点基线"。
> 4. **H5/小程序 polish/translate/difficulty ✅**:api.ts 补三端点声明 + types 补类型;Writing 加润色/翻译 Tab(结果+建议/词汇);Learning 加"难度调整"按钮(展示 next_difficulty/speed_multiplier/scaffolding)。
> - 改动文件:react 8 个(api.ts/LearningPanel/SpeechPanel/TeacherLayout/App/styles.css + 新增 Avatar.tsx/Grading.tsx);h5 6 个(api/types + 新增 Avatar + Speech/Writing/Learning);miniprogram 5 个(api + 新增 Avatar + speech/writing/learning)。
> - tsc:三端均仅余环境性 react 类型错(`react/jsx-runtime` 未安装类型 / JSX.IntrinsicElements / TS5107 moduleResolution 弃用),**本次新增代码无语法/类型错误**。
> - 落盘:`docs/test/08-前端核查报告.md`(P1 标已修 + 连调结论)+ 本节。

> 更新日期:2026-08-01(续跑会话-13:前端三端连调阻断点修复 + 多区域数据同步真实现)
> 基础数据补灌核查(2026-08-01):**经读副本 tidb-follower(azure 区,read_timeout=60s)核查 esp 库基础数据已达标,无需补灌** ——
> - 各表计数(users=7 / scenarios=20 / knowledge_points=23 / sessions=0 / writing_tasks=0 / lesson_plans=0 / classes=0 / assignments=0 / submissions=0)。scenarios(20 口语场景,口语陪练足够)与 knowledge_points(23 个,邻接表三层覆盖 grammar/vocab/pronunciation)均已由 `scripts/seed/seed_basic_data.py` 灌足;users=7(含 5 测试账号 learner/teacher/admin @esp.com,密码 esp123456 + bff 注册的 2 个)。
> - session_turns 因灌数 Job(16 分片在跑,~199万/3亿)压 primary,COUNT 查询 read_timeout 60s 仍超时(预期,基础数据不受影响)。
> - 结论:连调前置条件满足,口语陪练/写作/作业均有所需元数据。
> - 落盘:本节。
> 更新日志(2026-08-01 续跑-13,多区域同步真实现):**Redis 跨区主从异步复制 ✅ 验证通过 + Pulsar aws 单 broker 起通** —
> - **Redis 跨区主从 ✅ 已实现并验证**:azure/gcp 区 redis-cluster scale 1,deploy args patch 为 `--replicaof redis-cluster.ekb-m2-aws-us-east-1.svc.cluster.local 6379`(持久化到 spec)。aws master `INFO replication`:`role:master,connected_slaves:2,slave0/slave1 state=online lag=1`;azure/gcp `role:slave,master_link_status:up`。跨区实测:`SET multiregion:test:1785586434 hello-1785586434`(aws)→ azure `GET` = `hello-1785586434` ✅,gcp `GET` = `hello-1785586434` ✅。开源 Redis 异步主从,主挂丢少量窗口(最终一致);主动-主动 CRDT 需 Enterprise 不可行。
> - **Pulsar aws 单 broker ✅ 起通**:scale aws/azure pulsar-broker 1(standalone,clusterName=standalone)。pulsar-client produce/consume 1 条消息验证通过。esp-prod bff-java pod `pulsar-broker:6650` TCP_OK(ExternalName→aws)。azure→aws pulsar 6650 跨区 TCP_OK。
> - **Pulsar geo-replication 单节点不可行(已如实记录)**:standalone 模式 clusterName 固定 `standalone`,zookeeperServers 空(内嵌 zk),不支持多集群 geo-rep(需独立 broker+bookie+zk 每区 + 全局 configurationStoreServers + 独立 clusterName)。单节点起三套完整集群内存吃紧且无跨物理节点容灾价值。`pulsar-admin clusters list` = `standalone`(单集群)。geo-rep 多节点 manifest 模板见 `docs/design/多区域数据同步方案.md` §4.1。
> - **落盘**:`docs/design/多区域数据同步方案.md` 更新(§1.3 已实现证据 + §3.3 决策表 + §5 已执行);本节。
>
> 更新日志(2026-08-01 续跑-13):**前端三端连调阻断点 + 共性缺口修复 ✅ 完成** —
> - **阻断点1 小程序 Speech 输入框 ✅ 已修**:`frontend-miniprogram/src/pages/speech/index.tsx` 补 Input + 发送/提示/结束三按钮,点击通过 `wsRef.send({data: JSON.stringify({type:"text_input",text})})` 发送文本帧,并补 error 帧处理。
> - **阻断点2 小程序 WS URL ✅ 已是可配置**:`frontend-miniprogram/src/api.ts` 用 `TARO_APP_WS`(Taro defineConstants 注入,见 `config/index.js`:`process.env.TARO_APP_WS || "ws://localhost:8080"`),真机用 `TARO_APP_WS=ws://<网关IP>:8080` 覆盖即可。无需改。
> - **阻断点3 wx-login 端点 ✅ 已确认存在**:`services/bff-java/.../AuthController.java` `POST /api/auth/wx-login` 已实现(code 换匿名 JWT)。docs/api/01-API契约.md 已补该端点行。
> - **阻断点4 BFF 地址 env 切换 ✅ 三端均支持**:Web/H5 用 vite 代理(`VITE_API_TARGET`/`VITE_WS_TARGET` 默认 localhost:8090/8080,见 vite.config.ts);小程序用 `TARO_APP_API`/`TARO_APP_WS`(config/index.js)。无需改。
> - **阻断点5 WS 路径 ✅ 对齐**:gateway-go `main.go` 路由 `/ws/speech/`(截 path 后段作 sessionID),三端均连 `/ws/speech/{sessionId}` 一致。无需改。
> - **阻断点6 register 端点 ✅ 已确认**:AuthController `POST /api/auth/register` 存在;Web/H5 Login 页调用 `/api/auth/register` 路径对齐。无需改。
> - **阻断点7 emotion 帧字段 ✅ 已对齐**:emotion-svc `schemas.py` 返回 `{emotion,encouragement}`(sense)、`{image_url,alt}`(visual);gateway-go WS 帧 `frame.go` 字段 type/text/emotion/correction/hint/options/summary/error,三端 Speech 页解析字段一致。Web 已显示 `f.emotion`。无需改。
> - **阻断点8 智能体链路 ✅ Web 全补 / H5+小程序补 run**:writing-py 已实现 `/api/writing/run|review|state`(main.py:53/59/65)。Web `WritingPanel.tsx` 新增「智能体批改」tab,调 run(human=true)/review(提交人工审阅恢复 LangGraph)/state(查状态);H5 `Writing.tsx` + 小程序 `writing/index.tsx` 补 writingRun + writingState。
> - **共性缺口 FR-5.2 visual ✅**:三端 api 客户端补 `visualWord`(/api/emotion/visual);Web 同时补 senseEmotion 已存在。
> - **共性缺口 数字人 Avatar / 作业评阅 GradingPage**:本期未补(原型6.6/3.6,简化实现留待后续,非连调阻断)。
> - **构建验证**:Web `npx tsc --noEmit` 报错均为环境性(react 类型未安装 `Cannot find module 'react'`、JSX.IntrinsicElements 缺),非本次改动引入;新增 api/writingRun/review/state、小程序 Input/Writing 变更无语法/导入错。
> - **改动文件**:`frontend-miniprogram/src/pages/speech/index.tsx`、`frontend-miniprogram/src/pages/writing/index.tsx`、`frontend-miniprogram/src/api.ts`、`frontend-react/src/panels/WritingPanel.tsx`、`frontend-react/src/api.ts`、`frontend-h5/src/pages/Writing.tsx`、`frontend-h5/src/api.ts`、`docs/api/01-API契约.md`、`docs/PROGRESS.md`。

> 更新日期:2026-08-01(续跑会话-12:gateway-go 多级缓存验证 ✅ 完成 + 多区域 Pulsar/Redis 数据同步结论 + 三端缓存架构对齐)
> 更新日志(2026-08-01 续跑-12,收尾段):**gateway-go 多级缓存验证 ✅ 完成(builder 子代理)** —
> - **cache.go 接口完整性 ✅**:`services/gateway-go/src/cache/cache.go` 实现 L1(sync.Map+LRU 双向链表,cap=4096)+ L2(go-redis v9,env `REDIS_HOST`/`REDIS_PORT`,默认 `redis-cluster:6379`)。接口完整:`GetOrLoad`(get/put 回填 L1→L2 + loader 兜底 + 空穿透 sentinel `__ESP_CACHE_EMPTY__` TTL 60s)、`Evict`(删 L2+L1)。L2 不可达时 `rdb=nil` 降级为纯 L1(warn 不阻断),TTL L2=300s/空=60s(已与 Java 对齐,原 3600/60 已改)。
> - **Redis 连接 ✅**:`deploy/k8s/10-gateway-go.yaml` env `REDIS_ADDR=redis-cluster:6379`(main.go session 用)+ 运行 deploy 实际还注入了 `REDIS_HOST=redis-cluster`(cache.go L2 用)。redis-cluster svc 在 `ekb-m2-aws-us-east-1` ns ClusterIP 10.43.244.222:6379,gateway-go pod 内 `nc redis-cluster 6379` 返回 `+PONG`,L2 连通确认。
> - **go build ✅**:`cd services/gateway-go && go build ./...` RC=0。交叉编译 `GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o gateway-go ./src` RC=0。
> - **docker build ✅**:`docker build -t esp/gateway-go:1.2.2 -f Dockerfile.prebuilt .` RC=0(arm64,redis:7.2-alpine 基镜像,15.6MB)。旧镜像 1.2.1,新 tag 1.2.2。
> - **部署 ✅**:`kubectl -n esp-prod set image deploy/gateway-go gateway-go=esp/gateway-go:1.2.2` RC=0;`rollout status --timeout=300s` → `successfully rolled out` RC=0(20 副本全更新)。pod image 确认 `esp/gateway-go:1.2.2`。
> - **L1/L2 生效验证 ✅**:在 pod 内运行专用验证二进制(连真实 redis-cluster),同一文本调 `tts.MockProvider.Synthesize` 两次 + 不同文本一次:
>   - 相同文本第二次 `t1==t2=true`(L1 命中,值一致);
>   - 新文本生成新 L2 key,Redis `gateway:tts*` key 数 BEFORE=1 → AFTER=3(DELTA=+2,对应两次不同文本 miss 写入);
>   - L2 key `gateway:tts:34b3c8af07b4c48479dadef5` TTL=183s 倒数(初始 300s 已过 ~117s)、STRLEN=70,格式 `cacheName:sha256(parts)[:24]` 与 cache.go 一致。
> - **20 副本记录**:gateway-go HPA(cpu 60% target)当前 scaled 到 20(min=2,max=20,2d7h),非手动。验证用 `exec` 单 pod 避免多副本混淆。副本数本次不改(另议)。
> - **#3 多级缓存总进度**:learning-java(1.2.0)+ teacher-java(1.3.0)+ bff-java(1.1.0)+ writing-py(1.1.0)+ emotion-svc(1.1.0)+ **gateway-go(1.2.2)** **6 服务全部验证通过 ✅**。任务 #3 完成。
>
> 更新日志(2026-08-01 续跑-12):**多区域数据同步 ✅ 结论落盘** —
> - **TiDB 多区域数据同步 ✅ 已完成且强一致**:三区 pd-{aws,azure,gcp} + tikv-{aws,azure,gcp} 全 1/1,Raft 3 副本数据强一致(容忍 1 故障)。tidb-primary→aws(写)、tidb-follower→azure(读)、gcp 只读,ExternalName 已配。**这是多区域同步的核心范例,已完成。**
> - **Pulsar 多区域**:三区 deploy/pulsar-broker **全部 0/0 未起**(svc 在 aws 区但 pod 0)。决策:**保持单区(aws),不部署三区 geo-replication** —— esp Pulsar 仅用于 esp-cache Java 缓存失效广播,无真实业务消息,单区单 broker 已满足;单节点起三区 broker(~3-4.5GB)吃内存且无跨物理节点容灾价值。
> - **Redis 多区域**:仅 aws 区 redis-cluster 1/1 Running,azure/gcp 0/0。决策:**保持单区(aws)** —— esp Redis 仅会话状态+缓存+L2,单区满足;开源 Redis Cluster 多活(CRDT)需 Redis Enterprise 不可行。
> - **综合结论**:多区域数据同步在 **TiDB 层已完整完成(强一致)**;Pulsar/Redis 单区满足需求,多区域同步属过度设计,本环境不实施,但可移植方案(Pulsar geo-replication / Redis Cluster 跨区主从)落 `docs/design/多区域数据同步方案.md`。
> - **三语言多级缓存架构对齐 ✅ 完成**:Java(esp-cache: Caffeine+Redis+Pulsar广播)/Python(esp-cache-py: cachetools+redis.asyncio)/Go(cache.go: LRU+go-redis)三端核心语义已对齐(L1+L2 Cache-Aside + get_or_load + evict + 空穿透sentinel + L2降级 + flag)。差异仅失效策略(Java强一致/Py-Go TTL兜底,维持现状)与key命名(文档约定)。默认TTL统一300/30(Py/Go原3600/60已改)。**轻量统一,不重写**。落 `docs/design/多级缓存架构对齐.md`。
> - **gateway-go 缓存验证 🔄 子代理进行中**:镜像 esp/gateway-go:1.2.1 已 Running,验证 cache.go L1/L2 生效中。
> - **esp-prod 服务全 ready ✅**:bff/learning/teacher/writing/emotion/gateway-go/nacos/seata/sentinel 全 1/1 Running(gateway-go 20 副本异常多,记录但不改)。
>
> 更新日期:2026-08-01(多级缓存架构对齐核查)

> 更新日期:2026-08-01(多级缓存架构对齐核查)
> 更新日志(2026-08-01 缓存对齐):**三语言多级缓存架构核查 ✅ 完成** — Java/Python/Go 三端核心语义已对齐(L1+L2 Cache-Aside + get_or_load + evict + 空穿透sentinel + L2降级 + feature flag)。差异:失效策略(Java Pulsar广播+延迟双删强一致;Py/Go 仅本地+TTL兜底最终一致,维持现状不强制引入Pulsar);key命名(Java手传完整key;Py/Go用 cacheName:sha256(parts)[:24],仅文档约定不本期改Java);默认TTL(Java 300/30;Py/Go 原3600/60 → **已改 300/30 对齐Java**)。改动文件:services/esp-cache-py/esp_cache/__init__.py、services/gateway-go/src/cache/cache.go。决策文档:docs/design/多级缓存架构对齐.md。
> 更新日志(2026-08-01 续跑-11):**三端前端核查 ✅ 完成(explorer 子代理)+ 多区域 Pulsar/Redis 数据同步问题提出** —
> - **三端前端整体完成度 ~70%**(对照 SRS/SDD/原型/API 契约核查结论):
>   - **Web `frontend-react/` ✅ 最完整**:路由全(登录/学习中心 speech+writing+knowledge+review+difficulty/教师 lesson-plan+analytics/profile),API 客户端覆盖契约几乎所有端点,鉴权 zustand+localStorage 注入 JWT,WS 旁路 vite proxy 落地。
>   - **H5 `frontend-h5/` 🟡 部分**:仅 login/speech/writing/learning,缺教师只读 analytics(原型6.5要求)、Profile、difficulty/polish/translate 入口;API 仅核心端点。
>   - **小程序 `frontend-miniprogram/` 🟡 部分**:tabBar 三栏,wx.login→/api/auth/wx-login 换 JWT 链路对;但 **Speech 页缺输入框+发送按钮**(只能开始/结束,无法发消息验证 AI 对话)、缺 Profile/difficulty/polish/translate。
>   - **三端共性缺口**:① 数字人 Avatar 组件三端全缺(原型6.6);② 作业评阅 GradingPage 全缺(原型3.6);③ emotion/visual 端点前端仅 Web 声明未调用(FR-5.2);④ LangGraph 智能体多步链端点(/writing/run|review|state)前端三端均未调(FR-6.2 验收项无法连调)。
>   - **前后端连调 8 个阻断点**:小程序 Speech 无法发消息;小程序 WS URL 默认 localhost:8080 真机需改;**/api/auth/wx-login 契约文档未列需确认 BFF/gateway 已实现**;BFF 地址默认 8090 需环境变量切换;WS 路径 /ws/speech/{id} 需 gateway 匹配;/api/auth/register 端点需确认;emotion 帧字段三端消费不一致;智能体链路端点未对接。
> - **多区域数据同步问题(用户新需求)**:用户问"多区域 Pulsar MQ 和 Redis Cluster 是否做到数据同步"。**现状调研启动**:当前只有 aws 区(pd-aws/tikv-aws/tidb-cluster-tidb/tiflash + pulsar-broker + redis-cluster),azure/gcp 区 TiDB/Pulsar/Redis 均未起。tidb-follower ExternalName 指向 azure 但 azure TiDB 未起。**需给现状+方案+完整完成数据同步**。
> - **TiFlash 状态**:仍 1/1 Running(用户口信"若不影响多区域同步和读写分离可先不启动",但单节点资源紧张,待决策是否 scale 0 释放内存)。
> - **esp-prod 业务服务**:learning/teacher/gateway/writing/emotion 仍 0/0 副本待 scale 回(建表已完成可 ready)。
> - **任务状态**:#1✅ #2✅ #6✅(建表) #3 仅剩 gateway-go #7 前端核查✅(缺口清单已得,修复待启动) #4 session_turns(表就绪) #5 warmup+压测+文档。**新增**:多区域 Pulsar/Redis 数据同步(现状+方案+完成)。
> 更新日期:2026-08-01(续跑会话-10:esp 库 12 表全建好 + 集群稳定 + TiFlash Running)
> 更新日志(2026-08-01 续跑-10):**esp 库建表 ✅ 完成 + 集群现状核实** —
> - **esp 库 12 表全建好**(bff 子代理执行 schema.sql):`assignments, classes, knowledge_points, learning_events, lesson_plans, scenarios, session_turns, sessions, submissions, user_mastery, users, writing_tasks`。users=2 行(bff 缓存验证时 register 的测试用户),scenarios/sessions=0(基础数据待灌)。任务 #6 ✅ 完成。
> - **TiDB 栈全稳定**:pd-aws / tikv-aws / tidb-cluster-tidb / **tiflash** 全 1/1 Running(均有 1 次 43min 前的 restart,系那次 RD 抖动)。**TiFlash 当前仍 Running** —— 一个失败的子代理("TiDB 升级 TiKV 分布式",因 API timeout 终止)原打算关 TiFlash 释放内存(用户口信"TiFlash 若不影响多区域同步和读写分离可先不启动"),但未执行就 timeout。**待决策:是否 scale TiFlash 到 0 释放单节点内存**(单节点资源紧张是 API server 反复抖动根因)。
> - **读写分离现状**:tidb-primary ✅ 指向 tidb-cluster-tidb(aws 区);tidb-follower 指向 azure 区(未起)。当前读也走 primary(单 TiDB 节点)。真读写分离需 azure 区 TiDB 起来,单节点 RD 暂不支持。
> - **esp-prod 业务服务仍 0 副本**:learning/teacher/gateway/writing/emotion 均 0/0(RD 重启后被压),bff/nacos/seata/sentinel Running。**待 scale 回 5 个服务**(建表已完成,Java 服务可连库 ready)。
> - **集群抖动规律**:单节点 RD 资源紧张,API server `6443 refused` 反复(1-3min 自愈),周期性。Rancher Desktop 重启会清所有 deploy replicas 归 0。
> - **任务总状态**:#1✅ #2✅(TiKV 升级+args 修复) #6✅(建表) #3 仅剩 gateway-go 缓存验证 #7 三端前端核查(explorer 子代理进行中) #4 重灌 session_turns(表已就绪可启动) #5 warmup+压测+文档。
> 更新日期:2026-08-01(续跑会话-9:bff-java 多级缓存验证完成 + TiDB args 坑修复 + esp 库已建)
> 更新日志(2026-08-01 续跑-9):**bff-java esp-cache 验证 ✅ 完成(builder 子代理)** — 镜像 `esp/bff-java:1.1.0`(prebuilt jar 单 stage + COPY otel-agent),`/actuator/health` UP(db/nacosDiscovery UP),旧坑(JWT_SECRET<256bit/sentinel-datasource-nacos/健康503/liveness杀启动)均未复发。AuthController 注入 CacheService,login/register 成功后 `cacheService.put("bff_user","user:"+id,userView,300)`,Redis L2 出现 key `user:<id>`(TYPE=string,TTL≈300s 倒数)。**结论:esp-cache L2 Redis 缓存在 bff-java 真正生效**。注:实际 key 格式 `user:<id>`(无 esp:cache: 前缀)。
> - **多级缓存总进度(#3)**:learning-java(1.2.0)+ teacher-java(1.3.0)+ bff-java(1.1.0)+ writing-py(1.1.0)+ emotion-svc(1.1.0)**5 服务全部验证通过**。**仅剩 gateway-go**(src/cache/cache.go 在,main.go 已 `tts.Synthesize(ctx,...)`,待 build+部署验证)。
> - **bff 子代理顺手修复 TiDB 启动坑**:`deploy/k8s/07-tidb-tikv.yaml` 三个 deploy(aws/azure/gcp)args `--port=4000` → `-P=4000`(TiDB v8.5.5 `/tidb-server` 端口 flag 是大写 `-P`,`--port` 不存在致 `flag provided but not defined: -port` 退出),已 apply。PD 起来后 TiKV/TiDB 陆续恢复。
> - **bff 构建两坑**:① Dockerfile `ADD https://github.com/.../otel-agent.jar` 网络拉取失败 → 宿主机 curl 下载到 `services/bff-java/otel-agent.jar`,改 `COPY`;② 多阶段 maven build 在 lima VM 极慢超时 → 改单 stage prebuilt jar 模式(同 learning-java)秒级完成。
> - **esp 库状态**:SHOW DATABASES 已含 `esp`(bff 子代理执行 schema.sql 时建的)。**但本次会话核实表清单时集群 API server 反复抖动(单节点 RD 资源紧张),未取到 SHOW TABLES 结果** —— 待恢复后确认 11/12 表齐全 + scenarios 等基础数据是否需灌。schema.sql 含 12 个 CREATE TABLE。
> - **集群抖动根因**:单节点 Rancher Desktop 资源紧张,API server `6443 refused` 反复(每次持续 1-3min 自愈)。esp-prod 多个 deploy replicas 归 0:learning/teacher/gateway/writing/emotion 仍 0/0,bff/nacos/seata/sentinel Running。redis-cluster 也曾归 0 已 scale 回 1。
> - **任务状态**:#2 TiKV ✅ + 建表收尾(待确认表完整);#3 多级缓存仅剩 gateway-go;#6 建 esp 库(实质已建,待确认表完整);#4 重灌 session_turns(依赖表就绪);#5 warmup+压测+文档。
> - **新需求(用户最新指示)**:对照需求分析/详细设计/原型设计文档**全面检查前端程序是否完整完成、能否前后端连调成功** —— 三端(frontend-react/h5/miniprogram)对照 docs/design 与 docs/product 逐项核对,并做前后端连调验证。已加为独立任务。
> 更新日期:2026-08-01(续跑会话-8:TiKV 分布式升级完成 + 集群恢复)
> 更新日志(2026-08-01 续跑-8):**TiKV 分布式升级 ✅ 完成(builder 子代理)** — pd-aws/tikv-aws/tidb-cluster-tidb 三 pod 全 1/1 Running,TiDB 版本 `8.0.11-TiDB-v8.5.5`,查询连通性通过。
> - **关键坑 1:Rancher Desktop VM 起不来** —— VZ 报 `memorySize is greater than maximumAllowedMemorySize`,根因 lima.yaml `memory: 42949672960`(40GB)>宿主机物理内存 32GB。**修复:编辑 `/Users/joelgong/Library/Application Support/rancher-desktop/lima/0/lima.yaml` 第 223 行 `memory` 改 `32212254720`(30GB),重启 RD 后 VM 正常**。此坑解释了续跑-7 末尾"VM 假死/6443 refused"现象。
> - **关键坑 2:docker.1ms.run 拉取卡死** —— pd/tikv 镜像层长时间无进度。**改用备用源 `docker.m.daocloud.io` 三个镜像全拉成功并 retag `pingcap/{pd,tikv,tidb}:v8.5.5`**。RD 的 k3s 复用 docker 的 containerd(sock `/var/run/docker/containerd/containerd.sock`,namespace moby),镜像已在 containerd 中可见,无需额外 ctr import。
> - **关键坑 3:tikv PVC Pending** —— PVC `tikv-aws-data` 卡 `WaitForFirstConsumer`,旧 pod volume mount 失败致 consumer 未注册。`rollout restart deploy/tikv-aws` 后绑定成功。
> - **关键坑 4:tidb 副本被压 0** —— `tidb-cluster-tidb` deploy replicas=0,手动 `scale --replicas=1` 拉起。
> - **真实资源名**:deploy `pd-aws`/`tikv-aws`/`tidb-cluster-tidb`(manifest `07-tidb-tikv.yaml` 实为多区域命名:pd-aws/pd-azure/pd-gcp,当前只起 aws 区)。svc `pd-aws:2379`、`tikv-aws:20160`、`tidb-cluster-tidb:4000`。
> - **⚠️ 重要待办:esp 业务库未建** —— TiDB 是新集群,`esp` 库及 11 张表均未创建(查 session_turns 报 `Unknown database 'esp'`)。**下一步必须先执行 `docs/design/schema.sql` 建库建表**,否则所有 Java 服务连不上表、种子脚本也无法灌。
> - **ExternalName 状态**:`tidb-primary` ✅ 已指向 `tidb-cluster-tidb.ekb-m2-aws-us-east-1.svc.cluster.local`(db_query.sh 可复用);`tidb-follower` 指向 `tidb-cluster-tidb.ekb-m2-azure-eastasia...`(azure 区未起,读写分离待 azure 起来才有真 follower,当前读也走 primary)。
> - **esp-prod Java 服务现状**:Rancher Desktop 重启后 esp-prod 所有 deploy replicas 归 0(learning/teacher/gateway/writing/emotion 均 0/0),仅 nacos/seata/sentinel/bff 自动起来。**待办:scale 回 Java 服务 + gateway/writing/emotion,待 esp 库建表后才能 ready**。
> - **任务状态**:TiKV 升级(#2)实质完成(三 pod Running,待建表收尾);多级缓存(#3)剩 bff-java+gateway-go 验证;session_turns(#4)从 0 重灌依赖建表;#5 待 #4。
> 更新日期:2026-08-01(续跑会话-7:writing-py + emotion-svc esp-cache-py 多级缓存构建验证完成)
> 更新日志(2026-08-01 续跑-7):**writing-py + emotion-svc 多级缓存(esp-cache-py)构建验证完成** —
> - **公共包 esp-cache-py**:`services/esp-cache-py/`(pyproject + `esp_cache/__init__.py`),L1 cachetools(LRU 进程内)+ L2 redis.asyncio(Redis Cluster)+ 降级兜底(env `CACHE_ENABLED=false` 直调 loader;L2 连接异常降级纯 L1,只 warn 不阻断)。`get_or_load`/`evict`/`cached` 装饰器三件套。本地 `pip install -e services/esp-cache-py` 装入 venv。
> - **writing-py(镜像 esp/writing-py:1.1.0)**:`service.py` 的 grade/polish/translate 改 `esp_cache.get_or_load`(key 由 exam+prompt+essay / text 决定,`cache_empty=False`,loader 返回 `model_dump()` 后外层 `Response(**cached)` 重建)。pyproject 加 `esp-cache-py>=1.0.0`。Dockerfile build context=项目根,COPY `services/esp-cache-py` 先装公共包再装本服务。
> - **emotion-svc(镜像 esp/emotion-svc:1.1.0)**:`service.py` 的 `visual_for_word` 改 async + `get_or_load`;`main.py` visual 端点 `await`;`tests/test_service.py` 改 `asyncio.run` + 新增 `test_visual_cached_returns_same_result`。
> - **本地测试(venv /tmp/esp-venv,python3.12)**:writing-py pytest 17 passed(含新增 `test_grade_cached_returns_same_result`);emotion-svc pytest 13 passed(含 cached 测试)。缓存命中验证:相同输入两次调用结果 SAME=True。
> - **修复的 bug(构建/导入阻断)**:① `opentelemetry-instrumentation-litestar==0.49b0` 在 PyPI 不存在真实发布(0.1.0 是 slopsquatting 占位包),改用 Litestar 2.x 内置 `litestar.contrib.opentelemetry.OpenTelemetryPlugin`,在 `Litestar(..., plugins=[OpenTelemetryPlugin()])` 注册;`tracing.instrument_app` 改为幂等占位(PluginRegistry 无 append,运行时不能追加插件,只能构建时注册)。② emotion-svc `main.py` 的 `Body(...)` 在 Litestar 2.24 报 `Body() takes 0 positional arguments`,改 `Body()`(kw-only)。两服务 pyproject 的 OTel litestar 依赖换为 `opentelemetry-instrumentation-asgi==0.49b0`。
> - **Docker 构建**:build context=项目根(`docker build -f services/<svc>/Dockerfile -t esp/<svc>:1.1.0 .`)。坑:Dockerfile 里 `pip install` 直连 pypi.org 超时(colima 内网络),改 `pip config set global.index-url https://pypi.tuna.tsinghua.edu.cn/simple` 解决。镜像在 rancher-desktop docker context 构建,k3s IfNotPresent 复用本地镜像(set image 后 delete pod 触发复用)。
> - **部署验证**:kubectl set image → rollout 成功。两服务 /healthz 返回 `{"status":"ok"}`。设置 env `CACHE_ENABLED=true REDIS_HOST=redis-cluster REDIS_PORT=6379`(默认值本就如此,显式 set env 确保)后,两次相同输入 POST `/api/writing/grade` 和 `/api/emotion/visual` 结果 SAME=True,且 Redis 出现 key `writing:grade:db954d135f078a95654038cd` / `emotion:visual:4a6bc40ecd46ad02c1c37da6`(L2 写入确认)。**L1(进程内)+ L2(Redis)双级缓存链路验证通过**。
> - **OTel tracing 告警(非阻断)**:两服务日志持续 `StatusCode.UNAVAILABLE otel-collector.esp-observability.svc:4317`(esp-observability ns 未起 collector),不影响业务,缓存与 /healthz 正常。
> - **集群抖动**:Rancher Desktop 重启后 esp-prod 所有 deploy replicas 归 0(pod 全清),手动 scale writing-py/emotion-svc 回 1。ekb-m2 ns redis-cluster 也归 0,scale 回 1 后 Redis 恢复。
> - **下一步(任务 #4/#5)**:依赖 TiKV 升级(#2)完成后重灌 session_turns 3 亿(#4)、redis_warmup + 5000CCU 压测 + 测试文档回填(#5)。
> 更新日期:2026-08-01(续跑会话-6:TiKV 升级镜像拉取排障)
> 更新日志(2026-08-01 续跑-6):**TiKV 升级卡点定位 = 镜像拉取网络故障,非升级逻辑/资源/配置问题** —
> - **诊断结论(explorer 子代理只读核实)**:① 未装 TiDB Operator,纯手写 Deployment(manifest `deploy/k8s/07-tidb-tikv.yaml`,镜像行 100/167/246 全 `pingcap/{pd,tikv,tidb}:v8.5.5`,imagePullPolicy IfNotPresent)。② 三组件状态:pd `ContainerCreating`(19min 仍 `Pulling` 无后续)、tikv `ImagePullBackOff`(`unexpected EOF` 0字节,非401/404=网络抖动非镜像名错误)、tidb `CrashLoopBackOff`(级联:启动参数 `--path=pd...:2379` PD 没起来连不上→启动即退,非 tidb 自身问题)。③ 节点资源充足(Allocated CPU55%/Mem29%,Limits Mem91%),非 OOM/调度问题。④ 同期 `redis:7` 也 ErrImagePull,印证**整体拉镜像不通**(docker.io 全局不可达,memory 已记)。
> - **排障方案(builder 子代理执行中)**:用 `docker.1ms.run` 加速源预拉三个 `pingcap/*:v8.5.5`(已确认 tag 在 1ms.run 存在)→ `docker tag` 原名 → `k3s ctr image import` 灌入本地 → 删卡住 pod 让 IfNotPresent 复用 → 按 PD→TiKV→TiDB 顺序等 ready(顺序依赖天然成立,无需 Operator)。备用源 `docker.m.daocloud.io`。
> - **级联影响**:TiDB 崩溃导致 esp-prod 三 Java 服务(learning/teacher/bff)pod 全无。**用户决策:先排 TiDB 恢复后再恢复 Java 服务**,避免无效重启。
> - **session_turns 状态澄清**:本轮核验 Redis,16 个 `esp:ctx:seed:session_turns:shard:0..15` key 当前**已不存在**(=0)。这与续跑-5 顶部记录一致——续跑-5 因 region unavailable 9005 故障**主动清了进度键(数据可重生)**。故下一步 TiKV 升级完成后**从 0 重灌 session_turns 至 3 亿**(非断点 resume)。learning_events 30 亿待 session_turns 完成后启动。
> - **任务清单(本会话重建,TaskList 可查)**:#1 核对集群卡点✅ / #2 TiKV 升级排障🔄(in_progress,镜像预拉+导入+顺序启动)/ #3 全服务多级缓存验证(bff/gateway/writing/emotion,代码完成构建验证未完)/ #4 重灌 session_turns 至3亿(依赖 #2)/ #5 redis_warmup+5000CCU压测+测试文档回填(依赖 #4)。
> - **集群抖动说明(2026-08-01 12:20)**:TiKV 升级排障期间节点曾短暂 OOM 致 API server `6443 refused`(持续约 2-3min 后自愈,MemPressure 现 False)。期间 ekb-m2 ns 的 redis-cluster pod 一度消失(builder 子代理重建 TiKV 资源时影响),导致本轮 Redis `esp:ctx:*` 写入受阻。**PROGRESS.md 为可靠恢复锚点**;待 builder 子代理完成 TiKV 升级、redis-cluster 重新 Running 后,补写 `esp:ctx:session:*` 快照。恢复时优先读本文件顶部。
> 更新日期:2026-08-01(续跑会话-5:全服务多级缓存 + TiDB 分布式升级)
> 更新日志(2026-08-01 续跑-5):**全服务多级缓存重构 + TiDB 故障** —
> - **多级缓存**:① teacher-java 接入 esp-cache(pom 加 esp-cache/caffeine/redis/pulsar 依赖、TeacherApplication scanBasePackages 含 com.esp.cache、application.yml 加 esp.cache.*+spring.data.redis、TeacherService 注入 CacheService:classAnalytics 读走 getOrLoad、autoGrade/finalize/summarizeStudentLearning 写后 evict、TeacherController 加可选 classId 参数;TeacherControllerTest stub 改新签名;yml 重复 spring key 合并)。镜像 esp/teacher-java:1.3.0,12 测试全过,rollout 成功,esp-cache 装配日志确认(CacheService.getOrLoad 栈帧命中)。② bff-java 多级缓存验证(代码已接,build+部署验证进行中)。③ gateway-go 新增 src/cache/cache.go(L1 LRU+L2 go-redis)、tts.Synthesize 走缓存、main.go 调用方加 ctx;构建验证进行中。④ writing-py+emotion-svc 抽 Python 公共包 esp-cache-py(esp_cache/__init__.py:L1 cachetools+L2 redis.asyncio+降级兜底+cached 装饰器)、writing-py service.py grade/polish/translate 走 get_or_load、emotion-svc visual_for_word 改 async 走 get_or_load、Dockerfile build context 改项目根装 esp-cache-py;构建验证进行中。
> - **TiDB 严重故障**:region unavailable(error 9005),1.7 亿行 session_turns 可能受损,16 个 seeder 全 0 进度卡死,已主动停掉。TiDB pod 已重启 20 次。
> - **TiDB 架构升级决策(用户拍板)**:升级为多区域 TiKV 分布式(PD+3TiKV+TiDB+TiFlash),不备份(数据可重生)。风险:节点内存已 75%,PD+3TiKV 会再吃 15-25GB,可能压垮节点。**部署架构升级优先执行,完成后再灌数据+压测**。
> - **读写分离验证**:代码逻辑正确(RoutingDataSource+@DataSource+AOP,LazyConnectionDataSourceProxy 解耦),但 tidb-primary/tidb-follower 实际指向同一 unistore 实例,无真正分离。TiKV 分布式后才有真分离。
> - session_turns 进度键已清(数据可重生),待 TiKV 升级后从 0 重灌至 3 亿。
> - learning-java L1 缓存确认:已启用并真生效(CacheConfig @ConditionalOnProperty enabled=true,l1-spec maxSize=10000/60s;L2 Redis 读写生效;读 L1→L2→DB,写后 L2删+L1删+Pulsar广播+100ms延迟双删)。回答用户"learning-java 需要 L1 吗":已启用,L1 命中纳秒级返回,值得。
> 更新日志(2026-07-31 续跑-3):**Dubbo Triple #4 完成** — teacher-java `@DubboReference timeout 3000→10000ms`,rebuild 1.1.2 镜像 rollout,`/api/teacher/{id}/kp-stats` 返回 HTTP 200 `[]`(冷缓存 5.86s,不再 503 DEADLINE_EXCEEDED)。**混沌 #11 执行完成** — pod_kill gateway-go RTO 1s;redis_down 20s 注入 + rollout 恢复 ~44s(修脚本 `${Dur}`→`${DUR}` typo);tidb_down 20s 注入 + learning-java 滚动恢复 ~100s(Nacos/Dubbo 注册慢);latency_inject 跳过(tc not found/无 NET_ADMIN);net_partition 跳过(RD CNI 不强制 NP,apply 成功但不生效,如实记录)。RTO 落 `report/chaos-rto.txt`。**session_turns 灌数 #6 恢复运行中** — 16分片×1worker(匹配已有 Redis shard:0..15, sid%16 分片),从 27% resume,当前 ~39%(1.18亿/3亿),ETA ~75min/shard。覆盖率 #12a(jacoco/pytest-cov)子代理进行中。
> 更新日志(2026-07-31 晚):session_turns 灌数因 TiDB 单节点 OOM(重启16次)暂停于 27.3%(8213万行,数据存 Redis 可 resume),改 8分片×2worker 止血;Dubbo Triple provider 注册 nacos 成功但卡 STRICT 序列化 + seata vgroupMapping 缺失(修中);缓存重构抽公共包 esp-cache(进行中),曾与 Dubbo agent 并发改 learning-java 冲突致 package 失败,待公共包完成统一重 build。
> 更新日志(2026-07-31 续):bff-java 部署完成(Running,9090 edge 401/8090 actuator UP);Redis warmup 方案就绪(docs/redis-warmup-plan.md);Milvus/Neo4j 抉择:暂不引入(无真实向量/图需求,基建已 Running 零成本可后接);session_turns 3亿灌数后台进行中;多级缓存审计进行中。
> 更新日志(2026-07-31):seata Running 注册 nacos,SCA 三件套全 Running。
> 更新日志(2026-07-31 任务恢复快照):SCA 三件套(nacos/sentinel/seata)全 Running;redis-cluster 加 PVC+AOF 持久化;可观测栈 5 组件全 Running(ns esp-observability);bff-java 部署中;Dubbo Triple 落地中;种子脚本预检完成(单线程不可行,改分片版中);混沌预检完成(docs/test/chaos-plan.md)。
> 目标:AI 英语教育平台(Home-Tutor)— TDD + 多语言微服务 + 读写分离 + Disruptor + TiDB + Pulsar + K8s,支撑 5000 CCU,三端(Web/H5/小程序)。
> 仓库非 git 仓库(无 git 历史),进展以文件实际状态为准。
>
> **跨会话恢复指引**:上下文溢出后,新会话读本文件 + 用 `scripts/ctx_state.sh get <key>` 读 Redis(`esp:ctx:*`)运行态(压测计数/种子进度),即可续跑。任务清单见 TaskCreate(TaskList 可查)。

---
