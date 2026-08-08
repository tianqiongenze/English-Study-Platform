# P2-4 OTel Collector DaemonSet + Gateway 双层 — 就绪 + 迁移指南

> 评审基线:教授级高工 / 大厂资深架构师。
> 状态:双层 Collector yaml + tail-sampling 配置就绪;**单节点 RD 无意义(DaemonSet 每节点一个,单节点=1 个,与现有 Deployment 等价)**,真正双层 + tail-sampling 降量需多节点。

## 一、为什么单节点不可行(已验证瓶颈)

P1-5 traceId 实测(docs/test/05-traceId实测.md)暴露:单节点资源瓶颈下,单一 otel-collector
(Deployment 1 副本)→ Tempo/Loki 导出不可靠(DeadlineExceeded + DNS fail),跨服务贯通 trace 未稳定验证。

根因:单节点所有服务 + collector + Tempo/Loki 挤在一起,collector 批处理与 Tempo ingest 争抢
CPU/内存,高负载下 OTLP 导出超时丢 trace。多节点下用双层 Collector 分流 + tail-sampling 降量可解。

## 二、双层 Collector 架构(目标)

```
[各服务 Pod]  --OTLP-->  [Collector DaemonSet(每节点1个,边缘)]
                              │  tail-sampling(只保留 10% + 全部 ERROR span)+ 批处理
                              ▼
                        [Collector Gateway(集群1个,中心)]
                              │  按 backend 分流
                              ├──> Tempo(traces)
                              ├──> Loki(logs)
                              └──> Prometheus(metrics)
```

- **DaemonSet(边缘层)**:每节点一个,就近接收本节点服务 OTLP,做 tail-sampling(降量)+ 本地批处理。
  降量是关键:全量 trace 在大规模下撑爆 Tempo,采 10% + 全部 ERROR + 全量慢查询(span>1s)。
- **Gateway(中心层)**:集群 1 个(高可用 2 副本),接收所有 DaemonSet 的流,按 backend 分流到 Tempo/Loki/Prometheus。
  集中管理后端连接,后端故障不影响边缘接收。

## 三、当前单节点配置(已部署,esp-observability ns)

现有 `otel-collector` Deployment 1 副本,config:otlp 接收 → resource + batch 处理 →
otlp/tempo + loki + prometheus 导出。**无 tail-sampling,全量导出**。单节点下这就是瓶颈源。

## 四、就绪产物

1. **deploy/k8s/otel-collector-daemonset.yaml**(本项新增)— DaemonSet 边缘层 + tail-sampling。
2. **deploy/k8s/otel-collector-gateway.yaml**(本项新增)— Gateway 中心层。
3. **docs/design/p2-4-otel-collector-tiering.md**(本文件)— 迁移 runbook。

## 五、多节点启用步骤(runbook)

### 5.1 部署顺序(先 Gateway 后 DaemonSet)
```bash
# 1. 中心 Gateway(先起,接收边缘流)
kubectl apply -f deploy/k8s/otel-collector-gateway.yaml
kubectl -n esp-observability rollout status deploy/otel-collector-gateway

# 2. 边缘 DaemonSet(每节点一个,改各服务 OTLP endpoint 指向本节点 DaemonSet)
kubectl apply -f deploy/k8s/otel-collector-daemonset.yaml
kubectl -n esp-observability rollout status ds/otel-collector-daemonset
```

### 5.2 各服务 OTLP endpoint 切换
各服务 env `OTEL_EXPORTER_OTLP_ENDPOINT` 从中心 collector 改为 DaemonSet:
```yaml
# 原(单层):
OTEL_EXPORTER_OTLP_ENDPOINT: http://otel-collector.esp-observability.svc.cluster.local:4317
# 改(双层,走本节点 DaemonSet):
OTEL_EXPORTER_OTLP_ENDPOINT: http://otel-collector-daemonset.esp-observability.svc.cluster.local:4317
# 或用 Downward API 取本节点 IP(更稳):
# OTEL_EXPORTER_OTLP_ENDPOINT: http://$(NODE_IP):4317
```

### 5.3 验证 trace 贯通
```bash
# 1. 触发一个跨服务请求(gateway → learning)
kubectl -n esp-prod exec deploy/gateway-go -- curl -s http://learning-java/api/scenarios

# 2. 查 Tempo 是否有端到端 trace(多 service span 串成一条)
kubectl -n esp-observability port-forward svc/tempo 3200:3200 &
curl -s http://localhost:3200/api/search?q=service.name=gateway-go | jq '.traces | length'

# 3. 确认 tail-sampling 生效:trace 总量应降到 ~10%(+ ERROR + 慢查询全留)
```

### 5.4 回退
若双层不稳,各服务 endpoint 改回单层 `otel-collector`,DaemonSet scale 0。

## 六、tail-sampling 策略(边缘层)

| 采样规则 | 比例 | 理由 |
|---|---|---|
| 常规请求 | 10% | 大规模下全量撑爆 Tempo,10% 足够排查 |
| ERROR span | 100% | 错误必须全留,排查根因 |
| 慢查询(span>1s) | 100% | 性能问题排查必须 |
| DB 写操作 span | 100% | 写正确性排查 |

> tail-sampling 需 collector 在内存缓存 span 尾部(默认 30s)再决策,边缘层内存需预留
> (DaemonSet 内存 limit ≥512Mi)。大规模下可配 `decision_cache` 避免重复决策。

## 七、单节点 vs 多节点

| 维度 | 单节点 RD(当前) | 多节点生产(目标) |
|---|---|---|
| Collector 形态 | Deployment 1 副本(单层) | DaemonSet(边缘)+ Gateway(中心)双层 |
| tail-sampling | 无(全量,单节点资源勉强) | 有(10% + ERROR + 慢查询,降量) |
| trace 贯通 | 不稳定(DeadlineExceeded) | 稳定(边缘就近接收 + 中心分流) |
| 后端隔离 | collector 与 Tempo 同节点争抢 | DaemonSet 就近,Gateway 专管后端 |

**单节点 RD**:保持现有单层 collector(资源勉强够,trace 贯通不稳定但属环境约束)。
双层 yaml 已就绪,迁移多节点后按 §五 启用,无需改服务代码(只改 OTLP endpoint env)。

## 八、风险与权衡

- **tail-sampling 丢 trace**:采 10% 会丢 90% 常规 trace,排查偶发问题可能缺数据。
  缓解:ERROR + 慢查询 100% 留;重要业务 span 打特殊属性(如 `trace.sample=true`)强制采。
- **DaemonSet 资源**:每节点一个 collector,占节点资源(DaemonSet 模式天然水平扩展,资源可控)。
- **Gateway 单点**:Gateway 2 副本 + 反亲和,避免单点;后端故障时 Gateway 缓冲(批处理 + 重试)。
