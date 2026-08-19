FROM debian:bookworm-slim
# Oracle OpenJDK 26.0.2 JRE(glibc,huaweicloud 镜像源拉取,绕过 adoptium 403)
# tarball 需随 build context 一起 COPY(宿主 /tmp/jdk26-aarch64-glibc.tar.gz)
RUN apt-get update && apt-get install -y --no-install-recommends curl ca-certificates libfreetype6 && rm -rf /var/lib/apt/lists/*
COPY jdk26-aarch64-glibc.tar.gz /tmp/jdk26.tar.gz
RUN mkdir -p /opt/java && tar xzf /tmp/jdk26.tar.gz -C /opt/java --strip-components=1 && rm /tmp/jdk26.tar.gz
ENV JAVA_HOME=/opt/java
ENV PATH=$JAVA_HOME/bin:$PATH
RUN java -version 2>&1 | grep 'version "26'
# RWP铁律根因根治(第①层):Dubbo3.3.6写${HOME}/.dubbo,无ENV HOME→HOME=/→写/.dubbo(只读根fs失败);
# 设HOME=/home/esp让Dubbo默认路径落可写家目录/home/esp/.dubbo而非根/.dubbo
RUN useradd -r -u 1000 -m -d /home/esp esp
ENV HOME=/home/esp
USER 1000
