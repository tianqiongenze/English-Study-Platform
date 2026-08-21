-- Home-Tutor TiDB Schema (MySQL 协议兼容)
SET NAMES utf8mb4;

CREATE TABLE IF NOT EXISTS users (
  id BIGINT NOT NULL PRIMARY KEY AUTO_RANDOM,
  role ENUM('learner','teacher','admin') NOT NULL DEFAULT 'learner',
  name VARCHAR(64) NOT NULL,
  email VARCHAR(128) NOT NULL,
  pwd_hash VARCHAR(128) NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_email (email)
);

CREATE TABLE IF NOT EXISTS scenarios (
  id BIGINT NOT NULL PRIMARY KEY AUTO_RANDOM,
  code VARCHAR(32) NOT NULL,
  title VARCHAR(128) NOT NULL,
  system_prompt TEXT NOT NULL,
  difficulty TINYINT NOT NULL DEFAULT 3,
  lang VARCHAR(8) NOT NULL DEFAULT 'en',
  UNIQUE KEY uk_code (code)
);

CREATE TABLE IF NOT EXISTS sessions (
  id BIGINT NOT NULL PRIMARY KEY AUTO_RANDOM,
  user_id BIGINT NOT NULL,
  scenario_id BIGINT NOT NULL,
  status ENUM('active','ended') NOT NULL DEFAULT 'active',
  started_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  ended_at TIMESTAMP NULL,
  KEY idx_user (user_id)
);

CREATE TABLE IF NOT EXISTS session_turns (
  id BIGINT NOT NULL PRIMARY KEY AUTO_RANDOM,
  session_id BIGINT NOT NULL,
  role ENUM('user','ai','system') NOT NULL,
  content TEXT NOT NULL,
  audio_ref VARCHAR(256) NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_session (session_id)
);

CREATE TABLE IF NOT EXISTS knowledge_points (
  id BIGINT NOT NULL PRIMARY KEY AUTO_RANDOM,
  code VARCHAR(32) NOT NULL,
  title VARCHAR(128) NOT NULL,
  parent_id BIGINT NULL,
  difficulty TINYINT NOT NULL DEFAULT 3,
  UNIQUE KEY uk_code (code)
);

CREATE TABLE IF NOT EXISTS user_mastery (
  user_id BIGINT NOT NULL,
  kp_id BIGINT NOT NULL,
  mastery_score DECIMAL(5,2) NOT NULL DEFAULT 0,
  last_review_at TIMESTAMP NULL,
  review_count INT NOT NULL DEFAULT 0,
  next_review_at TIMESTAMP NULL,
  version INT NOT NULL DEFAULT 0,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, kp_id)
);

CREATE TABLE IF NOT EXISTS learning_events (
  id BIGINT NOT NULL PRIMARY KEY AUTO_RANDOM,
  user_id BIGINT NOT NULL,
  event_type VARCHAR(32) NOT NULL,
  payload JSON NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_user_time (user_id, created_at)
);

CREATE TABLE IF NOT EXISTS writing_tasks (
  id BIGINT NOT NULL PRIMARY KEY AUTO_RANDOM,
  user_id BIGINT NOT NULL,
  prompt TEXT NOT NULL,
  essay TEXT NOT NULL,
  score DECIMAL(5,2) NULL,
  feedback JSON NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_user (user_id)
);

CREATE TABLE IF NOT EXISTS lesson_plans (
  id BIGINT NOT NULL PRIMARY KEY AUTO_RANDOM,
  teacher_id BIGINT NOT NULL,
  topic VARCHAR(128) NOT NULL,
  content JSON NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_teacher (teacher_id)
);

CREATE TABLE IF NOT EXISTS classes (
  id BIGINT NOT NULL PRIMARY KEY AUTO_RANDOM,
  teacher_id BIGINT NOT NULL,
  name VARCHAR(64) NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS assignments (
  id BIGINT NOT NULL PRIMARY KEY AUTO_RANDOM,
  class_id BIGINT NOT NULL,
  type ENUM('oral','written') NOT NULL,
  content TEXT NOT NULL,
  due_at TIMESTAMP NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_class (class_id)
);

CREATE TABLE IF NOT EXISTS submissions (
  id BIGINT NOT NULL PRIMARY KEY AUTO_RANDOM,
  assignment_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  content TEXT NOT NULL,
  auto_score DECIMAL(5,2) NULL,
  auto_feedback JSON NULL,
  final_score DECIMAL(5,2) NULL,
  status ENUM('pending','auto_graded','finalized') NOT NULL DEFAULT 'pending',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_assignment (assignment_id)
);

-- ========== 结构化业务日志表(ADR-observability-rebuild)==========
-- 双轨:原始日志→Loki(仅标签索引);结构化日志→本表(TiDB SQL 可聚合分析)。
-- 写主(tidb-primary),分析读走 follower(tidb-follower)。降级:写失败仅 warn 不阻断业务。
CREATE TABLE IF NOT EXISTS app_logs (
  id BIGINT NOT NULL AUTO_RANDOM PRIMARY KEY,
  ts DATETIME(3) NOT NULL,
  service VARCHAR(32) NOT NULL,      -- bff/learning/teacher/writing/emotion/gateway
  level VARCHAR(8) NOT NULL,          -- INFO/WARN/ERROR
  event VARCHAR(64) NOT NULL,         -- auth.login/writing.grade/learning.event/behavior.*
  trace_id VARCHAR(64),               -- OTel traceId 关联 Loki/Tempo
  user_id BIGINT,
  payload JSON,                       -- 结构化字段(耗时/评分/IP/场景 等)
  INDEX idx_ts (ts),
  INDEX idx_service_event (service, event, ts),
  INDEX idx_trace (trace_id),
  INDEX idx_user_event (user_id, event, ts)
);

-- ========== 写作增强服务(writing-coach)表 ==========
CREATE TABLE IF NOT EXISTS writing_templates (
  id BIGINT NOT NULL AUTO_RANDOM PRIMARY KEY,
  category VARCHAR(32) NOT NULL,        -- argumentative/letter/chart/narrative
  title VARCHAR(128) NOT NULL,
  structure TEXT NOT NULL,              -- 段落结构骨架
  difficulty TINYINT NOT NULL DEFAULT 2,-- 1-3
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  INDEX idx_category_diff (category, difficulty)
);

CREATE TABLE IF NOT EXISTS writing_essays (
  id BIGINT NOT NULL AUTO_RANDOM PRIMARY KEY,
  category VARCHAR(32) NOT NULL,
  title VARCHAR(128) NOT NULL,
  content TEXT NOT NULL,
  difficulty TINYINT NOT NULL DEFAULT 2,
  word_count INT NOT NULL DEFAULT 0,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  INDEX idx_category_diff (category, difficulty)
);

CREATE TABLE IF NOT EXISTS writing_plans (
  id BIGINT NOT NULL AUTO_RANDOM PRIMARY KEY,
  user_id BIGINT NOT NULL,
  weak_points_json JSON NOT NULL,
  plan_json TEXT NOT NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'active',
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  INDEX idx_user (user_id, created_at)
);

CREATE TABLE IF NOT EXISTS writing_progress (
  id BIGINT NOT NULL AUTO_RANDOM PRIMARY KEY,
  user_id BIGINT NOT NULL,
  plan_id BIGINT NOT NULL,
  task_id VARCHAR(64) NOT NULL,
  completed TINYINT NOT NULL DEFAULT 0,
  score DECIMAL(5,1) NOT NULL DEFAULT 0,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  INDEX idx_user_plan (user_id, plan_id, created_at)
);

-- ========== 7 新服务扩展表(ADR adr-new-services.md,S1~S7,2026-08-06)==========
-- 表名以用户方案为准;全部 BIGINT AUTO_RANDOM 主键 + DATETIME(3) + 索引,对齐现有风格。

-- S1 发音评分(pronunciation-rs)
CREATE TABLE IF NOT EXISTS pronunciation_attempts (
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

-- S2 听力训练(listening-rs)
CREATE TABLE IF NOT EXISTS listening_exercises (
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
CREATE TABLE IF NOT EXISTS listening_attempts (
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

-- S3 词汇记忆(vocab-srs-go,SM-2+ 算法)
CREATE TABLE IF NOT EXISTS vocabulary (
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
CREATE TABLE IF NOT EXISTS user_vocabulary (
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
  version INT NOT NULL DEFAULT 0,           -- P0-D2(2026-08-21):乐观锁版本列,防并发 lost update
  UNIQUE INDEX uk_user_word (user_id, word_id),
  INDEX idx_due (user_id, due_at)
);

-- S4 能力测评(assessment-rs,CEFR/雅思/托福模考 + IRT)
CREATE TABLE IF NOT EXISTS assessment_sessions (
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
CREATE TABLE IF NOT EXISTS assessment_answers (
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

-- S5 游戏化(gamification-go)
CREATE TABLE IF NOT EXISTS user_streaks (
  id BIGINT NOT NULL AUTO_RANDOM PRIMARY KEY,
  user_id BIGINT NOT NULL UNIQUE,
  current_streak INT NOT NULL DEFAULT 0,
  longest_streak INT NOT NULL DEFAULT 0,
  last_active_date DATE NOT NULL,
  total_xp BIGINT NOT NULL DEFAULT 0,
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  INDEX idx_streak (current_streak DESC)
);
CREATE TABLE IF NOT EXISTS achievements (
  id BIGINT NOT NULL AUTO_RANDOM PRIMARY KEY,
  code VARCHAR(64) NOT NULL UNIQUE,
  title VARCHAR(128) NOT NULL,
  description TEXT,
  icon VARCHAR(256),
  xp_reward INT NOT NULL DEFAULT 0,
  criteria JSON NOT NULL                    -- 达成条件
);
CREATE TABLE IF NOT EXISTS user_achievements (
  id BIGINT NOT NULL AUTO_RANDOM PRIMARY KEY,
  user_id BIGINT NOT NULL,
  achievement_id BIGINT NOT NULL,
  unlocked_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  UNIQUE INDEX uk_user_ach (user_id, achievement_id),
  INDEX idx_user (user_id)
);

-- S6 阅读训练(reading-go)
CREATE TABLE IF NOT EXISTS reading_passages (
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
CREATE TABLE IF NOT EXISTS reading_attempts (
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

-- S7 LLM 统一网关(llm-orchestrator-rs,调用日志 + 成本追踪)
CREATE TABLE IF NOT EXISTS llm_call_logs (
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

-- P1-10 Disruptor outbox(关键学情事件可靠投递,防 RingBuffer DROP 丢数据)
-- 当 Disruptor RingBuffer 满时(背压丢弃),关键业务事件落本表兜底,
-- 由 OutboxRelay 异步轮询重投 Disruptor(或直接落 learning_events)。
-- 仅关键事件(security/教务/计费类)入 outbox;埋点类可保持 DROP。
CREATE TABLE IF NOT EXISTS learning_event_outbox (
  id BIGINT NOT NULL AUTO_RANDOM PRIMARY KEY,
  user_id BIGINT NOT NULL,
  event_type VARCHAR(32) NOT NULL,
  payload JSON NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'pending',  -- pending/done/failed
  retry_count INT NOT NULL DEFAULT 0,
  next_retry_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_status_retry (status, next_retry_at),
  KEY idx_user_time (user_id, created_at)
);
