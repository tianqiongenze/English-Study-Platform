# English-Study-Platform — 详细设计文档 (SDD)

| 项 | 值 |
|---|---|
| 文档编号 | ESP-SDD-001 |
| 版本 | 5.0 |
| 日期 | 2026-07-31 |
| 状态 | 基线评审稿(对齐实现 + 互联网大厂标准,细化到子系统/模块/应用/智能体/类/接口 + LangGraph 写作智能体) |
| 上游 | ESP-SRS-001, ESP-SAD-001 |

本文件给出各子系统、模块、应用、智能体、类与接口级详细设计,作为编码与 TDD 的直接依据。每个模块含:职责、目录结构、关键类/接口(含签名)、核心算法、读写分离标注、TDD 测试要点、实现现状注记。

---

## 第 0 章 子系统划分与依赖

| 子系统 | 应用/进程 | 语言 | 智能体/核心域 | 关键依赖 |
|---|---|---|---|---|
| 实时口语子系统 | gateway-go | Go | 口语陪练智能体(Dialog Engine) | Redis、(Pulsar 预留) |
| 自适应学习子系统 | learning-java | Java | 学习路径智能体、难度调节器、复习调度器 | TiDB(R/W)、Disruptor、(Redis 配置预留) |
| 写作批改子系统 | writing-py | Python | 写作批改智能体(Examiner) | Claude LLM |
| 教师端子系统 | teacher-java | Java | 教案智能体、学情分析、评阅编排 | TiDB(R/W, TiFlash) |
| 情感多模态子系统 | emotion-svc | Python | 情绪感知、视觉辅助 | 无外部依赖(本期纯计算) |
| 前端子系统 | frontend-react | TS | 学习者/教师交互面板 | 各服务 REST/WS |

子系统调用关系:`gateway-go ──Pulsar──► learning-java(落库)`,`teacher-java ──► writing-py(预留,本期启发式)`,`learning-java ──Disruptor──► TiDB`。

---

## 第 1 章 gateway-go(口语陪练实时面)

### 1.1 目录结构
```
services/gateway-go/
  src/
    llm/         # Provider 抽象 + Anthropic + Mock
    asr/         # AsrProvider(Mock)
    tts/         # TtsProvider(Mock)
    session/     # 会话管理(Redis 持久化)
    ws/          # WebSocket handler + 帧编解码
    scenario/    # 场景仓库
    dialog/      # 对话引擎(纠错+hint+编排)
    server.go(main 内联) / main.go
```

### 1.2 关键类型与接口
```go
// ws.Frame —— 帧结构
type Frame struct {
    Type    string  `json:"type"`    // audio_chunk|text_input|ai_reply|correction|hint|end|error
    Seq     int     `json:"seq,omitempty"`
    Data    string  `json:"data,omitempty"`     // base64 音频/文本
    Text    string  `json:"text,omitempty"`
    Original string `json:"original,omitempty"` // correction
    Corrected string `json:"corrected,omitempty"`
    Options []string `json:"options,omitempty"` // hint
    Summary string  `json:"summary,omitempty"` // end
    Message string  `json:"message,omitempty"` // error
}
func DecodeFrame(raw []byte) (Frame, error)
func (f Frame) Encode() ([]byte, error)

// llm.Provider —— LLM 抽象
type Provider interface { Chat(ctx, system, user string) (string, error) }
type MockProvider struct{}            // 确定性关键词回复
type AnthropicProvider struct{...}    // httpx → /v1/messages
func New() Provider                   // LLM_MODE=mock → Mock

// session.Manager —— Redis 会话存储
type Manager struct{ rdb *redis.Client; ttl time.Duration }
func NewManager(rdb, ttl) *Manager
func (m) Create(ctx, *Session) error
func (m) Get(ctx, id) (*Session, error)
func (m) AppendTurn(ctx, id, Turn) error   // 历史裁剪 50 轮
func (m) End(ctx, id) error
type Session struct{ ID, UserID, Scenario, System string; History []Turn; Difficulty int}

// dialog.Engine —— 口语陪练智能体
type Engine struct{ LLM llm.Provider; HintIdle time.Duration}
func (e) Process(ctx, *Session, text) (Result, error)  // 纠错+LLM回复+turn
func (e) Hint(*Session) Frame                           // 场景化选项

// asr.Provider / tts.Provider —— Mock
type Provider interface{ Recognize(chunks []string) string }
type Provider interface{ Synthesize(text string) string }

// scenario.Repository —— 场景仓库(内存 map,airport/interview/cafe)
func NewRepository() *Repository
func (r) List() []Scenario; func (r) Get(code) (Scenario,bool); func (r) Add(Scenario)
```

### 1.3 口语会话流程(智能体编排)
1. WS 连接 → `handleSpeech` → 取 session_id → `sessions.Get`。
2. 收 `audio_chunk` → `asr.Recognize` → 文本;收 `text_input` → 文本(或 `/hint` 直发 hint)。
3. `sessions.AppendTurn(user)` → `engine.Process`:正则纠错(`I wants`→`I want`)→ 拼 system+history+user → `llm.Chat` → ai_reply 帧。
4. ai_reply 帧附 `tts.Synthesize` 音频 → 推送。
5. `end` → 推送 end 帧 + `sessions.End`。
6. (预留)会话结束发 Pulsar `speech.session` → learning-worker 落 session_turns。

### 1.4 读写分离
- 写:session_turns 经 Pulsar → learning-worker → TiDB Primary(本期会话存 Redis,落库预留)。
- 读:会话历史 `GET /api/sessions/{id}` → TiDB Follower(本期从 Redis 读)。

### 1.5 健康与可观测
- `/healthz`(200)、`/metrics`(promhttp:WS 连接数、帧处理延迟)。
- 日志:slog JSON。

### 1.6 TDD 要点(已实现)
- `ws.Frame` 编解码往返、缺字段、非法 JSON(`ws/frame_test.go`,100% 覆盖)。
- `session.Manager` 增删改查 + 裁剪 50 轮 + end 移除(miniredis,80.6% 覆盖)。
- `dialog.Engine` 回复+turn、主谓纠错、正确句不纠、场景化 hint(`dialog/engine_test.go`,90.3%)。
- `scenario.Repository` list/get/add(`scenario/repository_test.go`,100%)。
- `llm.New()` mock 选择(`llm/provider_test.go`,21.7% — Mock 路径已测,Anthropic 需网络故低)。

### 1.7 现状注记
- 鉴权占位:`userIDFromContext` 返回固定 1(JWT 库仅用于生成 session id)。生产须在 BFF/Ingress 强制校验 JWT。
- `HintIdle` 字段已设但未接超时检测,hint 仅由 `/hint` 显式触发。
- ASR/TTS 仅 Mock,接口与真实厂商对齐。

---

## 第 2 章 learning-java(自适应学习业务面 + Disruptor)

### 2.1 目录结构
```
services/learning-java/src/main/java/com/esp/learning/
  config/      DataSourceConfig(读写分离双池 + LazyConnectionDataSourceProxy)、ServiceConfig
  rw/          DataSource(@)、DataSourceType(READ/WRITE)、RoutingDataSource、RoutingHolder、DataSourceAspect
  domain/      LearningEventEntity、UserMastery、UserMasteryId
  repository/  LearningEventRepository、UserMasteryRepository(自定义查询 + TiFlash hint)
  service/     LearningService、DifficultyAdjuster、EbbinghausScheduler
  disruptor/   LearningEvent、LearningEventFactory、LearningEventDisruptor、BatchEventHandler、
               LearningEventConsumer(@)、DbLearningEventConsumer
  web/         LearningController
```

### 2.2 读写分离实现(第 4 章落地)
```java
// rw 包
enum DataSourceType { READ, WRITE }
@interface DataSource { DataSourceType value() default WRITE; boolean consistent() default false; boolean tiflash() default false; }
class RoutingHolder { static ThreadLocal<DataSourceType> TL; set/get/clear; }   // 默认 WRITE
class RoutingDataSource extends AbstractRoutingDataSource { determineCurrentLookupKey() → TL.get(); }
@aspect class DataSourceAspect { @Around("@annotation(DataSource)") → 设 TL,consistent/tiflash 处理,finally clear; }
// config
@Bean primaryDataSource(Hikari, WRITE_DB_URL), readDataSource(Hikari, READ_DB_URL)
@Bean routingDataSource(primary, read) → LazyConnectionDataSourceProxy 包裹
```
- `primaryDataSource` → TiDB Primary;`readDataSource` → TiDB Follower。
- `@DataSource(value=READ, consistent=true)` → 强制 WRITE 池(写后强一致读)。
- TiFlash:Repository 自定义查询带 `/*+ read_from_storage(tiflash[user_mastery]) */` hint。

### 2.3 Disruptor 设计(学情事件流)
```java
class LearningEvent { long userId; String eventType,payload; long createdAt; clear()/set... }
class LearningEventFactory implements EventFactory<LearningEvent>
@Component class LearningEventDisruptor {
  Disruptor<LearningEvent>(factory, 65536, DaemonThread, MULTI, BlockingWaitStrategy);
  boolean publish(userId, type, payload)   // tryNext() 满即丢 + dropped++
  long publishedCount()/droppedCount()
  @PostConstruct start(); @PreDestroy shutdown(){ handler.drain(); disruptor.shutdown(); }
}
class BatchEventHandler implements EventHandler<LearningEvent> {
  BatchEventHandler(consumer, batchSize=200, flushMs=500);
  onEvent(...) 累积; 达 batch 或定时 flush → consumer.consume(buf,0,n);  drain();
}
interface LearningEventConsumer { void consume(LearningEvent[], int off, int len); }
@Component class DbLearningEventConsumer implements LearningEventConsumer {
  consume(...) → 映射 LearningEventEntity[] → repo.saveAll(batch);  // 写主库
  consumedCount();
}
```
- RingBuffer size=65536(2 的幂);`tryNext()` 非阻塞,满即丢并记 `dropped`。
- 批量:每 200 条或 500ms flush 一次。

### 2.4 核心算法(智能体域)
```java
@Component class EbbinghausScheduler {
  static final List<Long> INTERVALS_DAYS = [1,2,4,7,15,30];
  Instant nextReviewAt(int reviewCount, boolean correct, Instant from);
  long currentIntervalDays(int reviewCount);
}  // 答对推进下一间隔;答错重置 idx=0(1d);封顶 30d

class DifficultyAdjuster {
  static final MIN=1, MAX=5, UP=0.8, DOWN=0.4;
  DifficultyAdjuster(int windowSize);
  int recordAndAdjust(int current, boolean correct);  // 滑动窗口满才评估;>0.8↑,<0.4↓,否则保持;清窗
  boolean needsScaffolding(int cur, int next);
  static double speedMultiplier(int difficulty);       // 0.8 + (d-1)*0.15 → 0.8~1.4
}
```

### 2.5 REST 接口(LearningController)
| 方法 | 路径 | 标注 | 说明 |
|---|---|---|---|
| POST | `/api/learning/event` | — | 入 Disruptor,202/503 |
| POST | `/api/learning/placement` | WRITE | 批量写 mastery 基线 |
| GET | `/api/learning/review` | READ | 今日复习清单 |
| GET | `/api/learning/mastery/weak` | READ | 薄弱知识点(按 score 升序) |
| POST | `/api/learning/difficulty` | — | 动态难度档位 + 语速 + 降维标记 |

### 2.6 TDD 要点(已实现)
- `EbbinghausScheduler`:各间隔推进、答错重置、封顶 30d(`EbbinghausSchedulerTest`)。
- `DifficultyAdjuster`:窗口未满保持、达 5 不再加、达 1 不再减、中率保持、降维判断、语速单调(`DifficultyAdjusterTest`)。
- `LearningEventDisruptor`:500 条批量消费、8 线程 ×5000=40000 事件吞吐 ≥5000 ev/s 且全消费(`LearningEventDisruptorTest`)。
- `RoutingHolder`:默认 WRITE、set/clear(`RoutingHolderTest`)。
- `LearningController`(@WebMvcTest):event 202/503、difficulty 响应、placement 默认。
- `LearningService`:reportEvent 委托、initMastery、review 正/误/缺、difficulty/speed 委托、dueReview/weakest 委托。

### 2.7 现状注记
- Pulsar 未接(预留);学情事件走 Disruptor 直落库。
- Redis 配置/依赖存在但无代码使用(预留缓存)。
- `tiflash` 注解属性为声明式,实际路由靠 SQL hint。
- `DbLearningEventConsumer.saveAll` 路径未对真实 DB 集成测试(吞吐测试用内存 consumer)。

---

## 第 3 章 writing-py(写作批改 AI 面 + LangGraph 写作智能体)

### 3.1 目录结构
```
services/writing-py/app/
  main.py        # Litestar app: /healthz /api/writing/{grade,polish,translate,review,notes}
  llm.py         # LlmProvider(Protocol, async) + Mock + AnthropicAsync + extract_json + new_provider
  schemas.py     # Pydantic v2 模型(Grade/Polish/Translate/Review/Notes)
  service.py     # WritingService(async grade/polish/translate, 兼容旧单步契约)
  agent.py       # LangGraph 写作智能体:批改→润色→人工干预→学习笔记 StateGraph
  graph_state.py # WritingState(TypedDict) + 节点定义
  asgi.py        # 导出 app 供 granian 加载
```

### 3.2 关键类与接口
```python
class LlmProvider(Protocol):
    async def chat(self, system: str, user: str) -> str: ...

class WritingService:                       # 单步兼容入口(供压测/学习者自助)
    async def grade(req) -> GradeResponse
    async def polish(req) -> PolishResponse
    async def translate(req) -> TranslateResponse

# === LangGraph 写作智能体(ADR-18,批改→润色→人工干预→学习笔记) ===
class WritingState(TypedDict):
    prompt: str; essay: str; exam: str
    grade_result: dict | None
    polished: dict | None
    teacher_feedback: str          # 教师介入内容,空则自动模式
    notes: dict | None
    status: str                    # running|awaiting_review|done

def build_writing_graph(llm: LlmProvider, checkpointer) -> CompiledGraph:
    g = StateGraph(WritingState)
    g.add_node("grade", grade_node)     # LLM 评分官 → score/feedback/grammar/coherence
    g.add_node("polish", polish_node)   # LLM editor → 基于 grade_result 润色
    g.add_node("notes", notes_node)     # LLM 综合 grade+polish+teacher_feedback → 学习笔记
    g.set_entry_point("grade")
    g.add_edge("grade", "polish")
    g.add_edge("polish", "notes")       # 默认自动模式:polish 直达 notes
    g.add_edge("notes", END)
    # 人工干预模式:compile(interrupt_before=["notes"]) → polish 后挂起
    return g.compile(checkpointer=checkpointer, interrupt_before=interrupt_before)

# 端点:
# POST /api/writing/run        → 启动链,返回 thread_id(自动模式直达 notes)
# POST /api/writing/run?human=true → 启动链并在 polish 后挂起(interrupt_before notes)
# POST /api/writing/review      → 教师提交 teacher_feedback,Command(resume) 恢复 → notes
# GET  /api/writing/state/{thread_id} → 查询当前状态/挂起点
```

### 3.3 批改流程(LangGraph 多步链 + 人工干预)
1. `POST /api/writing/run` `{prompt, essay, exam, human?}` → 初始化 `WritingState`,启动 graph。
2. **grade 节点**:LLM 评分官角色 → `grade_result`(score/feedback/grammar/coherence)。
3. **polish 节点**:LLM editor 角色 → 基于 `grade_result` 给 `polished`(polished + suggestions)。
4. **人工干预(human=true)**:`interrupt_before=["notes"]` → graph 挂起,checkpoint 持久化,返回 `thread_id` + `status=awaiting_review`。教师经 `POST /api/writing/review` `{thread_id, teacher_feedback}` 提交反馈 → `Command(resume)` 恢复。
5. **notes 节点**:LLM 综合 `grade_result` + `polished` + `teacher_feedback` → `notes`(生词表/语法点/个性化改进清单)。
6. **降级**:无人工介入(`human=false` 或默认)`teacher_feedback=""`,notes 直接基于 grade+polish 生成(自动模式,供压测)。
7. (预留)落库 writing_tasks(Primary):thread_id、state、最终结果。

### 3.4 TDD 要点(已实现单步 + 新增 LangGraph)
- mock 批改结构校验(score 0~30、grammar 命中 "I wants"、coherence 0~10)。
- 润色 suggestions 非空、翻译 vocab ≥3。
- `extract_json` 处理 code-fence / 前导散文。
- API 层:`test_api.py`(healthz、grade 422、polish、translate)。
- **新增 LangGraph 测试**:`test_agent.py`:自动模式 run 直达 notes;human 模式 polish 后挂起 `status=awaiting_review`;review 恢复后 notes 含 teacher_feedback;断点续跑 checkpoint 一致。

### 3.5 现状注记
- 单步 grade/polish/translate 保留(Litestar async),兼容旧契约与压测。
- 新增 LangGraph 多步链(批改→润色→人工干预→学习笔记),生产用 Redis checkpointer,测试用 MemorySaver。
- LLM 异步调用(`httpx.AsyncClient`);Anthropic 路径需网络,单测覆盖 Mock。

---

## 第 4 章 teacher-java(教师端业务面)

### 4.1 目录结构
```
services/teacher-java/src/main/java/com/esp/teacher/
  config/DataSourceConfig  rw/{DataSource,DataSourceType,RoutingDataSource,RoutingHolder,DataSourceAspect}
  domain/Submission  repository/SubmissionRepository(classWeakness via TiFlash hint)
  service/{LessonPlanGenerator, TeacherService}  web/TeacherController
```

### 4.2 关键类与接口
```java
record LessonPlan { String passage; List<String> exercises, games, discussion; }
class LessonPlanGenerator { LessonPlan generate(String topic, String level); }  // 模板化,可替换 LLM

@Service class TeacherService {
    LessonPlan generateLessonPlan(topic, level);
    @DataSource(value=READ, tiflash=true) List<Object[]> classAnalytics(Long classId);  // 走 TiFlash
    @DataSource(WRITE) int autoGrade(Long assignmentId);   // 逐份 scoreContent+feedbackFor, status→auto_graded
    @DataSource(WRITE) void finalize(Long submissionId, BigDecimal finalScore);  // final_score, status→finalized
    static BigDecimal scoreContent(String content);  // 长度40 + 句点30 + 连词30 = 0~100
    static String feedbackFor(String content);
}
interface SubmissionRepository extends JpaRepository<Submission,Long> {
    List<Submission> findByAssignmentId(Long);
    @Query(value="... /*+ read_from_storage(tiflash[submissions]) */ ...", nativeQuery=true)
    List<Object[]> classWeakness(Long classId);
}
```

### 4.3 REST 接口(TeacherController)
| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/teacher/lesson-plan` | 教案 JSON |
| GET | `/api/teacher/classes/{id}/analytics` | 班级共性难题(TiFlash) |
| POST | `/api/teacher/assignments/{id}/auto-grade` | 批量评阅 |
| POST | `/api/teacher/submissions/{id}/finalize` | 教师终审 |

### 4.4 读写分离
- 教案生成:内存计算(无 DB 写,可扩展落 lesson_plans Primary)。
- 学情分析:`@DataSource(READ, tiflash=true)` + SQL hint → TiFlash 列存。
- 作业列表/评阅:`findByAssignmentId`(READ)、`save`(WRITE)。

### 4.5 TDD 要点(已实现)
- `LessonPlanGenerator`:完整性(习题≥3、讨论≥2)、默认主题。
- `TeacherService` 评分:`scoreContent` 空/连词、`feedbackFor` 短/缺连词。
- `TeacherController`(@WebMvcTest):lesson-plan、analytics、auto-grade、finalize。

### 4.6 现状注记
- 无 LLM(教案模板化,作业评分启发式),可替换为调 writing-py。
- 无 Disruptor/Redis。

---

## 第 5 章 emotion-svc(情感化多模态)

### 5.1 关键
```python
NEGATIVE = {sad, tired, frustrated, angry, anxious, ...}
POSITIVE = {happy, excited, great, good, love, confident, ...}
def sense_emotion(text) -> SenseResponse  # 命中负面→安慰;正面→鼓励;中性→通用鼓励
def visual_for_word(word, context="") -> VisualResponse  # placehold.co/600x400?text=<word>
```

### 5.2 TDD 要点(已实现)
- 情绪关键词命中(负面/正面/中性)。
- visual 返回 https URL、空词兜底。
- API 层:sense 各分支、visual 422、healthz。

### 5.3 现状注记
- 纯启发式/占位(无真实情绪模型/图像生成),接口与真实模型对齐。

---

## 第 5.5 章 emotion-svc(情感化多模态)— 同 writing-py,改用 Litestar + Granian

emotion-svc 与 writing-py 同属 Python AI 面,框架迁移一致(Litestar + Granian),差异仅业务逻辑为关键词情绪分类 + 占位图。详见第 5 章与第 9 章(架构决策落地)。其 `sense_emotion` / `visual_for_word` 函数签名不变,仅 `app/main.py` 由 `FastAPI()` 改为 `Litestar()`。

---

## 第 9 章 架构决策落地与重构详细设计(ADR-08 / ADR-09)

> 本章对应 SAD ADR-08(BFF + Spring Cloud Alibaba)、ADR-09(Python AI 面 Litestar + Granian),给出落地的子系统/模块/类/接口级设计。

### 9.1 新增子系统:ESP-BFF(Spring Cloud Gateway + Spring Cloud Alibaba)
**职责**:边缘 API 网关。统一接入 HTTPS/WSS,JWT(RS256)校验,按前缀路由到各后端服务,边缘限流(Sentinel),跨域,请求头透传(`X-User-Id`/`X-Role`)。Java 服务间 RPC 通过 Nacos 注册发现 + Dubbo Triple。

**目录结构**:
```
services/bff/                          # 新增 Java/Spring Boot 服务
  pom.xml                              # spring-cloud-starter-gateway + spring-cloud-starter-alibaba-{nacos-discovery,nacos-config,sentinel,dubbo}
  src/main/java/com/esp/bff/
    BffApplication.java
    config/
      GatewayConfig.java               # RouteLocator: /api/sessions,/ws→gateway-go; /api/learning→learning-java; /api/writing→writing-py; /api/teacher→teacher-java; /api/emotion→emotion-svc
      SecurityConfig.java              # JWT RS256 校验 GlobalFilter,白名单 /auth/*,/healthz
    filter/
      JwtAuthGlobalFilter.java         # 实现 GlobalFilter:解析 Bearer JWT → 注入 X-User-Id/X-Role;失败 401
      RequestIdGlobalFilter.java       # 注入 X-Request-Id(	trace_id)
    sentinel/
      GatewayFlowRules.java            # 按路由 QPS 限流(学习者写批改 50r/s,学情事件不限流)
  src/main/resources/
    application.yml                    # spring.cloud.gateway.routes + nacos + sentinel + jwt.public-key
    bootstrap.yml                      # nacos config
```

**关键接口/类签名**:
```java
@Component
public class JwtAuthGlobalFilter implements GlobalFilter, Ordered {
    Mono<Void> filter(ServerWebExchange exchange, GatewayFilterChain chain);  // 解析 JWT → headers.add(X-User-Id), 不通过 401
    int getOrder();  // -100
}
// RouteLocator: /api/learning → lb://learning-java (Dubbo 服务仍走 HTTP 网关聚合,内部 RPC 用 Triple)
```
**与 Dubbo 的关系**:BFF 仅做边缘 HTTP 聚合与鉴权;**Java 服务间调用**(如 teacher-java 调 learning-java 的学情查询)用 Dubbo Triple RPC(经 Nacos 发现),不经 BFF。BFF 不被替换,边缘职责不变(ADR-08)。

### 9.2 Java 服务间 Dubbo Triple(learning-java ↔ teacher-java)
**目标**:teacher-java 学情分析需要按 user 维度查学情,原走 REST;改用 Dubbo Triple 强类型 RPC,提升吞吐与契约稳定性。
**设计**:
- 新增 `services/learning-api`(共享契约模块):`@DubboService` 接口 `LearningRpcService`(`List<KpStat> userStats(Long userId)`),参数/返回为可序列化 DTO。
- learning-java 实现 `@DubboService` 注解的 `LearningRpcServiceImpl`,委托 `LearningService`。
- teacher-java `@DubboReference(check=false)` 注入 `LearningRpcService`,在 `TeacherService.classAnalytics` 中调用。
- 注册中心:Nacos(`spring.cloud.nacos.discovery`)。
- 协议:Triple(HTTP/2,兼容 gRPC),端口 50051。
- **约束**:Dubbo 仅用于内部 Java 服务间;对外仍由 BFF HTTP 暴露。gateway-go(Python/Go)不参与 Dubbo。

### 9.3 Python AI 面:FastAPI → Litestar + Granian(writing-py / emotion-svc)
**迁移映射表**:
| FastAPI | Litestar | 说明 |
|---|---|---|
| `FastAPI(title=...)` | `Litestar(route_handlers=[...], openapi_config=OpenAPIConfig(title=...))` | 应用入口 |
| `@app.get/post(path, response_model=X)` | `@get(path)` / `@post(path)` 装饰器 + `typing` 返回注解 | 路由声明 |
| `def handler(req: ReqModel)` | `async def handler(data: ReqModel) -> RespModel` | 全异步 + 注入 |
| `uvicorn app.main:app` | `granian app.main:app --interface asgi` | ASGI 服务器 |
| Pydantic v2 模型 | Pydantic v2 模型(不变) | schemas 零改动 |
| 同步 `httpx.post` | `httpx.AsyncClient` + `await` | LLM 调用异步化 |

**writing-py 新结构**:
```
services/writing-py/app/
  main.py        # Litestar app + Granian 入口(async handlers)
  llm.py         # LlmProvider(Protocol, async chat) + Mock + AnthropicAsync(httpx.AsyncClient)
  service.py     # WritingService(async grade/polish/translate, await llm.chat)
  schemas.py     # Pydantic v2(不变)
  asgi.py        # 导出 app 供 granian 加载
```
**关键接口**:
```python
class LlmProvider(Protocol):
    async def chat(self, system: str, user: str) -> str: ...

class AnthropicLlmProvider:
    def __init__(self, ...): self._client = httpx.AsyncClient(timeout=30.0)
    async def chat(self, system, user) -> str: resp = await self._client.post(...); ...

@post("/api/writing/grade")
async def grade(data: GradeRequest, service: WritingService) -> GradeResponse:
    return await service.grade(data)
```
**Granian 启动**:`granian app.asgi:app --interface asgi --host 0.0.0.0 --port 8082 --workers 2`(多 worker 提并发;Rust 内核)。Dockerfile CMD 改为 `granian ...`。

**emotion-svc**:同构,`sense`/`visual` 改 `async def`,返回模型不变。

### 9.4 共享读写分离模块抽取(消除 learning/teacher rw 包重复)
现状:learning-java 与 teacher-java 的 `rw` 包逐字复制(仅包名不同,ADR 调研确认)。**重构**:抽取 `services/common-rw`(共享 jar)含 `DataSourceType`/`DataSource`/`RoutingDataSource`/`RoutingHolder`/`DataSourceAspect`/`DataSourceConfig`,两个 Java 服务 `依赖 com.esp:common-rw`。减少重复,统一读写分离语义。teacher 的 `tiflash` 注解字段在 Aspect 中真正消费(修正现状"注解形同摆设"):READ+tiflash 时路由仍走 read 池(由 SQL hint 决定走 TiFlash),并在 ThreadLocal 标记供日志观测。

### 9.5 重构清单与影响面
| 变更 | 影响服务 | 契约影响 | 测试影响 |
|---|---|---|---|
| 新增 BFF | 新服务 | 新增边缘入口,后端 API 路径不变 | 新增 BFF 路由/JWT 单测 |
| Dubbo Triple RPC | learning-java, teacher-java, 新 learning-api | 内部,对外 API 不变 | 新增 RPC 集成测试 |
| Litestar + Granian | writing-py, emotion-svc | OpenAPI 路径/模型不变,前后端零改动 | 测试改 Litestar TestClient |
| common-rw 抽取 | learning-java, teacher-java | 无 | rw 测试迁入 common-rw |
| TiFlash 注解修正 | common-rw | 无 | 新增 tiflash 路由断言 |
| LangGraph 写作智能体 | writing-py | 新增 /api/writing/{run,review,state};旧单步端点保留 | 新增 test_agent.py |

### 9.6 LangGraph 写作智能体落地(ADR-18,批改→润色→人工干预→学习笔记)
**目标**:将写作批改智能体从单步 LLM 调用升级为 LangGraph 多步有状态链,支持教师人工干预与断点续跑。

**模块/类/接口级设计**:
```
services/writing-py/app/
  graph_state.py
    class WritingState(TypedDict): prompt, essay, exam, grade_result, polished, teacher_feedback, notes, status
  agent.py
    grade_node(state, llm) -> {grade_result, status}        # LLM 评分官
    polish_node(state, llm) -> {polished}                    # LLM editor,基于 grade_result
    notes_node(state, llm) -> {notes, status:"done"}         # 综合 grade+polish+teacher_feedback
    build_writing_graph(llm, checkpointer, human: bool) -> CompiledGraph
    WritingAgent: 封装 graph + checkpointer,提供 run/resume/get_state
  main.py 新增端点:
    POST /api/writing/run          -> {thread_id, status, ...result}
    POST /api/writing/review       -> {thread_id, teacher_feedback} -> 恢复 -> notes
    GET  /api/writing/state/{tid}   -> 当前状态
```

**人工干预机制**:
- `human=true` 时 `compile(interrupt_before=["notes"])`,polish 完成后 graph 挂起,checkpoint 持久化(MemorySaver 测试 / Redis 生产)。
- 教师经 `POST /api/writing/review` 提交 `teacher_feedback`(可改写润色、补充评语),`graph.invoke(None, config, command=Command(resume={"teacher_feedback": ...}))` 恢复 → notes 节点消费反馈。
- `human=false`(默认/压测):无 interrupt,polish 直达 notes,`teacher_feedback=""`。

**checkpointer 选型**:
- 测试:`MemorySaver`(内存,进程内)。
- 生产:`langgraph.checkpoint.redis.RedisSaver`(Redis Cluster,跨 Pod 共享,支持 Pod 重启后续跑)。

**降级与可观测**:
- LLM mock 模式下各节点返回确定性 JSON,链路可离线压测。
- 每个节点 emit OTel span(`writing.grade` / `writing.polish` / `writing.notes`),与前端 trace_id 串联。
- 旧单步端点 `/api/writing/{grade,polish,translate}` 保留,兼容前端旧调用与压测脚本。

**与 teacher-java 的协同**:
- 教师人工干预入口可由 teacher-java BFF 路由代理到 writing-py `/api/writing/review`(教师端"作业评阅"页调用),形成"AI 批改 → 教师介入 → 生成学习笔记"闭环(对齐 PRD F14 + F18)。

---

## 第 6 章 跨切面:读写分离注解规范
```java
@DataSource(DataSourceType.READ)        // 走 Follower
public MasteryView getMastery(Long userId){...}

@DataSource(value=DataSourceType.READ, tiflash=true)  // 走 TiFlash 列存
public List<KpStat> classAnalytics(Long classId){...}

@DataSource(DataSourceType.WRITE)       // 走 Primary
public void saveEvent(LearningEvent e){...}

@DataSource(value=DataSourceType.READ, consistent=true)  // 写后强一致读 → 实际走 Primary
public UserMastery freshRead(Long userId, Long kpId){...}
```

---

## 第 7 章 前端子系统(三端:Web / H5 / 小程序)+ 数字人 + 智能体

### 7.1 三端技术选型与目录
| 端 | 技术栈 | 目录 | 选型依据 |
|---|---|---|---|
| Web | React 18 + Vite + TS + react-router + SWR + zustand | `frontend-react/` | 桌面全功能,学习者+教师 |
| H5 | React 18 + Vite + TS + antd-mobile + react-router | `frontend-h5/` | 移动竖屏,学习者全功能+教师只读 |
| 小程序 | Taro 4 + React + TS | `frontend-miniprogram/` | 一套 React 语法编译多端;WS 用 Taro.connectSocket |

三端共享 API 契约类型(`frontend-shared/types`,或各端内联),均通过 BFF(8090)鉴权后路由,WS 旁路直连 gateway-go(ADR-13)。

### 7.2 数字人(Digital Human)子系统
- **前端**:`Avatar` 组件(Web canvas/SVG 唇形同步;H5/小程序对应实现),消费 TTS 音频帧驱动口型,消费 emotion-svc 情绪切换表情。
- **后端**:gateway-go 在 `ai_reply` 帧附带 TTS 音频(base64)与情绪标签;emotion-svc 提供情绪识别接口。
- **降级**:数字人不可用时回退静态头像 + 文本气泡。
- **接口**:`Frame{ type:"ai_reply", data:"<base64-audio>", emotion:"happy|neutral|encouraging", text:"..." }`。

### 7.3 智能体(Agent)子系统
四类智能体,均经各自服务编排,LLM 调用统一走 `LlmProvider` 抽象(直连 Anthropic API;复用本机代理端点;压测 mock):
| 智能体 | 所在服务 | 职责 | LLM |
|---|---|---|---|
| 口语陪练智能体 | gateway-go `dialog.Engine` | 纠错+回复+场景化引导 | Claude |
| 写作批改智能体 | writing-py `WritingService` | 评分+润色+翻译精读+实时建议 | Claude |
| 学习路径智能体 | learning-java `LearningService` | 薄弱点定位+复习排序+动态难度 | Claude(规划)/启发式(高频) |
| 教案智能体 | teacher-java `LessonPlanGenerator` | 一键结构化教案 | Claude |

LLM 端点配置:`ANTHROPIC_BASE_URL`、`ANTHROPIC_API_KEY`、模型 `claude-opus-4-8[1M]`(`glm_for_coding`);`LLM_MODE=mock` 切确定性 stub 供压测。

### 7.4 前端路由(Web)
```
/auth/login  /auth/register
/learning/speech   /learning/writing   /learning/knowledge  /learning/review  /learning/difficulty
/teacher/lesson-plan  /teacher/analytics  /teacher/grading
/profile
```
H5 路由同构(竖屏布局);小程序页面用 Taro pages。

设计与原型见 `docs/product/02-原型设计.md`(v2.0 三端)。

---

## 第 8 章 TDD 总策略
- 红-绿-重构:每个核心类先写失败测试 → 实现 → 重构。
- Go:`go test -cover ./...`(dialog 90.3%、scenario 100%、session 80.6%、ws 100%)。
- Java:`mvn test`(JUnit5 + Mockito + H2);jacoco 覆盖率(核心逻辑 ≥80%)。
- Python:`pytest --cov`(writing/emotion 全绿)。
- 覆盖率达标详见 `docs/test/01-单元测试报告.md`。
