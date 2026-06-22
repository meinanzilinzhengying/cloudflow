# CloudFlow Multi-Region Deployment Guide

This document describes the architecture and procedures for deploying CloudFlow across multiple geographic regions.

---

## 1. Architecture Overview

### 1.1 Active-Active Mode
- All regions run active CloudFlow control-plane, data-plane, and query-service instances.
- Traffic is routed to the nearest healthy region based on geo-DNS or latency-based routing.
- Data is written to the local region and replicated asynchronously to other regions.
- Use case: Low-latency ingestion, high availability, disaster recovery.

### 1.2 Active-Passive Mode
- One region (primary) serves all read/write traffic.
- Secondary region(s) maintain warm standby with replicated data but do not serve traffic.
- Failover is initiated manually or via automated health checks.
- Use case: Cost optimization, compliance requirements (data residency), simplified operational model.

### 1.3 Recommended Topology
```
                +-----------------+
                |   Global DNS    |
                |  (Geo/Latency)  |
                +--------+--------+
                         |
         +---------------+---------------+
         |               |               |
         v               v               v
    +---------+     +---------+     +---------+
    |  LB-CN  |     |  LB-US  |     |  LB-EU  |
    +----+----+     +----+----+     +----+----+
         |               |               |
    +----+----+     +----+----+     +----+----+
    | Region  |     | Region  |     | Region  |
    | cn-east |     | us-west |     | eu-west |
    |         |     |         |     |         |
    | CP/DP/  |     | CP/DP/  |     | CP/DP/  |
    | Query   |     | Query   |     | Query   |
    |         |     |         |     |         |
    | etcd    |<--->| etcd    |<--->| etcd    |
    | Kafka   |<--->| Kafka   |<--->| Kafka   |
    | DB      |<--->| DB      |<--->| DB      |
    +---------+     +---------+     +---------+
```

---

## 2. Data Synchronization

### 2.1 etcd (Service Discovery & Coordination)
- Deploy an etcd cluster spanning regions (minimum 3 nodes per region, 5 total for quorum).
- Use `--initial-cluster` with nodes from all regions.
- Cross-region etcd peers should use dedicated, low-latency links or VPNs.
- Leader election and service registrations are globally consistent.
- Reference: `services/control-plane/service.go` (etcd init, leader election)

### 2.2 Kafka (Event Streaming)
- Deploy a Kafka cluster with MirrorMaker 2 or Kafka MirrorMaker for cross-region replication.
- Topics: `flow-events`, `metric-events`, `alert-events`, `audit-events`
- Replication factor: 3 per region; inter-region replication factor: 2 (to minimize RPO).
- Use `--replication-factor=3` and `min.insync.replicas=2` for durability.
- Reference: `services/shared/kafka/kafka.go`

### 2.3 Database Replication
- **Relational DB (MySQL/OceanBase/TiDB)**: Use native replication or TiDB's built-in multi-region Raft consensus.
- **ClickHouse**: Use ReplicatedMergeTree tables with ZooKeeper/Keeper for cross-region replication.
- **VictoriaMetrics**: Use vmagent or native cluster version for remote-write replication.
- **Loki**: Use S3/GCS as shared object storage with replication policies.

---

## 3. DNS & Load Balancer Configuration

### 3.1 DNS Configuration
- Use a global DNS service (e.g., Cloudflare, Route 53, Alibaba Cloud DNS) with:
  - **Geo-based routing**: Route users to the nearest region.
  - **Latency-based routing**: Route to the lowest-latency healthy endpoint.
  - **Health checks**: Automatic failover to healthy regions.
- Record example:
  ```
  api.cloudflow.io   A   1.2.3.4   (cn-east)
  api.cloudflow.io   A   5.6.7.8   (us-west)
  api.cloudflow.io   A   9.10.11.12 (eu-west)
  ```

### 3.2 Load Balancer (L4/L7)
- **Regional LBs**: Nginx/Envoy/HAProxy in each region for local traffic distribution.
- **Global LB**: Anycast or DNS-based load balancer for cross-region traffic steering.
- **Health Check Endpoints**:
  - `/healthz` for HTTP health checks
  - gRPC health checks via `grpc_health_v1`
- **Session Affinity**: Not required for stateless services; use JWT for authentication.

### 3.3 Ingress & TLS
- Terminate TLS at the regional LB or edge proxy.
- Use wildcard certificates (`*.cloudflow.io`) or per-region certificates.
- Enforce TLS 1.2+ and HSTS headers.

---

## 4. Failover Procedure

### 4.1 Automated Failover (Active-Active)
1. Health checks detect regional degradation (e.g., >5% error rate or >2s latency).
2. DNS/Load balancer removes unhealthy region from rotation.
3. Traffic is automatically rerouted to healthy regions.
4. etcd quorum is maintained across remaining regions.
5. Kafka consumers in healthy regions process replicated backlog.

### 4.2 Manual Failover (Active-Passive)
1. **Assess**: Confirm primary region failure via monitoring dashboards and alerts.
2. **Promote**: Update DNS records to point to the secondary region.
3. **Activate**: Start passive region services (control-plane, data-plane, query-service).
4. **Verify**: Run smoke tests against the promoted region (`/healthz`, sample queries).
5. **Notify**: Update status pages and notify stakeholders.
6. **Recover**: Once the primary region is restored, decide whether to fail back or maintain the new primary.

### 4.3 Rollback Steps
- If failover causes data inconsistency, pause writes and verify database replication lag.
- Use `etcdctl` to verify leader election state.
- Check Kafka consumer lag to ensure event backlog is processed.

---

## 5. Region-Specific Configuration

### 5.1 Config Overrides per Region
Each region should use a region-specific config file or environment variables:

```yaml
# region-cn-east.yaml
region: "cn-east"
etcd:
  endpoints: ["etcd-1.cn-east:2379", "etcd-2.cn-east:2379", "etcd-3.cn-east:2379"]
kafka:
  brokers: ["kafka-1.cn-east:9092", "kafka-2.cn-east:9092"]
database:
  host: "mysql.cn-east"
  read_replicas: ["mysql-read-1.cn-east", "mysql-read-2.cn-east"]
clickhouse:
  host: "clickhouse.cn-east"
object_storage:
  bucket: "cloudflow-cn-east"
  endpoint: "oss-cn-east.aliyuncs.com"
```

### 5.2 Environment Variables
```bash
export CLOUDFLOW_REGION="cn-east"
export CLOUDFLOW_ETCD_ENDPOINTS="etcd-1.cn-east:2379,etcd-2.cn-east:2379"
export CLOUDFLOW_DB_HOST="mysql.cn-east"
export CLOUDFLOW_CLICKHOUSE_HOST="clickhouse.cn-east"
export CLOUDFLOW_KAFKA_BROKERS="kafka-1.cn-east:9092"
export CLOUDFLOW_VICTORIA_METRICS_ADDR="http://vm.cn-east:8428"
export CLOUDFLOW_LOKI_ADDR="http://loki.cn-east:3100"
```

### 5.3 Service Startup with Region Config
```bash
export GOWORK=/opt/cloudflow/go.work
export GOROOT=/usr/local/go
export PATH=/usr/local/go/bin:$PATH
cd /opt/cloudflow

# Load region-specific config
source /etc/cloudflow/region-config.sh

# Start services with injected config
/usr/local/go/bin/go run ./services/control-plane/cmd --config=/etc/cloudflow/control-plane.yaml
/usr/local/go/bin/go run ./services/data-plane/cmd --config=/etc/cloudflow/data-plane.yaml
```

### 5.4 Cross-Region Network
- Enable VPC peering, VPN, or dedicated interconnects between regions.
- Restrict cross-region traffic to service ports (9001, 9002, 9009, etc.) via security groups.
- Encrypt all cross-region traffic with TLS/mTLS.

---

## 6. Operational Runbooks

### 6.1 Verify Multi-Region Health
```bash
# Check etcd cluster health
etcdctl endpoint health --endpoints=$(echo $ETCD_ENDPOINTS | tr ',' ' ')

# Check Kafka replication lag
kafka-consumer-groups.sh --bootstrap-server $KAFKA_BROKERS --describe --group cloudflow

# Check ClickHouse replication
clickhouse-client -q "SELECT database, table, is_leader, total_replicas FROM system.replicas"
```

### 6.2 Monitor Replication Lag
- **Database**: `Seconds_Behind_Master` (MySQL) or replication queue depth (ClickHouse).
- **Kafka**: Consumer group lag metrics (`kafka.consumer.lag`).
- **etcd**: `etcd_server_leader_changes_seen_total` should be stable.

### 6.3 Disaster Recovery Drill
- Schedule quarterly failover drills in a non-production environment.
- Validate RTO (Recovery Time Objective) < 15 minutes and RPO (Recovery Point Objective) < 5 minutes.

---

## References

- `services/control-plane/service.go` (etcd, agent management)
- `services/data-plane/service.go` (ClickHouse, VictoriaMetrics, Loki)
- `services/shared/discovery/registry.go` (service discovery)
- `services/shared/kafka/kafka.go` (event streaming)
