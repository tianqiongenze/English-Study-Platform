# ADR:写作增强服务(writing-coach)架构决策

> 日期:2026-08-05
> 状态:已采纳(方案 A — 独立解耦服务)
> 关联:ADR-09(Litestar+Granian)、读写分离、Pulsar EDA、esp-cache-py 多级缓存

## 1. 背景

现有 `writing-py` 负责 AI 批改/润色/翻译/作文 review(Litestar+Granian,LangGraph)。
用户要求新增能力:**英语写作模板/范文推荐、批改与评分、写作能力提升方案及其实施**。
这些是"教学辅导"类能力,与 writing-py 的"单次批改"职责不同,且需持久化(模板库、提升计划、进度跟踪)。

## 2. 方案抉择

### 方案 A:独立新服务 `writing-coach`(推荐,采纳)
- 新增微服务,与 writing-py 解耦。writing-py 保持"无状态 AI 批改引擎",writing-coach 承担"写作教学辅导"。
- **优点**:职责单一、可独立扩缩容、不污染 writing-py、符合现有多语言微服务架构(Go/Java/Python 分治)。
- **缺点**:多一个服务部署;与 writing-py 有协同调用。

### 方案 B:扩 writing-py
- 把模板/范文/提升方案塞进 writing-py。
- **否决**:writing-py 已是 AI 重计算服务,混入 CRUD+教学逻辑违反单一职责;LangGraph 状态机与模板推荐耦合难维护。

### 方案 C:Java 服务
- **否决**:Python 生态(模板/范文 NLP 检索、LangChain 提升方案生成)更合适,与 writing-py 同栈便于复用 esp-cache-py/esp-db-py。

## 3. 选型(方案 A)

| 维度 | 决策 | 理由 |
|---|---|---|
| 语言/框架 | Python + Litestar + Granian | 与 writing-py 同栈,复用公共包,AI 生态 |
| 端口 | 8085 | 接续 8084(emotion) |
| 数据库 | TiDB(读写分离,esp-db-py) | 模板/范文/计划需持久化 |
| 缓存 | esp-cache-py(L1 cachetools + L2 Redis Cluster) | 模板/范文热点缓存 |
| 消息 | Pulsar `persistent://esp/business/writing-coach-events` | 提升方案生成异步化 |
| AI | anthropic SDK + LangGraph | 提升方案生成多步 agent |
| 服务协同 | Dubbo Triple 不接(纯 REST);被 gateway/learning 调用 | Python 侧 REST 简单 |
| 可观测 | OTel agent(python -m opentelemetry.agent)→ otel-collector | trace 链路贯通 |

## 4. 核心能力(API)

| 端点 | 方法 | 用途 |
|---|---|---|
| `/api/writing-coach/templates` | GET | 按类别(议论文/应用文/图表作文)推荐模板 |
| `/api/writing-coach/essays` | GET | 范文推荐(按主题/难度) |
| `/api/writing-coach/assess` | POST | 综合批改+评分(调 writing-py grade + 自有 rubric) |
| `/api/writing-coach/plan` | POST | 生成写作能力提升方案(基于历史薄弱点) |
| `/api/writing-coach/plan/{user_id}` | GET | 查询用户提升方案 |
| `/api/writing-coach/progress` | POST | 记录练习进度(实施提升方案) |
| `/healthz` | GET | 健康 |

## 5. 与其他服务协同

- **writing-py**:writing-coach `/assess` 内部 httpx 调 `writing-py:80/api/writing/grade` 拿基础评分,再叠加 rubric 维度评分(语法/连贯/词汇/任务完成)。
- **learning-java**:writing-coach `/plan` 调 `learning-java:80/api/learning/mastery/weak?user_id=` 拿用户薄弱点,据此生成针对性提升方案。
- **gateway-go**:新路由 `/api/writing-coach` → writing-coach:8085(Ingress + NodePort 30085)。
- **Pulsar**:提升方案生成完成发事件,learning 可消费更新 mastery。

## 6. 数据模型(新增 TiDB 表)

- `writing_templates(id, category, title, structure, difficulty, created_at)`
- `writing_essays(id, category, title, content, difficulty, word_count)`
- `writing_plans(id, user_id, weak_points_json, plan_json, status, created_at, updated_at)`
- `writing_progress(id, user_id, plan_id, task_id, completed, score, created_at)`

## 7. 部署

- `deploy/k8s/30-other-services.yaml` 加 writing-coach Deployment(镜像 esp/writing-coach:1.0.0,端口 8085)
- `deploy/k8s/45-httptest-nodeports.yaml` 加 NodePort 30085
- `deploy/k8s/40-ingress.yaml` 加 `/api/writing-coach` 路由
