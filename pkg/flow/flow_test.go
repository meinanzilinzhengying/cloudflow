package flow

import (
	"testing"
	"unsafe"

	"cloud-flow-agent/internal/ebpfconsumer/pool"
	edge "cloud-flow/proto"
)

func TestUnifiedFlow_BasicCreation(t *testing.T) {
	f := New()
	if f.SchemaVersion != CurrentSchema {
		t.Fatalf("expected schema version %d, got %d", CurrentSchema, f.SchemaVersion)
	}
	if f.Timestamp != 0 {
		t.Fatal("expected zero timestamp")
	}
}

func TestUnifiedFlow_FluentSetters(t *testing.T) {
	f := New().
		SetL3("192.168.1.1", "10.0.0.1").
		SetL4(12345, 80, ProtoTCP, 0x02).
		SetL7(ProtoHTTP, 1, "/api/v1/users", 200).
		SetProcess(1234, "nginx", "nginx").
		SetK8s("web-pod-abc", "default", "web-deploy", "web-svc", "node-1").
		SetTrace("0123456789abcdef0123456789abcdef", "0102030405060708090a0b0c0d0e0f10", "").
		SetHost("host-1", "worker-01").
		SetTenant("tenant-abc").
		SetMetrics(1024, 10, 5000000, DirIngress)

	// 验证 L3
	if f.SrcIP.String() != "192.168.1.1" {
		t.Fatalf("expected src_ip=192.168.1.1, got %s", f.SrcIP.String())
	}
	if f.DstIP.String() != "10.0.0.1" {
		t.Fatalf("expected dst_ip=10.0.0.1, got %s", f.DstIP.String())
	}
	if f.IPVersion != 4 {
		t.Fatalf("expected ip_version=4, got %d", f.IPVersion)
	}

	// 验证 L4
	if f.SrcPort != 12345 {
		t.Fatalf("expected src_port=12345, got %d", f.SrcPort)
	}
	if f.DstPort != 80 {
		t.Fatalf("expected dst_port=80, got %d", f.DstPort)
	}
	if f.Protocol != ProtoTCP {
		t.Fatal("expected protocol=tcp")
	}

	// 验证 L7
	if f.L7Protocol != ProtoHTTP {
		t.Fatal("expected l7_protocol=http")
	}
	if f.Path.String() != "/api/v1/users" {
		t.Fatalf("expected path=/api/v1/users, got %s", f.Path.String())
	}
	if f.StatusCode != 200 {
		t.Fatalf("expected status_code=200, got %d", f.StatusCode)
	}

	// 验证 Process
	if f.PID != 1234 {
		t.Fatalf("expected pid=1234, got %d", f.PID)
	}

	// 验证 K8s
	if f.Pod.String() != "web-pod-abc" {
		t.Fatalf("expected pod=web-pod-abc, got %s", f.Pod.String())
	}
	if f.Namespace.String() != "default" {
		t.Fatalf("expected namespace=default, got %s", f.Namespace.String())
	}

	// 验证 Trace
	if f.TraceID.String() != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("expected trace_id, got %s", f.TraceID.String())
	}

	// 验证 Tenant
	if f.TenantID.String() != "tenant-abc" {
		t.Fatalf("expected tenant_id=tenant-abc, got %s", f.TenantID.String())
	}

	// 验证 Metrics
	if f.Bytes != 1024 {
		t.Fatalf("expected bytes=1024, got %d", f.Bytes)
	}
	if f.LatencyNs != 5000000 {
		t.Fatalf("expected latency_ns=5000000, got %d", f.LatencyNs)
	}

	// 验证 Presence
	if !f.IsPresent(FieldSrcIP) {
		t.Fatal("expected FieldSrcIP to be present")
	}
	if !f.IsPresent(FieldTraceID) {
		t.Fatal("expected FieldTraceID to be present")
	}
	if !f.IsPresent(FieldTenantID) {
		t.Fatal("expected FieldTenantID to be present")
	}
}

func TestUnifiedFlow_IPv6(t *testing.T) {
	f := New()
	f.SetL3("2001:db8::1", "2001:db8::2")

	if f.IPVersion != 6 {
		t.Fatalf("expected ip_version=6, got %d", f.IPVersion)
	}
	if !f.SrcIP.IsIPv4() {
		t.Log("IPv6 address correctly not IPv4")
	}
	if f.SrcIP.String() != "2001:db8::1" {
		t.Fatalf("expected 2001:db8::1, got %s", f.SrcIP.String())
	}
}

func TestUnifiedFlow_Serialization(t *testing.T) {
	f := New().
		SetL3("192.168.1.1", "10.0.0.1").
		SetL4(12345, 80, ProtoTCP, 0x02).
		SetMetrics(1024, 10, 5000000, DirIngress)

	data := f.Serialize()
	if len(data) == 0 {
		t.Fatal("serialization returned empty data")
	}

	// 反序列化
	f2 := &UnifiedFlow{}
	err := f2.Deserialize(data)
	if err != nil {
		t.Fatalf("deserialization failed: %v", err)
	}

	if f2.SrcIP.String() != f.SrcIP.String() {
		t.Fatalf("src_ip mismatch: %s vs %s", f2.SrcIP.String(), f.SrcIP.String())
	}
	if f2.SrcPort != f.SrcPort {
		t.Fatalf("src_port mismatch: %d vs %d", f2.SrcPort, f.SrcPort)
	}
	if f2.Bytes != f.Bytes {
		t.Fatalf("bytes mismatch: %d vs %d", f2.Bytes, f.Bytes)
	}
}

func TestUnifiedFlow_SerializationRoundTrip(t *testing.T) {
	f := New().
		SetL3("10.0.0.1", "10.0.0.2").
		SetL4(80, 443, ProtoTCP, 0x12).
		SetL7(ProtoHTTP, 0, "/health", 200).
		SetProcess(100, "myapp", "myapp").
		SetK8s("pod-1", "prod", "deploy-1", "svc-1", "node-1").
		SetTrace("abc123", "def456", "").
		SetTenant("t1").
		SetMetrics(500, 5, 100000, DirEgress)

	data := f.Serialize()
	f2 := &UnifiedFlow{}
	f2.Deserialize(data)

	// 验证所有字段
	if f2.SrcIP.String() != "10.0.0.1" {
		t.Fatalf("src_ip: %s", f2.SrcIP.String())
	}
	if f2.DstPort != 443 {
		t.Fatalf("dst_port: %d", f2.DstPort)
	}
	if f2.L7Protocol != ProtoHTTP {
		t.Fatal("l7_protocol mismatch")
	}
	if f2.Pod.String() != "pod-1" {
		t.Fatalf("pod: %s", f2.Pod.String())
	}
	if f2.TraceID.String() != "abc123" {
		t.Fatalf("trace_id: %s", f2.TraceID.String())
	}
	if f2.TenantID.String() != "t1" {
		t.Fatalf("tenant_id: %s", f2.TenantID.String())
	}
}

func TestConverter_ParsedFlowToUnified(t *testing.T) {
	c := NewConverter()

	parsed := &pool.ParsedFlow{
		SrcIP:     [4]byte{192, 168, 1, 100},
		DstIP:     [4]byte{10, 0, 0, 1},
		SrcPort:   54321,
		DstPort:   80,
		Protocol:  6, // TCP
		TCPFlags:  0x02,
		Bytes:     1024,
		Packets:   10,
		LatencyNs: 5000000,
		Direction: 1,
		CPU:       2,
	}

	raw := &pool.RawEvent{
		Type: pool.EventTypeTCP,
		CPU:  2,
	}

	f := c.ParsedFlowToUnified(raw, parsed)

	if f.SrcIP.String() != "192.168.1.100" {
		t.Fatalf("src_ip: %s", f.SrcIP.String())
	}
	if f.DstIP.String() != "10.0.0.1" {
		t.Fatalf("dst_ip: %s", f.DstIP.String())
	}
	if f.Protocol != ProtoTCP {
		t.Fatal("expected TCP")
	}
	if f.Bytes != 1024 {
		t.Fatalf("bytes: %d", f.Bytes)
	}
}

func TestConverter_MetricDataToUnified(t *testing.T) {
	c := NewConverter()

	m := &edge.MetricData{
		Timestamp: 1700000000000,
		SrcIp:     "192.168.1.1",
		DstIp:     "10.0.0.1",
		SrcPort:   80,
		DstPort:   443,
		Protocol:  edge.ProtocolTCP,
		Bytes:     2048,
		Packets:   20,
		Latency:   5000,
		Tags:      map[string]string{"service": "web"},
	}

	f := c.MetricDataToUnified(m)

	if f.SrcIP.String() != "192.168.1.1" {
		t.Fatalf("src_ip: %s", f.SrcIP.String())
	}
	if f.Protocol != ProtoTCP {
		t.Fatal("expected TCP")
	}
	if f.Tags.Get("service") != "web" {
		t.Fatalf("expected service=web, got %s", f.Tags.Get("service"))
	}
}

func TestConverter_UnifiedToMetricData(t *testing.T) {
	c := NewConverter()

	f := New().
		SetL3("192.168.1.1", "10.0.0.1").
		SetL4(80, 443, ProtoTCP, 0).
		SetMetrics(2048, 20, 5000000, DirIngress).
		SetK8s("pod-1", "default", "deploy-1", "svc-1", "node-1").
		SetTrace("abc123", "def456", "").
		SetTenant("t1")

	m := c.UnifiedToMetricData(f)

	if m.SrcIp != "192.168.1.1" {
		t.Fatalf("src_ip: %s", m.SrcIp)
	}
	if m.Bytes != 2048 {
		t.Fatalf("bytes: %d", m.Bytes)
	}
	if m.Tags["pod"] != "pod-1" {
		t.Fatalf("pod: %s", m.Tags["pod"])
	}
	if m.Tags["trace_id"] != "abc123" {
		t.Fatalf("trace_id: %s", m.Tags["trace_id"])
	}
	if m.Tags["tenant_id"] != "t1" {
		t.Fatalf("tenant_id: %s", m.Tags["tenant_id"])
	}
}

func TestRouter_BasicRouting(t *testing.T) {
	r := NewRouter()

	// TCP 流 -> flow.l4
	f1 := New().SetL3("10.0.0.1", "10.0.0.2").SetL4(80, 443, ProtoTCP, 0)
	d1 := r.Route(f1)
	if d1.Primary != RouteFlowL4 {
		t.Fatalf("expected RouteFlowL4, got %s", d1.Primary)
	}

	// HTTP 流 -> flow.l7
	f2 := New().SetL3("10.0.0.1", "10.0.0.2").SetL4(80, 443, ProtoTCP, 0).SetL7(ProtoHTTP, 1, "/api", 200)
	d2 := r.Route(f2)
	if d2.Primary != RouteFlowL7 {
		t.Fatalf("expected RouteFlowL7, got %s", d2.Primary)
	}

	// Trace -> traces
	f3 := New().SetL3("10.0.0.1", "10.0.0.2").SetL4(80, 443, ProtoTCP, 0).SetTrace("abc", "def", "")
	d3 := r.Route(f3)
	if d3.Primary != RouteTraces {
		t.Fatalf("expected RouteTraces, got %s", d3.Primary)
	}
}

func TestPresence_Bitmap(t *testing.T) {
	var p Presence
	p.Set(FieldSrcIP)
	p.Set(FieldDstIP)
	p.Set(FieldTraceID)

	if !p.IsSet(FieldSrcIP) {
		t.Fatal("FieldSrcIP should be set")
	}
	if !p.IsSet(FieldDstIP) {
		t.Fatal("FieldDstIP should be set")
	}
	if !p.IsSet(FieldTraceID) {
		t.Fatal("FieldTraceID should be set")
	}
	if p.IsSet(FieldTenantID) {
		t.Fatal("FieldTenantID should not be set")
	}
}

func TestFlowBatch(t *testing.T) {
	b := NewFlowBatch(FlowDataL4, "probe-1", 10)

	for i := 0; i < 5; i++ {
		f := New().SetL3("10.0.0.1", "10.0.0.2").SetL4(uint16(1000+i), 80, ProtoTCP, 0)
		b.Add(f)
	}

	b.Finalize()

	if b.Header.Count != 5 {
		t.Fatalf("expected count=5, got %d", b.Header.Count)
	}
	if b.ChecksumString() == "" {
		t.Fatal("expected non-empty checksum")
	}
}

func TestSchemaRegistry(t *testing.T) {
	r := NewSchemaRegistry()

	info, ok := r.Get(SchemaV1)
	if !ok {
		t.Fatal("V1 schema should exist")
	}
	if info.Version != SchemaV1 {
		t.Fatalf("expected version %d, got %d", SchemaV1, info.Version)
	}
	if len(info.Fields) == 0 {
		t.Fatal("expected non-empty fields")
	}

	if !r.IsCompatible(SchemaV1) {
		t.Fatal("V1 should be compatible")
	}
	if r.IsCompatible(999) {
		t.Fatal("999 should not be compatible")
	}
}

func TestMemoryAlignment(t *testing.T) {
	size := SizeOf()
	align := AlignOf()

	t.Logf("UnifiedFlow size: %d bytes", size)
	t.Logf("UnifiedFlow align: %d bytes", align)

	if align < 4 {
		t.Fatalf("expected alignment >= 4, got %d", align)
	}
}

func BenchmarkUnifiedFlow_Serialization(b *testing.B) {
	f := New().
		SetL3("192.168.1.1", "10.0.0.1").
		SetL4(12345, 80, ProtoTCP, 0x02).
		SetL7(ProtoHTTP, 1, "/api/v1/users", 200).
		SetProcess(1234, "nginx", "nginx").
		SetK8s("web-pod-abc", "default", "web-deploy", "web-svc", "node-1").
		SetTrace("0123456789abcdef0123456789abcdef", "0102030405060708090a0b0c0d0e0f10", "").
		SetTenant("tenant-abc").
		SetMetrics(1024, 10, 5000000, DirIngress)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.Serialize()
	}
}

func BenchmarkUnifiedFlow_Deserialization(b *testing.B) {
	f := New().
		SetL3("192.168.1.1", "10.0.0.1").
		SetL4(12345, 80, ProtoTCP, 0x02).
		SetMetrics(1024, 10, 5000000, DirIngress)

	data := f.Serialize()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f2 := &UnifiedFlow{}
		f2.Deserialize(data)
	}
}

func BenchmarkConverter_ParsedFlowToUnified(b *testing.B) {
	c := NewConverter()
	parsed := &pool.ParsedFlow{
		SrcIP:     [4]byte{192, 168, 1, 100},
		DstIP:     [4]byte{10, 0, 0, 1},
		SrcPort:   54321,
		DstPort:   80,
		Protocol:  6,
		Bytes:     1024,
		Packets:   10,
		LatencyNs: 5000000,
	}
	raw := &pool.RawEvent{Type: pool.EventTypeTCP}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.ParsedFlowToUnified(raw, parsed)
	}
}

func BenchmarkConverter_UnifiedToMetricData(b *testing.B) {
	c := NewConverter()
	f := New().
		SetL3("192.168.1.1", "10.0.0.1").
		SetL4(80, 443, ProtoTCP, 0).
		SetMetrics(2048, 20, 5000000, DirIngress).
		SetK8s("pod-1", "default", "deploy-1", "svc-1", "node-1")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.UnifiedToMetricData(f)
	}
}

// 验证 unsafe.Sizeof 编译通过
var _ = unsafe.Sizeof(UnifiedFlow{})
