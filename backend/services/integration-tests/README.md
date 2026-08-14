# integration-tests — ESP 跨服务代码级集成测试

> 真实集群基础设施(TiDB / Redis / Pulsar)**禁用 H2**。代码级断言,非 HTTP 探活。

## 测试类清单

| 类 | 场景 | 集群依赖 |
|---|---|---|
| `T1_CrossServiceDubboTripleIT` | teacher→learning Dubbo Triple 真实跨服务 RPC | TiDB + learning-java provider(tri://learning-java:50091) |
| `T2_ReadWriteSplitIT` | 读写分离路由(ThreadLocal + AOP + 真实主从库) | TiDB tidb-primary / tidb-follower |
| `T3_MultiLevelCacheIT` | 多级缓存 L1 Caffeine / L2 Redis / 防穿透 / 延迟双删 | Redis(redis-cluster:6379) |
| `T4_DisruptorPipelineIT` | Disruptor 多级流水线 FIFO + 计数 | 无(进程内) |

## 前置条件

### 1. 公共包 mvn install

集成测试依赖 ESP 公共包,需先在本地仓库 install:

```bash
cd services/esp-api && mvn -q install
cd ../esp-rw-java && mvn -q install
cd ../esp-cache && mvn -q install
cd ../esp-disruptor && mvn -q install
```

### 2. 集群基础设施可达

- K8s context `rancher-desktop`,ns `esp-prod`。
- ExternalName 短名(集群内 Pod 可解析):
  - TiDB 写:`tidb-primary:4000` / 读:`tidb-follower:4000`,DB `esp`,root 无密码。
  - Redis:`redis-cluster:6379`(经 ExternalName → `redis-cluster-new`,Cluster 模式 3主3从,无密码)。
  - Pulsar:`pulsar-broker:6650`(T3 已关 consumer,producer 延迟创建失败降级,不强依赖)。
- `learning-java` provider 已部署,DNS `learning-java` 可解析,Triple 端口 50091 可达。
- `user_mastery` 表有种子数据(499 万行,T1 取测试 userId)。

## 执行方式

### 集群内 Pod 执行(推荐)

集成测试需访问集群内 DNS 短名,最稳方式是在 esp-prod 内的一个 Pod 里跑:

```bash
# 假设有一个带 mvn + 源码的 debug pod,或用 learning-java 的镜像挂载源码
kubectl -n esp-prod exec deploy/learning-java -- bash -c '
  cd /app/integration-tests &&
  mvn -q test -Dit.skip=false'
```

### host 执行(需端口转发)

host 上跑需把集群短名转发到本地端口,并改 application-it.yml 的 host 为 localhost:

```bash
# 端口转发(单节点铁律:与压测/灌装串行,不并行)
kubectl -n esp-prod port-forward svc/tidb-primary 44000:4000 &
kubectl -n esp-prod port-forward svc/tidb-follower 44001:4000 &
kubectl -n esp-prod port-forward svc/redis-cluster 6479:6379 &

cd services/integration-tests
mvn test -Dit.skip=false -Dspring.profiles.active=it
```

> 注意:host 执行时 Dubbo Triple 直连 `learning-java:50091` 无法解析,需额外 hosts 映射或改 url 为 `tri://localhost:<forward-port>`。推荐集群内 Pod 执行。

## 单节点铁律

Rancher Desktop 单节点:本模块 mvn 构建与集群压测/灌装**必须串行**,并行会饿死 VM CPU 致 API server TLS 超时 + 探针误杀雪崩。详见 `docs/PROGRESS.md`。

## 配置关键点

- `application-it.yml`:真实 TiDB jdbc-url(tidb-primary/tidb-follower:4000),**无 H2**。
- Redis Cluster 模式(`spring.data.redis.cluster.nodes=redis-cluster:6379`,经 ExternalName → redis-cluster-new)。
- Dubbo consumer `check=false` + `registry=N/A` + `url=tri://learning-java:50091/...` 直连,覆盖 `application-test.properties` 的 `dubbo.application.enable=false`。
- `esp.pulsar.consumer-enabled=false`:避免 consumer 阻塞启动。
- T4 Disruptor 进程内,无集群依赖,可独立跑。

## 已知限制

- T1 需 learning-java provider 在线;provider 未部署时该类会失败(check=false 不阻塞启动,但 RPC 调用超时)。
- T3 Pulsar 跨实例广播 `@Disabled`(单节点 consumer 关闭),广播逻辑由 `esp-cache` 的 `CacheBroadcastTest` 单测覆盖。
- T2 consistent=true 的 AOP 验证用同类内部方法读取 `RoutingHolder.get()`,非代理自调用(切面逻辑直接由方法体内 holder 状态证明)。
