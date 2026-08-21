package com.esp.teacher.repository;

import com.esp.teacher.domain.TeacherEventOutbox;
import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.Query;
import org.springframework.data.repository.query.Param;

import java.time.Instant;
import java.util.List;

/**
 * 教师事件 outbox 仓储(P0-D1,2026-08-21)。
 *
 * <p>{@link com.esp.teacher.outbox.OutboxRelay} 用 {@link #findPending(int, Instant)} 扫表投递。
 */
public interface TeacherEventOutboxRepository extends JpaRepository<TeacherEventOutbox, Long> {

    /** 扫 pending/failed 且到下次重试时间的行,限量。 */
    @Query("select o from TeacherEventOutbox o " +
           "where o.status in ('pending','failed') and o.retryCount < 8 and o.nextRetryAt <= :now " +
           "order by o.id asc")
    List<TeacherEventOutbox> findPending(@Param("now") Instant now);
}
