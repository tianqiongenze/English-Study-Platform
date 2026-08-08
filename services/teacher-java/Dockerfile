# teacher-java: Spring Boot 3 fat-jar, JRE 21
FROM eclipse-temurin:21-jre-noble
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

ENV JAVA_OPTS="-javaagent:/app/otel-agent.jar -XX:MaxRAMPercentage=75.0 -XX:+UseG1GC"
ENTRYPOINT ["sh", "-c", "exec java $JAVA_OPTS -jar /app/app.jar"]
