-- ============================================================
-- ESP schema-per-service Phase 0 DDL
-- 关联: docs/design/adr-schema-per-service.md (Accepted 2026-08-06)
--       docs/design/prd-new-services.md (N8/N10/N11/S1 dialogue)
-- 原则: 零风险 —— 只建新库/新表,不动 esp 老库任何表。
--       TiDB 兼容 MySQL,无 PostgreSQL schema 概念,用独立 database 模拟 schema。
--       esp_learning 按 ADR §5 Phase 3 设计为独立 database(已存在,此处 IF NOT EXISTS 占位)。
-- 日期: 2026-08-07
-- ============================================================

-- ---- 0. esp_learning 独立 DB(Phase 3 物理隔离预备,Phase 0 仅占位,不建表)----
CREATE DATABASE IF NOT EXISTS esp_learning;

-- ---- 1. esp_pronunciation (pronunciation-rs 待落地) ----
CREATE DATABASE IF NOT EXISTS esp_pronunciation;
CREATE TABLE IF NOT EXISTS esp_pronunciation.pronunciation_attempts (
  id           BIGINT       NOT NULL AUTO_RANDOM PRIMARY KEY,
  user_id      BIGINT       NOT NULL,
  ref_type     VARCHAR(32)  NOT NULL COMMENT 'word/sentence/passage/dialogue_turn',
  ref_id       BIGINT       NOT NULL COMMENT '对应实体 id',
  audio_url    VARCHAR(512) NOT NULL,
  score        DECIMAL(5,2) NOT NULL COMMENT 'GOP 综合评分 0-100',
  phoneme_scores JSON       NULL COMMENT '音素级评分明细',
  asr_text     TEXT         NULL COMMENT 'ASR 转写文本',
  duration_ms  INT          NOT NULL DEFAULT 0,
  created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_user_time (user_id, created_at),
  KEY idx_ref (ref_type, ref_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin COMMENT='发音跟读评分流水';

-- ---- 2. esp_listening (listening-rs 待落地) ----
CREATE DATABASE IF NOT EXISTS esp_listening;
CREATE TABLE IF NOT EXISTS esp_listening.listening_exercises (
  id           BIGINT       NOT NULL AUTO_RANDOM PRIMARY KEY,
  title        VARCHAR(128) NOT NULL,
  audio_url    VARCHAR(512) NOT NULL,
  duration_sec INT          NOT NULL DEFAULT 0,
  cefr_level   VARCHAR(8)   NOT NULL COMMENT 'A1~C2',
  question_json JSON        NOT NULL COMMENT '题目数组(题型/题干/选项/答案)',
  scene_tags   JSON         NULL,
  created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_cefr (cefr_level)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin COMMENT='听力练习题';

CREATE TABLE IF NOT EXISTS esp_listening.listening_attempts (
  id           BIGINT       NOT NULL AUTO_RANDOM PRIMARY KEY,
  user_id      BIGINT       NOT NULL,
  exercise_id  BIGINT       NOT NULL,
  answers_json JSON         NOT NULL COMMENT '用户作答明细',
  score        DECIMAL(5,2) NOT NULL,
  duration_ms  INT          NOT NULL DEFAULT 0,
  created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_user_time (user_id, created_at),
  KEY idx_exercise (exercise_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin COMMENT='听力答题流水';

-- ---- 3. esp_vocab (vocab-srs-go 待落地) ----
CREATE DATABASE IF NOT EXISTS esp_vocab;
CREATE TABLE IF NOT EXISTS esp_vocab.vocabulary (
  id            BIGINT       NOT NULL AUTO_RANDOM PRIMARY KEY,
  word          VARCHAR(128) NOT NULL,
  phonetic      VARCHAR(128) NULL,
  pos           VARCHAR(16)  NULL COMMENT '词性',
  definition    TEXT         NOT NULL,
  definition_zh TEXT         NULL,
  examples_json JSON         NULL,
  cefr_level    VARCHAR(8)   NULL,
  UNIQUE KEY uk_word (word),
  KEY idx_cefr (cefr_level)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin COMMENT='词汇主表';

CREATE TABLE IF NOT EXISTS esp_vocab.user_vocabulary (
  id            BIGINT       NOT NULL AUTO_RANDOM PRIMARY KEY,
  user_id       BIGINT       NOT NULL,
  word_id       BIGINT       NOT NULL,
  srs_stage     INT          NOT NULL DEFAULT 0 COMMENT 'SRS 阶段',
  interval_days INT          NOT NULL DEFAULT 1,
  ease_factor   DECIMAL(4,2) NOT NULL DEFAULT 2.50,
  due_at        DATETIME     NOT NULL,
  review_count  INT          NOT NULL DEFAULT 0,
  lapse_count   INT          NOT NULL DEFAULT 0,
  status        VARCHAR(16)  NOT NULL DEFAULT 'learning' COMMENT 'learning/review/mastered',
  last_reviewed DATETIME     NULL,
  created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_user_word (user_id, word_id),
  KEY idx_due (user_id, due_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin COMMENT='用户词汇 SRS 状态';

-- ---- 4. esp_assessment (assessment-rs 待落地) ----
CREATE DATABASE IF NOT EXISTS esp_assessment;
CREATE TABLE IF NOT EXISTS esp_assessment.assessment_sessions (
  id           BIGINT       NOT NULL AUTO_RANDOM PRIMARY KEY,
  user_id      BIGINT       NOT NULL,
  goal         VARCHAR(64)  NULL COMMENT '雅思/托福/职场口语等',
  cefr_result  VARCHAR(8)   NULL COMMENT '定级结果',
  status       VARCHAR(16)  NOT NULL DEFAULT 'in_progress' COMMENT 'in_progress/completed/abandoned',
  score        DECIMAL(5,2) NULL,
  started_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  finished_at  DATETIME     NULL,
  KEY idx_user_time (user_id, started_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin COMMENT='定级测试会话';

CREATE TABLE IF NOT EXISTS esp_assessment.assessment_answers (
  id            BIGINT       NOT NULL AUTO_RANDOM PRIMARY KEY,
  session_id    BIGINT       NOT NULL,
  question_id   BIGINT       NOT NULL,
  answer_json   JSON         NOT NULL,
  is_correct    TINYINT      NOT NULL DEFAULT 0,
  skill_tag     VARCHAR(32)  NULL COMMENT 'grammar/listening/reading/vocab',
  created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_session (session_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin COMMENT='定级测试答题明细';

-- ---- 5. esp_gamification (gamification-go 待落地) ----
CREATE DATABASE IF NOT EXISTS esp_gamification;
CREATE TABLE IF NOT EXISTS esp_gamification.user_streaks (
  user_id      BIGINT       NOT NULL PRIMARY KEY,
  current_days INT          NOT NULL DEFAULT 0,
  longest_days INT          NOT NULL DEFAULT 0,
  last_active  DATE         NULL,
  updated_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  KEY idx_last_active (last_active)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin COMMENT='用户连续学习天数';

CREATE TABLE IF NOT EXISTS esp_gamification.achievements (
  id           BIGINT       NOT NULL AUTO_RANDOM PRIMARY KEY,
  code         VARCHAR(64)  NOT NULL,
  name         VARCHAR(128) NOT NULL,
  description  VARCHAR(512) NULL,
  icon_url     VARCHAR(512) NULL,
  criteria_json JSON        NOT NULL COMMENT '解锁条件',
  UNIQUE KEY uk_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin COMMENT='成就定义';

CREATE TABLE IF NOT EXISTS esp_gamification.user_achievements (
  id            BIGINT       NOT NULL AUTO_RANDOM PRIMARY KEY,
  user_id       BIGINT       NOT NULL,
  achievement_id BIGINT      NOT NULL,
  unlocked_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_user_ach (user_id, achievement_id),
  KEY idx_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin COMMENT='用户已解锁成就';

-- ---- 6. esp_reading (reading-go 待落地) ----
CREATE DATABASE IF NOT EXISTS esp_reading;
CREATE TABLE IF NOT EXISTS esp_reading.reading_passages (
  id            BIGINT       NOT NULL AUTO_RANDOM PRIMARY KEY,
  title         VARCHAR(256) NOT NULL,
  content       MEDIUMTEXT   NOT NULL,
  word_count    INT          NOT NULL DEFAULT 0,
  cefr_level    VARCHAR(8)   NOT NULL,
  source_type   VARCHAR(32)  NULL COMMENT 'news/story/exam',
  questions_json JSON        NULL,
  created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_cefr (cefr_level)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin COMMENT='阅读篇章';

CREATE TABLE IF NOT EXISTS esp_reading.reading_attempts (
  id           BIGINT       NOT NULL AUTO_RANDOM PRIMARY KEY,
  user_id      BIGINT       NOT NULL,
  passage_id   BIGINT       NOT NULL,
  answers_json JSON         NULL,
  score        DECIMAL(5,2) NULL,
  duration_ms  INT          NOT NULL DEFAULT 0,
  created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_user_time (user_id, created_at),
  KEY idx_passage (passage_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin COMMENT='阅读答题流水';

-- ---- 7. esp_llm (llm-orchestrator-rs + S1 dialogue) ----
CREATE DATABASE IF NOT EXISTS esp_llm;

-- 7.1 成本治理核心表
CREATE TABLE IF NOT EXISTS esp_llm.llm_call_logs (
  id            BIGINT       NOT NULL AUTO_RANDOM PRIMARY KEY,
  trace_id      VARCHAR(64)  NULL,
  user_id       BIGINT       NULL,
  provider      VARCHAR(32)  NOT NULL COMMENT 'openai/azure/qwen/deepseek',
  model         VARCHAR(64)  NOT NULL,
  scene         VARCHAR(32)  NOT NULL COMMENT 'dialogue/pronunciation/expression_extract/assessment',
  prompt_tokens INT          NOT NULL DEFAULT 0,
  completion_tokens INT      NOT NULL DEFAULT 0,
  total_tokens  INT          NOT NULL DEFAULT 0,
  latency_ms    INT          NOT NULL DEFAULT 0,
  cost_usd      DECIMAL(10,6) NOT NULL DEFAULT 0,
  cache_hit     TINYINT      NOT NULL DEFAULT 0,
  status        VARCHAR(16)  NOT NULL DEFAULT 'ok' COMMENT 'ok/error/timeout',
  error_msg     VARCHAR(512) NULL,
  created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_user_time (user_id, created_at),
  KEY idx_scene_time (scene, created_at),
  KEY idx_provider_time (provider, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin COMMENT='LLM 调用成本与延迟流水';

-- 7.2 语义缓存(PRD §1.5 复用)
CREATE TABLE IF NOT EXISTS esp_llm.llm_semantic_cache (
  id            BIGINT       NOT NULL AUTO_RANDOM PRIMARY KEY,
  cache_key     VARCHAR(128) NOT NULL COMMENT 'embedding hash 或 prompt hash',
  embedding     VARBINARY(2048) NULL COMMENT '向量(Redis L2 为主,此表持久化备份)',
  prompt        TEXT         NOT NULL,
  response      MEDIUMTEXT   NOT NULL,
  model         VARCHAR(64)  NOT NULL,
  scene         VARCHAR(32)  NOT NULL,
  hit_count     INT          NOT NULL DEFAULT 0,
  expires_at    DATETIME     NULL,
  created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_cache_key (cache_key),
  KEY idx_scene_expires (scene, expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin COMMENT='LLM 语义缓存持久层';

-- 7.3 S1 dialogue 多轮对话(PRD §1.5)
CREATE TABLE IF NOT EXISTS esp_llm.dialogue_sessions (
  id            BIGINT       NOT NULL AUTO_RANDOM PRIMARY KEY,
  user_id       BIGINT       NOT NULL,
  scenario_id   BIGINT       NULL,
  persona_id    BIGINT       NULL,
  cefr_level    VARCHAR(8)   NULL,
  status        VARCHAR(16)  NOT NULL DEFAULT 'active' COMMENT 'active/ended/abandoned',
  created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  ended_at      DATETIME     NULL,
  KEY idx_user_time (user_id, created_at),
  KEY idx_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin COMMENT='AI 多轮对话会话';

CREATE TABLE IF NOT EXISTS esp_llm.dialogue_turns (
  id             BIGINT       NOT NULL AUTO_RANDOM PRIMARY KEY,
  session_id     BIGINT       NOT NULL,
  role           VARCHAR(16)  NOT NULL COMMENT 'user/assistant/system',
  content        MEDIUMTEXT   NOT NULL,
  asr_confidence DECIMAL(5,2) NULL,
  tts_url        VARCHAR(512) NULL,
  tokens_in      INT          NOT NULL DEFAULT 0,
  tokens_out     INT          NOT NULL DEFAULT 0,
  latency_ms     INT          NOT NULL DEFAULT 0,
  created_at     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_session_time (session_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin COMMENT='AI 对话逐轮记录';

CREATE TABLE IF NOT EXISTS esp_llm.dialogue_personas (
  id            BIGINT       NOT NULL AUTO_RANDOM PRIMARY KEY,
  name          VARCHAR(64)  NOT NULL,
  persona_prompt TEXT        NOT NULL,
  voice_id      VARCHAR(64)  NULL,
  accent        VARCHAR(16)  NULL,
  cefr_level    VARCHAR(8)   NULL,
  created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_cefr (cefr_level)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin COMMENT='对话角色卡';

-- ---- 8. esp_path (N8 course-path-go) ----
CREATE DATABASE IF NOT EXISTS esp_path;
CREATE TABLE IF NOT EXISTS esp_path.learning_paths (
  id            BIGINT       NOT NULL AUTO_RANDOM PRIMARY KEY,
  user_id       BIGINT       NOT NULL,
  goal          VARCHAR(64)  NULL COMMENT '雅思6.5/职场口语等',
  cefr_level    VARCHAR(8)   NULL,
  version       INT          NOT NULL DEFAULT 1,
  status        VARCHAR(16)  NOT NULL DEFAULT 'active',
  created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  KEY idx_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin COMMENT='用户学习路径';

CREATE TABLE IF NOT EXISTS esp_path.path_nodes (
  id               BIGINT       NOT NULL AUTO_RANDOM PRIMARY KEY,
  path_id          BIGINT       NOT NULL,
  skill_type       VARCHAR(32)  NOT NULL COMMENT 'lesson/knowledge/pronunciation/...',
  ref_id           BIGINT       NULL COMMENT '课时/知识点 id',
  order_idx        INT          NOT NULL DEFAULT 0,
  dependency_ids   JSON         NULL COMMENT '前置节点 id 数组',
  unlock_condition JSON         NULL COMMENT '解锁条件',
  status           VARCHAR(16)  NOT NULL DEFAULT 'locked',
  KEY idx_path_order (path_id, order_idx)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin COMMENT='路径节点';

CREATE TABLE IF NOT EXISTS esp_path.path_templates (
  id            BIGINT       NOT NULL AUTO_RANDOM PRIMARY KEY,
  name          VARCHAR(128) NOT NULL,
  goal          VARCHAR(64)  NULL,
  cefr_level    VARCHAR(8)   NULL,
  node_graph    JSON         NOT NULL COMMENT '体系课节点图',
  created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_goal_cefr (goal, cefr_level)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin COMMENT='体系课路径模板';

CREATE TABLE IF NOT EXISTS esp_path.path_progress (
  id            BIGINT       NOT NULL AUTO_RANDOM PRIMARY KEY,
  path_id       BIGINT       NOT NULL,
  node_id       BIGINT       NOT NULL,
  user_id       BIGINT       NOT NULL,
  status        VARCHAR(16)  NOT NULL DEFAULT 'locked' COMMENT 'locked/in_progress/completed',
  score         DECIMAL(5,2) NULL,
  completed_at  DATETIME     NULL,
  UNIQUE KEY uk_user_node (user_id, node_id),
  KEY idx_path_node (path_id, node_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin COMMENT='用户路径节点进度';

-- ---- 9. esp_tutor (N10 live-tutor-go) ----
CREATE DATABASE IF NOT EXISTS esp_tutor;
CREATE TABLE IF NOT EXISTS esp_tutor.tutors (
  id              BIGINT       NOT NULL AUTO_RANDOM PRIMARY KEY,
  name            VARCHAR(64)  NOT NULL,
  avatar          VARCHAR(512) NULL,
  bio             VARCHAR(1024) NULL,
  languages       JSON         NULL,
  rating          DECIMAL(3,2) NOT NULL DEFAULT 5.00,
  price_per_30min DECIMAL(10,2) NOT NULL DEFAULT 0,
  timezone        VARCHAR(32)  NULL,
  status          VARCHAR(16)  NOT NULL DEFAULT 'active',
  created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_status_rating (status, rating)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin COMMENT='外教档案';

CREATE TABLE IF NOT EXISTS esp_tutor.tutor_availability (
  id         BIGINT       NOT NULL AUTO_RANDOM PRIMARY KEY,
  tutor_id   BIGINT       NOT NULL,
  start_at   DATETIME     NOT NULL,
  end_at     DATETIME     NOT NULL,
  status     VARCHAR(16)  NOT NULL DEFAULT 'open' COMMENT 'open/locked/booked',
  KEY idx_tutor_time (tutor_id, start_at),
  KEY idx_status_time (status, start_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin COMMENT='外教可用时段';

CREATE TABLE IF NOT EXISTS esp_tutor.bookings (
  id            BIGINT       NOT NULL AUTO_RANDOM PRIMARY KEY,
  student_id    BIGINT       NOT NULL,
  tutor_id      BIGINT       NOT NULL,
  start_at      DATETIME     NOT NULL,
  duration_min  INT          NOT NULL DEFAULT 30,
  status        VARCHAR(16)  NOT NULL DEFAULT 'booked' COMMENT 'booked/completed/cancelled',
  package_id    BIGINT       NULL,
  created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_student_time (student_id, start_at),
  KEY idx_tutor_time (tutor_id, start_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin COMMENT='外教课预约';

CREATE TABLE IF NOT EXISTS esp_tutor.lesson_packages (
  id            BIGINT       NOT NULL AUTO_RANDOM PRIMARY KEY,
  student_id    BIGINT       NOT NULL,
  remaining     INT          NOT NULL DEFAULT 0,
  total         INT          NOT NULL DEFAULT 0,
  sku           VARCHAR(64)  NOT NULL,
  expires_at    DATETIME     NULL,
  created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_student (student_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin COMMENT='课时包';

CREATE TABLE IF NOT EXISTS esp_tutor.lesson_notes (
  id          BIGINT       NOT NULL AUTO_RANDOM PRIMARY KEY,
  booking_id  BIGINT       NOT NULL,
  tutor_id    BIGINT       NOT NULL,
  content     TEXT         NOT NULL,
  created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_booking (booking_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin COMMENT='课后笔记';

-- ---- 10. esp_media (N11 media-learn-rs) ----
CREATE DATABASE IF NOT EXISTS esp_media;
CREATE TABLE IF NOT EXISTS esp_media.media_clips (
  id            BIGINT       NOT NULL AUTO_RANDOM PRIMARY KEY,
  title         VARCHAR(256) NOT NULL,
  source_type   VARCHAR(32)  NOT NULL COMMENT 'movie/short_video/news',
  url           VARCHAR(512) NOT NULL,
  duration_sec  INT          NOT NULL DEFAULT 0,
  cefr_level    VARCHAR(8)   NULL,
  scene_tags    JSON         NULL,
  created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_cefr (cefr_level),
  KEY idx_source (source_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin COMMENT='媒体片段';

CREATE TABLE IF NOT EXISTS esp_media.media_subtitles (
  id           BIGINT       NOT NULL AUTO_RANDOM PRIMARY KEY,
  clip_id      BIGINT       NOT NULL,
  start_ms     INT          NOT NULL,
  end_ms       INT          NOT NULL,
  text         VARCHAR(1024) NOT NULL,
  word_tokens  JSON         NULL,
  KEY idx_clip_time (clip_id, start_ms)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin COMMENT='字幕条目';

CREATE TABLE IF NOT EXISTS esp_media.media_expressions (
  id           BIGINT       NOT NULL AUTO_RANDOM PRIMARY KEY,
  clip_id      BIGINT       NOT NULL,
  subtitle_id  BIGINT       NULL,
  expression   VARCHAR(256) NOT NULL,
  translation  VARCHAR(512) NULL,
  example      TEXT         NULL,
  card_type    VARCHAR(32)  NOT NULL DEFAULT 'phrase' COMMENT 'phrase/slang/idiom',
  KEY idx_clip (clip_id),
  KEY idx_subtitle (subtitle_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin COMMENT='地道表达卡片';

-- ============================================================
-- Phase 0 DDL 结束。共 10 database(esp_learning 占位)+ 29 张新表。
-- 老库 esp 的 18 张表零改动(零风险原则)。
-- ============================================================
