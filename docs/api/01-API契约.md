# Home-Tutor 平台 — API 契约 (OpenAPI 3.1 摘要)

> 完整契约以各服务 OpenAPI 自动生成为准(Litestar/Spring 自动产出 `/openapi.json`)。本文为人工基线。
> 鉴权:除 `/auth/*`、`/healthz`、`/metrics` 外,所有接口需 `Authorization: Bearer <JWT(RS256)>`,由 BFF 边缘校验并透传 `X-User-Id`/`X-Role`。

## 0. 公共
| Method | Path | 说明 |
|---|---|---|
| GET | `/healthz` | 各服务健康检查(200) |
| GET | `/metrics` | Prometheus 指标(gateway-go 用 promhttp;Java 用 /actuator/prometheus) |

## 1. 认证 `/auth`(BFF 承载)
| Method | Path | Body/Params | 响应 |
|---|---|---|---|
| POST | `/auth/register` | `{email,password,role,name}` | `{token,user}` |
| POST | `/auth/login` | `{email,password}` | `{token,user}` |
| POST | `/auth/wx-login` | `{code}` | `{token,user:{id,name,role}}`(小程序 wx.login code 换 JWT,ADR-14) |

## 2. 口语陪练 (gateway-go)
| Method | Path | 说明 |
|---|---|---|
| GET | `/api/scenarios` | 场景列表 `[{code,title,system,difficulty}]` |
| POST | `/api/sessions` | `{scenario_code}` → `{session_id,scenario}` |
| WS | `/ws/speech/{session_id}` | 口语实时流 |

### WS 消息帧(JSON)
```json
// client → server
{"type":"audio_chunk","seq":1,"data":"<base64-mock>"}
{"type":"text_input","text":"I'd like to check in."}
{"type":"text_input","text":"/hint"}
{"type":"end"}
// server → client
{"type":"ai_reply","text":"Sure! May I see your passport?","data":"<base64-tts>"}
{"type":"correction","original":"I wants","corrected":"I want","phoneme":null}
{"type":"hint","options":["Show passport","Ask for window seat"],"hints":[...]}
{"type":"end","summary":"Session ended."}
{"type":"error","message":"..."}
```

## 3. 自适应学习 (learning-java)
| Method | Path | Body/Params | 响应 |
|---|---|---|---|
| POST | `/api/learning/placement` | `{user_id,kp_ids[],score}` | `{initialized:n}` |
| POST | `/api/learning/event` | `{user_id,event_type,payload}` | 202 `{accepted:true}` / 503 |
| GET | `/api/learning/review?user_id=` | — | `[{userId,kpId,masteryScore,nextReviewAt}]` |
| GET | `/api/learning/mastery/weak?user_id=` | — | `UserMastery[]`(按 score 升序) |
| POST | `/api/learning/difficulty` | `{current,correct}` | `{next_difficulty,speed_multiplier,scaffolding}` |

## 4. 写作批改 (writing-py, Litestar + LangGraph)
| Method | Path | Body | 响应 |
|---|---|---|---|
| POST | `/api/writing/grade` | `{prompt,essay,exam}` | `{score,feedback,grammar[],coherence{score,transitions,paragraph_flow}}` |
| POST | `/api/writing/polish` | `{text}` | `{polished,suggestions[{original,polished,reason}]}` |
| POST | `/api/writing/translate` | `{text}` | `{translation,vocab[{word,pos,meaning,example}]}` |
| POST | `/api/writing/run` | `{prompt,essay,exam,human?}` | `{thread_id,status,grade_result,polished,notes}`(human=true 时 status=awaiting_review) |
| POST | `/api/writing/review` | `{thread_id,teacher_feedback}` | `{thread_id,status:done,notes}`(恢复 LangGraph → notes) |
| GET | `/api/writing/state/{thread_id}` | — | `{status,grade_result,polished,teacher_feedback,notes}` |

## 5. 教师端 (teacher-java)
| Method | Path | Body/Params | 响应 |
|---|---|---|---|
| POST | `/api/teacher/lesson-plan` | `{topic,level}` | `{passage,exercises[],games[],discussion[]}` |
| GET | `/api/teacher/classes/{id}/analytics` | — | `[{id,content,avgScore,count}]`(TiFlash) |
| POST | `/api/teacher/assignments/{id}/auto-grade` | — | `{graded:n}` |
| POST | `/api/teacher/submissions/{id}/finalize` | `{final_score}` | `{ok:true}` |

## 6. 情感化多模态 (emotion-svc, Litestar)
| Method | Path | Body | 响应 |
|---|---|---|---|
| POST | `/api/emotion/sense` | `{text,audio}` | `{emotion,encouragement}` |
| POST | `/api/emotion/visual` | `{word,context}` | `{image_url,alt}` |

## 7. 内部 RPC(Dubbo Triple,不经 BFF)
| 接口 | 提供者 | 消费者 | 方法 |
|---|---|---|---|
| `LearningRpcService` | learning-java | teacher-java | `List<KpStat> userStats(Long userId)` |

## 8. 错误码约定
| HTTP | 含义 |
|---|---|
| 400 | 请求参数错误 |
| 401 | 未认证 / JWT 无效 |
| 403 | 无权限(RBAC) |
| 422 | 语义校验失败(Pydantic) |
| 429 | 限流(Sentinel) |
| 503 | 服务不可用(如 Disruptor 满) |
