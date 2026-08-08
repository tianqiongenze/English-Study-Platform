# ESP 缓存机制现状审计

> 调研日期:2026-07-31 | 范围:services/ 下 8 个服务 | 模式:只读调研,不重构

## 一句话结论

ESP 项目**尚未建立多级缓存(L1 本地 + L2 分布式)**。当前只有 gateway-go 把 Redis 当作"会话状态库"使用(不是缓存,是主存),其余 7 个服务几乎零缓存;Java 服务虽有 spring-boot-starter-data-redis 依赖,但无任何 RedisTemplate/Cache 注解的实际调用,属"接了管子没通水"。

## 总表:各服务缓存现状

| 服务 | L1 本地缓存 | L2 Redis 分布式缓存 | 一致性机制 | 缺口 |
|------|------------|-------------------|-----------|------|
| gateway-go | 无(go-redis 外的 sync.RWMutex map 仅场景模板静态表,非 TTL 缓存) | **有**:`session:{id}` 存口语会话全文(Set/Get/Del,TTL 2h)—— 但这是**会话主存**而非缓存 | 仅 TTL 2h 兜底;无失效广播 | 无 L1;无业务数据缓存(无场景/用户/LLM 结果缓存) |
| learning-java | 无 | 依赖已加(pom + application.yml),**但代码无 RedisTemplate/opsForXxx/Cache 注解调用** | 无 | UserMastery/复习清单/薄弱知识点全打 TiDB,无缓存;热点读无兜底 |
| bff-java | 无 | 依赖已加(pom + application.yml),**代码无任何 Redis 调用**(限流走 Sentinel 单机内存规则) | 无 | JWT 校验/用户信息/聚合结果无缓存;限流仅本地规则非分布式 |
| teacher-java | 无 | 无(无依赖、无配置) | 无 | 教师批改/作业数据无缓存 |
| writing-py | 无 | 无(agent.py 注释提到 RedisSaver 仅设计预留,未实现) | 无 | 写作评分/agent checkpoint 无缓存无持久化 |
| emotion-svc | 无 | 无 | 无 | 情绪分析结果无缓存 |
| esp-api | 无 | 无 | 无 | — |
| esp-disruptor | 无 | 无 | 无 | Disruptor 事件管线纯内存+落库,不回写缓存 |

关键证据(路径:行号):
- gateway-go 会话主存:`services/gateway-go/src/session/manager.go:54`(Set TTL)、`:59`(Get)、`:95`(Del)
- gateway-go Redis 客户端创建:`services/gateway-go/src/main.go:55-56`
- gateway-go 场景静态 map(非缓存):`services/gateway-go/src/scenario/repository.go:15-18`(sync.RWMutex + map,无 TTL/淘汰)
- learning-java 依赖空载:`services/learning-java/pom.xml:40`(starter-data-redis)、`application.yml:31-33`(host 配),但 `src/main/java` 全量 grep `RedisTemplate/opsFor/@Cacheable` 零命中
- bff-java 依赖空载:`services/bff-java/pom.xml:179`、`application.yml:43-45`;`src/main/java` 全量 grep `Redis/redisTemplate/opsFor` 零命中
- bff-java 限流走 Sentinel 本地规则:`services/bff-java/src/main/java/com/esp/bff/config/RateLimitFilter.java:40-44`(FlowRuleManager.loadRules,单机)
- learning-java 业务直打 DB:`services/learning-java/src/main/java/com/esp/learning/service/LearningService.java:59,67,76`(dueReview/weakestPoints/review 全走 Repository)
- writing-py RedisSaver 仅注释:`services/writing-py/app/agent.py:6`
- esp-disruptor 无缓存回写:`services/esp-disruptor/src/main/java/com/esp/disruptor/` 全量 grep `cache/redis` 零命中

## 三大缺口清单

### 缺口 1:L1 本地缓存完全缺失
- 全项目零 Caffeine/Guava Cache/LoadingCache/sync.Map(作缓存用)/lru/freecache/bigcache/ristretto/lru_cache/TTLCache/cachetools。
- 热点数据(用户 mastery、场景模板、JWT 公钥、教师配置、写作评分模板)在每次请求都跨网络查 DB 或重算,无本地兜底。
- 唯一的 sync.RWMutex map(gateway-go scenario)是静态预置表,无容量上限、无 TTL、无淘汰策略,不能算缓存基础设施。

### 缺口 2:L2 Redis 仅 gateway-go 单点使用,且非"缓存"语义
- 只有 gateway-go 真正调用了 Redis,但语义是**会话主存**(网关无状态化设计),不是缓存(无 DB 回源兜底,Redis 挂了会话就丢)。
- learning-java / bff-java 引入了 starter-data-redis 依赖和配置,但**代码层零调用**,等于摆设。
- teacher-java / writing-py / emotion-svc / esp-api / esp-disruptor 连依赖都没有。
- 即:整个项目没有"业务读缓存 + DB 回源"的标准缓存模式存在。

### 缺口 3:缓存一致性机制完全空白
- 无 @CacheEvict / @CachePut / cache.delete / invalidate / 延迟双删 任何代码命中。
- 无基于 Pulsar / TiDB binlog / Canal 的缓存失效广播(全项目 grep `canal/binlog` 零命中;Pulsar 仅在 learning-java 注释中提及"可叠加 Pulsar 发布",未实现)。
- 无 L1↔L2 跨节点失效机制(因为根本没有 L1)。
- 仅 gateway-go session 靠 TTL 2h 兜底,但那是会话过期,不是缓存一致性。
- esp-disruptor / learning-java Disruptor 消费 learning_events 时**只落库,不回写/失效任何缓存**(无 cache 代码路径)。

## 现存问题小结
1. 热点数据未缓存:UserMastery(复习清单/薄弱点)、场景模板、JWT 校验、写作评分、教师批改 —— 全部直打 TiDB 或重算,5000 CCU 下 DB 压力无缓冲。
2. 只有单级(gateway-go session 且是主存非缓存),无任何服务具备 L1+L2 双级结构。
3. 一致性保障为零:没有失效、没有广播、没有双删,只有 TTL(且仅 session 用)。一旦后续加缓存,需从零搭建一致性方案。
4. 依赖与代码脱节:learning-java / bff-java 引了 Redis 依赖却不用,属技术债(要么用起来要么先移除)。

## 结论
**ESP 当前不存在"多级缓存"体系**,只有 gateway-go 一个"Redis-as-primary-store"的零散用法。要达到企业级缓存能力,需从零设计 L1(Caffeine / Ristretto / Python cachetools)+ L2(Redis)+ 一致性(失效广播/延迟双删/TTL 兜底)的完整方案。
