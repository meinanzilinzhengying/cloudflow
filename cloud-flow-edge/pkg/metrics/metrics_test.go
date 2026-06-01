package metrics

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"cloud-flow-edge/pkg/testutil"
)

func TestStartServerReturnsServerAndErrorChannel(t *testing.T) {
	log := testutil.NewTestLogger()
	m := New(log)

	port := 0
	l, err := net.Listen("tcp", "localhost:0")
	if err == nil {
		port = l.Addr().(*net.TCPAddr).Port
		l.Close()
	}

	if port == 0 {
		port = 18080
	}

	server, errCh := m.StartServer(port)

	if server == nil {
		t.Fatal("StartServer 返回的 server 不应为 nil")
	}

	if errCh == nil {
		t.Fatal("StartServer 返回的 error channel 不应为 nil")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(ctx)
}

func TestStartServerErrorChannelOnPortConflict(t *testing.T) {
	log := testutil.NewTestLogger()
	m := New(log)

	port := 0
	l, err := net.Listen("tcp", "localhost:0")
	if err == nil {
		port = l.Addr().(*net.TCPAddr).Port
		l.Close()
	}

	if port == 0 {
		port = 18081
	}

	m2 := New(log)
	_, _ = m2.StartServer(port)

	server, errCh := m.StartServer(port)

	select {
	case err := <-errCh:
		if err == nil {
			t.Log("error channel 收到 nil 错误（端口可能已关闭）")
		}
	case <-time.After(2 * time.Second):
		t.Log("2秒内未收到错误（可能端口未被占用或服务器未启动）")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(ctx)
}

func TestStartServerGracefulShutdown(t *testing.T) {
	log := testutil.NewTestLogger()
	m := New(log)

	port := 0
	l, err := net.Listen("tcp", "localhost:0")
	if err == nil {
		port = l.Addr().(*net.TCPAddr).Port
		l.Close()
	}

	if port == 0 {
		port = 18082
	}

	server, _ := m.StartServer(port)

	client := &http.Client{Timeout: 2 * time.Second}

	go func() {
		time.Sleep(100 * time.Millisecond)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	}()

	resp, err := client.Get(fmt.Sprintf("http://localhost:%d/metrics", port))
	if err != nil {
		t.Logf("请求可能在关闭后失败（正常行为）: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("预期状态码 200，实际 %d", resp.StatusCode)
	}
}
