# gRPC 替换 HTTP — LCO 链路向量化优化(下一 Phase 规划)

## 背景与目标
当前 LCO(Large Context Orchestration)链路用 HTTP+JSON 在组件间传 2560 维 float32 向量:
- lco-proxy(Go)→ embedding(Python FastAPI):`POST /embed` body `{"text","model"}` → `{"vector":[2560 floats]}` JSON 文本编码
- 2560 维 float32 JSON ≈ 25KB/请求(浮点数字符串);protobuf packed repeated float ≈ 10KB/请求(4 字节/float)
- 理论收益:体积 ~2.5x↓ + protobuf 二进制解析快于 JSON text parse
- 目标:在兼容期内并行 HTTP+gRPC,验证收益后切流,gRPC 成为 LCO 向量化主链路。

## 替换范围(已调研,2026-08-20 会话23-19)
| 调用点 | 当前协议 | 替换 | 优先级 |
|--------|---------|------|--------|
| lco-proxy → embedding | HTTP/JSON | gRPC + protobuf | P1(最大收益) |
| lco-proxy → milvus | REST API v2/vectordb | 保留(Milvus 有 gRPC SDK 但 REST 简化期合理) | P3(DEFERRED) |
| lco-proxy → neo4j | bolt(已二进制) | 不换(bolt 非 HTTP JSON) | — |
| orchestrator-rs | Rust 独立 | 不在本轮范围 | — |

## 实现方案

### Phase 5.1 — proto 定义 + 代码生成(P1,本轮规划落地)
1. 新建 `tools/lco-proxy/proto/embedding.proto`:
   ```protobuf
   syntax = "proto3";
   package lco.embedding.v1;
   option go_package = "lco-proxy/internal/proto/embeddingpb";

   service EmbeddingService {
     rpc Embed(EmbedRequest) returns (EmbedResponse);
     rpc EmbedBatch(EmbedBatchRequest) returns (EmbedBatchResponse);
     rpc Health(HealthRequest) returns (HealthResponse);
   }
   message EmbedRequest { string text = 1; string model = 2; }
   message EmbedResponse { repeated float vector = 1; string model = 2; int32 dim = 3; string device = 4; }
   message EmbedBatchRequest { repeated EmbedRequest items = 1; }
   message EmbedBatchResponse { repeated EmbedResponse results = 1; }
   message HealthRequest {}
   message HealthResponse { bool ready = 1; map<string,bool> loaded = 2; }
   ```
2. 装 `protoc-gen-go` + `protoc-gen-go-grpc`:`go install google.golang.org/protobuf/cmd/...@latest`
3. 生成 Go stub:`protoc --go_out=. --go-grpc_out=. proto/embedding.proto`
4. embedding 服务(Python)加 gRPC server:用 `grpcio` + `grpcio-tools` 生成 Python stub,与 FastAPI HTTP 并行监听(新端口 15731)。

### Phase 5.2 — lco-proxy gRPC client + 兼容期并行(P2)
1. lco-proxy config 加 `LCO_EMBEDDING_GRPC_ADDR`(默认 127.0.0.1:15731),`LCO_EMBED_PROTOCOL=http|grpc`(默认 http 兼容期)。
2. l2.go `embed()` 函数:protocol=grpc 走 gRPC client,protocol=http 走原 HTTP(保留 fallback)。
3. 实测对比:同 prompt 的 embed 延迟 + 体积,验证 2.5x 收益。

### Phase 5.3 — 切流 gRPC + HTTP 下线(P3)
1. 验证 gRPC 稳定后,`LCO_EMBED_PROTOCOL=grpc` 默认。
2. HTTP /embed 保留(向后兼容,调测用),但主链路走 gRPC。
3. 更新 start-lco-embedding.sh 启动 gRPC server。

## 依赖与工具链
- Go: `google.golang.org/grpc v1.82.1`(已有,indirect→promote direct)
- Go: `google.golang.org/protobuf v1.36.11`(已有)
- Python: `grpcio`, `grpcio-tools`(新增,requirements.txt)
- protoc 34.1 已装,需补 protoc-gen-go/protoc-gen-go-grpc

## 风险与铁律合规
- **SVD**:gRPC 端口 15731 非标,不与 HTTP 15730 混用;proto 打 tag 版本。
- **RWP**:gRPC server 复用 embedding 的 HOME/HF_HOME 配置,无新增可写路径。
- **ASV**:embed gRPC 返回的 vector 与 HTTP 版逐字节对比(同 model 同 text 必须一致),实测验证。
- **CLC**:gRPC 失败降级回 HTTP(fallback),不阻断 L2(与现有 L2 降级一致)。
- **SCS**:grpcio/grpcio-tools pin 版本,无秘密。

## 本轮(会话23-19)交付
- [x] 调研完成:调用点/协议/依赖/工具链/收益分析
- [x] proto 设计草稿(本文件)
- [ ] 实际 proto 文件 + 代码生成(需装 protoc-gen-go,下一会话)
- [ ] gRPC server/client 实现(P2/P3,下一 Phase)

## 下一步
1. 装 protoc-gen-go:`go install google.golang.org/protobuf/cmd/protoc-gen-go@latest` + `protoc-gen-go-grpc@latest`
2. 写 `tools/lco-proxy/proto/embedding.proto`
3. 生成 Go stub,跑通 hello-world 级 gRPC(不接真实 embedding)
4. Python embedding 加 gRPC server(与 FastAPI 并行)
5. lco-proxy 加 gRPC client + 协议开关,兼容期并行实测
