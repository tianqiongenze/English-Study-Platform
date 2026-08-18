# ESP 宿主中间件混沌测试计划

> 最后更新: 2026-08-18

## 1. 测试目标

验证 **macOS 宿主机上运行的中间件故障**对 K8s 集群内 ESP 业务服务的影响范围和恢复能力。

**为什么需要这个测试**：2026-08-18 会话中，Nacos 迁移到 CRDB 存储失败导致 Nacos 崩溃，**全部 Java 服务(bff/learning/teacher) CrashLoopBackOff**，暴露了 Nacos 单点故障的严重性。需要系统性验证所有宿主中间件的故障影响。

## 2. 宿主中间件清单

| 中间件 | 端口 | 用途 | ESP 服务依赖 |
|--------|------|------|-------------|
| **Nacos** | 8848 | 服务注册/配置中心 | bff-java, learning-java, teacher-java(Dubbo config-center), admin-go(Governance API), Sentinel 规则 |
| **Prometheus** | 9090 | 指标采集+告警 | Grafana(dashboards), admin(/metrics/query), alertmanager(forwarding) |
| **Loki** | 3100 | 日志聚合 | Grafana(logs panel) |
| **Alertmanager** | 9093 | 告警路由 | Prometheus(alerts→通知) |
| **OTel Collector** | 4317/4318/8889 | 遥测网关 | Prometheus(metrics), Grafana(traces), Loki(logs) |
| **Grafana** | 3000 | 可视化面板 | 无下游(终端消费者) |
| **Tempo** | 3200 | 分布式追踪存储 | Grafana(traces panel) |

## 3. 故障影响矩阵

| 故障中间件 | 影响的服务 | 预期行为 | 严重度 | 恢复方式 |
|-----------|-----------|---------|--------|---------|
| **Nacos kill** | bff-java, learning-java, teacher-java → CrashLoopBackOff | Dubbo config-center fail-fast=true → 启动失败 | **P0 致命** | 重启 Nacos + Pod 自动恢复 |
| **Nacos kill** | admin-go → Governance API 返回空 | 静默降级，不崩溃 | P2 低 | 重启 Nacos |
| **Prometheus kill** | Grafana(dashboards 404), admin(/metrics 空) | 监控数据中断，业务不受影响 | P1 中 | 重启 Prometheus，数据重采 |
| **Loki kill** | Grafana(logs panel 404) | 日志查询不可用 | P2 低 | 重启 Loki |
| **Alertmanager kill** | Prometheus(alerts 丢弃) | 告警不通知，业务不受影响 | P2 低 | 重启 Alertmanager |
| **OTel Collector kill** | Prometheus(metrics 空), Grafana(traces 断) | 指标/追踪断流，日志可能断 | P1 中 | 重启 Collector |
| **Grafana kill** | 无(监控界面不可用) | 运维失去可视化 | P3 低 | 重启 Grafana |
| **Tempo kill** | Grafana(traces panel 404) | 追踪查询不可用 | P2 低 | 重启 Tempo |

## 4. 测试用例清单(21 个场景)

### 4.1 Nacos 故障(3 场景) — P0 最高优先

| ID | 场景 | 操作 | 验证点 |
|----|------|------|--------|
| H-01 | Nacos 崩溃 | `kill -9 $(pidof nacos)` | bff/learning/teacher 是否 CrashLoopBackOff？探针行为？ |
| H-02 | Nacos 挂起 | `kill -STOP $(pidof nacos)` | Dubbo 超时行为？服务是否仍能处理存量请求？ |
| H-03 | Nacos 恢复 | `kill -CONT` + 等待就绪 | 服务是否自动重连恢复？需重启 Pod？ |

### 4.2 Prometheus 故障(3 场景) — P1

| ID | 场景 | 操作 | 验证点 |
|----|------|------|--------|
| H-04 | Prometheus 崩溃 | kill -9 | Grafana dashboards 是否 404？admin /metrics 是否空？ |
| H-05 | Prometheus 挂起 | kill -STOP | 采集是否超时？Grafana 是否显示"no data"？ |
| H-06 | Prometheus 恢复 | kill -CONT | 数据是否自动恢复？告警规则是否重新加载？ |

### 4.3 Loki 故障(3 场景) — P2

| ID | 场景 | 操作 | 验证点 |
|----|------|------|--------|
| H-07 | Loki 崩溃 | kill -9 | Grafana logs panel 是否 404？ |
| H-08 | Loki 挂起 | kill -STOP | 日志查询超时行为？ |
| H-09 | Loki 恢复 | kill -CONT | 日志查询是否恢复？历史日志是否丢失？ |

### 4.4 Alertmanager 故障(3 场景) — P2

| ID | 场景 | 操作 | 验证点 |
|----|------|------|--------|
| H-10 | Alertmanager 崩溃 | kill -9 | Prometheus alerting 是否报错？业务不受影响？ |
| H-11 | Alertmanager 挂起 | kill -STOP | 告警是否积压？恢复后是否批量发送？ |
| H-12 | Alertmanager 恢复 | kill -CONT | 告警路由是否恢复？ |

### 4.5 OTel Collector 故障(3 场景) — P1

| ID | 场景 | 操作 | 验证点 |
|----|------|------|--------|
| H-13 | OTel Collector 崩溃 | kill -9 | Prometheus metrics 是否中断？traces/logs 是否断流？ |
| H-14 | OTel Collector 挂起 | kill -STOP | 指标采集超时？OTLP 发送失败日志？ |
| H-15 | OTel Collector 恢复 | kill -CONT | 指标/追踪是否恢复？数据是否丢失窗口？ |

### 4.6 Grafana 故障(3 场景) — P3

| ID | 场景 | 操作 | 验证点 |
|----|------|------|--------|
| H-16 | Grafana 崩溃 | kill -9 | 业务服务不受影响？dashboard 不可用？ |
| H-17 | Grafana 挂起 | kill -STOP | 前端超时行为？ |
| H-18 | Grafana 恢复 | kill -CONT | dashboard 是否恢复？登录状态是否保留？ |

### 4.7 Tempo 故障(3 场景) — P2

| ID | 场景 | 操作 | 验证点 |
|----|------|------|--------|
| H-19 | Tempo 崩溃 | kill -9 | traces 查询 404？Prometheus 是否报错？ |
| H-20 | Tempo 挂起 | kill -STOP | OTLP 发送超时？ |
| H-21 | Tempo 恢复 | kill -CONT | 追踪查询是否恢复？历史 traces 是否丢失？ |

## 5. 执行步骤

### 前置条件
```bash
# 1. 确认所有中间件运行中
./scripts/status-host-middleware.sh

# 2. 确认 K8s 服务正常
kubectl -n esp-prod get pods | grep -v Running | head -5 || echo "All pods Running"

# 3. 备份 Nacos 配置(关键!)
cp ~/nacos/nacos/conf/application.properties ~/nacos/nacos/conf/application.properties.pre-chaos
```

### 执行命令
```bash
# 单个测试
./scripts/chaos/host-middleware-chaos.sh --target nacos --action kill --yes
# 等待观察...
./scripts/chaos/host-middleware-chaos.sh --target nacos --action resume --yes

# 批量测试(按优先级)
for svc in nacos prometheus otel-collector loki alertmanager grafana tempo; do
    echo "=== Testing $svc ==="
    ./scripts/chaos/host-middleware-chaos.sh --target $svc --action kill --yes
    sleep 30
    ./scripts/chaos/host-middleware-chaos.sh --target $svc --action resume --yes
    sleep 15
done
```

### 恢复验证
每个测试后必须验证:
1. 中间件进程存活
2. 端口监听正常
3. K8s Pod 全部 Running
4. Gateway :30080 和 BFF :30009 健康检查通过
5. Nacos 服务注册表完整

## 6. 已知风险

| 风险 | 缓解措施 |
|------|---------|
| Nacos kill 导致 Java 全挂 | 测试前备份配置；恢复后 Pod 自动重启可能需要 90s(JVM 启动) |
| Prometheus kill 丢数据 | TSDB 有 WAL，kill 后重启数据不丢（除最后几秒） |
| OTel Collector kill 影响面广 | 它是 traces/logs/metrics 的汇聚点，kill 影响最大 |
| macOS 重启清理 /tmp/ 数据 | 已迁移到 ~/observability/data/，不受影响 |

## 7. 改进项(测试后)

根据测试结果，可能需要:
1. **Nacos 高可用**: 3 节点集群 + MySQL/CRDB 存储(当前单点)
2. **Java 服务 Nacos 降级**: fail-fast: false + watch.enabled: false
3. **Sentinel 规则本地缓存**: 避免 Nacos 挂时限流规则丢失
4. **Prometheus 数据持久化**: 已迁移到 ~/observability/data/
5. **混沌测试 CI 集成**: 每次部署前自动跑关键场景(H-01, H-04, H-13)
