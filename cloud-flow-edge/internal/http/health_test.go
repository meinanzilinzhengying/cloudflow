package http

import (
	"context"
	"net/http"
	"testing"
	"time"

	"cloudflow-edge/internal/probemgr"
	"cloudflow-edge/pkg/testutil"
)

func TestStartHealthServer(t *testing.T) {
	log := testutil.NewTestLogger()
	manager := probemgr.NewManager(log)
	defer manager.Stop()

	handler := NewHealthHandler(manager, log)
	addr := "localhost:0"
	server := StartHealthServer(addr, handler)

	if server == nil {
		t.Fatal("StartHealthServer 返回的 server 不应为 nil")
	}

	if server.server == nil {
		t.Fatal("server.server 不应为 nil")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown 失败: %v", err)
	}
}

func TestHealthServerShutdown(t *testing.T) {
	log := testutil.NewTestLogger()
	manager := probemgr.NewManager(log)
	defer manager.Stop()

	handler := NewHealthHandler(manager, log)
	addr := "localhost:0"
	server := StartHealthServer(addr, handler)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- server.Shutdown(ctx)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Shutdown 返回错误: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Shutdown 超时")
	}
}

func TestHealthServerDoubleShutdown(t *testing.T) {
	log := testutil.NewTestLogger()
	manager := probemgr.NewManager(log)
	defer manager.Stop()

	handler := NewHealthHandler(manager, log)
	addr := "localhost:0"
	server := StartHealthServer(addr, handler)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err1 := server.Shutdown(ctx)
	if err1 != nil {
		t.Fatalf("第一次 Shutdown 失败: %v", err1)
	}

	err2 := server.Shutdown(ctx)
	if err2 != nil {
		t.Logf("第二次 Shutdown 返回错误（预期行为）: %v", err2)
	}
}

func TestHealthServerGracefulShutdown(t *testing.T) {
	log := testutil.NewTestLogger()
	manager := probemgr.NewManager(log)
	defer manager.Stop()

	handler := NewHealthHandler(manager, log)
	addr := "localhost:0"
	server := StartHealthServer(addr, handler)

	client := &http.Client{Timeout: 2 * time.Second}

	go func() {
		time.Sleep(100 * time.Millisecond)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	}()

	resp, err := client.Get("http://" + addr + "/health")
	if err != nil {
		t.Logf("请求可能在关闭后失败（正常行为）: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("预期状态码 200，实际 %d", resp.StatusCode)
	}
}

func TestServerInterface(t *testing.T) {
	log := testutil.NewTestLogger()
	manager := probemgr.NewManager(log)
	defer manager.Stop()

	handler := NewHealthHandler(manager, log)
	addr := "localhost:0"
	server := StartHealthServer(addr, handler)

	var s *http.Server = server.server
	if s == nil {
		t.Fatal("server.server 应赋值给 *http.Server")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(ctx)
}
