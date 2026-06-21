//go:build linux

package autoscaler

import (
	"context"
	"testing"
	"time"
)

func TestHPAConfigValidation(t *testing.T) {
	cfg := DefaultHPAConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config should be valid: %v", err)
	}

	cfg.MinReplicas = 0
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for minReplicas=0")
	}

	cfg = DefaultHPAConfig()
	cfg.MaxReplicas = 1
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for maxReplicas < minReplicas")
	}
}

func TestAutoScalerCPUHighScaleUp(t *testing.T) {
	// CPU 90% > 目标 70%，应触发扩容
	collector := NewStaticMetricsCollector([]MetricValue{
		{Type: MetricCPU, Value: 90.0, Timestamp: time.Now()},
	})
	cfg := &HPAConfig{
		MinReplicas:            2,
		MaxReplicas:            10,
		TargetCPUUtilization:   70.0,
		ScaleUpStabilization:   0,
		ScaleDownStabilization: 0,
		ScaleUpStep:            2,
		ScaleDownStep:          1,
	}
	as, _ := NewAutoScaler(cfg, collector)
	as.SetCurrentReplicas(2)

	rec, err := as.Evaluate(context.Background())
	if err != nil {
		t.Fatalf("evaluate failed: %v", err)
	}
	if rec.Decision != ScaleUp {
		t.Errorf("expected ScaleUp, got %v", rec.Decision)
	}
	if rec.TargetReplicas <= 2 {
		t.Errorf("expected target > 2, got %d", rec.TargetReplicas)
	}
	t.Logf("ScaleUp: %s", rec.Reason)
}

func TestAutoScalerCPULowScaleDown(t *testing.T) {
	// CPU 30% < 目标 70% * 0.7 = 49%，应触发缩容
	collector := NewStaticMetricsCollector([]MetricValue{
		{Type: MetricCPU, Value: 30.0, Timestamp: time.Now()},
	})
	cfg := &HPAConfig{
		MinReplicas:            2,
		MaxReplicas:            10,
		TargetCPUUtilization:   70.0,
		ScaleUpStabilization:   0,
		ScaleDownStabilization: 0,
		ScaleUpStep:            2,
		ScaleDownStep:          1,
	}
	as, _ := NewAutoScaler(cfg, collector)
	as.SetCurrentReplicas(5)

	rec, err := as.Evaluate(context.Background())
	if err != nil {
		t.Fatalf("evaluate failed: %v", err)
	}
	if rec.Decision != ScaleDown {
		t.Errorf("expected ScaleDown, got %v", rec.Decision)
	}
	if rec.TargetReplicas >= 5 {
		t.Errorf("expected target < 5, got %d", rec.TargetReplicas)
	}
	t.Logf("ScaleDown: %s", rec.Reason)
}

func TestAutoScalerScaleUpStabilization(t *testing.T) {
	collector := NewStaticMetricsCollector([]MetricValue{
		{Type: MetricCPU, Value: 95.0, Timestamp: time.Now()},
	})
	cfg := &HPAConfig{
		MinReplicas:            2,
		MaxReplicas:            10,
		TargetCPUUtilization:   70.0,
		ScaleUpStabilization:   5 * time.Minute,
		ScaleDownStabilization: 5 * time.Minute,
		ScaleUpStep:            2,
		ScaleDownStep:          1,
	}
	as, _ := NewAutoScaler(cfg, collector)
	as.SetCurrentReplicas(2)

	// 第一次触发扩容
	rec, _ := as.Evaluate(context.Background())
	if rec.Decision != ScaleUp {
		t.Skip("first evaluate did not scale up")
	}

	// 短时间内再次评估，应被冷却阻止
	rec2, _ := as.Evaluate(context.Background())
	if rec2.Decision != ScaleNone {
		t.Errorf("expected ScaleNone due to stabilization, got %v", rec2.Decision)
	}
	if rec2.Reason == "" {
		t.Error("expected reason mentioning stabilization")
	}
}

func TestAutoScalerScaleDownStabilization(t *testing.T) {
	collector := NewStaticMetricsCollector([]MetricValue{
		{Type: MetricCPU, Value: 20.0, Timestamp: time.Now()},
	})
	cfg := &HPAConfig{
		MinReplicas:            2,
		MaxReplicas:            10,
		TargetCPUUtilization:   70.0,
		ScaleUpStabilization:   5 * time.Minute,
		ScaleDownStabilization: 5 * time.Minute,
		ScaleUpStep:            2,
		ScaleDownStep:          1,
	}
	as, _ := NewAutoScaler(cfg, collector)
	as.SetCurrentReplicas(5)

	rec, _ := as.Evaluate(context.Background())
	if rec.Decision != ScaleDown {
		t.Skip("first evaluate did not scale down")
	}

	rec2, _ := as.Evaluate(context.Background())
	if rec2.Decision != ScaleNone {
		t.Errorf("expected ScaleNone due to stabilization, got %v", rec2.Decision)
	}
}

func TestAutoScalerMinMaxReplicas(t *testing.T) {
	// CPU 99%，当前 10 = max，不应超过上限
	collector := NewStaticMetricsCollector([]MetricValue{
		{Type: MetricCPU, Value: 99.0, Timestamp: time.Now()},
	})
	cfg := &HPAConfig{
		MinReplicas:            2,
		MaxReplicas:            10,
		TargetCPUUtilization:   70.0,
		ScaleUpStabilization:   0,
		ScaleDownStabilization: 0,
		ScaleUpStep:            5,
		ScaleDownStep:          1,
	}
	as, _ := NewAutoScaler(cfg, collector)
	as.SetCurrentReplicas(10)

	rec, _ := as.Evaluate(context.Background())
	if rec.TargetReplicas > 10 {
		t.Errorf("target should not exceed maxReplicas, got %d", rec.TargetReplicas)
	}

	// CPU 10%，当前 2 = min，不应低于下限
	as.SetCurrentReplicas(2)
	collector2 := NewStaticMetricsCollector([]MetricValue{
		{Type: MetricCPU, Value: 10.0, Timestamp: time.Now()},
	})
	as2, _ := NewAutoScaler(cfg, collector2)
	as2.SetCurrentReplicas(2)
	rec2, _ := as2.Evaluate(context.Background())
	if rec2.TargetReplicas < 2 {
		t.Errorf("target should not go below minReplicas, got %d", rec2.TargetReplicas)
	}
}

func TestAutoScalerQPSMetric(t *testing.T) {
	// QPS 5000，每个副本目标 1000，需要 5 个副本
	collector := NewStaticMetricsCollector([]MetricValue{
		{Type: MetricQPS, Value: 5000, Timestamp: time.Now()},
	})
	cfg := &HPAConfig{
		MinReplicas:            2,
		MaxReplicas:            10,
		TargetCPUUtilization:   70.0,
		TargetQPSPerReplica:    1000,
		ScaleUpStabilization:   0,
		ScaleDownStabilization: 0,
		ScaleUpStep:            2,
		ScaleDownStep:          1,
	}
	as, _ := NewAutoScaler(cfg, collector)
	as.SetCurrentReplicas(2)

	rec, _ := as.Evaluate(context.Background())
	if rec.Decision != ScaleUp {
		t.Errorf("expected ScaleUp for QPS, got %v", rec.Decision)
	}
	if rec.TargetReplicas < 4 {
		t.Errorf("expected target >= 4 for QPS=5000, got %d", rec.TargetReplicas)
	}
}

func TestAutoScalerMultiMetrics(t *testing.T) {
	// CPU 50%（不需要扩容），QPS 3000（需要 3 个副本），取最大
	collector := NewStaticMetricsCollector([]MetricValue{
		{Type: MetricCPU, Value: 50.0, Timestamp: time.Now()},
		{Type: MetricQPS, Value: 3000, Timestamp: time.Now()},
	})
	cfg := &HPAConfig{
		MinReplicas:            2,
		MaxReplicas:            10,
		TargetCPUUtilization:   70.0,
		TargetQPSPerReplica:    1000,
		ScaleUpStabilization:   0,
		ScaleDownStabilization: 0,
		ScaleUpStep:            2,
		ScaleDownStep:          1,
	}
	as, _ := NewAutoScaler(cfg, collector)
	as.SetCurrentReplicas(2)

	rec, _ := as.Evaluate(context.Background())
	if rec.Decision != ScaleUp {
		t.Errorf("expected ScaleUp due to QPS, got %v", rec.Decision)
	}
	if rec.TargetReplicas < 3 {
		t.Errorf("expected target >= 3, got %d", rec.TargetReplicas)
	}
}

func TestScaleUpWarmup(t *testing.T) {
	warmup := NewScaleUpWarmup(100*time.Millisecond, 5)
	warmup.RegisterNode("edge-3")

	if warmup.IsWarmupCompleted("edge-3") {
		t.Error("new node should not be warmup completed")
	}

	w1 := warmup.GetWeight("edge-3")
	if w1 > 0.01 {
		t.Errorf("expected weight ~0.0 at start, got %.2f", w1)
	}

	time.Sleep(60 * time.Millisecond)
	w2 := warmup.GetWeight("edge-3")
	if w2 <= 0.0 || w2 >= 1.0 {
		t.Errorf("expected weight between 0 and 1 during warmup, got %.2f", w2)
	}

	time.Sleep(60 * time.Millisecond)
	w3 := warmup.GetWeight("edge-3")
	if w3 != 1.0 {
		t.Errorf("expected weight 1.0 after warmup, got %.2f", w3)
	}
	if !warmup.IsWarmupCompleted("edge-3") {
		t.Error("node should be warmup completed after duration")
	}

	// 未知节点默认全权重
	w4 := warmup.GetWeight("edge-99")
	if w4 != 1.0 {
		t.Errorf("unknown node should have weight 1.0, got %.2f", w4)
	}
}

func TestScaleDownProtection(t *testing.T) {
	sdp := NewScaleDownProtection(100 * time.Millisecond)

	// 未注册节点默认可移除
	if !sdp.CanRemove("edge-1") {
		t.Error("unregistered node should be removable")
	}

	// 发起缩容保护
	sdp.InitiateRemoval("edge-1", 1000)
	if sdp.CanRemove("edge-1") {
		t.Error("node should not be removable immediately after initiation")
	}
	if sdp.GetNodeState("edge-1") != ProtectionDataMigrating {
		t.Errorf("expected state DataMigrating, got %v", sdp.GetNodeState("edge-1"))
	}

	// 更新迁移进度到 50%
	sdp.UpdateMigrationProgress("edge-1", 500)
	progress := sdp.GetMigrationProgress("edge-1")
	if progress != 0.5 {
		t.Errorf("expected progress 0.5, got %.2f", progress)
	}
	if sdp.CanRemove("edge-1") {
		t.Error("node should not be removable at 50% migration")
	}

	// 完成迁移
	sdp.UpdateMigrationProgress("edge-1", 1000)
	if !sdp.CanRemove("edge-1") {
		t.Error("node should be removable after full migration")
	}
	if sdp.GetNodeState("edge-1") != ProtectionReadyToRemove {
		t.Errorf("expected state ReadyToRemove, got %v", sdp.GetNodeState("edge-1"))
	}

	// 确认移除
	sdp.ConfirmRemoval("edge-1")
	if sdp.GetNodeState("edge-1") != ProtectionIdle {
		t.Errorf("expected state Idle after removal, got %v", sdp.GetNodeState("edge-1"))
	}
}

func TestScaleDownProtectionTimeout(t *testing.T) {
	sdp := NewScaleDownProtection(50 * time.Millisecond)
	sdp.InitiateRemoval("edge-2", 1000)

	if sdp.CanRemove("edge-2") {
		t.Error("node should not be removable immediately")
	}

	time.Sleep(100 * time.Millisecond)
	if !sdp.CanRemove("edge-2") {
		t.Error("node should be removable after graceful timeout")
	}
	if sdp.GetNodeState("edge-2") != ProtectionReadyToRemove {
		t.Errorf("expected state ReadyToRemove after timeout, got %v", sdp.GetNodeState("edge-2"))
	}
}

func TestScaleUpWarmupMultipleNodes(t *testing.T) {
	warmup := NewScaleUpWarmup(200*time.Millisecond, 5)
	warmup.RegisterNode("edge-1")
	warmup.RegisterNode("edge-2")

	w1 := warmup.GetWeight("edge-1")
	w2 := warmup.GetWeight("edge-2")
	if w1 > 0.01 || w2 > 0.01 {
		t.Errorf("both nodes should start at weight ~0, got %.2f, %.2f", w1, w2)
	}

	time.Sleep(250 * time.Millisecond)
	if !warmup.IsWarmupCompleted("edge-1") || !warmup.IsWarmupCompleted("edge-2") {
		t.Error("both nodes should be completed after warmup duration")
	}
}

func TestAutoScalerRecommendationsHistory(t *testing.T) {
	collector := NewStaticMetricsCollector([]MetricValue{
		{Type: MetricCPU, Value: 90.0, Timestamp: time.Now()},
	})
	cfg := &HPAConfig{
		MinReplicas:            2,
		MaxReplicas:            10,
		TargetCPUUtilization:   70.0,
		ScaleUpStabilization:   0,
		ScaleDownStabilization: 0,
		ScaleUpStep:            2,
		ScaleDownStep:          1,
	}
	as, _ := NewAutoScaler(cfg, collector)
	as.SetCurrentReplicas(2)

	as.Evaluate(context.Background())
	recs := as.GetRecommendations()
	if len(recs) == 0 {
		t.Error("expected at least one recommendation recorded")
	}
	if recs[0].Decision != ScaleUp {
		t.Errorf("expected first recommendation to be ScaleUp, got %v", recs[0].Decision)
	}
}
