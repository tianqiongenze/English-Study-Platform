# Schema-per-Service Phase 1 执行计划(老表在线迁移)

> 状态:计划就绪(2026-08-07),**待维护窗口执行**
> 关联:`docs/design/adr-schema-per-service.md` §5 Phase 1、`docs/design/schema-per-service-phase1.sql`(DDL)
> 前置:Phase 0 ✅ 完成(esp_auth/esp_speaking/esp_writing/esp_teaching/esp_learning/esp_shared 6 schema 已建)

## 1. 风险评估(为什么不能立即执行)

- **不可逆**:`RENAME TABLE esp.X TO esp_Y.X` 是在线操作(TiDB 秒级不锁表),但服务 JDBC URL 仍指向 `esp` 库,RENAME 后查旧表名 → `1146 Table doesn't exist`,**服务立即断**。
- **影响面**:10 个在跑服务(bff/gateway/learning/teacher/writing-py/writing-coach 等)+ seata 全局事务(undo_log)+ 30 亿数据(迁移的是表名,非数据,但服务连接需同步改)。
- **必须串行**:每批 RENAME → 改对应服务连接串(env)→ rollout restart → e2e 验证。任一步遗漏即断服务。单节点 VM 资源紧张,串行重启 5 批 + 每批验证,预计 40-60 分钟维护窗口。
- **app_logs 跨服务**:6 服务共写,RENAME 到 esp_shared 后这些服务的 app_logs 写入路径需同步改(或保留 esp 别名)。

## 2. 执行顺序(ADR §5,bff→gateway→writing→teacher→learning)

| Batch | 服务 | 迁移表 | 目标 schema | 连接串改造 |
|---|---|---|---|---|
| 1 | bff-java | users | esp_auth | `WRITE/READ_DB_URL` 库名 esp→esp_auth |
| 2 | gateway-go | scenarios, sessions | esp_speaking | `WRITE/READ_DB_NAME` esp→esp_speaking(确认 gateway 用 DB_NAME env) |
| 3 | writing-py + writing-coach | writing_tasks/templates/essays/plans/progress | esp_writing | writing-py DB URL 库名改 esp_writing |
| 4 | teacher-java | classes/assignments/submissions/lesson_plans | esp_teaching | `WRITE/READ_DB_URL` 库名 esp→esp_teaching |
| 5 | learning-java | user_mastery/knowledge_points/learning_events/session_turns | esp_learning | `WRITE/READ_DB_URL` 库名 esp→esp_learning |
| 公共 | 6 服务 | app_logs | esp_shared | 各服务 app_logs 写入路径改 esp_shared(或保留 esp 共享视图) |

## 3. 每批执行步骤(模板)

1. **预检**:确认目标 schema 存在 + 源表在 esp + 服务当前连接串。
2. **RENAME**:`scripts/db_query.sh` 执行本批 DDL(phase1.sql 对应段)。
3. **改连接串**:`kubectl -n esp-prod set env deploy/<svc> WRITE_DB_URL=... READ_DB_URL=...`(库名换新 schema)。
4. **重启**:`kubectl -n esp-prod rollout restart deploy/<svc>` + 等 Ready。
5. **验证**:
   - health 端点 UP;
   - 该服务核心 API 冒烟(e2e 子集);
   - 该服务日志无 1146/连接错误;
   - TiDB 查新 schema 表可读。
6. **回滚预案**:若验证失败,`RENAME TABLE esp_Y.X TO esp.X` 回滚 + 还原连接串 + 重启。

## 4. RBAC(Phase 1 收尾,机制根治跨服务越界)

每服务建独立 DB 用户,只授权自己 schema 读写 + esp_shared.app_logs 写:
```sql
CREATE USER 'svc_bff'@'%' IDENTIFIED BY '<rand>';
GRANT SELECT,INSERT,UPDATE,DELETE ON esp_auth.* TO 'svc_bff'@'%';
GRANT INSERT ON esp_shared.app_logs TO 'svc_bff'@'%';
-- 同理 svc_gateway(esp_speaking)/svc_writing(esp_writing)/svc_teacher(esp_teaching)/svc_learning(esp_learning)
```
跨服务直连他人表在 DB 层被拒(ADR §4 治理 scenarios/sessions JOIN 等)。**RBAC 启用需先确认所有跨服务访问已改 RPC(Phase 2),否则会断**。故 RBAC 作为 Phase 2 完成后的收尾门禁。

## 5. 当前状态(2026-08-07)

- ✅ Phase 1 DDL 脚本就绪:`docs/design/schema-per-service-phase1.sql`(5 batch + 公共 app_logs)。
- ✅ 本计划文档就绪。
- ⏳ **待执行**:需用户确认维护窗口(可接受 40-60 分钟断服 + 串行重启 5 批服务)。
- ⏳ Phase 2(耦合治理代码改)前置:emotion→Pulsar、learning 推荐改 RPC,需先做才能开 RBAC。

## 6. 决策建议

Phase 1 是真正的"在线迁移 + 服务重启"工程,非纯 DDL。**建议**:
- 若用户要立即推进:选低峰窗口,按 Batch 1→5 严格串行,每批验证后再下一批,准备回滚脚本。
- 若用户接受延后:Phase 1 脚本/计划已就绪,等正式维护窗口或多节点环境(可蓝绿)执行;当前单节点验证环境保持 esp 共享库不停服。
- RBAC 必须等 Phase 2 完成后启用(否则跨服务访问全断)。
