# ESP — Claude Code 工作约定(长期记忆 / 上下文工程)

> 本文件是项目级长期记忆。每次会话自动加载。对话历史会被总结/截断,**所有需要跨会话保留的事实必须落盘到文件,不得只存在于对话中**。

## 0. 项目一句话
AI 英语教育平台(Home-Tutor / ESP):多语言微服务(Go/Java/Python)+ 读写分离 + Disruptor + TiDB + Pulsar + K8s,目标 5000 CCU、三端(Web/H5/小程序)、30 亿测试数据、企业级监控、HPA 弹性、全测试阶段可人工测试。

## 1. 上下文工程铁律(防止上下文溢出)
1. **文件系统 = 长期记忆,对话 = 临时工作区**。进度、决策、坑、任务清单只写文件。详见 `docs/PROGRESS.md`。
2. **Subagent 做上下文隔离**:任何"读多文件 / 跑长命令 / 大量搜索"的子任务,派 Subagent(Agent 工具)去做,主线程只收结论。不要把大段日志/文件内容拉进主上下文。
3. **严格控制工具输出**:
   - Bash 命令必须 `| head` / `| tail` / `| grep` / `--tail=N` 限流;禁止裸 `cat` 大文件、裸 `kubectl logs`(用 `--tail=20`)。
   - 跑测试/构建:只看 `RC` 和失败摘要,不要全量 stdout。用 `2>&1 | tail -30`。
   - 压测/种子:输出重定向到 `report/` 或 `logs/` 文件,主线程只 Read 摘要行。
4. **外部缓存(Redis)做跨会话运行态**:压测计数、压测状态、种子进度写 Redis(key 前缀 `esp:ctx:*`),不靠对话记忆。
5. **任务清单用 TaskCreate/TaskUpdate**,跨会话可恢复;关键里程碑同步写 `docs/PROGRESS.md`。
6. **每完成一个大阶段立即更新 `docs/PROGRESS.md`**(已完成项+路径、下一步、新坑)。

## 2. 执行模式
- 已授权"全程零确认"(见 memory `exec-mode-bypass`)。日常 build/test/deploy/kubectl/docker/locust 不暂停确认。
- 仅在**不可逆高风险操作**(删 namespace、force push、infra teardown、删生产数据)前口头提示一句再执行。
- settings.json 已设 `Bash(*)/Read(**)/Edit(**)/Write(**)` allow + `bypassPermissions`。

## 3. 环境
- K8s:Rancher Desktop 单节点,context `rancher-desktop`,命名空间 `esp-prod`。
- 基础设施复用 ns `ekb-m2-aws-us-east-1`(TiDB/Redis Cluster/Pulsar),通过 ExternalName 短名 `tidb-primary`/`tidb-follower`/`redis-cluster`/`pulsar-broker` 访问。
- TiDB: `tidb-primary:4000` root 无密码,DB `esp`,11 张表(已建,schema 见 `docs/design/schema.sql`)。
- 各服务端口:gateway-go 8080、learning 8081、writing 8082、teacher 8083、emotion 8084。NodePort gateway-go 30080。
- 非 git 仓库 —— 无 git 安全网,改动靠文件 + PROGRESS.md 追踪。

## 4. Subagent 分工(见 `.claude/agents/*.md`)
- `esp-explorer`:只读搜索/定位代码与资源,返回路径+结论。
- `esp-builder`:跑构建/测试/部署,返回 RC+失败摘要。
- `esp-data`:种子数据与压测,输出落 `report/`/`logs/`,返回摘要。
- `esp-doc`:回填测试文档/写文档,返回写入路径清单。

## 5. 常用命令速查
- 查 TiDB 表:见 `scripts/db_query.sh`(pymysql pod 封装,已验证可用)。
- 服务日志:`kubectl -n esp-prod logs deploy/<svc> --tail=20`。
- 压测:分布式 locust 见 `deploy/locust/README.md`。
