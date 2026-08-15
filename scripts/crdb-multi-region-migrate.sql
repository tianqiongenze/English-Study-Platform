-- =====================================================================================
-- ESP CockroachDB 多区域演进 — 可移植迁移脚本(Task #161)
-- =====================================================================================
-- 目标:把当前 19 个 ESP 数据库(全部 locality NULL,非多区域默认态)升级为声明式
--      多区域拓扑(REGION 存活目标 + 逐表 GLOBAL / REGIONAL BY TABLE / REGIONAL BY ROW)。
--
-- ⚠️ 单节点验证约束(铁律,显式 DEFERRED):本脚本【不得】在现有单节点 RD 集群执行。
--   当前 crdb-0/1/2 的 locality 为 --locality=zone=z1/z2/z3,【无 region 层】;
--   SET PRIMARY REGION "aws" 会因找不到 region=aws 节点而直接报错。
--   执行前置条件(Phase 0)见 scripts/crdb-multi-region-migrate-README.md;满足后方可跑本脚本。
--
-- 设计依据:docs/design/cockroachdb-vs-tidb-multi-region-plan.md §1.3/§1.4/§3.2
--   · REGIONAL BY ROW 表必须有显式 region 列(crdb_internal_region 枚举),来源 = users.home_region
--     (注册网关写入;JWT claim 带 region,零额外查询)。
--   · GLOBAL 表适合极小/读多/低频写(scenarios 字典类)。
--   · REGIONAL BY TABLE 适合单写者组件(outbox/undo_log)。
--   · 写路径下限 ≈ 1×跨区 RTT(SURVIVE REGION FAILURE 无本地写),吞吐受 Raft 流水线限制而非单条 RTT。
--
-- 区域命名(与多节点阶段目标对齐):aws / azure / gcp(短名;生产可换 aws-us-east-1 等全名)。
-- =====================================================================================

-- -------------------------------------------------------------------------------------
-- Phase 1:数据库级区域声明(每个 ESP 数据库)
--   ⚠️ ADD REGION 无 IF NOT EXISTS,重复执行会报 duplicate region;脚本设计为一次性迁移,
--      失败可单独重试对应语句(语句间无事务依赖,可分段执行)。
-- -------------------------------------------------------------------------------------

-- 工具:生成全部数据库的 SET PRIMARY REGION 语句(DB 列表见下表,手工维护)。
-- 全部 19 库:esp, esp_auth, esp_content, esp_course_path, esp_emotion, esp_gamification,
--   esp_learning, esp_listening, esp_live_tutor, esp_llm, esp_pronunciation, esp_reading,
--   esp_shared, esp_speaking, esp_teacher, esp_vocab, esp_writing, esp_assessment, esp_live_tutor

-- 每个库重复以下三连(以 esp_learning 为例):
ALTER DATABASE esp_learning SET PRIMARY REGION "aws";
ALTER DATABASE esp_learning ADD REGION "azure";
ALTER DATABASE esp_learning ADD REGION "gcp";
-- 存活目标:REGION(跨 3 区 5 voters,任一区挂仍多数派);ZONE 目标适合单区演进期。
ALTER DATABASE esp_learning SURVIVE REGION FAILURE;

-- 其余 18 库同构(PRIMARY REGION aws + ADD azure/gcp + SURVIVE REGION FAILURE)。
-- 注意:esp 为迁移期遗留库(esp.users=202 / esp.learning_events=8000,主数据在域库),
--   可延迟到域库全绿后再迁移,或整体下线。本脚本先给域库。

-- -------------------------------------------------------------------------------------
-- Phase 2:表级 locality 声明
--   REGIONAL BY ROW 表需先加 region 列(不可由 user_id 哈希推导,必须是业务明确归属地):
--     ALTER TABLE t ADD COLUMN region crdb_internal_region NOT VISIBLE NOT NULL DEFAULT 'aws'
--       CONSTRAINT "row locality region" CHECK (region IN ('aws','azure','gcp'));
--     ALTER TABLE t SET LOCALITY REGIONAL BY ROW;
--   之后由 JWT region claim 写入 / UPDATE users.home_region 触发 CRDB 后台跨区搬迁。
-- -------------------------------------------------------------------------------------

-- ---------- GLOBAL(极小/读多/低频写):零跨区本地读 ----------
ALTER TABLE esp_content.scenarios             SET LOCALITY GLOBAL;
ALTER TABLE esp_learning.knowledge_points     SET LOCALITY GLOBAL;
ALTER TABLE esp_vocab.vocabulary              SET LOCALITY GLOBAL;
ALTER TABLE esp_listening.listening_exercises SET LOCALITY GLOBAL;
ALTER TABLE esp_reading.reading_passages      SET LOCALITY GLOBAL;
ALTER TABLE esp_llm.dialogue_personas         SET LOCALITY GLOBAL;
ALTER TABLE esp_writing.writing_templates     SET LOCALITY GLOBAL;
ALTER TABLE esp_gamification.achievements     SET LOCALITY GLOBAL; -- 成就定义(静态)

-- ---------- REGIONAL BY TABLE(单写者 / home 区集中):home=aws ----------
ALTER TABLE esp_learning.learning_event_outbox SET LOCALITY REGIONAL BY TABLE IN PRIMARY REGION;
ALTER TABLE esp_shared.undo_log                SET LOCALITY REGIONAL BY TABLE IN PRIMARY REGION;
ALTER TABLE esp_llm.llm_semantic_cache         SET LOCALITY REGIONAL BY TABLE IN PRIMARY REGION;
ALTER TABLE esp_shared.app_logs                SET LOCALITY REGIONAL BY TABLE IN PRIMARY REGION; -- 低价值,集中写可接受

-- ---------- REGIONAL BY ROW(用户维度读写):先加 region 列,再设 locality ----------
-- users(登录点读本地化;uk_email 跨区唯一由 CRDB 保证)
ALTER TABLE esp_auth.users ADD COLUMN region crdb_internal_region NOT VISIBLE NOT NULL DEFAULT 'aws'
  CONSTRAINT "row locality region" CHECK (region IN ('aws','azure','gcp'));
ALTER TABLE esp_auth.users SET LOCALITY REGIONAL BY ROW;

-- learning_events(最大表,append-heavy;事件按用户归属本地化写入)
ALTER TABLE esp_learning.learning_events ADD COLUMN region crdb_internal_region NOT VISIBLE NOT NULL DEFAULT 'aws'
  CONSTRAINT "row locality region" CHECK (region IN ('aws','azure','gcp'));
ALTER TABLE esp_learning.learning_events SET LOCALITY REGIONAL BY ROW;

-- sessions / session_turns(会话维 append+顺序读)
ALTER TABLE esp_speaking.sessions ADD COLUMN region crdb_internal_region NOT VISIBLE NOT NULL DEFAULT 'aws'
  CONSTRAINT "row locality region" CHECK (region IN ('aws','azure','gcp'));
ALTER TABLE esp_speaking.sessions SET LOCALITY REGIONAL BY ROW;

ALTER TABLE esp_learning.session_turns ADD COLUMN region crdb_internal_region NOT VISIBLE NOT NULL DEFAULT 'aws'
  CONSTRAINT "row locality region" CHECK (region IN ('aws','azure','gcp'));
ALTER TABLE esp_learning.session_turns SET LOCALITY REGIONAL BY ROW;

-- user_mastery(PK=(user_id,kp_id),region 随 user 走;SM2 复习同用户高频点写)
ALTER TABLE esp_learning.user_mastery ADD COLUMN region crdb_internal_region NOT VISIBLE NOT NULL DEFAULT 'aws'
  CONSTRAINT "row locality region" CHECK (region IN ('aws','azure','gcp'));
ALTER TABLE esp_learning.user_mastery SET LOCALITY REGIONAL BY ROW;

-- teacher / writing / speaking 业务表(用户归属维度)
ALTER TABLE esp_teacher.classes      ADD COLUMN region crdb_internal_region NOT VISIBLE NOT NULL DEFAULT 'aws'
  CONSTRAINT "row locality region" CHECK (region IN ('aws','azure','gcp'));
ALTER TABLE esp_teacher.classes      SET LOCALITY REGIONAL BY ROW;
ALTER TABLE esp_teacher.assignments  ADD COLUMN region crdb_internal_region NOT VISIBLE NOT NULL DEFAULT 'aws'
  CONSTRAINT "row locality region" CHECK (region IN ('aws','azure','gcp'));
ALTER TABLE esp_teacher.assignments  SET LOCALITY REGIONAL BY ROW;
ALTER TABLE esp_teacher.submissions  ADD COLUMN region crdb_internal_region NOT VISIBLE NOT NULL DEFAULT 'aws'
  CONSTRAINT "row locality region" CHECK (region IN ('aws','azure','gcp'));
ALTER TABLE esp_teacher.submissions  SET LOCALITY REGIONAL BY ROW;
ALTER TABLE esp_teacher.lesson_plans ADD COLUMN region crdb_internal_region NOT VISIBLE NOT NULL DEFAULT 'aws'
  CONSTRAINT "row locality region" CHECK (region IN ('aws','azure','gcp'));
ALTER TABLE esp_teacher.lesson_plans SET LOCALITY REGIONAL BY ROW;

ALTER TABLE esp_writing.writing_tasks     ADD COLUMN region crdb_internal_region NOT VISIBLE NOT NULL DEFAULT 'aws'
  CONSTRAINT "row locality region" CHECK (region IN ('aws','azure','gcp'));
ALTER TABLE esp_writing.writing_tasks     SET LOCALITY REGIONAL BY ROW;
ALTER TABLE esp_writing.writing_essays    ADD COLUMN region crdb_internal_region NOT VISIBLE NOT NULL DEFAULT 'aws'
  CONSTRAINT "row locality region" CHECK (region IN ('aws','azure','gcp'));
ALTER TABLE esp_writing.writing_essays    SET LOCALITY REGIONAL BY ROW;
ALTER TABLE esp_writing.writing_progress  ADD COLUMN region crdb_internal_region NOT VISIBLE NOT NULL DEFAULT 'aws'
  CONSTRAINT "row locality region" CHECK (region IN ('aws','azure','gcp'));
ALTER TABLE esp_writing.writing_progress  SET LOCALITY REGIONAL BY ROW;

ALTER TABLE esp_assessment.assessment_sessions ADD COLUMN region crdb_internal_region NOT VISIBLE NOT NULL DEFAULT 'aws'
  CONSTRAINT "row locality region" CHECK (region IN ('aws','azure','gcp'));
ALTER TABLE esp_assessment.assessment_sessions SET LOCALITY REGIONAL BY ROW;
ALTER TABLE esp_assessment.assessment_answers  ADD COLUMN region crdb_internal_region NOT VISIBLE NOT NULL DEFAULT 'aws'
  CONSTRAINT "row locality region" CHECK (region IN ('aws','azure','gcp'));
ALTER TABLE esp_assessment.assessment_answers  SET LOCALITY REGIONAL BY ROW;

ALTER TABLE esp_pronunciation.pronunciation_attempts ADD COLUMN region crdb_internal_region NOT VISIBLE NOT NULL DEFAULT 'aws'
  CONSTRAINT "row locality region" CHECK (region IN ('aws','azure','gcp'));
ALTER TABLE esp_pronunciation.pronunciation_attempts SET LOCALITY REGIONAL BY ROW;

ALTER TABLE esp_listening.listening_attempts ADD COLUMN region crdb_internal_region NOT VISIBLE NOT NULL DEFAULT 'aws'
  CONSTRAINT "row locality region" CHECK (region IN ('aws','azure','gcp'));
ALTER TABLE esp_listening.listening_attempts SET LOCALITY REGIONAL BY ROW;

ALTER TABLE esp_reading.reading_attempts ADD COLUMN region crdb_internal_region NOT VISIBLE NOT NULL DEFAULT 'aws'
  CONSTRAINT "row locality region" CHECK (region IN ('aws','azure','gcp'));
ALTER TABLE esp_reading.reading_attempts SET LOCALITY REGIONAL BY ROW;

ALTER TABLE esp_vocab.user_vocabulary ADD COLUMN region crdb_internal_region NOT VISIBLE NOT NULL DEFAULT 'aws'
  CONSTRAINT "row locality region" CHECK (region IN ('aws','azure','gcp'));
ALTER TABLE esp_vocab.user_vocabulary SET LOCALITY REGIONAL BY ROW;

ALTER TABLE esp_llm.dialogue_sessions ADD COLUMN region crdb_internal_region NOT VISIBLE NOT NULL DEFAULT 'aws'
  CONSTRAINT "row locality region" CHECK (region IN ('aws','azure','gcp'));
ALTER TABLE esp_llm.dialogue_sessions SET LOCALITY REGIONAL BY ROW;
ALTER TABLE esp_llm.dialogue_turns    ADD COLUMN region crdb_internal_region NOT VISIBLE NOT NULL DEFAULT 'aws'
  CONSTRAINT "row locality region" CHECK (region IN ('aws','azure','gcp'));
ALTER TABLE esp_llm.dialogue_turns    SET LOCALITY REGIONAL BY ROW;
ALTER TABLE esp_llm.llm_call_logs     ADD COLUMN region crdb_internal_region NOT VISIBLE NOT NULL DEFAULT 'aws'
  CONSTRAINT "row locality region" CHECK (region IN ('aws','azure','gcp'));
ALTER TABLE esp_llm.llm_call_logs     SET LOCALITY REGIONAL BY ROW;

ALTER TABLE esp_gamification.user_achievements ADD COLUMN region crdb_internal_region NOT VISIBLE NOT NULL DEFAULT 'aws'
  CONSTRAINT "row locality region" CHECK (region IN ('aws','azure','gcp'));
ALTER TABLE esp_gamification.user_achievements SET LOCALITY REGIONAL BY ROW;
ALTER TABLE esp_gamification.user_streaks       ADD COLUMN region crdb_internal_region NOT VISIBLE NOT NULL DEFAULT 'aws'
  CONSTRAINT "row locality region" CHECK (region IN ('aws','azure','gcp'));
ALTER TABLE esp_gamification.user_streaks       SET LOCALITY REGIONAL BY ROW;

-- 注:esp_speaking.sessions 与 esp_auth.sessions 同时存在,本脚本按当前主数据所在库处理;
--   遗留 esp 库(esp.sessions/esp.learning_events 等)迁移优先级低,见 Phase 1 注释。

-- -------------------------------------------------------------------------------------
-- Phase 3:验证查询
-- -------------------------------------------------------------------------------------
-- 1. 数据库级:SELECT database_name, primary_region, regions, survival_goal FROM [SHOW DATABASES];
--    期望:primary_region='aws', regions={aws,azure,gcp}, survival_goal='region'。
-- 2. 表级:SELECT * FROM crdb_internal.tables WHERE locality LIKE '%REGIONAL%' OR locality LIKE '%GLOBAL%';
-- 3. RBR 行搬迁:UPDATE users SET home_region='gcp' WHERE ... → SHOW RANGES FROM TABLE users 观察搬迁。
-- 4. 分区分布:SELECT city, count(*) FROM [SHOW RANGES FROM TABLE esp_learning.learning_events];
