package com.esp.web.common;

import org.springframework.boot.context.properties.ConfigurationProperties;

import java.util.ArrayList;
import java.util.List;

/**
 * esp-web-common 自动配置属性({@code esp.web.common.*})。
 *
 * <pre>
 * esp:
 *   web:
 *     common:
 *       base-packages: com.esp.bff                # 逗号分隔,GlobalExceptionHandler 扫描包范围
 *       exclude-path-prefixes: /actuator          # 逗号分隔,路径前缀白名单(默认 /actuator)
 *       app-log:
 *         enabled: true                           # 总开关(默认 true)
 *         service: bff                            # app_logs.service 值(默认回退 spring.application.name)
 *         jdbc-template-name: sharedJdbcTemplate  # 可选:指定命名 JdbcTemplate(learning 用 esp_shared)
 *         sql: INSERT ... CAST(? AS JSONB)        # 可选:覆盖 app_logs INSERT SQL(默认按 storage.type 自动选 JSON/JSONB)
 * </pre>
 */
@ConfigurationProperties("esp.web.common")
public class WebCommonProperties {

    /** GlobalExceptionHandler 扫描包范围(逗号分隔);空=处理全部控制器。 */
    private List<String> basePackages = new ArrayList<>();

    /** 路径前缀白名单(逗号分隔);命中即不拦截,天然保护 actuator 探针/指标。 */
    private List<String> excludePathPrefixes = new ArrayList<>(List.of("/actuator"));

    private final AppLog appLog = new AppLog();

    public List<String> getBasePackages() {
        return basePackages;
    }

    public void setBasePackages(List<String> basePackages) {
        this.basePackages = basePackages;
    }

    public List<String> getExcludePathPrefixes() {
        return excludePathPrefixes;
    }

    public void setExcludePathPrefixes(List<String> excludePathPrefixes) {
        this.excludePathPrefixes = excludePathPrefixes;
    }

    public AppLog getAppLog() {
        return appLog;
    }

    public static class AppLog {

        /** 总开关。false 时 AppLogClient.log 直接 no-op。 */
        private boolean enabled = true;

        /** app_logs.service 值;空=回退 spring.application.name。 */
        private String service = "";

        /** 可选:指定 JdbcTemplate bean 名(如 sharedJdbcTemplate);空=注入主库 JdbcTemplate。 */
        private String jdbcTemplateName = "";

        /** 可选:覆盖 app_logs INSERT SQL 模板;空=按 esp.rw.storage.type 自动选 CAST(? AS JSON)/JSONB。 */
        private String sql = "";

        public boolean isEnabled() {
            return enabled;
        }

        public void setEnabled(boolean enabled) {
            this.enabled = enabled;
        }

        public String getService() {
            return service;
        }

        public void setService(String service) {
            this.service = service;
        }

        public String getJdbcTemplateName() {
            return jdbcTemplateName;
        }

        public void setJdbcTemplateName(String jdbcTemplateName) {
            this.jdbcTemplateName = jdbcTemplateName;
        }

        public String getSql() {
            return sql;
        }

        public void setSql(String sql) {
            this.sql = sql;
        }
    }
}
