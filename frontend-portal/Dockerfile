# frontend-portal:统一多端入口(Web/H5/小程序),纯静态 nginx,无构建步骤。
# 只托管一个 index.html(门户页)+ nginx 反代到 frontend-react/frontend-h5/traefik。
# 镜像源用国内加速(docker.io 直拉超时),与 frontend-react/h5 一致。
FROM swr.cn-north-4.myhuaweicloud.com/ddn-k8s/docker.io/nginx:1.27-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY index.html /usr/share/nginx/html/index.html
EXPOSE 80
