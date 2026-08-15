package com.esp.web.common;

import org.junit.jupiter.api.Test;
import org.springframework.jdbc.core.JdbcTemplate;

import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.verifyNoInteractions;

/**
 * AppLogClient 单测:mock JdbcTemplate,断言异步落库(INSERT app_logs)与降级行为。
 */
class AppLogClientTest {

    private static final String SQL = AppLogClient.MYSQL_INSERT_SQL;
    private static final String CRDB_SQL = AppLogClient.CRDB_INSERT_SQL;

    @Test
    void log_asyncInsertsIntoAppLogs_withDefaultService() {
        JdbcTemplate jdbc = mock(JdbcTemplate.class);
        AppLogClient client = new AppLogClient(jdbc, "test-svc", true);

        client.log("INFO", "behavior.click", 42L, Map.of("action", "click"));

        verify(jdbc, org.mockito.Mockito.timeout(3000)).update(
                eq(SQL), any(), eq("test-svc"), eq("INFO"), eq("behavior.click"), any(), eq(42L), any());
    }

    @Test
    void log_withExplicitService_overridesDefault() {
        JdbcTemplate jdbc = mock(JdbcTemplate.class);
        AppLogClient client = new AppLogClient(jdbc, "test-svc", true);

        client.log("learning", "INFO", "behavior.click", 1L, null);

        verify(jdbc, org.mockito.Mockito.timeout(3000)).update(
                eq(SQL), any(), eq("learning"), eq("INFO"), eq("behavior.click"), any(), eq(1L), any());
    }

    @Test
    void crdbSqlTemplate_usesJsonbCastAndQualifiedTable() {
        JdbcTemplate jdbc = mock(JdbcTemplate.class);
        AppLogClient client = new AppLogClient(jdbc, CRDB_SQL, "test-svc", true);

        client.log("INFO", "behavior.click", 42L, Map.of("action", "click"));

        // CRDB(PG wire):CAST(? AS JSONB) 且表名两段式 esp_shared.app_logs
        org.junit.jupiter.api.Assertions.assertTrue(
                CRDB_SQL.contains("CAST(? AS JSONB)"), "CRDB 模板应为 JSONB 转型");
        org.junit.jupiter.api.Assertions.assertTrue(
                CRDB_SQL.contains("esp_shared.app_logs"), "CRDB 模板应为 esp_shared.app_logs 两段式");
        org.junit.jupiter.api.Assertions.assertTrue(
                SQL.contains("CAST(? AS JSON)"), "MySQL 模板应为 JSON 转型");
        org.junit.jupiter.api.Assertions.assertFalse(
                SQL.contains("JSONB"), "MySQL 模板不得含 JSONB");
        verify(jdbc, org.mockito.Mockito.timeout(3000)).update(
                eq(CRDB_SQL), any(), eq("test-svc"), eq("INFO"), eq("behavior.click"), any(), eq(42L), any());
    }

    @Test
    void log_whenDisabled_noInsert() {
        JdbcTemplate jdbc = mock(JdbcTemplate.class);
        AppLogClient client = new AppLogClient(jdbc, "test-svc", false);

        client.log("INFO", "behavior.click", 1L, Map.of());

        verifyNoInteractions(jdbc);
    }

    @Test
    void log_nullJdbc_noop() {
        AppLogClient client = new AppLogClient(null, "test-svc", true);
        client.log("INFO", "behavior.click", 1L, Map.of());
        // 不抛异常,直接 no-op
    }

    @Test
    void droppedCount_startsAtZero() {
        AppLogClient client = new AppLogClient(mock(JdbcTemplate.class), "test-svc", true);
        assertEquals(0, client.getDroppedCount());
    }
}
