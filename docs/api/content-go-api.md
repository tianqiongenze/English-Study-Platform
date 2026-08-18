# content-go API 契约

> 版本:1.0  日期:2026-08-17
> 服务:content-go  端口:8096  基路径:`/api/content`
> 鉴权:所有端点需 `Authorization: Bearer <JWT>`,BFF 注入可信 `X-User-Id` / `X-Role` 头

---

## 1. 健康检查

| 方法 | 路径 | 认证 | 响应 |
|---|---|---|---|
| GET | `/healthz` | 无 | 200 `{status:"UP"}` |
| GET | `/livez` | 无 | 200 |
| GET | `/readyz` | 无 | 200 `{status:"UP",db_read:true}` / 503 `{status:"DOWN",db_read:"error"}` |

---

## 2. 学习端(任意登录用户)

### GET /api/content/courses

课程列表,按考试标签过滤。

**参数**:

| 参数 | 类型 | 必填 | 默认 | 说明 |
|---|---|---|---|---|
| exam_tag | string | 否 | (空=全标签) | CET4/CET6/TOEFL/IELTS/GENERAL |
| status | string | 否 | published | published/all(管理端用 all) |

**响应 200**:
```json
[
  {
    "id": 1,
    "code": "cet4",
    "title": "CET-4 核心词汇",
    "exam_tag": "CET4",
    "cefr_range": "B1-B2",
    "description": "大学英语四级核心词汇精选",
    "status": "published"
  }
]
```

### GET /api/content/courses/:id

课程详情(仅 published)。

**响应 200**:
```json
{
  "id": 1,
  "code": "cet4",
  "title": "CET-4 核心词汇",
  "exam_tag": "CET4",
  "cefr_range": "B1-B2",
  "description": "大学英语四级核心词汇精选"
}
```

**响应 404**: `{error:"not found"}`

### GET /api/content/courses/:id/lessons

课时列表,按 seq 排序(仅 published)。

**响应 200**:
```json
[
  {
    "id": 10,
    "course_id": 1,
    "seq": 1,
    "title": "Unit 1: Daily Vocabulary",
    "skill": "vocab",
    "cefr": "B1"
  }
]
```

### GET /api/content/lessons/:id/items

题目列表(不含 answer,仅 published+is_current)。

**响应 200**:
```json
[
  {
    "id": 100,
    "lesson_id": 10,
    "item_type": "reading",
    "prompt": {
      "passage": "The quick brown fox...",
      "options": ["A. jumped", "B. jumps", "C. jumping", "D. jump"]
    },
    "difficulty": 3,
    "cefr": "B1",
    "tags": ["topic-animals", "type-grammar"]
  }
]
```

### POST /api/content/items/:id/answer

提交作答,自动判分,写 content_attempts,发 learning_event_outbox 事件。

**请求体**:
```json
{
  "answers": {"option_index": 2},
  "duration_ms": 15000
}
```

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| answers | object | 是 | 单选:{option_index:int};vocab:{} |
| duration_ms | int | 是 | 作答耗时(毫秒) |

**判分规则**:

| item_type | 逻辑 | 正确 score | 错误 score |
|---|---|---|---|
| vocab | 始终 correct=true | 1 | — |
| reading/listening/grammar/cloze | option_index == correct_option | 100 | 0 |

**响应 200**:
```json
{
  "correct": true,
  "score": 100,
  "explanation": "The correct answer is C because..."
}
```

**响应 401**: `{error:"unauthorized"}`(X-User-Id 缺失或 <=0)
**响应 404**: `{error:"item not found"}`
**响应 500**: `{error:"..."}`

---

## 3. 管理端(role=admin)

### POST /api/content/courses

创建课程。

**请求体**:
```json
{
  "code": "cet4",
  "title": "CET-4 核心词汇",
  "exam_tag": "CET4",
  "cefr_range": "B1-B2",
  "description": "大学英语四级核心词汇精选"
}
```

**响应 201**: `{id: 1}`

### PUT /api/content/courses/:id

更新课程(含状态流转)。

**请求体**:
```json
{
  "title": "CET-4 核心词汇(修订版)",
  "status": "published",
  "cefr_range": "B1-B2"
}
```

**响应 200**: `{id: 1}`

### POST /api/content/lessons

创建课时。

**请求体**:
```json
{
  "course_id": 1,
  "seq": 1,
  "title": "Unit 1: Daily Vocabulary",
  "skill": "vocab",
  "cefr": "B1"
}
```

**响应 201**: `{id: 10}`

### POST /api/content/items

创建题目。

**请求体**:
```json
{
  "lesson_id": 10,
  "item_type": "reading",
  "prompt": {"passage": "...", "options": ["A", "B", "C", "D"]},
  "answer": {"correct_option": 2},
  "explanation": "The correct answer is C because...",
  "difficulty": 3,
  "cefr": "B1",
  "tags": ["topic-education", "type-inference"],
  "source": "ESP Original",
  "license": "CC-BY-4.0"
}
```

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| lesson_id | int64 | 是 | 所属课时 |
| item_type | string | 是 | vocab/reading/listening/grammar/cloze |
| prompt | object | 是 | 结构化题干 |
| answer | object | 是 | 答案与判分基准 |
| explanation | string | 否 | 解析 |
| difficulty | int | 否 | 1-5,默认 3 |
| cefr | string | 否 | CEFR 等级 |
| tags | string[] | 否 | 主题/题型标签 |
| source | string | 否 | 内容来源(版权标注) |
| license | string | 否 | 授权标注 |

**响应 201**: `{id: 100}`

### PUT /api/content/items/:id

更新题目:状态流转 + 编辑建新版本(is_current 快照)。

**请求体**(状态流转):
```json
{"status": "published"}
```

**请求体**(编辑建新版本):
```json
{
  "edit": {
    "lesson_id": 10,
    "item_type": "reading",
    "prompt": {"passage": "...(修订)..."},
    "answer": {"correct_option": 2},
    "explanation": "...",
    "difficulty": 4,
    "cefr": "B1",
    "tags": [],
    "source": "ESP Original",
    "license": "CC-BY-4.0"
  }
}
```

**响应 200**(仅状态): `{id: 100}`
**响应 200**(编辑建新版): `{id: 101, superseded: 100}`

### POST /api/content/import

批量导入题目(1..5000 items)。

**请求体**:
```json
[
  {
    "lesson_id": 10,
    "item_type": "reading",
    "prompt": {...},
    "answer": {...},
    "explanation": "...",
    "difficulty": 3,
    "cefr": "B1",
    "tags": [],
    "source": "ESP Original",
    "license": "CC-BY-4.0"
  }
]
```

**响应 201**: `{created: 100}`
**响应 400**: `{error:"batch size must be 1..5000"}`
**响应 500**: `{error:"...", index: 5}` — 第 5 项失败,前面已成功的不回滚(见 I-6)

---

## 4. 错误响应格式

所有错误响应统一格式:

```json
{
  "error": "human readable message"
}
```

| HTTP 状态码 | 场景 |
|---|---|
| 400 | 请求体校验失败 / 批量导入空或超限 |
| 401 | 未登录(X-User-Id 缺失或 <=0) |
| 403 | 非 admin 访问管理接口 |
| 404 | 资源不存在 |
| 500 | DB 错误 |

---

## 5. 事件产出

submitAnswer 成功后写入 `esp_learning.learning_event_outbox`:

```json
{
  "user_id": 42,
  "item_id": 100,
  "item_type": "reading",
  "category": "reading",
  "correct": true,
  "score": 100,
  "source": "content-library",
  "occurred_at": "2026-08-17T10:30:00Z"
}
```

事件类型: `content.item_answered`
幂等键: message_id(random 32-char hex, ON CONFLICT DO NOTHING)
中继: outbox_relay(2s 轮询) → `esp_learning.learning_events` → gamification-go(XP) / learning-java(掌握度)
