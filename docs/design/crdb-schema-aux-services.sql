-- ============================================================
-- ESP CRDB 辅助服务 schema(TiDB→CRDB 方言转换)
-- #140 全服务切 CRDB + #142 Go 统一数据访问层
--
-- 转换规则:
--   AUTO_RANDOM         → BIGINT DEFAULT unique_rowid() (分布式主键)
--   DATETIME/DATETIME(3)→ TIMESTAMPTZ
--   JSON                → JSONB
--   TINYINT             → INT2
--   VARBINARY           → BYTES
--   MEDIUMTEXT/TEXT     → STRING
--   ENGINE=... CHARSET  → 删除(CRDB 无此概念)
--   ON UPDATE CURRENT_TIMESTAMP → 删除(CRDB 不支持,updated_at 由应用维护)
--   enum('a','b')       → STRING + CHECK
--   迁移保留源 id(显式 INSERT id,unique_rowid() 仅兜底新行)
-- ============================================================

-- ---- 0. esp_speaking(库不存在,需新建;conversation-go 会话归档)----
CREATE DATABASE IF NOT EXISTS esp_speaking;

CREATE TABLE IF NOT EXISTS esp_speaking.scenarios (
  id            BIGINT      NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  code          STRING(32)  NOT NULL,
  title         STRING(128) NOT NULL,
  system_prompt STRING      NOT NULL,
  difficulty    INT2        NOT NULL DEFAULT 3,
  lang          STRING(8)   NOT NULL DEFAULT 'en',
  UNIQUE (code)
);

-- #140 决策:应用生成会话 ID(sess_<unixms>_<8hex>,见 gateway-go newID),故 id 列用 STRING 而非 BIGINT,
-- 与 Redis 会话 ID 同型,ListHistory/End 归档可直接存取。已建集群执行:
--   ALTER TABLE esp_speaking.sessions ALTER COLUMN id TYPE STRING;
CREATE TABLE IF NOT EXISTS esp_speaking.sessions (
  id          STRING    NOT NULL PRIMARY KEY,
  user_id     BIGINT    NOT NULL,
  scenario_id BIGINT    NOT NULL,
  status      STRING(16) NOT NULL DEFAULT 'active' CHECK (status IN ('active','ended')),
  started_at  TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  ended_at    TIMESTAMPTZ NULL,
  INDEX idx_user (user_id)
);

-- ---- 1. esp_pronunciation (pronunciation-rs) ----
CREATE TABLE IF NOT EXISTS esp_pronunciation.pronunciation_attempts (
  id             BIGINT       NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  user_id        BIGINT       NOT NULL,
  ref_type       STRING(32)   NOT NULL,
  ref_id         BIGINT       NOT NULL,
  audio_url      STRING(512)  NOT NULL,
  score          DECIMAL(5,2) NOT NULL,
  phoneme_scores JSONB        NULL,
  asr_text       STRING       NULL,
  duration_ms    INT          NOT NULL DEFAULT 0,
  created_at     TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_user_time (user_id, created_at),
  INDEX idx_ref (ref_type, ref_id)
);

-- ---- 2. esp_listening (listening-go) ----
CREATE TABLE IF NOT EXISTS esp_listening.listening_exercises (
  id            BIGINT       NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  title         STRING(128)  NOT NULL,
  audio_url     STRING(512)  NOT NULL,
  duration_sec  INT          NOT NULL DEFAULT 0,
  cefr_level    STRING(8)    NOT NULL,
  question_json JSONB        NOT NULL,
  scene_tags    JSONB        NULL,
  created_at    TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_cefr (cefr_level)
);

CREATE TABLE IF NOT EXISTS esp_listening.listening_attempts (
  id           BIGINT       NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  user_id      BIGINT       NOT NULL,
  exercise_id  BIGINT       NOT NULL,
  answers_json JSONB        NOT NULL,
  score        DECIMAL(5,2) NOT NULL,
  duration_ms  INT          NOT NULL DEFAULT 0,
  created_at   TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_user_time (user_id, created_at),
  INDEX idx_exercise (exercise_id)
);

-- ---- 3. esp_vocab (vocab-srs-go) ----
CREATE TABLE IF NOT EXISTS esp_vocab.vocabulary (
  id            BIGINT       NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  word          STRING(128)  NOT NULL,
  phonetic      STRING(128)  NULL,
  pos           STRING(16)   NULL,
  definition    STRING       NOT NULL,
  definition_zh STRING       NULL,
  examples_json JSONB        NULL,
  cefr_level    STRING(8)    NULL,
  UNIQUE (word),
  INDEX idx_cefr (cefr_level)
);

CREATE TABLE IF NOT EXISTS esp_vocab.user_vocabulary (
  id            BIGINT        NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  user_id       BIGINT        NOT NULL,
  word_id       BIGINT        NOT NULL,
  srs_stage     INT           NOT NULL DEFAULT 0,
  interval_days INT           NOT NULL DEFAULT 1,
  ease_factor   DECIMAL(4,2)  NOT NULL DEFAULT 2.50,
  due_at        TIMESTAMPTZ   NOT NULL,
  review_count  INT           NOT NULL DEFAULT 0,
  lapse_count   INT           NOT NULL DEFAULT 0,
  status        STRING(16)    NOT NULL DEFAULT 'learning',
  last_reviewed TIMESTAMPTZ   NULL,
  created_at    TIMESTAMPTZ   NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (user_id, word_id),
  INDEX idx_due (user_id, due_at)
);

-- ---- 4. esp_assessment (assessment-rs) ----
CREATE TABLE IF NOT EXISTS esp_assessment.assessment_sessions (
  id           BIGINT       NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  user_id      BIGINT       NOT NULL,
  goal         STRING(64)   NULL,
  cefr_result  STRING(8)    NULL,
  status       STRING(16)   NOT NULL DEFAULT 'in_progress',
  score        DECIMAL(5,2) NULL,
  started_at   TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
  finished_at  TIMESTAMPTZ  NULL,
  INDEX idx_user_time (user_id, started_at)
);

CREATE TABLE IF NOT EXISTS esp_assessment.assessment_answers (
  id            BIGINT       NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  session_id    BIGINT       NOT NULL,
  question_id   BIGINT       NOT NULL,
  answer_json   JSONB        NOT NULL,
  is_correct    INT2         NOT NULL DEFAULT 0,
  skill_tag     STRING(32)   NULL,
  created_at    TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_session (session_id)
);

-- ---- 5. esp_gamification (gamification-go) ----
CREATE TABLE IF NOT EXISTS esp_gamification.user_streaks (
  user_id      BIGINT       NOT NULL PRIMARY KEY,
  current_days INT          NOT NULL DEFAULT 0,
  longest_days INT          NOT NULL DEFAULT 0,
  last_active  DATE         NULL,
  updated_at   TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_last_active (last_active)
);

CREATE TABLE IF NOT EXISTS esp_gamification.achievements (
  id            BIGINT       NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  code          STRING(64)   NOT NULL,
  name          STRING(128)  NOT NULL,
  description   STRING(512)  NULL,
  icon_url      STRING(512)  NULL,
  criteria_json JSONB        NOT NULL,
  UNIQUE (code)
);

CREATE TABLE IF NOT EXISTS esp_gamification.user_achievements (
  id             BIGINT       NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  user_id        BIGINT       NOT NULL,
  achievement_id BIGINT       NOT NULL,
  unlocked_at    TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (user_id, achievement_id),
  INDEX idx_user (user_id)
);

-- ---- 6. esp_reading (reading-go) ----
CREATE TABLE IF NOT EXISTS esp_reading.reading_passages (
  id             BIGINT       NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  title          STRING(256)  NOT NULL,
  content        STRING       NOT NULL,
  word_count     INT          NOT NULL DEFAULT 0,
  cefr_level     STRING(8)    NOT NULL,
  source_type    STRING(32)   NULL,
  questions_json JSONB        NULL,
  created_at     TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_cefr (cefr_level)
);

CREATE TABLE IF NOT EXISTS esp_reading.reading_attempts (
  id           BIGINT       NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  user_id      BIGINT       NOT NULL,
  passage_id   BIGINT       NOT NULL,
  answers_json JSONB        NULL,
  score        DECIMAL(5,2) NULL,
  duration_ms  INT          NOT NULL DEFAULT 0,
  created_at   TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_user_time (user_id, created_at),
  INDEX idx_passage (passage_id)
);

-- ---- 7. esp_llm (llm-orchestrator-rs + dialogue) ----
CREATE TABLE IF NOT EXISTS esp_llm.llm_call_logs (
  id               BIGINT        NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  trace_id         STRING(64)    NULL,
  user_id          BIGINT        NULL,
  provider         STRING(32)    NOT NULL,
  model            STRING(64)    NOT NULL,
  scene            STRING(32)    NOT NULL,
  prompt_tokens    INT           NOT NULL DEFAULT 0,
  completion_tokens INT          NOT NULL DEFAULT 0,
  total_tokens     INT           NOT NULL DEFAULT 0,
  latency_ms       INT           NOT NULL DEFAULT 0,
  cost_usd         DECIMAL(10,6) NOT NULL DEFAULT 0,
  cache_hit        INT2          NOT NULL DEFAULT 0,
  status           STRING(16)    NOT NULL DEFAULT 'ok',
  error_msg        STRING(512)   NULL,
  created_at       TIMESTAMPTZ   NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_user_time (user_id, created_at),
  INDEX idx_scene_time (scene, created_at),
  INDEX idx_provider_time (provider, created_at)
);

CREATE TABLE IF NOT EXISTS esp_llm.llm_semantic_cache (
  id            BIGINT       NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  cache_key     STRING(128)  NOT NULL,
  embedding     BYTES        NULL,
  prompt        STRING       NOT NULL,
  response      STRING       NOT NULL,
  model         STRING(64)   NOT NULL,
  scene         STRING(32)   NOT NULL,
  hit_count     INT          NOT NULL DEFAULT 0,
  expires_at    TIMESTAMPTZ  NULL,
  created_at    TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (cache_key),
  INDEX idx_scene_expires (scene, expires_at)
);

CREATE TABLE IF NOT EXISTS esp_llm.dialogue_sessions (
  id            BIGINT       NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  user_id       BIGINT       NOT NULL,
  scenario_id   BIGINT       NULL,
  persona_id    BIGINT       NULL,
  cefr_level    STRING(8)    NULL,
  status        STRING(16)   NOT NULL DEFAULT 'active',
  created_at    TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
  ended_at      TIMESTAMPTZ  NULL,
  INDEX idx_user_time (user_id, created_at),
  INDEX idx_status (status)
);

CREATE TABLE IF NOT EXISTS esp_llm.dialogue_turns (
  id             BIGINT       NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  session_id     BIGINT       NOT NULL,
  role           STRING(16)   NOT NULL,
  content        STRING       NOT NULL,
  asr_confidence DECIMAL(5,2) NULL,
  tts_url        STRING(512)  NULL,
  tokens_in      INT          NOT NULL DEFAULT 0,
  tokens_out     INT          NOT NULL DEFAULT 0,
  latency_ms     INT          NOT NULL DEFAULT 0,
  created_at     TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_session_time (session_id, created_at)
);

CREATE TABLE IF NOT EXISTS esp_llm.dialogue_personas (
  id             BIGINT       NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  name           STRING(64)   NOT NULL,
  persona_prompt STRING       NOT NULL,
  voice_id       STRING(64)   NULL,
  accent         STRING(16)   NULL,
  cefr_level     STRING(8)    NULL,
  created_at     TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_cefr (cefr_level)
);

-- ---- 8. esp_writing 补表(writing_tasks/undo_log 已在 CRDB)----
CREATE TABLE IF NOT EXISTS esp_writing.writing_plans (
  id              BIGINT       NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  user_id         BIGINT       NOT NULL,
  weak_points_json JSONB       NOT NULL,
  plan_json       STRING       NOT NULL,
  status          STRING(16)   NOT NULL DEFAULT 'active',
  created_at      TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at      TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_user_time (user_id, created_at)
);

CREATE TABLE IF NOT EXISTS esp_writing.writing_progress (
  id         BIGINT       NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  user_id    BIGINT       NOT NULL,
  plan_id    BIGINT       NOT NULL,
  task_id    STRING(64)   NOT NULL,
  completed  INT2         NOT NULL DEFAULT 0,
  score      DECIMAL(5,1) NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_user_plan (user_id, plan_id, created_at)
);

CREATE TABLE IF NOT EXISTS esp_writing.writing_templates (
  id         BIGINT       NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  category   STRING(32)   NOT NULL,
  title      STRING(128)  NOT NULL,
  structure  STRING       NOT NULL,
  difficulty INT2         NOT NULL DEFAULT 2,
  created_at TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_category_diff (category, difficulty)
);

CREATE TABLE IF NOT EXISTS esp_writing.writing_essays (
  id         BIGINT       NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  category   STRING(32)   NOT NULL,
  title      STRING(128)  NOT NULL,
  content    STRING       NOT NULL,
  difficulty INT2         NOT NULL DEFAULT 2,
  word_count INT          NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_category_diff (category, difficulty)
);
