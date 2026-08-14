# emotion-svc: Litestar + Granian (ADR-09)
# build context = 项目根目录(esp-cache-py 在 services/esp-cache-py,作为公共多级缓存包安装)
# 基镜像用 python:3.13-slim(默认即 Debian12 bookworm,等价 3.13-slim-bookworm 但本环境 docker.1ms.run 镜像源对 -bookworm tag 返回 403,3.13-slim 可正常解析)
FROM python:3.13-slim
# 安全基线升级:Debian12 bookworm perl-base 已修(对比 trixie 5.40.1-6 的 4 CRIT)。apt upgrade 补 util-linux/ncurses 等
# 注:deb.debian.org 在本环境不可达,改用 USTC 镜像
RUN sed -i 's|http://deb.debian.org/debian|http://mirrors.ustc.edu.cn/debian|g; s|http://deb.debian.org/debian-security|http://mirrors.ustc.edu.cn/debian-security|g' /etc/apt/sources.list.d/debian.sources 2>/dev/null \
    || sed -i 's|http://deb.debian.org/debian|http://mirrors.ustc.edu.cn/debian|g; s|http://deb.debian.org/debian-security|http://mirrors.ustc.edu.cn/debian-security|g' /etc/apt/sources.list \
    ; apt-get update && apt-get upgrade -y && rm -rf /var/lib/apt/lists/*
WORKDIR /app
# 提前设置 pip 镜像源(单节点环境 PyPI 直连慢/不稳,统一走清华源)
RUN pip config set global.index-url https://pypi.tuna.tsinghua.edu.cn/simple
# Python 3.12+ slim 镜像默认不带 setuptools/wheel(ensurepip 已移除),esp-cache-py 的 PEP 517 构建需要,预装
RUN pip install --no-cache-dir --retries 5 --timeout 60 setuptools>=68 wheel
COPY shared/esp-cache-py ./esp-cache-py
RUN pip install --no-cache-dir ./esp-cache-py
# 装公共读写分离数据访问包 esp-db-py(主写 tidb-primary / 从读 tidb-follower)
COPY shared/esp-db-py ./esp-db-py
RUN pip install --no-cache-dir ./esp-db-py
COPY services/emotion-svc/pyproject.toml ./
RUN pip install --no-cache-dir --retries 5 --timeout 60 . \
    && pip install --no-cache-dir --retries 5 --timeout 60 pytest httpx
COPY services/emotion-svc/app ./app
COPY services/emotion-svc/tests ./tests
EXPOSE 8084
# OpenTelemetry → OTel Collector (esp-observability ns, OTLP gRPC 4317)
ENV OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector.esp-observability.svc.cluster.local:4317
ENV OTEL_EXPORTER_OTLP_PROTOCOL=grpc
ENV OTEL_SERVICE_NAME=emotion-svc
ENV OTEL_RESOURCE_ATTRIBUTES=service.namespace=esp-prod,deployment.environment=esp-prod
# 安全:非 root 运行(ADR-security-scanning §4);USER 须在 chown 之后、CMD 之前
RUN groupadd -r app && useradd -r -g app app && chown -R app:app /app
USER app
CMD ["granian", "app.asgi:app", "--interface", "asgi", "--host", "0.0.0.0", "--port", "8084", "--workers", "2"]
