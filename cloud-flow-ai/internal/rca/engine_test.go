package rca_test

import (
	"testing"
	"time"

	"github.com/meinanzilinzhengying/cloudflow/ai/internal/rca"
)

func TestRCAEngine(t *testing.T) {
	engine := rca.NewRCAEngine()
	engine.LoadBuiltinPatterns()

	// 设置拓扑依赖
	engine.SetTopologyDeps(map[string][]string{
		"api":    {"db"},
		"web":    {"api", "cache"},
		"worker": {"db", "queue"},
	})

	t.Run("Analyze with cascade pattern", func(t *testing.T) {
		anomalies := []*rca.AnomalyEvent{
			{ID: "1", Service: "db", Metric: "latency", Value: 1500, Severity: "critical", Timestamp: time.Now().Add(-2 * time.Minute)},
			{ID: "2", Service: "api", Metric: "latency", Value: 3000, Severity: "critical", Timestamp: time.Now().Add(-1 * time.Minute)},
			{ID: "3", Service: "api", Metric: "error_rate", Value: 0.08, Severity: "warning", Timestamp: time.Now()},
		}

		result := engine.Analyze(anomalies)
		if result == nil {
			t.Fatal("expected result")
		}
		if len(result.RootCauses) == 0 {
			t.Error("expected root causes")
		}
		if len(result.Recommendations) == 0 {
			t.Error("expected recommendations")
		}
		if len(result.CauseChain) == 0 {
			t.Error("expected cause chain")
		}
	})

	t.Run("Analyze with memory pattern", func(t *testing.T) {
		anomalies := []*rca.AnomalyEvent{
			{ID: "1", Service: "worker", Metric: "memory", Value: 95, Severity: "critical", Timestamp: time.Now().Add(-1 * time.Minute)},
			{ID: "2", Service: "worker", Metric: "error_rate", Value: 0.15, Severity: "critical", Timestamp: time.Now()},
			{ID: "3", Service: "queue", Metric: "depth", Value: 15000, Severity: "warning", Timestamp: time.Now()},
		}

		result := engine.Analyze(anomalies)
		if result == nil {
			t.Fatal("expected result")
		}
	})

	t.Run("Analyze empty anomalies", func(t *testing.T) {
		result := engine.Analyze([]*rca.AnomalyEvent{})
		if result == nil {
			t.Fatal("expected result")
		}
		if len(result.RootCauses) > 0 {
			t.Error("expected no root causes for empty input")
		}
	})

	t.Run("Add and match pattern", func(t *testing.T) {
		pattern := &rca.AnomalyPattern{
			ID:   "test-pattern",
			Name: "Test Pattern",
			Symptoms: []rca.Symptom{
				{Service: "svc1", Metric: "cpu", Operator: ">", Threshold: 90},
			},
			RootCauses: []string{"high CPU"},
		}
		engine.AddAnomalyPattern(pattern)

		anomalies := []*rca.AnomalyEvent{
			{Service: "svc1", Metric: "cpu", Value: 95, Severity: "critical"},
		}
		result := engine.Analyze(anomalies)
		if result.PatternMatch != "Test Pattern" {
			t.Errorf("expected pattern match, got %s", result.PatternMatch)
		}
	})
}
