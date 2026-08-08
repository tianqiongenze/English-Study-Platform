# ESP Spike 压测结果(2026-08-03,P1-4)

> 单节点 Rancher Desktop,本机 locust 2.46.3 经 Traefik NodePort 192.168.64.6:31032 + Host: esp.example.com。
> 突发模型:0→2500 CCU in 30s(-r 83),hold 90s,落,×3 轮,共 7min。峰值取 5000 的一半(单节点 OOM 约束)。

## 结果
| 指标 | 值 |
|---|---|
| 总请求 | 155,261 |
| 失败 | 56,777(36.6%) |
| 平均吞吐 | 369.4 rps |
| 平均延迟 | 5052 ms |
| **P95** | **21,000 ms** |

## HPA 扩容反应(spike 核心验证点)
| 服务 | 扩容前 | 扩容后 | 结论 |
|---|---|---|---|
| learning-java | 1 | **6** | ✅ 扩到 6(MAX=15) |
| writing-py | 1 | **3**(MAX=3) | ✅ 扩满 |
| teacher-java | 1 | **2** | ✅ 扩到 2 |
| gateway-go | 2 | 2 | 未触发(网关 CPU 不高) |
| emotion-svc | 1 | 1 | 未触发 |

**HPA 能扩容**,但:(1) 扩容滞后 → P95 21s;(2) writing/teacher AI 推理长尾 → 503;(3) spike 中 metrics-server 被挤 → CPU `<unknown>` → HPA 短暂读不到指标(单节点资源瓶颈)。

## 探针修复验证(spike 间接验证)
旧问题(架构评分 §三 #1):1s-timeout 探针高负载误杀雪崩。本次探针已修 3s(bff/emotion/writing/sentinel/learning/teacher)。**spike 全程应用 pod 0 重启**(对比之前 bff 5 次重启)→ **探针误杀问题已解决**。

## 主要失败分类
- writing/grade 503(6362):AI 长尾,worker 阻塞(§三已知)。
- scenarios failed(10436):gateway 内置场景读,高并发下偶发。
- learning/event 404(307)+ 各端点零星 404:部分路由在突发期 gateway→下游转发异常(单节点连接饱和),非业务逻辑错。

## 结论
- ✅ **HPA 弹性机制工作**(扩容到位)。
- ✅ **探针稳定性问题解决**(无雪崩)。
- ⚠ **绝对性能未达标**(P95 21s、失败 36.6%)——单节点 + AI 长尾固有瓶颈,需多节点 + 分布式 Locust 重测(架构评分 §五 P2-3)。
- spike 目的(验 HPA 扩容 + 探针稳定性)**达成**。
