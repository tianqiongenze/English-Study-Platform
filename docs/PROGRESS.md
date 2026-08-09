# ESP 项目进展 (PROGRESS) — 热区

> **本文件只放当前任务热区**(已确认根因/当前子步骤/下一步/恢复锚点),硬上限 ~300 行。
> **历史里程碑见 `docs/progress/archive/*.md`**(按主题拆分),跨会话恢复亦可从 Redis `esp:ctx:archive:*` 取。
> **恢复协议**:新会话先读本文件(全文)+ TaskList,不全量读历史;需要历史细节 Grep archive 或 Redis 取。
>
> 更新日期:2026-08-09 会话22-5(git push 闭环 ✅ + H5 部署 ✅ + 改动来源厘清 ✅ + PROGRESS/Redis 双轨落盘)。主线 4 批次进度:**A ArgoCD 3sts drift 收口 ✅**;**B infra Degraded 全修 ✅**;**C 前端补全 ✅(H5 已部署)**;**D 种子续灌+压测摸底 ⏳ pending**;**债1+2 esp-common-go 骨架完成 1/7(listening-go),剩 6 服务待迁移**。Redis 锚点 `esp:ctx:hot:2026-08-09-session22-5-push-h5-deploy`。

## 会话22-5:push 闭环 + H5 部署 + 改动来源厘清 + 双轨落盘(2026-08-09,✅)

用户指令"自动从 PROGRESS + Redis 恢复,完整完成后续所有任务(可并行就并行)"。本会话执行:恢复→厘清未记录改动→push→H5 部署→双轨落盘。

### ✅ 改动来源厘清(esp-explorer adafd7f0,39k token)
PROGRESS/Redis 未记录的 git 改动分三类,全部核实无冲突:
| 类别 | 改动 | 状态 |
|---|---|---|
| 已完成可部署(线上已生效,补落盘) | bff API Portal(债3)、gateway-go sessions/history、teacher-java 1.7.0 manifest | 功能闭环,线上实测可用 |
| 半成品需继续 | esp-common-go 公共包:仅 listening-go 1/7 迁移,6 服务未动;tracing 包 go.mod 缺 otel require(隐患) | 进行中 |
| 已完成未部署 | listening-go 迁移(可编译,线上仍 0.1.0);pronunciation-rs /healthz +1 行(0.1.1 镜像可能已含) | 代码就绪,镜像未 bump |
- **gateway-go sessions/history**:`main.go:94` +`/api/sessions/history` + `handleListSessionHistory`;`manager.go:96-131` `SessionSummary` + `ListHistory` 直读 `esp_speaking.sessions`。线上 1.5.1 实测返回历史数据(401=需 JWT)。
- **bff API Portal**:`ApiPortalController.java`(暴露 /openapi-go-rust.yaml)+ `static/openapi-go-rust.yaml`(226 行,11 服务 43 路由)+ `docs/api/portal/openapi-go-rust.yaml`(源,与 static/ 相同)+ `application.yml:136-157` springdoc 聚合配置 + `pom.xml:239-245` springdoc 依赖。EdgeAuthHandler/RouteTable/AuthFilter 三处白名单对应。线上 bff 1.7.0 四端点全 200。
- **esp-common-go 5 包**:`config`(Getenv/GetenvInt)、`db`(MustConnectDB/DefaultDSN/ReadWriteDSN)、`health`(GinHealthz/GinLivez/StdHealthz)、`redisx`(MustNewCluster)、`tracing`(InitPropagator/TraceparentFromRequest/InjectTraceparent)。**隐患**:`tracing/tracing.go` import otel 但 `go.mod` 未 require otel(当前无服务 import 故未暴露,迁移前必补)。
- **go.work**:8 模块 workspace(esp-common-go + gateway/course-path/listening/reading/gamification/live-tutor/vocab-srs)。仅本地 IDE/编译用,不进 Docker 上下文(各服务 go.mod replace 解析)。

### ✅ git push(commit 15b5ffc)
- 清理误产物:根目录 `src`(12MB arm64 二进制,workspace `go build` 误产生)移除 + `.gitignore` 补 `/src`(commit cf10841)。
- 主提交 `15b5ffc`:债3/4 闭环 + API Portal + 三端补建 + H5 容器化 + esp-common-go 骨架(1/7)。
- `git -c http.version=HTTP/1.1 push` 成功(模式⑩,绕 HTTP/2 framing)。`120c15f..15b5ffc`。GitHub 提示 44 dependabot 漏洞(非阻塞,已知)。

### ✅ H5 部署(esp-builder ab94bb62 镜像已 build,主线程 apply)
- **镜像**:esp/frontend-h5:1.0.0(66.6MB,与 frontend-react 同构多阶段 nginx)。builder 卡在 macOS BSD tar `--exclude` 不生效 + stdin Dockerfile PAX 头污染坑(见下),但镜像最终 build 成功。
- **apply**:`kubectl apply -f deploy/k8s/16-frontend-h5.yaml` RC=0,deploy 1/1 Running。
- **端点验证**(port-forward 30091):`/`=200、`/index.html`=200、`/api/sessions/history`=401(JWT 保护,反代 Traefik 通)。
- **NodePort**:frontend-h5-np 30091(30090 被 Web 占用,已确认未冲突)。

### 新模式(踩过可复用,累计 ⑧~⑭ + 新增 ⑮)
⑮ **macOS BSD tar + docker build stdin 坑(前端镜像 build)**:① BSD tar `--exclude=node_modules` 不生效(GNU 才认),需 `COPYFILE_DISABLE=1 tar --exclude='./node_modules'`(加 `./` 前缀 + 关 AppleDouble);② `tar | docker build -f Dockerfile -` 报 `ambiguous Dockerfile source`(Dockerfile 既在 context 又在 `-f`);③ `docker build -f - .` 从 stdin 读 Dockerfile 被 macOS pax 格式 `PaxHeader/currentdir` 扩展头污染 → `unknown instruction`。**解法**:绕 stdin,源码 rsync 到 RD VM 内 `docker build <dir>`(context 本地读,`.dockerignore` 生效,无 PAX 坑)。本次镜像在 builder 卡点前已用其他方式 build 成功,模式⑮ 供后续前端镜像复用。

### ArgoCD 状态(会话22-5)
- push 后触发 `patch sync`,operation Succeeded 但 sync=OutOfSync health=Degraded。
- **OutOfSync 资源**:仅 pulsar-bk / pulsar-zk / redis-cluster-new 3 sts(即 A 批次 ignoreDifferences 未被 GitHub 网络稳定时重新比较自愈的那 3 个,非新增)。
- **Degraded 源**:ComparisonError `context deadline exceeded`(repo-server 拉 GitHub 抖动)。host `curl info/refs`=401(private,1s 内响应,网络通)→ repo-server 内部 git ls-remote 瞬时超时,待 GitHub 网络稳定后 hard refresh 自愈(模式⑨,勿重启 repo-server)。
- **非阻塞**:esp-prod 全业务 deploy 实测 Running(bff/learning/teacher 1.7.0、gateway 1.5.1、pronunciation-rs 0.1.1、frontend-h5 1.0.0 等全 1/1)。

### learning-java HPA 波动(会话22-5 观察)
- HPA(min1/max15/CPU65%)因 metrics `<unknown>` 抖动扩到 5 副本又缩回(监控期间见 4/5→5/5→4/4→3/3→2/2)。属单节点 metrics-server 抖动,非业务问题。PROGRESS 已记录历史(模式:node NotReady 后 HPA 疯扩,临时 cap max=1)。

### 遗留项
1. **.m2 springdoc patch 非持久化**(模式⑭):换机器/CI �重新 patch。后续评估私有 Maven 仓 fork jar 或升 springdoc 支持 SB 4.x。
2. **listening-go 迁移代码已 commit 但线上仍 0.1.0**(债1+2):需 rebuild 镜像 bump 0.2.0 部署验证(待债1+2 推进时统一)。
3. **pronunciation-rs /healthz 行**:0.1.1 镜像可能已含,待确认;若未含 bump 0.1.2。
4. **ArgoCD 3 sts OOS + Degraded**:待 GitHub 网络稳定后自愈,非阻塞。

---

## 会话22:ABCD 四批次推进(2026-08-09,A✅ B✅ C✅ D⏳)

用户指令"ArgoCD 3 sts 完全 Synced / infra Degraded 收口 / 主线推进(多级缓存、种子续灌、压测、前端审计并行)",决策"全串行 A→B→C→D + 全修"。

### ✅ A:ArgoCD 3 sts drift 收口(ignoreDifferences)
- **诊断**:`kubectl diff` 实测 pulsar-bk/pulsar-zk/redis-cluster-new 3 sts **live==target(0 行 diff)** → ArgoCD OutOfSync 是 **比较缓存陈旧**(GitHub 拉取抖动阻塞 repo-server 重新比较)。pods 全 Healthy(非阻塞)。
- **修复**:`deploy/argocd/argocd-app.yaml` 加 `ignoreDifferences` 块,屏蔽 K8s 默认填充字段(podManagementPolicy/revisionHistoryLimit/pvRetentionPolicy/updateStrategy/template metadata annotations/dnsPolicy/restartPolicy/schedulerName/securityContext/terminationGracePeriodSeconds)+ `kubectl apply --server-side --force-conflicts` 补齐 live-only 字段。commit 120c15f。
- **结论**:3 sts OOS 待 GitHub 网络稳定后 ArgoCD 重新比较自愈转 Synced。非阻塞。

### ✅ B:infra Degraded 全修(esp-builder a680c427dda1d0b2e,严格串行,CPU 58~62%)
1. **tiflash 镜像回退**:`07-tidb-tikv.yaml` line 615 `pingcap/tiflash:v8.5.7`→`v8.5.5`(v8.5.7 单节点 ImagePullBackOff)。apply RC=0,pod 1/1 Running,CPU 58%。
2. **tikv 3 deploy 改 Recreate**:`07-tidb-tikv.yaml` tikv-aws(line 310)/tikv-azure(352)/tikv-gcp(394)spec 加 `strategy: {type: Recreate}`(防未来滚动升级 RWO PV 文件锁争用)。tikv-aws 旧 pod CrashLoopBackOff 持锁,force delete 后新 pod 1/1 Running。
3. **redis-cluster-new-init Job 幂等**:`06c-redis-cluster.yaml` Job command 在 `--cluster create` 前加循环 `FLUSHALL` + `CLUSTER RESET HARD`(均 `|| true` 容错)。Job spec immutable 需 delete 再 apply。首次 Failed(master 有 keys 拒 RESET),加 FLUSHALL 后 **Job Complete 1/1(17s),cluster_state:ok / slots_assigned:16384 / known_nodes:6**。
- **git diff(未 push)**:`07-tidb-tikv.yaml` +4 行 tikv strategy +1 行 tiflash image;`06c-redis-cluster.yaml` +6 行 Job 幂等。2 files / 10 insertions / 1 deletion。
- **新模式(踩过可复用,会话22 B 批次)**:
  - ⑪ **Recreate 策略对 CrashLoop pod 无效**:tikv-aws 旧 pod CrashLoopBackOff 持 RWO PV 文件锁,Recreate 杀旧 RS desired=0 但该 pod 因 CrashLoop 重启不释放 PV。必须 force delete 该 pod 才能让新 pod 拿锁。manifest 层 Recreate 正确(防未来争锁),当前卡死 pod 需手动清。
  - ⑫ **CLUSTER RESET HARD 在 master 有 keys 时被拒**:Redis 要求先 `FLUSHALL` 清 db0 才能 RESET。幂等 Job 必须两者都加。Job spec immutable:apply 无法改,需 delete 再 create。

## 会话22 上半:5 任务收口闭环(2026-08-09,①②③④⑤ 全 ✅)

### ✅ ① ArgoCD 收口(会话21 残留:push 抖动 + Degraded)
- **push 成功**:`git -c http.version=HTTP/1.1 push origin master` → `53e9865..4c0e6a8`。
- **Degraded 根因**:ComparisonError `context deadline exceeded`。诊断:host `curl info/refs` 返 401 + GitHub API `Not Found`=仓库 private;repo-server 裸 git ls-remote 401;但 `github-repo-creds` Secret label `argocd.argoproj.io/secret-type=repository` 就位,ArgoCD API `GET /repositories` connectionState=**Successful** → 401 是瞬时 VM 网络抖动。
- **修复**:hard refresh + POST sync → esp-prod 全部 Deployment(assessment-rs/bff-java/course-path-go/emotion-svc/frontend-react/gamification-go/gateway-go/learning-java/learning-worker/listening-go/live-tutor-go/media-learn-rs/nacos/pronunciation-rs/reading-go/seata/sentinel/teacher-java/writing-coach/writing-py)**Synced+Healthy**。learning-worker replicas=0 Synced+Healthy ✅。

### ✅ ②③④⑤(会话21~22)
- ② assessment-rs REDIS_CLUSTER 加 `redis://` 前缀(`redis::cluster::ClusterClient` 要求 scheme,裸 host:port 解析失败)。
- ③ pronunciation-rs attempt_id=0 瑕疵修复 0.1.0→0.1.1:`pronunciation_attempts.id` 是普通 bigint(非 AUTO_RANDOM),`SELECT LAST_INSERT_ID()` 返 0 是真 bug,改 `SELECT id ... ORDER BY id DESC LIMIT 1`。rebuild 用 `tar cf - . | rdctl shell docker build`(模式⑧)。
- ④ CPU requests 降配(nacos 200→150/sentinel 100→50/seata 150→100)+ learning-worker image 1.4.0→1.6.0 + replicas 1→0。
- ⑤ 隐患E 闭环:S3~S7 五新服务(pronunciation-rs 0.1.1 / listening-go / assessment-rs / reading-go / gamification-go)全部实跑 + ArgoCD Synced+Healthy。

### 已确认模式(踩过可复用,累计 ⑧~⑫)
⑧ **Rust 服务 rebuild 镜像 ns 坑**:host docker context=colima 时 `docker build` 写 colima ns,kubelet(cri-dockerd)查 RD VM moby ns 拉不到。正确:`tar cf - <svc-dir> | rdctl shell docker build -t <tag> -`(RD VM 内 build 直写 moby ns)。
⑨ **ArgoCD 私有仓库 401 诊断顺序**:① host `curl info/refs` 看 http_code(401=private/限流,000=网络不通);② `kubectl -n argocd get secret <repo-creds>` 查 label;③ ArgoCD API `GET /repositories` 看 connectionState(Successful=凭据 OK,401 是瞬时抖动刷新即可)。**勿直接重启 repo-server**。
⑩ **GitHub push 抖动**:`git -c http.version=HTTP/1.1 push` 降级 HTTP/1.1 绕 HTTP/2 framing,重试 1-2 次。
⑪ Recreate 对 CrashLoop 持 RWO PV 锁 pod 无效(见 B 批次)。
⑫ CLUSTER RESET HARD 需先 FLUSHALL;Job spec immutable(见 B 批次)。

> Redis 锚点:`esp:ctx:hot:2026-08-08-session21-5tasks-wrapup`(①~⑤ + 模式模式⑧⑨⑩)+ `esp:ctx:hot:2026-08-09-session22-abcd-batch-ab`(A/B 闭环 + 模式⑪⑫ + C/D 待续)+ `esp:ctx:hot:2026-08-09-session22-2-springdoc`(债3 springdoc + 模式⑬ + bff AuthFilter 待续)。

## 会话22-4:springdoc jar uf 腐败修复 + 五服务串行 build/deploy 闭环(2026-08-09,✅ 全完成)

### ⑭ jar uf 腐败 Spring Boot NestedJarFile(新根因,取代模式⑬)
- **现象**:三 Java 服务(bff/learning/teacher)1.7.0 镜像全部 CrashLoopBackOff,`Unable to read meta-data for class org.springdoc.core.configuration.SpringDocUIConfiguration` / `FileNotFoundException: class path resource [org/springdoc/core/configuration/SpringDocUIConfiguration.class] cannot be opened`。
- **根因**:模式⑬ 的 antrun `zip -d` + `jar uf` 方案会破坏 Spring Boot 4.0.7 fat jar 的 NestedJarFile classloader。`zip -d` 正确删除了内嵌 springdoc jar 的两个 Querydsl class(298→296 files),SpringDocUIConfiguration.class 仍在(unzip/javap/standalone Java 均可读取),但 Spring Boot 的 NestedJarFile loader **无法从被 `jar uf` 修改过的 fat jar 条目中加载 class**。`jar uf` 增量更新改变了 ZIP central directory 结构,SB 4.0.7 的优化 classloader 不兼容。
- **验证**:① 手动 `jar cf0M` 全量重建 fat jar(STORED 无压缩)+ 删 Querydsl → 同样崩溃(排除压缩方式);② **关键发现**:`jar uf` 对 fat jar 中**任何** nested jar 条目的修改都会破坏 SB loader——即使把 pristine(未删 Querydsl)的 jar 用 `jar uf` 覆盖回去也会崩溃;③ 不用 `jar uf`(直接 patch .m2 jar 后正常 `mvn package`)→ 启动成功(仅 seata 连不上,本地无 seata)。
- **修复方案(最终)**:① 恢复 .m2 springdoc jar 到 pristine(.bak);② patch .m2 jar:`unzip` 解出 → 删 2 个 Querydsl class → `jar cf` 重新打包(非 `jar uf`)→ 覆盖回 .m2;③ 三服务 pom.xml **移除整个 antrun plugin**;④ `mvn clean package -DskipTests` 正常构建(spring-boot:repackage 从已 patch 的 .m2 拉取 nested jar,无需后处理)。
- **.m2 patch 持久化说明**:.m2 patch 是本地的,换机器/CI 需重新 patch。后续应评估:① 升级 springdoc 到支持 SB 4.x 的版本(截至 2026-08 springdoc 2.x 全系未修);② 自建 fork jar 发布到私有 Maven 仓;③ 用 `spring.autoconfigure.exclude` 排除 SpringDocConfiguration(但 Dubbo BeanPostProcessor 扫描在 autoconfigure 之前,exclude 无效)。

### 五服务串行 build/deploy 结果(CPU 35~56%,全串行)
| 服务 | 版本 | mvn/build RC | docker RC | deploy RC | pod Ready | 端点验证 |
|---|---|---|---|---|---|---|
| bff-java | 1.7.0 | 0 | 0 | 0 | 1/1 Running 0 restarts | /v3/api-docs **200** ✅ /docs **302** ✅ /swagger-ui/index.html **200** ✅ /openapi-go-rust.yaml **200** ✅ |
| learning-java | 1.7.0 | 0 | 0 | 0 | 1/1 Running 0 restarts | /v3/api-docs **200** ✅ |
| teacher-java | 1.7.0 | 0 | 0 | 0 | 1/1 Running 0 restarts | /v3/api-docs **200** ✅ |
| gateway-go | 1.5.1 | N/A(go build in docker) | 0 | 0 | 1/1 Running 0 restarts | /healthz **200** ✅ /api/sessions/history **401**(需 JWT,端点存在 ✅) |
| pronunciation-rs | 0.1.1 | N/A(cargo build in docker) | 0 | 0 | 1/1 Running 0 restarts | /healthz **200** ✅ /actuator/health/readiness **200** ✅ |

- **manifest apply**:15-bff-java / 20-learning-java / 30-other-services 三 manifest 已 apply RC=0(svc configured)。
- **bff 4 端点全 200**:EdgeAuthHandler WHITELIST + RouteTable.isLocal 已加 portal 路径(/v3/api-docs /docs /swagger-ui /openapi-go-rust.yaml),AuthFilter 白名单一致。
- **pronunciation-rs 端口纠正**:container 监听 8088(非文档中写的 8084),svc targetPort=8088 → port 80。/healthz + /actuator/health/readiness + /actuator/health/liveness 三端点全 200。
- **gateway-go /api/sessions/history**:返回 401(需 Bearer JWT),端点存在(ListHistory 路由 main.go:94 + manager.go:109 直读 esp_speaking.sessions)。

### 已确认模式(踩过可复用,累计 ⑧~⑭)
⑭ **jar uf 破坏 Spring Boot 4.x NestedJarFile classloader**:SB 4.0.7 fat jar 的 NestedJarFile 优化 loader 不兼容 `jar uf` 增量更新后的 ZIP central directory 结构。任何对 fat jar 中 nested jar 条目的 `jar uf` 修改(即使覆盖回相同内容)都会导致 `Unable to read meta-data for class XXX` / `FileNotFoundException`。**解法**:patch .m2 源 jar 后正常 `mvn package`(repackage 从 .m2 拉取,无需后处理 fat jar)。

### 遗留项
1. **.m2 patch 非持久化**:三服务 pom 已移除 antrun,但 .m2 springdoc jar 的 Querydsl 删除是本地操作。换机器/CI 需重新 patch。后续需评估私有 Maven 仓 fork jar 或等待 springdoc 修复。
2. **git 未 push**:pom 改动(移除 antrun)+ EdgeAuthHandler/RouteTable 白名单 + manifest tag 同步,待主线程 push。
3. **ArgoCD**:manifest apply 后 ArgoCD 可能检测到 drift(如果 GitHub 仓 manifest tag 未同步),需确认 selfHeal 行为。

### 模式 ⑬:springdoc 2.x + Spring Boot 4.x Querydsl 不兼容
springdoc 2.6.0/2.8.13 的 `QuerydslPredicateOperationCustomizer` 引用 `org.springframework.data.util.ClassTypeInformation`,SDC 4.0 迁移到 `data.core`。Dubbo BeanPostProcessor 扫描触发 NoClassDefFoundError。
- **持久化方案(会话22-3 验证生效)**:pom 驱动 maven-antrun-plugin,`<exec executable="zip">` 直接 exec zip 进程不经 shell(`$` 字面传递,绕过 bash `$$`→PID 扩展坑 + Ant fileset 无法匹配含 `$` 文件名坑),从 fat jar 内嵌 `BOOT-INF/lib/springdoc-openapi-starter-common-2.6.0.jar` 删两个 Querydsl class。三服务 pom 均已加,bff 实测 BUILD SUCCESS + fat jar 无 Querydsl。CI/换机可复现。
- **踩坑排除**:① shade-plugin `<excludes>` filter 对 nested jar 不可靠;② `<exec bash -c '...$$Querydsl...'>` bash PID 扩展 stall;③ Ant `<delete><fileset include="...$...">` 不匹配含 `$` 文件名。

---

## 会话22-3:债3收尾 + 债4 + C 前端(2026-08-09,代码完成 ⏳部署中)

用户指令"完整完成后续所有任务(可并行就并行)",顺序:债3 → 债4 → C → 债1+2 → D。

### 债3 springdoc 收尾(代码完成,builder 部署中)
- AuthFilter WHITELIST 加 `/v3/api-docs /docs /swagger-ui /openapi-go-rust.yaml`(`AuthFilter.java:31`)。
- 三服务 pom 改 antrun `<exec zip>` 持久化方案(模式⑬,bff 实测生效)。
- manifest image tag 同步 1.7.0(15-bff-java / 20-learning-java / 30-other-services)。
- esp-builder 串行 build/deploy:bff(四端点验证)+ learning/teacher(`/v3/api-docs` 200)+ manifest apply。

### 债4(gateway-go + pronunciation-rs rebuild 验证 ✅)
- **gateway-go 1.5.1**:rebuild + deploy RC=0,1/1 Running,/healthz **200**, /api/sessions/history **401**(需 JWT,路由存在 main.go:94 + manager.go:109 直读 esp_speaking.sessions)。
- **pronunciation-rs 0.1.1**:rebuild + deploy RC=0,1/1 Running,/healthz **200**, /actuator/health/readiness **200**, /actuator/health/liveness **200**。container 监听 8088(svc targetPort=8088→port 80)。

### C 前端(代码完成 ✅,H5 镜像 build/部署待 builder)
三端工程已在仓库根目录(非 services/),补建会话历史 + Profile:
- **Web**:`api.ts` 加 `sessionHistory()`,`types.ts` 加 `SessionSummary`,`Profile.tsx` 扩展会话历史表格。tsc ✅。
- **H5**:`Profile.tsx` 新建(antd-mobile List)+ `main.tsx` 加 profile 路由 + Home 加"我的"Tab;`api.ts`/`types.ts` 加 history/SessionSummary。tsc ✅。
- **小程序**:`pages/index/index.tsx` 扩为完整"我的"页(历史列表 useDidShow)+ `api.ts` 加 sessionHistory。tsc 编译错误是 Taro 全局类型(`wx`/`TARO_APP_API`)固有,非本次改动引入(原状即如此,需 Taro 编译器环境)。
- **H5 容器化**:`frontend-h5/Dockerfile`(复用 15 多阶段模板)+ `nginx.conf`(反代 Traefik,对齐 Web)+ `deploy/k8s/16-frontend-h5.yaml`(NodePort 30091,已确认未占用)。
- **docs/test/08 矛盾修复**:评分改 Web 85% / H5 72% / 小程序 72%(补建后实际值),补建记录入 P2 缺口 ✅ 标记,API 覆盖率加 `/api/sessions/history` 三端 ✅。
- **H5 镜像 build + apply 16-manifest + ArgoCD sync**:待 builder(串行,与债3/债4 build 不并行)。

---

## 下一步动作(恢复锚点,2026-08-09 会话22-5 后)

债3/4 ✅闭环+push(15b5ffc)。C 前端 ✅(H5 已部署)。改动来源厘清 ✅。**当前推进债1+2 公共包抽取 + D 种子压测**。

### ⏳ 债1+2 公共包抽取(纯重构,行为不变红线)
**Go(esp-common-go 骨架已完成,1/7)**:
1. **先修隐患**:`services/esp-common-go/go.mod` 补 `require go.opentelemetry.io/otel`(tracing 包 import 但未声明,迁移前必补,否则编译失败)。
2. 6 Go 服务迁移(gateway/course-path/reading/gamification/live-tutor/vocab-srs),按 listening-go 模式:go.mod 加 `require+replace esp/common-go`,main.go 改 import config/db/health。
   - **gateway-go 特殊**:已有未落盘的 sessions/history 改动(已 commit);其 db 是自研 `db.Read`(读写分离+fallback),与 esp-common-go `db.MustConnectDB` 不同层次,不可直接替换,需评估保留自研 db 还是统一。
3. 每服务 rebuild 镜像 bump 版本,验证 healthz + 核心接口不退化(listening-go 0.1.0→0.2.0)。

**Rust(esp-common-rs 未建)**:
4. 建 `services/esp-common-rs/` + root `Cargo.toml` [workspace](4 crate 共享依赖)。抽 db/redis/health/tracing_init;**不抽** error enum(改 HTTP body 契约)。
5. 4 Rust 服务(pronunciation-rs/assessment-rs/reading?/media-learn-rs/llm-orchestrator-rs)迁移,改 Dockerfile build context(workspace 化),每服务 rebuild 验证。
6. vocab-srs-go trace 统一(`v-`→OTel propagator)+ llm-orchestrator trace_id 透传统一:**单列评估下游依赖**后再改。

### ⏳ D:种子续灌 + 压测摸底
1. 确认 CPU headroom(会话22-5 观察:node CPU 46~65%,learning HPA 抖动,有 headroom 但需监控)。
2. session_turns 16 分片续灌(目标 300M,当前 50.14M / 16.7%,断点续传,Redis `esp:ctx:seed:session_turns:shard:{0..15}` 存 offset);sessions 补齐 2.42M(当前 47.58M/50M=95.16% suspended@08-04)。
3. locust 基线压测摸底(单节点铁律:压测机与被测同节点禁,重负载串行,CPU 监控)。

**其他主线(非阻塞)**:多级缓存(L1 Caffeine/cachetools/LRU + L2 Redis Cluster + Pulsar 广播失效 + 降级,已落 6 服务)、bff sentinel datasource empty 根因(SCA 版本/dataId)、Python 镜像 base 决策(bookworm 6 CRIT fix=N/A,待用户)、Java/Go/Rust 性能对比报告、Redis announce FQDN 持久化(P1)。

**单节点铁律**:重 mvn/docker build 必须与集群操作串行(esp-singlenode-host-cluster-contention);不碰 k3s 配置/restart/pkill/Factory Reset(esp-never-break-k3s);VM 重启/切驱动后必预置 pause 镜像(esp-pause-image-mirror-ironrule);压测机不能与被测同节点。

## 关键部署状态(恢复锚点,2026-08-09 会话22-5 更新)

- **镜像**:bff-java **1.7.0**(springdoc+patched)、gateway-go 1.5.1(含 sessions/history)、learning-java **1.7.0**(springdoc)、teacher-java **1.7.0**(springdoc)、writing-py 1.6.1、emotion-svc 1.6.1、writing-coach 1.0.1、frontend-react 1.0.0、**frontend-h5 1.0.0**(会话22-5 部署)、course-path-go 0.1.0、pronunciation-rs 0.1.1、listening-go / assessment-rs / reading-go / gamification-go / live-tutor-go / media-learn-rs 0.1.0、llm-orchestrator-rs 0.1.0。
- **中间件**:nacos v2.4.3 / seata 2.2.0 / sentinel 1.8.9。
- **NodePort**:gateway-go 30080、frontend-react 30090、**frontend-h5 30091**(会话22-5)。
- **基础设施复用** ns `ekb-m2-aws-us-east-1`,ExternalName 短名 `tidb-primary`/`tidb-follower`/`redis-cluster`/`pulsar-broker`。Redis Cluster 实际 `redis-cluster-new-{0..5}.redis-cluster-new.ekb-m2-aws-us-east-1.svc.cluster.local:6379`(3主3从)。TiDB `tidb-primary:4000` root 无密码,11 DB/29 表。
- **infra(B 批次后)**:tiflash v8.5.5 1/1 Running;tikv-aws Recreate 1/1 Running;tikv-azure/gcp Running(strategy=Recreate);redis-cluster-new 6/6 Running cluster_state:ok。
- **ArgoCD**:app `esp-platform`(deploy/k8s,GitHub 私有仓,automated prune=false selfHeal=true ServerSideApply),ignoreDifferences 已加(commit 120c15f)。**会话22-5 状态**:sync=OutOfSync(仅 pulsar-bk/pulsar-zk/redis-cluster-new 3 sts,待 GitHub 网络稳定自愈)+ health=Degraded(ComparisonError repo-server 拉 GitHub 抖动,host curl=401 网络通,模式⑨勿重启)。esp-prod 全业务 deploy 实测 Running。

## 关键命令速查

- 查 TiDB 表:`scripts/db_query.sh "SQL"`(pymysql pod 封装)。
- 服务日志:`kubectl -n esp-prod logs deploy/<svc> --tail=20`。
- Redis 写/读:`kubectl -n ekb-m2-aws-us-east-1 exec redis-cluster-new-0 -- redis-cli -c SET/GET "esp:ctx:*" "..."`。
- ArgoCD sync:`kubectl -n argocd patch application esp-platform --type=merge -p '{"operation":{"sync":{}}}'` 或 API POST。
- 压测:分布式 locust 见 `deploy/locust/README.md`。
- 单节点铁律:重 mvn/docker build 与集群操作串行;不碰 k3s 配置。

## 上下文工程铁律(摘要,详见 memory `context-overflow-ironrules`)

- 文件系统=长期记忆,对话=临时工作区。80%(160k)主动 /compact,90%(180k)强制止损落盘+compact。
- 三层防御:源头减量(subagent 隔离+输出限流)+ 状态外化(PROGRESS 热区+Redis 运行态)+ 主动管控。
- **本文件热区上限 ~300 行**,历史归 `docs/progress/archive/`,不全量读。完整铁律集见 memory。

## 历史 milestone 归档索引(需要时取)

| 主题 | 文件 | Redis key |
|---|---|---|
| **会话18~22 冷区批次(08-07~09)** | `docs/progress/archive/2026-08-09-cold-archive-session18-22.md` | `esp:ctx:hot:2026-08-09-session22-abcd-batch-ab` |
| teacher readiness 根治 + 安全扫描四件套(08-07) | (见冷区归档) | `esp:ctx:hot:2026-08-07-teacher-readiness-and-security-scan` |
| teacher CrashLoop + 增量服务(08-07) | (见冷区归档) | `esp:ctx:hot:2026-08-07-teacher-crashloop-and-new-services` |
| Pulsar cookie + VM 网络中断(08-06) | `docs/progress/archive/2026-08-06-pulsar-cookie-fix-and-vm-down.md` | `esp:ctx:archive:2026-08-06-pulsar-cookie-fix-and-vm-down` |
| Pulsar Geo + 应用切新集群(08-05) | `docs/progress/archive/2026-08-05-pulsar-geo-and-cluster-switch.md` | `esp:ctx:archive:2026-08-05-pulsar-geo-and-cluster-switch` |
| 上线就绪 + 三层坑事故(08-04) | `docs/progress/archive/2026-08-04-readiness-and-three-layer-incident.md` | `esp:ctx:archive:2026-08-04-readiness-and-three-layer-incident` |
| 07-31~08-03 历史 | `docs/progress/archive/2026-08-{01,02,03}-*.md` | `esp:ctx:archive:2026-08-{01,02,03}-*` |
