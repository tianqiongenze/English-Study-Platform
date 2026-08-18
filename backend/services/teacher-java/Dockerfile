# teacher-java: Spring Boot fat-jar, JRE 26
# 升级 JDK26(旧基底 #FROM eclipse-temurin:21-jre-alpine)
FROM esp/jdk26-jre:debian-slim
WORKDIR /app
COPY target/teacher-java-1.0.0.jar /app/app.jar

# OpenTelemetry Java agent (v2.10.0) for auto-instrumentation
COPY otel-agent.jar /app/otel-agent.jar

EXPOSE 8083

# OTEL export config -> Collector in esp-observability ns (OTLP gRPC 4317)
ENV OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector.esp-observability.svc.cluster.local:4317
ENV OTEL_EXPORTER_OTLP_PROTOCOL=grpc
ENV OTEL_SERVICE_NAME=teacher-java
ENV OTEL_TRACES_EXPORTER=otlp
ENV OTEL_RESOURCE_ATTRIBUTES=service.namespace=esp-prod,deployment.environment=esp-prod

# JDK26 优化: ZGC(低延迟<1ms)+容器感知+虚拟线程+String去重+内存预触
# 注: JDK26 与 otel-javaagent 不兼容,已移除 -javaagent 参数
ENV JAVA_OPTS="-XX:+UseZGC \
  -XX:+UseContainerSupport -XX:ActiveProcessorCount=0 \
  -XX:MaxRAMPercentage=75.0 \
  -XX:+UseStringDeduplication \
  -XX:+AlwaysPreTouch \
  -Dspring.threads.virtual.enabled=true"
ENTRYPOINT ["sh", "-c", "exec java $JAVA_OPTS -jar /app/app.jar"]
