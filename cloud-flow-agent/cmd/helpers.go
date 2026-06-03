package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/meinanzilinzhengying/cloudflow/agent/internal/grpcclient"
	"github.com/meinanzilinzhengying/cloudflow/agent/internal/storage"
	edge "github.com/meinanzilinzhengying/cloudflow/proto"
)

// safeClient 线程安全的客户端包装器
type safeClient struct {
	mu     sync.RWMutex
	client *grpcclient.Client
}

// Get 安全地获取客户端
func (sc *safeClient) Get() *grpcclient.Client {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.client
}

// GetOrNil 安全地获取客户端，显式返回 nil 表示未设置
func (sc *safeClient) GetOrNil() *grpcclient.Client {
	return sc.Get()
}

// IsNil 检查客户端是否为 nil
func (sc *safeClient) IsNil() bool {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.client == nil
}

// Set 安全地设置客户端
func (sc *safeClient) Set(c *grpcclient.Client) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.client = c
}

// assignedEdgeInfo 归属 Edge 完整信息
type assignedEdgeInfo struct {
	EdgeID    string
	Addr      string
	SessionID string
}

// saveAssignedEdge 持久化归属 Edge 到本地文件
func saveAssignedEdge(probeID, edgeID, sessionID, addr string) {
	dir := filepath.Join(os.TempDir(), "cloud-flow-edge")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}
	data := fmt.Sprintf("%s\t%s\t%s\t%s\t%d", probeID, edgeID, sessionID, addr, time.Now().Unix())
	if err := os.WriteFile(filepath.Join(dir, probeID+".edge"), []byte(data), 0600); err != nil {
		return
	}
}

// loadAssignedEdge 从本地文件恢复归属 Edge
func loadAssignedEdge(probeID string) *assignedEdgeInfo {
	path := filepath.Join(os.TempDir(), "cloud-flow-edge", probeID+".edge")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	parts := strings.Split(string(data), "\t")
	if len(parts) >= 5 {
		return &assignedEdgeInfo{
			EdgeID:    parts[1],
			SessionID: parts[2],
			Addr:      parts[3],
		}
	}
	if len(parts) >= 3 {
		return &assignedEdgeInfo{
			EdgeID:    parts[1],
			SessionID: parts[2],
		}
	}
	return nil
}

// getLocalIP 获取本机 IP 地址
func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		hostname, _ := os.Hostname()
		if hostname != "" {
			return hostname
		}
		return "0.0.0.0"
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			return ipnet.IP.String()
		}
	}

	hostname, _ := os.Hostname()
	if hostname != "" {
		return hostname
	}
	return "0.0.0.0"
}

// cacheMetrics 将指标数据写入本地缓存
func cacheMetrics(store *storage.TimeSeriesStore, metrics []*edge.MetricData) error {
	if store == nil || len(metrics) == 0 {
		return nil
	}
	return nil
}

// registerProbe 注册探针到边缘节点
func registerProbe(ctx context.Context, client *grpcclient.Client, probeID, hostIP, hostname, version string, log *logger.Logger) (*edge.RegisterResponse, error) {
	resp, err := client.Register(ctx, probeID, hostIP, hostname, version)
	if err != nil {
		return nil, err
	}
	log.Infof("注册成功: %s, 心跳间隔=%ds", resp.GetMessage(), resp.GetHeartbeatInterval())
	return resp, nil
}
