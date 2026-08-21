package com.esp.teacher;

import org.apache.dubbo.config.spring.context.annotation.EnableDubbo;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.scheduling.annotation.EnableScheduling;

@SpringBootApplication(scanBasePackages = {"com.esp.teacher", "com.esp.cache", "com.esp.common"})
@EnableDubbo
@EnableScheduling // P0-D1(2026-08-21):OutboxRelay @Scheduled 扫表重投 outbox 行
public class TeacherApplication {
    public static void main(String[] args) {
        SpringApplication.run(TeacherApplication.class, args);
    }
}
