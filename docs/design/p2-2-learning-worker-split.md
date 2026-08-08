# P2-2 learning-worker 拆独立 Deployment 设计

> 评审基线:教授级高工 / 大厂资深架构师,按工业级可水平扩展对标。
> 状态:代码+部署就绪(单节点 RD 默认不 apply worker,保持 learning-java 单进程一体化运行)。

## 一、动机

learning-java 当前是"HTTP 入口(Dubbo Triple + Spring MVC)+ Disruptor 消费落库"同进程。
5000 CCU 压测暴露:Disruptor 批量 flush(BatchEventHandler 200 条/批或 500ms 超时)在
高负载下与 HTTP 请求线程争抢 CPU/JVM 资源,拖长 HTTP P95。工业级做法:把"消费落库"
这类 CPU 密集、可异步、对延迟不敏感的后台进程从"请求响应"路径剥离,独立 Deployment
水平扩展,互不影响。

## 二、拆分方案(进程间解耦 via Pulsar)

```
原(单进程):  HTTP → LearningService.reportEvent → Disruptor(进程内) → BatchEventHandler → DB

拆分后:
  learning-java(HTTP):  reportEvent → Pulsar topic LEARNING_EVENT_REPORT → (立即返回)
  learning-worker:      Pulsar 订阅 → Disruptor(进程内) → BatchEventHandler → DB
```

接缝点:`LearningService.reportEvent`(LearningService.java)。
- `esp.disruptor.enabled=true`(默认,单进程一体化):走进程内 Disruptor,无 Pulsar。
- `esp.disruptor.enabled=false`(worker 模式):发 Pulsar `learning.event.report`,由 worker 消费。

worker 侧 `LearningEventConsumer` 新增订阅 `LEARNING_EVENT_REPORT`,收到后回调
`learningService.reportEvent` 进 worker 进程内 Disruptor → 落库。落库唯一入口不变:
`DbLearningEventConsumer.consume`(learning_events 表)。

## 三、改动清单

1. `LearningEventDisruptor.java`:加 `@ConditionalOnProperty("esp.disruptor.enabled", matchIfMissing=true)`。
   worker 模式入口侧 enabled=false 时不装配。
2. `LearningService.java`:`reportEvent` 双路径(Disruptor 或 Pulsar);构造器改用
   `ObjectProvider<LearningEventDisruptor>`(可选注入)+ `EventPublisher` + `@Value esp.disruptor.enabled`。
   新增 `LearningEventReport` record 作为 Pulsar 事件体。
3. `EventTopics.java`:新增 `LEARNING_EVENT_REPORT = "learning.event.report"`。
4. `LearningEventConsumer.java`:新增订阅 `LEARNING_EVENT_REPORT` + `handleEventReport`。
5. `deploy/k8s/21-learning-worker.yaml`:新建 worker Deployment(同镜像,env 区分角色)。
6. 测试:`LearningServiceTest` 构造器签名同步 + 新增 `reportEvent_workerMode_forwardsToPulsar`。

## 四、角色切换(env)

| env | learning-java(api) | learning-worker |
|---|---|---|
| ESP_DISRUPTOR_ENABLED | false(入口不消费) | true(worker 消费) |
| PULSAR_CONSUMER_ENABLED | true(仍订阅 user.registered 等) | true(订阅 LEARNING_EVENT_REPORT) |
| SEATA_ENABLED | true(参与全局事务) | false(落库无全局事务,减负) |
| 暴露端口 | 8081 HTTP + 50091 Dubbo | 仅 actuator 健康(8081) |

> 单进程一体化模式(当前 RD 默认):learning-java `ESP_DISRUPTOR_ENABLED` 不设(默认 true),
> 不 apply worker Deployment,行为与拆分前完全一致 —— 零回归。

## 五、单节点 RD 约束与多节点启用步骤

**单节点 RD**:默认保持单进程一体化(不 apply worker),因单节点资源不足以同时跑
learning-java + learning-worker 两个 JVM,且单节点无水平扩展收益。代码+部署已就绪,
验证拆分逻辑靠单测(`reportEvent_workerMode_forwardsToPulsar`)。

**多节点启用步骤**:
1. `kubectl apply -f deploy/k8s/21-learning-worker.yaml`
2. learning-java Deployment 加 env `ESP_DISRUPTOR_ENABLED=false`(入口切 Pulsar 投递)
3. 滚动重启 learning-java,确认 reportEvent 走 Pulsar(日志 "forwarded to pulsar")
4. 观察 learning-worker 日志:订阅 LEARNING_EVENT_REPORT 成功 + Disruptor 消费落库
5. 按学情事件吞吐水平扩 worker replicas(Disruptor 消费 CPU 密集,HPA 按 CPU)

## 六、风险与权衡

- **消息可靠性**:Pulsar 投递失败降级只 warn(与现有 EDA 一致,业务可容忍少量学情事件丢失)。
  若需至少一次,可后续加重试队列(当前 TTL 兜底 + Disruptor 背压丢弃,与原进程内语义一致)。
- **重复消费**:worker 消费 LEARNING_EVENT_REPORT 后回调 reportEvent,若 worker 也 enabled=true
  会再次进 Disruptor —— 但 worker 侧入口不接收 HTTP,reportEvent 只被 Pulsar consumer 调用,
  不会循环(消费的是入口发的,不是自己发的)。单进程模式本 topic 无消息。
- **顺序**:同一 userId 的事件顺序在跨进程后由 Pulsar 分区保证(若 topic 分区,需按 userId 做 key 分区)。
  当前 topic 未分区,单 worker 顺序消费,顺序保留;多 worker 需按 userId 分区。
