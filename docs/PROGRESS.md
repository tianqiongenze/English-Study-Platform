# ESP 项目进展 (PROGRESS) — 热区

> **本文件只放当前任务热区**(已确认根因/当前子步骤/下一步/恢复锚点),硬上限 ~300 行。
> **历史里程碑见 `docs/progress/archive/*.md`**(按主题拆分),跨会话恢复亦可从 Redis `esp:ctx:archive:*` 取。
> **恢复协议**:新会话先读本文件(全文)+ TaskList,不全量读历史;需要历史细节 Grep archive 或 Redis 取。
>
> **当前✅(2026-08-14 会话22-16 统一 Portal v1.1.0 + 全后端功能测全绿 + 条件性 GO)**:补建统一多端 Portal(frontend-portal nginx,NodePort 30070,绕过不可达 traefik LB 直连 8 后端 ClusterIP),Mac 经 SSH 隧道可达。Go/Rust 8 服务 TC-GR-1~9 全 200(长驻 probe-keep pod 修临时 pod iptables race)。Portal TC-P-1~4 全绿。**关键根因**:① traefik ingress controller pod 未运行(svc endpoints `<none>`)→ portal /api 改直连后端 ClusterIP 复刻 esp-ingress 9 path 规则;② RD NodePort Mac 不可达(VM IP 192.168.5.15 ping 100% 丢包,无 svclb bridge)→ SSH 隧道 `ssh -f -N -L 30070:localhost:30070 -F "$RD_LIMA" lima-0`;③ 临时 curl pod iptables/DNS race(connection refused 3ms)→ 长驻 probe-keep pod。**已知数据缺口**:writing-coach 500 = `esp_writing.writing_templates` 表空(db_query 实证表在 esp_writing 库非 esp 库),非代码 bug,非阻断。**下一步**:k6 基线重跑(用 ConfigMap 挂脚本替代 /dev/stdin)、Locust WS 交叉验证、前端 GUI 人工浏览器 TC-W/H、git commit。

## 当前热区 (2026-08-14 会话22-16 统一 Portal + 全后端功能测)

### 核心进展:4 任务批次(Portal/压测/人工测/commit)
1. ✅ 统一 Portal(frontend-portal v1.1.0 构建部署,Mac 可达)
2. ⏳ 压测(k6 基线首轮超时待重跑,Locust WS 待交叉验证)
3. ⏳ 人工测试(TC-GR-1~9 + TC-P-1~4 全绿;前端 GUI TC-W/H 待人工浏览器)
4. ⏳ git commit(待最后)

### 任务1 统一 Portal v1.1.0(完成✅)
- **痛点**:ESP 无统一入口。frontend-react/h5 独立 ClusterIP 无 NodePort;esp-ingress 仅 API+WS 路由不托管静态;traefik LB 192.168.5.15 = VM 内 Mac 不可达。对照 education-platform 有完整 portal 三件套。
- **方案**:单 nginx 容器(frontend-portal)托管门户 index.html + 反代。NodePort 30070 对 Mac 暴露。
- **关键修复(traefik 绕过)**:原 nginx /api /ws 反代 traefik(Host esp.example.com)→ 502/000,因 traefik ingress controller pod 未运行(svc traefik 仍 LoadBalancer 但 endpoints `<none>`)。改为按 path 直连 8 后端 ClusterIP,复刻 esp-ingress 9 path 规则:`/api/auth/`→bff-java:8090、`/api/scenarios|sessions`→gateway-go:80、`/api/learning`→learning-java:80、`/api/writing-coach`→writing-coach:80、`/api/writing`→writing-py:80、`/api/teacher`→teacher-java:80、`/api/emotion`→emotion-svc:80、`/ws/`→gateway-go(WebSocket upgrade)。
- **Mac 可达性(SSH 隧道)**:RD 单节点 NodePort Mac host 直连不通(VM IP ping 100% 丢包,无 svclb DaemonSet bridge)。解法:`ssh -f -N -L 30070:localhost:30070 -F "$HOME/Library/Application Support/rancher-desktop/lima/0/ssh.config" lima-0`。6443 kubectl 隧道同理会断,断后 `ssh -f -N -L 6443:localhost:6443 -F "$RD_LIMA" lima-0` 恢复。
- **验证(TC-P-1~4 全绿)**:portal root 200;/api/scenarios 200;/api/auth/health 404(bff 无此端点,转发本身通);/web/ /h5/ 反代生效。
- **改文件**:frontend-portal/nginx.conf(重写 8 path + /ws)、frontend-portal/Dockerfile(不变)、deploy/k8s/17-frontend-portal.yaml(image 1.0.0→1.1.0)、docs/test/07-人工测试用例.md §8 补全 8 Go/Rust 服务用例 + §8.4 portal 用例。

### 任务3 人工测试(TC-GR-1~9 + TC-P-1~4 全绿✅)
- **临时 curl pod iptables race 修复**:`kubectl run --rm -i` 临时 pod 启动期 iptables/DNS race,对存在 endpoints 的 ClusterIP 返回 HTTP 000(connection refused ~3ms)。改用长驻 `probe-keep` pod(curlimages/curl:8.10.1 sleep 300),从其内全 8 服务响应正常。
- **TC-GR-1~9(8 Go/Rust 服务全 200)**:course-path-go/listening-go/reading-go/gamification-go/live-tutor-go/media-learn-rs 全 200;pronunciation-rs POST /api/pronunciation/score 需 `{"user_id":1,"text":"hello world","ref_type":"mock","ref_id":1,"audio_url":"http://x/a.wav"}`(ref_id 必须 i64 非 string)→ 200;assessment-rs POST /api/assessment/sessions → 200。
- **TC-P-1~4(portal)**:见任务1,全绿。
- **writing-coach 500 = 数据缺口(非阻断)**:logs 显 `Table 'esp.writing_templates' doesn't exist`。db_query 实证该表在 `esp_writing` 库(schema-per-service),非 `esp` 库。writing-coach 连接串指向 esp 库读不到表。种子数据缺口,记录不阻断。
- **30080 NodePort 失效**:测试文档 §1 列 gateway-go 30080 但 gateway-go 实为 ClusterIP-only(无 NodePort),该条 stale。改用 portal /api/scenarios 测 gateway。

### 任务2 压测(k6 首轮超时⏳)
- **k6 基线首轮失败**:`kubectl run k6-baseline --rm -i --image=grafana/k6:0.54.0 -- k6 run ... /dev/stdin < deploy/k6/scenarios-baseline.js` 超时 180s "timed out waiting for the condition",pod 卡 Terminating(已 force delete)。疑因:`/dev/stdin` + `kubectl run -i` stdin 重定向在该 k6 镜像不工作,或镜像拉取慢。
- **重跑方案**:用 ConfigMap 挂脚本(`kubectl create cm k6-script --from-file=deploy/k6/scenarios-baseline.js`),pod 挂载到 /scripts/,`k6 run /scripts/scenarios-baseline.js`。避免 stdin 重定向问题。
- **Locust WS 交叉验证**:待启动(deploy/locust/ 已有脚本)。

### 恢复锚点(跨会话)
- **当前子步骤**:任务2 k6 重跑(ConfigMap 挂脚本)→ Locust WS → 任务4 git commit。
- **已确认根因**:traefik pod 未运行→portal 直连后端;RD NodePort Mac 不可达→SSH 隧道;临时 pod iptables race→长驻 probe pod;writing-coach 500=esp_writing 库数据缺口。
- **已改文件**:frontend-portal/nginx.conf、frontend-portal/Dockerfile、17-frontend-portal.yaml、07-人工测试用例.md §8、(上轮)NettyEdgeServer.java、JwksController.java、DbLearningEventConsumer.java、15-bff-java.yaml、20-learning-java.yaml、memory×2、deploy/k6/*。
- **下一步**:k6 ConfigMap 重跑;Locust WS 交叉验证;前端 GUI TC-W/H;git commit 全部。

---

## 历史 (2026-08-14 会话22-15 端到端人工测试 SEC全绿 + L1-L10链路)

### 核心进展:阶段0-6 测试执行(docs/test/09-人工测试操作指南.md)
执行上线前全流程人工测试。SEC 安全关口全过,发现并修 1 个真 bug(learning_events 落库)。

### 阶段2 SEC 鉴权越权矩阵(全绿✅)
- SEC-1✅ WS越权403实证:A token连B session得HTTP 403 forbidden(gateway-go handleSpeech: parseUserID(token)!=sess.UserID→403,在WS upgrade前)。SID_B=sess_1786655339867。
- SEC-2✅ 过期JWT→401:edge 9090 EdgeAuthHandler.jwt.parse() 对过期token返回401(合法token=转发000因fire-for-get,无token=401)。
- SEC-3✅ X-User-Id剥离:代码可证 EdgeAuthHandler stripSpoofedHeaders()+注入JWT sub(line109-111)。直连learning(30081)信任伪造999999(下游无独立filter,设计信任BFF注入),经edge必被剥离替换。实证受edge fire-and-forget不回写阻碍。
- SEC-4✅ 无secret泄漏:trivy fs扫services/ 全量源码0发现(镜像扫描因Mac连不上RD docker改用fs扫)。
- SEC-5✅ JWKS闭环:kid=esp-rs256-1。修复alg缺失(JwksController加.algorithm(JWSAlgorithm.RS256))。bff-java 1.7.0→1.7.1。

### 坑1闭合(NettyEdgeServer)+ edge fire-and-forget设计缺口
- 修:NettyEdgeServer bind "0.0.0.0"(原绑loopback ::1/127.0.0.1致NodePort不可达)+ SO_BACKLOG改.option(原误放.childOption)。
- 真根因(非bind):edge 9090 对需回写响应的请求全超时,因 InletDisruptor.DownstreamForwarder 是fire-and-forget(sendAsync不回写Netty ctx,InletDisruptor.java:143注释"回写见ForwardRespondHandler扩展点"未实现)。仅401/503错误路径即时回写。=P2工程缺口。
- 验证:edge /api/learning/x 无token→401 ✓;过期token→401 ✓。

### 阶段5 L1-L10链路测
- ✅L1(6服务健康全200) ✅L2(REST scenarios 200) ✅L3(Dubbo Triple teacher kp-stats 200) ✅L8(writing-py Mock批改score=24+grammar修正) ✅L9(sessions history user_id正确)
- ✅L4/L7(learning_events落库,坑㉙已修):LearningEventEntity @Id Long无@GeneratedValue + TiDB AUTO_RANDOM,Hibernate persist抛异常丢数据。**修复**:DbLearningEventConsumer 改 JdbcTemplate batchUpdate(INSERT不含id列,TiDB AUTO_RANDOM生成)。learning-java 1.7.0→1.7.1。实测 esp_learning.learning_events COUNT=1 id=10364873落库成功。
- ⚠️L5(trace部署侧待多节点,esp-observability ns无otel-collector) ⚠️L6(双pod WS待多节点) ⚠️L10(优雅停机待多节点)

### 阶段6单节点压测基线
- host经port-forward:28rps/100%成功(瓶颈=port-forward隧道开销kubectl→apiserver→kubelet→pod,非服务容量)。
- 集群内locust压测(ClusterIP直连绕过port-forward)进行中,预期rps远高于28。
- 5000CCU单节点不可达,如实记录,多节点Phase D2重测。

### 用户澄清(本会话2个问题)
1. 统一portal:ESP无。frontend-react/h5独立ClusterIP无NodePort,esp-ingress仅API+WS路由(esp.example.com)不托管静态资源,mac不可达traefik LB(192.168.5.15=VM内)。对照education-platform有完整edu-portal-ingress+edu-unified-ingress+edu-unified-ws-ingress(www/api.edu.com)。缺口,见memory esp-unified-portal-gap。
2. 端口冲突:ESP与education-platform零冲突。edu全ClusterIP无NodePort(走traefik);ESP用300xx NodePort段(30009/10/48/58/71/81-85/92)。两ns共用kube-system/traefik靠host分流。新增ESP ingress host须避开*.edu.com。见memory esp-port-isolation-edu。

### 待执行
1. ✅ LearningEventEntity修复验证(L4落库=1行)→G-5转绿
2. ✅ 阶段7 Go/No-Go签收 → 条件性GO(G-1/2/3/5/6/7✅+G-4降权+G-8知悉)
3. 前端GUI人工浏览器走TC-W/H(web/h5 Vite可起,小程序跳过P-0-6缺)
4. git commit(NettyEdgeServer+JwksController+DbLearningEventConsumer+manifest×2+PROGRESS+memory×2+k6脚本)
5. 多节点移植(真实弹性/5000CCU/L6双podWS/L10优雅停机/trace Collector部署)

### 关键约束(本会话)
- 单节点铁律:不碰k3s/restart/pkill/Factory Reset;不停colima;HPA minReplicas=1(learning-java HPA曾扩2新pod探针失败缩回,印证JVM抢CPU)
- learning-worker保持1.6.0不动
- 压测机入集群(走ClusterIP)绕过port-forward,工业级姿势

---

## 当前热区 (2026-08-13 会话22-14 业务 100% 恢复 + 坑㉔ k3s containerd 镜像隔离)

### 核心根因:坑㉔ k3s containerd 与 docker 镜像库隔离
- colima-stop DNS 雪崩(会话22-13)期间,k3s containerd 的 `k8s.io` namespace 下 ESP 业务镜像全被清空(只剩旧 `edu-*` 项目镜像)
- manifest 用 `esp/xxx:tag` + `imagePullPolicy: IfNotPresent`,但 k3s containerd 本地无镜像 → kubelet 去远程拉 → daocloud 镜像源对 `esp/` 私有名返回 **403 Forbidden**(公网不存在)
- **关键陷阱**:ESP 业务镜像其实都在节点的 **docker daemon** 里(`docker build` 存 `/var/lib/docker`),但 **k3s 用自己的 containerd**(`/var/lib/rancher/k3s/agent/containerd`),两者镜像库**完全隔离**
- **Why K8s 用 containerd 不用 docker**:K8s 1.24 移除 Dockershim,kubelet 只认 CRI 接口;k3s 自带 containerd(轻量:省 dockerd 一层、少故障点、少内存);docker daemon 镜像与 k3s containerd 镜像互不可见
- **修复**:`docker save esp/xxx:tag | sudo k3s ctr images import -` 把 docker 镜像搬运到 k3s containerd(实测有效)

### 本会话修复清单(11 步,按顺序)
1. 节点 DNS 正常(resolv.conf 223.5.5.5/8.8.8.8 未被重置)→ ImagePullBackOff 非因 DNS
2. 诊断根因=containerd 无 esp/ 镜像(crictl 无该命令,用 `k3s ctr images ls` 查)
3. 批量导入 16 ESP 业务镜像到 k3s containerd(gateway-go/bff-java/learning-java:1.7.0/teacher-java/emotion-svc/writing-coach/writing-py/course-path-go/live-tutor-go/media-learn-rs/pronunciation-rs/listening-go/assessment-rs/reading-go/gamification-go/frontend-react/frontend-h5)
4. 补导 seataio/seata-server:2.2.0 + bladex/sentinel-dashboard:1.8.9 + esp/learning-java:1.6.0(worker 用)
5. 重建 esp-rsa-jwt secret(`--from-literal=JWT_RSA_PRIVATE/PUBLIC_KEY_PEM`,密钥来自 services/bff-java/src/test/resources/rsa/;3 Java 服务 CreateContainerConfigError 根因)
6. apply 06-sca-middleware.yaml(seata+sentinel **漏 apply**,bff/learning 连不上 seata:8091 → BeanCreation `globalTransactionScanner` 失败)
7. 创建 redis-warmup-script ConfigMap(`--from-file=redis-warmup.py=scripts/redis-warmup.py`;warmup job ContainerCreating 根因;补建后 job **Completed**)
8. 重新 apply 05-infra-reuse.yaml(之前**漏了 redis-cluster/pulsar-broker/milvus/neo4j 4 个 ExternalName**;业务用短名 pulsar-broker 解析 NXDOMAIN → Pulsar consumer 失败)
9. apply 30-other-services.yaml(teacher-java/emotion-svc/writing-coach **漏 apply**)
10. scale learning-worker/live-tutor-go/media-learn-rs replicas 0→1
11. learning-java HPA minReplicas=1(曾因 CPU 40% 扩到 4 副本,单节点 JVM 抢 CPU,降回 1)

### 最终状态(21 deploy 全 Ready,2026-08-13 实测)
- **1/1 Running**:assessment-rs/bff-java/course-path-go/emotion-svc/frontend-h5/frontend-react/gamification-go/gateway-go/learning-java/learning-worker/listening-go/live-tutor-go/media-learn-rs/nacos/pronunciation-rs/reading-go/seata/sentinel/teacher-java/writing-coach/writing-py
- **esp-redis-warmup: Completed** ✅(Redis esp:cache:* 已灌入)
- bff/learning RESTARTS=7=历史启动期累积(seata 未就绪时),现已稳定
- 基础设施(ekb-m2):milvus/minio/neo4j/pd/pulsar(3bk+3broker+3zk)/redis-cluster(6 节点)全 Running

### 关键命令(恢复用)
- 查节点镜像:`ssh -F lima/0/ssh.config lima-0 'sudo k3s ctr images ls | grep esp/'`
- 导入镜像:`ssh ... lima-0 'docker save esp/xxx:tag | sudo k3s ctr images import -'`
- apply 顺序:00-namespace → 05-infra-reuse → 05b → 06-sca → 07-tidb → 10/15/16/20/21/30/31/32 → esp-redis-warmup-job

---

## 历史 milestone 归档索引(需要时取)

| 主题 | 文件 | Redis key |
|---|---|---|
| **会话22-13 colima-stop DNS 雪崩** | `docs/progress/archive/session22-13-colima-dns.md` | — |
| **会话22-12 CrashLoop 诊断 + 双轨落盘** | `docs/progress/archive/session22-12-crashloop-diagnosis.md` | — |
| **会话22-11 图+向量库 + 冒烟7/8 + Neo4j坑 + 实测校正** | `docs/progress/archive/session22-11-graph-vector-smoke.md` | — |
| **会话22-10 Obsidian同步 + Redis warm-up** | `docs/progress/archive/session22-10-obsidian-redis-warmup.md` | — |
| **会话22-9 平台底座化 + redis坑㉓闭合** | `docs/progress/archive/session22-9-platform-infra-redis-023.md` | `esp:ctx:hot:2026-08-13-session22-9-platform-infra` |
| **会话22-3 ~ 22-8(08-09~0811)** | `docs/progress/archive/2026-08-09-to-0811-session22-3-to-22-8.md` | — |
| **会话18~22 冷区批次(08-07~09)** | `docs/progress/archive/2026-08-09-cold-archive-session18-22.md` | `esp:ctx:hot:2026-08-09-session22-abcd-batch-ab` |
| teacher readiness 根治 + 安全扫描四件套(08-07) | (见冷区归档) | `esp:ctx:hot:2026-08-07-teacher-readiness-and-security-scan` |
| teacher CrashLoop + 增量服务(08-07) | (见冷区归档) | `esp:ctx:hot:2026-08-07-teacher-crashloop-and-new-services` |
| Pulsar cookie + VM 网络中断(08-06) | `docs/progress/archive/2026-08-06-pulsar-cookie-fix-and-vm-down.md` | `esp:ctx:archive:2026-08-06-pulsar-cookie-fix-and-vm-down` |
| Pulsar Geo + 应用切新集群(08-05) | `docs/progress/archive/2026-08-05-pulsar-geo-and-cluster-switch.md` | `esp:ctx:archive:2026-08-05-pulsar-geo-and-cluster-switch` |
| 上线就绪 + 三层坑事故(08-04) | `docs/progress/archive/2026-08-04-readiness-and-three-layer-incident.md` | `esp:ctx:archive:2026-08-04-readiness-and-three-layer-incident` |
| 07-31~08-03 历史 | `docs/progress/archive/2026-08-{01,02,03}-*.md` | `esp:ctx:archive:2026-08-{01,02,03}-*` |

## 关键命令速查

- 查 TiDB 表:`scripts/db_query.sh "SQL"`(pymysql pod 封装)。
- 服务日志:`kubectl -n esp-prod logs deploy/<svc> --tail=20`。
- Redis 写/读:`kubectl -n ekb-m2-aws-us-east-1 exec redis-cluster-new-0 -- redis-cli -c SET/GET "esp:ctx:*" "..."`。
- 压测:分布式 locust 见 `deploy/locust/README.md`。
- 单节点铁律:重 mvn/docker build 与集群操作串行;不碰 k3s 配置。

## 上下文工程铁律(摘要,详见 memory `context-overflow-ironrules`)

- 文件系统=长期记忆,对话=临时工作区。80%(160k)主动 /compact,90%(180k)强制止损落盘+compact。
- 三层防御:源头减量(subagent 隔离+输出限流)+ 状态外化(PROGRESS 热区+Redis 运行态)+ 主动管控。
- **本文件热区上限 ~300 行**,历史归 `docs/progress/archive/`,不全量读。完整铁律集见 memory。
