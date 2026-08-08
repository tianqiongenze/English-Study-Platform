# ESP 前后端连调功能测试(接口 e2e)

> 覆盖"前端真实调用后端"的接口级端到端契约:鉴权链路 + 口语场景(gateway-go)+ 下游业务(learning/writing/emotion/teacher)。
> 对应 docs/test/07 人工测试的"自动化版"——07 是人工点,e2e 是 CI 可跑的接口断言。

## 两份实现
1. **Newman collection**(标准):`esp-e2e.postman_collection.json` + `esp-env.json`。有 Node/newman 环境时用。
2. **Python 等价实现**:`esp_e2e.py`。集群内无 Node 时用(stdlib urllib,零依赖),断言语义与 collection 一致。

## 运行(集群内,经 ClusterIP 直连)
```bash
POD=$(kubectl -n esp-prod get pod -l app=writing-py -o jsonpath='{.items[0].metadata.name}')
kubectl -n esp-prod cp deploy/newman/esp_e2e.py "$POD:/tmp/esp_e2e.py"
kubectl -n esp-prod exec "$POD" -- python3 /tmp/esp_e2e.py
```

## 运行(本机 Mac,经 NodePort)
需把 BASE 改为 `http://localhost:30080`(gateway NodePort),bff/下游经 gateway 路由或各自 NodePort。
小程序端需微信开发者工具 + 合法域名配置。

## 用例矩阵(19 项)

| 分组 | 用例 | 断言 |
|---|---|---|
| 健康检查 | HC gateway/writing | 200 |
| 鉴权链路 | AUTH-1 wx-login 签发 JWT | 200 + token + role=learner |
| | AUTH-2 无 token | 401/403 |
| | AUTH-3 伪造 X-User-Id 无 token | 401/403(头不生效) |
| 口语场景 | SPK-1 场景列表 | 200 + 非空数组 + code 字段 |
| | SPK-2 创建会话 | 200 + session_id + scenario |
| | SPK-3 WS 越权(他人 token) | 403/401 |
| 下游业务 | LRN-1 学情上报 / LRN-2 复习 | 非 5xx |
| | WRT-1 写作批改 / EMO-1 情绪感知 / TCH-1 知识点统计 | 非 5xx(服务可达) |

## 2026-08-03 实测结果
**19/19 通过**。鉴权(JWT 签发/无 token 拒/伪造头拒)+ 口语(场景/会话/WS 越权)+ 下游 4 服务全部连通。

## 落盘执行报告

两版实现(Python `esp_e2e.py` + bash `run-e2e-host.sh`)均把每次执行结果落盘到 `report/e2e/`(可用环境变量 `REPORT_DIR` 覆盖)。无论 pass/fail 都写,便于 CI 消费与历史回溯。

### 产物

| 版本 | 产物 | 说明 |
|---|---|---|
| Python | `esp-e2e-{ts}.json` | 结构化结果:run_ts/total/passed/failed/results[] |
| Python | `esp-e2e-{ts}.junit.xml` | JUnit XML,每个用例一个 `<testcase>`,失败带 `<failure>` |
| Python | `esp-e2e-latest.json` / `esp-e2e-latest.junit.xml` | latest 覆盖(cp,非 symlink) |
| bash | `esp-e2e-host-{ts}.txt` | 人类可读:逐用例结果 + 汇总 |
| bash | `esp-e2e-host-{ts}.junit.xml` | JUnit XML(sed 转义 `& < >`) |
| bash | `esp-e2e-host-latest.txt` / `esp-e2e-host-latest.junit.xml` | latest 覆盖 |

- 时间戳 `ts` 用 UTC:`%Y%m%dT%H%M%SZ`(bash)/ `%Y-%m-%dT%H:%M:%S`(Python,文件名去 `:` `-`)。
- Python 版 XML 转义用 `xml.sax.saxutils.escape`;bash 版用 `sed` 转义 `& < >`。
- 退出码:两版均 `0 if failed==0 else 1`,CI 可直接据退出码判定。

### CI 消费

- JUnit XML 可被 Jenkins/GitHub Actions/GitLab CI 的测试报告插件直接解析(artifact 上传 `report/e2e/*.junit.xml`)。
- `latest.*` 供仪表盘/门户固定链接展示最近一次结果。
- Python 版 `--json` 仍向 stdout 打印结构化 JSON(标记 `__JSON__`),便于管道下游解析;落盘独立于 `--json`,默认就写。

### 运行示例

```bash
# Python 版(集群内,默认落盘 report/e2e)
kubectl -n esp-prod exec deploy/writing-py -- python3 /tmp/esp_e2e.py
# 自定义目录
kubectl -n esp-prod exec deploy/writing-py -- python3 /tmp/esp_e2e.py --report-dir /tmp/e2e-reports

# bash 版(本机 host ingress)
REPORT_DIR=report/e2e bash deploy/newman/run-e2e-host.sh
```

运行结束终端会 echo 落盘路径清单,如:
```
[落盘报告]
  report/e2e/esp-e2e-20260804T120000Z.json
  report/e2e/esp-e2e-20260804T120000Z.junit.xml
  report/e2e/esp-e2e-latest.json
  report/e2e/esp-e2e-latest.junit.xml
```

## 说明
- 下游业务用例断言"非 5xx / 服务可达",因部分依赖 LLM/DB 数据(writing 依赖 LLM,单节点可能超时)。完整业务正确性由 docs/test/02 集成测试(INT-1~4)+ 07 人工测试覆盖。
- WS 越权用 HTTP 升级握手探测(验证 403 拒绝),完整 WS 帧级由 locust ccu5000_ws.py 覆盖。
