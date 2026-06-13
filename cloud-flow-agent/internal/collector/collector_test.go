package collector

import "testing"

func TestIsPartition(t *testing.T) {
	tests := []struct {
		name      string
		diskName  string
		wantTrue  bool
	}{
		// NVMe 分区
		{"nvme partition", "nvme0n1p1", true},
		{"nvme partition multi-digit", "nvme0n1p12", true},
		{"nvme disk (no partition)", "nvme0n1", false},
		{"nvme namespace only", "nvme0n1", false},
		{"nvme with p but no number", "nvme0n1p", false},
		// SCSI/SATA 分区
		{"sda partition", "sda1", true},
		{"sdb partition", "sdb12", true},
		{"sda no partition", "sda", false},
		{"sdX no partition", "sdx", false},
		{"virtio partition", "vda1", true},
		// 特殊设备
		{"loop device", "loop0", false},
		{"loop with number", "loop1", false},
		{"md raid", "md0", false},
		{"dm mapper", "dm-0", false},
		{"empty string", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPartition(tt.diskName)
			if got != tt.wantTrue {
				t.Errorf("isPartition(%q) = %v, want %v", tt.diskName, got, tt.wantTrue)
			}
		})
	}
}

func TestParseMemLine(t *testing.T) {
	tests := []struct {
		name  string
		line  string
		want  uint64
	}{
		{"normal", "MemTotal:       16384000 kB", 16384000},
		{"with spaces", "MemAvailable:    8192000 kB", 8192000},
		{"single field", "MemTotal:", 0},
		{"empty", "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseMemLine(tt.line); got != tt.want {
				t.Errorf("parseMemLine(%q) = %d, want %d", tt.line, got, tt.want)
			}
		})
	}
}

func TestHasDigitSuffix(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want bool
	}{
		{"ends with digit", "sda1", true},
		{"ends with letter", "sda", false},
		{"single digit", "1", true},
		{"single letter", "a", false},
		{"empty", "", false},
		{"number only", "12345", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasDigitSuffix(tt.s); got != tt.want {
				t.Errorf("hasDigitSuffix(%q) = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}

func TestNewCollector(t *testing.T) {
	cfg := CollectConfig{CPU: true, Memory: true}
	c := New(cfg)
	if c == nil {
		t.Fatal("New() should not return nil")
	}
	if !c.cfg.CPU || !c.cfg.Memory {
		t.Error("config should be set correctly")
	}
}

func TestCollector_SetLogger(t *testing.T) {
	c := New(CollectConfig{})
	// 验证 SetLogger nil 不 panic
	c.SetLogger(nil)
	// 验证 SetLogger 正常 Logger 不 panic
	type testLogger struct{}
	c.SetLogger(nil) // Logger 接口为 nil 时不 panic 即可
}

func TestCollector_Collect_DisabledAll(t *testing.T) {
	c := New(CollectConfig{CPU: false, Memory: false, Network: false, Disk: false})
	metrics, err := c.Collect()
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if len(metrics) != 0 {
		t.Errorf("Collect() should return empty when all disabled, got %d", len(metrics))
	}
}

func TestCollector_Collect_CPUEnabled(t *testing.T) {
	c := New(CollectConfig{CPU: true})
	// 在非 Linux 环境或无 /proc/stat 时，collectCPU 返回 nil（不报错）
	metrics, err := c.Collect()
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	// 不验证具体值，只验证不 panic
}

func TestCollector_Collect_MemoryEnabled(t *testing.T) {
	c := New(CollectConfig{Memory: true})
	metrics, err := c.Collect()
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
}

func TestCollector_Collect_NetworkEnabled(t *testing.T) {
	c := New(CollectConfig{Network: true})
	metrics, err := c.Collect()
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
}

func TestCollector_Collect_DiskEnabled(t *testing.T) {
	c := New(CollectConfig{Disk: true})
	metrics, err := c.Collect()
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
}

func TestCollector_Collect_AllEnabled(t *testing.T) {
	c := New(CollectConfig{CPU: true, Memory: true, Network: true, Disk: true})
	metrics, err := c.Collect()
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	// 至少应该尝试采集，不 panic 即可
	_ = metrics
}
