# writing-coach: Litestar + Granian (ADR-09). build context = 项目根目录。
FROM python:3.13-slim
WORKDIR /app
RUN pip config set global.index-url https://pypi.tuna.tsinghua.edu.cn/simple
COPY services/esp-cache-py ./esp-cache-py
RUN pip install --no-cache-dir ./esp-cache-py
COPY services/esp-db-py ./esp-db-py
RUN pip install --no-cache-dir ./esp-db-py
COPY services/writing-coach/pyproject.toml ./
RUN pip install --no-cache-dir --retries 5 --timeout 60 . \
    && pip install --no-cache-dir --retries 5 --timeout 60 pytest httpx
COPY services/writing-coach/app ./app
EXPOSE 8085
ENV PORT=8085
ENV OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector.esp-observability.svc.cluster.local:4317
ENV OTEL_EXPORTER_OTLP_PROTOCOL=grpc
ENV OTEL_SERVICE_NAME=writing-coach
ENV OTEL_RESOURCE_ATTRIBUTES=service.namespace=esp-prod,deployment.environment=esp-prod
CMD ["granian", "app.main:app", "--interface", "asgi", "--host", "0.0.0.0", "--port", "8085", "--workers", "2"]
# 安全:非 root 运行(ADR-security-scanning §4)
RUN groupadd -r app && useradd -r -g app app && chown -R app:app /app
USER app
