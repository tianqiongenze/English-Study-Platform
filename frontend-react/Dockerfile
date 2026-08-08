# frontend-react: 多阶段构建,nginx 静态托管 + /api 反代到集群内 BFF/网关
# 镜像源用国内加速(docker.io 直拉超时)
FROM swr.cn-north-4.myhuaweicloud.com/ddn-k8s/docker.io/node:20-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm config set registry https://registry.npmmirror.com && npm ci --no-audit --no-fund
COPY . .
RUN npm run build

FROM swr.cn-north-4.myhuaweicloud.com/ddn-k8s/docker.io/nginx:1.27-alpine
# nginx 配置:静态托管 dist + /api 反代到集群内服务(运行时由 K8s Service DNS 解析)
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 80
