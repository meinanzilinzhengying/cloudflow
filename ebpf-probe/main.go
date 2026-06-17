package main

import (
	"bufio"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2"
)

// ========== 配置 ==========

var (
	probeID            = envOrDefault("PROBE_ID", "vm2-probe")
	ifaceName          = envOrDefault("INTERFACE", "ens33")
	clickHouse         = envOrDefault("CLICKHOUSE_ADDR", "192.168.58.130")
	clickHouseUser     = envOrDefault("CLICKHOUSE_USER", "default")
	clickHousePassword = envOrDefault("CLICKHOUSE_PASSWORD", "")
	clickHouseDB       = envOrDefault("CLICKHOUSE_DATABASE", "cloudflow")
	apiPort            = envOrDefault("API_PORT", "9090")
	flushInt           = 10 * time.Second
	metricsInt         = 30 * time.Second
)

// ========== 数据结构 ==========

type FlowRecord struct {
	SrcIP    string
	DstIP    string
	SrcPort  uint16
	DstPort  uint16
	Protocol string
	Bytes    uint64
	Packets  uint64
	SynFlag  bool
	FinFlag  bool
	RstFlag  bool
}

type HTTPRecord struct {
	SrcIP      string
	DstIP      string
	SrcPort    uint16
	DstPort    uint16
	Method     string
	Host       string
	URL        string
	StatusCode int
	Bytes      uint64
	LatencyMs  float64
}

type DNSRecord struct {
	SrcIP      string
	DstIP      string
	QueryName  string
	QueryType  string
	ResponseIP string
}

type HostMetrics struct {
	CPUPercent    float64
	MemoryPercent float64
	DiskPercent   float64
	NetRxBytes    uint64
	NetTxBytes    uint64
	DiskReadBytes  uint64
	DiskWriteBytes uint64
}

// ========== 全局状态 ==========

var (
	mu           sync.Mutex
	flows        map[string]*FlowRecord
	httpRequests []HTTPRecord
	dnsQueries   []DNSRecord
	probeRunning = true
	probeStart   = time.Now()
	totalPackets uint64
	totalBytes   uint64
)

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ========== 系统指标采集 ==========

func getHostMetrics() HostMetrics {
	m := HostMetrics{}

	// CPU 使用率
	if data, err := os.ReadFile("/proc/stat"); err == nil {
		line := strings.Split(string(data), "\n")[0]
		parts := strings.Fields(line)
		if len(parts) > 4 {
			// cpu  user nice system idle iowait ...
			user, _ := strconv.ParseUint(parts[1], 10, 64)
			nice, _ := strconv.ParseUint(parts[2], 10, 64)
			system, _ := strconv.ParseUint(parts[3], 10, 64)
			idle, _ := strconv.ParseUint(parts[4], 10, 64)
			total := user + nice + system + idle
			if total > 0 {
				m.CPUPercent = float64(user+nice+system) / float64(total) * 100
			}
		}
	}

	// 内存使用率
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		var total, avail uint64
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "MemTotal:") {
				fmt.Sscanf(line, "MemTotal: %d", &total)
			} else if strings.HasPrefix(line, "MemAvailable:") {
				fmt.Sscanf(line, "MemAvailable: %d", &avail)
			}
		}
		if total > 0 {
			m.MemoryPercent = float64(total-avail) / float64(total) * 100
		}
	}

	// 磁盘使用率
	if out, err := exec.Command("df", "/").Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		if len(lines) > 1 {
			parts := strings.Fields(lines[1])
			if len(parts) > 4 {
				use, _ := strconv.Atoi(strings.TrimSuffix(parts[4], "%"))
				m.DiskPercent = float64(use)
			}
		}
	}

	// 网络 IO
	if data, err := os.ReadFile("/proc/net/dev"); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, ifaceName+":") {
				parts := strings.Fields(line)
				if len(parts) > 10 {
					m.NetRxBytes, _ = strconv.ParseUint(parts[1], 10, 64)
					m.NetTxBytes, _ = strconv.ParseUint(parts[9], 10, 64)
				}
				break
			}
		}
	}

	// 磁盘 IO
	if data, err := os.ReadFile("/proc/diskstats"); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, " sda ") || strings.Contains(line, " vda ") || strings.Contains(line, " nvme0n1 ") {
				parts := strings.Fields(line)
				if len(parts) > 13 {
					m.DiskReadBytes, _ = strconv.ParseUint(parts[5], 10, 64)
					m.DiskReadBytes *= 512
					m.DiskWriteBytes, _ = strconv.ParseUint(parts[9], 10, 64)
					m.DiskWriteBytes *= 512
				}
				break
			}
		}
	}

	return m
}

// ========== HTTP 管理 API ==========

type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

func startAPIServer() {
	mux := http.NewServeMux()

	// 探针状态
	mux.HandleFunc("/api/probe/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		mu.Lock()
		defer mu.Unlock()

		status := map[string]interface{}{
			"probe_id":      probeID,
			"running":       probeRunning,
			"interface":     ifaceName,
			"uptime_seconds": time.Since(probeStart).Seconds(),
			"total_packets": totalPackets,
			"total_bytes":   totalBytes,
			"active_flows":  len(flows),
			"metrics":       getHostMetrics(),
		}
		json.NewEncoder(w).Encode(APIResponse{Success: true, Data: status})
	})

	// 启动探针
	mux.HandleFunc("/api/probe/start", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		mu.Lock()
		probeRunning = true
		mu.Unlock()
		log.Printf("[API] 探针已启动")
		json.NewEncoder(w).Encode(APIResponse{Success: true, Data: "probe started"})
	})

	// 停止探针
	mux.HandleFunc("/api/probe/stop", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		mu.Lock()
		probeRunning = false
		mu.Unlock()
		log.Printf("[API] 探针已停止")
		json.NewEncoder(w).Encode(APIResponse{Success: true, Data: "probe stopped"})
	})

	// 重启探针
	mux.HandleFunc("/api/probe/restart", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		mu.Lock()
		probeRunning = true
		flows = make(map[string]*FlowRecord)
		totalPackets = 0
		totalBytes = 0
		mu.Unlock()
		log.Printf("[API] 探针已重启")
		json.NewEncoder(w).Encode(APIResponse{Success: true, Data: "probe restarted"})
	})

	// 系统指标
	mux.HandleFunc("/api/probe/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		metrics := getHostMetrics()
		json.NewEncoder(w).Encode(APIResponse{Success: true, Data: metrics})
	})

	// 原始指标 (Prometheus格式兼容)
	mux.HandleFunc("/api/probe/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(APIResponse{Success: true, Data: "healthy"})
	})

	// OPTIONS 预检
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
	})

	log.Printf("[API] 管理API启动在端口 %s", apiPort)
	if err := http.ListenAndServe(":"+apiPort, mux); err != nil {
		log.Printf("[API] HTTP服务异常: %v", err)
	}
}

// ========== 协议解析 ==========

func htons(v uint16) uint16 {
	return (v << 8) | (v >> 8)
}

func parseHTTP(payload []byte, srcIP, dstIP string, srcPort, dstPort uint16) *HTTPRecord {
	text := string(payload)
	if len(text) < 8 {
		return nil
	}

	// HTTP 请求
	if strings.HasPrefix(text, "GET ") || strings.HasPrefix(text, "POST ") ||
		strings.HasPrefix(text, "PUT ") || strings.HasPrefix(text, "DELETE ") ||
		strings.HasPrefix(text, "HEAD ") || strings.HasPrefix(text, "PATCH ") ||
		strings.HasPrefix(text, "OPTIONS ") {
		rec := &HTTPRecord{
			SrcIP:   srcIP,
			DstIP:   dstIP,
			SrcPort: srcPort,
			DstPort: dstPort,
			Bytes:   uint64(len(payload)),
		}

		parts := strings.SplitN(text, " ", 3)
		if len(parts) >= 2 {
			rec.Method = parts[0]
			rec.URL = parts[1]
		}

		// 解析 Host 头
		scanner := bufio.NewScanner(strings.NewReader(text))
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(strings.ToLower(line), "host:") {
				rec.Host = strings.TrimSpace(line[5:])
				break
			}
		}

		return rec
	}

	// HTTP 响应
	if strings.HasPrefix(text, "HTTP/") {
		rec := &HTTPRecord{
			SrcIP:   dstIP,
			DstIP:   srcIP,
			SrcPort: dstPort,
			DstPort: srcPort,
			Bytes:   uint64(len(payload)),
		}

		parts := strings.SplitN(text, " ", 3)
		if len(parts) >= 2 {
			code, err := strconv.Atoi(parts[1])
			if err == nil {
				rec.StatusCode = code
			}
		}
		return rec
	}

	return nil
}

func parseDNS(payload []byte, srcIP, dstIP string, srcPort, dstPort uint16) *DNSRecord {
	if len(payload) < 12 {
		return nil
	}

	// DNS header: transaction ID(2) + flags(2) + questions(2) + answers(2) + authority(2) + additional(2)
	// 跳过 header 后是查询名
	questions := binary.BigEndian.Uint16(payload[4:6])
	if questions == 0 {
		return nil
	}

	rec := &DNSRecord{
		SrcIP: srcIP,
		DstIP: dstIP,
	}

	// 解析查询名
	offset := 12
	var nameParts []string
	for offset < len(payload) {
		length := int(payload[offset])
		if length == 0 {
			break
		}
		if length&0xC0 == 0xC0 {
			// 指针压缩
			offset += 2
			break
		}
		if offset+1+length > len(payload) {
			break
		}
		nameParts = append(nameParts, string(payload[offset+1:offset+1+length]))
		offset += 1 + length
	}
	rec.QueryName = strings.Join(nameParts, ".")

	// 解析查询类型
	if offset+4 <= len(payload) {
		qtype := binary.BigEndian.Uint16(payload[offset+2:])
		switch qtype {
		case 1:
			rec.QueryType = "A"
		case 28:
			rec.QueryType = "AAAA"
		case 5:
			rec.QueryType = "CNAME"
		case 15:
			rec.QueryType = "MX"
		case 16:
			rec.QueryType = "TXT"
		case 2:
			rec.QueryType = "NS"
		default:
			rec.QueryType = fmt.Sprintf("TYPE%d", qtype)
		}
	}

	return rec
}

// ========== IPv4 包处理 ==========

func processIPv4(payload []byte) {
	if len(payload) < 20 {
		return
	}

	versionIHL := payload[0]
	ihl := int(versionIHL&0x0F) * 4
	if ihl > len(payload) {
		return
	}

	protocol := payload[9]
	srcIP := net.IP(payload[12:16]).String()
	dstIP := net.IP(payload[16:20]).String()

	// 跳过本地回环
	if srcIP == "127.0.0.1" || dstIP == "127.0.0.1" {
		return
	}

	protoName := "IP"
	srcPort, dstPort := uint16(0), uint16(0)
	var tcpFlags uint8 = 0
	var tcpPayload []byte

	switch protocol {
	case 6: // TCP
		protoName = "TCP"
		if len(payload) >= ihl+20 {
			srcPort = binary.BigEndian.Uint16(payload[ihl : ihl+2])
			dstPort = binary.BigEndian.Uint16(payload[ihl+2 : ihl+4])
			tcpFlags = payload[ihl+13]
			tcpDataOffset := int((payload[ihl+12] >> 4) * 4)
			if ihl+tcpDataOffset < len(payload) {
				tcpPayload = payload[ihl+tcpDataOffset:]
			}
		}

		// HTTP 解析 (端口 80, 8080, 8000-9999)
		if (dstPort == 80 || dstPort == 8080 || dstPort == 8000 ||
			dstPort == 3000 || dstPort == 443 || dstPort == 8443 ||
			(dstPort >= 8000 && dstPort <= 9999)) && len(tcpPayload) > 0 {
			if httpRec := parseHTTP(tcpPayload, srcIP, dstIP, srcPort, dstPort); httpRec != nil {
				mu.Lock()
				httpRequests = append(httpRequests, *httpRec)
				if len(httpRequests) > 1000 {
					httpRequests = httpRequests[len(httpRequests)-500:]
				}
				mu.Unlock()
			}
		}

	case 17: // UDP
		protoName = "UDP"
		if len(payload) >= ihl+8 {
			srcPort = binary.BigEndian.Uint16(payload[ihl : ihl+2])
			dstPort = binary.BigEndian.Uint16(payload[ihl+2 : ihl+4])
		}

		// DNS 解析
		if (srcPort == 53 || dstPort == 53) && len(payload) > ihl+8 {
			dnsPayload := payload[ihl+8:]
			if dnsRec := parseDNS(dnsPayload, srcIP, dstIP, srcPort, dstPort); dnsRec != nil {
				mu.Lock()
				dnsQueries = append(dnsQueries, *dnsRec)
				if len(dnsQueries) > 1000 {
					dnsQueries = dnsQueries[len(dnsQueries)-500:]
				}
				mu.Unlock()
			}
		}

	case 1:
		protoName = "ICMP"
	}

	key := fmt.Sprintf("%s:%d->%s:%d:%s", srcIP, srcPort, dstIP, dstPort, protoName)

	mu.Lock()
	defer mu.Unlock()

	synFlag := (tcpFlags & 0x02) != 0
	finFlag := (tcpFlags & 0x01) != 0
	rstFlag := (tcpFlags & 0x04) != 0

	if ag, ok := flows[key]; ok {
		ag.Bytes += uint64(len(payload))
		ag.Packets++
		if synFlag {
			ag.SynFlag = true
		}
		if finFlag {
			ag.FinFlag = true
		}
		if rstFlag {
			ag.RstFlag = true
		}
	} else {
		flows[key] = &FlowRecord{
			SrcIP:    srcIP,
			DstIP:    dstIP,
			SrcPort:  srcPort,
			DstPort:  dstPort,
			Protocol: protoName,
			Bytes:    uint64(len(payload)),
			Packets:  1,
			SynFlag:  synFlag,
			FinFlag:  finFlag,
			RstFlag:  rstFlag,
		}
	}
	totalPackets++
	totalBytes += uint64(len(payload))
}

// ========== ClickHouse 写入 ==========

func flushToClickHouse(db *sql.DB) {
	mu.Lock()
	if !probeRunning {
		mu.Unlock()
		return
	}

	snapshot := flows
	flows = make(map[string]*FlowRecord)

	httpSnapshot := httpRequests
	httpRequests = nil

	dnsSnapshot := dnsQueries
	dnsQueries = nil
	mu.Unlock()

	now := time.Now()
	flowCount := 0

	// 写入流量数据
	for _, ag := range snapshot {
		_, err := db.Exec(
			`INSERT INTO cloudflow.flows 
			(timestamp, probe_id, src_ip, dst_ip, src_port, dst_port, protocol, bytes, packets, latency_ms, service, syn_flag, fin_flag, rst_flag) 
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			now, probeID, ag.SrcIP, ag.DstIP, ag.SrcPort, ag.DstPort, ag.Protocol,
			ag.Bytes, ag.Packets, 0.0, "",
			btoi(ag.SynFlag), btoi(ag.FinFlag), btoi(ag.RstFlag),
		)
		if err != nil {
			log.Printf("[EBPF] 写入 flows 失败: %v", err)
		} else {
			flowCount++
		}
	}

	// 写入 HTTP 请求数据
	httpCount := 0
	for _, hr := range httpSnapshot {
		_, err := db.Exec(
			`INSERT INTO cloudflow.flows 
			(timestamp, probe_id, src_ip, dst_ip, src_port, dst_port, protocol, bytes, packets, latency_ms, service, http_method, http_host, http_url, http_status) 
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			now, probeID, hr.SrcIP, hr.DstIP, hr.SrcPort, hr.DstPort, "HTTP",
			hr.Bytes, 1, hr.LatencyMs, hr.Host, hr.Method, hr.Host, hr.URL, hr.StatusCode,
		)
		if err != nil {
			// 可能字段不存在，忽略
		} else {
			httpCount++
		}
	}

	// 写入 DNS 数据
	dnsCount := 0
	for _, dr := range dnsSnapshot {
		_, err := db.Exec(
			`INSERT INTO cloudflow.flows 
			(timestamp, probe_id, src_ip, dst_ip, src_port, dst_port, protocol, bytes, packets, latency_ms, service, dns_query, dns_type) 
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			now, probeID, dr.SrcIP, dr.DstIP, 53, 53, "DNS",
			0, 1, 0.0, "", dr.QueryName, dr.QueryType,
		)
		if err != nil {
			// 可能字段不存在
		} else {
			dnsCount++
		}
	}

	log.Printf("[EBPF] flow=%d http=%d dns=%d", flowCount, httpCount, dnsCount)
}

// 写入主机指标到 ClickHouse
func flushMetricsToClickHouse(db *sql.DB) {
	m := getHostMetrics()
	now := time.Now()

	_, err := db.Exec(
		`INSERT INTO cloudflow.host_metrics 
		(timestamp, probe_id, cpu_percent, memory_percent, disk_percent, net_rx_bytes, net_tx_bytes, disk_read_bytes, disk_write_bytes) 
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		now, probeID, m.CPUPercent, m.MemoryPercent, m.DiskPercent, m.NetRxBytes, m.NetTxBytes, m.DiskReadBytes, m.DiskWriteBytes,
	)
	if err != nil {
		log.Printf("[METRICS] 写入 host_metrics 失败: %v", err)
	} else {
		log.Printf("[METRICS] cpu=%.1f%% mem=%.1f%% disk=%.1f%%", m.CPUPercent, m.MemoryPercent, m.DiskPercent)
	}
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ========== 主程序 ==========

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Printf("═══════════════════════════════════════════")
	log.Printf("[EBPF-Probe v2.0] 启动")
	log.Printf("  probe_id:  %s", probeID)
	log.Printf("  interface: %s", ifaceName)
	log.Printf("  clickhouse: %s", clickHouse)
	log.Printf("  api_port:  %s", apiPort)
	log.Printf("═══════════════════════════════════════════")

	// 连接 ClickHouse
	dsn := fmt.Sprintf("http://%s:8123?username=%s&password=%s&database=%s",
		clickHouse, clickHouseUser, clickHousePassword, clickHouseDB)
	db, err := sql.Open("clickhouse", dsn)
	if err != nil {
		log.Fatalf("ClickHouse 连接失败: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Printf("[WARNING] ClickHouse ping 失败: %v", err)
	} else {
		log.Printf("[OK] ClickHouse 连接成功")
	}

	// 创建指标表
	createMetricsTable(db)

	// 启动管理 API
	go startAPIServer()

	// 打开 AF_PACKET
	fd, err := syscall.Socket(syscall.AF_PACKET, syscall.SOCK_RAW, int(htons(syscall.ETH_P_ALL)))
	if err != nil {
		log.Fatalf("AF_PACKET 失败: %v", err)
	}
	defer syscall.Close(fd)
	log.Printf("[OK] AF_PACKET 抓包已就绪 (fd=%d)", fd)

	flows = make(map[string]*FlowRecord)

	// 定时器
	flowTicker := time.NewTicker(flushInt)
	metricsTicker := time.NewTicker(metricsInt)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		for {
			select {
			case <-flowTicker.C:
				flushToClickHouse(db)
			case <-metricsTicker.C:
				flushMetricsToClickHouse(db)
			case <-sig:
				log.Printf("[EBPF] 收到停止信号，正在清理...")
				flushToClickHouse(db)
				flushMetricsToClickHouse(db)
				os.Exit(0)
			}
		}
	}()

	// 抓包主循环
	buf := make([]byte, 65536)
	for {
		n, _, err := syscall.Recvfrom(fd, buf, 0)
		if err != nil {
			if strings.Contains(err.Error(), "interrupted") {
				continue
			}
			log.Printf("[EBPF] 读包错误: %v", err)
			continue
		}
		if n < 14 {
			continue
		}

		// 仅处理 IPv4 (0x0800)
		if buf[12] == 0x08 && buf[13] == 0x00 {
			if probeRunning {
				processIPv4(buf[14:n])
			}
		}
	}
}

func createMetricsTable(db *sql.DB) {
	sql := `CREATE TABLE IF NOT EXISTS cloudflow.host_metrics (
		timestamp DateTime64(3),
		probe_id String,
		cpu_percent Float64,
		memory_percent Float64,
		disk_percent Float64,
		net_rx_bytes UInt64,
		net_tx_bytes UInt64,
		disk_read_bytes UInt64,
		disk_write_bytes UInt64
	) ENGINE = MergeTree()
	ORDER BY (probe_id, timestamp)
	TTL timestamp + INTERVAL 30 DAY`
	
	if _, err := db.Exec(sql); err != nil {
		log.Printf("[SETUP] 创建 host_metrics 表: %v", err)
	} else {
		log.Printf("[SETUP] host_metrics 表已就绪")
	}
}
