# ESP 向量检索 / 图谱关系 可行性调研

> 调研日期:2026-07-31。目的:判断是否引入 Milvus / Neo4j,为架构决策提供依据。

## 1. 功能需求清单(按需求来源核对)

| 功能点 | 文档出处 | 是否真实需求 | 需要向量检索? | 需要图遍历? | TiDB 够用? |
|---|---|---|---|---|---|
| 知识图谱分析(薄弱点定位) | SRS FR-2.1 | 是 | 否 | 否(树形 parent_id) | 够(user_mastery 排序) |
| 动态难度调整 | SRS FR-2.2 | 是 | 否 | 否 | 够(滑动窗口阈值) |
| 艾宾浩斯复习规划 | SRS FR-2.3 | 是 | 否 | 否 | 够(next_review_at) |
| 学情事件流 | SRS FR-2.4 | 是 | 否 | 否 | 够(Disruptor→TiDB) |
| 写作润色/评分/翻译 | SRS FR-3.1/3.2/3.3 | 是 | 否 | 否 | 够(LLM 直连,无召回) |
| 教案生成 | SRS FR-4.1 | 是 | 否 | 否 | 够(模板/LLM) |
| 学情 OLAP 分析 | SRS FR-4.2 | 是 | 否 | 否 | 够(TiFlash 列存) |
| 作业智能评阅 | SRS FR-4.3 | 是 | 否 | 否 | 够(启发式评分) |
| 相似题目/相似范文/相似课程推荐 | 无 | 否 | — | — | — |
| 个性化内容推荐(基于行为) | 无 | 否 | — | — | — |
| 知识点前置/后续依赖路径 | 无(仅 parent_id 树) | 否 | — | — | — |
| 用户-课程-老师-知识点复杂关系查询 | 无 | 否 | — | — | — |

**结论**:现有产品/设计文档中**没有任何功能**明确要求向量检索(相似召回)或图遍历(多跳关系路径)。"知识图谱"在 ESP 语境下指**知识点树 + 掌握度**,是树形邻接表 + 数值排序,关系型完全胜任。

## 2. 现状:LLM 调用与 schema

### 2.1 LLM 调用现状
- 统一抽象:`LlmProvider`(Go `services/gateway-go/src/llm`、Py `services/writing-py/app/llm.py`),方法为 `chat(system, user) -> str`,**纯文本对话**,无 embedding 接口。
- writing-py:LangGraph 三节点 `grade/polish/notes`(`services/writing-py/app/graph_state.py`),每个节点 `llm.chat(system, user)` 单轮调用,**不召回相似范文**,评分由 LLM 直接给出 + `extract_json` 解析。
- gateway-go:`dialog.Engine` 拼历史轮次(system + history + user)调 LLM,ASR/TTS 均为 Mock(`asr.MockProvider`/`tts.MockProvider`),**无 ASR 语义匹配**。
- teacher-java:`LessonPlanGenerator` 模板化 + 可替换 LLM;`autoGrade` 是长度/句点/连词启发式,**无 embedding**。
- 全代码库 grep `embed|vector|similarity|cosine` 在业务源码中**零命中**(仅 .venv 依赖误命中)。

### 2.2 schema 现状(docs/design/schema.sql,11 表)
- `knowledge_points(id, code, title, parent_id, difficulty)`:**自引用邻接表**,树形知识点,无前置/后续依赖边表。
- `user_mastery(user_id, kp_id, mastery_score, next_review_at, ...)`:掌握度 + 复习调度,复合主键。
- `learning_events`、`writing_tasks(essay TEXT)`、`lesson_plans`、`classes/assignments/submissions`:均为扁平关系表。
- **无向量列**、**无图结构边表**、**无题库/课程/范文池表**(writing_tasks 是用户提交记录,非范文库)。

## 3. 基础设施现状

| 组件 | ns | 状态 | 镜像/版本 | 备注 |
|---|---|---|---|---|
| milvus | ekb-m2-aws-us-east-1 | Running (1/1, 109d) | `milvusdb/milvus:v2.5.0` | 同样在 azure/gcp 分区 Running |
| milvus-minio | ekb-m2-aws-us-east-1 | Running | `minio:RELEASE.2024-05-28T17-19-04Z` | Milvus 对象存储后端 |
| neo4j | ekb-m2-aws-us-east-1 | Running (1/1, 109d) | `neo4j:5.25.1-community-bullseye` | community 版,单实例 |

基础设施**已就绪且在运行**,引入无需重新部署,仅需业务服务接入。

## 4. 抉择建议

### Milvus:**不引入(当前阶段)**
理由:
1. 现有功能(写作批改、学情、复习)均由 LLM 直连 + TiDB 关系查询实现,**无相似召回需求**。
2. PRD/SRS 未出现"相似题目/范文/课程/个性化推荐"等功能诉求。
3. 引入会新增 embedding 生成链路(需调 embedding API 灌库)、向量同步一致性维护、服务依赖,与当前 5000 CCU 闭环目标偏离。

**保留可能性**:若未来产品增加"相似范文参考""错题相似题推荐""基于学习行为的个性化内容推荐",则 Milvus v2.5.0 已在集群就绪,可直接接入 writing-py / learning-java,embedding 可复用 Anthropic/OpenAI embedding API 或本地模型。

### Neo4j:**不引入(当前阶段)**
理由:
1. ESP 的"知识图谱"是**树形**(`parent_id`),非多跳有向图,TiDB 邻接表 + 递归 CTE 足够。
2. 无"知识点前置/后续依赖路径""用户-课程-老师多跳关系查询"等图遍历需求。
3. 复习路径是艾宾浩斯时间触发,非图路径推荐。

**保留可能性**:若未来增加"知识点依赖 DAG + 前置薄弱点阻断分析""多跳学习路径推荐",Neo4j 5.25.1 已就绪。

### 若引入的重构影响面(仅作未来参考)
- Milvus:writing-py(范文 embedding 灌库 + 召回)、learning-java(题目 embedding)、新增 embedding 生成 worker、schema 加 vector_id 映射表。
- Neo4j:learning-java(知识点 DAG 导入 + 路径查询)、schema 加 kp_dependency 边表或全量迁移知识点到 Neo4j。

## 5. 量级预估(若未来引入)

### 向量库
- 范文 embedding:writing_tasks 按 30 亿测试数据规模估算,真实范文池若建 1~10 万篇范文,向量 1~10 万条 × 1536 维 ≈ 0.6~6GB(Milvus 无压力)。
- 题目 embedding:题库假设 1~5 万题,同量级。
- 课程/知识点 embedding:数百~数千条,可忽略。
- **合计预估:< 20 万向量**,Milvus 单实例绰绰有余。

### 图库
- 知识点节点:数百~数千(按 schema knowledge_points 规模)。
- 依赖边:若建 DAG,约数千~数万条。
- 用户-行为节点:不建议入图(高频写,留在 TiDB),仅知识点 DAG 入图。
- **合计预估:< 1 万节点 / < 5 万边**,Neo4j community 单实例足够。

## 6. 一句话结论

**Milvus 和 Neo4j 基础设施已在集群 Running(可用),但当前 ESP 无任何真实功能需要向量检索或图遍历,两者均暂不引入;待产品出现"相似召回"或"知识点 DAG 路径推荐"需求时再接入,零基建成本。**
