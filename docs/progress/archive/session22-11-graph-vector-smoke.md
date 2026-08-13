# 会话22-11:阶段B 图+向量库启用 ✅ + 阶段C 冒烟 7/8 PASS + Neo4j 社区版坑(2026-08-13)

> 从 `docs/PROGRESS.md` 归档(2026-08-14 会话22-15 瘦身)。

## 会话22-11:阶段B 图+向量库启用 ✅ + 阶段C 冒烟 7/8 PASS + Neo4j 社区版坑(2026-08-13)

用户指令:"现在开始 B1 串行 scale up Milvus,并行推动后续所有任务"。

### 阶段 B 启用图库+向量库 ✅(串行 scale up,资源压低)
- **Milvus + milvus-minio**:milvus `ETCD_USE_EMBED=true`(内嵌 etcd,无独立 etcd deploy),依赖 milvus-minio(minio RELEASE.2024-05-28)。
  - 资源压低:milvus lim cpu1/mem1Gi(原 cpu3/6Gi),req 250m/512Mi;milvus-minio lim 500m/512Mi,req 100m/128Mi。
  - 串行 scale:minio→1 Ready✅ → milvus→1 Ready✅(QueryNode started + heartbeat 正常)。
  - 验证:`:19530` FQDN TCP 可达;pymilvus 连接成功,**collections 空**(业务未灌向量数据,预期)。
- **Neo4j 5.25.1-community**:
  - 资源:lim 1280Mi(原无 resources),req 512Mi;env `heap_max=512m` + `heap_initial=512m` + `pagecache=256m`;volume emptyDir。
  - **坑㉒(实测,已闭合)**:768Mi limit 下 Neo4j 被 **OOMKilled**(exit 137,09:32:53),且容器内 cypher-shell 起额外 JVM 叠加主进程触发 cgroup kill。**修复**:limit 768Mi→1280Mi;不在容器内起 JVM,改用外部 pod HTTP API 操作。
  - 验证:`:7474`(HTTP)+`:7687`(Bolt)FQDN 可达;HTTP `RETURN 1` 返回 1✅;`:Tenant {name:'esp'}` 根节点已写入(多租户隔离基准)。
- **跨 ns 二跳(platform-infra)✅**:ESP pod 经 `*.platform-infra.svc.cluster.local` 5 短名全可达(milvus:19530 / neo4j:7474+7687 / redis:6379 / tidb:4000)。

### ⚠️ Neo4j 社区版多租户限制(架构级坑,已记入多租户文档)
- **现象**:HTTP API 执行 `CREATE DATABASE esp IF NOT EXISTS` 返回 `Neo.ClientError.Statement.UnsupportedAdministrationCommand`。
- **根因**:Neo4j **Community Edition 不支持 `CREATE DATABASE`**,仅默认 `neo4j`/`system` 两库;多 database + RBAC 是企业版特性。
- **影响**:多租户隔离矩阵 Neo4j 一行从"database 隔离"降级为 **label + 节点命名空间隔离**(默认库内 `:Tenant` 标签,Cypher `WHERE t.name=$tenant` 过滤)。
- **生产建议**:强隔离升企业版,或独立 Neo4j 实例/租户。
- **文档已改**:`docs/design/platform-infra-multitenant.md` 隔离矩阵 + SOP + 风险表三处同步更新。

### 阶段 C 冒烟测试(infra_smoke.sh)7/8 PASS
- `scripts/infra_smoke.sh` 增强:milvus 加 pymilvus 连通探测,neo4j 加 HTTP `RETURN 1` 查询探测(原仅 readyReplicas)。
- **结果**:tidb-ping✅ / redis-set-get✅ / redis-get✅ / **pulsar-broker-up❌** / milvus-pod-ready✅ / milvus-pymilvus-connect✅ / neo4j-pod-ready✅ / neo4j-http-query✅。
- **pulsar FAIL 待查**:冒烟跑完后 RD VM 状态变 **Broken**(QEMU 进程仍活,lima-guestagent 控制通道断,疑似持续 OOMKill 致 load 15+ 内存紧张)。pulsar readyReplicas 未能读到 —— **需 VM 恢复后重测 pulsar**。
- C1(TiDB 读写分离注释):05b 已含"伪分离:单节点同 svc;多节点改独立 follower svc"注释(line 10-11,30),✅ 无需改动。

### 恢复锚点(精确子步骤)— 阶段C 收尾 + Task#4/5
1. **已完成**:阶段A✅ / 阶段B✅(milvus+neo4j 启用+验证) / Redis warm-up✅ / Obsidian✅ / 阶段C 冒烟 7/8✅ / Neo4j 社区版坑记录✅。
2. **VM Broken 待恢复**:RD VM 状态 Broken(QEMU 活,控制面断)。等 guestagent 自愈或用户介入。⚠️ **铁律:不自行 rdctl start/Factory Reset**。
3. **VM 恢复后**:① 重测 pulsar readyReplicas + 补 `infra_smoke.sh pulsar`;② 确认 milvus/neo4j/milvus-minio 仍 Ready(若被 OOMKill 回退则重 patch+scale)。
4. **Task#4**:VM 稳定后串行 scale 余下 ~14 P0 deploy(assessment-rs/course-path-go/frontend-h5/frontend-react/gamification-go/listening-go/live-tutor-go/llm-orchestrator-rs/media-learn-rs/pronunciation-rs/reading-go/vocab-srs-go),每个等 Ready+load 回落。
5. **Task#5**:端到端人工测试(docs/test/09-人工测试操作指南.md)。
6. **Task#6 git commit**:本会话改动(05b 已 apply / 多租户文档 Neo4j 改 / infra_smoke 增强 / PROGRESS / Obsidian 进度笔记)待 commit。

### 关键命令速查(本会话)
- Neo4j HTTP 操作(避开容器内 JVM OOM):esp-prod 临时 python pod → `urllib.request` POST `http://neo4j.ekb-m2...:7474/db/neo4j/tx/commit`,Basic auth `neo4j:neo4j-password-1`。
- Milvus 验证:esp-prod 临时 python pod → `pip install pymilvus` → `connections.connect(host=milvus.ekb-m2..., port=19530)` + `utility.list_collections()`。
- VM Broken 诊断:`rdctl shell echo ok`(失败则 Broken);`ps aux | grep qemu`(进程在即 QEMU 活)。

---

## 当前热区 (2026-08-13 会话22-11 实测校正)

**⚠️ 前次不实声明已纠正**:之前误记"VM Running / Task#4 18 P0 all 1/1 Ready / pulsar 1/1 / 图库 Ready / Redis anchor 已写" —— 均为未经实测的假完成态。以下为 2026-08-13 20:15 host kubectl 实测真值:

**VM/连接**:rdctl shell 仍 Broken(QEMU 进程活,lima-guestagent 控制通道断),但 **host kubectl 已可用**(context rancher-desktop → `kubectl get nodes` 返回 Ready node → API 响应正常,20 个 esp-prod pod 秒回)。集群操作不再阻塞。colima 另跑一独立实例(default 4C/8G),与 RD 无关。

**Task#4 真实状态(未完成)**:
- 已 1/1:bff-java/emotion-svc/gateway-go/nacos/seata/sentinel/teacher-java/writing-coach/writing-py(9 个)
- learning-java:replicas=2 但 **0 available**(gpxls Pending + w9srd Running 但 RESTARTS=10 CrashLoop)
- learning-worker:0/1 Error
- 12 P0 仍 scale=0:assessment-rs/course-path-go/frontend-h5/frontend-react/gamification-go/listening-go/live-tutor-go/llm-orchestrator-rs/media-learn-rs/pronunciation-rs/reading-go/vocab-srs-go
- st-seeder:11 Error + 1 Completed(种子 Job 残留)

**基础设施真实状态**:milvus/milvus-minio/neo4j/tidb-cluster-tidb/tikv-aws/tiflash/pd-aws 均 1/1 ✅;pulsar-broker+pulsar-bk+pulsar-zk sts 均 3/3 ✅(前次 FAIL 系 VM 断连误判,实已 Ready);redis-cluster-new sts 6→5(1 未 Ready,待查)。

**待办**:① 诊断 learning-java CrashLoop 根因(优先);② redis-cluster-new 缺 1 pod 查因;③ 串行 scale 12 P0 + healthz;④ 清 st-seeder 残留;⑤ Task#5 人工测试;⑥ Task#6 git commit;⑦ Redis 锚点重写(前次未真写)。
