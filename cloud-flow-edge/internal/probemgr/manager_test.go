package probemgr

import (
	"testing"
	"time"

	"cloud-flow-edge/pkg/testutil"
)

func TestRegister(t *testing.T) {
	log := testutil.NewTestLogger()
	mgr := NewManager(log)

	p := mgr.Register("probe-1", "10.0.0.1", "host-a", "v1.0")
	if p.ID != "probe-1" {
		t.Fatalf("期望 ID=probe-1, 实际 %s", p.ID)
	}
	if p.Status != "online" {
		t.Fatalf("期望 Status=online, 实际 %s", p.Status)
	}
	if p.HostIP != "10.0.0.1" {
		t.Fatalf("期望 HostIP=10.0.0.1, 实际 %s", p.HostIP)
	}
}

func TestRegisterPreservesRegisteredAt(t *testing.T) {
	mgr := NewManager(testutil.NewTestLogger())

	p1 := mgr.Register("probe-1", "10.0.0.1", "host-a", "v1.0")
	registeredAt := p1.RegisteredAt

	time.Sleep(10 * time.Millisecond)
	p2 := mgr.Register("probe-1", "10.0.0.2", "host-a-updated", "v2.0")

	if !p2.RegisteredAt.Equal(registeredAt) {
		t.Fatalf("重新注册应保留 RegisteredAt: 期望 %v, 实际 %v", registeredAt, p2.RegisteredAt)
	}
	if p2.HostIP != "10.0.0.2" {
		t.Fatalf("重新注册应更新 HostIP: 期望 10.0.0.2, 实际 %s", p2.HostIP)
	}
	if p2.Status != "online" {
		t.Fatalf("重新注册应重置为 online: 实际 %s", p2.Status)
	}
}

func TestHeartbeat(t *testing.T) {
	mgr := NewManager(testutil.NewTestLogger())
	mgr.Register("probe-1", "10.0.0.1", "host-a", "v1.0")

	if ok := mgr.Heartbeat("probe-1"); !ok {
		t.Fatal("心跳应返回 true")
	}
	if ok := mgr.Heartbeat("probe-unknown"); ok {
		t.Fatal("未知探针心跳应返回 false")
	}
}

func TestGetProbe(t *testing.T) {
	mgr := NewManager(testutil.NewTestLogger())
	mgr.Register("probe-1", "10.0.0.1", "host-a", "v1.0")

	p, ok := mgr.GetProbe("probe-1")
	if !ok {
		t.Fatal("GetProbe 应返回 true")
	}
	if p.ID != "probe-1" {
		t.Fatalf("期望 ID=probe-1, 实际 %s", p.ID)
	}

	_, ok = mgr.GetProbe("probe-missing")
	if ok {
		t.Fatal("不存在的探针应返回 false")
	}
}

func TestGetAllProbes(t *testing.T) {
	mgr := NewManager(testutil.NewTestLogger())
	mgr.Register("probe-1", "10.0.0.1", "host-a", "v1.0")
	mgr.Register("probe-2", "10.0.0.2", "host-b", "v1.0")
	mgr.Register("probe-3", "10.0.0.3", "host-c", "v1.0")

	all := mgr.GetAllProbes()
	if len(all) != 3 {
		t.Fatalf("期望 3 个探针, 实际 %d", len(all))
	}
}

func TestRemoveOfflineProbes(t *testing.T) {
	mgr := NewManager(testutil.NewTestLogger())

	// 注册两个探针，一个超时，一个正常
	mgr.Register("probe-ok", "10.0.0.1", "host-a", "v1.0")
	mgr.Register("probe-stale", "10.0.0.2", "host-b", "v1.0")

	// 手动修改 stale 探针的 LastHeartbeat 为更早的时间
	mgr.mu.Lock()
	mgr.probes["probe-stale"].LastHeartbeat = time.Now().Add(-2 * time.Minute)
	mgr.mu.Unlock()

	removed := mgr.RemoveOfflineProbes(1 * time.Minute)
	if removed != 1 {
		t.Fatalf("期望移除 1 个探针, 实际 %d", removed)
	}

	_, ok := mgr.GetProbe("probe-stale")
	if ok {
		t.Fatal("超时探针应已被移除")
	}

	_, ok = mgr.GetProbe("probe-ok")
	if !ok {
		t.Fatal("正常探针不应被移除")
	}
}

func TestRemoveOfflineProbesEmpty(t *testing.T) {
	mgr := NewManager(testutil.NewTestLogger())
	removed := mgr.RemoveOfflineProbes(1 * time.Minute)
	if removed != 0 {
		t.Fatalf("空管理器应移除 0 个, 实际 %d", removed)
	}
}

func TestStartAndStopCleanup(t *testing.T) {
	mgr := NewManager(testutil.NewTestLogger())
	mgr.Register("probe-1", "10.0.0.1", "host-a", "v1.0")

	// 启动清理协程，间隔极短以便测试
	mgr.StartCleanup(10*time.Millisecond, 50*time.Millisecond)
	time.Sleep(100 * time.Millisecond)

	mgr.Stop()
	// Stop 后不应 panic
}
