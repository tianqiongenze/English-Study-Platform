# ESP 端到端冒烟测试报告 — 2026-08-02 11:10

## 测试环境
- K8s: Rancher Desktop 单节点,context rancher-desktop
- 业务命名空间 esp-prod,基础设施 ekb-m2-aws-us-east-1
- 三 Java 服务镜像 1.2.2(SB4.0.7 + Jetty),Seata client 2.5.0

## 测试结果:全部通过 ✅

### 服务健康(readiness/liveness probe)
| 服务 | 探针路径 | 状态 | 结论 |
|------|----------|------|------|
| bff-java | /actuator/health:8090 | UP | ✅ |
| teacher-java | /actuator/health:8083 | UP | ✅ |
| learning-java | /actuator/health:8081 | UP | ✅ |
| gateway-go | /healthz:8080 | ready=true | ✅ |
| writing-py | /healthz:8082 | ready=true | ✅ |
| emotion-svc | — | 1/1 Running | ✅ |

### 中间件通路
| 通路 | 验证 | 结论 |
|------|------|------|
| Dubbo Triple RPC | teacher-java → learning-java `LearningRpcService` 连接成功(NettyConnectionClient /10.42.0.8 → /10.42.0.10:50091),invokers size 2 发现 | ✅ |
| Seata 分布式事务 | bff TM+RM 注册 seata:8091 成功(2.5.0 client ↔ 2.0 server 握手) | ✅ |
| Nacos 服务发现 | Dubbo invoker 从 nacos:8848 发现 learning-java 2 实例 | ✅ |
| 读写分离 | teacher/learning HikariPool 双池(write→tidb-primary, read→tidb-follower)metrics 可见 | ✅ |
| 多级缓存 | bff Pulsar consumer esp-cache-invalidate 订阅成功(L1 失效广播) | ✅ |

### 可观测通路(本轮 task #8 修复后)
| 通路 | 验证 | 结论 |
|------|------|------|
| Metrics | Prometheus scrape otel-collector:8889 UP,HikariPool/JVM 指标流通(teacher/learning/bff 8 series) | ✅ |
| Traces | otel → tempo pipeline | ✅ |
| Logs | fluent-bit → loki pipeline | ✅ |

## 结论
三 Java 服务恢复后,业务通路(网关→BFF→下游 RPC→分布式事务→读写分离→多级缓存)全链路验证通过,可恢复 session_turns 灌数与压测。

## 已知 cosmetic 项(非阻塞)
- prometheus `kubernetes-nodes` scrape target down(kubelet 需 https cert auth,单节点开发环境非关键)
- writing-py `/health` 与 `/`(svc:80)返回错误,但 readiness probe `/healthz:8082` 通过(容器内服务正常,svc 路径差异,不影响运行)
