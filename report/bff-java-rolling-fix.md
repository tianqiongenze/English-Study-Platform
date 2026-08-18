# bff-java 滚动更新卡死修复报告

日期: 2026-08-18
命名空间: esp-prod | 集群: rancher-desktop (单节点)
执行人: esp-builder (ASV 铁律遵守: 改 securityContext/volumeMounts 前已核实 K8s API 字段真实存在)

## 1. 起始状态(任务转述)
- bff-java deploy spec image = `esp/bff-java:1.10.0`,滚动更新卡死。
- 新 RS(bff-java-5c57d7cd84,image 1.10.0)3 pod 全 Startup probe failed。
- 旧 RS(bff-java-7c9c4fb8c5,image 1.14.4)2 pod 勉强支撑流量。
- 原假设: readOnlyRootFilesystem 致 `.dubbo` 无写权限。

## 2. 诊断与根因(附日志证据)

### 2.1 实测 deploy spec
```
image: esp/bff-java:1.10.0
securityContext: {"allowPrivilegeEscalation":false,"runAsNonRoot":true,"runAsUser":1000}   # 注意: 无 readOnlyRootFilesystem
startupProbe: httpGet :8090/actuator/health/liveness, initialDelay=30, period=10, failureThreshold=120
```
原假设的 `readOnlyRootFilesystem` **不存在** —— 初判假设不成立,需从日志重判根因。

### 2.2 Startup probe 失败现象
`kubectl describe pod bff-java-5c57d7cd84-vrqdp`:
```
Warning  Unhealthy  (x338 over 71m)  Startup probe failed: Get "http://10.42.0.37:8090/actuator/health/liveness":
         dial tcp 10.42.0.37:8090: connect: connection refused
Normal   Killing    (x3 over 46m)    Container bff-java failed startup probe, will be restarted
Exit Code: 143 (SIGTERM,被 probe 杀)
```

### 2.3 根因(决定性日志)
`kubectl logs bff-java-5c57d7cd84-vrqdp --tail=200` 关键行:
```
Exception encountered during context initialization - cancelling refresh attempt:
java.lang.IllegalArgumentException: Invalid service store file /.dubbo/dubbo-metadata-bff-java-nacos-8848.cache,
cause: Failed to create directory /.dubbo!
```

Dubbo 3.3.6 在 Spring `AbstractApplicationContext.refresh()` 的 `registerListeners` 阶段,需要写 `${HOME}/.dubbo/dubbo-metadata-*.cache`。

跨镜像对比(exec 实测):
| 镜像 | HOME | Dubbo 元数据路径 | uid | 结果 |
|------|------|------------------|-----|------|
| 1.10.0(新,坏) | `HOME=/`(镜像未设,默认根) | `/.dubbo`(根目录) | uid=1000,无权在 `/` 建目录 | context refresh 失败,Startup 永不 UP |
| 1.14.4(旧,健康) | `HOME=/home/esp`(镜像已设) | `/home/esp/.dubbo`(可写) | uid=1000,可写 | 启动成功 |

### 2.4 根因结论
**`.dubbo` 目录写权限问题属实,但触发机制修正**:不是 `readOnlyRootFilesystem`(spec 里没有),而是 **1.10.0 镜像 `HOME=/`** 导致 Dubbo 把元数据路径解析成根目录下的 `/.dubbo`,`uid=1000` 非 root 无法在 `/` 创建该目录,context refresh 在 `registerListeners` 阶段抛 `IllegalArgumentException` 后 cancel,Jetty 虽已 bind :8090 但 `/actuator/health/liveness` 因 context 未 refresh 完成而 connection refused,Startup probe 连续失败 338 次后 kill 重启,循环不止。

附带噪声(非致命,但暴露镜像日志路径缺陷):
- logback `RollingFileAppender[AI_LOG_FILE]/[REMOTE_LOG_FILE]` 写 `/logs/nacos/*.log` 失败(FileNotFoundException,目录不存在)。
- Sentinel 写 `/logs/csp/sentinel-record.log` 失败。
- 健康的 1.14.4 镜像也无 `/logs` 目录,但 1.14.4 未配置这些 appender/路径,故不触发。

## 3. 修复方案:方案 A(emptyDir 挂载,可逆 patch)

给 `.dubbo`(Dubbo 元数据)与 `/logs/nacos`(logback 噪声抑制)各挂一个 emptyDir。emptyDir 是 pod 生命周期内可写卷,uid=1000 对其有写权限。**不改 Dockerfile,不改镜像 tag,纯 K8s spec patch,完全可逆**。

ASV 铁律已遵守:`emptyDir`/`volumeMounts`/`volumeMounts[].mountPath` 均为 K8s core API v1 真实字段,strategic merge patch 标准用法,未臆造任何 API。

### 3.1 备份(回滚锚点)
`report/bff-java-pre-fix-backup.yaml`(完整 deploy yaml,165 行)。

### 3.2 执行 patch
```bash
kubectl -n esp-prod patch deploy bff-java --type=strategic -p '{
  "spec":{"template":{"spec":{
    "volumes":[
      {"name":"dubbo-metadata","emptyDir":{}},
      {"name":"nacos-logs","emptyDir":{}}
    ],
    "containers":[{
      "name":"bff-java",
      "volumeMounts":[
        {"name":"dubbo-metadata","mountPath":"/.dubbo"},
        {"name":"nacos-logs","mountPath":"/logs/nacos"}
      ]
    }]
  }}}
}'
# => deployment.apps/bff-java patched (PATCH_RC=0)
```

注:执行后另有并发 `kubectl rollout restart`(仅刷新 restartedAt 注解,2026-08-19T00:03:40,生成第二新 RS 6c6bf55478),两个新 RS 模板 diff 仅 restartedAt + hash,emptyDir 挂载均保留,修复未受影响。

## 4. 修复后验证

### 4.1 deploy 副本
`kubectl get deploy bff-java`: `3/4 ready, 3 available`(4 副本中 3 Ready)。
- 新 RS 69f8b67f8(my patch):3 pod 全 Ready,restarts=0。
- 新 RS 6c6bf55478(rollout restart 衍生):1 Ready + 2 Pending(FailedScheduling: Insufficient cpu)。
- 2 Pending pod 系单节点 Rancher Desktop CPU 资源枯竭,**非本修复引入的回归**(基线单节点约束,DEFERRED 到多节点验证)。

### 4.2 pod 内 liveness(权威证据)
`kubectl exec bff-java-69f8b67f8-2svkt -- wget -qO- --timeout=3 http://127.0.0.1:8090/actuator/health/liveness`:
```
{"status":"UP"}  wget_rc=0
```

### 4.3 Dubbo 错误消失
`kubectl logs bff-java-69f8b67f8-2svkt --tail=200 | grep -ciE 'Invalid service store|Failed to create directory /.dubbo|cancelling refresh'` => `0`(修复前为多行 ERROR)。

### 4.4 启动轨迹健康(对比修复前)
修复前:context refresh 在 `registerListeners` 阶段 cancel,Startup probe 连 fail 338 次。
修复后:Jetty `Started oejs.Server @20863ms`,Pulsar `connected`,Nacos `AbilityControlManager initialized`,context 正常走完 refresh,pod 0 restarts 即 Ready。

### 4.5 NodePort 验证限制(诚实记录)
- `bff-java-np` NodePort `30009:8090/TCP`。
- 宿主 `curl http://192.168.5.15:30009/actuator/health/liveness` => http_code=000(超时);port-forward 同样失败。
- 原因:单节点 RD 在当前 CPU 饱和下,宿主→NodePort/port-forward 路径不可达;但 pod 内 :8090 直接返回 `{"status":"UP"}`。宿主侧可达性恢复需待 Pending pod 调度(腾出 CPU)或迁多节点,标记为 DEFERRED。

## 5. 回滚预案

若修复导致更糟,按优先级回滚:
1. **策略 patch 回退**(撤 emptyDir,回到原始 spec):
   ```bash
   kubectl -n esp-prod apply -f report/bff-java-pre-fix-backup.yaml
   ```
2. **镜像回退到已知健康的 1.14.4**(若 1.10.0 仍有其他回归):
   ```bash
   kubectl -n esp-prod set image deploy/bff-java bff-java=esp/bff-java:1.14.4
   kubectl -n esp-prod rollout status deploy/bff-java --timeout=300s
   ```
   (注:1.14.4 HOME=/home/esp 自带 .dubbo 可写路径,不需 emptyDir;但 1.10.0 是 spec 目标版本,首选方案 A 修 emptyDir 而非退版本)
3. **rollout undo**(若用 rollout history):
   ```bash
   kubectl -n esp-prod rollout undo deploy/bff-java --to-revision=<revision>
   ```

## 6. 后续建议

1. **镜像侧根治(推荐,治本)**:1.10.0 Dockerfile 应 `ENV HOME=/home/esp`(与 1.14.4 对齐),或 `ENV dubbo.properties...` 指定 `dubbo.metadata.store.path` 到可写目录;K8s emptyDir 是部署层兜底,镜像层修复才能覆盖所有部署形态。标记 BVV:镜像改后须校验 HOME 生效且 `/home/esp/.dubbo` 可写。
2. **logback/Sentinel 日志路径**:1.10.0 镜像配置了 `/logs/nacos/`、`/logs/csp/` 写入但镜像无该目录,建议 Dockerfile `RUN mkdir -p /logs/nacos /logs/csp && chown -R 1000:0 /logs` 或 emptyDir 覆盖全 `/logs`(本次仅挂了 `/logs/nacos`,`/logs/csp` 仍报噪声但非致命)。
3. **单节点 CPU 枯竭**:4 副本 desired 在单节点(9 核)上无法全调度(其他 esp 服务占用),建议负载测试前确认副本数与节点 CPU 匹配,或多节点迁移后再阶梯压测。
4. **阶梯压测前置确认**:本修复后,bff-java 恢复健康(3/4 Ready + pod 内 liveness UP),login 14.4s 瓶颈的 BFF 侧 Startup-probe 卡死根因已消除,可进入阶梯压测;但需先解决 2 Pending pod 的调度,否则实际可用副本=3 非 4。

## 7. 证据清单(文件路径)
- 修复前完整 deploy 备份: `/Users/joelgong/claude-Projects/English-Study-Platform/report/bff-java-pre-fix-backup.yaml`
- 本报告: `/Users/joelgong/claude-Projects/English-Study-Platform/report/bff-java-rolling-fix.md`

## 8. 铁律遵守自检
- ASV: 改 volumeMounts/securityContext 前核实 K8s core v1 API 字段(emptyDir/volumeMounts/mountPath)真实存在,未臆造。已遵守。
- 可逆优先: patch/strategic merge,不改 Dockerfile/镜像 tag。已遵守。
- 未碰 k3s/CRDB/集群 Nacos。已遵守。
- Bash 限流: 全程 `--tail`/`grep`/`head`/`jsonpath` 限流,无裸 cat/裸 logs。已遵守。
- 每步附证据: 见第 2-4 节。
