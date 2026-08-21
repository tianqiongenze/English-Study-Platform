# 前端 nginx 静态托管基础镜像(唯一非标 tag,禁劫持标准名 per SVD)
#
# 用途:ESP 前端应用(frontend-react apps/web / apps/h5)的多阶段构建 runtime stage。
#       基于 nginx:1.27-alpine,提供干净的 nginx 运行期 + 默认 SPA fallback 配置。
# 派生自:esp/frontend-react:1.2.1(其基础层为 nginx:1.27-alpine)。
#         选择派生 frontend-react 而非裸拉 nginx:1.27-alpine,是为了规避本环境 docker.io /
#         swr 镜像源不稳(见 memory esp-registry-mirror-outage-2026-08-16),复用本地已有镜像层。
#
# 派生时清理了 frontend-rel 添加的 per-app nginx.conf + dist 覆盖,只保留 nginx:1.27-alpine 基础层 +
# 一个通用 SPA 默认 server 块。业务镜像再 COPY 自己的 nginx.conf + dist 覆盖。
#
# SVD 铁律:本镜像 tag 为 `*-derived` 唯一非标命名,严禁 `docker tag esp/nginx-runtime:... nginx:...`
#          劫持标准镜像名。
#
# 何时更新:nginx 安全补丁 / 前端构建工具链升级 / 新增前端公共 nginx 模块指令时。
FROM esp/frontend-react:1.2.1

# 重置为干净 nginx 运行期:删掉 frontend-rel 叠加的 per-app nginx.conf + dist,
# 基础(nginx:1.27-alpine)层保留,只移除 app-specific 覆盖。
USER root
RUN rm -rf /usr/share/nginx/html/* /etc/nginx/conf.d/default.conf
COPY <<'EOF' /etc/nginx/conf.d/default.conf
server {
    listen       80;
    server_name  _;
    root   /usr/share/nginx/html;
    index  index.html;
    location / {
        try_files $uri $uri/ /index.html;
    }
}
EOF
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
