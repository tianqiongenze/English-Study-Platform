# P2-3 分布式 Locust 多节点 5000 CCU 重测 — 就绪 + 迁移指南

> 评审基线:教授级高工 / 大厂资深架构师。
> 状态:locustfile + 分布式 helm 模板就绪;**单节点 RD 物理上无法跑多节点 Locust**,真正 5000 CCU 重测需多节点服务器。

## 一、为什么单节点不可行(已验证瓶颈)

2026-08-02 单节点 5000 CCU 压测(架构评分 §三)暴露:
1. 压测机(locust-runner pod)与被测系统同节点,测的是"单节点混合上限"非纯系统上限。
2. 单节点资源不足以同时支撑被测 7 服务 + locust + 监控栈,5000 CCU 下 writing 503/teacher 503 频发,
   根因是单节点 CPU/内存瓶颈 + AI 推理长尾,非设计缺陷。
3. **结论**:5000 CCU SLO(G-01)在单节点环境物理不可达,必须分布式 Locust master+workers 独立节点。

## 二、分布式 Locust 架构(目标)

```
[Locust Master]  ← Web UI + 调度   (独立节点或集群内 1 pod)
      │
      ├── [Worker 1]  ← N 个并发用户   (独立节点)
      ├── [Worker 2]
      ├── [Worker 3]
      └── [Worker N]
      │
      ▼
[Traefik Ingress → gateway-go → 下游服务]  (被测集群,独立节点组)
```

- Master:调度 + 聚合统计,不产生负载。
- Workers:实际产生并发用户,按 `--worker` 模式连 Master。
- 被测集群与压测集群**物理隔离**(不同节点),避免压测吃被测资源。

## 三、就绪产物(deploy/locust/)

1. **locustfile.py** — 已有,5000 CCU 场景(scenarios + sessions + writing + review)。
2. **deploy/locust/distributed-locust.yaml**(本项新增)— master + worker Deployment 模板。
3. **deploy/locust/README.md** — 更新多节点执行 runbook。

## 四、多节点启用步骤(runbook)

### 4.1 前置:多节点集群就绪
- 被测集群:≥3 worker 节点(learning/teacher/writing 分布部署,HPA 可水平扩)。
- 压测集群:≥2 独立节点(专跑 locust worker,与被测节点不重叠),或独立 K8s 集群。
- 用 `kubectl taint` 把 locust worker 调度到压测专用节点,避免与被测服务混部。

### 4.2 部署分布式 Locust
```bash
# 1. master(1 个)
kubectl apply -f deploy/locust/distributed-locust.yaml   # 含 master + worker 模板

# 2. 按目标 CCU 扩 worker:5000 CCU / 每 worker 1000 CCU ≈ 5 workers
kubectl -n esp-loadtest scale deploy/locust-worker --replicas=5

# 3. 等 workers 连上 master
kubectl -n esp-loadtest logs deploy/locust-master --tail=20 | grep "worker"
```

### 4.3 执行 5000 CCU 压测
```bash
# master 无头模式触发(无需 Web UI)
kubectl -n esp-loadtest exec deploy/locust-master -- locust \
  -f /mnt/locust/locustfile.py \
  --headless -u 5000 -r 50 -t 300s \
  --host http://gateway-go.esp-prod.svc.cluster.local \
  --csv /mnt/locust/report/ccu5000-distributed
# -u 5000 总用户,-r 50 每秒新增 50(100s 爬坡到 5000),-t 300s 持续 5 分钟
```

### 4.4 SLO 验收
| SLO | 目标 | 单节点实测 | 多节点目标 |
|---|---|---|---|
| G-01 | 5000 CCU | ❌ 533 rps(单节点) | ≥1000 rps |
| G-04 | 写作 50rps P95<2s | ❌ P95 73s | P95<2s |
| G-06 | 学情 5000 ev/s | ❌ 277 ev/s | ≥5000 ev/s |

### 4.5 结果落盘
```bash
kubectl -n esp-loadtest cp deploy/locust-master:/mnt/locust/report/ report/ccu5000-distributed/
# 更新 docs/test/03-性能与压测报告.md 多节点实测列
```

## 五、单节点就绪验证(已做)

- locustfile.py 语法校验通过(单节点 1000 CCU tierA2 已跑,790 rps/P95 1.3s/失败率 0.001%)。
- distributed-locust.yaml 模板就绪,单节点不 apply(资源不足)。
- 迁移到多节点后按 §四 执行,无需改 locustfile。

## 六、风险与权衡

- **worker 数量**:每 worker 1000 CCU 是经验值,实际按 worker 节点 CPU 调(4 核节点约 1000-1500 CCU)。
- **被测入口**:压测打 Traefik Ingress(生产路径)还是 gateway-go ClusterIP(绕 Ingress)?
  生产验收打 Ingress;纯服务上限打 ClusterIP。本 runbook 默认 ClusterIP(与单节点压测一致,可比)。
- **数据污染**:压测用 user_id 99999/88888 段,压后可按 user_id 段清理(或用独立 DB schema)。
