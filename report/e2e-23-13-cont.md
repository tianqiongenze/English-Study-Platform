# E2E 全链路冒烟测试报告 (会话23-13-cont)

> 方法论:verification-before-completion(每步附证据:HTTP code/响应体/RC,失败定位根因不粉饰)
> 时间:2026-08-18
> 环境:K8s rancher-desktop ns esp-prod,30 pod Running,CRDB 3/3 stable

## 最终结果:14 通过 / 0 失败 —— ALL GREEN

| # | 测试项 | HTTP | 耗时(s) | 结果 |
|---|--------|------|---------|------|
| 1 | gateway-health `/healthz` (GW:30080) | 200 | 0.09 | PASS |
| 2 | register `/api/auth/register` | 200 | 0.77-1.02 | PASS |
| 3 | login `/api/auth/login` | 200 | 0.64 | PASS |
| 4 | courses `/api/content/courses` | 200 | - | PASS |
| 5 | circles `/api/community/circles` | 200 | - | PASS |
| 6 | rankings `/api/gamification/rankings?type=xp&scope=global` | 200 | - | PASS |
| 7 | digital-human `/api/gamification/me/digital-human` | 200 | - | PASS |
| 8 | learning-review `/api/learning/review` | 200 | - | PASS |
| 9 | learning-recommend `/api/learning/recommend?user_id=<uid>` | 200 | - | PASS |
| 10 | teacher `/api/teacher/<uid>/kp-stats` | 200 | 7.39 | PASS(偏慢) |
| 11 | writing `/api/writing/grade` (POST) | 200 | 7.68 | PASS(偏慢) |
| 12 | emotion `/api/emotion/recent/<uid>?limit=1` | 200 | 4.62 | PASS |
| 13 | vocab `/api/vocab/due?uid=<uid>` | 200 | 1.82 | PASS |
| 14 | admin `http://127.0.0.1:30099/healthz` | 200 | 0.02 | PASS |

## 执行过程与障碍清除

### 障碍1:SSH tunnel 转发失效 → 改用 kubectl port-forward
- 现象:本地 `curl localhost:30009/api/auth/login` 超时(http=000, time=15s),但集群内 `kubectl exec gateway-go -- wget bff-java:8090/api/auth/login` 返回 401(可达)
- 根因:宿主原有 SSH tunnel(ssh PID 68723/68719)虽在 LISTEN,但转发到 lima VM NodePort 的链路在 bff-java 滚动卡死状态下路由异常
- 修复:kill 旧 SSH tunnel,改用 kubectl port-forward:
  - `kubectl -n esp-prod port-forward svc/bff-java-np 30009:8090`
  - `kubectl -n esp-prod port-forward svc/gateway-go-np 30080:80`
  - `kubectl -n esp-prod port-forward svc/esp-admin-np 30099:80`
- 修复后验证:BFF readiness UP(0.48s)、GW healthz 200(0.09s)、admin healthz 200(0.02s)

### 障碍2:E2E 脚本自身 bug(3 处,已修复)
1. **`((PASS++))` 与 `set -e` 冲突**:`((0++))` 后缀自增在初始值 0 时返回退出码 1,触发 `set -e` 在 gateway-health 后立即退出。改为 `PASS=$((PASS+1))`。
2. **AUTH 头构造错误**:`AUTH="-H 'Authorization: Bearer $TOKEN'"` 单引号在值内,curl 收到字面单引号,header 值变成 `'Bearer xxx'` 导致 401,且 `-o /dev/null -w '%{http_code}'` 拼出 `401000000`。改为 bash 数组 `AUTH=(-H "Authorization: Bearer $TOKEN")`,所有调用处用 `"${AUTH[@]}"`。
3. **userId 提取路径错误**:登录响应是嵌套 `{"user":{"id":...}}`,脚本取顶层 `userId` 得空。改为 `json.load(...).get('user',{}).get('id','')`。
4. **vocab 接口缺 uid 参数**:接口要 `?uid=<id>`,脚本只传 `/api/vocab/due` 得 400 `invalid uid`。补 `?uid=${USERID:-0}`。

### 障碍3:bff-java 滚动更新卡死(未修,旧 pod 承载流量)
- 现象:Deployment spec image=`esp/bff-java:1.10.0`(回滚目标),但 1.10.0 镜像有 Dubbo 元数据写权限 bug,新 RS `5c57d7cd84` 3 pod 全部 Startup probe failed:
  ```
  java.lang.IllegalArgumentException: Invalid service store file /.dubbo/dubbo-metadata-bff-java-nacos-8848.cache, cause: Failed to create directory /.dubbo!
  ```
- 旧 RS `7c9c4fb8c5`(image 1.14.4)2 pod 1/1 healthy,承载流量(集群内 readiness UP)
- 影响:BFF 业务可用但非全副本健康;teacher→learning Dubbo RPC 偶发 DEADLINE_EXCEEDED(10s 超时),导致 teacher 接口偶发 503(本次 E2E 未触发,但单测时出现过)
- DEFERRED:解除滚动卡死(将 Deployment image 推进到可用的 1.14.4 或更高)属于集群修复范畴,超出 E2E 冒烟任务边界;记录待后续处理

## 关联文件
- 脚本(已修 3 处 bug):`/Users/joelgong/claude-Projects/English-Study-Platform/scripts/phase3-e2e-smoke.sh`
- 原始日志:`/Users/joelgong/claude-Projects/English-Study-Platform/logs/e2e-23-13.log`
