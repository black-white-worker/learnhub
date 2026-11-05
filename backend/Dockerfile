# 学习平台后端镜像：多阶段构建（参考 videonest 的部署方式）
# 阶段一：在 golang 容器里编译出静态二进制
FROM golang:1.27-alpine AS builder

# 国内网络环境使用 goproxy.cn 加速依赖下载
ENV GOPROXY=https://goproxy.cn,direct
WORKDIR /build

# 先拷贝依赖清单，利用 Docker 层缓存
COPY go.mod go.sum ./
# --mount=type=cache：把 Go 模块缓存挂到持久缓存盘（不进镜像层）。
# 这样 go.mod 再怎么加依赖，已下载的模块直接复用，不会整层重新下载，
# 也不会在构建缓存里堆积重复的 300MB+ 依赖层。
RUN --mount=type=cache,target=/go/pkg/mod go mod download

# 拷贝源码并编译
COPY . .
# 编译缓存同样挂到持久缓存盘：只改一个文件时，增量编译，不整层重编。
# 入口在 cmd/api（拆分微服务时每个服务对应一个 cmd/<name>）。
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 GOOS=linux go build -trimpath -o learningplatform-api ./cmd/api

# 阶段二：只保留可执行文件和配置，镜像更小
FROM alpine:3.21
# 装时区、根证书、ffmpeg（视频 faststart 转码）、LibreOffice（docx/pptx 转 PDF 预览）、
# 中文字体（否则转出的 PDF 中文是方框）。
# ffmpeg 用 apk 装，不再依赖 mwader/static-ffmpeg 镜像：那个镜像要走 Docker Hub 拉 76MB，
# 国内服务器经常卡在 0B；apk 走阿里云镜像，稳定；ffprobe 也包含在 ffmpeg 包里。
# 分三层装：某一层失败后重跑，前面的层直接复用；apk 包缓存也挂在持久盘。

# 1) 基础工具 + ffmpeg
RUN --mount=type=cache,target=/var/cache/apk \
    sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
    && (grep -q '/community' /etc/apk/repositories \
    || echo 'https://mirrors.aliyun.com/alpine/v3.21/community' >> /etc/apk/repositories) \
    && apk add --cache-dir=/var/cache/apk tzdata ca-certificates ffmpeg \
    && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime

# 2) LibreOffice：包最多、解包最慢，单独一层。
#    --no-scripts 跳过 apk triggers（2C2G 上 gtk-update-icon-cache / gdk-pixbuf 等
#    trigger 容易以 "1 error; N MiB in M packages" 失败），再手动补运行所需的缓存。
RUN --mount=type=cache,target=/var/cache/apk \
    apk add --no-scripts --cache-dir=/var/cache/apk \
    libreoffice-writer libreoffice-impress \
    hicolor-icon-theme \
    && (gdk-pixbuf-query-loaders --update-cache || true) \
    && (update-mime-database /usr/share/mime || true) \
    && (glib-compile-schemas /usr/share/glib-2.0/schemas || true) \
    && (gtk-update-icon-cache -q -t -f /usr/share/icons/hicolor || true)

# 3) 中文字体：文泉驿比 font-noto-cjk 小得多、装得快（Noto 下载 70MB、解包几百 MB）；
#    同样跳过 trigger，手动跑 fc-cache 生成字体缓存
RUN --mount=type=cache,target=/var/cache/apk \
    apk add --no-scripts --cache-dir=/var/cache/apk fontconfig font-wqy-zenhei \
    && (fc-cache -f || true)
WORKDIR /app
COPY --from=builder /build/learningplatform-api .
COPY --from=builder /build/etc/learningplatform-api.yaml ./etc/learningplatform-api.yaml
EXPOSE 8888
CMD ["./learningplatform-api", "-f", "etc/learningplatform-api.yaml"]
