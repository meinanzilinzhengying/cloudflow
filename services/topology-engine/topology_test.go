// P3: 拓扑引擎测试 — 依赖分析、钻取、异常检测、实时更新
package topologyengine

import (
	"context"
	"testing"
	"time"

	graph "github.com/meinanzilinzhengying/cloudflow/services/topology-engine/graph"
	"github.com/meinanzilinzhengying/cloudflow/services/topology-engine/analysis"
	"github.com/meinanzilinzhengying/cloudflow/services/topology-engine/drilldown"
	"github.com/meinanzilinzhengying/cloudflow/services/topology-engine/anomaly"
	"github.com/meinanzilinzhengying/cloudflow/services/topology-engine/realtime"
	"github.com/stretchr/testify/assert"
)

// ============================================================================
// 一、依赖分析测试
// ============================================================================

// buildTestGraph 构建测试拓扑图
func buildTestGraph() *graph.Graph {
	g := graph.NewGraph(graph.GraphTypeService, "test-tenant", 100, 1000)

	// 构建三层拓扑: gateway → service-a → service-b, service-a → service-c

	g.AddOrUpdateNode("gateway", "gateway", "service", "default", nil)
	g.AddOrUpdateNode("service-a", "service-a", "service", "default", nil)
	g.AddOrUpdateNode("service-b", "service-b", "service", "default", nil)
	g.AddOrUpdateNode("service-c", "service-c", "service", "default", nil)
	g.AddOrUpdateNode("service-d", "service-d", "service", "default", nil)

	// gateway -> service-a (高流量)
	g.AccumulateEdge("gateway", "service-a", 1000000, 1000, 50000000, 0) // 50ms latency
	// service-a -> service-b (正常)
	g.AccumulateEdge("service-a", "service-b", 500000, 500, 30000000, 0)
	// service-a -> service-c (高延迟)
	g.AccumulateEdge("service-a", "service-c", 200000, 200, 200000000, 0) // 200ms latency
	// service-c -> service-d (错误)
	g.AccumulateEdge("service-c", "service-d", 100000, 100, 10000000, 50)

	g.RecomputeWeights()
	return g
}

// buildComplexGraph 构建复杂测试图（带环）
func buildComplexGraph() *graph.Graph {
	g := graph.NewGraph(graph.GraphTypeService, "test-tenant", 100, 1000)

	g.AddOrUpdateNode("a", "a", "service", "default", nil)
	g.AddOrUpdateNode("b", "b", "service", "default", nil)
	g.AddOrUpdateNode("c", "c", "service", "default", nil)
	g.AddOrUpdateNode("d", "d", "service", "default", nil)

	g.AccumulateEdge("a", "b", 1000, 10, 10000000, 0)
	g.AccumulateEdge("b", "c", 800, 8, 15000000, 0)
	g.AccumulateEdge("c", "d", 600, 6, 20000000, 0)
	g.AccumulateEdge("d", "a", 400, 4, 25000000, 0) // 环: d -> a
	g.AccumulateEdge("a", "c", 500, 5, 12000000, 0)

	g.RecomputeWeights()
	return g
}

func TestDependencyAnalyzer_AnalyzeUpstream(t *testing.T) {
	g := buildTestGraph()
	da := analysis.NewDependencyAnalyzer()

	// 分析 service-a 的上游依赖
	result := da.AnalyzeUpstream(g, "service-a", 3)
	assert.NotNil(t, result)
	assert.Equal(t, graph.NodeID("service-a"), result.TargetNode)
	assert.GreaterOrEqual(t, len(result.DirectDeps), 1)
	assert.Equal(t, "gateway", string(result.DirectDeps[0].NodeID))
	assert.Equal(t, 1, result.MaxDepth)
}

func TestDependencyAnalyzer_AnalyzeDownstream(t *testing.T) {
	g := buildTestGraph()
	da := analysis.NewDependencyAnalyzer()

	// 分析 service-a 的下游影响
	result := da.AnalyzeDownstream(g, "service-a", 3)
	assert.NotNil(t, result)
	assert.Equal(t, graph.NodeID("service-a"), result.SourceNode)
	assert.GreaterOrEqual(t, len(result.DirectDeps), 2)
	assert.GreaterOrEqual(t, len(result.CriticalDeps), 1)
	assert.GreaterOrEqual(t, result.MaxDepth, 1)
}

func TestCriticalPathAnalyzer_FindCriticalPath(t *testing.T) {
	g := buildTestGraph()
	cpa := analysis.NewCriticalPathAnalyzer()

	// 查找 gateway -> service-d 的关键路径
	path := cpa.FindCriticalPath(g, "gateway", "service-d")
	assert.NotNil(t, path)
	assert.GreaterOrEqual(t, path.TotalHops, 3)
	assert.Greater(t, path.TotalLatency, 0.0)
	assert.NotNil(t, path.Bottleneck)
}

func TestCriticalPathAnalyzer_FindAllPaths(t *testing.T) {
	g := buildComplexGraph()
	cpa := analysis.NewCriticalPathAnalyzer()

	paths := cpa.FindAllPaths(g, "a", "d", 10)
	assert.GreaterOrEqual(t, len(paths), 1)
	for _, p := range paths {
		assert.GreaterOrEqual(t, p.TotalHops, 1)
	}
}

func TestCycleDetector_DetectCycles(t *testing.T) {
	g := buildComplexGraph()
	cd := analysis.NewCycleDetector()

	cycles := cd.DetectCycles(g)
	assert.GreaterOrEqual(t, len(cycles), 1)
	for _, c := range cycles {
		assert.GreaterOrEqual(t, c.Length, 2)
	}
}

func TestFailurePropagationAnalyzer_Analyze(t *testing.T) {
	g := buildTestGraph()
	fpa := analysis.NewFailurePropagationAnalyzer()

	result := fpa.AnalyzeFailurePropagation(g, "service-a", 3)
	assert.NotNil(t, result)
	assert.Equal(t, graph.NodeID("service-a"), result.SourceNode)
	assert.GreaterOrEqual(t, result.TotalAffected, 1)
	assert.GreaterOrEqual(t, result.CriticalCount, 1)
}

func TestDependencyAnalyzer_GenerateHealthReport(t *testing.T) {
	g := buildTestGraph()
	da := analysis.NewDependencyAnalyzer()

	report := da.GenerateHealthReport(g)
	assert.NotNil(t, report)
	assert.Greater(t, report.TotalNodes, 0)
	assert.Greater(t, report.TotalEdges, 0)
	assert.GreaterOrEqual(t, report.CriticalNodes, 0)
	assert.GreaterOrEqual(t, report.Cycles, 0)
}

// ============================================================================
// 二、钻取测试
// ============================================================================

func TestDrillDownAnalyzer_DrillDown(t *testing.T) {
	g := buildTestGraph()
	dda := drilldown.NewDrillDownAnalyzer()

	result := dda.DrillDown(g, "service-a", drilldown.LevelService, drilldown.LevelService)
	assert.NotNil(t, result)
	assert.NotNil(t, result.Context)
	assert.Equal(t, drilldown.LevelService, result.Context.CurrentLevel)
	assert.NotNil(t, result.Summary)
}

func TestDrillDownAnalyzer_BuildAggregateView(t *testing.T) {
	g := buildTestGraph()
	dda := drilldown.NewDrillDownAnalyzer()

	view := dda.BuildAggregateView(g, drilldown.LevelService)
	assert.NotNil(t, view)
	assert.GreaterOrEqual(t, view.NodeCount, 1)
	assert.GreaterOrEqual(t, view.EdgeCount, 0)
}

func TestDrillDownAnalyzer_RecommendDrillDown(t *testing.T) {
	g := buildTestGraph()
	dda := drilldown.NewDrillDownAnalyzer()

	recs := dda.RecommendDrillDown(g, drilldown.LevelService, 5)
	assert.NotNil(t, recs)
	assert.LessOrEqual(t, len(recs), 5)
}

func TestBreadcrumbBuilder(t *testing.T) {
	bb := drilldown.NewBreadcrumbBuilder()
	steps := []drilldown.DrillDownStep{
		{Level: drilldown.LevelNamespace, NodeID: "ns1", NodeName: "namespace-1"},
		{Level: drilldown.LevelService, NodeID: "svc-a", NodeName: "service-a"},
	}
	result := bb.BuildBreadcrumb(steps)
	assert.Equal(t, 2, len(result))

	formatted := drilldown.FormatBreadcrumb(result)
	assert.Contains(t, formatted, "namespace-1")
	assert.Contains(t, formatted, "service-a")
}

func TestLevelOperations(t *testing.T) {
	assert.Equal(t, 0, drilldown.LevelDepth(drilldown.LevelNamespace))
	assert.Equal(t, 1, drilldown.LevelDepth(drilldown.LevelService))
	assert.Equal(t, 2, drilldown.LevelDepth(drilldown.LevelPod))
	assert.Equal(t, 3, drilldown.LevelDepth(drilldown.LevelProcess))
	assert.Equal(t, 4, drilldown.LevelDepth(drilldown.LevelInstance))

	assert.Equal(t, drilldown.LevelService, drilldown.NextFinerLevel(drilldown.LevelNamespace))
	assert.Equal(t, drilldown.LevelPod, drilldown.NextFinerLevel(drilldown.LevelService))
	assert.Equal(t, drilldown.LevelNamespace, drilldown.NextCoarserLevel(drilldown.LevelService))

	assert.True(t, drilldown.LevelDepth("unknown") < 0)
}

// ============================================================================
// 三、异常检测测试
// ============================================================================

func buildAnomalyTestGraph() *graph.Graph {
	g := graph.NewGraph(graph.GraphTypeService, "test-tenant", 100, 1000)

	// 健康节点
	g.AddOrUpdateNode("healthy", "healthy", "service", "default", nil)
	g.AccumulateEdge("gateway", "healthy", 100000, 100, 10000000, 0)

	// 高错误率节点
	g.AddOrUpdateNode("high-error", "high-error", "service", "default", nil)
	g.AccumulateEdge("gateway", "high-error", 100000, 100, 10000000, 90)

	// 高延迟节点
	g.AddOrUpdateNode("high-latency", "high-latency", "service", "default", nil)
	g.AccumulateEdge("gateway", "high-latency", 100000, 100, 1500000000, 0)

	// 流量下降节点
	g.AddOrUpdateNode("low-traffic", "low-traffic", "service", "default", nil)
	g.AccumulateEdge("gateway", "low-traffic", 100, 1, 10000000, 0)

	g.RecomputeWeights()
	return g
}

func TestAnomalyDetector_DetectNodeAnomalies(t *testing.T) {
	g := buildAnomalyTestGraph()
	ad := anomaly.NewAnomalyDetector(anomaly.DefaultAnomalyConfig())

	anomalies := ad.DetectNodeAnomalies(g)
	assert.GreaterOrEqual(t, len(anomalies), 1)

	// 检查是否检测到高错误率
	hasErrorAnomaly := false
	hasLatencyAnomaly := false
	for _, a := range anomalies {
		if a.Type == anomaly.TypeErrorRate && a.Severity == anomaly.SeverityCritical {
			hasErrorAnomaly = true
		}
		if a.Type == anomaly.TypeLatency && a.Severity == anomaly.SeverityCritical {
			hasLatencyAnomaly = true
		}
	}
	assert.True(t, hasErrorAnomaly, "应检测到高错误率异常")
	assert.True(t, hasLatencyAnomaly, "应检测到高延迟异常")
}

func TestAnomalyDetector_DetectEdgeAnomalies(t *testing.T) {
	g := buildAnomalyTestGraph()
	ad := anomaly.NewAnomalyDetector(anomaly.DefaultAnomalyConfig())

	anomalies := ad.DetectEdgeAnomalies(g)
	assert.GreaterOrEqual(t, len(anomalies), 1)
}

func TestAnomalyDetector_GenerateReport(t *testing.T) {
	g := buildAnomalyTestGraph()
	ad := anomaly.NewAnomalyDetector(anomaly.DefaultAnomalyConfig())

	report := ad.GenerateReport(g)
	assert.NotNil(t, report)
	assert.GreaterOrEqual(t, report.TotalAnomalies, 1)
	assert.GreaterOrEqual(t, report.CriticalCount, 1)
	assert.GreaterOrEqual(t, len(report.TopIssues), 1)
	assert.NotNil(t, report.PropagationPath)
}

func TestAnomalyDetector_VisualMarker(t *testing.T) {
	ad := anomaly.NewAnomalyDetector(anomaly.DefaultAnomalyConfig())
	marker := ad.CreateVisualMarker(anomaly.SeverityCritical, anomaly.TypeErrorRate)
	assert.NotNil(t, marker)
	assert.Equal(t, "#FF4444", marker.Color)
	assert.True(t, marker.Blinking)
	assert.Equal(t, "⚠️", marker.Icon)
}

func TestTopologyMarker(t *testing.T) {
	g := buildAnomalyTestGraph()
	ad := anomaly.NewAnomalyDetector(anomaly.DefaultAnomalyConfig())
	anomalies := ad.DetectNodeAnomalies(g)

	tm := anomaly.NewTopologyMarker()
	marked := tm.ApplyMarks(g, anomalies)
	assert.NotNil(t, marked)
	assert.GreaterOrEqual(t, marked.AnomalyCount, 1)
	assert.GreaterOrEqual(t, len(marked.Nodes), 1)
}

func TestRootCauseAnalyzer(t *testing.T) {
	g := buildAnomalyTestGraph()
	ad := anomaly.NewAnomalyDetector(anomaly.DefaultAnomalyConfig())
	anomalies := ad.DetectNodeAnomalies(g)

	rca := anomaly.NewRootCauseAnalyzer()
	result := rca.AnalyzeRootCause(g, anomalies)
	assert.NotNil(t, result)
	assert.GreaterOrEqual(t, len(result.Suggestions), 1)
}

func TestMetricHistory(t *testing.T) {
	h := &anomaly.MetricHistory{
		NodeID:  "test-node",
		MaxSize: 10,
	}

	for i := 0; i < 15; i++ {
		h.AddRecord(0.01*float64(i), float64(i)*10, uint64(i)*1000, uint64(i), int64(i))
	}

	assert.Equal(t, 10, len(h.ErrorRates))
	assert.Greater(t, h.AvgErrorRate(), 0.0)
	assert.Greater(t, h.AvgLatency(), 0.0)
	assert.Greater(t, h.AvgTraffic(), uint64(0))
	assert.GreaterOrEqual(t, h.StdDevLatency(), 0.0)
}

// ============================================================================
// 四、实时更新测试
// ============================================================================

func TestRealtimeEngine_IncrementalUpdate(t *testing.T) {
	re := realtime.NewRealtimeEngine(realtime.DefaultRealtimeConfig())
	re.Start(context.Background())
	defer re.Stop()

	changes := []realtime.TopologyChange{
		{
			Type:     realtime.ChangeTypeAddNode,
			NodeID:   "node-1",
			NodeName: "node-1",
			NodeType: "service",
		},
		{
			Type:   realtime.ChangeTypeAddEdge,
			Source: "node-1",
			Target: "node-2",
			Bytes:  1000,
		},
	}

	version, err := re.IncrementalUpdate("tenant-1", "service", changes)
	assert.NoError(t, err)
	assert.Greater(t, version, uint64(0))

	// 验证版本
	v := re.GetVersion("tenant-1", "service")
	assert.Equal(t, version, v)
}

func TestRealtimeEngine_FullRefresh(t *testing.T) {
	re := realtime.NewRealtimeEngine(realtime.DefaultRealtimeConfig())
	re.Start(context.Background())
	defer re.Stop()

	g := graph.NewGraph(graph.GraphTypeService, "tenant-1", 100, 1000)
	g.AddOrUpdateNode("a", "a", "service", "default", nil)
	g.AddOrUpdateNode("b", "b", "service", "default", nil)
	g.AccumulateEdge("a", "b", 1000, 10, 10000000, 0)
	g.RecomputeWeights()

	version, err := re.FullRefresh("tenant-1", "service", g)
	assert.NoError(t, err)
	assert.Greater(t, version, uint64(0))

	// 验证图
	retrieved, ok := re.GetGraph("tenant-1", "service")
	assert.True(t, ok)
	assert.NotNil(t, retrieved)
}

func TestRealtimeEngine_VersionConsistency(t *testing.T) {
	re := realtime.NewRealtimeEngine(realtime.DefaultRealtimeConfig())

	changes := []realtime.TopologyChange{
		{Type: realtime.ChangeTypeAddNode, NodeID: "n1", NodeName: "n1"},
	}

	v1, _ := re.IncrementalUpdate("t1", "service", changes)
	v2, _ := re.IncrementalUpdate("t1", "service", changes)

	assert.Greater(t, v2, v1)
	assert.Equal(t, 1, re.CompareVersion("t1", "service", v1))
	assert.Equal(t, 0, re.CompareVersion("t1", "service", v2))
	assert.True(t, re.IsStale("t1", "service", v1))
	assert.False(t, re.IsStale("t1", "service", v2))
}

func TestEventBus(t *testing.T) {
	eb := realtime.NewEventBus(10)

	ch := eb.Subscribe("t1:service")
	assert.NotNil(t, ch)

	eb.Publish(realtime.TopologyEvent{
		Type:      realtime.EventTypeIncrementalUpdate,
		TenantID:  "t1",
		GraphType: "service",
		Version:   1,
	})

	select {
	case event := <-ch:
		assert.Equal(t, realtime.EventTypeIncrementalUpdate, event.Type)
		assert.Equal(t, uint64(1), event.Version)
	case <-time.After(100 * time.Millisecond):
		t.Error("未收到事件")
	}

	eb.Unsubscribe("t1:service", ch)
}

func TestSubscriptionManager(t *testing.T) {
	sm := realtime.NewSubscriptionManager()

	var notifiedVersion uint64
	var notified bool
	callback := func(v uint64) {
		notifiedVersion = v
		notified = true
	}

	sm.Subscribe("t1:service", callback)
	sm.Notify("t1:service", 42)

	// 等待异步通知
	time.Sleep(50 * time.Millisecond)
	assert.True(t, notified)
	assert.Equal(t, uint64(42), notifiedVersion)
}

func TestChangeLog(t *testing.T) {
	cl := realtime.NewChangeLog(10)

	changes := []realtime.TopologyChange{
		{Type: realtime.ChangeTypeAddNode, NodeID: "n1"},
	}
	cl.Append("t1:service", 1, changes)
	cl.Append("t1:service", 2, changes)

	result, err := cl.GetChanges("t1:service", 0, 2)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(result), 1)
}

// ============================================================================
// 五、综合集成测试
// ============================================================================

func TestTopologyEngine_Integration(t *testing.T) {
	g := buildTestGraph()

	// 1. 依赖分析
	da := analysis.NewDependencyAnalyzer()
	upstream := da.AnalyzeUpstream(g, "service-a", 3)
	assert.NotNil(t, upstream)

	downstream := da.AnalyzeDownstream(g, "service-a", 3)
	assert.NotNil(t, downstream)

	// 2. 异常检测
	ad := anomaly.NewAnomalyDetector(anomaly.DefaultAnomalyConfig())
	report := ad.GenerateReport(g)
	assert.NotNil(t, report)

	// 3. 钻取分析
	dda := drilldown.NewDrillDownAnalyzer()
	view := dda.BuildAggregateView(g, drilldown.LevelService)
	assert.NotNil(t, view)
	assert.GreaterOrEqual(t, view.NodeCount, 1)

	// 4. 健康报告
	healthReport := da.GenerateHealthReport(g)
	assert.NotNil(t, healthReport)
	assert.Greater(t, healthReport.TotalNodes, 0)
}

func TestTopologyEngine_CycleDetection(t *testing.T) {
	g := buildComplexGraph()
	cd := analysis.NewCycleDetector()

	cycles := cd.DetectCycles(g)
	assert.GreaterOrEqual(t, len(cycles), 1)

	for _, c := range cycles {
		assert.GreaterOrEqual(t, c.Length, 2)
		t.Logf("检测到循环依赖: %s", c.String())
	}
}

func TestTopologyEngine_CriticalPath(t *testing.T) {
	g := buildTestGraph()
	cpa := analysis.NewCriticalPathAnalyzer()

	path := cpa.FindCriticalPath(g, "gateway", "service-d")
	assert.NotNil(t, path)
	assert.NotNil(t, path.Bottleneck)
	assert.GreaterOrEqual(t, path.TotalHops, 3)
	assert.Greater(t, path.TotalLatency, 0.0)

	// 验证瓶颈节点的贡献度
	if path.Bottleneck != nil {
		assert.Greater(t, path.Bottleneck.LatencyContribution, 0.0)
	}
}
