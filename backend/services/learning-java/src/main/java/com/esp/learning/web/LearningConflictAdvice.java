package com.esp.learning.web;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.core.Ordered;
import org.springframework.core.annotation.Order;
import org.springframework.dao.DataIntegrityViolationException;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.orm.ObjectOptimisticLockingFailureException;
import org.springframework.web.bind.annotation.ExceptionHandler;
import org.springframework.web.bind.annotation.RestControllerAdvice;

import java.util.Map;

/**
 * learning 域冲突映射(幂等语义)。
 *
 * <p>为什么独立命名 + 最高优先级:esp-web-common 共享包注册了同 simple-name 的
 * {@code GlobalExceptionHandler}(bean 名默认同为 globalExceptionHandler),自动装配 @Bean
 * 后注册会按名覆盖组件扫描到的 advice,导致应用侧 409 兜底失效 → 并发/重复定级裸 500
 * (人工抽测 #4 / StrictMode 双发)。改名避开 bean 覆盖,并以 @Order(HIGHEST_PRECEDENCE)
 * 确保本域 advice 优先于共享 advice 处理冲突异常。
 *
 * <p>乐观锁/主键冲突统一 409(幂等语义);其余未捕获异常交回共享 advice 保持默认 500。
 */
@RestControllerAdvice
@Order(Ordered.HIGHEST_PRECEDENCE)
public class LearningConflictAdvice {

    private static final Logger log = LoggerFactory.getLogger(LearningConflictAdvice.class);

    @ExceptionHandler({ObjectOptimisticLockingFailureException.class, DataIntegrityViolationException.class})
    public ResponseEntity<Map<String, Object>> conflict(RuntimeException ex) {
        log.warn("learning conflict(dedup/idempotency): {}", ex.toString());
        return ResponseEntity.status(HttpStatus.CONFLICT)
                .body(Map.of("status", 409, "error", "conflict", "message", "重复初始化已幂等处理,请刷新重试"));
    }
}
