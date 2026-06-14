package main

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2"
)

var (
	probeID    = envOrDefault("PROBE_ID", "vm2-probe")
	ifaceName  = envOrDefault("INTERFACE", "ens33")
	clickHouse = envOrDefault("CLICKHOUSE_ADDR", "192.168.58.130:9000")
	flushInt   = 10 * time.Second
)

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

type flowAggregate struct {
	SrcIP   string
	DstIP   string
	SrcPort uint16
	DstPort uint16
	Protocol string
	Bytes   uint64
	Packets uint64
}

var (
	mu    sync.Mutex
	flows map[string]*flowAggregate
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Printf("[EBPF-Probe] 启动: probe=%s iface=%s", probeID, ifaceName)

	db, err := sql.Open("clickhouse",
		fmt.Sprintf("tcp://%s?username=default&password=ClickHouse2024Secure&database=cloudflow",
			clickHouse))
	if err != nil {
		log.Fatalf("ClickHouse 连接失败: %v", err)
	}
	defer db.Close()

	// 打开 AF_PACKET 原始套接字抓真实网络包
	fd, err := syscall.Socket(syscall.AF_PACKET, syscall.SOCK_RAW, int(htons(syscall.ETH_P_ALL)))
	if err != nil {
		log.Fatalf("打开 AF_PACKET 失败: %v (需要 root 权限，且可能需要 CAP_NET_RAW)", err)
	}
	defer syscall.Close(fd)

	log.Printf("[EBPF-Probe] 开始抓包 (fd=%d)", fd)

	flows = make(map[string]*flowAggregate)

	// 定时刷新到 ClickHouse
	ticker := time.NewTicker(flushInt)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		for {
			select {
			case <-ticker.C:
				flushToClickHouse(db)
			case <-sig:
				flushToClickHouse(db)
				log.Printf("[EBPF-Probe] 停止")
				os.Exit(0)
			}
		}
	}()

	// 抓包主循环
	buf := make([]byte, 65536)
	for {
		n, _, err := syscall.Recvfrom(fd, buf, 0)
		if err != nil || n < 14 {
			continue
		}

		// 解析以太网帧，只处理 IPv4 (0x0800)
		// buf[12]=0x08, buf[13]=0x00 表示 IPv4
		if n > 14 && buf[12] == 0x08 && buf[13] == 0x00 {
			processIPv4(buf[14:n])
		}
	}
}

func htons(v uint16) uint16 {
	return (v << 8) | (v >> 8)
}

func processIPv4(payload []byte) {
	if len(payload) < 20 {
		return
	}

	versionIHL := payload[0]
	ihl := (versionIHL & 0x0F) * 4
	if int(ihl) > len(payload) {
		return
	}

	protocol := payload[9]
	srcIP := net.IP(payload[12:16]).String()
	dstIP := net.IP(payload[16:20]).String()

	protoName := "IP"
	srcPort, dstPort := uint16(0), uint16(0)

	switch protocol {
	case 6: // TCP
		protoName = "TCP"
		if len(payload) >= int(ihl)+4 {
			srcPort = binary.BigEndian.Uint16(payload[ihl : ihl+2])
			dstPort = binary.BigEndian.Uint16(payload[ihl+2 : ihl+4])
		}
	case 17: // UDP
		protoName = "UDP"
		if len(payload) >= int(ihl)+4 {
			srcPort = binary.BigEndian.Uint16(payload[ihl : ihl+2])
			dstPort = binary.BigEndian.Uint16(payload[ihl+2 : ihl+4])
		}
	case 1:
		protoName = "ICMP"
	}

	key := srcIP + ":" + strconv.Itoa(int(srcPort)) + "->" + dstIP + ":" + strconv.Itoa(int(dstPort)) + ":" + protoName

	mu.Lock()
	defer mu.Unlock()

	if ag, ok := flows[key]; ok {
		ag.Bytes += uint64(len(payload))
		ag.Packets++
	} else {
		flows[key] = &flowAggregate{
			SrcIP:   srcIP,
			DstIP:   dstIP,
			SrcPort: srcPort,
			DstPort: dstPort,
			Protocol: protoName,
			Bytes:   uint64(len(payload)),
			Packets: 1,
		}
	}
}

func flushToClickHouse(db *sql.DB) {
	mu.Lock()
	snapshot := flows
	flows = make(map[string]*flowAggregate)
	mu.Unlock()

	if len(snapshot) == 0 {
		return
	}

	now := time.Now()
	count := 0

	for _, ag := range snapshot {
		_, err := db.Exec(
			"INSERT INTO cloudflow.flows (timestamp, probe_id, src_ip, dst_ip, src_port, dst_port, protocol, bytes, packets, latency_ms, service) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			now, probeID, ag.SrcIP, ag.DstIP, ag.SrcPort, ag.DstPort, ag.Protocol,
			ag.Bytes, ag.Packets, 0.0, "",
		)
		if err != nil {
			log.Printf("[EBPF-Probe] 写入 ClickHouse 失败: %v", err)
		} else {
			count++
		}
	}

	if count > 0 {
		log.Printf("[EBPF-Probe] 写入 %d 条流量记录", count)
	}
}
