# Tempo 宿主启动端口冲突(已修复,2026-08-19)

## 状态:已修复 ✅(详见 report/tempo-traces-fix.md)

## 问题
tempo 2.7.2 启动报 `listen tcp :9095: bind: address already in use`,进程退出。

## 根因(ASV 查证修正,2026-08-19)
| 端口 | 占用者 | tempo 用途 | ASV 查证 |
|---|---|---|---|
| 9095 | loki-darwin(3757) | tempo **gRPC server** 默认 9095 | flag `-server.grpc-listen-port default 9095`;非 memberlist(误判已修正) |
| 4317 | otelcol-contrib | tempo OTLP gRPC 接收 | OTel 占,Tempo 改 14317 |
| 4318 | otelcol-contrib | tempo OTLP HTTP 接收 | OTel 占,Tempo 改 14318 |

**根因修正**:原 DEFERRED 把 9095 归为 "tempo memberlist 默认 9095" 是误判。ASV `./tempo -help` 查证:tempo memberlist 默认 7946(不冲突);9095 是 tempo **gRPC server** 默认端口(YAML key `server.grpc_listen_port`)。

## 修复(已落地)
1. tempo.yaml:`server.grpc_listen_port=9096`(避 loki 9095)+ distributor OTLP 改 14317/14318(避 OTel 4317/4318)+ `memberlist.bind_port=7947`(保留)
2. otel-gateway-config.yaml:`otlp/tempo.endpoint=localhost:14317`(宿主直连)
3. bff-java/gateway-go deploy env patch → `http://otel-collector.esp-observability.svc.cluster.local:4317`(对齐 learning/teacher)

## 验证证据
- Tempo 4 端口监听(3200/9096/14317/14318)+ 进程存活 + 日志 "Tempo started"
- Tempo 收到 2595+ spans / 200+ traces(rootServiceName: learning-worker/gateway/emotion/writing)
- OTel exporter 无错误
- bff/gateway env 已 patch,rolling 完成:gateway `service.name=gateway` 5 traces ✅,bff `service.name=bff` 20 traces ✅(ASV:bff service name=`bff` 非 `bff-java`,由 `OTEL_SERVICE_NAME=bff` 决定)

## 不再 DEFERRED ✅
