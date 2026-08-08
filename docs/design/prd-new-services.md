# PRD:ESP 新增服务(N8 course-path-go / N10 live-tutor-go / N11 media-learn-rs / S1 dialogue 子模块)

> 状态:Proposed(2026-08-07)
> 关联 ADR:`adr-new-services.md`(10 服务矩阵,N9 并入 S1)
> 关联 schema:`adr-schema-per-service.md`(各服务独立 schema,Phase 0 落地)
> 作者:架构组(对标 Duolingo/Cambly/Cake/EF,大厂 PRD 规范)

本 PRD 覆盖 11→10 矩阵中新增/扩展的 4 项(N9 dialogue 已并入 S1,作为 S1 子模块需求)。每个条目含:用户故事 / 场景 / SLA / 容量目标 / 数据模型要点 / 依赖 / 验收。

---

## 一、S1 llm-orchestrator-rs —— dialogue 子模块(原 N9 并入)

### 1.1 用户故事
- 作为学员,我希望与 AI 进行多轮情景对话(点咖啡/机场/面试),AI 扮演固定角色,我能练口语不被评判。
- 作为学员,我希望对话流式出字(SSE/WS),延迟低,且能回看历史。
- 作为运营,我希望多轮对话复用统一 LLM 成本治理/限流/语义缓存,不另起连接池。

### 1.2 场景
- 情景对话:预设场景卡(scenario)+ 角色卡(persona),学员输入文本/语音 → ASR → LLM 多轮 → TTS 流式回放。
- 角色扮演:角色卡定义性格/口音/难度,LLM system prompt 注入。
- 自由对话:无场景约束,LLM 自适应 CEFR 难度。

### 1.3 SLA
- 首字节延迟 P95 < 1.2s(含 LLM TTFT),全句 P95 < 4s。
- 并发对话 session:单节点 200,多节点 5000(5000 CCU 假设 10% 同时对话)。
- 对话上下文窗口:保留最近 20 轮,超出走摘要压缩(复用 S1 LLM)。

### 1.4 容量目标
- 日活对话 session:50 万;平均轮次/session:8;总 LLM token/日:估算 2 亿(成本治理核心)。
- session 状态存 Redis Cluster(`esp:dialogue:session:{id}` TTL 2h),不落 TiDB 实时写。

### 1.5 数据模型要点(schema:`esp_llm`)
- `dialogue_sessions(id, user_id, scenario_id, persona_id, cefr_level, status, created_at)`
- `dialogue_turns(id, session_id, role, content, asr_confidence, tts_url, tokens_in, tokens_out, latency_ms, created_at)`
- `dialogue_personas(id, name, persona_prompt, voice_id, accent, cefr_level)`
- 复用 S1 的 `llm_call_logs`(成本治理)/`llm_semantic_cache`。

### 1.6 依赖
- S1 llm-orchestrator-rs 主干(provider 路由/限流/语义缓存)。
- S3 pronunciation-rs 的 ASR(语音输入转写)与 TTS(可选,复用 provider)。
- scenarios 表(与 gateway-go 口语场景共享)。
- Redis Cluster(session 状态 + 语义缓存 L2)。

### 1.7 验收
- 多轮对话流式出字可跑通(SSE),首字节 P95 < 1.2s。
- session 断线 2h 内可恢复(Redis TTL 内)。
- 成本计入 S1 统一账本(`llm_call_logs`)。
- 契约测试:SSE 流式帧格式 + session 状态机。

---

## 二、N8 course-path-go —— 学习路径编排(留存引擎,P0)

### 2.1 用户故事
- 作为新学员,我希望系统根据我的 CEFR 定级 + 目标(雅思6.5/职场口语)生成一条个性化学习路径(技能树)。
- 作为学员,我希望看到"解锁下一课"的进度,完成 A 课才能学 B 课(依赖关系)。
- 作为运营,我希望路径可 A/B(体系课 vs 自由路径),衡量留存差异。

### 2.2 场景
- 首次进入:调 S5 assessment-rs 定级 → 生成路径(技能树节点 + 推荐顺序)。
- 课时解锁:完成节点的前置依赖 + 达标分 → 解锁后继。
- 路径推荐:基于 user_mastery 弱项动态插拔节点(调 learning-java mastery + S2 vocab 弱词)。

### 2.3 SLA
- 路径生成 P95 < 800ms(首登,含定级结果读取)。
- 节点解锁判定 P95 < 100ms(读 Redis 缓存)。
- 路径刷新(弱项插拔)异步,端到端 < 5s。

### 2.4 容量目标
- 路径节点总数:5000(覆盖 CEFR A1~C2 + 雅思/托福/职场);
- 并发路径查询:5000 CCU 假设 5% 同时查路径 = 250 QPS。
- 路径状态写:每学员完成一课写一次,~500 QPS 峰值。

### 2.5 数据模型要点(schema:`esp_path`)
- `learning_paths(id, user_id, goal, cefr_level, version, status, created_at)`
- `path_nodes(id, path_id, skill_type, ref_id(课时/知识点id), order_idx, dependency_ids(json), unlock_condition(json), status)`
- `path_templates(id, name, goal, cefr_level, node_graph(json))` —— 体系课模板
- `path_progress(id, path_id, node_id, user_id, status, score, completed_at)`

### 2.6 依赖
- S5 assessment-rs(定级结果)。
- learning-java(user_mastery 弱项)。
- S2 vocab-srs-go(词汇弱词)。
- gamification-go(完成节点发事件 → 连胜/成就)。
- Redis(路径状态缓存 `esp:path:{user_id}` TTL 1h)。

### 2.7 验收
- 首登生成路径可跑通,节点依赖解锁逻辑正确。
- A/B 路径(模板 vs 个性化)可切换。
- 契约测试:路径生成/解锁 API。

---

## 三、N10 live-tutor-go —— 真人外教 1v1(P1,scaffold 级)

### 3.1 用户故事
- 作为学员,我希望能预约真人外教 1v1 视频课,按时进入直播间。
- 作为外教,我希望能看到课表、学员水平、历史笔记。
- 作为运营,我希望能计费(按时长/课时包)与排课(时区/可用时段)。

### 3.2 场景
- 预约:学员选外教 + 时段 → 创建订单(seata 事务:扣课时包 + 锁时段)→ 排课。
- 上课:到点双方进直播间(WebRTC 信令 via live-tutor-go)→ IM 文字 + 视频流。
- 结课:计费(扣课时包)+ 外教写课后笔记 → 事件发 gamification(成就)。

### 3.3 SLA
- 预约 P95 < 500ms;信令建连 P95 < 2s(单节点占位回声,多节点真验)。
- 排课并发:5000 CCU 假设 1% 同时预约 = 50 QPS。
- 单节点 RD:scaffold + 信令回声占位,**不在单节点做 RTC 真压**(CPU/MEM 不够)。

### 3.4 容量目标
- 外教池:1 万;日预约量:5 万;并发直播:500(多节点目标)。
- 信令服务:每路 1 WS 长连,500 并发 = 500 长连(多节点)。

### 3.5 数据模型要点(schema:`esp_tutor`)
- `tutors(id, name, avatar, bio, languages(json), rating, price_per_30min, timezone, status)`
- `tutor_availability(id, tutor_id, start_at, end_at, status)` —— 可用时段
- `bookings(id, student_id, tutor_id, start_at, duration_min, status, package_id, created_at)`
- `lesson_packages(id, student_id, remaining, total, sku, expires_at)`
- `lesson_notes(id, booking_id, tutor_id, content, created_at)`
- 信令状态存 Redis(`esp:live:room:{booking_id}` TTL 1h)。

### 3.6 依赖
- Seata(预约事务:扣课时包 + 锁时段)。
- WebRTC 信令(单节点占位 echo,多节点接 SFU/MCU)。
- Redis(房间状态 + 信令临时态)。
- gamification-go(结课成就事件)。

### 3.7 验收(单节点 scaffold 级)
- 预约 API 可跑通(seata 事务)+ 信令回声占位 WS 可连。
- manifest apply 通过,HPA min=0 待命多节点。
- 契约测试:预约/信令握手桩。

---

## 四、N11 media-learn-rs —— 真实语料学习(P1,scaffold 级)

### 4.1 用户故事
- 作为学员,我希望看影视/短视频片段学地道表达,字幕可点查词 + 跟读评分。
- 作为运营,我希望片段可打标签(场景/难度/表达点)+ 生成表达卡片。

### 4.2 场景
- 浏览:按场景/难度筛选片段 → 播放(字幕同步)。
- 学习:点字幕词 → 查词(S2 vocab)→ 跟读 → S3 pronunciation 评分。
- 卡片:片段自动抽取地道表达 → 生成卡片 → 推送 SRS(S2)。

### 4.3 SLA
- 片段列表 P95 < 200ms;字幕同步精度 < 100ms。
- 字幕对齐(离线预计算,不在线):单节点只跑预录样本验证。
- 跟读评分复用 S3 GOP,P95 < 600ms。

### 4.4 容量目标
- 片段库:1 万;字幕条目:50 万;并发播放:5000 CCU 假设 10% = 500。
- 单节点 RD:scaffold + 字幕对齐离线样本,**不在线转码**(CPU 密集压垮单节点)。

### 4.5 数据模型要点(schema:`esp_media`)
- `media_clips(id, title, source_type, url, duration_sec, cefr_level, scene_tags(json), created_at)`
- `media_subtitles(id, clip_id, start_ms, end_ms, text, word_tokens(json))`
- `media_expressions(id, clip_id, subtitle_id, expression, translation, example, card_type)`
- 字幕对齐结果存 Redis(`esp:media:align:{clip_id}` TTL 24h)。

### 4.6 依赖
- S2 vocab-srs-go(查词 + 表达卡入 SRS)。
- S3 pronunciation-rs(跟读评分)。
- S1 llm-orchestrator-rs(表达抽取 LLM,离线 batch)。
- 对象存储(片段/字幕文件,单节点用 emptyDir/PVC 占位,多节点接 S3/MinIO)。
- Redis(对齐结果缓存)。

### 4.7 验收(单节点 scaffold 级)
- 片段列表/字幕同步 API 可跑通(预录样本)。
- 表达抽取离线 LLM 可跑(经 S1)。
- manifest apply 通过,HPA min=0 待命多节点。
- 契约测试:列表/字幕/卡片桩。

---

## 五、共性工程要求(4 项均需)

1. **OTel traceId 贯通**:bff → 新服务 → 下游(S1/S2/S3/learning),Span 名规范 `<svc>.<op>`。
2. **契约测试**:每项至少 1 个 Pact 契约(消费者 bff 视角)。
3. **schema 独立**:N8→esp_path / N10→esp_tutor / N11→esp_media / S1 dialogue→esp_llm,并入 schema-per-service Phase 0。
4. **双轨落盘**:每项里程碑同步 docs/PROGRESS.md + Redis `esp:ctx:session:recovery-newservices`。
5. **性能对比数据点**:S1(Rust dialogue)/ N8(Go path)/ learning(Java)在"读 Redis 缓存 + 返回 JSON"同职责基准纳入 `perf-comparison-java-go-rust.md`。
