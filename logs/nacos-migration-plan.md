# Nacos Migration: esp-prod -> platform-infra (shared)

## 当前状态 (2026-08-17)

### esp-prod/nacos
- **Deploy**: `nacos` (1/1 Running, 22h)
- **Service**: ClusterIP 10.43.52.92 (ports 8848/9848/9849)
- **已注册服务 (4个)**:
  | 服务名 | Group | 注册方式 | NACOS地址 |
  |--------|-------|---------|-----------|
  | learning-java | esp | K8s auto-injected NACOS_HOST=nacos | 10.43.52.92 |
  | seata-server | SEATA_GROUP | K8s auto-injected NACOS_HOST=nacos | 10.43.52.92 |
  | bff-java | DEFAULT_GROUP | NACOS_ADDR=nacos:8848 | 10.43.52.92 |
  | providers:com.esp.api.dubbo.LearningRpcService:1.0.0:esp | esp | (learning-java发布) | 10.43.52.92 |

- **配置 (Phase A后)**:
  | dataId | group | 内容 |
  |--------|-------|------|
  | esp-bff-routes | DEFAULT_GROUP | BFF路由表 (10条规则) |

- **writing-coach**: 有NACOS_HOST环境变量但未注册为服务(可能未开启服务发现)

### ekb-m2-aws-us-east-1/nacos
- **Deploy**: `nacos` (1/1 Running, 24h)
- **Service**: ClusterIP 10.43.206.253 (ports 8848/9848/9849)
- **NodePort**: nacos-np 10.43.204.106 (8848:30048)
- **已注册服务**: 0
- **配置**: 0

### platform-infra/nacos
- **Service**: ExternalName -> nacos.ekb-m2-aws-us-east-1.svc.cluster.local
- 作为共享入口，指向 ekb-m2 的 nacos

---

## Phase A (已完成): 创建 BFF 路由配置

```bash
kubectl -n esp-prod exec deploy/nacos -- curl -s -X POST \
  "http://127.0.0.1:8848/nacos/v1/cs/configs" \
  -d "dataId=esp-bff-routes" \
  -d "group=DEFAULT_GROUP" \
  -d "type=json" \
  --data-urlencode 'content=...'
```
**结果**: true (配置已写入)

---

## Phase B (待执行): 数据迁移 + 服务切换

### B1: 导出 esp-prod/nacos 全部数据
```bash
# 配置
kubectl -n esp-prod exec deploy/nacos -- curl -s \
  "http://127.0.0.1:8848/nacos/v1/cs/configs?dataId=&group=&pageNo=1&pageSize=100" > nacos-backup-configs.json

# 服务实例(逐个)
kubectl -n esp-prod exec deploy/nacos -- curl -s \
  "http://127.0.0.1:8848/nacos/v1/ns/catalog/instances?serviceName=learning-java&groupName=esp&pageNo=1&pageSize=100" > nacos-backup-instances.json
```

### B2: 导入到 ekb-m2/nacos
```bash
# 创建配置
kubectl -n ekb-m2-aws-us-east-1 exec deploy/nacos -- curl -s -X POST \
  "http://127.0.0.1:8848/nacos/v1/cs/configs" \
  -d "dataId=esp-bff-routes" \
  -d "group=DEFAULT_GROUP" \
  -d "type=json" \
  --data-urlencode 'content=<从备份读取>'
```

### B3: 修改服务 NACOS 指向 (逐个 rollout)
需要修改的服务及环境变量：

| 服务 | 当前值 | 目标值 |
|------|--------|--------|
| bff-java | NACOS_ADDR=nacos:8848 | NACOS_ADDR=nacos.platform-infra:8848 |
| learning-java | NACOS_HOST=nacos (auto) | NACOS_HOST=nacos.platform-infra |
| seata | NACOS_HOST=nacos (auto) | NACOS_HOST=nacos.platform-infra |
| writing-coach | NACOS_HOST=nacos (auto) | NACOS_HOST=nacos.platform-infra |

注意: K8s auto-injected的环境变量无法直接修改，需要在 Deployment spec 中显式设置 env 或修改 Service 名称。

**方案**: 在 esp-prod 中创建一个指向 ekb-m2 nacos 的 ExternalName Service，替换原 nacos Service:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: nacos
  namespace: esp-prod
spec:
  type: ExternalName
  externalName: nacos.ekb-m2-aws-us-east-1.svc.cluster.local
```
这样所有使用 `nacos:8848` 的服务无需修改配置即可切换到 ekb-m2。

### B4: 逐个 rollout restart + 验证注册
```bash
kubectl -n esp-prod rollout restart deploy/bff-java
kubectl -n esp-prod rollout restart deploy/learning-java
kubectl -n esp-prod rollout restart deploy/seata
kubectl -n esp-prod rollout restart deploy/writing-coach

# 验证 ekb-m2/nacos 有全部服务
kubectl -n ekb-m2-aws-us-east-1 exec deploy/nacos -- curl -s \
  "http://127.0.0.1:8848/nacos/v1/ns/catalog/services?pageNo=1&pageSize=100"
```

### B5: 确认 ekb-m2/nacos 健康后 scale esp-prod/nacos 到0
```bash
kubectl -n esp-prod scale deploy/nacos --replicas=0
# 观察所有服务是否正常（无断连）
kubectl -n esp-prod get pods
```

---

## Phase C (最终清理)

### C1: 删除 esp-prod 的 nacos 资源
```bash
kubectl -n esp-prod delete svc nacos-external  # 删除旧ClusterIP（如果已替换）
kubectl -n esp-prod delete deploy nacos
# 保留 ExternalName 类型的 nacos Service 作为共享入口
```

### C2: 验证 platform-infra/nacos ExternalName
```bash
kubectl -n platform-infra get svc nacos -o yaml
# 确认 externalName: nacos.ekb-m2-aws-us-east-1.svc.cluster.local
```

### C3: 验证全局连通性
```bash
# 从 esp-prod pod 测试
kubectl -n esp-prod exec deploy/bff-java -- curl -s "http://nacos.platform-infra:8848/nacos/v1/ns/catalog/services?pageNo=1&pageSize=100"
```

---

## 风险与回滚

| 风险 | 影响 | 回滚 |
|------|------|------|
| 服务切换期间短暂断连 | 2-3秒服务发现不可用 | 恢复 esp-prod/nacos replicas=1 |
| ekb-m2/nacos 崩溃 | 所有依赖服务发现的服务异常 | scale esp-prod/nacos replicas=1, 修改回 NACOS_HOST=nacos |
| Seata与Nacos连接异常 | 分布式事务失败 | 回滚Seata到旧Nacos,检查事务日志 |
