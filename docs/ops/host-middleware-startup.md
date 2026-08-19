# ESP 宿主中间件运维手册

> 中间件从 K8s 迁移到 macOS 宿主机(Rancher Desktop Lima VM)后的完整运维指南。
> 最后更新: 2026-08-19

---

## 0. ⚠️ 项目运行前提(必读,2026-08-19 确立)

**宿主中间件必须先启动,否则 ESP 全部 Java 服务 CrashLoopBackOff。**

下沉到宿主的中间件(Nacos / Prometheus / Loki / Alertmanager / OTel Collector / Grafana / Tempo)是 ESP 服务运行期**硬依赖**:

- **Nacos:8848**(最关键):BFF / learning-java / teacher-java 等所有 Java 服务通过 Dubbo `NacosDynamicConfiguration` 启动时连 Nacos 做配置中心 + 注册中心。**Nacos 不起 → Dubbo `Failed to create nacos config service client: server status check failed` → 全 Java 服务 CrashLoop**。
- **CRDB:26257**(经 launchd ssh tunnel 自愈到集群 `esp-crdb`):Nacos 自身的外部存储 + ESP 部分服务的数据源。通道断 → Nacos 起不来或写不进。
- **Redis Cluster:6379**(集群内 `redis-cluster-new`,经 `redis-cluster-selfheal.sh` 自愈):ESP 会话态 + `esp:ctx:*` 跨会话锚点。VM 重启后 nodes.conf 旧 IP 致 `cluster_state:fail` → CLUSTERDOWN 拒写。
- **OTel:4317/4318**:ESP 服务 trace/metrics 上报端点,断则服务启动可能卡(取决于配置是否 required)。

### 每次开机 / VM 重启后的标准启动顺序

```bash
# 1. 启动全部宿主中间件(含 Nacos,内部含 Redis selfheal [8/8] + CRDB 通道 launchd 自愈)
bash scripts/start-host-middleware.sh

# 2. 验证 Nacos 真正 ready(不只是端口在,要 metrics UP)
curl -s http://localhost:8848/nacos/v1/ns/operator/metrics   # 期望 {"status":"UP"}
curl -s http://localhost:8848/nacos/v1/console/health/liveness  # 期望 HTTP 200

# 3. 验证 Redis Cluster 自愈成功(VM 重启后必跑,脚本幂等已集成在 start [8/8])
kubectl -n ekb-m2-aws-us-east-1 exec redis-cluster-new-0 -- redis-cli -c CLUSTER INFO | grep cluster_state  # 期望 cluster_state:ok

# 4. 验证 CRDB 通道(nacos 写 crdb 的路径)
nc -z 127.0.0.1 26257 && echo "CRDB channel OK"

# 5. 中间件全绿后,再重启卡死的 Java 服务(否则重启无用,会再次 CrashLoop)
kubectl -n esp-prod rollout restart deploy/bff-java deploy/learning-java deploy/teacher-java
```

### 反模式(禁止)

- ❌ **先 `kubectl rollout restart` Java 服务,再起中间件**:服务会立即 CrashLoop,Nacos 未起时 Dubbo 配置客户端构建失败,滚动更新永远卡在 MinimumReplicasUnavailable。
- ❌ **只起 Nacos 不验 `metrics UP`**:Tomcat 端口 8848 监听 ≠ Nacos 就绪(Nacos 还在加载 CRDB schema),Java 服务此时连会 `server status check failed`。必须 `{"status":"UP"}`。
- ❌ **VM 重启后不跑 Redis selfheal**:nodes.conf 持久化旧 PodIP,`cluster_state:fail`,所有 `esp:ctx:*` 锚点读不了,跨会话恢复失明。
- ❌ **停宿主中间件时不重启依赖它的 Java 服务**:Nacos 一断,Dubbo 靠本地缓存保活一段时间后会失效,Java 服务静默半死。

---

## 1. 架构总览

```
┌─────────────────────────────────────────────────────────────┐
│ macOS 宿主机 (~/observability/ + ~/nacos/)                   │
│                                                              │
│  Nacos:8848  Prometheus:9090  Loki:3100  Alertmanager:9093  │
│  OTel Collector:4317/4318/8889  Grafana:3000                │
│  Tempo:3200 (二进制损坏，待修复)                              │
└──────────┬───────────────────────────────────────────────────┘
           │
           │  K8s DNS: ExternalName → host.lima.internal:port
           ▼
┌─────────────────────────────────────────────────────────────┐
│ esp-observability namespace (K8s)                           │
│  prometheus → host.lima.internal:9090                       │
│  loki → host.lima.internal:3100                             │
│  alertmanager → host.lima.internal:9093                     │
│  grafana → host.lima.internal:3000                          │
│  otel-collector → host.lima.internal:4317/4318/8889         │
│  tempo → host.lima.internal:3200 (待修复)                   │
│  otel-collector-daemonset (仍在 K8s)                        │
└─────────────────────────────────────────────────────────────┘
```

Pod 通过 K8s CoreDNS 解析 ExternalName Service → `host.lima.internal` → 宿主进程，双向可达。

---

## 2. 组件清单

| 组件 | 端口 | 配置文件 | 数据目录 | 用途 |
|------|------|----------|----------|------|
| **Nacos** | 8848 | `~/nacos/nacos/conf/application.properties` | Derby 内嵌 | 服务注册/配置中心 |
| **Prometheus** | 9090 | `~/observability/prometheus.yml` | `~/observability/prometheus-data/` | 指标采集+告警(7d保留) |
| **Loki** | 3100 | `~/observability/loki-fixed.yaml` | `~/observability/loki-data/` | 日志聚合 |
| **Alertmanager** | 9093 | `~/observability/alertmanager.yml` | `~/observability/alertmanager-data/` | 告警路由 |
| **OTel Collector** | 4317(gRPC) / 4318(HTTP) / 8889 | `~/observability/otel-gateway-config.yaml` | — | 遥测收集网关 |
| **Grafana** | 3000 | `~/observability/grafana.ini` + `grafana-datasources.yaml` | `~/observability/grafana/data/` | 可视化面板 |
| **Tempo** | 3200 | `~/observability/tempo.yaml` | — | 分布式追踪存储 |

### 配套管理脚本

| 脚本 | 用途 |
|------|------|
| `scripts/start-host-middleware.sh` | 按依赖顺序启动全部组件 |
| `scripts/stop-host-middleware.sh` | 按反依赖顺序停止，graceful SIGTERM → SIGKILL |
| `scripts/status-host-middleware.sh` | 检查进程/端口/健康检查，输出彩色状态表 |

---

## 3. 快速启动

```bash
# 一键启动全部宿主中间件
./scripts/start-host-middleware.sh
```

**按依赖顺序**:
1. Nacos (standalone 模式)
2. Prometheus (配置已含 10 条告警规则)
3. Loki
4. Alertmanager
5. OTel Collector (gateway)
6. ~~Tempo~~ — 二进制损坏，脚本会自动跳过并打印 WARNING
7. Grafana

每个组件启动前检查端口是否已被占用(防重复启动)，启动后检查进程存活+端口监听。

### 检查状态

```bash
./scripts/status-host-middleware.sh
```

### 停止全部

```bash
./scripts/stop-host-middleware.sh
```

---

## 4. Nacos 运维

### 启动
```bash
cd ~/nacos/nacos/bin
./startup.sh -m standalone
```

### 停止
```bash
cd ~/nacos/nacos/bin
./shutdown.sh
```

### 访问控制台
- URL: http://localhost:8848/nacos/
- 默认用户: nacos/nacos

### 注意事项
- **存储模式**: Derby 内嵌(非 MySQL)，standalone 单机模式。重启不丢数据(持久化到 `~/nacos/nacos/data/`)，但无高可用
- **Sentinel 规则**: bff-java 的限流规则(nacos: `bff-java-flow-rules`)存在 Nacos 配置中心
- **K8s 可达**: esp-prod 中 `nacos` ExternalName → `nacos.ekb-m2-aws-us-east-1.svc.cluster.local`(跨 ns，也连的是宿主 Nacos 或基础 Nacos)

---

## 5. Prometheus 运维

### 启动
```bash
cd ~/observability
nohup ./prometheus \
  --config.file=$HOME/observability/prometheus.yml \
  --storage.tsdb.path=$HOME/observability/prometheus-data \
  --storage.tsdb.retention.time=7d \
  --web.enable-lifecycle \
  --web.enable-rule-api \
  --web.listen-address=:9090 \
  > logs/prometheus.log 2>&1 &
```

### 热加载配置(无需重启)
```bash
curl -X POST http://localhost:9090/-/reload
```

### 停止
```bash
kill $(cat ~/observability/pids/prometheus.pid)
```

### 抓取目标
- **OTel Collector 指标出口**: `otel-collector.esp-observability.svc.cluster.local:8889`
- **K8s Pod 指标**: `kubernetes_sd_configs` 自动发现 esp-optimization Pod
- **K8s 节点指标**: `kubernetes_sd_configs` 自动发现

### 告警规则 (rules.yml)
10 条规则覆盖:
1. `ServiceDown` — Pod 不健康 2 分钟
2. `HighErrorRate` — 5xx > 5% 持续 5 分钟
3. `HighLatencyP99` — P99 > 2s 持续 5 分钟
4. `HighLatencyP95Gateway` — Gateway P95 > 1s 持续 5 分钟
5. `PodRestart` — 10 分钟内重启 > 3 次
6. `PodCrashLoop` — 重启次数 > 5
7. `ErrorBudgetBurn` — 5 分钟内错误率 > 10 倍 SLO 消耗
8. `HighCPUSaturation` — CPU 使用 > 80%
9. `HighMemorySaturation` — 内存使用 > 85%
10. `TiDBConnectionPoolSaturation` — 连接池使用 > 80%

---

## 6. Loki 运维

### 启动
```bash
cd ~/observability
nohup ./loki \
  -config.file=$HOME/observability/loki-fixed.yaml \
  -config.expand-env=true \
  > logs/loki.log 2>&1 &
```

### 健康检查
```bash
curl http://localhost:3100/ready
```

### 注意事项
- 配置 `localhost:3100`(非 host.lima.internal)
- 单节点 inmemory ring，无复制
- Schema v13 (TSDB) 存储

---

## 7. Alertmanager 运维

### 启动
```bash
cd ~/observability
nohup ./alertmanager \
  --config.file=$HOME/observability/alertmanager.yml \
  --storage.path=$HOME/observability/alertmanager-data \
  --web.listen-address=:9093 \
  > logs/alertmanager.log 2>&1 &
```

### 注意事项
- **当前告警无出口**: `webhook_configs` 全注释，告警只存本地不通知。需要配置 webhook/email/Slack 才能接收告警通知
- Prometheus 通过 `alertmanager.esp-observability.svc.cluster.local:9093` 发送告警到此服务

---

## 8. OTel Collector 运维

### 启动
```bash
cd ~/observability
nohup ./otelcol \
  --config=$HOME/observability/otel-gateway-config.yaml \
  > logs/otel-collector.log 2>&1 &
```

### 三路导出
| 信号 | 导出目标 | 说明 |
|------|----------|------|
| Traces | `tempo.esp-observability.svc.cluster.local:4317` | **Tempo 待修复，traces 导出会失败** |
| Logs | `loki.esp-observability.svc.cluster.local:3100` | K8s DNS → ExternalName → 宿主 Loki |
| Metrics | `localhost:8889` (Prometheus 拉取) | Prometheus 已配置抓取 8889 |

### 注意事项
- K8s 中仍有 `otel-collector-daemonset`(DaemonSet,节点级采集)
- 收集的自定义 metric: `esp_http_request_duration_milliseconds`, `esp_http_requests_total` 等

---

## 9. Grafana 运维

### 启动
```bash
cd ~/observability
nohup ./grafana/bin/grafana \
  -homepath=$HOME/observability/grafana \
  -config=$HOME/observability/grafana.ini \
  cfg:paths.data=$HOME/observability/grafana/data \
  cfg:paths.logs=$HOME/observability/grafana/logs \
  > logs/grafana.log 2>&1 &
```

### 访问面板
- URL: http://localhost:3000
- 匿名访问已启用(viewer 权限)

### 配置的数据源
| 数据源 | 地址 | 类型 |
|--------|------|------|
| Prometheus | `prometheus.esp-observability.svc.cluster.local:9090` | 基础设施/K8s |
| Loki | `loki.esp-observability.svc.cluster.local:3100` | 日志 |
| Tempo | `tempo.esp-observability.svc.cluster.local:3200` | **待修复** |

### 已有仪表盘
- 集群资源监控
- 基础设施监控
- 微服务监控

---

## 10. Tempo (分布式追踪) — 待修复

**状态: 二进制损坏(仅 9 字节文本)，无法运行。**

### 修复方案
```bash
# 下载 macOS arm64 正确二进制
cd ~/observability
curl -L -o tempo https://github.com/grafana/tempo/releases/download/v2.7.2/tempo_2.7.2_darwin_arm64
chmod +x tempo

# 启动
nohup ./tempo \
  -config.file=$HOME/observability/tempo.yaml \
  > logs/tempo.log 2>&1 &
```

### 配置要点 (tempo.yaml)
- 端口: 3200 (HTTP + gRPC OTLP 接收)
- 存储: 本地文件 `/var/tempo/blocks` + WAL `/var/tempo/wal`
- Ingester: max_block_duration 5m

---

## 11. K8s 集成: ExternalName Services

宿主中间件通过 `esp-optimization` 中的 ExternalName Service 暴露给 K8s pod:

```yaml
# 示例: prometheus 的 ExternalName Service
apiVersion: v1
kind: Service
metadata:
  name: prometheus
  namespace: esp-observability
spec:
  type: ExternalName
  externalName: host.lima.internal
  ports:
    - port: 9090
```

Pod 内通过 `prometheus.esp-optimization.svc.cluster.local:9090` 访问宿主 Prometheus。

**K8s 内地址解析路径**:
```
Pod → CoreDNS → prometheus.esp-optimization.svc.cluster.local
    → ExternalName: host.lima.internal
    → Lima VM 网络 → macOS 宿主进程
```

---

## 12. 已知问题与待修复

| # | 问题 | 严重度 | 说明 |
|---|------|--------|------|
| 1 | **Tempo 二进制损坏** | 高 | ~/observability/tempo 仅 9 字节，需重新下载 Grafana Tempo macOS arm64 二进制 |
| 2 | **Prometheus rule_files 路径** | 中 | 已修复: `/etc/prometheus/rules.yml` → `$HOME/observability/rules.yml` |
| 3 | **Prometheus kubernetes_sd_configs 需 K8s API 认证** | 高 | 宿主机 Prometheus 执行 `kubernetes_sd_configs` 需 ServiceAccount token 或 kubeconfig，原 esp-observability ns 的 SA 已随 Deployment 删除 |
| 4 | **数据持久化在 /tmp/** | 中 | Prometheus/Loki/Alertmanager/Grafana 数据在 `/tmp/`，macOS 重启会清理。生产环境需迁移到持久目录 |
| 5 | **Alertmanager 告警无出口** | 低 | webhook_configs 全注释，需配置通知渠道(邮件/webhook) |
| 6 | **K8s 清理** | 低 | `10-observability.yaml` 中仍有 Prometheus K8s Deployment 定义(已停)，与 ExternalName 冲突。建议清理 |

---

## 13. 多节点迁移注意事项

从单节点迁移到多节点时，宿主中间件需重新规划：

1. **Prometheus**: 改为 K8s Deployment(带 PVC)或使用 Thanos/Federation 联邦
2. **Nacos**: 切 MySQL 存储 + 3 节点集群模式
3. **Loki**: 分布式模式(ingester + distributor + querier)
4. **Alertmanager**: 集群模式(mesh peer)
5. **OTel Collector**: 保持 DaemonSet + Gateway 分离架构
6. **Grafana**: 使用 PV 持久化数据源和仪表盘
7. **Tempo**: 微服务模式或 single-binary + S3 backend
