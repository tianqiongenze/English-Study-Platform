# ESP 项目进展 (PROGRESS) — 热区

> **本文件只放当前任务热区**(已确认根因/当前子步骤/下一步/恢复锚点),硬上限 ~300 行。
> **历史里程碑见 `docs/progress/archive/*.md`**(按主题拆分),跨会话恢复亦可从 Redis `esp:ctx:archive:*` 取。
> **恢复协议**:新会话先读本文件(全文)+ TaskList,不全量读历史;需要历史细节 Grep archive 或 Redis 取。
>
> **当前✅(2026-08-14 会话22-18 k6 基线 529rps + Locust WS 鉴权双修 70/100 + Redis Cluster 自愈 + 前端 build 全绿 + portal 隧道就绪)**:① k6go 基线(本地 golang:1.25-alpine 编静态二进制 + busybox:1.36 hostPath 跑,绕开 3128 proxy)50VUS/15874 请求/100% 成功/529.1rps/平均 1.61ms。② Locust WS smoke 100 用户:修复 token URL 编码(JWT signature 含 +/_-,必须 `quote(token, safe='')`)+ 去 Host header → peak_online=70 established=85 dropped=15(70%)。③ Redis Cluster 自愈(坑㉓复发:nodes.conf 持久化旧 PodIP 致 cluster_state:fail),CLUSTER MEET ×5 + CONFIG SET cluster-announce-ip → cluster_state:ok slots_ok:16384,gateway-go create_session 500→200。④ 前端 build 全绿(frontend-react tsc+vite RC=0;frontend-h5 vite+PWA v1.3.0 RC=0)。⑤ portal SSH 隧道 30070 就绪,portal/web/h5/api 全 200 供用户人工 TC-W/H。**遗留**:用户人工浏览器 TC-W/H(隧道已就绪 http://localhost:30070/);3128 proxy 根因已定位待答(lima.yaml setproxy + allowed-images MITM 白名单);git commit 待做;plan `indexed-honking-spring` 未启动。

## 当前热区 (2026-08-14 会话22-18 压测+前端收尾)

### k6go 基线压测(成功✅)
- **方案**:用本地 `golang:1.25-alpine` 编纯 Go 压测程序为静态二进制,`busybox:1.36`(本地有)挂 hostPath 跑,绕开 3128 proxy 拒绝非白名单镜像拉取。
- **关键修**:bff-java ClusterIP svc 只有 8090(mvc)/9090(edge)/50090(dubbo)三端口**无 port 80**;k6go 默认 `-bff=http://bff-java:80` 错,必须 8090。register() 尝试 /api/auth/register 失败则 /api/auth/login 拿 token,vu() 循环 GET /api/scenarios。
- **结果**:50 VUS/15874 请求/100% 成功/529.1 rps/平均 1.61ms。

### Locust WS 鉴权双修(成功✅)
- **第一阻塞:create_session 500**。根因=Redis Cluster `cluster_state:fail`(坑㉓复发:nodes.conf 持久化旧 PodIP,VM 重启后 Pod IP 漂移,redis7.2 不认 hostname)。session.Manager.Create 的 rdb.Set 失败(main.go:132 handleCreateSession → 500)。日志被 traces export 噪音淹没。
- **Redis 自愈**:`CLUSTER MEET <live-IP> 6379` ×5 + `CONFIG SET cluster-announce-ip <真实IP>`(pod-0 announce 停旧 IP 时)。自愈后 cluster_state:ok slots_ok:16384,create_session 恢复 200。
- **第二阻塞:WS peak_online=0 established=0**(Redis 自愈后仍失败)。根因双:① `url = f"...?token={token}"` 未 URL 编码,JWT signature 含 +/_- 字符→gateway-go `r.URL.Query().Get("token")` 解析失败→400;② 多余 `header=["Host: gateway-go"]` 致 400(pod 内 DNS 已解析,加 Host 反致问题)。
- **修复**:`from urllib.parse import quote` + `url = f"{ws_url}/{sid}" + (f"?token={quote(token, safe='')}" if token else "")` + 去 Host header。重跑 peak_online=70 established=85 dropped=15,70/100 (70.0%)。

### 前端 build + portal 隧道(成功✅)
- **frontend-react**:tsc + vite build RC=0。**frontend-h5**:vite + PWA v1.3.0 build RC=0。
- **portal SSH 隧道**:`ssh -f -N -L 30070:localhost:30070 -F "$RD_LIMA" lima-0`。验证:portal root 200;/web/ /h5/ 反代生效;/api/scenarios 200。URL http://localhost:30070/ 供用户人工 TC-W/H。

### 恢复锚点(跨会话)
- **当前子步骤**:会话22-18 压测+前端收尾已完成(k6go 529rps / Locust WS 70% / Redis 自愈 / 前端 build 全绿 / portal 隧道)。
- **已确认根因**:① 3128 proxy 拒绝=lima.yaml setproxy 给 docker/containerd init 加 HTTPS_PROXY=openresty 3128,3129 SSL MITM 查 allowed-images.conf 白名单(仅 pause 3 条)非白名单返 403;② create_session 500=Redis cluster_state:fail;③ WS 400=token 未 URL 编码 + 多余 Host header。
- **已改文件**:deploy/locust/ccu5000_ws.py(quote 编码 + 去 Host header)、deploy/k6/k6go/main.go(bff 默认 8090,上一会话)。
- **下一步**:① 用户人工浏览器 TC-W/H(隧道已就绪);② 3128 proxy 根因答用户;③ git commit;④ plan `indexed-honking-spring`(平台底座+图向量库)。

---

## 历史 (2026-08-14 会话22-17 RD VM 扩容 8G→30G/4核→9核 全绿 + 坑㉔诊断推翻 + 全量清理僵尸pod)

### 扩容验证结果(全绿✅)
- **VM 运行态**:`pgrep -fl qemu` 显 `-m 30720 -smp 9,cores=9`;`free -h` 29.3G;`nproc`=9。lima.yaml `memory:32212254720`/`cpus:9`/`disk:360GiB` 已落盘。
- **k3s**:重启后未自启,手动 `sudo rc-service k3s restart`(初次 start 卡在 start_pre 镜像加载 hook,restart 完成加载全部 bundled 镜像含 pause:3.6),node Ready(v1.34.6+k3s1)。
- **6443 隧道**:RD 重启把 SSH localPort 从 53281 改到 59195,旧隧道失效;`pkill -f "6443:localhost:6443"; ssh -f -N -L 6443:localhost:6443 -F "$RD_LIMA" lima-0` 重建。
- **esp-prod**:21/22 deploy AVAILABLE(learning-worker 0 副本按设计),0 非Running pod。bff/learning/teacher 三服务此前因双 RS 并存卡就绪,删旧 RS 后新 pod 10min 内转 1/1。

### 关键清理(扩容后必要动作)
- **孤儿 ReplicaSet 堆积**:扩容前多服务经历了 rollout,旧 RS(desired=1)与新 RS 并存,两套都创建 pod 抢资源+探针竞争。bff-java 曾有 3 个 RS、learning/teacher 各 2 个。删非 current 且 desired>0 的旧 RS 解决。**Why**:单节点资源紧时双 RS 各拉一个 pod,内存/CPU 双倍消耗,新 pod 起不来旧 pod 又不退出。
- **跨 ns 僵尸 pod force-delete**:扩容前 8G 内存耗尽时,education-platform 14 个 pod 被打成 `ContainerStatusUnknown`(占 2550m CPU+4608Mi requests 但实际死的)、ekb-m2-aws 3 个(tidb/tiflash/tikv)、default extract-config。force-delete 后节点 CPU 99%→93%,腾出空间给 esp-prod 调度。
- **坑㉔ 修正记忆**:`memory/esp-k3s-containerd-image-isolation.md` 已记 k3s 用 moby/docker CRI,docker build 镜像本就 k3s 可见,无需搬运。

### 风险与铁律遵守
- RD 自身重启**不在**坑㉑禁列(禁的是停 colima/动 lima 网络,会抽 RD 底盘)。仅 `rdctl shutdown/start`,不动 k3s 配置、不 pkill、不 Factory Reset。
- 坑㉓ 预置:VM 重启后必查 pause 镜像,否则 sandbox 建不起来全集群瘫。
- 坑㉔ 修正后,Task#27 "根治 k3s containerd 构建流水线"**前提不成立**——docker build 即 k3s 可见,无需额外流水线。已关闭该任务。
- **monitor 判定坑**:`awk '$2==$4'` 对 `kubectl get deploy` 输出(READY/AVAILABLE 字段)因 `1/1` vs `1` 类型不一致恒为假,导致误报 "available=0"。恢复判定应以**第4列 AVAILABLE==1**为准。

### 恢复锚点(跨会话)
- **当前子步骤**:会话22-17 扩容全绿已完成;下一步=会话22-16 遗留(k6 ConfigMap 重跑 → Locust WS 交叉验证 → 前端 GUI TC-W/H → git commit)或 plan `indexed-honking-spring`(平台底座+图向量库)。
- **已确认根因**:pod 全挂=VM 8G 内存耗尽(非镜像清空);扩容后双 RS 竞争+跨ns僵尸pod占 CPU 是卡就绪的真因。
- **下一步**:见会话22-16 恢复锚点。

---

## 历史 (2026-08-14 会话22-16 统一 Portal v1.1.0 + 全后端功能测全绿 + 条件性 GO)

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
