package main

import (
	"context"
	"fmt"
	"time"

	edge "github.com/meinanzilinzhengying/cloudflow/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	conn, err := grpc.NewClient("192.168.58.130:9002", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Printf("Failed to connect: %v\n", err)
		return
	}
	defer conn.Close()

	client := edge.NewProbeServiceClient(conn)

	// 构造测试数据
	batch := &edge.MetricsBatch{
		ProbeId: "test-probe-001",
		Metrics: []*edge.MetricData{
			{
				Name:      "cpu_usage",
				Value:     0.95,
				Timestamp: time.Now().UnixMilli(),
				ProbeId:   "test-probe-001",
				Tags:       map[string]string{"host": "test"},
			},
		},
	}

	fmt.Println("Calling SendMetrics...")
	resp, err := client.SendMetrics(context.Background(), batch)
	if err != nil {
		fmt.Printf("SendMetrics failed: %v\n", err)
		return
	}
	fmt.Printf("SendMetrics succeeded: %+v\n", resp)
}
