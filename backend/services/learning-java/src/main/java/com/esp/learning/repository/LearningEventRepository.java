package com.esp.learning.repository;

import com.esp.cache.CacheService;
import com.esp.learning.domain.LearningEventEntity;
import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.Query;

import java.util.List;

public interface LearningEventRepository extends JpaRepository<LearningEventEntity, Long> {

    /** TiFlash 列存聚合:按 user 统计事件数(OLAP,带 tiflash hint)。 */
    @Query(value = "select /*+ read_from_storage(tiflash[learning_events]) */ user_id, count(*) " +
            "from learning_events group by user_id", nativeQuery = true)
    List<Object[]> aggregateByUser();

    /**
     * 聚合查询的多级缓存包装(ADR-多级缓存)。
     *
     * <p>key learning:agg:users,长 TTL(30min,低频重查询)。
     * 失效由 {@code DbLearningEventConsumer} 落库后触发:
     * {@code cacheService.evict("learning:agg", "learning:agg:users")}。
     *
     * <p>default 方法在 Spring Data JPA 接口上,运行期由 Spring 生成代理实例调用。
     * 调用方注入 CacheService 后传入即可,接口本身不持有 CacheService 字段
     * (JPA 代理无法注入字段),故采用参数传入。
     */
    default List<Object[]> aggregateByUserCached(CacheService cacheService) {
        return cacheService.getOrLoad(
                "learning:agg",
                "learning:agg:users",
                () -> aggregateByUser(),
                1800L, // 30min
                false);
    }
}
