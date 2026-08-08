# Pulsar 三区 + Geo-Replication 方案

> ESP 事件总线架构升级设计。路线(用户已确认):Pulsar 三区 + Geo-Replication。
> 阶段定位:**单节点 Rancher Desktop 阶段不落地执行**,多节点服务器移植时的设计蓝本。

---

## 1. 现状

| 维度 | 现状 |
|---|---|
| 部署 | 单 broker `pulsar-broker` deploy,ns `ekb-m2-aws-us-east-1`,**已 scale 0**(OOMKilled,节点内存 91%) |
| 应用 | esp-cache 用 Pulsar 做 `esp-cache-invalidate` topic 跨实例 L1 失效广播(EventPublisher sendAsync,失败降级 warn) |
| 事件 topic(esp-cache/event/EventTopics) | user.registered / teacher.finalized / teacher.autograde.requested / session.ended / writing.graded / learning.event.report |
| 客户端 | PulsarClientHolder 延迟初始化;CacheConfig 有 `esp.pulsar.consumer-enabled` flag |
| 应用侧连接 | `pulsar://pulsar-broker:6650`(单 broker) |
| 影响 | scale 0 → L1 失效广播断;靠 TTL 60s 兜底,业务降级不致命 |

---

## 2. 目标

三区各 1 broker,Geo-Replication 跨区复制**持久化业务事件**;缓存失效广播不强跨区(各区内广播即可)。

---

## 3. Geo-Replication 原理

- Pulsar 原生 Geo-Replication:配置三区 broker 两两互为 tenant 的 replication cluster,topic 级别开启 replication。
- **异步复制**,适合 EDA 事件最终一致(非强一致)。
- 消息在本地 broker 持久化后异步推送到远区,远区 broker 作为远区 tenant 的订阅者收消息并重投本地 topic。

---

## 4. ESP 事件分类与复制策略

| 事件 topic | 性质 | 跨区复制? | 理由 |
|---|---|---|---|
| learning.event.report | 持久化业务事件 | **是**(三区 Geo-Replication) | 各区分析都需要全量学习事件 |
| writing.graded | 持久化业务事件 | **是** | 跨区查询成绩一致性 |
| session.ended | 持久化业务事件 | **是** | 会话终态跨区可见 |
| user.registered | 持久化业务事件 | **是** | 用户画像跨区 |
| teacher.finalized | 持久化业务事件 | **是** | 批改结果跨区 |
| esp-cache-invalidate | 缓存失效广播 | **否** | 各区有独立 Redis Cluster,跨区靠 DB 兜底 + TTL;区内广播即可 |
| teacher.autograde.requested | 批改请求 | **否**(区内就近) | 批改就近处理,不跨区 |

---

## 5. 应用侧改动

| 项 | 现状 | 改动 |
|---|---|---|
| PulsarClient | 单 broker `pulsar://pulsar-broker:6650` | 多 broker:`pulsar://pulsar-broker-aws:6650,pulsar-broker-azure:6650,pulsar-broker-gcp:6650`(就近主连) |
| EventPublisher | 无差别 sendAsync | 按 topic 决定是否开 replication(`producer.property("replication-clusters", ...)`) |
| producer 策略 | sendAsync 失败降级 | 持久化业务事件 producer 改 send 同步 + 失败重试(保证不丢);缓存广播维持 sendAsync 降级 |
| consumer | flag `esp.pulsar.consumer-enabled` | 跨区复制 topic 的 consumer 订阅本地 tenant 的 replicated topic |

---

## 6. K8s manifest 草案

草案文件:`deploy/k8s/06d-pulsar-geo.yaml`(见同目录)。

要点:
- 三区各 1 `pulsar-broker` deploy + ZooKeeper(或 KRaft 模式轻量化)+ BookKeeper。
- Geo-Replication 配置 Job:`pulsar-admin tenants create` + `pulsar-admin namespaces set-replication-clusters`。
- manifest 内注释初始化步骤(建 ZK/BK → 建 broker → 配 replication → 建 tenant/ns → topic 开 replication)。

---

## 7. 资源与风险

| 项 | 评估 |
|---|---|
| 资源 | Pulsar 重(ZK + BK + Broker),三区全量单节点 RD **跑不动**(当前单 broker 已 OOMKilled) |
| scale 0 影响 | 缓存失效广播断 → L1 失效靠 TTL 60s 兜底,业务降级不致命;无持久化业务消息(ESP 事件目前未强依赖 Pulsar 持久化) |
| Geo-Replication 延迟 | 异步,跨区事件最终一致(秒级),ESP 可接受 |
| 运维复杂度 | ZK/BK 三区 + replication 配置,运维门槛高 |

---

## 8. 结论(分阶段路线)

| 阶段 | 动作 | 理由 |
|---|---|---|
| 当前(单节点 RD) | 维持单 broker **scale 0**(无业务消息依赖);评估是否 scale 0→1 恢复单实例验证缓存广播链路 | RD 跑不动三区 Pulsar;缓存广播挂了 TTL 兜底不致命 |
| 多节点移植(目标) | **上三区 Pulsar + Geo-Replication**(持久化业务事件跨区) | EDA 事件最终一致跨区可见 |

**明确标注:单节点 RD 阶段不落地三区 Pulsar,多节点移植时执行。**

---

## 关键架构决策点(一句话结论)

1. **复制策略**:持久化业务事件(learning/writing/session/user/teacher)跨区 Geo-Replication;缓存失效广播不跨区(各区独立 Redis + DB 兜底)。
2. **producer 语义**:持久化事件同步 send + 重试,缓存广播异步降级。
3. **当前降级**:scale 0 可接受,靠 TTL 60s 兜底 L1 失效。
4. **落地阶段**:单节点 RD 不落地,多节点移植时上三区 Geo-Replication。
