
> ## 5000 CCU 实压测 + 架构评分修订 + 测试工程化缺口(2026-08-02)
> ## 5000 CCU 实压测 + 架构评分修订 + 测试工程化缺口(2026-08-02)
> **用户要求**:① 清理调试 pod + 启动压测;② 从教授级高工/大厂架构师视角 + 联网最佳实践给架构打分,给优化重构方向;③ 评估是否缺功能/Bug/混沌测试流程,子系统/公共包划分是否符合大厂标准,补齐上线标准;④ 沉淀经验供所有 Claude Code 项目采纳。
>
> **Part A 压测环境搭建 ✅**:
> - Mac 与集群均无法连 pypi.org(SSL handshake timeout),locust 无法 pip 装。改用官方镜像 `locustio/locust:2.31.5`(集群内拉取成功)。
> - **locust-runner pod**(`deploy/locust/locust-runner.yaml`):集群内运行,入口走 Traefik ClusterIP(`traefik.kube-system:80`)+ Host 头,绕过外部 NodePort;`ESP_JWT_SECRET` 从 `esp-secrets` 注入。
> - **JWT 鉴权适配**:gateway-go 的 `/api/sessions` 需 Bearer JWT(HS256,sub=userID),原 locustfile 无 token 全 401。改 `locustfile.py`/`ccu5000_ws.py` 加 `mint_jwt()`(hmac HS256,secret 从 `ESP_JWT_SECRET` env),每虚拟用户随机 userID 签发。WS 路径用 `?token=` + owner 校验。
> - **探针误杀修复(压测暴露)**:gateway-go `/healthz` timeout=1s 在高负载下来不及应答 → liveness 误杀(r=9/10)→ 连接中断 + HPA 抓不到高 CPU 不扩容。改 timeout 1→3s、period 10→15s、failureThreshold 3→5(10-gateway-go.yaml + live patch)。seata 同理(r=8)→ 06-sca-middleware.yaml 探针放宽。
>
> **Part B 5000 CCU 实压测结果 ✅(单节点,如实记录)**:
> - **warmup 100 CCU 45s**:14288 req,0 失败,317 rps,P95 240ms —— 健康。
> - **tierA2 1000 CCU 90s(探针修复后)**:71416 req,1 失败,790 rps,P95 1.3s —— 基本健康。
> - **tierC 5000 CCU 220s(seata 探针修复后)**:130483 req,9500 失败(7.3%),533 rps,P95 25s。
>   - per-endpoint:learning/event 277 rps(P95 9.1s)、writing/grade 47 rps(P95 73s)、teacher/lesson-plan 57 rps(P95 62s)、sessions 5 rps(P95 84s)、scenarios 119 rps(P95 15s)。
>   - 主要失败:teacher 503×1895(HPA 扩到 2 但新 pod 启动期不可用)、writing 503/502×1934(LLM 长尾阻塞 worker)、learning 502/500×154。
>   - HPA:learning 扩到 4(3 个 ready=false 启动期)、teacher 扩到 2、writing 扩到 3。单节点资源瓶颈,新 pod 启动慢(注册 + 资源争抢)。
> - **SLO 对照**:G-01 5000 CCU 单节点不可达(需多节点);G-04 写作 47rps 接近 50 但 P95 73s 远超 2s;G-06 学情 277rps 远低 5000;G-02 scenarios P95 15s(非真实 ASR/LLM 链路)。
> - **产物**:`report/tierA2_*.csv`、`report/tierC_*.csv`(已导出 host)。压测后已 scale down 到 baseline(bff/emotion/learning/teacher/writing 各 1,gateway 2)。
>
> **Part C 架构评分修订 ✅**:`docs/design/架构评分与重构方向.md` 全面重写。
> - **加权总分 6.85→7.05/10(B+)**。拉分:可观测 5.5→7.5(7 pod 实跑+metrics 端到端通)、安全 4.5→7.0(WS 鉴权+PDB+NP)、EDA 5.0→6.5(5 topic 闭环)。掉分:工程化 6.0→5.5(测试回填不全+零 CI/CD)。
> - **5000 CCU 暴露 4 真问题**:① 探针高负载误杀(已修)② writing/teacher 503 高发(AI 长尾+HPA 跟不上)③ HPA 扩容但不健康(单节点资源)④ 压测机与被测同节点(测的是混合上限)。
> - **子系统/公共包划分结论**:粒度基本符合大厂标准(公共能力下沉+契约独立+多语言公共包),但**质量保障工程化不达标**:零 CI/CD、零契约测试、公共包零单测、bff-java 安全核心零覆盖。
>
> **Part D 测试工程化缺口盘点 ✅**(对照大厂上线标准):
> - **阻断上线(P0)**:① 安全测试 SEC-1~4 全 `_` 未执行;② esp-disruptor/esp-rw-java/esp-cache 公共包零单测;③ bff-java 零单测(JWT/Auth);④ 无 CI/CD pipeline;⑤ 无契约测试(Pact)。
> - **建议补(P1)**:soak/spike 压测、traceId 贯通实测、Loki 标签基数压测、preStop graceful shutdown。
> - **重构方向**:P0(5 项阻断)→ P1(6 项提升)→ P2(6 项架构演进,多节点)。详见评分文档 §五。
>
> **Part E 经验沉淀(供所有 Claude Code 项目采纳)**:见下方"本轮可复用经验"节 + 待写入全局 memory。
>
> **跨会话恢复**:本节 + Redis `esp:ctx:upgrade-status` + `esp:ctx:session:last_update` 三处对齐。
>
> ## 本轮可复用经验(2026-08-02 压测+评分,供所有项目采纳)
> 1. **K8s 探针 timeout 不能用默认 1s**:高负载下 `/healthz` 来不及应答 → liveness 误杀 → 连接中断 + HPA 抓不到高 CPU 不扩容 → 雪崩。设计期按 startup/liveness/readiness 三探针分离 + timeout≥3s + failureThreshold≥5。**ESP 实测 gateway-go r=9/10、seata r=8 均此因**。
> 2. **压测机不能与被测系统同节点**:单节点跑 locust 测的是"被测系统 + 压测机"混合上限,非纯系统上限。生产用独立 Locust master+workers 节点。**ESP 单节点 5000 CCU 不可达,部分因此**。
> 3. **HPA 扩容速度 < 流量突增速度**:AI 推理长尾 + 新 pod 启动慢(Nacos/Dubbo 注册 + 资源争抢)→ 扩容跟不上 → 503。需预热(surge)或队列削峰(Disruptor/Pulsar)。**ESP writing/teacher 503 高发此因**。
> 4. **Mac/集群 pypi 不可达时压测方案**:用官方预装镜像(locustio/locust)+ ConfigMap 挂脚本 + 集群内 Traefik ClusterIP 入口,绕过 Mac pip 装不上 + 外部 NodePort 不稳。
> 5. **JWT 鉴权端点压测必须 mint token**:鉴权端点(如 ESP `/api/sessions` 需 Bearer JWT)不带 token 全 401,压测脚本必须内置 `mint_jwt()`(hmac HS256,secret 从 K8s Secret 注入 env),不能假设端点无鉴权。
> 6. **公共包/安全核心必须有单测**:被多服务依赖的公共库(路由/缓存/环形缓冲)bug 会放大,安全核心(JWT/鉴权)零覆盖是上线阻断项。大厂标准:公共库 ≥80%、安全代码 100%。
> 7. **架构评分要区分"设计层"与"实现层"**:设计文档再扎实(A-/A),实现层(B-/B)的工程化缺口(测试/CI/契约)是真正阻断上线的因素。评分必须含实测回填,不能只看文档。
>
> 更新日期:2026-08-02(架构评分+依赖升级+EDA重构 启动)
> ## 架构评分 + 全量依赖升级 + Pulsar EDA 重构(2026-08-02 启动)
> **用户要求**:① 从教授级高工/大厂架构师视角+联网最佳实践给架构打分,给优化重构方向;② 全部中间件/框架/JDK 升级到最新稳定版(消bug/攻击);③ 按评分在测试前并行优化完毕。后续追加:④ 评分中 EDA(5.0分)质疑——Pulsar 退化为缓存广播,排查所有服务是否有真实 producer/consumer 场景并重构。
> **跨会话恢复**:本节 + Redis `esp:ctx:session` + `esp:ctx:upgrade-status` 三处对齐即可续跑。
>
> **里程碑-1 架构评分 ✅ 完成**:`docs/design/架构评分与重构方向.md`。加权总分 **6.85/10(B)**。设计层 A-/实现层 B-。11 维度逐项打分。P0 短板(必须本轮做):① gateway-go WS 零鉴权(userIDFromContext 硬编码 return 1,会话劫持高危)② JWT 实际 HS256 非 RS256 ③ 缺 PDB/NetworkPolicy。P1:可观测栈未实际运行、依赖版本滞后(2024中-2025初水平)。P2:缓存一致性三端割裂、WS多Pod广播无实测、learning-worker未隔离。
>   - 明确边界:learning 高频学情落库继续走进程内 Disruptor(最优,Pulsar 反增网络跳数);Pulsar 只在真跨服务事件场景才上。
>
> **里程碑-2 版本清单核对 ✅ 完成**:当前版本已全量清点(Java/Python/Go/前端/K8s镜像/Dockerfile基镜像)。关键滞后:Spring Boot 3.3.4/3.3.5→3.5.16、Spring Cloud 2023.0.3→2025.1.2、SCA 2023.0.1.2→2025.1.0.0、Go 1.22→1.25.12、React 18→19+Vite5→8、langgraph 0.2.x→1.2.10(大版本破坏性)、Nacos v2.4.3→3.2.3、TiDB v8.5.5→8.5.7、Redis 7.2→8.x、Pulsar→4.x。
>
> **里程碑-3 停灌数腾内存 ✅ 完成**:16 个 seed-st-shard Job 全停(delete job),pod 0 残留。进度落 Redis:`esp:ctx:seed:session_turns:status` = paused@2026-08-01:~2350万/3亿,16 shards resume-ready + 16 个 shard offset key 保留。节点内存 93%→90%(释放有限,tikv/pulsar 常驻)。**灌数待升级+重构完成后重启 resume**。
>
> **里程碑-4 依赖升级 🔄 进行中(多轨并行)**:
>   - **Java ✅ 7/7 成功**(含 bff-java):Spring Boot 3.3.4/3.3.5→**3.5.16**、Spring Cloud 2023.0.3→**2025.1.2**、SCA 2023.0.1.2→**2025.1.0.0**、Dubbo 3.3.0→**3.3.6**、JJWT 0.12.6→**0.13.0**、MySQL/J 9.1.0→**9.7.0**、Caffeine→3.2.4、Pulsar client 3.2.4→**3.3.9**、SLF4J→2.0.18、Mockito→5.18.0、Jackson→2.19.4、HikariCP→6.3.3、Netty→4.1.136.Final。7 模块(esp-api/esp-disruptor/esp-rw-java/esp-cache/learning-java/teacher-java/bff-java)全 compile/install RC=0。bff sentinel adapter 删显式 1.8.8 让 SCA BOM 管(1.8.9)。**唯一改源码**:learning-java `AppLogClient.java` 删 3 个未用 import(新版 javac 简单名冲突)。
>   - **【再升级】Spring Boot 3.5.16 → 4.0.7 ✅ 完成(2026-08-02)**:用户指示按 SB4 最新版对标。3 个业务服务(bff-java/teacher-java/learning-java)parent 全改 `4.0.7`(Spring Framework 7.0.8)。**三个 SB4 破坏性变更已修复**:
>     1. **Undertow 移除 → Jetty**:SB4 BOM 已删 `undertow.version`/`spring-boot-starter-undertow`。按用户指示用 Jetty(排除 `spring-boot-starter-tomcat` + 加 `spring-boot-starter-jetty`,Jetty 12.1.10 由 BOM 管理)。**实测确认**:bff-java 日志 `o.e.j.e.servlet.ServletContextHandler`(org.eclipse.jetty.ee.servlet)= Jetty 已接管 servlet 容器。
>     2. **`@MockBean` → `@MockitoBean`**:包路径从 `org.springframework.boot.test.mock.mockito` 迁到 `org.springframework.test.context.bean.override.mockito`。修 teacher/learning 的 ControllerTest。
>     3. **`@WebMvcTest` 包迁移 + 新 starter**:包从 `org.springframework.boot.test.autoconfigure.web.servlet` 迁到 `org.springframework.boot.webmvc.test.autoconfigure`;且 SB4 拆出 `spring-boot-starter-webmvc-test`(不再由 starter-test 传递),teacher/learning pom 显式加此 test scope 依赖。
>     - **构建**:3 服务 `mvn clean package` 全 BUILD SUCCESS(test 全过)。镜像 `esp/{bff,teacher,learning}-java:1.1.0` 已 build + deploy。K8s 实测 pod 启动日志 `spring-boot-4.0.7.jar`/`spring-beans-7.0.8.jar` 确认 SB4+SF7 运行。
>     - **遗留 CrashLoopBackOff**:新 pod 因 **Seata server 不可达**(`Failed to get available servers`)启动失败 —— 属 task #10 中间件迁移范围(Seata 2.2→2.7),非 SB4 问题。Jetty/SB4 本身运行正常。
>   - **Go ✅ 完成**:go 1.25.0、go-redis v9.21.0、otel 1.44.0、grpc 1.83.0、protobuf 1.36.11、prometheus client 1.24.1、mysql driver 1.10.0、golang-jwt v5.3.1。`go build ./...` RC=0。go.sum 待 `go mod tidy`(网络受限暂留)。
>   - **前端 ✅ 完成**:三端 React 18→**19.2**、react-router 6→**7.9**、Vite 5→**7.1**、Zustand 4→**5**、SWR→2.4、TypeScript→5.9、@vitejs/plugin-react→5、antd-mobile(h5)→5.42、vite-plugin-pwa→1.0、Taro 4.0.9→**4.2.1**(React 保留 18,兼容性保守)。三端 npm install RC=0 + tsc RC=0。**React 19 兼容坑已修**:JSX 全局 namespace 移除,`JSX.Element`→`React.JSX.Element`(App.tsx/main.tsx)+ App.tsx 补 `import React`;h5 api.ts headers Record 收窄加 `as Record<string,string>`。
>   - **Python ✅ 基本完成**:venv 装 langgraph 1.2.10/litestar 2.24/anthropic 0.120/redis 8.1。**pyproject 回写 ✅**:writing-py + emotion-svc litestar>=2.24.0、anthropic>=0.120.0(writing)、OTel==1.30.0(原 1.28)、writing 新增 pulsar-client>=3.5.0(EDA)。writing-py main.py grade 接入 writing.graded + py_compile RC=0。**P1 待办**:langgraph 0.2→1.x 代码迁移(agent.py 用 0.2 API,breaking;pyproject >=0.2.50 兼容 1.2.10 但运行需迁移)。
>   - **K8s 镜像 tag + Dockerfile ✅ 基本完成**:
>     - **Redis ✅**:7.2-alpine → **8.0-alpine**(三处)。注:8.0→8.8 是有状态迁移(承载 esp:ctx:* 会话态),需单独停机,标 P1。
>     - **TiDB ✅**:全系 v8.5.5 → **v8.5.7**(pd/tikv/tidb/tiflash,10 处)。
>     - **Nacos ✅(清单)**:v2.4.3 → **v3.1.1**。⚠ 2→3 breaking(IAM/协议/client SDK),ekb-m2 已运行集群,apply 前需 client SDK 全升 + 数据迁移,标 P1。
>     - **Seata ✅(清单)**:seataio/seata-server:2.2.0 → **apache/seata-server:2.7.0**(官方仓库,原 seataio 有漏洞)。仓库+版本双变,配置 2.x application.yml(memory esp-seata-deploy-pitfalls),apply 前验证。
>     - **Sentinel**:bladex/sentinel-dashboard:1.8.8 保持(社区最新,无 1.8.10 镜像)。
>     - **Dockerfile ✅**:eclipse-temurin:21-jre-jammy→**noble**(4 Java Dockerfile)、golang:1.22-alpine→**1.25-alpine**、alpine:3.20→**3.22**、python:3.12-slim→**3.13-slim**(writing-py/emotion-svc/16-seed yaml)。JDK 维持 21(LTS,Spring Boot 3.5 最佳)。
>     - **待办**:Pulsar→4.x、可观测栈(OTel/Loki/Grafana/Tempo/Fluent Bit/Prometheus)未在 deploy/k8s 裸 yaml(疑 compose/helm),待确认。中间件 apply 到 ekb-m2 运行集群均需走迁移流程,非改 tag 即生效。
>
> **里程碑-5 Pulsar EDA 重构 ✅ Java 侧完成**:5 场景方案定,复用 esp-cache PulsarClient+sendAsync 降级模式。Java 侧 3 事件(user.registered/teacher.finalized/teacher.autograde.requested)已完成;py/go 侧 2 事件(writing.graded/session.ended)待后续。producer 延迟初始化+send 失败降级不阻断业务;consumer 幂等 + esp.pulsar.consumer-enabled 可关。**不改 Pulsar 的**:learning_events 高频落库(Disruptor 最优)。
> **Java 侧 EDA 重构落地清单**:
> - **esp-cache 公共事件包 ✅**:新建 `event/` 包,4 个类(EventTopics 常量/UserRegisteredEvent/TeacherFinalizedEvent/AutoGradeRequestedEvent)+ EventPublisher(通用发布器,复用 PulsarClientHolder+sendAsync 降级,每个 topic 一 Producer 按需创建)。
> - **任务1 user.registered(bff-java→learning-java)✅**:bff-java AuthController.register 写用户后发 `user.registered` 事件(EventPublisher.publish),含 userId/email/role;learning-java LearningEventConsumer 监听该 topic,收到后查 knowledge_points 全量 kpId → 调 LearningService.initMastery(幂等:已存在跳过)。补死代码断链(原 initMastery 无调用路径)。
> - **任务2 teacher.finalized(teacher-java→learning-java)✅**:teacher-java TeacherService.finalize 写 final_score 后发 `teacher.finalized` 事件,含 userId/kpId(用 assignmentId 占位,kp 映射待 writing-py 回填)/finalScore;learning-java LearningEventConsumer 监听,收到后 upsert user_mastery(mastery_score=finalScore,幂等)。
> - **任务3 teacher.autograde.requested(teacher-java 内部削峰)✅**:teacher-java TeacherService.autoGrade 改为只发 `teacher.autograde.requested` 事件立即返回;TeacherEventConsumer 监听,收到后同步执行原评阅逻辑(dopteacherService.doAutoGrade 循环评阅+缓存失效)。解耦故障域+削峰(避免长事务阻塞 HTTP 线程)。
> - **编译验证 ✅**:esp-cache install RC=0 + bff-java/learning-java/teacher-java compile RC=0。唯一修复:doAutoGrade 方法权限改为 public(原包访问权限 consumer 无法调)。
> - **设计模式**:
>   - EventPublisher 与 CacheBroadcast 同模式(PulsarClientHolder 延迟初始化+sendAsync 降级,broker 不可用时 producer 创建/发送均静默降级)。
>   - Consumer 启动走 CompletableFuture 异步后台线程,失败不阻断应用启动(esp.pulsar.consumer-enabled=false 可关)。
>   - 幂等:initMastery 前过滤已存在;user_mastery upsert(同主键覆盖)。
> - **py/go 侧 2 事件 ✅ 完成(本轮收尾)**:
>   - **任务4 writing.graded(writing-py→teacher-java)✅**:`services/writing-py/app/pulsar_producer.py`(延迟初始化+send_async 降级,PULSAR_ENABLED=false 默认 no-op);修 URL bug(缺 pulsar:// 前缀);main.py grade 端点批改完成后 publish_writing_graded;pyproject 加 pulsar-client>=3.5.0。teacher-java TeacherEventConsumer 新增 writing.graded 监听(订阅 esp-teacher-writing-graded,记录统计;TODO:user_id 透传后回写 mastery)。
>   - **任务5 session.ended(gateway-go→learning-java)✅**:`services/gateway-go/src/pulsar/producer.go`(延迟初始化+sendAsync 降级)。**build-tag feature-gate**:网络受限无法 go get pulsar-client-go,故 producer.go 加 `//go:build pulsar_eda`,配套 `producer_stub.go`(`!pulsar_eda` no-op)——默认 `go build` RC=0,生产 `go build -tags pulsar_eda` 启用真实 Pulsar。manager.go End 接 session.ended。learning LearningEventConsumer 新增 session.ended 监听 → reportEvent(会话结束入 Disruptor 驱动复习)。esp-cache 新增 SessionEndedEvent/WritingGradedEvent POJO(@JsonProperty snake_case + @JsonIgnoreProperties 容错)。
>   - **编译验证 ✅**:esp-cache install RC=0 + learning/teacher/bff compile RC=0;gateway-go build + test(dialog/llm/scenario/session/ws)全 ok;writing-py py_compile RC=0。
> - **EDA 总结**:5 个真实业务 topic 全部 producer+consumer 闭环;默认全降级(PULSAR_ENABLED=false / pulsar_eda tag off)不连 broker 不影响业务;启用即异步事件,失败只 log 不阻断。Disruptor 仍管 learning_events 高频落库(最优,不引网络跳数)。
>
> **P0 安全重构 ✅ 完成(本轮)**:
> - **WS 鉴权 ✅**:gateway-go `userIDFromContext` 硬编码 return 1(会话劫持高危)→ `parseUserID`(HS256 JWT 解析,s.jwtSecret 与 bff 共享)+ `userIDFromRequest`(Authorization: Bearer)。createSession 无/非法 token 返 401;handleSpeech upgrade 前 `?token=` 校验 token userID==会话归属 UserID(防越权连他人会话),不符返 403。JWT_SECRET 默认对齐 bff(esp-dev-secret-change-me)。build+test 通过。
> - **PDB ✅**:新建 `deploy/k8s/41-pdb.yaml`,6 服务各 minAvailable:1,selector app 与现有 deployment labels 全匹配。
> - **NetworkPolicy ✅**:新建 `deploy/k8s/40-networkpolicy.yaml`,default-deny ingress + 允许 esp-prod 内部互通 + gateway NodePort 入站(命名空间级隔离;单节点 CNI 不强制,多节点 Calico/Cilium 生效)。
> - **JWT HS256→RS256 → 降 P1**:HS256≥256bit 已非阻断(弱密钥坑已修);RS256 是生产增强(RSA 密钥对 + 跨语言 Java/Go 公钥分发),暂留 HS256 标 P1。
>
> **本轮新增 P1 待办(测试前非阻断)**:① langgraph 0.2→1.x 代码迁移(agent.py);② JWT RS256;③ writing.graded 缺 user_id(需 BFF X-User-Id 透传才能驱动 teacher mastery);④ 有状态中间件大版本迁移(apply 前准备):Nacos 2→3 / Redis 8.0→8.8 / Seata;⑤ 可观测栈镜像升级(Pulsar→4.x + OTel/Loki/Grafana/Tempo/Fluent Bit/Prometheus,未在 deploy/k8s 裸 yaml 待确认);⑥ bff-java sentinel adapter 显式版本删除让 SCA BOM 管理。
>
> **任务清单**:TaskList #1✅#2✅#3✅(Java7/7 Go 前端 Python镜像 基本完成)#4✅(EDA 5 topic 闭环 + P0 安全)#5✅(已落盘)。
> **跨会话恢复**:Redis `esp:ctx:upgrade-status`(JSON 快照)+ `esp:ctx:session:last_update` + 本节三处对齐。
> - **文件清单**:
>   - esp-cache:`services/esp-cache/src/main/java/com/esp/cache/event/EventTopics.java`/`UserRegisteredEvent.java`/`TeacherFinalizedEvent.java`/`AutoGradeRequestedEvent.java`/`EventPublisher.java`
>   - learning-java:`services/learning-java/src/main/java/com/esp/learning/event/LearningEventConsumer.java`
>   - teacher-java:`services/teacher-java/src/main/java/com/esp/teacher/event/TeacherEventConsumer.java`(新建)/`TeacherService.java`(改 autoGrade/finalize)
>   - bff-java:`services/bff-java/src/main/java/com/esp/bff/handler/AuthController.java`(改 register)
>
> **P0 安全重构 ⏸ 待启动**:WS 鉴权落地 / JWT HS256→RS256 / 补 PDB+NetworkPolicy。
>
> **任务清单**:TaskList #1✅#2✅#3🔄(Java7/7✅ Go✅ 前端✅ Python🔄 镜像🔄)#4🔄(EDA落地中 P0安全待)#5⏸。
> **跨会话恢复**:Redis `esp:ctx:upgrade-status`(JSON 快照)+ `esp:ctx:session:last_update` + 本节三处对齐。
>
> 更新日期:2026-08-01(中间件/框架/CVE 版本基线调研)
> ## 中间件/框架/运行时版本基线调研(2026-08-01)
> **任务**:联网调研 ESP 技术栈所有关键组件最新稳定版(截至 2026-07/08)+ 已知重要 CVE,作为升级与安全基线。
> **执行情况**:通过 gh API + pypi/maven 元数据 + WebSearch 完成。WebFetch 受网络策略限制不可用,改用 gh API releases/tags 直查。
> **关键结论(详见下方清单)**:
> - JDK LTS:21 仍为生产 LTS(Temurin 21.0.9+10),25 已 GA(Temurin 25.0.4+7)但非 LTS;建议 ESP 维持 JDK 21。
> - Go 1.25.x(最新 1.25.12);Python 3.14.6(3.15 仍 beta,勿用)。
> - Spring Boot 3.5.16(4.x 已 GA 但破坏性变更,维持 3.5.x);Spring Cloud 2025.1.2;Spring Cloud Alibaba 2025.1.0.0。
> - Dubbo 3.2.20;Seata 2.6.0;Sentinel 1.8.10;Nacos 3.2.3(Nacos 3.x 架构升级,注意兼容)。
> - TiDB 8.5.7;Redis 8.10.0(注意:Redis 7.x 仍是常见稳定线,8.x 已为主推);Pulsar 4.2.3(3.x 已过时,4.x 主线)。
> - LangGraph 1.2.10;anthropic-python 0.120.2;Litestar 2.24.0;redis-py 8.1.0。
> - React 19.2.8;Vite 8.2.0;antd 6.5.3;Taro 4.2.1。
> **CVE**:Spring 2025 有 CVE-2024-38816(Spring Webflux path traversal,已修);Dubbo 历史 CVE-2023-45122,3.2.x 已修;Redis 7.4/8.0 历史 CVE-2024-46981(已修);Go 1.23 CVE-2025-22869(net/http),1.25 已修;Log4j2 后续 CVE-2025-31651(已修,2.24.3+)。无新增紧急未修复项。
> **现状**:调研完成,数据已存 Redis(key=esp:ctx:version-baseline:2026-08-01)。可作为依赖升级/安全审计基线。
> **下一步**:① 若决定升级,优先 Nacos 2.x→3.x(破坏性)与 Dubbo/Seata 对齐版本矩阵;② TiDB 8.5.7 与 Redis 8.x 升级前在测试环境验证;③ 前端 React 18→19 / antd 5→6 需联调。
> - 落盘:本节。

> 更新日期:2026-08-01(日志系统架构重构 + 用户行为分析推荐)
> ## 日志系统架构重构 + 用户行为分析与推荐(2026-08-01)
> **用户问题**:所有服务/应用的日志系统和日志分析功能完善了吗?日志分析能否用 TiDB/TiKV?现有日志系统收集前端用户行为日志吗?请重构实现用户行为分析与相关推荐。
> **架构决策(双轨)**:
> 1. 原始容器日志(stdout/stderr)→ Fluent Bit DaemonSet → Loki(仅索引 esp_app/esp_ns 标签,7d 保留)—— 运维排障。
> 2. 结构化业务日志(学情/写作批改/教师操作/审计/鉴权/用户行为)→ 各服务 AppLogClient 异步写 TiDB `app_logs` 表 —— SQL 可聚合分析。
> 3. **TiDB 不存原始日志**(高吞吐追加 + Raft 写放大不合适),只存结构化日志;TiKV 是 TiDB 存储引擎不独立暴露,日志经 TiDB SQL 接口即可。
> **Part A 架构文档 ✅**:`docs/design/日志系统架构.md`(双轨数据流图、选型理由、典型 SQL、降级策略)。
> **Part B app_logs 表 ✅(已建表)**:`docs/design/schema.sql` 追加 app_logs 表(id/ts/service/level/event/trace_id/user_id/payload JSON + 4 索引)。**已通过 scripts/db_query.sh 在 TiDB tidb-primary 执行 CREATE TABLE,SHOW TABLES 确认 app_logs 表已建成**(esp 库现 13 表)。
> **Part C 各服务结构化日志写入 AppLogClient ✅(代码就绪,未 build 部署)**:
> - Java(learning/teacher/bff):新建 `com/esp/{learning,teacher,bff}/common/AppLogClient.java`,异步 ThreadPoolExecutor(队列 4096,满则丢弃保业务)+ 降级 warn 不阻断 + OTel span traceId 提取。learning-java ServiceConfig 补 JdbcTemplate Bean。
> - Python(writing/emotion):新建 `app/logging_client.py`,后台线程 + queue.Queue(4096)+ pymysql 直连 tidb-primary + 降级。
> - Go(gateway):新建 `src/log/client.go`,后台 goroutine + chan(4096)+ database/sql + 降级。
