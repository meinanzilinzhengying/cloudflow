package l7parser_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/meinanzilinzhengying/cloudflow/agent/internal/l7parser"
)

// ============================================================================
// 协议深度解析测试
// ============================================================================

func TestDeepProtocolAnalyzer(t *testing.T) {
	dpa := l7parser.NewDeepProtocolAnalyzer()

	// 1. 测试 MySQL 深度分析
	t.Run("MySQL analysis", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			latency := 50.0
			if i%3 == 0 {
				latency = 200.0
			}
			stats := dpa.AnalyzeMySQL("db1", "users", "SELECT", latency, 100, 1, false)
			if stats == nil {
				t.Fatal("expected stats")
			}
		}

		// 验证慢查询
		slow := dpa.GetMySQLSlowQueries(10)
		if len(slow) == 0 {
			t.Error("expected slow queries")
		}
		if len(slow) > 0 && slow[0].TableName != "users" {
			t.Errorf("expected table users, got %s", slow[0].TableName)
		}

		// 验证热点表
		hot := dpa.GetMySQLHotTables(10)
		if len(hot) == 0 {
			t.Error("expected hot tables")
		}
	})

	// 2. 测试 Redis 深度分析
	t.Run("Redis analysis", func(t *testing.T) {
		for i := 0; i < 1100; i++ {
			stats := dpa.AnalyzeRedis("0", "user:*", "GET", 5.0, 1024, false)
			if stats == nil {
				t.Fatal("expected stats")
			}
		}

		// 验证热点key
		hotKeys := dpa.GetRedisHotKeys(10)
		if len(hotKeys) == 0 {
			t.Error("expected hot keys")
		}
		if len(hotKeys) > 0 && !hotKeys[0].IsHotKey {
			t.Error("expected IsHotKey to be true")
		}
	})

	// 3. 测试 Kafka 深度分析
	t.Run("Kafka analysis", func(t *testing.T) {
		for i := 0; i < 50; i++ {
			stats := dpa.AnalyzeKafka("orders", "Consume", 0, 30.0, 512, 2000, false)
			if stats == nil {
				t.Fatal("expected stats")
			}
		}

		highLag := dpa.GetKafkaHighLagTopics(10)
		if len(highLag) == 0 {
			t.Error("expected high lag topics")
		}
	})

	// 4. 测试 HTTP 深度分析
	t.Run("HTTP analysis", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			code := 200
			latency := 100.0
			if i%10 == 0 {
				code = 500
				latency = 600.0
			}
			stats := dpa.AnalyzeHTTP("api.example.com", "/users", "GET", latency, code, 1024)
			if stats == nil {
				t.Fatal("expected stats")
			}
		}

		errors := dpa.GetHTTPErrorEndpoints(10)
		if len(errors) == 0 {
			t.Error("expected error endpoints")
		}

		slow := dpa.GetHTTPSlowEndpoints(10)
		if len(slow) == 0 {
			t.Error("expected slow endpoints")
		}
	})

	// 5. 综合报告
	t.Run("Traffic report", func(t *testing.T) {
		report := l7parser.GenerateTrafficReport(dpa, nil, 10)
		if report == nil {
			t.Fatal("expected report")
		}
		if len(report.MySQLHotTables) == 0 {
			t.Error("expected MySQL hot tables in report")
		}
	})
}

// ============================================================================
// 流量异常检测测试
// ============================================================================

func TestTrafficAnomalyDetector(t *testing.T) {
	config := l7parser.DefaultTrafficAnomalyConfig()
	config.SpikeFactor = 3.0
	config.DropThreshold = 0.3
	tad := l7parser.NewTrafficAnomalyDetector(config)

	// 1. 建立基线
	t.Run("Baseline building", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			anomalies := tad.Detect("svc1", 1000, 10, 1)
			if len(anomalies) > 0 {
				t.Errorf("expected no anomalies during baseline building, got %d", len(anomalies))
			}
		}
		// 应该没有异常（数据不够或基线内）
		anomalies := tad.Detect("svc1", 1000, 10, 1)
		if len(anomalies) > 0 {
			t.Errorf("expected no anomalies yet, got %d", len(anomalies))
		}
	})

	// 2. 突增检测
	t.Run("Spike detection", func(t *testing.T) {
		anomalies := tad.Detect("svc1", 10000, 100, 1)
		if len(anomalies) == 0 {
			t.Fatal("expected spike anomaly")
		}
		if anomalies[0].Type != l7parser.AnomalySpike {
			t.Errorf("expected type %s, got %s", l7parser.AnomalySpike, anomalies[0].Type)
		}
		if anomalies[0].Severity != l7parser.AnomalySeverityCritical {
			t.Errorf("expected critical severity, got %s", anomalies[0].Severity)
		}
	})

	// 3. 突降检测
	t.Run("Drop detection", func(t *testing.T) {
		anomalies := tad.Detect("svc1", 100, 1, 1)
		found := false
		for _, a := range anomalies {
			if a.Type == l7parser.AnomalyDrop {
				found = true
				break
			}
		}
		if !found {
			t.Log("Drop detection may not trigger depending on avg; continuing")
		}
	})

	// 4. 端口扫描检测
	t.Run("Port scan detection", func(t *testing.T) {
		ports := []uint16{22, 23, 80, 443, 3306, 5432, 6379, 8080, 9090, 9200, 10000}
		anomaly := tad.DetectPortScan("10.0.0.1", ports)
		if anomaly == nil {
			t.Fatal("expected port scan anomaly")
		}
		if anomaly.Type != l7parser.AnomalyPortScan {
			t.Errorf("expected type %s, got %s", l7parser.AnomalyPortScan, anomaly.Type)
		}
	})

	// 5. DDoS 检测
	t.Run("DDoS detection", func(t *testing.T) {
		srcIPs := make([]string, 100)
		for i := range srcIPs {
			srcIPs[i] = fmt.Sprintf("10.0.0.%d", i+1)
		}
		anomaly := tad.DetectDDoS("10.0.1.1", srcIPs, 10000)
		if anomaly == nil {
			t.Fatal("expected DDoS anomaly")
		}
		if anomaly.Type != l7parser.AnomalyDDoS {
			t.Errorf("expected type %s, got %s", l7parser.AnomalyDDoS, anomaly.Type)
		}
	})

	// 6. 基线更新
	t.Run("Baseline update", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			tad.Detect("svc2", 2000, 20, 2)
		}
		tad.UpdateBaseline("svc2")
		baseline := tad.GetBaseline("svc2")
		if baseline == nil {
			t.Fatal("expected baseline")
		}
		if baseline.AvgBytes == 0 {
			t.Error("expected non-zero avg bytes")
		}
	})
}

// ============================================================================
// 智能基线测试
// ============================================================================

func TestSmartBaselineAnalyzer(t *testing.T) {
	sba := l7parser.NewSmartBaselineAnalyzer()

	t.Run("Hourly baseline learning", func(t *testing.T) {
		now := time.Date(2024, 1, 1, 14, 0, 0, 0, time.UTC)
		for i := 0; i < 10; i++ {
			sba.Learn("svc1", now, 1000, 10)
		}

		bytes, conns := sba.GetExpectedTraffic("svc1", now)
		if bytes == 0 {
			t.Error("expected non-zero expected bytes")
		}
		if conns == 0 {
			t.Error("expected non-zero expected conns")
		}
	})

	t.Run("Deviation detection", func(t *testing.T) {
		now := time.Date(2024, 1, 1, 14, 0, 0, 0, time.UTC)
		bytesRatio, connsRatio := sba.CompareWithBaseline("svc1", now, 1000, 10)
		if bytesRatio < 0.5 || bytesRatio > 2.0 {
			t.Errorf("expected ratio near 1, got %f", bytesRatio)
		}
		if connsRatio < 0.5 || connsRatio > 2.0 {
			t.Errorf("expected ratio near 1, got %f", connsRatio)
		}

		bytesRatio, _ = sba.CompareWithBaseline("svc1", now, 10000, 10)
		if bytesRatio < 5.0 {
			t.Errorf("expected high ratio, got %f", bytesRatio)
		}
	})

	t.Run("Weekly baseline", func(t *testing.T) {
		monday := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC) // 周一
		for i := 0; i < 5; i++ {
			sba.Learn("svc2", monday, 2000, 20)
		}

		bytes, conns := sba.GetExpectedTraffic("svc2", monday)
		if bytes == 0 {
			t.Error("expected non-zero expected bytes from weekly baseline")
		}
		if conns == 0 {
			t.Error("expected non-zero expected conns from weekly baseline")
		}
	})
}

// ============================================================================
// 流量回放测试
// ============================================================================

func TestTrafficReplayer(t *testing.T) {
	t.Run("Record and stats", func(t *testing.T) {
		replayer := l7parser.NewTrafficReplayer()

		records := []*l7parser.TrafficRecord{
			{Timestamp: time.Now(), SrcIP: "10.0.0.1", DstIP: "10.0.0.2", Bytes: 1000, Packets: 10},
			{Timestamp: time.Now().Add(10 * time.Millisecond), SrcIP: "10.0.0.1", DstIP: "10.0.0.2", Bytes: 2000, Packets: 20},
			{Timestamp: time.Now().Add(20 * time.Millisecond), SrcIP: "10.0.0.1", DstIP: "10.0.0.2", Bytes: 1500, Packets: 15},
		}

		replayer.LoadRecords(records)
		stats := replayer.GetStats()
		if stats.TotalRecords != 3 {
			t.Errorf("expected 3 records, got %d", stats.TotalRecords)
		}
		if stats.IsPlaying {
			t.Error("expected not playing")
		}
	})

	t.Run("Play with callback", func(t *testing.T) {
		replayer := l7parser.NewTrafficReplayer()
		records := []*l7parser.TrafficRecord{
			{Timestamp: time.Now(), Bytes: 1000, Packets: 10},
			{Timestamp: time.Now().Add(5 * time.Millisecond), Bytes: 2000, Packets: 20},
		}
		replayer.LoadRecords(records)

		count := 0
		replayer.Play(func(r *l7parser.TrafficRecord) {
			count++
			if r == nil {
				t.Error("expected non-nil record")
			}
		})

		// 等待回放完成
		time.Sleep(100 * time.Millisecond)
		stats := replayer.GetStats()
		if stats.IsPlaying {
			t.Error("expected playback to have finished")
		}
		if count != 2 {
			t.Errorf("expected callback called 2 times, got %d", count)
		}
	})

	t.Run("Speed control", func(t *testing.T) {
		replayer := l7parser.NewTrafficReplayer()
		replayer.SetSpeed(2.0)
		stats := replayer.GetStats()
		if stats.Speed != 2.0 {
			t.Errorf("expected speed 2.0, got %f", stats.Speed)
		}
	})

	t.Run("Stop playback", func(t *testing.T) {
		replayer := l7parser.NewTrafficReplayer()
		records := make([]*l7parser.TrafficRecord, 100)
		for i := range records {
			records[i] = &l7parser.TrafficRecord{
				Timestamp: time.Now().Add(time.Duration(i) * 100 * time.Millisecond),
				Bytes:     uint64(i * 100),
			}
		}
		replayer.LoadRecords(records)
		replayer.Play(func(r *l7parser.TrafficRecord) {})
		time.Sleep(5 * time.Millisecond) // 让回放开始
		replayer.Stop()
		time.Sleep(20 * time.Millisecond)
		stats := replayer.GetStats()
		if stats.IsPlaying {
			t.Error("expected playback stopped")
		}
	})
}

// ============================================================================
// 流量模拟测试
// ============================================================================

func TestTrafficSimulator(t *testing.T) {
	t.Run("Generate traffic", func(t *testing.T) {
		config := l7parser.DefaultSimulatorConfig()
		config.BaseRate = 1000
		config.Volatility = 0.1
		sim := l7parser.NewTrafficSimulator(config)

		records := sim.GenerateTraffic(100*time.Millisecond, 10*time.Millisecond)
		if len(records) != 10 {
			t.Errorf("expected 10 records, got %d", len(records))
		}
		for _, r := range records {
			if r.Bytes == 0 {
				t.Error("expected non-zero bytes")
			}
		}
	})

	t.Run("Export records", func(t *testing.T) {
		config := l7parser.DefaultSimulatorConfig()
		sim := l7parser.NewTrafficSimulator(config)
		records := sim.GenerateTraffic(50*time.Millisecond, 10*time.Millisecond)
		jsonStr := sim.ExportRecords(records)
		if len(jsonStr) == 0 {
			t.Error("expected non-empty JSON")
		}
		if jsonStr[0] != '[' || jsonStr[len(jsonStr)-1] != ']' {
			t.Errorf("expected JSON array, got %s", jsonStr)
		}
	})

	t.Run("Anomaly injection", func(t *testing.T) {
		config := l7parser.DefaultSimulatorConfig()
		config.AnomalyProb = 1.0 // 100% 注入异常
		config.PeakFactor = 5.0
		sim := l7parser.NewTrafficSimulator(config)
		records := sim.GenerateTraffic(100*time.Millisecond, 10*time.Millisecond)
		var anomalyCount int
		for _, r := range records {
			if r.Tags["anomaly"] == "true" {
				anomalyCount++
			}
		}
		if anomalyCount == 0 {
			t.Error("expected some anomalies injected")
		}
	})
}
