package nlq_test

import (
	"testing"

	"github.com/meinanzilinzhengying/cloudflow/ai/internal/nlq"
)

func TestNLQEngine(t *testing.T) {
	engine := nlq.NewNLQEngine()

	t.Run("Convert traffic query Chinese", func(t *testing.T) {
		result := engine.Convert("查询最近1小时的流量")
		if result.SQL == "" {
			t.Fatal("expected SQL")
		}
		if result.Table != "flows" {
			t.Errorf("expected table flows, got %s", result.Table)
		}
		if result.Confidence <= 0 {
			t.Error("expected confidence > 0")
		}
	})

	t.Run("Convert error analysis Chinese", func(t *testing.T) {
		result := engine.Convert("查询最近24小时错误率超过5%的服务")
		if result.SQL == "" {
			t.Fatal("expected SQL")
		}
		if result.Table == "" {
			t.Error("expected table")
		}
	})

	t.Run("Convert with service filter", func(t *testing.T) {
		result := engine.Convert("查询服务 api 最近1小时的流量")
		if result.SQL == "" {
			t.Fatal("expected SQL")
		}
	})

	t.Run("Convert with aggregation", func(t *testing.T) {
		result := engine.Convert("查询最近1小时的总流量")
		if result.SQL == "" {
			t.Fatal("expected SQL")
		}
	})

	t.Run("Convert with limit", func(t *testing.T) {
		result := engine.Convert("查询前10条告警")
		if result.SQL == "" {
			t.Fatal("expected SQL")
		}
	})

	t.Run("Convert empty query", func(t *testing.T) {
		result := engine.Convert("")
		if result.Confidence > 0 {
			t.Error("expected zero confidence for empty query")
		}
	})

	t.Run("Convert English query", func(t *testing.T) {
		result := engine.Convert("show me the last 1 hour flows")
		if result.SQL == "" {
			t.Fatal("expected SQL for English query")
		}
	})

	t.Run("List schemas", func(t *testing.T) {
		schemas := engine.ListSchemas()
		if len(schemas) == 0 {
			t.Error("expected schemas")
		}
	})

	t.Run("Add synonym", func(t *testing.T) {
		engine.AddSynonym("测试", "test")
		result := engine.Convert("查询测试流量")
		if result.SQL == "" {
			t.Fatal("expected SQL after adding synonym")
		}
	})

	t.Run("Convert latency query with order", func(t *testing.T) {
		result := engine.Convert("查询最近1小时延迟最高的服务")
		if result.SQL == "" {
			t.Fatal("expected SQL")
		}
		if result.Parsed == nil {
			t.Fatal("expected parsed context")
		}
	})
}
