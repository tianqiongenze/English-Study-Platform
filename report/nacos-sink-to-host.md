# Nacos 下沉宿主机执行报告(会话23-13续3)

执行时间:2026-08-19 00:00 ~ 00:30
执行人:esp-builder subagent
用户决策:已告知风险仍坚持下沉(裸进程无自愈 / 单点放大 / 通道脆弱)

## 结论:下沉未完成,已回滚到集群 Nacos(下沉前稳态)

下沉的基础设施链路验证成功(ExternalName 切换 + 网络可达 + bff 实例注册到宿主 Nacos healthy),
但 **learning-java / teacher-java 的 Dubbo provider 实例未注册成功**(metadata report 5-10 持续失败),
不满足 Step 5 "服务发现正常" 的验证门槛,故未执行 Step 6(删集群 Nacos),已触发 Step 7 回滚。

## 关键发现:learning 注册失败不是下沉引入的回归

回滚到集群 Nacos 后,learning-java 在集群 Nacos 的实例列表 hosts:[] 同样为空。
即 **learning-java 的 Dubbo provider 实例注册本身就有既有缺陷**,与 Nacos 下沉无关。
下沉没有让任何服务的注册状态变坏(bff 在宿主 Nacos healthy,在集群 Nacos 也 healthy)。

## 执行步骤与证据

### Step 1:导出集群 Nacos 数据 + 备份 yaml(成功,可逆)
- 注册表导出:`report/nacos-cluster-services-export-20260819.json` → `{"count":1,"doms":["bff-java"]}`(只有 bff-java,learning 本就未注册)
- 配置导出:`report/nacos-cluster-configs-export-20260819.zip` → empty(空,符合前提)
- deploy yaml 备份:`report/nacos-cluster-deploy-backup.yaml`(4523 bytes)
- svc yaml 备份:`report/nacos-cluster-svc-backup.yaml`(2322 bytes,nacos ClusterIP + nacos-np NodePort 30048)

### Step 2:patch ExternalName 指向宿主(成功,可逆)
- 原值(回滚锚点):`nacos.ekb-m2-aws-us-east-1.svc.cluster.local`
- patch:`kubectl -n esp-prod patch svc nacos -p '{"spec":{"externalName":"host.lima.internal"}}'` → patched RC=0
- verify:`host.lima.internal`(通过)

### Step 3:pod 测宿主 Nacos 可达性(成功)
- `kubectl -n esp-prod exec deploy/gateway-go -- nc -zv host.lima.internal 8848` → `host.lima.internal (192.168.5.2:8848) open` RC=0
- bff-java 无 curl/nc,改用 getent + nc:`nacos` → 192.168.5.2(host.lima.internal);`nc -zv nacos 8848` → open RC=0
- grpc 端口:gateway-go `nc -zv nacos 9848` open,`nc -zv nacos 9849` open(Dubbo metadata report gRPC 端口可达)
- 宿主 Nacos 监听:lsof 确认 8848/9848/9849 三端口 LISTEN(PID 12795)

### Step 4:rolling restart Java 服务(部分卡死,记录不阻塞)
- `kubectl -n esp-prod rollout restart deploy/bff-java deploy/learning-java deploy/teacher-java` → restarted RC=0
- learning-java/teacher-java rollout status 120s 超时(learning 1/2 更新、teacher 旧副本待终止)
- bff-java rollout 120s 超时(3/4 更新)
- 根因:`Insufficient cpu`(单节点资源不足,FailedScheduling 0/1 nodes available)——非下沉问题,HPA 弹性扩缩容 + 单节点资源紧张导致新副本 Pending
- 任务说明:bff-java 有独立 subagent 修,记录不阻塞

### Step 5:验证服务发现(部分失败 → 触发回滚)
- 宿主 Nacos 注册表:`{"count":1,"doms":["bff-java"]}` bff-java 出现
- bff-java 实例:`curl localhost:8848/nacos/v1/ns/instance/list?serviceName=bff-java` → 1 个 healthy 实例(10.42.0.82:8090,`healthy:true`,`preserved.register.source:SPRING_CLOUD`)— **成功**
- learning-java 实例:`hosts:[]` 空 — **失败**
- teacher-java 实例:`hosts:[]` 空 — **失败**
- learning 新 pod r24nr(10.42.0.83)日志持续报 Dubbo metadata report 失败:
  - `[5-10] Failed register interface application mapping for service esp/com.esp.api.dubbo.LearningRpcService:1.0.0`
    `This may be caused by configuration server disconnected`
  - `[1-37] publish nacos metadata failed`
  - 证据:`report/nacos-sink-learning-metadata-fail-evidence.txt`

### Step 6:删集群 Nacos —— **未执行**(Step 5 未通过)

### Step 7:回滚(已执行,恢复下沉前稳态)
- `kubectl -n esp-prod patch svc nacos -p '{"spec":{"externalName":"nacos.ekb-m2-aws-us-east-1.svc.cluster.local"}}'` → patched RC=0
- verify:`nacos.ekb-m2-aws-us-east-1.svc.cluster.local`(恢复原值)
- `kubectl -n esp-prod rollout restart deploy/bff-java deploy/learning-java deploy/teacher-java`(重连集群 Nacos)
- 回滚后集群 Nacos 注册表:`{"count":1,"doms":["bff-java"]}`(与下沉前一致)
- bff-java 实例在集群 Nacos:10.42.0.111:8090 healthy:true — **回滚成功**
- learning-java 在集群 Nacos:hosts:[] 空(与下沉前一致,既有缺陷)

## 根因分析:learning metadata report 5-10 失败

### learning 的 Dubbo 配置(jar 内 application.yml,已实测)
```yaml
dubbo:
  application:
    name: learning-java
  protocol:
    name: tri
    port: 50091
  registry:
    address: nacos://${NACOS_HOST:nacos}:8848?username=&password=
    group: ${DUBBO_GROUP:esp}
  provider:
    group: ${DUBBO_GROUP:esp}
    version: 1.0.0
```

### 可疑点:registry.address 带 `?username=&password=`(空认证参数)
- learning env 只有 `NACOS_HOST=nacos`(无端口),Dubbo 用 `nacos://${NACOS_HOST:nacos}:8848?username=&password=`
- bff-java env 有 `NACOS_ADDR=nacos:8848` + `NACOS_NS=public`(配置不同,bff 是 consumer 不发 metadata,所以不受影响)
- Dubbo 3.3 默认 metadata-report.address 复用 registry.address,带空 `?username=&password=`
- 空认证参数可能触发 Nacos 客户端认证路径,导致 metadata 写入(config API)被拒 → 5-10 "configuration server disconnected"
- 但此配置下沉前就存在,集群 Nacos 下 learning 同样注册不上(hosts:[]),**非下沉引入**

### 排除项
- 网络:pod → host.lima.internal 8848/9848/9849 全 open(已实测)
- 宿主 Nacos 监听:8848/9848/9849 三端口 LISTEN(standalone 模式,默认无鉴权,namespace=public 存在)
- 集群 Nacos svc:纯 ExternalName 无 ports 定义(DNS CNAME,端口透传到 host.lima.internal)

## 3 个硬伤 DEFERRED 标注(用户已知情坚持,诚实记录)

### DEFERRED-1:通道脆弱(VM 重启断,无自愈)
- 宿主 Nacos ↔ CRDB 通道依赖 ssh mux port-forward PID 57226(无 launchd 托管)
- VM 重启则通道断,宿主 Nacos 写 CRDB 失败,无自动恢复机制
- 状态:本次执行未触发 VM 重启,通道可用;但生产级无自愈是硬伤

### DEFERRED-2:宿主 Nacos 裸进程无 K8s 自愈
- PID 12795 是裸 java 进程,非 K8s pod,无 readiness/liveness probe,无 restart policy
- 进程挂掉无自动拉起,需人工 `nohup` 启动
- 状态:本次执行进程稳定;但单点无自愈违反 SRE 基线

### DEFERRED-3:单点放大违反 Nacos 优化诉求
- Nacos 集群模式诉求多节点 raft + 去中心化;下沉到单宿主进程是单点放大
- 5000 CCU + 三端规模下单点 Nacos 是可用性瓶颈(注册中心宕则全服务发现瘫痪)
- 状态:违背 Nacos 集群设计初衷,仅作单节点验证环境妥协

## 产物清单(绝对路径)
- /Users/joelgong/claude-Projects/English-Study-Platform/report/nacos-cluster-services-export-20260819.json
- /Users/joelgong/claude-Projects/English-Study-Platform/report/nacos-cluster-configs-export-20260819.zip
- /Users/joelgong/claude-Projects/English-Study-Platform/report/nacos-cluster-deploy-backup.yaml
- /Users/joelgong/claude-Projects/English-Study-Platform/report/nacos-cluster-svc-backup.yaml
- /Users/joelgong/claude-Projects/English-Study-Platform/report/nacos-sink-learning-metadata-fail-evidence.txt
- /Users/joelgong/claude-Projects/English-Study-Platform/report/nacos-sink-to-host.md(本报告)

## 下一步(若要继续下沉,必须先解决)
1. 修 learning-java Dubbo provider 注册缺陷:核查 `?username=&password=` 空认证参数是否为 metadata report 5-10 根因
   (ASV 铁律:改前查 Dubbo 3.3 NacosMetadataReport.storeMetadata 对空 username 的处理,确认是移除空参数还是配独立 metadata-report.address)
2. 修 learning-java Deployment env 补齐 NACOS_ADDR/NACOS_NS(与 bff 对齐),消除配置不一致
3. learning provider 实例在集群 Nacos healthy 后,再重试 Step 2-5 下沉
4. 解决 HPA + 单节点 CPU 资源不足导致的 rolling Pending(独立于下沉,但会阻塞 rolling 验证)

## 诚实声明
- 未执行 Step 6(删集群 Nacos),集群 Nacos 保留运行,可逆性完整保留
- 下沉基础设施链路成功(ExternalName + 网络 + bff 注册),失败集中在 learning 既有 Dubbo 配置缺陷
- 回滚已完成,bff 在集群 Nacos healthy,业务恢复下沉前稳态
- learning 注册缺陷非本次下沉引入,需单独修复后才具备完成下沉的条件
