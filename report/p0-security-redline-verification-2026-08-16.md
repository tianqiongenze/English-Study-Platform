# P0 安全红线第一波 — 端到端验证报告

> 日期:2026-08-16。范围:架构评审 5.9/10 指出的 P0 安全红线 6 项(私钥收敛/learning鉴权/IDOR/role锁/NodePort/NetworkPolicy)。
> 验证方式:实集群命令取证(kubectl exec + wget/curl),所有断言给出原始响应片段。全部 6 项 PASS。
> 配套代码改动:git b89a517(会话22-21)、71176af(会话22-26)。

## 结论速览

| 项 | 断言 | 结果 | 证据 |
|---|---|---|---|
| P0-1 私钥收敛 | 私钥仅 bff;learning/gateway 仅公钥;旧 secret 已删 | ✅ | §1 |
| P0-2 learning 鉴权 | 无 token → 401;带 token → 200 | ✅ | §2 |
| P0-3 IDOR 越权 | user_id 覆写被忽略;learner→teacher 403 | ✅ | §3 |
| P0-4 register 锁 role | role=admin → 强制 learner | ✅ | §4 |
| P0-5 NodePort 收敛 | 业务服务全 ClusterIP | ✅ | §5 |
| P0-6 NetworkPolicy 收窄 | 精确白名单(同ns/kube-system/NodePort端口) | ✅ | §6 |

---

## 1. P0-1 私钥收敛(签发私钥不再扩散)

**断言**:`JWT_RSA_PRIVATE_KEY_PEM` 仅存在于 bff-java;learning-java/gateway-go 为空;旧单 secret `esp-rsa-jwt` 已删除。

实测:
```
kubectl get secret esp-rsa-jwt-sign esp-rsa-jwt-verify
  esp-rsa-jwt-sign     Opaque   2  (private+public)   # 仅 bff envFrom
  esp-rsa-jwt-verify   Opaque   1  (public only)      # learning/gateway/worker/teacher envFrom
kubectl get secret esp-rsa-jwt → NotFound(已删)

pod env 实测:
  learning-java: PRIV=[](空)  PUB=[SET]   ✅
  gateway-go:    PRIV=[](空)  PUB=[SET]   ✅
  bff-java:      PRIV=[SET]   PUB=[SET]   ✅(唯一签发方)
```

**结论**:签发私钥收敛至 bff 单服务,任一其他服务 RCE 无法再伪造 admin JWT。原方案里 5 服务共持私钥的扩散面已消除。

---

## 2. P0-2 learning-java 接鉴权

**断言**:无 token 访问 `/api/learning/review` → 401;带合法 learner token → 200。

实测:
```
无 token:
  wget http://learning-java:80/api/learning/review
  → HTTP/1.1 401 Unauthorized            ✅

带 token(attacker 注册所得 learner token):
  wget -H "Authorization: Bearer <token>" http://learning-java:80/api/learning/review
  → 200 []  (新用户无到期复习项,返回空列表)   ✅
```

**结论**:learning 不再信任 `X-User-Id` header,鉴权走 JWT。白名单(actuator/healthz)与业务路径区分正常。

---

## 3. P0-3 IDOR 越权修复

**断言**:① gateway sessions/history 的 `?user_id=` 覆写不再生效(用户ID只能来自 JWT);② learner token 访问 teacher `/{userId}/kp-stats` → 403。

实测(attacker_id=1201748677655330818,victim_id=1201748631401070594):
```
① attacker token + ?user_id=victim_id:
   GET /api/sessions/history?user_id=1201748631401070594
   → {"sessions":[],"user_id":1201748677655330818}   ✅ 返回 attacker 自己的 id,覆写被忽略

② learner token → teacher kp-stats(victim 路径参数):
   GET /api/teacher/1201748631401070594/kp-stats
   → HTTP/1.1 403 Forbidden               ✅

   无 token → teacher:
   → HTTP/1.1 401 Unauthorized            ✅
```

**结论**:查询参数 user_id 不再是信任边界;路径参数越权被角色/归属校验拒绝。

---

## 4. P0-4 register 锁 role

**断言**:`POST /api/auth/register` 带 `"role":"admin"` → 服务端强制返回 learner。

实测:
```
POST /api/auth/register  {"email":"p0verify...","password":"Test1234!","name":"p0verify","role":"admin"}
→ {"user":{...,"role":"learner",...},"token":"eyJ...role:learner..."}   ✅ 请求体 role 被忽略,强制 learner
```

**结论**:自注册无法提权 teacher/admin。teacher/admin 仅后台运营开通(管理接口)。

---

## 5. P0-5 NodePort 收敛

**断言**:业务服务(learning/teacher/writing/emotion/writing-coach)全部 ClusterIP,无 NodePort;仅保留对外必要端口。

实测 `kubectl get svc`:
```
learning-java   ClusterIP  (无 NodePort)   ✅
teacher-java    ClusterIP  (无 NodePort)   ✅
writing-py      ClusterIP  (无 NodePort)   ✅
emotion-svc     ClusterIP  (无 NodePort)   ✅
writing-coach   ClusterIP  (无 NodePort)   ✅

保留(业务必需对外):
  bff-java-np   8090:30009  9090:30010
  gateway-go-np 80:30080
  frontend-react-np 80:30090  frontend-h5-np 80:30091  frontend-portal-np 80:30070
```

**结论**:业务服务不再对公网裸奔;统一由 traefik ingress(或 bff/gateway)暴露。

---

## 6. P0-6 NetworkPolicy 收窄

**断言**:`esp-allow-internal` 从"放行所有 namespace"(坑㉕ 修复遗留)收窄为精确白名单。

实测 policy spec(`kubectl get networkpolicy esp-allow-internal -o yaml`):
```
ingress 规则:
  1. podSelector: {}            → esp-prod 同 namespace 任意 pod(服务间通信)
  2. namespaceSelector: kube-system, ports: 80/8090/9090   → traefik ingress
  3. ipBlock: 0.0.0.0/0, ports: 80/8080/8090/9090/8848/8858/7091/8091
     → 仅放行 NodePort DNAT 目标端口(gateway 80/bff 8090-9090/nacos 8848/sentinel 8858/seata 7091-8091)
policyTypes: [Ingress]
```

**结论**:跨 namespace 的 pod(如 edu 项目)不再能访问 esp-prod 任意端口;仅 traefik(kube-system)与 NodePort 外部入口白名单可达。坑㉕ 的"全放行"已闭合。

---

## 7. 回归(正向路径未被破坏)

| 场景 | 结果 |
|---|---|
| gateway `/api/scenarios` 带 token | 200,返回 scenarios 数据 ✅ |
| learning `/api/learning/review` 带 token | 200 `[]`(新用户)✅ |
| bff register 完整流程 | 200,签发合法 JWT(role=learner)✅ |

## 8. 遗留与后续

- teacher 内部"教师查班级内学生"的归属放行逻辑需按角色细化(当前 403 是严格拒绝,后续管理接口补充)。
- P0 各单项在重构计划(indexed-honking-spring.md)中的实现 commit:b89a517(会话22-21)+ 71176af(会话22-26)。
- 本报告证据全部取自实集群命令,无模拟。
