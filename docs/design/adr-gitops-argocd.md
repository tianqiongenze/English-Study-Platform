# ADR: GitOps — 引入 ArgoCD 替代手动 kubectl apply

- 日期:2026-08-07
- 状态:Accepted(待 git 迁入后落地)
- 关联:ESP 项目 `deploy/gitops/`、`deploy/k8s/`、`.github/workflows/security.yml`

---

## 1. 背景

ESP 项目当前部署方式为**手动 `kubectl apply -f deploy/k8s/`**,存在三类工程化缺口:

1. **无审计**:谁在何时改了集群状态,只能靠 `kubectl get events` / Pod 日志回溯,没有与变更单关联的提交链。
2. **无漂移检测**:开发期临时 `kubectl edit` / `kubectl scale` 后,集群状态与仓库 manifest 不一致,后续 apply 可能覆盖或被覆盖,无告警。
3. **无回滚**:回滚靠人记忆上一版镜像 tag + 手动重 apply,慢且易错;30 亿测试数据 + 5000 CCU 场景下,一次错误部署的成本极高。

而 ESP 已有 CI(`.github/workflows/security.yml`)跑安全扫描与测试,但**部署环节与 CI 耦合**——一旦 CI 直接 kubectl apply,部署动作分散在多个 workflow,审计与回滚更难统一。

## 2. 决策

**引入 ArgoCD 作为 GitOps 控制器,替代手动 kubectl apply**,采用 App-of-Apps 模式纳管三类工作负载:

| Application       | 命名空间                  | source.path              |
|-------------------|--------------------------|--------------------------|
| `esp-apps`        | esp-prod                 | deploy/k8s(排除监控/子目录)|
| `esp-monitoring`  | esp-observability        | deploy/k8s/monitoring    |
| `esp-infra`       | ekb-m2-aws-us-east-1     | deploy/k8s(05/06*/07*)   |

syncPolicy.automated 开启 `selfHeal: true` + `prune: true`,实现漂移自动修复 + 删除自动回收。交付物见 `deploy/gitops/argocd-project.yaml`(AppProject)与 `deploy/gitops/argocd-apps.yaml`(App-of-Apps)。

## 3. 选型理由:ArgoCD vs Flux

| 维度          | ArgoCD                                  | Flux                                  |
|---------------|-----------------------------------------|---------------------------------------|
| UI            | 内置 Web UI,可视化 sync 状态/diff/历史 | 无原生 UI(需 weave-gitops 等补充)    |
| kubectl 风格  | CRD(Application/AppProject)+ kubectl 友好 | 基于 kustomize/helm,CRD 体验稍弱     |
| 国内大厂落地  | 更常见,招聘与运维经验可得性高          | 相对小众                              |
| 漂移检测      | 内置,UI 可见 diff                       | 内置,但可视化弱                      |
| 多集群        | 支持                                    | 支持                                   |

结论:**选 ArgoCD**。对教学/演示场景(用户兼具学术与工业背景)与后续多节点迁移,UI + kubectl 友好 + 国内大厂常见三者权重最高。

## 4. 架构

```
┌─────────────┐    push/merge     ┌──────────────────┐
│  git repo   │ ────────────────> │   GitHub main    │
│ (单一事实源) │                   │ (manifest 目录)  │
└─────────────┘                   └────────┬─────────┘
                                           │ watch (webhook/poll)
                                           ▼
                                  ┌──────────────────┐
                                  │     ArgoCD       │
                                  │ (argocd ns)      │
                                  └────────┬─────────┘
                                           │ sync (kubectl apply/diff)
                                           ▼
                              ┌────────────────────────────┐
                              │   K8s cluster (Rancher)    │
                              │  esp-prod / esp-observ /   │
                              │  ekb-m2-aws-us-east-1      │
                              └────────────────────────────┘
```

App-of-Apps 模式:root Application(`esp-root`)指向 `deploy/gitops/` 目录,自动 sync 出 3 个子 Application,子 Application 再 sync 实际工作负载。新增应用只需往 `deploy/k8s/` 加 manifest 并提交,无需改 ArgoCD 配置。

## 5. 落地步骤

1. **git 迁入**(待用户提供 GitHub URL):`git init` → 首次提交 → push 到 `https://github.com/<ORG>/esp.git`,默认分支 `main`。
2. **安装 ArgoCD**(占位说明,不写安装 manifest——太大):
   ```
   kubectl create namespace argocd
   kubectl apply -n argocd \
     -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml
   ```
3. **应用 AppProject + App-of-Apps**:
   ```
   kubectl apply -f deploy/gitops/argocd-project.yaml
   kubectl apply -f deploy/gitops/argocd-apps.yaml
   ```
4. **配置 repo credential**:在 ArgoCD 中配置 GitHub repo 的 SSH deploy key 或 HTTPS token(命令:`argocd repo add https://github.com/<ORG>/esp.git --username <user> --password <token>`)。
5. **验证 sync**:`argocd app get esp-apps` 看同步状态为 Synced + Healthy;Web UI 确认 3 个子 Application 均绿。
6. **占位替换**:全文搜索 `<ORG>` 替换为实际 GitHub org/user;确认 targetRevision 与实际默认分支一致。

## 6. 与现有 CI 的关系

职责分离,互不替代:

- **CI(security.yml)**:跑单元测试 + 安全扫描(Trivy/Gitleaks/semgrep),产出镜像,推 registry。**不直接 kubectl apply**。
- **CD(GitOps/ArgoCD)**:watch git 仓库 manifest 变更(含镜像 tag 更新),自动 sync 到集群。

镜像更新链路:CI 构建并推镜像 → 更新 manifest 中 image tag(可加 step 或人工 PR)→ ArgoCD 检测到 git 变更 → sync。这样部署动作有 git commit 审计,回滚即 `git revert`。

## 7. 风险与缓解

| 风险                          | 缓解                                                                 |
|-------------------------------|----------------------------------------------------------------------|
| selfHeal 覆盖手动 kubectl 调试 | 开发期可在 Application 临时关 selfHeal(annotation 或 spec 改 false)|
| Secret 管理风险(明文入库)   | AppProject 已 `namespaceResourceBlacklist` 拒绝 Secret;建议后续上 sealed-secrets 或 external-secret,本轮占位 |
| 单节点 Rancher 资源紧张        | ArgoCD 控制面轻量(数百 MB 内存),单节点可承载;多节点迁移时随集群走 |
| 仓库未迁入前无法实际 sync     | manifest/ADR 已就位,repoURL 占位;用户提供 URL 后替换即可          |
| prune 误删非托管资源          | Application path 精确限定目录 + include/exclude,避免误纳管         |

## 8. 占位项清单(用户需填)

- `https://github.com/<ORG>/esp.git` 中的 `<ORG>`:出现在 argocd-project.yaml(1 处)、argocd-apps.yaml(4 处:root + 3 子 app)。
- `targetRevision: main`:按实际默认分支名调整(若用 master 或 release/* 需改)。
- repo credential:GitHub token / SSH deploy key,在 ArgoCD 中配置(非 manifest 内)。
