# ESP k6 压测(集群内,绕过 host port-forward)

> 与 `deploy/locust/` 的分工:**k6 打 REST API 吞吐基线**(单机 Go 引擎,轻量高并发),**Locust 留给 WS 口语会话场景**(L6,会话生命周期建模强)。两者交叉验证 rps。

## 为什么用 k6 + 集群内执行

- **绕过 host port-forward 开销**:port-forward 是 `kubectl→apiserver→kubelet→pod` 的用户态 HTTP 隧道,每个请求经 apiserver 中转,压测时成为瓶颈(host 测得仅 28 rps,非服务真实容量)。
- **压测机入集群走 ClusterIP 直连**:单节点铁律下"压测机与被测同 VM"是唯一正确姿势(生产环境压测机也在集群内/同 VPC)。
- **k6 优势**:Go 事件驱动,单机轻松数万 VU;CLI 友好;原生 HTTP/WS/gRPC;输出 P50/P95/P99 + rps。

## 执行(集群内 k6 pod)

```bash
# scenarios REST 基线(默认 50 VU,10s 爬坡 + 30s 稳态 + 10s 回落)
kubectl -n esp-prod run k6-baseline --rm -i --restart=Never \
  --image=grafana/k6:0.54.0 --command -- k6 run - < deploy/k6/scenarios-baseline.js

# 高并发(100 VU,单节点上限勿超 200 防 OOM)
kubectl -n esp-prod run k6-baseline --rm -i --restart=Never \
  --image=grafana/k6:0.54.0 --command -- \
  k6 run -e VUS=100 - < deploy/k6/scenarios-baseline.js
```

环境变量覆盖:
- `HOST`(默认 `http://gateway-go:80`)— 被测服务 ClusterIP
- `BFF`(默认 `http://bff-java:80`)— 鉴权服务(注册/登录拿 JWT)
- `VUS`(默认 50)— 虚拟用户数

## 注意(单节点铁律)

- **VUS 上限 200**:单节点 RD 9核/29G,k6 pod 与被测同 VM,过高并发会触发 OOM / 探针误杀雪崩。
- **不与主机重操作并行**:压测期间禁在 host 跑 mvn/docker build(主机重操作与集群操作串行)。
- **GET /api/scenarios 幂等只读**:不写数据,安全可反复跑。
- **JWT**:脚本内每个 VU 用唯一邮箱 `k6_${__VU}@loadtest.com` 注册(role=learner,避开 register role 非 enum 坑),已注册则降级 login。
- 压测后观察 node 负载,异常立即 `kubectl delete pod k6-baseline`。

## 输出指标

- `http_reqs` / `iteration_duration` / `http_req_failed` — k6 内置
- `scenarios_duration_ms`(P95 < 3000ms 阈值)— scenarios 端点独立耗时
- `login_errors` — 注册/登录失败计数

## 与 Locust 对照

跑完 k6 后,对比 `deploy/locust/` 打同一 scenarios 接口的 rps。预期两者接近(都走 ClusterIP),且都远高于 host port-forward 的 28 rps,证明 28 是隧道开销。差异点:Locust Python GIL 限制单进程吞吐,高并发需分布式 worker;k6 单进程即可。
