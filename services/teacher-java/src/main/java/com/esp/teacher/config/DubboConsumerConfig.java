package com.esp.teacher.config;

import com.esp.api.dubbo.LearningRpcService;
import org.apache.dubbo.config.annotation.DubboReference;
import org.springframework.boot.autoconfigure.condition.ConditionalOnMissingBean;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.context.annotation.Primary;

/**
 * ADR-08 Dubbo Triple RPC consumer 配置。
 * @DubboReference 字段 learningRpcRef 是 Dubbo 生成的代理 bean;
 * learningRpcService() @Bean 加 @Primary 作为 TeacherService/TeacherController 构造注入的首选,
 * 测试环境(@WebMvcTest)通过 @MockBean(LearningRpcService.class) 覆盖此 bean(@ConditionalOnMissingBean 让路)。
 */
@Configuration
public class DubboConsumerConfig {

    @DubboReference(check = false, version = "1.0.0", group = "esp", timeout = 10000)
    private LearningRpcService learningRpcRef;

    @Primary
    @Bean
    @ConditionalOnMissingBean(LearningRpcService.class)
    public LearningRpcService learningRpcService() {
        return learningRpcRef;
    }
}
