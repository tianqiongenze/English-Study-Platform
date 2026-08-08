# English-Study-Platform — 需求分析文档 (SRS)

| 项 | 值 |
|---|---|
| 文档编号 | ESP-SRS-001 |
| 版本 | 5.0 |
| 日期 | 2026-07-31 |
| 状态 | 基线评审稿(对齐实现 + 互联网大厂标准 + 异地多活/数字人/智能体) |
| 编制 | 高校 AI 与大数据专业教授级高工 |
| 依据 | 《AI英语教育平台的核心功能.pdf》 |
| 上游 | ESP-PRD-001 产品需求文档 |

---

## 第 1 章 引言

### 1.1 编写目的
本需求规格说明书(SRS)对 English-Study-Platform(以下简称 ESP / Home-Tutor)的功能需求、非功能需求、用户角色、用例、业务流程、数据需求、约束与验收标准进行完整、可追溯的定义,作为后续架构设计、详细设计、编码、测试、部署与验收的唯一需求基线。每条需求均可双向追溯到 PRD 与下游设计/代码/测试。

### 1.2 适用读者
- 架构师 / 后端工程师(Go / Java / Python)
- 前端工程师(React)
- 测试与性能工程师(Locust)
- DevOps / SRE(K8s / Docker)
- 产品与教学业务方

### 1.3 项目背景
PDF 描述的现代 AI 英语教育平台已超越传统背单词软件,依托大语言模型(LLM)、自适应算法与语音识别技术,构建全方位智能教学体系,从"工具化"向"人格化"演进,成为 24h 在线、永不疲倦、最懂学习进度的私人教练。ESP 即按此愿景落地。

### 1.4 术语与缩略语
| 术语 | 含义 |
|---|---|
| CCU | Concurrent Connected User,同时在线用户数 |
| LLM | Large Language Model,大语言模型 |
| ASR/TTS | 语音识别 / 语音合成 |
| SRS / SAD / SDD | 需求规格 / 架构设计 / 详细设计文档 |
| FR / NFR | 功能需求 / 非功能需求 |
| UC | Use Case 用例 |
| OLTP / OLAP | 在线事务处理 / 在线分析处理 |
| HTAP | 混合事务分析处理 |
| R/W Split | 读写分离 |
| Disruptor | LMAX 无锁环形队列 |
| Ebbinghaus | 艾宾浩斯遗忘曲线 |
| P95 / P99 | 百分位延迟 |
| QPS / TPS | 每秒查询 / 事务数 |
| SLO / SLI | 服务等级目标 / 指标 |
| ADR | Architecture Decision Record 架构决策记录 |
| RBAC | 基于角色的访问控制 |
| DLQ | Dead Letter Queue 死信队列 |
| BFF | Backend For Frontend 面向前端的后端 |
| TiKV / TiFlash / TiCDC | TiDB 行存引擎 / 列存引擎 / 跨集群增量同步组件 |
| 异地多活 | 跨区域容灾部署,主写区 + 只读副本 |
| 数字人 Digital Human | 虚拟形象(唇形/表情)随 TTS+情绪驱动 |
| 智能体 Agent | 具备多步推理/工具调用/状态机的 LLM 编排单元 |
| LangGraph | 基于有状态图的 LLM 智能体编排框架 |

### 1.5 参考文档
1. 《AI英语教育平台的核心功能.pdf》—— 功能愿景来源
2. ESP-PRD-001 产品需求文档(见 `docs/product/01-PRD.md`)
3. ESP-原型设计(见 `docs/product/02-原型设计.md`)
4. IEEE 830-1998 SRS 推荐实践
5. ISO/IEC 25010 软件产品质量模型
6. Google SRE SLO/SLI 实践

---

## 第 2 章 总体描述

### 2.1 产品愿景
构建一套面向 C 端学习者 + B 端教师的双角色、多模态、自适应的 AI 英语教育平台,具备:
1. 沉浸式 AI 口语陪练(多场景角色扮演 + 即时纠错 + 启发式引导)
2. 个性化自适应学习路径(知识图谱 + 动态难度 + 艾宾浩斯复习)
3. 智能写作辅助与深度批改(语义级润色 + 逻辑评估 + 预测分值)
4. 全能型教师端工具(教案生成 + 学情大数据 + 作业智能评阅)
5. 情感化与多模态交互(情绪感知 + 视觉辅助)

### 2.2 系统目标(量化 SLO)
| 编号 | 指标(SLI) | 目标值(SLO) | 测量手段 |
|---|---|---|---|
| G-01 | 同时在线用户 | 5000 CCU | Locust WebSocket 长连接压测 |
| G-02 | 口语会话文本引导 P95 延迟 | < 800ms | 网关 Prometheus histogram |
| G-03 | 口语会话文本引导 P99 延迟 | < 1500ms | 同上 |
| G-04 | 写作批改吞吐 | ≥ 50 req/s | Locust HTTP 压测 |
| G-05 | 学习路径查询 P99 | < 200ms | Locust |
| G-06 | 学情事件入口吞吐 | ≥ 5000 events/s | Disruptor benchmark + 压测 |
| G-07 | 系统可用性 | ≥ 99.9% | K8s 多副本 + 健康检查 + 监控告警 |
| G-08 | 水平扩展 | 无状态服务线性扩容 | K8s HPA |
| G-09 | 单元测试覆盖率(核心逻辑) | ≥ 80% | 覆盖率工具 |
| G-10 | 错误预算(月度) | 失败率 < 0.1% | Locust 失败统计 |

### 2.3 用户角色与特征
| 角色 | 标识 | 特征 | 主要使用模块 |
|---|---|---|---|
| 学习者 | Learner | C 端,以提升英语能力为目标,使用移动/桌面浏览器 | 1、2、3、5 |
| 教师 | Teacher | B 端,管理班级与作业,关注教学效率 | 4(兼看 2 学情) |
| 管理员 | Admin | 平台运营,管理用户/场景/内容/配额 | 全局后台 |

### 2.4 运行环境
- 浏览器:Chrome/Edge/Safari 最新两个大版本
- 网络:公网 HTTPS/WSS;内网服务间 HTTP/gRPC
- 服务端:Linux x86_64,K8s 集群
- 本地开发验证:Docker Compose(含 TiDB 集群)

### 2.5 设计约束
- C-1:多语言微服务(Go 实时面 / Java 核心业务面 / Python AI 面),按最优原则选型。
- C-2:Java 核心业务服务须在合适位置采用 LMAX Disruptor 无锁消息队列。
- C-3:数据库采用分布式 TiDB(含 TiKV 行存 + TiFlash 列存);架构须引入**读写分离**。
- C-4:消息中间件 Apache Pulsar;缓存 Redis Cluster。
- C-5:LLM 采用 Anthropic Claude API;语音 ASR/TTS 本期 Mock(接口与真实厂商对齐)。
- C-6:面向 K8s 生产部署;同时提供 Docker Compose 本地全栈验证。
- C-7:TDD 测试驱动开发,核心逻辑先写测试。
- C-8:API Key / 敏感配置走 env / K8s Secret,禁止硬编码。
- C-9:BFF/边缘网关用 Spring Cloud Gateway 4.x,叠加 Spring Cloud Alibaba(Nacos 注册配置 + Sentinel 流控熔断 + Dubbo Triple 服务间 RPC);Java 服务间优先 Dubbo Triple(见 SAD ADR-08)。
- C-10:Python AI 服务采用 Litestar + Granian(全异步,Rust ASGI 内核),LLM 调用异步化(见 SAD ADR-09)。
- C-11:基础设施(TiDB/Redis/Pulsar)在容器中已存在则复用,不重复部署(见 SAD ADR-07/10/11)。

### 2.6 假设与依赖
- A-1:学习者网络下行 ≥ 2Mbps,上行 ≥ 1Mbps(音频流)。
- A-2:LLM API 可达且配额充足;压测时切 mock 模式规避费用与限流。
- A-3:K8s 集群具备 TiDB Operator / Pulsar Helm chart 部署能力。

---

## 第 3 章 功能需求(对应 PDF 5 大模块)

> 每条需求含:唯一编号、优先级(P0 必须 / P1 重要 / P2 可选)、描述、输入、输出、验收准则、实现落点。

### 3.1 模块一:沉浸式 AI 交互口语陪练(对应服务 gateway-go)

#### FR-1.1 多场景角色扮演(P0)
- **描述**:AI 模拟真实生活场景(机场登机、面试、咖啡店点餐、自定义),不按预设剧本,自由对话。
- **输入**:用户选择场景 code 或自定义场景描述。
- **输出**:建立会话,返回 session_id 与首句 AI 问候。
- **验收**:GET `/api/scenarios` 返回 ≥ 3 预置场景;创建会话后 1s 内收到 AI 首句。
- **落点**:`scenario.Repository`(内置 airport/interview/cafe)、`/api/sessions`。

#### FR-1.2 即时语音纠错(P0)
- **描述**:实时捕捉发音(音标级)、语调、重音;语法错误即时指出并给地道 Rephrasing。
- **输入**:WebSocket 音频帧序列(audio_chunk)+ 可选文本输入。
- **输出**:ai_reply / correction 帧(original→corrected, phoneme)、rephrasing 建议。
- **验收**:输入 "I wants coffee" → 收到 correction "I wants"→"I want" + rephrasing;Mock ASR 将音频帧映射为预设文本。
- **落点**:`dialog.Engine`(正则 `I wants`→`I want` 修正)+ `asr.MockProvider`。

#### FR-1.3 启发式对话引导(P0)
- **描述**:用户卡壳(静默 > N 秒或发送 "/hint")时,AI 提供若干建议选项或关键词。
- **输入**:超时信号或显式 hint 请求。
- **输出**:hint 帧 {options:[...], keywords:[...]}。
- **验收**:触发 hint 后返回 2~4 个选项,均与当前场景相关。
- **落点**:`dialog.Engine.Hint`(按场景返回选项)。

#### FR-1.4 会话持久化与回放(P1)
- **描述**:会话逐轮落库,可保存/回放/生成学习笔记。
- **输入**:会话结束(end 帧)。
- **输出**:session_turns 持久化;`GET /api/sessions/{id}` 返回完整对话与摘要。
- **验收**:结束后查询返回所有轮次;摘要含错误统计与建议。
- **落点**:`session.Manager`(Redis 历史裁剪 50 轮)。

### 3.2 模块二:个性化自适应学习路径(对应服务 learning-java)

#### FR-2.1 知识图谱分析(P0)
- **描述**:初测 + 日常练习定位薄弱环节(虚拟语气、商务词汇等)。
- **输入**:初测答题结果。
- **输出**:user_mastery 基线,薄弱知识点列表。
- **验收**:初测后 mastery_score 落库;返回薄弱点按掌握度升序。
- **落点**:`LearningService.initMastery`(写主库)、`weakestPoints`(读副本)。

#### FR-2.2 动态难度调整(P0)
- **描述**:根据实时正确率调整词汇难度、听力语速;困难时降维解析。
- **输入**:学情事件(event_type=answer)。
- **输出**:下一题难度档位(1~5)、语速倍率。
- **验收**:正确率 > 80% 难度上调;正确率 < 40% 难度下调并附降维解析。
- **落点**:`DifficultyAdjuster`(滑动窗口,阈值 0.8/0.4,档位 1~5,语速 0.8~1.4x)。

#### FR-2.3 个性化复习规划(P0)
- **描述**:艾宾浩斯遗忘曲线,临界点精准推送曾学单词/语法。
- **输入**:当前时间。
- **输出**:今日复习清单(next_review_at <= now 的知识点)。
- **验收**:按 1d/2d/4d/7d/15d/30d 间隔计算 next_review_at;查询返回到期项。
- **落点**:`EbbinghausScheduler`(间隔 [1,2,4,7,15,30] 天,答错重置)。

#### FR-2.4 学情事件流(P0,高性能路径)
- **描述**:所有练习事件经 Disruptor 高并发入队 → 异步落库 + 触发算法。
- **输入**:POST `/api/learning/event` {event_type, payload}。
- **输出**:202 Accepted(立即返回,不阻塞)。
- **验收**:单实例 Disruptor 入队 ≥ 5000 events/s;事件最终落 TiDB learning_events。
- **落点**:`LearningEventDisruptor`(RingBuffer 65536,批量 200/500ms 落库)。

### 3.3 模块三:智能写作辅助与深度批改(对应服务 writing-py)

#### FR-3.1 语法与风格润色(P0)
- **描述**:单复数、句式多样性,建议简单句合并为长难句。
- **输入**:{text}。
- **输出**:{polished, suggestions[]}(含原句→改写、原因)。
- **验收**:输入含主谓不一致错误时 suggestions 非空;polished 语法正确。
- **落点**:`WritingService.polish` + `MockLlmProvider`/`AnthropicLlmProvider`。

#### FR-3.2 逻辑连贯性评估(P0)
- **描述**:托福/雅思作文论点清晰度、段落过渡分析,预测分值。
- **输入**:{prompt, essay}。
- **输出**:{score(0~30/9), feedback, grammar[], coherence{}}。
- **验收**:返回 0~30 分数与连贯性子项;feedback 含具体改进点。
- **落点**:`WritingService.grade`(`extract_json` 兜底解析)。

#### FR-3.3 翻译与对照学习(P1)
- **描述**:长文本一键精读,生词/短语从语境提取生成笔记。
- **输入**:{text}。
- **输出**:{translation, vocab[]}(词形、释义、例句)。
- **验收**:返回中文翻译 + 不少于 3 个生词条目。
- **落点**:`WritingService.translate`。

### 3.4 模块四:全能型教学辅助工具(教师端,对应服务 teacher-java)

#### FR-4.1 一键生成教案(P0)
- **描述**:输入主题(如"环保")自动生成课文、习题、课堂游戏、讨论题。
- **输入**:{topic, level}。
- **输出**:教案 JSON{passage, exercises[], games[], discussion[]}。
- **验收**:返回结构完整;习题 ≥ 3;讨论题 ≥ 2。
- **落点**:`LessonPlanGenerator`(模板化,可替换 LLM)。

#### FR-4.2 学情大数据分析(P0)
- **描述**:全班共性难题分析,课堂针对性讲解建议。
- **输入**:class_id。
- **输出**:共性薄弱知识点 Top-N、建议讲解点。
- **验收**:基于 TiDB TiFlash 列存 OLAP 查询;返回排序后的难点。
- **落点**:`TeacherService.classAnalytics`(`@DataSource(READ, tiflash=true)` + SQL hint)。

#### FR-4.3 作业智能评阅(P0)
- **描述**:朗读/书面作业 90% 预评分,教师终审。
- **输入**:assignment_id。
- **输出**:每份 submission auto_score + auto_feedback,status=auto_graded。
- **验收**:批量评阅后 status 迁移;教师可 finalize。
- **落点**:`TeacherService.autoGrade`(长度+句点+连词启发式评分) + `finalize`。

### 3.5 模块五:情感化与多模态交互(对应服务 emotion-svc)

#### FR-5.1 情绪感知(P1)
- **描述**:感知用户语气,语音语调给予鼓励或安慰。
- **输入**:{audio/text}(本期文本为主,音频 Mock)。
- **输出**:{emotion, encouragement}。
- **验收**:输入含负面情绪词时返回安慰语;正面时返回鼓励语。
- **落点**:`sense_emotion`(负面/正面关键词集分类)。

#### FR-5.2 视觉辅助(P1)
- **描述**:根据所学单词实时生成图片辅助理解(如 "Breathtaking" → 山河图)。
- **输入**:{word, context}。
- **输出**:{image_url, alt}。
- **验收**:返回可访问 image_url(Mock 本期返回占位图 URL)。
- **落点**:`visual_for_word`(`placehold.co` 占位图)。

### 3.6 模块六:数字人与智能体编排(横切,对应前端 + 各服务 + LLM)

#### FR-6.1 数字人(P1)
- **描述**:口语陪练中数字人形象,唇形随 TTS 音频、表情随情绪(emotion-svc)驱动;三端(Web/H5/小程序)均可呈现。
- **输入**:WS `ai_reply` 帧(`data`=base64 TTS 音频 + `emotion` 标签)。
- **输出**:前端 `Avatar` 组件驱动唇形/表情。
- **验收**:AI 回复时数字人唇形与音频同步(延迟 ≤200ms),情绪标签切换表情;降级为静态头像。
- **落点**:前端 `Avatar` 组件 + gateway-go `ai_reply` 帧附 TTS+emotion + emotion-svc 情绪识别。

#### FR-6.2 智能体编排(P1)
- **描述**:四类智能体(口语/写作/学习路径/教案)主动建议,带"💡"标识可采纳/忽略,显示依据;LLM 统一 `LlmProvider` 抽象。
- **输入**:各模块上下文(会话/作文/学情/主题)。
- **输出**:智能体建议(纠错/润色/路径/教案)。
- **验收**:各模块均能触发智能体建议;写作智能体支持多步链(批改→润色→人工干预→学习笔记)。
- **落点**:gateway-go `dialog.Engine`(口语)、writing-py `WritingService`+LangGraph(写作)、learning-java `LearningService`(路径)、teacher-java `LessonPlanGenerator`(教案)。

---

## 第 4 章 用例(核心 UC)

### UC-01 学习者进行口语陪练
1. 学习者登录 → 选择场景"机场登机" → 创建会话
2. 通过麦克风说话(音频帧)→ Mock ASR → 文本
3. 网关调用 LLM 生成回复 + 纠错 + rephrasing → 推送帧
4. 卡壳 → 触发 hint
5. 结束 → 生成笔记落库

### UC-02 学习者走自适应学习路径
1. 初测 → 生成 mastery 基线
2. 每日练习 → 学情事件入 Disruptor → 落库 + 动态难度
3. 到期复习项推送 → 复习 → 更新 next_review_at

### UC-03 学习者提交作文批改
1. 输入 prompt + essay → POST `/api/writing/grade`
2. Python 服务调 Claude → 返回分数 + feedback
3. 落库 writing_tasks

### UC-04 教师生成教案 + 布置作业
1. 输入主题"环保" → 生成教案
2. 基于教案创建 assignment → 推送给班级
3. 查看班级学情分析

### UC-05 教师智能评阅作业
1. 触发 assignment 批量 auto-grade
2. 系统逐份调 AI → auto_score / auto_feedback
3. 教师终审 finalize

---

## 第 5 章 非功能需求(ISO 25010 映射)

| 编号 | 类别 | 需求 | 落点 |
|---|---|---|---|
| NFR-PERF-1 | 性能效率 | 满足第 2.2 章全部 G 指标 | 各服务 + 压测 |
| NFR-PERF-2 | 性能效率 | 读写分离:写主库,读副本;读副本延迟 < 1s | `RoutingDataSource` + TiDB Follow Read |
| NFR-REL-1 | 可靠性 | 服务多副本,单 Pod 故障不影响可用性 | K8s Deployment minReplicas≥2 |
| NFR-REL-2 | 可靠性 | Pulsar 消息持久化,消费失败重试 + 死信 | Pulsar topic + DLQ |
| NFR-SEC-1 | 安全性 | JWT(RS256)鉴权,角色 RBAC | Ingress/BFF 校验 |
| NFR-SEC-2 | 安全性 | API Key / DB 密码走 Secret;传输 TLS | K8s Secret |
| NFR-SEC-3 | 安全性 | PII(邮箱)加密存储或脱敏 | 写入加密 |
| NFR-MAINT-1 | 可维护性 | 结构化 JSON 日志 + trace_id | slog / logback JSON |
| NFR-MAINT-2 | 可维护性 | Prometheus metrics + Grafana 仪表 | micrometer / promhttp |
| NFR-MAINT-3 | 可维护性 | OpenTelemetry 分布式追踪 | OTel SDK |
| NFR-TEST-1 | 可测试性 | TDD,核心逻辑覆盖率 ≥ 80% | go test / jacoco / pytest-cov |
| NFR-TEST-2 | 可测试性 | Locust 压测脚本纳入交付 | deploy/locust/ |
| NFR-DEPLOY-1 | 可移植性 | Docker 镜像 + K8s manifest 可一键部署 | deploy/k8s/ |
| NFR-DEPLOY-2 | 可移植性 | Docker Compose 本地全栈可起 | deploy/docker/ |
| NFR-SCAL-1 | 可扩展性 | 无状态服务 K8s HPA 横向扩容 | HPA autoscaling/v2 |
| NFR-SCAL-2 | 可扩展性 | TiDB / Redis Cluster / Pulsar 均可水平扩 | Operator/Helm |

---

## 第 6 章 数据需求
- 详见架构设计文档第 5 章 ER 与 schema.sql。
- 关键:learning_events 为高频写表,需分区/索引优化;writing_tasks.feedback 等用 JSON 列。
- AUTO_RANDOM 主键避免热点;idx_user_time 覆盖高频读路径。

## 第 7 章 验收标准(总)
1. 5 大模块端到端链路跑通(含 React 前端骨架点击)。
2. Locust 5000 CCU 压测通过,2.2 章 G 指标达标。
3. 单元测试全绿,覆盖率达标。
4. K8s manifest 可 apply;Docker Compose 本地可起。
5. 读写分离生效:读流量命中副本,压测下主库写压力可控。

## 第 8 章 需求追踪矩阵(摘要)
| 需求 | 对应模块 | 验收方式 |
|---|---|---|
| FR-1.x | 口语陪练(gateway-go) | WS 压测 + 单测 |
| FR-2.x | 自适应学习(learning-java + Disruptor) | HTTP 压测 + 单测 |
| FR-3.x | 写作批改(writing-py) | HTTP 压测 + 单测 |
| FR-4.x | 教师端(teacher-java) | HTTP 单测 + 集成 |
| FR-5.x | 情感多模态(emotion-svc) | HTTP 单测 |
| NFR-PERF-2 | 读写分离 | 架构设计 + 监控验证 |
| NFR-REL-1 | 可靠性(多副本+HPA) | K8s 副本数与 HPA 验证报告(见 `docs/test/`) |
| NFR-SCAL-1 | 水平扩展 | HPA 弹性扩容验证报告(见 `docs/test/`) |

> 注:本章验收矩阵新增"测试证据"列的完整版本见 `docs/test/07-HPA弹性扩容验证报告.md` 与 `docs/test/03-性能与压力测试报告.md`。需求 G-01~G-10 与 NFR 的逐条验证结论汇总于 `docs/test/09-验收与需求追踪矩阵.md`。
