# ESP Redis Cluster Warm-up 预热方案

> 状态:设计中(2026-07-31)。待 session_turns 种子跑完后实施。
> 关联:docs/test/00-测试总览.md §4、docs/test/03-性能与压测报告.md §2、docs/design/03-详细设计文档-SDD.md §2.7。

## 1. 调研结论:现有 Redis 用法

### 1.1 哪些服务真用 Redis
| 服务 | 用否 | key 模式 | 数据结构 | TTL | 证据 |
|---|---|---|---|---|---|
| gateway-go | ✅ 生产用 | `session:{sessionId}` | STRING(JSON 序列化的 Session 对象) | 2 小时(`2*time.Hour`) | `services/gateway-go/src/session/manager.go:39,54,59,87`;`services/gateway-go/src/main.go:55-59` |
| bff-java | ⚠️ 配置有,代码无 | (无 RedisTemplate/Cacheable 调用) | - | - | `services/bff-java/src/main/resources/application.yml:43-45` 配 host;Java 代码无 `RedisTemplate/@Cacheable/@EnableCaching`(grep 计数 0)。SDD §2.7 注记"Redis 配置/依赖存在但无代码使用(预留缓存)" |
| learning-java | ⚠️ 配置有,代码无 | - | - | - | `services/learning-java/src/main/resources/application.yml:31-32` 配 host;代码无 Redis API 调用 |
| teacher-java | ❌ 无 | - | - | - | application.yml 无 redis 配置 |
| writing-py | ⚠️ 设计预留 | `langgraph checkpoint` 命名空间 | (由 RedisSaver 管理) | (由 RedisSaver) | `services/writing-py/app/agent.py:6` 注释"生产 RedisSaver",当前测试用 MemorySaver,未接 Redis |
| emotion-svc | ❌ 无 | - | - | - | app 代码无 Redis |

### 1.2 种子脚本/运行态用 Redis(非业务缓存)
- `scripts/seed/seed_learning_events.py:59,75` —— 进度 key `esp:ctx:seed:learning_events:shard:{shard_id}`,STRING(行数)
- `scripts/seed/seed_session_turns.py:23,38` —— 进度 key `esp:ctx:seed:session_turns:shard:{shard_id}`,STRING(行数)
- `scripts/ctx_state.sh` —— 跨会话运行态 `esp:ctx:*`(压测计数、里程碑等)

### 1.3 key 命名规则归纳
- 业务(已实现):`session:{uuid}` —— gateway-go 唯一在用,前缀 `session:`,无 `esp:` 根前缀。
- 运行态(已实现):`esp:ctx:*` —— 进度/计数/里程碑。
- 设计预留(未实现):bff 限流计数、缓存。SAD §3 提"Redis 令牌桶/Sentinel 限流",但 bff-java 实际限流走 Sentinel(本地),未落 Redis。

## 2. 适合放 Redis 的业务热数据(基于 schema.sql + PRD)

TiDB 12 张表中,按"高频读 / 低频写 / 量级可控"筛选:

| 候选数据 | TiDB 表 | 读频率 | 写频率 | 量级 | 适合缓存 | 缓存结构 |
|---|---|---|---|---|---|---|
| 场景模板(scenarios) | scenarios | 每次开新会话读 | 极低(运营改) | 几十~几百行 | ✅ 强烈适合 | HASH(全量,key `esp:cache:scenario:{code}`) |
| 知识点树(knowledge_points) | knowledge_points | 学情计算/展示高频 | 极低 | 数百行 | ✅ 适合 | HASH + ZSET(按 parent_id 索引) |
| 用户基本信息 | users | 每请求读(BFF 鉴权) | 低 | 5050 行(种子 user) | ✅ 适合 | STRING(JSON),key `esp:cache:user:{id}` |
| 用户掌握度快照 | user_mastery | 学情展示高频 | 中(每次 review) | 5050×N(每用户数十 kp) | ✅ 适合(热点用户) | HASH,key `esp:cache:mastery:{user_id}` |
| 热门会话历史 | session_turns | 会话续读 | 高(流式写) | 3 亿行(只缓存热点) | ⚠️ 仅热点子集 | LIST(最近 N 轮),key `esp:cache:turns:{session_id}` |
| 活跃会话状态 | (gateway session) | 每帧读 | 每轮写 | 并发会话数 | ✅ 已在用 | STRING(`session:*`) |

> learning_events(30 亿行)/ writing_tasks / lesson_plans / classes / assignments / submissions:写多或低频读,不预热。

## 3. Warm-up 方案(要灌的 key)

### 3.1 灌数清单
| # | key 命名 | 结构 | 量级 | 来源 | TTL | 用途 |
|---|---|---|---|---|---|---|
| W1 | `esp:cache:scenario:{code}` | HASH(title/system_prompt/difficulty/lang) | 全量(~20,对应 scenarios 表) | TiDB `SELECT * FROM scenarios` | 24h | gateway 开新会话免查库 |
| W2 | `esp:cache:user:{id}` | STRING(JSON:`{id,role,name,email}`) | 5050(全量种子用户) | TiDB `SELECT id,role,name,email FROM users` | 1h | BFF 鉴权/聚合免查库 |
| W3 | `esp:cache:mastery:{user_id}` | HASH(kp_id->mastery_score, top-N 字段) | 5050(每用户一行聚合) | TiDB 聚合 `user_mastery` | 30min | 学情展示热点读 |
| W4 | `esp:cache:turns:recent:{session_id}` | LIST(最近 10 轮 content) | top-10000 热门 session | TiDB `session_turns` 按 session_id 取最近 10 行 | 10min | 会话续读热点(补充 session:* 活态) |
| W5 | `esp:ctx:warmup:done` | STRING("1") | 1 | 脚本写入 | 永久 | 灌数完成标志(验证用) |

### 3.2 示例 value
```
W1  HSET esp:cache:scenario:airport title "At the Airport" system_prompt "..." difficulty 3 lang en
W2  SET esp:cache:user:1 '{"id":1,"role":"learner","name":"user_1","email":"u1@esp.test"}'
W3  HSET esp:cache:mastery:1 kp_1 0.65 kp_2 0.80 kp_3 0.42 ... (top 20 kp)
W4  RPUSH esp:cache:turns:recent:1001 '{"role":"user","content":"Hi"}' '{"role":"ai","content":"Hello"}' ...
```

### 3.3 量级与容量估算(Redis 5Gi PVC)
- W1:20 key × ~1KB = 20KB
- W2:5050 key × ~120B = ~600KB
- W3:5050 key × ~400B(20 字段) = ~2MB
- W4:10000 key × ~2KB(10 轮 × ~200B) = ~20MB
- 合计业务预热 ~23MB,远低于 5Gi。
- 含 AOF overhead 与碎片,实际占用 < 100MB。
- 结论:5Gi 可轻松容纳全量用户(5050)+ 热点会话(1万)+ 全量场景;若未来用户增长到 100 万,仍可容纳(~120MB for W2)。**无需只取子集**,5050 用户全量预热即可覆盖压测所有压测虚拟用户。

### 3.4 量级建议(达 warm up 效果又不撑爆)
- **必灌**:W1(20)、W2(5050)、W5(1) —— 压测前最低保证,BFF/gateway 热路径全命中。
- **建议灌**:W3(5050) —— 学情展示热路径,占 ~2MB,性价比高。
- **按需灌**:W4(1万热点 session) —— 仅当压测脚本读历史会话时灌;若压测只走新会话(`session:*` 运行态自生成)可跳过。
- **上限红线**:总 key 数 < 20 万、总内存 < 1Gi(留 4Gi 给运行态 `session:*` 与 `esp:ctx:*`)。当前方案远低于红线。

## 4. 灌数脚本设计

### 4.1 文件:`scripts/seed/redis_warmup.py`(新建,docs/test/00 已引用此名)
- 参考 `seed_learning_events.py` 的分片模式,但 Redis 灌数量级小,无需分片;单进程 pipeline 批量即可。
- 连接:`redis.Redis(host="redis-cluster", port=6379, decode_responses=True)`(集群内 pod 执行,或本机 port-forward)。
- 数据来源:优先从 TiDB 读(pymysql,复用 seed 脚本的 TIDB_HOST/PORT/USER/DB 常量),保证与种子数据一致。
- 灌数流程:
  1. `SELECT * FROM scenarios` → 批量 HSET W1
  2. `SELECT id,role,name,email FROM users` → pipeline SET W2(带 TTL)
  3. 聚合 `SELECT user_id, kp_id, mastery_score FROM user_mastery` → 每 user 取 top20 kp → HSET W3
  4. (可选)`SELECT session_id, role, content FROM session_turns WHERE session_id IN (top 10000) ORDER BY created_at DESC LIMIT 10` → RPUSH W4
  5. `SET esp:ctx:warmup:done 1` + 记录各 key 计数到 `esp:ctx:warmup:stats`
- pipeline 批量 500 条/批,避免单条 RTT。
- 幂等:先 `DEL` 对应前缀(可选 `--flush` 参数),再灌;支持重复执行。

### 4.2 执行入口(集群内)
```bash
kubectl -n esp-prod run redis-warmup --rm -i --restart=Never \
  --image=python:3.12-slim \
  --env REDIS_HOST=redis-cluster --env TIDB_HOST=tidb-primary \
  -- python3 /scripts/seed/redis_warmup.py
```
(实际:把脚本 ConfigMap 挂载或 cp 进 pod;或用现有 pymysql pod)

## 5. 验证方法(灌后)

```bash
# 1. 总 key 数(应含 warmup key + 运行态 esp:ctx:*)
redis-cli -h redis-cluster DBSIZE

# 2. 抽样验证
redis-cli -h redis-cluster GET esp:ctx:warmup:done           # => "1"
redis-cli -h redis-cluster GET esp:cache:user:1              # => JSON
redis-cli -h redis-cluster HGETALL esp:cache:scenario:airport
redis-cli -h redis-cluster HLEN esp:cache:mastery:1          # => 20

# 3. 各类 key 计数
redis-cli -h redis-cluster --scan --pattern 'esp:cache:user:*' | wc -l   # => 5050
redis-cli -h redis-cluster --scan --pattern 'esp:cache:scenario:*' | wc -l # => ~20
redis-cli -h redis-cluster --scan --pattern 'esp:cache:mastery:*' | wc -l  # => 5050

# 4. 内存占用
redis-cli -h redis-cluster INFO memory | grep used_memory_human
```

## 6. 注意事项与依赖

- **前置依赖**:session_turns 种子跑完(W3/W4 依赖 user_mastery / session_turns 数据已落库)。当前两表 TiDB 计数 0(PROGRESS §本轮任务状态 #6)。
- **gateway-go session:* 不预热**:会话状态是运行态,压测时由压测流量自然生成;预热它无意义且会过期。
- **bff-java 预留缓存**:当前 bff 代码未读 Redis(见 §1.1),warm up 的 `esp:cache:user:*` 在 bff 接入缓存代码前不会被命中。本方案按"设计先行"灌入,待 bff/learning 补 `@Cacheable` 或 RedisTemplate 读取后即可生效;压测若仅打 gateway-go WS 链路,实际命中只有 W1(若 gateway 改读 scenario 缓存)。
- **AOF 持久化**:redis-cluster 已开 `--appendonly yes`(deploy/k8s/06b-redis-pvc.yaml),warm up 数据重启不丢;但 warm up key 带 TTL,长期会过期,压测前需重灌或调长 TTL。
- **非集群模式**:当前 redis-cluster 实为单实例 deployment(非真正 6 主 6 从 Cluster),`redis.Redis(host=...)` 直连即可,无需 `RedisCluster` 客户端。SAD §3 描述的"6 主 6 从"为生产目标,当前 RD 单节点。

## 7. 脚本实现状态(2026-08-04 更新)

- **脚本已就绪**:`scripts/seed/redis_warmup.py`(未运行,待种子灌装完成后执行)。
- **key 命名以代码为准**(plan §3.1 的 `esp:cache:*` 前缀为旧结论,已弃用):
  - W1 `esp:cache:scenario:{code}` **取消** —— gateway-go scenario 走进程内 map(`scenario/repository.go`),不经 Redis,预热无效。
  - W2 改为 `user:{id}`(对齐 `AuthController` `put(CACHE_USER,"user:"+id,...)`),value 为 Jackson default-typing Map 序列化。
  - W3 改为 `learning:due:{uid}` / `learning:weak:{uid}`(对齐 `LearningService.dueReview/weakestPoints`),value 为 `List<UserMastery>` Jackson default-typing 序列化。**尽力而为**:Python 复刻 POJO 类型签名脆弱,反序列化失败时 `CacheService` 会 try-catch 降级回 DB,不影响正确性。
  - W4 跳过 —— 无服务读 `esp:cache:turns:recent:*`,压测新会话由 `session:*` 运行态自生。
- **运行方式**:`kubectl -n esp-prod cp` 到 writing-py pod(pymysql+redis 已装)后 `python3 /tmp/redis_warmup.py`,支持 `--stage W2,W3a,W3b` / `--limit N` / `--flush`。
- **前置依赖**:users / user_mastery 种子须先灌入 TiDB(本脚本只读 TiDB,不写)。当前两表计数见 `docs/PROGRESS.md`。
- **TTL**:对齐 esp-cache 默认 `l2-ttl-seconds=300`(5min),压测前需重灌(或调长 TTL)。
