//go:build integration

package grpcclient

import (
	"context"
	"testing"
	"time"

	"cloud-flow-agent/pkg/logger"
	edge "cloud-flow/proto"
)

func TestClientReconnect(t *testing.T) {
	// 创建测试日志器
	log := logger.New(logger.Config{Level: "error", Format: "console"})

	// 创建客户端
	client, err := NewClient("localhost:9091", "test-api-key", TLSConfig{Enabled: false}, log)
	if err != nil {
		t.Fatalf("创建客户端失败: %v", err)
	}
	defer client.Close()

	// 测试心跳
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = client.Heartbeat(ctx, "test-probe")
	if err != nil {
		t.Errorf("心跳失败: %v", err)
	}

	// 测试注册
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := client.Register(ctx, "test-probe", "127.0.0.1", "test-host", "1.0.0")
	if err != nil {
		t.Errorf("注册失败: %v", err)
	}

	if !resp.Success {
		t.Errorf("注册失败，响应: %v", resp)
	}

	// 测试发送指标
	batch := &edge.MetricsBatch{
		ProbeId: "test-probe",
		Metrics: []*edge.MetricData{
			{
				Timestamp: time.Now().Unix(),
				SrcIp:     "10.0.0.1",
				DstIp:     "10.0.0.2",
				Protocol:  "tcp",
				Bytes:     1024,
				Packets:   10,
			},
		},
	}

	err = client.SendMetrics(ctx, batch)
	if err != nil {
		t.Errorf("发送指标失败: %v", err)
	}
}
