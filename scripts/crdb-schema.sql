-- =====================================================================================
-- ESP CockroachDB Schema — 按有界上下文分库(Task #136 Part 1)
-- =====================================================================================
-- 运行环境: CRDB v24.3 StatefulSet(3 副本), ns esp-crdb, svc crdb-public:26257(PG wire)。
-- 执行方式: cockroach sql --insecure -d defaultdb -f scripts/crdb-schema.sql
--          (或 psql/psycopg2 执行;CREATE DATABASE IF NOT EXISTS 幂等;表已存在会报错,
--           需先 DROP 对应库再重建,避免静默 schema 漂移)。
--
-- 域库划分(6 域库 / 14 表)+ 与 TiDB(单库 esp / esp_learning / esp_shared)映射:
--   esp_auth    : users, sessions
--   esp_learning: learning_events, learning_event_outbox, session_turns, user_mastery, knowledge_points
--   esp_content : scenarios
--   esp_teacher : classes, assignments, submissions, lesson_plans
--   esp_writing : writing_tasks
--   esp_shared  : app_logs
--
-- DDL 转换规则(MySQL/TiDB → PG/CRDB):
--   AUTO_INCREMENT / AUTO_RANDOM 主键  →  BIGINT NOT NULL DEFAULT unique_rowid() PRIMARY KEY
--   DATETIME / TIMESTAMP              →  TIMESTAMPTZ DEFAULT now()
--   JSON                              →  JSONB
--   TINYINT / INT                     →  INTEGER / BIGINT
--   ENUM(...)                         →  VARCHAR + CHECK(列内枚举约束,保数据完整性)
--   ON UPDATE CURRENT_TIMESTAMP       →  CRDB 不支持,由应用层维护(见 user_mastery.updated_at 注释)
--   外键(KEY/FK)                     →  全部禁掉(压测写放大 + 分库跨库 FK 不可行);仅保留二级索引
--
-- 与 TiDB 行数对比比例(CRDB 灌装目标 / TiDB 实测量,等量或降比例,诚实可比):
--   users               1,000,000 / TiDB 1,002,651                ≈ 1.0x
--   scenarios              10,000 / TiDB 10,000                    = 1.0x
--   knowledge_points       10,000 / TiDB 10,000                    = 1.0x
--   user_mastery        5,000,000 / TiDB 4,998,730                ≈ 1.0x
--   learning_events    10,000,000 / esp_learning 10,526,432        ≈ 0.95x(对应主库 esp≈100M 为 0.1x)
--   sessions            5,000,000 / TiDB ≈50,000,000               = 0.1x
--   session_turns       5,000,000 / TiDB ≈50,000,000               = 0.1x
--   writing_tasks       2,000,000 / TiDB ≈20,000,000               = 0.1x
--   lesson_plans        1,000,000 / TiDB 1,010,000                ≈ 0.99x
--   classes               500,000 / TiDB 500,000                   = 1.0x
--   assignments         2,000,000 / TiDB 5,030,000                ≈ 0.4x
--   submissions         2,000,000 / TiDB 10,084,999               ≈ 0.2x
--   learning_event_outbox      0  / TiDB 0                         = 1.0x(空表,仅结构)
--   app_logs              85,000   / esp_shared.app_logs 85,601   ≈ 0.99x
--   合计 ≈ 32,605,000 行(对比 TiDB 主库量级 30 亿 ≈ 1%,单节点 RD 可行规模)
-- =====================================================================================

-- ---------------- esp_auth:用户与认证会话 ----------------
CREATE DATABASE IF NOT EXISTS esp_auth;

CREATE TABLE esp_auth.users (
  id         BIGINT NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  role       VARCHAR(16) NOT NULL DEFAULT 'learner' CHECK (role IN ('learner','teacher','admin')),
  name       VARCHAR(64) NOT NULL,
  email      VARCHAR(128) NOT NULL,
  pwd_hash   VARCHAR(128) NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uk_email UNIQUE (email)
);

CREATE TABLE esp_auth.sessions (
  id          BIGINT NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  user_id     BIGINT NOT NULL,
  scenario_id BIGINT NOT NULL,
  status      VARCHAR(16) NOT NULL DEFAULT 'active' CHECK (status IN ('active','ended')),
  started_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  ended_at    TIMESTAMPTZ NULL,
  INDEX idx_user (user_id)
);

-- ---------------- esp_content:场景内容(低变更,GLOBAL 型) ----------------
CREATE DATABASE IF NOT EXISTS esp_content;

CREATE TABLE esp_content.scenarios (
  id            BIGINT NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  code          VARCHAR(32) NOT NULL,
  title         VARCHAR(128) NOT NULL,
  system_prompt TEXT NOT NULL,
  difficulty    INTEGER NOT NULL DEFAULT 3,
  lang          VARCHAR(8) NOT NULL DEFAULT 'en',
  CONSTRAINT uk_code UNIQUE (code)
);

-- ---------------- esp_learning:学情事件流 / 对话 / 掌握度 ----------------
CREATE DATABASE IF NOT EXISTS esp_learning;

CREATE TABLE esp_learning.learning_events (
  id         BIGINT NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  user_id    BIGINT NOT NULL,
  event_type VARCHAR(32) NOT NULL,
  payload    JSONB NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  -- P1-8 幂等键:发布侧 UUID,随 Pulsar 载荷透传;NULL 允许(PG/CRDB 唯一索引不约束 NULL),
  -- 旧行保持 NULL 不冲突,新写入必有值 → ON CONFLICT (message_id) DO NOTHING 去重。
  message_id STRING NULL,
  INDEX idx_user_time (user_id, created_at),
  UNIQUE INDEX learning_events_message_id_key (message_id)
);

CREATE TABLE esp_learning.learning_event_outbox (
  id            BIGINT NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  message_id    STRING NULL, -- P1-8 幂等键透传(重投时 ON CONFLICT 去重)
  user_id       BIGINT NOT NULL,
  event_type    VARCHAR(32) NOT NULL,
  payload       JSONB NULL,
  status        VARCHAR(16) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','done','failed')),
  retry_count   INTEGER NOT NULL DEFAULT 0,
  next_retry_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  INDEX idx_status_retry (status, next_retry_at),
  INDEX idx_user_time (user_id, created_at)
);

CREATE TABLE esp_learning.session_turns (
  id         BIGINT NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  session_id BIGINT NOT NULL,
  role       VARCHAR(8) NOT NULL CHECK (role IN ('user','ai','system')),
  content    TEXT NOT NULL,
  audio_ref  VARCHAR(256) NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  INDEX idx_session (session_id)
);

CREATE TABLE esp_learning.user_mastery (
  user_id        BIGINT NOT NULL,
  kp_id          BIGINT NOT NULL,
  mastery_score  DECIMAL(5,2) NOT NULL DEFAULT 0,
  last_review_at TIMESTAMPTZ NULL,
  review_count   INTEGER NOT NULL DEFAULT 0,
  next_review_at TIMESTAMPTZ NULL,
  version        INTEGER NOT NULL DEFAULT 0,
  -- TiDB 为 ON UPDATE CURRENT_TIMESTAMP;CRDB 无此语义,由应用层写 updated_at。
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, kp_id)
);

CREATE TABLE esp_learning.knowledge_points (
  id         BIGINT NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  code       VARCHAR(32) NOT NULL,
  title      VARCHAR(128) NOT NULL,
  parent_id  BIGINT NULL,
  difficulty INTEGER NOT NULL DEFAULT 3,
  CONSTRAINT uk_code UNIQUE (code)
);

-- ---------------- esp_teacher:教务(班级/作业/提交/教案) ----------------
CREATE DATABASE IF NOT EXISTS esp_teacher;

CREATE TABLE esp_teacher.classes (
  id         BIGINT NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  teacher_id BIGINT NOT NULL,
  name       VARCHAR(64) NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE esp_teacher.assignments (
  id         BIGINT NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  class_id   BIGINT NOT NULL,
  type       VARCHAR(8) NOT NULL CHECK (type IN ('oral','written')),
  content    TEXT NOT NULL,
  due_at     TIMESTAMPTZ NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  INDEX idx_class (class_id)
);

CREATE TABLE esp_teacher.submissions (
  id            BIGINT NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  assignment_id BIGINT NOT NULL,
  user_id       BIGINT NOT NULL,
  content       TEXT NOT NULL,
  auto_score    DECIMAL(5,2) NULL,
  auto_feedback JSONB NULL,
  final_score   DECIMAL(5,2) NULL,
  status        VARCHAR(16) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','auto_graded','finalized')),
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  INDEX idx_assignment (assignment_id)
);

CREATE TABLE esp_teacher.lesson_plans (
  id         BIGINT NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  teacher_id BIGINT NOT NULL,
  topic      VARCHAR(128) NOT NULL,
  content    JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  INDEX idx_teacher (teacher_id)
);

-- ---------------- esp_writing:写作任务 ----------------
CREATE DATABASE IF NOT EXISTS esp_writing;

CREATE TABLE esp_writing.writing_tasks (
  id         BIGINT NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  user_id    BIGINT NOT NULL,
  prompt     TEXT NOT NULL,
  essay      TEXT NOT NULL,
  score      DECIMAL(5,2) NULL,
  feedback   JSONB NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  INDEX idx_user (user_id)
);

-- ---------------- esp_shared:结构化业务日志 ----------------
CREATE DATABASE IF NOT EXISTS esp_shared;

CREATE TABLE esp_shared.app_logs (
  id          BIGINT NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  ts          TIMESTAMPTZ NOT NULL DEFAULT now(),  -- TiDB DATETIME(3) → CRDB TIMESTAMPTZ(微秒;如需毫秒可用 TIMESTAMPTZ(3))
  service     VARCHAR(32) NOT NULL,                -- bff/learning/teacher/writing/emotion/gateway
  level       VARCHAR(8) NOT NULL,                 -- INFO/WARN/ERROR
  event       VARCHAR(64) NOT NULL,                -- auth.login/writing.grade/learning.event/behavior.*
  trace_id    VARCHAR(64),
  user_id     BIGINT,
  payload     JSONB,
  INDEX idx_ts (ts),
  INDEX idx_service_event (service, event, ts),
  INDEX idx_trace (trace_id),
  INDEX idx_user_event (user_id, event, ts)
);

-- ============================================================================
-- feature-expansion(功能拓展):内容库 / 圈子社区 / 排行榜 新增表
-- ============================================================================

-- ---------------- esp_content 内容库:course → lesson → content_item ----------------
-- 统一策展目录(四六级/托福/雅思/教材),所有用户共享;版本快照 is_current,状态生命周期。
CREATE TABLE esp_content.courses (
  id          BIGINT NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  code        VARCHAR(32) NOT NULL,          -- cet4/cet6/toefl/ielts/textbook
  title       VARCHAR(128) NOT NULL,
  exam_tag    VARCHAR(16) NOT NULL,          -- CET4/CET6/TOEFL/IELTS/GENERAL
  cefr_range  VARCHAR(16) NOT NULL DEFAULT '',
  description TEXT,
  cover_url   VARCHAR(256),
  status      VARCHAR(16) NOT NULL DEFAULT 'published', -- draft/pending/published/archived
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uk_course_code UNIQUE (code)
);

CREATE TABLE esp_content.lessons (
  id         BIGINT NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  course_id  BIGINT NOT NULL,
  seq        INT NOT NULL DEFAULT 0,
  title      VARCHAR(128) NOT NULL,
  skill      VARCHAR(16) NOT NULL,           -- vocab/reading/listening/grammar/cloze
  cefr       VARCHAR(16) NOT NULL DEFAULT '',
  status     VARCHAR(16) NOT NULL DEFAULT 'published',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  INDEX idx_course_seq (course_id, seq)
);

CREATE TABLE esp_content.content_items (
  id          BIGINT NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  lesson_id   BIGINT NOT NULL,
  item_type   VARCHAR(16) NOT NULL,          -- vocab/reading/listening/grammar/cloze
  prompt      JSONB NOT NULL,                -- 结构化题干(word/def/passage/options/audio_ref)
  answer      JSONB,                         -- 答案与判分基准
  explanation TEXT,
  difficulty  INT NOT NULL DEFAULT 3,        -- 1-5
  cefr        VARCHAR(16) NOT NULL DEFAULT '',
  tags        STRING[],                      -- 主题/题型标签
  source      VARCHAR(64),                   -- 内容来源(版权标注)
  license     VARCHAR(32),                   -- 授权标注
  version     INT NOT NULL DEFAULT 1,        -- 版本快照(编辑建新版本,不覆盖)
  is_current  BOOL NOT NULL DEFAULT true,
  status      VARCHAR(16) NOT NULL DEFAULT 'published', -- draft/pending/published/archived
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  INDEX idx_lesson (lesson_id),
  INDEX idx_current (is_current, status)
);

-- 内容作答(内容库学习进度真源;经 learning_event_outbox 发事件供 XP/掌握度)
CREATE TABLE esp_content.content_attempts (
  id           BIGINT NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  user_id      BIGINT NOT NULL,
  item_id      BIGINT NOT NULL,
  answers_json JSONB,
  score        DECIMAL(5,2) NOT NULL DEFAULT 0,
  correct      BOOL NOT NULL DEFAULT false,
  duration_ms  INT NOT NULL DEFAULT 0,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  INDEX idx_user_item (user_id, item_id),
  INDEX idx_item (item_id)
);

-- ---------------- esp_community 圈子社区 ----------------
CREATE DATABASE IF NOT EXISTS esp_community;

CREATE TABLE esp_community.circles (
  id           BIGINT NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  name         VARCHAR(64) NOT NULL,
  description  TEXT,
  invite_code  VARCHAR(8) NOT NULL,
  owner_id     BIGINT NOT NULL,
  member_limit INT NOT NULL DEFAULT 50,
  status       VARCHAR(16) NOT NULL DEFAULT 'active', -- active/closed
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uk_invite_code UNIQUE (invite_code),
  INDEX idx_owner (owner_id)
);

CREATE TABLE esp_community.circle_memberships (
  circle_id BIGINT NOT NULL,
  user_id   BIGINT NOT NULL,
  role      VARCHAR(16) NOT NULL DEFAULT 'member',   -- owner/admin/member
  joined_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (circle_id, user_id),
  INDEX idx_user (user_id)
);

CREATE TABLE esp_community.circle_posts (
  id           BIGINT NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  circle_id    BIGINT NOT NULL,
  user_id      BIGINT NOT NULL,
  body         TEXT NOT NULL,
  status       VARCHAR(16) NOT NULL DEFAULT 'published', -- published/hidden(举报达阈值自动隐藏)
  report_count INT NOT NULL DEFAULT 0,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  INDEX idx_circle_time (circle_id, created_at)
);

CREATE TABLE esp_community.circle_post_comments (
  id         BIGINT NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  post_id    BIGINT NOT NULL,
  user_id    BIGINT NOT NULL,
  body       TEXT NOT NULL,
  status     VARCHAR(16) NOT NULL DEFAULT 'published',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  INDEX idx_post (post_id)
);

CREATE TABLE esp_community.content_reports (
  id          BIGINT NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  target_type VARCHAR(16) NOT NULL,          -- post/comment/circle
  target_id   BIGINT NOT NULL,
  reporter_id BIGINT NOT NULL,
  reason      VARCHAR(128),
  status      VARCHAR(16) NOT NULL DEFAULT 'pending', -- pending/reviewed/dismissed
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- UK 防重复举报:同一用户对同一目标只能举报一次
ALTER TABLE esp_community.content_reports ADD CONSTRAINT uk_report_user_target UNIQUE (reporter_id, target_type, target_id);
-- 索引:举报列表查询按 target_type + target_id 过滤
CREATE INDEX IF NOT EXISTS idx_content_reports_target ON esp_community.content_reports (target_type, target_id);

-- ---------------- esp_gamif 排行榜快照(历史/大盘 percentile) ----------------
CREATE DATABASE IF NOT EXISTS esp_gamif;

CREATE TABLE esp_gamif.leaderboard_snapshots (
  id         BIGINT NOT NULL DEFAULT unique_rowid() PRIMARY KEY,
  scope_type VARCHAR(16) NOT NULL,           -- global/region/circle/category
  scope_id   VARCHAR(64) NOT NULL,           -- all/region_code/circle_id/category
  period     VARCHAR(16) NOT NULL,           -- daily/weekly/monthly
  rank       INT NOT NULL,
  user_id    BIGINT NOT NULL,
  score      DECIMAL(18,4) NOT NULL,
  total_members INT,                          -- 偏差④:当日该榜总人数,支持 percentile
  ts         TIMESTAMPTZ NOT NULL DEFAULT now(),
  INDEX idx_scope_period_rank (scope_type, scope_id, period, rank),
  INDEX idx_scope_period_user (scope_type, scope_id, period, user_id)
);

-- 兼容已建表:补列(幂等)
ALTER TABLE esp_gamif.leaderboard_snapshots ADD COLUMN IF NOT EXISTS total_members INT;
CREATE INDEX IF NOT EXISTS idx_scope_period_user ON esp_gamif.leaderboard_snapshots (scope_type, scope_id, period, user_id);

-- ---------------- esp_auth.users 扩展(地域 / 排行隐私 / 社区资料) ----------------
ALTER TABLE esp_auth.users ADD COLUMN IF NOT EXISTS region_code STRING;      -- ISO2 或 CN/GB-T(如 CN/440300)
ALTER TABLE esp_auth.users ADD COLUMN IF NOT EXISTS ranking_opt_out BOOL NOT NULL DEFAULT false;
ALTER TABLE esp_auth.users ADD COLUMN IF NOT EXISTS avatar_url STRING;
ALTER TABLE esp_auth.users ADD COLUMN IF NOT EXISTS bio STRING;
