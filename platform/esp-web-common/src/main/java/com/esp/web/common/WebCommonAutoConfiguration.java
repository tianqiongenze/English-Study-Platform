package com.esp.web.common;

import org.springframework.beans.factory.ObjectProvider;
import org.springframework.beans.factory.config.ConfigurableBeanFactory;
import org.springframework.boot.autoconfigure.AutoConfiguration;
import org.springframework.boot.autoconfigure.condition.ConditionalOnMissingBean;
import org.springframework.boot.autoconfigure.condition.ConditionalOnWebApplication;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.core.env.Environment;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.util.StringUtils;

/**
 * esp-web-common 自动装配(经 META-INF/spring/...AutoConfiguration.imports 注册)。
 *
 * <p>装配两个共享 Bean:
 * <ul>
 *   <li>{@link GlobalExceptionHandler} —— 统一错误 envelope;包范围/路径白名单读
 *       {@code esp.web.common.base-packages} / {@code esp.web.common.exclude-path-prefixes}。</li>
 *   <li>{@link AppLogClient} —— 异步写 app_logs;service 名读 {@code esp.web.common.app-log.service}
 *       (缺省回退 spring.application.name),DB 连接默认主库 JdbcTemplate,可经
 *       {@code esp.web.common.app-log.jdbc-template-name} 指定命名 bean(如 learning 的 sharedJdbcTemplate)。</li>
 * </ul>
 *
 * <p>使用方在 application.yml 至少配置 {@code esp.web.common.base-packages} 指向自身控制器包
 * (bff=com.esp.bff / learning=com.esp.learning / teacher=com.esp.teacher),否则 advice 处理全部控制器。
 */
@AutoConfiguration
@ConditionalOnWebApplication(type = ConditionalOnWebApplication.Type.SERVLET)
@EnableConfigurationProperties(WebCommonProperties.class)
public class WebCommonAutoConfiguration {

    @Bean
    @ConditionalOnMissingBean(GlobalExceptionHandler.class)
    public GlobalExceptionHandler globalExceptionHandler(WebCommonProperties props) {
        return new GlobalExceptionHandler(props.getBasePackages(), props.getExcludePathPrefixes());
    }

    @Bean
    @ConditionalOnMissingBean(AppLogClient.class)
    public AppLogClient appLogClient(ObjectProvider<JdbcTemplate> jdbcTemplateProvider,
                                     WebCommonProperties props,
                                     ConfigurableBeanFactory beanFactory,
                                     Environment environment) {
        String service = StringUtils.hasText(props.getAppLog().getService())
                ? props.getAppLog().getService()
                : environment.getProperty("spring.application.name", "unknown");
        String sql = resolveInsertSql(props, environment);
        JdbcTemplate jt = resolveJdbcTemplate(jdbcTemplateProvider, props.getAppLog().getJdbcTemplateName(), beanFactory);
        if (jt == null && props.getAppLog().isEnabled()) {
            throw new IllegalStateException(
                    "esp-web-common: 未找到可用的 JdbcTemplate 来创建 AppLogClient。请配置数据源,"
                            + "或设置 esp.web.common.app-log.enabled=false 关闭应用日志。");
        }
        return new AppLogClient(jt, sql, service, props.getAppLog().isEnabled());
    }

    /**
     * 解析 app_logs INSERT SQL 模板:
     * 显式配置 {@code esp.web.common.app-log.sql} 优先;否则按 {@code esp.rw.storage.type}
     * 自动选 TiDB(CAST(? AS JSON))或 CockroachDB(CAST(? AS JSONB)),表名统一 esp_shared.app_logs。
     */
    private String resolveInsertSql(WebCommonProperties props, Environment environment) {
        if (StringUtils.hasText(props.getAppLog().getSql())) {
            return props.getAppLog().getSql();
        }
        String storageType = environment.getProperty("esp.rw.storage.type", "tidb");
        return "cockroachdb".equals(storageType)
                ? AppLogClient.CRDB_INSERT_SQL
                : AppLogClient.MYSQL_INSERT_SQL;
    }

    private JdbcTemplate resolveJdbcTemplate(ObjectProvider<JdbcTemplate> provider, String beanName,
                                             ConfigurableBeanFactory beanFactory) {
        if (StringUtils.hasText(beanName)) {
            return beanFactory.getBean(beanName, JdbcTemplate.class);
        }
        // 唯一/@Primary 的 JdbcTemplate;无或多候选(无 primary)时返回 null 由调用方决策。
        return provider.getIfUnique();
    }
}
