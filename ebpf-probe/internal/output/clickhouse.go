package output

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2"
)

type ClickHouse struct {
	db *sql.DB
}

type Event struct {
	Timestamp time.Time
	ProbeID   string
	Category  string
	EventType string
	SrcIP     string
	DstIP     string
	SrcPort   uint16
	DstPort   uint16
	Protocol  string
	Bytes     uint64
	Packets   uint64
	LatencyMs float64
	Service   string
	Details   string
	Tags      string
}

func NewClickHouse(addr, user, password, database string) (*ClickHouse, error) {
	dsn := fmt.Sprintf("clickhouse://%s:9000?username=%s&password=%s&database=%s", addr, user, password, database)
	db, err := sql.Open("clickhouse", dsn)
	if err != nil { return nil, err }
	if err := db.Ping(); err != nil { return nil, err }
	ch := &ClickHouse{db: db}
	ch.createTables()
	return ch, nil
}

func (c *ClickHouse) createTables() {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS cloudflow.ebpf_events (
			timestamp DateTime, probe_id String, category String, event_type String,
			src_ip String, dst_ip String, src_port UInt16, dst_port UInt16, protocol String,
			bytes UInt64, packets UInt64, latency_ms Float64, service String, details String, tags String
		) ENGINE = MergeTree() ORDER BY (probe_id, category, timestamp) TTL timestamp + INTERVAL 30 DAY`,
		`CREATE TABLE IF NOT EXISTS cloudflow.host_metrics (
			timestamp DateTime, probe_id String, cpu_percent Float64, memory_percent Float64,
			disk_percent Float64, net_rx_bytes UInt64, net_tx_bytes UInt64, disk_read_bytes UInt64, disk_write_bytes UInt64
		) ENGINE = MergeTree() ORDER BY (probe_id, timestamp) TTL timestamp + INTERVAL 30 DAY`,
		`CREATE TABLE IF NOT EXISTS cloudflow.process_events (
			timestamp DateTime, probe_id String, pid UInt32, ppid UInt32, comm String, exe String, args String, event_type String
		) ENGINE = MergeTree() ORDER BY (probe_id, timestamp) TTL timestamp + INTERVAL 30 DAY`,
		`CREATE TABLE IF NOT EXISTS cloudflow.file_events (
			timestamp DateTime, probe_id String, pid UInt32, comm String, filename String, operation String, result Int32
		) ENGINE = MergeTree() ORDER BY (probe_id, timestamp) TTL timestamp + INTERVAL 30 DAY`,
		`CREATE TABLE IF NOT EXISTS cloudflow.syscall_events (
			timestamp DateTime, probe_id String, pid UInt32, comm String, syscall_nr UInt64, latency_ns UInt64, count UInt64
		) ENGINE = MergeTree() ORDER BY (probe_id, timestamp) TTL timestamp + INTERVAL 30 DAY`,
	}
	for _, q := range queries {
		if _, err := c.db.Exec(q); err != nil {
			log.Printf("[SETUP] 建表: %v", err)
		}
	}
}

func (c *ClickHouse) WriteEvent(e *Event) error {
	_, err := c.db.Exec(
		`INSERT INTO cloudflow.ebpf_events (timestamp, probe_id, category, event_type, src_ip, dst_ip, src_port, dst_port, protocol, bytes, packets, latency_ms, service, details, tags)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.Timestamp, e.ProbeID, e.Category, e.EventType, e.SrcIP, e.DstIP, e.SrcPort, e.DstPort, e.Protocol, e.Bytes, e.Packets, e.LatencyMs, e.Service, e.Details, e.Tags,
	)
	return err
}

func (c *ClickHouse) WriteBatch(events []*Event) error {
	if len(events) == 0 {
		return nil
	}
	query := "INSERT INTO cloudflow.ebpf_events (timestamp, probe_id, category, event_type, src_ip, dst_ip, src_port, dst_port, protocol, bytes, packets, latency_ms, service, details, tags) VALUES "
	args := make([]interface{}, 0, len(events)*15)
	for i := 0; i < len(events); i++ {
		query += "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?),"
	}
	query = query[:len(query)-1]
	for _, e := range events {
		args = append(args, e.Timestamp, e.ProbeID, e.Category, e.EventType, e.SrcIP, e.DstIP, e.SrcPort, e.DstPort, e.Protocol, e.Bytes, e.Packets, e.LatencyMs, e.Service, e.Details, e.Tags)
	}
	_, err := c.db.Exec(query, args...)
	return err
}

func (c *ClickHouse) WriteMetrics(probeID string, cpu, mem, disk float64, netRx, netTx, diskRead, diskWrite uint64) error {
	_, err := c.db.Exec(
		`INSERT INTO cloudflow.host_metrics (timestamp, probe_id, cpu_percent, memory_percent, disk_percent, net_rx_bytes, net_tx_bytes, disk_read_bytes, disk_write_bytes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		time.Now(), probeID, cpu, mem, disk, netRx, netTx, diskRead, diskWrite,
	)
	return err
}

func (c *ClickHouse) WriteProcessEvent(ts time.Time, probeID string, pid, ppid uint32, comm, exe, args, eventType string) error {
	_, err := c.db.Exec(
		`INSERT INTO cloudflow.process_events (timestamp, probe_id, pid, ppid, comm, exe, args, event_type)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		ts, probeID, pid, ppid, comm, exe, args, eventType,
	)
	return err
}

func (c *ClickHouse) WriteFileEvent(ts time.Time, probeID string, pid uint32, comm, filename, operation string, result int32) error {
	_, err := c.db.Exec(
		`INSERT INTO cloudflow.file_events (timestamp, probe_id, pid, comm, filename, operation, result)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		ts, probeID, pid, comm, filename, operation, result,
	)
	return err
}

func (c *ClickHouse) WriteSyscallEvent(ts time.Time, probeID string, pid uint32, comm string, syscallNr, latencyNs, count uint64) error {
	_, err := c.db.Exec(
		`INSERT INTO cloudflow.syscall_events (timestamp, probe_id, pid, comm, syscall_nr, latency_ns, count)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		ts, probeID, pid, comm, syscallNr, latencyNs, count,
	)
	return err
}

func (c *ClickHouse) Close() error { return c.db.Close() }
func (c *ClickHouse) Flush() { log.Printf("[OUTPUT] ClickHouse flushed") }

func (c *ClickHouse) QueryRow(query string, args ...interface{}) *sql.Row {
	return c.db.QueryRow(query, args...)
}

func (c *ClickHouse) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return c.db.Query(query, args...)
}
