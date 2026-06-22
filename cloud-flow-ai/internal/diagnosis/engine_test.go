package diagnosis_test

import (
	"testing"

	"github.com/meinanzilinzhengying/cloudflow/ai/internal/diagnosis"
)

func TestDiagnosisEngine(t *testing.T) {
	engine := diagnosis.NewDiagnosisEngine()

	t.Run("Diagnose CPU high load", func(t *testing.T) {
		result := engine.Diagnose("app1", "performance", []diagnosis.Symptom{
			{Metric: "cpu", Value: 85, Severity: "warning"},
		})
		if result.Diagnosis == "" {
			t.Error("expected diagnosis")
		}
		if len(result.Remedies) == 0 {
			t.Error("expected remedies")
		}
		if result.Confidence == 0 {
			t.Error("expected confidence > 0")
		}
	})

	t.Run("Diagnose memory leak", func(t *testing.T) {
		result := engine.Diagnose("app2", "resource", []diagnosis.Symptom{
			{Metric: "memory", Value: 95, Severity: "critical"},
			{Metric: "memory_growth_rate", Value: 8, Severity: "critical"},
		})
		if result.Diagnosis == "" {
			t.Error("expected diagnosis")
		}
	})

	t.Run("Diagnose database connection pool", func(t *testing.T) {
		result := engine.Diagnose("api", "database", []diagnosis.Symptom{
			{Metric: "db_connections", Value: 95, Severity: "critical"},
			{Metric: "latency", Value: 3000, Severity: "critical"},
		})
		if result.Diagnosis == "" {
			t.Error("expected diagnosis")
		}
	})

	t.Run("Diagnose empty symptoms", func(t *testing.T) {
		result := engine.Diagnose("app", "test", []diagnosis.Symptom{})
		if result.Diagnosis == "症状数据不足，无法诊断" {
			// expected
		} else if result.Diagnosis == "" {
			t.Error("expected diagnosis for empty symptoms")
		}
	})

	t.Run("Get knowledge", func(t *testing.T) {
		entries := engine.GetKnowledge("performance")
		if len(entries) == 0 {
			t.Error("expected knowledge entries for performance")
		}
	})

	t.Run("Search knowledge", func(t *testing.T) {
		entries := engine.SearchKnowledge("CPU")
		if len(entries) == 0 {
			t.Error("expected search results for CPU")
		}
	})

	t.Run("Get history", func(t *testing.T) {
		history := engine.GetHistory("", 10)
		if len(history) == 0 {
			t.Error("expected history records")
		}
	})

	t.Run("Get stats", func(t *testing.T) {
		stats := engine.GetStats()
		if stats.TotalDiagnoses == 0 {
			t.Error("expected total diagnoses > 0")
		}
	})

	t.Run("AutoFix non-autofixable", func(t *testing.T) {
		result := engine.Diagnose("app", "test", []diagnosis.Symptom{
			{Metric: "cpu", Value: 85, Severity: "warning"},
		})
		fix := engine.AutoFix("app", result)
		if fix == "" {
			t.Error("expected autofix result")
		}
	})

	t.Run("Add and get rules", func(t *testing.T) {
		initialCount := len(engine.GetRules())
		engine.AddRule(diagnosis.DiagnosisRule{
			ID: "custom-rule", Name: "Custom", Category: "test",
			Conditions: []diagnosis.Condition{{Metric: "custom", Operator: ">", Value: 100}},
			Diagnosis: "custom diagnosis", Confidence: 0.8,
		})
		rules := engine.GetRules()
		if len(rules) != initialCount+1 {
			t.Errorf("expected %d rules, got %d", initialCount+1, len(rules))
		}
	})
}
