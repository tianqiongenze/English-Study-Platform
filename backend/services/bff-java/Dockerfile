# BFF Java: prebuilt fat-jar, JRE 26 (升级 JDK26;旧基底 #FROM eclipse-temurin:21-jre-alpine)
FROM esp/jdk26-jre:debian-slim
WORKDIR /app
COPY target/bff-java-1.0.0.jar /app/app.jar
COPY otel-agent.jar /app/otel-agent.jar

EXPOSE 8090 9090 50090

ENV OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector.esp-observability.svc.cluster.local:4317
ENV OTEL_EXPORTER_OTLP_PROTOCOL=grpc
ENV OTEL_SERVICE_NAME=bff-java
ENV OTEL_TRACES_EXPORTER=otlp
ENV OTEL_RESOURCE_ATTRIBUTES=service.namespace=esp-prod,deployment.environment=esp-prod

# JDK26 优化: G1GC(启动期比ZGC省,HPA扩容抢ready优先)+TieredStopAtLevel=1(C1-only跳C2峰值,短生命周期pod适配)+Xshare:auto(默认CDS省引导类加载)+容器感知+虚拟线程+String去重+内存预触
# 注: JDK26 与 otel-javaagent 不兼容,已移除 -javaagent 参数
# P1-D21(2026-08-21,k6压测bff 151s冷启动+OOM根治):真正大头是Nacos gRPC 3s超时反复重试(VM高负载时gRPC慢超3s),
# ASV查证(nacos-client-3.1.1 DefaultGrpcClientConfig.java:213)property真名 nacos.remote.client.grpc.timeout 默认3000ms,
# 经JVM系统属性-D被NacosClientProperties的SourceType.JVM层读取注入,SCA不暴露此key,故在此Dockerfile直接-D覆盖到8s。
ENV JAVA_OPTS="-XX:+UseG1GC \
  -XX:TieredStopAtLevel=1 \
  -Xshare:auto \
  -XX:+UseContainerSupport -XX:ActiveProcessorCount=0 \
  -XX:MaxRAMPercentage=75.0 \
  -XX:+UseStringDeduplication \
  -XX:+AlwaysPreTouch \
  -Dspring.threads.virtual.enabled=true \
  -Dnacos.remote.client.grpc.timeout=8000 \
  -Dnacos.remote.client.grpc.server.check.timeout=8000"
ENTRYPOINT ["sh", "-c", "exec java $JAVA_OPTS -jar /app/app.jar"]
