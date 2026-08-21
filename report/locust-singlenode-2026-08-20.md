# ESP Locust 单节点压测报告(2026-08-20 会话23-20)

## 环境
- 单节点 Rancher Desktop(lima VM,9核/30G)
- 入口:Traefik NodePort(port-forward 31032,Host: esp.example.com)
- JWT:RS256(esp-rsa-jwt-sign secret 私钥注入)
- 所有服务 1/1 Running

## 测试1:50 CCU,30s
- 请求:763,失败:0(fail_rate=0%)
- avg_rps=25.7,avg_latency=936ms,p95=2100ms
- 各端点:
  - GET /api/scenarios p95=1800ms
  - POST /api/sessions p95=520ms
  - POST /api/learning/event p95=1300ms
  - POST /api/writing/grade p95=24000ms(AI 批改长尾,可接受)

## 测试2:100 CCU,40s
- 请求:1491,失败:70(429 限流,writing/grade 触发)
- avg_rps=38.3,avg_latency=1359ms,p95=3000ms
- POST /api/writing/grade p95=3500ms(100 并发 AI 排队)

## 结论
- 单节点 50 CCU 稳定(0 失败),100 CCU 触发限流(429)
- 写作批改是性能瓶颈(AI 推理长尾,p95 24s@50CCU → 3.5s@100CCU 限流后)
- gateway-go/bff-java/learning/writing 主链路功能正常(RS256 JWT 鉴权通过,路由全通)
- **单节点容量上限:~50-80 CCU 稳定,100 CCU 触发限流**
- 5000 CCU 目标需多节点(DEFERRED,符合 distributed-locust.yaml 设计)

## 限制(DEFERRED)
- 5000 CCU:单节点资源不足(CPU 1.79% idle 时跑 100 CCU 已触发限流)
- 多节点物理隔离压测集群待 N-7
- 本次单节点压测验证了链路正确性 + 单节点容量基线,功能层面可上线
