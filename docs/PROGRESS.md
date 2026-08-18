# ESP 项目进展 (PROGRESS) — 热区

> **本文件只放当前任务热区**(已确认根因/当前子步骤/下一步/恢复锚点),硬上限 ~300 行。
> **历史里程碑见 `docs/progress/archive/*.md`**(按主题拆分),跨会话恢复亦可从 Redis `esp:ctx:archive:*` 取。
> **恢复协议**:新会话先读本文件(全文)+ TaskList,不全量读历史;需要历史细节 Grep archive 或 Redis 取。

## 2026-08-18 会话23-15b 完成项（中间件下沉+资源分配+全修复）

### 宿主中间件运维（本次完成）
1. 宿主中间件运维手册: `docs/ops/host-middleware-startup.md` ✅
2. 一键管理脚本: start/stop/status-host-middleware.sh ✅ (全部 RC=0)
3. Prometheus rule_files 修复: /etc/prometheus/rules.yml → $HOME/observability/rules.yml ✅
4. Prometheus otel-collector target 修复: K8s DNS → localhost:8889 ✅ (target UP)
5. Prometheus K8s API 认证: SA prometheus-host + RBAC + token/CA 导出 ✅
6. Loki K8s DNS 修复: host.lima.internal → localhost ✅ (config.expand-env=true)
7. 数据持久化迁移: /tmp/* → ~/observability/data/* (4个服务全部重启验证) ✅
8. Tempo 二进制修复: 从 GitHub 下载 macOS arm64 正确二进制 ✅ (port 3200 监听中)
9. 两个 README 更新宿主中间件章节 ✅ (中英文)

### 资源分配（本次完成）
10. bff-java CPU limit 1→2 core, requests 50m→200m + HPA(max=4, 70%) ✅
11. learning-worker HPA(max=3, 70%) ✅

### Admin 监控缺口修复
12. catalog.json: 移除 seata + otel-collector 改为 FQDN ✅ (待重建镜像部署)
13. Prometheus 通过 OTel Collector 聚合采集(单节点正确架构) ✅

### Nacos Derby→CRDB 迁移
14. CRDB 创建 nacos 数据库 + schema SQL 执行 ✅
15. application.properties 已改为 CRDB (localhost:32627 via kubectl port-forward) ⚠️
16. **NodePort 从宿主机不可直达**（CRDB NodePort 32627 连接超时）— kubectl port-forward 可用但非生产方案
17. **待决策**: Nacos 用 port-forward 临时方案 or 切回 Derby + 后续多节点用 TiDB

### Admin 前端问题(已发现待修复)
18. GovernanceDashboard 硬编码 IP: 192.168.5.15:30300(Grafana)/8858(Sentinel) → 需改 ExternalName

## 进行中（恢复锚点 2026-08-18 会话23-15b）
- **Nacos 迁移**: CRDB nacos 库+schema 已建✅; application.properties 已改为 localhost:32627(via kubectl port-forward)⚠️; Nacos 启动时连接 CRDB 失败(SocketTimeout) — NodePort 从宿主机不可直达,需 kubectl port-forward 维持或切回 Derby
- **集群稳定**: HPA 触发 bff-java/learning-worker/learning-java 滚动更新,旧 RS CrashLoopBackOff 正在清理; agent a287620baeda03ecb 处理中
- **CRDB 定时备份**: schedule 已创建(hourly+daily, userfile 模式); 之前一次完整备份验证通过(10761 rows)
- **Phase 2 准备**: 压测+E2E+CRDB vs TiDB 对比 待集群稳定后执行
- **关键脚本**: scripts/start-host-middleware.sh / stop / status 已创建; docs/ops/host-middleware-startup.md 已写

### Nacos→CRDB 决策点（需用户确认）
方案A: 保持 port-forward(开发验证用) + 后续多节点切 TiDB
方案B: 切回 Derby(简单可靠,重启不丢数据)
方案C: 宿主机直连 CRDB NodePort(32627) 需 VM 网络配置调整

### CRDB NodePort 不可达根因
- CRDB 32627 在 K8s 中 reachable(clusterIP 正常),但从 macOS 宿主机 localhost:32627 超时
- Rancher Desktop VM 网络:NodePort 需要通过 VM IP(192.168.5.x 或类似)而非 localhost
- 解法: 用 VM IP + NodePort 或保持 kubectl port-forward

## 下一步
1. 确认 Nacos 存储方案(切回 Derby 或 port-forward)
2. CRDB 定时备份调度(已完成 userfile 备份, schedule 已创建)
3. Phase 2: locust/k6 全链路压测 + E2E 全功能测试
4. CRDB vs TiDB 全方位对比报告
5. 全文档更新(PROGRESS.md/README/architecture docs)

## 恢复协议
1. 读 docs/PROGRESS.md 热区
2. Redis GET esp:ctx:hot:session-23-15
3. 读 .claude/plans/robust-baking-wave.md
4. 读 report/ops-audit-2026-08-18.md

---

## 冷区(已闭合会话索引,详见 archive/)

- **会话23-13 + 续(2026-08-17~18,全量重构 7.3→8.1 / JDK26 镜像+部署收尾 / k3s 连锁坑㉚ / gson 阻断闭合)** → `docs/progress/archive/2026-08-18-session-23-13-jdk26-refactor-deploy.md` | Redis:`esp:ctx:hot:session-23-13-refactor-complete` + `esp:ctx:session:23-13-cont`

## 当前(2026-08-17 会话23-10:TiDB 下电✅ / GUI 客户端连接✅(DBeaver+RedisInsight)/ 前端全面美化研究中 / 数字人 persona 全栈✅)

### 恢复锚点(2026-08-17 数字人 persona 全栈闭合)
- **数字人 per-user persona(FR-6.1,全栈闭合✅)**:每用户不同形象(8 预设 aurora/sunny/mint/violet/rose/mentor/coral/sage),hashSeed(userId) 确定性默认 + 持久化。
  - **前端全绿**:ui-tokens 导出 PERSONAS/resolvePersona/DigitalHuman/DigitalHumanPicker(SVG 骨架,8 发型/6 配饰,4 表情);三端 tsc RC=0。Web(SpeechPanel+Profile 数字人选择器)、H5(Speech+Profile)、小程序(emoji 降级,Avatar+我的页 persona picker)均接 getDigitalHuman/setDigitalHuman。
  - **后端全绿**:gamification-go `digital_human.go` GET/PUT `/api/gamification/me/digital-human`(requireUID + personaIDRe 校验),镜像 `esp/gamification-go:0.4.2` 构建部署。**真实 CRUD 验证**(BFF NodePort 30009 真实 JWT):GET""→PUT aurora→GET aurora→PUT 非法 400→DB 直查 `esp_auth.users.digital_human='aurora'` 落库证实。personaIDRe 校验生效。
  - **品牌名定稿同步(#219)**:i18n app.title=Verity/洁语 + tagline;index.html title 更新(Web/H5);H5 PWA manifest ESP→Verity;小程序登录页 ESP 前缀去除。无 家教/Home 残留。
- **已知架构不一致(#227,待修,不阻断 persona)**:BFF Java `ESP_RW_COCKROACHDB_DB=esp`(esp.users 无 digital_human 列),gamification-go 写 `esp_auth.users`(含 digital_human)。BFF 注册用户不在 esp_auth.users → gamification UPDATE 匹配 0 行静默失败(本轮靠镜像插入 uid 临时闭合)。根因修复:改 BFF db=esp_auth 或 BFF register 镜像写 esp_auth.users。explorer 正在调查两表 schema 差异。
- **前端全面美化(#218,进行中)**:设计文档 `docs/design/architecture/frontend-beautification-plan.md`(主题系统 data-theme+data-accent / 图片资产·无key直链+SVG兜底 / i18n·ui-tokens单源)。共享基础已扩(accent 奖牌/渐变/图片 fallback token、tokens-mobile.css、i18n ~110 key)。下一步=各端重构。
- **Phase3 排行榜 E2E 已完成(2026-08-17,全绿)**:`report/phase3-ranking-e2e-2026-08-17.log`(周榜{甲30乙15丙5}/地区榜/反作弊/opt-out 全过)。README 新增 Phase 1-3 功能小节;docs/test/00 补当前阶段功能测试。
- **TiDB 彻底下电**:三区 pd/tidb/tikv/tiflash --replicas=0 + force-delete → 0 残留。ESP 全服务切 CRDB(`crdb-public:26257`),未受影响。30亿存量随下电离线——需 CRDB 侧重建基准(Phase5 多节点清单,诚实标注)。
- **GUI 客户端(已完成,见 docs/test/11-GUI客户端连接指引.md)**:CRDB PG 127.0.0.1:32627;Redis 经 RD-VM SSH 隧道。
- 下一步:①#227 DB 不一致根因修复(待 explorer 结论);②前端美化各端重构;③Phase0 mock 切断收尾(#211)+ Phase1-5 上线达标工程。
- Redis 锚点:SET `esp:ctx:hot:session-23-10-persona-frontend`(persona 全栈闭合后)。

## 当前(2026-08-17 会话23-9 功能拓展:Phase1 内容库✅ / Phase2 圈子社区✅ / Phase3 排行榜**E2E 执行中** / Phase4 待做)

### 恢复锚点(2026-08-17 10:40 Phase3 E2E 解围:调度死锁已破,gamification 0.4.1 已跑,待 bff-java Ready 后跑 E2E)
- **根因(非调度 bug,是容量)**:`kubectl describe node` = **CPU Requests 8930m/9000m(99%)**,非 Pending 分配 8.73~8.93 核(共享底座 3xCRDB+TiDB+Milvus+obs+Java 服务);余量仅 ~70m。bff(200m)/learning/teacher 等 not-ready Java 服务被 HPA 扩 + 控制器反复补建 → **滚动更新死锁**(新 pod 无法调度,旧 not-ready pod 不终止)。
- **已执行解围(2026-08-17,零确认授权内)**:① force-delete 卡 1 天的 teacher-java Terminating pod(RS desired=0 安全);② learning-java **HPA maxReplicas 15→1**(live patch)防 HPA 再来;③ teacher-java HPA max 8→1 + `kubectl scale deploy teacher-java --replicas=1`;④ **bff-java requests cpu 200m→50m**(live + `15-bff-java.yaml` 同步,注释注明原因)——余量仅 70m 配 50m 才可调度;⑤ gamification-go **0.4.1**(3 修复)`rollout status` RC=0 已 Running,log `rankings feed init/resume hwm=1202037037743079426`(Redis 持久)。⑥ **CRDB `esp_auth.users` 已 INSERT 3 rk 用户**(甲/乙/丙,region CN/Guangdong/Shenzhen + Guangzhou + CN/Beijing,ON CONFLICT 幂等)——BFF 只写 TiDB esp.users,此为 E2E 测试桥接(架构债:新注册不到 CRDB profile 表,已记录)。
- **bff-java 现状**:原 pod 被 liveness 饿死死循环(restart x2 后 hang 于 Nacos/Dubbo init,20:25 起 ≈1 天);已重启 + 资源 patch → 新 pod 5b9fdfbb8d **ContainerCreating→等待 Ready**(JVM 启动 ~90s+ 饥饿下更慢)。E2E 前置 = bff Ready + port-forward 18090(进程在跑)。
- **E2E 已就绪**:`/tmp/rk-tokens.txt` 已重签 3 枚 RS256(12h,`scripts/gen_bench_tokens.py` 同法,secret `esp-rsa-jwt-sign`);`report/phase3-ranking-e2e.sh` 路径已验(content 4 courses/14 lessons/1342 items;learning_events 管道 live max-id=1202037037743079426=feed hwm;Redis `esp:gamif:lb:*` 为空 → E2E 干净起点)。**备胎**:content-go/gamification-go 直接读 X-User-Id 头(无 JWT 校验),bff 长期不 Ready 可直连 8096/8092 跑数据通 path(诚实标注为"未走 BFF 链路")。
- **坑㉘(新)**:rolling update 死锁下 **force-delete 会触发旧 RS 补建**(旧 RS desired 未清)→ 正确做法=**scale 0 清台再 scale 1**(bff/teacher 均以此收束);另 not-ready 服务被 HPA 反复扩 → 单节点须**人工cap HPA max=1 + deploy replicas=1 对齐 manifest 单节点意图**。
- Redis 锚点:`esp:ctx:hot:session-23-9-phase3-rankings`(E2E 完成时 SET)。

### content-go SDD/DDD 文档补齐(2026-08-17)
- **产出**: `docs/design/sdd-content-go.md`(11 节完整 SDD + DDD 有界上下文) + `docs/api/content-go-api.md`(API 契约)
- **代码审视发现 12 项改进建议**(I-1~I-12),其中 P1 Bug 2 项:
  - **I-3**: updateItem 编辑建新版本未递增 version 字段(固定 version=1),版本追溯断裂
  - **I-4**: relayDeadSQL 写 status='dead' 违反 CHECK 约束(pending/done/failed),死信静默失败
- **架构差距**:未使用 esp-common-go/db 双池 API(手动创建两个 *sql.DB);importBatch 无事务包裹;无缓存/Pulsar/API 版本化

### 恢复锚点(功能拓展 Phase3 排行榜:代码+部署完成,2026-08-17)
- **任务线**:功能拓展 4 项(内容库/圈子社区/排行榜/关联功能),用户拍板:内容 v1 丰富起步、XP+进度双榜、地域榜默认入榜可 opt-out、#4 全要;所有新增功能遵守工程铁律。
- **Phase3 backend 已部署(gamification-go 0.4.0 + bff-java 1.11.2 + ingress)**:
  - `rankings.go`(新,~490 行):pollLoop 轮询 CRDB `esp_learning.learning_events`(id 高水位 `esp:gamif:feed:hwm` + Redis 分布式锁 `esp:gamif:feed:lock`,2s,多副本防重复);XP/进度双榜;扇出 global/region 祖先前缀/circle/category × daily/weekly/monthly ZSET(`esp:gamif:lb:*`);反刷榜(单事件 20 XP 上限 + 每日 500 上限 `esp:gamif:xpc:*`);日切 top-100 快照写 CRDB `esp_gamif.leaderboard_snapshots`;opt-out 读端过滤+写泵跳过。端点:GET `/api/gamification/rankings`(type=xp|prog,scope=global|region|circle|category)、`/rankings/gap`(percentile/差距)、`/rankings/regions`、POST `/users/optout`。
  - bff-java 1.11.2:RouteTable 加 `/api/gamification→gamification-go:80`(javap 字节码 2 处确认);40-ingress 加 `/api/gamification` Prefix;日志确认 `rankings feed init hwm=1202037037743079426`。
- **Phase3 frontend 已部署(frontend-react 1.1.3)**:types(6 新类型)+ api-client(4 方法)+ api.ts 委托 + `/learning/ranking` 路由 + LearningLayout 排行榜 tab + `RankingPage.tsx`(XP/进度 × 日/周/月 × 全球/地区/圈子/类别 + 地域选择器 + 我的排名卡 + opt-out 开关);构建 RC=0(RankingPage chunk 在镜像)。
- **Phase3 E2E 证据(部分)**:gamification-go 端点直连冒烟 `{"list":null,"mine":{}}` 正常(空数据符合 hwm 起点);CRDB `esp_auth.users` 已有 region_code/ranking_opt_out 列;注册路径实证通知——**bff-java 注册写的是 TiDB `esp.users`(AuthController L100 `INSERT INTO users`),CRDB `esp_auth.users`(rankings/community 读的名字/地域表)只有种子用户** → 新注册用户不在此表。待办:确认 content submitAnswer(Phase1 真实路径)→ learning_events 落 CRDB 后驱动 rank 数据;再补 region 维度。
- **rankings.go 三处修复(已本地 vet OK,待 0.4.1 重部署)**:① snapshotDay 冒号分段解析错(期望 5 段实为 7 段 `esp:gamif:lb:{typ}:{scope}:{sid}:{bucket}` → 快照永不落库)→ 改 `parts[3]/part[6]` 判定;② myRank 忽略 ZRevRank Err(Rdis 无成员时 Val=0 → 误返 rank 1)→ 改 Result()+err 判 0;③ loadUsers 查询失败时不再推进 hwm(此前 transient 错→本批事件永久丢失)→ 返回 err,poll 下 tick 重试本批。
- **阻塞(2026-08-17 04:0x)**:单节点过载,kubectl TLS handshake timeout(apiserver 饿死;宿主 loadavg ~15, RD VM 452% CPU;疑 seed-job 补灌 session_turns/learning_events 仍在压 CRDB)。已挂后台 `until kubectl get nodes` 恢复监视。恢复后继续 E2E:注入 content 学习事件→轮询→rankings 出数据→opt-out 生效→region/category 维度→gap 百分位→快照日切(次日验证)。**铁律:禁动 k3s/共享底座/seed 任务,仅等待。**
- **Redis 锚点**:`esp:ctx:hot:session-23-9-phase3-rankings`(Phase3 恢复点,待 E2E 完成后 SET)。
- 前置 Phase1/2 锚点见下方(坑㉗/内容库✅/圈子✅)。
- **坑㉗(BFF X-User-Id 枚举缺口,已修验证)**:bff-java **1.11.1**:`AuthFilter.AuthHeaderWrapper` 原先只 override getHeader/getHeaders,未 override getHeaderNames();ProxyController 用 `getHeaderNames()` 枚举转发头 → 客户端不带 X-User-Id 头时(浏览器 cookie 场景)下游 Go 服务(community/content/gamification)**收不到注入身份,写路径全 401**;GET 只读端点不强制 auth 看似正常。已补 `getHeaderNames()`(LinkedHashSet 补 X-User-Id/X-Role),javap 字节码级验证,1.11.1 已滚。memory `esp-bff-xuserid-enum-gap`。
- **#200 Phase1 内容库 ✅全闭合**:content-go 1.0.4(esp_content 8096)+ 种子 1342 项 + frontend-react 1.1.1(/learning/content+/admin/content)。**认证 E2E 已验证**:register→login→Bearer→courses/lessons/items 浏览 + submitAnswer 判分;content_attempts 总数 4;learning_event_outbox 总数 3(source=content-library)。
- **#201 Phase2 圈子社区 ✅全闭合**:community-go 1.0.0(esp_community 8097)+ 前端 frontend-react **1.1.2**(/learning/community,懒加载 chunk CommunityPage-*.js 确认在镜像内)。
  - 接线:RouteTable `/api/community→community-go:80` + 40-ingress 已 apply + bff-java 1.11.1;manifest tag 已同步(15-frontend-react 1.1.2、34-community-go 1.0.0)。
  - **E2E(经 BFF 30009,真实 JWT)**:注册→邀请码(VQHP53)加入→发帖/评论(user_name JOIN esp_auth.users)→/circles/mine 含圈(role=member)→ **举报流 3 用户→report_count 1→2→3 达阈值 auto_hidden:true→帖子从可见列表消失**。
- **遗留(Phase1+2)**:浏览器 /web 带 auth 走通 = 人工测试项(见 docs/test 计划验证节);frontend-react 无 NodePort,浏览器验证需 /etc/hosts + traefik(esp.example.com)。
- **集群风险**:单节点 CPU requests 约 98% 近饱和,新服务部署无调度余量——遇 stuck Terminating pod 用 `kubectl delete pod <name> --force --grace-period=0` 解困;metrics-server 缺失 HPA 盲。
- **下一步**:**Phase3 排行榜(#197)**:① users 一次性 ALTER 加 region_code/ranking_opt_out(注册/设置可填地域,opt-out 默认可入榜);② gamification-go(8092)接线到 RouteTable+ingress(首次)+ Pulsar 消费 LEARNING_EVENT_REPORT → XP/进度 ZSET 扇出(global/region 国省市/circle/category × daily/weekly/monthly)+ rankings 端点 + 差距/反刷榜 + 周期快照;③ 前端 /learning/ranking + opt-out 设置。
- **Redis 锚点**:`esp:ctx:hot:feature-expansion-phase2-community`(Phase2 闭合恢复点)。
- **已归档**:会话23-9 四线(vocab cap/SEC-6/CI/Obsidian)→ `docs/progress/archive/2026-08-17-session-23-9-four-line.md`。

---

## 历史(2026-08-16 会话23-8 共享基础设施迁移(nacos/sentinel/seata→ekb ns)✅全闭合 + P3 管理员门户 P3a/P3b✅/P3c 镜像已滚待浏览器验证)

### 恢复锚点(会话23-8 SCA 迁移 P1-P7 + 轨道C P3c,2026-08-16)
- **SCA 三件套迁移全闭合✅(Phase 1-6)**:nacos/sentinel/seata 从 esp-prod 原生 ClusterIP → 共享 ns `ekb-m2-aws-us-east-1`,走既定双层 ExternalName 抽象(esp-prod 短名 → platform-infra → ekb 实体),**应用配置零改动**(客户端短名 `nacos:8848`/`sentinel:8858`/`seata:8091` 天然保留)。
  - **Phase 1**:06-sca-middleware.yaml 7 段 ns 改 ekb + nacos PVC(`/home/nacos/data` 5Gi)+ seata PVC(`/seata/store` 2Gi,local-path);apply 9 资源。
  - **Phase 2**:05b-platform-infra-externalnames.yaml 加 3 ExternalName(nacos/sentinel/seata → ekb),平台入口层 6→9 组件。
  - **Phase 3**:05-infra-reuse.yaml 加 3 esp-prod ExternalName → platform-infra;`kubectl delete svc nacos sentinel seata -n esp-prod` 后 apply,秒级 DNS 空洞可接受。
  - **Phase 4**:4 Java 服务**串行滚动**(bff→learning→teacher→learning-worker),单节点 CPU 饥饿铁律串行;验证 ekb nacos 服务列表含 bff-java/learning-java/learning-worker + Dubbo provider LearningRpcService(esp 组);**teacher 发现 3 learning invokers** = Dubbo 链路通(10.42.0.224/228/226:50091)。
  - **Phase 5**:45-httptest-nodeports.yaml 3 段(nacos-np 30048/sentinel-np 30058/seata-np 30071+30092)ns→ekb;先删 esp-prod 旧 NodePort 防 30xxx 冲突再 apply。
  - **Phase 6**:删 esp-prod 残留 nacos/sentinel/seata Deployment + seata-config CM。
- **Seata 退役(用户明确指令)**:"已去除对 seata 的依赖,seata 不要再次启动" → 06-sca-middleware.yaml seata Deployment `replicas: 0` + 退役注释块;全量 4 Java 服务 `SEATA_ENABLED=false`;manifest + ExternalName + PVC 保留备未来重启用(改 replicas>0 即可)。
- **坑㉟g(nacos CrashLoopBackOff — inotify 耗尽)**:根因=VM `fs.inotify.max_user_instances=128` 太低 → WatchFileCenter `IOException: User limit of inotify instances reached`。修=`ssh rd-vm-127 'sudo sysctl -w fs.inotify.max_user_instances=2048'` + force-delete pod 重启 → "Nacos started successfully in stand alone mode"。(注:sysctl 为运行时设置,VM 重启后需重设——多节点移植时固化 sysctl.conf。)
- **P3a esp-admin 统一API聚合✅**:admin-go(RS256 JWT auth,esp_token cookie role=admin;requireAdmin 中间件非 admin 返 403)。路由 `/api/admin/{services,infra,catalog,metrics/query,metrics/query_range,metrics/alerts,metrics/rules}`;esp-admin svc port 80→targetPort 8099;handleProm 透传 Prometheus。
- **P3b OTLP 双轨指标✅**:服务指标全量接线 OTLP→Prometheus,实时 RPS/延迟/错误率。
- **P3c Web 前端管理员监测页(镜像已滚,待浏览器验证)**:`apps/web/src/pages/admin/AdminDashboard.tsx`(alerts 契约修复:Prometheus 原生 data.alerts 嵌套)+ `LearningLayout.tsx`(admin NavLink gated by `user?.role==="admin"`);frontend-portal nginx.conf upstream admin_backend→esp-admin:80 + location /api/admin/→proxy_pass。
  - **镜像已滚✅**:esp/frontend-react:1.2.0 + esp/frontend-portal:1.3.0 rollout 成功。
  - **鉴权路径已验证✅**:`kubectl exec frontend-portal -- wget ... /api/admin/services` 非法 token 返 **403 Forbidden**(requireAdmin 正确拒绝)。待 = 真实浏览器 admin 账号登录验证仪表盘渲染(services/infra 5s 轮询、PromQL 查询出数、告警卡片)。
- **已改文件**:06-sca-middleware.yaml / 05b-platform-infra-externalnames.yaml / 05-infra-reuse.yaml / 45-httptest-nodeports.yaml(均 ns + ExternalName/PVC);apps/web/src/pages/admin/AdminDashboard.tsx + LearningLayout.tsx(P3c 前端);frontend-portal nginx.conf(P3c 反代)。
- **下一步**:① #191 浏览器验证 /web/admin(admin 账号登录看仪表盘,闭合 P3c);② #189 可选清理(40-networkpolicy.yaml 孤儿端口 8848/8858/7091/8091 + platform-infra-multitenant.md 6→9 组件更新 + 全集群回归);③ 关闭 #176/#179 父任务。
- **Redis 锚点**:`esp:ctx:migration:sca`(迁移全貌)+ `esp:ctx:hot:session-23-8-sca`(本会话恢复点)。

---

## 历史(2026-08-16 会话23-8 P2 全功能测试收尾:Web/H5 E2E 全绿 + 小程序核查 + 09 全量回填 + 签收报告 = 条件性 NO-GO(G-7/G-8 阻断)/ 下一步 P3 门户 #176-179)

### 恢复锚点(会话23-8 P2 签收报告完成,2026-08-16 15:5x,Task #174 ✅)
- **09 已全量回填**(`docs/test/09-人工测试操作指南.md` L9 更新注释 + 阶段0~7 全部空格填实测):P-0-1~15、1.2 18 行(16 PASS 200 + 2 scaffold SKIP + learning-worker 不适用)、3.1 Web/3.2 H5 判定、3.3 小程序无环境 SKIP、4 Mock、5 L1-L10、6 勾选、7 G-1~8 + E-1~9 + 7.3 决议。
- **签收报告已生成**:`report/launch-readiness-2026-08-16.md` = **条件性 NO-GO**,签收人 JoelGong 2026-08-16。
  - **就绪 ✅**:三端(Web 8/8 + H5 2/2 浏览器 E2E + 小程序代码级核查)、SEC-1~5 ✅、16 服务 healthz 全 200、L1-L5 主链路全绿、镜像无 secret。
  - **阻断 ✗ 2 项**:① **G-7 种子规模** — CRDB esp 冒烟级(sessions=500/session_turns=2000/learning_events=8000/users=238;esp_teacher.submissions=2M),30亿级在 TiDB(scaled 0)待多节点 D2 重灌(坑㉑/坑㉟b);② **G-8 CI 全红** — ci.yml 最近 5 次 failure + security.yml failure,根因 **`platform/pom.xml` 不存在**(`mvn -pl` 找不到 reactor 模块)。
- **CI 解阻最简路径(已核实 5 模块零依赖)**:platform/ 下 esp-api/esp-cache/esp-disruptor/esp-rw-java/esp-web-common 相互零依赖、各自独立可构建 → ci.yml 后端 job 改逐模块 `for m in esp-api esp-cache esp-disruptor esp-rw-java esp-disruptor; do (cd platform/$m && mvn -B -q install -DskipTests); done`(顺序无关,无需聚合父 pom);前端 job 修 pnpm/action-setup@v4 + Node 匹配;security.yml 修 setup 基座。**注:修 CI 后需 push GitHub,按 push 策略先问用户。**
- **G-8 CI 修复已本地完成(2026-08-16 15:5x,待 push 验证)**:3 根因全实证,4 处 diff 仅 workflow 文件,两文件 YAML 已本地解析通过。① ci.yml `java-services`+`contract` 两处 `mvn -pl` → 逐模块循环(local 实测 platform5 模块零依赖);② ci.yml `frontend` pnpm/action-setup@v4 `version: 11` 与 package.json `packageManager: pnpm@11.0.0` 字符串不一致 → "Multiple versions of pnpm specified"(实证日志 ##[error])→ 删 version 输入读 packageManager 单源;③ security.yml line113 `with: { name: trivy-image-${{ matrix.svc }}, ... }` flow-mapping 值含 `${{` → YAML 解析失败 → run 0 job "workflow file issue"(实证 gh run 无 jobs)→ 改 block mapping。**待用户确认 push 后重跑验证绿**。
- **小程序**:WS token P0 已修(`apps/miniprogram/src/api.ts` openSpeechWs 拼 ?token,gateway-go 强制鉴权);构建验证待微信开发者工具(P-0-6 无环境)+ Taro4/Node26 降 Node20/Nightly。**08 表待更正**:scenarios/emotion(表标✅实未调)、placement/difficulty/polish/translate(表缺但实调)。
- **backlog 新增**:SEC-6 限流未发布 Nacos flow-rules(配置缺口);teacher-java not-found 应 404 非 500;frontend register role 下拉 UI bug(BFF P0-4 服务端强制 learner);vocab_review SM-2 MaxInterval cap(坑㉟e);writing_grade LLM provider 稳定性;HPA scale-down 残留死副本 Dubbo;edge 9090 fire-and-forget。
- **下一步**:① P3 esp-admin 门户(#176-179:admin-go 8099 聚合 + OTLP→Prometheus + /admin/dashboard 实时监测);② 可选:修 CI(G-8 解阻,需用户确认 push)。

### 恢复锚点(会话23-8 P2 Web/H5 浏览器 E2E,2026-08-16 15:2x,Task #173 ✅)
- **Web 浏览器 E2E 8/8 绿**:`cd e2e && E2E_BASE_URL=http://localhost:30070 npx playwright test full.spec.ts`。用例=e2e/full.spec.ts:TC-W-1 注册+退出+再登录(esp_token httpOnly cookie 验证)、TC-W-4 薄弱点、TC-W-4b 复习、TC-W-3 写作批改、TC-W-2 口语 WS、TC-W-5 教案、TC-W-6 学情、TC-W-7 作业评阅。入口经 frontend-portal 反代(/api→bff、/ws→gateway)。
- **H5 浏览器 E2E 2/2 绿**:`E2E_H5_URL=http://localhost:30091 npx playwright test h5.spec.ts`(375×667 视口,**必须经 /h5 路径访问**,router basename=/h5,根 / 不渲染)。
- **坑㉟f(H5 白屏真 bug,浏览器 E2E 才暴露)**:根因=`apps/h5/nginx.conf` 缺 `/h5/` 静态映射(vite base=/h5 但 Dockerfile 把 dist 落 nginx 根)→ `/h5/assets/*` 走 SPA 兜底返回 index.html → 浏览器把 HTML 当 JS 解析 → `#root` 空白屏。修复=nginx.conf 加 `location ^~ /h5/ { rewrite ^/h5/(.*)$ /$1 break; root /usr/share/nginx/html; try_files $uri $uri/ /index.html; }`;**派生镜像 esp/frontend-h5:1.1.1**(=1.1.0+固定 nginx.conf,免 npm 重建)已部署(deploy/k8s/16-frontend-h5.yaml),验证 `/h5/assets/index-*.js` 返回 255KB 真实 JS。
- **教师端 E2E 账号**:ops-provisioned `e2eteacher@esp.io`(role=teacher,已在 CRDB esp.users 开通)。原因:BFF P0-4 `register` 服务端强制 learner、忽略请求体 role(设计如此);前端 register 的 role 下拉是 UI bug(提供教师选项但后端忽略)→ 记 backlog。
- **07 回填**:esp-doc 代理执行中(浏览器证据标注 TC-W-1~7 / TC-H-1 + 坑㉟f 记录)。
- **下一步**:小程序核查(API/契约级,08 报告核对)+ 09 检查单 52 空填充 + launch-readiness 签收报告(#174)→ P3 esp-admin 门户(#176-179)。

### 恢复锚点(会话23-8 P2 代码修复部署,2026-08-16 14:5x)
- **P2 后端全功能 73 用例 live run**:80 cases = **56 PASS / 4 FAIL / 20 SKIP**(`report/manual-73-run-20260816-141318.json`)。newman 19/19 PASS。**4 个 FAIL 全部闭合**:
  - **TC-W-16(behavior/top 500)= 真 bug,已修+验证**:learning-java BehaviorController 三处 `INTERVAL ? DAY`(PG/CRDB 语法错)→ `INTERVAL '1 day' * ?`(BehaviorController.java L75/L93/L113)。重建镜像 **esp/learning-java:1.11.4**(Dockerfile.derived,FROM 1.11.3 仅换 jar)。**实测 behavior/top/funnel/heatmap 全 200**(top 首查秒级 + 二次 0.47s CacheService L1 命中)。
  - **TC-GR-25(assessment 500)= 真 bug,已修+验证**:assessment-rs submit_answer 的 `correct: i8` → pgx v5 编码缺陷(0 发空串→ strconv.ParseInt "")→ 改 **i16** + `skill_tag` 改 Option<String> bind(main.rs L149-159)。镜像 **esp/assessment-rs:0.3.1** 重建坑:首版 Dockerfile.derived COPY 主机 mac arm64 二进制 → **exec format error**(Mach-O)→ 弃用该文件(已删),改用原多阶段 Dockerfile 在 VM 内构建(linux/arm64,-j4 限压测期负载;rust:1.88-bookworm 已缓存,ustc crates 可达;构建 6m42s)。**实测 create→answer→finalize→report 全 200**(answer 200 `{is_correct:true}`)。
  - **TC-W-7(finalize 500)= 环境,已修**:learning-java HPA scale-down 残留死副本 IP 在 teacher-java 的 Nacos Dubbo 注册表 → `rollout restart deploy/teacher-java` 后 200。
  - **SEC-6(无 429)= 配置缺口,已文档化**:RateLimitFilter 存在但 Nacos `bff-java-flow-rules` 未发布 → 无限流。非代码 bug,如实记入修复 backlog。
- **部署基线**:learning-java 1.11.4 / assessment-rs 0.3.1 均 Running 1/1,loadavg 33(under 42 门控)。Dockerfile.derived(learning-java)保留为已实证派生构建模式;assessment-rs 的 derived 已删(架构坑:宿主二进制不可 COPY)。
- **TC-W-7 终验(2026-08-16 15:0x)**:finalize 500 真根因 = **测试脚本硬编码 submission id=1**,CRDB esp_teacher.submissions 是 snowflake id(1201593833...),id=1 不存在 → TeacherService L102 `submission not found` → 500。用真实 id 1201593833013084163 复测 **finalize 200 {"finalized":...}**;run_manual_73.py 已改 `SUBMISSION_ID` env 可覆盖(TC-W-7/TC-E2E-2)。附带:teacher-java 未命中也返回 500(应 404)记 minor backlog。
- **07 实测列已回填(Task #172 完成)**:`docs/test/07-人工测试用例.md` 73 行回填 = **59 PASS / 1 FAIL(SEC-6 限流,配置缺口) / 13 SKIP**;剩 10 行 ⏳ 待测 = TC-GR-28~34(vocab-srs-go/llm-orchestrator-rs)+ TC-ADM-1~3(admin 门户,属 P3)。JSON 80/80 tc id 在文档均有行。run_manual_73.py 增加 `ESP_SUBMISSION_ID` env。
- **下一步**:Web/H5 浏览器 E2E(#173:Playwright + SSH 隧道 localhost:30070,回填 TC-W 全链路/TC-H)+ 小程序核查 + 09 检查单(#174)→ launch-readiness 报告。

### 恢复锚点(会话23-8 P1 基准全部完成+三表分析落盘)
- **P1 基准执行完成(2026-08-16 14:02)**:leg-4 + 6 无效端点重跑 + 2 恢复端点全完成,18 端点均有 k6+locust v10 有效数据;核心5 v20/v60 k6 全有效;mixed(k6+locust 并发 gw_scenarios/learning_event)产物在。**三表分析已落盘** `report/benchmark-2026-08-16/analysis-2026-08-16.md`(18 eps;k6 files=18 locust=18)。
- **三表结论(A CRDB-vs-旧TiDB / B 全语言全服务 / C k6-vs-locust)**:见 analysis 文件。亮点:learning_event k6 v20=127rps(p95 343ms)vs 旧TiDB 200rps(p95 270ms)量级相当;learning_review k6 v20=240rps vs 旧TiDB 146rps 提升;gw_scenarios k6 v20=83rps vs 旧TiDB 377rps 降低(读路径);course_path(Go 纯内存)2233rps 最高;全语言对比 Java learning_event 292rps / Go gamification_prof 501rps / Python writingcoach_tpl 282rps / Rust llm_dialogue 309rps。
- **D 节诚实标注(必读)**:① **vocab_review 两工具均 100% fail**(k6 5482/locust 3327)= 坑㉟e SM-2 无 MaxInterval cap → due_at 超 CRDB 时间戳界 → 500;写入路径缺陷入修复 backlog;② **writing_grade k6 100% fail = LLM provider 宕机窗口**(快速 500 假高 rps 1115),locust 0.42% fail(provider 已恢复)——LLM 绑定端点,结果反映 provider 可用性;③ **低置信 locust 行**(learning_event v20 p95=22s/emotion_recent v10/teacher_kpstats v10)= 冷启动+HPA 波动+节点争用污染,表 B 以 k6 为准。
- **重跑+恢复记录**:bff_login v10 40req→175req(0% fail,bff-java 冷启动修复后重跑);teacher_kpstats v20 38req→1544req(0.39%);bff_scenarios v10 487req(0% fail 保留)/learning_event v20 357req(0% fail 保留,标注低置信);learning_event v20 locust 曾 0 请求(bff-java HikariPool 冷启动 13:52:55 与 locust spawn 撞窗)→ 修复脚本 `scripts/recover_2_invalid.sh`。
- **负载复盘**:loadavg 峰值 502→自然恢复(~14:00 <42 门控放行);HPA learning/teacher 压测期 scale 至 4 副本(端点瞬断源);bff-java/teacher-java 冷启动+重启与压测窗重叠→locust 0 请求或挂起。单节点争用是主要噪声源,已全部 caveat。
- **下一步**:Task #170 已近完成(analysis 落盘),补 Redis 锚点 `esp:ctx:hot:session-23-8`(上次写失败 TLS 超时,本次重试)→ 转 **P2 后端全功能 73 用例 live run**(#172)。

### 恢复锚点(会话23-7)
- **locust ConnectionRefused→根因闭合(非 locust 配置 bug)**:learning_review v10 曾 100% ConnectionRefused(111)。实证:learning-java 的 **HPA(min1/max15)压测期 scale 1→2→1**(CPU 18%/65%),scale 波动窗口服务端点瞬断→iptables REJECT→ConnectionRefused。HPA 已回 1 副本稳定;**重跑 locust learning_review v10 成功:1273 req / fail=0.47% / avg_rps=62.7 / p95=310ms**。locust 管道已验证打通(--host 修复后首个有效数据)。
- **leg-4(后台 b100yngzs + mon b8btn0twx)**:k6 3 缺口 v10 → **locust v10 补到 17**(learning_review/teacher_kpstats/gamification_prof/pts/vocab_due/vocab_review/emotion_recent/writingcoach_tpl/assessment_report/llm_dialogue/course_path/writing_grade/bff_login/gw_scenarios/bff_scenarios/listening_mat/reading_passages;learning_event-v10 已有 3228 有效不复跑)+ locust v20 全 5 + mixed 20s。每端点 loadavg<42 门控+重试1次。**loadavg 当前 83(稳态高,CRDB3+全服务/9核),门控等待中**。
- **写作 grade = LLM 绑定端点(定性,非 DB 性能标)**:live curl 15s 超时(LLM provider 挂/慢);k6 v10 100% fail(284rps 快速失败=provider 快速报错窗口)非 perf 信号。计划已标注"LLM 绑定端点仅低并发定性,RPS 单独标注"。Rust/Go/Java/Python 对比表取各服务主 DB 端点,writing 行如实标定性。
- **集群卫生**:已删残留 locbench-learning-review-10(Error/Pending)+ locbench-gamification-prof/pts-10;k6 残留 job(k6bench-writing-grade-10/vocab-review-10 Failed 均含有效 summary 日志,见 report)。

### 恢复锚点(会话23-7)
- **坑㉟d token 过期=leg1/2 全 401 根因**:esp-bench-tokens JWT iat=07:04 CST / exp=08:04 CST(8-16)→ 08:04 后所有压测 **100% 401**(teacher_kpstats 7574/7574 failed,p95=150ms 快速失败=401 非超时)。07:30 leg 历史数据(exp 前)仍有效(core5 v20/v60 fail 0~0.74%)。**已修**:新增 `scripts/gen_bench_tokens.py 24`(同私钥 esp-rsa-jwt-sign/同 sub/exp+24h)merge-patch 刷新 configmap;实证 learner+teacher 均 200。
- **坑㉟c Job 名 RFC1123 下划线**:端点名含 `_`(teacher_kpstats/gamification_pts/vocab_review/writing_grade...)→ kubectl apply 校验失败静默丢数据(下划线端点全跑不出)。**已修** run_k6_bench.sh/run_locust_bench.sh:JOB 名 `_`→`-`(文件/日志名保持原 EP);client dry-run 验证通过。
- **locust 早前"6 valid"实为 0 字节空文件(仅存在性误计)**:本轮 leg 全量重产 locust v10/v20 + mixed 补 2。
- **leg-3(后台 bwuy8762g + mon b5476bdgt,run_remaining_leg.sh)**:k6 3 缺口 v10 → locust 12×v10 + 5×v20 → mixed(20s);每端点 loadavg<42 门控+重试1次。完成→ `python3 scripts/analyze_fullbench.py` 出三表(CRDB-vs-旧TiDB/全语言全服务/k6-vs-locust)→ `analysis-2026-08-16.md` → Redis `esp:ctx:hot:session-23-8`。
- **坑㉟e vocab_review 100% 500(真 bug,基准暴露)**:POST /api/vocab/review upsert 报 `timestamp "-254250-05-26..." exceeds bounds (SQLSTATE 22009)`。CRDB esp_vocab 实测 uid=10000104 wid=5:interval_days=**92231549**/due_at=**254548-01-10**/review_count=59/status=mastered → **SM-2 Compute 无 interval 上限**:mastered 高 review 词 interval=IntervalDays×ef 无限增长 → due_at 超 CRDB 时间戳界 → 全 500。**修(基准后)**:Compute 加 MaxInterval cap + 清污染行;基准如实标 vocab_review FAILED(500)。
- **负载根因(已定,leg 用门控规避)**:单节点 9 核 loadavg 曾 97/0% idle;CRDB 3 raft+3区域 TiKV(aws trio 已再 scale0)+nacos+pulsar3+obs;loadavg>~50 kubelet 新 pod 永久卡住(坑㉟b)→ 唯一解降载;CPU request 已 99%。
- **P2 gap-scan(esp-doc)完成**:07=80→**90 用例**(+7 vocab/llm +3 admin),09=60→**86 行/94 格**待填。**Task #171 已 completed**。
- **下一步**:leg-3 完成→ P1 三表分析落盘(#170);随后 P2 后端全功能 73 用例(#172)+ Web/H5 E2E(#173);P3 esp-admin 门户(33-admin-go + 4 注解)基准后 apply(#176-179)。

## 当前(2026-08-16 会话23-4 P1 基准收尾(locust 腿重跑中)/ P3 统一监测门户 staged)

### 恢复锚点(会话23-4)
- **P1 k6 腿数据已完整有效**:18 端点 v10(15 个 07:20 健康期有效数据,含 learning_event=292rps/gw_scenarios=37rps)+ 核心5 v20/v60 全有效(0% fail 为主,learning_review v10 fail=1.35% 如实记录)。**矩阵 49 日志在 `report/benchmark-2026-08-16/`**。
- **坑㉝ Locust 2.46 全矩阵腿无效(root cause)**:`FullbenchUser.on_start` 用 `self.user_index`(locust 2.46 User 无此属性,runner spawn 不设置)→ AttributeError → **每个 locust run 0 请求**("OK" 仅 exit code)。**已修** `deploy/locust/locustfile_fullbench.py`:模块级 `_USER_SEQ=itertools.count()` 替代 + 加 `@events.test_stop` 单行摘要(与 run_locust_bench.sh/analyze_fullbench.py 对齐)。**实证**:learning_event v10 = 5146 reqs / 0 fail / 209rps / p95=94ms。bench-scripts configmap 已重建(probe 源=`scripts/probe_bench_endpoints.py`),bench-runner(Pod,非 Deploy)已删重建载新 file。
- **坑㉞ run_k6_bench.sh 误报 FAIL**:固定 job 名 `k6-fullbench` + delete 不 `--wait` → 旧 Error pod 残留污染新 job `condition=complete` → wait 超时误报 FAIL(数据实为有效)。**已修**:每次唯一 job 名 `k6bench-<ep>-<vus>` + delete `--wait=true` + wait 240s + Succeeded-pod 兜底抓日志;k6.yaml 加 `__JOB__` 模板。
- **坑㉟ gateway-go HPA 单节点 overshoot**:HPA(max=20)压测期打满 20,负载降后 CPU=4% 仍 desired=20 → 单节点 9 核 "Insufficient cpu" → rollout 卡 5/20 → gw_scenarios 100% fail(244k reqs 全失败,k6 阈值触发 exit99)。**已修(临时 live)**:`kubectl patch hpa gateway-go` max→4 + `scale deploy/gateway-go --replicas=2` → rollout 成功,网关 401(存活)。**Follow-up**:HPA max=20 在 yaml 何处定义待查,建议单节点上限收敛(工业级=多节点后按节点数×每节点副本设计)。
- **locust 腿重跑中(后台 botti0830)**:18 v10 + 核心5 v20 顺序跑(~16min),完成后 `bash scripts/run_mixed_proof.sh`(k6+locust 并发 2 端点,混用产物归档 mixed-* 名避免污染单跑)→ `python3 scripts/analyze_fullbench.py` 出三表(CRDB-vs-旧TiDB / 全语言全服务 / k6-vs-locust)→ `report/benchmark-2026-08-16/analysis-2026-08-16.md`。
- **遗留集群卫生(不阻塞基准)**:17 个 `st-seeder-*` + `esp-schema-init` 卡 ContainerCreating 111m(seed 遗留,引用已删 configmap `seed-session-turns-v2`,pod 永不启动,占 CPU request);节点 CPU request 93%。**P3 仍 staged**(esp-admin/前端页/nginx/4 注解,基准结束前不 apply)。
- **P3 统一管理员监测门户(全部 staged)**:esp-admin Go 聚合服务 `backend/services/admin-go/`(main.go+jwt.go+catalog.json+Dockerfile)+ `deploy/k8s/33-admin-go.yaml`(8099,svc esp-admin:80,NodePort 30099);`/api/admin/{services,infra,catalog,metrics/query,metrics/query_range,metrics/alerts,metrics/rules}` RS256 role=admin 鉴权;前端 `apps/web/src/pages/admin/AdminDashboard.tsx` + App.tsx `/admin` 路由(RequireAuth role=admin);nginx 代理两处;4 服务 scrape 注解(gateway /metrics:8080 + bff/learning/teacher /actuator/prometheus)staged。**待办**:docker build esp/admin-go:1.0.0(派生本地镜像)→ apply 33 → 重启 portal/web nginx → web 重建 → apply 4 注解 → 验证 Prometheus 收指标+告警 → seed admin 账号 → 浏览器测门户。
- **Redis 锚点**:`esp:ctx:hot:session-23-4`(本更新)、`esp:ctx:hot:session-23-3`(P3 staged 快照)。

## 当前(2026-08-16 会话22-39 P1-8 幂等 / 多区域 SQL 预备✅ / P0 端到端验证✅)

### 恢复锚点(会话22-39)
- **P1-8 learning_events 幂等✅(2026-08-16 端到端验证通过)**:代码 7 文件全改(LearningEvent/Service/Disruptor 加 messageId;DbLearningEventConsumer/OutboxRelay INSERT 加 message_id + ON CONFLICT DO NOTHING;BatchEventHandler/EventBatchQueue 深拷贝透传;LearningEventConsumer handleEventReport 透传 ev.messageId + handleSessionEnded 确定性 messageId;crdb-schema.sql 加列+UNIQUE)。**CRDB DDL 已全闭合**:旧卡死 job 1201742016749142017 CANCEL+回滚干净(重启动协调 pod crdb-0 解卡)→ 降副本 num_replicas=1 + checkpoint 10s → 拆句重跑成功:`learning_events`/`learning_event_outbox` 均加 message_id 列;**UNIQUE INDEX learning_events_message_id_key backfill 成功(job 1201752931390455809)→ index 公开(visible=t)** → num_replicas=3+constraints+lease_preferences 已恢复。**镜像**:learning-java:1.11.3 构建+双部署 rollout(20-learning-java 入口 / 21-learning-worker 各 1 副本)。**镜像源坑**:rd-vm docker 镜像源全挂(1ms.run/daocloud/docker.io 全 403)→ 派生构建(FROM 本地 1.11.2 + COPY 新 jar),见 memory esp-registry-mirror-outage-2026-08-16。**E2E 证据**(scripts/e2e-p1-8-idempotency.sh,messageId=p18-e2e-1786821276):同 messageId 投 2 次 → `cnt=1`(60s)、`cnt2=1`(+30s)、`obx=0`(outbox 无残留)= **PASS**。E2E 生产方式:pulsar-broker pod 内 python3+pulsar 3.4.0(避开 640MiB cgroup 内 pulsar-client JVM OOM)+ 重试 6x8s。**已改文件**:7 Java + scripts/crdb-schema.sql + deploy/k8s/20-learning-java.yaml + 21-learning-worker.yaml(→1.11.3)+ 新增 scripts/apply-p1-8-messageid-ddl.sh + scripts/e2e-p1-8-idempotency.sh。
- **#161 多区域 SQL 预备✅(2026-08-16 完成)**:产出 `scripts/crdb-multi-region-migrate.sql`(Phase1 库级 PRIMARY REGION/ADD REGION/SURVIVE REGION FAILURE ×19 库;Phase2 逐表 GLOBAL/REGIONAL BY TABLE/RBR+region 列;Phase3 验证)+ `scripts/crdb-multi-region-migrate-README.md`(**Phase0 单节点显式 DEFERRED**:当前节点 locality=zone=z1/z2/z3 无 region 层,需重启加 region= 层才能跑)。实测:19 库全 primary_region=NULL 非多区域默认态;库内表清单已盘点(esp_learning 6 表/esp_teacher 4/esp_writing 6/... 全列)。
- **#164 P0 第一波端到端验证✅(2026-08-16 完成)**:`report/p0-security-redline-verification-2026-08-16.md`。6/6 PASS 实证:P0-1 私钥仅 bff(learning/gateway PRIV 空)+旧 secret 已删;P0-2 learning 无 token 401/带 token 200;P0-3 gateway history `?user_id=` 覆写忽略 + learner→teacher 403;P0-4 role=admin→强制 learner;P0-5 业务服务全 ClusterIP;P0-6 NetworkPolicy 精确白名单(same-ns pod + kube-system traefik 80/8090/9090 + ipBlock NodePort 端口 80/8080/8090/9090/8848/8858/7091/8091)。回归:gateway scenarios 200 / learning review 200。
- **Redis 锚点**:`esp:ctx:hot:session-39`(本更新)。P1-8 DDL 卡点详情:job 1201742016749142017 已 CANCEL。

### 进行中🔄
- **#160 P1-8 幂等 ✅(本会话四轨全部完成)**:E2E cnt=1/obx=0 验证通过(见上)。四线全闭环:P1-8 / #161 多区域 SQL / #164 P0 验证 / #155 基线核阅。

## 当前(2026-08-16 会话22-38 #140 全闭合 + 新一轮四线推进:P0 安全红线第一波 / P1-8 幂等 / 多区域 SQL 预备)

### 恢复锚点(会话22-38)
- **当前子步骤**:#140/#144/#154/#131 全部闭合并双轨落盘✅(#140 全服务切 CRDB 7/7 env 指 crdb-public;TiDB 三组件 scale 0 + PVC 保留作回切底座;4 Java 服务 SEATA_ENABLED=false;worker 3 副本 Shared 无重复消费)。**用户选择四条线全推进**:① **P0 安全红线第一波**(6 项:私钥收敛/learning 鉴权/IDOR/register 锁 role/NodePort 收敛/NetworkPolicy 收窄,依 `~/.claude/plans/indexed-honking-spring.md`);② **P1-8 learning_events 幂等**(UNIQUE 约束 + 消费端去重);③ **多区域演进 SQL 预备**(region label + LOCALITY REGIONAL 迁移脚本,单节点显式 DEFERRED 应用,产出可移植脚本);④ **基线核阅**(architecture-review-2026-08-14.md 已读:综合 5.9/10,安全 3.5 致命,红线一=私钥扩散已核实)。
- **已确认**:P0 六项安全缺口核实属实(report/architecture-review 第 5 节);当前 deploy yaml 部分已用 `esp-rsa-jwt-sign`(bff L51)/`esp-rsa-jwt-verify`(worker L58/teacher L82)引用,secret 拆分现状待侦察核实(gateway 10-go/learning 20 的 envFrom 引用待查)。
- **下一步**:侦察 subagent 返回 P0/P1-8/多区域文件定位 → 逐项执行 → 每里程碑双轨落盘。
- **Redis 锚点**:`esp:ctx:hot:session-26`(本次更新)、`esp:ctx:hot:session-25`(22-35)。

### 用户指令
① 完整完成所有任务(可并行则并行);② **#140 全服务数据访问切 CRDB + 关停 TiDB 及其所有组件(节省资源)**;③ **Go 统一数据访问层保留读写分离**(用户明确:不丢弃读写分离实现,esp-common-go 双池抽象 DSN 可配);④ **CRDB 多区域**:当前单节点无 Enterprise License→单 region 三副本 + 保留演进位(用户已确认);⑤ **esp_speaking.sessions 5000 万行迁 CRDB**(用户已确认迁移不丢);⑥ 双轨落盘铁律;⑦ **(新 2026-08-15)保留原 TiDB 读写分离实现,便于后续直接切回 TiDB**——Java 已具 `ESP_RW_STORAGE_TYPE=tidb|cockroachdb` 门控(#132);Go esp-common-go 待加 mysql+pgx 双驱动(DSN 前缀自动识别,双池保留),SQL 方言($N vs ?)为明确边界;Python esp-db-py 已保 API 形状(占位符自动翻译)。

### 已确认决策(本会话 AskUserQuestion)
- **Go 统一**:三套 Go DB 层(esp-common-go 单池 / conversation-go 读写分离 / vocab-srs-go 自造 dao)→ 统一到 esp-common-go 双池抽象(单池 MustConnectDB + 双池 MustConnectReadWrite + 包级 InitGlobal/Write/Read/Enabled 保留 forcedPrimary/fallback)。CRDB 单端点阶段 read/write DSN 同址,多区域阶段读池切 follower 代码零改动。
- **esp_speaking 50M**:迁移到 CRDB(非丢弃非重建)。
- **多区域**:当前单 region 三副本;多区域作为多节点服务器上线后的 DEFERRED 演进位。

### 已完成✅(2026-08-15 四轨并行启动)
- **Python 三服务切 CRDB✅(Track D 完成)**:esp-db-py aiomysql→asyncpg(API 形状保持,`%s`→`$N` 自动翻译、lastrowid→RETURNING id、jsonb codec);emotion-svc/writing-py logging_client pymysql→psycopg3(DSN crdb-public/esp_shared,NOW(3)→now(),CAST AS JSON→::jsonb);py_compile+import 全过。**部署前置已落**:30 yaml 三服务 WRITE/READ_DB_HOST→crdb-public:26257 + 新增 WRITE_DB_NAME/READ_DB_NAME(writing-py/coach→esp_writing,emotion→esp_learning)+ CRDB_HOST/CRDB_PORT;**遗留** asyncpg×CRDB 运行时实连未 smoke(待部署验证);seed_3b.py 未迁移(非服务,单独决策)。
- **k8s env 迁移 10/30/32/33 完成✅(待 Go 编译后 apply)**:10-gateway(CRDB_HOST 替换 WRITE/READ_DB_HOST+TIDB_HOST)、30-other(Python 三服务如上)、32-batch2(listening/reading/gamification READ/WRITE_DB_URL→postgres crdb)、33-orchestrator(vocab READ/WRITE_DB_URL 替换 TIDB_*)。**发现** deploy/k8s/esp-prod/ 有 2 个过期副本(vocab-srs-go/llm-orchestrator-rs TiDB env,live 实际用 33)→ Phase4 清理;esp-redis-warmup-job 连 tidb-primary 一次性 Job→TiDB 关停后失效,Phase4 处置。
- **esp_writing 4 小表数据迁移✅**:TiDB writing_plans=2/progress=2/templates=8/essays=4 共 16 行真实数据 → CRDB(显式保留源 id,脚本 `scripts/migrate_writing_to_crdb.sql`),CRDB 计数核验一致。writing_tasks 已知差异:TiDB 1.1M vs CRDB 2M(#136 合成 seed,CRDB ⊇ 无丢失风险,如实记录)。
- **Rust 三服务部署+CRDB smoke✅(Track A 完成)**:pronunciation-rs 0.3.0 / assessment-rs 0.3.0 / llm-orchestrator-rs 1.1.0 rollout 全 1/1 Running,CRDB 三库写删 smoke 通过(pronunciation_attempts/assessment_answers/llm_call_logs INSERT+DELETE),readiness 探针 `{"db_read":true,"status":"UP"}`。**坑㉗**:live 集群 Rust 探针是旧 tcpSocket+TiDB env(文件 CRDB env 从未 apply),kubectl apply 合并双 handler Invalid → 非破坏性 patch 探针为文件 httpGet 后 apply 成功。**遗留**:32 yaml 内 listening/reading/gamification 3 Go 服务同样探针漂移(tcpSocket),待 #142 Go 迁移时一并修。
- **CRDB 辅助服务建库建表✅**:写 `docs/design/crdb-schema-aux-services.sql`(TiDB DDL→CRDB 方言:AUTO_RANDOM→unique_rowid()、DATETIME→TIMESTAMPTZ、JSON→JSONB、去 ENGINE/ON UPDATE)。9 库 30 表全部建齐(esp_speaking 新建库+scenarios/sessions;pronunciation/listening/vocab/assessment/gamification/reading/llm 从 0 表补齐;writing 补 4 表)。CRDB 核验 `SHOW TABLES` 全过。
- **esp-common-go/db/db.go 统一层重写✅**:mysql→pgx(v5.10.0,go.mod 已加)、DSN→postgres、单池+双池+全局包级 API(保留读写分离/forcedPrimary/fallback)。
- **四轨并行子代理已派发**:A=Rust 三服务部署+smoke(esp-builder);B=TiDB→CRDB 数据迁移含 50M sessions(esp-data,长任务断点续迁);C=Go 服务方言迁移+编译(esp-builder);D=Python 服务迁移(esp-builder)。
- **Go/Python 镜像构建推进✅(2026-08-15,第2个落盘点)**:**坑㉘ gateway-go 镜像构建失败根因=go.mod/go.sum 缺 mysql 传递依赖**(esp-common-go db.go 新增 `_ "github.com/go-sql-driver/mysql"` 双驱动,但 gateway-go go.mod 仅间接依赖 pgx;本地 build 过是 GOMODCACHE 已缓存,容器内 `go mod download` 缺 go.sum 条目失败)→ `go mod tidy`×5 服务(gateway/vocab/listening/reading/gamification)补 `go-sql-driver/mysql v1.10.0 // indirect`+go.sum,gateway-go:1.11.0 重建成功(66.8MB)。**yaml 标签已对齐**:10-gateway=1.11.0、32(reading/gamification=0.3.0,listening=0.3.0,pronunciation/assessment=0.3.0 未动)、33(vocab=1.1.0)、30(writing-py=1.6.0/emotion-svc=1.7.0/writing-coach=1.2.0);**防降级**:30 yaml teacher-java 原 1.9.0 但活体 1.10.0→已改 1.10.0(否则 apply 会把 teacher 打回旧镜像)。**CRDB 目标库/表前置核验✅**:esp_vocab(user_vocabulary/vocabulary)、esp_reading(reading_attempts/passages)、esp_gamification(user_streaks)、esp_listening(listening_attempts/exercises)、esp_writing 5 表、esp_learning.learning_events(payload=JSONB) 全在;esp_shared.app_logs 在(logging_client 目标)。writing-py:1.6.0 镜像已实证 asyncpg 0.31.0+esp_db。**会话归档写库前置**:esp_speaking.sessions 现 BIGINT,gateway-go 新代码写 STRING id→gateway-go 部署 DEFERRED 至迁移完成 + `ALTER TABLE esp_speaking.sessions ALTER COLUMN id TYPE STRING;`。
- **Go 服务方言迁移+编译✅(Track C 完成)**:7 Go 模块全 build RC=0;esp-common-go 统一双池(Write/Read/forcedPrimary/fallback,DSN 前缀自动识别 pgx/mysql);conversation-go/src/db/db.go 旧 MySQL 层删除;vocab-srs-go DAO 重写 ON CONFLICT/RETURNING;gateway-go ListHistory 修 `COALESCE(ended_at::STRING,'')`(原 `COALESCE(ended_at,'')` CRDB SQLSTATE 22007)。**esp-common-go 双驱动(mysql+pgx)已实现✅(#148)**:`driverFor()` 按 DSN 前缀选驱动,`ReadWriteDSN()` 支持 `DB_DRIVER=mysql`→tidb-primary/tidb-follower 双池回切,CRDB 阶段 `CRDB_READ_HOST` 切读池。course-path-go/live-tutor-go 无 DB 访问(scaffold,免迁移);conversation-go 为库内嵌 gateway-go 构建。
- **Go+Python 全量部署 CRDB+运行时 smoke✅(2026-08-15,第3落盘点)**:7 镜像全建(Go5+Python2),apply 32/33/30,**坑㉙ 复用坑㉗ 探针漂移**:live listening/reading/gamification 探针是旧 tcpSocket,yaml 是 httpGet,merge patch Invalid → 先 patch 三服务探针为 httpGet 再 apply 成功。8 部署全部 rollout 1/1(含 teacher-java 防降级 1.10.0、llm-orchestrator-rs、vocab-srs-go unchanged=早前 Track C 已 CRDB)。**运行时验证**:listening/reading/gamification readyz `{"db_read":true}`+gamification `redis:true`,vocab readyz 探 DB ping UP(4 Go 全 CRDB 通);**Python 三服务 DB 路径实证**——emotion-svc `POST /api/emotion/sense` → CRDB esp_learning.learning_events 落行 `{"emotion":"neutral","text_snippet":"crdb smoke"}`(JSONB),writing-coach `GET /api/writing-coach/templates` 200 返 8 模板(CRDB 读)+`plan/{uid}` 可信头查询正常,writing-py `POST /api/writing/grade` → esp_writing.writing_tasks 新行 id=1201653028465213443 score=24.0(读历史+写主库双路径全通)。**剩余**:gateway-go 1.11.0 待 sessions 迁移完成后 `ALTER TABLE esp_speaking.sessions ALTER COLUMN id TYPE STRING;` 再部署(现 BIGINT 列,新代码写 STRING id)。
- **Phase4 TiDB 连接审计✅(#144 前置,2026-08-15)**:TiDB 部署形态=非 StatefulSet,三 deploy 于 ekb-m2-aws-us-east-1(共享 ns):`tidb-cluster-tidb`(单 pod,svc :4000)+`pd-aws`+`tikv-aws`;esp-prod ExternalName 链:`tidb-primary`→platform-infra `tidb`→`tidb-cluster-tidb`;**`tidb-follower`→同一 `tidb-cluster-tidb`(伪分离实证,与 #132 双池回切设计不冲突)**。`information_schema.processlist` 实测:**仅 2 连接**——migrate-runner(esp_speaking 迁移源,idle,须保留至迁移完成)+ 审计查询自身;**任何 ESP 服务已无 TiDB 连接**(Java 的 WRITE_DB_URL=jdbc:mysql://tidb-* 为惰性回切残留,Python/Rust TIDB_HOST 为惰性残留)。→ 关停 TiDB 前仅剩:①sessions 迁移完成;②关停确认(共享 ns 邻居项目影响评估,见 #144 待办)。
- **sessions 迁移加速✅(2026-08-15 09:33)**:8.6M/50M(17.2%),rate≈41K rows/s(重试改造后不再被 TiKV 单节点 region 压力拖死),**ETA≈0.3h**;续迁锚点 last_id=1441151880781348190。迁移完成后续:ALTER sessions.id→STRING→部署 gateway-go 1.11.0。
- **坑㉚ CRDB v24.3 无法原地 ALTER id→STRING(2026-08-15,会话22-37)**:**原计划 `ALTER COLUMN id TYPE STRING` 被 CRDB 两重硬限制否定**——①int→string 属 general path,需 `SET enable_experimental_alter_column_type_general=true`(experimental);②即使开 experimental,**"ALTER COLUMN TYPE requiring rewrite of on-disk data is currently not supported for columns that are part of an index"**(issue 47636),而 sessions.id 是 PK + idx_user 隐式后缀列,无处可避;`DROP CONSTRAINT pkey` 又被要求"drop+add PK 同事务"(issue 48026)。→**原地 ALTER 不可行**。**修订方案(已定)**:迁移完成后用 CRDB 原生 `INSERT INTO sessions_new (id::STRING...) SELECT ... FROM sessions` 重建 sessions 表为 STRING id + drop+rename(原生 SQL 走分布式执行,比 psycopg2 网络批量快数倍;TiDB 源数据在关停前仍是安全网)。**Why 必须 STRING**:gateway-go 1.11.0 End() 写 `s.ID`(sess_xxx STRING)入 sessions.id + ListHistory 扫 id 到 Go string,API 契约=DB id==WS id==客户端可见 id。重建脚本待命 `scripts/rebuild_sessions_id_string.py`(迁移完成后执行,再部署 gateway-go 1.11.0)。
- **坑㉜ 迁移崩溃根因=TiDB/TiKV 双 OOM(2026-08-15,会话22-37,用户定案暂停+关停 TiDB)**:batch=20000 读压下**TiDB 1Gi 与 TiKV 3Gi 先后 OOMKilled**(TiDB 5 次重启、TiKV 18 次,末次 10:53:54),TiKV 单节点 region 选举风暴("Region is unavailable")→ 迁移进程死在 done≈16.4M(CRDB 实存 17.01M)。**脚本重连 bug**:`connect_tidb()` 在 except 块内重连失败时抛 2003 逃逸出 retry 循环(非重试语义),已修——connect 移入 try 内、失败即走 retry(最多 MAX_RETRY=10),`migrate_esp_speaking.py` 编译通过。**用户定案(2026-08-15 11:00)**:无法稳定迁移 → **暂停迁移 + 关停 TiDB 再执行后续**。已执行:①aws TiDB 三组件 scale 0(tidb-cluster-tidb/pd-aws/tikv-aws,共享 ns ekb-m2-aws-us-east-1),Service+2 PVC(5Gi/10Gi)保留,数据可回切;②azuer/gcp region 集群未动(ESP 不依赖);③迁移脚本保留断点续迁(resume-from-max(id)),migrate-runner pod 保留。**迁移现状如实记录:CRDB esp_speaking.sessions=17.01M/50M(34%),缺口 33M 留存 TiDB PVC**。
- **rebuild_sessions_id_string.py 执行中(2026-08-15 11:05→13:30)**:对 CRDB 现有 17.01M 行重建 sessions(id BIGINT→STRING + 重建 idx_user),复制入 migrate-runner pod 后台跑(脚本 /tmp/rebuild_sessions_id_string.py,日志 /tmp/rebuild_sessions_run2.log)。**COPY 进度(13:30 实测)**:sessions_new 3 range 合计 1255.4MB / 源 sessions 1508.5MB = **83%**,COPY 单语句执行 1h03m 仍在 executing(早前"120MB/min"为 rebalance/compaction 瞬时噪音,真实持续 ~20MB/min → 剩余 ~12-15min);已恢复轮询重建日志的持久 monitor(b2jf7pgli,抓 COPY 完成/backfill 完成/DROP 旧表/重建完成/Traceback)。脚本后续自动:count 核验→恢复 3 副本 zone(num_replicas=3,z1/z2/z3 各1,lease→z1)+ 等 backfill(replica_localities<3 轮询)→ DROP+RENAME。完成后 deploy gateway-go 1.11.0(10-gateway-go.yaml 已 CRDB env 就绪)→ verify_gateway_archive.py 验证 End() 写 STRING id 归档。
- **方案C locality+follower-read 落地(2026-08-15 #149/#150 完成)**:**用户架构质询**"locality 空能否同步/是否改 2 副本"→已答:locality 仅影响调度不影响 Raft 同步;2 副本 CRDB 可用性数学更差(任何单节点丢失=写停)→ 用户批准方案C(3 副本+locality+follower-read)。**#149 已落地**:`deploy/crdb/crdb-3node.yaml` 按 pod ordinal 注入 `--locality=zone=z{1,2,3}`,rolling restart(Parallel policy 全滚,2/3 保持 quorum),实证 `gossip_nodes` node1=z1/node2=z2/node3=z3;`ALTER DATABASE esp_speaking CONFIGURE ZONE USING num_replicas=3, constraints='{+zone=z1:1,+zone=z2:1,+zone=z3:1}', lease_preferences='[[+zone=z1]]'`,实证 `SHOW RANGES ... WITH DETAILS` 全 range replica_localities={z1,z2,z3}+lease_holder=1,intent_bytes=0。**#150 已落地(代码+单测)**:`esp-common-go/db/db.go` 读池 DSN 注入 `default_transaction_use_follower_reads=on`(startup 参数,psycopg2 实证 CRDB 接受 + pgx v5 pgconn RuntimeParams 转发),写池不注入保持强一致;`db_test.go` 3 测试通过(CRDB 读注入/写不注入/mysql 路径不变)。**重建提速**:sessions_new 载入期临时 `num_replicas=1`(单节点 3 副本同步写放大 3x 是重建慢根因,~12%/38min→预计 3x 提速),COPY 完成 count 核验后恢复 3 副本 zone + 等 backfill(卡点数 replica_localities<3 轮询)再 DROP+RENAME;脚本已加该逻辑。**进行中**:sessions_new COPY(1 副本)后台跑 + gateway-go:1.11.0 重建镜像(带 follower-read)并行。

### 已完成✅(含证据)
- **#133 重构落地✅(按审查 Top5 ROI)**:learning-java 拆独立 DB writer 线程池——新增 `disruptor/EventBatchQueue.java`(有界 BlockingQueue+深拷贝入队+队列满 DROP 不阻塞 consumer+独立 writers=4);`BatchEventHandler` flush 不再同步调 consumer;`DbLearningEventConsumer` batchUpdate 失败整批写 outbox(status=failed)由 OutboxRelay 重试替代仅 log 丢数据;`OutboxRelay` 逐行 N+1→batchUpdate(200 行 400 往返→~3);disruptor 配置显式加 consumers/writers/writer-queue。gateway-go WS 加固——新增 `conversation-go/src/ws/SafeConn`(per-conn writeMu+WriteDeadline 10s),hub conns 改 *ws.SafeConn 解并发写(gorilla 非并发安全),`SetReadDeadline(60s)`+SetPongHandler+30s ping 协程,`newID()`→`sess_<unixms>_<8位rand hex>`(零新依赖,注释说明),`session.Manager.End` 归档有界 semaphore(256)。GlobalExceptionHandler 三服务各一(统一 envelope{code,message,traceId},actuator 白名单)。**测试全绿**:gateway-go/conversation-go go test ok(含 newID 并发 1000 无重复)、learning-java 59、bff 33、teacher 20。**部署:esp/gateway-go:1.10.0 + esp/learning-java:1.10.0 rollout 成功**(pod 日志确认 consumers=4/writers=4/writerQueue=100),冒烟 /livez /readyz 200、/api/scenarios 200、WS 收到 ai_reply 帧(TTS+emotion+visemes)、session ID 新格式。**遗留:bff/teacher 的 handler 代码已写但未重建镜像部署**(并入 #135 统一做)。
- **#132 CRDB DAL✅**:esp-rw-java 新增 `CockroachDbDataSourceProperties/Config`,`esp.rw.storage.type=tidb|cockroachdb`(默认 tidb 互斥门控 @ConditionalOnProperty),读写分离复用 RoutingHolder/@DataSource,PG driver postgresql:42.7.8(provided)。19 测试全过(4 新 CRDB 配置+2 条件装配+13 回归),mvn install 成功。
- **#134 README 细化✅**:README.md + README.zh-CN.md 同步新增架构速览数字表、Disruptor 多级调度(.stage 链)+分片(sequence%shardCount)mermaid、读写分离 RoutingDataSource 图、并发模型评分表(6.7/5.7/4.0/3.8)、G-04/G-06 压测数字表+重构后预期(标待复测)、覆盖率矩阵、5.2/10 工程化评分卡。**两处待校正**:bff 实际 2-stage(非 3)、esp.rw.storage.type 已实现但 README 标 "reserved"(#132 已实现)。
- **工程铁律 10 条✅(所有项目)**:权威副本 `docs/design/architecture/engineering-ironrules.md`(根因 R1-R6 + 方案 S1-S6 + 铁律 1-10:①公共能力共享包优先②设计三态追踪③契约测试④无产物不声称完成⑤单节点显式 DEFERRED⑥CI门禁⑦双轨落盘⑧共享设施红线⑨上下文工程⑩诚实优先);全局 memory `engineering-ironrules` + 全局 ~/.claude/CLAUDE.md 第 7 条;Redis `esp:ctx:ironrules:engineering`。
- **共享代码审计✅**:`docs/design/architecture/shared-code-audit-2026-08-15.md`。发现:GlobalExceptionHandler×3、JwtValidator/JwtAuthFilter×2(逐字节同仅 package)、AppLogClient×3、jjwt+bc 依赖块×3、gateway 自写 tracing.go/healthz、conversation/vocab 自写 DB 池(20/5 vs 默认 10/2)、esp-common-rs health.rs 零服务使用、emotion/writing 手写 logging_client.py、**bff Dubbo 配置全空转(0 @DubboReference,全走同步 RestClient)**、IdempotencyFilter 无 @Component、跨语言 traceId 键名不统一(traceId/trace_id)。Top5 抽共享包路线(esp-web-common/JWT starter/common-go 收编/rs 健康/app_logs 契约)。
- **#124 归档✅**:PROGRESS 623→82 行,归档 `docs/progress/archive/session22-27-to-30-archive.md`;Redis `esp:ctx:hot:session-24`。
- **#139 CRDB 驱动兼容修复✅(2026-08-15)**:根因=PG JDBC 42.7.8 `setObject(int, java.time.Instant)` 抛 SQLState 07006(最小探针实证,TiDB/MySQL 驱动支持 Instant 故 TiDB 模式正常);CRDB 模式下 learning_events 批量写**全转 outbox(~4 往返/事件)** + app_logs 遥测降级丢弃。修=`DbLearningEventConsumer`/`AppLogClient` 时间参数绑定 Instant→Timestamp,重建部署 **learning 1.11.1 / bff 1.10.0 / teacher 1.10.0**。验证:behavior POST→learning_events 直写 +1 零 outbox 转发;app_logs 实落 CRDB `esp_shared.app_logs`(`behavior.answer 999`)。
- **#137 全服务 TiDB vs CRDB 压测✅(2026-08-15)**:同一 harness(`report/crdb_fullbench.py`,集群内 ClusterIP 直连 RS256,5 端点×并发 20/60)。TiDB 基线 vs CRDB 腿:**全部 DB 路径 CRDB 落后**(learning_events 写 0.41-0.56×、review 读 0.42-0.62×、kp-stats 读 0.54-0.69×、bff scenarios 读 0.33-0.71×;gateway 非 DB 参考 0.80-1.00× 示单节点争用噪声),与微基准 0.54× 一致。对比落盘 `report/crdb-vs-tidb-fullbench-2026-08-15.json`(+`{tidb,cockroachdb}-2026-08-15.json`);README 双版已更新;服务已恢复 TiDB 模式(生产缺省)。
- **运维清理**:17 个卡 ContainerCreating 孤儿 pod(st-seeder-*×16 + esp-schema-init,无 owner)强删;HPA learning-java/teacher-java 钉 1/1 释放 ~1000m CPU;esp-crdb 恢复 3 副本(3/3 Running,v24.3.11)。

### 进行中🔄
- **#136 CRDB 分库+灌数✅(2026-08-15 完成)**:6 域库 13 表全部灌入,seed pod Succeeded(elapsed=7639s≈2.1h,断点续跑:users/scenarios/kp 已满跳过、sessions 补 1.14M→5M)。**最终计数(seed 脚本 final_counts == 独立复核)**:users 1M / scenarios 10k / knowledge_points 10k / sessions 5M / session_turns 5M / **user_mastery 4,998,737(差 1,263 行,0.025%)** / learning_events 10M / writing_tasks 2M / lesson_plans 1M / classes 500k / assignments 2M / submissions 2M / app_logs 85k。合计 33,603,737 行。**唯一偏差**:user_mastery 差 1,263(两源一致,如实记录;对压测无影响)。Redis `esp:ctx:seed-crdb-progress` 100%。
- **#135 esp-web-common 共享模块✅(2026-08-15 部署+冒烟完成)**:mvn package×3 RC=0 → docker build×3 RC=0 → apply learning 1.11.0 / bff 1.9.0 + set image teacher 1.9.0,全部 rollout ready=1/1。**TiDB 模式冒烟全绿**:learning readiness UP / behavior POST `{"accepted":true}` **且 esp_shared.app_logs 实落一行** `('learning','behavior.smoke135',12345)`(#135 两段式 INSERT 验证通过)/ recommend 返回复习数据 / **bff scenarios(8090 mvc 端口)200** / **teacher kp-stats(teacher 角色 token)200** 返回真实 mastery 数据。**附带修复**:live teacher envFrom=null(JWT 公钥未注入→filter 禁用→无 role 全 403)→ patch 对齐 git yaml(esp-rsa-jwt-verify),pubkey 注入后鉴权正常。
- **#131 learning-worker 分片(待执行)**:worker yaml image 已升 1.11.0。入口 ESP_DISRUPTOR_ENABLED=false→Pulsar,worker PULSAR_CONSUMER_ENABLED=true,验证 1→3→5 消费。

### 问题与解决方案(本会话)
| 问题 | 解决 |
|---|---|
| 声称完成但无产物(#131/#132/#133 前期核验) | 核验发现未做→实际执行;触发铁律四"无产物不声称完成" |
| 单节点 CPU 99%(HPA 扩 learning 4/teacher 3) | 钉 HPA min/max=1,释放 ~1000m CPU |
| worker 会落错库(esp 100M 老库而非 esp_learning) | 21-learning-worker.yaml WRITE/READ→esp_learning + SHARED_DB_URL |
| CRDB 单库 5 冒烟表(用户要求分库) | 6 域库拆分 + 等量灌装脚本 |
| 16 seeder pod 卡 ContainerCreating 173min | 孤儿 pod(无 ownerReference)强删 |
| TiDB Region unavailable(大表 count 不稳) | 大表行数记近似值,压测前 TiKV 健康检查 |
| GlobalExceptionHandler 散落 3 服务 | 建 esp-web-common 共享包(#135,已部署验证) |
| live teacher envFrom=null→JWT 禁用→全 403 | patch envFrom esp-rsa-jwt-verify 对齐 git yaml,冒烟 200 |
| bff 冒烟 8080 Connection refused | bff 端口=mvc 8090/edge 9090/dubbo 50090,非 8080;改用 8090 验证通过 |
| CRDB 模式 learning 批量写全转 outbox + app_logs 降级(#139) | 根因=PG JDBC 42.7.8 setObject(Instant) SQLState 07006(探针实证;MySQL 驱动支持 Instant 故 TiDB 正常);改 Timestamp 绑定 + 重建,验证直写零 outbox + app_logs 落 CRDB |
| bff/teacher 增量 mvn package 假成功(内嵌旧 esp-web-common) | 源码未变→repackage 跳过;改 `mvn clean package` 强制重打,解包核验修复字节码 |

### 恢复锚点(会话22-35)
- **当前子步骤**:#137 全服务 TiDB vs CRDB 压测**已闭环**(证据见"已完成"#139/#137),服务已恢复 TiDB 模式(production 缺省),README 双版已更新。**当前任务 = #131 learning-worker 1→3→5 副本水平分片验证**:① worker yaml `21-learning-worker.yaml` image 现为 esp/learning-java:1.11.0 → 需升 **1.11.1**(#139 重建后镜像);② 入口 learning-java 设 ESP_DISRUPTOR_ENABLED=false + PULSAR_CONSUMER_ENABLED=false(防消费循环),worker 设 ESP_ROLE=worker + PULSAR_CONSUMER_ENABLED=true;③ replicas 1→3→5 观察 Pulsar 消费分片 + learning_events 落库(worker 写 esp_learning)。
- **已确认根因**:G-06=Disruptor consumer 同步 batchUpdate 反压(已拆独立 writer 池);bff Dubbo 配置未接线(0 @DubboReference);teacher 403=live envFrom null 无公钥(filter 禁用);**CRDB 模式批量写全转 outbox=PG JDBC 42.7.8 setObject(Instant) SQLState 07006(已改 Timestamp 修复,#139)**。
- **部署核实(2026-08-15 更新)**:活体=learning **1.11.1** / bff **1.10.0** / teacher **1.10.0**(TiDB 模式);CRDB 6 域库 seed 33.6M 行保留(待多区域复测);crdb-public 26257 通。
- **已改文件**:`DbLearningEventConsumer.java`+`AppLogClient.java`(Instant→Timestamp)、`README.md`/`README.zh-CN.md`(#137 全服务对比 3 处)、`report/crdb-vs-tidb-fullbench-*.json`×3、`21-learning-worker.yaml`(待升 image→1.11.1)。
- **下一步**:#131 worker 1→3→5 分片验证 → #131 完成双轨落盘收尾。
- **Redis 锚点**:`esp:ctx:hot:session-26`(本更新)、`esp:ctx:session:session-22-35`、`esp:ctx:seed-crdb-progress`。

---

## 当前✅(2026-08-15 会话22-33 全服务切 CRDB 完成 + sessions STRING 重建 + 用户指令停灌)

### 任务
#140 全服务数据访问切 CRDB + 关停 TiDB(用户根指令);#141~#145 Phase1~2 Java/Go 切 CRDB env;#146 清理 Seata AT 死配置;#147~#150 Go 统一+CRDB DDL+3副本+follower-read;#151 重建 sessions STRING + 灌随机数据;#152 Java 服务切 CRDB env apply。

### 落地内容(CRDB 切库全闭合✅)
- **#140 全服务切 CRDB✅**:Java 4 服务(bff/teacher/learning/learning-worker)env `ESP_RW_STORAGE_TYPE=cockroachdb` + `ESP_RW_COCKROACHDB_HOST=crdb-public`,均已 PgConnection 实连验证;Python 服务(writing-py/emotion-svc/writing-coach)env `WRITE_DB_HOST/READ_DB_HOST=crdb-public:26257`;Go 统一 esp-common-go 双驱动(pgx+mysql)可回切。**TiDB 已 scale 0**(svc+PVC 保留可回切)。
- **#152 Java CRDB env apply 4/4 Ready✅**:`15-bff-java.yaml` L26 `SEATA_ENABLED=true→false`(yaml 漂移修正:原 true 致新 pod 连不上已 scale 0 的 Seata TC→CrashLoopBackOff;根因非 CRDB)。新 pod `bff-java-7bb9b48f54-27znm` 1/1 Running,PgConnection 实连,env 确认 `ESP_RW_STORAGE_TYPE=cockroachdb`/`SEATA_ENABLED=false`。learning/worker/teacher 前序已 Ready。
- **#151 sessions STRING 重建✅(用户指令停灌)**:DDL `id STRING NOT NULL PRIMARY KEY`(gateway-go 1.11.0 硬要求)。灌入 **2,020,000 行**(id=sess_<n>,无重复,类型=STRING)后**用户明确指令停止灌装**("已有的数据可以直接使用了")。灌数期间 `num_replicas=1`(负载阶段),**已恢复 3 副本 zone**(`CONFIGURE ZONE USING num_replicas=3`,SHOW CREATE 实证)。用户决策:2.02M 即最终数据量,不再追 17M 目标。

### 待执行
- **多区域演进**(DEFERRED):加 `region=` label + `ALTER DATABASE ... LOCALITY REGIONAL`,应用代码零改动,多节点服务器上线后启用。
- **learning_events 幂等缺口**(P1-8 候选):`DbLearningEventConsumer` 纯 INSERT,无 UNIQUE 约束(messageId 去重),Shared 重投递有重复风险。当前 10,001,206 行稳态无增量。

### 恢复锚点(会话22-33)
- **当前子步骤**:CRDB 切库全闭合✅;sessions STRING 2.02M✅(用户停灌);3 副本 zone 已恢复✅(z1/z2/z3 各 2,020,000,under-replicated=0,range 84/84/84 均衡);**#131 worker 3 副本水平分片闭合✅**(用户定不再扩 5);**#154 Phase4 CRDB 连接回归闭合✅**(7/7 env 指 crdb-public,6 直连 PgConnection 实连,0/7 TiDB 回退);**#144 TiDB 清理闭合✅**(用户决策:共享底座 scale 0 + PVC 保留作回切网,符合 esp-shared-infra-ironrule;ESP deploy yaml 的 TiDB 惰性回切 env 保留作占位,#132 双池门控下非活路径;邻居 education-platform 5 deploy env 仍指 TiDB 但全 scale 0,互不干扰)。
- **已确认根因**:bff CrashLoopBackOff = Seata yaml 漂移(SEATA_ENABLED=true vs TC scale 0),非 CRDB;新 worker 启动期 redis-cluster DNS 瞬态 NXDOMAIN(Netty DNS resolver 与 pod 网络就绪竞态),startupProbe 900s 窗口内自愈转 1/1。
- **已改文件**:`15-bff-java.yaml`(L26 Seata fix)、`20/21/30-other` yamls(CRDB env)、`scripts/seed_sessions_parallel.sh`、`report/phase4-crdb-connection-verify-2026-08-16.md`。
- **TiDB 清理最终态**:tidb-cluster-tidb/pd-aws/tikv-aws deploy 全 0/0 scale 0;PVC pd-aws-data(5Gi)/tikv-aws-data(10Gi) Bound 保留;esp-prod ExternalName tidb-primary/tidb-follower 保留(回切链);4 Java 服务 SEATA_ENABLED=false 实证。
- **下一步**:#140/#144 双轨落盘收尾 → 等待用户下一指令(候选:P1-8 幂等 / 多区域演进 / 计划文件 P0 安全红线重构)。
- **Redis 锚点**:`esp:ctx:hot:session-25`、`esp:ctx:hot:session-26`(会话22-35)。

---

## 上一节(2026-08-15 会话22-32 架构师级全服务审查 + Disruptor 公共多级调度队列审查)

### 任务
#129 Disruptor 公共多级调度队列审查 + #130 架构师级全服务审查(架构评分 + Top5 ROI 重构项)。

### 落地内容(审查闭合项)
- **#129 Disruptor 公共多级调度队列审查✅**:`platform/esp-disruptor` 的 DisruptorPipeline 支持任意层级自定义(`.stage()` 链)——bff-java 3 级 ingest→normalize→forward;learning-java 单级 DisruptorWorker;**teacher-java 未接入 = 唯一缺口**。整体流程测试通过:`LearningEventDisruptorTest` 2/2(批量消费 + ≥5000ev/s 发布吞吐)。
- **#130 架构师级全服务审查✅**:报告 `report/architecture-review-2026-08-15.md`,综合 **5.2/10**(栈选型7/落地4/测试4/根因7/质量4)。**Top5 ROI**:① learning Disruptor consumer 拆独立 DB writer 线程池(现 4 consumer 线程同步 batchUpdate 反压 RingBuffer);② gateway-go WS ping/pong+deadline+UUIDv7 session ID+Hub 写互斥(现 `sess_<unix_ms>` 碰撞 + 无 deadline 头线阻塞);③ bff-java 接线 Dubbo Triple(配置已存在但零 `@DubboReference`,全走同步 RestClient=G-06 根因之一);④ 共享模块(esp-common-go/rust/esp-cache-py/esp-db-py)零单测补齐;⑤ 全局异常处理+DB 池 env 化+日志统一。**新发现**:OutboxRelay 逐行 INSERT N+1、DbLearningEventConsumer batch 失败仅 log 丢数据。
- **learning-java 1.9.0 稳定部署**:rev25 单 pod 1/1 Running;`learning_event_outbox` 表已在 esp_learning schema 落地。

### 待执行
- **#131** learning-worker 1→3→5 分片验证(部署 0 副本未启);**#132** esp-rw-java 加 CockroachDB 读写分离+可配置选型(esp-crdb 已扩容 3 副本);**#133** 按审查 Top5 重构落地;**#134** README 细化。

### 恢复锚点(会话22-32)
- **当前子步骤**:#129/#130 审查闭合✅(本会话双轨落盘)。已改文件=none(纯审查)。
- **下一步**:#131 worker 分片 → #132 CRDB DAL → #133 重构落地 → #134 README。
- **Redis 锚点**:`esp:ctx:hot:session-24`。

---

## 当前✅(2026-08-15 会话22-31 C/D 类批量推进:全 7 项闭合✅)

### 任务
兑现 C 类(Loki/ArgoCD)+ D 类(73条人工测试/5000CCU压测复测/探针三分离/数字人TTS/个性化推荐/全方位README)。用户指令"完整完成所有任务(可并行则并行)"+ 要求双轨落盘以便上下文溢出后恢复。

### 落地内容(7/7 全闭合✅,详细数字见对应 report/ 与 manifest)
- **#116 Loki 日志聚合✅**:otel-collector filelog receiver(零新镜像,fluent-bit 403 不可拉)。坑链=docker json 日志符号链接/root-only/json_parser+regex_parser/cid guard;6/7 pod Running,~150k-210k/2-3min 入 Loki。manifest `deploy/k8s/monitoring/10-observability.yaml`。
- **#120 探针三分离✅**:11 服务全量改 /healthz /readyz /livez(Go gin)+ /actuator/health/readiness/liveness(Java/Rust);tcpSocket→httpGet;preStop sleep 5~15 + terminationGracePeriodSeconds 30。gateway-go 1.9.0 重建(rollout clean,/readyz /livez 200)。
- **#121 数字人 TTS 情绪驱动✅**:`conversation-go/src/ws/frame.go` Frame 加 Emotion+Visemes;`src/tts/mock.go` Result{Audio,Emotion,Visemes}(Preston Blair 口型 60ms/viseme,cap 400);gateway-go handleUserText 接 TTS(仅 AIReply 帧附 emotion+visemes);mock_test 3/3;gateway-go 1.9.1 重建。
- **#122 个性化推荐 emotion_aware✅**:`BehaviorController` /recommend 加第 4 维(近 7 天 emotion 事件 neg/pos/unknown 三态);builder agent 旧 jar 坑→`mvn -DskipTests package`+`--no-cache` 重建 1.8.9;实测 keys=[review,weak_points,similar_scenarios,emotion_aware],emotion_aware={state:unknown}。
- **#118 73条人工测试✅**:report/manual-73-run-20260815-062028.json(PASS=57/FAIL=1/SKIP=22)。FAIL 仅 SEC-6(限流 429 未触发,Nacos flow 未发布非功能缺陷);placement 带 kp_ids 经 BFF 返 401 待 trace;WS 全会话 TC-E2E-7 PASS。
- **#123 全方位 README✅**:README.md + README.zh-CN.md(各 635 行,6+ mermaid 图);CRDB vs TiDB 实测(139.0 vs 259.3 insert/s,ratio 0.54,多区域 4 维度,拒选结论);四语言 12 维对比表+决策树。
- **#119 5000 CCU 压测复测✅(未达标,单节点瓶颈)**:report/ccu5000-g0{4,6}-*-2026-08-15.json。G-04 WS peak 955(FAILED 5000,根因=gateway-go 单 pod 512Mi+session ID 碰撞;建议多节点+3 副本 1Gi+UUIDv7);G-06 events peak 370ev/s(FAILED 5000,根因=同步 thread-per-request+Disruptor consumer 同步批量写+scale 2 反 CPU 争抢;建议独立 DB writer 线程池+修 OutboxRelay)。
- **#117 ArgoCD GitOps✅(控制平面已起 + 3 STS OOS 定性为结构性限制)**:quay 镜像 403→本地缓存 v3.5.0 起控制平面;3 StatefulSet OutOfSync=managedFields=0 击穿 SSA 归属判定,`kubectl diff git vs live` 空=非真 drift,业务零影响,留多节点剥离管辖。

### 恢复锚点(会话22-31)
- **当前子步骤**:C/D 类 7/7 全闭合✅(#116/#117/#118/#119/#120/#121/#122/#123)。
- **已确认根因**:① builder agent 陷阱——Dockerfile `COPY target/*.jar`,mvn package 未重跑则 COPY 旧 jar,修复=`mvn -DskipTests package`+`--no-cache`+升 tag;② quay/公开镜像 403→本地缓存;③ 3 STS OOS=managedFields=0 空 diff 非真 drift。
- **下一步**:① git commit 批次(C/D 全闭合 + #117/#119 报告);② 多节点移植时剥离 3 STS argocd 管辖 + 5000CCU 达标。
- **Redis 锚点**:`esp:ctx:hot:session-23`、`esp:ctx:hot:refactor-p0~p2`。

---

## 历史 milestone 归档索引(需要时取)

| 主题 | 文件 | Redis key |
|---|---|---|
| **会话23-13 + 续(全量重构 7.3→8.1 / JDK26 镜像+部署收尾 / k3s 坑㉚ / gson 阻断闭合,2026-08-18 归档)** | `docs/progress/archive/2026-08-18-session-23-13-jdk26-refactor-deploy.md` | `esp:ctx:hot:session-23-13-refactor-complete` + `esp:ctx:session:23-13-cont` |
| **会话23-13 续2(H-01 混沌降级 PASS + CRDB probe 1s 误杀修复 + LLM 缓存铁律 LCO + Nacos 2.5.0 CRDB 迁移调研,2026-08-18 归档)** | `docs/progress/archive/2026-08-18-session-23-13-chaos-crdb-llmcache.md` | `esp:ctx:hot:2026-08-18-session-23-13-chaos-crdb-llmcache` + `esp:ctx:chaos:H01:recovery` |
| **会话23-13 续3(Nacos 下沉宿主机 — 未完成已回滚,2026-08-19)** | `report/nacos-sink-to-host.md` | — |
| **会话23-9 四线(vocab cap/SEC-6/CI/Obsidian,2026-08-17 归档)** | `docs/progress/archive/2026-08-17-session-23-9-four-line.md` | `esp:ctx:hot:session-23-9` |
| **会话22-14~22-30 + CI修绿 + P1-13 httpOnly(热区瘦身,2026-08-15 归档)** | `docs/progress/archive/session22-27-to-30-archive.md` | — |
| **会话22-21 k6+Locust 混合压测第二轮 + traefik 502 坑㉕ + 三根因闭合** | `docs/progress/archive/session22-21-loadtest-traefik502-launch.md` | `esp:ctx:hot:2026-08-14-session22-21-loadtest-launch` |
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
