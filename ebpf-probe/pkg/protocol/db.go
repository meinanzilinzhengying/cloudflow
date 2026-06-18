package protocol

import (
	"bytes"
	"fmt"
	"strings"
	"time"
)

type DBRecord struct {
	Timestamp   time.Time
	ProbeID     string
	SrcIP       string
	DstIP       string
	SrcPort     uint16
	DstPort     uint16
	DBType      string // mysql / redis
	Query       string
	LatencyMs   float64
	IsSlow      bool
	ErrorCode   int
	ErrorMsg    string
}

func ParseMySQL(payload []byte, srcIP, dstIP string, srcPort, dstPort uint16, direction uint8) *DBRecord {
	if len(payload) < 5 {
		return nil
	}
	rec := &DBRecord{
		Timestamp: time.Now(),
		SrcIP:     srcIP, DstIP: dstIP,
		SrcPort: srcPort, DstPort: dstPort,
		DBType:  "mysql",
	}
	if direction == 1 { // request
		rec.SrcIP, rec.DstIP = rec.DstIP, rec.SrcIP
		rec.SrcPort, rec.DstPort = rec.DstPort, rec.SrcPort
	}
	// Simple MySQL command type detection
	cmdType := payload[4]
	switch cmdType {
	case 0x03:
		rec.Query = "Query: " + string(payload[5:])
	case 0x16:
		rec.Query = "Prepare"
	case 0x17:
		rec.Query = "Execute"
	case 0x1e:
		rec.Query = "Binlog Dump"
	default:
		rec.Query = fmt.Sprintf("Cmd(0x%02x)", cmdType)
	}
	rec.Query = sanitizeSQL(rec.Query)
	return rec
}

func ParseRedis(payload []byte, srcIP, dstIP string, srcPort, dstPort uint16, direction uint8) *DBRecord {
	if len(payload) < 1 {
		return nil
	}
	rec := &DBRecord{
		Timestamp: time.Now(),
		SrcIP:     srcIP, DstIP: dstIP,
		SrcPort: srcPort, DstPort: dstPort,
		DBType:  "redis",
	}
	if direction == 1 { // request
		rec.SrcIP, rec.DstIP = rec.DstIP, rec.SrcIP
		rec.SrcPort, rec.DstPort = rec.DstPort, rec.SrcPort
	}
	// Redis RESP protocol
	if payload[0] == '*' {
		// Array
		parts := bytes.Split(payload, []byte("\r\n"))
		if len(parts) >= 3 {
			rec.Query = strings.TrimSpace(string(parts[2]))
		}
	} else if payload[0] == '+' || payload[0] == '-' || payload[0] == ':' || payload[0] == '$' {
		rec.Query = "Response"
	} else {
		rec.Query = string(payload[:min(50, len(payload))])
	}
	return rec
}

func sanitizeSQL(sql string) string {
	if len(sql) > 200 {
		sql = sql[:200] + "..."
	}
	return sql
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
