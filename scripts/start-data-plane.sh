#!/bin/bash
export CLICKHOUSE_ADDR=172.18.0.8
export CLICKHOUSE_PORT=9000
export CLICKHOUSE_USER=default
export CLICKHOUSE_PASSWORD=
export CLICKHOUSE_DATABASE=cloudflow
export CONTROL_PLANE_ADDR=172.18.0.1:8001
export GRPC_ADDR=:9002
export METRICS_ADDR=:9102

cd /opt/cloudflow
exec /opt/cloudflow/bin/data-plane -grpc-addr=":9002" -metrics-addr=":9102" 2>&1
