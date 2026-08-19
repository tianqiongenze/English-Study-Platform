# Nacos 下沉宿主机执行报告(会话23-13续3,2026-08-19)——完成

> 用户决策:已告知风险(通道脆弱/裸进程无自愈/单点放大)仍坚持下沉。完整闭环已完成,集群 Nacos 已删除。

## 结论:下沉完成,集群 Nacos 已删,服务发现闭环验证通过

下沉的基础设施链路全部验证成功:ExternalName 切换 + 网络可达 + Dubbo provider/consumer 注册到宿主 Nacos healthy + bff 业务健康 UP。集群 Nacos deploy/svc/pvc 全删,ESP 服务删后仍 healthy(独立于集群 Nacos)。

## 历史回放(中途误判 + 纠正)

### 第一次执行(误判回滚)
首测 learning-java 实例用 `serviceName=learning-java&groupName=DEFAULT_GROUP` → hosts:[] 空,误判"Dubbo provider 没注册",触发 Step7 回滚到集群 Nacos。

### 第二次执行(纠正 group + 闭环成功)
- 关键纠正:Dubbo 3.3 应用级注册用 **group=esp**(非 DEFAULT_GROUP),`groupName=esp` 查得 learning-java + providers:LearningRpcService 两服务 healthy。
- ASV 核实(javap):Dubbo NacosConnectionManager.setProperties 对 `?username=&password=` 空串有 isNotEmpty 守卫,空认证不传 Nacos 客户端,不触发 login——推翻"空认证致 5-10"假设。
- metadata report 5-10 是非致命降级,不影响实例注册/服务发现。
- 真根因(rolling 卡死):HPA + 单节点 Insufficient cpu 致新副本 Pending,非下沉问题。

## 执行步骤与证据(第二次,成功闭环)

### Step 1:导出集群 Nacos数据 + 备份 yaml(可逆留底)
- 注册表:`report/nacos-cluster-services-export-20260819.json` → `{"count":1,"doms":["bff-java"]}`(只有 bff-java)
- 配置:`report/nacos-cluster-configs-export-20260819.zip` → empty
- deploy/svc yaml 备份:`report/nacos-cluster-deploy-backup.yaml` + `report/nacos-cluster-svc-backup.yaml`

### Step 2:patch ExternalName 指向宿主(成功,可逆)
- `kubectl -n esp-prod patch svc nacos -p '{"spec":{"externalName":"host.lima.internal"}}'` → patched
- verify:`host.lima.internal`(通过)

### Step 3:pod 测宿主 Nacos 可达性(成功)
- gateway-go `nc -zv host.lima.internal 8848` → `192.168.5.2:8848 open`
- bff-java `nc -zv nacos 8848` → open(DNS nacos→192.168.5.2)
- gateway-go `nc -zv nacos 9848/9849` → open(gRPC metadata 端口通)
- 宿主 Nacos 监听:lsof 8848/9848/9849 三端口 LISTEN(PID 12795)

### Step 4:rolling restart Java 服务(部分慢,非下沉问题)
- `kubectl -n esp-prod rollout restart deploy/bff-java deploy/learning-java deploy/teacher-java`
- rolling 多数 120s 超时,根因 Insufficient cpu(单节点资源不足),HPA 扩缩容 + Pending churn

### Step 5:验证服务发现(成功闭环)
- 宿主 Nacos esp group service/list:`{"count":2,"doms":["learning-java","providers:com.esp.api.dubbo.LearningRpcService:1.0.0:esp"]}`
- learning-java(esp group)实例:**4 个 healthy**(10.42.0.170/0.54/0.83/0.142:50091,Dubbo provider triple)
- bff-java(DEFAULT_GROUP)实例:3 个 healthy(10.42.0.138/0.141:8090 等)
- teacher-java consumer 日志:`[INSTANCE_REGISTER] Notify urls for subscribe url consumer://...LearningRpcService... url size: 2`(成功订阅 learning 2 provider 实例)
- bff actuator/health:HTTP 200 UP(discovery.services=["bff-java"] + CRDB crdbReadDataSource/crdbWriteDataSource UP + Redis cluster_size:3 slots_up:16384 slots_fail:0)

### Step 6:删集群 Nacos(执行,Step5 通过后)
- `kubectl -n ekb-m2-aws-us-east-1 delete deploy nacos` → deleted
- `kubectl -n ekb-m2-aws-us-east-1 delete svc nacos nacos-np` → deleted(无引用 30048/nacos-np,已扫描确认)
- `kubectl -n ekb-m2-aws-us-east-1 delete pvc nacos-data-pvc` → deleted(Derby 旧数据,已切 CRDB + 有导出留底)
- 确认:`kubectl get deploy,svc,pvc -n ekb-m2-aws-us-east-1 | grep nacos` 空
- 删后验证:ESP 服务仍 healthy(learning 4 实例 + bff 3 实例在宿主 Nacos),独立于集群 Nacos

## CRDB 通信稳定性深度分析(用户质疑)

### 现状(脆弱已实证)
- 宿主 Nacos(PID 12795,localhost:8848)↔ CRDB 走 `ssh -L 26257` mux port-forward(PID 57226)
- HikariPool 4 条 ESTABLISHED 连接 127.0.0.1:26257
- 通道当前活,但本质脆弱

### 脆弱点
1. ssh mux 进程(PID 57226)是裸进程,无 launchd/supervisord 托管,崩溃/VM 重启即断
2. 断后 127.0.0.1:26257 端口消失,Nacos HikariPool 连接失效,Nacos 写 CRDB 全挂且无自愈
3. VM 重启后 lima ssh sock 路径可能轮转(memory esp-rd-docker-ssh-port-rotation)

### 根因(非 CRDB 配置,是网络拓扑)
- **192.168.5.15(db-crdb-np LoadBalancer EXTERNAL-IP)宿主 nc 超时**——宿主机路由表无 192.168.5.0 项,LB/NodePort IP 宿主均不可达
- Rancher Desktop 单节点宿主↔VM 网络隔离:宿主无 pod CIDR(10.42.x)/LB IP(192.168.5.x)路由,**只能 ssh tunnel**
- 宿主网桥:192.168.64.1(lima VM 宿主侧网关),但 VM 内部 K8s 网络对宿主不可路由

### 彻底处理方案(分层,按工业级 + 可移植多节点)

**层1:单节点环境内,根治 ssh 脆弱(✅ 已完成 2026-08-19)**
- launchd plist(`/Users/joelgong/Library/LaunchAgents/com.esp.nacos-crdb-channel.plist`)托管 ssh -L 26257
- 每 30s 健康检查 + 自愈脚本(`scripts/nacos-crdb-channel-health.sh`):`nc -z 127.0.0.1:26257` 失败则重连 ssh port-forward
- 关键实现:用 `ssh -F <lima ssh.config> -L 26257:... lima-0`(读 config 文件复用 ControlPath mux),**不用** `-o ControlPath=...`(路径含空格 "Application Support" 致 ssh 报 `keyword controlpath extra arguments`)
- 兜底:ssh 失败时 `kubectl port-forward`(nohup 脱离)兜底
- 自愈验证(2026-08-19):kill ssh mux 后 20s 内 launchd 自动重连,26257 监听恢复 + Nacos healthy(OK)
- 比 bare ssh 强:VM 重启后 launchd 自动拉起,断线自动重连
- 局限:仍依赖 ssh + lima ssh.config 路径稳定(sock 轮转 memory esp-rd-docker-ssh-port-rotation 需 config 路径同步;当前 config 由 Lima 生成,重启后重建)

**层2:多节点移植,根治网络隔离(架构正解)**
- 多节点 K8s 下,宿主 Nacos 所在节点即 K8s node,pod CIDR 直连可路由
- ClusterIP/NodePort 直达,**彻底消除 ssh tunnel**
- ssh tunnel 本就是单节点 RD 的临时妥协,多节点自然消解

## 3 硬伤 DEFERRED 标注(用户已知情坚持,诚实记录)

### DEFERRED-1:通道脆弱(VM 重启断,无自愈)— 层1 已根治
- 宿主 Nacos↔CRDB 依赖 ssh mux port-forward(PID 57226),原无 launchd 托管
- VM 重启则通道断,宿主 Nacos 写 CRDB 失败,无自动恢复机制
- ✅ 层1 已根治(2026-08-19):launchd agent `com.esp.nacos-crdb-channel` 每 30s 健康检查 + 自愈重连,kill ssh 后 20s 内自动恢复,26257 + Nacos healthy 验证通过
- 根治:层1 launchd 托管(✅ 已完成)+ 层2 多节点移植(根治,DEFERRED 待多节点迁移)

### DEFERRED-2:宿主 Nacos 裸进程无 K8s 自愈
- PID 12795 是裸 java 进程,非 K8s pod,无 readiness/liveness probe,无 restart policy
- 进程挂掉无自动拉起,需人工启动
- 违背 SRE 基线

### DEFERRED-3:单点放大违反 Nacos 优化诉求
- Nacos 集群模式诉求多节点 raft + 去中心化;下沉到单宿主进程是单点放大
- 5000 CCU + 三端规模下单点 Nacos 是可用性瓶颈(注册中心宕则全服务发现瘫痪)
- 违背 Nacos 集群设计初衷,仅作单节点验证环境妥协

## 产物清单(绝对路径)
- /Users/joelgong/claude-Projects/English-Study-Platform/report/nacos-cluster-services-export-20260819.json
- /Users/joelgong/claude-Projects/English-Study-Platform/report/nacos-cluster-configs-export-20260819.zip
- /Users/joelgong/claude-Projects/English-Study-Platform/report/nacos-cluster-deploy-backup.yaml
- /Users/joelgong/claude-Projects/English-Study-Platform/report/nacos-cluster-svc-backup.yaml
- /Users/joelgong/claude-Projects/English-Study-Platform/report/nacos-sink-learning-metadata-fail-evidence.txt
- /Users/joelgong/claude-Projects/English-Study-Platform/report/nacos-sink-to-host.md(本报告,最终完成版)
- /Users/joelgong/claude-Projects/English-Study-Platform/docs/PROGRESS.md(热区已更新)
- Obsidian:/Users/joelgong/Documents/Obsidian/人工智能项目/ESP平台/{进度同步,关键坑记录}.md(已同步)
- Redis Cluster:`esp:ctx:hot:session-23-13-nacos-sink` + `esp:ctx:session:23-13-nacos-sink-cont`(已写入)

## 下一步(若要彻底解决 CRDB 通道)
1. 层1 自愈:写 launchd plist + 健康检查脚本托管 ssh port-forward 26257(根治 ssh 脆弱)
2. Phase 2:locust/k6 全链路压测 + E2E 全功能测试(集群稳定后)
3. CRDB vs TiDB 全方位对比报告
4. 全文档更新(README/architecture docs 补 CRDB 连接拓扑设计)

## 诚实声明
- 集群 Nacos 已删(deploy/svc/pvc 全清),不可逆,但 report/ 有 yaml 备份可重建
- 下沉基础设施链路成功 + Dubbo provider(learning 4 实例)/consumer(bff 3 实例 + teacher 订阅)全注册宿主 Nacos healthy
- bff 业务健康 HTTP200 UP(CRDB 读写分离 + Redis cluster + discovery 全 UP)
- CRDB 通道(ssh port-forward)当前活但脆弱,根治方案分层已给(层1 launchd 待实施,层2 多节点移植正解)
- 3 硬伤 DEFERRED 诚实标注,用户已知情坚持下沉
