#!/bin/bash
# XKit 集成测试环境变量配置
# 使用方法: source deploy/integration/env.sh
#
# 这些环境变量会让 testcontainers 跳过容器启动，
# 直接使用 kube-play 预启动的服务。

export XKIT_REDIS_ADDR="localhost:6379"
export XKIT_MONGO_URI="mongodb://localhost:27017"
export XKIT_CLICKHOUSE_ADDR="localhost:9000"
export XKIT_KAFKA_BROKERS="localhost:9092"
export XKIT_PULSAR_URL="pulsar://localhost:6650"
export XKIT_ETCD_ENDPOINTS="localhost:2379"

echo "✅ XKit 集成测试环境变量已设置:"
echo "   XKIT_REDIS_ADDR=$XKIT_REDIS_ADDR"
echo "   XKIT_MONGO_URI=$XKIT_MONGO_URI"
echo "   XKIT_CLICKHOUSE_ADDR=$XKIT_CLICKHOUSE_ADDR"
echo "   XKIT_KAFKA_BROKERS=$XKIT_KAFKA_BROKERS"
echo "   XKIT_PULSAR_URL=$XKIT_PULSAR_URL"
echo "   XKIT_ETCD_ENDPOINTS=$XKIT_ETCD_ENDPOINTS"
echo ""
echo "💡 现在可以运行: task integration-test 或 go test -tags=integration ./pkg/..."
