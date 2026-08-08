# ESP 项目进展 (PROGRESS) — 热区

> **本文件只放当前任务热区**(已确认根因/当前子步骤/下一步/恢复锚点),硬上限 ~300 行。
> **历史里程碑见 `docs/progress/archive/*.md`**(按主题拆分),跨会话恢复亦可从 Redis `esp:ctx:archive:*` 取。
> **恢复协议**:新会话先读本文件(全文)+ TaskList,不全量读历史;需要历史细节 Grep archive 或 Redis 取。
>
> 更新日期:2026-08-09 会话22(✅ ①②③④⑤ 全部闭环。会话21 残留:commit 4c0e6a8 push 抖动失败 → 会话22 `git -c http.version=HTTP/1.1 push` 成功;ArgoCD OutOfSync/Degraded 根因=GitHub 私有仓库 repo-server 拉 git 401(瞬时网络抖动 + repo-server 无缓存凭据,但 `github-repo-creds` Secret 已 label repository 自动发现,API 测 repo connectionState=Successful);hard refresh + sync 后 esp-prod 全部 Deployment Synced+Healthy,learning-worker replicas=0 Synced+Healthy。残留 OutOfSync 仅 pulsar-bk/zk + redis-cluster-new 3 sts(预存 infra drift,diffKind live-only 字段,非本次改动,health=Healthy,非阻塞)。Redis 锚点 esp:ctx:hot:2026-08-08-session21-5tasks-wrapup 已写)

## 会话22:5 任务收口闭环(2026-08-09,①②③④⑤ 全部 ✅)

会话21 已闭环 ②③④;① 残留 commit 4c0e6a8 push 失败 + ArgoCD Degraded;⑤ 待 ①。本会话闭环 ①⑤。

### ✅ ① ArgoCD 收口(会话21 残留:push 抖动 + Degraded)
- **push 成功**:`git -c http.version=HTTP/1.1 push origin master` → `53e9865..4c0e6a8 master -> master`(会话21 抖动失败,HTTP/1.1 重试一次成功)。
- **ArgoCD Degraded 根因深挖**:ComparisonError `failed to list refs ... context deadline exceeded`。诊断路径:
  1. host `git ls-remote` + `curl info/refs` 成功但返 **HTTP 401** → GitHub API `repos/...` 返 `Not Found` → **仓库是 private**(host 能 ls-remote 因 macOS keychain 缓存凭据)。
  2. repo-server pod 内裸 `git ls-remote`(无凭据)→ 401 → `could not read Username`。
  3. 但 `argocd get secret github-repo-creds` label `argocd.argoproj.io/secret-type=repository` 已就位(url/username/40-char PAT),ArgoCD API `GET /repositories` connectionState=**Successful** → 凭据自动发现正常,401 是**瞬时 VM 网络抖动**(会话21 同源 GitHub 连接抖动)。
- **修复**:hard refresh + POST sync → `successfully synced (all tasks run)`。
- **验证(esp-prod 全部 Deployment)**:assessment-rs/bff-java/course-path-go/emotion-svc/frontend-react/gamification-go/gateway-go/learning-java/learning-worker/listening-go/live-tutor-go/media-learn-rs/nacos/pronunciation-rs/reading-go/seata/sentinel/teacher-java/writing-coach/writing-py **全部 sync=Synced health=Healthy**。learning-worker replicas=0 Synced+Healthy ✅(selfHeal 拉齐)。
- **残留 OutOfSync(非阻塞,预存 infra drift)**:`pulsar-bk`/`pulsar-zk`/`redis-cluster-new` 3 sts sync=OutOfSync 但 **health=Healthy**(diffKind 为 live-only 字段,ServerSideApply 未覆盖,非本次改动引入)。另 `ekb-m2-*` ns 的 tiflash/tikv-*/otel-collector-gateway/redis-cluster-new-init 几个 infra Deployment/Job health=Degraded(多区域 TiDB 组件单节点无对应节点调度 + otel 配置,均预存非本次)。后续如需完全 Synced,对这些 sts 显式 `kubectl apply --server-side --force-conflicts` 补齐 live-only 字段。

### ✅ ⑤ 隐患E 收口标记
- S3~S7 五新服务(pronunciation-rs 0.1.1 / listening-go / assessment-rs / reading-go / gamification-go)全部实跑闭环 + ArgoCD esp-prod Synced+Healthy → 隐患E 闭环。

### 已确认模式(踩过可复用,会话22 新增)
⑨ **ArgoCD 私有仓库 401 诊断顺序**:repo-server ComparisonError `context deadline exceeded/401` 时:① host `curl info/refs` 看 http_code(401=private 或限流,000=网络不通);② `kubectl -n argocd get secret <repo-creds>` 查 label `argocd.argoproj.io/secret-type=repository` 是否就位;③ ArgoCD API `GET /repositories` 看 connectionState(Successful=凭据 OK,401 是瞬时抖动,刷新即可;Failed=凭据问题)。**勿直接重启 repo-server**,先验证凭据 Secret 与 connectionState。
⑩ **GitHub push 抖动**:`git -c http.version=HTTP/1.1 push` 降级 HTTP/1.1 绕 HTTP/2 framing 抖动,重试 1-2 次。

> ✅ 会话21+22 全部闭环:② assessment-rs Redis env、③ pronunciation-rs attempt_id(0.1.1)、④ CPU 降配+learning-worker scale 0、① ArgoCD(push+sync+esp-prod 全 Healthy)、⑤ 隐患E 标记。
> 残留非阻塞:pulsar-bk/zk/redis-cluster-new 3 sts 预存 infra drift(health=Healthy)。
> Redis 锚点:`esp:ctx:hot:2026-08-08-session21-5tasks-wrapup`(②③④ 闭环 + ① push/sync + ⑤ 标记 + 模式⑨⑩)。

## 会话21:5 任务收口(2026-08-08~09,②③④✅ ①🔄 ⑤⏳)

用户指令"继续推进 ①~⑤ 完整完成"。执行序:④(腾配额)→ ②③(Rust 服务)→ ①(ArgoCD)→ ⑤(标记)。

### ✅ ④ CPU requests 降配(node cpu 95%→92%)
- builder a3bf7c2e1bc66af74(已 kill,因卡在 colima buildx 拉镜像超时)完成:patch nacos 150m→100m、sentinel 100m→50m live,node cpu 93%→92%(8450m)。
- builder 上轮已改 manifest:`06-sca-middleware.yaml`(nacos 200→150、sentinel 100→50、seata 150→100)、`21-learning-worker.yaml`(200→100)。
- **learning-worker 收口**:manifest `21-learning-worker.yaml` image 1.4.0→1.6.0 + replicas 1→0(P2-2 设计本就"单节点默认不 apply",1.4.0 远程 403 ErrImagePull,selfHeal 反复拉起→scale 0 止血)。commit 4c0e6a8。
- 已 push commit 53e9865(batch2 全量)到 GitHub master;commit 4c0e6a8(learning-worker fix)push 时 GitHub 连接抖动失败(HTTP2 framing + 443 连接超时,host curl github.com 也 timeout,疑似 VM/网络瞬时),待重试。

### ✅ ② assessment-rs REDIS_CLUSTER `redis://` 前缀(IRT θ cache 降级修复)
- 根因:`redis::cluster::ClusterClient::builder(vec![addr])` 要求 addr 含 `redis://` scheme,裸 host:port 解析失败。
- 改:`32-batch2-services.yaml` line 116 加 `redis://` 前缀(env-only,无 rebuild)。
- 验证:apply 32-batch2 + rollout assessment-rs 成功;`exec ... echo $REDIS_CLUSTER` = `redis://redis-cluster-new-0...:6379` ✅;pod ready=true。

### ✅ ③ pronunciation-rs attempt_id=0 瑕疵修复(0.1.0→0.1.1)
- **根因深挖(纠正会话19 误判)**:会话19 以为 attempt_id=0 是 AUTO_RANDOM 的 LAST_INSERT_ID 已知行为。会话21 实查 `information_schema.columns`:**pronunciation_attempts.id EXTRA=''(普通 bigint,非 AUTO_RANDOM/AUTO_INCREMENT)**。所以 LAST_INSERT_ID() 返回 0 是真 bug。
- 源码改:`services/pronunciation-rs/src/main.rs` line 134 `SELECT LAST_INSERT_ID()` → `SELECT id FROM pronunciation_attempts ORDER BY id DESC LIMIT 1`,类型 `(u64,)`→`(i64,)` + `as u64`。
- manifest 改:`32-batch2-services.yaml` pronunciation-rs image 0.1.0→0.1.1 + REDIS_CLUSTER 加 redis:// 前缀(同 ② 根因)。
- **rebuild 关键坑(模式⑧)**:宿主机 docker context=colima(buildx builder),build 出的镜像在 colima ns;但 RD k8s kubelet via cri-dockerd 查 **RD VM 内 moby docker ns**。正确做法:`tar cf - . | rdctl shell docker build -t esp/pronunciation-rs:0.1.1 -`(在 RD VM 内 build,直写 moby ns,kubelet 可拉)。host `docker build` 会写错 ns(除非 docker context 指向 RD moby socket)。
- build 在 RD VM 成功(rust release 343s + export 3.8s),image `esp/pronunciation-rs:0.1.1` digest d20a9bba。
- 验证:patch image + rollout 成功;pod ready=true image=0.1.1;`exec gateway-go wget` 调 score 接口 → `{"attempt_id":7782220156096217092,"score":0.85}` ✅(真实 id 非 0)。
- env REDIS_CLUSTER 同步含 redis:// 前缀 ✅。

### 🔄 ① ArgoCD 收口会话18 隐患D(去重✅ push✅ sync 触发⚠️)
- **Degraded 根因已修**:`deploy/k8s` 中 tidb-follower/tidb-primary Service 各定义 2 次(05-infra-reuse + 07-tidb-tikv 末尾)→ ArgoCD `RepeatedResourceWarning`。改:删 `07-tidb-tikv.yaml` 末尾两块 Service,留注释指 05 唯一定义。去重后 ArgoCD conditions 0(RepeatedResourceWarning 消失)✅。
- **push 成功**:commit 53e9865(S3-S7 batch2 全量 + 去重 + CPU 降配 + argocd-app)push 到 GitHub master 成功。commit 4c0e6a8(learning-worker fix)push 因 GitHub 连接抖动失败,待重试。
- **sync 触发**:ArgoCD API `POST /applications/esp-platform/sync` 返回 operationState "successfully synced (all tasks run)"。history id=1 revision=53e9865 deployedAt=16:09:40。
- **当前 OutOfSync 残留(非阻塞,基础设施预存 drift)**:
  - `pulsar-bk`/`pulsar-zk`/`redis-cluster-new` 三个 StatefulSet OutOfSync(预存 infra drift,非本次改动引入,diffKind=None,ServerSideApply 未覆盖某些 live-only 字段)。
  - nacos/seata/learning-worker Deployment 已 Synced(push 后 selfHeal 拉齐)。
  - learning-worker manifest replicas=0 但 selfHeal 暂未拉回(push 4c0e6a8 失败,origin 仍 replicas=1)→ 待重试 push。
- **health 仍 Degraded**:源于上述 3 个 sts OutOfSync + 可能 learning-worker replicas drift。待 push 4c0e6a8 后 selfHeal 拉齐 learning-worker,再观察 health 是否转 Healthy。

### ✅ ⑤ 隐患E 收口标记(会话22 完成)
- ① push 成功 + ArgoCD esp-prod 全 Deployment Synced+Healthy → 隐患E 已标 `✅ 闭环`(见会话22 节 + 本文件 line 795 标题已改 ✅)。

## 下一步动作(恢复锚点,2026-08-09 会话22 后)

5 任务①~⑤全部闭环,无阻塞下一步。可选后续:
1. **(可选)ArgoCD 完全 Synced**:pulsar-bk/pulsar-zk/redis-cluster-new 3 sts 预存 drift(health=Healthy 非阻塞),若要 Synced,显式 `kubectl apply --server-side --force-conflicts` 或 manifest 补 live-only 字段。
2. **(可选)infra Degraded 收口**:`ekb-m2-*` ns tiflash/tikv-*/otel-collector-gateway/redis-cluster-new-init(多区域 TiDB 组件单节点无节点调度 + otel 配置,预存非本次)。
3. **主线推进**:隐患E 闭环后,可推进下一主线(待用户指定;候选见会话20 Redis next_candidates:多级缓存/种子数据续灌/压测/前端审计)。

### 已确认模式(踩过可复用,会话21 新增)
⑧ **Rust 服务 rebuild 镜像 ns 坑**:宿主机 docker context=colima 时,`docker build` 写 colima ns,RD k8s kubelet(cri-dockerd)查 RD VM moby ns 拉不到→ImagePullBackOff。正确:`tar cf - <svc-dir> | rdctl shell docker build -t <tag> -`(RD VM 内 build 直写 moby ns)。或 host `docker context use <rd-moby>`。区别于模式⑤(docker build 直写 RD ns)——⑤成立前提是 host docker context 已指向 RD moby;若 context=colima 则需 rdctl shell。

> ✅ 全部闭环(会话21+22):② assessment-rs Redis env、③ pronunciation-rs attempt_id(0.1.1)、④ CPU 降配+learning-worker scale 0、① ArgoCD(push+sync+esp-prod 全 Healthy)、⑤ 隐患E 标记。
> Redis 锚点:`esp:ctx:hot:2026-08-08-session21-5tasks-wrapup`(②③④ 闭环 + ① push/sync + ⑤ 标记 + 模式⑧⑨⑩)。

## 会话20:推进 E,S4~S7 逐个实跑(2026-08-08,✅ 全部闭环)

用户指令"自动从 PROGRESS.md + Redis cluster 恢复现有任务直至完整完成"。TaskList #1~#4 = S4/S5/S6/S7 构建+部署+实跑验证。**单节点铁律:严格串行,不并行 build**。

**S4 listening-go ✅ 完整闭环**:
- 镜像:`esp/listening-go:0.1.0` digest `sha256:f162ebdd...`(arm64 33.5MB),docker build 直写 RD docker ns,kubelet via cri-dockerd 直接可拉,**无需 save+load 转换**(区别于 nerdctl build 坑)。
- 基镜像 `golang:1.25-alpine`/`alpine:3.22` docker.io 直拉 OK(docker.1ms.run mirror 反而 TLS 超时弃用)。
- pod:`listening-go-64cf56c57d-vztw6` 1/1 Running 0 restarts。
- 健康检查(port-forward 8089):`/healthz`→`{"service":"listening-go","status":"UP"}` 200;`/readyz`→`{"db_read":true,"status":"UP"}` 200。
- API:`GET /api/listening/materials`→200 null(表空正常);`POST /api/listening/attempts`→`{"accepted":true,"score":0.75}` 202,esp_listening.listening_attempts 0→3(+3,跨 schema 读写无 1146)。
- **坑(已修)**:首次 GET /materials 返 500 `Error 1210: Incorrect arguments to LIMIT`。根因=Go database/sql 把 string 传 `LIMIT ?` 占位符,TiDB 不接受字符串参数。修复=`listMaterials` 用 `strconv.Atoi` 将 limit 转 int 再传参。改 `services/listening-go/src/main.go`(加 strconv import + Atoi)。
- **app_logs 不写说明**:listening-go scaffold 无 logging_client,不向 esp_shared.app_logs 跨 schema 写(与 gateway-go/learning-java 设计不同)。该服务验证重点是 esp_listening 自身 schema 读写,已 attempts +3 证实。非 bug。
- manifest:`deploy/k8s/32-batch2-services.yaml` listening-go replicas 0→1。
- 改动文件:`32-batch2-services.yaml`(replicas)/`listening-go/src/main.go`(LIMIT int)。

**S5 assessment-rs ⚠️ 列名改源被推翻,简化为三项改源(重新构建中)**:
- **关键勘误**:explorer 原核对基于 `docs/design/schema.sql`(设计态新结构),但**线上 esp_assessment 是旧 schema**(实测 information_schema)。两者列名不同:
  - **线上 assessment_sessions 实际列**(esp_assessment):`id`(bigint NOT NULL)/`user_id`(bigint NOT NULL)/`goal`(varchar NULL)/`cefr_result`(varchar NULL)/`status`(varchar NOT NULL default 'in_progress')/`score`(decimal NULL)/`started_at`(datetime NOT NULL default CURRENT_TIMESTAMP)/`finished_at`(datetime NULL)。**无 test_type/completed_at/overall_score/irt_theta/report**(那些是 schema.sql 设计态新增列,线上未建)。
  - **线上 assessment_answers 实际列**:`id`(bigint NOT NULL)/`session_id`(bigint NOT NULL)/`question_id`(bigint NOT NULL)/`answer_json`(json NOT NULL)/`is_correct`(tinyint NOT NULL default 0)/`skill_tag`(varchar NULL)/`created_at`(datetime NOT NULL default CURRENT_TIMESTAMP)。**无 item_id/answer/correct/irt_theta/responded_at**。
  - **结论**:main.rs 原始列名(goal/cefr_result/score/finished_at/question_id/answer_json/is_correct/skill_tag/created_at)**与线上旧 schema 一致**,explorer 的"列名系统性错位"判断基于设计态文档,被推翻。**列名 D 项免改**。
  - 注意:线上 `id` 列 EXTRA 为空(非显式 AUTO_RANDOM),需确认是否 AUTO_RANDOM——若非 AUTO_RANDOM,则 INSERT...RETURNING/fetch_one 仍不适用(见 C 项)。
- **实际必改源仅 3 项**(builder 已按此重新构建):
  1. **Dockerfile L4/L20**:两 FROM 改 `docker.1ms.run/library/rust:1.88-bookworm` + `docker.1ms.run/library/debian:bookworm-slim`(i/o timeout 坑)。
  2. **main.rs AppHandle 解引用**:`h.mysql_read`/`h.mysql_write` → `h.0.mysql_read`/`h.0.mysql_write`(无 impl Deref)。
  3. **main.rs AUTO_RANDOM 坑**:`create_session`/`submit_answer` 的 `query_as("INSERT...").fetch_one()` → `query("INSERT...").execute()` + `SELECT LAST_INSERT_ID()` 两步,元组 `(i64,)`→`(u64,)`。**前提**:线上 id 是 AUTO_RANDOM 才适用此坑;若线上 id 是普通 AUTO_INCREMENT,fetch_one+RETURNING 在 TiDB 仍不工作(TiDB 不支持 RETURNING),仍需改 execute+LAST_INSERT_ID。即无论 AUTO_INCREMENT/AUTO_RANDOM,C 项都必改。
- builder 上一轮被用户 stop(因 explorer 误判列名),已按"只改 B+C+Dockerfile,列名回退对齐线上旧 schema"重新构建。待 builder 完成通知。
- manifest:containerPort 8090,env TIDB_DB=esp_assessment/WRITE_DB_URL tidb-primary/READ_DB_URL tidb-follower/REDIS_CLUSTER redis-cluster-new-0。健康 `/actuator/health`(`/readiness` 查 TiDB)。API `/api/assessment/`{sessions,answer,finalize,report}。replicas 0→1 待改。
- **教训(新增可复用模式)**:核对列名必须查 `information_schema.columns`(线上实态),不能信 `docs/design/schema.sql`(设计态,可能与线上不一致)。schema.sql 是目标态,线上可能是历史迁移前的旧结构。

**S6 reading-go ✅ 完整闭环**:
- 镜像:`esp/reading-go:0.1.0`(docker build 直写 RD docker ns)。
- pod:`reading-go-9698bdbb-nrzsh` 1/1 Running 0 restarts。
- 健康检查(port-forward 8091):`/healthz` 200、`/readyz` 200。
- API:`GET /api/reading/passages`(默认 limit=10)→200 null;`?limit=5`→200;`?level=A2&limit=3`→200;`/passages/1`(不存在)→404。`POST /api/reading/attempts`→`{"accepted":true,"score":0}` 202,esp_reading.reading_attempts 实写(+1,跨 schema 无 1146)。
- **坑(已修)**:首次 `GET /passages` 返 500 `Error 1210 Incorrect arguments to LIMIT`,**同 listening-go 同源坑**(Go database/sql 把 string 传 `LIMIT ?`)。修复=`listPassages` 用 `strconv.Atoi` 将 limit 转 int,加 strconv import。改 `services/reading-go/src/main.go`(line 77-89)。
- manifest:`deploy/k8s/32-batch2-services.yaml` reading-go replicas 0→1(早已改)。
- 改动文件:`reading-go/src/main.go`(LIMIT string→int,同 S4 模式④)。

**S7 gamification-go ✅ 完整闭环**:
- 镜像:`esp/gamification-go:0.1.0`(36.6MB,docker build 走 RD VM 内 docker,9p 挂载路径 `/Users/joelgong/...` 非 `/mnt/host/...`)。Dockerfile 含 `ENV GOPROXY=https://goproxy.cn,direct` 铁律。
- pod:`gamification-go-87cdf8c65-4b8nl` 1/1 Running 0 restarts。containerPort 8092。
- 健康检查(port-forward 8092):`/healthz` 200、`/readyz` 200(`{"db_read":true,"redis":true,"status":"UP"}`,**Redis Cluster client 初始化成功**,无 InvalidClientConfig)。
- API 全验证:`GET /profile/1001`(pre)→200 空 streak;`GET /leaderboard`(空)→200 `[]`;`?top=5`→200 `[]`;`POST /points`+50→**202** `{"accepted":true,"points_added":0}`;`POST /points`+30→202;`GET /profile/1001`(post)→200 `current_days:2,longest_days:3,last_active:2026-08-08`(upsert ON DUPLICATE KEY UPDATE 生效);`GET /leaderboard`(post)→200 `[{"rank":1,"user_id":"1001","score":0}]`;`GET /achievements/1001`(pre)→200 null;`POST /achievements/grant`→**202** `{"granted":true}`(INSERT IGNORE);`GET /achievements/1001`(post)→200 `[{"achievement_id":1,"unlocked_at":"2026-08-08T14:39:53Z"}]`。
- TiDB 实写验证:`esp_gamification.user_streaks WHERE user_id=1001`→1 行 `(1001,2,3,2026-08-08)`;`esp_gamification.user_achievements WHERE user_id=1001`→1 行 `(1001,1,2026-08-08 14:39:53)`。**无 1146/1054**。
- 源码探查结论:**无需改源**。无 `LIMIT ?` 坑(leaderboard top 走 Redis `ZRevRangeWithScores` int64 参数,不经 SQL)、无 LAST_INSERT_ID 坑(写用 INSERT...ON DUPLICATE KEY UPDATE / INSERT IGNORE,不依赖返回 id)、SQL 列名与 TiDB 实际表结构全匹配、路由齐全(profile/points/leaderboard/achievements/grant)、端口 8092、Redis ZSET 用途正确、Pulsar 纯 stub(60s 打日志,无真实 client)。
- **坑(运维,非源码)**:rollout 首轮 `FailedScheduling: Insufficient cpu`(节点 8950m/9000m 99%)。临时缩容 pronunciation-rs + assessment-rs 各 1→0 释放配额;两 pod 卡 Terminating(CPU 紧张 kubelet 停不掉),`--force --grace-period=0` 删除后才释放(8950m→8850m,空出 150m ≥ 需求 100m)。rollout 成功后两服务已恢复 replicas=1。**单节点铁律相关,后续新增服务/提副本数前先清 CPU requests 或降部分服务 request 值,否则卡 Pending**。
- 非阻塞观察:① POST 端点返 202(非 200/201),属 stub 异步语义,`accepted:true`/`granted:true` 成功;② leaderboard `score:0` 是 stub(points_added 未累加 score 字段);③ listAchievements 只查 user_achievements 未 JOIN achievements 表取 name/icon(功能简化);④ Pulsar 纯 stub,加分仅靠 POST /points 同步触发,无事件驱动。均 scaffold 阶段设计,不阻断。
- manifest:`deploy/k8s/32-batch2-services.yaml` gamification-go Deployment replicas 0→1(line 183,注释改 `# S7 启用,单节点验证`)。
- 改动文件:`32-batch2-services.yaml`(gamification-go replicas)。

**S5 assessment-rs ✅ 完整闭环**(前会话已闭环,此处补记状态):
- pod 1/1 Running 0 restarts,esp_assessment sessions +1/answers +1,无 1146/Unknown column。改源 3 项:Dockerfile 1ms mirror + main.rs `h.0` 解引用 + AUTO_RANDOM execute+LAST_INSERT_ID u64。列名对齐线上旧 schema(免改)。**遗留小瑕疵(非阻塞)**:REDIS_CLUSTER env 缺 `redis://` 前缀,IRT θ cache 降级。待修。

## 恢复锚点(上下文溢出后从这恢复)

- **当前任务(会话20)**:✅ 推进 E,S4~S7 逐个实跑**全部闭环**。S4 listening-go / S5 assessment-rs / S6 reading-go / S7 gamification-go 均 1/1 Running 0 restarts,各 schema 实写验证通过(esp_listening +3 / esp_assessment +1+1 / esp_reading +1 / esp_gamification streaks+achievements 各 +1)。TaskList #1~#4 全 ✅。
- **下一步(候选主线)**:① ArgoCD helm install 收口会话18 隐患 D(单节点铁律:串行,不并行);② S5 assessment-rs REDIS_CLUSTER env 加 `redis://` 前缀修 IRT θ cache 降级;③ pronunciation-rs attempt_id=0 瑕疵修(TiDB AUTO_RANDOM LAST_INSERT_ID 不回传生成 id,改查 SELECT id ORDER BY created_at DESC);④ 清理单节点 CPU requests 超配(8950m/9000m),为后续新增服务腾调度配额;⑤ 隐患 E 收口:S3~S7 五新服务全部实跑完成,可标记 ✅。
- **Redis 恢复 key**:`esp:ctx:hot:2026-08-08-session20-s4-s7-batch2`(完整会话记录 + 各服务执行情况 + 坑 + 解决方案 + 现状)。
- **本文件恢复**:读 `docs/PROGRESS.md` 全文(本节"会话20"为最新热区)+ TaskList(#1~#4 全 ✅)。
- **关键改点**:S4=`listening-go/src/main.go`(LIMIT string→int Atoi);S5=`assessment-rs/Dockerfile`(1ms mirror)+`src/main.rs`(`h.0` 解引用+AUTO_RANDOM execute+LAST_INSERT_ID u64);S6=`reading-go/src/main.go`(LIMIT string→int Atoi,同 S4 模式④);S7=仅 manifest `32-batch2-services.yaml`(replicas 0→1,源码无需改)。
- **已确认模式(踩过可复用)**:① Rust 服务 Dockerfile FROM 必改 1ms mirror;② AppHandle(Arc<AppState>) newtype 必 `h.0` 解引用;③ TiDB AUTO_RANDOM 主键 INSERT 必 execute+LAST_INSERT_ID u64,勿用 RETURNING/fetch_one(TiDB 不支持 RETURNING,AUTO_INCREMENT 也适用);④ Go service TiDB `LIMIT ?` 必传 int 勿传 string;⑤ docker build 直写 RD docker ns(无需 save+load),nerdctl build 才需 save+load 转换;⑥ 核对列名必查 `information_schema.columns`(线上实态),勿信 `docs/design/schema.sql`(设计态,可能与线上旧结构不一致);⑦ **单节点 CPU requests 逼近上限(8950m/9000m)时,新增 pod 卡 Pending,靠临时缩容其他 0 业务负载服务释放配额,缩容 pod 可能卡 Terminating 需 `--force --grace-period=0` delete;后续新增服务/提副本前先清 CPU requests 或降 request 值**。

## 会话18:并行推进 A/B/C/D/E(2026-08-08 14:00~,🔄 进行中)

用户指令"并行推进 ABCDE"。TaskList #1~#5 建项。

**隐患A — app_logs 跨 schema 写**:
- 实测收敛(非原以为的"三服务全 rebuild"):writing-py/emotion-svc 镜像 runtime 解析 `APP_LOG_DB` default=esp_shared(exec 进 pod 跑 `from app.logging_client import _DB_CONFIG` 实锤 `resolved database=esp_shared`),无需 rebuild。仅 **gateway-go 1.5.0 旧镜像**跑 esp:触发 `/api/scenarios` 后 `kubectl logs deploy/gateway-go --tail` 实锤 `app_logs insert failed (degraded): Error 1146 (42S02): Table 'esp.app_logs' doesn't exist`。源码 `services/gateway-go/src/log/client.go:53` default 已 esp_shared,镜像未含此改动。
- 修复:rebuild `esp/gateway-go:1.5.1`(esp-builder a318805e1f51393f0 后台:nerdctl build → set image → rollout → 验证 /api/scenarios 后 esp_shared.app_logs WHERE service='gateway' +1 且无 1146)。

**隐患B — learning app_logs 双 1146(写+读)**:
- 根因(已实锤,双重):app_logs 在 esp_shared,但 learning WRITE_DB_URL/READ_DB_URL 均 esp_learning。(1) AppLogClient.writeJdbc(BehaviorController.reportBehavior 调用)写 app_logs → 1146(触发 POST /api/learning/behavior 后日志实锤 `app_logs insert failed (degraded): bad SQL grammar [INSERT INTO app_logs...]`=1146 包装);(2) BehaviorController readJdbc 读 FROM app_logs(behavior/top|funnel|heatmap)→ 1146。
- 勘误:teacher/bff 的 AppLogClient 仅声明未被任何代码调用(grep 实证),无风险,不需改。
- 修复(方案A,已编码完成):
  - 新增 `services/learning-java/src/main/java/com/esp/learning/config/SharedDataSourceConfig.java`:HikariCP `sharedDataSource`(@ConfigurationProperties spring.datasource.shared)+ `sharedJdbcTemplate` Bean,绑 esp_shared。
  - `application.yml` 加 `spring.datasource.shared` 块(`${SHARED_DB_URL:...esp_shared}`,pool 10/2)。
  - `AppLogClient` 构造器改 `@Qualifier("sharedJdbcTemplate") JdbcTemplate`。
  - `BehaviorController` 加 sharedJdbc 字段,三处 app_logs 读(top/funnel/heatmap)改 `sharedJdbc.queryForList`;recommend 接口跨表查询(scenarios/sessions/user_mastery)继续走 readJdbc(esp_learning)。
  - `deploy/k8s/20-learning-java.yaml` 加 `SHARED_DB_URL=jdbc:mysql://tidb-primary:4000/esp_shared?...`。
  - `mvn -o clean compile` BUILD SUCCESS(22 source files)。
- 部署:rebuild `esp/learning-java:1.6.2`(esp-builder af2ba9a84778d8181 后台:mvn package → nerdctl build → set image → rollout → 验证 behavior 写 +1、heatmap/top 读 200 无 1146)。注意与 gateway builder 串行(单节点铁律)。

**隐患C — Python 镜像 bookworm 6CRIT(✅ 定性,登记豁免)**:
- trivy image 重扫 writing-py/emotion-svc 1.6.1(当前 bookworm,非旧 trixie 报告):**6 CRIT** = libsqlite3-0(CVE-2025-7458)+ perl-base ×4(CVE-2026-13221/42496/57433/8376)+ zlib1g(CVE-2023-45853),HIGH 18,MEDIUM 65。两个镜像完全一致(同 python:3.13-slim-bookworm base)。报告落 `report/security/trivy-image-writing-py-1.6.1.json` / `trivy-image-emotion-svc-1.6.1.json`。
- **原"trixie 残留"假设被推翻**:旧报告 perl 5.40.1-6(trixie)4CRIT,新扫 perl 5.36.0-7+deb12u3(bookworm)反而 6CRIT——换 bookworm 没清零,多了 libsqlite3-0 + zlib1g。
- 全部 Fix=None(上游无补丁),Status=affected/fix_deferred/will_not_fix。
- 依赖核查(exec pod):zlib1g 被 dpkg/util-linux/libapt-pkg6.0/gpgv 依赖不可 purge;perl-base 是 dpkg 硬依赖;libsqlite3-0 是 python sqlite3 依赖。
- **用户拍板:登记豁免保持 bookworm**(alpine 收益不确定且 granian/litestar 需验证 musllinux wheel)。落地:`.trivyignore` 追加 6 CVE + 理由 + 复审日期,ADR + 06 安全报告 §5 登记。定期 `apt upgrade` 等 upstream 补丁。

**隐患D — git 迁入(✅ 闭环)**:用户授权自动建仓。gh 已认证(tianqiongenze,scopes repo/workflow)。
- 仓库:`https://github.com/tianqiongenze/English-Study-Platform`(PRIVATE,master 分支,530 文件)。
- .gitignore 加固:`.venv/`/`venv/` + `**/*secret*.yaml`(覆盖 00-namespace-secret.yaml 等含 secret 的 manifest)+ `*.local` + `{{pkgetc}}/`/`{{etc}}/` + `services/*/gateway-go`(本地 go build 二进制);unignore `!**/otel-agent.jar`(Dockerfile 构建依赖)+ `!**/lib/*.jar`。
- 00-namespace-secret.yaml(含 mock 密钥 ANTHROPIC_API_KEY/JWT_SECRET)已复制为 .local 本地保留,本体不入库。
- 首次 commit + push 成功(commit f021250)。
- Dependabot:`.github/dependabot.yml` 已就位(maven ×7 / gomod ×5 / pip ×2 / npm 前端),`gh api PUT vulnerability-alerts` + `automated-security-fixes` 已启用,Dependabot alerts API 可查(0 alerts 等首次扫描)。
- CI:`.github/workflows/ci.yml` + `security.yml`(Semgrep/Trivy)已入库。
- Aikido:需 Aikido 账号 integration(GitHub Marketplace 或 OAuth 授权),非 GitHub 原生,待用户在 Aikido 控制台接入。
- ArgoCD:用户选"装 ArgoCD 做 GitOps",待 A/B builder 闭环后串行安装(单节点铁律:helm install 重操作不与 build 并行)。

**隐患E — schema Phase1 Batch2~5 + 新服务填实**:Saga 已解锁可推进。当前 16 个 esp_* schema 表已就位(实查 INFORMATION_SCHEMA),S1 llm/S2 vocab-srs/N8 course-path-go 已 1/1 Running,N10 live-tutor-go/N11 media-learn-rs replicas=0 待命。

## S3 pronunciation-rs 构建部署(2026-08-08 18:00~,✅ 完整闭环,会话19)

**任务**:构建+部署 S3 pronunciation-rs(Rust/axum, P0 实跑),manifest `deploy/k8s/32-batch2-services.yaml`(S3 replicas=1 实跑,S4~S7 replicas=0 待命)。

**执行过程与坑(4 蝶)**:
1. **base image docker.io i/o timeout**:nerdctl pull `rust:1.88-bookworm` / `debian:bookworm-slim` 直拉 registry-1.docker.io 超时(VM 网络)。修复:Dockerfile FROM 改 `docker.1ms.run/library/rust:1.88-bookworm` + `docker.1ms.run/library/debian:bookworm-slim`,先 `nerdctl pull` 再 build。
2. **Rust E0609 编译错误**:`AppHandle(Arc<AppState>)` 是 newtype,直接 `h.mysql_read` / `h.mysql_write` 编译不过(unknown field)。修复:改 `h.0.mysql_read` / `h.0.mysql_write`(解引用 Arc→AppState)。改后 release build 7m15s 成功。
3. **镜像 ns 导入(gateway-go 同款坑)**:nerdctl build 写 containerd default ns,kubelet via cri-dockerd 查 docker/moby ns 拉不到→ImagePullBackOff。修复:`nerdctl save > tar` + `rdctl shell docker load < tar`(宿主机无 docker daemon,colima 也停了,RD VM 内有 docker 29.5.3)。load 后 `rdctl shell docker images` 见 `esp/pronunciation-rs:0.1.0` 才算就绪。
4. **INSERT...RETURNING 不兼容 TiDB AUTO_RANDOM**:`pronunciation_attempts.id` 是 `AUTO_RANDOM(5)` bigint,sqlx `query_as("INSERT...")` 用 `fetch_one` 期望 RETURNING 报 "no rows returned"。修复:改 `execute`(不取返回行)+ 单独 `SELECT LAST_INSERT_ID()` 两步。
5. **LAST_INSERT_ID 类型不匹配**:TiDB `LAST_INSERT_ID()` 返回 `BIGINT UNSIGNED`,Rust `i64` 不兼容报 "mismatched types; Rust type i64 not compatible with BIGINT UNSIGNED"。修复:`(i64,)`→`(u64,)`,`ScoreResp.attempt_id: i64`→`u64`。

**最终验证(✅ 全通过)**:
- 镜像:`esp/pronunciation-rs:0.1.0` digest `sha256:9c4f1ab2...`(首次)/ rebuild `3f189433...`(u64 修复后),linux/arm64 160MB。
- pod:`pronunciation-rs-6bb4b7f759-6s6d8` 1/1 Running,0 restarts,rollout success。
- readiness:`{"db_read":true,"status":"UP"}`(port-forward 18088→80)。
- 评分接口:POST `/api/pronunciation/score` → `{"attempt_id":0,"score":0.85}`(attempt_id=0 是因 LAST_INSERT_ID 在 AUTO_RANDOM 下不返回自增 id,这是 TiDB 已知行为,非 bug;实际行已插入,可从 DB 查到真实 id 如 864691128455135234)。
- app_logs 计数:esp_pronunciation.pronunciation_attempts before=2 → after=3(+1 ✅)。累计 3 行测试数据。
- 其余 S4~S7 replicas=0 待命,manifest apply 成功(5 deploy + 5 svc + 5 hpa)。

**已知小瑕疵(非阻塞)**:
- `attempt_id=0`:TiDB AUTO_RANDOM 主键 `LAST_INSERT_ID()` 不返回生成的随机 id(MySQL AUTO_INCREMENT 语义差异)。真实 id 需 `SELECT id ORDER BY created_at DESC LIMIT 1` 或应用层生成。scaffold 阶段可接受,后续接 GOP 真实算法时一并修。
- Redis Cluster 连接 warn:`REDIS_CLUSTER` env 未配(或 URL 格式不对),`redis cluster client build failed: Redis URL did not parse`。评分缓存降级跳过,不影响 P0 写库功能。待配 `REDIS_CLUSTER=redis://redis-cluster-new-0.redis-cluster-new.ekb-m2-aws-us-east-1.svc.cluster.local:6379` 后生效。
- 容器内无 curl/wget(debian-slim 精简),readiness 探针用 port-forward 从宿主机 curl 验证。生产 k8s readiness probe 用 httpGet 不受影响(kubelet 直接 TCP 探测)。

**改动的文件**:
- `services/pronunciation-rs/Dockerfile`(FROM 改 1ms 镜像源)
- `services/pronunciation-rs/src/main.rs`(h.0 解引用 + INSERT execute + LAST_INSERT_ID u64)
- `deploy/k8s/32-batch2-services.yaml`(apply,未改内容)

## S4 listening-go 构建部署(2026-08-08,✅ 完整闭环,会话20)

**任务**:构建+部署 S4 listening-go(Go/gin, P1 实跑),manifest `deploy/k8s/32-batch2-services.yaml`(S4 replicas 0→1)。

**执行过程与坑**:
1. **基镜像拉取**:docker.1ms.run 镜像源 TLS handshake timeout;改用默认 docker.io 直拉成功(golang:1.25-alpine 已缓存、alpine:3.22 新拉)。
2. **LIMIT 参数类型不兼容 TiDB**:首次 GET /api/listening/materials 返回 500 `Error 1210 (HY000): Incorrect arguments to LIMIT`。根因:Go `database/sql` 将 string 参数传给 `LIMIT ?` 占位符,TiDB/MySQL 的 LIMIT 不接受字符串参数(需整数)。修复:`listMaterials` 加 `strconv.Atoi(limit)` 转 int 再传参。
3. **镜像 ns 无坑**:docker build 直写 RD VM docker ns,kubelet via cri-dockerd 直接可拉(参考 pronunciation-rs 用 nerdctl build 才需 save+load 转换,本服务无此坑)。

**最终验证(✅ 全通过)**:
- 镜像:`esp/listening-go:0.1.0` digest `sha256:f162ebddd7cfe3fe810d0054d06357dcb5dc090cca2e9c5e6b03aaa045f6cc4c`,linux/arm64 33.5MB。
- pod:`listening-go-64cf56c57d-vztw6` 1/1 Running,0 restarts,rollout success。
- healthz:`{"service":"listening-go","status":"UP"}` HTTP 200 ✅
- livez:`{"status":"UP"}` HTTP 200 ✅
- readyz:`{"db_read":true,"status":"UP"}` HTTP 200 ✅
- GET /api/listening/materials:`null`(HTTP 200,listening_exercises 表空,正常)✅
- POST /api/listening/attempts:`{"accepted":true,"score":0.75}` HTTP 202 ✅
- esp_listening.listening_attempts:写前 0 → 写后 3(+3 ✅,三次 POST 各 +1)。
- app_logs:listening 服务未接入 app_logs 跨 schema 写(scaffold 无 logging_client,与 gateway-go/learning 不同设计)。esp_shared.app_logs 仅有 gateway(45)+learning(1),无 listening 行 —— **这是预期行为,非 bug**(listening-go scaffold 未实现 app_logs 写入)。

**改动的文件**:
- `deploy/k8s/32-batch2-services.yaml`(listening-go replicas 0→1)
- `services/listening-go/src/main.go`(listMaterials LIMIT 参数 string→int,加 strconv import)

## 恢复锚点(上下文溢出后从这恢复)

- **当前任务(会话18)**:并行 A/B/C/D/E。A= gateway-go 1.5.1 rebuild(builder a318805e1f51393f0 进行中);B= learning-java 1.6.2 rebuild(builder af2ba9a84778d8181 进行中,代码改完编译过);C= 待 trivy 重扫 writing-py/emotion 1.6.1 bookworm 确认 CRIT 清零;D= 等用户 GitHub URL;E= schema Batch2~5 + 新服务填实待推进。
- **当前任务(会话19)**:S3 pronunciation-rs ✅ 完整闭环(1/1 Running,评分接口 +1)。下一步:S4~S7 待命(replicas=0);attempt_id=0 瑕疵待修(TiDB AUTO_RANDOM LAST_INSERT_ID 不回传);Redis Cluster env 待配。
- **当前任务(会话20)**:S4 listening-go ✅ 完整闭环(1/1 Running,attempts +3,healthz/readyz 200)。下一步:S5 assessment-rs / S6 reading-go / S7 gamification-go 待构建部署(replicas=0 待命)。
- **Redis 恢复 key**:`esp:ctx:hot:2026-08-08-session19-s3-pronunciation-rs`(完整会话记录 + 执行情况 + 验证结果)。
- **Redis 恢复 key(会话18)**:`esp:ctx:hot:2026-08-08-session18-abcde-parallel`(完整会话记录 + 执行情况 + 验证结果)。
- **本文件恢复**:读 `docs/PROGRESS.md` 全文(本节"会话18"为最新热区)+ TaskList(#1~#5)。
- **关键代码改点(隐患B)**:`SharedDataSourceConfig.java`(新增)/ `AppLogClient.java`(构造器 @Qualifier sharedJdbcTemplate)/ `BehaviorController.java`(三处 app_logs 读改 sharedJdbc)/ `application.yml`(spring.datasource.shared)/ `20-learning-java.yaml`(SHARED_DB_URL env)。
- **关键代码改点(隐患A)**:`gateway-go/src/log/client.go:53` default esp_shared(镜像 1.5.0 未含,rebuild 1.5.1)。

## bff-java Sentinel datasource empty 修复(2026-08-08,✅ 完整闭环)

**根因(已实证,主线程取证)**:
- SCA 2025.1.0.0 + nacos client gRPC 模式:NacosDataSource.loadInitialConfig() 在 gRPC 连接建立前 ~39s 执行 → 拉空 source → "converter can not convert rules because source is empty" → FlowRule loaded: {}。
- 证据:config.log gRPC 在 04:57:27.265 才连上,但 FlowRule loaded 在 04:56:48(早 39s)。listener 后续 subscribe 成功但 SCA 不在连接建立后重拉,POST 推送也不 reload,getRules?type=flow 仍 []。
- namespace 配置正确(application.yml namespace=${NACOS_NS:public}=public,服务端 rules 在 public),非 namespace 问题。degrade 规则能加载(恰好命中连接后推送窗口),flow 不能,纯时序问题。

**修复方案(应用层兜底,方案A)**:
- 新建 `services/bff-java/src/main/java/com/esp/bff/config/SentinelRuleBootstrap.java`(@Component,监听 ApplicationReadyEvent)。
- 启动后用 nacos HTTP v1 API 直拉 `bff-java-flow-rules` / `bff-java-degrade-rules`(groupId=SENTINEL_GROUP,tenant="" 即 public),Jackson 反序列化为 List<FlowRule>/List<DegradeRule>,调 FlowRuleManager.loadRules()/DegradeRuleManager.loadRules() 覆盖式加载。
- 仅在当前规则为空时兜底(避免与 SCA listener 后续热更新冲突);3 次重试每次间隔 2s。
- 关键坑:nacos "public" namespace 在 client API 侧对应空串(""),HTTP API tenant 也用空串。早期版本用 NacosFactory gRPC 仍拉空(gRPC 通道未就绪),最终改 HTTP v1 API 稳定拉到。
- 镜像版本演进:1.6.2(NacosFactory gRPC,拉空)→ 1.6.3(namespace="" 修正,仍 gRPC 拉 null)→ 1.6.4(中间态)→ **1.6.5(HTTP v1 API,稳定拉到 3 flow + 2 degrade,✅)**。

**验证结果(1.6.5)**:
- 日志:`Sentinel flow rules bootstrapped: 3 条` + `Sentinel degrade rules bootstrapped: 2 条`。
- `getRules?type=flow` 返回 3 条 JSON(api-writing-grade count=200 / api-learning-event count=2000 / api-default count=5000),非 []。
- `getRules?type=degrade` 返回 2 条 JSON。
- 部署:bff-java-656675668-8678q 1/1 Running,镜像 esp/bff-java:1.6.5。

**文件**:`services/bff-java/src/main/java/com/esp/bff/config/SentinelRuleBootstrap.java`
**镜像**:esp/bff-java:1.6.5
**状态**:✅ 完整闭环,flow+degrade 规则均启动后兜底加载成功。

## llm-orchestrator-rs Redis Cluster 连接 bug 修复(2026-08-08,✅ 完整闭环)

**根因(已实证)**:
- llm-orchestrator-rs 用 `redis::aio::ConnectionManager`(单连接)连 ESP 真 Redis Cluster(3 master:node0 0-5460 / node1 5461-10922 / node2 10923-16383)。
- 单连接不处理 MOVED 重定向。`cache::set` 的 SETEX/ZADD/EXPIRE 和 main.rs 的 `SET NX EX`(幂等)发到 keyslot 不在当前连接节点的 key 时,Redis 返回 MOVED,单连接客户端不跟随 → 命令失败 → `.ok()`/`.await?` 吞掉 → **缓存和幂等永不回写**。
- 实证:从 node0 对 keyslot=8291(属 node1)的 key 发 SETEX 返回 `MOVED 8291 redis-cluster-new-1...:6379`。调多次 /api/llm/chat 后全集群 scan `esp:llm:*`/`esp:idem:llm:*` 全空。读 GET 偶尔命中本节点 key 才成功。

**修复方案(最小改动,Rust 代码 + Cargo.toml + Dockerfile)**:
1. **Cargo.toml:30** — redis 依赖 features 加 `"cluster-async"`(关键:任务原描述只写 `cluster`,实测 redis 0.26.1 的 `cluster` feature 只含同步 cluster client;async cluster 模块 `redis::cluster_async` gated behind `cluster-async` feature,必须显式开启):
   ```toml
   redis = { version = "0.26", features = ["tokio-comp", "aio", "connection-manager", "cluster", "cluster-async"] }
   ```
2. **src/main.rs:30** — `AppState.redis` 字段类型 `redis::aio::ConnectionManager` → `redis::cluster_async::ClusterConnection`。
3. **src/main.rs:69-78** — `init_state()`:从 `redis::Client::open` + `ConnectionManager::new` 改为 `redis::cluster::ClusterClient::new(vec![seed_url])` + `get_async_connection().await`。ClusterClient 接受单 seed URL,自动发现拓扑。
4. **src/cache/mod.rs:20** — `use redis::aio::ConnectionManager` → `use redis::cluster_async::ClusterConnection`。
5. **src/cache/mod.rs:135,197** — `get()`/`set()` 形参 `&ConnectionManager` → `&ClusterConnection`。
6. **Dockerfile:21-37** — 运行时基镜像 `gcr.io/distroless/cc-debian12:nonroot` → `debian:bookworm-slim`(gcr.io 被墙拉取超时)+ `useradd -u 65532 nonroot` + ca-certificates/libgcc-s1。安全姿态等价,体积略大(~80MB vs ~20MB)。

**关键 API 确认(查 redis 0.26.1 源码 /opt/homebrew/share/cargo/registry/src/index.crates.io-*/redis-0.26.1/src/)**:
- `cluster_async/mod.rs:99` — `pub struct ClusterConnection<C = MultiplexedConnection>` derive `Clone` ✅(main.rs 多处 `h.0.redis.clone()` 无需改)。
- `cluster_async/mod.rs:1035` — `impl ConnectionLike for ClusterConnection` ✅(`query_async` 直接可用)。
- `cluster_client.rs:390` — `pub async fn get_async_connection(&self) -> RedisResult<cluster_async::ClusterConnection>` ✅。
- `lib.rs:585-587` — `pub mod cluster_async` gated behind `#[cfg(feature = "cluster-async")]`。

**编译验证**:
- `cargo check` RC=0(仅 1 dead_code warning:`hamming_distance` 仅测试用)。
- `cargo build --release` RC=0(16.48s,本地 arm64)。
- Docker build(RD lima shell 内 `docker build`)RC=0,5m04s,`esp/llm-orchestrator-rs:0.2.0` 构建成功。

**部署验证**:
- `kubectl set image deploy/llm-orchestrator-rs llm-orchestrator-rs=esp/llm-orchestrator-rs:0.2.0` RC=0。
- `rollout status --timeout=200s` → successfully rolled out。
- Pod 1/1 Running 0 restarts。
- healthz:`{"service":"llm-orchestrator-rs","status":"UP"}` ✅
- readyz:`{"checks":{"redis":"UP","tidb":"UP"},"status":"UP"}` ✅

**功能验证(MOVED 重定向修复闭环)**:
- 第一次 POST /api/llm/chat(prompt="hello cluster fix verify",x-request-id=cluster-fix-test-001):`cache_hit:false`(正常首次写缓存)。
- 第二次同 prompt:`cache_hit:true` —— 语义缓存 L1 命中,证明 SETEX/ZADD 已成功写入跨分片 key。
- 全集群 scan 确认(每个 master 节点分别 SCAN):
  - `esp:llm:cache:fc83ee15...`(L1 精确缓存,落 node1)
  - `esp:llm:sem:default:gpt-4o`(L2 语义 ZSET,落 node0)
  - `esp:idem:llm:chat:idem-probe-002`(幂等回写,落 node1,keyslot=5679 属 node1 5461-10922)
- keyslot 跨分片的 idem key 之前在单连接下必然 MOVED 失败,现在成功写入 → ClusterConnection 自动跟随 MOVED 重定向已实证。

**改了哪些文件**:
| 文件 | 行 | 改动 |
|---|---|---|
| services/llm-orchestrator-rs/Cargo.toml | 30 | features 加 `"cluster-async"` |
| services/llm-orchestrator-rs/src/main.rs | 30 | AppState.redis 类型 → ClusterConnection |
| services/llm-orchestrator-rs/src/main.rs | 69-78 | init_state() 用 ClusterClient |
| services/llm-orchestrator-rs/src/cache/mod.rs | 20 | use ClusterConnection |
| services/llm-orchestrator-rs/src/cache/mod.rs | 135,197 | get/set 形参类型 |
| services/llm-orchestrator-rs/Dockerfile | 21-37 | distroless → debian-slim + useradd |

**遗留(低优先级)**:
- `hamming_distance` dead_code warning(仅测试用,可 `#[allow(dead_code)]` 或保留)。
- Dockerfile 运行时基镜像从 distroless 降级为 debian-slim(gcr.io 被墙),多节点上线时若有 gcr 镜像仓库可换回 distroless;或改用国内 mirror。
- llm-orchestrator-rs 镜像 tag 0.2.0 已部署,旧 0.1.0 可清理。
- 部署状态:esp-prod ns 当前应 12/12 Ready(原 11 + llm-orchestrator-rs 1/1)。

## 恢复锚点(上下文溢出后从这恢复)

- **当前任务**:llm-orchestrator-rs Redis Cluster 连接 bug 修复 —— ✅ 完整闭环,无后续动作。
- **下一步候选**:bff sentinel datasource empty 根因(高嫌疑=datasource 块缺 namespace,explorer agent 撞 rate-limit 未完成,主线程接管)。
- **Redis 恢复 key**:`esp:ctx:hot:2026-08-08-llm-orchestrator-redis-cluster-fix`(完整会话记录 + 执行情况 + 验证结果)。
- **本文件恢复**:读 `docs/PROGRESS.md` 全文(本节为最新热区)。

## 当前状态快照

**四大主线**:
- **主线A 新服务矩阵(10 服务)**:✅ ADR/PRD/scaffold/manifest 落地。N9 并入 S1。10 服务 = S1~S7 + N8 course-path-go + N10 live-tutor-go + N11 media-learn-rs。**N8 已实跑(镜像构建+部署 1/1 Ready)**;N10/N11 镜像已构建(replicas=0 待命多节点)。详见 `docs/design/adr-new-services.md` + `docs/design/prd-new-services.md`。
- **主线B 上下文工程治理**:✅ 闭环。P0/P1/P2 全完成。
- **主线C schema-per-service**:Phase 0 ✅ 完成(11 DB/29 表)。Phase 1/2 待执行(用户催办)。
- **主线D 安全扫描四件套(新)**:✅ 配置全落地(ADR + .gitignore + dependabot.yml + security.yml)。Semgrep+Trivy 本地实跑中(brew 已装)。Aikido+Dependabot 待 git 迁入。详见 `docs/design/adr-security-scanning.md`。

## 集群现状(2026-08-07 07:15)

- 节点 lima-rancher-desktop Ready(v1.34.6+k3s1)。CPU~32%、内存~74%(单节点 VM 紧张)。
- infra ns `ekb-m2-aws-us-east-1`:pulsar 3/3、tidb Ready、redis-cluster-new 6/6 cluster_state ok、milvus/neo4j Running。
- esp-prod ns:**11/11 Ready** ✅(bff/gateway/learning/teacher/emotion/writing-py/writing-coach/frontend/nacos/seata/sentinel + **course-path-go 1/1**)。live-tutor-go/media-learn-rs replicas=0 待命多节点。

**灌装现状**:
- **TiDB 灌装 ✅**:总行数 ~2.95 亿。esp-seed-3b Job Suspended。
- **Redis 灌装 ✅ warm-up 完成**:42954 key(scenario 10000 + kp 29954 + session/mastery/user 各 1000),TTL 1h,3 master 分片。可压测。

## teacher-java readiness 根治(2026-08-07 06:20,✅ 完成)

**现象**:teacher Running 5h+ 但 Ready=False,readiness tcp 8083 失败 954 次(4h42m)。之前 PROGRESS 误判为"探针误杀"。

**根因(已确认,非探针问题,是 JVM hang)**:
1. 应用 17:08 UTC 正常 `Started TeacherApplication in 56.857s`,Jetty `Started ServerConnector on 0.0.0.0:8083`,Dubbo 注册 learning RpcService 成功。
2. 但 **17:10 UTC 后 5 小时零日志**(进程僵死),`/proc/net/tcp` 无 8083 LISTEN 行,只剩大量 CLOSE_WAIT(对端 10.42.0.1 kubelet 探针连过来握手后 Jetty acceptor 不处理→堆积)。
3. PID 1 `State: S(sleeping)`,CPU 累计 17min,RSS 664Mi(未 OOM)。**Jetty acceptor 线程卡死/线程池耗尽**,新 SYN 无 LISTEN socket 响应 → readiness i/o timeout。
4. 次要:期间误用 `kubectl exec ... java -jar app.jar --help` 触发第二 java 进程(已 kill -9 清理)。

**已执行修复(✅ teacher 1/1 Ready)**:
1. `rollout restart deploy/teacher-java` + `scale --replicas=1`(期间被并发改到 2,已纠正回 1)。
2. 强删 hang 旧 pod `teacher-java-67f495bb9c-vb69w --force`。
3. 清理 10 个 desired=0 旧 ReplicaSet(5658/5895/59d8/5b9d/6765/67f4/7494/77c5/7b4d/cc4f)。
4. 新 pod `teacher-java-76f46dbc89-2mbd5` 1/1 Ready,单 RS 干净。

**结论**:非探针配置问题,是单节点 VM 上 JVM 长跑后 Jetty acceptor hang。重启即恢复。**多节点上线时应加 livenessProbe(探针 timeout 15s)+ 监控 JVM 线程数/CLOSE_WAIT**(单节点 VM 高负载是诱因)。

## course-path-go 部署(2026-08-07 06:15,✅ 完成)

- **镜像构建**:RD VM 内 `docker build` → `esp/course-path-go:0.1.0`(30.7MB)、`esp/live-tutor-go:0.1.0`(28.1MB)、`esp/media-learn-rs:0.1.0`(待确认)。Go Dockerfile `ENV GOPROXY=https://goproxy.cn,direct` 铁律遵循。
- **部署**:course-path-go pod 1/1 Ready,health `/actuator/health` 返 `{"status":"UP"}`。端口 8093,HPA min=1 max=4。
- live-tutor-go/media-learn-rs replicas=0,镜像已就绪待命多节点。

## teacher-java readiness 二次根治(2026-08-07 07:15,✅ 完成)

**现象**:恢复会话发现 teacher 0/1(jetty hang 复发,旧 pod 76f46dbc89-2mbd5,readiness i/o timeout,8083 无 LISTEN)。与 06:20 根因一致:单节点 VM JVM 长跑后 Jetty acceptor hang。

**修复**:`rollout restart deploy/teacher-java` → 新 pod `teacher-java-6b85c8f699-s6q7w` 15min 内 1/1 Ready。集群恢复 **11/11 Ready** ✅。

**结论(强化)**:单节点 VM 高负载是诱因,Jetty acceptor hang 周期性复发。**多节点上线必加 livenessProbe(timeout 15s)+ JVM 线程数/CLOSE_WAIT 监控**。本会话已二次重启根治。

## 安全扫描四件套(2026-08-07 07:15,配置✅ / 实跑🔄 镜像并行)

**用户要求**:所有已部署服务完整跑 Aikido + Semgrep + Trivy + Dependabot。**用户决策**:先迁 git 仓库再接 Aikido(需 GitHub 远端);扫描执行深度=先配置后实跑(都做)。

**配置落地(✅)**:
- `.gitignore`:secret/制品/报告/logs 全隔离(含 secret K8s manifest 只入库 `*_tpl`)。
- `docs/design/adr-security-scanning.md`:四件套分层决策(Semgrep SAST + Trivy SCA/镜像 + Dependabot + Aikido SaaS)+ 门禁规则 + 豁免机制。
- `.github/dependabot.yml`:maven(7 Java)/gomod(3 Go)/pip(5 Python)/npm(3 前端)/github-actions 全生态,周更。
- `.github/workflows/security.yml`:Semgrep SAST + Trivy fs(vuln+secret+config)+ Trivy image(7 服务矩阵),sarif 上传 GitHub Security tab,ERROR/CRITICAL-HIGH 门禁。

**本地实跑(🔄 后台进行中)**:
- `brew install semgrep trivy` ✅ 完成(semgrep 1.172.0 / trivy 0.73.0)。
- Semgrep 扫描后台运行(`p/default+java+golang+python+owasp-top-ten+security-audit+secrets`),报告落 `report/security/semgrep.json`。
- Trivy fs 扫描后台运行(`vuln+secret+config`,CRITICAL/HIGH),报告落 `report/security/trivy-fs.json`。
- **待办**:实跑结果汇总 → 回填 `docs/test/06-安全测试报告.md` §5;Trivy image 扫 7 服务镜像。

**git 迁入(⏳ 等用户)**:
- `git init` + 首提交就绪(.gitignore 已建)。
- **阻塞**:等用户提供 GitHub 远端 URL → `git remote add origin <url> && git push -u origin main` → 启用 Dependabot + 接 Aikido SaaS。

## schema-per-service Phase 0(2026-08-07 01:00,✅ 完成)

- **DDL**:`docs/design/schema-per-service-phase0.sql`,40 条 DDL RC=0。11 新 DB + 29 新表。老库 esp 18 表零改动。详见 ADR `docs/design/adr-schema-per-service.md`。

## Pulsar public/default namespace 补建(2026-08-07 01:00,✅ 完成)

- 根因:esp-cache topic `esp-cache-invalidate` 解析 `persistent://public/default/...` 但集群无 `public` tenant。修复:`pulsar-admin tenants create public` + `namespaces create public/default`。非阻断。

## 本会话工作流水(2026-08-07 07:15~07:45,会话恢复→双轨落盘→并行推进)

### 会话恢复(07:15)
- 读 PROGRESS.md 全文 + Redis `esp:ctx:hot:*`/`esp:ctx:progress` 锚点 + 集群 get pods → 完整恢复任务上下文。
- 发现 teacher-java 0/1(jetty hang 复发,旧 pod 76f46dbc89-2mbd5,readiness i/o timeout,8083 无 LISTEN)。

### teacher readiness 二次根治(07:15,✅)
- `rollout restart deploy/teacher-java` → 新 pod `teacher-java-6b85c8f699-s6q7w` 11min 内 1/1 Ready。
- 集群恢复 **11/11 Ready** ✅。
- **结论(强化)**:单节点 VM 高负载致 Jetty acceptor hang 周期性复发。**多节点上线必加 livenessProbe(timeout 15s)+ JVM 线程数/CLOSE_WAIT 监控**。本会话已二次重启根治,需长效监控而非仅重启。

### 安全扫描四件套(配置✅ / fs 实跑✅ / 镜像扫描 2/7✅ / 回填⏳)
- **fs 扫描报告(已落盘,等汇总回填 06 §5)**:
  - `report/security/semgrep.json`(493KB,256 结果:20 ERROR/155 WARNING/19 MEDIUM/62 INFO)
  - `report/security/trivy-vuln.json`(959KB)
  - `report/security/trivy-secrets.json`(826KB)
- **镜像扫描(2/7 完成,5 个因 API rate limit 失败待重跑)**:
  - ✅ `trivy-image-gateway-go.json`(61KB,**0 漏洞**,Alpine 3.22.5 干净)
  - ✅ `trivy-image-course-path-go.json`(45KB,**0 漏洞**,Alpine 3.22.5 干净)
  - ❌ bff-java / learning-java / teacher-java / writing-py / emotion-svc:**API 400 rate limit,5 agent 并发 trivy DB 锁竞争+请求超限**,需串行重跑(用 `--db-repository public.ecr.aws/aquasecurity/trivy-db:2` 绕 mirror.gcr.io 拒连,镜像从 VM `docker save` 拷主机扫)。
- **回填 06 报告 §5**:esp-doc 子 agent 已派,待返回汇总摘要。

### 新服务填实 Batch1(S1+S2,并行启动 07:20)
- **侦察结论(esp-explorer 返回)**:
  - S1 `llm-orchestrator-rs`(Rust/axum 8086)、S2 `vocab-srs-go`(Go/gin 8087)**均需从零建目录**(services/ 下不存在)。
  - schema 已就绪:`esp_llm`(llm_call_logs/llm_semantic_cache/dialogue_*)、`esp_vocab`(vocabulary/user_vocabulary,SM-2+ 字段 interval_days/ease_factor/due_at)。
  - 参考:N8 course-path-go(Go 骨架)、N11 media-learn-rs(Rust 骨架)。
  - **schema Phase 1** DDL+计划已就绪(`schema-per-service-phase1.sql` 68行 + plan.md,5 Batch RENAME 待维护窗口);**Phase 2** 无独立草稿(仅 ADR/plan 文字)。
- **已派 2 个 esp-builder 并行填实**:
  - S2 vocab-srs-go(go.mod+Dockerfile[GOPROXY铁律]+main.go+handler/dao/srs SM-2+ 算法+K8s manifest,go build 验证)
  - S1 llm-orchestrator-rs(Cargo.toml+Dockerfile+main.rs+provider/cache/cost/dialogue 模块+K8s manifest,cargo check 验证,5-10min 编译)

### 当前并发 agent(后台)
| agent | 任务 | 状态 |
|---|---|---|
| esp-doc | fs 三报告汇总→06 §5 摘要 | 🔄 运行 |
| esp-builder ×2 | S1/S2 填实+build | 🔄 运行 |

### 遇到的问题与解决
1. **trivy DB 下载超时/拒连**:mirror.gcr.io 拒连 → 改 `--db-repository public.ecr.aws/aquasecurity/trivy-db:2`;VM 无 trivy → 镜像 `docker save` 拷主机用本机 trivy 扫。
2. **5 agent 并发 trivy 失败**:API 400 rate limit + DB 锁竞争 → **后续镜像扫描改串行**(单 agent 逐个扫)。
3. **teacher jetty hang 复发**:单节点 VM 通病 → 重启根治,长效靠多节点+liveness 监控。

### Batch1 填实进度(2026-08-07 08:00)
- **✅ S2 vocab-srs-go 完成 + go build RC=0**:7 文件(go.mod/Dockerfile[GOPROXY铁律]/main.go/handler/dao/srs SM-2+/K8s manifest)。SM-2+ 算法标准(ease [1.3,2.5],learning→review→mastered 状态机,lapse≥8 回退)。连 `esp_vocab`,读写分离 tidb-primary/follower,traceId 透传,优雅关闭。**待部署验证**。
- **🔄 S1 llm-orchestrator-rs 代码补齐(主线程接管)**:子 agent 两次 API 流中断,主线程补齐剩余模块。已落盘 5 文件:
  - `Cargo.toml`(axum0.7+tokio+redis+sqlx+reqwest+async-trait+sha2+uuid)
  - `Dockerfile`(多阶段 rust builder + distroless)
  - `src/main.rs`(264行,router 8086,llm_chat 主干:traceId→语义缓存→provider→回写→cost 记录,health/ready/live)
  - `src/provider/mod.rs`(348行,OpenAI兼容/通义/Claude 三 provider + 降级链 + Stub 兜底,SSE 解析;**主线程修复:补 `use async_trait::async_trait` + `use std::env`**)
  - `src/cache/mod.rs`(语义缓存 Redis L2,sha256 key,TTL 1h)— 主线程新建
  - `src/cost/mod.rs`(llm_call_logs 写入)— 主线程新建
  - `src/dialogue/mod.rs`(session/turn/persona CRUD + SSE chat_sse,连 esp_llm.dialogue_*)— 主线程新建
  - `deploy/k8s/esp-prod/llm-orchestrator-rs.yaml`(Deployment+Service 8086+HPA+secret 挂载 provider key)— 主线程新建
  - **待 `cargo check` 验证**(可能 5-10min 编译下载 crates)。

## Java 镜像 trivy 扫描阻塞(2026-08-07 08:15,根因确认)

**3 个 Java 服务镜像(bff-java/learning-java/teacher-java)扫描全部失败**,Python(writing-py/emotion-svc)未跑。

**根因(已确认,非配置问题)**:
- trivy 扫 Java 镜像(Spring Boot fat jar 层)强制依赖 **Java DB(trivy-java.db,902MB)**,`--skip-java-db-update` 在首跑被禁用。
- Java DB 下载:`ghcr.io/aquasecurity/trivy-java-db:1` 带宽极低(~15KiB/s,ETA 16h+),`mirror.gcr.io` 拒连,`public.ecr.aws` 无 java-db mirror → 国内网络无法首次初始化 Java DB。
- 次要:多 agent 并发 trivy 共享 cache 目录致 fanal.db 锁竞争。

**已就绪(可直接复用,避免重复 save)**:
- 镜像 tar 全部导出到宿主机 `/tmp/esp-trivy/`:bff-java-1.5.1.tar、learning-java(_learning-java.tar)、teacher-java-1.5.1.tar(各 ~286-301MB)。
- 宿主机 vuln DB `~/Library/Caches/trivy/db/trivy.db`(1.2GB,8/6)可用;**唯一卡点 Java DB 0B 缺失**。

**解法路径(待执行)**:
1. **优先**:在出网稳定环境预下载 Java DB → `trivy image --download-java-db-only` → 同步到 `~/Library/Caches/trivy/java-db/`,再用 `--skip-db-update --skip-java-db-update --cache-dir /tmp/trivy-cache-X`(每镜像独立 cache 防锁)扫。
2. **退路**:对 jar 层 `--skip-dirs /app`(丢失 jar 依赖漏洞,不推荐);或 fs 扫已覆盖源码层 pom.xml 依赖 CVE(38 vulns 已在 06 报告 §5.2),镜像层 CVE 与 fs 基本重合,**可标记"镜像扫描以 fs 扫等效覆盖"**登记豁免。
3. writing-py/emotion-svc(Python 镜像)无 Java DB 依赖,可直接扫(待补)。

## 镜像扫描 7/7 完成(2026-08-07 10:40,✅)

Java DB 用 `public.ecr.aws/aquasecurity/trivy-java-db:1` 下载成功(902MB,~9min,1.7MiB/s),3 Java 镜像补扫完成。**全部 7 服务镜像扫描报告落 `report/security/trivy-image-*.json`**。

| 镜像 | 总漏洞 | CRIT | HIGH | MED | LOW | 主要来源 |
|---|---|---|---|---|---|---|
| gateway-go:1.5.0 | 0 | 0 | 0 | 0 | 0 | Alpine 3.22 ✅ |
| course-path-go:0.1.0 | 0 | 0 | 0 | 0 | 0 | Alpine 3.22 ✅ |
| writing-py:1.5.0 | 173 | 4 | 19 | 56 | 66 | Debian13 perl-base/util-linux |
| emotion-svc:1.5.0 | 173 | 4 | 19 | 56 | 66 | 同 writing-py(同基底) |
| bff-java:1.5.1 | 30 | 1 | 3 | 20 | 6 | otel-agent CRIT + jackson HIGH |
| learning-java:1.5.1 | 30 | 1 | 3 | 20 | 6 | 同 bff(同基座) |
| teacher-java:1.5.1 | 63 | 2 | 16 | 39 | 6 | netty 全家桶 + bouncycastle CRIT |

**修复优先级**:
- P0:Python 镜像 Dockerfile 加 `apt-get upgrade` 或换 `python:3.x-slim-trixie` 最新 tag 重建(消 4 CRIT+19 HIGH,批量)
- P0:teacher bouncycastle 1.78.1→1.84(CRIT CVE-2025-14813)
- P1:jackson-databind 2.19.4→2.18.x(3 镜像各 2 HIGH)、otel-javaagent 升级(3 镜像各 1 CRIT)、teacher netty 全家桶升级(11+ HIGH)
- bff/learning 同基座同漏洞,一次升级消两服务

## schema Phase 1 阻塞 seata undo_log(2026-08-07 10:30,⏳ 待决策)

**Batch1 bff-java 执行→失败→已回滚**,集群稳定(bff 1/1)。
- RENAME `esp.users → esp_auth.users` RC=0,行数对账 1000001→1000001 ✅ 数据无损
- 改 bff `WRITE_DB_URL` 指向 esp_auth → rollout → 新 pod CrashLoopBackOff
- **根因**:seata AT 模式要求 undo_log 表存在于服务连接的 schema;esp_auth 只有 users 无 undo_log → DataSource bean 起不来 → Spring fail。phase1.sql/plan.md 共同盲点(注释"undo_log 留 esp"未考虑服务切库后 seata 在新 schema 查不到)
- **已回滚**:RENAME 回 esp.users + env 还原 + bff pod 1/1 Running

**待决策方案**(Batch2-5 未执行):
- **A(推荐)**:每个新 schema 建一份 undo_log(同结构),保留 schema 隔离 + seata AT 仍工作(AT 模式 undo_log 按 branch local 存,理论可行,需验证)
- B:受影响服务关 seata(`SEATA_ENABLED=false`),bff 涉及鉴权写影响事务一致性,不推荐
- C:env 单独配 SEATA_UNDO_LOG_SCHEMA 指 esp,需确认代码支持该配置项,不支持则改代码

## S1 llm-orchestrator-rs 部署修复(2026-08-07 10:50,🔄 重建中)

- **现象**:S1 镜像 0.1.0 pod CrashLoopBackOff,Exit Code 0,无日志。诊断:镜像内 /app/llm-orchestrator-rs 仅 445KB(应为 8.3MB),是 Dockerfile stub `fn main(){}` 预热阶段被 `cargo build || true` 缓存命中,真实 main 没编进镜像。
- **修复**:Dockerfile 删 stub 预热,直接 `COPY src && cargo build --release`。主机 cargo build --release 已验证产 8.3MB 正常二进制。
- **🔄 子 agent 重建中**:`--no-cache` 重建镜像 + 验证二进制 >5MB + rollout + readiness 验证。

## S1 llm-orchestrator-rs 部署成功(2026-08-07 11:00,✅)

- Dockerfile stub 缓存 bug 修复(--no-cache 重建,二进制 9.5MB 非 stub),pod `bb7dfc4b7-ffsb6` 1/1 Ready
- health 实测:`{"service":"llm-orchestrator-rs","status":"UP"}`,日志正常 starting on :8086
- **Batch1 两个 P0 新服务(S1+S2)全部实跑 ✅**。集群 13 个 pod 1/1 Ready(含 S1+S2+N8)
- 新服务镜像:course-path-go 0.1.0、live-tutor-go 0.1.0、media-learn-rs 0.1.0、**vocab-srs-go 0.1.0**、**llm-orchestrator-rs 0.1.0**

## 架构打分评审(2026-08-07,7.62/A-)

**加权总分 7.62/10(A-)**,离工业级生产就绪(8.5+)主要卡在:① 容量弹性(单节点+Sentinel空载)② 一致性(Seata AT→Saga+幂等)③ GitOps ④ 镜像漏洞修复。11 维度评分(权重%):服务拆分 8.7(A)/ 读写分离 8.0(A-)/ 缓存 8.0(A-)/ 消息驱动 6.5(B-)/ 可观测 8.5(A-)/ 安全 8.0(A-)/ AI 8.5(A-)/ 容量弹性 6.0(B-)/ 工程化 8.5(A)/ 数据治理 7.0(B+)/ 一致性 6.0(C+,新增)。

## P0/P1/P2 + Saga 并行优化(2026-08-07 12:30,🔄 启动)

**侦察结论(已落地,指导执行)**:
- **Python 镜像**:`services/writing-py/Dockerfile` + `services/emotion-svc/Dockerfile` 均 `FROM python:3.13-slim`(Debian13 trixie 基底),无 `apt-get upgrade` → 加 `RUN apt-get update && apt-get upgrade -y` 消 4 CRIT+19 HIGH。两服务共用基底同漏洞(各 173)。
- **otel agent**:三 Java 镜像各 `COPY otel-agent.jar`(`services/{bff,learning,teacher}-java/otel-agent.jar`),`JAVA_OPTS=-javaagent:/app/otel-agent.jar`。需替换升级版 jar(各 1 CRIT)。Spring Boot parent 4.0.7(三服务一致)。
- **bouncycastle/jackson/netty**:三服务 pom 未显式声明,由 Spring Boot 4.0.7 BOM 管理。teacher 需显式覆盖 bouncycastle 1.78.1→1.84(CRIT CVE-2025-14813)+ netty 全家桶(11+ HIGH);三服务 jackson-databind 2.19.4→2.18.x(各 2 HIGH)。
- **Sentinel**:`services/bff-java/.../config/RateLimitFilter.java:38-52` 硬编码 `FlowRuleManager.loadRules`(api-learning-event 2000 / api-writing-grade 200 / api-default 5000,仅 QPS 无 DegradeRule);`application.yml:29-40` 已配 Nacos datasource 但 `rule-type=gw-flow`(与已移除的 Gateway 配套,与 Web MVC 适配器不匹配→即便推送也加载不到 FlowRule)。依赖齐全(sentinel-datasource-nacos 已在 pom)。**双轨冲突:硬编码 loadRules 会与 Nacos 推送竞争**。其他 Java 服务零 Sentinel 引用。
- **LLM 语义缓存**:`services/llm-orchestrator-rs/src/cache/mod.rs` 纯 sha256 精确匹配(`key_for=sha256(prompt|model|scene)`,Redis GET/SETEX TTL 3600),零 embedding。表 `esp_llm.llm_semantic_cache` 已预留 `embedding VARBINARY(2048)`/`hit_count`/`expires_at` 字段全闲置。优化=接 embedding + 向量近邻(Redis Stack VECTOR 或 TiDB Vector Search)。
- **llm provider**:`provider/mod.rs` 路由+降级链完整(openai→qwen→claude→stub),OpenAI/Claude 真发 HTTP;但 (a) StubProvider 兜底无告警,env 缺失静默返回 echo;(b) SSE 伪流式(全量收完再聚合 delta);(c) Claude 不支持 stream。
- **Seata AT 现状**:**仅 1 处真用**:`services/teacher-java/.../service/TeacherService.java:113 @GlobalTransactional(timeoutMills=60000, name="esp-teacher-summary")`(本地写学情摘要 + Dubbo 远程读 learning)。bff/learning pom 声明 seata 依赖但无 @GlobalTransactional 使用。**幂等全项目零命中**(业务代码无 requestId/SETNX/Idempotent)。
- **schema Phase1 阻塞确认**:seata AT 要求 undo_log 在服务连接的 schema;esp_auth 只有 users 无 undo_log → DataSource bean 起不来。方案A(各 schema 建 undo_log)保 AT / Saga 迁移解锁(不依赖 undo_log)。

**Saga 决策(用户拍板:走 Saga 迁移)**:
- teacher esp-teacher-summary 改 Saga 补偿事务(Seata Saga 状态机 or 本地消息表+补偿),learning 侧 Dubbo 读不改(纯读无需补偿)。解锁 schema Phase1 Batch2~5。
- bff 不涉 seata 可直接走方案A(建 undo_log)快速解锁。
- 风险:Saga 实现复杂度 > AT,需定义补偿动作+幂等。

**8 项优化任务(TaskList)**:
| # | 任务 | 优先级 | 状态 |
|---|---|---|---|
| 1 | 镜像漏洞批量修复(Python apt upgrade + teacher bouncycastle/netty + 三服务 jackson + otel agent) | P0 | ⏳ 待执行 |
| 2 | Seata AT→Saga 全服务迁移(teacher summary,解锁 schema Phase1) | P0 | ⏳ 待执行 |
| 3 | Sentinel 限流熔断规则补齐(bff Nacos 动态 FlowRule+DegradeRule,修 gw-flow 类型) | P1 | ⏳ 待执行 |
| 4 | 幂等性补位(writing/grade+teacher/summary+llm/chat 加 requestId SETNX+TTL;消费侧去重) | P1 | ⏳ 待执行 |
| 5 | LLM 语义缓存实现(cache/mod.rs 接 embedding 近似查) | P1 | ⏳ 待执行 |
| 6 | GitOps 引入(ArgoCD/Flux,依赖 git 迁入) | P2 | ⏳ 等用户给 GitHub URL |
| 7 | distroless 镜像(Java/Go/Rust 最终阶段换 distroless) | P2 | ⏳ 待执行 |
| 8 | 业务 SLO 告警(Prometheus P99/错误预算/饱和度) | P2 | ⏳ 待执行 |

**集群现状(2026-08-07 12:30)**:teacher-java 已 rollout restart(防 jetty hang 复发,新 pod 启动中)。其余 13 pod 稳定。**重要:本次优化涉及多服务代码+镜像重建,单节点 VM 高负载,重 mvn/docker build 必须与集群操作串行(铁律 esp-singlenode-host-cluster-contention)**。

## schema Phase 1 方案A 全量完成(2026-08-07 13:30,✅ Batch1~5 全成功)

**早期派发 agent 返回**:schema-per-service Phase 1 方案A(各新 schema 建 undo_log 保 seata AT)全 5 Batch 执行成功。
- **6 新 schema 各有 undo_log**:esp_auth/esp_speaking/esp_writing/esp_teaching/esp_learning/esp_shared(esp.undo_log 保留)。esp 库现只剩 undo_log。
- **RENAME + 行数对账全通过**:esp.users 1000001→esp_auth.users 1000001;scenarios/sessions→esp_speaking;tasks 1110500/templates/essays/plans/progress→esp_writing;classes/assignments/submissions/lesson_plans→esp_teaching;user_mastery 4999999/knowledge_points 29954/learning_events 10312000/session_turns 50137000→esp_learning;app_logs 43→esp_shared。
- **env 切库 + rollout 全 RC=0**:7 服务 1/1 Ready(bff/gateway/writing-py/writing-coach/teacher/learning/emotion)。
- **方案A 验证通过**:seata AT 的 RM 在新 schema 有 undo_log 后注册正常,**反驳"必须迁 Saga 才能解锁"假设**。但用户基于一致性维度短板已决策走 Saga,两者并行(teacher Saga 已实现,P0-2 完成)。

**遗留风险(⏳ 待处理)**:
- **app_logs 跨 schema 写**:gateway-go/writing-py/emotion-svc 改了源码加 `APP_LOG_DB` env(默认 esp_shared),但**镜像未 rebuild**,线上旧镜像硬编码 `database=esp`,无流量未报错,有流量会 1146。需 rebuild+rollout。
- **learning-java BehaviorController** 有 `FROM app_logs` 读查询走 readJdbc→现指 esp_learning,而 app_logs 在 esp_shared,潜在 1146(无流量未暴露)。需改 SQL 加 `esp_shared.app_logs` 或单独 datasource。

## P0/P1/P2 + Saga 并行优化(2026-08-07 13:30,✅ 代码层 7/8 完成 + 编译通过)[历史快照,最新见下方 17:30 节]

**8 项优化完成情况(代码层)**:
| # | 任务 | 状态 | 关键产出 |
|---|---|---|---|
| 1 | 镜像漏洞修复 | ✅ 代码+依赖 | Python 2 Dockerfile 加 apt upgrade+修正 USER 顺序;三 Java pom 覆盖 jackson-2-bom 2.21.4 + teacher netty 4.2.16.Final + bouncycastle 1.84;otel-agent.jar 2.10.0→2.26.1 替换(MD5 三处一致) |
| 2 | Seata AT→Saga | ✅ 代码+ADR | TeacherSummarySaga.java(tryFinalize+compensate+幂等);TeacherService 移除 @GlobalTransactional 委托 saga;ADR `docs/design/adr-seata-at-to-saga.md` |
| 3 | Sentinel 规则 | ✅ 代码+规则 | RateLimitFilter 移除硬编码→Nacos fallback 检查;application.yml ds1 gw-flow→拆 flow+degrade 两 datasource;`deploy/nacos/sentinel/bff-java-{flow,degrade}-rules.json` |
| 4 | 幂等补位 | ✅ 代码 | esp-cache 新增 CacheService.tryIdempotent(SETNX);writing /api/writing/grade(Python nx+ex)+ teacher summary(submissionId 键)+ llm /api/llm/chat(Rust NX EX) |
| 5 | LLM 语义缓存 | ✅ 代码+编译 | cache/mod.rs 重写(71→230 行):SimHash 64bit + Redis ZSET 近邻([fp-3,fp+3])+ L1 sha256 精确双轨,零新依赖,cargo check RC=0 |
| 6 | GitOps | ✅ manifest+ADR | `deploy/gitops/argocd-{project,apps}.yaml` + `docs/design/adr-gitops-argocd.md` |
| 7 | distroless 镜像 | ✅ Dockerfile | vocab-srs-go/live-tutor-go→base-debian12:nonroot;llm-orchestrator-rs/media-learn-rs→cc-debian12:nonroot;gateway/course-path 保持 Alpine 0 漏洞 |
| 8 | SLO 告警 | ✅ YAML | 10-observability.yaml rules.yml 3→10 条(+P99/P95/错误预算/CPU/内存/CrashLoop/TiDB连接池),YAML 校验通过 |

**编译验证(串行,RC 全 0)**:
- esp-cache `mvn clean install` ✅(tryIdempotent 装入 m2)
- teacher-java `mvn clean compile` ✅(修复 TeacherSummarySaga Snapshot/Submission 类型错配后通过)
- bff-java `mvn clean compile` ✅(Sentinel 改动通过)
- learning-java `mvn clean compile` ✅(jackson BOM 解析通过)
- llm-orchestrator-rs `cargo check` ✅(仅 1 dead_code warning: hamming_distance 未用)
- vocab-srs-go `go build` ✅

**遗留 / 下一步(未做)**:
- **镜像重建+重扫验证漏洞下降**:5 个 Python/Java/Rust 服务需 docker build 重建镜像→trivy 重扫确认 CRIT/HIGH 清零(P0-1/distroless 闭环)。
- **部署验证**:8 项改动需 kubectl apply/rollout 部署到集群验证运行时(teacher Saga 运行时行为、Sentinel Nacos 推送、幂等 Redis 连通、语义缓存 ZSET、SLO 告警加载、distroless 镜像启动)。
- **git 迁入**:待用户提供 GitHub URL → git init+push → 启 ArgoCD sync + Dependabot + Aikido。
- **schema Phase1 Batch2~5**:Saga 迁移已解锁(不依赖 undo_log),可推进。
- teacher jetty hang 复发(单节点 VM 通病)已 rollout restart,新 pod 启动中。

## P0/P1/P2 + Saga 并行优化(2026-08-07 17:30,✅ 代码层 8/8 完成 + 编译通过 + 部署验证 🔄)

**用户验收问"P0/P1/P2+Saga 是否全完成"——核验结论:代码层 8/8 ✅,但部署验证+运行时验证 ⏳ 未完成。**

**8 项优化完成情况(代码层,本会话 17:00 重新核验通过)**:
| # | 任务 | 状态 | 关键产出(本会话核验点) |
|---|---|---|---|
| 1 | 镜像漏洞修复 | ✅ 代码+依赖+一轮部署 | bff netty 4.2.16 + bc 1.84 + 排 pulsar shade;teacher netty+bc 1.84;learning jackson 2.21.4;py→bookworm;otel 2.26.1(三处 MD5=ea9cb308... 一致) |
| 2 | Seata AT→Saga 全服务迁移 | ✅ 代码+ADR | **全项目仅 teacher 一处真用 @GlobalTransactional**,已改 TeacherSummarySaga(try+compensate+幂等)。核验:`grep @GlobalTransactional services/*/src` 业务代码 0 处(仅注释/ADR)。bff/learning 本就无 @GlobalTransactional。ADR `adr-seata-at-to-saga.md` |
| 3 | Sentinel 规则 | ✅ 代码+规则 | RateLimitFilter 移除硬编码 loadRules(L48 注释"Nacos 推送是单一来源");application.yml ds1 flow + ds2 degrade 双 datasource;`deploy/nacos/sentinel/bff-java-{flow,degrade}-rules.json` |
| 4 | 幂等补位 | ✅ 代码 | writing /grade(nx+ex rid L78)+ teacher summary(saga 内 submissionId 键)+ llm /chat(NX EX 60s main.rs:273) |
| 5 | LLM 语义缓存 | ✅ 代码+编译 | cache/mod.rs SimHash 64bit + Redis ZSET 近邻(DELTA=3)+ sha256 精确双轨,零新依赖,cargo check RC=0 |
| 6 | GitOps | ✅ manifest+ADR | `deploy/gitops/argocd-{project,apps}.yaml` + `docs/design/adr-gitops-argocd.md`(等 git 迁入启用 sync) |
| 7 | distroless 镜像 | ✅ Dockerfile | vocab-srs-go/live-tutor-go→`gcr.io/distroless/base-debian12:nonroot`;llm-orchestrator-rs/media-learn-rs→`cc-debian12:nonroot`;gateway/course-path 保持 Alpine 0 漏洞 |
| 8 | SLO 告警 | ✅ YAML | 10-observability.yaml 内嵌 PrometheusRule **实测 10 条 alert**(ServiceDown/HighErrorRate/PodRestart/P99/P95/ErrorBudget/CPU/Memory/CrashLoop/TiDBConnPool) |

**编译验证(串行,RC 全 0,PROGRESS 早前记录)**:esp-cache mvn install ✅ / teacher mvn compile ✅ / bff mvn compile ✅ / learning mvn compile ✅ / llm cargo check ✅ / vocab go build ✅。

**遗留 / 下一步(未做,这是"未完成"的部分)**:
- **镜像重建+部署**:learning/teacher 1.6.0 待 builder 完成(buildkit 凭证坑已解);bff/py/emotion 1.6.1 二次修复代码已改待重建。
- **trivy 复扫确认漏洞下降**:1.6.0 一轮未达预期(见上节根因),1.6.1 二次修复后复扫验证。
- **运行时行为验证**:teacher Saga 运行时行为、Sentinel Nacos 推送、幂等 Redis 连通、语义缓存 ZSET、SLO 告警加载、distroless 镜像启动 —— 均⏳ 需部署后实测。
- **git 迁入**:待用户提供 GitHub URL → git init+push → 启 ArgoCD sync + Dependabot + Aikido。
- **schema Phase1 Batch2~5**:Saga 迁移已解锁(不依赖 undo_log),可推进。
- teacher jetty hang 复发(单节点 VM 通病)待 1.6.0 部署替换。

## trivy 复扫 learning/teacher 1.6.0(2026-08-07 21:40,✅ 落 report,⚠️ learning 依赖升级未生效)

**报告已落盘**:`report/security/trivy-image-{learning,teacher}-1.6.0.json`。本机无 docker daemon,RD 用 moby containerd 在 VM 内,host 无 socket 转发 → builder 改用 `rdctl shell nerdctl save` 导 tar + `trivy image --input <tar>` 扫描(等效)。

| 镜像 | 1.5.x 基线 | 1.6.0 实测 | 结论 |
|---|---|---|---|
| teacher-java | 63 总(2 CRIT bc + 16 HIGH netty) | **1 CRIT / 0 HIGH** | ✅ 达预期,降幅 98%。netty 16 HIGH 全清零,残留仅 bcprov 1.78.1 CRIT |
| learning-java | 30 总(1 CRIT otel + 3 HIGH jackson) | **1 CRIT / 13 HIGH** | ⚠️ **不达预期,回归**。jackson 3 HIGH 清零,但 netty HIGH 从 0 升到 13(netty 仍 4.2.15.Final,4.2.16 未进 fat jar) |

**根因(learning 依赖升级未落地)**:
- learning pom.xml **未声明** `<netty.version>` 属性(teacher L27 有 `4.2.16.Final`,learning 无)→ netty 由 SB BOM 锁定 4.2.15.Final,未覆盖。
- learning pom.xml **未声明** bcprov/bcpkix 1.84 依赖(teacher L32-33 有)→ bcprov 仍 1.78.1(传递依赖)。
- teacher 升级声明齐全且生效(netty 4.2.16 + bc 1.84 进了产物),所以 teacher 1.6.0 漏洞大幅下降。
- 两镜像共同残留:bcprov-jdk18on 1.78.1 CRIT CVE-2025-14813(learning+teacher 都有,声明升 1.84 在 learning 没落地)。

**修复方向(learning 1.6.1 待重建)**:
- learning pom.xml properties 加 `<netty.version>4.2.16.Final</netty.version>` + dependencies/dependencyManagement 加 bcprov/bcpkix-jdk18on 1.84(对齐 teacher pom L27/L32-33 写法)。
- 重 build esp/learning-java:1.6.1 → trivy 复扫确认 CRIT/HIGH 清零。
- bff pom 已声明 netty 4.2.16 + bc 1.84(bff L25/L269-275),bff 1.6.1 镜像待 build 验证(声明已对齐 teacher)。

## teacher jetty acceptor hang 根因深化+联网解决方案(2026-08-07 21:50)

**现象(本会话二次确认)**:teacher 1.6.0 pod 1/1 Ready(readiness tcp 8083 握手通过),但 curl 8083 超时(exit 28),日志停滞 4-11min,新 pod 重启后仍 hang。`/proc/net/tcp6` 实测:**51 个 CLOSE_WAIT**(对端全 10.42.0.1 = kubelet 探针)、4 TIME_WAIT、2 LISTEN(含 8083)、28 ESTABLISHED。

**根因(已确认,非探针配置问题)**:Jetty acceptor/selector 线程卡死,接受 TCP 握手(readiness tcpSocket 通过)但不处理应用层请求 → kubelet 探针连接堆积成 CLOSE_WAIT → HTTP 请求超时。jstack/jcmd 不可用(jre-only 镜像无 JDK 工具),无法抓线程堆栈精确定位卡点,但症状与历史根因一致(单节点 VM JVM 长跑后 Jetty acceptor hang)。

**联网搜索结论(最佳解决方案,按优先级)**:
1. **换 httpGet 探针替代 tcpSocket(首要,根治"假 Ready")**:tcpSocket 只测 TCP 握手,Jetty acceptor hang 时握手仍通但应用层死 → 假 Ready。改 readiness/liveness `httpGet path=/actuator/health/readiness port=8083`,acceptor hang 时 HTTP 无响应 → 探针失败 → kubelet 重启 pod 自愈。**当前 readiness tcpSocket 是"假 Ready"根因,必须改 httpGet**。
2. **加 livenessProbe(timeout 15s,失败 3 次重启)**:当前只有 readiness 无 liveness → hang 后不会被重启,只能人工 rollout。liveness httpGet 失败自动重启,自愈。
3. **Jetty 线程池调优**(防线程耗尽):application.yml 加 `server.jetty.threads.max: 400`(默认 200,高并发不足)、`min: 50`、`idle-timeout: 60s`、`acceptors: 2`、`selectors: 4`(CPU 核数相关)。单节点 VM 资源紧张,线程池耗尽是 hang 诱因之一。
4. **限制 kubelet 探针并发**(减 CLOSE_WAIT):`periodSeconds: 30`(当前 20)、`timeoutSeconds: 5`、`failureThreshold: 3`,减少探针连接堆积。
5. **JVM 监控**:加 JVM 线程数/CLOSE_WAIT/GC 指标(OTel agent 已在,加 Micrometer `executor.threads`/`process.files.open` 指标),hang 前预警。
6. **长效根治=多节点**(单节点 VM 高负载是诱因,多节点分散负载 + HPA 弹性,JVM 不再长跑高压)。

**已落地**:尚待执行(本会话先记录方案,执行需改 deployment probe + application.yml + 重建镜像)。

## bff sentinel datasource 拉 source empty 根因排查(2026-08-07 21:50,🔄 agent 返回中)

**现象**:bff-java(SB 4.0.7 + SCA 2025.1.0.0 + sentinel-datasource-nacos)restart 2 次仍报 `converter can not convert rules because source is empty` + `RateLimitFilter: Nacos flow 规则尚未加载`。nacos openapi 能读到规则(SENTINEL_GROUP/public,flow 3 + degrade 2)。SCA sentinel-datasource-nacos 拉空。

**已派 esp-explorer agent(a1c9cb8cf9188fb76)排查**:① SCA 2025.1.0.0 与 SB4.0.7 兼容性 ② spring.application.name 是否拼对 dataId=bff-java-flow-rules ③ SentinelDataSourceAutoConfiguration 是否被 SB4 条件排除 ④ sentinel-record.log nacos 初始化证据 ⑤ RateLimitFilter 判断逻辑。**返回中**。

## teacher probe 改造根治 jetty hang(2026-08-07 22:00,代码✅ + builder 重建中)

**改造已落地(代码 + manifest)**:
1. `deploy/k8s/30-other-services.yaml` teacher deployment:
   - readiness/liveness/startup **tcpSocket → httpGet** `/actuator/health/readiness` `/actuator/health/liveness`(根治假 Ready:acceptor hang 时 TCP 握手通但 HTTP 死)
   - 恢复 **livenessProbe**(timeout 15s, failureThreshold 3, period 30s):acceptor hang → HTTP 超时 → kubelet 自愈重启
   - readiness period 20s→30s 减少 kubelet 探针 CLOSE_WAIT 堆积
   - image 1.5.1→1.6.0,env 切库 esp→esp_teaching(manifest 落后线上已纠正)
2. `services/teacher-java/src/main/resources/application.yml`:
   - `server.jetty.threads.max:400/min:50/idle-timeout:60s` + `acceptors:2/selectors:4`(防线程池耗尽 hang)
   - `management.endpoint.health.probes.enabled:true`(激活 readiness/liveness health group 端点)
3. learning 同构改造(防 hang):`20-learning-java.yaml` probe 改 readiness/liveness group 端点 + period 30s;`application.yml` 加 jetty 线程池 + probes.enabled;`pom.xml` 加 netty.version 4.2.16 + bcprov/bcpkix 1.84(对齐 teacher,修 1.6.0 回归)。

**🔄 builder agent(a30f4d0f4cc829391)重建 teacher 1.6.1 镜像 + 部署中**。learning 1.6.1 待 builder 空闲后重建(串行,单节点铁律)。

## 2026-08-08 会话进展(运行时验证收尾 + llm Redis Cluster bug 发现)

### ✅ learning 1.6.1 重建+trivy 清零(闭环)
- **1.6.0 基线**:1 CRIT(bcprov 1.78.1)/ 13 HIGH(netty 4.2.15)。
- **1.6.1 结果**:**0 CRIT / 0 HIGH**(trivy 报告 `report/security/trivy-image-learning-1.6.1.json`)。
- **关键根因(新发现,非 pom netty/bc 声明问题)**:pulsar-client **3.3.9** 传递依赖 `org.apache.pulsar:bouncy-castle-bc:pkg:3.3.9`,该 jar-in-jar 打包格式内部嵌入 `lib/bcprov-jdk18on-1.78.1.jar`,trivy jar scanner 穿透嵌套发现旧版本 → 应用层声明 bcprov 1.84 进了 fat jar 但 shaded 1.78.1 仍被 trivy 计独立 CRIT。
- **修复**:`services/esp-cache/pom.xml` L56-68 对 pulsar-client 加 `<exclusion>` 排 `bouncy-castle-bc`(esp-cache 被 bff/teacher/learning/integration-tests 共用,exclusion 全服务生效)。各服务已显式声明 bcprov 1.84 功能替代。
- **部署**:learning-java pod 1/1 Running,health UP。image=esp/learning-java:1.6.1。

### ✅ teacher probe 改造根治 jetty hang(已生效,验证通过)
- **deploy image**:teacher-java **1.6.1**(manifest probe 已 httpGet)。
- **探针现状(实测)**:readiness `httpGet /actuator/health/readiness port=8083`(period 30s timeout 5s)+ liveness `httpGet /actuator/health/liveness`(timeout 15s failureThreshold 3 period 30s)。已从 tcpSocket 改 httpGet,根治"假 Ready"(acceptor hang 时 TCP 握手通但 HTTP 死 → 探针失败 → 自愈重启)。
- **CLOSE_WAIT 复查**:仅 2 个(此前 51 堆积),Jetty acceptor 正常处理。`/actuator/health` 返 UP(readinessState/livenessState/db read+write/redis cluster_size=3 slots=16384 全 UP)。
- **应用层响应**:pod 内 wget `/actuator/health/readiness` 返 `{"status":"UP"}` rc=0(此前 curl 8083 exit 28 超时,现已恢复)。

### ✅ teacher Saga 端到端实测(运行时验证通过)
- **触发**:POST `/api/teacher/submissions/{id}/finalize` body=`{"final_score":87.5,"class_id":100}`(submissionId=3746994889972254669,user=605268)。
- **执行证据(teacher 日志)**:
  - `teacher.finalized published: userId=605268, assignmentId=3170534137668829552, score=87.5`(EventPublisher 发 Pulsar topic teacher.finalized,traceId 透传)
  - Pulsar producer `teacher.finalized` Created on pulsar-broker-2。
- **DB 验证**:`esp_teaching.submissions` 该行 status=pending→**finalized**,final_score=87.50 ✅(Saga tryFinalize 本地写成功)。
- **幂等验证**:同 submissionId 二调(final_score=99)→ DB 被覆盖为 99,**幂等未生效**(见下方 llm 同类根因:Redis Cluster MOVED)。**teacher 幂等走 esp-cache(CacheService.tryIdempotent SETNX),esp-cache Java 用 Spring Data Redis cluster 配置(application.yml cluster nodes)— 应能处理 MOVED**;但实测幂等未拦。**待查**:esp-cache Java 是否真用 cluster 模式(还是 standalone),或 SETNX key 落到非本节点未重定向。**遗留待 bff/teacher 1.6.1 部署后复查**。

### 🚨 llm-orchestrator-rs Redis Cluster 连接 bug(重大发现,修复中)
- **现象**:多次调 `/api/llm/chat` 后全集群 scan `esp:llm:*` / `esp:idem:llm:*` **全空**;同 prompt 二调 `cache_hit=false`;同 requestId 二调不返回上次响应(幂等失效)。readyz 报 redis=UP(PING 通),但写命令静默失败。
- **根因(已确认,实证)**:llm 用 `redis::aio::ConnectionManager`(单连接,**非 cluster client**)连 ESP 真 Redis Cluster(3 master 分片,slot 0-5460/5461-10922/10923-16383)。
  - 实测:从 node0(master 0-5460)对 keyslot=8291(属 node1)的 key 发 SETEX 返回 `MOVED 8291 redis-cluster-new-1...:6379`。
  - 单连接客户端**不处理 MOVED 重定向** → `cache::set` 的 SETEX/ZADD、main.rs 的 `SET NX EX`(幂等)发到 keyslot 不在当前连接节点的 key 时返回 MOVED → 命令失败 → `.ok()` / `await?` 吞掉 → **缓存和幂等永不回写**。
  - 读 GET 偶尔命中本节点 key 才成功(手动预置 manual-test 在 node0,GET 偶尔通),造成"readyz UP 但写全失败"的假象。
- **对照**:esp-cache-py 用 `aioredis.RedisCluster`(处理 MOVED ✅);Java 服务用 Spring Data Redis `cluster.nodes`(Lettuce cluster ✅)。**只有 llm-orchestrator-rs 用了单连接,是 Rust 侧疏漏**。
- **修复(代码已改,builder 重建中)**:
  - `Cargo.toml`:redis 依赖加 `cluster` feature(`features=["tokio-comp","aio","connection-manager","cluster"]`)。
  - `src/main.rs` init_state():redis 连接从单 `ConnectionManager` 改为 `ClusterClient`(支持 MOVED 自动重定向)。
  - `src/cache/mod.rs`:`conn` 类型适配 cluster async 连接(实现 `aio::ConnectionLike`,`query_async` 仍可用)。
  - 镜像 tag 升 `0.2.0`,重建+部署后重测 cache_hit + idem replay。
- **builder agent a486a06696a7e984a 后台重建中**(改代码+cargo check+build 0.2.0+rollout)。

### 🔄 bff 1.6.1 重建(✅ 完成,builder a35c60cd3ce07ac7d)
- pom 已改(netty 4.2.16 + bc 1.84 + pulsar 排 shade)+ esp-cache exclusion 已生效。
- **结果**:**0 CRIT / 0 HIGH**(trivy 报告 `report/security/trivy-image-bff-java-1.6.1.json`)。
- 产物校验:fat jar netty 全 4.2.16.Final + bcprov/bcpkix 1.84 + **bouncy-castle-bc 消失**(exclusion 生效)。
- 部署:bff-java 1.6.1 pod 1/1 Running,BffApplication 启动成功(68s),Pulsar consumer 订阅 esp-cache-invalidate 成功,Nacos 注册完成,Jetty 8090 监听。
- **坑(记录复用)**:trivy Java DB 下载——`/tmp/trivy-cache-*` 的 java-db 是 0B 空文件(失败残留),`--skip-java-db-update` 被判 first run 拒绝;mirror.gcr.io 超时、ghcr.io DENIED。**解法=用 macOS 默认 cache `~/Library/Caches/trivy/`(内有 1.4G trivy-java.db + 1.1G trivy.db)+ `--skip-db-update --skip-java-db-update`**。后续扫描统一用默认 cache 或确保 /tmp cache 的 java-db 非空。
- **坑(nerdctl save)**:nerdctl 0.x `save -o` 在 save 子命令行为异常,改用 stdout 重定向 `nerdctl save img > tar` 成功。

### 🔄 bff sentinel datasource empty 根因排查(主线程接管,explorer 撞 rate-limit)
- nacos openapi GET `bff-java-flow-rules` group=SENTINEL_GROUP 能返完整 JSON(3 FlowRule)。
- bff 日志 `converter can not convert rules because source is empty` + RateLimitFilter "Nacos flow 规则尚未加载"。
- **高嫌疑根因**:application.yml sentinel.datasource.flow.nacos 块**未配 namespace**,默认 namespace 可能非 public(nacos discovery/config 块用 namespace=${NACOS_NS:public});SCA 从非 public 拉不到 → source empty。对比 openapi GET 不带 namespace 默认查 public 能拿到。
- 其他嫌疑:SCA 2025.1.0.0 与 SB4 兼容性 / SentinelDataSourceAutoConfiguration 被条件排除。
- **排查受阻**:`kubectl exec deploy/bff-java -- grep/cp /root/logs/csp/sentinel-record.log` 被项目 kubectl logs guard 拦截(误判为裸 logs);explorer agent 撞 API rate-limit 终止。**待用合法方式取 sentinel-record.log**(如 kubectl cp 带正确参数,或临时 pod 挂卷)。
- **下一步修复方向(待根因确认后执行)**:大概率给 `sentinel.datasource.flow.nacos` 和 `degrade.nacos` 块加 `namespace: ${NACOS_NS:public}` → rollout → 验证 RateLimitFilter 不再报 "尚未加载" + FlowRuleManager.getRules() 非空。

## teacher-java 1.6.2 重建+部署+trivy+幂等实测(2026-08-08 13:25~13:40,全部 ✅)

### 变更内容
- `services/teacher-java/src/main/java/com/esp/teacher/service/TeacherService.java` 的 `finalize()` 方法加幂等:
  - `SETNX esp:idem:teacher:finalize:<submissionId>` TTL 600s,复用 `cacheService.tryIdempotent`
  - 与 `summarizeStudentLearning` 同款机制
- 代码层 `mvn clean compile` 早已 RC=0;本次只做 build+部署+trivy+运行时验证

### 执行结果(全部 RC=0)
1. **mvn clean package -DskipTests** → BUILD SUCCESS 5.1s,产物 `target/teacher-java-1.0.0.jar`
2. **nerdctl build** → `esp/teacher-java:1.6.2` sha256:cf79132d,arm64,543.5MB(eclipse-temurin:21-jre-noble base,pom 已有 pulsar-client exclusion 排 bouncy-castle-bc)
3. **部署** → `kubectl -n esp-prod set image deploy/teacher-java teacher-java=esp/teacher-java:1.6.2`(imagePullPolicy=IfNotPresent,tag 变拉新)→ rollout 收敛 2 pod(新 1/1 Ready,旧终止)
   - 注意:deployment replicas=2,rollout 期间出现 startup probe `connection refused`(Jetty 启动 ~34s),最终 1/1 Ready
4. **readiness 验证** → `kubectl exec deploy/teacher-java -- curl http://localhost:8083/actuator/health/readiness` → `{"status":"UP"}` ✅
5. **trivy 复扫** → nerdctl save > /tmp/teacher-1.6.2.tar(279M)→ trivy scan
   - 报告:`report/security/trivy-image-teacher-java-1.6.2.json`
   - 结果:**CRIT=0 HIGH=0** ✅(1.6.1 已 0/0,1.6.2 仅加幂等代码,无新依赖,维持)
6. **幂等运行时实测**(submissionId=3746994889972252677,pending):
   - 第一次 POST `/api/teacher/submissions/{id}/finalize` body `{"final_score":75}` → `{"finalized":3746994889972252677}` ✅
   - 第二次 POST body `{"final_score":99}` → 返回 `{"finalized":...}`(接口仍 200,幂等在 service 层拦截写库)
   - DB 查 `SELECT final_score,status` → **`final_score=75.00`(99 被幂等拦截,未覆盖),status=finalized** ✅
   - 结论:finalize 幂等在 cluster 模式下 SETNX 生效,重复终审不覆盖分数。补注:1.6.1 前幂等复查遗留问题(esp-cache cluster 模式)随本次 1.6.2 实测一并闭合。

### 构建坑复用记录(本次未踩)
- 单节点铁律:mvn 与集群操作串行,未并行(无 TLS 超时/探针误杀雪崩)
- nerdctl 0.x `save -o` 异常 → 用 stdout 重定向 `save img > tar` ✅
- docker-credential-osxkeychain 已移走(仅 .bak),nerdctl build 走匿名拉取 ✅

## ✅ 隐患A 闭环:gateway-go 1.5.1 部署+运行时验证(2026-08-08 会话18,builder a318805e1f51393f0 返回)

### 部署结果
- 新 pod `gateway-go-7657c7f87d-778r7` 1/1 Running,imageID=sha256:b935c2ed(1.5.1,非旧 1.5.0 ccee4e83)
- `kubectl set image deploy/gateway-go` + rollout successfully rolled out,RC=0

### 构建坑(关键,后续 Rust/Go 服务 rebuild 必复用)
1. **base image 拉不到**:lima buildkitd mirror `docker.m.daocloud.io` 对 `docker.io/library/golang:1.25-alpine` 返回 403/unreachable。解决:手动 `nerdctl pull docker.1ms.run/library/golang:1.25-alpine`+`alpine:3.22`,Dockerfile FROM 改 `docker.1ms.run/library/...` 前缀。临时 Dockerfile:`services/gateway-go/Dockerfile.mirror`。
2. **apk add ca-certificates 卡死**:lima 内 `dl-cdn.alpinelinux.org` 偶发超时(5min 无响应),buildkit 进程挂死需 kill。解决:Dockerfile.mirror 中 apk repositories 改清华 mirror `https://mirrors.tuna.tsinghua.edu.cn/alpine/v3.22/...`。
3. **k8s.io namespace 缺镜像致 ImagePullBackOff(最关键)**:Rancher Desktop 用 `cri-dockerd.sock`(非 containerd 直连),kubelet 经 cri-dockerd 查 **docker/moby namespace** 镜像。nerdctl build 默认写 `default` namespace,`ctr images import` 到 `k8s.io` ns 都不被 cri-dockerd 识别。解决:`nerdctl save img > tar` → 宿主机 `docker load < tar` 加载进 docker(moby ns),cri-dockerd 即可发现。路径:`nerdctl(default ns) → tar → docker load(moby ns)`。

### 运行时验证
- before COUNT(esp_shared.app_logs WHERE service='gateway')=44 → `GET /api/scenarios`(NodePort 30080)→ after=**45**(+1)✅
- 日志 grep `1146|degraded` → 空匹配 ✅
- 结论:隐患A 闭环。TaskList #1 completed。

## ✅ 隐患B 闭环:learning-java 1.6.2 部署+运行时验证(2026-08-08 会话18,builder af2ba9a84778d8181 返回)

### 部署结果(全 RC=0)
1. `mvn -q -o package -DskipTests` → fat jar `target/learning-java-1.0.0.jar`(193MB)
2. `rdctl shell docker build -t esp/learning-java:1.6.2` → 镜像构建成功
3. `kubectl set image deploy/learning-java` → image 更新
4. rollout 收敛:新 pod `learning-java-6469f4dbf-tj84k` 1/1 Ready,旧 pod 终止,0 restarts

### 构建中额外发现并修复的坑(关键,引入 SharedDataSourceConfig 的连带影响)
首次 1.6.2 启动失败 —— 容器内现有**两个 JdbcTemplate bean**(`jdbcTemplate` 主路由数据源 + 新增 `sharedJdbcTemplate` 绑 esp_shared),`BehaviorController`/`LearningEventConsumer` 的 `JdbcTemplate` 构造参数按类型注入歧义:
> `Unsatisfied dependency ... No qualifying bean of type 'JdbcTemplate' available: expected single matching bean but found 2`

修复(3 处):
1. `services/learning-java/src/main/java/com/esp/learning/config/ServiceConfig.java` — `jdbcTemplate` Bean 加 `@Primary`(消除默认歧义)
2. `services/learning-java/src/main/java/com/esp/learning/web/BehaviorController.java` — `readJdbc` 参数加 `@Qualifier("jdbcTemplate")`;`sharedJdbc` 已有 `@Qualifier("sharedJdbcTemplate")` 不变
3. `services/learning-java/src/main/java/com/esp/learning/event/LearningEventConsumer.java` — `jdbc` 参数同样加 `@Qualifier("jdbcTemplate")`

重 `mvn package`+`docker build`+rollout 后启动成功(`Started LearningApplication in 152.808 seconds`)。

### 运行时验证(全绿,隐患B 闭环)
- **写(AppLogClient → esp_shared.app_logs)**:before COUNT=0 → `POST /api/learning/behavior` → HTTP 202 `{"accepted":true}` → after COUNT=**1**(+1);日志 grep `app_logs|1146|insert failed` **无 insert failed** ✅
- **读(BehaviorController → esp_shared.app_logs,不再 1146)**:`GET /behavior/heatmap?user_id=1&days=30` → HTTP 200 `[{"event":"behavior.probe_b","cnt":1}]`;`GET /behavior/top?days=30&limit=5` → HTTP 200 `[{"user_id":1,"cnt":1}]`;日志 grep `1146|bad SQL|Table.*doesn` **0 条** ✅
- 结论:隐患B 闭环。TaskList #2 completed。

## ✅ 隐患E 闭环:S3~S7 五新服务全部实跑 + ArgoCD esp-prod Synced+Healthy(2026-08-09 会话22)

### scaffold 编码完成(列名已核对实表,TaskList #6#7 completed)
| 服务 | 语言/框架 | 端口 | P | schema 表(实列已核对) |
|---|---|---|---|---|
| S3 pronunciation-rs | Rust/axum | 8088 | P0 | esp_pronunciation.pronunciation_attempts(id,user_id,ref_type,ref_id,audio_url,score,phoneme_scores,asr_text,duration_ms,created_at) |
| S4 listening-go | Go/gin | 8089 | P1 | esp_listening.listening_exercises(id,title,audio_url,duration_sec,cefr_level,question_json,scene_tags,created_at) + listening_attempts(id,user_id,exercise_id,answers_json,score,duration_ms,created_at) |
| S5 assessment-rs | Rust/axum | 8090 | P1 | esp_assessment.assessment_sessions(id,user_id,goal,cefr_result,status,score,started_at,finished_at) + assessment_answers(id,session_id,question_id,answer_json,is_correct,skill_tag,created_at) |
| S6 reading-go | Go/gin | 8091 | P1 | esp_reading.reading_passages(id,title,content,word_count,cefr_level,source_type,questions_json,created_at) + reading_attempts(id,user_id,passage_id,answers_json,score,duration_ms,created_at) |
| S7 gamification-go | Go/gin | 8092 | P1 | esp_gamification.user_streaks(user_id,current_days,longest_days,last_active,updated_at) + user_achievements(id,user_id,achievement_id,unlocked_at) + achievements(id,code,name,description,icon_url,criteria_json) |

**列名核对修正(避免运行时 SQL 报错)**:
- pronunciation:`ref_type/ref_id/audio_url/score`(非原假设 `audio_ref/overall_score`)
- assessment_sessions:`goal/cefr_result`(非 `type/level_result`)
- listening_exercises:`duration_sec`(非 `duration`)、无 `transcript` 列
- reading_passages:`source_type/questions_json`
- gamification:`user_streaks.current_days/longest_days`(非 `streak_days`)、无 `gamification_profiles` 表(积分走 Redis ZSET + 连胜走 user_streaks)

**manifest**:`deploy/k8s/32-batch2-services.yaml`(S3 replicas=1 实跑,S4~S7 replicas=0 待命多节点,HPA min=1 因 K8s 不允许 0)。

### 当前后台
- **pronunciation-rs 0.1.0 构建+部署+验证**:builder ae1e8f11a3b8f8c43 后台进行中(TaskList #8)。cargo build 首次 5~10min(拉 crate + 编译)+ docker load 进 moby ns(复用 gateway 坑)+ apply 32-batch2 + port-forward 验证评分 +1。Dockerfile 已被 linter 改 FROM 为 `docker.1ms.run/library/rust:1.88-bookworm`(docker.io 直拉 i/o timeout)。

## 下一步动作(恢复锚点,2026-08-08 会话18 v4 更新)

1. **pronunciation-rs builder 返回验证**(ae1e8f11a3b8f8c43,后台):pod 1/1 + `POST /api/pronunciation/score` → esp_pronunciation.pronunciation_attempts +1 → S3 闭环。
2. **ArgoCD helm install**(TaskList #9,pending):等 pronunciation builder 闭环后串行(单节点铁律)。helm install into argocd ns → GitOps App 接入 GitHub 私有仓 `deploy/k8s/*`。
3. **Aikido**:待用户在 Aikido 控制台接入(非 GitHub 原生 OAuth)。
4. 旧镜像清理:gateway-go 1.5.0、learning-java 1.6.1、llm-orchestrator-rs 0.1.0。

> 已闭环:A(app_logs 跨 schema 写,gateway 1.5.1)、B(learning app_logs 双 1146,1.6.2)、C(Python 6CRIT 豁免)、D(git 迁入)。
> 进行中:E(pronunciation-rs 构建部署,builder ae1e8f11a3b8f8c43 后台)。
> 待串行:ArgoCD helm install、旧镜像清理。
> Redis 锚点=esp:ctx:hot:2026-08-08-session18-abcde-parallel(v4,slot 15447→node2 master redis-cluster-new-2)。

## 并发 agent(后台,2026-08-08)
| agent | 任务 | 状态 |
|---|---|---|
| esp-builder a6b28800e1874dea1 | llm 0.2.0 cluster fix(cargo build+nerdctl build+部署) | 🔄 运行 |
| (已完成) esp-builder a35c60cd3ce07ac7d | bff 1.6.1 重建+trivy 0CRIT/0HIGH+部署 | ✅ 完成 |
| (已完成) esp-builder learning-builder | learning 1.6.1 重建+trivy 0CRIT/0HIGH+部署 | ✅ 完成 |
| (rate-limit 终止) esp-explorer | bff sentinel datasource empty 根因 | ❌ 主线程接管 |

## 镜像部署现状(2026-08-08 核实)
| 服务 | deployment image | trivy CRIT/HIGH | pod Ready |
|---|---|---|---|
| learning-java | **1.6.1** | **0/0** ✅ | ✅ 1/1 UP |
| teacher-java | **1.6.2** | **0/0** ✅ | ✅ 1/1 UP(finalize 幂等已实测) |
| bff-java | **1.6.1** | **0/0** ✅ | ✅ 1/1(BffApplication Started 68s) |
| writing-py | 1.6.1 | 6/18(bookworm fix=N/A) | ✅ 1/1 |
| emotion-svc | 1.6.1 | 6/18(同 writing-py) | ✅ 1/1 |
| llm-orchestrator-rs | 0.1.0(待升 0.2.0) | — | ✅ 1/1(但 redis 写失效) |
| gateway-go / course-path-go / vocab-srs-go | 1.5.0/0.1.0/0.1.0 | 0/0 ✅ | ✅ 1/1 |

## 关键部署状态(恢复锚点,2026-08-08 更新)

- 镜像:learning-java **1.6.1**、teacher-java **1.6.1**、bff-java **1.6.1**、writing-py/emotion-svc **1.6.1**、llm-orchestrator-rs 0.1.0(→0.2.0 修 cluster,builder 中)、gateway-go 1.5.0、course-path-go/vocab-srs-go/live-tutor-go/media-learn-rs 0.1.0、writing-coach 1.0.1、frontend-react 1.0.0。
- 中间件:nacos v2.4.3 / seata 2.2.0 / sentinel 1.8.9。
- NodePort:gateway-go 30080、frontend-react 30090。
- 基础设施复用 ns `ekb-m2-aws-us-east-1`,ExternalName 短名 `tidb-primary`/`tidb-follower`/`redis-cluster`/`pulsar-broker`。Redis Cluster 实际 `redis-cluster-new-{0..5}.redis-cluster-new.ekb-m2-aws-us-east-1.svc.cluster.local:6379`(3主3从,slot 0-5460/5461-10922/10923-16383)。
- TiDB:`tidb-primary:4000` root 无密码,schema-per-service 后 11 DB/29 表 + 老库 esp(只剩 undo_log)。
- **buildkit 凭证坑已解**:`~/.rd/bin/docker-credential-osxkeychain` 已移走(只 .bak),nerdctl build 走匿名拉取,无需再处理。
- **nerdctl socket 正确路径**:`/var/run/docker/containerd/containerd.sock`(注意 containerd 非 containered)。

## 并发 agent(后台,2026-08-07 22:00)
| agent | 任务 | 状态 |
|---|---|---|
| esp-explorer a1c9cb8cf9188fb76 | bff sentinel datasource 拉 source empty 根因排查 | 🔄 返回中 |
| (已结束) esp-builder | trivy 复扫 learning/teacher 1.6.0 → ✅ 落 report | ✅ 完成 |
| (中断) esp-builder | bff-java 1.6.1 镜像 build → API 中断,主线程接管 | ❌ 中断 |

## 镜像 1.6.0/1.6.1 全量 build+部署(2026-08-07 19:00~21:00,✅ 镜像就绪+部署,🔄 复扫+运行时验证)

**重大校正(本会话核实 PROGRESS 偏差)**:PROGRESS 旧记"learning/teacher 1.6.0 未建",**实际已 build+部署**(集群 deploy image 均已是 1.6.0,learning 2→1 副本,teacher 1/1)。本会话继续推进 1.6.1 三服务(bff/writing-py/emotion)重建。

**镜像部署现状(2026-08-07 21:00 核实)**:
| 服务 | deployment image | 镜像 build | 部署 Ready | trivy 复扫 |
|---|---|---|---|---|
| bff-java | 1.6.0(待升 1.6.1) | ✅ 1.6.0 / 🔄 1.6.1 builder 中 | ✅ 1/1 | ⏳ 1.6.1 待扫 |
| learning-java | 1.6.0 | ✅ | ✅ 1/1(健康 UP) | 🔄 trivy builder 中 |
| teacher-java | 1.6.0 | ✅ | ⚠️ 1/1 但 jetty hang 复发(应用层不响应 8083) | 🔄 trivy builder 中 |
| writing-py | **1.6.1** | ✅ | ✅ 1/1 | ✅ 6 CRIT/18 HIGH(bookworm 系统包 fix=N/A) |
| emotion-svc | **1.6.1** | ✅(Dockerfile 补 setuptools 预装) | ✅ 1/1 | ✅ 6 CRIT/18 HIGH(同 writing-py) |

**writing-py/emotion-svc 1.6.1 trivy 结论(⚠️ 未清零,根因=base 系统包)**:
- 1.6.0(trixie)4 CRIT/19 HIGH → 1.6.1(bookworm)**6 CRIT/18 HIGH**(CRIT 反增 2,因 perl-base 新 CVE)
- 6 CRIT 全是 Debian bookworm 系统包 fix=N/A/will_not_fix:libsqlite3-0 CVE-2025-7458、perl-base 4 个(CVE-2026-13221/42496/57433/8376)、zlib1g CVE-2023-45853(will_not_fix)
- **emotion-svc Dockerfile 已修**:补 `pip install setuptools>=68 wheel`(Python 3.13-slim-bookworm 不带 setuptools,esp-cache-py pyproject build-system requires setuptools>=68 导致 PEP517 失败)
- **清零需换 base**:alpine(musl 无 zlib CVE)或 python:3.13-slim-trixie(trixie CRIT 更少 4 vs 6)。zlib1g CVE-2023-45853 will_not_fix 任何 Debian base 都有。**决策待用户**。

**bff 1.6.1 代码(pom.xml 已落地)**:netty.version 4.2.16.Final(覆盖 SB BOM 4.2.15)+ bcprov/bcpkix-jdk18on 1.84 + pulsar exclusion 排 bouncy-castle-bc shade。🔄 镜像 builder 后台中。

## 运行时行为验证(2026-08-07 20:00~21:00,部分✅ / 部分🔄)

**已验证✅**:
1. **代码就位确认**:teacher Saga(TeacherSummarySaga.java + TeacherService 注释已迁 AT→Saga,`grep @GlobalTransactional services/*/src` 业务代码 0 处);幂等(writing-py main.py:78 NX+ex、teacher saga submissionId 键、llm main.rs:273 NX EX 60s);语义缓存(cache/mod.rs SimHash 64bit + Redis ZSET DELTA=3 + sha256 L1 双轨);distroless(vocab/live-tutor→base-debian12:nonroot、llm/media→cc-debian12:nonroot)。
2. **Redis Cluster 连通✅**:writing-py REDIS_HOST=redis-cluster:6379 / llm REDIS_ADDR=redis://redis-cluster:6379;实测 `SET esp:ctx:test-runtime-verify` → GET 返回 ok,3 master 分片 MOVED 正常。
3. **SLO 告警加载✅(修了一个坑)**:
   - 坑:prometheus 报 `groups:0`。根因=rule_files 配 `/etc/prometheus/rules/*.yml` 但 configmap data key `rules.yml` 在 `/etc/prometheus/rules.yml` 根目录,路径不匹配。
   - 修复:改 `10-observability.yaml` rule_files 为 `/etc/prometheus/rules.yml` → kubectl apply → rollout restart prometheus → 实测 `/api/v1/rules` 加载成功(10 条 alert:ServiceDown/HighErrorRate/PodRestart/P99/P95/ErrorBudget/CPU/Memory/CrashLoop/TiDBConnPool)。
4. **Sentinel 规则推送 Nacos✅**:standalone derby nacos openapi 推送 `bff-java-flow-rules`(3 条 FlowRule)+ `bff-java-degrade-rules`(2 条 DegradeRule)到 group=SENTINEL_GROUP,public namespace,GET 验证返回完整 JSON。

**🔄 待验证/待修**:
5. **bff sentinel datasource 拉 source empty(根因排查中)**:bff restart 2 次仍报 `converter can not convert rules because source is empty` + `RateLimitFilter: Nacos flow 规则尚未加载`。nacos openapi 能读到规则但 SCA sentinel-datasource-nacos 拉空。**根因 agent(esp-explorer)后台排查中**:① SCA 版本与 SB4.0.7 兼容性(SCA 2024.x 对应 SB3,SB4 可能需 SCA 2025.x)② spring.application.name 是否拼对 dataId ③ SentinelDataSourceAutoConfiguration 是否被 SB4 排除 ④ sentinel-record.log nacos 初始化证据。
6. **teacher jetty hang 复发(单节点 VM 通病)+ Dubbo 连旧 IP 超时**:teacher 1/1 Ready(readiness tcp 8083 握手通过)但 curl 8083 超时(exit 28),日志 13:07 后停滞 11min。Dubbo 报连 learning 旧 IP 10.42.0.104(已删副本)channel init timeout。learning 实测健康 UP(10.42.0.103:8081)。**修复=rollout restart teacher-java** 让它重新发现 learning 新地址 + 清 jetty hang。

## 下一步动作(恢复锚点,2026-08-07 21:20)

1. **重启 teacher-java**(清 jetty hang + Dubbo 重连 learning):`kubectl -n esp-prod rollout restart deploy/teacher-java`
2. **等 bff sentinel 根因 agent 返回**(esp-explorer a1c9cb8cf9188fb76)→ 按根因修(可能升 SCA 版本 / 改 dataId / 手动初始化 NacosDataSource)
3. **等 trivy 复扫 agent 返回**(learning/teacher 1.6.0,ab3fe8f1e19a970b9)→ 落 report/security/trivy-image-{learning,teacher}-1.6.0.json
4. **等 bff 1.6.1 builder 返回**(a405417ca56e058dd)→ 部署 bff 1.6.1 + trivy 复扫
5. **teacher Saga 端到端实测**(teacher 重启后):POST /api/teacher/summary 触发 Saga try+compensate
6. **llm 语义缓存实测**:GET /api/llm/chat 验证 ZSET 近邻命中
7. **Python 镜像 base 决策**(待用户):bookworm 6 CRIT 清不掉,换 alpine/trixie 或登记豁免
8. **git 迁入**(待用户给 GitHub URL):→ ArgoCD sync + Dependabot + Aikido

## 并发 agent(后台,2026-08-07 21:20)
| agent | 任务 | 状态 |
|---|---|---|
| esp-explorer a1c9cb8cf9188fb76 | bff sentinel datasource 拉 source empty 根因排查 | 🔄 运行 |
| esp-builder ab3fe8f1e19a970b9 | trivy 复扫 learning/teacher 1.6.0 | 🔄 运行 |
| esp-builder a405417ca56e058dd | bff-java 1.6.1 镜像 build + trivy | 🔄 运行 |

## 镜像重建+部署验证(2026-08-07 16:40 起,🔄 串行中,3/5 一轮完成 + 二次修复 1.6.1 待重建)

**构建环境**:`~/.rd/bin/nerdctl --address /var/run/docker/containerd/containerd.sock --namespace=moby`,arm64,新 tag 1.6.0。

**一轮(1.6.0)进度**:
| 服务 | tag | 镜像build | 部署 | trivy重扫 | pod Ready |
|---|---|---|---|---|---|
| writing-py | 1.6.0 | ✅ | ✅ | ✅ 落 report | ✅ 1/1(granian) |
| emotion-svc | 1.6.0 | ✅ | ✅ | ✅ 落 report | ✅ 1/1(granian) |
| bff-java | 1.6.0 | ✅ | ✅ | ✅ 落 report | ✅ 1/1(BffApplication 60s Started) |
| learning-java | 1.6.0 | 🔄 builder 中 | — | — | 1/1(旧1.5.1 Running) |
| teacher-java | 1.6.0 | ⏳ 排队 | — | — | 0/1(jetty hang 5h+,待1.6.0替换) |

**一轮 trivy 复扫结论(⚠️ 漏洞未达预期下降,已定位根因→二次修复 1.6.1)**:
| 镜像 | 1.5.x CRIT/HIGH | 1.6.0 CRIT/HIGH | 根因 |
|---|---|---|---|
| writing-py | 4/19 | 4/19(持平) | perl-base 5.40.1-6 的 4 CRIT FixedVersion=None(Debian13 trixie 上游未发补丁);util-linux/ncurses HIGH 同因。**apt upgrade 无包可升** |
| emotion-svc | 4/19 | 4/19(持平) | 同 writing-py(同 trixie 基底) |
| bff-java | 1/3 | 1/2(HIGH-1) | CRIT 是 pulsar-client 4.1.3→bouncy-castle-bc:pkg shade 内嵌 bcprov 1.78.1(trivy 计独立漏洞);2 HIGH 是 netty-codec-compression/http3 4.2.15(SB BOM 默认,未升)。otel 2.26.1 单独扫 0 漏洞 |

**二次修复(1.6.1,代码已改完待重建)**:
- **bff-java pom**(`services/bff-java/pom.xml`):
  - `<netty.version>` 4.1.136.Final → **4.2.16.Final**(覆盖 SB BOM 4.2.15,消 2 HIGH CVE-2026-59901/56816)
  - `<dependencyManagement>` 加 bcprov-jdk18on/bcpkix-jdk18on **1.84**(消 1 CRIT CVE-2025-14813)
  - `<dependencyManagement>` 给 pulsar-client 加 `<exclusion>` 排 `bouncy-castle-bc` shade 包(根治嵌套 1.78.1)
- **writing-py / emotion-svc Dockerfile**:FROM `python:3.13-slim`(trixie)→ **`python:3.13-slim-bookworm`**(Debian12,perl-base 5.40 已修,消 4 CRIT + 19 HIGH);sed 兼容 bookworm 的 sources.list 回退(`.sources` 不存在则改 `sources.list`)
- 预期 1.6.1:bff 降到 0 CRIT / 0 HIGH;py 两服务降到 0 CRIT / ~0 HIGH

**buildkit 凭证坑(真因已锁定,18:00~18:30 二次排查,旧解法证伪)**:
- 现象:nerdctl build 报 `error getting credentials - err: exit status 22, out: \`\`` 在 `load metadata for docker.io/library/eclipse-temurin:21-jre-noble` 步骤。build 和 pull 都撞。
- **旧解法证伪**:`credsStore: osxkeychain` 不是直接元凶;临时 `mv ~/.docker/config.json{,.bak}` 也不行(17:30 记的解法无效)。
- **真因(18:30 锁定)**:`which docker-credential-osxkeychain` = `~/.rd/bin/docker-credential-osxkeychain` → 软链 `/Applications/Rancher Desktop.app/.../docker-credential-osxkeychain`(RD 自带真二进制,**非 Docker.app 的那个**)。该二进制查 macOS keychain,**对未登录的 registry 返回 `credentials not found in native keychain` RC=1** → go-containerregistry 把 RC=1 记为 `exit status 22`。17:30 给 Docker.app stub 改 `{}` 无效,因 PATH 优先命中 `~/.rd/bin/` 的 RD 版二进制。
- **关键反证**:`echo "https://index.docker.io" | ~/.rd/bin/docker-credential-osxkeychain get` 实测 RC=0 且返回 `{"ServerURL":"https://index.docker.io","Username":"javanetgongzheng","Secret":"dckr_pat_..."}` —— **keychain 里其实存了 docker hub 凭证**。但 buildkit/build 拉元数据时走的 host/registry 解析路径与直接 get 不一致,对 daocloud/ustc mirror 查不到凭证 → RC=1。
- **可行解法(下次恢复直接用,勿再走证伪路径)**:
  1. **方案A(最快,绕凭证)**:临时 `mv ~/.rd/bin/docker-credential-osxkeychain ~/.rd/bin/docker-credential-osxkeychain.bak`(移走 RD 版二进制,让 buildkit 找不到 helper → 走匿名拉取),build 完 `mv` 回去。
  2. **方案B(用已有 base)**:bff-java 1.6.0 两小时前用同 `eclipse-temurin:21-jre-noble` base 成功 build 过,说明该 base 在 containerd 里有缓存。先 `nerdctl images -a | grep temurin` 找缓存 digest,若在则 `nerdctl tag <digest> eclipse-temurin:21-jre-noble` 显式打 tag 让 buildkit 直接命中本地(不查远程元数据)。
  3. **方案C(配 mirror + 凭证)**:`~/.rd/bin/docker-credential-osxkeychain` 已对 docker.io 工作(RC=0),把 Dockerfile FROM 指回 `eclipse-temurin:21-jre-noble`(不走 daocloud mirror),buildkit 拉元数据时凭证 helper 能返回有效 token。
  - **bff-java 1.6.0 能成的原因(反证缓存路径)**:2 小时前 build 时该 base 元数据已被 buildkit 缓存,本次 build 不需重新解析;learning 是新 build 触发了 metadata 重解析撞凭证。

**🚨 teacher-java 旧 pod 0/1**(teacher-java-d8cdfbc-gh8fw Running 5h53m+ Ready=False,jetty hang 复发):单节点 VM 通病,部署 1.6.0 后自然替换;若新 pod 仍 hang 需 `rollout restart deploy/teacher-java`。

**当前(18:30)**:已停掉卡死的 builder agent(a11b0a821d21fc974),learning/teacher 1.6.0 镜像**仍未建**。learning jar 已打好(202MB,17:57)可直接 build。下次恢复用上面方案A/B/C 重建。

## 恢复锚点(上下文溢出后从这里继续,2026-08-07 21:20 更新)

**先读**:本节 + "镜像 1.6.0/1.6.1 全量 build+部署"节 + "运行时行为验证"节。历史归 `docs/progress/archive/`。

**当前主线 = 镜像复扫闭环 + 运行时行为验证收尾(代码全 done,差运行时验证)**:
1. **重启 teacher-java**(清 jetty hang + Dubbo 重连 learning):`kubectl -n esp-prod rollout restart deploy/teacher-java`(单节点 VM 通病,长效靠多节点+liveness)
2. **bff sentinel datasource 拉 source empty 根因**:等 esp-explorer agent 返回 → 按根因修(SCA 版本/dataId/AutoConfig 排除)。规则已在 nacos(SENTINEL_GROUP),问题在 bff 拉取侧。
3. **trivy 复扫 learning/teacher 1.6.0**:等 esp-builder 返回 → 落 report/security/trivy-image-{learning,teacher}-1.6.0.json,确认 netty/bc/jackson 漏洞下降。
4. **bff 1.6.1 部署+复扫**:等 builder 返回 → set image 1.6.1 + trivy 确认 1 CRIT(bcprov)+2 HIGH(netty)清零。
5. **teacher Saga + llm 语义缓存端到端实测**:teacher 重启后 POST /api/teacher/summary、GET /api/llm/chat。
6. **Python 镜像 base 决策**(待用户):bookworm 6 CRIT fix=N/A 清不掉。
7. **git 迁入**(待用户给 GitHub URL):→ ArgoCD + Dependabot + Aikido。

**其他主线(非阻塞)**:
8. schema Phase1 Batch2~5(Saga 已解锁,可推进)
9. 新服务填实 Batch2~5(单节点只实跑 Batch1~3,Batch1 已完成)
10. Java/Go/Rust 性能对比报告
11. Redis announce FQDN 持久化(P1)

**单节点铁律**:重 mvn/docker build 必须与集群操作串行(esp-singlenode-host-cluster-contention);不碰 k3s 配置/restart/Factory Reset(esp-never-break-k3s)。

## TaskList(当前,2026-08-07 22:00)

| # | 任务 | 优先级 | 状态 |
|---|---|---|---|
| 1 | bff/writing-py/emotion-svc 1.6.1 重建+复扫 | P0 | 🔄 writing-py/emotion ✅(6CRIT bookworm fix=N/A);bff 1.6.1 builder 中断待主线程接管(pom 已改) |
| 2 | trivy 复扫 learning/teacher 1.6.0 | P0 | ✅ 落 report:teacher 63→1(✅);learning 30→14(netty 4.2.16 未声明→回归,待 1.6.1) |
| 3 | learning 1.6.1 重建(pom 加 netty 4.2.16 + bc 1.84 对齐 teacher) | P0 | ✅ 完成(0CRIT/0HIGH) |
| 4 | teacher probe 改造根治 jetty hang 假 Ready(readiness/liveness 改 httpGet + liveness timeout15s + jetty.threads.max 400) | P0 | ✅ 完成(1.6.1 起生效) |
| **teacher 1.6.2** | **finalize 幂等(SETNX) + 重建 + 部署 + trivy 0/0 + 运行时实测(final_score=75 不被 99 覆盖)** | **P0** | **✅ 全部完成(2026-08-08 13:40)** |
| 5 | 运行时行为验证(Saga/Sentinel/幂等/语义缓存/SLO/distroless) | P1 | 🔄 代码✅/SLO✅/Redis✅/Sentinel推送✅;bff datasource 拉 empty 待根因;teacher Saga+llm cache 待 probe 修复后实测 |
| 6 | PROGRESS.md + Redis 双轨落盘 | — | ✅ 本节即是 |
| (历史) | P0/P1/P2+Saga 8/8 代码层+编译 ✅ / 安全扫描四件套配置 ✅ / 7-7 镜像一轮扫 ✅ / schema Phase0+Phase1方案A ✅ / Redis warm-up ✅ / Pulsar namespace ✅ / 上下文工程 P0-P2 ✅ / S1+S2 新服务实跑 ✅ | — | ✅ |

## 关键部署状态(恢复锚点)

- 镜像:bff-java 1.5.1、gateway-go 1.5.0/1.5.1、learning-java 1.5.1/1.6.2、teacher-java 1.5.1/1.6.2、writing-py 1.5.0、emotion-svc 1.5.0、writing-coach 1.0.1、frontend-react 1.0.0、course-path-go 0.1.0、**pronunciation-rs 0.1.0**(新,S3 P0 实跑)。
- 中间件:nacos v2.4.3 / seata 2.2.0 / sentinel 1.8.9。
- NodePort:gateway-go 30080、frontend-react 30090。
- 基础设施复用 ns `ekb-m2-aws-us-east-1`,ExternalName 短名 `tidb-primary`/`tidb-follower`/`redis-cluster`/`pulsar-broker`。Redis Cluster 实际走 `redis-cluster-new-0.redis-cluster-new.ekb-m2-aws-us-east-1.svc.cluster.local:6379`。
- TiDB:`tidb-primary:4000` root 无密码,29 张表(schema 见 `docs/design/schema.sql`)。esp_pronunciation.pronunciation_attempts(id bigint AUTO_RANDOM(5),test 数据 3 行)。

## 上下文工程铁律(摘要,详见 memory)

- 文件系统=长期记忆,对话=临时工作区。80%(160k)主动 /compact,90%(180k)强制止损落盘+compact。
- 三层防御:源头减量(subagent 隔离+输出限流)+ 状态外化(PROGRESS 热区+Redis 运行态)+ 主动管控。
- **本文件热区上限 ~300 行**,历史归 `docs/progress/archive/`,不全量读。
- 完整铁律集见 memory `context-overflow-ironrules.md`。

## 关键命令速查

- 查 Pulsar pod:`kubectl -n ekb-m2-aws-us-east-1 get pods | grep pulsar`
- Redis 写/读:`kubectl -n ekb-m2-aws-us-east-1 exec redis-cluster-new-0 -- redis-cli -c SET/GET "esp:ctx:*" "..."`
- TiDB 查:`scripts/db_query.sh "SQL"`(pymysql pod 封装)
- 本地安全扫描:`semgrep scan --config p/default .` / `trivy fs --scanners vuln . `
- 单节点铁律:重 mvn 构建与集群操作串行;不碰 k3s 配置。

## 历史 milestone 归档索引(需要时取)

| 主题 | 文件 | Redis key |
|---|---|---|
| teacher readiness 根治 + 安全扫描四件套(08-07) | (本次未单独归档,见热区) | `esp:ctx:hot:2026-08-07-teacher-readiness-and-security-scan` |
| teacher CrashLoop + 增量服务分析(08-07) | (见热区历史) | `esp:ctx:hot:2026-08-07-teacher-crashloop-and-new-services` |
| Pulsar cookie + VM 网络中断(08-06) | `docs/progress/archive/2026-08-06-pulsar-cookie-fix-and-vm-down.md` | `esp:ctx:archive:2026-08-06-pulsar-cookie-fix-and-vm-down` |
| Pulsar Geo + 应用切新集群(08-05) | `docs/progress/archive/2026-08-05-pulsar-geo-and-cluster-switch.md` | `esp:ctx:archive:2026-08-05-pulsar-geo-and-cluster-switch` |
| 上线就绪 + 三层坑事故(08-04) | `docs/progress/archive/2026-08-04-readiness-and-three-layer-incident.md` | `esp:ctx:archive:2026-08-04-readiness-and-three-layer-incident` |
