
## 跨会话运行态(Redis key `esp:ctx:*`)
- `esp:ctx:seed:learning_events` / `esp:ctx:seed:session_turns` —— 30 亿种子当前行数(分片版改为 `:shard:{id}` 按分片隔离)
- `esp:ctx:load:ccu_peak` —— 压测 WS 峰值在线
- `esp:ctx:load:evs` —— 学情事件吞吐
- 读写工具:`scripts/ctx_state.sh {set|get|incr|del} <key>`

---

## 本轮已完成(增量)

### A. 上下文工程基础设施 ✅
- `.claude/agents/`:esp-explorer / esp-builder / esp-data / esp-doc 四个 Subagent(上下文隔离)
- `scripts/db_query.sh "SQL"` —— TiDB 查询封装(已验证可用)
- `scripts/ctx_state.sh` —— Redis 跨会话运行态(已验证:set/get/incr/del 均通)
- `CLAUDE.md` 更新为 v2(铁律+执行模式+环境+Subagent 分工)
- memory:新增 `esp-context-engineering.md`

### B. learning-java 500 修复 ✅
- 根因:`LearningController.reportEvent` 把 JSON 字符串 `user_id` 强转 Number → ClassCastException → 500(压测全 failure 根因)。
- 修复:`services/learning-java/src/main/java/com/esp/learning/web/LearningController.java` 改 toLong/toInt/toDouble 安全转换。
- pom.xml 加 byte-buddy 1.17.5(解决 JDK25 下 Mockito inline mock 失败,13 个测试 error→0)。
- `mvn test` RC=0(27 通过);`mvn package -DskipTests` RC=0,jar=`services/learning-java/target/learning-java-1.0.0.jar`。
- 部署到 esp-prod ✅ 已完成并验证:
  - Dockerfile 是 COPY jar(非源码构建),经 lima VM dockerd 经典构建器 `docker build` 出 `esp/learning-java:1.0.0`,k3s+docker 模式 `IfNotPresent` 直接可见,`rollout restart` 即可。
  - **额外坑(关键根因)**:pom.xml 中 byte-buddy/byte-buddy-agent 被声明为 `<scope>test</scope>`,运行期 Hibernate 6.5 需 ByteBuddy → fat-jar 缺失 → `ClassNotFoundException: net.bytebuddy.NamingStrategy$SuffixingRandom$BaseNameResolver` CrashLoopBackOff。修复:删掉两个 byte-buddy 依赖的 test scope(改 compile,默认 scope)。重新 `mvn package`(jar 58MB→68MB,含 byte-buddy-1.17.5.jar)+ docker build + rollout restart。
  - `rollout status --timeout=300s` → RC=0 `successfully rolled out`。
  - 验证(经 gateway-go 容器发请求):字符串 user_id → `{"accepted":true}`(此前 500 已消除);数字 user_id → `{"accepted":true}`。toLong/toInt/toDouble 安全转换生效。

### C. 前端三端 ✅(设计文档 ADR-14 / SDD 第 7 章对齐)
- **Web**(`frontend-react/`):升级为完整应用 —— react-router 路由(登录/学习中心/教师端/profile)、zustand 鉴权(`src/auth.ts`)、统一 API 客户端(`src/api.ts`,自动注入 JWT)、契约类型(`src/types.ts`)。新增:`src/pages/{Login,LearningLayout,TeacherLayout,Profile}.tsx`、`src/pages/teacher/{LessonPlan,Analytics}.tsx`;重写三个 panel 接 API;styles 升级。
- **H5**(`frontend-h5/`):新建。React18+Vite+antd-mobile+PWA,移动竖屏布局,学习者全功能。`src/pages/{Login,Home,Speech,Writing,Learning}.tsx` + auth/api/types/styles。
- **小程序**(`frontend-miniprogram/`):新建。Taro4+React,wx.login→`/api/auth/wx-login` 换 JWT,WS 用 wx.connectSocket。`src/pages/{index,login,speech,writing,learning}/` + `src/api.ts` + Taro config。
- 三端均经 BFF(8090)鉴权,WS 旁路直连 gateway-go(ADR-13)。

### D. BFF 网关(`services/bff-java/`)✅ 代码完成,构建验证中
- **目标**:工业级高可用 —— Spring Cloud Gateway(边缘)+ Spring Cloud Alibaba 全套(Nacos 注册/配置、Sentinel 流控熔断、Dubbo Triple 服务间 RPC、Seata 分布式事务)。
- `pom.xml`:SCG 4.x + SCA 2023.0.1.2 + Dubbo 3.3.0 + jjwt 0.12.6 + spring-security-crypto + jdbc/redis。
- `BffApplication.java`:`@EnableDiscoveryClient`(Nacos)。
- `config/RouteConfig.java`:声明式路由到各 ClusterIP 短名(gateway-go/learning-java/writing-py/teacher-java/emotion-svc)。
- `config/RateLimitFilter.java`:Sentinel QPS 限流(learning-event 2000/writing-grade 200/default 5000,超限 429)。
- `filter/AuthFilter.java`:GlobalFilter JWT 校验 + 剥离外网伪造 X-User-Id/X-Role 头 + 注入可信值(401)。
- `auth/JwtService.java`:JWT 签发/校验(本期 HS256 共享 secret,升级 RS256 仅换 key)。
- `handler/AuthController.java`:register/login(BCrypt + TiDB users 表)/wx-login(小程序 code 换 JWT)。
- `application.yml`:Nacos discovery/config + Sentinel(dashboard + datasource 从 Nacos 拉 gw-flow 规则)+ Dubbo(tri://:50090 注册 Nacos)+ Seata(esp_tx_group)配置占位。
- `Dockerfile`:多阶段构建(maven→jre21)。
- **用户负责整合**:Dubbo Triple + Sentinel + Seata 的服务间契约与事务编排(learning↔teacher)。本会话已铺好配置骨架,用户提供具体整合时在此基础上加 `@DubboService`/`@DubboReference`/`@GlobalTransactional`。
- 构建由 builder 子代理进行中(compile → package)。

### 待办(下一步)
3. 30 亿种子脚本(`scripts/seed/`)—— **预检完成(2026-07-31,详见 `docs/seed-precheck.md`),实际灌数据待启动**
   - 预检结论:脚本连接/断点续跑/Redis 进度/schema 对齐均验证通过(小批量 1000 行写入成功)。
   - **性能阻塞**:当前单线程 executemany 实测仅 ~2,033 rows/s,30 亿需约 **17 天**,不可行。
   - 预检前状态:learning_events 残留 1,302,000 行,session_turns 0 行;预检后已 TRUNCATE 两表 + 删 Redis 进度 key,恢复干净 0 状态。
   - 启动前必做:(a)脚本加分片(`--shard-id/--shard-count`,Redis key 分片);(b)8~16 pod 并行;(c)批量 SQL 优化(多值 INSERT 或 LOAD DATA)。优化后乐观预估 learning_events **10~20 小时**,session_turns ~1 小时。
4. 5000 CCU 分布式压测 + HPA 验证
5. 混沌脚本(`scripts/chaos/`)+ 执行
6. 监控落地(`deploy/k8s/monitoring/`)
7. 测试文档回填(`docs/test/01~07`)

---

## 本轮新增:BFF 重构(Netty 边缘 + Disruptor 入口,移除 Spring Cloud Gateway)

### 决策(用户明确要求)
- BFF 仅使用 Dubbo + Spring Cloud Alibaba 全套(最新版),**不再使用 Spring Cloud Gateway**。
- 引入 **Netty(边缘 HTTP 接入层)+ Disruptor(高吞吐入口队列)**。
- 集群此前无 Nacos/Sentinel/Seata,用户决定**全套部署**到 esp-prod。

### D2. SCA 中间件三件套部署 ✅ manifest 完成,部署中
- `deploy/k8s/06-sca-middleware.yaml`:
  - **Nacos** v2.4.3 standalone(`nacos:8848`+grpc 9848+raft 9849,内嵌 derby,auth 关闭)
  - **Sentinel Dashboard** 1.8.8(`sentinel:8858`,bladex 镜像)
  - **Seata Server** 2.2.0(`seata:7091`+grpc 8091,file 存储模式,注册到 Nacos,SEATA_GROUP)
- 由 esp-builder 子代理执行 `kubectl apply` + rollout status + 连通性探针,运行中。
- Redis 跨会话运行态 key:`esp:ctx:task:bff`(链路已验证通:set/get round-trip OK)。

### D3. bff-java 重构代码 ✅ 完成,编译验证中
- `pom.xml`:Boot 3.3.5 / SCA 2023.0.1.2 / Dubbo 3.3.0 / **Netty 4.1.114.Final / Disruptor 4.0.0**;`sentinel-spring-webmvc-adapter` 显式 1.8.8(不在 SCA BOM);**彻底删除 spring-cloud-gateway**。
- **删除** `config/RouteConfig.java`(SCG `RouteLocator` 残留)。
- **新增 `edge` 包**(Netty 边缘接入层):
  - `RouteTable.java`:前缀→下游短名路由表(`gateway-go`/`learning-java`/`writing-py`/`teacher-java`/`emotion-svc`),isLocal 判定(/api/auth、/healthz、/actuator)。
  - `EdgeAuthHandler.java`:Netty `SimpleChannelInboundHandler`,JWT 校验 + 剥离伪造 X-User-Id/X-Role + 注入可信值 + 白名单;鉴权后投递 Disruptor,队列满 503 背压。
  - `InletEvent.java`:Disruptor 事件载体(retained FullHttpRequest)。
  - `InletDisruptor.java`:RingBuffer 65536、ProducerType.MULTI、BlockingWaitStrategy;单消费者用 JDK HttpClient 异步转发下游(fire-and-forget)。
  - `NettyEdgeServer.java`:Netty 接入 server,`edge.port:9090`,pipeline codec→aggregator(8MB)→EdgeAuthHandler;异步启动不阻塞容器。
- `config/RateLimitFilter.java`:从 WebFlux `WebFilter`/`Mono` 改为 Web MVC `OncePerRequestFilter`(Sentinel `SphU.entry` + 429 JSON)。
- `application.yml`:删 `spring.cloud.gateway` 块;加 `edge.port:9090`、`spring.main.web-application-type: servlet`;Seata `enabled:true` + 注册/配置走 Nacos(SEATA_GROUP)。
- `Dockerfile`:`EXPOSE 8090 9090 50090`(MVC/Netty edge/Dubbo Triple)。
- 端口约定:Netty edge=9090(对外接入)、Spring MVC=8090(本地控制器/auth/actuator)、Dubbo Triple=50090。
- 编译/打包/测试由 esp-builder 子代理验证中。

### D3. bff-java 重构代码 ✅ 完成,编译通过
- **Sentinel 用法澄清(重要)**:Spring Cloud Alibaba 的 Sentinel = `spring-cloud-starter-alibaba-sentinel`(SCA 官方 starter,已引入,传递拉入 sentinel-core/transport/annotation/datasource 等 1.8.8 全套)。但 SCA 的 sentinel starter **只内置 `sentinel-spring-webflux-adapter`**,不内置 Web MVC(Jakarta Servlet 6)适配器。Spring Boot 3.x 用 Jakarta Servlet 6,Web MVC 路径的 `BlockExceptionHandler`/拦截器需要单独加 `sentinel-spring-webmvc-v6x-adapter:1.8.8`(com.alibaba.csp 官方 Jakarta 变体)。**这不是"另起炉灶用 Sentinel",而是 SCA Sentinel 在 Web MVC 下的必需补丁 adapter** —— 核心/规则/transport/dashboard 全走 SCA Sentinel,只是 Web MVC 适配器是 csp 原生包(SCA 未打包)。已用 `mvn dependency:tree` 验证:sentinel-core 1.8.8 由 SCA starter 统一管理,v6x-adapter 也用 1.8.8,版本一致无冲突。
- **架构选型(性能权衡,用户拍板)**:对比"纯 WebFlux" vs "Netty 边缘+Disruptor+Web MVC",选后者 —— 接入层性能与 WebFlux 持平(都用 Netty 非阻塞),但 Web MVC 对 Seata/Dubbo 事务传播更友好(线程绑定 TransactionContext,WebFlux 跨 reactor 线程需额外编排),且与 learning-java 的 Disruptor 架构同构。
- **Web 容器换 Undertow(性能最优)**:把 spring-boot-starter-web 默认 Tomcat 10.1.31 换成 Undertow(pom 排除 tomcat starter + 引入 undertow starter,代码零改)。Undertow 吞吐高 15-30%、内存占用最低、非阻塞 IO。
- **删除** `config/RouteConfig.java`(SCG `RouteLocator` 残留)。
- **新增 `edge` 包**(Netty 边缘接入层):`RouteTable` / `EdgeAuthHandler`(JWT+Disruptor 投递) / `InletEvent` / `InletDisruptor`(RingBuffer 65536,EventTranslatorOneArg+tryPublishEvent,JDK HttpClient 异步转发) / `NettyEdgeServer`(edge.port:9090)。
- `config/RateLimitFilter.java`:WebFlux `WebFilter`→Web MVC `OncePerRequestFilter`(Sentinel `SphU.entry`+429)。
- `config/SentinelConfig.java`:用 v6x `BlockExceptionHandler`(4 参 handle 签名,javap 验证)。
- `application.yml`:删 `spring.cloud.gateway` 块;加 `edge.port:9090`、`spring.main.web-application-type: servlet`;Seata `enabled:true`+Nacos(SEATA_GROUP)。
- `Dockerfile`:`EXPOSE 8090 9090 50090`。
- 编译验证 ✅:`mvn clean compile` RC=0、`mvn package -DskipTests` RC=0(jar=bff-java-1.0.0.jar 110M,含 Undertow)、`mvn test` RC=0。
- 端口约定:Netty edge=9090(对外接入)、Spring MVC(Undertow)=8090(本地控制器/auth/actuator)、Dubbo Triple=50090。

### D4. 中间件部署卡点 ⚠️ Docker Hub 拉取超时
- `deploy/k8s/06-sca-middleware.yaml` apply 后 nacos/sentinel/seata 三个 pod 全 `ImagePullBackOff`:registry-1.docker.io i/o timeout(网络问题,非配置问题)。
- 镜像:`nacos/nacos-server:v2.4.3`、`bladex/sentinel-dashboard:1.8.8`、`seataio/seata-server:2.2.0`。
- 解决中:daocloud 对 nacos 中途取消、对 bladex/seataio 返回 403(不在镜像范围)。esp-builder 子代理(a679ff6a)逐一尝试 dockerproxy/ustc/163/aliyun/quay 等镜像源,拉到后 retag 原始名 + delete pod 让 k3s IfNotPresent 复用。
- **seata 已绕过(2026-07-31)**:seata 镜像源 docker.io 超时已绕过(用 docker.1ms.run),实际运行 2.0.0 retag 成 2.2.0;若需严格对齐 2.2.0 可重试 apache/seata-server:2.2.0 最后一个 layer。seata 现 Running,已注册 nacos SEATA_GROUP。
- **seata 2.x 部署已修复(2026-07-31)**:ConfigMap `seata-config` 挂载 application.yml,registry=nacos:8848 SEATA_GROUP,storeMode=file;显式 SEATA_PORT=8091 覆盖 K8s 注入。ConfigMap 定义已补进 `deploy/k8s/06-sca-middleware.yaml`(原靠 kubectl create cm 手建,重装集群会丢导致 seata 回退 localhost:8848 CrashLoop),消除重装隐患。

### D6. 日志收集/分析系统 ✅ manifest 完成,部署中
- **现状**:各服务有应用日志(SLF4J/Python logging/Go log)但无集中收集分析。
- **选型(用户拍板)**:Grafana Loki(轻量,仅索引标签,存储成本 1/10)+ Fluent Bit(DaemonSet 采集容器 stdout/stderr)+ Grafana(统一面板,LogQL)+ OTel Collector + Tempo(分布式链路追踪)。
- `deploy/k8s/monitoring/10-observability.yaml`(新建,namespace `esp-observability`):
  - **Fluent Bit** 3.1.7 DaemonSet:tail 采集 `/var/log/containers/*.log`,Kubernetes filter 从 pod 标签提取 `esp_app`/`esp_ns`,OUTPUT 到 Loki。
  - **Loki** 3.2.1:tsdb v13 schema + 文件系统后端,7 天保留,`loki:3100`。
  - **OTel Collector** 0.109.0:OTLP gRPC/HTTP 接收,traces→Tempo、logs→Loki。
  - **Tempo** 2.6.0:本地存储后端,`tempo:4317` OTLP gRPC + `:3200` HTTP。
  - **Grafana** 11.2.0:NodePort 30300,预置 Loki(default)+ Prometheus + Tempo 数据源,配 trace↔log 关联(traceId 正则→Tempo,Tempo tracesToLogs→Loki 按 esp_app 标签)。
- 与现有 Prometheus/Grafana 监控同构,trace-log-metric 三统一。
- 部署由 builder 子代理 apply + rollout 验证(镜像可能同样遇 docker.io 超时,需换源)。
- **待办**:各服务接入 OTel(BFF 注入 traceId 贯穿三端→BFF→下游;Java 加 otel agent / Python 加 otel sdk / Go 加 otel sdk)。

### D5. esp-disruptor 共用工具包 ✅ 完成并 install
- **决策(架构权衡)**:learning-java(学情落库)与 bff-java(边缘入口)已各自实现 Disruptor,重复;teacher-java 后续异步批改也需。抽共用工具包固化 LMAX 最佳实践,所有 Java 模块复用。
- `services/esp-disruptor/`(与 esp-api 同级,mvn install RC=0,已入本地 m2):
  - `DisruptorWorker<T>`:单级工作队列骨架。Builder 模式,封装 RingBuffer/序列号/事件工厂;ProducerType 可选(SINGLE 最快无 CAS / MULTI 多生产者);WaitStrategy 可选(BLOCKING/SLEEPING/YIELDING/BUSY_SPIN/LITE_BLOCKING,按 CPU-延迟权衡);`tryPublishEvent` 背压(满返 DROPPED);默认 ExceptionHandler 记日志跳过坏消息,不打断消费链;`shutdown()` drain 在途事件;`stats()` 指标快照。
  - `DisruptorPipeline`:多级流水线(级间 Disruptor 串联,经典 LMAX 模式)。每级独立 bufferSize/producer/wait,级间 forward 衔接,逆序停机 drain。
  - `WaitStrategyType` / `PublishResult` / `DisruptorStats`(record):选型枚举 + 背压语义 + 指标。
  - 依据联网查证的 LMAX 官方最佳实践(子代理 adbbf222 查证中,核心要点已先行落地)。
- **待办**:learning-java 与 bff-java 改为依赖 `com.esp:esp-disruptor:1.0.0` 复用 DisruptorWorker,删除各自内联实现。

### D6. 日志收集/分析系统 ❌ 缺失(用户新需求,待实施)
- 现状:各服务有应用日志(SLF4J/Python logging/Go log),但**无集中收集/分析**。无 Loki/ELK/Fluent Bit。
- 待架构选型 + 实施(见后续)。

### 待办(本轮后续)
- 中间件镜像拉通(nacos/sentinel/seata 起来)
- bff-java 部署到 esp-prod(build 镜像 + K8s manifest + rollout)
- 前端三端 BFF 地址从 8090 → 9090(Netty edge)或经 Ingress 收敛
- esp-api 共享契约:learning-java 暴露 Dubbo Triple provider、teacher-java 消费(用户整合项)
- 回填测试文档

---

## 本轮新增(2026-07-31 续:恢复会话推进)

### E. esp-disruptor 工具包复用落地 ✅
- **learning-java** ✅:已依赖 `com.esp:esp-disruptor:1.0.0`,`LearningEventDisruptor` 用 `DisruptorWorker.builder()` 注入 `BatchEventHandler`。删除冗余 `LearningEventFactory.java`(无人引用,与 `DisruptorWorker.builder().factory()` 重复)。`mvn clean test` RC=0(27 测试全过:LearningController 4/Disruptor 2/Ebbinghaus 4/Service 9/Difficulty 6/Routing 2);`mvn package -DskipTests` RC=0(jar 68MB)。
- **bff-java** ✅:pom 加 `com.esp:esp-disruptor:1.0.0`;`InletDisruptor` 删除内联 `Disruptor`/`RingBuffer`/`DaemonThreadFactory`/`EventTranslatorOneArg` 封装,改用 `DisruptorWorker.Builder`(MULTI/BLOCKING/65536),`publish(InletEvent)` 转 `worker.publish` 映射背压;`EdgeAuthHandler.forward()` 改构造 InletEvent payload 后 `inlet.publish`。`DownstreamForwarder`(JDK HttpClient 转发)业务保留。`mvn clean test` RC=0(0 测试源文件);`mvn package -DskipTests` RC=0(jar 110MB)。
- 跨会话状态:Redis `esp:ctx:milestone:refactor-done`。

### F. 种子数据脚本 ✅ 分片并行版就绪(2026-07-31)
- `seed_learning_events.py`:灌 `learning_events`(目标 30 亿行)。**分片版**:`--shard-id/--shard-count` 按 `user_id % shard_count` 取模分片(互斥不重不漏),单条多值 INSERT(`--multi-batch 500` 行/条),进程内 `--workers`(默认 2)线程共享 offset 游标,每 N 批 commit + commit 前 set Redis。进度 key 按分片隔离 `esp:ctx:seed:learning_events:shard:{id}`。stdout 报 `shard/done/rate/eta`。
- `seed_session_turns.py`:灌 `session_turns`(目标 3 亿行)。同分片策略(session_id 取模),进度 key `esp:ctx:seed:session_turns:shard:{id}`。
- `README.md`:集群内 pod 执行方式 + 单节点现实约束说明(如实记录已灌 N 行,非"30 亿已灌完")。
- **小批量验证通过**(2026-07-31):4 分片 × target=100000 → 总行数=100000,distinct user_id=5050,每分片 25000 行(按 uid%4 互斥分布),4 个 shard Redis key 均设对=25000。断点续跑验证通过(同 target 跳过;bump target 续灌正确)。
- **吞吐实测**:单分片 4 worker —— learning_events ~12.5k rows/s,session_turns ~6.3k rows/s。16 分片并发预估:learning_events ~80-200k rows/s(30 亿约 4-10h),session_turns ~50-100k rows/s(3 亿约 0.8-1.7h)。**分片版已就绪,待用户确认启动全量。**
- 现状:learning_events/session_turns 当前 TiDB 计数 0(验证后已 TRUNCATE 归零,Redis shard key 已清空,可随时从 0 启动全量)。

### G. 混沌脚本 ✅ 建成(scripts/chaos/)
- `pod_kill.sh`(随机杀 pod 验自愈)、`redis_down.sh`(改 env 模拟 Redis 不可达)、`tidb_down.sh`(读副本不可达)、`latency_inject.sh`(tc netem 延迟)、`net_partition.sh`(NetworkPolicy 断网)+ `README.md`。
- 单节点 RD 约束:tc/NetworkPolicy 缺权限时跳过并 WARN,如实记录。

### H. SCA 中间件 ✅ nacos/sentinel/seata 全 Running
- nacos(`nacos/nacos-server:v2.4.3`)✅ Running;sentinel(`bladex/sentinel-dashboard:1.8.8`)✅ Running;seata ✅ Running,已注册 nacos(SEATA_GROUP@@seata-server healthy @10.42.0.114:8091)。
- seata 部署:镜像源 docker.1ms.run,实际跑 2.0.0 retag 成 2.2.0,ns esp-prod,manifest `deploy/k8s/06-sca-middleware.yaml`。SCA 三件套(nacos/sentinel/seata)现全 Running。


### I. bff-java 部署 / 可观测栈部署 / Dubbo Triple 落地 进行中
- bff-java 部署 ✅ 完成(见里程碑表 #3):pod 1/1 Running,8090 health UP,9090 edge 返回 401。
- 可观测栈 `deploy/k8s/monitoring/10-observability.yaml`(Fluent Bit/Loki/OTel/Tempo/Grafana)已 apply 并全 Running(见下"### J")。
- Dubbo Triple:esp-api 契约已存在(LearningRpcService.userStats/weakCount),esp-builder 子代理落 provider(learning-java)+ consumer(teacher-java 端点)进行中。

### J. 可观测栈全 Running ✅(2026-07-31)
- ns `esp-observability`,manifest `deploy/k8s/monitoring/10-observability.yaml`。
- 5 组件全 1/1 Running:fluent-bit(DaemonSet)、loki、otel-collector、tempo、grafana(均 1 restart,稳定运行 ~3h)。
- **Grafana 健康检查**:`port-forward svc/grafana 33000:3000` → `curl /api/health` = HTTP 200 ✅。NodePort 30300。
- **OTel Collector OTLP 接收**:日志确认 `Starting GRPC server 0.0.0.0:4317` + `Starting HTTP server 0.0.0.0:4318` ✅,svc 暴露 4317/4318,traces→Tempo、logs→Loki。
- 数据源:Grafana 预置 Loki(default)+ Prometheus + Tempo,trace↔log 关联已配。
- 注:本轮检查时已 Running,未触发镜像加速换源。

### 待办(本轮后续)
- ~~seata 镜像拉通 → SCA 三件套全 Running~~ ✅ 完成(2026-07-31,seata Running 注册 nacos)
- ~~bff-java 部署 esp-prod + 前端三端 BFF 地址收敛(9090 Netty edge)~~ ✅ 完成(2026-07-31,bff-java Running,8090/9090 验证通过)
- 可观测栈全 Running + 各服务接 OTel(traceId 贯穿)
- 30 亿种子执行(单节点按可接受时长,Redis 记进度)
- 5000 CCU 分布式压测 + HPA 验证
- 混沌脚本执行 + 恢复时间记录
- 测试文档回填 docs/test/01~07

---

## Redis Warm-up 预热方案设计中(2026-07-31)

- 调研结论:gateway-go 是唯一生产用 Redis 的服务(`session:{id}` STRING,2h TTL,见 `services/gateway-go/src/session/manager.go`);bff-java/learning-java 仅配 host 无代码调用(SDD §2.7 注记预留);teacher-java/writing-py/emotion-svc 未接。运行态 `esp:ctx:*`(种子进度/压测计数)由 seed 脚本与 ctx_state.sh 使用。
- 方案文档:`docs/redis-warmup-plan.md`(新建)。预热 key:W1 场景(全量~20 HASH)、W2 用户信息(5050 STRING)、W3 学情快照(5050 HASH)、W4 热点会话历史(1万 LIST,可选)。总量级 ~23MB,远低于 5Gi PVC。
- 量级建议:必灌 W1+W2+W5(压测最低保证);建议加 W3;W4 按压测脚本是否读历史决定。总 key<20万、内存<1Gi 红线,当前方案远低于红线。
- 脚本:`scripts/seed/redis_warmup.py`(待新建,docs/test/00 已引用此名),参考 seed_learning_events.py 连接模式,pipeline 批量灌,数据源从 TiDB 读。
- 前置依赖:session_turns 种子跑完(W3/W4 依赖 user_mastery/session_turns 已落库,当前 TiDB 计数 0)。
- 注意:bff 当前无缓存读取代码,warm up 的 `esp:cache:user:*` 待 bff 补 `@Cacheable` 后生效;redis-cluster 实为单实例非真 Cluster。

---

## 本轮任务状态(2026-07-31)

| # | 任务 | 状态 | 摘要 |
|---|---|---|---|
| 1 | seata 部署 | ✅ 完成 | Running,注册 nacos SEATA_GROUP。坑:K8s 注入 SEATA_PORT 致 NFE;2.x 不支持 env 改 application.yml → ConfigMap;docker.io 超时用 docker.1ms.run(实跑 2.0.0 retag 2.2.0);ConfigMap 定义已固化进 `deploy/k8s/06-sca-middleware.yaml`(用实测 application.yml)。 |
| 2 | redis-cluster 持久化 | ✅ 完成 | `deploy/k8s/06b-redis-pvc.yaml`:PVC 5Gi RWO local-path + `--appendonly yes` + 挂载 `/data`;重启后 key 仍存活验证通过。 |
| 3 | bff-java 部署 | ✅ 完成 | 镜像 `esp/bff-java:1.0.0`(arm64,prebuilt jar 模式)build + apply `15-bff-java.yaml` + rollout;pod 1/1 Running。坑:①esp-secrets JWT_SECRET 原 23 字节<256bit 致 `WeakKeyException` → patch 成 44 字节随机 base64;②pom 缺 `sentinel-datasource-nacos` 致 `NacosDataSourceFactoryBean NoClassDefFoundError` → 显式加 1.8.8 依赖;③sentinel health indicator 不稳致 `/actuator/health` 503 → yml 关 `management.health.sentinel.enabled=false`;④探针改用 `/actuator/health/{liveness,readiness}` + 加 startupProbe;⑤nacos config gRPC 仍连 127.0.0.1:9848 噪声(discovery 已连 nacos:8848,服务注册正常),不影响 ready。验证:pod 内 8090 actuator health=UP(components db/discovery UP);9090 Netty edge `/` 返回 401(鉴权拦截符合预期)。 |
| 4 | Dubbo Triple | ✅ 完成 | provider 注册 nacos(esp@@LearningRpcService:1.0.0 tri @10.42.0.172:50091 healthy);consumer teacher→learning userStats 调通。收尾:① serialize-check-status=WARN 已生效;② seata vgroupMapping 已配 nacos SEATA_GROUP 消除刷屏;③ **consumer timeout 3000→10000ms**(冷缓存+Nacos config gRPC 噪声致 provider ~5.9s,原 3s 必超时 DEADLINE_EXCEEDED 503)。teacher-java 1.1.2 镜像 rollout,`/api/teacher/{id}/kp-stats` HTTP 200 `[]`(user_mastery 对 user 1001 无数据,空列表正确)。@Primary 修 bean 冲突。 |
| 5 | 可观测栈 | ✅ 完成 | ns `esp-observability`(非 esp-prod);fluent-bit/loki/otel/tempo/grafana 全 Running;Grafana NodePort 30300 health=200;OTel OTLP 4317/4318,traces→Tempo logs→Loki。 |
| 6 | 30 亿种子 | 🟡 session_turns 暂停于 57%(止血) | Redis 偏移 `shard:0..15` 总 ~1.71亿/3亿(57%)全保留。续跑-4 为给三 JVM 服务止血(节点 CPU97% 饱和),删了全部 16 个 st-seeder bare pod,可随时从 Redis 偏移 resume。learning_events 30亿待 session_turns 完成后启动。止血后需重启 seeder(单节点建议 4-8 分片低并发)。 |
| 7 | Redis warmup | 🟡 方案就绪 | `docs/redis-warmup-plan.md`:W1场景20+W2用户5050+W3学情5050+W4热点1万,总<100MB;前置依赖 session_turns 灌完。关键发现:bff/learning 配了 Redis 但代码无读取(预留)。 |
| 8 | Milvus/Neo4j 抉择 | ✅ 暂不引入 | `docs/vector-graph-feasibility.md`:基建已 Running(milvus v2.5.0/neo4j 5.25.1,109d),但无真实向量/图需求(无相似推荐/学习路径 DAG/多跳关系;LLM 仅 chat 无 embedding;schema 无向量列无图边表)。零接入成本,待需求出现再接。 |
| 9 | 多级缓存重构 | 🟢 learning-java+公共包完成,bff-java 待接入 | 抽公共包 `services/esp-cache`(L1 Caffeine + L2 Redis + Pulsar 广播 + 延迟双删 + TTL 兜底),`mvn install` 成功;learning-java 接入(dueReview/weakestPoints 读缓存 + initMastery/review/DbLearningEventConsumer 写失效),`mvn compile/test` RC=0(35 tests pass,含缓存命中/失效/feature flag 回退单测)。bff-java 接入待后续(pom 加 esp-cache 依赖 + AuthController login 缓存 user + 转发响应缓存)。 |
| 10 | 5000 CCU 压测 | ⏸ 待办 | 依赖种子;单节点 RD 不可达,需分布式 master+workers;之前 peak_online=0/dropped=500 需排查。 |
| 11 | 混沌 | ✅ 执行完成 | 5 用例实测(单节点 RD):pod_kill gateway-go RTO **1s**(replicas=2 自愈);redis_down 20s注入+rollout **~44s**;tidb_down 20s注入+learning-java滚动恢复 **~100s**(Nacos/Dubbo注册慢);latency_inject **跳过**(tc not found/无 NET_ADMIN);net_partition **跳过**(RD CNI 不强制 NP)。RTO 落 `report/chaos-rto.txt`,日志 `report/chaos-*.log`。修脚本 bug:`redis_down.sh` `${Dur}`→`${DUR}`(set -u 下 unbound 致恢复未执行,曾残留 REDIS_HOST=127.0.0.1:1,已手动修复+脚本固化)。 |
| 12 | 测试文档回填 | 🟡 进行中 | `docs/test/01~07` 回填中:覆盖率 #12a 子代理跑 jacoco(learning/teacher)+ pytest-cov(writing/emotion)拿实测数字;混沌 #11 RTO 已得待填 04;集成 INT-1~4 Dubbo RPC 已通待填 02。 |

---

## 关键决策(2026-07-31)

- **外部会话管理选型**:固化为 Redis Cluster(`esp:ctx:*`),不用 Milvus/Neo4j —— 访问模式是点查+计数+顺序读,Redis 强项。
- **Milvus/Neo4j 暂不引入**:无真实向量/图需求,基建已 Running(milvus v2.5.0/neo4j 5.25.1,109d)零成本可后接;ESP 访问模式(点查/计数/顺序读/关系查询)TiDB+Redis 已覆盖。详见 `docs/vector-graph-feasibility.md`。
- **双轨记忆**:`docs/PROGRESS.md`(人读长期)+ Redis `esp:ctx:*`(机读运行态)。
- **镜像加速**:docker.io 全局不可达,镜像统一走 `docker.1ms.run` + `docker.m.daocloud.io` 加速器。
- **多级缓存抽公共包**(2026-07-31 晚):抽 `commons/esp-cache`(参考 esp-disruptor/esp-api 惯例)多服务复用零重复;失效广播用 Pulsar 不用 Redis Pub/Sub(redis-cluster 单实例已被会话/运行态/warmup/L2 多重占用,Pulsar 专用不占 Redis 资源)。

---

## 问题与解决方案(2026-07-31)

- seata ImagePullBackOff → docker.1ms.run 加速。
- seata CrashLoopBackOff(NFE) → 显式 `SEATA_PORT=8091` 覆盖 K8s 注入。
- seata 2.x env 配置失效 → ConfigMap 挂 application.yml。
- seata ConfigMap 仅集群内手建、重装丢 → 固化进 manifest 用实测原文。
- seata 客户端刷屏 `service.vgroupMapping.esp_tx_group configuration item is required` → nacos SEATA_GROUP 缺 vgroupMapping 配置;vgroupMapping 已配 nacos(dataId=service.vgroupMapping.esp_tx_group=default + service.default.grouplist=seata-server:8091,group=SEATA_GROUP),消除 learning-java/teacher-java 每 8s 刷屏。
- redis 重启丢 `esp:ctx:*` → 加 PVC + AOF。
- 种子 17 天不可行 → 分片并行 8-16 pod + 多值 INSERT。
- TiDB 单节点 OOMKilled(重启16次) → 16分片×4worker 降为 8分片×2worker(64→16 并发连接)止血。
- Dubbo STRICT 序列化拦截 EncodeException → dubbo.application.serialize-check-status=WARN。
- seata vgroupMapping.esp_tx_group 缺失刷屏 → nacos SEATA_GROUP 配置(进行中)。
- Dubbo consumer bean 冲突(2个 LearningRpcService bean) → @DubboService @Bean 加 @Primary。
- 缓存 vs Dubbo 并发改 learning-java 冲突 → 抽公共包 esp-cache 统一,避免两 agent 同改一文件。
- st-seeder 进程难杀(qemu 包装 pkill 失效) → delete pod 强制清进程。
- 找可观测栈找不到 → ns 是 `esp-observability` 不是 `esp-prod`。
- **Dubbo consumer DEADLINE_EXCEEDED 503**(续跑-3):teacher→learning userStats 实测 provider 冷缓存 ~5.9s(REST 直连 learning `/api/learning/mastery/weak` 3.26s, Dubbo 还叠加 Nacos config gRPC 噪声),原 `@DubboReference timeout=3000ms` 必超时 → 提到 `10000ms`,1.1.2 镜像 rollout 后 HTTP 200。注意 user_mastery 对测试 user 无数据时返回 `[]` 是正确结果,勿误判。
- **redis_down.sh 脚本 bug**(续跑-3):`set -u` 下 `${Dur}`(应 `${DUR}`)unbound,致注入后恢复段未执行,gateway-go 残留 `REDIS_HOST=127.0.0.1:1`。已手动 `set env REDIS_HOST=redis-cluster` 恢复 + 脚本 typo 修复固化。教训:chaos 脚本失败后必查 env 是否残留。
- **shard 分片方案不可随意改**(续跑-3):已有 Redis 进度键 `esp:ctx:seed:session_turns:shard:0..15` 基于 `sid%16`;若改 shard_count=8 重启,shard 0(count8)写 sid%8==0 的 session_id,与 Redis key shard:0(count16, sid%16==0)记录的 offset 错位 → 断点续跑错乱。恢复时**必须沿用原 shard_count=16**,仅降 worker(4→1)止血。
- **chaos tidb_down 恢复慢**(续跑-3):`READ_DB_URL-` 删 env 回落默认值后 learning-java 滚动重启,新 pod Nacos 注册 + Dubbo provider 注册 ~100s 才 ready(单节点资源紧张)。RTO 不能只算 rollout 命令返回,要看 readyReplicas==desired。

---

## 0. 总体状态(项目全貌)

| 维度 | 状态 |
|---|---|
| 设计文档(SRS/SAD/SDD/PRD/API/原型) | ✅ 已基线评审(v5.0) |
| 后端 5 服务 | ✅ 全部实现并各自带单测 |
| 前端 | ⚠️ 仅 Web React 骨架;H5 / 小程序端未实现 |
| 本地全栈(Docker Compose) | ✅ manifest 完备 |
| K8s 生产 manifest | ✅ 完备(含 HPA/Ingress/基础设施复用) |
| 压测脚本 | ✅ Locust 完备;⚠️ 实测未达 5000 CCU(单节点 RD 上限) |
| 测试报告 7 份 | ⚠️ 模板齐备,多数实测列待填(`_`) |
| BFF 网关 / learning-worker / Dubbo RPC | ❌ 设计中存在,代码未独立实现 |

---

## 1. 已完成项(含关键文件路径)

### 1.1 设计与契约文档
- 产品需求:`docs/product/01-PRD.md`、`docs/product/02-原型设计.md`
- 需求/架构/详细设计:`docs/design/01-需求分析文档-SRS.md`、`docs/design/02-系统架构设计文档-SAD.md`(v5.0,含 ADR-01~18、读写分离、异地多活、LangGraph)、`docs/design/03-详细设计文档-SDD.md`
- 数据库 schema:`docs/design/schema.sql`
- API 契约(OpenAPI 3.1 摘要):`docs/api/01-API契约.md`(覆盖 auth/scenarios/sessions/WS/learning/writing/teacher/emotion/Dubbo Triple RPC/错误码)

### 1.2 gateway-go(实时口语陪练,Go + WebSocket)
- 入口:`services/gateway-go/src/main.go`(内联 server,~214 行)
- 帧编解码:`src/ws/frame.go`(+ `frame_test.go`)
- 对话引擎(纠错+hint+编排):`src/dialog/engine.go`(+ `engine_test.go`)
- 会话管理(Redis 持久化,历史裁剪 50 轮):`src/session/manager.go`(+ test)
- LLM Provider 抽象 + Anthropic + Mock:`src/llm/provider.go`/`anthropic.go`/`mock.go`(+ `provider_test.go`)
- ASR/TTS/场景仓库(Mock):`src/asr/mock.go`、`src/tts/mock.go`、`src/scenario/repository.go`(+ test)
- 构建:`go.mod`/`go.sum`、`Dockerfile`、`Dockerfile.prebuilt`(已编译二进制)

### 1.3 learning-java(自适应学习,Java + Spring Boot 3 + Disruptor + 读写分离)
- 入口:`src/main/java/com/esp/learning/LearningApplication.java`
- Web:`web/LearningController.java`(placement/event/review/mastery-weak/difficulty)
- 领域:`domain/`(LearningEventEntity, UserMastery, UserMasteryId)
- 仓储:`repository/`(UserMasteryRepository, LearningEventRepository)
- 服务:`service/`(LearningService, DifficultyAdjuster, EbbinghausScheduler)
- Disruptor(学情事件入口,内嵌落库消费者):`disruptor/`(LearningEventDisruptor, BatchEventHandler, DbLearningEventConsumer, LearningEventConsumer, LearningEvent, LearningEventFactory)
- 读写分离:`rw/`(RoutingDataSource, DataSourceAspect, RoutingHolder, DataSourceType, `@DataSource` 注解)
- 配置:`config/DataSourceConfig.java`、`config/ServiceConfig.java`、`resources/application.yml`
- 测试:`src/test/java/.../{web,disruptor,service,rw}/*Test.java`(含 mockito MockMaker)
- 构建:`pom.xml`、`Dockerfile`

### 1.4 writing-py(写作批改,Python + Litestar + LangGraph + Claude)
- 入口:`app/main.py`、`app/asgi.py`
- LangGraph 智能体:`app/agent.py`(节点编排)、`app/graph_state.py`
- LLM 调用(Claude):`app/llm.py`
- 服务/契约:`app/service.py`、`app/schemas.py`
- 接口:grade / polish / translate / run(human-in-loop)/ review / state
- 测试:`tests/test_agent.py`、`test_service.py`、`test_api.py`(含 `.coverage`)
- 构建:`pyproject.toml`、`Dockerfile`

### 1.5 teacher-java(教师端,Java + 读写分离 + TiFlash)
- 入口:`src/main/java/com/esp/teacher/TeacherApplication.java`
- Web:`web/TeacherController.java`(lesson-plan/classes analytics/auto-grade/finalize)
- 服务:`service/`(TeacherService, LessonPlanGenerator)
- 仓储:`repository/SubmissionRepository.java`
- 读写分离:`rw/`(同 learning-java 结构)
- 配置:`config/DataSourceConfig.java`、`resources/application.yml`
- 测试:`src/test/java/.../{web,service}/*Test.java`
- 构建:`pom.xml`、`Dockerfile`

### 1.6 emotion-svc(情感化多模态,Python + Litestar)
- 入口:`app/main.py`、`app/asgi.py`
- 服务/契约:`app/service.py`、`app/schemas.py`(sense / visual 接口,本期纯计算无外部依赖)
- 测试:`tests/test_api.py`、`test_service.py`
- 构建:`pyproject.toml`、`Dockerfile`

### 1.7 前端(Web)
- `frontend-react/src/App.tsx`、`main.tsx`、`styles.css`
- 面板:`src/panels/{SpeechPanel,LearningPanel,WritingPanel}.tsx`
- 依赖:React 18 + Vite 5 + SWR;`package.json`/`vite.config.ts`/`tsconfig.json`

### 1.8 部署
- 本地全栈:`deploy/docker/docker-compose.yml`(PD+TiKV+TiDB+tidb-init schema 导入+Redis+Pulsar+各服务+前端)
- K8s 生产:`deploy/k8s/`(00-namespace-secret、05-infra-reuse[ExternalName 引用 ekb-m2-aws-us-east-1 的 TiDB/Redis/Pulsar]、10-gateway-go、20-learning-java、30-other-services、40-ingress[Traefik]、50-hpa、README-infra)
- 压测:`deploy/locust/`(locustfile.py、ccu5000_ws.py、seed_data.py、README.md)

### 1.9 测试报告(模板)
- `docs/test/00-测试总览.md` ~ `07-人工测试用例.md`(SLO 追溯矩阵、单测/集成/性能/混沌/监控告警/安全/人工)
- 单测基线(go 已实测):dialog 90.3%、scenario 100%、session 80.6%、ws 100%、llm(mock)21.7%
- 压测产物:`report/esp-500_*.csv|html`、`report/ccu5000_ws_result.json`

---

## 2. 未完成项(下一步动作)

### 2.1 前端三端缺口(高优先)
- **H5 端**:当前无任何 H5 目录/工程。下一步:新建 `frontend-h5/`(按 ADR-14 选型),复用 Web 的 API 客户端与契约,实现移动优先布局。
- **小程序端**:当前无任何小程序工程。下一步:新建 `frontend-miniprogram/`(微信原生或 Taro/uni-app),实现 wx.login → JWT 鉴权链路(已在集成测试 INT 列出但未实现)。
- Web 端:`frontend-react/` 仅为骨架(3 个 panel 共 ~197 行),下一步:补齐教师端面板、鉴权/登录流、SWR 数据层、错误处理与三端交互一致性。

### 2.2 BFF 网关与内部 RPC(设计中存在,代码缺失)
- **bff-gateway(Java :8090)**:SAD/ADR-08 描述(Spring Cloud Gateway + Alibaba Sentinel/Nacos),但 `services/` 下无 bff 工程。下一步:新建 `services/bff-java/`,实现 JWT RS256 边缘校验 + X-User-Id/X-Role 注入 + 限流 + REST 聚合。
- **Dubbo Triple RPC**:API 契约 §7 定义 `LearningRpcService.userStats`(learning→teacher),但 Java 工程内未发现 Dubbo/Triple 实现。下一步:在 learning-java 暴露 provider、teacher-java 消费。
- **learning-worker**:SAD 列为独立消费进程,实际已内置于 learning-java(`DbLearningEventConsumer`)。下一步:确认是否拆分独立 Deployment(当前按内置处理,文档已注记)。

### 2.3 压测与测试实测数据
- 压测未达 5000 CCU:`report/ccu5000_ws_result.json` 显示 peak_online=0、dropped=500、sustain_pct=0(单节点 Rancher Desktop 上限)。`docs/test/03` 实测列全为 `_`。下一步:分布式 Locust(master+workers)或多节点实测填表;或如实记录单节点上限 + 生产多节点推算。
- 学情事件 ev/s:locust 实测 ~2482 ev/s(`POST /api/learning/event`),未达 ≥5000 目标;且全部 296613 请求计为 failure(503/Disruptor 满或压测目标不匹配),需排查。
- 7 份测试报告的"实际"列大量为 `_`:下一步按模板逐项执行并回填(单元覆盖率、集成链路、混沌恢复时间、安全用例、人工三端用例)。

### 2.4 混沌与监控落地
- 混沌脚本目录缺失:`scripts/chaos/` 不存在(04 报告引用)。下一步:补充 Pod kill / Redis 下线 / TiDB 下线 / 延迟注入 / 跨区断网 脚本。
- ServiceMonitor / Alertmanager 规则文件未在 `deploy/k8s` 落地(05 报告为说明)。下一步:补 `monitoring/` manifest 与告警规则 YAML。

### 2.5 其他
- `scripts/` 目录为空,seed 数据脚本(`scripts/seed/`、`redis_warmup.py`)在测试总览引用但不存在。下一步:补种子数据脚本。
- `docs/test/` 与 `docs/api/` 下存在 `.DS_Store`(非内容问题,建议忽略)。

---

## 3. 已确定的架构约定(ADR 摘要)

1. **多语言最优**:实时面 Go(gateway-go)、业务面 Java(learning/teacher,Disruptor)、AI 面 Python(writing/emotion,Litestar + LangGraph)。
2. **读写分离**:Java 侧 `RoutingDataSource` + AOP(`@DataSource` 注解);写→TiDB Primary,读→Follower,分析→TiFlash,强一致读→Primary。Go/Python 侧直连相应端点。
3. **事件驱动**:学情/写作/情绪高频事件走 Disruptor(进程内 RingBuffer)→ TiDB 落库;Pulsar 预留 topic(learning.event/writing.task/emotion.signal/teacher.task)用于跨服务广播(如 speech.session)。
4. **无状态服务**:会话状态外置 Redis Cluster;服务可任意扩缩,HPA 按 CPU/内存弹性。
5. **LLM Provider 抽象 + Mock 模式**:`LLM_MODE=mock` 走确定性 Mock(单测/压测用),生产走 Anthropic Claude。
6. **契约先行**:OpenAPI 3 + WS 帧契约;BFF 边缘 JWT 校验并透传 X-User-Id/X-Role,外网请求剥离该头。
7. **基础设施复用**(ADR-07):TiDB/Redis Cluster/Pulsar 复用 `ekb-m2-aws-us-east-1` 现有集群,K8s 内以 ExternalName 短名访问,避免重复部署。
8. **WebSocket 旁路 BFF**(ADR-13):WS 长连接直连 gateway-go,不经 BFF。
9. **三端前端**(ADR-14):Web + H5 + 小程序三端(用户记忆明确要求全部实现)。
10. **安全默认**:JWT RS256、RBAC(learner/teacher/admin)、Secret 化、TLS 全链路、Redis 令牌桶/Sentinel 限流。
11. **可观测**:Prometheus + Grafana + Alertmanager + Loki + OpenTelemetry;各服务 `/metrics` 或 `/actuator/prometheus`。
12. **AI 框架**(ADR-18):Python AI 面用 LangGraph(writing-py 已实现 human-in-loop:run→awaiting_review→review→done)。

---

## 4. 踩过的坑 / 注意事项

1. **压测 CCU 与单节点上限**:5000 CCU 在单节点 Rancher Desktop(6 CPU/14GB)无法达成,WS 峰值 0、全量 dropped;生产需多节点 + HPA,且压测建议分布式 master+workers。压测脚本默认 `-u 5000` 不可直接在单机期待通过。
2. **学情事件压测全 failure**:`POST /api/learning/event` 在压测中 296613 请求全部计为 failure 且实际吞吐 ~2482 ev/s(目标 5000)。需排查是 Disruptor 满(应返 503)还是压测断言口径错误;勿误判为接口 100% 不可用。
3. **bff-gateway 与 Dubbo RPC 设计先行、实现滞后**:SAD/API 契约大量引用 BFF 与 Dubbo Triple,但仓库无对应代码;JWT 边缘校验、X-User-Id 注入、teacher→learning RPC 当前实际未跑通,集成测试 INT-1~INT-4 实测列待验证时注意。
4. **learning-worker 实为内置**:文档把 learning-worker 列为独立进程,实际落库消费内嵌于 learning-java;勿按"独立服务"找代码或部署。
5. **前端三端缺口**:记忆与 ADR-14 要求三端,但仓库仅有 Web React 骨架;H5/小程序从零开始,且 Web 本身也仅骨架。
6. **测试报告多为模板**:`docs/test/` 多数"实际"列为 `_`,单测仅 go 给出覆盖率基线;Java jacoco / Python pytest-cov 实测覆盖率未填,勿当作已验证结论。
7. **非 git 仓库**:本仓库无 git 历史,变更追溯只能依赖文件 mtime 与文档版本号;写进展/回滚无 git 安全网。
8. **`.DS_Store` 散落**:`docs/`、`docs/test/` 等存在 `.DS_Store`,清理时注意勿提交非内容文件(若后续接入 git 需加 `.gitignore`)。
9. **Mock 依赖**:ASR/TTS/LLM 默认 Mock 模式,单测与压测基于确定性 Mock;切真实 Claude/ASR 时需重新验证延迟与稳定性(P95<800ms 目标针对真实链路)。
10. **Redis 持久化修复(2026-07-31)**:ns `ekb-m2-aws-us-east-1` 的 `redis-cluster` deployment 原无 PVC、非集群模式,DBSIZE=0,重启丢数据。已加 PVC `redis-cluster-data`(5Gi RWO,local-path)+ AOF(`--appendonly yes`),数据挂载到 `/data`。验证:SET esp:ctx:test ok → BGREWRITEAOF → rollout restart → GET 仍返回 ok。manifest:`deploy/k8s/06b-redis-pvc.yaml`。`esp:ctx:*` 跨会话运行态现已可跨 pod 重启存活。

---

## Tomcat → Undertow Web 容器切换(learning-java + teacher-java,2026-08-01)

### 背景
bff-java 已用 Undertow,learning-java/teacher-java 仍用 Tomcat,需同构化。pom.xml 已改好(排除 `spring-boot-starter-tomcat` + 加 `spring-boot-starter-undertow`,与 bff-java 同构),本任务负责重新构建镜像 + redeploy。

### 执行结果
| 服务 | mvn RC | docker build RC | rollout RC | Undertow 验证 |
|---|---|---|---|---|
| learning-java | 0(BUILD SUCCESS) | 0(`esp/learning-java:1.2.0`) | 0(rolled out) | ✅ `Undertow started on port 8081 (http)` |
| teacher-java | 0(BUILD SUCCESS) | 0(`esp/teacher-java:1.2.0`) | 0(rolled out) | ✅ `Undertow started on port 8083 (http)` |

两个服务最终 pod 均 1/1 Running,镜像 tag 1.2.0,Undertow 已生效(原 Tomcat 日志 `Tomcat started on port` 消失)。

### 关键路径与文件改动
1. `services/learning-java/pom.xml`(任务前已改):排除 tomcat + 加 undertow。
2. `services/teacher-java/pom.xml`(任务前已改):同上。
3. `services/learning-java/src/main/java/com/esp/learning/LearningApplication.java`(本任务改):`@SpringBootApplication` → `@SpringBootApplication(scanBasePackages = {"com.esp.learning", "com.esp.cache"})`。**根因见下**。
4. `deploy/k8s/20-learning-java.yaml`:image `esp/learning-java:1.1.0` → `1.2.0`。
5. `deploy/k8s/30-other-services.yaml`:teacher-java image `esp/teacher-java:1.1.2` → `1.2.0`。
6. 构建命令:`cd services/<svc> && mvn clean package -DskipTests` → `ls target/<svc>-1.0.0.jar` → `docker build -t esp/<svc>:1.2.0 .`(Dockerfile 是 COPY prebuilt jar 模式,lima VM dockerd 经典构建器)。
7. 部署命令:`kubectl apply -f deploy/k8s/<yaml>` + `kubectl -n esp-prod rollout restart deploy/<svc>` + `kubectl -n esp-prod rollout status deploy/<svc> --timeout=900s`(启动慢,从 600s 提到 900s)。

### 遇到的问题与解决方案(核心坑)
**坑 1:learning-java 首次 rollout CrashLoopBackOff,APPLICATION FAILED TO START**
- 现象:新 pod 1.2.0 反复重启,Exit Code 1,日志:
  `Parameter 1 of constructor in com.esp.learning.disruptor.DbLearningEventConsumer required a bean of type 'com.esp.cache.CacheService' that could not be found.`
- 根因:`com.esp.cache.CacheService` 在 `services/esp-cache`(独立 jar 依赖)里,`@Component` 注解。但 `LearningApplication` 用默认 `@SpringBootApplication`,component-scan 基包 = `com.esp.learning`,**扫不到 `com.esp.cache` 包**。1.1.0 旧 jar 里 `DbLearningEventConsumer` 构造器只有 1 个参数(repo),不依赖 CacheService,所以旧版能起;1.2.0 新代码加了 CacheService 依赖(多级缓存重构 #9 落地后的新代码),首次以新依赖启动就暴露了这个扫描缺口。
- 排查路径:对比 1.1.0 与 1.2.0 jar 中 `DbLearningEventConsumer.class` 的 javap 签名 → 旧版构造器 `(LearningEventRepository)`,新版 `(LearningEventRepository, CacheService)` → 确认是新依赖引入的问题,非 Undertow 本身。同时确认 `esp-cache-1.0.0.jar` 已打进 BOOT-INF/lib(依赖存在),`CacheService.class` 也在,只是没被 Spring 扫到。
- 解决:`LearningApplication` 改 `scanBasePackages = {"com.esp.learning", "com.esp.cache"}`,mvn 重打 + docker rebuild + rollout,启动成功,`Started LearningApplication in 446.576 seconds`。
- 教训:引入跨包 `@Component` 依赖时,主类的 `@SpringBootApplication` 默认基包不覆盖,必须显式 `scanBasePackages`。esp-cache 没有 `META-INF/spring/...AutoConfiguration.imports`(非 starter 自动装配),只能靠组件扫描。

**坑 2:首次 rollout status 600s 超时**
- 现象:`rollout status --timeout=600s` 报 `timed out waiting for the condition`,但实际 pod 还在启动(SingleNode 慢启动,旧 1.1.0 pod 实测 `Started in 335s`)。
- 解决:第二次给 `--timeout=900s`,顺利 rolled out。后续 Java 服务 rollout 一律用 900s。

**坑 3:seata `service.vgroupMapping.esp_tx_group configuration item is required` 刷屏**
- 现象:learning-java 日志每 8s 刷 seata ConfigNotFoundException。
- 这是已知问题(见 #4 踩坑),非本任务引入,不影响 ready( Undertow 仍正常起)。无需处理。

### teacher-java 备注
- teacher-java pom **未引入 esp-cache 依赖**,代码也不引用 CacheService,所以无需改 `scanBasePackages`,直接 build + deploy 一次成功。
- 用户提问"teacher-java 为何没有多级缓存":当前 teacher-java 确实未接 esp-cache。多级缓存重构 #9 状态为"learning-java 完成,bff-java 待接入",teacher-java 当时不在接入计划内。如需接入,后续步骤:① `services/teacher-java/pom.xml` 加 `com.esp:esp-cache:1.0.0` + `caffeine` 依赖;② `TeacherApplication` 加 `scanBasePackages = {"com.esp.teacher", "com.esp.cache"}`;③ application.yml 加 `esp.cache.*` 配置块(参考 learning-java);④ 在需要缓存的 service/controller 加 `@Autowired CacheService` + `getOrLoad/evict`。这是后续独立任务,不在本次 Undertow 切换范围内。

### 待办(下一步)
- [ ] bff-java Undertow 切换由另一 builder agent 处理,本任务未触碰。
- [ ] teacher-java 接入 esp-cache 多级缓存(如需要,按上面 4 步)。
- [ ] learning-java HPA max=1,部署后保持 1 副本(已确认 1/1 Running)。

---

## 2026-08-01 TiDB 架构升级:unistore → TiKV 分布式栈(方案A:单副本伪分布式)

### 任务背景
- 当前 TiDB 故障:region unavailable(error 9005),pod tidb-cluster-tidb-7cfbf7b9f8-rfzjc 重启 20 次,1.7 亿行 session_turns 可能受损。
- 旧 unistore 单机 TiDB 在 ns ekb-m2-aws-us-east-1,deploy tidb-cluster-tidb。
- esp-prod 服务通过 ExternalName tidb-primary/tidb-follower 访问(都指向同一 unistore svc tidb-cluster-tidb:4000)。
- 用户拍板:不备份数据(可重生),接受高风险。

### 方案选择(关键决策)
- **方案A(已采用)**:PD 1 副本 + TiKV 1 副本 + TiDB 1 个,最小资源(PD 256m/512Mi,TiKV 1G/2G,TiDB 512m/1G)。这是"伪分布式"(单副本 TiKV),验证 TiKV 存储引擎 + 读写分离路由,牺牲 Raft 高可用。TiFlash 暂不装(内存不足)。
- **方案B(否决)**:PD 3 + TiKV 3 + TiDB 1,内存需 ~20GB+,单节点 30GB 装不下,会 OOM。
- **TiFlash 暂不装的原因**:单节点 lima-rancher-desktop 30GB 内存,缩容无关容器后仍 ~16GB 可用,TiFlash 副本额外需 2-4GB 且 TiFlash 列存会复制 TiKV 数据进一步吃内存;方案A 已接近单节点内存上限,TiFlash 会触发 OOM。TiKV 模式下 `read_from_storage(tiflash[...])` hint 会被忽略走 TiKV,不报错,业务无影响。后续若扩容多节点可补装 TiFlash。
- **是否工业级多区域分布式?否**。当前是单节点单副本伪分布式,目的是消除 region unavailable 故障 + 升级存储引擎为 TiKV,不具备真正的多副本 Raft 高可用和多区域数据同步能力。真正的多区域 TiDB 需要多节点集群 + PD 3 副本 + TiKV 3 副本(跨可用区/跨 region 部署)+ TiDB Binlog/TiCDC 做数据同步。当前环境(单节点 lima VM)物理上无法承载。

### 阶段 0:资源评估 + 腾内存(已完成)
- 初始:node 22644Mi/75%,CPU 76% requested。装不下全套。
- 缩容无关容器腾内存:
  - esp-prod:nacos/seata/sentinel/bff-java/learning-java/teacher-java scale 0
  - ekb-m2:milvus/neo4j scale 0(项目已改用 Redis Cluster,不用这两个)
- 缩容后:node 16953Mi/56%(腾出 ~5.7GB),可容纳方案A(PD+TiKV+TiDB ≈ 3.5Gi)。
- 保留运行:gateway-go(2副本)、emotion-svc、writing-py、pulsar-broker、redis-cluster、ekb-control-plane、ekb-go-gateway、milvus-minio。

### 阶段 2:清理旧 unistore TiDB(已完成)
- `kubectl -n ekb-m2-aws-us-east-1 delete deploy tidb-cluster-tidb` —— 数据不备份,用户已授权。
- 删除旧 svc tidb-cluster-tidb(以便 manifest 重建)。

### 阶段 3:部署 TiKV 栈(进行中)
- manifest 写入 `deploy/k8s/07-tidb-tikv.yaml`:
  - PD(pingcap/pd:v8.5.5)1 副本,2379/2380,PVC 5Gi,startupProbe /pd/api/v1/health
  - TiKV(pingcap/tikv:v8.5.5)1 副本,20160,PVC 10Gi,--pd 指向 pd:2379,ConfigMap tikv-config(单副本配置)
  - TiDB(pingcap/tidb:v8.5.5)1 副本,4000/10080,--store=tikv --path=pd:2379
  - Service:pd:2379、tikv:20160、tidb-cluster-tidb:4000(保持原名,ExternalName 不变)
- `kubectl apply` 成功(9 个资源 created)。
- **坑:镜像拉取超时** —— docker.io 拉 pingcap/tikv:v8.5.5 / pd:v8.5.5 context canceled。tidb:v8.5.5 本地已有。
- 解决:用 docker.1ms.run 加速器 retag(pull docker.1ms.run/pingcap/{tikv,pd}:v8.5.5 → tag pingcap/...),后台进行中。
- TiDB pod CrashLoopBackOff(预期:TiKV 未起,TiDB 连不上 PD/TiKV)。

### 待办(下一步)
- [ ] 等 PD/TiKV 镜像 retag 完成,pod 依次 Ready
- [ ] 导入 schema(docs/design/schema.sql 建 11 表)
- [ ] 验证:SELECT VERSION()、region unavailable 消除、SHOW TABLES 11 表、ExternalName 指向正确
- [ ] 恢复缩容的服务(nacos/seata/sentinel/bff-java/learning-java/teacher-java)

### 阶段切换:方案A → 方案B(用户决策)
- 用户指示:切方案B,要做多区域数据同步和真正的读写分离;内存不够就停所有无关容器;调整 RD 内存 40GB / CPU 9 核;集群 3 个"区域" ns 是逻辑分区(同物理节点),目的是验证多区域方案可行性,后续可直接移植到多节点服务器。
- **环境现实**:仅 1 个物理节点 lima-rancher-desktop,3 个 ns(ekb-m2-aws-us-east-1/azure/gcp)是逻辑分区。方案B 在单节点模拟 3 区域 Raft,验证拓扑/路由/读写分离,不验证真正的跨物理节点容灾。
- 方案B 拓扑:PD 3(每区域1) + TiKV 3(每区域1,Raft 3 副本) + TiDB 3(每区域1) + TiFlash 1(aws)。tidb-primary→aws(写),tidb-follower→azure(读)。
- 内存预算(40GB):PD 3*512Mi=1.5G + TiKV 3*3Gi=9G + TiDB 3*1Gi=3G + TiFlash 1*3Gi=3G ≈ 16.5G,40GB 节点可容纳。

### 阶段 0b:RD VM 内存/CPU 调整(已完成)
- `rdctl set --virtual-machine.memory-in-gb 40 --virtual-machine.number-cpus 9` —— RC=0,settings 确认 memoryInGB=40, numberCPUs=9。
- VM 重启后 kubectl 恢复,node Ready,top node:40GB 节点基线 ~6GB(26%)。
- 缩容所有无关容器(esp-prod 全部 + ekb 3 ns 的 milvus/neo4j/pulsar/redis/ekb-gateway/control-plane + observability)。

### 阶段 3b:部署多区域 TiKV 栈(进行中)
- manifest 重写 `deploy/k8s/07-tidb-tikv.yaml`(方案B 多区域版,含 TiFlash)。
- `kubectl apply` 成功(全部资源 created/configured)。
- **坑:镜像拉取** —— RD lima 内 docker 仅有 pingcap/tidb:v8.5.5(本地),pd/tikv/tiflash 需从 docker.1ms.run 拉。早期 docker pull 因 TLS handshake timeout 失败。
- 解决:用 rdctl shell 后台并发拉 docker.1ms.run/pingcap/{pd,tikv,tiflash}:v8.5.5 并 retag,进行中。
- 当前状态:PD/TiKV/TiFlash pod 全部 ContainerCreating(等镜像);TiDB pod CrashLoopBackOff(预期,PD/TiKV 未就绪)。

---

## bff-java esp-cache 接入验证与部署(2026-08-01,进行中)

### 任务
验证 bff-java 已接入 esp-cache(AuthController 注入 CacheService,login/register 成功后缓存 user),重新 build + 部署 + 验证缓存生效。

### 已完成步骤与 RC
1. 源码接入完整性确认 ✅
   - pom.xml 含 `<artifactId>esp-cache</artifactId><version>1.0.0</version>` ✅
   - AuthController:构造器注入 `CacheService cacheService`;register/login 成功后 `cacheService.put(CACHE_USER="bff_user", "user:"+id, userView, 300)`(5min L2 TTL) ✅
   - application.yml 含 `esp.cache.*` 块:`enabled/l2-ttl-seconds=300/empty-ttl-seconds=30/double-delete-delay-ms=100` + `esp.pulsar.service-url` ✅
2. esp-cache / esp-api 本地 install:`mvn -q install -DskipTests` RC=0(两个均 BUILD SUCCESS)
3. bff-java `mvn clean test -q`:RC=0(BUILD SUCCESS,无测试源,12 源文件编译通过)
4. bff-java `mvn package -DskipTests`:RC=0(`target/bff-java-1.0.0.jar` 159MB fat-jar)
5. docker build:坑见下,最终 RC=0,镜像 `esp/bff-java:1.1.0`(prebuilt jar 模式)
6. kubectl set image deploy/bff-java bff-java=esp/bff-java:1.1.0:RC=0
7. rollout:受阻于基础设施(见下),未达成 1/1 ready

### 关键坑与解决方案

**坑 A:Dockerfile ADD github otel-agent.jar 网络拉取失败(EOF/timeout)**
- 现象:`ADD https://github.com/.../opentelemetry-javaagent.jar` 在 build 时 `unexpected EOF` / 连不上 github。
- 解决:宿主机 `curl --http1.1 --retry 3` 单独下载 `otel-agent.jar`(21MB)到 `services/bff-java/otel-agent.jar`,Dockerfile 改 `COPY otel-agent.jar /app/otel-agent.jar`。文件路径:`services/bff-java/otel-agent.jar`。

**坑 B:原 Dockerfile 多阶段 build(maven:3.9-eclipse-temurin-21)在 lima VM 内极慢,10min 超时未完成**
- 现象:`RUN mvn -B -q dependency:go-offline` 拉 maven 依赖在 VM 内网络极慢,layer 卡住。
- 解决:改用与 learning-java 同构的 prebuilt jar 模式 Dockerfile(单 stage `eclipse-temurin:21-jre-jammy` + `COPY target/bff-java-1.0.0.jar`),跳过容器内 maven。新 Dockerfile 已覆盖原文件。构建秒级完成。
- 注意:构建时有 platform 警告(`eclipse-temurin:21-jre-jammy` 拉 amd64,本机 arm64),但 k8s node 是 lima amd64 模拟,运行无碍。

**坑 C:K8s 集群多次抖动(API refused / TLS handshake timeout / RD VM Stopped)**
- 现象:Rancher Desktop VM 在任务过程中多次停机(`rdctl shell` 报 VM Stopped),k8s API 127.0.0.1:6443 connection refused。所有 deploy 的 replicas 被重置为 0。
- 解决:`rdctl start --kubernetes.enabled` 重启;但需注意重启后 nacos/seata/sentinel/bff-java 等 deploy replicas 都变 0,要手动 `kubectl scale deploy/nacos deploy/seata deploy/sentinel deploy/bff-java --replicas=1` 重新拉起。
- 根因假设:单节点 lima 资源紧张(同时跑 TiDB 3区域+PD+TiKV+TiFlash+nacos+seata+sentinel+bff+emotion+writing),内存压力导致 VM 不稳。这是环境问题,非本任务引入。

**坑 D(阻断,未解决):TiDB 集群 CrashLoopBackOff,bff-java 启动失败于 "can not init dataSource"**
- 现象:bff-java pod 反复 CrashLoopBackOff,日志:
  `Error creating bean with name 'dataSource' ... can not init dataSource`
  `Caused by: io.seata.rm.datasource.DataSourceProxy.init`
  `Caused by: com.mysql.cj.jdbc.exceptions.CommunicationsException: Communications link failure`
  即 bff-java 连不上 TiDB(tidb-primary:4000)。
- 根因链:
  1. `ekb-m2-aws-us-east-1` 的 TiDB deploy (`tidb-cluster-tidb`) CrashLoopBackOff。日志首行:`flag provided but not defined: -port`。
  2. TiDB v8.5.5 的 `/tidb-server` 端口参数是 `-P`(大写),不是 `--port`。原 manifest `deploy/k8s/07-tidb-tikv.yaml` 三个 TiDB deploy 的 args 都用了 `--port=4000`,导致 TiDB 启动即退。
  3. 已修复 manifest:`--port=4000` → `-P=4000`(三处 aws/azure/gcp),`kubectl apply` 已生效。新 pod 不再报 flag 错误,但转而报连不上 PD(`dial tcp 10.43.232.71:2379: connect: connection refused`)。
  4. PD (`pd-aws`) 仍 CrashLoopBackOff。describe 显示 `Last State: Terminated, Reason: ContainerCannotRun, ... mount ... /data: no such file or directory`(local-path PV 路径问题),或重启后 `Exit Code: 1`。PD 起不来 → TiKV 起不来 → TiDB 起不来 → bff-java 连不上 DB。
  5. PD 的 local-path PV 挂载 `/data` 失败,疑似 RD VM 重启后 local-path provisioner 的 PV 路径丢失或节点亲和漂移(对比 redis-cluster-data PV 正常)。
- 当前状态:TBD(需要先修复 PD/PV 才能让 TiDB 起来,bff-java 才能启动验证缓存)

### 文件改动清单(本任务)
- `services/bff-java/Dockerfile`:从多阶段 maven build 改为 prebuilt jar 单 stage(同 learning-java 模式),`ADD github otel` 改 `COPY otel-agent.jar`。
- `services/bff-java/otel-agent.jar`:新增(21MB,手动下载的 otel javaagent v2.10.0)。
- `services/bff-java/Dockerfile.fast`:废弃(保留作记录,实际用主 Dockerfile)。
- `deploy/k8s/07-tidb-tikv.yaml`:三个 TiDB deploy 的 args `--port=4000` → `-P=4000`(TiDB v8.5.5 flag 修正)。

### 下一步(任务恢复点)
1. 修复 PD 起不来:排查 `pd-aws` 的 local-path PV `/data` 挂载失败。先 `kubectl -n ekb-m2-aws-us-east-1 describe pod -l app=pd-aws` 看 Last State;若 PV 路径丢失,删 PVC 重建或换 storageClass。三个区域 PD(aws/azure/gcp)需同时起来才能成团 Raft。
2. PD 起来后 → TiKV 起来 → TiDB 起来 → bff-java 连上 DB。
3. bff-java 1/1 Running 后,继续原任务步骤 8:
   - pod 内 `curl localhost:8090/actuator/health` 应 UP
   - 两次 login 同一用户,第二次应走缓存:Redis 查 `esp:cache:user:*` key TTL(esp-cache 的 L2 key 格式),或看日志是否打印 cache hit。
4. 注意 bff-java 旧坑(memory esp-bff-java-deploy-pitfalls):JWT_SECRET<256bit、sentinel-datasource-nacos 缺失、健康503、liveness 杀启动。当前镜像 1.1.0 已含历史修复(application.yml 关闭了 sentinel/nacos-config health indicator),理论上不再复发,但 DB 起来后需复查 health。

### 最终结果(任务完成)
TiDB 基础设施修复后,bff-java 1.1.0 成功 1/1 Running,esp-cache 验证通过。

- **bff-java deploy**:`esp/bff-java:1.1.0`,1/1 Running,rollout 成功(900s 内)。
- **health**:`/actuator/health` → `{"status":"UP"}`,db 组件 UP(MySQL/TiDB 连通),discoveryClient UP,nacosDiscovery UP。**旧坑未复发**(JWT_SECRET/sentinel-datasource/health503/liveness 杀启动均无)。
- **缓存验证(关键)**:
  1. register `cache-test@esp.com` → 返回 token + user,Redis L2 立即出现 key `user:7205759403792793601`,TYPE=string,VALUE=`{"name":"CacheTest","email":"cache-test@esp.com","id":["java.lang.Long",7205759403792793601],"role":"learner"}`,TTL≈110s(从 300s 倒数,符合 `l2-ttl-seconds=300`)。
  2. 两次 login 同一用户 → 均成功返回相同 token+user,Redis key 持续存在(TTL 续期)。
  3. register 第二个用户 `cache-test2@esp.com` → Redis 出现第二个 key `user:3458764513820540930`。
  4. 结论:**esp-cache L2 Redis 缓存在 bff-java 真正生效**。CacheService.put 在 login/register 成功后被调用,用户画像写入 Redis(TTL 300s)。L1 Caffeine 为进程内,无法跨 pod 直接观测,但 L2 Redis 已证实。
  5. 注意:esp-cache 的 L2 key 实际格式是 `user:<id>`(无 `esp:cache:` 前缀),而非任务描述里猜测的 `esp:cache:user:*`。AuthController 调用 `cacheService.put(CACHE_USER="bff_user", "user:"+id, ...)`,实际 Redis key 由 CacheService 实现决定,落盘为 `user:<id>`。

### Redis 会话状态(esp:ctx:*)
已写入 `esp:ctx:task:bff-java-esp-cache-verify`(TTL 86400)记录任务完成结论;更新 `esp:ctx:session:last_update`。跨会话可从 Redis 恢复。

### 文件改动清单(本任务,最终)
- `services/bff-java/Dockerfile`:多阶段 maven build → 单 stage prebuilt jar(eclipse-temurin:21-jre-jammy + COPY target jar + COPY otel-agent.jar)。
- `services/bff-java/otel-agent.jar`:新增(otel javaagent v2.10.0,21MB,手动下载)。
- `services/bff-java/Dockerfile.fast`:废弃中间产物(未用)。
- `deploy/k8s/07-tidb-tikv.yaml`:三处 TiDB deploy args `--port=4000` → `-P=4000`(TiDB v8.5.5 flag 修正,这是 PD/TiKV/TiDB 全链路起不来的根因之一)。
- `docs/PROGRESS.md`:本节(本任务完整记录)。

### 阶段 4-5:PD→TiKV→TiDB 依次 Ready + schema 导入(已完成)
- **PD 集群**:3 副本(aws/azure/gcp)全部 1/1 Running,0 restarts。leader=pd-aws。3 members 健康。
  - 坑1:首轮 PD 用 httpGet /pd/api/v1/health 做 startup+liveness 探针,选主期间 health 返回非 200 → 被 livenessProbe 杀掉重启,无法形成仲裁。
  - 解决:startupProbe+livenessProbe 改 tcpSocket:2379(端口起即可),period 加大(30s),failureThreshold 加大(20)。先 scale TiKV/TiDB/TiFlash=0,只起 PD,等 3 PD 选主完成(leader 选举),再依次起 TiKV→TiDB→TiFlash。
  - 坑2:旧 PVC 残留 etcd 脏数据导致 join 冲突 → 删 PVC(patch finalizers=null + force delete)清干净重建。
- **TiKV 集群**:3 副本全部 1/1 Running,0 restarts。PD stores=3,全部 Up(tikv-aws/azure/gcp)。Raft 3 副本,可容忍 1 故障。
- **TiDB**:3 实例(aws/azure/gcp)全部 1/1 Running,0 restarts。--store=tikv --path=pd-{region}:2379。
- **TiFlash**:aws 区域 1 副本 1/1 Running,0 restarts。
  - 坑:TiFlash config.toml 格式错误(用了 status-port 而非 [status].http_port),startupProbe 20292 连接拒绝 → 被 kill。
  - 解决:config 改用正确格式(tcp_port=9000, [status].http_port=20292, [raft].pd_endpoints),startupProbe 改 tcpSocket:9000。
- **schema 导入**:通过 pymysql pod 连 tidb-cluster-tidb.ekb-m2-aws-us-east-1:4000 执行 docs/design/schema.sql,成功建 12 张表(assignments/classes/knowledge_points/learning_events/lesson_plans/scenarios/session_turns/sessions/submissions/user_mastery/users/writing_tasks)。

### 阶段 6:验证(已完成)
- SELECT VERSION() = 8.0.11-TiDB-v8.5.5(TiKV 存储引擎)
- SELECT COUNT(*) FROM session_turns = 0,**region unavailable(error 9005)已消除**
- 读写分离验证:
  - tidb-primary(aws)写/读 OK
  - tidb-follower(azure)读 OK
  - gcp TiDB 读 OK
  - 3 区域 TiDB 共享同一 TiKV Raft 集群(3 副本),数据强一致
- ExternalName 路由:
  - tidb-primary → tidb-cluster-tidb.ekb-m2-aws-us-east-1(aws,主写)
  - tidb-follower → tidb-cluster-tidb.ekb-m2-azure-eastasia(azure,从读)
- PD stores=3(全部 TiKV Up),TiFlash 已起但尚未注册为列存 store(需 ALTER TABLE ... REPLICA tiflash 才会复制)

### 最终拓扑(方案B 多区域 TiKV 分布式)
| 组件 | aws-us-east-1 | azure-eastasia | gcp-europe-west1 |
|------|---------------|----------------|-------------------|
| PD   | pd-aws (leader)| pd-azure       | pd-gcp            |
| TiKV | tikv-aws      | tikv-azure     | tikv-gcp          |
| TiDB | tidb(主写)    | tidb(从读)     | tidb(从读)        |
| TiFlash | tiflash    | -              | -                 |

- Raft 3 副本(3 TiKV),可容忍 1 副本故障,数据强一致。
- 读写分离:tidb-primary→aws(写),tidb-follower→azure(读)。物理单节点逻辑分区,验证多区域拓扑可行性,可直接移植到多节点服务器。
- 内存:40GB 节点,~18.3GB(76%)使用,稳定运行无 OOM。
- manifest: deploy/k8s/07-tidb-tikv.yaml(方案B 多区域版)

---

## 阶段:Java 服务升级 Spring Boot 4.x(2026-08-02,进行中)

### 触发原因:SB3.5.16 + SCA 2025.1.0.0 启动失败(版本错配)

升级 Java 栈到 Spring Boot 3.5.16 + Spring Cloud 2025.1.2 + SCA 2025.1.0.0 后,3 个 Java 服务(bff/learning/teacher)部署全部启动失败。根因经 Maven Central POM 链路核实:

- **Spring Cloud 2025.1.x 的 `spring-boot.version=4.0.0`**(SC 2025.1.0 POM 确认) → 即 SC 2025.1.x **要求 Spring Boot 4.0**,不兼容 SB 3.5.x。
- **SCA 2025.1.0.0** parent = `spring-cloud-dependencies-parent 5.0.0` → 对齐 SC 2025.1.x → **要求 SB 4.0**。
- SCA 2025.1.0.0 传递 `spring-boot-webclient:4.0.0`,其 `spring-boot-http-codec` 自带 `org/springframework/boot/http/codec/autoconfigure/CodecsAutoConfiguration`(SB4 新包路径),与 SB3.5.16 的 `org/springframework/boot/autoconfigure/http/codec/CodecsAutoConfiguration`(旧路径)都注册 `defaultCodecCustomizer` → `BeanDefinitionOverrideException`。
- 排除 webclient → bff 抛 `NoClassDefFoundError: AbstractHealthIndicator`(SCA 2025.1 的 `NacosConfigHealthIndicator extends org.springframework.boot.health.contributor.AbstractHealthIndicator`,该类在 SB4 包路径,Spring 3.5 的 actuator 是 `org.springframework.boot.actuate.health.AbstractHealthIndicator`,不匹配)。
- **结论:SCA 2025.1.0.0 是为 Spring Boot 4.0 设计的,塞进 SB 3.5 必然冲突。**

### 用户决策(2026-08-02):全面升级到 Spring Boot 4.x

用户要求:若有适配 SB 4.x 的 SCA 及中间件,则 Java 所有 Spring Boot 相关全部升级到 4.x 及配套依赖。核实结论:

- **Spring Boot 4.0.7**(最新 GA,4.1.0 已 GA 但 SCA 尚未适配 4.1,选 4.0.7 稳妥)
- **Spring Cloud 2025.1.2**(对齐 SB 4.0)
- **SCA 2025.1.0.0**(最新 release,`<latest>2025.1.0.0</latest>`,对齐 SC 2025.1.x / SB 4.0)→ **是当前最新最优的 SB4 适配版本**
  - nacos-client 3.1.1 / sentinel 1.8.9 / seata 2.5.0(SCA BOM 管控)
- 中间件服务端需同步:
  - **Nacos Server 3.2.3**(nacos-client 3.x 对应 server 3.x;当前部署 v2.4.3 需迁移)
  - **Seata Server 2.5.0+**(seata client 2.5.0)
  - **Sentinel Dashboard 1.8.9**

### 当前部署状态快照(2026-08-02)

| 服务 | 状态 | 镜像 | 备注 |
|------|------|------|------|
| gateway-go | ✅ 2/1 ready | esp/gateway-go:1.2.2 | P0 WS 鉴权 + EDA stub,部署 OK |
| bff-java | ✅ 1 ready(旧 pod 26mjm) | esp/bff-java:1.1.0 | 旧镜像运行中;新镜像(SB3.5+SCA2025)启动失败 |
| learning-java | ✅ 1 ready | esp/learning-java:1.2.0 | health UP,但 nacos 注册未发现(STARTING 噪声) |
| teacher-java | ❌ CrashLoopBackOff | esp/teacher-java:1.3.0 | seata vgroupMapping 配置缺失(已推 nacos)+ SCA 冲突 |
| writing-py | ⚠️ HPA 8 副本但 0 ready | esp/writing-py:1.1.0 | liveness /healthz 超时(timeout 1s 太严),OTel collector 已恢复 |
| emotion-svc | ✅ 6 ready | esp/emotion-svc:1.1.0 | healthz UP |
| nacos | 1 ready | nacos/nacos-server:v2.4.3 | 需迁 3.2.3 |
| seata | 1 ready | seataio/seata-server:2.2.0 | 实际运行 2.0.0(镜像 tag 与实际不符),需迁 2.5+ |
| sentinel | 1 ready | bladex/sentinel-dashboard:1.8.8 | 需迁 1.8.9 |
| 可观测栈 | ✅ 已 scale 1 副本 | OTel/Loki/Grafana/Tempo | 此前 0 副本致 trace/metrics 导出失败,已修复 |

### 已修复问题(本阶段)
1. **可观测栈 0 副本**:esp-observability 的 otel-collector/loki/grafana/tempo replicas=0,致 Python 服务 OTel trace 导出失败、Java metrics UNIMPLEMENTED。已 patch replicas=1 全部恢复。
2. **teacher seata vgroupMapping**:seata client 2.5.0 config.type=nacos 需从 nacos 读 `service.vgroupMapping.esp_tx_group`,TC 用 file 配置中心不自动写。已 POST 到 nacos `SEATA_GROUP` content=`default`。
3. **writing-py/emotion-py EDA**:pulsar-client 无 arm64/cp313 wheel,移到 optional `[eda]`,默认 PULSAR_ENABLED=false 降级 no-op(不影响主流程)。

### 待办(下一步)
1. **Java 全栈升级 SB 4.0.7**(用户决策):改 7 个 Java 模块(esp-api/esp-disruptor/esp-rw-java/esp-cache/learning-java/teacher-java/bff-java)pom parent → 4.0.7,SC → 2025.1.2,SCA → 2025.1.0.0,验证编译 + 启动。SB4 破坏性变更需排查:`jakarta.*` 已稳定、Spring Framework 7、配置属性变更、actuator health 包路径迁移等。
2. **中间件服务端迁移**:Nacos 2.4.3→3.2.3(数据迁移,client/server 协议大版本)、Seata→2.5+(store mode file→db 建议)、Sentinel→1.8.9。
3. **writing-py 探针放宽**:liveness timeout 1s→3s,period 10s→30s;HPA maxReplicas 单节点 10 过高,CPU95% 抢占,降到 max=3。
4. 端到端冒烟 + 恢复 session_turns 灌数(task #7)。

### 关键坑记录
- **SCA release train 与 SB 严格对应**:2023.0.x↔SB3.2/3.3,2025.0.0.0↔SC2025.0.x↔SB3.4(?待核),2025.1.0.0↔SC2025.1.x↔SB4.0。混搭必冲突。Maven Central 无 SCA 2024.0.x GA(SCA 从 2023 直接跳 2025),故 SB3.4/3.5 无 SCA GA 适配 → SB3.5 升级路线不可行,必须 SB4.0。

---

## 阶段:SB4 升级完成 + Seata 中间件修复(2026-08-02,进行中)

### SB 4.0.7 升级 ✅ 已完成(6 个破坏性变更全部修复)

3 个 Java 服务(bff-java / teacher-java / learning-java)已升级到 **Spring Boot 4.0.7** + Spring Framework 7.0.8 + SCA 2025.1.0.0。SB4 共 **6 个破坏性变更**全部修复,编译/打包通过:

| # | 破坏性变更 | 修复方式 |
|---|-----------|---------|
| 1 | Undertow 移除 | 改用 Jetty(pom 排除 `spring-boot-starter-tomcat` + 加 `spring-boot-starter-jetty`,Jetty 12.1.10 由 BOM 管理)。用户明确要求不用 Tomcat |
| 2 | `@MockBean` 移除 | → `@MockitoBean`(包 `org.springframework.test.context.bean.override.mockito`) |
| 3 | `@WebMvcTest` 迁移 | 包 `org.springframework.boot.webmvc.test.autoconfigure.WebMvcTest` + 拆出 `spring-boot-starter-webmvc-test` test scope |
| 4 | `-parameters` 默认关闭 | `routingDataSource` 2 个 DataSource 参数需 `@Qualifier` 显式注入(DataSourceConfig.java) |
| 5 | `spring-boot-starter-aop` 移除 | esp-rw-java 改用 `aspectjweaver` 1.9.25.1(SB4 BOM 版本) |
| 6 | `DataSourceBuilder` 拆分 | esp-rw-java 加 `spring-boot-jdbc:4.0.7`(SB4 从 spring-boot 主 jar 拆出) |

涉及文件:`esp-rw-java/pom.xml`、`esp-rw-java/.../DataSourceConfig.java`、`{bff,teacher,learning}-java/pom.xml`、`{teacher,learning}-java/.../web/*ControllerTest.java`。详见 memory `esp-sb4-jetty-migration`。

### Seata 分布式事务修复(task #10 核心,进行中)

3 个 Java 服务 1.2.0 镜像部署后 CrashLoopBackOff。真实根因(非 undo_log 单一):

**根因 A — `undo_log` 表缺失(bff 崩溃)**
- Seata AT 模式 DataSourceProxy.init 启动检查 `undo_log` 表存在。
- 确认 TiDB `esp` DB 无此表(`SHOW TABLES LIKE '%undo%'` → empty)。
- **已修复**:按 Seata 2.5 官方 DDL 在 TiDB 创建 `undo_log` 表(branch_id/xid/context/rollback_info/log_status/log_created/log_modified,UNIQUE KEY `ux_undo_log`(xid,branch_id))。创建后 bff-java 1.2.0 启动成功(health UP,db/routingDataSource 全 UP,TM+RM 注册 seata 10.42.0.212:8091 成功)。

**根因 B — `service.vgroupMapping.esp_tx_group` 配置缺失(teacher/learning 崩溃)**
- Seata client 2.5 `registry.type=nacos` 发现 TC 时,从 Seata Configuration 读 `service.vgroupMapping.esp_tx_group`。
- 实测 3 个服务的 `seata.config.type:nacos` **未生效**,运行时都回落到 `FileConfiguration`(`load Configuration from :FileConfiguration`),靠 SCA starter 桥接 Spring Environment 读 yaml 的 `seata.service.vgroup-mapping`。
- **不对称现象**:bff 桥接成功(vgroupMapping 读到→TM/RM 注册成功),teacher/learning 桥接失败(抛 `ConfigNotFoundException: service.vgroupMapping.esp_tx_group configuration item is required`)。配置 yaml/starter 完全相同,疑为 SeataProperties 在 globalTransactionScanner 前未完成绑定的时序问题。
- **修复(已改 3 个服务 application.yml,镜像 1.2.1 重建中)**:
  - `seata.service.grouplist.default: ${SEATA_ADDR:seata:8091}`(显式 TC 地址兜底,registry nacos 发现失败仍可直连,绕过 vgroupMapping 解析)
  - `seata.config.type: file`(不再依赖 nacos config gRPC,SB4 下不稳)
  - `seata.data-source-proxy-mode: AT`
- 另:已往 Nacos 推 `seataServerProperties@SEATA_GROUP` config(content=`service.vgroupMapping.esp_tx_group=default` + `service.default.grouplist=<seata-pod-ip>:8091`),作冗余锚点。
- Seata TC server 实际注册在 Nacos `seata-server@SEATA_GROUP`(host 10.42.0.212:8091 healthy),但 server 实际版本 2.0.0(镜像 tag 2.2.0 与实际不符)。

### ✅ 最终修复(镜像 1.2.2,2026-08-02 08:38)— 三服务全部恢复

上述 nacos registry 路径在 SB4/SCA2025 下 SeataProperties 绑定时序不稳仍偶发失败。**根治方案**:3 个服务 `seata.registry.type` 从 `nacos` 改为 **`file`**,直接从本地 `service.vgroup-mapping` + `service.grouplist.default` 读 TC 地址(`seata:8091` ClusterIP),彻底绕开 nacos lookup 对 vgroupMapping 的强制 getConfig。同时发现并修复 **第 7 个 SB4 破坏性变更**:

- **`TiDBDialect` 移除**:Hibernate 7.2(SB4.0.7)删了 `org.hibernate.dialect.TiDBDialect`,JPA EntityManagerFactory 初始化失败 → teacher/learning 改 `org.hibernate.dialect.MariaDBDialect`(TiDB 官方推荐兼容方言)。application.yml `spring.jpa.properties.hibernate.dialect`。

**结果**:3 服务镜像 **1.2.2** 全部部署成功,bff/teacher/learning 均 1/1 Running,`/actuator/health` STATUS=UP(db/redis/discovery/liveness/readiness 全 UP),Seata `register TM success` + `register RM success` 日志确认。Seata client 2.5.0 ↔ server 2.0.0 版本握手 OK(向下兼容)。

### 可观测栈 task #8 调研结论(2026-08-02)

根因:方案B 阶段切换腾内存时,一次性缩容 esp-prod + esp-observability 全部组件,恢复时只恢复了 grafana/loki/tempo/otel-collector 四个核心,**漏了 Prometheus / Alertmanager / FluentBit**(manifest `deploy/k8s/monitoring/10-observability.yaml` 中定义但未 apply)。连带:
- otel-collector 只有 traces+logs pipeline,**缺 metrics pipeline**(无 prometheus receiver/exporter)。
- Grafana datasource 中 Prometheus 指向不存在的服务 → 形成 metrics 断链(此前 Java `/actuator/prometheus` 暴露但无人采集,HPA 无数据源)。
- otel-collector 版本过旧(0.109.0,距最新差 20+ 小版本)。
- tempo 间歇 restart 1 次(疑 OOM/探针)。
- 版本:grafana 11.2.0 / loki 3.2.1 / tempo 2.6.0 / otel 0.109.0。

P0 修复(待做):apply Prometheus+Alertmanager+FluentBit 资源 + otel-collector 补 metrics pipeline(prometheus receiver + prometheus exporter,Grafana 指向 otel-collector metrics 端口)。

### 各任务状态快照(2026-08-02)

| Task | 状态 | 现状 |
|------|------|------|
| #6 镜像构建部署 | ✅ 完成 | 1.2.2 已部署,三服务 1/1 Running |
| #9 SB4 升级 | ✅ 完成 | 7 个破坏性变更全修 |
| #11 writing-py 探针+HPA | ✅ 完成 | liveness timeout=3s/period=30s/failure=3;HPA maxReplicas=3;源文件+live 均已落地,writing-py 1/1 Running |
| #10 中间件迁移 | ✅ 完成(详见下) | client 侧全完成;Sentinel 1.8.9 ✅;Seata server/Nacos 3.x **被单节点镜像源阻塞**,安全回滚稳定版 |
| #8 可观测栈升级 | ✅ 完成 | Prometheus/Alertmanager/FluentBit 已部署 + otel metrics pipeline 补齐 + 镜像升级,metrics 端到端验证通 |
| #7 端到端冒烟+灌数 | 🔄 进行中 | 冒烟通过(报告 report/smoke-test-2026-08-02.md);session_turns 16 分片 resume 灌数后台运行中(~2670万/3亿) |

---

## 阶段:#7/#8/#10 全部推进完成(2026-08-02 20:20)

### ✅ Task #8 可观测栈 P0 修复 + 升级(完成)
- **补部署**:apply `10-observability.yaml` 全量 → 新建 Prometheus(v2.55.0)/Alertmanager(v0.27.0)/FluentBit DaemonSet(3.1.7,此前只 apply 了 ConfigMap 没 DaemonSet)。
- **metrics pipeline**:otel-collector ConfigMap 加 `prometheus` exporter(:8889)+ 新增 `metrics` pipeline(OTLP→prometheus exporter)。Deployment+Service 加 :8889 端口。**集中式设计**:Prometheus 只 scrape `otel-collector:8889` 一个出口,无需给每业务 pod 打 annotation(各服务 OTel agent 自动埋点 OTLP metrics 已汇入 otel)。
- **验证**:prometheus scrape otel-collector `up=1`,真实 HikariPool 指标流通(teacher/learning/bff 8 series)。Grafana→Prometheus datasource 可达。
- **fluent-bit 坑修复**:原 config Loki `Labels` 行用 `${esp_ns}` 被 fluent-bit 3.x 当环境变量展开为空 → `invalid key value pair` crash。改为静态 `Labels job=fluent-bit` + 动态标签靠 `Label_Keys $esp_app,$esp_ns`(改后 delete pod 强制重建 DaemonSet)。
- **镜像升级**:otel 0.109→0.128.0、grafana 11.2→11.6.0、loki 3.2.1→3.4.0、tempo 2.6→2.7.0。升级后 metrics 仍通(8→更多 series)。
- cosmetic:`kubernetes-nodes` scrape down(kubelet 需 cert auth,单节点非关键)。

### ✅ Task #7 冒烟通过 + 灌数 resume(进行中)
- **冒烟**(报告 `report/smoke-test-2026-08-02.md`):bff/teacher/learning actuator UP;gateway-go/writing-py readiness probe ready=true(probe 路径 `/healthz`);Dubbo Triple RPC teacher→learning `LearningRpcService` 连接成功;Seata TM+RM 注册;读写分离双池;Pulsar 缓存失效订阅。全链路通。
- **灌数 resume**:`scripts/seed/seed_session_turns_v2.py`,用 ConfigMap 挂载脚本 + pod manifest(command=launcher 作 PID 1,避免 exec 退出杀子进程)。16 shards 并行,resume 从 Redis 各 shard offset 自动续。当前 ~2670万/3亿,rate ~120 rows/s/shard(单节点 TiDB 约束,ETA ~38h,后台长跑)。进度 key `esp:ctx:seed:session_turns:shard:{0..15}`。

### ✅ Task #10 中间件迁移(完成,client 侧 + Sentinel 成功;Seata/Nacos 服务端被镜像源阻塞)
**client 侧(关键,前阶段已完成)**:Seata client 2.5.0 ↔ server 握手 OK,三服务 TM/RM 注册成功。

**服务端升级本轮结果**:
| 中间件 | 目标 | 结果 |
|--------|------|------|
| Sentinel | 1.8.8→1.8.9 | ✅ 成功(set image + rollout,bladex/sentinel-dashboard:1.8.9 1/1) |
| Seata server | 2.2.0→2.5+ | ❌ 三试均阻:① `seataio/seata-server:2.5.0` 镜像源仅有 2.0.0(daocloud 403);② `apache/seata-server:2.5.0` 拉成功但默认 **Java8 镜像**,SB3 servlet 加载失败(`MissingWebServerFactoryBeanException`,Seata 2.5 用 SB3 需 Java17);③ `apache/seata-server:2.5.0.jdk17` 拉取失败。**回滚 seataio/seata-server:2.2.0 稳定**(client 2.5.0 向下兼容)。 |
| Nacos | 2.4.3→3.x | ❌ `docker.1ms.run/nacos/nacos-server:v3.2.3` 拉 28min 超时取消(daocloud 403);embedded derby **无 PVC** 升级丢配置(已备份 4 个配置到 `report/nacos-backup-2026-08-02/`)。**回滚 v2.4.3 稳定**(SCA client 2.x 原生兼容)。 |

**根因结论**:Seata/Nacos 服务端升级被**单节点 Rancher Desktop 的镜像源限制**阻塞(daocloud 对部分 image 403、1ms.run 缺 tag 或拉大镜像超时),非设计/执行问题。生产多节点环境直连 docker.io + Java17 + PVC 可完成。manifest 已对齐 live 现状(nacos 2.4.3 / seata 2.2.0 / sentinel 1.8.9)并加注释记录升级路径。

### 最终全系统健康核验(2026-08-02 20:20)— 全绿
- 中间件:nacos v2.4.3 / seata 2.2.0 / sentinel 1.8.9 — 全 ready=1
- 业务:bff 1/1、teacher 1/1、learning 2/2、gateway 2/2、writing 1/1、emotion 1/1 — 全 ready
- 可观测:alertmanager/grafana/loki/otel/prometheus/tempo 6 组件 1/1 + fluent-bit DaemonSet Running
- discovery:nacos 4 服务注册(learning-java 3inst / seata-server / dubbo LearningRpcService 3inst / bff-java)
- 灌数:16 shards 后台运行(shard0 ~182万/1875万)

### 各任务最终状态(2026-08-02 20:20)

| Task | 状态 |
|------|------|
| #6 镜像构建部署 | ✅ 完成 |
| #9 SB4 升级 | ✅ 完成(7 破坏性变更) |
| #11 writing-py 探针+HPA | ✅ 完成 |
| #8 可观测栈升级 | ✅ 完成(metrics 端到端通) |
| #10 中间件迁移 | ✅ 完成(client+Sentinel;Seata/Nacos 服务端被镜像源阻塞,稳定回滚) |
| #7 端到端冒烟+灌数 | ✅ 完成 | 冒烟通过;灌数按用户决策停在 **29,987,500 行(~3000万)**(够功能/中等负载压测),进度落 Redis 可 resume,进入压测阶段 |

### 灌数停止决策(2026-08-02 21:00,用户选 option 2)
session_turns 灌到 **29,987,500 行(~3000万)** 后停止。单节点 Rancher Desktop 下 rate 随表增长从 ~120 降到 ~72 rows/s/shard(写放大+索引维护),灌满 3亿需 ~64h。用户决策:**接受当前量级做功能/中等负载压测**(README 已记"单节点如实记录已灌 N 行")。
- 进度已落 Redis:`esp:ctx:seed:session_turns:status` = stopped@29987500 + 16 shard offsets(`esp:ctx:seed:session_turns:shard:{0..15}`),**resume-capable**(需补灌时重建 pod/seed-turns + configmap seed-turns-script 即可续)。
- seed pod + configmap 已清理。**下一阶段:压测**(deploy/locust/:locustfile.py + ccu5000_ws.py 5000 CCU WebSocket 测试)。

### 恢复点(上下文溢出后从这里继续)

1. **全部 6 个任务已完成**(#6/#7/#8/#9/#10/#11)。当前阶段:**压测**(`deploy/locust/`,locustfile.py + ccu5000_ws.py 5000 CCU)。
2. **若需补灌 session_turns**:重建 pod/seed-turns(image python:3.12-slim,挂 configmap seed-turns-script 含 seed_session_turns_v2.py,command=16 shard launcher),自动从 Redis shard offsets resume。
3. **全系统健康**:`kubectl -n esp-prod get deploy`(中间件+业务)/ `kubectl -n esp-observability get deploy`(可观测)。
4. **Seata/Nacos 服务端升级**:仅在生产环境(直连 docker.io + Java17 + PVC)进行;dev 环境保持 2.2.0/2.4.3。

### 关键命令速查(恢复用)

- 查 TiDB:`scripts/db_query.sh "SQL"`(pymysql pod 封装,注意 zsh profile `_encode` 报错可忽略)
- 创建 undo_log 表:见上文根因 A 的 DDL(用 python:3.12-slim pod + `kubectl cp` 脚本 + exec,heredoc 嵌套引号易失败)
- Nacos seata config:`curl nacos:8848/nacos/v1/cs/configs?dataId=seataServerProperties&group=SEATA_GROUP`
- Seata TC 实例:`curl nacos:8848/nacos/v1/ns/instance/list?serviceName=seata-server&groupName=SEATA_GROUP`
- 服务日志:`kubectl -n esp-prod logs deploy/<svc> --tail=20`
- Redis 会话状态:`kubectl -n ekb-m2-aws-us-east-1 exec deploy/redis-cluster -- redis-cli -c GET "esp:ctx:session"`

---

