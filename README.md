# English-Study-Platform

> AI 英语教育平台(Home-Tutor)— 软件工程化全流程实现:TDD + 多语言微服务 + 读写分离 + Disruptor + TiDB + Pulsar + K8s,支撑 5000 CCU。

## 仓库结构
```
docs/design/        需求(SRS)/架构(SAD)/详细设计(SDD)/schema
docs/api/           API 契约
services/
  gateway-go/       实时口语陪练网关(Go + WebSocket)
  learning-java/    自适应学习(Java + Spring Boot 3 + Disruptor + 读写分离)
  writing-py/       写作批改(Python + FastAPI + Claude)
  teacher-java/     教师端(Java + 读写分离 + TiFlash)
  emotion-svc/      情感化多模态(Python)
frontend-react/     React 前端骨架
deploy/
  docker/           docker-compose 本地全栈(含 TiDB 集群)
  k8s/              生产 K8s manifest(Deployment/Service/HPA/Ingress)
  locust/           5000 CCU 压测脚本
```

## 各服务构建与测试
```bash
# Go 网关
cd services/gateway-go && go test ./... && go build -o gateway-go ./src

# Java learning
cd services/learning-java && mvn test

# Java teacher
cd services/teacher-java && mvn test

# Python writing
cd services/writing-py && pip install -e .[test] && pytest

# Python emotion
cd services/emotion-svc && pip install -e .[test] && pytest
```

## 本地全栈启动
```bash
cd deploy/docker && docker compose up -d
# 前端
cd frontend-react && npm i && npm run dev
```

## K8s 生产部署
```bash
kubectl apply -f deploy/k8s/
# 基础设施(TiDB/Redis Cluster/Pulsar)见 deploy/k8s/README-infra.md
```

## 压测(5000 CCU)
```bash
locust -f deploy/locust/locustfile.py --headless -u 5000 -r 100 --run-time 5m --host http://localhost:8080
```

详见 `docs/design/` 下三份设计文档。
