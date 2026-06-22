package probe_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/meinanzilinzhengying/cloudflow/agent/internal/probe"
)

// ============================================================================
// 版本管理测试
// ============================================================================

func TestVersion(t *testing.T) {
	v1 := probe.Version{Major: 1, Minor: 2, Patch: 3, Build: "abc123"}
	v2 := probe.Version{Major: 1, Minor: 2, Patch: 4}
	v3 := probe.Version{Major: 1, Minor: 2, Patch: 3}

	t.Run("String", func(t *testing.T) {
		if s := v1.String(); s != "1.2.3-abc123" {
			t.Errorf("expected 1.2.3-abc123, got %s", s)
		}
	})

	t.Run("Compare newer", func(t *testing.T) {
		if v2.Compare(v1) <= 0 {
			t.Error("expected v2 > v1")
		}
	})

	t.Run("Compare equal", func(t *testing.T) {
		if v3.Compare(v1) != 0 {
			t.Error("expected v3 == v1 (patch equal)")
		}
	})

	t.Run("IsNewerThan", func(t *testing.T) {
		if !v2.IsNewerThan(v1) {
			t.Error("expected v2 newer than v1")
		}
		if v1.IsNewerThan(v2) {
			t.Error("expected v1 not newer than v2")
		}
	})
}

func TestVersionManager(t *testing.T) {
	current := probe.Version{Major: 1, Minor: 0, Patch: 0}
	vm := probe.NewVersionManager(current)

	newVersion := probe.Version{Major: 1, Minor: 1, Patch: 0, ReleaseAt: time.Now()}
	vm.RegisterAvailableVersion(newVersion, "Added new features")

	t.Run("Get current version", func(t *testing.T) {
		v := vm.GetCurrentVersion()
		if v.Major != 1 || v.Minor != 0 || v.Patch != 0 {
			t.Errorf("unexpected current version: %s", v)
		}
	})

	t.Run("Get latest version", func(t *testing.T) {
		latest := vm.GetLatestVersion()
		if latest == nil {
			t.Fatal("expected latest version")
		}
		if latest.Minor != 1 {
			t.Errorf("expected minor=1, got %d", latest.Minor)
		}
	})

	t.Run("Check upgrade available", func(t *testing.T) {
		info, err := vm.CheckUpgrade()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info == nil {
			t.Fatal("expected upgrade info")
		}
		if !info.TargetVersion.IsNewerThan(info.CurrentVersion) {
			t.Error("expected target newer than current")
		}
	})

	t.Run("Check no upgrade needed", func(t *testing.T) {
		// Create a new VM with only current version available
		vm2 := probe.NewVersionManager(current)
		vm2.RegisterAvailableVersion(current, "Current version")
		info, err := vm2.CheckUpgrade()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info != nil {
			t.Error("expected no upgrade available")
		}
	})

	t.Run("Mandatory upgrade", func(t *testing.T) {
		vm.SetMandatoryVersion(newVersion)
		info, _ := vm.CheckUpgrade()
		if info == nil {
			t.Fatal("expected upgrade info")
		}
		if !info.Mandatory {
			t.Error("expected mandatory upgrade")
		}
	})
}

// ============================================================================
// 升级引擎测试
// ============================================================================

func TestUpgradeEngine(t *testing.T) {
	current := probe.Version{Major: 1, Minor: 0, Patch: 0}
	vm := probe.NewVersionManager(current)
	vm.RegisterAvailableVersion(probe.Version{Major: 1, Minor: 1, Patch: 0, ReleaseAt: time.Now()}, "New version")

	strategy := probe.DefaultUpgradeStrategy()
	strategy.AutoUpgrade = false
	ue := probe.NewUpgradeEngine(vm, strategy)

	probe1 := &probe.ProbeInfo{
		ID:       "probe-1",
		HostName: "host1",
		IP:       "10.0.0.1",
		Version:  current,
		Status:   probe.ProbeStatusOnline,
		Tags:     map[string]string{"env": "prod"},
	}
	ue.RegisterProbe(probe1)

	t.Run("Register and get probe", func(t *testing.T) {
		p := ue.GetProbe("probe-1")
		if p == nil {
			t.Fatal("expected probe")
		}
		if p.HostName != "host1" {
			t.Errorf("expected hostname=host1, got %s", p.HostName)
		}
	})

	t.Run("Get all probes", func(t *testing.T) {
		probes := ue.GetAllProbes()
		if len(probes) != 1 {
			t.Errorf("expected 1 probe, got %d", len(probes))
		}
	})

	t.Run("Can upgrade now", func(t *testing.T) {
		// Default window is 02:00-04:00, so should be false during normal test time
		canUpgrade := ue.CanUpgradeNow()
		// This depends on test time, just check it doesn't panic
		t.Logf("can upgrade now: %v", canUpgrade)
	})

	t.Run("Start upgrade", func(t *testing.T) {
		ue.SetDownloadFunc(func(version probe.Version, progress chan float64) (string, error) {
			close(progress)
			return "/tmp/test.pkg", nil
		})
		ue.SetInstallFunc(func(packagePath string) error {
			return nil
		})

		task, err := ue.StartUpgrade("probe-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if task == nil {
			t.Fatal("expected upgrade task")
		}
		if task.ProbeID != "probe-1" {
			t.Errorf("expected probe_id=probe-1, got %s", task.ProbeID)
		}
		if task.Status != probe.UpgradeStatusPending {
			t.Errorf("expected status=pending, got %s", task.Status)
		}
	})

	t.Run("Start upgrade non-existent probe", func(t *testing.T) {
		_, err := ue.StartUpgrade("non-existent")
		if err == nil {
			t.Error("expected error for non-existent probe")
		}
	})

	t.Run("Get upgrade history", func(t *testing.T) {
		// Wait for async upgrade to complete
		time.Sleep(200 * time.Millisecond)
		history := ue.GetUpgradeHistory("probe-1")
		if len(history) == 0 {
			t.Error("expected upgrade history")
		}
	})
}

// ============================================================================
// 配置管理测试
// ============================================================================

func TestConfigManager(t *testing.T) {
	cm := probe.NewConfigManager()

	template := &probe.ConfigTemplate{
		ID:          "template-1",
		Name:        "Standard Config",
		Description: "Standard probe configuration",
		Category:    "standard",
		Content: map[string]interface{}{
			"collect_interval": 10,
			"buffer_size":      1024,
		},
		Version: "1.0.0",
	}
	if err := cm.RegisterTemplate(template); err != nil {
		t.Fatal(err)
	}

	t.Run("Register and get template", func(t *testing.T) {
		tmpl := cm.GetTemplate("template-1")
		if tmpl == nil {
			t.Fatal("expected template")
		}
		if tmpl.Name != "Standard Config" {
			t.Errorf("expected name=Standard Config, got %s", tmpl.Name)
		}
	})

	t.Run("Get all templates", func(t *testing.T) {
		templates := cm.GetAllTemplates()
		if len(templates) != 1 {
			t.Errorf("expected 1 template, got %d", len(templates))
		}
	})

	t.Run("Deploy config", func(t *testing.T) {
		config, err := cm.DeployConfig("probe-1", "template-1", map[string]interface{}{
			"custom_key": "custom_value",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if config == nil {
			t.Fatal("expected config")
		}
		if config.Status != probe.ConfigStatusPending {
			t.Errorf("expected status=pending, got %s", config.Status)
		}
		if config.Content["collect_interval"] != 10 {
			t.Errorf("expected collect_interval=10, got %v", config.Content["collect_interval"])
		}
		if config.Content["custom_key"] != "custom_value" {
			t.Errorf("expected custom_key=custom_value, got %v", config.Content["custom_key"])
		}
	})

	t.Run("Get probe config", func(t *testing.T) {
		config := cm.GetProbeConfig("probe-1")
		if config == nil {
			t.Fatal("expected config")
		}
	})

	t.Run("Confirm config applied", func(t *testing.T) {
		cm.ConfirmConfigApplied("probe-1", true)
		config := cm.GetProbeConfig("probe-1")
		if config.Status != probe.ConfigStatusApplied {
			t.Errorf("expected status=applied, got %s", config.Status)
		}
	})

	t.Run("Config history", func(t *testing.T) {
		history := cm.GetConfigHistory("probe-1")
		if len(history) == 0 {
			t.Error("expected config history")
		}
	})

	t.Run("Rollback config", func(t *testing.T) {
		// Need at least 2 successful configs for rollback
		_, err := cm.DeployConfig("probe-1", "template-1", map[string]interface{}{"key": "v2"})
		if err != nil {
			t.Fatal(err)
		}
		cm.ConfirmConfigApplied("probe-1", true)
		
		rolledBack, err := cm.RollbackConfig("probe-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rolledBack == nil {
			t.Fatal("expected rolled back config")
		}
	})

	t.Run("Rollback with no history", func(t *testing.T) {
		_, err := cm.RollbackConfig("probe-2")
		if err == nil {
			t.Error("expected error for no history")
		}
	})
}

// ============================================================================
// 灰度发布测试
// ============================================================================

func TestCanaryManager(t *testing.T) {
	cm := probe.NewCanaryManager()

	for i := 1; i <= 10; i++ {
		cm.RegisterProbe(&probe.ProbeInfo{
			ID:       fmt.Sprintf("probe-%d", i),
			HostName: fmt.Sprintf("host%d", i),
			Status:   probe.ProbeStatusOnline,
			Tags: map[string]string{
				"env": func() string {
					if i <= 5 {
						return "prod"
					}
					return "staging"
				}(),
			},
		})
	}

	version := probe.Version{Major: 2, Minor: 0, Patch: 0}
	strategy := probe.CanaryStrategy{
		Percentage: 30,
		AutoProgress: true,
	}

	release, err := cm.CreateRelease(version, strategy)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("Create release", func(t *testing.T) {
		if release == nil {
			t.Fatal("expected release")
		}
		if release.Status != probe.CanaryStatusPending {
			t.Errorf("expected status=pending, got %s", release.Status)
		}
	})

	t.Run("Start release", func(t *testing.T) {
		if err := cm.StartRelease(release.ID); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		
		rel := cm.GetRelease(release.ID)
		if rel.Status != probe.CanaryStatusRunning {
			t.Errorf("expected status=running, got %s", rel.Status)
		}
	})

	t.Run("Get selected probes", func(t *testing.T) {
		selected := cm.GetSelectedProbes(release.ID)
		if len(selected) == 0 {
			t.Error("expected selected probes")
		}
		// 30% of 10 = 3 probes
		if len(selected) != 3 {
			t.Errorf("expected 3 selected probes, got %d", len(selected))
		}
	})

	t.Run("Promote release", func(t *testing.T) {
		if err := cm.PromoteRelease(release.ID); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		rel := cm.GetRelease(release.ID)
		if rel.Status != probe.CanaryStatusPromoted {
			t.Errorf("expected status=promoted, got %s", rel.Status)
		}
	})

	t.Run("Rollback release", func(t *testing.T) {
		version2 := probe.Version{Major: 2, Minor: 1, Patch: 0}
		release2, _ := cm.CreateRelease(version2, probe.CanaryStrategy{Count: 2})
		cm.StartRelease(release2.ID)
		if err := cm.RollbackRelease(release2.ID); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		rel := cm.GetRelease(release2.ID)
		if rel.Status != probe.CanaryStatusRolledBack {
			t.Errorf("expected status=rolled_back, got %s", rel.Status)
		}
	})

	t.Run("Tag selector", func(t *testing.T) {
		version3 := probe.Version{Major: 3, Minor: 0, Patch: 0}
		release3, err := cm.CreateRelease(version3, probe.CanaryStrategy{
			TagSelector: map[string]string{"env": "prod"},
		})
		if err != nil {
			t.Fatal(err)
		}
		cm.StartRelease(release3.ID)
		selected := cm.GetSelectedProbes(release3.ID)
		if len(selected) != 5 {
			t.Errorf("expected 5 prod probes, got %d", len(selected))
		}
	})

	t.Run("Get all releases", func(t *testing.T) {
		releases := cm.GetAllReleases()
		if len(releases) == 0 {
			t.Error("expected releases")
		}
	})
}

// ============================================================================
// 性能评估测试
// ============================================================================

func TestPerformanceEvaluator(t *testing.T) {
	thresholds := probe.DefaultPerformanceThresholds()
	pe := probe.NewPerformanceEvaluator(thresholds)

	baseline := &probe.ProbePerformance{
		CPUPercent:     20.0,
		MemoryMB:       100.0,
		MemoryPercent:  10.0,
		DropRate:       0.1,
		LatencyMs:      10.0,
		Throughput:     1000,
		BaselineCPUPercent:    20.0,
		BaselineMemoryMB:        100.0,
		BaselineLatencyMs:       10.0,
	}
	pe.SetBaseline("probe-1", baseline)

	t.Run("Evaluate normal performance", func(t *testing.T) {
		current := &probe.ProbePerformance{
			CPUPercent:     25.0,
			MemoryMB:       110.0,
			MemoryPercent:  11.0,
			DropRate:       0.2,
			LatencyMs:      12.0,
			BaselineCPUPercent:    20.0,
			BaselineMemoryMB:        100.0,
			BaselineLatencyMs:       10.0,
		}
		impact := pe.Evaluate("probe-1", current)
		if impact.ImpactLevel != probe.ImpactLevelNone {
			t.Errorf("expected level=none, got %s", impact.ImpactLevel)
		}
		if impact.Score > 30 {
			t.Errorf("expected low score, got %.1f", impact.Score)
		}
	})

	t.Run("Evaluate high CPU impact", func(t *testing.T) {
		current := &probe.ProbePerformance{
			CPUPercent:     85.0,
			MemoryMB:       120.0,
			MemoryPercent:  12.0,
			DropRate:       0.5,
			LatencyMs:      15.0,
			BaselineCPUPercent:    20.0,
			BaselineMemoryMB:        100.0,
			BaselineLatencyMs:       10.0,
		}
		impact := pe.Evaluate("probe-1", current)
		if impact.ImpactLevel == probe.ImpactLevelNone {
			t.Error("expected non-none level for high CPU")
		}
	})

	t.Run("Evaluate critical impact", func(t *testing.T) {
		current := &probe.ProbePerformance{
			CPUPercent:     90.0,
			MemoryMB:       500.0,
			MemoryPercent:  95.0,
			DropRate:       8.0,
			LatencyMs:      120.0,
			BaselineCPUPercent:    20.0,
			BaselineMemoryMB:        100.0,
			BaselineLatencyMs:       10.0,
		}
		impact := pe.Evaluate("probe-1", current)
		if impact.ImpactLevel != probe.ImpactLevelCritical {
			t.Errorf("expected level=critical, got %s", impact.ImpactLevel)
		}
	})

	t.Run("Check alert", func(t *testing.T) {
		current := &probe.ProbePerformance{
			CPUPercent:     85.0,
			MemoryMB:       200.0,
			MemoryPercent:  20.0,
			DropRate:       1.0,
			LatencyMs:      30.0,
			BaselineCPUPercent:    20.0,
			BaselineMemoryMB:        100.0,
			BaselineLatencyMs:       10.0,
		}
		alert := pe.CheckAlert("probe-1", current)
		if alert == nil {
			t.Fatal("expected alert")
		}
		if alert.Level == probe.ImpactLevelNone {
			t.Error("expected non-none alert level")
		}
		if len(alert.Recommendations) == 0 {
			t.Error("expected recommendations")
		}
	})

	t.Run("Check no alert for normal performance", func(t *testing.T) {
		current := &probe.ProbePerformance{
			CPUPercent:     25.0,
			MemoryMB:       105.0,
			MemoryPercent:  10.5,
			DropRate:       0.1,
			LatencyMs:      11.0,
			BaselineCPUPercent:    20.0,
			BaselineMemoryMB:        100.0,
			BaselineLatencyMs:       10.0,
		}
		alert := pe.CheckAlert("probe-1", current)
		if alert != nil {
			t.Errorf("expected no alert, got %v", alert)
		}
	})

	t.Run("Get baseline", func(t *testing.T) {
		b := pe.GetBaseline("probe-1")
		if b == nil {
			t.Fatal("expected baseline")
		}
		if b.CPUPercent != 20.0 {
			t.Errorf("expected cpu=20.0, got %.1f", b.CPUPercent)
		}
	})
}
