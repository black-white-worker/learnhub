# 学习平台后端镜像：多阶段构建（参考 videonest 的部署方式）
# 阶段一：在 golang 容器里编译出静态二进制
FROM golang:1.27-alpine AS builder

# 国内网络环境使用 goproxy.cn 加速依赖下载
ENV GOPROXY=https://goproxy.cn,direct
WORKDIR /build

# 先拷贝依赖清单，利用 Docker 层缓存
COPY go.mod go.sum ./
RUN go mod download

# 拷贝源码并编译
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o learningplatform-api learningplatform.go

# 阶段二：只保留可执行文件和配置，镜像更小
FROM alpine:3.21
RUN apk add --no-cache tzdata ca-certificates \
    && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime
WORKDIR /app
COPY --from=builder /build/learningplatform-api .
COPY --from=builder /build/etc/learningplatform-api.yaml ./etc/learningplatform-api.yaml
EXPOSE 8888
CMD ["./learningplatform-api", "-f", "etc/learningplatform-api.yaml"]
