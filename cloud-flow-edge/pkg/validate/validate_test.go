package validate

import (
	"strings"
	"testing"

	edge "cloud-flow/proto"
)

func TestRegisterProbeRequest_Valid(t *testing.T) {
	req := &edge.RegisterProbeRequest{
		ProbeId:  "probe-host-1",
		HostIp:   "10.0.0.1",
		Hostname: "worker-1",
		Version:  "v1.0.0",
	}
	if err := RegisterProbeRequest(req); err != nil {
		t.Fatalf("合法请求不应报错: %v", err)
	}
}

func TestRegisterProbeRequest_EmptyProbeID(t *testing.T) {
	req := &edge.RegisterProbeRequest{ProbeId: ""}
	if err := RegisterProbeRequest(req); err == nil {
		t.Fatal("空 probe_id 应报错")
	}
}

func TestRegisterProbeRequest_LongProbeID(t *testing.T) {
	req := &edge.RegisterProbeRequest{
		ProbeId: strings.Repeat("x", 200),
	}
	if err := RegisterProbeRequest(req); err == nil {
		t.Fatal("超长 probe_id 应报错")
	}
}

func TestRegisterProbeRequest_LongHostname(t *testing.T) {
	req := &edge.RegisterProbeRequest{
		ProbeId:  "p1",
		Hostname: strings.Repeat("h", 300),
	}
	if err := RegisterProbeRequest(req); err == nil {
		t.Fatal("超长 hostname 应报错")
	}
}

func TestHeartbeatRequest_Valid(t *testing.T) {
	req := &edge.HeartbeatRequest{ProbeId: "p1", Timestamp: 1234567890}
	if err := HeartbeatRequest(req); err != nil {
		t.Fatalf("合法心跳不应报错: %v", err)
	}
}

func TestHeartbeatRequest_EmptyProbeID(t *testing.T) {
	req := &edge.HeartbeatRequest{ProbeId: ""}
	if err := HeartbeatRequest(req); err == nil {
		t.Fatal("空 probe_id 应报错")
	}
}

func TestMetricsBatch_Valid(t *testing.T) {
	batch := &edge.MetricsBatch{
		ProbeId: "p1",
		Metrics: []*edge.MetricData{
			{Timestamp: 100, SrcIp: "10.0.0.1", DstIp: "cpu", Protocol: "cpu", Bytes: 5000},
		},
	}
	if err := MetricsBatch(batch); err != nil {
		t.Fatalf("合法 batch 不应报错: %v", err)
	}
}

func TestMetricsBatch_EmptyProbeID(t *testing.T) {
	batch := &edge.MetricsBatch{ProbeId: "", Metrics: []*edge.MetricData{{}}}
	if err := MetricsBatch(batch); err == nil {
		t.Fatal("空 probe_id 应报错")
	}
}

func TestMetricsBatch_EmptyMetrics(t *testing.T) {
	batch := &edge.MetricsBatch{ProbeId: "p1", Metrics: []*edge.MetricData{}}
	if err := MetricsBatch(batch); err == nil {
		t.Fatal("空 metrics 应报错")
	}
}

func TestMetricsBatch_TooManyMetrics(t *testing.T) {
	metrics := make([]*edge.MetricData, maxMetricsPerBatch+1)
	for i := range metrics {
		metrics[i] = &edge.MetricData{Timestamp: 100}
	}
	batch := &edge.MetricsBatch{ProbeId: "p1", Metrics: metrics}
	if err := MetricsBatch(batch); err == nil {
		t.Fatal("超量 metrics 应报错")
	}
}

func TestMetricsBatch_TooManyTags(t *testing.T) {
	tags := make(map[string]string)
	for i := 0; i < maxTagsCount+1; i++ {
		tags[strings.Repeat("k", i+1)] = "v"
	}
	batch := &edge.MetricsBatch{
		ProbeId: "p1",
		Metrics: []*edge.MetricData{{Timestamp: 100, Tags: tags}},
	}
	if err := MetricsBatch(batch); err == nil {
		t.Fatal("超量 tags 应报错")
	}
}

func TestMetricsBatch_LongTagKey(t *testing.T) {
	batch := &edge.MetricsBatch{
		ProbeId: "p1",
		Metrics: []*edge.MetricData{{
			Timestamp: 100,
			Tags:      map[string]string{strings.Repeat("k", 200): "v"},
		}},
	}
	if err := MetricsBatch(batch); err == nil {
		t.Fatal("超长 tag key 应报错")
	}
}

func TestMetricsBatch_LongTagValue(t *testing.T) {
	batch := &edge.MetricsBatch{
		ProbeId: "p1",
		Metrics: []*edge.MetricData{{
			Timestamp: 100,
			Tags:      map[string]string{"key": strings.Repeat("v", 600)},
		}},
	}
	if err := MetricsBatch(batch); err == nil {
		t.Fatal("超长 tag value 应报错")
	}
}

func TestTraceBatch_Valid(t *testing.T) {
	batch := &edge.TraceBatch{
		ProbeId: "p1",
		Spans: []*edge.TraceSpanData{
			{TraceId: "t1", SpanId: "s1", Service: "svc-a"},
		},
	}
	if err := TraceBatch(batch); err != nil {
		t.Fatalf("合法 trace batch 不应报错: %v", err)
	}
}

func TestTraceBatch_TooManySpans(t *testing.T) {
	spans := make([]*edge.TraceSpanData, maxSpansPerBatch+1)
	for i := range spans {
		spans[i] = &edge.TraceSpanData{TraceId: "t1"}
	}
	batch := &edge.TraceBatch{ProbeId: "p1", Spans: spans}
	if err := TraceBatch(batch); err == nil {
		t.Fatal("超量 spans 应报错")
	}
}

func TestProfilingBatch_Valid(t *testing.T) {
	batch := &edge.ProfilingBatch{
		ProbeId: "p1",
		Profiles: []*edge.ProfilingData{
			{Type: "cpu", Stack: "main->foo", Count: 10},
		},
	}
	if err := ProfilingBatch(batch); err != nil {
		t.Fatalf("合法 profiling batch 不应报错: %v", err)
	}
}

func TestProfilingBatch_LongStack(t *testing.T) {
	batch := &edge.ProfilingBatch{
		ProbeId: "p1",
		Profiles: []*edge.ProfilingData{
			{Type: "cpu", Stack: strings.Repeat("x", 70000)},
		},
	}
	if err := ProfilingBatch(batch); err == nil {
		t.Fatal("超长 stack 应报错")
	}
}

func TestProfilingBatch_TooManyProfiles(t *testing.T) {
	profiles := make([]*edge.ProfilingData, maxProfilesPerBatch+1)
	for i := range profiles {
		profiles[i] = &edge.ProfilingData{Type: "cpu"}
	}
	batch := &edge.ProfilingBatch{ProbeId: "p1", Profiles: profiles}
	if err := ProfilingBatch(batch); err == nil {
		t.Fatal("超量 profiles 应报错")
	}
}
