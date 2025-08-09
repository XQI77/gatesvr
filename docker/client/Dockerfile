# 多阶段构建 - 客户端测试程序
FROM golang:1.24-alpine AS builder

# 设置构建参数
ARG BUILD_DATE
ARG GIT_COMMIT
ARG VERSION=latest

# 安装构建依赖
RUN apk add --no-cache git ca-certificates tzdata

# 设置工作目录
WORKDIR /app

# 复制go.mod和go.sum文件，利用Docker缓存
COPY go.mod go.sum ./
RUN go mod download

# 复制源代码
COPY . .

# 构建客户端二进制文件
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.Version=${VERSION} -X main.BuildDate=${BUILD_DATE} -X main.GitCommit=${GIT_COMMIT}" \
    -o /app/bin/client ./cmd/client

# 运行阶段 - 最小化镜像
FROM alpine:latest

# 安装运行时依赖
RUN apk --no-cache add ca-certificates tzdata curl

# 创建非root用户
RUN addgroup -g 1000 appgroup && \
    adduser -u 1000 -G appgroup -s /bin/sh -D appuser

# 设置工作目录
WORKDIR /app

# 从构建阶段复制二进制文件
COPY --from=builder /app/bin/client ./client

# 复制证书文件（如果客户端需要）
COPY --from=builder /app/certs ./certs/

# 修改文件权限
RUN chown -R appuser:appgroup /app && \
    chmod +x /app/client

# 切换到非root用户
USER appuser

# 暴露健康检查端口
EXPOSE 19080

# 健康检查
HEALTHCHECK --interval=30s --timeout=10s --start-period=40s --retries=3 \
    CMD ./client -health-check 2>/dev/null || exit 1

# 设置标签
LABEL maintainer="gatesvr-team" \
      version="${VERSION}" \
      description="gatesvr测试客户端" \
      build-date="${BUILD_DATE}" \
      git-commit="${GIT_COMMIT}"

# 启动命令 (默认连接到gatesvr容器的QUIC端口)
ENTRYPOINT ["./client"]
CMD ["-server", "gatesvr:8453", "-interactive"]