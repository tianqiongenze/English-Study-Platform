# ESP 多节点迁移清单

> 生成日期:2026-08-17
> 用途:从单节点 Rancher Desktop 验证拓扑迁移到多节点生产拓扑(3+ 节点)的完整待办清单
> 依据:Phase5 重构后架构重评(7.77/10) + 5000 CCU 实压测(533rps/P95 25s/7.3% 失败)暴露的瓶颈 + TOGAF Phase D/E/F 治理缺口

---

## 1. 基础设施与集群

| 类别 | 项目 | 当前状态 | 多节点目标 | 依赖 | 优先级 |
|---|---|---|---|---|---|
| 容量 | 5000 CCU 压测达标 | 单节点 533rps / P95 25s / 失败率 7.3% | 多节点 5000 CCU: 吞吐 >= 2000rps / P95 < 2s / 失败率 < 0.1% | 多节点 K8s(3 节点+) + 分布式 Locust(master+5 workers 独立节点) | P0 |
| 基础设施 | metrics-server | 未部署(单节点 K8s 无 HPA 基础) | 部署 metrics-server,HPA 有 CPU/MEM 数据源 | K8s metrics-server helm chart | P0 |
| 基础设施 | CNI 升级 | flannel(单节点,不支持 NetworkPolicy) | Cilium(eBPF 绕 kube-proxy) 或 Calico(保守);支持 NetworkPolicy + NetworkPolicy 验证 | 多节点 K8s | P0 |
| 基础设施 | Istio Service Mesh | 未部署 | Ambient mesh(ztunnel);mTLS + L7 流量管理 | 多节点 CNI + Istio 1.24+ | P1 |
| 节点 | K8s 多节点集群 | Rancher Desktop 单节点(8G/9核) | 3+ 节点(建议 4C/16G worker x3,16C/32G master x1) | 服务器采购/云资源 | P0 |

## 2. CI/CD 与镜像仓库

| 类别 | 项目 | 当前状态 | 多节点目标 | 依赖 | 优先级 |
|---|---|---|---|---|---|
| CI/CD | Harbor 镜像仓库 | DEFERRED(宿主 docker daemon 残缺,镜像搬运受限) | 私有 Harbor 镜像仓库;CI push / ArgoCD pull | Docker.app 修复 or 独立 Harbor Pod | P0 |
| CI/CD | 完整 CI Pipeline | GitHub Actions 10 job 已有;缺 Jenkins/GitLab 集成 | Harbor ready + CD ArgoCD syncPolicy(已配) 完整闭环 | Harbor ready | P1 |
| CI/CD | ArgoCD 多集群 | 单集群 ArgoCD app/project(已配) | ArgoCD 集成新集群;syncPolicy 自动同步 | 多节点 K8s ready | P0 |

## 3. 安全

| 类别 | 项目 | 当前状态 | 多节点目标 | 依赖 | 优先级 |
|---|---|---|---|---|---|
| 安全 | mTLS | 未部署(JWT RS256 应用层已闭环) | Ambient mesh mTLS(Istio 1.24+);服务间零信任 | Istio Ambient 部署 | P1 |
| 安全 | securityContext | 已补齐(9 部署:allowPrivilegeEscalation:false + runAsNonRoot:true) | 保持;补 Pod Security Standards(PSS) 强制 | 多节点 ready | done |
| 安全 | Ingress WAF | 单节点无外网入口(占位) | Ingress 网络层防护(OWASP CRS + 限速 + bot 防护) | Istio Gateway / EnvoyFilter | P1 |
| 安全 | NetworkPolicy | 已 patch namespaceSelector:{} 放行所有 ns(坑㉕修复) | 收紧回 namespace 级精细化;按服务最小权限 | CNI 支持 NetworkPolicy(Cilium/Calico) | P0 |

## 4. 数据与存储

| 类别 | 项目 | 当前状态 | 多节点目标 | 依赖 | 优先级 |
|---|---|---|---|---|---|
| 数据 | CockroachDB 30 亿数据重建 | TiDB 下电离线;CRDB 初步 schema 已建 | CRDB 3 节点集群;30 亿 learning_events 重建基准 | 多节点 CRDB 集群部署 | P0 |
| 数据 | Redis Cluster 多区域 | 已升真 Cluster(hostname 模式);单节点 6 pod | Redis Cluster 多区域独立(各区域不强同步) | 多节点 K8s;Redis cluster 拓扑验证 | P1 |
| 数据 | Pulsar 稳定性 | 频繁重启(12 次/19h);单节点资源受限 | Pulsar 集群(3 broker + 3 bookie);Geo Replication 3 区域 | 独立节点(4C/16G x3) | P0 |
| 数据 | TiDB 读写分离 | follower lag 已在代码层防御(@DataSource(consistent=true) + @Version) | TiDB 多节点集群;follower lag 实测验证 | TiDB 多节点部署 | P1 |

## 5. 可观测性

| 类别 | 项目 | 当前状态 | 多节点目标 | 依赖 | 优先级 |
|---|---|---|---|---|---|
| 可观测 | 双层 OTel Collector | 单层(已工作,tail sampling 10% 已 patch) | 双层(DaemonSet 边缘 + Gateway 中心);tail sampling 10% 独立于应用 | 多节点 DaemonSet | P1 |
| 可观测 | 分布式 Locust | 单节点 pod 内 locust(与被测同节点) | master + 5 workers 独立节点;分布式压测 | 多节点 K8s;Locust 镜像就绪 | P0 |
| 可观测 | Tempo/Loki 多节点 | 单节点单副本 | Tempo 2+ 副本 + Loki 2+ 副本;反亲和 | 多节点 K8s | P1 |
| 可观测 | Prometheus 告警规则 | 7 pod 已部署;告警规则触发未实测 | AlertManager + 告警规则实测(ServiceDown/HighErrorRate) | Prometheus 部署就绪 | P1 |

## 6. 混沌测试(多节点才可执行)

| 类别 | 项目 | 当前状态 | 多节点目标 | 依赖 | 优先级 |
|---|---|---|---|---|---|
| 混沌 | net_partition | CNI 不支持(单节点 flannel) | Cilium/Calico NetworkPolicy 真实网络分区注入 | 多节点 CNI(Cilium/Calico) | P1 |
| 混沌 | latency_inject | 缺 NET_ADMIN(capability) | tc netem 真实延迟注入 | 特权容器 或 Cilium bandwidth policy | P2 |
| 混沌 | pod-kill(多副本) | 单副本(无实际驱逐效果) | PDB 验证:2+ 副本下驱逐 1 个,服务不中断 | 多副本部署 + PDB 配置 | P1 |
| 混沌 | node-failure | 单节点(不适用) | 模拟节点宕机:Pod 调度漂移 + 数据一致性验证 | 多节点 K8s | P2 |

## 7. 业务功能(待引擎接入)

| 类别 | 项目 | 当前状态 | 多节点目标 | 依赖 | 优先级 |
|---|---|---|---|---|---|
| 业务 | ASR(NLP) | DEFERRED(listening 服务 Mock 模式) | 真实 ASR 引擎接入(Google/阿里云/讯飞) | 第三方 ASR 服务 + API Key | P1 |
| 业务 | NLP(阅读理解) | DEFERRED(reading 服务 Mock 模式) | 真实 NLP 引擎接入 | 第三方 NLP 服务 | P1 |
| 业务 | TTS | Mock 模式已闭环 | 真实 TTS 引擎接入(conversation-go) | 第三方 TTS 服务 | P1 |

## 8. 治理与运维

| 类别 | 项目 | 当前状态 | 多节点目标 | 依赖 | 优先级 |
|---|---|---|---|---|---|
| 治理 | Sentinel Dashboard | Nacos 动态规则已配(限流/熔断) | 保留 Sentinel 控制台;独立 Pod;多节点规则同步 | 独立 Sentinel Pod | P2 |
| 治理 | 架构治理委员会 | 章程已补(architecture-board-charter.md) | 实际运行:周期例会 + 架构合规审查 | 人员到位 | P1 |
| 治理 | TOGAF Phase A/E/F | 5.0/5.0/5.5(治理欠账) | 利益相关者矩阵 + 过渡架构路线图 + 里程碑依赖矩阵 | 仅文档工作 | P0 |
| 运维 | Helm Charts | 手动 kubectl apply(26 manifest) | Helm chart 标准化;values.yaml 多环境(dev/staging/prod) | Chart 开发 | P1 |
| 运维 | DR Runbook | 多区域数据同步方案已设计 | region 切换演练步骤 + 健康判断 + 回切 | 多区域部署 | P1 |

---

## 迁移执行顺序(建议)

```
Phase A(准备,并行):
  ├── 多节点 K8s 集群搭建(3+ 节点)
  ├── Harbor 镜像仓库部署
  ├── CNI 升级(Cilium/Calico)
  ├── metrics-server 部署
  └── TOGAF A/E/F 文档补全(仅文档,无部署)

Phase B(核心服务迁移):
  ├── CRDB 3 节点集群部署 + 数据迁移
  ├── Pulsar 集群(3 broker)部署
  ├── Redis Cluster 多节点拓扑验证
  ├── 分布式 Locust 部署
  └── 5000 CCU 压测达标验证

Phase C(观测+安全):
  ├── 双层 OTel Collector(DaemonSet+Gateway)
  ├── Tempo/Loki/Prometheus 多副本
  ├── Istio Ambient mesh(可选)
  ├── NetworkPolicy 精细化
  └── Ingress WAF

Phase D(混沌+治理):
  ├── 混沌测试(net_partition/latency/pod-kill/node-failure)
  ├── PDB 实测验证
  ├── Sentinel Dashboard 独立部署
  ├── DR Runbook 演练
  └── TOGAF 治理闭环
```

---

## 单节点铁律(迁移期间必须遵守)

1. **共享基础设施优先保障**:Redis/Pulsar/TiDB 是多项目群共用底座,ESP 操作不得损害。
2. **VM 运行时禁停 colima/动 lima 网络**:历史事故(2026-08-13)根因。
3. **不碰 k3s 配置/restart/pkill/Factory Reset**。
4. **主机重操作与集群操作必须串行**:并行饿死 VM CPU。
5. **VM 重启后必预置 pause 镜像**:否则 sandbox 建不起来。
6. **rc-service docker restart 连锁 stop k3s**:改 daemon.json 后勿无脑重启 docker。
