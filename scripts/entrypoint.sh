#!/bin/sh

# 替换配置文件中的环境变量
envsubst < /nats/conf/jetstream.template.conf > /nats/conf/jetstream.conf

# 执行 NATS 服务器
exec nats-server --config /nats/conf/jetstream.conf