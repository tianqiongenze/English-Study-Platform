# P1-6 缓存 key 统一 + user_id 透传 — 探查地图(2026-08-03)

> esp-explorer 产出。实现待 RS256/langgraph/Pact 收尾后派 esp-builder(避免 learning-java/writing-py 同模块并发构建竞态)。

## A. 缓存 key 割裂(确认)

| 端 | 生成方式 | 实际 Redis key 示例 | 代码位置 |
|---|---|---|---|
| Java (esp-cache) | 明文,调用方传完整 key | `learning:due:42` | `CacheService.java:86`(`getOrLoad(cacheName,key,...)`) |
| Python (esp-cache-py) | `{cache_name}:{sha256(json(parts))[:24]}` | `learning:due:a3f8b7c9...` | `esp_cache/__init__.py:59-61`(`_key`) |
| Go (gateway-go) | `{cacheName}:{sha256(json(parts))[:24]}` | `learning:due:a3f8b7c9...` | `src/cache/cache.go:133-136`(`keyOf`) |

Java 调用方拼 key 示例:`LearningService.java:68` `"learning:due:"+userId`;`AuthController.java:65` `"user:"+id`。

**影响**:跨语言读对方缓存必 miss(明文 vs hash 不重叠)。当前各端自给自足未爆雷,定时炸弹。

**修复方向**:Python/Go 对齐 Java 明文 key(改 `_key`/`keyOf` 为 `{cache_name}:{part1}:{part2}`,去掉 sha256;并统一 cache_name 命名)。Java 改动面大(所有调用方),不选。

## B. user_id 透传断点(5 处)

完整链路图:
```
BFF (X-User-Id 注入 OK,InletDisruptor 透传所有 header)
  → writing-py grade (body 有 user_id 字段但默认0,未注入事件)  [断点1]
    → publish_writing_graded({"status":"graded"}) 无 user_id    [断点1]
      → teacher-java 收 writing.graded,只 log 不回写            [断点2]
        → (缺失) teacher 回写 mastery 桥接                       [断点5]

(此路通)teacher.autograde.requested → teacher.doAutoGrade → teacher.finalized(userId) → learning upsert mastery
```

| # | 位置 | 问题 | 修复 |
|---|---|---|---|
| 1 | `writing-py/app/main.py:43` + `schemas.py:15` | writing.graded payload 无 user_id;GradeRequest.user_id 默认0 未用 | main.py 构造 payload 时注入 `GradeRequest.user_id`;WritingGradedEvent 加 userId 字段 |
| 2 | `teacher-java TeacherEventConsumer.java:110-112` | 收 writing.graded 仅 log,mastery 回写未实现 | 实现回写:调 TeacherService/LearningService 更新 mastery |
| 3 | `learning-java LearningController.java:23,34,45` | 不读 X-User-Id header,依赖 body/param user_id | 加 `@RequestHeader(value="X-User-Id", required=false)` 作为 fallback/优先源 |
| 4 | 全下游 | BFF 透传 X-User-Id 无人消费 | 配合断点3 消费 |
| 5 | writing.graded → teacher.finalized | 两事件链未桥接 | writing 批改结果驱动 mastery:writing.graded(含userId)→ teacher 回写(断点2 实现)即可,不必新事件 |

**关键事实**:teacher.autograde 链路(finalized 含 userId → learning mastery)已通;缺的是 writing.graded 驱动 mastery 这条。修复断点1+2 即可打通 writing→mastery。断点3+4 是 X-User-Id header 消费增强(让 learning 端点不依赖调用方显式传 user_id)。

## 实现范围(给 builder)
- esp-cache-py `_key` + gateway-go `keyOf`:改明文 join,去 sha256。
- writing-py main.py:43 + WritingGradedEvent 加 userId。
- teacher-java TeacherEventConsumer:实现 writing.graded→mastery 回写。
- learning-java LearningController:加 X-User-Id header 读取。
- 各端单测适配 + 跑 mvn test / pytest / go test。
