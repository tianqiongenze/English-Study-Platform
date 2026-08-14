-- Home-Tutor CRDB Schema(对齐 docs/design/schema.sql 的核心子集,用于 CRDB vs TiDB 对比压测)
-- 适配点:TiDB AUTO_RANDOM → CRDB unique_rowid()(有序唯一 BIGINT,写入友好);
--        ENUM → VARCHAR + CHECK(CRDB ENUM 对动态 DDL 不友好,压测用 VARCHAR 足够);
--        TINYINT → SMALLINT;DATETIME(3) → TIMESTAMP(3);JSON 保留(CRDB 原生支持);
--        ON UPDATE CURRENT_TIMESTAMP → CRDB 无此子句,由应用层/触发器补 updated_at(压测表不依赖)。

CREATE DATABASE IF NOT EXISTS esp;

\c esp;

-- 核心压测 4 表(对齐 #107 对比压测 plan:users / sessions / session_turns / learning_events)
CREATE TABLE IF NOT EXISTS users (
  id BIGINT NOT NULL PRIMARY KEY DEFAULT unique_rowid(),
  role VARCHAR(16) NOT NULL DEFAULT 'learner',
  name VARCHAR(64) NOT NULL,
  email VARCHAR(128) NOT NULL,
  pwd_hash VARCHAR(128) NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE INDEX uk_email (email)
);

CREATE TABLE IF NOT EXISTS scenarios (
  id BIGINT NOT NULL PRIMARY KEY DEFAULT unique_rowid(),
  code VARCHAR(32) NOT NULL,
  title VARCHAR(128) NOT NULL,
  system_prompt TEXT NOT NULL,
  difficulty SMALLINT NOT NULL DEFAULT 3,
  lang VARCHAR(8) NOT NULL DEFAULT 'en',
  UNIQUE INDEX uk_code (code)
);

CREATE TABLE IF NOT EXISTS sessions (
  id BIGINT NOT NULL PRIMARY KEY DEFAULT unique_rowid(),
  user_id BIGINT NOT NULL,
  scenario_id BIGINT NOT NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'active',
  started_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  ended_at TIMESTAMP NULL,
  INDEX idx_user (user_id)
);

CREATE TABLE IF NOT EXISTS session_turns (
  id BIGINT NOT NULL PRIMARY KEY DEFAULT unique_rowid(),
  session_id BIGINT NOT NULL,
  role VARCHAR(16) NOT NULL,
  content TEXT NOT NULL,
  audio_ref VARCHAR(256) NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_session (session_id)
);

CREATE TABLE IF NOT EXISTS learning_events (
  id BIGINT NOT NULL PRIMARY KEY DEFAULT unique_rowid(),
  user_id BIGINT NOT NULL,
  event_type VARCHAR(32) NOT NULL,
  payload JSON NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_user_time (user_id, created_at)
);

-- 多区域验证用:为后续 region 测试预留(单节点跑 default region)
-- 多节点落地时用:ALTER DATABASE esp CONFIGURE ZONE USING ... constraints/region。
