-- ============================================================
-- ESP schema-per-service Phase 1 DDL(老表在线迁移)
-- 关联: docs/design/adr-schema-per-service.md §5 Phase 1
-- 原则: TiDB RENAME TABLE 在线操作(秒级不锁表),但服务连接串仍指向 esp 库,
--       RENAME 后服务查旧表名会 1146。故 Phase 1 必须串行:
--         RENAME 一批 → 改对应服务 JDBC URL(指向新 schema)→ 重启 → e2e 验证。
--       顺序: bff(users) → gateway(scenarios/sessions) → writing(5表) → teacher(4表) → learning(4表)。
-- 风险: 不可逆;影响 10 个在跑服务 + seata undo_log;30 亿数据迁移名而非数据。
-- 执行窗口: 维护窗口(可接受短暂断服),单节点 VM 串行重启 5 批。
-- 日期: 2026-08-07
-- 状态: 待执行(脚本就绪,执行前需确认维护窗口 + 各服务连接串改造就绪)
-- ============================================================

-- 注意:每批 RENAME 前确认目标 schema 已建(Phase 0 已建 esp_auth/esp_speaking/esp_writing/esp_teaching/esp_learning/esp_shared)。
-- 注意:undo_log 留在 esp(seata 全局事务表,不迁)。

-- ---- Batch 1: bff-java → esp_auth ----
-- users(鉴权主表)
RENAME TABLE esp.users TO esp_auth.users;

-- ---- Batch 2: gateway-go → esp_speaking ----
-- scenarios, sessions(会话与场景入口)
RENAME TABLE esp.scenarios TO esp_speaking.scenarios;
RENAME TABLE esp.sessions TO esp_speaking.sessions;

-- ---- Batch 3: writing-py + writing-coach → esp_writing ----
-- writing_tasks, writing_templates, writing_essays, writing_plans, writing_progress
RENAME TABLE esp.writing_tasks TO esp_writing.writing_tasks;
RENAME TABLE esp.writing_templates TO esp_writing.writing_templates;
RENAME TABLE esp.writing_essays TO esp_writing.writing_essays;
RENAME TABLE esp.writing_plans TO esp_writing.writing_plans;
RENAME TABLE esp.writing_progress TO esp_writing.writing_progress;

-- ---- Batch 4: teacher-java → esp_teaching ----
-- classes, assignments, submissions, lesson_plans
RENAME TABLE esp.classes TO esp_teaching.classes;
RENAME TABLE esp.assignments TO esp_teaching.assignments;
RENAME TABLE esp.submissions TO esp_teaching.submissions;
RENAME TABLE esp.lesson_plans TO esp_teaching.lesson_plans;

-- ---- Batch 5: learning-java → esp_learning ----
-- user_mastery, knowledge_points, learning_events(ADR §4 归 learning 独占), session_turns
RENAME TABLE esp.user_mastery TO esp_learning.user_mastery;
RENAME TABLE esp.knowledge_points TO esp_learning.knowledge_points;
RENAME TABLE esp.learning_events TO esp_learning.learning_events;
RENAME TABLE esp.session_turns TO esp_learning.session_turns;

-- ---- 公共: app_logs → esp_shared(6 服务共写,保留)----
RENAME TABLE esp.app_logs TO esp_shared.app_logs;

-- ---- 验证(执行后跑)----
-- SELECT TABLE_SCHEMA, TABLE_NAME FROM information_schema.tables
--   WHERE TABLE_SCHEMA IN ('esp_auth','esp_speaking','esp_writing','esp_teaching','esp_learning','esp_shared')
--   ORDER BY TABLE_SCHEMA, TABLE_NAME;
-- esp 库应只剩 undo_log(+ 任何未迁表)。

-- ============================================================
-- 配套:每批 RENAME 后改服务连接串(示例,teacher Batch 4)
-- ============================================================
-- 旧: WRITE_DB_URL=jdbc:mysql://tidb-primary:4000/esp?useSSL=false&allowPublicKeyRetrieval=true
-- 新: WRITE_DB_URL=jdbc:mysql://tidb-primary:4000/esp_teaching?useSSL=false&allowPublicKeyRetrieval=true
--     READ_DB_URL=jdbc:mysql://tidb-follower:4000/esp_teaching?...
-- 改 deploy env → kubectl -n esp-prod set env deploy/teacher-java WRITE_DB_URL=... READ_DB_URL=... → rollout restart
-- gateway-go: WRITE_DB_NAME/READ_DB_NAME 改 esp_speaking(需确认 gateway 代码用 DB_NAME env 而非 URL)
-- writing-py: DB URL 改 esp_writing
-- bff-java: DB URL 改 esp_auth
-- learning-java: DB URL 改 esp_learning
-- 注意:app_logs 6 服务共写 esp_shared,这些服务的 app_logs 写入路径需改 esp_shared(或保留 esp 共享别名)。
