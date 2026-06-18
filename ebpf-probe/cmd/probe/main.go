package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	ebpfprobe "github.com/meinanzilinzhengying/ebpf-probe"
	"github.com/meinanzilinzhengying/ebpf-probe/internal/api"
	"github.com/meinanzilinzhengying/ebpf-probe/internal/collector"
	"github.com/meinanzilinzhengying/ebpf-probe/internal/kernel"
	"github.com/meinanzilinzhengying/ebpf-probe/internal/output"
	"github.com/meinanzilinzhengying/ebpf-probe/pkg/platform"
)

var (
	probeID            = envOrDefault("PROBE_ID", platform.Hostname())
	edgeAddr           = envOrDefault("EDGE_ADDR", "192.168.58.130:9102")
	clickHouseAddr     = envOrDefault("CLICKHOUSE_ADDR", "192.168.58.130")
	clickHouseUser     = envOrDefault("CLICKHOUSE_USER", "default")
	clickHousePassword = envOrDefault("CLICKHOUSE_PASSWORD", "")
	clickHouseDB       = envOrDefault("CLICKHOUSE_DATABASE", "cloudflow")
	apiPort            = envOrDefault("API_PORT", "9090")
	ifaceName          = envOrDefault("INTERFACE", "ens33")
)

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("[CloudFlow eBPF Probe v%s]\n", ebpfprobe.Version)
	fmt.Printf("  probe_id:   %s\n", probeID)
	fmt.Printf("  platform:   %s\n", platform.Detect())
	fmt.Printf("  kernel:     %s\n", kernel.Version())
	fmt.Printf("  btf:        %v\n", kernel.HasBTF())
	fmt.Printf("  edge:       %s\n", edgeAddr)
	fmt.Printf("  clickhouse: %s\n", clickHouseAddr)
	fmt.Printf("  api_port:   %s\n", apiPort)
	fmt.Println("═══════════════════════════════════════════")

	kernelCap := kernel.DetectCapabilities()
	log.Printf("[KERNEL] 可用钩子: %+v", kernelCap.AvailableHooks)

	// EdgeClient 用于输出
	out, err := output.NewEdgeClient(edgeAddr)
	if err != nil {
		log.Fatalf("[FATAL] EdgeClient 初始化失败: %v", err)
	}
	defer out.Close()
	log.Printf("[OK] Edge 输出就绪")

	// ClickHouse 用于 API 查询
	ch, err := output.NewClickHouse(clickHouseAddr, clickHouseUser, clickHousePassword, clickHouseDB)
	if err != nil {
		log.Fatalf("[FATAL] ClickHouse 查询客户端初始化失败: %v", err)
	}
	defer ch.Close()
	log.Printf("[OK] ClickHouse 查询就绪")

	// 使用默认配置（所有扩展功能默认关闭）
	cfg := collector.DefaultConfig()

	mgr := collector.NewManager(out, probeID, ifaceName, cfg)
	if err := mgr.Init(kernelCap); err != nil {
		log.Fatalf("[FATAL] 采集器初始化失败: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := mgr.Start(ctx); err != nil {
		log.Fatalf("[FATAL] 采集器启动失败: %v", err)
	}
	log.Printf("[OK] 所有采集器已启动")

	go api.Start(apiPort, mgr, ch)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	log.Printf("[EBPF] 收到停止信号，正在清理...")
	cancel()
	mgr.Stop()
	out.Flush()
	log.Printf("[EBPF] 已安全退出")
}
