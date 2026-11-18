# syntax=docker/dockerfile:1

# ----------------------------------------------------------------------------
# 构建阶段：编译静态 actx-web 二进制
# ----------------------------------------------------------------------------
FROM golang:1.26-alpine AS builder

# 国内网络无法直连 proxy.golang.org，走公共镜像代理。
# snowflakeid 等私有/冷门依赖代理未缓存时会回退 direct 模式，需要 git。
ENV GOPROXY=https://goproxy.cn,direct

# dl-cdn.alpinelinux.org 在国内网络不可达，换清华镜像源。
RUN sed -i 's#https://dl-cdn.alpinelinux.org#https://mirrors.tuna.tsinghua.edu.cn#g' /etc/apk/repositories \
    && apk add --no-cache --no-check-certificate ca-certificates \
    && apk add --no-cache git

WORKDIR /src

# 先拷依赖清单再拷源码，利用层缓存加速重复构建。
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# CGO_ENABLED=0 产出静态二进制，运行镜像无需任何 Go 依赖。
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
    -o /out/actx-web ./cmd/actx-web

# ----------------------------------------------------------------------------
# 运行阶段：轻量 alpine + agent 常用工具
# ----------------------------------------------------------------------------
# agent 会在容器内执行 shell 命令：busybox 自带 ls/grep/find/sed，
# 额外装 git（版本操作）、curl（调试）、ca-certificates（HTTPS API 调用）。
FROM alpine:3.20

RUN sed -i 's#https://dl-cdn.alpinelinux.org#https://mirrors.tuna.tsinghua.edu.cn#g' /etc/apk/repositories \
    && apk add --no-cache ca-certificates git curl tzdata

# 非 root 运行。
RUN addgroup -S actx && adduser -S actx -G actx

# /workspace 是会话默认 cwd（挂载你的项目代码到这里）；
# 预建 .actx 目录让线程日志落在 /workspace/.actx/logs 而非向上逃逸。
# /data/actx 是 ACTX_HOME（config.toml / auth.json），挂卷持久化。
RUN mkdir -p /workspace/.actx /data/actx \
    && chown -R actx:actx /workspace /data/actx

WORKDIR /workspace
ENV ACTX_HOME=/data/actx

COPY --from=builder /out/actx-web /usr/local/bin/actx-web

USER actx

EXPOSE 8088

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8088/health || exit 1

ENTRYPOINT ["actx-web", "--addr", "0.0.0.0:8088"]
