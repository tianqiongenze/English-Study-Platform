# DB 访问吞吐能力优化方案(学习/教学/BFF 三 Java 服务)

> 2026-08-15 会话22-27 | 状态:方案+配置落地✅,编译验证✅,**未部署**(待确认窗口)
> 范围:backend/services/{bff-java, learning-java, teacher-java}(learning-worker 复用 learning-java 镜像,配置同源)+ platform/esp-rw-java

## 一、现状诊断

### 1.1 架构事实

- 读写分离:esp-rw-java 提供 `RoutingDataSource` + `RoutingHolder`(ThreadLocal)+ `@DataSource` AOP 切面;`LazyConnectionDataSourceProxy` 保证事务开启时不提前取连接。
- **虚拟线程已全量开启**:三服务 `spring.threads.virtual.enabled: true`(2026-08-06 #6 落地),Jetty/MVC 请求处理已跑在虚拟线程上。
- **@DataSource 方法体全为同步 JDBC**:learning/teacher 共 9 个 @DataSource 方法,无 @Async/CompletableFuture;P2-15 已探查确认无跨线程 ThreadLocal 传递,路由正确。
- 缓存前置已有:esp-cache(L1 Caffeine 1w/60s + L2 Redis 5min + Pulsar 失效广播 + TTL 抖动 + singleflight)。

### 1.2 同步 JDBC 的吞吐上限模型

同步阻塞模型下,单个连接池的吞吐上限:

```
Pool QPS 上限 = poolSize × (1000ms / avgRTT_ms)
```

TiDB 单点查询(点查/索引扫,含网络往返)按 P99 RTT 10ms、P50 5ms 估:

| 池大小 | P50(5ms) | P99(10ms) |
|---|---|---|
| 20 | 4,000 QPS | 2,000 QPS |
| 50 | 10,000 QPS | 5,000 QPS |

### 1.3 5000 CCU 目标下的需求测算

- 5000 CCU(并发在线),按工业经验活跃请求率 ~10% → **约 500 并发在途请求**。
- 其中经缓存前置(esp-cache L1+L2 命中率经验值 70%+),落到 DB 的读 ≈ 150 QPS;写(学习事件/测评提交,Disruptor 批量缓冲后)≈ 100 QPS 峰值。
- **结论:20 连接/池(4,000 QPS @P50)对 5000 CCU 有 >10x 余量**,瓶颈从来不在连接数,而在 TiKV 单节点 region 压力(坑㉑:种子灌入期 TiKV 单节点 region 报 ERR)。

### 1.4 现状问题

| 服务 | write 池 | read 池 | shared 池 | 合计 | 问题 |
|---|---|---|---|---|---|
| learning-java | 50 | 50 | 10 | **110** | 池过大,高并发下瞬时 110 连接打满 TiDB,加剧 TiKV region 热点 |
| teacher-java | 30 | 30 | - | 60 | 偏大(30 未设 minimum-idle,冷启动抖动) |
| bff-java | 20 | 20 | - | 40 | ✅ 已达标 |
| **合计** | | | | **210** | 单节点 TiDB 理想承载 ≤100 业务连接 |

另外:learning/teacher 均**未配 Hibernate JDBC 批量参数**(默认逐条 INSERT/UPDATE),learning-java 已有 2 处 saveAll 调用但未吃到批量化收益。

## 二、四层优化方案(推荐组合:②+①+③+④,最小侵入)

### ① HikariCP 池参数收敛(每实例 write=20 / read=20)

- **新值**:learning 50→20(双池),teacher 30→20(双池,补 minimum-idle:5),bff 20 维持;learning shared(app_logs)10→维持。
- 全集群业务连接 210→**130**,对单节点 TiDB 降压;吞吐容量(20/池 = 2k~4k QPS)对 5000 CCU 目标仍有 10x 余量(见 1.3 测算)。
- **理由(为什么不是更大池)**:虚拟线程开启后,请求线程近乎无限,"排队等连接"发生在 Hikari 池上而非线程池上——池是天然的限流器。池超过 DB 承载只会把排队转移到 TiDB/TiKV 内部(更糟:锁等待+region 热点+RTT 膨胀,吞吐反而下降)。**池大小 ≈ DB 能健康服务的并发数**,这是 HikariCP 官方公式 `connections = core_count × 2 + spindle_count` 的本意;TiDB 计算在 TiKV 侧,单节点验证环境下 20/池是工业合理值,多节点迁移时可按 TiDB 节点数水平放大。
- 迁移多节点时:每服务实例池不变,靠**多实例横向扩**(HPA)放大总连接,DB 端 tidb `token_limit` 与 TiKV 容量同步评估。

### ② JDK21+ 虚拟线程(已开启,本次确认语义不变)

- 三服务已 `spring.threads.virtual.enabled: true`,**本次无代码改动**;关键确认:虚拟线程下阻塞 JDBC 变廉价(park 虚拟线程不占平台线程),平台线程不再是瓶颈,吞吐瓶颈正确转移到 Hikari 池 → 与 ① 配合,池即限流器。
- **RoutingHolder ThreadLocal 在虚拟线程下语义验证**(本次落注释):每个虚拟线程是独立 `Thread`,`ThreadLocal` 读写作用于该虚拟线程自身,请求处理全链路(Jetty accept → 虚拟线程 execute → @DataSource AOP set/proceed/clear)在**同一虚拟线程内完成**,无跨线程传递,路由正确。⚠️ 唯一注意:若未来引入 @Async/线程池/CompletableFuture 异步链,必须显式传递路由上下文(ScopedValue JDK25 已 stable,不可变+有界共享,是更优解,备注 TODO 不强改)。
- R2DBC/全异步化:**明确不建议**——Spring Data JPA 生态(Hibernate 6 阻塞 ORM、Seata AT 数据源代理、esp-rw-java 路由层)全面不支持 R2DBC,改造成本(重写 Repository 层+事务语义+Seata 迁移)远超收益;虚拟线程已用 ~0 代码成本拿到异步化的吞吐收益。

### ③ Hibernate JDBC 批量写入(learning/teacher)

```yaml
spring.jpa.properties.hibernate:
  jdbc.batch_size: 50          # 批量绑定上限(50 与 MySQL 驱动 max_allowed_packet 及事务时长平衡)
  order_inserts: true          # 同类型实体排序后同批 INSERT,批满 50 才真正 flush
  order_updates: true
```

- 生效点:learning-java 的 saveAll 链路(UserMastery 批量、Disruptor flush 链路落库)+ teacher Submission 批量。逐条→50 条/往返,写路径 DB 往返次数降 ~50x。
- **红线(坑㉖)**:严禁 JpaRepository.findAll() 千万行表全表扫描(learning_events 30 亿级),业务去重一律投影查询;本参数只优化既有 saveAll,不鼓励更多全量读。

### ④ 缓存前置(已有 esp-cache,本次无改动)

- 读多接口(top/funnel/heatmap/aggregateByUser)已走 L1 Caffeine+L2 Redis,命中后不打 DB;5000 CCU 下这层把 DB 读压到 ~30%,是吞吐公式里**分母(落到 DB 的 QPS)的削减器**,与 ①②③ 正交叠加。

## 三、配置落地清单(2026-08-15,未部署)

| 文件 | 改动 |
|---|---|
| backend/services/learning-java/src/main/resources/application.yml | write/read 池 50→20 + minimum-idle;新增 hibernate batch 三参数 |
| backend/services/teacher-java/src/main/resources/application.yml | write/read 池 30→20 + minimum-idle:5;新增 hibernate batch 三参数 |
| backend/services/bff-java/src/main/resources/application.yml | 池 20 维持,仅补注释理由(bff 无 JPA,不加 batch 参数) |
| platform/esp-rw-java/src/main/java/com/esp/rw/RoutingHolder.java | 虚拟线程 ThreadLocal 语义验证注释 + ScopedValue TODO |

## 四、部署与回归步骤(待用户确认窗口执行)

```bash
# 1. 构建镜像(串行,主机重负载不与集群操作并行)
cd backend/services/learning-java && mvn -B package -DskipTests -q && docker build -t esp/learning-java:1.8.7 .
cd ../teacher-java && mvn -B package -DskipTests -q && docker build -t esp/teacher-java:1.7.8 .
# 2. 滚动更新(配置在镜像内,需换 tag)
kubectl -n esp-prod set image deploy/learning-java learning-java=esp/learning-java:1.8.7
kubectl -n esp-prod set image deploy/learning-worker learning-worker=esp/learning-java:1.8.7
kubectl -n esp-prod set image deploy/teacher-java teacher-java=esp/teacher-java:1.7.8
kubectl -n esp-prod rollout status deploy/learning-java --timeout=180s
# 3. 回归
#  - 健康:kubectl -n esp-prod get pods -l app in (learning-java,learning-worker,teacher-java)
#  - 池指标:GET :808x/actuator/metrics/hikaricp.connections.active (预期 active ≤20)
#  - 批量生效:压测写路径后查 TiDB general log / hibernate 日志 batchSize,或 actuator/metrics/http.server.request 数对比
#  - 功能:behavior/top、submission 提交、learning 事件落库 smoke
#  - 压测:k6 500 CCU 档对比 p99 与 DB QPS(基线 vs 本次)
```

## 五、风险与回退

- 池收敛风险:极端 DB 慢查询时排队提前到应用层(connection-timeout 30s 默认),表现为接口 RT 上升而非 DB 雪崩——这是**以应用层背压换 DB 稳定**的预期取舍;回退=改回 50 并换镜像 tag。
- batch_size 风险:大批量 + 长事务占用连接变久,50 上限已控;Seata AT 代理下批量 SQL 的 undo_log 生成不受影响(逐行镜像)。
- 多节点迁移:池参数按实例不变,靠副本数放大;TiDB 连接总数 = 副本 × 40,规划 tidb token_limit 与 TiKV 节点数匹配。
