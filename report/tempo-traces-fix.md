# Tempo 端口冲突 + bff/gateway traces 断 修复(2026-08-19,ASV 查证)

## 状态:已修复(核心链路 OTel→Tempo 打通,bff/gateway env 已 patch)

## 端口规划表(目标架构)
| 端口 | 占用者 | 用途 | 状态 |
|---|---|---|---|
| 3200 | tempo | HTTP API/metrics | ✅ 监听 |
| 9096 | tempo | gRPC server(原默认 9095) | ✅ 改 9096,避开 loki |
| 14317 | tempo | OTLP gRPC 接收(OTel 转发目标) | ✅ 改 14317,避开 OTel 4317 |
| 14318 | tempo | OTLP HTTP 接收 | ✅ 改 14318,避开 OTel 4318 |
| 7947 | tempo | memberlist(原默认 7946) | ✅ 显式保留,不冲突 |
| 4317 | otelcol-contrib | OTLP gRPC 接收(ESP 服务上报) | ✅ OTel 网关 |
| 4318 | otelcol-contrib | OTLP HTTP 接收 | ✅ |
| 8889 | otelcol-contrib | Prometheus metrics exporter | ✅ |
| 9095 | loki-darwin(3757) | loki memberlist | 不动(共享基础设施) |

链路:`ESP 服务 → OTel(4317/4318)→ Tempo(14317)→ 存储;OTel metrics→Prometheus(8889);OTel logs→Loki(3100)`

## ASV 查证(关键,不臆造)
tempo 2.7.2 CLI flag 实测(`./tempo -help`):
- `-server.grpc-listen-port int  gRPC server listen port. (default 9095)` ← **这是 9095 冲突真凶**
- `-server.http-listen-port int  HTTP server listen port. (default 80)`
- `-memberlist.bind-port int  Port for memberlist to communicate on (default 7946)` ← memberlist 默认 7946,不是 9095

**根因修正**:之前 DEFERRED 报告把 9095 归为 "tempo memberlist 默认 9095" 是误判。ASV 查证:tempo memberlist 默认 7946(不冲突);9095 是 tempo **gRPC server** 默认端口(`server.grpc_listen_port`),与 loki memberlist 9095 冲突。

flag 是 dash-case(`-server.grpc-listen-port`),YAML 配置 key 是 snake_case(`server.grpc_listen_port`),memberlist YAML key = `memberlist.bind_port`(已合法,只是没解决 9095)。

## Step 1 诊断(只读,实测)
- 端口归属:`lsof -iTCP:..., -sTCP:LISTEN` → loki 9095 / otelcol 4317,4318,8889 / tempo 无监听(未起)
- OTel 运行进程:`./otelcol-contrib --config=otel-gateway-config.yaml`(注意:用的是简版 otel-gateway-config.yaml,非完整版 otel-config.yaml)
- OTel exporter 原 `otlp/tempo.endpoint=tempo.esp-observability.svc.cluster.local:4317`(K8s svc 名,宿主 NXDOMAIN,转发不到)
- bff-java deploy env:`OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector.esp-prod.svc.cluster.local:4317`(esp-prod 无此 svc,NXDOMAIN)
- gateway-go deploy env:`OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4317`(短名,esp-prod 解析不到 esp-observability svc)
- learning/teacher endpoint=`http://otel-collector.esp-observability.svc.cluster.local:4317`(ExternalName→宿主 OTel:4317 ✅,正确,但 OTel→Tempo 断)
- K8s svc 确认:`esp-observability/otel-collector` ExternalName→`host.lima.internal`:4317/4318/8889 ✅

## Step 2 Tempo 修复(基于 ASV 真实 key)
改 `~/observability/tempo.yaml`:
```yaml
server:
  http_listen_port: 3200
  grpc_listen_port: 9096        # ASV:原默认 9095 与 loki 冲突,改 9096
memberlist:
  bind_port: 7947              # ASV:flag -memberlist.bind-port 默认 7946,保留 7947
distributor:
  receivers:
    otlp:
      protocols:
        grpc:
          endpoint: 0.0.0.0:14317   # OTel 占 4317,Tempo 改 14317
        http:
          endpoint: 0.0.0.0:14318
```
启动:`cd ~/observability && nohup ./tempo -config.file=tempo.yaml > logs/tempo.log 2>&1 &`

验证证据:
- 端口监听:`tempo 29753 *:3200 / *:9096 / *:14317 / *:14318` 全 LISTEN ✅
- 进程存活:PID 29753 STAT=SN ✅
- 日志无 error/fatal/panic/bind fail;`msg="Tempo started"` ✅

## Step 3 OTel 转发修复
改 `~/observability/otel-gateway-config.yaml`(运行中配置)+ `otel-config.yaml`(备用):
```yaml
exporters:
  otlp/tempo:
    endpoint: localhost:14317   # 宿主 OTel → 宿主 Tempo(14317,避开 OTel 自身 4317)
    tls:
      insecure: true
```
重启:kill 旧 otelcol(PID 94159,kill 普通信号无效,用 kill -9 强制)+ 重起。

坑:旧 OTel 进程 kill 后未死,占用 8888(旧 config 的 prometheus exporter 默认 8888),致新 OTel 启动 `bind: address already in use`。解法:`kill -9 94159` 强杀后重起,端口释放。

验证证据:
- 端口:otelcol PID 32325 监听 4317/4318/8889 ✅
- 日志:`Starting GRPC server endpoint=0.0.0.0:4317` + `Everything is ready. Begin running and processing data.` ✅
- exporter 日志无 error ✅

## Step 4 bff/gateway endpoint 修复
```
kubectl -n esp-prod set env deploy/gateway-go OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector.esp-observability.svc.cluster.local:4317
kubectl -n esp-prod set env deploy/bff-java  OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector.esp-observability.svc.cluster.local:4317
```
两 deploy env updated ✅。env 已 patch 到 deploy spec,对齐 learning/teacher。

rolling restart 受限:新 pod Pending(`0/1 nodes available: 1 Insufficient cpu`),单节点资源约束,旧 pod 仍 Running(服务不中断)。env 改动已落 spec,等 CPU 释放新 pod 调度上即生效。**这是单节点资源 DEFERRED,非配置错误。**

## Step 5 端到端 traces 验证(全链路打通)
- Tempo 接收计数:`tempo_distributor_spans_received_total{tenant="single-tenant"}` 从 812 → 2595 spans(随 bff 请求涌入)✅
- Tempo search API:`curl http://localhost:3200/api/search?limit=200` 返回 200 traces,`rootServiceName` 分布:`learning-worker`(145)+ `gateway`(19)+ `emotion`(18)+ `writing`(18)
- **按 service.name tag 搜索(确凿证据)**:
  - `service.name=gateway`:5 traces ✅(gateway-go 新 pod,env 已 patch 正确 endpoint)
  - `service.name=bff`:20 traces ✅(20 次 bff 请求全部采样命中)
  - `learning-worker`/`writing`/`emotion`:均可在 Tempo 查到
- OTel exporter 日志无转发错误 ✅

### ASV service.name 厘清(关键)
bff-java 上报的 service.name 是 **`bff`** 而非 `bff-java`,由 deploy env `OTEL_SERVICE_NAME=bff` 决定(OTel SDK 标准行为:`OTEL_SERVICE_NAME` env → `otel.service.name` resource attr,优先级高于 spring application name)。按 `service.name=bff-java` 搜 0 是预期(无此 service),按 `service.name=bff` 搜 20 traces 证明 bff 链路完全打通。

### bff-java/gateway-go rolling 结果(单节点 CPU 限制已自行消化)
- gateway-go:新 pod `97756d788-jwh28` 1/1 Running,env=`http://otel-collector.esp-observability.svc.cluster.local:4317` ✅,日志 `otel tracing initialized` 无连接错误
- bff-java:rollout 完成(exit 0),新 pod `58c4d5d995-kfkgd` 1/1 Running,env 同上 ✅,日志无 OTel 报错;旧 pod 逐步 Terminating
- 期间新 pod 曾 Pending(`Insufficient cpu`),随旧 pod 释放 CPU 后自动调度上来,rolling 完成

## Grafana 可见性
Grafana 在 `:3000` 运行(PID 7732)。Tempo 数据源若已配 `http://localhost:3200`,即可在 Grafana Explore→Tempo 面板查询 `service=learning-worker` / `service=writing` 的 traces。Tempo API 已可查 50 traces。

## ASV 铁律践行
1. tempo 2.7.2 配置 key 全部用 `./tempo -help` 实测查证:`-server.grpc-listen-port`(→YAML `server.grpc_listen_port`)、`-memberlist.bind-port`(→YAML `memberlist.bind_port`),未臆造
2. 之前误判根因("tempo memberlist 默认 9095")经 ASV 查证修正:9095 实为 tempo gRPC server 默认端口
3. OTel collector exporter endpoint 改 `localhost:14317` 对齐 tempo 实际监听端口
4. 配置验证以实际启动为准(`-print-config-stderr` 非有效 flag,改用实际启动 + 端口监听 + 日志验证)

## 集成确认
- `scripts/start-host-middleware.sh`:tempo 启动用 `-config.file="$TEMPO_CFG"`(tempo.yaml),无 hardcode 9095/4317 给 tempo,脚本无需改(改的是 tempo.yaml 内容,脚本正确引用)
- `report/tempo-port-conflict-deferred.md`:已标注修复完成,不再是 DEFERRED

## 单节点约束(DEFERRED)
- bff-java/gateway-go 新 pod Pending(CPU 不足),env 已 patch 到 deploy spec,等资源释放自动生效
- 非 traces 链路配置问题,核心链路(OTel→Tempo)已通
