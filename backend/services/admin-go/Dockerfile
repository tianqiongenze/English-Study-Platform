# esp-admin-go — 统一管理员监测门户后端。零外部依赖,构建无需网络。
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod ./
COPY src ./src
ENV GOPROXY=https://goproxy.cn,direct
RUN CGO_ENABLED=0 go build -o /out/admin-go ./src

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
# 安全:非 root 运行(与 gateway-go 一致)
RUN addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=builder /out/admin-go /app/admin-go
EXPOSE 8099
ENV PORT=8099
USER app
ENTRYPOINT ["/app/admin-go"]
