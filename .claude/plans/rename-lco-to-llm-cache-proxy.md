# lco-proxy → llm-cache-proxy 改名计划(2026-08-20 会话23-20)

## 背景
Claude Code 切流回退后,LCO 主要服务 ESP 业务侧 orchestrator(非 Claude Code)。
"lco-proxy" 名与 ESP 其它项目区分度低。改为 `llm-cache-proxy`(通用,去 ESP 前缀,可多项目复用)。
保留后续对 Claude Code LLM 多级缓存的支持(env 切回 15720 即重用)。

## 改名范围(精确盘点)

### A. Go module + import(核心,编译级)
- `tools/lco-proxy/go.mod`: `module lco-proxy` → `module llm-cache-proxy`
- 14 处 import `"lco-proxy/internal/..."` → `"llm-cache-proxy/internal/..."`
  - 涉及:l2.go/l3.go/manager.go/config.go/main.go 等
- go.work: `./tools/lco-proxy` → `./tools/llm-cache-proxy`

### B. 目录/文件重命名
- `tools/lco-proxy/` → `tools/llm-cache-proxy/`(git mv 整目录)
- `tools/lco-proxy/cmd/lco-proxy/` → `tools/llm-cache-proxy/cmd/llm-cache-proxy/`(cmd 目录名)
- `tools/lco-proxy/Dockerfile` 内路径更新
- `.github/workflows/lco-proxy-ci.yml` → `llm-cache-proxy-ci.yml` + 内容路径更新

### C. 二进制/日志/脚本(宿主运行态)
- `/tmp/lco-proxy-test` → `/tmp/llm-cache-proxy-test`(LCO_BIN)
- `/tmp/lco-proxy.log` → `/tmp/llm-cache-proxy.log`
- `scripts/start-lco.sh` → `scripts/start-llm-cache.sh`(重命名 + 内容更新)
- `scripts/start-lco-embedding.sh` → `scripts/start-llm-cache-embedding.sh`
- `scripts/verify-lco-switch.sh` → `scripts/verify-llm-cache-switch.sh`
- pkill 模式:`lco-proxy-test` → `llm-cache-proxy-test`

### D. 文档(10+ 处,非 archive)
- `docs/design/lco-proxy-sdd.md` → `llm-cache-proxy-sdd.md`
- `docs/design/lco-proxy-tool-integration.md` → `llm-cache-proxy-tool-integration.md`
- `docs/design/architecture/adr-lco-proxy.md` → `adr-llm-cache-proxy.md`
- 其它文档(docs/PROGRESS.md 等)内的 `lco-proxy` 文本替换
- archive/ 不改(历史快照,只读)

### E. embedding 服务(宿主资源,用户要求兼顾)
- 代码内 logger 名 `lco-embedding` → `llm-cache-embedding`(app.py/grpc_server.py)
- 注释里 `lco-proxy` → `llm-cache-proxy`
- **venv 名保留** `~/lco-embedding-venv`(移动 venv 风险高,PATH/激活脚本都依赖,保留)
- **模型目录保留** `~/lco-models`(数据资产,不动)
- grpc_stubs/ Python import 不受影响(相对路径)

## 保留不改(降低风险)
- **env 前缀 LCO_**:保留(LCO_L2_THRESHOLD/LCO_EMBED_PROTOCOL 等)。改 env 前缀破坏运行态配置一致性,收益低。代码注释说明"LCO_ 前缀保留历史兼容"。
- **端口 15720/15730/15731**:不变。
- **Milvus collection** lco_prompts:保留(数据资产,重建丢缓存)。
- **archive/ 文档**:历史快照不改。

## 执行步骤(顺序)
1. **git mv 目录**:`tools/lco-proxy` → `tools/llm-cache-proxy` + cmd 子目录
2. **改 go.mod module 名** + go.work 路径
3. **批量改 import**:`sed` 替换 14 处 `"lco-proxy/` → `"llm-cache-proxy/`
4. **编译验证**:`go build ./cmd/llm-cache-proxy/`(goproxy.cn)
5. **改脚本**:重命名 start-*.sh + 内容(LCO_BIN/日志路径/pkill 模式)
6. **改 CI**:workflow 文件重命名 + 路径
7. **改文档**:非 archive 文档 lco-proxy → llm-cache-proxy(文本替换 + 文件重命名)
8. **改 embedding 代码**:logger 名 + 注释
9. **宿主重启**:停旧 lco-proxy/embedding → 用新脚本/二进制启动 → 验证 LCO 三层 ok + gRPC 链路
10. **清理旧 /tmp 文件**:lco-proxy-test/log 等旧名文件

## 验证(ASV/CLC)
- `go build` 全模块 OK(15/15)
- lco-proxy 重启后 healthz 三层 ok
- gRPC embedding 链路正常(LCO_EMBED_PROTOCOL=grpc)
- /unload + 懒加载仍工作
- 集群 pod 不受影响(不改 k8s deploy,只改宿主工具)

## 风险与回退
- Go module 改名是编译级,若 import 漏改会 build 失败(易发现,sed 批量+编译验证)。
- 回退:git mv 回 + sed 回(非 git 仓库,但有 PROGRESS 追踪)。
- 运行态:改名期间 lco-proxy 短暂下线(~30s),ESP 业务侧 orchestrator 降级直穿 provider(非致命)。
