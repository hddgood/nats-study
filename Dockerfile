# 第一阶段：构建 NATS 服务器
FROM golang:1.22-alpine AS builder

# 安装构建依赖
RUN apk add --update git

# 设置工作目录
WORKDIR /build

# 复制本地的源代码
COPY . .

# 构建 nats-server
RUN go build -v -o /nats-server

# 第二阶段：创建最终镜像
FROM alpine:latest

RUN apk add --no-cache gettext

# 安装必要的系统包
RUN apk add --update ca-certificates && \
    mkdir -p /nats/bin && \
    mkdir -p /nats/conf && \
    mkdir -p /data && \
    mkdir -p /var/run/nats

# # 复制配置文件
# COPY conf/jetstream.conf /nats/conf/jetstream.conf

# 从构建阶段复制二进制文件
COPY --from=builder /nats-server /bin/nats-server

# 暴露端口
EXPOSE 4222 8222 6222 5222

# 设置工作目录
WORKDIR /nats

# ENV NATS_CONFIG_FILE=/nats/conf/jetstream.conf

COPY conf/jetstream.template.conf /nats/conf/
COPY scripts/entrypoint.sh /nats/
RUN chmod +x /nats/entrypoint.sh

# 使用环境变量启动
ENTRYPOINT ["/nats/entrypoint.sh"]