# SRS — 听力训练服务(listening-rs)需求规格说明书

> 版本:1.0  日期:2026-08-06
> 关联:`adr-new-services.md`、`adr-schema-per-service.md`、`schema.sql`(listening_exercises/listening_attempts)、`01-需求分析文档-SRS.md`

## 1. 背景

ESP 听力能力完全缺失。听力是语言输入核心,需独立服务 `listening-rs`(Rust)提供精听/泛听/听写/语速自适应 + 音频流式分发 + 转写。一期以素材库 + 题目练习 + 流式音频为主,转写复用 ASR provider。

## 2. 用户角色

| 角色 | 场景 |
|---|---|
| 学习者 | 选素材 → 流式听 → 做题/听写 → 拿分数;调语速 |
| 教师 | 上传/编排听力素材,查看班级听力进度 |
| 系统(learning-java) | 接收听力练习 mastery 回写 |

## 3. 功能需求(FR)

### FR-1 听力素材库
- FR-1.1 按 level(A1-C2)+ category(news/conversation/lecture/exam)列出 listening_exercises。
- FR-1.2 返回 audio_uri/transcript/questions JSON/duration_sec。
- FR-1.3 热点素材缓存(L1 进程 + L2 Redis Cluster)。

### FR-2 音频流式分发
- FR-2.1 `/api/listening/stream/{exercise_id}` chunked HTTP,首字节 < 1s。
- FR-2.2 支持语速参数(speed 0.75/1.0/1.25/1.5),服务端 resample 或客户端调。

### FR-3 听力练习与听写
- FR-3.1 `/api/listening/attempt` 记录 user_id+exercise_id+answers+mode(intensive/extensive/dictation)+time_spent,落 listening_attempts。
- FR-3.2 自动判分(客观题)+ 听写文本相似度判分(编辑距离/BLEU)。
- FR-3.3 返回 score + 正确答案 + transcript。

### FR-4 转写(一期复用 ASR)
- FR-4.1 `/api/listening/transcribe` 接收 audio_uri,调 ASR provider 返回文本(供听写对照)。

### FR-5 进度与统计
- FR-5.1 `/api/listening/progress?user_id=` 返回已练习素材数/总时长/平均分。

## 4. 非功能需求(NFR)

| 编号 | 维度 | 要求 |
|---|---|---|
| NFR-1 | 性能 | 流式首字节 < 1s;列表/attempt P95 < 100ms(缓存命中) |
| NFR-2 | 可用性 | ASR 不可达降级:transcribe 返回空+提示,不阻断练习;单节点 RD 99% |
| NFR-3 | 可观测 | OTel service.name=listening-rs;事件日志→esp_shared.app_logs |
| NFR-4 | 安全 | 不直接对外,经 bff 前置 JWT |
| NFR-5 | 数据 | 写 tidb-primary(esp_listening)/ 读 tidb-follower;Redis 缓存 |
| NFR-6 | 扩展 | axum + tokio 流式;HPA 按 CPU+并发连接扩 |
| NFR-7 | 容错 | ASR timeout 5s + 降级;音频文件缺失返回 404 |

## 5. 边界

- **不做** ASR 训练(复用 provider)。
- **不做** 音频合成 TTS。
- **不做** 鉴权/用户管理。
- 二期做实时转写流(WebSocket),一期 HTTP chunked。

## 6. 验收标准(AC)

- AC-1:端点(/healthz + /list + /stream + /attempt + /transcribe + /progress)全 200。
- AC-2:stream 首字节 < 1s,chunked 正常。
- AC-3:attempt 落 listening_attempts,progress 可聚合。
- AC-4:ASR 不可达时 transcribe 降级不 5xx。
- AC-5:trace Tempo 可见(service.name=listening-rs)。
- AC-6:单测 ≥80%。
