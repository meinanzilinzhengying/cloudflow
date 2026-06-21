package prediction_test

import (
	"testing"
	"time"

	"github.com/meinanzilinzhengying/cloudflow/ai/internal/prediction"
)

func TestPredictionEngine(t *testing.T) {
	engine := prediction.NewPredictionEngine()

	// 准备数据：cpu 使用率持续上升
	now := time.Now()
	for i := 0; i < 50; i++ {
		engine.Record("cpu:app1", now.Add(time.Duration(i)*time.Minute), float64(30+i))
	}

	// 准备数据：内存使用率
	for i := 0; i < 50; i++ {
		engine.Record("memory:app1", now.Add(time.Duration(i)*time.Minute), float64(40)+float64(i)*0.5)
	}

	// 准备数据：延迟数据（稳定）
	for i := 0; i < 50; i++ {
		engine.Record("latency:app1", now.Add(time.Duration(i)*time.Minute), 100.0)
	}

	t.Run("Predict upward trend", func(t *testing.T) {
		result := engine.Predict("cpu:app1", 1*time.Hour)
		if result.Trend != prediction.TrendUp {
			t.Errorf("expected upward trend, got %s", result.Trend)
		}
		if result.RiskLevel == prediction.RiskLow {
			t.Error("expected non-low risk for upward trend")
		}
		if result.Confidence <= 0 {
			t.Error("expected confidence > 0")
		}
	})

	t.Run("Predict stable trend", func(t *testing.T) {
		result := engine.Predict("latency:app1", 1*time.Hour)
		if result.Trend != prediction.TrendStable {
			t.Errorf("expected stable trend, got %s", result.Trend)
		}
	})

	t.Run("Predict empty data", func(t *testing.T) {
		result := engine.Predict("nonexistent", 1*time.Hour)
		if result.Trend != prediction.TrendUnknown {
			t.Errorf("expected unknown trend for empty data, got %s", result.Trend)
		}
	})

	t.Run("Predict capacity", func(t *testing.T) {
		cap := engine.PredictCapacity("cpu:app1", 100)
		if cap.CurrentUsage <= 0 {
			t.Error("expected current usage > 0")
		}
		if len(cap.Recommendations) == 0 {
			t.Error("expected recommendations")
		}
	})

	t.Run("Predict failure", func(t *testing.T) {
		indicators := []prediction.RiskIndicator{
			{Metric: "cpu", Current: 85, Threshold: 80},
			{Metric: "memory", Current: 92, Threshold: 90},
		}
		fail := engine.PredictFailure("app1", indicators)
		if fail.Probability <= 0 {
			t.Error("expected failure probability > 0")
		}
		if len(fail.Recommendations) == 0 {
			t.Error("expected recommendations")
		}
		if fail.FailureType == "" {
			t.Error("expected failure type")
		}
	})

	t.Run("Predict all", func(t *testing.T) {
		results := engine.PredictAll(1 * time.Hour)
		if len(results) == 0 {
			t.Error("expected prediction results")
		}
	})

	t.Run("Get time series keys", func(t *testing.T) {
		keys := engine.GetTimeSeriesKeys()
		if len(keys) == 0 {
			t.Error("expected time series keys")
		}
	})

	t.Run("Record batch", func(t *testing.T) {
		points := []prediction.DataPoint{
			{Timestamp: now.Add(100 * time.Minute), Value: 200},
			{Timestamp: now.Add(101 * time.Minute), Value: 201},
		}
		engine.RecordBatch("cpu:app2", points)
		result := engine.Predict("cpu:app2", 1*time.Hour)
		if result.Trend == prediction.TrendUnknown {
			t.Error("expected trend after batch record")
		}
	})
}
