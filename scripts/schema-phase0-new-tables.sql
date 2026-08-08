-- =============================================================================
-- Phase 0 零风险建表:13 张新服务表按 7 schema 建立
-- 依据:docs/design/adr-schema-per-service.md §3 表→schema 映射、§5 Phase 0
-- 说明:
--   - 新服务窗口期成本最低、收益最大:建 schema + 建表,不动任何老表。
--   - esp_learning 作为独立 DB 建立(学情高频写,为 Phase 3 物理隔离预留);
--     其内部表(user_mastery/knowledge_points/learning_events/session_turns)
--     迁入属 Phase 1,本脚本只建库不建/不迁老表。
--   - 所有表保持原 DDL 风格:BIGINT AUTO_RANDOM 主键 + DATETIME(3) + 索引。
--   - 执行环境:TiDB(MySQL 协议兼容),tidb-primary:4000 root 无密码。
-- =============================================================================

-- -----------------------------------------------------------------------------
-- 0. 独立 DB:esp_learning(学情高频写,Phase 3 物理隔离预留)
--    Phase 1 将从老 esp 库迁入:user_mastery, knowledge_points,
--    learning_events, session_turns(本脚本只建库,不建/不迁老表)。
-- -----------------------------------------------------------------------------
CREATE DATABASE IF NOT EXISTS esp_learning;

-- -----------------------------------------------------------------------------
-- 1. 7 个新服务 schema(逻辑隔离,不拆 database)
-- -----------------------------------------------------------------------------
CREATE SCHEMA IF NOT EXISTS esp_pronunciation;  -- S1 pronunciation-rs
CREATE SCHEMA IF NOT EXISTS esp_listening;      -- S2 listening-rs
CREATE SCHEMA IF NOT EXISTS esp_vocab;          -- S3 vocab-srs-go
CREATE SCHEMA IF NOT EXISTS esp_assessment;     -- S4 assessment-rs
CREATE SCHEMA IF NOT EXISTS esp_gamification;   -- S5 gamification-go
CREATE SCHEMA IF NOT EXISTS esp_reading;        -- S6 reading-go
CREATE SCHEMA IF NOT EXISTS esp_llm;            -- S7 llm-orchestrator-rs

-- =============================================================================
-- S1 发音评分(pronunciation-rs)→ esp_pronunciation
-- =============================================================================
CREATE TABLE IF NOT EXISTS esp_pronunciation.pronunciation_attempts (
  id BIGINT NOT NULL AUTO_RANDOM PRIMARY KEY,
  user_id BIGINT NOT NULL,
  target_text TEXT NOT NULL,
  audio_uri VARCHAR(256) NOT NULL,          -- 对象存储引用
  overall_score DECIMAL(5,2) NOT NULL,      -- 综合分 0-100
  phoneme_scores JSON NOT NULL,             -- 音素级 [{phoneme, score, expected, actual}]
  fluency_score DECIMAL(5,2),               -- 流利度
  completeness_score DECIMAL(5,2),          -- 完整度
  pronunciation_score DECIMAL(5,2),         -- 发音分
  feedback JSON,                            -- 纠错建议(重音/语调/最小对立体)
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  INDEX idx_user (user_id, created_at)
);

-- =============================================================================
-- S2 听力训练(listening-rs)→ esp_listening
-- =============================================================================
CREATE TABLE IF NOT EXISTS esp_listening.listening_exercises (
  id BIGINT NOT NULL AUTO_RANDOM PRIMARY KEY,
  level VARCHAR(16) NOT NULL,               -- CEFR A1-C2
  category VARCHAR(32) NOT NULL,            -- news/conversation/lecture/exam
  audio_uri VARCHAR(256) NOT NULL,
  transcript TEXT NOT NULL,
  questions JSON NOT NULL,                  -- 题目+选项
  duration_sec INT NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  INDEX idx_level_cat (level, category)
);
CREATE TABLE IF NOT EXISTS esp_listening.listening_attempts (
  id BIGINT NOT NULL AUTO_RANDOM PRIMARY KEY,
  user_id BIGINT NOT NULL,
  exercise_id BIGINT NOT NULL,
  answers JSON NOT NULL,
  score DECIMAL(5,2),
  mode VARCHAR(16) NOT NULL,                -- intensive/extensive/dictation
  time_spent_sec INT,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  INDEX idx_user (user_id, created_at),
  INDEX idx_exercise (exercise_id)
);

-- =============================================================================
-- S3 词汇记忆(vocab-srs-go,SM-2+ 算法)→ esp_vocab
-- =============================================================================
CREATE TABLE IF NOT EXISTS esp_vocab.vocabulary (
  id BIGINT NOT NULL AUTO_RANDOM PRIMARY KEY,
  word VARCHAR(64) NOT NULL,
  pos VARCHAR(16),                          -- 词性
  definition TEXT NOT NULL,
  phonetic VARCHAR(128),
  frequency_rank INT,                       -- 词频(COCA/BNC)
  cefr VARCHAR(8),                          -- A1-C2
  root VARCHAR(64),                         -- 词根
  affix VARCHAR(64),                        -- 词缀
  example TEXT,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  UNIQUE INDEX uk_word (word),
  INDEX idx_freq (frequency_rank),
  INDEX idx_cefr (cefr)
);
CREATE TABLE IF NOT EXISTS esp_vocab.user_vocabulary (
  id BIGINT NOT NULL AUTO_RANDOM PRIMARY KEY,
  user_id BIGINT NOT NULL,
  word_id BIGINT NOT NULL,
  srs_interval INT NOT NULL DEFAULT 1,      -- SRS 间隔(天)
  srs_ease FLOAT NOT NULL DEFAULT 2.5,      -- 难度系数(SM-2 ease)
  due_at DATETIME(3) NOT NULL,              -- 下次到期
  review_count INT NOT NULL DEFAULT 0,
  lapse_count INT NOT NULL DEFAULT 0,       -- 遗忘次数
  last_review_at DATETIME(3),
  mastery_score DECIMAL(5,2) NOT NULL DEFAULT 0,
  UNIQUE INDEX uk_user_word (user_id, word_id),
  INDEX idx_due (user_id, due_at)
);

-- =============================================================================
-- S4 能力测评(assessment-rs,CEFR/雅思/托福模考 + IRT)→ esp_assessment
-- =============================================================================
CREATE TABLE IF NOT EXISTS esp_assessment.assessment_sessions (
  id BIGINT NOT NULL AUTO_RANDOM PRIMARY KEY,
  user_id BIGINT NOT NULL,
  test_type VARCHAR(32) NOT NULL,           -- cefr/ielts/toefl/cet4/cet6
  status VARCHAR(16) NOT NULL DEFAULT 'in_progress',
  started_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  completed_at DATETIME(3),
  overall_score DECIMAL(6,2),
  skill_scores JSON,                        -- {listening, reading, speaking, writing}
  irt_theta FLOAT,                          -- IRT 能力参数 θ
  report JSON,                              -- 详细能力报告 + 弱项诊断
  INDEX idx_user (user_id, started_at),
  INDEX idx_status (status)
);
CREATE TABLE IF NOT EXISTS esp_assessment.assessment_answers (
  id BIGINT NOT NULL AUTO_RANDOM PRIMARY KEY,
  session_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  item_id VARCHAR(64) NOT NULL,             -- 题目 ID
  answer TEXT,
  correct TINYINT,
  irt_theta FLOAT,
  responded_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  INDEX idx_session (session_id),
  INDEX idx_user (user_id)
);

-- =============================================================================
-- S5 游戏化(gamification-go)→ esp_gamification
-- =============================================================================
CREATE TABLE IF NOT EXISTS esp_gamification.user_streaks (
  id BIGINT NOT NULL AUTO_RANDOM PRIMARY KEY,
  user_id BIGINT NOT NULL UNIQUE,
  current_streak INT NOT NULL DEFAULT 0,
  longest_streak INT NOT NULL DEFAULT 0,
  last_active_date DATE NOT NULL,
  total_xp BIGINT NOT NULL DEFAULT 0,
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  INDEX idx_streak (current_streak DESC)
);
CREATE TABLE IF NOT EXISTS esp_gamification.achievements (
  id BIGINT NOT NULL AUTO_RANDOM PRIMARY KEY,
  code VARCHAR(64) NOT NULL UNIQUE,
  title VARCHAR(128) NOT NULL,
  description TEXT,
  icon VARCHAR(256),
  xp_reward INT NOT NULL DEFAULT 0,
  criteria JSON NOT NULL                    -- 达成条件
);
CREATE TABLE IF NOT EXISTS esp_gamification.user_achievements (
  id BIGINT NOT NULL AUTO_RANDOM PRIMARY KEY,
  user_id BIGINT NOT NULL,
  achievement_id BIGINT NOT NULL,
  unlocked_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  UNIQUE INDEX uk_user_ach (user_id, achievement_id),
  INDEX idx_user (user_id)
);

-- =============================================================================
-- S6 阅读训练(reading-go)→ esp_reading
-- =============================================================================
CREATE TABLE IF NOT EXISTS esp_reading.reading_passages (
  id BIGINT NOT NULL AUTO_RANDOM PRIMARY KEY,
  level VARCHAR(16) NOT NULL,               -- CEFR
  category VARCHAR(32) NOT NULL,
  title VARCHAR(256) NOT NULL,
  content TEXT NOT NULL,
  word_count INT NOT NULL,
  questions JSON NOT NULL,                  -- 题目+选项+答案+解析
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  INDEX idx_level_cat (level, category)
);
CREATE TABLE IF NOT EXISTS esp_reading.reading_attempts (
  id BIGINT NOT NULL AUTO_RANDOM PRIMARY KEY,
  user_id BIGINT NOT NULL,
  passage_id BIGINT NOT NULL,
  answers JSON NOT NULL,
  score DECIMAL(5,2),
  time_spent_sec INT,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  INDEX idx_user (user_id, created_at),
  INDEX idx_passage (passage_id)
);

-- =============================================================================
-- S7 LLM 统一网关(llm-orchestrator-rs,调用日志 + 成本追踪)→ esp_llm
-- =============================================================================
CREATE TABLE IF NOT EXISTS esp_llm.llm_call_logs (
  id BIGINT NOT NULL AUTO_RANDOM PRIMARY KEY,
  user_id BIGINT,
  service VARCHAR(64) NOT NULL,             -- 调用方服务
  provider VARCHAR(32) NOT NULL,            -- anthropic/openai/...
  model VARCHAR(64) NOT NULL,
  prompt_tokens INT,
  completion_tokens INT,
  latency_ms INT,
  cost_micros BIGINT,                       -- 成本(微美元)
  status VARCHAR(16) NOT NULL,              -- ok/fallback/rate_limited/error
  cache_hit TINYINT NOT NULL DEFAULT 0,
  trace_id VARCHAR(64),
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  INDEX idx_user_time (user_id, created_at),
  INDEX idx_service_time (service, created_at),
  INDEX idx_status (status, created_at)
);

-- =============================================================================
-- Phase 1 迁入预告(esp_learning 独立 DB,本脚本只建库不迁表)
-- -----------------------------------------------------------------------------
-- 以下 4 张老表当前在老 esp 库,Phase 1 将以 TiDB RENAME TABLE(在线、秒级、
-- 不锁表)从 esp 迁入 esp_learning,本脚本不执行 RENAME。
--   1. user_mastery      → esp_learning.user_mastery      (用户掌握度主表)
--   2. knowledge_points  → esp_learning.knowledge_points  (知识点字典)
--   3. learning_events   → esp_learning.learning_events   (学情事件;§4 治理后
--                            learning 独占,emotion 改发 Pulsar 事件由 learning 消费落库)
--   4. session_turns     → esp_learning.session_turns     (会话轮次明细)
-- 迁移顺序遵循 ADR §5 Phase 1:bff→gateway→writing→teacher→learning,
-- 每批一个服务验证后再进下一批。
-- =============================================================================
